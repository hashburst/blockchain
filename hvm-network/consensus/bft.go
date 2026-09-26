package consensus

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"hashburst/wallet"
)

// Phase 3D implements a Tendermint-style, round-based BFT network protocol on
// top of the Phase 3C validator registry. Protocol V2 remains activation-gated;
// these types have no production effect until an explicit later activation.

const NilBlockHash = "nil"

type BFTStep string

const (
	StepProposal    BFTStep = "PROPOSAL"
	StepPrevote     BFTStep = "PREVOTE"
	StepPrecommit   BFTStep = "PRECOMMIT"
	StepRoundChange BFTStep = "ROUND_CHANGE"
)

type NetworkConfig struct {
	ProposalTimeout   time.Duration `json:"proposal_timeout"`
	PrevoteTimeout    time.Duration `json:"prevote_timeout"`
	PrecommitTimeout  time.Duration `json:"precommit_timeout"`
	RoundTimeoutDelta time.Duration `json:"round_timeout_delta"`
	MaxRound          uint64        `json:"max_round"`
	MaxFutureHeight   uint64        `json:"max_future_height"`
	MaxMessageBytes   int           `json:"max_message_bytes"`
}

func DefaultNetworkConfig() NetworkConfig {
	return NetworkConfig{
		ProposalTimeout:   3 * time.Second,
		PrevoteTimeout:    2 * time.Second,
		PrecommitTimeout:  2 * time.Second,
		RoundTimeoutDelta: 500 * time.Millisecond,
		MaxRound:          64,
		MaxFutureHeight:   2,
		MaxMessageBytes:   4 << 20,
	}
}

func (c NetworkConfig) Validate() error {
	if c.ProposalTimeout <= 0 || c.PrevoteTimeout <= 0 || c.PrecommitTimeout <= 0 {
		return fmt.Errorf("consensus timeouts must be positive")
	}
	if c.RoundTimeoutDelta < 0 {
		return fmt.Errorf("round timeout delta cannot be negative")
	}
	if c.MaxRound == 0 {
		return fmt.Errorf("max consensus round must be positive")
	}
	if c.MaxFutureHeight == 0 {
		return fmt.Errorf("max future height must be positive")
	}
	if c.MaxMessageBytes < 64*1024 {
		return fmt.Errorf("max consensus message bytes too small")
	}
	return nil
}

func (c NetworkConfig) TimeoutFor(step BFTStep, round uint64) time.Duration {
	var base time.Duration
	switch step {
	case StepProposal:
		base = c.ProposalTimeout
	case StepPrevote:
		base = c.PrevoteTimeout
	case StepPrecommit:
		base = c.PrecommitTimeout
	default:
		base = c.ProposalTimeout
	}
	// Linear backoff is deterministic and bounded by MaxRound. It avoids a
	// network-wide tight loop when one or more validators are offline.
	return base + time.Duration(round)*c.RoundTimeoutDelta
}

func canonicalBFTBlockHash(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" || v == NilBlockHash {
		return NilBlockHash, nil
	}
	v = strings.TrimPrefix(v, "0x")
	if len(v) != 64 {
		return "", fmt.Errorf("block hash must be 32 bytes or nil")
	}
	if _, err := hex.DecodeString(v); err != nil {
		return "", fmt.Errorf("block hash: %w", err)
	}
	return v, nil
}

type Prevote struct {
	ChainID          uint64 `json:"chain_id"`
	Height           uint64 `json:"height"`
	Round            uint64 `json:"round"`
	BlockHash        string `json:"block_hash"`
	ValidatorSetRoot string `json:"validator_set_root"`
	ValidatorID      string `json:"validator_id"`
	Signature        string `json:"signature"`
}

func PrevoteSignBytes(chainID, height, round uint64, blockHash, validatorSetRoot string) []byte {
	h, _ := canonicalBFTBlockHash(blockHash)
	var b bytes.Buffer
	putString(&b, "HASHBURST_BFT_PREVOTE_V1")
	putUint64(&b, chainID)
	putUint64(&b, height)
	putUint64(&b, round)
	putString(&b, h)
	putString(&b, strings.ToLower(canonicalHex(validatorSetRoot)))
	return b.Bytes()
}

func PrevoteSignHash(chainID, height, round uint64, blockHash, validatorSetRoot string) []byte {
	return wallet.Keccak256(PrevoteSignBytes(chainID, height, round, blockHash, validatorSetRoot))
}

