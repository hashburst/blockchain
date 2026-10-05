package mainnetidentity

import (
	"encoding/json"
	"hashburst/blockchain"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture() blockchain.ProtocolV2Config {
	c := blockchain.DefaultProtocolV2Config()
	c.ChainID = 4735489
	c.ActivationHeight = 0
	c.ConsensusActivationHeight = 5
	m := blockchain.ApprovedMainnetGenesisImport()
	c.GenesisImport = &m
	return c
}
func TestEnrollmentPossessionProtocolBindingAndNoOverwrite(t *testing.T) {
	parent, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(parent, "identity")
	c := fixture()
	o := Options{Config: c, NodeID: "mainnet-v1", IP: "127.0.0.1", P2PPort: 31317, TEPPublicKey: strings.Repeat("12", 32)}
	if e := Generate(out, o); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(out, "public.json"))
	if e != nil {
		t.Fatal(e)
	}
	var v Identity
	if e = json.Unmarshal(b, &v); e != nil {
		t.Fatal(e)
	}
	if e = Verify(c, v); e != nil {
		t.Fatal(e)
	}
	if e = Generate(out, o); e == nil {
		t.Fatal("overwrote identity")
	}
	after, _ := os.ReadFile(filepath.Join(out, "public.json"))
	if string(after) != string(b) {
		t.Fatal("public file changed")
	}
	for _, name := range []string{"p2p.key", "consensus.key", "operator.key", "reward.key"} {
		s, e := os.Stat(filepath.Join(out, name))
		if e != nil || s.Mode().Perm() != 0600 {
			t.Fatal("private key permissions")
		}
	}
	wrong := c
	wrong.ChainID = 4735490
	if Verify(wrong, v) == nil {
		t.Fatal("cross-chain accepted")
	}
	wrong = c
	wrong.Validator.MinBondUnits++
	if Verify(wrong, v) == nil {
		t.Fatal("changed protocol accepted")
	}
	bad := v
	bad.ConsensusProof = strings.Repeat("00", 65)
	if Verify(c, bad) == nil {
		t.Fatal("bad consensus proof accepted")
	}
	bad = v
	bad.PeerProof = "00"
	if Verify(c, bad) == nil {
		t.Fatal("bad peer proof accepted")
	}
	v.Registration.Sender = "0x0000000000000000000000000000000000000001"
	if Verify(c, v) == nil {
		t.Fatal("tampered operator accepted")
	}
}
