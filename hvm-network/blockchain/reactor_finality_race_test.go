package blockchain

import (
	"errors"
	"hashburst/consensus"
	"strings"
	"testing"
)

func TestTimeoutReconcilesDurableHeadBeforeSyncCallback(t *testing.T) {
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

	reactor, err := NewConsensusReactor(target, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = reactor.Start(); err != nil {
		t.Fatal(err)
	}
	defer reactor.Stop()

	// Reproduce the interval after sync commits, before its callback obtains r.mu.
	target.mu.Lock()
	applied, err := target.extendLocked([]*Block{finalized})
	target.mu.Unlock()
	if err != nil || !applied {
		t.Fatalf("commit: applied=%v err=%v", applied, err)
	}
	if reactor.Status().Height != nextHeight {
		t.Fatal("fixture did not retain stale reactor")
	}
	if err = target.ValidateConsensusProposalForVote(proposal); !errors.Is(err, errProposalBehindFinality) {
		t.Fatalf("obsolete proposal classification: %v", err)
	}
	if err = reactor.HandleTimeout(); err != nil {
		t.Fatal(err)
	}
	if got := reactor.Status().Height; got != nextHeight+1 {
		t.Fatalf("reactor height=%d want=%d", got, nextHeight+1)
	}
	if !reactor.Running() {
		t.Fatal("reactor stopped")
	}
	if err = reactor.SyncToFinalizedHead(finalized); err != nil {
		t.Fatal(err)
	}
	if got := reactor.Status().Height; got != nextHeight+1 {
		t.Fatal("callback rewound reactor")
	}
}

func TestProposalIndexAheadIsNotStaleFinality(t *testing.T) {
	s := setupPhase3DChains(t)
	bc := s.nodes[0]
	height := uint64(bc.Height() + 1)
	proposer, _ := bc.CurrentValidatorSet(height).Proposer(height, 0)
	b, err := bc.BuildConsensusProposal(proposer.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	b.Index += 2
	err = bc.ValidateConsensusProposalForVote(b)
	if err == nil || errors.Is(err, errProposalBehindFinality) {
		t.Fatalf("invalid future index must remain rejected: %v", err)
	}
}
