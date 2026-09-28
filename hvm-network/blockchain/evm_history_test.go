package blockchain

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/holiman/uint256"
	execution "hashburst/evm-execution"
)

func TestEVMHistoricalReadsAcrossFinalityAndRestart(t *testing.T) {
	cfg := phase3CTestConfig()
	cfg.ChainID = execution.TestnetID
	cfg.EVM = &EVMConfig{ActivationHeight: 7, GasLimit: 1_000_000, BaseFeeWei: 1}
	s := setupPhase3DChainsWithConfig(t, cfg)
	for _, n := range s.nodes {
		n.SetMempool(NewMempool())
		n.storage.durable = true
	}
	node := s.nodes[0]
	sender := common.HexToAddress(s.vals[0].operator.Address())
	key, err := crypto.ToECDSA(s.vals[0].operator.PrivateKeyBytes())
	if err != nil {
		t.Fatal(err)
	}
	contract := crypto.CreateAddress(sender, node.state.Sequence(sender.Hex()))
	finalizeEVMFixture(t, s)
	type query struct {
		method string
		params []any
		want   json.RawMessage
	}
	var queries []query
	clientFor := func(n *Blockchain) *rpc.Client {
		srv, e := n.NewEthereumRPC(nil)
		if e != nil {
			t.Fatal(e)
		}
		http := httptest.NewServer(srv)
		t.Cleanup(http.Close)
		t.Cleanup(srv.Stop)
		c, e := rpc.DialHTTP(http.URL)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(c.Close)
		return c
	}
	client := clientFor(node)
	capture := func(height int) {
		tag := hexutil.EncodeUint64(uint64(height))
		call := map[string]any{"from": sender, "to": contract}
		for _, q := range []query{
			{method: "eth_getBalance", params: []any{sender, tag}},
			{method: "eth_getTransactionCount", params: []any{sender, tag}},
			{method: "eth_getCode", params: []any{contract, tag}},
			{method: "eth_getStorageAt", params: []any{contract, "0x0", tag}},
			{method: "eth_call", params: []any{call, tag}},
			{method: "eth_estimateGas", params: []any{call, tag}},
		} {
			if e := client.Call(&q.want, q.method, q.params...); e != nil {
				t.Fatalf("%s at %s: %v", q.method, tag, e)
			}
			queries = append(queries, q)
		}
	}
	capture(7)
	runtime, _ := hex.DecodeString("60005460005260206000f3") // return slot zero
	init := append([]byte{0x60, byte(len(runtime)), 0x60, 12, 0x60, 0, 0x39, 0x60, byte(len(runtime)), 0x60, 0, 0xf3}, runtime...)
	// Initialize storage in the constructor then return the runtime.
	prefix, _ := hex.DecodeString("602a600055")
	init[3] += byte(len(prefix))
	init = append(prefix, init...)
	tx := types.NewTx(&types.DynamicFeeTx{ChainID: new(big.Int).SetUint64(cfg.ChainID), Nonce: node.state.Sequence(sender.Hex()), Gas: 200000, GasFeeCap: big.NewInt(3), GasTipCap: big.NewInt(1), Data: init})
	tx, err = types.SignTx(tx, types.NewCancunSigner(new(big.Int).SetUint64(cfg.ChainID)), key)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := tx.MarshalBinary()
	for _, n := range s.nodes {
		if _, e := n.AdmitEthereum(raw); e != nil {
			t.Fatal(e)
		}
	}
	finalizeEVMFixture(t, s)
	capture(8)
	if node.state.evm.db.GetState(contract, common.Hash{}) != common.HexToHash("0x2a") {
		t.Fatal("fixture storage")
	}
	finalizeEVMFixture(t, s)
	// Queries must distinguish predeployment from postdeployment snapshots.
	if bytes.Equal(queries[0].want, queries[6].want) || bytes.Equal(queries[1].want, queries[7].want) || bytes.Equal(queries[2].want, queries[8].want) || bytes.Equal(queries[3].want, queries[9].want) || bytes.Equal(queries[4].want, queries[10].want) {
		t.Fatal("fixture must exercise changed balance, nonce, code, storage and call output")
	}
	check := func(c *rpc.Client) {
		for _, q := range queries {
			var got json.RawMessage
			if e := c.Call(&got, q.method, q.params...); e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(got, q.want) {
				t.Fatalf("%s %v: got %s want %s", q.method, q.params, got, q.want)
			}
		}
		for _, tag := range []string{"0x6", "0xffffffffffff", "earliest"} {
			var out any
			if e := c.Call(&out, "eth_getBalance", sender, tag); e == nil {
				t.Fatalf("accepted unavailable tag %s", tag)
			}
		}
		var gas hexutil.Uint64
		if e := c.Call(&gas, "eth_estimateGas", map[string]any{"from": sender, "to": sender}); e != nil || gas != 21000 {
			t.Fatalf("default pending estimate %d %v", gas, e)
		}
	}
	// Concurrent API clients read the same pinned snapshot while finality advances.
	var wg sync.WaitGroup
	failures := make(chan error, 4)
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for attempt := 0; attempt < 20; attempt++ {
				st, b, e := (&EthereumBackend{Chain: node}).Snapshot(context.Background(), 7)
				if e != nil {
					failures <- e
					return
				}
				if b.Number != 7 || len(st.GetCode(contract)) != 0 {
					failures <- fmt.Errorf("historical snapshot changed")
					return
				}
			}
		}()
	}
	finalizeEVMFixture(t, s)
	wg.Wait()
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	before := node.state.Root()
	check(client)
	// Speculative proposal execution must not install a historical snapshot.
	proposer, _ := node.CurrentValidatorSet(11).Proposer(11, 0)
	if _, e := node.BuildConsensusProposal(proposer.ID, 0); e != nil {
		t.Fatal(e)
	}
	if _, _, e := (&EthereumBackend{Chain: node}).Snapshot(context.Background(), 11); e == nil {
		t.Fatal("speculative height exposed")
	}
	if node.state.Root() != before {
		t.Fatal("read or simulation mutated canonical state")
	}
	// A caller must not be able to mutate retained state through a returned copy.
	st, _, err := (&EthereumBackend{Chain: node}).Snapshot(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	st.SetBalance(sender, uint256.NewInt(0), tracing.BalanceChangeUnspecified)
	check(client)
	journal := filepath.Join(node.storage.dir, "consensus-bft-signatures.jsonl")
	prefixJournal, e := os.ReadFile(journal)
	if e != nil {
		t.Fatal(e)
	}
	fresh, e := OpenExistingBlockchain(node.storage.dir, cfg, node.Blocks[0].Hash, 6, node.Blocks[6].Hash)
	if e != nil {
		t.Fatal(e)
	}
	check(clientFor(fresh))
	after, e := os.ReadFile(journal)
	if e != nil || !bytes.Equal(prefixJournal, after) {
		t.Fatal("historical replay changed journal")
	}
	if fresh.state.Root() != before || fresh.state.evm.root != node.state.evm.root {
		t.Fatal("replay changed roots")
	}
}

func TestEVMHistoryBoundedAndHashBound(t *testing.T) {
	cfg := phase3CTestConfig()
	cfg.ChainID = execution.TestnetID
	cfg.EVM = &EVMConfig{ActivationHeight: 7, GasLimit: 1000000, BaseFeeWei: 1}
	s := setupPhase3DChainsWithConfig(t, cfg)
	finalizeEVMFixture(t, s)
	h := &evmReadHistory{}
	for i := 0; i < evmReadHistoryLimit+2; i++ {
		h.remember(&Block{Index: i, Hash: "canonical"}, s.nodes[0].state)
	}
	if _, e := h.snapshot(&Block{Index: 0, Hash: "canonical"}); e == nil {
		t.Fatal("eviction failed")
	}
	if _, e := h.snapshot(&Block{Index: evmReadHistoryLimit, Hash: "fork"}); e == nil {
		t.Fatal("wrong hash accepted")
	}
	if _, e := h.snapshot(&Block{Index: evmReadHistoryLimit, Hash: "canonical"}); e != nil {
		t.Fatal(e)
	}
}