func NewSignedPrevote(chainID, height, round uint64, blockHash, validatorSetRoot, validatorID string, signer *wallet.Wallet) (Prevote, error) {
	if signer == nil {
		return Prevote{}, fmt.Errorf("nil validator signer")
	}
	h, err := canonicalBFTBlockHash(blockHash)
	if err != nil {
		return Prevote{}, err
	}
	sig, err := signer.Sign(PrevoteSignHash(chainID, height, round, h, validatorSetRoot))
	if err != nil {
		return Prevote{}, err
	}
	return Prevote{ChainID: chainID, Height: height, Round: round, BlockHash: h, ValidatorSetRoot: validatorSetRoot, ValidatorID: validatorID, Signature: hex.EncodeToString(sig)}, nil
}

func VerifyPrevote(v Prevote, validator Validator) error {
	if canonicalHex(v.ValidatorID) != canonicalHex(validator.ID) {
		return fmt.Errorf("prevote validator id mismatch")
	}
	if v.ChainID == 0 || v.Height == 0 {
		return fmt.Errorf("prevote chain/height must be non-zero")
	}
	h, err := canonicalBFTBlockHash(v.BlockHash)
	if err != nil {
		return err
	}
	if len(strings.TrimPrefix(v.ValidatorSetRoot, "0x")) != 64 {
		return fmt.Errorf("prevote validator set root must be 32 bytes")
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(v.Signature, "0x"))
	if err != nil || len(sig) != 65 {
		return fmt.Errorf("prevote signature must be 65-byte recoverable signature")
	}
	recovered, err := wallet.RecoverAddress(PrevoteSignHash(v.ChainID, v.Height, v.Round, h, v.ValidatorSetRoot), sig)
	if err != nil {
		return err
	}
	if !wallet.AddressEqual(recovered, validator.ConsensusAddress) {
		return fmt.Errorf("prevote signed by %s, validator consensus key is %s", recovered, validator.ConsensusAddress)
	}
	return nil
}

type PrevoteCertificate struct {
	ChainID          uint64    `json:"chain_id"`
	Height           uint64    `json:"height"`
	Round            uint64    `json:"round"`
	BlockHash        string    `json:"block_hash"`
	ValidatorSetRoot string    `json:"validator_set_root"`
	SignedPower      uint64    `json:"signed_power"`
	TotalPower       uint64    `json:"total_power"`
	Votes            []Prevote `json:"votes"`
}

func BuildPrevoteCertificate(chainID, height, round uint64, blockHash, validatorSetRoot string, set ValidatorSet, votes []Prevote) (PrevoteCertificate, error) {
	qc := PrevoteCertificate{ChainID: chainID, Height: height, Round: round, BlockHash: blockHash, ValidatorSetRoot: validatorSetRoot, Votes: append([]Prevote(nil), votes...)}
	if err := qc.Verify(set); err != nil {
		return PrevoteCertificate{}, err
	}
	return qc, nil
}

func (q *PrevoteCertificate) Verify(set ValidatorSet) error {
	if q == nil {
		return fmt.Errorf("missing prevote certificate")
	}
	if q.Height != set.Height {
		return fmt.Errorf("prevote certificate height mismatch")
	}
	if !strings.EqualFold(q.ValidatorSetRoot, set.Root()) {
		return fmt.Errorf("prevote certificate validator set root mismatch")
	}
	wantHash, err := canonicalBFTBlockHash(q.BlockHash)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(q.Votes))
	var signed uint64
	for i, vote := range q.Votes {
		gotHash, err := canonicalBFTBlockHash(vote.BlockHash)
		if err != nil {
			return fmt.Errorf("prevote certificate vote %d: %w", i, err)
		}
		if vote.ChainID != q.ChainID || vote.Height != q.Height || vote.Round != q.Round || !strings.EqualFold(gotHash, wantHash) || !strings.EqualFold(vote.ValidatorSetRoot, q.ValidatorSetRoot) {
			return fmt.Errorf("prevote certificate vote %d does not match certificate", i)
		}
		id := strings.ToLower(canonicalHex(vote.ValidatorID))
		if _, dup := seen[id]; dup {
			return fmt.Errorf("duplicate validator prevote %s", vote.ValidatorID)
		}
		seen[id] = struct{}{}
		validator, power, ok := set.Find(vote.ValidatorID)
		if !ok {
			return fmt.Errorf("prevote from non-active validator %s", vote.ValidatorID)
		}
		if err := VerifyPrevote(vote, validator); err != nil {
			return fmt.Errorf("prevote certificate vote %d: %w", i, err)
		}
		if ^uint64(0)-signed < power {
			return fmt.Errorf("prevote signed power overflow")
		}
		signed += power
	}
	total := set.TotalPower()
	threshold := QuorumThreshold(total)
	if total == 0 || signed < threshold {
		return fmt.Errorf("insufficient prevote quorum power: signed=%d threshold=%d total=%d", signed, threshold, total)
	}
	q.BlockHash = wantHash
	q.SignedPower = signed
	q.TotalPower = total
	return nil
}

