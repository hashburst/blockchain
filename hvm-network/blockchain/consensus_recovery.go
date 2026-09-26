package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hashburst/consensus"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const recoveryFile = "consensus-recovery.json"
const recoveryLimit = 32 << 20

// Written before signatures can escape the reactor. A restart enters a fresh
// round with the same lock; it never treats a later round as permission to unlock.
type consensusRecovery struct {
	Version                             int
	ChainID, Height, ReservedRound      uint64
	ValidatorID, ParentHash, ConfigHash string
	LockedRound, ValidRound             int64
	LockedHash, ValidHash               string
	LockedBlock, ValidBlock             *Block
	LockedQC, ValidQC                   *consensus.PrevoteCertificate
}
type recoveryEnvelope struct {
	Payload json.RawMessage
	SHA256  string
}

func (bc *Blockchain) recoveryConfigHash() string {
	b, _ := json.Marshal(bc.v2Config)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// readRecovery validates the snapshot against the already verified chain and
// signing journal. Missing snapshots are accepted only before pending signatures.
func (bc *Blockchain) readRecovery(id string) (*consensusRecovery, error) {
	height := uint64(bc.Height() + 1)
	records, err := bc.bftJournal.PendingRecords(bc.v2Config.ChainID, height, id)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(bc.storage.dir, recoveryFile)
	st, err := os.Lstat(path)
	if os.IsNotExist(err) && len(records) == 0 {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("recovery snapshot required: %w", err)
	}
	if !st.Mode().IsRegular() || st.Size() > recoveryLimit {
		return nil, fmt.Errorf("invalid recovery file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var env recoveryEnvelope
	if err = strictRecoveryJSON(raw, &env); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(env.Payload)
	if env.SHA256 != hex.EncodeToString(digest[:]) {
		return nil, fmt.Errorf("recovery checksum mismatch")
	}
	var s consensusRecovery
	if err = strictRecoveryJSON(env.Payload, &s); err != nil {
		return nil, err
	}
	if s.Version != 1 || s.ChainID != bc.v2Config.ChainID || !strings.EqualFold(s.ValidatorID, id) || s.ConfigHash != bc.recoveryConfigHash() {
		return nil, fmt.Errorf("recovery identity/config mismatch")
	}
	if s.Height < height && len(records) == 0 {
		return nil, nil
	} // finalized heights no longer constrain signing
	if s.Height != height || !strings.EqualFold(s.ParentHash, bc.HeadSnapshot().Hash) {
		return nil, fmt.Errorf("recovery height/parent mismatch")
	}
	if s.ReservedRound >= bc.v2Config.ConsensusNetwork.MaxRound {
		return nil, fmt.Errorf("recovery exhausted consensus rounds")
	}
	set := bc.CurrentValidatorSet(height)
	checkValue := func(round int64, hash string, b *Block, qc *consensus.PrevoteCertificate, requireBlock bool) error {
		if round == -1 {
			if hash != "" || b != nil || qc != nil {
				return fmt.Errorf("unexpected recovery value")
			}
			return nil
		}
		if round < 0 || uint64(round) > s.ReservedRound || qc == nil || qc.ChainID != s.ChainID || qc.Height != height || qc.Round != uint64(round) || !strings.EqualFold(qc.BlockHash, hash) || hash == consensus.NilBlockHash {
			return fmt.Errorf("invalid recovery certificate coordinates")
		}
		if err := qc.Verify(set); err != nil {
			return fmt.Errorf("recovery QC: %w", err)
		}
		if b == nil {
			if requireBlock {
				return fmt.Errorf("missing locked block")
			}
			return nil
		}
		if uint64(b.Index) != height || b.ConsensusRound != uint64(round) || !strings.EqualFold(b.Hash, hash) {
			return fmt.Errorf("recovery block/QC mismatch")
		}
		return bc.ValidateConsensusProposalForVote(b)
	}
	if err = checkValue(s.LockedRound, s.LockedHash, s.LockedBlock, s.LockedQC, true); err != nil {
		return nil, err
	}
	if err = checkValue(s.ValidRound, s.ValidHash, s.ValidBlock, s.ValidQC, false); err != nil {
		return nil, err
	}
	if s.ValidRound < s.LockedRound {
		return nil, fmt.Errorf("recovery valid round below lock")
	}
	for _, rec := range records {
		if rec.Height != height || rec.Round > s.ReservedRound || !strings.EqualFold(rec.ValidatorSetRoot, set.Root()) {
			return nil, fmt.Errorf("journal ahead of recovery snapshot")
		}
		// Every precommit must have been preceded by a durable certified lock.
		if rec.Step == consensus.StepPrecommit && (s.LockedRound < int64(rec.Round) || (s.LockedRound == int64(rec.Round) && !strings.EqualFold(s.LockedHash, rec.BlockHash))) {
			return nil, fmt.Errorf("journal precommit not covered by recovered lock")
		}
	}
	return &s, nil
}

func strictRecoveryJSON(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing recovery data")
	}
	return nil
}

func writeRecovery(path string, s *consensusRecovery) error {
	payload, err := json.Marshal(s)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	raw, err := json.Marshal(recoveryEnvelope{payload, hex.EncodeToString(sum[:])})
	if err != nil {
		return err
	}
	if len(raw) > recoveryLimit {
		return fmt.Errorf("recovery snapshot too large")
	}
	if st, e := os.Lstat(path); e == nil && !st.Mode().IsRegular() {
		return fmt.Errorf("recovery target not regular")
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".recovery-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func (r *ConsensusReactor) durableRecovery() bool {
	return r.signer != nil && r.bc.storage != nil && r.bc.storage.durable
}
func (r *ConsensusReactor) recoveryFailureLocked(err error) error {
	r.recoveryErr = fmt.Errorf("persistent consensus recovery: %w", err)
	r.running = false
	r.runningState.Store(false)
	return r.recoveryErr
}
func (r *ConsensusReactor) persistRecoveryLocked(reserve uint64) error {
	if r.recoveryErr != nil {
		return r.recoveryErr
	}
	if !r.durableRecovery() {
		return nil
	}
	if reserve < r.round {
		reserve = r.round
	}
	if r.validRound >= 0 && reserve < uint64(r.validRound) {
		reserve = uint64(r.validRound)
	}
	if reserve < r.reservedRound {
		reserve = r.reservedRound
	}
	s := &consensusRecovery{Version: 1, ChainID: r.bc.v2Config.ChainID, Height: r.height, ReservedRound: reserve, ValidatorID: r.validatorID, ParentHash: r.bc.HeadSnapshot().Hash, ConfigHash: r.bc.recoveryConfigHash(), LockedRound: r.lockedRound, ValidRound: r.validRound, LockedHash: r.lockedHash, ValidHash: r.validHash, LockedBlock: r.lockedBlock, ValidBlock: r.validBlock, LockedQC: r.lockedQC, ValidQC: r.validQC}
	if err := writeRecovery(filepath.Join(r.bc.storage.dir, recoveryFile), s); err != nil {
		return r.recoveryFailureLocked(err)
	}
	r.reservedRound = reserve
	return nil
}

func (r *ConsensusReactor) restoreRecoveryLocked() error {
	if r.recoveryErr != nil {
		return r.recoveryErr
	}
	if !r.durableRecovery() {
		return nil
	}
	s, err := r.bc.readRecovery(r.validatorID)
	if err != nil {
		return r.recoveryFailureLocked(err)
	}
	if s == nil {
		return nil
	}
	r.height = s.Height
	r.round = s.ReservedRound
	r.reservedRound = s.ReservedRound
	r.lockedRound, r.lockedHash, r.lockedBlock, r.lockedQC = s.LockedRound, s.LockedHash, s.LockedBlock, s.LockedQC
	r.validRound, r.validHash, r.validBlock, r.validQC = s.ValidRound, s.ValidHash, s.ValidBlock, s.ValidQC
	return nil
}

// An uncertain journal write poisons the running validator as well as the
// journal object; a subsequent timeout must not silently continue signing.
func (r *ConsensusReactor) signingErrorLocked(err error) error {
	if r.durableRecovery() {
		if health := r.bc.bftJournal.Healthy(); health != nil {
			return r.recoveryFailureLocked(health)
		}
	}
	return err
}
