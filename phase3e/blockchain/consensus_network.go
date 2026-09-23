package blockchain

// consensus_network.go — Phase 3D validator consensus gossip.
//
// This protocol is deliberately separate from /hashburst/1.0.0 chain sync so
// legacy nodes can coexist while Protocol V2/consensus remains activation
// gated. Network peers relay signed consensus objects; the libp2p peer that
// carries a frame is not trusted as the validator identity. Origin authority is
// established only by the consensus-key signature verified by ConsensusReactor.

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"hashburst/consensus"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const (
	ConsensusProtocolID protocol.ID = "/hashburst/consensus/2.0.0"

	consensusMsgProposal    = "proposal"
	consensusMsgPrevote     = "prevote"
	consensusMsgPrecommit   = "precommit"
	consensusMsgRoundChange = "round_change"
	consensusMsgEvidence    = "evidence"
	consensusMsgFinalized   = "finalized"

	consensusSeenLimit          = 8192
	consensusOutboundQueueLimit = 4096
)

type consensusWireMessage struct {
	Type        string                           `json:"type"`
	Proposal    *ConsensusProposal               `json:"proposal,omitempty"`
	Prevote     *consensus.Prevote               `json:"prevote,omitempty"`
	Precommit   *consensus.Vote                  `json:"precommit,omitempty"`
	RoundChange *consensus.RoundChange           `json:"round_change,omitempty"`
	Evidence    *consensus.BFTDoubleSignEvidence `json:"evidence,omitempty"`
	Finalized   *Block                           `json:"finalized,omitempty"`
}

type ConsensusNetworkStatus struct {
	Protocol        string `json:"protocol"`
	ConnectedPeers  int    `json:"connected_peers"`
	SeenMessages    int    `json:"seen_messages"`
	OutboundQueued  int    `json:"outbound_queued"`
	MaxMessageBytes int    `json:"max_message_bytes"`
	DeliveryPending int    `json:"delivery_pending"`
	DeliveryDropped uint64 `json:"delivery_dropped"`
	OutboundDropped uint64 `json:"outbound_dropped"`
}

type consensusOutboundFrame struct {
	payload []byte
	except  peer.ID
}

// Libp2pConsensusNetwork implements ConsensusTransport. Constructing it only
// installs a stream handler; it never starts the reactor and cannot activate
// consensus while ConsensusActivationHeight is disabled.
type Libp2pConsensusNetwork struct {
	host    host.Host
	reactor *ConsensusReactor
	cfg     consensus.NetworkConfig

	mu              sync.Mutex
	seen            map[string]struct{}
	seenOrder       []string
	outbound        chan consensusOutboundFrame
	delivery        *consensusDelivery
	outboundDropped atomic.Uint64
}

func NewLibp2pConsensusNetwork(h host.Host, reactor *ConsensusReactor, cfg consensus.NetworkConfig) (*Libp2pConsensusNetwork, error) {
	if h == nil {
		return nil, fmt.Errorf("nil libp2p host")
	}
	if reactor == nil {
		return nil, fmt.Errorf("nil consensus reactor")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	n := &Libp2pConsensusNetwork{
		host: h, reactor: reactor, cfg: cfg, seen: make(map[string]struct{}),
		outbound: make(chan consensusOutboundFrame, consensusOutboundQueueLimit),
	}
	n.delivery = &consensusDelivery{send: n.sendPayloadToPeer}
	go n.outboundLoop()
	h.SetStreamHandler(ConsensusProtocolID, n.handleStream)
	reactor.SetTransport(n)
	reactor.bc.attachConsensusNetwork(n)
	return n, nil
}

func (n *Libp2pConsensusNetwork) Status() ConsensusNetworkStatus {
	n.mu.Lock()
	seen := len(n.seen)
	n.mu.Unlock()
	peers := 0
	if n.host != nil && n.host.Network() != nil {
		peers = len(n.host.Network().Peers())
	}
	status := ConsensusNetworkStatus{Protocol: string(ConsensusProtocolID), ConnectedPeers: peers, SeenMessages: seen, OutboundQueued: len(n.outbound), MaxMessageBytes: n.cfg.MaxMessageBytes, OutboundDropped: n.outboundDropped.Load()}
	if n.delivery != nil {
		status.DeliveryPending, status.DeliveryDropped = n.delivery.status()
	}
	return status
}

func consensusMessageID(payload []byte) string {
	h := sha256.Sum256(payload)
	return hex.EncodeToString(h[:])
}

func (n *Libp2pConsensusNetwork) markSeen(id string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, ok := n.seen[id]; ok {
		return false
	}
	n.seen[id] = struct{}{}
	n.seenOrder = append(n.seenOrder, id)
	if len(n.seenOrder) > consensusSeenLimit {
		drop := n.seenOrder[0]
		n.seenOrder = n.seenOrder[1:]
		delete(n.seen, drop)
	}
	return true
}

func (n *Libp2pConsensusNetwork) encode(msg consensusWireMessage) ([]byte, string, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, "", err
	}
	if len(payload) == 0 || len(payload) > n.cfg.MaxMessageBytes {
		return nil, "", fmt.Errorf("consensus message size %d exceeds limit %d", len(payload), n.cfg.MaxMessageBytes)
	}
	return payload, consensusMessageID(payload), nil
}