type RoundChange struct {
	ChainID          uint64 `json:"chain_id"`
	Height           uint64 `json:"height"`
	NextRound        uint64 `json:"next_round"`
	ValidatorSetRoot string `json:"validator_set_root"`
	ValidatorID      string `json:"validator_id"`
	Signature        string `json:"signature"`
}

func RoundChangeSignBytes(chainID, height, nextRound uint64, validatorSetRoot string) []byte {
	var b bytes.Buffer
	putString(&b, "HASHBURST_BFT_ROUND_CHANGE_V1")
	putUint64(&b, chainID)
	putUint64(&b, height)
	putUint64(&b, nextRound)
	putString(&b, strings.ToLower(canonicalHex(validatorSetRoot)))
	return b.Bytes()
}

func RoundChangeSignHash(chainID, height, nextRound uint64, validatorSetRoot string) []byte {
	return wallet.Keccak256(RoundChangeSignBytes(chainID, height, nextRound, validatorSetRoot))
}

func NewSignedRoundChange(chainID, height, nextRound uint64, validatorSetRoot, validatorID string, signer *wallet.Wallet) (RoundChange, error) {
	if signer == nil {
		return RoundChange{}, fmt.Errorf("nil validator signer")
	}
	if nextRound == 0 {
		return RoundChange{}, fmt.Errorf("round change must target round > 0")
	}
	sig, err := signer.Sign(RoundChangeSignHash(chainID, height, nextRound, validatorSetRoot))
	if err != nil {
		return RoundChange{}, err
	}
	return RoundChange{ChainID: chainID, Height: height, NextRound: nextRound, ValidatorSetRoot: validatorSetRoot, ValidatorID: validatorID, Signature: hex.EncodeToString(sig)}, nil
}

func VerifyRoundChange(v RoundChange, validator Validator) error {
	if canonicalHex(v.ValidatorID) != canonicalHex(validator.ID) {
		return fmt.Errorf("round-change validator id mismatch")
	}
	if v.ChainID == 0 || v.Height == 0 || v.NextRound == 0 {
		return fmt.Errorf("round-change chain/height/round must be non-zero")
	}
	if len(strings.TrimPrefix(v.ValidatorSetRoot, "0x")) != 64 {
		return fmt.Errorf("round-change validator set root must be 32 bytes")
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(v.Signature, "0x"))
	if err != nil || len(sig) != 65 {
		return fmt.Errorf("round-change signature must be 65-byte recoverable signature")
	}
	recovered, err := wallet.RecoverAddress(RoundChangeSignHash(v.ChainID, v.Height, v.NextRound, v.ValidatorSetRoot), sig)
	if err != nil {
		return err
	}
	if !wallet.AddressEqual(recovered, validator.ConsensusAddress) {
		return fmt.Errorf("round-change signed by %s, validator consensus key is %s", recovered, validator.ConsensusAddress)
	}
	return nil
}

func FaultThreshold(totalPower uint64) uint64 {
	if totalPower == 0 {
		return 0
	}
	return (totalPower - 1) / 3
}

// CatchupThreshold is f+1. Seeing f+1 authenticated validators at a future
// round proves at least one honest validator has advanced, so a lagging node may
// safely fast-forward its pacemaker without treating the message as finality.
func CatchupThreshold(totalPower uint64) uint64 {
	if totalPower == 0 {
		return 0
	}
	return FaultThreshold(totalPower) + 1
}

