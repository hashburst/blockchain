package main

import (
	"encoding/json"
	"hashburst/protocolv2"
	"hashburst/wallet"
	"os"
	"path/filepath"
	"testing"
)

func TestTransferDomainAndIntent(t *testing.T) {
	w, _ := wallet.NewWallet()
	to, _ := wallet.NewWallet()
	tx := protocolv2.TransactionV2{Version: 2, ChainID: 4735490, Type: protocolv2.TxHBTTransfer, Sender: w.Address(), To: to.Address(), ValueUnits: 100, Sequence: 1, ComputeLimit: 100000, MaxFeeUnits: 101}
	raw, _ := json.Marshal(tx)
	signed, e := decodeTransfer(raw, 4735490, w.Address())
	if e != nil {
		t.Fatal(e)
	}
	if e = signed.Sign(w); e != nil {
		t.Fatal(e)
	}
	if e = signed.Verify(4735490); e != nil {
		t.Fatal(e)
	}
	if e = signed.Verify(4735489); e == nil {
		t.Fatal("cross-chain signature accepted")
	}
	for _, mutate := range []func(*protocolv2.TransactionV2){func(x *protocolv2.TransactionV2) { x.ChainID = 4735489 }, func(x *protocolv2.TransactionV2) { x.Type = protocolv2.TxContractCall }, func(x *protocolv2.TransactionV2) { x.Data = []byte{1} }, func(x *protocolv2.TransactionV2) { x.ValueUnits = -1 }, func(x *protocolv2.TransactionV2) { x.Sender = to.Address() }, func(x *protocolv2.TransactionV2) { x.Signature = "old" }} {
		bad := tx
		mutate(&bad)
		raw, _ = json.Marshal(bad)
		if _, e = decodeTransfer(raw, 4735490, w.Address()); e == nil {
			t.Fatal("unsafe intent accepted")
		}
	}
}
func TestExclusivePrivateOutput(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	p := filepath.Join(dir, "key.json")
	if e := writeNew(p, []byte("original")); e != nil {
		t.Fatal(e)
	}
	if e := writeNew(p, []byte("replacement")); e == nil {
		t.Fatal("overwrite")
	}
	b, _ := readPrivate(p)
	if string(b) != "original" {
		t.Fatal("changed")
	}
	os.Chmod(p, 0644)
	if _, e := readPrivate(p); e == nil {
		t.Fatal("public key file allowed")
	}
}

func TestCreateEncryptedOnly(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	path := filepath.Join(dir, "wallet.json")
	oldArgs, oldStdin := os.Args, os.Stdin
	defer func() { os.Args = oldArgs; os.Stdin = oldStdin }()
	r, w, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	w.WriteString("test-password-long-enough\n")
	w.Close()
	os.Stdin = r
	os.Args = []string{"hashburst-wallet", "create", "--chain-id", "4735490", "--out", path}
	if e = run(); e != nil {
		t.Fatal(e)
	}
	b, e := readPrivate(path)
	if e != nil {
		t.Fatal(e)
	}
	key, e := wallet.DecryptV3(b, "test-password-long-enough")
	if e != nil || !wallet.IsValidAddress(key.Address()) {
		t.Fatal("restore failed", e)
	}
	if _, e = wallet.DecryptV3(b, "incorrect-password"); e == nil {
		t.Fatal("wrong password accepted")
	}
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) != 1 {
		t.Fatal("unexpected plaintext or extra output", e)
	}
}
