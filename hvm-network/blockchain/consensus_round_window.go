package blockchain

// Testnet pacemaker recovery keeps signed rounds monotonic beyond the former
// lifetime budget. Legacy and other chain IDs retain their existing policy.
// MaxRound remains the message retention/future window and timeout-backoff cap.
// int64 bounds are required by validRound/lockedRound and leave room for +1.
const lastSafeConsensusRound uint64 = 1<<63 - 2

func consensusRoundLimit(cfg ProtocolV2Config) uint64 {
	if cfg.ChainID == 4735490 {
		return lastSafeConsensusRound
	}
	return cfg.ConsensusNetwork.MaxRound
}

func (r *ConsensusReactor) roundLimit() uint64 { return consensusRoundLimit(r.bc.v2Config) }

func (r *ConsensusReactor) acceptsRound(round uint64) bool {
	if round > r.roundLimit() {
		return false
	}
	if r.bc.v2Config.ChainID != 4735490 {
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
	if r.bc.v2Config.ChainID != 4735490 || r.round <= r.cfg.MaxRound {
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
