package blockchain

// APoW work admission is independent of BFT voting. A work proof earns no
// balance until its containing block is finalized by the existing quorum path.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"math"
	"math/big"
	"strings"

	"hashburst/wallet"
)

var ErrAPoWUnavailable = errors.New("APoW work not available for current parent")

// APoWConfig is a consensus change, disabled when absent. Parameters must be
// identical in every validator's pinned configuration before activation.
type APoWConfig struct {
	GasLimit         uint64 `json:"gas_limit,omitempty"` // zero inherits original EVM limit; otherwise activates with APoW
	ActivationHeight uint64 `json:"activation_height"`
	InitialBits      uint8  `json:"initial_bits"`
	MinBits          uint8  `json:"min_bits"`
	MaxBits          uint8  `json:"max_bits"`
	Window           uint64 `json:"window"`
	TargetSeconds    uint64 `json:"target_seconds"`
}

func (c ProtocolV2Config) APoWEnabledAt(h int) bool {
	return c.APoW != nil && h >= 0 && uint64(h) >= c.APoW.ActivationHeight
}
func (c ProtocolV2Config) validateAPoWConfig() error {
	a := c.APoW
	if a == nil {
		return nil
	}
	if a.ActivationHeight == 0 || a.ActivationHeight == DisabledActivationHeight || !c.EVMEnabledAt(int(a.ActivationHeight)) || !c.ConsensusEnabledAt(int(a.ActivationHeight)) {
		return fmt.Errorf("APoW requires explicit EVM and BFT activation")
	}
	if a.GasLimit != 0 && (a.GasLimit < c.EVM.GasLimit || a.GasLimit > 30_000_000) {
		return fmt.Errorf("APoW gas limit must retain/increase original limit within 30000000")
	}
	if a.MinBits < 1 || a.MaxBits > 240 || a.MinBits > a.InitialBits || a.InitialBits > a.MaxBits || a.Window < 2 || a.Window > 100000 || a.TargetSeconds < 1 || a.TargetSeconds > 86400 {
		return fmt.Errorf("invalid APoW retarget parameters")
	}
	return nil
}

// APoWProof binds the author and beneficiary before hashing. Relaying a proof
// cannot redirect its reward. Parent binding prevents reuse at another height.
// EpochStart is inherited from the verified parent, not supplied by a miner.
type APoWProof struct {
	ChainID     uint64 `json:"chain_id"`
	Height      uint64 `json:"height"`
	ParentHash  string `json:"parent_hash"`
	PoH         int64  `json:"poh"`
	Bits        uint8  `json:"bits"`
	EpochStart  int64  `json:"epoch_start"`
	Author      string `json:"author"`
	Beneficiary string `json:"beneficiary"`
	Nonce       uint64 `json:"nonce"`
	Signature   string `json:"signature"`
}

func (p APoWProof) Digest() [32]byte {
	c := newCanonicalBuffer()
	c.putString("HASHBURST_APOW_SHA256_V1")
	c.putUint64(p.ChainID)
	c.putUint64(p.Height)
	c.putString(p.ParentHash)
	c.putInt64(p.PoH)
	c.putUint64(uint64(p.Bits))
	c.putInt64(p.EpochStart)
	c.putString(p.Author)
	c.putString(p.Beneficiary)
	c.putUint64(p.Nonce)
	return sha256.Sum256(c.bytes())
}
func (p APoWProof) meetsTarget() bool {
	if p.Bits < 1 || p.Bits > 240 {
		return false
	}
	h := p.Digest()
	for i := uint8(0); i < p.Bits; i++ {
		if h[i/8]&(0x80>>(i%8)) != 0 {
			return false
		}
	}
	return true
}
func (p APoWProof) Verify() error {
	if !wallet.IsValidAddress(p.Author) || !wallet.IsValidAddress(p.Beneficiary) || p.Author != strings.ToLower(p.Author) || p.Beneficiary != strings.ToLower(p.Beneficiary) || p.Beneficiary == "0x0000000000000000000000000000000000000000" {
		return fmt.Errorf("APoW requires canonical nonzero reward address and author")
	}
	if len(p.ParentHash) != 64 || p.ParentHash != strings.ToLower(p.ParentHash) {
		return fmt.Errorf("invalid APoW parent")
	}
	if _, err := hex.DecodeString(p.ParentHash); err != nil {
		return err
	}
	if !p.meetsTarget() {
		return fmt.Errorf("insufficient APoW work")
	}
	if len(p.Signature) != 130 || p.Signature != strings.ToLower(p.Signature) {
		return fmt.Errorf("invalid APoW signature encoding")
	}
	sig, err := hex.DecodeString(p.Signature)
	if err != nil {
		return err
	}
	if !crypto.ValidateSignatureValues(sig[64], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:64]), true) {
		return fmt.Errorf("noncanonical APoW signature")
	}
	h := p.Digest()
	author, err := wallet.RecoverAddress(h[:], sig)
	if err != nil || !strings.EqualFold(author, p.Author) {
		return fmt.Errorf("APoW author signature mismatch")
	}
	return nil
}
func MineAPoW(ctx context.Context, p APoWProof, signer *wallet.Wallet, beneficiary string) (*APoWProof, error) {
	if signer == nil {
		return nil, fmt.Errorf("missing work signer")
	}
	p.Author = strings.ToLower(signer.Address())
	p.Beneficiary = strings.ToLower(beneficiary)
	p.Signature = ""
	if p.Bits < 1 || p.Bits > 240 || !wallet.IsValidAddress(p.Beneficiary) {
		return nil, fmt.Errorf("invalid work job")
	}
	for {
		if p.Nonce%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if p.meetsTarget() {
			h := p.Digest()
			sig, err := signer.Sign(h[:])
			if err != nil {
				return nil, err
			}
			p.Signature = hex.EncodeToString(sig)
			return &p, p.Verify()
		}
		if p.Nonce == math.MaxUint64 {
			return nil, fmt.Errorf("nonce space exhausted")
		}
		p.Nonce++
	}
}

