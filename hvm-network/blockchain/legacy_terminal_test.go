package blockchain

import (
	"hashburst/wallet"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLegacyTerminalZeroIssuance(t *testing.T) {
	w, e := wallet.NewWallet()
	if e != nil {
		t.Fatal(e)
	}
	dest, e := wallet.NewWallet()
	if e != nil {
		t.Fatal(e)
	}
	blocks := []*Block{NewGenesisBlock()}
	for i := 1; i < 10; i++ {
		tx := NewSystemReward(w.Address(), 50)
		tx.Nonce = int64(i)
		tx.ID = tx.HashTransaction()
		b := NewBlock([]*Transaction{tx}, blocks[i-1].Hash, i, PoH(blocks[i-1].ProofOfTime))
		b.Timestamp = time.Unix(1750000000+int64(i), 0).UTC()
		if e = b.MineBlock(); e != nil {
			t.Fatal(e)
		}
		blocks = append(blocks, b)
	}
	anchor := blocks[9].Hash
	data := "test-terminal-bound-to-" + anchor
	tx := NewDataTransaction(w.Address(), dest.Address(), 450, data)
	if e = tx.Sign(w); e != nil {
		t.Fatal(e)
	}
	b := NewBlock([]*Transaction{tx}, anchor, 10, PoH(blocks[9].ProofOfTime))
	b.Timestamp = blocks[9].Timestamp.Add(time.Second)
	if e = b.MineBlock(); e != nil {
		t.Fatal(e)
	}
	blocks = append(blocks, b)
	s, e := verifyLegacyTerminal(blocks, anchor, w.Address(), dest.Address(), data)
	if e != nil {
		t.Fatal(e)
	}
	if s.BalanceUnits(w.Address()) != 0 || s.BalanceUnits(dest.Address()) != LegacyCloseUnits {
		t.Fatal("supply mismatch")
	}
	if e = ValidateBlockAgainst(blocks[9], blocks[10], DefaultMiningReward); e == nil {
		t.Fatal("live rules accepted terminal exception")
	}
	if _, e = VerifyLegacyTerminal(blocks); e == nil {
		t.Fatal("unpinned fixture accepted in production")
	}
	dir := t.TempDir()
	st := NewChainStorage(dir)
	for _, b := range blocks {
		if e = st.SaveBlock(b); e != nil {
			t.Fatal(e)
		}
	}
	reread, e := ReadLegacyPair(dir, 11)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = verifyLegacyTerminal(reread, anchor, w.Address(), dest.Address(), data); e != nil {
		t.Fatal(e)
	}
	for name, mutate := range map[string]func(*Block){
		"reward":    func(b *Block) { b.Transactions = append(b.Transactions, NewSystemReward(dest.Address(), 50)) },
		"amount":    func(b *Block) { tx := *b.Transactions[0]; tx.Amount = 449; b.Transactions = []*Transaction{&tx} },
		"signature": func(b *Block) { tx := *b.Transactions[0]; tx.Signature = ""; b.Transactions = []*Transaction{&tx} },
		"metadata":  func(b *Block) { b.ProtocolChainID = 4735489 },
		"poh":       func(b *Block) { b.ProofOfTime++ },
	} {
		t.Run(name, func(t *testing.T) {
			b := *blocks[10]
			mutate(&b)
			if e := b.MineBlock(); e != nil {
				t.Fatal(e)
			}
			bad := append(append([]*Block{}, blocks[:10]...), &b)
			if _, e := verifyLegacyTerminal(bad, anchor, w.Address(), dest.Address(), data); e == nil {
				t.Fatal("invalid terminal accepted")
			}
		})
	}
	if _, e = verifyLegacyTerminal(append(blocks, blocks[10]), anchor, w.Address(), dest.Address(), data); e == nil {
		t.Fatal("post-terminal block accepted")
	}
	p := filepath.Join(dir, "blockchain.idx")
	raw, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p, raw[:len(raw)-1], 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = ReadLegacyPair(dir, 11); e == nil {
		t.Fatal("truncated index accepted")
	}
}
