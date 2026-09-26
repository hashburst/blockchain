package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
	bDNS    *blockchain.BlockchainDNS
	syncer  *blockchain.Syncer
	nodeID  string
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "wallet" {
		os.Exit(wallet.RunCLI(os.Args[2:]))
	}
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("HashBurst Blockchain Node starting...")

	nodeID = envStr("NODE_ID", "hashburst-node")
	rpcPort := envInt("RPC_PORT", 8009)
	p2pPort := envInt("P2P_PORT", 30307)
	externalIP := envStr("EXTERNAL_IP", "")
	rewardAddr := envStr("REWARD_ADDRESS", "")
	bootstrapEnv := envStr("BOOTSTRAP_PEERS", "")
	keyPath := envStr("P2P_KEY_PATH", "/var/lib/hashburst/node_p2p.key")
	tepPubkey := envStr("TEP_PUBKEY", "")
	rpcEndpoint := envStr("RPC_ENDPOINT", fmt.Sprintf("http://localhost:%d/api", rpcPort))
	storageDir := envStr("STORAGE_DIR", "/var/lib/hashburst")

	bc = blockchain.NewBlockchainWithDir(storageDir)
	bc.MiningReward = 50.0
	mp = blockchain.NewMempool()
	bDNS = blockchain.NewBlockchainDNS(bc)

	if rewardAddr == "" {
		log.Fatalf("REWARD_ADDRESS not set.\n" +
			"  Create your  wallet:  hashburst-node wallet new --dir /root/keystore --password-file /root/.hbpass\n" +
			"  Put your address in /etc/hashburst/env as REWARD_ADDRESS=0x...")
	}
	if !wallet.IsValidAddress(rewardAddr) {
		log.Fatalf("REWARD_ADDRESS %q is not a valid EVM address", rewardAddr)
	}
	log.Printf("Reward address: %s", rewardAddr)

	// Wallet d'identità del nodo: firma le NODE_REGISTRATION. Separato dal
	// reward wallet — zero fondi, se compromesso non tocca valore.
	var nodeWallet *wallet.Wallet
	if ksDir := os.Getenv("NODE_KEYSTORE"); ksDir != "" {
		pwFile := os.Getenv("NODE_KEYSTORE_PASSWORD_FILE")
		pw, err := os.ReadFile(pwFile)
		if err != nil {
			log.Fatalf("NODE_KEYSTORE_PASSWORD_FILE non leggibile: %v", err)
		}
		matches, _ := filepath.Glob(filepath.Join(ksDir, "UTC--*"))
		if len(matches) == 0 {
			log.Fatalf("nessun keystore in %s", ksDir)
		}
		nodeWallet, err = wallet.LoadKeystore(matches[0], strings.TrimSpace(string(pw)))
		if err != nil {
			log.Fatalf("caricamento wallet d'identità nodo: %v", err)
		}
		log.Printf("Node identity wallet: %s", nodeWallet.Address())
	} else {
		log.Fatalf("NODE_KEYSTORE non impostato: le NODE_REGISTRATION non possono essere firmate")
	}

	identity, err := LoadOrCreateP2PIdentity(keyPath)
	if err != nil {
		log.Fatalf("P2P identity error: %v", err)
	}

	p2pNode, err = blockchain.NewP2PNode(bc, mp, p2pPort, identity.PrivKey)
	if err != nil {
		log.Printf("P2P warning: %v", err)
	} else {
		p2pNode.StartMDNS()
		ctx := context.Background()
		for _, addr := range strings.Split(bootstrapEnv, ",") {
			addr = strings.TrimSpace(addr)
			if addr == "" {
				continue
			}
			if err := p2pNode.Connect(ctx, addr); err != nil {
				log.Printf("Bootstrap %s: %v", addr, err)
			}
		}
		p2pNode.ConnectFromBlockchainDNS(ctx, nodeID)

		// Loop di riconnessione: ogni 45s, se non siamo connessi ai bootstrap
		// peer, riconnetti. Senza questo, quando un peer si riavvia la
		// connessione muore e non si ristabilisce mai — la causa dei "peers 0"
		// dopo ogni restart. Una rete che deve restare su da sola non puo'
		// dipendere da riconnessioni manuali.
		go func() {
			bootstrapAddrs := strings.Split(bootstrapEnv, ",")
			for {
				time.Sleep(45 * time.Second)
				if len(p2pNode.Host.Network().Peers()) > 0 {
					continue // gia' connesso a qualcuno
				}
				rctx := context.Background()
				for _, addr := range bootstrapAddrs {
					addr = strings.TrimSpace(addr)
					if addr == "" {
						continue
					}
					if err := p2pNode.Connect(rctx, addr); err != nil {
						log.Printf("riconnessione a %s fallita: %v", addr, err)
					} else {
						log.Printf("riconnesso a bootstrap peer")
					}
				}
				p2pNode.ConnectFromBlockchainDNS(rctx, nodeID)
			}
		}()
	}

	// Syncer: sincronizzazione catena + gossip mempool. Prende mp per depositare
	// le transazioni ricevute via gossip.
	syncer = blockchain.NewSyncer(bc, mp, p2pNode.Host)
	bc.SetMempool(mp)
	bc.SetSyncer(syncer)
	go func() {
		for {
			time.Sleep(30 * time.Second)
			syncer.HelloAll()
		}
	}()

	// NODE_REGISTRATION — una sola volta per nodo, persistita con flag file
	go func() {
		time.Sleep(5 * time.Second)

		flagFile := filepath.Join(storageDir, "node_registered.flag")

		if data, err := os.ReadFile(flagFile); err == nil {
			// Il flag registra peerID|tep_pubkey. Se la pubkey attuale combacia
			// con quella nel flag, siamo gia' registrati col dato giusto: esci.
			// Se differisce (es. TEP attivato dopo), il flag e' obsoleto: si
			// prosegue per ri-registrare con la pubkey nuova.
			parts := strings.SplitN(string(data), "|", 2)
			flaggedPubkey := ""
			if len(parts) == 2 {
				flaggedPubkey = parts[1]
			}
			if flaggedPubkey == tepPubkey {
				log.Printf("NODE_REGISTRATION: flag found (pubkey coerente) — skipping")
				return
			}
			log.Printf("NODE_REGISTRATION: flag obsoleto (pubkey cambiata), ri-registro")
		}

		if existing, ok := bDNS.GetNode(nodeID); ok {
			// Gia' in catena. Ma se la TEP pubkey registrata differisce da
			// quella attuale (es. TEP e' stato attivato dopo la prima
			// registrazione), ri-registriamo per aggiornarla. GetAllNodes
			// tiene l'ultima per NodeID, quindi la nuova vince sulla vecchia.
			if existing.TEPPubkey == tepPubkey {
				log.Printf("NODE_REGISTRATION: found in blockchain, pubkey aggiornata — nodeID=%s", nodeID)
				os.WriteFile(flagFile, []byte(identity.PeerID.String()+"|"+tepPubkey), 0644)
				return
			}
			log.Printf("NODE_REGISTRATION: pubkey cambiata (catena=%q, attuale=%q), ri-registro",
				existing.TEPPubkey[:min(8, len(existing.TEPPubkey))], tepPubkey[:min(8, len(tepPubkey))])
			// NON esce: prosegue a costruire e inviare la nuova registrazione.
		}

		multiaddrs := []string{}
		if p2pNode != nil {
			for _, addr := range p2pNode.GetMultiaddrs() {
				if !strings.Contains(addr, "127.0.0.1") &&
					!strings.Contains(addr, "172.") {
					multiaddrs = append(multiaddrs, addr)
				}
			}
		}

		tx := blockchain.NewNodeRegistration(nodeWallet.Address(), blockchain.NodeRecord{
			NodeID:      nodeID,
			PeerID:      identity.PeerID.String(),
			Multiaddrs:  multiaddrs,
			TEPPubkey:   tepPubkey,
			TEPPort:     47777,
			RPCEndpoint: rpcEndpoint,
			ExternalIP:  externalIP,
			Version:     "1.0.0",
			ChainID:     1337,
		})
		if err := tx.Sign(nodeWallet); err != nil {
			log.Printf("firma NODE_REGISTRATION fallita: %v", err)
		}
		mp.AddTransactionOnce(tx)

		// GOSSIP: propaga la registrazione ai peer. Essenziale per i nodi
		// sync-only (MINER_ENABLED=false): non minano, quindi la loro
		// registrazione deve raggiungere il nodo che mina, che la include
		// in un blocco. Senza gossip, un nodo sync-only non entrerebbe mai
		// nel registro on-chain.
		if syncer != nil {
			syncer.GossipTx(tx)
		}

		log.Printf("NODE_REGISTRATION queued | nodeID=%s peerID=%s",
			nodeID, identity.PeerID)

		go func() {
			// Ri-gossippa la registrazione a ogni giro finché non appare in
			// catena. Non dipende dal timing della connessione: se al primo
			// tentativo non c'erano peer connessi, il giro dopo riprova. Un
			// nodo sync-only continua a ri-annunciarsi finché il miner non
			// include la sua registrazione in un blocco.
			for i := 0; i < 40; i++ {
				time.Sleep(3 * time.Second)
				if _, ok := bDNS.GetNode(nodeID); ok {
					os.WriteFile(flagFile, []byte(identity.PeerID.String()+"|"+tepPubkey), 0644)
					log.Printf("NODE_REGISTRATION: mined and flag saved")
					return
				}
				// Ri-propaga solo nei primi tentativi: serve a coprire il caso
				// in cui alla prima emissione non c'erano peer connessi. Dopo,
				// se e' gia' arrivata, ContainsTx la scarta lato ricevente;
				// continuare a ri-propagare sarebbe solo rumore.
				if syncer != nil && i < 5 {
					syncer.GossipTx(tx)
				}
			}
			log.Printf("NODE_REGISTRATION: warning — not confirmed after 120s")
		}()
	}()

	go startRPC(rewardAddr, rpcPort, identity.PeerID.String())
	go miningLoop(rewardAddr)
	go tepPeerSyncLoop(nodeID, storageDir)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Signal %v — shutdown", sig)
}