// Retarget uses only finalized parent data; no wall clock, host load or floating
// point. At a window boundary change at most one bit (factor two), within bounds.
func expectedAPoW(prev *Block, cfg ProtocolV2Config) (uint8, int64, error) {
	a := cfg.APoW
	if prev == nil || !cfg.APoWEnabledAt(prev.Index+1) {
		return 0, 0, fmt.Errorf("APoW inactive")
	}
	if uint64(prev.Index+1) == a.ActivationHeight {
		return a.InitialBits, prev.Timestamp.Unix(), nil
	}
	if prev.APoW == nil {
		return 0, 0, fmt.Errorf("APoW parent proof missing")
	}
	bits, start := prev.APoW.Bits, prev.APoW.EpochStart
	if (uint64(prev.Index+1)-a.ActivationHeight)%a.Window == 0 {
		elapsed := prev.Timestamp.Unix() - start
		target := int64(a.Window * a.TargetSeconds)
		if elapsed < target/2 && bits < a.MaxBits {
			bits++
		} else if elapsed > target*2 && bits > a.MinBits {
			bits--
		}
		start = prev.Timestamp.Unix()
	}
	return bits, start, nil
}
func validateAPoWEnvelope(prev, b *Block, cfg ProtocolV2Config) error {
	if !cfg.APoWEnabledAt(b.Index) {
		if b.APoW != nil {
			return fmt.Errorf("APoW proof before activation")
		}
		return nil
	}
	if b.EffectiveVersion() != BlockVersionAPoW || b.APoW == nil {
		return fmt.Errorf("APoW block/proof required")
	}
	bits, start, err := expectedAPoW(prev, cfg)
	if err != nil {
		return err
	}
	p := b.APoW
	if p.ChainID != cfg.ChainID || p.Height != uint64(b.Index) || p.ParentHash != prev.Hash || p.PoH != b.ProofOfTime || p.Bits != bits || p.EpochStart != start {
		return fmt.Errorf("APoW challenge or retarget mismatch")
	}
	if err := p.Verify(); err != nil {
		return err
	}
	return validateRewardRecipient(b, p.Beneficiary)
}
func (bc *Blockchain) APoWJob() (*APoWProof, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	prev := bc.headLocked()
	bits, start, err := expectedAPoW(prev, bc.v2Config)
	if err != nil {
		return nil, err
	}
	return &APoWProof{ChainID: bc.v2Config.ChainID, Height: uint64(prev.Index + 1), ParentHash: prev.Hash, PoH: poHWithTicks(prev.ProofOfTime, bc.v2Config.EffectivePoHTicks()), Bits: bits, EpochStart: start}, nil
}

// One bounded candidate per current parent. Selection is local policy, not a
// claim of fair allocation: consensus accepts any valid proof for that parent.
func (bc *Blockchain) SubmitAPoW(p APoWProof) error {
	if err := p.Verify(); err != nil {
		return err
	}
	bc.mu.Lock()
	defer bc.mu.Unlock()
	prev := bc.headLocked()
	bits, start, err := expectedAPoW(prev, bc.v2Config)
	if err != nil {
		return err
	}
	if p.ChainID != bc.v2Config.ChainID || p.Height != uint64(prev.Index+1) || p.ParentHash != prev.Hash || p.Bits != bits || p.EpochStart != start || p.PoH != poHWithTicks(prev.ProofOfTime, bc.v2Config.EffectivePoHTicks()) {
		return fmt.Errorf("stale or foreign APoW proof")
	}
	if old := bc.apowWork; old != nil && old.ParentHash == p.ParentHash {
		a, b := old.Digest(), p.Digest()
		if hex.EncodeToString(a[:]) <= hex.EncodeToString(b[:]) {
			return nil
		}
	}
	bc.apowWork = &p
	return nil
}

func cloneAPoW(p *APoWProof) *APoWProof {
	if p == nil {
		return nil
	}
	copy := *p
	return &copy
}

// EVMGasLimitAt preserves historic execution and GASLIMIT semantics. Never
// overwrite EVM.GasLimit to increase capacity on an already executed chain.
func (c ProtocolV2Config) EVMGasLimitAt(height int) uint64 {
	if c.EVM == nil {
		return 0
	}
	if c.APoWEnabledAt(height) && c.APoW.GasLimit != 0 {
		return c.APoW.GasLimit
	}
	return c.EVM.GasLimit
}
