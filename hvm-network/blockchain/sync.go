package blockchain

// sync.go — chain synchronization and V1/V2 transaction gossip.

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"hashburst/consensus"
	"hashburst/protocolv2"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	MsgHello       = "hello"
	MsgGetBlocks   = "get_blocks"
	MsgBlocks      = "blocks"
	MsgNewBlock    = "new_block"
	MsgNewTx       = "new_tx"
	MsgNewTxV2     = "new_tx_v2"
	maxFrameBytes  = 8 << 20
	maxBlocksBatch = 500
)

type syncMessage struct {
	Type                     string                    `json:"type"`
	Height                   int                       `json:"height,omitempty"`
	PoHTicks                 int64                     `json:"poh_ticks,omitempty"`
	FinalizedHeight          int                       `json:"finalized_height,omitempty"`
	FinalizedHash            string                    `json:"finalized_hash,omitempty"`
	ConsensusProtocolVersion string                    `json:"consensus_protocol_version,omitempty"`
	FromIndex                int                       `json:"from_index,omitempty"`
	Blocks                   []*blockWire              `json:"blocks,omitempty"`
	Tx                       *txWire                   `json:"tx,omitempty"`
	TxV2                     *protocolv2.TransactionV2 `json:"tx_v2,omitempty"`
}

type blockWire struct {
	Version                 uint16                        `json:"version,omitempty"`
	ProtocolChainID         uint64                        `json:"protocol_chain_id,omitempty"`
	Index                   int                           `json:"index"`
	TimestampNs             int64                         `json:"timestamp_ns"`
	Transactions            []txWire                      `json:"transactions"`
	TransactionsV2          []*protocolv2.TransactionV2   `json:"transactions_v2,omitempty"`
	PrevHash                string                        `json:"prev_hash"`
	Hash                    string                        `json:"hash"`
	ProofOfWork             int64                         `json:"pow"`
	ProofOfTime             int64                         `json:"poh"`
	HBTStateRoot            string                        `json:"hbt_state_root,omitempty"`
	HVMStateRoot            string                        `json:"hvm_state_root,omitempty"`
	ReceiptsRoot            string                        `json:"receipts_root,omitempty"`
	ValidatorSetRoot        string                        `json:"validator_set_root,omitempty"`
	ValidatorStateRoot      string                        `json:"validator_state_root,omitempty"`
	AuthorValidatorID       string                        `json:"author_validator_id,omitempty"`
	ProposerID              string                        `json:"proposer_id,omitempty"`
	ConsensusRound          uint64                        `json:"consensus_round,omitempty"`
	ValidRound              int64                         `json:"valid_round,omitempty"`
	ValidPrevoteCertificate *consensus.PrevoteCertificate `json:"valid_prevote_certificate,omitempty"`
	FinalityCertificate     *consensus.QuorumCertificate  `json:"finality_certificate,omitempty"`
}

type txWire struct {
	ID        string  `json:"id"`
	Sender    string  `json:"sender"`
	Receiver  string  `json:"receiver"`
	Amount    float64 `json:"amount"`
	Nonce     int64   `json:"nonce"`
	Data      string  `json:"data,omitempty"`
	Signature string  `json:"signature,omitempty"`
	PubKey    string  `json:"pubkey,omitempty"`
}

func txToWire(t *Transaction) *txWire {
	return &txWire{ID: t.ID, Sender: t.Sender, Receiver: t.Receiver, Amount: t.Amount, Nonce: t.Nonce, Data: t.Data, Signature: t.Signature, PubKey: t.PubKey}
}

func (tw *txWire) toTx() *Transaction {
	return &Transaction{ID: tw.ID, Sender: tw.Sender, Receiver: tw.Receiver, Amount: tw.Amount, Nonce: tw.Nonce, Data: tw.Data, Signature: tw.Signature, PubKey: tw.PubKey}
}

