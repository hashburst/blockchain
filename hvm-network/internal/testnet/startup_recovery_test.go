package testnet

import (
	"context"
	"hashburst/wallet"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A bound P2P port is deliberately used as an ordering probe. Recovery must
// reject invalid durable state before attempting to open that listener.
func TestRecoveryCheckedBeforeP2PListener(t *testing.T) {
	c := fixture(t)
	c.Protocol.ActivationHeight = 1
	c.Protocol.ConsensusActivationHeight = 1
	s, err := Prepare(c, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Signer, err = wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	s.Config.ValidatorID = "startup-validator"
	path := filepath.Join(c.DataDir, "consensus-recovery.json")
	before := []byte(`{"Payload":{},"SHA256":"invalid"}`)
	if err = os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	s.Config.P2PPort = listener.Addr().(*net.TCPAddr).Port
	err = Run(context.Background(), s)
	if err == nil || !strings.Contains(err.Error(), "recovery checksum mismatch") {
		t.Fatalf("durable recovery must precede P2P listener: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatal("recovery file modified")
	}
	for _, name := range []string{"consensus-votes.jsonl", "consensus-bft-signatures.jsonl"} {
		b, e := os.ReadFile(filepath.Join(c.DataDir, name))
		if e != nil || len(b) != 0 {
			t.Fatalf("signing journal changed: %s %v", name, e)
		}
	}
	// RPC socket must also be released on the startup error.
	rpc, err := net.Listen("tcp", c.RPCListen)
	if err != nil {
		t.Fatal(err)
	}
	rpc.Close()
}
