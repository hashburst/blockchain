package blockchain

import (
	"fmt"
	"strings"
	"testing"

	"hashburst/consensus"
)

func TestHVMNetworkRepeatedCertifiedReproposal(t *testing.T) {
	s := setupPhase3DChains(t)
	bc := s.nodes[0]
	set := bc.CurrentValidatorSet(7)
	proposer, _ := set.Proposer(7, 0)
	block, err := bc.BuildConsensusProposal(proposer.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	originalHash, originalAuthor := block.Hash, block.AuthorValidatorID
	for round := uint64(0); round < 3; round++ {
		var votes []consensus.Prevote
		for i := 0; i < 3; i++ {
			v := set.Validators[i]
			signer := s.byID[strings.ToLower(v.ID)].consensus
			vote, err := s.nodes[i].SignConsensusPrevote(7, round, block.Hash, set.Root(), v.ID, signer)
			if err != nil {
				t.Fatal(err)
			}
			votes = append(votes, vote)
		}
		qc, err := consensus.BuildPrevoteCertificate(s.cfg.ChainID, 7, round, block.Hash, set.Root(), set, votes)
		if err != nil {
			t.Fatal(err)
		}
		proposer, _ = set.Proposer(7, round+1)
		block, err = bc.ReproposeConsensusValue(proposer.ID, round+1, block, round, qc)
		if err != nil {
			t.Fatalf("reproposal into round %d: %v", round+1, err)
		}
		if block.Hash != originalHash || block.AuthorValidatorID != originalAuthor {
			t.Fatal("certified content or author changed")
		}
		for _, peer := range s.nodes {
			if err := peer.ValidateConsensusProposalForVote(block); err != nil {
				t.Fatal(err)
			}
		}
		for _, mutation := range []string{"author", "hash", "height", "root", "round", "signature", "quorum"} {
			bad := cloneBlockForConsensus(block)
			switch mutation {
			case "author":
				bad.AuthorValidatorID = proposer.ID
			case "hash":
				bad.Hash = strings.Repeat("0", 64)
			case "height":
				bad.ValidPrevoteCertificate.Height++
			case "root":
				bad.ValidPrevoteCertificate.ValidatorSetRoot = strings.Repeat("0", 64)
			case "round":
				bad.ValidPrevoteCertificate.Round++
			case "signature":
				bad.ValidPrevoteCertificate.Votes[0].Signature = "00"
			case "quorum":
				bad.ValidPrevoteCertificate.Votes = bad.ValidPrevoteCertificate.Votes[:1]
			}
			if err := bc.ValidateConsensusProposalForVote(bad); err == nil {
				t.Fatalf("tampered %s accepted", mutation)
			}
		}
		// A certificate for another chain must never authorize this value.
		bad := cloneBlockForConsensus(block)
		bad.ValidPrevoteCertificate.ChainID++
		if err := bc.ValidateConsensusProposalForVote(bad); err == nil {
			t.Fatal("foreign-chain certificate accepted")
		}
		foreignVotes := make([]consensus.Prevote, 0, 3)
		for i := 0; i < 3; i++ {
			v := set.Validators[i]
			vote, err := consensus.NewSignedPrevote(s.cfg.ChainID+1, 7, round, block.Hash, set.Root(), v.ID, s.byID[strings.ToLower(v.ID)].consensus)
			if err != nil {
				t.Fatal(err)
			}
			foreignVotes = append(foreignVotes, vote)
		}
		foreignQC, err := consensus.BuildPrevoteCertificate(s.cfg.ChainID+1, 7, round, block.Hash, set.Root(), set, foreignVotes)
		if err != nil {
			t.Fatal(err)
		}
		bad = cloneBlockForConsensus(block)
		bad.ValidPrevoteCertificate = &foreignQC
		if err := bc.ValidateConsensusProposalForVote(bad); err == nil {
			t.Fatal("valid foreign-chain QC accepted")
		}
		if _, err := bc.ReproposeConsensusValue(proposer.ID, round+1, block, round, foreignQC); err == nil {
			t.Fatal("built using foreign-chain QC")
		}
	}
}

func TestHVMNetworkResumeFourValidatorsAfterLostMessages(t *testing.T) {
	s := setupPhase3DChains(t)
	bus, reactors := attachPhase3DReactors(t, s, []bool{true, true, true, true})
	for _, r := range reactors {
		if err := r.Start(); err != nil {
			t.Fatal(err)
		}
	}
	// Deliver enough traffic to produce locks, then lose the remaining frames
	// at the administrative stop boundary (including possible precommits).
	bus.drainUntil(1000, func() bool {
		for _, r := range reactors {
			if r.lockedRound >= 0 {
				return true
			}
		}
		return false
	})
	for _, r := range reactors {
		r.Stop()
	}
	bus.queue = nil
	for _, r := range reactors {
		if err := r.Start(); err != nil {
			t.Fatal(err)
		}
	}
	done := func() bool {
		for _, bc := range s.nodes {
			if bc.FinalizedHeight() < 7 {
				return false
			}
		}
		return true
	}
	for attempt := 0; attempt < 12 && !done(); attempt++ {
		bus.drainUntil(20000, done)
		if done() {
			break
		}
		for _, r := range reactors {
			if err := r.HandleTimeout(); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, r := range reactors {
		r.Stop()
	}
	if !done() {
		t.Fatal("lost messages across resume stalled finality")
	}
	for _, bc := range s.nodes {
		if bc.Blocks[7].Hash != s.nodes[0].Blocks[7].Hash {
			t.Fatal("conflicting finalized blocks")
		}
	}
}

func TestHVMNetworkResumePreservesSafetyState(t *testing.T) {
	s := setupPhase3DChains(t)
	r, err := NewConsensusReactor(s.nodes[0], "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	// Observer fixture isolates lifecycle state retention from signing/building.
	r.round = 3
	r.lockedRound, r.validRound = 2, 2
	r.lockedHash, r.validHash = "retained", "retained"
	r.validQC = &consensus.PrevoteCertificate{Round: 2}
	r.validBlock = &Block{Index: 7}
	r.lockedBlock = r.validBlock
	qc, block := r.validQC, r.validBlock
	r.Stop()
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	if r.round != 4 || r.lockedRound != 2 || r.validRound != 2 || r.validQC != qc || r.validBlock != block || r.lockedBlock != block || r.lockedHash != "retained" {
		t.Fatalf("resume lost state: %+v", r.Status())
	}
	if !r.Running() || r.deadline.IsZero() {
		t.Fatal("resume did not activate pacemaker")
	}
	if err := r.Start(); err != nil || r.round != 4 {
		t.Fatal("Start not idempotent")
	}
	r.Stop()
	r.round = r.cfg.MaxRound
	if err := r.Start(); err == nil {
		t.Fatal("exhausted round resumed")
	}
	if r.Running() || r.running || !r.deadline.IsZero() {
		t.Fatal("resume failure not fail-closed")
	}
}

func TestHVMNetworkResumeSigningFailureStopsReactor(t *testing.T) {
	s := setupPhase3DChains(t)
	r, err := NewConsensusReactor(s.nodes[0], s.vals[0].id, s.vals[0].consensus, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	r.Stop()
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	rc, ok := r.roundChanges[1][strings.ToLower(s.vals[0].id)]
	if !ok {
		t.Fatal("resume did not produce signed round-change")
	}
	v, _, ok := s.nodes[0].CurrentValidatorSet(7).Find(s.vals[0].id)
	if !ok {
		t.Fatal("validator missing")
	}
	if err := consensus.VerifyRoundChange(rc, v); err != nil {
		t.Fatal(err)
	}
	r.Stop()
	s.nodes[0].bftJournalErr = fmt.Errorf("injected journal failure")
	if err := r.Start(); err == nil {
		t.Fatal("journal failure ignored")
	}
	if r.Running() || r.running || !r.deadline.IsZero() {
		t.Fatal("signing failure did not stop reactor")
	}
}