func blockToWire(b *Block) *blockWire {
	txs := make([]txWire, len(b.Transactions))
	for i, t := range b.Transactions {
		txs[i] = *txToWire(t)
	}
	txsV2 := make([]*protocolv2.TransactionV2, 0, len(b.TransactionsV2))
	for _, tx := range b.TransactionsV2 {
		txsV2 = append(txsV2, tx.Clone())
	}
	return &blockWire{
		Version: b.Version, ProtocolChainID: b.ProtocolChainID, Index: b.Index, TimestampNs: b.Timestamp.UnixNano(),
		Transactions: txs, TransactionsV2: txsV2,
		PrevHash: b.PrevHash, Hash: b.Hash, ProofOfWork: b.ProofOfWork, ProofOfTime: b.ProofOfTime,
		HBTStateRoot: b.HBTStateRoot, HVMStateRoot: b.HVMStateRoot, ReceiptsRoot: b.ReceiptsRoot,
		ValidatorSetRoot: b.ValidatorSetRoot, ValidatorStateRoot: b.ValidatorStateRoot, AuthorValidatorID: b.AuthorValidatorID,
		ProposerID: b.ProposerID, ConsensusRound: b.ConsensusRound, ValidRound: b.ValidRound, ValidPrevoteCertificate: clonePrevoteQC(b.ValidPrevoteCertificate), FinalityCertificate: cloneQC(b.FinalityCertificate),
	}
}

func (bw *blockWire) toBlock() *Block {
	txs := make([]*Transaction, len(bw.Transactions))
	for i := range bw.Transactions {
		txs[i] = bw.Transactions[i].toTx()
	}
	txsV2 := make([]*protocolv2.TransactionV2, 0, len(bw.TransactionsV2))
	for _, tx := range bw.TransactionsV2 {
		txsV2 = append(txsV2, tx.Clone())
	}
	return &Block{
		Version: bw.Version, ProtocolChainID: bw.ProtocolChainID, Index: bw.Index, Timestamp: time.Unix(0, bw.TimestampNs).UTC(),
		Transactions: txs, TransactionsV2: txsV2,
		PrevHash: bw.PrevHash, Hash: bw.Hash, ProofOfWork: bw.ProofOfWork, ProofOfTime: bw.ProofOfTime,
		HBTStateRoot: bw.HBTStateRoot, HVMStateRoot: bw.HVMStateRoot, ReceiptsRoot: bw.ReceiptsRoot,
		ValidatorSetRoot: bw.ValidatorSetRoot, ValidatorStateRoot: bw.ValidatorStateRoot, AuthorValidatorID: bw.AuthorValidatorID,
		ProposerID: bw.ProposerID, ConsensusRound: bw.ConsensusRound, ValidRound: bw.ValidRound, ValidPrevoteCertificate: clonePrevoteQC(bw.ValidPrevoteCertificate), FinalityCertificate: cloneQC(bw.FinalityCertificate),
	}
}

