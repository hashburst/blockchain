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
	for _, r := range j.entries {
		if r.ChainID == chainID && strings.EqualFold(r.ValidatorID, validatorID) && r.Height >= height {
			return fmt.Errorf("pending signing journal at height %d: recover finalized state as observer before enabling validator", r.Height)
		}
	}
	return nil
}
