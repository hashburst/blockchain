package blockchain

import (
    "context"
    "fmt"
    "log"
    "time"
    
    libp2p "github.com/libp2p/go-libp2p"
    peerstore "github.com/libp2p/go-libp2p/core/peerstore"
    peer "github.com/libp2p/go-libp2p/core/peer"
    host "github.com/libp2p/go-libp2p/core/host"
    network "github.com/libp2p/go-libp2p/core/network"
    multiaddr "github.com/multiformats/go-multiaddr"
    "github.com/libp2p/go-libp2p/core/protocol"
    "github.com/libp2p/go-libp2p/p2p/net/connmgr"
    "github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

// P2PNetwork gestisce le operazioni della rete P2P per sincronizzare la Mempool
type P2PNetwork struct {
    Host    host.Host
    Mempool *Mempool
    ctx     context.Context
}

// NewP2PNetwork crea una nuova rete P2P
func NewP2PNetwork(mempool *Mempool) *P2PNetwork {
    ctx := context.Background()
    connManager, _ := connmgr.NewConnManager(100, 400, time.Minute)

    // Creazione di un nuovo host libp2p
    host, err := libp2p.New(
        libp2p.ConnectionManager(connManager),
    )
    if err != nil {
        log.Fatalf("Errore nella creazione dell'host libp2p: %v", err)
    }

    fmt.Printf("Host creato con ID: %s\n", host.ID().Pretty())

    network := &P2PNetwork{
        Host:    host,
        Mempool: mempool,
        ctx:     ctx,
    }

    // Inizializzare il listener per le connessioni in entrata
    network.Host.SetStreamHandler(protocol.ID("/mempool/1.0.0"), network.handleStream)

    // Avviare mDNS per scoprire peer locali nella rete
    network.setupMDNS()

    return network
}

// setupMDNS imposta un servizio mDNS per la scoperta locale dei peer
func (p *P2PNetwork) setupMDNS() {
    service := mdns.NewMdnsService(p.ctx, p.Host, time.Second*10, "")
    service.RegisterNotifee(p)
}

// handleStream gestisce le connessioni in entrata per la sincronizzazione delle transazioni
func (p *P2PNetwork) handleStream(s network.Stream) {
    log.Println("Connessione in arrivo accettata")
    defer s.Close()

    var tx Transaction
    err := readTransaction(s, &tx)
    if err != nil {
        log.Println("Errore nella lettura della transazione: ", err)
        return
    }

    // Aggiungi la transazione ricevuta alla Mempool
    p.Mempool.ReceiveTransaction(tx)
}

// BroadcastTransaction trasmette una transazione a tutti i peer connessi
func (p *P2PNetwork) BroadcastTransaction(tx Transaction) {
    log.Printf("Trasmettendo transazione con ID: %s\n", tx.ID)

    for _, peerID := range p.Host.Peerstore().Peers() {
        if peerID == p.Host.ID() {
            continue
        }

        log.Printf("Inviando transazione a peer: %s\n", peerID.Pretty())
        stream, err := p.Host.NewStream(p.ctx, peerID, protocol.ID("/mempool/1.0.0"))
        if err != nil {
            log.Printf("Errore nella creazione del stream verso peer %s: %v\n", peerID, err)
            continue
        }
        defer stream.Close()

        err = writeTransaction(stream, &tx)
        if err != nil {
            log.Printf("Errore nella scrittura della transazione verso il peer: %v\n", err)
        }
    }
}

// writeTransaction scrive una transazione in uno stream
func writeTransaction(s network.Stream, tx *Transaction) error {
    _, err := s.Write([]byte(tx.ID + "\n"))
    return err
}

// readTransaction legge una transazione da uno stream
func readTransaction(s network.Stream, tx *Transaction) error {
    buf := make([]byte, 1024)
    n, err := s.Read(buf)
    if err != nil {
        return err
    }
    tx.ID = string(buf[:n])
    return nil
}

// HandlePeerFound gestisce quando viene trovato un nuovo peer mDNS
func (p *P2PNetwork) HandlePeerFound(info peer.AddrInfo) {
    fmt.Printf("Peer trovato tramite mDNS: %s\n", info.ID.Pretty())
    err := p.Host.Connect(p.ctx, info)
    if err != nil {
        fmt.Printf("Errore durante la connessione al peer: %v\n", err)
    }
}

/*package main

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
*/
