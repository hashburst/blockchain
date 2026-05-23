package api

import (
	"net/http"

	"github.com/hashburst/blockchain/blockchain"
	"github.com/hashburst/blockchain/config"
	"github.com/hashburst/blockchain/poh"
)

type API struct {
	blockchain *blockchain.Blockchain
	poh        *poh.PoHGenerator
	config     *config.Config
}

func New(bc *blockchain.Blockchain, poh *poh.PoHGenerator, cfg *config.Config) *API {
	return &API{
		blockchain: bc,
		poh:        poh,
		config:     cfg,
	}
}

func (a *API) Router() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/transactions", a.applyMiddlewares(http.HandlerFunc(a.HandleTransaction)))
	mux.Handle("/blocks", a.applyMiddlewares(http.HandlerFunc(a.HandleGetBlocks)))
	mux.Handle("/users", a.applyMiddlewares(http.HandlerFunc(a.HandleGetUsers)))
	mux.Handle("/wallets", a.applyMiddlewares(http.HandlerFunc(a.HandleGetWallets)))
	return mux
}
