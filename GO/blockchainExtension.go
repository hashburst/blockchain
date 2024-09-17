package blockchain

import "log"

// Blockchain represents a list of blocks
type Blockchain struct {
	Blocks       []*Block
	PendingTXs   []*Transaction // Pool of transactions waiting to be mined
	MiningReward float64         // Reward for mining a block
}

// AddBlock adds a mined block to the chain using PoH and PoW
func (bc *Blockchain) AddBlock(minerAddress string) {
	latestBlock := bc.Blocks[len(bc.Blocks)-1]

	// Create reward transaction for the miner
	rewardTX := NewTransaction("System", minerAddress, bc.MiningReward)
	bc.PendingTXs = append(bc.PendingTXs, rewardTX)

	// Mine the block
	newBlock := NewBlock(bc.PendingTXs, latestBlock.Hash, PoH(latestBlock.ProofOfTime))
	newBlock.MineBlock()

	// Add the block to the chain and clear pending transactions
	if bc.ValidateBlock(newBlock) {
		bc.Blocks = append(bc.Blocks, newBlock)
		bc.PendingTXs = []*Transaction{}
	} else {
		log.Println("Block validation failed")
	}
}

// ValidateBlock checks if the block is valid
func (bc *Blockchain) ValidateBlock(newBlock *Block) bool {
	// Validate PoH and PoW values
	if newBlock.PrevHash != bc.Blocks[len(bc.Blocks)-1].Hash || !ValidatePoH(newBlock.ProofOfTime) {
		return false
	}
	return true
}
