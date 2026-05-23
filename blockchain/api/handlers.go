package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/hashburst/blockchain/blockchain"
)

func (a *API) HandleTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var tx blockchain.Transaction
	if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	tx.Timestamp = time.Now().Unix()

	block, err := a.blockchain.AddBlock([]blockchain.Transaction{tx}, a.poh.GetLastHash())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondWithJSON(w, http.StatusCreated, block)
}

func (a *API) HandleGetBlocks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	blocks := a.blockchain.GetBlocks()
	respondWithJSON(w, http.StatusOK, blocks)
}

func (a *API) HandleGetUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	// Esempio: recupera utenti dalla blockchain
	users := make([]map[string]interface{}, 0)
	for _, block := range a.blockchain.GetBlocks() {
		for _, tx := range block.Transactions {
			if tx.Type == "USER" && tx.UserData != nil {
				users = append(users, map[string]interface{}{
					"userId":    tx.UserID,
					"field":     tx.UserData.Field,
					"newValue":  tx.UserData.NewValue,
					"timestamp": tx.Timestamp,
				})
			}
		}
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(users),
		"users": users,
	})
}

func (a *API) HandleGetWallets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	// Esempio: recupera wallet dalla blockchain
	wallets := make([]map[string]interface{}, 0)
	for _, block := range a.blockchain.GetBlocks() {
		for _, tx := range block.Transactions {
			if tx.Type == "WALLET" && tx.Wallet != nil {
				wallets = append(wallets, map[string]interface{}{
					"address":   tx.Wallet.Address,
					"ownerId":   tx.UserID,
					"signature": tx.Wallet.Signature,
					"timestamp": tx.Timestamp,
				})
			}
		}
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"count":   len(wallets),
		"wallets": wallets,
	})
}
