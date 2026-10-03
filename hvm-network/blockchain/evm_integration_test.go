package blockchain

import (
	"bytes"
	"context"
	"encoding/hex"
	"math/big"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/libp2p/go-libp2p/core/peer"
	"hashburst/consensus"
	execution "hashburst/evm-execution"
	"hashburst/protocolv2"
)

func finalizeEVMFixture(t *testing.T, s phase3DSetup) *Block {
	t.Helper()
	head := s.nodes[0].Height() + 1
	set := s.nodes[0].CurrentValidatorSet(uint64(head))
	proposer, _ := set.Proposer(uint64(head), 0)
	b, err := s.nodes[0].BuildConsensusProposal(proposer.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	votes := make([]consensus.Vote, 0, 4)
	for i, v := range s.vals {
		vote, e := s.nodes[i].SignConsensusPrecommit(b, v.id, v.consensus)
		if e != nil {
			t.Fatal(e)
		}
		votes = append(votes, vote)
	}
	qc, err := consensus.BuildQuorumCertificate(s.cfg.ChainID, uint64(head), 0, b.Hash, b.ValidatorSetRoot, set, votes)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range s.nodes {
		if err = node.FinalizeConsensusProposal(b, qc); err != nil {
			t.Fatal(err)
		}
	}
	for _, node := range s.nodes {
		if node.HeadSnapshot().Hash != b.Hash || node.state.evm.root != b.EVMStateRoot {
			t.Fatal("validator divergence")
		}
	}
	return s.nodes[0].HeadSnapshot()
}

func TestEVMRealMempoolFourValidatorsAndPersistentReplay(t *testing.T) {
	cfg := phase3CTestConfig()
	cfg.ChainID = execution.TestnetID
	cfg.EVM = &EVMConfig{ActivationHeight: 7, GasLimit: 1_000_000, BaseFeeWei: 1}
	s := setupPhase3DChainsWithConfig(t, cfg)
	for _, node := range s.nodes {
		node.SetMempool(NewMempool())
		node.storage.durable = true
	}
	originalGenesis := testHistoryBlock(t,s.nodes[0],0).Hash
	finalizeEVMFixture(t, s) // explicit activation, no public data or genesis touched
	node := s.nodes[0]
	sender := common.HexToAddress(s.vals[0].operator.Address())
	key, err := crypto.ToECDSA(s.vals[0].operator.PrivateKeyBytes())
	if err != nil {
		t.Fatal(err)
	}
	peers := make([]*P2PNode, 4)
	syncers := make([]*Syncer, 4)
	for i, n := range s.nodes {
		p, e := NewP2PNodeWithListenIP(n, n.mempool, "127.0.0.1", 0, nil)
		if e != nil {
			t.Fatal(e)
		}
		peers[i] = p
		defer p.Host.Close()
		syncers[i] = NewSyncer(n, n.mempool, p.Host)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for i := 1; i < 4; i++ {
		if e := peers[0].Host.Connect(ctx, peer.AddrInfo{ID: peers[i].Host.ID(), Addrs: peers[i].Host.Addrs()}); e != nil {
			t.Fatal(e)
		}
	}
	rpcServer, err := node.NewEthereumRPC(syncers[0].GossipEthereum)
	if err != nil {
		t.Fatal(err)
	}
	defer rpcServer.Stop()
	server := httptest.NewServer(rpcServer)
	defer server.Close()
	client, err := rpc.DialHTTP(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	wsServer := httptest.NewServer(rpcServer.WebsocketHandler([]string{"http://localhost"}))
	defer wsServer.Close()
	ws, err := rpc.DialWebsocket(context.Background(), "ws"+strings.TrimPrefix(wsServer.URL, "http"), "http://localhost")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	heads := make(chan map[string]any, 16)
	headSub, err := ws.EthSubscribe(context.Background(), heads, "newHeads")
	if err != nil {
		t.Fatal(err)
	}
	defer headSub.Unsubscribe()
	sign := func(to *common.Address, data []byte, value int64, gas uint64) ([]byte, common.Hash) {
		n := node.state.Sequence(sender.Hex())
		tx := types.NewTx(&types.DynamicFeeTx{ChainID: new(big.Int).SetUint64(cfg.ChainID), Nonce: n, To: to, Value: big.NewInt(value), Gas: gas, GasFeeCap: big.NewInt(3), GasTipCap: big.NewInt(1), Data: data})
		tx, e := types.SignTx(tx, types.NewCancunSigner(new(big.Int).SetUint64(cfg.ChainID)), key)
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := tx.MarshalBinary()
		return raw, tx.Hash()
	}
	submit := func(raw []byte, want common.Hash) {
		var got common.Hash
		if e := client.CallContext(context.Background(), &got, "eth_sendRawTransaction", hexutil.Bytes(raw)); e != nil {
			t.Fatal(e)
		}
		if got != want {
			t.Fatal("hash changed")
		}
		if node.mempool.Size() != 1 {
			t.Fatal("not in real mempool")
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			ready := true
			for _, n := range s.nodes {
				ready = ready && n.mempool.Size() == 1
			}
			if ready {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("Ethereum gossip did not reach four mempools")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	to := common.HexToAddress("0x0000000000000000000000000000000000001234")
	value := int64(123456789123)
	before := node.state.evm.db.GetBalance(sender).ToBig()
	raw, hash := sign(&to, nil, value, 21000)
	submit(raw, hash)
	if node.state.evm.db.GetBalance(to).Sign() != 0 {
		t.Fatal("RPC admission changed canonical balance")
	}
	b := finalizeEVMFixture(t, s)
	select {
	case event := <-heads:
		if event["hash"] != common.HexToHash(b.Hash).Hex() {
			t.Fatal("WebSocket used wrong finalized hash")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing finalized head notification")
	}
	if node.mempool.Size() != 0 {
		t.Fatal("finalized transaction retained")
	}
	for _, n := range s.nodes {
		if n.state.evm.db.GetBalance(to).ToBig().Cmp(big.NewInt(value)) != 0 || n.state.BalanceUnits(to.Hex()) != 12 {
			t.Fatal("native/wei accounting mismatch")
		}
	}
	want := new(big.Int).Sub(before, big.NewInt(value+42000))
	if node.state.evm.db.GetBalance(sender).ToBig().Cmp(want) != 0 {
		t.Fatal("sender fee mismatch")
	}
	var receipt map[string]any
	if err = client.Call(&receipt, "eth_getTransactionReceipt", hash); err != nil {
		t.Fatal(err)
	}
	if receipt["blockHash"] != common.HexToHash(b.Hash).Hex() || receipt["status"] != "0x1" {
		t.Fatalf("receipt not tied to HashBurst finalized block: %v", receipt)
	}
	if _, err = node.AdmitEthereum(raw); err == nil {
		t.Fatal("replay admitted")
	}
	// Real deployment and storage/log mutation, each finalized by all validators.
	runtime, _ := hex.DecodeString("602a60005560006000a000")
	init := append([]byte{0x60, byte(len(runtime)), 0x60, 12, 0x60, 0, 0x39, 0x60, byte(len(runtime)), 0x60, 0, 0xf3}, runtime...)
	nonce := node.state.Sequence(sender.Hex())
	contract := crypto.CreateAddress(sender, nonce)
	raw, hash = sign(nil, init, 0, 200000)
	submit(raw, hash)
	finalizeEVMFixture(t, s)
	logs := make(chan *types.Log, 4)
	logSub, err := ws.EthSubscribe(context.Background(), logs, "logs", map[string]any{"address": contract.Hex()})
	if err != nil {
		t.Fatal(err)
	}
	defer logSub.Unsubscribe()
	raw, hash = sign(&contract, nil, 0, 100000)
	submit(raw, hash)
	finalizeEVMFixture(t, s)
	select {
	case event := <-logs:
		if event.TxHash != hash || event.BlockHash != common.HexToHash(node.HeadSnapshot().Hash) {
			t.Fatal("WebSocket log mismatch")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing finalized log")
	}
	r, _ := node.ethereumReceiptLocked(hash)
	if r == nil || len(r.Logs) != 1 || node.state.evm.db.GetState(contract, common.Hash{}) != common.HexToHash("0x2a") {
		t.Fatal("contract execution missing")
	}
	wire := blockToWire(node.HeadSnapshot()).toBlock()
	if wire.GenerateHash() != node.HeadSnapshot().Hash {
		t.Fatal("wire lost EVM payload")
	}
	// Wallet-facing queries use the same finalized block, receipt and log index.
	var blockResult map[string]any
	if err = client.Call(&blockResult, "eth_getBlockByHash", common.HexToHash(node.HeadSnapshot().Hash), true); err != nil {
		t.Fatal(err)
	}
	if blockResult["hash"] != common.HexToHash(node.HeadSnapshot().Hash).Hex() {
		t.Fatal("block API mismatch")
	}
	var txResult map[string]any
	if err = client.Call(&txResult, "eth_getTransactionByHash", hash); err != nil {
		t.Fatal(err)
	}
	if txResult["hash"] != hash.Hex() || txResult["from"] != strings.ToLower(sender.Hex()) {
		t.Fatalf("transaction metadata: %v", txResult)
	}
	var history map[string]any
	if err = client.Call(&history, "eth_feeHistory", "0x2", "latest", []float64{0, 50, 100}); err != nil {
		t.Fatal(err)
	}
	if len(history["baseFeePerGas"].([]any)) != 3 {
		t.Fatal("fee history length")
	}
	var queriedLogs []types.Log
	if err = client.Call(&queriedLogs, "eth_getLogs", map[string]any{"blockHash": common.HexToHash(node.HeadSnapshot().Hash), "address": contract}); err != nil {
		t.Fatal(err)
	}
	if len(queriedLogs) != 1 || queriedLogs[0].TxHash != hash {
		t.Fatal("historical logs mismatch")
	}
	// A native transfer after Ethereum execution spends the SAME account, retaining dust.
	beforeNative := node.state.evm.db.GetBalance(sender).ToBig()
	native := protocolv2.NewTransactionV2(cfg.ChainID, protocolv2.TxHBTTransfer, sender.Hex(), to.Hex(), 1, node.state.Sequence(sender.Hex()), 21000, 1000, nil)
	if err = native.Sign(s.vals[0].operator); err != nil {
		t.Fatal(err)
	}
	if err = node.AdmitTransactionV2(native); err != nil {
		t.Fatal(err)
	}
	nativeBlock := finalizeEVMFixture(t, s)
	nativeReceipt := node.receipts[strings.ToLower(strings.TrimPrefix(native.HashHex(), "0x"))]
	delta, _ := execution.NativeToWei(1 + nativeReceipt.FeeUnits)
	if node.state.evm.db.GetBalance(sender).ToBig().Cmp(new(big.Int).Sub(beforeNative, delta)) != 0 {
		t.Fatal("native and Ethereum ledger diverged")
	}
	if nativeBlock.EVMStateRoot == "" {
		t.Fatal("native effect absent from EVM commitment")
	}
	value += execution.WeiPerNativeUnit
	// Reopen only from the persisted native chain and preserved signing journal.
	restarted := s.nodes[3]
	journal := filepath.Join(restarted.storage.dir, "consensus-bft-signatures.jsonl")
	prefix, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := OpenExistingBlockchain(restarted.storage.dir, cfg, originalGenesis, 6, testHistoryBlock(t,restarted,6).Hash)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.state.evm.root != node.state.evm.root || fresh.state.Root() != node.state.Root() {
		t.Fatal("replay changed roots")
	}
	if fresh.state.evm.db.GetBalance(to).ToBig().Cmp(big.NewInt(value)) != 0 || fresh.state.evm.db.GetState(contract, common.Hash{}) != common.HexToHash("0x2a") {
		t.Fatal("replay lost dust/storage")
	}
	rr, _ := fresh.ethereumReceiptLocked(hash)
	if rr == nil || rr.TxHash != hash || len(rr.Logs) != 1 {
		t.Fatal("replay lost receipts")
	}
	after, _ := os.ReadFile(journal)
	if !bytes.Equal(prefix, after) {
		t.Fatal("replay changed signing journal")
	}
	fresh.SetMempool(NewMempool())
	s.nodes[3] = fresh
	finalizeEVMFixture(t, s)
	after, _ = os.ReadFile(journal)
	if !bytes.HasPrefix(after, prefix) || len(after) <= len(prefix) {
		t.Fatal("restart signing prefix not preserved")
	}
	observer := NewBlockchainWithDirAndV2Config(t.TempDir(), cfg)
	for _, block := range node.Blocks[1:] {
		if err = observer.AppendBlock(blockToWire(block).toBlock()); err != nil {
			t.Fatal(err)
		}
	}
	if observer.state.evm.root != node.state.evm.root || observer.HeadSnapshot().Hash != node.HeadSnapshot().Hash {
		t.Fatal("observer divergence")
	}
	unsigned, err := os.ReadFile(filepath.Join(observer.storage.dir, "consensus-bft-signatures.jsonl"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(unsigned) != 0 {
		t.Fatal("observer signed")
	}
	t.Log("EVM_HASHBURST_MEMPOOL_ACCOUNTING_FOUR_VALIDATORS_REPLAY_WS_OBSERVER_OK")
}

func TestEVMActivationAndTamperGuards(t *testing.T) {
	cfg := phase3CTestConfig()
	cfg.ChainID = execution.TestnetID
	cfg.EVM = &EVMConfig{ActivationHeight: 7, GasLimit: 1_000_000, BaseFeeWei: 1}
	s := setupPhase3DChainsWithConfig(t, cfg)
	node := s.nodes[0]
	if err := validateEVMEnvelope(&Block{Index: 6, EthereumTransactions: [][]byte{{1}}}, cfg); err == nil {
		t.Fatal("preactivation accepted")
	}
	proposer, _ := node.CurrentValidatorSet(7).Proposer(7, 0)
	b, err := node.BuildConsensusProposal(proposer.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	b.EVMStateRoot = strings.Repeat("1", 64)
	b.Hash = b.GenerateHash()
	if err = node.ValidateConsensusProposalForVote(b); err == nil {
		t.Fatal("forged root accepted")
	}
	if node.Height() != 6 {
		t.Fatal("speculation changed canonical chain")
	}
	legacy := DefaultProtocolV2Config()
	legacy.EVM = cfg.EVM
	if legacy.Validate() == nil {
		t.Fatal("legacy EVM activation accepted")
	}
}