type RoundChangeCertificate struct {
	ChainID          uint64        `json:"chain_id"`
	Height           uint64        `json:"height"`
	NextRound        uint64        `json:"next_round"`
	ValidatorSetRoot string        `json:"validator_set_root"`
	SignedPower      uint64        `json:"signed_power"`
	TotalPower       uint64        `json:"total_power"`
	Changes          []RoundChange `json:"changes"`
}

func BuildRoundChangeCertificate(chainID, height, nextRound uint64, validatorSetRoot string, set ValidatorSet, changes []RoundChange) (RoundChangeCertificate, error) {
	cert := RoundChangeCertificate{ChainID: chainID, Height: height, NextRound: nextRound, ValidatorSetRoot: validatorSetRoot, Changes: append([]RoundChange(nil), changes...)}
	if err := cert.Verify(set, QuorumThreshold(set.TotalPower())); err != nil {
		return RoundChangeCertificate{}, err
	}
	return cert, nil
}

func (c *RoundChangeCertificate) Verify(set ValidatorSet, threshold uint64) error {
	if c == nil {
		return fmt.Errorf("missing round-change certificate")
	}
	if c.Height != set.Height || !strings.EqualFold(c.ValidatorSetRoot, set.Root()) {
		return fmt.Errorf("round-change certificate set/height mismatch")
	}
	seen := make(map[string]struct{}, len(c.Changes))
	var signed uint64
	for i, rc := range c.Changes {
		if rc.ChainID != c.ChainID || rc.Height != c.Height || rc.NextRound != c.NextRound || !strings.EqualFold(rc.ValidatorSetRoot, c.ValidatorSetRoot) {
			return fmt.Errorf("round-change %d does not match certificate", i)
		}
		id := strings.ToLower(canonicalHex(rc.ValidatorID))
		if _, dup := seen[id]; dup {
			return fmt.Errorf("duplicate round-change validator %s", rc.ValidatorID)
		}
		seen[id] = struct{}{}
		validator, power, ok := set.Find(rc.ValidatorID)
		if !ok {
			return fmt.Errorf("round-change from non-active validator %s", rc.ValidatorID)
		}
		if err := VerifyRoundChange(rc, validator); err != nil {
			return fmt.Errorf("round-change %d: %w", i, err)
		}
		signed += power
	}
	if threshold == 0 || signed < threshold {
		return fmt.Errorf("insufficient round-change power: signed=%d threshold=%d", signed, threshold)
	}
	c.SignedPower = signed
	c.TotalPower = set.TotalPower()
	return nil
}

// SafeProposal enforces the lock rule. An unlocked validator may prevote any
// valid proposal. A locked validator may prevote a different block only when
// the proposal carries a valid prevote QC from a round not older than its lock.
func SafeProposal(lockedRound int64, lockedBlockHash string, proposalHash string, validRound int64, validQC *PrevoteCertificate, set ValidatorSet) error {
	proposalHash, err := canonicalBFTBlockHash(proposalHash)
	if err != nil || proposalHash == NilBlockHash {
		return fmt.Errorf("proposal requires non-nil block hash")
	}
	if lockedRound < 0 || lockedBlockHash == "" {
		if validRound >= 0 {
			if validQC == nil {
				return fmt.Errorf("proposal declares valid_round without prevote certificate")
			}
			if err := validQC.Verify(set); err != nil {
				return fmt.Errorf("proposal valid-round certificate: %w", err)
			}
			if int64(validQC.Round) != validRound || !strings.EqualFold(validQC.BlockHash, proposalHash) {
				return fmt.Errorf("proposal valid-round certificate does not certify proposal")
			}
		}
		return nil
	}
	lockedHash, err := canonicalBFTBlockHash(lockedBlockHash)
	if err != nil || lockedHash == NilBlockHash {
		return fmt.Errorf("invalid local lock")
	}
	if strings.EqualFold(lockedHash, proposalHash) {
		return nil
	}
	if validRound < lockedRound {
		return fmt.Errorf("proposal conflicts with lock round %d and has valid_round %d", lockedRound, validRound)
	}
	if validQC == nil {
		return fmt.Errorf("conflicting proposal requires prevote certificate")
	}
	if err := validQC.Verify(set); err != nil {
		return fmt.Errorf("conflicting proposal valid-round certificate: %w", err)
	}
	if int64(validQC.Round) != validRound || !strings.EqualFold(validQC.BlockHash, proposalHash) {
		return fmt.Errorf("conflicting proposal certificate does not certify proposal")
	}
	return nil
}