func writeFrame(w io.Writer, msg *syncMessage) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if len(payload) > maxFrameBytes {
		return fmt.Errorf("frame troppo grande: %d byte", len(payload))
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

func readFrame(r io.Reader) (*syncMessage, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > maxFrameBytes {
		return nil, fmt.Errorf("frame dichiarato troppo grande: %d", n)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	var msg syncMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

type Syncer struct {
	bc   *Blockchain
	host host.Host
	mp   *Mempool
	mu   sync.Mutex
}

func NewSyncer(bc *Blockchain, mp *Mempool, h host.Host) *Syncer {
	sy := &Syncer{bc: bc, mp: mp, host: h}
	h.SetStreamHandler(ProtocolID, sy.handleStream)
	return sy
}

func (sy *Syncer) handleStream(s network.Stream) {
	defer s.Close()
	msg, err := readFrame(bufio.NewReader(s))
	if err != nil {
		return
	}
	peerID := s.Conn().RemotePeer()
	if err := sy.dispatch(s, peerID, msg); err != nil {
		log.Printf("sync: %s da %s: %v", msg.Type, peerID, err)
	}
}

func (sy *Syncer) dispatch(s network.Stream, peerID peer.ID, msg *syncMessage) error {
	switch msg.Type {
	case MsgHello:
		return sy.onHello(peerID, msg)
	case MsgGetBlocks:
		return sy.onGetBlocks(s, msg)
	case MsgBlocks, MsgNewBlock:
		return sy.onBlocks(peerID, msg)
	case MsgNewTx:
		return sy.onNewTx(peerID, msg)
	case MsgNewTxV2:
		return sy.onNewTxV2(peerID, msg)
	default:
		return fmt.Errorf("tipo sconosciuto %q", msg.Type)
	}
}

func (sy *Syncer) onHello(peerID peer.ID, msg *syncMessage) error {
	ourTicks := sy.bc.TotalPoHTicks()
	log.Printf("sync: hello ricevuto da %s (suoi ticks=%d, nostri=%d, finalized=%d, consensus=%q)", peerID, msg.PoHTicks, ourTicks, msg.FinalizedHeight, msg.ConsensusProtocolVersion)
	if msg.PoHTicks <= ourTicks {
		return nil
	}
	return sy.requestBlocks(peerID, sy.bc.Height()+1)
}

func (sy *Syncer) onGetBlocks(s network.Stream, msg *syncMessage) error {
	blocks := sy.bc.SnapshotBlocksFrom(msg.FromIndex, maxBlocksBatch)
	out := make([]*blockWire, len(blocks))
	for i, b := range blocks {
		out[i] = blockToWire(b)
	}
	return writeFrame(s, &syncMessage{Type: MsgBlocks, Blocks: out})
}

func (sy *Syncer) onBlocks(peerID peer.ID, msg *syncMessage) error {
	sy.mu.Lock()
	defer sy.mu.Unlock()
	blocks := make([]*Block, 0, len(msg.Blocks))
	for _, bw := range msg.Blocks {
		blocks = append(blocks, bw.toBlock())
	}
	if len(blocks) == 0 {
		return nil
	}
	applied, err := sy.bc.TryExtendOrAdopt(blocks)
	if err != nil {
		return err
	}
	if applied {
		// TryExtendOrAdopt owns reactor reconciliation so every authenticated
		// finalized chain transition has the same pacemaker semantics, regardless
		// of which network path delivered it.
		log.Printf("sync: catena a height=%d (ticks=%d) da %s", sy.bc.Height(), sy.bc.TotalPoHTicks(), peerID)
	}
	return nil
}

func (sy *Syncer) onNewTx(peerID peer.ID, msg *syncMessage) error {
	if msg.Tx == nil {
		return nil
	}
	tx := msg.Tx.toTx()
	if err := tx.Verify(); err != nil {
		return fmt.Errorf("tx gossip non valida: %w", err)
	}
	if sy.bc.ContainsTx(tx.ID) {
		return nil
	}
	before := sy.mp.Size()
	sy.mp.AddTransaction(tx)
	if sy.mp.Size() > before {
		sy.gossipTxExcept(tx, peerID)
	}
	return nil
}

func (sy *Syncer) onNewTxV2(peerID peer.ID, msg *syncMessage) error {
	if msg.TxV2 == nil {
		return nil
	}
	if sy.bc.ContainsTxV2(msg.TxV2.HashHex()) {
		return nil
	}
	if sy.mp.HasTransactionV2(msg.TxV2.HashHex()) {
		return nil
	}
	before := sy.mp.SizeV2()
	if err := sy.bc.AdmitTransactionV2(msg.TxV2); err != nil {
		return fmt.Errorf("tx V2 gossip non valida: %w", err)
	}
	if sy.mp.SizeV2() > before {
		sy.gossipTxV2Except(msg.TxV2, peerID)
	}
	return nil
}

func (sy *Syncer) GossipTx(tx *Transaction) { sy.gossipTxExcept(tx, "") }

func (sy *Syncer) gossipTxExcept(tx *Transaction, except peer.ID) {
	msg := &syncMessage{Type: MsgNewTx, Tx: txToWire(tx)}
	for _, p := range sy.host.Network().Peers() {
		if p == except {
			continue
		}
		go func(pid peer.ID) {
			if err := sy.sendMessage(pid, msg); err != nil {
				log.Printf("sync: gossip tx a %s fallito: %v", pid, err)
			}
		}(p)
	}
}

func (sy *Syncer) GossipTxV2(tx *protocolv2.TransactionV2) { sy.gossipTxV2Except(tx, "") }

func (sy *Syncer) gossipTxV2Except(tx *protocolv2.TransactionV2, except peer.ID) {
	msg := &syncMessage{Type: MsgNewTxV2, TxV2: tx.Clone()}
	for _, p := range sy.host.Network().Peers() {
		if p == except {
			continue
		}
		go func(pid peer.ID) {
			if err := sy.sendMessage(pid, msg); err != nil {
				log.Printf("sync: gossip tx V2 a %s fallito: %v", pid, err)
			}
		}(p)
	}
}

func (sy *Syncer) requestBlocks(peerID peer.ID, fromIndex int) error {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		s, err := sy.host.NewStream(ctx, peerID, ProtocolID)
		if err != nil {
			cancel()
			return err
		}
		if err := writeFrame(s, &syncMessage{Type: MsgGetBlocks, FromIndex: fromIndex}); err != nil {
			s.Close()
			cancel()
			return err
		}
		resp, err := readFrame(bufio.NewReader(s))
		s.Close()
		cancel()
		if err != nil {
			return err
		}
		if resp.Type != MsgBlocks || len(resp.Blocks) == 0 {
			return nil
		}
		blocks := make([]*Block, 0, len(resp.Blocks))
		for _, bw := range resp.Blocks {
			blocks = append(blocks, bw.toBlock())
		}
		sy.mu.Lock()
		applied, err := sy.bc.TryExtendOrAdopt(blocks)
		height := sy.bc.Height()
		ticks := sy.bc.TotalPoHTicks()
		sy.mu.Unlock()
		if err != nil {
			return err
		}
		if applied {
			// Reactor reconciliation is part of TryExtendOrAdopt; catch-up and
			// single-block gossip therefore cannot diverge in pacemaker behavior.
			log.Printf("sync: catena a height=%d (ticks=%d) da %s", height, ticks, peerID)
		}
		if len(resp.Blocks) < maxBlocksBatch {
			return nil
		}
		fromIndex = height + 1
	}
}

func (sy *Syncer) sendMessage(peerID peer.ID, msg *syncMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := sy.host.NewStream(ctx, peerID, ProtocolID)
	if err != nil {
		return err
	}
	defer s.Close()
	return writeFrame(s, msg)
}

func (sy *Syncer) Hello(peerID peer.ID) error {
	head := sy.bc.HeadSnapshot()
	finalizedHeight := sy.bc.FinalizedHeight()
	finalizedHash := ""
	if head != nil && head.Index == finalizedHeight {
		finalizedHash = head.Hash
	}
	return sy.sendMessage(peerID, &syncMessage{
		Type: MsgHello, Height: sy.bc.Height(), PoHTicks: sy.bc.TotalPoHTicks(),
		FinalizedHeight: finalizedHeight, FinalizedHash: finalizedHash,
		ConsensusProtocolVersion: string(ConsensusProtocolID),
	})
}

func (sy *Syncer) HelloAll() {
	peers := sy.host.Network().Peers()
	for _, p := range peers {
		go func(pid peer.ID) {
			if err := sy.Hello(pid); err != nil {
				log.Printf("sync: hello a %s fallito: %v", pid, err)
			}
		}(p)
	}
}

func (sy *Syncer) BroadcastNewBlock(b *Block) {
	msg := &syncMessage{Type: MsgNewBlock, Blocks: []*blockWire{blockToWire(b)}}
	for _, p := range sy.host.Network().Peers() {
		go func(pid peer.ID) {
			if err := sy.sendMessage(pid, msg); err != nil {
				log.Printf("sync: broadcast a %s fallito: %v", pid, err)
			}
		}(p)
	}
}
