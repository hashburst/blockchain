package blockchain

// HVM pacemaker recovery keeps signed rounds monotonic beyond the former
// lifetime budget on both production HVM networks. Legacy and unrelated chain IDs
// retain their existing policy.
// MaxRound remains the message retention/future window and timeout-backoff cap.
// int64 bounds are required by validRound/lockedRound and leave room for +1.
const lastSafeConsensusRound uint64 = 1<<63 - 2

func usesSlidingConsensusRoundWindow(chainID uint64) bool {
	return chainID == 4735489 || chainID == 4735490
}

func consensusRoundLimit(cfg ProtocolV2Config) uint64 {
	if usesSlidingConsensusRoundWindow(cfg.ChainID) {
		return lastSafeConsensusRound
	}
	return cfg.ConsensusNetwork.MaxRound
}

func (r *ConsensusReactor) roundLimit() uint64 { return consensusRoundLimit(r.bc.v2Config) }

func (r *ConsensusReactor) acceptsRound(round uint64) bool {
	if round > r.roundLimit() {
		return false
	}
	if !usesSlidingConsensusRoundWindow(r.bc.v2Config.ChainID) {
		return true
	}
	if round >= r.round {
		return round-r.round <= r.cfg.MaxRound
	}
	return r.round-round <= r.cfg.MaxRound
}

func pruneOldRounds[T any](m map[uint64]T, floor uint64) {
	for round := range m {
		if round < floor {
			delete(m, round)
		}
	}
}

func (r *ConsensusReactor) pruneRoundWindowLocked() {
	if !usesSlidingConsensusRoundWindow(r.bc.v2Config.ChainID) || r.round <= r.cfg.MaxRound {
		return
	}
	floor := r.round - r.cfg.MaxRound
	pruneOldRounds(r.proposals, floor)
	pruneOldRounds(r.prevotes, floor)
	pruneOldRounds(r.precommits, floor)
	pruneOldRounds(r.prevoteQCs, floor)
	pruneOldRounds(r.precommitQCs, floor)
	pruneOldRounds(r.seenPrevotes, floor)
	pruneOldRounds(r.seenPrecommits, floor)
	pruneOldRounds(r.roundChanges, r.round)
	// Certified lock/value objects live separately and are never discarded.
	keep := map[string]bool{r.lockedHash: true, r.validHash: true}
	for _, p := range r.proposals {
		keep[p.Block.Hash] = true
	}
	for hash := range r.blocksByHash {
		if !keep[hash] {
			delete(r.blocksByHash, hash)
		}
	}
}