type PrevoteEquivocationEvidence struct {
	VoteA Prevote `json:"vote_a"`
	VoteB Prevote `json:"vote_b"`
}

func (e PrevoteEquivocationEvidence) ValidateBasic() error {
	if canonicalHex(e.VoteA.ValidatorID) != canonicalHex(e.VoteB.ValidatorID) || e.VoteA.ChainID != e.VoteB.ChainID || e.VoteA.Height != e.VoteB.Height || e.VoteA.Round != e.VoteB.Round || !strings.EqualFold(e.VoteA.ValidatorSetRoot, e.VoteB.ValidatorSetRoot) {
		return fmt.Errorf("prevote equivocation must target same validator/chain/height/round/set")
	}
	a, err := canonicalBFTBlockHash(e.VoteA.BlockHash)
	if err != nil {
		return err
	}
	b, err := canonicalBFTBlockHash(e.VoteB.BlockHash)
	if err != nil {
		return err
	}
	if strings.EqualFold(a, b) {
		return fmt.Errorf("prevote evidence does not conflict")
	}
	return nil
}

type PrecommitEquivocationEvidence struct {
	VoteA Vote `json:"vote_a"`
	VoteB Vote `json:"vote_b"`
}

func (e PrecommitEquivocationEvidence) ValidateBasic() error {
	if canonicalHex(e.VoteA.ValidatorID) != canonicalHex(e.VoteB.ValidatorID) || e.VoteA.ChainID != e.VoteB.ChainID || e.VoteA.Height != e.VoteB.Height || e.VoteA.Round != e.VoteB.Round || !strings.EqualFold(e.VoteA.ValidatorSetRoot, e.VoteB.ValidatorSetRoot) {
		return fmt.Errorf("precommit equivocation must target same validator/chain/height/round/set")
	}
	if strings.EqualFold(e.VoteA.BlockHash, e.VoteB.BlockHash) {
		return fmt.Errorf("precommit evidence does not conflict")
	}
	return nil
}

type BFTDoubleSignEvidence struct {
	Step      BFTStep                        `json:"step"`
	Prevote   *PrevoteEquivocationEvidence   `json:"prevote,omitempty"`
	Precommit *PrecommitEquivocationEvidence `json:"precommit,omitempty"`
}

func (e BFTDoubleSignEvidence) ValidateBasic() error {
	switch e.Step {
	case StepPrevote:
		if e.Prevote == nil || e.Precommit != nil {
			return fmt.Errorf("prevote evidence payload mismatch")
		}
		return e.Prevote.ValidateBasic()
	case StepPrecommit:
		if e.Precommit == nil || e.Prevote != nil {
			return fmt.Errorf("precommit evidence payload mismatch")
		}
		return e.Precommit.ValidateBasic()
	default:
		return fmt.Errorf("unsupported BFT evidence step %q", e.Step)
	}
}

type ProposalHeader struct {
	ChainID          uint64 `json:"chain_id"`
	Height           uint64 `json:"height"`
	Round            uint64 `json:"round"`
	BlockHash        string `json:"block_hash"`
	ValidatorSetRoot string `json:"validator_set_root"`
	ProposerID       string `json:"proposer_id"`
	ValidRound       int64  `json:"valid_round"`
	Signature        string `json:"signature"`
}

func ProposalSignBytes(chainID, height, round uint64, blockHash, validatorSetRoot, proposerID string, validRound int64) []byte {
	h, _ := canonicalBFTBlockHash(blockHash)
	var b bytes.Buffer
	putString(&b, "HASHBURST_BFT_PROPOSAL_V1")
	putUint64(&b, chainID)
	putUint64(&b, height)
	putUint64(&b, round)
	putString(&b, h)
	putString(&b, strings.ToLower(canonicalHex(validatorSetRoot)))
	putString(&b, strings.ToLower(canonicalHex(proposerID)))
	putInt64(&b, validRound)
	return b.Bytes()
}

func ProposalSignHash(chainID, height, round uint64, blockHash, validatorSetRoot, proposerID string, validRound int64) []byte {
	return wallet.Keccak256(ProposalSignBytes(chainID, height, round, blockHash, validatorSetRoot, proposerID, validRound))
}

