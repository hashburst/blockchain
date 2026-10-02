package blockchain

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	execution "hashburst/evm-execution"
	"hashburst/ledger"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBinaryStorageConvertedRuntimeReplay(t *testing.T) {
	cfg := phase3CTestConfig()
	cfg.ChainID = execution.TestnetID
	cfg.EVM = &EVMConfig{ActivationHeight: 7, GasLimit: 1000000, BaseFeeWei: 1}
	s := setupPhase3DChainsWithConfig(t, cfg)
	n := s.nodes[0]
	for _, node := range s.nodes {
		node.SetMempool(NewMempool())
	}
	finalizeEVMFixture(t, s)
	target := filepath.Join(t.TempDir(), "generation")
	_, err := ledger.WriteGeneration(target, uint64(len(n.Blocks)), func(i uint64) ([]byte, error) { return EncodeLedgerBlock(nil, n.Blocks[i]) })
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"consensus-votes.jsonl", "consensus-bft-signatures.jsonl"} {
		b, e := os.ReadFile(filepath.Join(n.storage.dir, name))
		if os.IsNotExist(e) {
			b = nil
		} else if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(target, name), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	restored, err := OpenExistingBlockchain(target, n.v2Config, testHistoryBlock(t,n,0).Hash, 6, testHistoryBlock(t,n,6).Hash)
	if err != nil {
		t.Fatal(err)
	}
	if restored.HeadSnapshot().Hash != n.HeadSnapshot().Hash || restored.state.Root() != n.state.Root() || restored.HVMStateRoot() != n.HVMStateRoot() || restored.validators.Root() != n.validators.Root() {
		t.Fatal("converted runtime differs")
	}
	if !restored.storage.binaryPayload {
		t.Fatal("format not retained")
	}
}

func TestBinaryStorageAppendAndCorruption(t *testing.T) {
	var b Block
	fillLedgerFixture(reflect.ValueOf(&b).Elem())
	b.Index = 0
	dir := filepath.Join(t.TempDir(), "generation")
	_, err := ledger.WriteGeneration(dir, 1, func(uint64) ([]byte, error) { return EncodeLedgerBlock(nil, &b) })
	if err != nil {
		t.Fatal(err)
	}
	st := NewChainStorage(dir)
	st.durable = true
	b.Index = 1
	if err = st.SaveBlock(&b); err != nil {
		t.Fatal(err)
	}
	blocks, err := st.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || !reflect.DeepEqual(*blocks[1], b) {
		t.Fatal("append roundtrip")
	}
	if err = verifyPersistentIndex(st.datPath, st.idxPath); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(st.datPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte("HBX2")) {
		t.Fatal("format changed")
	}
	raw[len(raw)-1] ^= 1
	if err = os.WriteFile(st.datPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = st.LoadAll(); err == nil {
		t.Fatal("corruption accepted")
	}
}

func TestRuntimeAsyncCheckpointFrozenAndDrained(t *testing.T) {
	cfg := phase3CTestConfig()
	cfg.ChainID = execution.TestnetID
	cfg.EVM = &EVMConfig{ActivationHeight: 7, GasLimit: 1000000, BaseFeeWei: 1}
	s := setupPhase3DChainsWithConfig(t, cfg)
	n := s.nodes[0]
	for _, node := range s.nodes {
		node.SetMempool(NewMempool())
	}
	finalizeEVMFixture(t, s)
	n.storage.durable = true
	if err := os.WriteFile(filepath.Join(n.storage.dir, "runtime.pin"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	height := n.Height()
	if err := n.saveRecoveryCheckpoint(height, n.state, n.hvmEngine, n.validators, n.receipts); err != nil {
		t.Fatal(err)
	}
	path := n.checkpointPath((height / checkpointInterval) % 2)
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	n.checkpointAsync = true
	if err = n.saveRecoveryCheckpoint(height, n.state, n.hvmEngine, n.validators, n.receipts); err != nil {
		t.Fatal(err)
	}
	n.checkpointAsync = false
	// Mutating live state after submission must not race with or alter the snapshot.
	n.state.balances["new-value-after-snapshot"] = 77
	n.CloseRecoveryWriter()
	n.CloseRecoveryWriter()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decode := func(b []byte) recoveryCheckpoint {
		t.Helper()
		z, e := gzip.NewReader(bytes.NewReader(b[32:]))
		if e != nil {
			t.Fatal(e)
		}
		defer z.Close()
		var cp recoveryCheckpoint
		if e = gob.NewDecoder(z).Decode(&cp); e != nil {
			t.Fatal(e)
		}
		return cp
	}
	before, after := decode(expected), decode(got)
	if !reflect.DeepEqual(before.Balances, after.Balances) || !reflect.DeepEqual(before.Sequences, after.Sequences) || before.Hash != after.Hash || before.PrefixHash != after.PrefixHash || before.EVM.Root != after.EVM.Root {
		t.Fatal("async snapshot changed or was not drained")
	}
}
