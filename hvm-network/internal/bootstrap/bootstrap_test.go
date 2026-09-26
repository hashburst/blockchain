package bootstrap

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hashburst/internal/testnet"
	"os"
	"path/filepath"
	"testing"
)

func identity(t *testing.T, root string, i byte) Identity {
	t.Helper()
	name := "node" + string(rune('a'+i))
	out := filepath.Join(root, name)
	key := make([]byte, 32)
	key[0] = i + 1
	if e := Generate(out, Options{ChainID: 987654321, NodeID: name, IP: "127.0.0.1", P2PPort: 31307 + int(i), TEPPublicKey: hex.EncodeToString(key)}); e != nil {
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
	return v
}
func TestBootstrapProvisionAndCheck(t *testing.T) {
	root := t.TempDir()
	ids := []Identity{}
	for i := byte(0); i < 4; i++ {
		ids = append(ids, identity(t, root, i))
	}
	out := filepath.Join(root, "network")
	if e := Assemble(out, 987654321, ids); e != nil {
		t.Fatal(e)
	}
	if e := Assemble(out, 987654321, ids); e == nil {
		t.Fatal("overwrote output")
	}
	if _, e := os.Stat(filepath.Join(out, "BOOTSTRAP_COMPLETE")); e != nil {
		t.Fatal(e)
	}
	var pin string
	for i := byte(0); i < 4; i++ {
		name := "node" + string(rune('a'+i))
		c, e := testnet.Load(filepath.Join(out, name+".validator.json"))
		if e != nil {
			t.Fatal(e)
		}
		c.DataDir = filepath.Join(root, name, "state")
		c.P2PKeyFile = filepath.Join(root, name, "p2p.key")
		c.ConsensusKeyFile = filepath.Join(root, name, "consensus.key")
		if e = os.Mkdir(c.DataDir, 0700); e != nil {
			t.Fatal(e)
		}
		for _, file := range []string{"blockchain.dat", "blockchain.idx"} {
			b, e := os.ReadFile(filepath.Join(out, "checkpoint", file))
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(c.DataDir, file), b, 0600); e != nil {
				t.Fatal(e)
			}
		}
		s, e := testnet.Prepare(c, true)
		if e != nil {
			t.Fatal(e)
		}
		if len(s.Chain.CurrentValidatorSet(8).Validators) != 4 {
			t.Fatal("validator set")
		}
		s.Close()
		s, e = testnet.Prepare(c, false)
		if e != nil {
			t.Fatal(e)
		}
		s.Close()
		if i == 0 {
			pin = c.Pin()
		} else if pin != c.Pin() {
			t.Fatal("different network pin")
		}
		c.Protocol.ChainID++
		if s, e = testnet.Prepare(c, false); e == nil {
			s.Close()
			t.Fatal("accepted chain mismatch")
		}
	}
}
func TestRejectInvalidBootstrap(t *testing.T) {
	root := t.TempDir()
	ids := []Identity{}
	for i := byte(0); i < 4; i++ {
		ids = append(ids, identity(t, root, i))
	}
	for _, id := range []uint64{0, 1337, ^uint64(0)} {
		if e := Assemble(filepath.Join(root, "forbidden"), id, ids); e == nil {
			t.Fatal("accepted chain")
		}
	}
	if e := Assemble(filepath.Join(root, "duplicate"), 987654321, []Identity{ids[0], ids[0], ids[2], ids[3]}); e == nil {
		t.Fatal("duplicate accepted")
	}
	ids[0].Validator.Signature = "00"
	if e := Assemble(filepath.Join(root, "bad"), 987654321, ids); e == nil {
		t.Fatal("bad signature")
	}
	for _, p := range []string{"forbidden", "duplicate", "bad"} {
		if _, e := os.Stat(filepath.Join(root, p)); !os.IsNotExist(e) {
			t.Fatal("wrote invalid state")
		}
	}
}
func TestIdentityNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	identity(t, root, 0)
	path := filepath.Join(root, "nodea")
	old, _ := os.ReadFile(filepath.Join(path, "consensus.key"))
	if e := Generate(path, Options{ChainID: 987654321, NodeID: "nodea", IP: "127.0.0.1", P2PPort: 31307, TEPPublicKey: hex.EncodeToString(make([]byte, 32))}); e == nil {
		t.Fatal("zero TEP key")
	}
	if e := newDir(path); e == nil {
		t.Fatal("existing directory")
	}
	now, _ := os.ReadFile(filepath.Join(path, "consensus.key"))
	if string(now) != string(old) {
		t.Fatal("key overwritten")
	}
}

func TestBootstrapFiveAndSixIdentities(t *testing.T) {
	for _, n := range []int{5, 6} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			root := t.TempDir()
			ids := []Identity{}
			for i := 0; i < n; i++ {
				ids = append(ids, identity(t, root, byte(i)))
			}
			out := filepath.Join(root, "network")
			if e := Assemble(out, 987654321, ids); e != nil {
				t.Fatal(e)
			}
			c, e := testnet.Load(filepath.Join(out, "nodea.validator.json"))
			if e != nil {
				t.Fatal(e)
			}
			if c.CheckpointHeight != n+3 || c.Protocol.ConsensusActivationHeight != uint64(n+4) || len(c.Bootnodes) != n-1 {
				t.Fatal("wrong bootstrap configuration")
			}
		})
	}
}
