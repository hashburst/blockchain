package blockchain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Mining difficulty (number of leading zeros in the hash)
const Difficulty = 4

// MineBlock runs the Proof of Work algorithm to find a valid block hash
func (b *Block) MineBlock() {
	var intHash big.Int
	var hash [32]byte
	for {
		record := b.GenerateHash()
		hash = sha256.Sum256([]byte(record))
		intHash.SetBytes(hash[:])
		if intHash.Cmp(big.NewInt(1).Lsh(big.NewInt(1), uint(256-Difficulty))) == -1 {
			break
		}
		b.ProofOfWork++
	}
	b.Hash = hex.EncodeToString(hash[:])
	fmt.Println("Block mined:", b.Hash)
}
