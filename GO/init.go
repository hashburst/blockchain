package blockchain

// InitBlockchain initializes the blockchain with the genesis block
func InitBlockchain() *Blockchain {
	genesisBlock := NewBlock("Genesis Block", "", PoH(0))
	blockchain := &Blockchain{
		Blocks: []*Block{genesisBlock},
	}
	return blockchain
}