func tepPeerSyncLoop(myNodeID, storageDir string) {
	peersFile := filepath.Join(storageDir, "tep/peers.json")
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		peers := bDNS.GetTEPPeers(myNodeID)
		if len(peers) == 0 {
			continue
		}
		type pj struct {
			Peers []blockchain.TEPPeerEntry `json:"peers"`
		}
		data, err := json.MarshalIndent(pj{Peers: peers}, "", "  ")
		if err != nil {
			continue
		}
		if err := os.WriteFile(peersFile, data, 0644); err != nil {
			log.Printf("tepSync: %v", err)
			continue
		}
		log.Printf("tepSync: %d peers → %s", len(peers), peersFile)
	}
}

// miningLoop produce blocchi SOLO se MINER_ENABLED=true. Su un nodo sync-only
// (MINER_ENABLED=false o assente) esce subito: il nodo riceve blocchi via sync
// ma non ne produce. Questo elimina i fork alla radice — un solo produttore di
// blocchi nella rete significa nessuna catena concorrente.
func miningLoop(minerAddr string) {
	if envStr("MINER_ENABLED", "false") != "true" {
		log.Println("Mining DISABILITATO (MINER_ENABLED != true): nodo in modalità sync-only")
		return
	}
	log.Println("Mining ABILITATO: questo nodo produce blocchi")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		txs := mp.SnapshotTransactions()
		txsV2 := mp.SnapshotTransactionsV2()
		if len(txs) > 0 || len(txsV2) > 0 {
			log.Printf("Mining block with %d V1 tx + %d V2 tx...", len(txs), len(txsV2))
			bc.SetPendingTransactions(txs, txsV2)
			if err := bc.AddBlock(minerAddr); err != nil {
				// Mempool is intentionally untouched on failure. The same txs may
				// be retried after the transient/invalid condition is resolved.
				log.Printf("Mining fallito: %v", err)
			}
		}
	}
}

