package blockchain

import (
	"context"
	"fmt"
	"log"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
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

type P2PNode struct {
	Host       host.Host
	Blockchain *Blockchain
	Mempool    *Mempool
}

func NewP2PNode(bc *Blockchain, mp *Mempool, port int) (*P2PNode, error) {
	cm, err := connmgr.NewConnManager(10, 100)
	if err != nil {
		return nil, fmt.Errorf("connmgr: %w", err)
	}
	h, err := libp2p.New(
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)),
		libp2p.ConnectionManager(cm),
	)
	if err != nil {
		return nil, fmt.Errorf("libp2p.New: %w", err)
	}
	node := &P2PNode{Host: h, Blockchain: bc, Mempool: mp}
	h.SetStreamHandler(ProtocolID, node.handleStream)
	log.Printf("P2P node: %s", h.ID())
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
	log.Printf("Msg from %s: %s", s.Conn().RemotePeer(), string(buf[:n]))
}

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
	log.Printf("Connected: %s", info.ID)
	return nil
}

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
		log.Printf("mDNS connect %s failed: %v", info.ID, err)
	} else {
		log.Printf("mDNS connected: %s", info.ID)
	}
}

func (p *P2PNode) SendMessage(ctx context.Context, peerID peer.ID, msg string) error {
	s, err := p.Host.NewStream(ctx, peerID, ProtocolID)
	if err != nil {
		return err
	}
	defer s.Close()
	_, err = s.Write([]byte(msg))
	return err
}
