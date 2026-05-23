package main

import (
	"fmt"
	"myblockchain/blockchain"
)

func main() {
	// Initialize the blockchain
	bc := blockchain.InitBlockchain()

	// Add some blocks
	bc.AddBlock("First block data")
	bc.AddBlock("Second block data")

	// Print the blockchain
	for _, block := range bc.Blocks {
		fmt.Printf("Block %d:\n", block.Index)
		fmt.Printf("Timestamp: %s\n", block.Timestamp)
		fmt.Printf("Data: %s\n", block.Data)
		fmt.Printf("Previous Hash: %s\n", block.PrevHash)
		fmt.Printf("Hash: %s\n", block.Hash)
		fmt.Printf("Proof of History: %d\n", block.ProofOfTime)
		fmt.Println()
	}
}
