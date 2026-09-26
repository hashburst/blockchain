package consensus

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"hashburst/wallet"
)

type Registry struct {
	mu         sync.RWMutex
	config     Config
	validators map[string]Validator
}

func NewRegistry(cfg Config) *Registry {
	if err := cfg.Validate(); err != nil {
		panic(err)
	}
	return &Registry{config: cfg, validators: make(map[string]Validator)}
}

func (r *Registry) Config() Config { return r.config }

func (r *Registry) Clone() *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := NewRegistry(r.config)
	for id, v := range r.validators {
		out.validators[id] = v
	}
	return out
}

func (r *Registry) ReplaceWith(other *Registry) {
	if other == nil {
		return
	}
	other.mu.RLock()
	defer other.mu.RUnlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config = other.config
	r.validators = make(map[string]Validator, len(other.validators))
	for id, v := range other.validators {
		r.validators[id] = v
	}
}

func (r *Registry) Register(operator string, req RegisterRequest, height uint64) (Validator, error) {
	if !wallet.IsValidAddress(operator) {
		return Validator{}, fmt.Errorf("invalid validator operator address")
	}
	if !wallet.IsValidAddress(req.RewardAddress) {
		return Validator{}, fmt.Errorf("invalid validator reward address")
	}
	if strings.TrimSpace(req.NodeID) == "" || len(req.NodeID) > 128 {
		return Validator{}, fmt.Errorf("invalid node_id")
	}
	if strings.TrimSpace(req.PeerID) == "" || len(req.PeerID) > 256 {
		return Validator{}, fmt.Errorf("peer_id required and must be <=256 bytes")
	}
	if req.BondUnits < r.config.MinBondUnits {
		return Validator{}, fmt.Errorf("validator bond %d below minimum %d", req.BondUnits, r.config.MinBondUnits)
	}
	canonicalPub, consensusAddr, err := wallet.CanonicalPublicKeyHex(req.ConsensusPubKey)
	if err != nil {
		return Validator{}, fmt.Errorf("consensus public key: %w", err)
	}
	if wallet.AddressEqual(consensusAddr, req.RewardAddress) {
		return Validator{}, fmt.Errorf("validator consensus key must be separate from reward wallet")
	}
	if wallet.AddressEqual(consensusAddr, operator) {
		return Validator{}, fmt.Errorf("validator consensus key must be separate from operator/node wallet")
	}
	id, err := ValidatorID(canonicalPub)
	if err != nil {
		return Validator{}, err
	}
	key := strings.ToLower(id)

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.validators[key]; exists {
		return Validator{}, fmt.Errorf("validator already registered")
	}
	for _, existing := range r.validators {
		if wallet.AddressEqual(existing.OperatorAddress, operator) && existing.Status != ValidatorExited {
			return Validator{}, fmt.Errorf("operator already controls validator %s", existing.ID)
		}
		if strings.EqualFold(existing.NodeID, req.NodeID) && existing.Status != ValidatorExited {
			return Validator{}, fmt.Errorf("node_id already bound to validator %s", existing.ID)
		}
		if strings.EqualFold(existing.PeerID, strings.TrimSpace(req.PeerID)) && existing.Status != ValidatorExited {
			return Validator{}, fmt.Errorf("peer_id already bound to validator %s", existing.ID)
		}
		if strings.EqualFold(existing.ConsensusPubKey, canonicalPub) && existing.Status != ValidatorExited {
			return Validator{}, fmt.Errorf("consensus key already registered")
		}
	}
	v := Validator{
		ID: id, NodeID: strings.TrimSpace(req.NodeID), PeerID: strings.TrimSpace(req.PeerID),
		OperatorAddress: operator, RewardAddress: req.RewardAddress,
		ConsensusPubKey: canonicalPub, ConsensusAddress: consensusAddr,
		EscrowAddress: ValidatorEscrowAddress(id), BondUnits: req.BondUnits,
		Status: ValidatorPending, RegisteredHeight: height, ActivationHeight: height + r.config.ActivationDelay,
	}
	if r.config.ActivationDelay == 0 {
		v.Status = ValidatorActive
	}
	r.validators[key] = v
	return v, nil
}

func (r *Registry) AdvanceHeight(height uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, v := range r.validators {
		switch v.Status {
		case ValidatorPending:
			if height >= v.ActivationHeight && v.BondUnits >= r.config.MinBondUnits {
				v.Status = ValidatorActive
			}
		case ValidatorJailed:
			if v.JailedUntilHeight != 0 && height >= v.JailedUntilHeight && v.BondUnits >= r.config.MinBondUnits {
				v.Status = ValidatorActive
				v.JailedUntilHeight = 0
			}
		}
		r.validators[id] = v
	}
}

func (r *Registry) Get(id string) (Validator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.validators[strings.ToLower(canonicalHex(id))]
	return v, ok
}

