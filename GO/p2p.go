package main

import (
	"net/http"
	"encoding/json"
	"myblockchain/blockchain"
)

var blockchain *blockchain.Blockchain

// Get the current state of the blockchain
func getBlockchain(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(blockchain)
}

func main() {
	blockchain = blockchain.InitBlockchain()

	// Add a simple HTTP server to share the blockchain state
	http.HandleFunc("/blockchain", getBlockchain)
	http.ListenAndServe(":8080", nil)
}