func (n *Libp2pConsensusNetwork) writePayload(w io.Writer, payload []byte) error {
	if len(payload) == 0 || len(payload) > n.cfg.MaxMessageBytes {
		return fmt.Errorf("invalid consensus frame size %d", len(payload))
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if count, err := w.Write(hdr[:]); err != nil {
		return err
	} else if count != len(hdr) {
		return io.ErrShortWrite
	}
	count, err := w.Write(payload)
	if err == nil && count != len(payload) {
		return io.ErrShortWrite
	}
	return err
}

func (n *Libp2pConsensusNetwork) readPayload(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	sz := int(binary.BigEndian.Uint32(hdr[:]))
	if sz <= 0 || sz > n.cfg.MaxMessageBytes {
		return nil, fmt.Errorf("consensus frame size %d outside 1..%d", sz, n.cfg.MaxMessageBytes)
	}
	payload := make([]byte, sz)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (n *Libp2pConsensusNetwork) handleStream(s network.Stream) {
	defer s.Close()
	if err := s.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		_ = s.Reset()
		return
	}
	payload, err := n.readPayload(s)
	if err != nil {
		_ = s.Reset()
		return
	}
	_ = s.SetReadDeadline(time.Time{})
	id := consensusMessageID(payload)
	if !n.markSeen(id) {
		return
	}
	var msg consensusWireMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}
	if err := n.dispatch(msg); err != nil {
		log.Printf("consensus: rejected %s from %s: %v", msg.Type, s.Conn().RemotePeer(), err)
		return
	}
	// Relay only after local cryptographic/consensus validation. Never keep a
	// stream handler (or a reactor callback reached from it) blocked on outbound
	// dial timeouts; the ordered outbound queue performs network I/O separately.
	n.queueRelay(payload, s.Conn().RemotePeer())
}

func (n *Libp2pConsensusNetwork) dispatch(msg consensusWireMessage) error {
	switch msg.Type {
	case consensusMsgProposal:
		if msg.Proposal == nil {
			return fmt.Errorf("proposal payload missing")
		}
		return n.reactor.HandleProposal(*msg.Proposal)
	case consensusMsgPrevote:
		if msg.Prevote == nil {
			return fmt.Errorf("prevote payload missing")
		}
		return n.reactor.HandlePrevote(*msg.Prevote)
	case consensusMsgPrecommit:
		if msg.Precommit == nil {
			return fmt.Errorf("precommit payload missing")
		}
		return n.reactor.HandlePrecommit(*msg.Precommit)
	case consensusMsgRoundChange:
		if msg.RoundChange == nil {
			return fmt.Errorf("round-change payload missing")
		}
		return n.reactor.HandleRoundChange(*msg.RoundChange)
	case consensusMsgEvidence:
		if msg.Evidence == nil {
			return fmt.Errorf("evidence payload missing")
		}
		return n.reactor.HandleEvidence(*msg.Evidence)
	case consensusMsgFinalized:
		if msg.Finalized == nil {
			return fmt.Errorf("finalized block payload missing")
		}
		return n.reactor.HandleFinalizedBlock(msg.Finalized)
	default:
		return fmt.Errorf("unknown consensus message type %q", msg.Type)
	}
}

