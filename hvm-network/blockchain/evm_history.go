package blockchain

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/core/state"
)

// This read-only index is not part of consensus or the persistent ledger. It is
// rebuilt during verified replay, and populated only after canonical commit.
// Retention is bounded: this node is not an archive-state RPC server.
const evmReadHistoryLimit = 256

type evmReadSnapshot struct {
	height int
	hash   string
	db     *state.StateDB
}

type evmReadHistory struct {
	slots [evmReadHistoryLimit]evmReadSnapshot
}

// Callers serialize access with Blockchain.mu. Never publish speculative state.
func (h *evmReadHistory) remember(b *Block, s *State) {
	if b == nil || b.Index < 0 || s == nil || s.evm == nil {
		return
	}
	h.slots[b.Index%evmReadHistoryLimit] = evmReadSnapshot{
		height: b.Index, hash: b.Hash, db: s.evm.db.Copy(),
	}
}

func (h *evmReadHistory) snapshot(b *Block) (*state.StateDB, error) {
	if h != nil && b != nil && b.Index >= 0 {
		s := h.slots[b.Index%evmReadHistoryLimit]
		if s.db != nil && s.height == b.Index && strings.EqualFold(s.hash, b.Hash) {
			return s.db.Copy(), nil
		}
	}
	return nil, fmt.Errorf("historical EVM state unavailable: outside retained canonical window (%d blocks)", evmReadHistoryLimit)
}

func (bc *Blockchain) rememberEVMStateLocked() {
	if bc.state == nil || bc.state.evm == nil || bc.blockCountLocked() == 0 {
		return
	}
	if bc.evmReadHistory == nil {
		bc.evmReadHistory = &evmReadHistory{}
	}
	bc.evmReadHistory.remember(bc.headLocked(), bc.state)
}
