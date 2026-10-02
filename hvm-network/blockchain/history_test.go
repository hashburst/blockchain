package blockchain

import (
	"bytes"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	execution "hashburst/evm-execution"
	"hashburst/ledger"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func testHistoryBlock(t *testing.T, bc *Blockchain, n int) *Block {
	t.Helper()
	b, e := bc.BlockAt(n)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestIndexedHistoryFourValidatorsAndObserver(t *testing.T) {
	cfg := phase3CTestConfig()
	cfg.ChainID = execution.TestnetID
	cfg.EVM = &EVMConfig{ActivationHeight: 7, GasLimit: 1000000, BaseFeeWei: 1}
	s := setupPhase3DChainsWithConfig(t, cfg)
	for _, n := range s.nodes {
		n.SetMempool(NewMempool())
	}
	finalizeEVMFixture(t, s)
	genesis := testHistoryBlock(t, s.nodes[0], 0).Hash
	pin := testHistoryBlock(t, s.nodes[0], 6).Hash
	prefixes := make([][]byte, 4)
	for i, n := range s.nodes {
		prefixes[i], _ = os.ReadFile(filepath.Join(n.storage.dir, "consensus-bft-signatures.jsonl"))
		opened, e := OpenExistingBlockchain(n.storage.dir, cfg, genesis, 6, pin)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { opened.CloseRecoveryWriter(); opened.CloseHistory() })
		if len(opened.Blocks) != 0 || opened.history == nil {
			t.Fatal("resident history retained")
		}
		opened.SetMempool(NewMempool())
		s.nodes[i] = opened
	}
	// Observer uses an independently converted binary generation and never signs.
	dir := filepath.Join(t.TempDir(), "observer")
	_, e := ledger.WriteGeneration(dir, uint64(s.nodes[0].Height()+1), func(n uint64) ([]byte, error) {
		b, e := s.nodes[0].BlockAt(int(n))
		if e != nil {
			return nil, e
		}
		return EncodeLedgerBlock(nil, b)
	})
	if e != nil {
		t.Fatal(e)
	}
	observer, e := OpenExistingBlockchain(dir, cfg, genesis, 6, pin)
	if e != nil {
		t.Fatal(e)
	}
	defer observer.CloseHistory()
	for round := 0; round < 3; round++ {
		b := finalizeEVMFixture(t, s)
		if _, e = observer.TryExtendOrAdopt([]*Block{b}); e != nil {
			t.Fatal(e)
		}
		for _, n := range append(s.nodes, observer) {
			if n.HeadSnapshot().Hash != b.Hash || n.state.Root() != s.nodes[0].state.Root() || n.state.evm.root != s.nodes[0].state.evm.root || len(n.Blocks) != 0 {
				t.Fatal("disk-backed node diverged")
			}
			api := &EthereumNodeAPI{bc: n}
			byNumber, e := api.GetBlockByNumber(rpc.BlockNumber(b.Index), false)
			if e != nil {
				t.Fatal(e)
			}
			byHash, e := api.GetBlockByHash(common.HexToHash(b.Hash), false)
			if e != nil || !reflect.DeepEqual(byHash, byNumber) {
				t.Fatal("RPC mismatch", e)
			}
			owned, e := n.BlockAt(b.Index)
			if e != nil {
				t.Fatal(e)
			}
			owned.Hash = "changed"
			if n.HeadSnapshot().Hash != b.Hash || testHistoryBlock(t, n, b.Index).Hash != b.Hash {
				t.Fatal("caller mutated history")
			}
		}
	}
	for i, n := range s.nodes {
		after, e := os.ReadFile(filepath.Join(n.storage.dir, "consensus-bft-signatures.jsonl"))
		if e != nil || !bytes.HasPrefix(after, prefixes[i]) || len(after) <= len(prefixes[i]) {
			t.Fatal("journal prefix/progress", e)
		}
	}
	signs, e := os.ReadFile(filepath.Join(dir, "consensus-bft-signatures.jsonl"))
	if e != nil && !os.IsNotExist(e) {
		t.Fatal(e)
	}
	if len(signs) != 0 {
		t.Fatal("observer signed")
	}
	reopened, e := OpenExistingBlockchain(dir, cfg, genesis, 6, pin)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.CloseHistory()
	if reopened.HeadSnapshot().Hash != observer.HeadSnapshot().Hash || reopened.state.evm.root != observer.state.evm.root {
		t.Fatal("append/reopen mismatch")
	}
}

func TestIndexedHistoryAncestorBoundary(t *testing.T) {
	bc := &Blockchain{v2Config: ProtocolV2Config{EVM: &EVMConfig{BaseFeeWei: 1, GasLimit: 1000000}}}
	for n := 0; n < 600; n++ {
		bc.Blocks = append(bc.Blocks, &Block{Index: n, Hash: fmt.Sprintf("%064x", n+1)})
	}
	w, e := bc.ancestorWindowLocked(599)
	if e != nil {
		t.Fatal(e)
	}
	if len(w) != 256 || w[0].Index != 343 {
		t.Fatal("unbounded ancestor window")
	}
	ctx := bc.ethereumContext(&Block{Index: 599}, w)
	for _, n := range []uint64{343, 400, 598} {
		if ctx.HashAt(n) != common.HexToHash(bc.Blocks[n].Hash) {
			t.Fatal("BLOCKHASH mismatch")
		}
	}
	for _, n := range []uint64{0, 342, 599, 600} {
		if ctx.HashAt(n) != (common.Hash{}) {
			t.Fatal("out-of-window hash")
		}
	}
}

func TestDurableHistoryWriteFailureRequiresReopen(t *testing.T) {
	s := NewChainStorage(t.TempDir())
	s.durable = true
	if e := s.SaveBlock(NewGenesisBlock()); e != nil {
		t.Fatal(e)
	}
	f, e := os.OpenFile(s.datPath, os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	f.Write([]byte{1, 2, 3})
	f.Close()
	before, e := os.ReadFile(s.datPath)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.SaveBlock(&Block{Index: 1}); e == nil {
		t.Fatal("accepted unindexed tail")
	}
	if e = s.SaveBlock(&Block{Index: 1}); e == nil {
		t.Fatal("retried poisoned storage")
	}
	after, _ := os.ReadFile(s.datPath)
	if !bytes.Equal(before, after) {
		t.Fatal("modified ambiguous durable state")
	}
	if e = s.Rewrite([]*Block{NewGenesisBlock()}); e == nil {
		t.Fatal("rewrote durable generation in place")
	}
}
