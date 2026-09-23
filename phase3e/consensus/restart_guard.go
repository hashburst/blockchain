package consensus

import (
	"fmt"
	"strings"
)

// CheckRestartHeight is deliberately conservative: journaled signatures at a
// pending height need lock/QC recovery, which this runtime does not reconstruct.
func (j *BFTSignJournal) CheckRestartHeight(chainID, height uint64, validatorID string) error {
	if j == nil || strings.TrimSpace(validatorID) == "" {
		return fmt.Errorf("signing journal/validator unavailable")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.failure != nil {
		return j.failure
	}
	for _, r := range j.entries {
		if r.ChainID == chainID && strings.EqualFold(r.ValidatorID, validatorID) && r.Height >= height {
			return fmt.Errorf("pending signing journal at height %d: recover finalized state as observer before enabling validator", r.Height)
		}
	}
	return nil
}

// PendingRecords returns a copy for restart validation; it never permits signing.
func (j *BFTSignJournal) PendingRecords(chainID, height uint64, validatorID string) ([]BFTSignRecord, error) {
	if j == nil || strings.TrimSpace(validatorID) == "" {
		return nil, fmt.Errorf("signing journal/validator unavailable")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.failure != nil {
		return nil, j.failure
	}
	var out []BFTSignRecord
	for _, r := range j.entries {
		if strings.EqualFold(r.ValidatorID, validatorID) {
			if r.ChainID != chainID {
				return nil, fmt.Errorf("journal chain identity mismatch")
			}
			if r.Height >= height {
				out = append(out, r)
			}
		}
	}
	return out, nil
}
