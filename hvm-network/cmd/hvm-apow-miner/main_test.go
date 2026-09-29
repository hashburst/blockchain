package main

import (
	"context"
	"encoding/json"
	"hashburst/blockchain"
	"hashburst/wallet"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMinerRejectsForeignChainBeforeSubmission(t *testing.T) {
	w, _ := wallet.NewWallet()
	posted := false
	s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posted = true
		}
		json.NewEncoder(rw).Encode(blockchain.APoWProof{ChainID: 4735489})
	}))
	defer s.Close()
	err := mineJob(context.Background(), s.URL, w, w.Address(), 4735490, time.Second)
	if err == nil || !strings.Contains(err.Error(), "chain ID") || posted {
		t.Fatalf("foreign chain signed/submitted: %v", err)
	}
}
func TestMinerSubmitsVerifiableBeneficiary(t *testing.T) {
	w, _ := wallet.NewWallet()
	beneficiary, _ := wallet.NewWallet()
	posted := false
	s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			json.NewEncoder(rw).Encode(blockchain.APoWProof{ChainID: 4735490, Height: 100, ParentHash: strings.Repeat("a", 64), Bits: 1})
			return
		}
		var proof blockchain.APoWProof
		if e := json.NewDecoder(r.Body).Decode(&proof); e != nil {
			t.Error(e)
		}
		if e := proof.Verify(); e != nil {
			t.Error(e)
		}
		if proof.Beneficiary != strings.ToLower(beneficiary.Address()) {
			t.Error("beneficiary changed")
		}
		posted = true
	}))
	defer s.Close()
	if e := mineJob(context.Background(), s.URL, w, beneficiary.Address(), 4735490, time.Second); e != nil {
		t.Fatal(e)
	}
	if !posted {
		t.Fatal("missing submission")
	}
}
func TestMinerKeyNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "miner.key")
	if e := generateKey(path); e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(path)
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("key permissions")
	}
	if _, e := wallet.FromPrivateKeyHex(strings.TrimSpace(string(raw))); e != nil {
		t.Fatal(e)
	}
	if e := generateKey(path); e == nil {
		t.Fatal("overwrote existing key")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(raw) {
		t.Fatal("key changed")
	}
}
