package main

import (
	"context"
	"hashburst/blockchain"
	"hashburst/consensus"
	"hashburst/wallet"
	"testing"
	"time"
)

func fixture(t *testing.T, chainID uint64) *blockchain.Block {
	t.Helper()
	w, e := wallet.NewWallet()
	if e != nil {
		t.Fatal(e)
	}
	p, e := blockchain.MineAPoW(context.Background(), blockchain.APoWProof{ChainID: chainID, Height: 1, ParentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PoH: 4000, Bits: 1}, w, w.Address())
	if e != nil {
		t.Fatal(e)
	}
	b := blockchain.NewBlockV2([]*blockchain.Transaction{blockchain.NewSystemReward(p.Beneficiary, 50)}, nil, p.ParentHash, 1, p.PoH, chainID)
	b.Timestamp = time.Unix(100, 123)
	b.APoW = p
	b.FinalityCertificate = &consensus.QuorumCertificate{}
	b.Hash = b.GenerateHash()
	return b
}

func TestReadRealStorageAndWorkBothNetworks(t *testing.T) {
	for _, chainID := range []uint64{mainnetChainID, testnetChainID} {
		t.Run(string(rune(chainID)), func(t *testing.T) {
			b := fixture(t, chainID)
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
			result, e := audit(got, chainID)
			if e != nil {
				t.Fatal(e)
			}
			if result["chain_id"] != chainID || result["system_reward_count"] != 1 || result["reward_hbt"] != 50 || result["reward_units"] != int64(5000000000) {
				t.Fatalf("unexpected audit result: %#v", result)
			}
		})
	}
}

func TestRejectWrongChain(t *testing.T) {
	b := fixture(t, testnetChainID)
	if _, e := audit(b, mainnetChainID); e == nil {
		t.Fatal("cross-network block accepted")
	}
	if _, e := audit(b, 1); e == nil {
		t.Fatal("unsupported chain accepted")
	}
}

func TestRejectWrongRewardAndProof(t *testing.T) {
	b := fixture(t, testnetChainID)
	b.Transactions[0].Amount = 49
	if _, e := audit(b, testnetChainID); e == nil {
		t.Fatal("wrong reward accepted")
	}
	b = fixture(t, testnetChainID)
	b.APoW.Signature = ""
	if _, e := audit(b, testnetChainID); e == nil {
		t.Fatal("bad signature accepted")
	}
}
