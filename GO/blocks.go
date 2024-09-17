package blockchain

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Block represents a single block in the blockchain
type Block struct {
	Index        int
	Timestamp    time.Time
	Data         string
	PrevHash     string
	Hash         string
	ProofOfTime  int64  // PoH value
}

// NewBlock creates a new block
func NewBlock(data string, prevHash string, poh int64) *Block {
	block := &Block{
		Index:       len(blockchain) + 1,
		Timestamp:   time.Now(),
		Data:        data,
		PrevHash:    prevHash,
		ProofOfTime: poh,
	}
	block.Hash = block.GenerateHash()
	return block
}

// GenerateHash creates a SHA256 hash of the block's data
func (b *Block) GenerateHash() string {
	record := string(b.Index) + b.Timestamp.String() + b.Data + b.PrevHash + string(b.ProofOfTime)
	h := sha256.New()
	h.Write([]byte(record))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}
