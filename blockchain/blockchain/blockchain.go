package blockchain

import "time"

type Blockchain struct {
	blocks     []*Block
	difficulty int
}

func New(difficulty int) *Blockchain {
	return &Blockchain{
		blocks:     []*Block{NewGenesisBlock()},
		difficulty: difficulty,
	}
}

func (bc *Blockchain) AddBlock(txs []Transaction, pohHash string) (*Block, error) {
	prevBlock := bc.blocks[len(bc.blocks)-1]
	
	newBlock := &Block{
		Index:        prevBlock.Index + 1,
		Timestamp:    time.Now().Unix(),
		Transactions: txs,
		PrevHash:     prevBlock.Hash,
		PoHHash:      pohHash,
	}

	newBlock.Mine(bc.difficulty)
	bc.blocks = append(bc.blocks, newBlock)
	return newBlock, nil
}

func (bc *Blockchain) GetBlocks() []*Block {
	return bc.blocks
}

func (bc *Blockchain) VerifyChain() bool {
	for i := 1; i < len(bc.blocks); i++ {
		current := bc.blocks[i]
		previous := bc.blocks[i-1]
		
		if current.PrevHash != previous.Hash {
			return false
		}
		
		if current.Hash != current.CalculateHash() {
			return false
		}
		
		if !isHashValid(current.Hash, bc.difficulty) {
			return false
		}
	}
	return true
}
