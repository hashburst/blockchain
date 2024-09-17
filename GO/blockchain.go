package blockchain

import "log"

// Blockchain represents a list of blocks
type Blockchain struct {
	Blocks []*Block
}

// AddBlock adds a new block to the chain using PoH consensus
func (bc *Blockchain) AddBlock(data string) {
	latestBlock := bc.Blocks[len(bc.Blocks)-1]
	newBlock := NewBlock(data, latestBlock.Hash, PoH(latestBlock.ProofOfTime))
	if bc.ValidateBlock(newBlock) {
		bc.Blocks = append(bc.Blocks, newBlock)
	} else {
		log.Println("Block validation failed")
	}
}

// ValidateBlock checks if the block is valid
func (bc *Blockchain) ValidateBlock(newBlock *Block) bool {
	// Check the previous hash
	if newBlock.PrevHash != bc.Blocks[len(bc.Blocks)-1].Hash {
		return false
	}

	// Check the PoH value
	if !ValidatePoH(newBlock.ProofOfTime) {
		return false
	}
	return true
}
