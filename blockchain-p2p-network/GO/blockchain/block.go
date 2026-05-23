package blockchain

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type Block struct {
	Index        int
	Timestamp    time.Time
	Transactions []*Transaction
	PrevHash     string
	Hash         string
	ProofOfWork  int64
	ProofOfTime  int64
}

func NewBlock(transactions []*Transaction, prevHash string, poh int64) *Block {
	b := &Block{Timestamp: time.Now(), Transactions: transactions, PrevHash: prevHash, ProofOfTime: poh}
	b.Hash = b.GenerateHash()
	return b
}

func (b *Block) GenerateHash() string {
	record := string(rune(b.Index)) + b.Timestamp.String() + b.PrevHash +
		string(rune(b.ProofOfTime)) + string(rune(b.ProofOfWork))
	for _, tx := range b.Transactions {
		record += tx.HashTransaction()
	}
	h := sha256.New()
	h.Write([]byte(record))
	return hex.EncodeToString(h.Sum(nil))
}