func startRPC(minerAddr string, port int, peerID string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok", "blockHeight": len(bc.Blocks),
			"node": nodeID, "peerID": peerID, "chainId": 1337,
		})
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		peers := 0
		if p2pNode != nil {
			peers = len(p2pNode.Host.Network().Peers())
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "online", "blockHeight": len(bc.Blocks),
			"peers": peers, "version": "1.0.0", "chainId": 1337,
			"tps": 0, "nodeId": nodeID, "peerID": peerID,
			"miner": minerAddr[:8] + "...",
		})
	})

	mux.HandleFunc("/api/blocks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		blocks := []map[string]interface{}{}
		for _, b := range bc.Blocks {
			blocks = append(blocks, map[string]interface{}{
				"number": b.Index, "version": b.EffectiveVersion(), "hash": b.Hash, "parentHash": b.PrevHash,
				"timestamp": b.Timestamp.Unix(), "transactions": len(b.Transactions) + len(b.TransactionsV2),
				"transactions_v1": len(b.Transactions), "transactions_v2": len(b.TransactionsV2),
				"hbt_state_root": b.HBTStateRoot, "hvm_state_root": b.HVMStateRoot,
				"receipts_root": b.ReceiptsRoot, "miner": minerAddr,
			})
		}
		if len(blocks) > 10 {
			blocks = blocks[len(blocks)-10:]
		}
		json.NewEncoder(w).Encode(blocks)
	})

	mux.HandleFunc("/api/transactions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		txs := []map[string]interface{}{}
		for _, b := range bc.Blocks {
			for _, tx := range b.Transactions {
				t := map[string]interface{}{
					"hash": tx.HashTransaction(), "from": tx.Sender,
					"to": tx.Receiver, "value": tx.Amount, "status": "success", "version": 1,
				}
				if tx.Receiver == "0x0000000000000000000000000000000000REGISTRY" {
					t["tx_type"] = "NODE_REGISTRATION"
				}
				txs = append(txs, t)
			}
			for _, tx := range b.TransactionsV2 {
				t := map[string]interface{}{
					"hash": "0x" + tx.HashHex(), "from": tx.Sender, "to": tx.To,
					"value_units": tx.ValueUnits, "sequence": tx.Sequence,
					"tx_type": tx.Type.String(), "version": 2,
				}
				if receipt, ok := bc.Receipt(tx.HashHex()); ok {
					t["status"] = map[bool]string{true: "success", false: "reverted"}[receipt.Success]
					t["compute_used"] = receipt.ComputeUsed
					t["fee_units"] = receipt.FeeUnits
					t["contract"] = receipt.Contract
					t["revert_reason"] = receipt.RevertReason
				}
				txs = append(txs, t)
			}
		}
		json.NewEncoder(w).Encode(txs)
	})

	mux.HandleFunc("/api/nodes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		result := []blockchain.NodeRecord{}
		for _, rec := range bDNS.GetAllNodes() {
			result = append(result, rec)
		}
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("/api/tep/peers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		peers := bDNS.GetTEPPeers(nodeID)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"peers": peers, "count": len(peers),
		})
	})

	mux.HandleFunc("/api/storage", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stats := bc.StorageStats()
		stats["blocks"] = len(bc.Blocks)
		stats["dir"] = bc.StorageDir()
		json.NewEncoder(w).Encode(stats)
	})

	// JSON-RPC native HashBurst + Ethereum-read compatibility. Phase 3B adds
	// signed TransactionV2/HVM methods but V2 remains activation-gated.
	rpcHandler := blockchain.NewRPCHandler(bc, mp, 1337)
	if syncer != nil {
		rpcHandler.SetV2Broadcaster(syncer)
	}
	mux.Handle("/rpc", rpcHandler)

	log.Printf("RPC listening on 0.0.0.0:%d", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", port), mux))
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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
