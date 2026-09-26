package consensus

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"hashburst/wallet"
)

type ValidatorStatus string

const (
	ValidatorPending   ValidatorStatus = "PENDING"
	ValidatorActive    ValidatorStatus = "ACTIVE"
	ValidatorJailed    ValidatorStatus = "JAILED"
	ValidatorUnbonding ValidatorStatus = "UNBONDING"
	ValidatorExited    ValidatorStatus = "EXITED"
)

type Config struct {
	MinBondUnits       int64  `json:"min_bond_units"`
	ActivationDelay    uint64 `json:"activation_delay_blocks"`
	UnbondingBlocks    uint64 `json:"unbonding_blocks"`
	JailBlocks         uint64 `json:"jail_blocks"`
	DoubleVoteSlashBPS uint32 `json:"double_vote_slash_bps"`
}

func (c Config) Validate() error {
	if c.MinBondUnits <= 0 {
		return fmt.Errorf("minimum validator bond must be positive")
	}
	if c.UnbondingBlocks == 0 {
		return fmt.Errorf("unbonding period must be non-zero")
	}
	if c.JailBlocks == 0 {
		return fmt.Errorf("jail period must be non-zero")
	}
	if c.DoubleVoteSlashBPS == 0 || c.DoubleVoteSlashBPS > 10_000 {
		return fmt.Errorf("double-vote slash bps must be 1..10000")
	}
	return nil
}

type Validator struct {
	ID                 string          `json:"id"`
	NodeID             string          `json:"node_id"`
	PeerID             string          `json:"peer_id,omitempty"`
	OperatorAddress    string          `json:"operator_address"`
	RewardAddress      string          `json:"reward_address"`
	ConsensusPubKey    string          `json:"consensus_pubkey"`
	ConsensusAddress   string          `json:"consensus_address"`
	EscrowAddress      string          `json:"escrow_address"`
	BondUnits          int64           `json:"bond_units"`
	SlashedUnits       int64           `json:"slashed_units"`
	Status             ValidatorStatus `json:"status"`
	RegisteredHeight   uint64          `json:"registered_height"`
	ActivationHeight   uint64          `json:"activation_height"`
	JailedUntilHeight  uint64          `json:"jailed_until_height,omitempty"`
	UnbondingEndHeight uint64          `json:"unbonding_end_height,omitempty"`
	LastEvidenceHeight uint64          `json:"last_evidence_height,omitempty"`
}

type RegisterRequest struct {
	NodeID          string `json:"node_id"`
	PeerID          string `json:"peer_id,omitempty"`
	ConsensusPubKey string `json:"consensus_pubkey"`
	RewardAddress   string `json:"reward_address"`
	BondUnits       int64  `json:"bond_units"`
}

type ValidatorSet struct {
	Height     uint64      `json:"height"`
	Validators []Validator `json:"validators"`
	Power      []uint64    `json:"power"`
}

func (s ValidatorSet) TotalPower() uint64 {
	var total uint64
	for _, p := range s.Power {
		if ^uint64(0)-total < p {
			return ^uint64(0)
		}
		total += p
	}
	return total
}

func (s ValidatorSet) Find(id string) (Validator, uint64, bool) {
	id = canonicalHex(id)
	for i, v := range s.Validators {
		if canonicalHex(v.ID) == id {
			var p uint64
			if i < len(s.Power) {
				p = s.Power[i]
			}
			return v, p, true
		}
	}
	return Validator{}, 0, false
}

// Proposer is deterministic round-robin over the validator IDs sorted by the
// registry. Voting power affects quorum but not proposer frequency in Phase 3C.
func (s ValidatorSet) Proposer(height, round uint64) (Validator, bool) {
	if len(s.Validators) == 0 {
		return Validator{}, false
	}
	idx := (height + round) % uint64(len(s.Validators))
	return s.Validators[idx], true
}

func (s ValidatorSet) Root() string {
	var b bytes.Buffer
	putString(&b, "HASHBURST_VALIDATOR_SET_V1")
	putUint64(&b, s.Height)
	putUint64(&b, uint64(len(s.Validators)))
	for i, v := range s.Validators {
		putString(&b, canonicalHex(v.ID))
		putString(&b, strings.ToLower(v.NodeID))
		putString(&b, strings.ToLower(v.OperatorAddress))
		putString(&b, strings.ToLower(v.RewardAddress))
		putString(&b, strings.ToLower(v.ConsensusPubKey))
		putInt64(&b, v.BondUnits)
		var p uint64
		if i < len(s.Power) {
			p = s.Power[i]
		}
		putUint64(&b, p)
	}
	h := sha256.Sum256(b.Bytes())
	return "0x" + hex.EncodeToString(h[:])
}

func QuorumThreshold(total uint64) uint64 {
	if total == 0 {
		return 0
	}
	// floor(2*total/3)+1 using big.Int to make overflow impossible.
	x := new(big.Int).SetUint64(total)
	x.Mul(x, big.NewInt(2))
	x.Div(x, big.NewInt(3))
	x.Add(x, big.NewInt(1))
	return x.Uint64()
}

func ValidatorID(consensusPubKey string) (string, error) {
	canonical, _, err := wallet.CanonicalPublicKeyHex(consensusPubKey)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte("HASHBURST_VALIDATOR_ID_V1\x00" + strings.ToLower(canonical)))
	return "0x" + hex.EncodeToString(h[:]), nil
}

func ValidatorEscrowAddress(validatorID string) string {
	h := wallet.Keccak256([]byte("HASHBURST_VALIDATOR_ESCROW_V1\x00"), []byte(strings.ToLower(validatorID)))
	return wallet.ToChecksumAddress(h[len(h)-wallet.AddressLength:])
}

func canonicalHex(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if strings.HasPrefix(s, "0x") {
		return s
	}
	return "0x" + s
}

func sortedValidators(m map[string]Validator) []Validator {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Validator, 0, len(ids))
	for _, id := range ids {
		out = append(out, m[id])
	}
	return out
}

func putUint64(b *bytes.Buffer, v uint64) {
	var x [8]byte
	binary.BigEndian.PutUint64(x[:], v)
	b.Write(x[:])
}

func putInt64(b *bytes.Buffer, v int64) { putUint64(b, uint64(v)) }
func putString(b *bytes.Buffer, s string) {
	putUint64(b, uint64(len(s)))
	b.WriteString(s)
}
