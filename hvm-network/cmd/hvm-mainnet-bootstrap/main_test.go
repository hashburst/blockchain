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
	if e = publishCheckpoint(c, g, out, ""); e != nil {
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
	if publishCheckpoint(c, g, out, "") == nil {
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
	if prepare(p, f, "", filepath.Join(dir, "no-source"), out) == nil {
		t.Fatal("accepted fabricated report")
	}
	if _, e := os.Stat(out); !os.IsNotExist(e) {
		t.Fatal("output created before verification")
	}
}

func TestRecoveryAttestationRejectsWithoutExactTerminalGeneration(t *testing.T) {
	c := fixtureProtocol(t)
	dir := t.TempDir()
	a := recoveryAttestation{}
	a.Schema = "hashburst-mainnet-legacy-freeze-reattestation-v1"
	a.Status = "recovery_attestation_not_original_evidence"
	a.LostEvidence.Filename = "FLEET-FREEZE-HA-1791220850167915000.json"
	a.LostEvidence.SHA256 = freezeHash
	a.LostEvidence.VerifierCommit = "0289779049a680a7f419be81d4d69504a9b97fbe"
	a.LostEvidence.Schema = "hashburst-managed-legacy-freeze-ha-v1"
	a.LostEvidence.Scope = "five managed legacy services at observation times"
	a.LostEvidence.RecoveryStatus = "original_bytes_not_found_after_controlled_coordinator_and_fleet_search"
	a.TerminalGeneration.SourceChainID = c.GenesisImport.SourceChainID
	a.TerminalGeneration.SourceHeight = c.GenesisImport.SourceHeight
	a.TerminalGeneration.SourceHash = c.GenesisImport.SourceHash
	a.TerminalGeneration.DatSHA256 = terminalDatHash
	a.TerminalGeneration.IdxSHA256 = terminalIdxHash
	a.TerminalGeneration.Recipient = c.GenesisImport.Recipient
	a.TerminalGeneration.Units = "45000000000"
	a.TerminalGeneration.AdditionalIssuance = "0"
	a.Guarantees = []string{
		"does_not_claim_to_reproduce_or_replace_the_lost_original_bytes",
		"does_not_change_the_consensus_bound_historical_freeze_sha256",
		"requires_exact_terminal_generation_file_hashes_and_full_legacy_terminal_verification",
		"does_not_activate_mainnet_or_execute_production_import",
	}
	raw, e := json.Marshal(a)
	if e != nil { t.Fatal(e) }
	if _, e = verifyRecoveryAttestation(raw, filepath.Join(dir, "missing-terminal"), c); e == nil {
		t.Fatal("re-attestation bypassed exact terminal generation")
	}
	a.ActivationAllowed = true
	raw, _ = json.Marshal(a)
	if _, e = verifyRecoveryAttestation(raw, filepath.Join(dir, "missing-terminal"), c); e == nil {
		t.Fatal("activatable re-attestation accepted")
	}
}

func TestVersionedRecoveryAttestationMatchesVerifierSchema(t *testing.T) {
	c := fixtureProtocol(t)
	raw, e := os.ReadFile(filepath.Join("..", "..", "deploy", "mainnet", "legacy-freeze.reattestation.v1.json"))
	if e != nil {
		t.Fatal(e)
	}
	var a recoveryAttestation
	if e = json.Unmarshal(raw, &a); e != nil {
		t.Fatal(e)
	}
	if a.Schema != "hashburst-mainnet-legacy-freeze-reattestation-v1" ||
		a.LostEvidence.Schema != "hashburst-managed-legacy-freeze-ha-v1" ||
		len(a.Guarantees) != 4 {
		t.Fatal("versioned re-attestation does not match verifier schema")
	}
	// The production verifier must get past strict JSON/semantic validation and fail only
	// because this test deliberately supplies no hash-pinned terminal generation.
	_, e = verifyRecoveryAttestation(raw, filepath.Join(t.TempDir(), "missing-terminal"), c)
	if e == nil {
		t.Fatal("versioned re-attestation bypassed exact terminal generation")
	}
	if !os.IsNotExist(e) {
		t.Fatalf("versioned re-attestation rejected before terminal verification: %v", e)
	}
}
