package main

import (
	"fmt"
	"myblockchain/blockchain"
	"myblockchain/wallet"
)

func main() {
	// Initialize the blockchain
	bc := blockchain.InitBlockchain()
	bc.MiningReward = 50.0

	// Create two wallets
	wallet1 := wallet.NewWallet()
	wallet2 := wallet.NewWallet()

	// Create a transaction from wallet1 to wallet2
	tx := blockchain.NewTransaction(wallet1.Address(), wallet2.Address(), 10)
	tx.SignTransaction(wallet1.PrivateKey)

	// Add the transaction to the pending pool
	bc.PendingTXs = append(bc.PendingTXs, tx)

	// Mine the block
	bc.AddBlock(wallet1.Address())

	// Print the blockchain
	for _, block := range bc.Blocks {
		fmt.Printf("Block %d:\n", block.Index)
		fmt.Printf("Timestamp: %s\n", block.Timestamp)
		fmt.Printf("Previous Hash: %s\n", block.PrevHash)
		fmt.Printf("Hash: %s\n", block.Hash)
		fmt.Printf("Proof of Work: %d\n", block.ProofOfWork)
		fmt.Printf("Proof of History: %d\n", block.ProofOfTime)
		for _, tx := range block.Transactions {
			fmt.Printf("Transaction: %s -> %s : %f\n", tx.Sender, tx.Receiver, tx.Amount)
		}
		fmt.Println()
	}
}
