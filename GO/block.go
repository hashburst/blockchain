package blockchain

import (
	"time"
)

// Block represents a single block in the blockchain
type Block struct {
	Index        int
	Timestamp    time.Time
	Transactions []*Transaction // List of transactions
	PrevHash     string
	Hash         string
	ProofOfWork  int64  // PoW value
	ProofOfTime  int64  // PoH value
}

// NewBlock creates a new block with transactions
func NewBlock(transactions []*Transaction, prevHash string, poh int64) *Block {
	block := &Block{
		Index:        len(blockchain) + 1,
		Timestamp:    time.Now(),
		Transactions: transactions,
		PrevHash:     prevHash,
		ProofOfTime:  poh,
	}
	block.Hash = block.GenerateHash()
	return block
}

// GenerateHash creates a SHA256 hash of the block's data including transactions
func (b *Block) GenerateHash() string {
	record := string(b.Index) + b.Timestamp.String() + b.PrevHash + string(b.ProofOfTime) + string(b.ProofOfWork)
	for _, tx := range b.Transactions {
		record += tx.HashTransaction()
	}
	h := sha256.New()
	h.Write([]byte(record))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}
