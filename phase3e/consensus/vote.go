package consensus

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"

	"hashburst/wallet"
)

type Vote struct {
	ChainID          uint64 `json:"chain_id"`
	Height           uint64 `json:"height"`
	Round            uint64 `json:"round"`
	BlockHash        string `json:"block_hash"`
	ValidatorSetRoot string `json:"validator_set_root"`
	ValidatorID      string `json:"validator_id"`
	Signature        string `json:"signature"`
}

func VoteSignBytes(chainID, height, round uint64, blockHash, validatorSetRoot string) []byte {
	var b bytes.Buffer
	putString(&b, "HASHBURST_FINALITY_PRECOMMIT_V1")
	putUint64(&b, chainID)
	putUint64(&b, height)
	putUint64(&b, round)
	putString(&b, strings.ToLower(strings.TrimPrefix(blockHash, "0x")))
	putString(&b, strings.ToLower(strings.TrimPrefix(validatorSetRoot, "0x")))
	return b.Bytes()
}

func VoteSignHash(chainID, height, round uint64, blockHash, validatorSetRoot string) []byte {
	return wallet.Keccak256(VoteSignBytes(chainID, height, round, blockHash, validatorSetRoot))
}

func NewSignedVote(chainID, height, round uint64, blockHash, validatorSetRoot, validatorID string, signer *wallet.Wallet) (Vote, error) {
	if signer == nil {
		return Vote{}, fmt.Errorf("nil validator signer")
	}
	h := VoteSignHash(chainID, height, round, blockHash, validatorSetRoot)
	sig, err := signer.Sign(h)
	if err != nil {
		return Vote{}, err
	}
	return Vote{
		ChainID: chainID, Height: height, Round: round,
		BlockHash: blockHash, ValidatorSetRoot: validatorSetRoot,
		ValidatorID: validatorID, Signature: hex.EncodeToString(sig),
	}, nil
}

func VerifyVote(v Vote, validator Validator) error {
	if canonicalHex(v.ValidatorID) != canonicalHex(validator.ID) {
		return fmt.Errorf("vote validator id mismatch")
	}
	if len(strings.TrimPrefix(v.BlockHash, "0x")) != 64 || len(strings.TrimPrefix(v.ValidatorSetRoot, "0x")) != 64 {
		return fmt.Errorf("vote has malformed block/set hash")
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(v.Signature, "0x"))
	if err != nil || len(sig) != 65 {
		return fmt.Errorf("vote signature must be 65-byte recoverable signature")
	}
	recovered, err := wallet.RecoverAddress(VoteSignHash(v.ChainID, v.Height, v.Round, v.BlockHash, v.ValidatorSetRoot), sig)
	if err != nil {
		return err
	}
	if !wallet.AddressEqual(recovered, validator.ConsensusAddress) {
		return fmt.Errorf("vote signed by %s, validator consensus key is %s", recovered, validator.ConsensusAddress)
	}
	return nil
}

type DoubleVoteEvidence struct {
	VoteA Vote `json:"vote_a"`
	VoteB Vote `json:"vote_b"`
}

func (e DoubleVoteEvidence) ValidateBasic() error {
	if canonicalHex(e.VoteA.ValidatorID) != canonicalHex(e.VoteB.ValidatorID) {
		return fmt.Errorf("double-vote evidence has different validator ids")
	}
	if e.VoteA.ChainID != e.VoteB.ChainID || e.VoteA.Height != e.VoteB.Height {
		return fmt.Errorf("double-vote evidence must target same chain/height")
	}
	if !strings.EqualFold(e.VoteA.ValidatorSetRoot, e.VoteB.ValidatorSetRoot) {
		return fmt.Errorf("double-vote evidence has different validator sets")
	}
	if strings.EqualFold(e.VoteA.BlockHash, e.VoteB.BlockHash) {
		return fmt.Errorf("double-vote evidence does not conflict")
	}
	return nil
}

type QuorumCertificate struct {
	ChainID          uint64 `json:"chain_id"`
	Height           uint64 `json:"height"`
	Round            uint64 `json:"round"`
	BlockHash        string `json:"block_hash"`
	ValidatorSetRoot string `json:"validator_set_root"`
	SignedPower      uint64 `json:"signed_power"`
	TotalPower       uint64 `json:"total_power"`
	Votes            []Vote `json:"votes"`
}

func BuildQuorumCertificate(chainID, height, round uint64, blockHash, validatorSetRoot string, set ValidatorSet, votes []Vote) (QuorumCertificate, error) {
	qc := QuorumCertificate{ChainID: chainID, Height: height, Round: round, BlockHash: blockHash, ValidatorSetRoot: validatorSetRoot, Votes: append([]Vote(nil), votes...)}
	if err := qc.Verify(set); err != nil {
		return QuorumCertificate{}, err
	}
	return qc, nil
}

func (q *QuorumCertificate) Verify(set ValidatorSet) error {
	if q == nil {
		return fmt.Errorf("missing quorum certificate")
	}
	if q.Height != set.Height {
		return fmt.Errorf("QC height %d does not match validator set height %d", q.Height, set.Height)
	}
	if !strings.EqualFold(q.ValidatorSetRoot, set.Root()) {
		return fmt.Errorf("QC validator set root mismatch")
	}
	seen := make(map[string]struct{}, len(q.Votes))
	var signed uint64
	for i, vote := range q.Votes {
		if vote.ChainID != q.ChainID || vote.Height != q.Height || vote.Round != q.Round || !strings.EqualFold(vote.BlockHash, q.BlockHash) || !strings.EqualFold(vote.ValidatorSetRoot, q.ValidatorSetRoot) {
			return fmt.Errorf("QC vote %d does not match certificate", i)
		}
		id := strings.ToLower(canonicalHex(vote.ValidatorID))
		if _, dup := seen[id]; dup {
			return fmt.Errorf("duplicate validator vote %s", vote.ValidatorID)
		}
		seen[id] = struct{}{}
		validator, power, ok := set.Find(vote.ValidatorID)
		if !ok {
			return fmt.Errorf("QC vote from non-active validator %s", vote.ValidatorID)
		}
		if err := VerifyVote(vote, validator); err != nil {
			return fmt.Errorf("QC vote %d: %w", i, err)
		}
		if ^uint64(0)-signed < power {
			return fmt.Errorf("signed power overflow")
		}
		signed += power
	}
	total := set.TotalPower()
	threshold := QuorumThreshold(total)
	if total == 0 || signed < threshold {
		return fmt.Errorf("insufficient quorum power: signed=%d threshold=%d total=%d", signed, threshold, total)
	}
	q.SignedPower = signed
	q.TotalPower = total
	return nil
}
