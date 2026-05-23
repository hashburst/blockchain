package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"hashburst/blockchain"
	"hashburst/wallet"
)

var (
	bc      *blockchain.Blockchain
	mp      *blockchain.Mempool
	p2pNode *blockchain.P2PNode
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("HashBurst Blockchain Node starting...")

	bc = blockchain.NewBlockchain()
	bc.MiningReward = 50.0
	mp = blockchain.NewMempool()

	rewardAddr := os.Getenv("REWARD_ADDRESS")
	if rewardAddr == "" {
		nodeWallet := wallet.NewWallet()
		rewardAddr = nodeWallet.Address()
		log.Printf("Generated node wallet: %s", rewardAddr)
	} else {
		log.Printf("Reward wallet: %s", rewardAddr)
	}

	rpcPort := envInt("RPC_PORT", 8009)
	p2pPort := envInt("P2P_PORT", 30307)

	var err error
	p2pNode, err = blockchain.NewP2PNode(bc, mp, p2pPort)
	if err != nil {
		log.Printf("P2P warning: %v — continuing without P2P", err)
	} else {
		p2pNode.StartMDNS()
		ctx := context.Background()
		for _, addr := range strings.Split(os.Getenv("BOOTSTRAP_PEERS"), ",") {
			addr = strings.TrimSpace(addr)
			if addr == "" {
				continue
			}
			if err := p2pNode.Connect(ctx, addr); err != nil {
				log.Printf("Bootstrap peer %s: %v", addr, err)
			}
		}
	}

	go startRPC(rewardAddr, rpcPort)
	go miningLoop(rewardAddr)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Signal %v received — shutdown", sig)
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			return n
		}
	}
	return def
}

func miningLoop(minerAddr string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		txs := mp.GetTransactions()
		if len(txs) > 0 {
			log.Printf("Mining block with %d transactions...", len(txs))
			bc.PendingTXs = append(bc.PendingTXs, txs...)
			bc.AddBlock(minerAddr)
		}
	}
}

func startRPC(minerAddr string, port int) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":      "ok",
			"blockHeight": len(bc.Blocks),
			"node":        os.Getenv("NODE_ID"),
			"chainId":     1337,
		})
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		peers := 0
		if p2pNode != nil {
			peers = len(p2pNode.Host.Network().Peers())
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":      "online",
			"blockHeight": len(bc.Blocks),
			"peers":       peers,
			"version":     "1.0.0",
			"chainId":     1337,
			"tps":         0,
			"nodeId":      os.Getenv("NODE_ID"),
			"miner":       minerAddr[:8] + "...",
		})
	})

	mux.HandleFunc("/api/blocks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		type blockJSON struct {
			Number       int    `json:"number"`
			Hash         string `json:"hash"`
			ParentHash   string `json:"parentHash"`
			Timestamp    int64  `json:"timestamp"`
			Transactions int    `json:"transactions"`
			Miner        string `json:"miner"`
		}
		blocks := []blockJSON{}
		for _, b := range bc.Blocks {
			blocks = append(blocks, blockJSON{
				Number: b.Index, Hash: b.Hash, ParentHash: b.PrevHash,
				Timestamp: b.Timestamp.Unix(), Transactions: len(b.Transactions),
				Miner: minerAddr,
			})
		}
		if len(blocks) > 10 {
			blocks = blocks[len(blocks)-10:]
		}
		json.NewEncoder(w).Encode(blocks)
	})

	mux.HandleFunc("/api/transactions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		type txJSON struct {
			Hash   string  `json:"hash"`
			From   string  `json:"from"`
			To     string  `json:"to"`
			Value  float64 `json:"value"`
			Status string  `json:"status"`
		}
		txs := []txJSON{}
		for _, b := range bc.Blocks {
			for _, tx := range b.Transactions {
				txs = append(txs, txJSON{
					Hash: tx.HashTransaction(), From: tx.Sender,
					To: tx.Receiver, Value: tx.Amount, Status: "success",
				})
			}
		}
		json.NewEncoder(w).Encode(txs)
	})

	addr := fmt.Sprintf("0.0.0.0:%d", port)
	log.Printf("RPC HTTP listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("RPC server error: %v", err)
	}
}
