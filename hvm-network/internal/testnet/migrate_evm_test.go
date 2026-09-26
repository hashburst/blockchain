package testnet

import (
	"bytes"
	"encoding/json"
	"hashburst/blockchain"
	"os"
	"path/filepath"
	"testing"
)

func migrationFixture(t *testing.T) (Config, Config, string, string) {
	c := fixture(t)
	c.Protocol.ChainID = 4735490
	c.Protocol.ActivationHeight = 6
	c.Protocol.ConsensusActivationHeight = 8
	// Fixture genesis is chain-ID independent before activation.
	s, e := Prepare(c, true)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	next := c
	next.Protocol.EVM = &blockchain.EVMConfig{ActivationHeight: 100000, GasLimit: 1000000, BaseFeeWei: 1}
	p := filepath.Join(t.TempDir(), "node.json")
	q := filepath.Join(t.TempDir(), "next.json")
	for path, v := range map[string]Config{p: c, q: next} {
		b, _ := json.Marshal(v)
		os.WriteFile(path, b, 0600)
	}
	return c, next, p, q
}
func TestEVMMigrationPreservesStateAndResumes(t *testing.T) {
	c, next, p, q := migrationFixture(t)
	before := map[string][]byte{}
	for _, n := range []string{"blockchain.dat", "blockchain.idx", "consensus-votes.jsonl", "consensus-bft-signatures.jsonl"} {
		before[n], _ = os.ReadFile(filepath.Join(c.DataDir, n))
	}
	if e := MigrateEVM(p, q); e != nil {
		t.Fatal(e)
	}
	if e := MigrateEVM(p, q); e != nil {
		t.Fatal("repeat", e)
	}
	// Simulate crash after new pin but before new configuration rename.
	b, _ := json.Marshal(c)
	os.WriteFile(p, b, 0600)
	if e := MigrateEVM(p, q); e != nil {
		t.Fatal("resume", e)
	}
	for n, want := range before {
		got, _ := os.ReadFile(filepath.Join(c.DataDir, n))
		if !bytes.Equal(want, got) {
			t.Fatal("modified", n)
		}
	}
	s, e := Prepare(next, false)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
}
func TestEVMMigrationRefusesIdentityAndLiveState(t *testing.T) {
	c, next, p, q := migrationFixture(t)
	bad := next
	bad.NodeID = "changed"
	if validateEVMTransition(c, bad) == nil {
		t.Fatal("identity accepted")
	}
	bad = next
	bad.Protocol.ChainID = 4735489
	if validateEVMTransition(c, bad) == nil {
		t.Fatal("mainnet accepted")
	}
	s, e := Prepare(c, false)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if MigrateEVM(p, q) == nil {
		t.Fatal("live state migrated")
	}
}

func TestEVMMigrationRejectsNearHeightAndChangedIntent(t *testing.T) {
	c, next, p, q := migrationFixture(t)
	next.Protocol.EVM.ActivationHeight = 1000
	b, _ := json.Marshal(next)
	os.WriteFile(q, b, 0600)
	if MigrateEVM(p, q) == nil {
		t.Fatal("near activation accepted")
	}
	pin, _ := os.ReadFile(filepath.Join(c.DataDir, "runtime.pin"))
	if !bytes.Equal(pin, diskPin(c)) {
		t.Fatal("pin changed on refusal")
	}
	next.Protocol.EVM.ActivationHeight = 100000
	b, _ = json.Marshal(next)
	os.WriteFile(q, b, 0600)
	if e := MigrateEVM(p, q); e != nil {
		t.Fatal(e)
	}
	next.Protocol.EVM.GasLimit++
	b, _ = json.Marshal(next)
	os.WriteFile(q, b, 0600)
	if MigrateEVM(p, q) == nil {
		t.Fatal("changed intent accepted")
	}
}
