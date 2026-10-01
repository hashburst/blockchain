// HBX2 storage codec for the existing Block type. It does not change consensus
// serialization, hashing, signing, or the current gob ledger. Timestamps preserve
// the instant and normalize to UTC; process-local monotonic clocks are not stored.
package blockchain

import (
	"encoding/binary"
	"errors"
	"hashburst/consensus"
	"hashburst/protocolv2"
	"math"
	"sync"
	"time"
)

const ledgerRecordLimit = 16 << 20

var errLedgerEncoding = errors.New("invalid or oversized HBX2 block")

type ledgerEncoder struct {
	b   []byte
	err error
}

func (e *ledgerEncoder) raw(b []byte) {
	if e.err != nil {
		return
	}
	if len(b) > ledgerRecordLimit-len(e.b) {
		e.err = errLedgerEncoding
		return
	}
	e.b = append(e.b, b...)
}
func (e *ledgerEncoder) u8(v uint8) { e.raw([]byte{v}) }
func (e *ledgerEncoder) u16(v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	e.raw(b[:])
}
func (e *ledgerEncoder) u32(v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	e.raw(b[:])
}
func (e *ledgerEncoder) u64(v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	e.raw(b[:])
}
func (e *ledgerEncoder) presence(v bool) {
	if v {
		e.u8(1)
	} else {
		e.u8(0)
	}
}
func (e *ledgerEncoder) count(n int, nilValue bool) {
	if nilValue {
		e.u32(math.MaxUint32)
	} else if n > 65536 {
		e.err = errLedgerEncoding
	} else {
		e.u32(uint32(n))
	}
}
func (e *ledgerEncoder) str(s string) {
	if len(s) > ledgerRecordLimit {
		e.err = errLedgerEncoding
		return
	}
	e.u32(uint32(len(s)))
	e.raw([]byte(s))
}
func (e *ledgerEncoder) bytes(b []byte) {
	if b == nil {
		e.u32(math.MaxUint32)
		return
	}
	if len(b) > ledgerRecordLimit {
		e.err = errLedgerEncoding
		return
	}
	e.u32(uint32(len(b)))
	e.raw(b)
}

type ledgerDecoder struct {
	b         []byte
	err       error
	remaining int
}

