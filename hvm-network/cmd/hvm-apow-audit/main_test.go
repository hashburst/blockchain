package main

import (
	"context"
	"hashburst/blockchain"
	"hashburst/consensus"
	"hashburst/wallet"
	"testing"
	"time"
)

func fixture(t *testing.T) *blockchain.Block {
	t.Helper()
	w, e := wallet.NewWallet()
	if e != nil {
		t.Fatal(e)
	}
	p, e := blockchain.MineAPoW(context.Background(), blockchain.APoWProof{ChainID: 4735490, Height: 1, ParentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PoH: 4000, Bits: 1}, w, w.Address())
	if e != nil {
		t.Fatal(e)
	}
	b := blockchain.NewBlockV2([]*blockchain.Transaction{blockchain.NewSystemReward(p.Beneficiary, 50)}, nil, p.ParentHash, 1, p.PoH, 4735490)
	b.Timestamp = time.Unix(100, 123)
	b.APoW = p
	b.FinalityCertificate = &consensus.QuorumCertificate{}
	b.Hash = b.GenerateHash()
	return b
}
func TestReadRealStorageAndWork(t *testing.T) {
	b := fixture(t)
	dir := t.TempDir()
	s := blockchain.NewChainStorage(dir)
	if e := s.SaveBlock(b); e != nil {
		t.Fatal(e)
	}
	got, e := readBlock(dir, 1)
	if e != nil {
		t.Fatal(e)
	}
	if got.Hash != b.Hash {
		t.Fatal("hash")
	}
	if _, e = audit(got); e != nil {
		t.Fatal(e)
	}
}
func TestRejectWrongRewardAndProof(t *testing.T) {
	b := fixture(t)
	b.Transactions[0].Amount = 49
	if _, e := audit(b); e == nil {
		t.Fatal("wrong reward accepted")
	}
	b = fixture(t)
	b.APoW.Signature = ""
	if _, e := audit(b); e == nil {
		t.Fatal("bad signature accepted")
	}
}
