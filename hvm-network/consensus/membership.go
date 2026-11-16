package consensus

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"hashburst/wallet"
)

const (
	MembershipActionSuspend   = "SUSPEND"
	MembershipActionRevoke    = "REVOKE"
	MembershipActionRotateKey = "ROTATE_CONSENSUS_KEY"
)

type MembershipProposal struct {
	ChainID         uint64 `json:"chain_id"`
	Version         uint64 `json:"version"`
	PreviousVersion uint64 `json:"previous_version"`
	PreviousSetRoot string `json:"previous_set_root"`
	ValidatorID     string `json:"validator_id"`
	Action          string `json:"action"`
	ConsensusPubKey string `json:"consensus_pubkey,omitempty"`
}

type MembershipVote struct {
	ProposalHash string `json:"proposal_hash"`
	ValidatorID  string `json:"validator_id"`
	Signature    string `json:"signature"`
}

type MembershipCertificate struct {
	Proposal MembershipProposal `json:"proposal"`
	Votes    []MembershipVote   `json:"votes"`
}

type MembershipState struct {
	mu       sync.Mutex
	version  uint64
	consumed map[string]struct{}
}

func NewMembershipState(version uint64) *MembershipState {
	return &MembershipState{
		version:  version,
		consumed: make(map[string]struct{}),
	}
}

func (s *MembershipState) Version() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version
}

func (s *MembershipState) Snapshot() MembershipStateSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	consumed := make([]string, 0, len(s.consumed))
	for h := range s.consumed {
		consumed = append(consumed, h)
	}
	sort.Strings(consumed)
	return MembershipStateSnapshot{
		Version:  s.version,
		Consumed: consumed,
	}
}

type MembershipStateSnapshot struct {
	Version  uint64   `json:"version"`
	Consumed []string `json:"consumed"`
}

func RestoreMembershipState(snapshot MembershipStateSnapshot) (*MembershipState, error) {
	s := NewMembershipState(snapshot.Version)
	for _, h := range snapshot.Consumed {
		h = strings.ToLower(strings.TrimSpace(h))
		if len(strings.TrimPrefix(h, "0x")) != 64 {
			return nil, fmt.Errorf("invalid consumed membership proposal hash")
		}
		if _, exists := s.consumed[h]; exists {
			return nil, fmt.Errorf("duplicate consumed membership proposal hash")
		}
		s.consumed[h] = struct{}{}
	}
	return s, nil
}

func validateMembershipProposal(p MembershipProposal) error {
	if p.ChainID == 0 {
		return fmt.Errorf("membership chain_id must be non-zero")
	}
	if p.Version != p.PreviousVersion+1 {
		return fmt.Errorf("membership version must advance exactly by one")
	}
	if len(strings.TrimPrefix(p.PreviousSetRoot, "0x")) != 64 {
		return fmt.Errorf("membership previous_set_root must be 32 bytes")
	}
	if strings.TrimSpace(p.ValidatorID) == "" {
		return fmt.Errorf("membership validator_id required")
	}

	switch p.Action {
	case MembershipActionSuspend, MembershipActionRevoke:
		if strings.TrimSpace(p.ConsensusPubKey) != "" {
			return fmt.Errorf("membership action %s must not carry consensus key", p.Action)
		}
	case MembershipActionRotateKey:
		if strings.TrimSpace(p.ConsensusPubKey) == "" {
			return fmt.Errorf("membership key rotation requires consensus key")
		}
		if _, _, err := wallet.CanonicalPublicKeyHex(p.ConsensusPubKey); err != nil {
			return fmt.Errorf("membership consensus public key: %w", err)
		}
	default:
		return fmt.Errorf("unsupported membership action %q", p.Action)
	}
	return nil
}

func MembershipProposalSignBytes(p MembershipProposal) ([]byte, error) {
	if err := validateMembershipProposal(p); err != nil {
		return nil, err
	}

	var b bytes.Buffer
	putString(&b, "HASHBURST_VALIDATOR_MEMBERSHIP_V1")
	putUint64(&b, p.ChainID)
	putUint64(&b, p.Version)
	putUint64(&b, p.PreviousVersion)
	putString(&b, strings.ToLower(canonicalHex(p.PreviousSetRoot)))
	putString(&b, strings.ToLower(canonicalHex(p.ValidatorID)))
	putString(&b, p.Action)

	if p.Action == MembershipActionRotateKey {
		canonical, _, err := wallet.CanonicalPublicKeyHex(p.ConsensusPubKey)
		if err != nil {
			return nil, err
		}
		putString(&b, strings.ToLower(canonical))
	} else {
		putString(&b, "")
	}

	return b.Bytes(), nil
}

func MembershipProposalSignHash(p MembershipProposal) ([]byte, error) {
	signBytes, err := MembershipProposalSignBytes(p)
	if err != nil {
		return nil, err
	}
	return wallet.Keccak256(signBytes), nil
}

func MembershipProposalHash(p MembershipProposal) (string, error) {
	h, err := MembershipProposalSignHash(p)
	if err != nil {
		return "", err
	}
	return "0x" + hex.EncodeToString(h), nil
}

func NewMembershipVote(p MembershipProposal, validatorID string, signer *wallet.Wallet) (MembershipVote, error) {
	if signer == nil {
		return MembershipVote{}, fmt.Errorf("membership signer required")
	}

	h, err := MembershipProposalSignHash(p)
	if err != nil {
		return MembershipVote{}, err
	}

	proposalHash := "0x" + hex.EncodeToString(h)

	sig, err := signer.Sign(h)
	if err != nil {
		return MembershipVote{}, err
	}
	if len(sig) != 65 {
		return MembershipVote{}, fmt.Errorf("membership signature must be 65-byte recoverable signature")
	}

	return MembershipVote{
		ProposalHash: proposalHash,
		ValidatorID:  validatorID,
		Signature:    hex.EncodeToString(sig),
	}, nil
}

