package main

import (
	"crypto/sha256"
	"encoding/hex"
	"hashburst/wallet"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCustodyProof(t *testing.T) {
	w, e := wallet.NewWallet()
	if e != nil {
		t.Fatal(e)
	}
	p, e := prove(w, w.Address())
	if e != nil {
		t.Fatal(e)
	}
	d := sha256.Sum256([]byte(p.Message))
	sig, e := hex.DecodeString(p.Signature)
	if e != nil {
		t.Fatal(e)
	}
	a, e := wallet.RecoverAddress(d[:], sig)
	if e != nil || a != w.Address() || !strings.HasPrefix(p.Message, domain) {
		t.Fatal("proof invalid")
	}
	other, _ := wallet.NewWallet()
	if _, e = prove(w, other.Address()); e == nil {
		t.Fatal("accepted wrong address")
	}
	p2, e := prove(w, w.Address())
	if e != nil || p.Message == p2.Message {
		t.Fatal("nonce reused")
	}
}
func TestPrivateFile(t *testing.T) {
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(dir, "wallet.key")
	if e = os.WriteFile(p, []byte("fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = readPrivate(p); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(dir, "link")
	if e = os.Symlink(p, link); e != nil {
		t.Fatal(e)
	}
	if _, e = readPrivate(link); e == nil {
		t.Fatal("symlink accepted")
	}
	if e = os.Chmod(p, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e = readPrivate(p); e == nil {
		t.Fatal("public private file accepted")
	}
}

func TestEncryptedCustody(t *testing.T) {
	w, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := w.EncryptV3("fixture-password", wallet.LightScryptN, wallet.LightScryptR, wallet.LightScryptP)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := wallet.DecryptV3(encrypted, "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = prove(loaded, w.Address()); err != nil {
		t.Fatal(err)
	}
	if _, err = wallet.DecryptV3(encrypted, "wrong-password"); err == nil {
		t.Fatal("wrong password accepted")
	}
}
