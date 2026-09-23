package consensus

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// BFTSignJournal is the Phase 3D fail-closed signing journal. Unlike the Phase
// 3C VoteJournal it keys the safety lock by height+round+step. This is required
// for legitimate view changes: an honest validator may vote for a different
// safe value in a later round, but may never equivocate within the same round
// and step.
type BFTSignJournal struct {
	mu      sync.Mutex
	path    string
	entries map[string]BFTSignRecord
}

type BFTSignRecord struct {
	Step             BFTStep `json:"step"`
	ChainID          uint64  `json:"chain_id"`
	Height           uint64  `json:"height"`
	Round            uint64  `json:"round"`
	ValidatorID      string  `json:"validator_id"`
	BlockHash        string  `json:"block_hash,omitempty"`
	ValidatorSetRoot string  `json:"validator_set_root"`
	Signature        string  `json:"signature"`
}

func OpenBFTSignJournal(path string) (*BFTSignJournal, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("BFT sign journal path required")
	}
	j := &BFTSignJournal{path: path, entries: make(map[string]BFTSignRecord)}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return j, nil
		}
		return nil, fmt.Errorf("open BFT sign journal: %w", err)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 1024*1024)
	line := 0
	for s.Scan() {
		line++
		raw := strings.TrimSpace(s.Text())
		if raw == "" {
			continue
		}
		var rec BFTSignRecord
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			return nil, fmt.Errorf("BFT sign journal line %d: %w", line, err)
		}
		if err := validateBFTSignRecord(rec); err != nil {
			return nil, fmt.Errorf("BFT sign journal line %d: %w", line, err)
		}
		key := bftJournalKey(rec)
		if previous, ok := j.entries[key]; ok {
			if !sameBFTSignRecord(previous, rec) {
				return nil, fmt.Errorf("BFT sign journal contains equivocation at %s", key)
			}
			continue
		}
		j.entries[key] = rec
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("read BFT sign journal: %w", err)
	}
	return j, nil
}

func validateBFTSignRecord(rec BFTSignRecord) error {
	if rec.ChainID == 0 || rec.Height == 0 || strings.TrimSpace(rec.ValidatorID) == "" || strings.TrimSpace(rec.ValidatorSetRoot) == "" || strings.TrimSpace(rec.Signature) == "" {
		return fmt.Errorf("incomplete BFT sign record")
	}
	switch rec.Step {
	case StepProposal, StepPrevote, StepPrecommit:
		if _, err := canonicalBFTBlockHash(rec.BlockHash); err != nil {
			return err
		}
	case StepRoundChange:
		if rec.Round == 0 {
			return fmt.Errorf("round-change record must target round > 0")
		}
	default:
		return fmt.Errorf("unsupported BFT journal step %q", rec.Step)
	}
	return nil
}

func bftJournalKey(rec BFTSignRecord) string {
	return fmt.Sprintf("%s/%d/%020d/%020d/%s", rec.Step, rec.ChainID, rec.Height, rec.Round, strings.ToLower(canonicalHex(rec.ValidatorID)))
}

func sameBFTSignRecord(a, b BFTSignRecord) bool {
	return a.Step == b.Step && a.ChainID == b.ChainID && a.Height == b.Height && a.Round == b.Round && strings.EqualFold(a.ValidatorID, b.ValidatorID) && strings.EqualFold(a.BlockHash, b.BlockHash) && strings.EqualFold(a.ValidatorSetRoot, b.ValidatorSetRoot) && strings.EqualFold(a.Signature, b.Signature)
}

func (j *BFTSignJournal) Record(rec BFTSignRecord) error {
	if j == nil {
		return fmt.Errorf("BFT sign journal unavailable")
	}
	if err := validateBFTSignRecord(rec); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	key := bftJournalKey(rec)
	if previous, ok := j.entries[key]; ok {
		if sameBFTSignRecord(previous, rec) {
			return nil
		}
		return fmt.Errorf("double-sign protection: conflicting %s already recorded at chain=%d height=%d round=%d validator=%s", rec.Step, rec.ChainID, rec.Height, rec.Round, rec.ValidatorID)
	}
	if err := os.MkdirAll(filepath.Dir(j.path), 0700); err != nil {
		return fmt.Errorf("create BFT journal directory: %w", err)
	}
	encoded, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("encode BFT journal record: %w", err)
	}
	f, err := os.OpenFile(j.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open BFT journal for append: %w", err)
	}
	if _, err := f.Write(append(encoded, '\n')); err != nil {
		_ = f.Close()
		return fmt.Errorf("append BFT journal: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync BFT journal: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close BFT journal: %w", err)
	}
	j.entries[key] = rec
	return nil
}

func (j *BFTSignJournal) Healthy() error {
	if j == nil {
		return fmt.Errorf("BFT sign journal unavailable")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return nil
}
