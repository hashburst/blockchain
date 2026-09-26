package blockchain

import (
	"strings"
	"testing"

	"hashburst/consensus"
)

func TestHVMNetworkChainSyncAdvancesRunningConsensusReactor(t *testing.T) {
	s := setupPhase3DChains(t)
	source := s.nodes[0]
	target := s.nodes[1]

	nextHeight := uint64(source.Height() + 1)
	set := source.CurrentValidatorSet(nextHeight)
	proposer, ok := set.Proposer(nextHeight, 0)
	if !ok {
		t.Fatal("scheduled proposer unavailable")
	}
	proposal, err := source.BuildConsensusProposal(proposer.ID, 0)
	if err != nil {
		t.Fatalf("build proposal: %v", err)
	}

	votes := make([]consensus.Vote, 0, 3)
	for i := 0; i < 3; i++ {
		v := set.Validators[i]
		fixture, ok := s.byID[strings.ToLower(v.ID)]
		if !ok {
			t.Fatalf("validator fixture %s missing", v.ID)
		}
		vote, err := consensus.NewSignedVote(s.cfg.ChainID, nextHeight, 0, proposal.Hash, proposal.ValidatorSetRoot, v.ID, fixture.consensus)
		if err != nil {
			t.Fatalf("precommit %d: %v", i, err)
		}
		votes = append(votes, vote)
	}
	qc, err := consensus.BuildQuorumCertificate(s.cfg.ChainID, nextHeight, 0, proposal.Hash, proposal.ValidatorSetRoot, set, votes)
	if err != nil {
		t.Fatalf("build QC: %v", err)
	}
	finalized := cloneBlockForConsensus(proposal)
	finalized.FinalityCertificate = cloneQC(&qc)

	reactor, err := NewConsensusReactor(target, s.vals[1].id, s.vals[1].consensus, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := reactor.Start(); err != nil {
		t.Fatalf("start target reactor: %v", err)
	}
	defer reactor.Stop()
	if got := reactor.Status().Height; got != nextHeight {
		t.Fatalf("initial reactor height=%d want=%d", got, nextHeight)
	}

	// Reproduce the real Phase 3E race at the authoritative chain boundary: a
	// QC-finalized block reaches this process through chain sync before the
	// consensus-finalized envelope. TryExtendOrAdopt must advance both the chain
	// and the attached running pacemaker; callers must not need a second hook.
	applied, err := target.TryExtendOrAdopt([]*Block{finalized})
	if err != nil {
		t.Fatalf("chain-sync finalized block: %v", err)
	}
	if !applied {
		t.Fatal("chain-sync finalized block was not applied")
	}
	if got := target.FinalizedHeight(); got != int(nextHeight) {
		t.Fatalf("finalized height=%d want=%d", got, nextHeight)
	}
	if got := reactor.Status().Height; got != nextHeight+1 {
		t.Fatalf("reactor remained stale after chain sync: height=%d want=%d", got, nextHeight+1)
	}

	// The dedicated finalized envelope may arrive later. It is a duplicate of
	// an already authenticated chain head and must be idempotent, not regress or
	// stall the reactor.
	if err := reactor.HandleFinalizedBlock(finalized); err != nil {
		t.Fatalf("duplicate consensus-finalized envelope: %v", err)
	}
	if got := reactor.Status().Height; got != nextHeight+1 {
		t.Fatalf("reactor height changed after duplicate finalized envelope: got=%d want=%d", got, nextHeight+1)
	}
}
