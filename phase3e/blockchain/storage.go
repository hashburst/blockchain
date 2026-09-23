package blockchain

// storage.go — persistenza blockchain su disco (gob + indice).
//
// MODIFICHE DI QUESTA VERSIONE:
//   - txOnDisk.PubKey: la pubkey delle firme recuperabili va persistita, o al
//     reload la verifica firma fallisce.
//   - SaveBlock spezzato in SaveBlock (locka) + saveBlockLocked (non locka),
//     così Rewrite può riusare la logica senza deadlock.
//   - Rewrite: riscrive dat+idx da zero. Serve al fork-choice quando si adotta
//     un ramo alternativo (chainops.go).

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"hashburst/consensus"
	"hashburst/protocolv2"
)

const (
	defaultStorageDir = "/var/lib/hashburst"
	chainFile         = "blockchain.dat"
	indexFile         = "blockchain.idx"
)

type blockOnDisk struct {
	Version                 uint16
	ProtocolChainID         uint64
	Index                   int
	TimestampNs             int64
	Transactions            []txOnDisk
	TransactionsV2          []txV2OnDisk
	PrevHash                string
	Hash                    string
	ProofOfWork             int64
	ProofOfTime             int64
	HBTStateRoot            string
	HVMStateRoot            string
	ReceiptsRoot            string
	ValidatorSetRoot        string
	ValidatorStateRoot      string
	AuthorValidatorID       string
	ProposerID              string
	ConsensusRound          uint64
	ValidRound              int64
	ValidPrevoteCertificate *consensus.PrevoteCertificate
	FinalityCertificate     *consensus.QuorumCertificate
}

type txOnDisk struct {
	ID        string
	Sender    string
	Receiver  string
	Amount    float64
	Data      string
	Nonce     int64
	PubKey    string
	Signature string
}

type txV2OnDisk struct {
	Version      uint16
	ChainID      uint64
	Type         uint16
	Sender       string
	To           string
	ValueUnits   int64
	Sequence     uint64
	ComputeLimit uint64
	MaxFeeUnits  int64
	Data         []byte
	Signature    string
	ID           string
}

type indexEntry struct {
	BlockNum uint64
	Offset   int64
	Size     uint32
}

type ChainStorage struct {
	durable bool // Strict persistent runtime: sync data before publishing its index.
	dir     string
	datPath string
	idxPath string
	mu      sync.Mutex
}

func NewChainStorage(dir string) *ChainStorage {
	if dir == "" {
		dir = defaultStorageDir
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("storage: cannot create dir %s: %v", dir, err)
	}
	return &ChainStorage{
		dir:     dir,
		datPath: filepath.Join(dir, chainFile),
		idxPath: filepath.Join(dir, indexFile),
	}
}

// SaveBlock aggiunge un blocco (prende il lock).
func (s *ChainStorage) SaveBlock(b *Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveBlockLocked(b)
}

// saveBlockLocked contiene la logica; presuppone il lock già preso.
func (s *ChainStorage) saveBlockLocked(b *Block) error {
	bod := blockToOnDisk(b)
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(bod); err != nil {
		return fmt.Errorf("encode block: %w", err)
	}
	data := buf.Bytes()

	f, err := os.OpenFile(s.datPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open dat: %w", err)
	}
	defer f.Close()

	offset, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("seek: %w", err)
	}

	sizeBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(sizeBuf, uint32(len(data)))
	if _, err := f.Write(sizeBuf); err != nil {
		return fmt.Errorf("write size: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write data: %w", err)
	}

	if s.durable {
		if err := f.Sync(); err != nil {
			return err
		}
	}
	return s.appendIndex(indexEntry{
		BlockNum: uint64(b.Index),
		Offset:   offset,
		Size:     uint32(len(data) + 4),
	})
}

// Rewrite sostituisce l'intero storage con la catena data. Usato dal
// fork-choice quando si adotta un ramo con più tick PoH.
func (s *ChainStorage) Rewrite(blocks []*Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Tronca entrambi i file, poi riscrive dal primo blocco.
	if err := os.Truncate(s.datPath, 0); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("truncate dat: %w", err)
	}
	if err := os.Truncate(s.idxPath, 0); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("truncate idx: %w", err)
	}
	for _, b := range blocks {
		if err := s.saveBlockLocked(b); err != nil {
			return fmt.Errorf("rewrite block #%d: %w", b.Index, err)
		}
	}
	return nil
}

func (s *ChainStorage) appendIndex(e indexEntry) error {
	f, err := os.OpenFile(s.idxPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, 20)
	binary.BigEndian.PutUint64(buf[0:8], e.BlockNum)
	binary.BigEndian.PutUint64(buf[8:16], uint64(e.Offset))
	binary.BigEndian.PutUint32(buf[16:20], e.Size)
	_, err = f.Write(buf)
	if err == nil && s.durable {
		err = f.Sync()
	}
	return err
}

func (s *ChainStorage) LoadAll() ([]*Block, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.datPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open dat: %w", err)
	}
	defer f.Close()

	blocks := []*Block{}
	for {
		sizeBuf := make([]byte, 4)
		if _, err := io.ReadFull(f, sizeBuf); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("read size: %w", err)
		}
		size := binary.BigEndian.Uint32(sizeBuf)

		data := make([]byte, size)
		if _, err := io.ReadFull(f, data); err != nil {
			return nil, fmt.Errorf("read data: %w", err)
		}

		var bod blockOnDisk
		if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&bod); err != nil {
			return nil, fmt.Errorf("decode block: %w", err)
		}
		blocks = append(blocks, blockFromOnDisk(&bod))
	}
	return blocks, nil
}