func NewSignedProposalHeader(chainID, height, round uint64, blockHash, validatorSetRoot, proposerID string, validRound int64, signer *wallet.Wallet) (ProposalHeader, error) {
	if signer == nil {
		return ProposalHeader{}, fmt.Errorf("nil proposer signer")
	}
	h, err := canonicalBFTBlockHash(blockHash)
	if err != nil || h == NilBlockHash {
		return ProposalHeader{}, fmt.Errorf("proposal block hash invalid")
	}
	if validRound >= int64(round) {
		return ProposalHeader{}, fmt.Errorf("valid_round must be older than proposal round")
	}
	sig, err := signer.Sign(ProposalSignHash(chainID, height, round, h, validatorSetRoot, proposerID, validRound))
	if err != nil {
		return ProposalHeader{}, err
	}
	return ProposalHeader{ChainID: chainID, Height: height, Round: round, BlockHash: h, ValidatorSetRoot: validatorSetRoot, ProposerID: proposerID, ValidRound: validRound, Signature: hex.EncodeToString(sig)}, nil
}

func VerifyProposalHeader(h ProposalHeader, proposer Validator) error {
	if canonicalHex(h.ProposerID) != canonicalHex(proposer.ID) {
		return fmt.Errorf("proposal proposer id mismatch")
	}
	if h.ChainID == 0 || h.Height == 0 {
		return fmt.Errorf("proposal chain/height must be non-zero")
	}
	blockHash, err := canonicalBFTBlockHash(h.BlockHash)
	if err != nil || blockHash == NilBlockHash {
		return fmt.Errorf("proposal block hash invalid")
	}
	if h.ValidRound >= int64(h.Round) {
		return fmt.Errorf("proposal valid_round must be older than round")
	}
	if len(strings.TrimPrefix(h.ValidatorSetRoot, "0x")) != 64 {
		return fmt.Errorf("proposal validator set root must be 32 bytes")
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(h.Signature, "0x"))
	if err != nil || len(sig) != 65 {
		return fmt.Errorf("proposal signature must be 65-byte recoverable signature")
	}
	recovered, err := wallet.RecoverAddress(ProposalSignHash(h.ChainID, h.Height, h.Round, blockHash, h.ValidatorSetRoot, h.ProposerID, h.ValidRound), sig)
	if err != nil {
		return err
	}
	if !wallet.AddressEqual(recovered, proposer.ConsensusAddress) {
		return fmt.Errorf("proposal signed by %s, proposer consensus key is %s", recovered, proposer.ConsensusAddress)
	}
	return nil
}

func (e BFTDoubleSignEvidence) ID() (string, error) {
	if err := e.ValidateBasic(); err != nil {
		return "", err
	}
	var chain, height, round uint64
	var validator, setRoot, a, b string
	switch e.Step {
	case StepPrevote:
		chain, height, round = e.Prevote.VoteA.ChainID, e.Prevote.VoteA.Height, e.Prevote.VoteA.Round
		validator, setRoot = e.Prevote.VoteA.ValidatorID, e.Prevote.VoteA.ValidatorSetRoot
		a, _ = canonicalBFTBlockHash(e.Prevote.VoteA.BlockHash)
		b, _ = canonicalBFTBlockHash(e.Prevote.VoteB.BlockHash)
	case StepPrecommit:
		chain, height, round = e.Precommit.VoteA.ChainID, e.Precommit.VoteA.Height, e.Precommit.VoteA.Round
		validator, setRoot = e.Precommit.VoteA.ValidatorID, e.Precommit.VoteA.ValidatorSetRoot
		a, b = canonicalHex(e.Precommit.VoteA.BlockHash), canonicalHex(e.Precommit.VoteB.BlockHash)
	}
	if strings.Compare(strings.ToLower(a), strings.ToLower(b)) > 0 {
		a, b = b, a
	}
	var buf bytes.Buffer
	putString(&buf, "HASHBURST_BFT_EQUIVOCATION_ID_V1")
	putString(&buf, string(e.Step))
	putUint64(&buf, chain)
	putUint64(&buf, height)
	putUint64(&buf, round)
	putString(&buf, strings.ToLower(canonicalHex(validator)))
	putString(&buf, strings.ToLower(canonicalHex(setRoot)))
	putString(&buf, strings.ToLower(a))
	putString(&buf, strings.ToLower(b))
	h := sha256.Sum256(buf.Bytes())
	return "0x" + hex.EncodeToString(h[:]), nil
}