func (n *Libp2pConsensusNetwork) sendPayloadToPeer(id peer.ID, payload []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s, err := n.host.NewStream(ctx, id, ConsensusProtocolID)
	if err == nil {
		deadline, _ := ctx.Deadline()
		err = n.sendOnStream(s, payload, deadline)
	}
	if err != nil {
		log.Printf("consensus: relay to %s failed: %v", id, err)
	}
}

func (n *Libp2pConsensusNetwork) relayPayload(payload []byte, except peer.ID) {
	for _, id := range n.host.Network().Peers() {
		if id == except {
			continue
		}
		if err := n.delivery.enqueue(id, payload); err != nil {
			log.Printf("consensus: %v", err)
		}
	}
}

type consensusWriteStream interface {
	io.WriteCloser
	SetWriteDeadline(time.Time) error
	Reset() error
}

func (n *Libp2pConsensusNetwork) sendOnStream(s consensusWriteStream, payload []byte, deadline time.Time) error {
	if err := s.SetWriteDeadline(deadline); err != nil {
		_ = s.Reset()
		return err
	}
	if err := n.writePayload(s, payload); err != nil {
		_ = s.Reset()
		return err
	}
	if err := s.Close(); err != nil {
		_ = s.Reset()
		return err
	}
	return nil
}

func (n *Libp2pConsensusNetwork) outboundLoop() {
	for frame := range n.outbound {
		n.relayPayload(frame.payload, frame.except)
	}
}

func (n *Libp2pConsensusNetwork) queueRelay(payload []byte, except peer.ID) {
	frame := consensusOutboundFrame{payload: append([]byte(nil), payload...), except: except}
	select {
	case n.outbound <- frame:
	default:
		// Explicit bounded loss under overload, never unbounded detached workers.
		n.outboundDropped.Add(1)
		log.Printf("consensus: outbound queue saturated at %d frames; frame dropped", cap(n.outbound))
	}
}

func (n *Libp2pConsensusNetwork) broadcast(msg consensusWireMessage) error {
	payload, id, err := n.encode(msg)
	if err != nil {
		return err
	}
	// Mark local-origin frames before broadcasting so an echo cannot be
	// dispatched back into the local reactor. Outbound network I/O is queued so
	// callers never hold the reactor mutex across libp2p dial/write timeouts.
	n.markSeen(id)
	n.queueRelay(payload, "")
	return nil
}

func (n *Libp2pConsensusNetwork) BroadcastConsensusProposal(v ConsensusProposal) error {
	return n.broadcast(consensusWireMessage{Type: consensusMsgProposal, Proposal: &v})
}
func (n *Libp2pConsensusNetwork) BroadcastConsensusPrevote(v consensus.Prevote) error {
	return n.broadcast(consensusWireMessage{Type: consensusMsgPrevote, Prevote: &v})
}
func (n *Libp2pConsensusNetwork) BroadcastConsensusPrecommit(v consensus.Vote) error {
	return n.broadcast(consensusWireMessage{Type: consensusMsgPrecommit, Precommit: &v})
}
func (n *Libp2pConsensusNetwork) BroadcastConsensusRoundChange(v consensus.RoundChange) error {
	return n.broadcast(consensusWireMessage{Type: consensusMsgRoundChange, RoundChange: &v})
}
func (n *Libp2pConsensusNetwork) BroadcastConsensusEvidence(v consensus.BFTDoubleSignEvidence) error {
	return n.broadcast(consensusWireMessage{Type: consensusMsgEvidence, Evidence: &v})
}
func (n *Libp2pConsensusNetwork) BroadcastConsensusFinalized(v *Block) error {
	if v == nil {
		return fmt.Errorf("nil finalized block")
	}
	copyBlock := cloneBlockForConsensus(v)
	return n.broadcast(consensusWireMessage{Type: consensusMsgFinalized, Finalized: copyBlock})
}
