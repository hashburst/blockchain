package main

import (
	"encoding/json"
	"hashburst/blockchain"
	"os"
	"path/filepath"
	"testing"
)

func fixtureProtocol(t *testing.T) blockchain.ProtocolV2Config {
	t.Helper()
	c := blockchain.DefaultProtocolV2Config()
	c.ChainID = blockchain.MainnetChainID
	m := blockchain.ApprovedMainnetGenesisImport()
	c.GenesisImport = &m
	return c
}
func TestPublishEconomicCheckpointAndRefuseOverwrite(t *testing.T) {
	c := fixtureProtocol(t)
	g, e := blockchain.MainnetEconomicGenesis(c)
	if e != nil {
		t.Fatal(e)
	}
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(dir, "candidate")
	if e = publishCheckpoint(c, g, out); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Join(out, "CHECKPOINT.json"))
	if e != nil {
		t.Fatal(e)
	}
	var m map[string]any
	if e = json.Unmarshal(raw, &m); e != nil {
		t.Fatal(e)
	}
	if m["activation_allowed"] != false || m["production_import_executed"] != false {
		t.Fatal("economic checkpoint claimed activation")
	}
	before, e := os.ReadFile(filepath.Join(out, "ledger", "blockchain.dat"))
	if e != nil {
		t.Fatal(e)
	}
	if publishCheckpoint(c, g, out) == nil {
		t.Fatal("overwrote existing generation")
	}
	after, e := os.ReadFile(filepath.Join(out, "ledger", "blockchain.dat"))
	if e != nil || string(before) != string(after) {
		t.Fatal("existing generation changed")
	}
	b, e := blockchain.OpenExistingBlockchain(filepath.Join(out, "ledger"), c, g.Hash, 0, g.Hash)
	if e != nil {
		t.Fatal(e)
	}
	defer b.CloseHistory()
	if b.HBTStateRoot() != g.HBTStateRoot || b.BalanceUnits(c.GenesisImport.Recipient) != 100000045000000000 {
		t.Fatal("economic reconciliation mismatch")
	}
}
func TestPreparationRejectsDifferentFreezeBeforeOutput(t *testing.T) {
	c := fixtureProtocol(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "protocol.json")
	f := filepath.Join(dir, "freeze.json")
	out := filepath.Join(dir, "out")
	if e := writeJSON(p, c); e != nil {
		t.Fatal(e)
	}
	if e := writeJSON(f, map[string]bool{"fleet_freeze_verified": true}); e != nil {
		t.Fatal(e)
	}
	if prepare(p, f, filepath.Join(dir, "no-source"), out) == nil {
		t.Fatal("accepted fabricated report")
	}
	if _, e := os.Stat(out); !os.IsNotExist(e) {
		t.Fatal("output created before verification")
	}
}
