package blockchain

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"hashburst/consensus"
	"hashburst/protocolv2"
)

// Block is backward compatible with the current V1 chain. Gob/JSON decoders of
// old blocks leave Version=0; EffectiveVersion treats that as legacy V1.
// Phase 3B V2 blocks add native/HVM/receipt commitments and TransactionV2.
type Block struct {
	Version                 uint16
	ProtocolChainID         uint64
	Index                   int
	Timestamp               time.Time
	Transactions            []*Transaction
	TransactionsV2          []*protocolv2.TransactionV2
	PrevHash                string
	Hash                    string
	ProofOfWork             int64
	ProofOfTime             int64
	HBTStateRoot            string
	HVMStateRoot            string
	ReceiptsRoot            string
	ValidatorSetRoot        string
	ValidatorStateRoot      string
	AuthorValidatorID       string // block-content author/reward owner; committed in block hash
	ProposerID              string // current BFT round proposer; consensus metadata, not block hash
	ConsensusRound          uint64
	ValidRound              int64
	ValidPrevoteCertificate *consensus.PrevoteCertificate
	FinalityCertificate     *consensus.QuorumCertificate
}

func (b *Block) EffectiveVersion() uint16 {
	if b == nil || b.Version == 0 {
		return BlockVersionLegacy
	}
	return b.Version
}

// NewBlock creates a legacy block. Its hash encoding is byte-for-byte the same
// as the pre-Phase-3B chain even though Version is populated in memory.
func NewBlock(transactions []*Transaction, prevHash string, index int, poh int64) *Block {
	if transactions == nil {
		transactions = []*Transaction{}
	}
	return &Block{
		Version:      BlockVersionLegacy,
		Index:        index,
		Timestamp:    time.Now().UTC(),
		Transactions: transactions,
		PrevHash:     prevHash,
		ProofOfTime:  poh,
		ProofOfWork:  0,
	}
}

func NewBlockV2(transactions []*Transaction, transactionsV2 []*protocolv2.TransactionV2, prevHash string, index int, poh int64, protocolChainID uint64) *Block {
	if transactions == nil {
		transactions = []*Transaction{}
	}
	if transactionsV2 == nil {
		transactionsV2 = []*protocolv2.TransactionV2{}
	}
	return &Block{
		Version:         BlockVersionV2,
		ProtocolChainID: protocolChainID,
		Index:           index,
		Timestamp:       time.Now().UTC(),
		Transactions:    transactions,
		TransactionsV2:  transactionsV2,
		PrevHash:        prevHash,
		ProofOfTime:     poh,
		ProofOfWork:     0,
		ValidRound:      -1,
	}
}

func NewGenesisBlock() *Block {
	b := &Block{
		Version:      BlockVersionLegacy,
		Index:        0,
		Timestamp:    time.Unix(0, GenesisTimestampNs).UTC(),
		Transactions: []*Transaction{},
		PrevHash:     GenesisPrevHash,
		ProofOfTime:  0,
		ProofOfWork:  0,
	}
	b.Hash = b.GenerateHash()
	return b
}

func (b *Block) GenerateHash() string {
	if b.EffectiveVersion() < BlockVersionV2 {
		return b.generateLegacyHash()
	}
	return b.generateV2Hash()
}

// generateLegacyHash MUST NOT change: existing block/genesis hashes depend on it.
func (b *Block) generateLegacyHash() string {
	c := newCanonicalBuffer()
	c.putString(ChainID)
	c.putInt64(int64(b.Index))
	c.putInt64(b.Timestamp.UnixNano())
	c.putString(b.PrevHash)
	c.putInt64(b.ProofOfTime)
	c.putInt64(b.ProofOfWork)
	c.putUint64(uint64(len(b.Transactions)))
	for _, tx := range b.Transactions {
		c.putString(tx.HashTransaction())
	}
	sum := sha256.Sum256(c.bytes())
	return hex.EncodeToString(sum[:])
}

func (b *Block) generateV2Hash() string {
	c := newCanonicalBuffer()
	c.putString("HASHBURST_BLOCK_V2")
	// Commit both HashBurst's canonical network domain and the numeric protocol
	// chain ID used by TransactionV2/RPC. This prevents an empty V2 block from
	// being byte-identical across environments configured with different IDs.
	c.putString(ChainID)
	c.putUint64(b.ProtocolChainID)
	c.putUint64(uint64(b.EffectiveVersion()))
	c.putInt64(int64(b.Index))
	c.putInt64(b.Timestamp.UnixNano())
	c.putString(b.PrevHash)
	c.putInt64(b.ProofOfTime)
	c.putInt64(b.ProofOfWork)

	c.putUint64(uint64(len(b.Transactions)))
	for _, tx := range b.Transactions {
		c.putString(tx.HashTransaction())
	}
	c.putUint64(uint64(len(b.TransactionsV2)))
	for _, tx := range b.TransactionsV2 {
		if tx == nil {
			c.putString("")
			continue
		}
		c.putString(tx.HashHex())
		c.putString(tx.Signature)
	}
	c.putString(b.HBTStateRoot)
	c.putString(b.HVMStateRoot)
	c.putString(b.ReceiptsRoot)
	c.putString(b.ValidatorSetRoot)
	c.putString(b.ValidatorStateRoot)
	c.putString(b.AuthorValidatorID)
	// Phase 3D deliberately excludes current-round proposer and round from the
	// block/content hash. A valid block may be safely re-proposed in a later BFT
	// round without changing its identity. AuthorValidatorID remains committed so
	// reward ownership cannot be rewritten during a view change.
	// FinalityCertificate deliberately does not enter the proposal hash. Votes
	// sign this hash; including the certificate would create a circular hash.

	sum := sha256.Sum256(c.bytes())
	return hex.EncodeToString(sum[:])
}
