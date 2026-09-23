package blockchain

import (
	"context"
	"fmt"
	"log"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/multiformats/go-multiaddr"
)

const ProtocolID = protocol.ID("/hashburst/1.0.0")

// P2PNode rappresenta il nodo P2P HashBurst
type P2PNode struct {
	Host       host.Host
	Blockchain *Blockchain
	Mempool    *Mempool
	DNS        *BlockchainDNS // discovery dalla blockchain
}

// NewP2PNode crea un nuovo nodo P2P.
// Se privKey non è nil, usa quella chiave per garantire un peer ID stabile.
// Questo è il cambio fondamentale: il peer ID è ora deterministico e
// registrato nella blockchain come identità permanente.
func NewP2PNode(bc *Blockchain, mp *Mempool, port int, privKey crypto.PrivKey) (*P2PNode, error) {
	return NewP2PNodeWithListenIP(bc, mp, "0.0.0.0", port, privKey)
}

// NewP2PNodeWithListenIP is used by isolated dev/test runtimes that must not
// expose their libp2p listener on every host interface. Production keeps the
// existing NewP2PNode behavior and therefore remains unchanged.
func NewP2PNodeWithListenIP(bc *Blockchain, mp *Mempool, listenIP string, port int, privKey crypto.PrivKey) (*P2PNode, error) {
	if listenIP == "" {
		listenIP = "0.0.0.0"
	}
	cm, err := connmgr.NewConnManager(10, 100)
	if err != nil {
		return nil, fmt.Errorf("connmgr: %w", err)
	}

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/tcp/%d", listenIP, port)),
		libp2p.ConnectionManager(cm),
	}

	// Usa la chiave persistente se fornita
	if privKey != nil {
		opts = append(opts, libp2p.Identity(privKey))
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("libp2p.New: %w", err)
	}

	node := &P2PNode{
		Host:       h,
		Blockchain: bc,
		Mempool:    mp,
		DNS:        NewBlockchainDNS(bc),
	}

	log.Printf("P2P node started | ID: %s", h.ID())
	for _, addr := range h.Addrs() {
		log.Printf("  Listen: %s/p2p/%s", addr, h.ID())
	}

	return node, nil
}

func (p *P2PNode) handleStream(s network.Stream) {
	defer s.Close()
	buf := make([]byte, 4096)
	n, err := s.Read(buf)
	if err != nil {
		return
	}
	log.Printf("P2P msg from %s: %s", s.Conn().RemotePeer(), string(buf[:n]))
}

// Connect si connette a un peer tramite multiaddr
func (p *P2PNode) Connect(ctx context.Context, addrStr string) error {
	maddr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		return fmt.Errorf("invalid multiaddr: %w", err)
	}
	info, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return fmt.Errorf("peer info: %w", err)
	}
	p.Host.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.PermanentAddrTTL)
	if err := p.Host.Connect(ctx, *info); err != nil {
		return fmt.Errorf("connect to %s: %w", info.ID, err)
	}
	log.Printf("P2P connected: %s", info.ID)
	return nil
}

// ConnectFromBlockchainDNS connette ai peer registrati nella blockchain.
// Questo è il cuore del DNS distribuito: niente file statici, niente DNS,
// niente Cloudflare — solo la blockchain come source of truth.
func (p *P2PNode) ConnectFromBlockchainDNS(ctx context.Context, excludeNodeID string) {
	addrs := p.DNS.GetP2PBootstrapAddrs(excludeNodeID)
	if len(addrs) == 0 {
		log.Println("BlockchainDNS: no registered nodes yet — using static bootstrap")
		return
	}
	log.Printf("BlockchainDNS: connecting to %d registered nodes", len(addrs))
	for _, addr := range addrs {
		if err := p.Connect(ctx, addr); err != nil {
			log.Printf("BlockchainDNS: %s — %v", addr, err)
		}
	}
}

// StartMDNS avvia la discovery locale via mDNS (LAN)
func (p *P2PNode) StartMDNS() {
	_ = mdns.NewMdnsService(p.Host, "hashburst-mdns", &mdnsNotifee{p: p})
	log.Println("mDNS discovery started")
}

type mdnsNotifee struct{ p *P2PNode }

func (n *mdnsNotifee) HandlePeerFound(info peer.AddrInfo) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n.p.Host.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.PermanentAddrTTL)
	if err := n.p.Host.Connect(ctx, info); err != nil {
		log.Printf("mDNS: connect %s failed: %v", info.ID, err)
	} else {
		log.Printf("mDNS: connected %s", info.ID)
	}
}

// SendMessage invia un messaggio a un peer
func (p *P2PNode) SendMessage(ctx context.Context, peerID peer.ID, msg string) error {
	s, err := p.Host.NewStream(ctx, peerID, ProtocolID)
	if err != nil {
		return err
	}
	defer s.Close()
	_, err = s.Write([]byte(msg))
	return err
}

// GetMultiaddrs restituisce gli indirizzi pubblici completi del nodo
func (p *P2PNode) GetMultiaddrs() []string {
	addrs := []string{}
	for _, addr := range p.Host.Addrs() {
		full := fmt.Sprintf("%s/p2p/%s", addr, p.Host.ID())
		addrs = append(addrs, full)
	}
	return addrs
}
