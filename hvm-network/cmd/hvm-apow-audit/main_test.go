package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"hashburst/blockchain"
	"hashburst/consensus"
	"hashburst/ledger"
	"hashburst/wallet"
	"os"
	"path/filepath"
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
		t.Run(fmt.Sprintf("chain-%d", chainID), func(t *testing.T) {
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


func writeHBX2Fixture(t *testing.T, dir string, b *blockchain.Block) {
	t.Helper()
	payload, err := blockchain.EncodeLedgerBlock(nil, b)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := ledger.AppendFrame(nil, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "blockchain.dat"), frame, 0600); err != nil {
		t.Fatal(err)
	}
	var idx [20]byte
	binary.BigEndian.PutUint64(idx[0:8], uint64(b.Index))
	binary.BigEndian.PutUint64(idx[8:16], 0)
	binary.BigEndian.PutUint32(idx[16:20], uint32(len(frame)))
	if err = os.WriteFile(filepath.Join(dir, "blockchain.idx"), idx[:], 0600); err != nil {
		t.Fatal(err)
	}
}

func TestReadHBX2StorageAndAudit(t *testing.T) {
	b := fixture(t, mainnetChainID)
	dir := t.TempDir()
	writeHBX2Fixture(t, dir, b)

	got, err := readBlock(dir, uint64(b.Index))
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash != b.Hash || got.ProtocolChainID != mainnetChainID {
		t.Fatalf("unexpected HBX2 block: %#v", got)
	}
	result, err := audit(got, mainnetChainID)
	if err != nil {
		t.Fatal(err)
	}
	if result["system_reward_count"] != 1 || result["reward_units"] != int64(5000000000) {
		t.Fatalf("unexpected audit result: %#v", result)
	}
}

func TestRejectCorruptHBX2Frame(t *testing.T) {
	b := fixture(t, mainnetChainID)
	dir := t.TempDir()
	writeHBX2Fixture(t, dir, b)

	path := filepath.Join(dir, "blockchain.dat")
	frame, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	frame[len(frame)-1] ^= 0xff
	if err = os.WriteFile(path, frame, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readBlock(dir, uint64(b.Index)); err == nil {
		t.Fatal("corrupt HBX2 frame accepted")
	}
}
