package blockchain

import (
	"crypto/sha256"
	"encoding/hex"
	"math/big"
)

const Difficulty = 4

func (b *Block) MineBlock() {
	target := new(big.Int).Lsh(big.NewInt(1), uint(256-Difficulty*4))
	for {
		hash := b.computeHash()
		hi := new(big.Int)
		hi.SetString(hash, 16)
		if hi.Cmp(target) == -1 {
			b.Hash = hash
			break
		}
		b.ProofOfWork++
	}
}

func (b *Block) computeHash() string {
	record := string(rune(b.Index)) + b.Timestamp.String() + b.PrevHash +
		string(rune(b.ProofOfTime)) + string(rune(b.ProofOfWork))
	for _, tx := range b.Transactions {
		record += tx.HashTransaction()
	}
	h := sha256.New()
	h.Write([]byte(record))
	return hex.EncodeToString(h.Sum(nil))
}