func (r *Registry) ByOperator(operator string) (Validator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, v := range r.validators {
		if wallet.AddressEqual(v.OperatorAddress, operator) && v.Status != ValidatorExited {
			return v, true
		}
	}
	return Validator{}, false
}

func (r *Registry) ActiveSet(height uint64) ValidatorSet {
	r.mu.RLock()
	defer r.mu.RUnlock()
	vals := make(map[string]Validator)
	for id, v := range r.validators {
		if v.Status == ValidatorActive && v.BondUnits >= r.config.MinBondUnits {
			vals[id] = v
		}
	}
	ordered := sortedValidators(vals)
	powers := make([]uint64, len(ordered))
	// Phase 3C intentionally uses one-validator-one-vote. HBT bond is an
	// eligibility/slashing collateral, not a mechanism for buying additional
	// finality power. Stake-weighted voting can only be introduced by a later
	// versioned consensus upgrade. This preserves the bootstrap 3-of-4 quorum
	// when four validators are active even if one posts excess bond.
	for i := range ordered {
		powers[i] = 1
	}
	return ValidatorSet{Height: height, Validators: ordered, Power: powers}
}

func (r *Registry) BeginUnbond(id, operator string, height uint64) (Validator, error) {
	key := strings.ToLower(canonicalHex(id))
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.validators[key]
	if !ok {
		return Validator{}, fmt.Errorf("validator not found")
	}
	if !wallet.AddressEqual(v.OperatorAddress, operator) {
		return Validator{}, fmt.Errorf("validator operator required")
	}
	if v.Status == ValidatorUnbonding || v.Status == ValidatorExited {
		return Validator{}, fmt.Errorf("validator status %s cannot begin unbond", v.Status)
	}
	v.Status = ValidatorUnbonding
	v.UnbondingEndHeight = height + r.config.UnbondingBlocks
	r.validators[key] = v
	return v, nil
}

func (r *Registry) Withdraw(id, operator string, height uint64) (Validator, int64, error) {
	key := strings.ToLower(canonicalHex(id))
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.validators[key]
	if !ok {
		return Validator{}, 0, fmt.Errorf("validator not found")
	}
	if !wallet.AddressEqual(v.OperatorAddress, operator) {
		return Validator{}, 0, fmt.Errorf("validator operator required")
	}
	if v.Status != ValidatorUnbonding {
		return Validator{}, 0, fmt.Errorf("validator is not unbonding")
	}
	if height < v.UnbondingEndHeight {
		return Validator{}, 0, fmt.Errorf("unbonding not complete: height %d < %d", height, v.UnbondingEndHeight)
	}
	amount := v.BondUnits
	if amount <= 0 {
		return Validator{}, 0, fmt.Errorf("validator has no withdrawable bond")
	}
	v.BondUnits = 0
	v.Status = ValidatorExited
	r.validators[key] = v
	return v, amount, nil
}

func (r *Registry) ApplyDoubleVoteEvidence(ev DoubleVoteEvidence, evidenceHeight uint64) (Validator, int64, error) {
	if err := ev.ValidateBasic(); err != nil {
		return Validator{}, 0, err
	}
	key := strings.ToLower(canonicalHex(ev.VoteA.ValidatorID))

	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.validators[key]
	if !ok {
		return Validator{}, 0, fmt.Errorf("evidence validator not found")
	}
	if v.Status == ValidatorExited {
		return Validator{}, 0, fmt.Errorf("validator already exited")
	}
	if err := VerifyVote(ev.VoteA, v); err != nil {
		return Validator{}, 0, fmt.Errorf("evidence vote A: %w", err)
	}
	if err := VerifyVote(ev.VoteB, v); err != nil {
		return Validator{}, 0, fmt.Errorf("evidence vote B: %w", err)
	}
	if v.LastEvidenceHeight == ev.VoteA.Height {
		return Validator{}, 0, fmt.Errorf("double-vote evidence already applied for height %d", ev.VoteA.Height)
	}
	slashBig := new(big.Int).Mul(big.NewInt(v.BondUnits), big.NewInt(int64(r.config.DoubleVoteSlashBPS)))
	slashBig.Div(slashBig, big.NewInt(10_000))
	slash := slashBig.Int64()
	if slash < 1 && v.BondUnits > 0 {
		slash = 1
	}
	if slash > v.BondUnits {
		slash = v.BondUnits
	}
	v.BondUnits -= slash
	v.SlashedUnits += slash
	// Slashing an already-unbonding validator must not accidentally reactivate
	// it after a jail timeout. The slash reduces the eventual withdrawal while
	// the existing unbonding clock remains in force. Other live states are jailed.
	if v.Status == ValidatorUnbonding {
		v.JailedUntilHeight = 0
	} else {
		v.Status = ValidatorJailed
		v.JailedUntilHeight = evidenceHeight + r.config.JailBlocks
	}
	v.LastEvidenceHeight = ev.VoteA.Height
	r.validators[key] = v
	return v, slash, nil
}

