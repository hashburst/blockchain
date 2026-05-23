package blockchain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

type Block struct {
	Index        int           `json:"index"`
	Timestamp    int64         `json:"timestamp"`
	Transactions []Transaction `json:"transactions"`
	PrevHash     string        `json:"prevHash"`
	Hash         string        `json:"hash"`
	PoHHash      string        `json:"pohHash"`
	Nonce        string        `json:"nonce"`
}

func NewGenesisBlock() *Block {
	return &Block{
		Index:     0,
		Timestamp: time.Now().Unix(),
		PrevHash:  "0",
	}
}

func (b *Block) CalculateHash() string {
	data, _ := json.Marshal(struct {
		Index        int           `json:"index"`
		Timestamp    int64         `json:"timestamp"`
		PrevHash     string        `json:"prevHash"`
		PoHHash      string        `json:"pohHash"`
		Nonce        string        `json:"nonce"`
		Transactions []Transaction `json:"transactions"`
	}{
		b.Index,
		b.Timestamp,
		b.PrevHash,
		b.PoHHash,
		b.Nonce,
		b.Transactions,
	})
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (b *Block) Mine(difficulty int) {
	for {
		b.Nonce = generateNonce()
		hash := b.CalculateHash()
		if isHashValid(hash, difficulty) {
			b.Hash = hash
			return
		}
	}
}

func generateNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate nonce: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func isHashValid(hash string, difficulty int) bool {
	prefix := strings.Repeat("0", difficulty)
	return strings.HasPrefix(hash, prefix)
}