func (d *ledgerDecoder) fail() { d.err = errLedgerEncoding }
func (d *ledgerDecoder) budget(n int) {
	if n < 0 || n > d.remaining {
		d.fail()
		return
	}
	d.remaining -= n
}
func (d *ledgerDecoder) raw(n int) []byte {
	if d.err != nil || n < 0 || n > len(d.b) {
		d.fail()
		return nil
	}
	b := d.b[:n]
	d.b = d.b[n:]
	return b
}
func (d *ledgerDecoder) u8() uint8 {
	b := d.raw(1)
	if b == nil {
		return 0
	}
	return b[0]
}
func (d *ledgerDecoder) u16() uint16 {
	b := d.raw(2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}
func (d *ledgerDecoder) u32() uint32 {
	b := d.raw(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}
func (d *ledgerDecoder) u64() uint64 {
	b := d.raw(8)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}
func (d *ledgerDecoder) presence() bool {
	v := d.u8()
	if v > 1 {
		d.fail()
	}
	return v == 1
}
func (d *ledgerDecoder) count() int {
	n := d.u32()
	if n == math.MaxUint32 {
		return -1
	}
	if n > 65536 || uint64(n) > uint64(len(d.b)) {
		d.fail()
		return -1
	}
	return int(n)
}
func (d *ledgerDecoder) str() string {
	n := d.u32()
	if uint64(n) > uint64(len(d.b)) {
		d.fail()
		return ""
	}
	d.budget(int(n))
	if d.err != nil {
		return ""
	}
	return string(d.raw(int(n)))
}
func (d *ledgerDecoder) bytes() []byte {
	n := d.u32()
	if n == math.MaxUint32 {
		return nil
	}
	if uint64(n) > uint64(len(d.b)) {
		d.fail()
		return nil
	}
	d.budget(int(n))
	if d.err != nil {
		return nil
	}
	b := make([]byte, int(n))
	copy(b, d.raw(int(n)))
	return b
}

// EncodeLedgerBlock appends a versioned storage payload to reusable caller scratch.
func EncodeLedgerBlock(dst []byte, b *Block) ([]byte, error) {
	if b == nil {
		return nil, errLedgerEncoding
	}
	e := ledgerEncoder{b: dst}
	e.raw([]byte("HBB2"))
	e.Block(b)
	if e.err != nil {
		return nil, e.err
	}
	return e.b, nil
}

// DecodeLedgerBlockInto owns all decoded strings/slices; no mmap references escape.
func DecodeLedgerBlockInto(data []byte, out *Block) error {
	if out == nil {
		return errLedgerEncoding
	}
	*out = Block{}
	if len(data) > ledgerRecordLimit || len(data) < 4 || string(data[:4]) != "HBB2" {
		return errLedgerEncoding
	}
	d := ledgerDecoder{b: data[4:], remaining: 64 << 20}
	d.Block(out)
	if len(d.b) != 0 {
		d.fail()
	}
	if d.err != nil {
		*out = Block{}
	}
	return d.err
}

var ledgerBlocks = sync.Pool{New: func() any { return new(Block) }}

// WithLedgerBlock borrows a decoded Block until fn returns. fn must not retain or
// mutate the block, its nested fields, or share it with another goroutine. The pool
// recycles the shell only: nested allocations are cleared, not retained unbounded.
func WithLedgerBlock(data []byte, fn func(*Block) error) error {
	b := ledgerBlocks.Get().(*Block)
	defer func() { *b = Block{}; ledgerBlocks.Put(b) }()
	if e := DecodeLedgerBlockInto(data, b); e != nil {
		return e
	}
	return fn(b)
}
func (e *ledgerEncoder) Block(v *Block) {
	e.presence(v.APoW != nil)
	if v.APoW != nil {
		e.APoWProof(v.APoW)
	}
	e.count(len(v.EthereumTransactions), v.EthereumTransactions == nil)
	for _, v := range v.EthereumTransactions {
		if e.err != nil {
			break
		}
		e.bytes(v)
	}
	e.str(v.EVMStateRoot)
	e.str(v.EVMReceiptsRoot)
	e.u64(uint64(v.EVMGasUsed))
	e.u16(uint16(v.Version))
	e.u64(uint64(v.ProtocolChainID))
	e.u64(uint64(v.Index))
	e.u64(uint64(v.Timestamp.Unix()))
	e.u32(uint32(v.Timestamp.Nanosecond()))
	e.count(len(v.Transactions), v.Transactions == nil)
	for _, v := range v.Transactions {
		if e.err != nil {
			break
		}
		e.presence(v != nil)
		if v != nil {
			e.Transaction(v)
		}
	}
	e.count(len(v.TransactionsV2), v.TransactionsV2 == nil)
	for _, v := range v.TransactionsV2 {
		if e.err != nil {
			break
		}
		e.presence(v != nil)
		if v != nil {
			e.protocolv2TransactionV2(v)
		}
	}
	e.str(v.PrevHash)
	e.str(v.Hash)
	e.u64(uint64(v.ProofOfWork))
	e.u64(uint64(v.ProofOfTime))
	e.str(v.HBTStateRoot)
	e.str(v.HVMStateRoot)
	e.str(v.ReceiptsRoot)
	e.str(v.ValidatorSetRoot)
	e.str(v.ValidatorStateRoot)
	e.str(v.AuthorValidatorID)
	e.str(v.ProposerID)
	e.u64(uint64(v.ConsensusRound))
	e.u64(uint64(v.ValidRound))
	e.presence(v.ValidPrevoteCertificate != nil)
	if v.ValidPrevoteCertificate != nil {
		e.consensusPrevoteCertificate(v.ValidPrevoteCertificate)
	}
	e.presence(v.FinalityCertificate != nil)
	if v.FinalityCertificate != nil {
		e.consensusQuorumCertificate(v.FinalityCertificate)
	}
}
func (d *ledgerDecoder) Block(v *Block) {
	if d.presence() {
		d.budget(512)
		if d.err == nil {
			v.APoW = new(APoWProof)
			d.APoWProof(v.APoW)
		}
	}
	if n := d.count(); n >= 0 && d.err == nil {
		d.budget(n * 512)
		if d.err == nil {
			v.EthereumTransactions = make([][]byte, n)
			for i := range v.EthereumTransactions {
				if d.err != nil {
					break
				}
				v.EthereumTransactions[i] = d.bytes()
			}
		}
	}
	v.EVMStateRoot = d.str()
	v.EVMReceiptsRoot = d.str()
	v.EVMGasUsed = uint64(d.u64())
	v.Version = uint16(d.u16())
	v.ProtocolChainID = uint64(d.u64())
	{
		indexValue := int64(d.u64())
		v.Index = int(indexValue)
		if int64(v.Index) != indexValue {
			d.fail()
		}
	}
	{
		sec := int64(d.u64())
		ns := d.u32()
		if ns >= 1e9 {
			d.fail()
		}
		v.Timestamp = time.Unix(sec, int64(ns)).UTC()
	}
	if n := d.count(); n >= 0 && d.err == nil {
		d.budget(n * 512)
		if d.err == nil {
			v.Transactions = make([]*Transaction, n)
			for i := range v.Transactions {
				if d.err != nil {
					break
				}
				if d.presence() {
					d.budget(512)
					if d.err == nil {
						v.Transactions[i] = new(Transaction)
						d.Transaction(v.Transactions[i])
					}
				}
			}
		}
	}
	if n := d.count(); n >= 0 && d.err == nil {
		d.budget(n * 512)
		if d.err == nil {
			v.TransactionsV2 = make([]*protocolv2.TransactionV2, n)
			for i := range v.TransactionsV2 {
				if d.err != nil {
					break
				}
				if d.presence() {
					d.budget(512)
					if d.err == nil {
						v.TransactionsV2[i] = new(protocolv2.TransactionV2)
						d.protocolv2TransactionV2(v.TransactionsV2[i])
					}
				}
			}
		}
	}
	v.PrevHash = d.str()
	v.Hash = d.str()
	v.ProofOfWork = int64(d.u64())
	v.ProofOfTime = int64(d.u64())
	v.HBTStateRoot = d.str()
	v.HVMStateRoot = d.str()
	v.ReceiptsRoot = d.str()
	v.ValidatorSetRoot = d.str()
	v.ValidatorStateRoot = d.str()
	v.AuthorValidatorID = d.str()
	v.ProposerID = d.str()
	v.ConsensusRound = uint64(d.u64())
	v.ValidRound = int64(d.u64())
	if d.presence() {
		d.budget(512)
		if d.err == nil {
			v.ValidPrevoteCertificate = new(consensus.PrevoteCertificate)
			d.consensusPrevoteCertificate(v.ValidPrevoteCertificate)
		}
	}
	if d.presence() {
		d.budget(512)
		if d.err == nil {
			v.FinalityCertificate = new(consensus.QuorumCertificate)
			d.consensusQuorumCertificate(v.FinalityCertificate)
		}
	}
}
func (e *ledgerEncoder) APoWProof(v *APoWProof) {
	e.u64(uint64(v.ChainID))
	e.u64(uint64(v.Height))
	e.str(v.ParentHash)
	e.u64(uint64(v.PoH))
	e.u8(v.Bits)
	e.u64(uint64(v.EpochStart))
	e.str(v.Author)
	e.str(v.Beneficiary)
	e.u64(uint64(v.Nonce))
	e.str(v.Signature)
}
func (d *ledgerDecoder) APoWProof(v *APoWProof) {
	v.ChainID = uint64(d.u64())
	v.Height = uint64(d.u64())
	v.ParentHash = d.str()
	v.PoH = int64(d.u64())
	v.Bits = d.u8()
	v.EpochStart = int64(d.u64())
	v.Author = d.str()
	v.Beneficiary = d.str()
	v.Nonce = uint64(d.u64())
	v.Signature = d.str()
}
func (e *ledgerEncoder) Transaction(v *Transaction) {
	e.str(v.ID)
	e.str(v.Sender)
	e.str(v.Receiver)
	e.u64(math.Float64bits(v.Amount))
	e.u64(uint64(v.Nonce))
	e.str(v.Data)
	e.str(v.Signature)
	e.str(v.PubKey)
}
func (d *ledgerDecoder) Transaction(v *Transaction) {
	v.ID = d.str()
	v.Sender = d.str()
	v.Receiver = d.str()
	v.Amount = math.Float64frombits(d.u64())
	v.Nonce = int64(d.u64())
	v.Data = d.str()
	v.Signature = d.str()
	v.PubKey = d.str()
}
func (e *ledgerEncoder) protocolv2TransactionV2(v *protocolv2.TransactionV2) {
	e.u16(uint16(v.Version))
	e.u64(uint64(v.ChainID))
	e.u16(uint16(v.Type))
	e.str(v.Sender)
	e.str(v.To)
	e.u64(uint64(v.ValueUnits))
	e.u64(uint64(v.Sequence))
	e.u64(uint64(v.ComputeLimit))
	e.u64(uint64(v.MaxFeeUnits))
	e.bytes(v.Data)
	e.str(v.Signature)
	e.str(v.ID)
}
func (d *ledgerDecoder) protocolv2TransactionV2(v *protocolv2.TransactionV2) {
	v.Version = uint16(d.u16())
	v.ChainID = uint64(d.u64())
	v.Type = protocolv2.TxType(d.u16())
	v.Sender = d.str()
	v.To = d.str()
	v.ValueUnits = int64(d.u64())
	v.Sequence = uint64(d.u64())
	v.ComputeLimit = uint64(d.u64())
	v.MaxFeeUnits = int64(d.u64())
	v.Data = d.bytes()
	v.Signature = d.str()
	v.ID = d.str()
}
func (e *ledgerEncoder) consensusVote(v *consensus.Vote) {
	e.u64(uint64(v.ChainID))
	e.u64(uint64(v.Height))
	e.u64(uint64(v.Round))
	e.str(v.BlockHash)
	e.str(v.ValidatorSetRoot)
	e.str(v.ValidatorID)
	e.str(v.Signature)
}
func (d *ledgerDecoder) consensusVote(v *consensus.Vote) {
	v.ChainID = uint64(d.u64())
	v.Height = uint64(d.u64())
	v.Round = uint64(d.u64())
	v.BlockHash = d.str()
	v.ValidatorSetRoot = d.str()
	v.ValidatorID = d.str()
	v.Signature = d.str()
}
func (e *ledgerEncoder) consensusPrevote(v *consensus.Prevote) {
	e.u64(uint64(v.ChainID))
	e.u64(uint64(v.Height))
	e.u64(uint64(v.Round))
	e.str(v.BlockHash)
	e.str(v.ValidatorSetRoot)
	e.str(v.ValidatorID)
	e.str(v.Signature)
}
func (d *ledgerDecoder) consensusPrevote(v *consensus.Prevote) {
	v.ChainID = uint64(d.u64())
	v.Height = uint64(d.u64())
	v.Round = uint64(d.u64())
	v.BlockHash = d.str()
	v.ValidatorSetRoot = d.str()
	v.ValidatorID = d.str()
	v.Signature = d.str()
}
func (e *ledgerEncoder) consensusQuorumCertificate(v *consensus.QuorumCertificate) {
	e.u64(uint64(v.ChainID))
	e.u64(uint64(v.Height))
	e.u64(uint64(v.Round))
	e.str(v.BlockHash)
	e.str(v.ValidatorSetRoot)
	e.u64(uint64(v.SignedPower))
	e.u64(uint64(v.TotalPower))
	e.count(len(v.Votes), v.Votes == nil)
	for _, v := range v.Votes {
		if e.err != nil {
			break
		}
		e.consensusVote(&v)
	}
}
func (d *ledgerDecoder) consensusQuorumCertificate(v *consensus.QuorumCertificate) {
	v.ChainID = uint64(d.u64())
	v.Height = uint64(d.u64())
	v.Round = uint64(d.u64())
	v.BlockHash = d.str()
	v.ValidatorSetRoot = d.str()
	v.SignedPower = uint64(d.u64())
	v.TotalPower = uint64(d.u64())
	if n := d.count(); n >= 0 && d.err == nil {
		d.budget(n * 512)
		if d.err == nil {
			v.Votes = make([]consensus.Vote, n)
			for i := range v.Votes {
				if d.err != nil {
					break
				}
				d.consensusVote(&v.Votes[i])
			}
		}
	}
}
func (e *ledgerEncoder) consensusPrevoteCertificate(v *consensus.PrevoteCertificate) {
	e.u64(uint64(v.ChainID))
	e.u64(uint64(v.Height))
	e.u64(uint64(v.Round))
	e.str(v.BlockHash)
	e.str(v.ValidatorSetRoot)
	e.u64(uint64(v.SignedPower))
	e.u64(uint64(v.TotalPower))
	e.count(len(v.Votes), v.Votes == nil)
	for _, v := range v.Votes {
		if e.err != nil {
			break
		}
		e.consensusPrevote(&v)
	}
}
func (d *ledgerDecoder) consensusPrevoteCertificate(v *consensus.PrevoteCertificate) {
	v.ChainID = uint64(d.u64())
	v.Height = uint64(d.u64())
	v.Round = uint64(d.u64())
	v.BlockHash = d.str()
	v.ValidatorSetRoot = d.str()
	v.SignedPower = uint64(d.u64())
	v.TotalPower = uint64(d.u64())
	if n := d.count(); n >= 0 && d.err == nil {
		d.budget(n * 512)
		if d.err == nil {
			v.Votes = make([]consensus.Prevote, n)
			for i := range v.Votes {
				if d.err != nil {
					break
				}
				d.consensusPrevote(&v.Votes[i])
			}
		}
	}
}