func (r *Registry) Root() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var b bytes.Buffer
	putString(&b, "HASHBURST_VALIDATOR_REGISTRY_V1")
	putInt64(&b, r.config.MinBondUnits)
	putUint64(&b, r.config.ActivationDelay)
	putUint64(&b, r.config.UnbondingBlocks)
	putUint64(&b, r.config.JailBlocks)
	putUint64(&b, uint64(r.config.DoubleVoteSlashBPS))
	ordered := sortedValidators(r.validators)
	putUint64(&b, uint64(len(ordered)))
	for _, v := range ordered {
		putString(&b, canonicalHex(v.ID))
		putString(&b, v.NodeID)
		putString(&b, v.PeerID)
		putString(&b, strings.ToLower(v.OperatorAddress))
		putString(&b, strings.ToLower(v.RewardAddress))
		putString(&b, strings.ToLower(v.ConsensusPubKey))
		putString(&b, strings.ToLower(v.ConsensusAddress))
		putString(&b, strings.ToLower(v.EscrowAddress))
		putInt64(&b, v.BondUnits)
		putInt64(&b, v.SlashedUnits)
		putString(&b, string(v.Status))
		putUint64(&b, v.RegisteredHeight)
		putUint64(&b, v.ActivationHeight)
		putUint64(&b, v.JailedUntilHeight)
		putUint64(&b, v.UnbondingEndHeight)
		putUint64(&b, v.LastEvidenceHeight)
	}
	h := sha256.Sum256(b.Bytes())
	return "0x" + hex.EncodeToString(h[:])
}

func (r *Registry) Snapshot() []Validator {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortedValidators(r.validators)
}

// ApplyBFTDoubleSignEvidence applies Phase 3D same-round equivocation evidence.
// Cross-round votes are NOT slashable by themselves: legitimate view changes
// may require an honest validator to vote for a different safe value later.
func (r *Registry) ApplyBFTDoubleSignEvidence(ev BFTDoubleSignEvidence, evidenceHeight uint64) (Validator, int64, error) {
	if err := ev.ValidateBasic(); err != nil {
		return Validator{}, 0, err
	}
	var validatorID string
	var evidenceVoteHeight uint64
	keyFrom := func(id string) string { return strings.ToLower(canonicalHex(id)) }
	switch ev.Step {
	case StepPrevote:
		validatorID = ev.Prevote.VoteA.ValidatorID
		evidenceVoteHeight = ev.Prevote.VoteA.Height
	case StepPrecommit:
		validatorID = ev.Precommit.VoteA.ValidatorID
		evidenceVoteHeight = ev.Precommit.VoteA.Height
	default:
		return Validator{}, 0, fmt.Errorf("unsupported BFT evidence step %q", ev.Step)
	}
	key := keyFrom(validatorID)

	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.validators[key]
	if !ok {
		return Validator{}, 0, fmt.Errorf("evidence validator not found")
	}
	if v.Status == ValidatorExited {
		return Validator{}, 0, fmt.Errorf("validator already exited")
	}
	switch ev.Step {
	case StepPrevote:
		if err := VerifyPrevote(ev.Prevote.VoteA, v); err != nil {
			return Validator{}, 0, fmt.Errorf("evidence prevote A: %w", err)
		}
		if err := VerifyPrevote(ev.Prevote.VoteB, v); err != nil {
			return Validator{}, 0, fmt.Errorf("evidence prevote B: %w", err)
		}
	case StepPrecommit:
		if err := VerifyVote(ev.Precommit.VoteA, v); err != nil {
			return Validator{}, 0, fmt.Errorf("evidence precommit A: %w", err)
		}
		if err := VerifyVote(ev.Precommit.VoteB, v); err != nil {
			return Validator{}, 0, fmt.Errorf("evidence precommit B: %w", err)
		}
	}
	if v.LastEvidenceHeight == evidenceVoteHeight {
		return Validator{}, 0, fmt.Errorf("equivocation evidence already applied for height %d", evidenceVoteHeight)
	}
	slashBig := new(big.Int).Mul(big.NewInt(v.BondUnits), big.NewInt(int64(r.config.DoubleVoteSlashBPS)))
	slashBig.Div(slashBig, big.NewInt(10_000))
	slash := slashBig.Int64()
	if slash < 1 && v.BondUnits > 0 {
		slash = 1
	}
	if slash > v.BondUnits {
		slash = v.BondUnits
	}
	v.BondUnits -= slash
	v.SlashedUnits += slash
	if v.Status == ValidatorUnbonding {
		v.JailedUntilHeight = 0
	} else {
		v.Status = ValidatorJailed
		v.JailedUntilHeight = evidenceHeight + r.config.JailBlocks
	}
	v.LastEvidenceHeight = evidenceVoteHeight
	r.validators[key] = v
	return v, slash, nil
}
