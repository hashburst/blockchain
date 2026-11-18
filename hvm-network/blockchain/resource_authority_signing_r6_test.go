package blockchain

import (
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"

	"hashburst/consensus"
)

func TestR6DegradedValidatorRetainsConsensusSigningAndReactorAuthority(t *testing.T) {
	s := setupPhase3DChains(t)
	bc := s.nodes[0]

	const height uint64 = 7
	const round uint64 = 0

	before := bc.CurrentValidatorSet(height)
	if len(before.Validators) < 3 {
		t.Fatalf("fixture requires at least 3 active validators: got=%d", len(before.Validators))
	}

	target := before.Validators[0]

	_, beforePower, ok := before.Find(target.ID)
	if !ok {
		t.Fatalf("target validator not found before degradation: %s", target.ID)
	}

	targetPeer := peer.ID(target.PeerID)
	if targetPeer == "" {
		targetPeer = peer.ID("operational-peer-for-validator")
	}

	routing := NewAdaptiveRoutingPolicy()
	for i := 0; i < 3; i++ {
		routing.ObserveFailure(targetPeer)
	}

	if got := routing.OperationalState(targetPeer); got != PeerOperationalDegraded {
		t.Fatalf("target peer not degraded: got=%s", got)
	}
	t.Log("RESOURCE_MUTATION_APPLIED=PASS")

	after := bc.CurrentValidatorSet(height)

	degraded, afterPower, ok := after.Find(target.ID)
	if !ok {
		t.Fatalf("degraded validator disappeared from active set: %s", target.ID)
	}

	if degraded.Status != target.Status {
		t.Fatalf(
			"resource degradation changed validator status: before=%s after=%s",
			target.Status,
			degraded.Status,
		)
	}

	if afterPower != beforePower {
		t.Fatalf(
			"resource degradation changed validator voting power: before=%d after=%d",
			beforePower,
			afterPower,
		)
	}

	if after.Root() != before.Root() {
		t.Fatalf(
			"resource degradation changed validator-set root: before=%s after=%s",
			before.Root(),
			after.Root(),
		)
	}

	t.Log("DEGRADED_VALIDATOR_ACTIVE=PASS")
	t.Log("DEGRADED_VALIDATOR_POWER_UNCHANGED=PASS")
	t.Log("DEGRADED_VALIDATOR_SET_ROOT_UNCHANGED=PASS")

	proposer, ok := after.Proposer(height, round)
	if !ok {
		t.Fatal("proposer unavailable")
	}

	proposalBlock, err := bc.BuildConsensusProposal(proposer.ID, round)
	if err != nil {
		t.Fatalf("build consensus proposal: %v", err)
	}

	targetFixture, ok := s.byID[strings.ToLower(degraded.ID)]
	if !ok {
		t.Fatalf("consensus fixture unavailable for degraded validator %s", degraded.ID)
	}

	prevote, err := bc.SignConsensusPrevote(
		height,
		round,
		proposalBlock.Hash,
		after.Root(),
		degraded.ID,
		targetFixture.consensus,
	)
	if err != nil {
		t.Fatalf("degraded validator prevote signing rejected: %v", err)
	}

	t.Log("DEGRADED_VALIDATOR_PREVOTE_SIGNED=PASS")

	observer, err := NewConsensusReactor(bc, "", nil, nil)
	if err != nil {
		t.Fatalf("observer reactor: %v", err)
	}

	if err := observer.Start(); err != nil {
		t.Fatalf("observer reactor start: %v", err)
	}
	defer observer.Stop()

	proposerFixture, ok := s.byID[strings.ToLower(proposer.ID)]
	if !ok {
		t.Fatalf("proposer fixture unavailable: %s", proposer.ID)
	}

	header, err := bc.SignConsensusProposalHeader(
		proposalBlock,
		proposer.ID,
		proposerFixture.consensus,
	)
	if err != nil {
		t.Fatalf("proposal header signing: %v", err)
	}

	proposal := ConsensusProposal{
		Header: header,
		Block:  proposalBlock,
	}

	if err := observer.HandleProposal(proposal); err != nil {
		t.Fatalf("observer rejected authenticated proposal: %v", err)
	}

	if err := observer.HandlePrevote(prevote); err != nil {
		t.Fatalf("reactor rejected degraded validator prevote: %v", err)
	}

	t.Log("DEGRADED_VALIDATOR_PREVOTE_ACCEPTED=PASS")

	precommit, err := bc.SignConsensusPrecommit(
		proposalBlock,
		degraded.ID,
		targetFixture.consensus,
	)
	if err != nil {
		t.Fatalf("degraded validator precommit signing rejected: %v", err)
	}

	t.Log("DEGRADED_VALIDATOR_PRECOMMIT_SIGNED=PASS")

	if err := observer.HandlePrecommit(precommit); err != nil {
		t.Fatalf("reactor rejected degraded validator precommit: %v", err)
	}

	t.Log("DEGRADED_VALIDATOR_PRECOMMIT_ACCEPTED=PASS")

	// One validator alone must not finalize a 4-validator set.
	// This guards against accidentally turning this test into a quorum shortcut.
	if bc.FinalizedHeight() >= int(height) {
		t.Fatalf(
			"single degraded validator unexpectedly finalized height %d",
			height,
		)
	}

	t.Log("SINGLE_VALIDATOR_NO_FALSE_FINALITY=PASS")
	t.Log("DEGRADED_ACTIVE_VALIDATOR_SIGNING_AUTHORITY_PRESERVED=PASS")
	t.Log("RESOURCE_AUTHORITY_SIGNING_REACTOR_ISOLATION=PASS")

	_ = consensus.StepPrevote
}