func verifyMembershipVote(p MembershipProposal, vote MembershipVote, validator Validator) error {
	h, err := MembershipProposalSignHash(p)
	if err != nil {
		return err
	}

	proposalHash := "0x" + hex.EncodeToString(h)
	if !strings.EqualFold(vote.ProposalHash, proposalHash) {
		return fmt.Errorf("membership vote proposal hash mismatch")
	}
	if !strings.EqualFold(canonicalHex(vote.ValidatorID), canonicalHex(validator.ID)) {
		return fmt.Errorf("membership vote validator mismatch")
	}

	sigHex := strings.TrimPrefix(strings.TrimSpace(vote.Signature), "0x")
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return fmt.Errorf("membership signature: %w", err)
	}
	if len(sig) != 65 {
		return fmt.Errorf("membership signature must be 65-byte recoverable signature")
	}

	recovered, err := wallet.RecoverAddress(h, sig)
	if err != nil {
		return fmt.Errorf("membership signature recovery: %w", err)
	}
	if !wallet.AddressEqual(recovered, validator.ConsensusAddress) {
		return fmt.Errorf(
			"membership vote signed by %s, validator consensus key is %s",
			recovered,
			validator.ConsensusAddress,
		)
	}
	return nil
}

func BuildMembershipCertificate(
	p MembershipProposal,
	previousSet ValidatorSet,
	votes []MembershipVote,
) (MembershipCertificate, error) {
	if err := validateMembershipProposal(p); err != nil {
		return MembershipCertificate{}, err
	}
	if !strings.EqualFold(p.PreviousSetRoot, previousSet.Root()) {
		return MembershipCertificate{}, fmt.Errorf("membership previous validator set root mismatch")
	}

	cert := MembershipCertificate{
		Proposal: p,
		Votes:    append([]MembershipVote(nil), votes...),
	}

	if err := cert.Verify(previousSet); err != nil {
		return MembershipCertificate{}, err
	}
	return cert, nil
}

func (c MembershipCertificate) Verify(previousSet ValidatorSet) error {
	if err := validateMembershipProposal(c.Proposal); err != nil {
		return err
	}
	if !strings.EqualFold(c.Proposal.PreviousSetRoot, previousSet.Root()) {
		return fmt.Errorf("membership certificate previous validator set root mismatch")
	}

	seen := make(map[string]struct{})
	var signed uint64

	for i, vote := range c.Votes {
		id := strings.ToLower(canonicalHex(vote.ValidatorID))
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("duplicate membership vote from %s", vote.ValidatorID)
		}

		validator, power, ok := previousSet.Find(vote.ValidatorID)
		if !ok {
			return fmt.Errorf("membership vote from non-active validator %s", vote.ValidatorID)
		}
		if err := verifyMembershipVote(c.Proposal, vote, validator); err != nil {
			return fmt.Errorf("membership vote %d: %w", i, err)
		}

		seen[id] = struct{}{}
		signed += power
	}

	threshold := QuorumThreshold(previousSet.TotalPower())
	if signed < threshold {
		return fmt.Errorf(
			"insufficient membership quorum power: signed=%d threshold=%d total=%d",
			signed,
			threshold,
			previousSet.TotalPower(),
		)
	}
	return nil
}

func (s *MembershipState) Apply(
	registry *Registry,
	previousSet ValidatorSet,
	cert MembershipCertificate,
) (Validator, error) {
	if registry == nil {
		return Validator{}, fmt.Errorf("membership registry required")
	}

	// The certificate must be authorized by the validator set that is
	// currently derivable from the canonical registry. A caller cannot
	// resurrect an older validator set merely by passing it as previousSet.
	currentSet := registry.ActiveSet(previousSet.Height)
	if !strings.EqualFold(previousSet.Root(), currentSet.Root()) {
		return Validator{}, fmt.Errorf("membership previous validator set is not current")
	}

	if err := cert.Verify(previousSet); err != nil {
		return Validator{}, err
	}

	h, err := MembershipProposalHash(cert.Proposal)
	if err != nil {
		return Validator{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if cert.Proposal.PreviousVersion != s.version {
		return Validator{}, fmt.Errorf(
			"membership previous version mismatch: got=%d want=%d",
			cert.Proposal.PreviousVersion,
			s.version,
		)
	}
	if cert.Proposal.Version != s.version+1 {
		return Validator{}, fmt.Errorf("membership version is not next version")
	}
	if _, exists := s.consumed[strings.ToLower(h)]; exists {
		return Validator{}, fmt.Errorf("membership proposal replay detected")
	}

	// Apply to a clone first so failed transitions cannot partially mutate
	// canonical registry or membership state.
	candidate := registry.Clone()

	var updated Validator
	switch cert.Proposal.Action {
	case MembershipActionSuspend:
		updated, err = candidate.applySuspend(cert.Proposal.ValidatorID)
	case MembershipActionRevoke:
		updated, err = candidate.applyRevoke(cert.Proposal.ValidatorID)
	case MembershipActionRotateKey:
		updated, err = candidate.applyConsensusKeyRotation(
			cert.Proposal.ValidatorID,
			cert.Proposal.ConsensusPubKey,
		)
	default:
		err = fmt.Errorf("unsupported membership action")
	}
	if err != nil {
		return Validator{}, err
	}

	registry.ReplaceWith(candidate)
	s.version = cert.Proposal.Version
	s.consumed[strings.ToLower(h)] = struct{}{}

	return updated, nil
}
