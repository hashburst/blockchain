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

// VoteJournal is a local, validator-side double-sign guard. It is deliberately
// NOT consensus state and never contains a private key. A validator must not
// sign two different proposal hashes at the same chain/height, even across
// different rounds. Persisting the first vote before returning it to the
// caller makes restart-induced double-signing fail closed.
type VoteJournal struct {
	mu      sync.Mutex
	path    string
	entries map[string]Vote
}

func OpenVoteJournal(path string) (*VoteJournal, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("vote journal path required")
	}
	j := &VoteJournal{path: path, entries: make(map[string]Vote)}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return j, nil
		}
		return nil, fmt.Errorf("open vote journal: %w", err)
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	// A vote record is small, but raise the scanner ceiling so a future metadata
	// extension cannot silently make an otherwise valid journal unreadable.
	s.Buffer(make([]byte, 4096), 1024*1024)
	line := 0
	for s.Scan() {
		line++
		raw := strings.TrimSpace(s.Text())
		if raw == "" {
			continue
		}
		var v Vote
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, fmt.Errorf("vote journal line %d: %w", line, err)
		}
		if err := validateJournalVote(v); err != nil {
			return nil, fmt.Errorf("vote journal line %d: %w", line, err)
		}
		key := voteJournalKey(v)
		if previous, ok := j.entries[key]; ok {
			if !sameVoteValue(previous, v) {
				return nil, fmt.Errorf("vote journal contains conflicting votes at chain=%d height=%d validator=%s", v.ChainID, v.Height, v.ValidatorID)
			}
			continue
		}
		j.entries[key] = v
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("read vote journal: %w", err)
	}
	return j, nil
}

func validateJournalVote(v Vote) error {
	if v.ChainID == 0 {
		return fmt.Errorf("vote chain_id must be non-zero")
	}
	if v.Height == 0 {
		return fmt.Errorf("vote height must be non-zero")
	}
	if strings.TrimSpace(v.ValidatorID) == "" {
		return fmt.Errorf("vote validator_id required")
	}
	if len(strings.TrimPrefix(v.BlockHash, "0x")) != 64 {
		return fmt.Errorf("vote block_hash must be 32 bytes")
	}
	if len(strings.TrimPrefix(v.ValidatorSetRoot, "0x")) != 64 {
		return fmt.Errorf("vote validator_set_root must be 32 bytes")
	}
	return nil
}

func voteJournalKey(v Vote) string {
	return fmt.Sprintf("%d/%020d/%s", v.ChainID, v.Height, strings.ToLower(canonicalHex(v.ValidatorID)))
}

func sameVoteValue(a, b Vote) bool {
	return a.ChainID == b.ChainID &&
		a.Height == b.Height &&
		a.Round == b.Round &&
		strings.EqualFold(a.ValidatorID, b.ValidatorID) &&
		strings.EqualFold(a.BlockHash, b.BlockHash) &&
		strings.EqualFold(a.ValidatorSetRoot, b.ValidatorSetRoot)
}

// Record persists a signed vote if this validator has not already voted at the
// same chain/height. Re-recording the exact same vote is idempotent. Any
// different proposal (including one in a later round) is rejected locally.
func (j *VoteJournal) Record(v Vote) error {
	if j == nil {
		return fmt.Errorf("vote journal unavailable")
	}
	if err := validateJournalVote(v); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()

	key := voteJournalKey(v)
	if previous, ok := j.entries[key]; ok {
		if sameVoteValue(previous, v) {
			return nil
		}
		return fmt.Errorf("double-sign protection: validator %s already voted at chain=%d height=%d for block %s (round=%d)", v.ValidatorID, v.ChainID, v.Height, previous.BlockHash, previous.Round)
	}

	if err := os.MkdirAll(filepath.Dir(j.path), 0700); err != nil {
		return fmt.Errorf("create vote journal directory: %w", err)
	}
	encoded, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode vote journal record: %w", err)
	}
	f, err := os.OpenFile(j.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open vote journal for append: %w", err)
	}
	if _, err := f.Write(append(encoded, '\n')); err != nil {
		_ = f.Close()
		return fmt.Errorf("append vote journal: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync vote journal: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close vote journal: %w", err)
	}
	// Update memory only after the journal record is durable.
	j.entries[key] = v
	return nil
}

func (j *VoteJournal) Get(chainID, height uint64, validatorID string) (Vote, bool) {
	if j == nil {
		return Vote{}, false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	key := fmt.Sprintf("%d/%020d/%s", chainID, height, strings.ToLower(canonicalHex(validatorID)))
	v, ok := j.entries[key]
	return v, ok
}
