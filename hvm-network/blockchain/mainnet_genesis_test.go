package blockchain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func importFixtureConfig() ProtocolV2Config {
	c := phase3CTestConfig()
	c.ChainID = MainnetChainID
	m := ApprovedMainnetGenesisImport()
	c.GenesisImport = &m
	return c
}
func TestMainnetImportAtomicAndSingleUse(t *testing.T) {
	s := NewState()
	m := ApprovedMainnetGenesisImport()
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.applyGenesisImport(MainnetChainID, m) == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatal("import not single-use")
	}
	if s.BalanceUnits(m.Recipient) != legacyImportUnits+founderAllocationUnits {
		t.Fatal("supply mismatch")
	}
	before := s.Root()
	copy := s.Clone()
	if copy.Root() != before {
		t.Fatal("clone lost receipt")
	}
	if copy.applyGenesisImport(MainnetChainID, m) == nil || copy.Root() != before {
		t.Fatal("repeat changed state")
	}
	restored := NewState()
	restored.ReplaceWith(copy)
	if restored.Root() != before {
		t.Fatal("replace lost receipt")
	}
	delete(copy.consumedImports, m.Nullifier())
	if copy.Root() == before {
		t.Fatal("receipt outside root")
	}
	bad := m
	bad.Units++
	empty := NewState()
	root := empty.Root()
	if empty.applyGenesisImport(MainnetChainID, bad) == nil || empty.Root() != root {
		t.Fatal("invalid import mutated state")
	}
	if empty.applyGenesisImport(4735490, m) == nil || empty.Root() != root {
		t.Fatal("cross chain import")
	}
}
func TestMainnetImportGenesisReopen(t *testing.T) {
	c := importFixtureConfig()
	dir := t.TempDir()
	n := NewBlockchainWithDirAndV2Config(dir, c)
	g := n.HeadSnapshot()
	if g.Hash == NewGenesisBlock().Hash {
		t.Fatal("genesis domain not separated")
	}
	for i := 0; i < 2; i++ {
		b, e := OpenExistingBlockchain(dir, c, g.Hash, 0, g.Hash)
		if e != nil {
			t.Fatal(e)
		}
		if b.state.Root() != n.state.Root() {
			t.Fatal("replay root mismatch")
		}
		if e = b.state.applyGenesisImport(MainnetChainID, *c.GenesisImport); e == nil {
			t.Fatal("reimport after restart")
		}
		b.CloseHistory()
	}
	wrong := c.detached()
	wrong.GenesisImport = nil
	if _, e := OpenExistingBlockchain(dir, wrong, g.Hash, 0, g.Hash); e == nil {
		t.Fatal("import configuration removed")
	}
	bad := cloneBlockForConsensus(g)
	bad.HBTStateRoot = "00"
	if validProtocolGenesis(bad, c) {
		t.Fatal("tampered genesis")
	}
}
func TestMainnetImportCheckpointAndFallback(t *testing.T) {
	// Synthetic consensus identities/parameters: this is not a production manifest.
	cfg := importFixtureConfig()
	cfg.EVM = &EVMConfig{ActivationHeight: 7, GasLimit: 1000000, BaseFeeWei: 1}
	s := setupPhase3DChainsWithConfig(t, cfg)
	n := s.nodes[0]
	n.storage.durable = true
	if e := os.WriteFile(filepath.Join(n.storage.dir, "runtime.pin"), []byte("isolated-mainnet-fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	finalizeEVMFixture(t, s)
	height := n.Height()
	if e := n.saveRecoveryCheckpoint(height, n.state, n.hvmEngine, n.validators, n.receipts); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 257; i++ {
		finalizeEVMFixture(t, s)
	}
	open := func() *Blockchain {
		b, e := OpenExistingBlockchainWithRecovery(n.storage.dir, n.v2Config, n.Blocks[0].Hash, 6, n.Blocks[6].Hash)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { b.CloseRecoveryWriter(); b.CloseHistory() })
		return b
	}
	b := open()
	if b.RecoveryStatus().Mode != "incremental" {
		t.Fatalf("checkpoint not used: %+v", b.RecoveryStatus())
	}
	if b.state.Root() != n.state.Root() {
		t.Fatal("checkpoint changed root")
	}
	if b.state.applyGenesisImport(MainnetChainID, *s.cfg.GenesisImport) == nil {
		t.Fatal("checkpoint permitted reimport")
	}
	for slot := 0; slot < 2; slot++ {
		p := n.checkpointPath(slot)
		if data, e := os.ReadFile(p); e == nil {
			data[len(data)/2] ^= 1
			if e = os.WriteFile(p, data, 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	full := open()
	if full.RecoveryStatus().Mode != "full" || full.state.Root() != n.state.Root() {
		t.Fatal("fallback mismatch")
	}
	if full.state.applyGenesisImport(MainnetChainID, *s.cfg.GenesisImport) == nil {
		t.Fatal("fallback permitted reimport")
	}
	cp := recoveryCheckpoint{Balances: n.state.balances, Sequences: n.state.sequences, ConsumedImports: n.state.consumedImports}
	data, e := json.Marshal(cp)
	if e != nil {
		t.Fatal(e)
	}
	var decoded recoveryCheckpoint
	if e = json.Unmarshal(data, &decoded); e != nil {
		t.Fatal(e)
	}
	recovered := NewState()
	recovered.balances = decoded.Balances
	recovered.sequences = decoded.Sequences
	recovered.consumedImports = decoded.ConsumedImports
	if recovered.Root() != n.state.Root() {
		t.Fatal("checkpoint serialization lost receipt")
	}
}
