package testnet

import (
	"bytes"
	"encoding/json"
	"hashburst/blockchain"
	"os"
	"path/filepath"
	"testing"
)

func apowMigrationFixture(t *testing.T) (Config, Config, string, string) {
	_, old, p, q := migrationFixture(t)
	if e := MigrateEVM(p, q); e != nil {
		t.Fatal(e)
	}
	next := old
	next.Protocol.APoW = &blockchain.APoWConfig{ActivationHeight: 200000, InitialBits: 8, MinBits: 4, MaxBits: 24, Window: 32, TargetSeconds: 5, GasLimit: 2000000}
	data, _ := json.Marshal(next)
	if e := os.WriteFile(q, data, 0600); e != nil {
		t.Fatal(e)
	}
	return old, next, p, q
}
func TestAPoWMigrationResumesWithoutStateOrIdentityChanges(t *testing.T) {
	old, next, p, q := apowMigrationFixture(t)
	before := map[string][]byte{}
	for _, name := range []string{"blockchain.dat", "blockchain.idx", "consensus-votes.jsonl", "consensus-bft-signatures.jsonl"} {
		b, e := os.ReadFile(filepath.Join(old.DataDir, name))
		if e != nil {
			t.Fatal(e)
		}
		before[name] = b
	}
	if e := MigrateAPoW(p, q); e != nil {
		t.Fatal(e)
	}
	if e := MigrateAPoW(p, q); e != nil {
		t.Fatal(e)
	}
	// Simulate interruption after pin replacement and before config replacement.
	data, _ := json.Marshal(old)
	os.WriteFile(p, data, 0600)
	if e := MigrateAPoW(p, q); e != nil {
		t.Fatal("resume", e)
	}
	for name, want := range before {
		got, e := os.ReadFile(filepath.Join(old.DataDir, name))
		if e != nil || !bytes.Equal(got, want) {
			t.Fatal("state changed", name, e)
		}
	}
	got, e := Load(p)
	if e != nil || got.Pin() != next.Pin() {
		t.Fatal("candidate not installed", e)
	}
	state, e := Prepare(next, false)
	if e != nil {
		t.Fatal(e)
	}
	state.Close()
	next.Protocol.APoW.GasLimit++
	data, _ = json.Marshal(next)
	os.WriteFile(q, data, 0600)
	if MigrateAPoW(p, q) == nil {
		t.Fatal("different candidate accepted")
	}
}
func TestAPoWMigrationRejectsRulesAndLiveState(t *testing.T) {
	old, next, p, q := apowMigrationFixture(t)
	bad := next
	bad.Protocol.EVM = &blockchain.EVMConfig{ActivationHeight: old.Protocol.EVM.ActivationHeight, GasLimit: 2000000, BaseFeeWei: 1}
	if validateAPoWTransition(old, bad) == nil {
		t.Fatal("retroactive gas change accepted")
	}
	bad = next
	bad.NodeID = "changed"
	if validateAPoWTransition(old, bad) == nil {
		t.Fatal("identity changed")
	}
	bad = next
	bad.Protocol.ChainID = 4735489
	if validateAPoWTransition(old, bad) == nil {
		t.Fatal("mainnet accepted")
	}
	state, e := Prepare(old, false)
	if e != nil {
		t.Fatal(e)
	}
	defer state.Close()
	if MigrateAPoW(p, q) == nil {
		t.Fatal("live state migrated")
	}
}
func TestAPoWMigrationRefusesNearActivation(t *testing.T) {
	old := fixture(t)
	old.Protocol.ChainID = 4735490
	old.Protocol.ActivationHeight = 6
	old.Protocol.ConsensusActivationHeight = 8
	old.Protocol.EVM = &blockchain.EVMConfig{ActivationHeight: 8, GasLimit: 200000, BaseFeeWei: 1}
	state, e := Prepare(old, true)
	if e != nil {
		t.Fatal(e)
	}
	state.Close()
	next := old
	next.Protocol.APoW = &blockchain.APoWConfig{ActivationHeight: 1000, InitialBits: 4, MinBits: 1, MaxBits: 16, Window: 4, TargetSeconds: 5}
	p, q := filepath.Join(t.TempDir(), "node.json"), filepath.Join(t.TempDir(), "next.json")
	for path, c := range map[string]Config{p: old, q: next} {
		data, _ := json.Marshal(c)
		os.WriteFile(path, data, 0600)
	}
	if MigrateAPoW(p, q) == nil {
		t.Fatal("insufficient margin accepted")
	}
	pin, _ := os.ReadFile(filepath.Join(old.DataDir, "runtime.pin"))
	if !bytes.Equal(pin, diskPin(old)) {
		t.Fatal("pin mutated")
	}
}