func (s *ChainStorage) Exists() bool {
	_, err := os.Stat(s.datPath)
	return err == nil
}

func (s *ChainStorage) Stats() map[string]interface{} {
	info, err := os.Stat(s.datPath)
	if err != nil {
		return map[string]interface{}{"exists": false}
	}
	return map[string]interface{}{
		"exists":     true,
		"size_mb":    float64(info.Size()) / 1024 / 1024,
		"size_bytes": info.Size(),
		"modified":   info.ModTime().Format(time.RFC3339),
	}
}

func blockToOnDisk(b *Block) blockOnDisk {
	txs := make([]txOnDisk, len(b.Transactions))
	for i, tx := range b.Transactions {
		txs[i] = txOnDisk{
			ID: tx.ID, Sender: tx.Sender, Receiver: tx.Receiver,
			Amount: tx.Amount, Nonce: tx.Nonce, Data: tx.Data,
			PubKey: tx.PubKey, Signature: tx.Signature,
		}
	}
	txsV2 := make([]txV2OnDisk, len(b.TransactionsV2))
	for i, tx := range b.TransactionsV2 {
		if tx == nil {
			continue
		}
		txsV2[i] = txV2OnDisk{
			Version: tx.Version, ChainID: tx.ChainID, Type: uint16(tx.Type),
			Sender: tx.Sender, To: tx.To, ValueUnits: tx.ValueUnits,
			Sequence: tx.Sequence, ComputeLimit: tx.ComputeLimit, MaxFeeUnits: tx.MaxFeeUnits,
			Data: append([]byte(nil), tx.Data...), Signature: tx.Signature, ID: tx.ID,
		}
	}
	return blockOnDisk{
		Version: b.Version, ProtocolChainID: b.ProtocolChainID, Index: b.Index, TimestampNs: b.Timestamp.UnixNano(),
		Transactions: txs, TransactionsV2: txsV2,
		PrevHash: b.PrevHash, Hash: b.Hash, ProofOfWork: b.ProofOfWork, ProofOfTime: b.ProofOfTime,
		HBTStateRoot: b.HBTStateRoot, HVMStateRoot: b.HVMStateRoot, ReceiptsRoot: b.ReceiptsRoot,
		ValidatorSetRoot: b.ValidatorSetRoot, ValidatorStateRoot: b.ValidatorStateRoot, AuthorValidatorID: b.AuthorValidatorID,
		ProposerID: b.ProposerID, ConsensusRound: b.ConsensusRound, ValidRound: b.ValidRound, ValidPrevoteCertificate: clonePrevoteQC(b.ValidPrevoteCertificate), FinalityCertificate: cloneQC(b.FinalityCertificate),
	}
}

func blockFromOnDisk(bod *blockOnDisk) *Block {
	txs := make([]*Transaction, len(bod.Transactions))
	for i, t := range bod.Transactions {
		txs[i] = &Transaction{
			ID: t.ID, Sender: t.Sender, Receiver: t.Receiver,
			Amount: t.Amount, Nonce: t.Nonce, Data: t.Data,
			PubKey: t.PubKey, Signature: t.Signature,
		}
	}
	txsV2 := make([]*protocolv2.TransactionV2, len(bod.TransactionsV2))
	for i, t := range bod.TransactionsV2 {
		txsV2[i] = &protocolv2.TransactionV2{
			Version: t.Version, ChainID: t.ChainID, Type: protocolv2.TxType(t.Type),
			Sender: t.Sender, To: t.To, ValueUnits: t.ValueUnits,
			Sequence: t.Sequence, ComputeLimit: t.ComputeLimit, MaxFeeUnits: t.MaxFeeUnits,
			Data: append([]byte(nil), t.Data...), Signature: t.Signature, ID: t.ID,
		}
	}
	return &Block{
		Version: bod.Version, ProtocolChainID: bod.ProtocolChainID, Index: bod.Index, Timestamp: time.Unix(0, bod.TimestampNs).UTC(),
		Transactions: txs, TransactionsV2: txsV2,
		PrevHash: bod.PrevHash, Hash: bod.Hash, ProofOfWork: bod.ProofOfWork, ProofOfTime: bod.ProofOfTime,
		HBTStateRoot: bod.HBTStateRoot, HVMStateRoot: bod.HVMStateRoot, ReceiptsRoot: bod.ReceiptsRoot,
		ValidatorSetRoot: bod.ValidatorSetRoot, ValidatorStateRoot: bod.ValidatorStateRoot, AuthorValidatorID: bod.AuthorValidatorID,
		ProposerID: bod.ProposerID, ConsensusRound: bod.ConsensusRound, ValidRound: bod.ValidRound, ValidPrevoteCertificate: clonePrevoteQC(bod.ValidPrevoteCertificate), FinalityCertificate: cloneQC(bod.FinalityCertificate),
	}
}

func cloneQC(qc *consensus.QuorumCertificate) *consensus.QuorumCertificate {
	if qc == nil {
		return nil
	}
	out := *qc
	out.Votes = append([]consensus.Vote(nil), qc.Votes...)
	return &out
}

func clonePrevoteQC(qc *consensus.PrevoteCertificate) *consensus.PrevoteCertificate {
	if qc == nil {
		return nil
	}
	out := *qc
	out.Votes = append([]consensus.Prevote(nil), qc.Votes...)
	return &out
}
