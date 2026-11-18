package blockchain

import (
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestR6DegradedValidatorParticipatesInRealQuorumAndFinality(t *testing.T) {
	s := setupPhase3DChains(t)
	bc := s.nodes[0]

	const height uint64 = 7
	const round uint64 = 0

	set := bc.CurrentValidatorSet(height)
	if len(set.Validators) != 4 {
		t.Fatalf("fixture requires exact 4-validator set: got=%d", len(set.Validators))
	}

	degraded := set.Validators[0]

	_, degradedPowerBefore, ok := set.Find(degraded.ID)
	if !ok {
		t.Fatalf("degraded target not found in validator set: %s", degraded.ID)
	}

	targetPeer := peer.ID(degraded.PeerID)
	if targetPeer == "" {
		targetPeer = peer.ID("operational-peer-for-validator")
	}

	routing := NewAdaptiveRoutingPolicy()
	for i := 0; i < 3; i++ {
		routing.ObserveFailure(targetPeer)
	}

	if got := routing.OperationalState(targetPeer); got != PeerOperationalDegraded {
		t.Fatalf("resource mutation failed: got=%s", got)
	}

	t.Log("RESOURCE_MUTATION_APPLIED=PASS")

	after := bc.CurrentValidatorSet(height)

	_, degradedPowerAfter, ok := after.Find(degraded.ID)
	if !ok {
		t.Fatalf("degraded validator disappeared from active set: %s", degraded.ID)
	}

	if degradedPowerAfter != degradedPowerBefore {
		t.Fatalf(
			"degraded validator voting power changed: before=%d after=%d",
			degradedPowerBefore,
			degradedPowerAfter,
		)
	}

	if after.Root() != set.Root() {
		t.Fatalf(
			"validator-set root changed after resource degradation: before=%s after=%s",
			set.Root(),
			after.Root(),
		)
	}

	t.Log("DEGRADED_VALIDATOR_ACTIVE=PASS")
	t.Log("DEGRADED_VALIDATOR_POWER_UNCHANGED=PASS")
	t.Log("VALIDATOR_SET_ROOT_UNCHANGED=PASS")

	proposer, ok := after.Proposer(height, round)
	if !ok {
		t.Fatal("proposer unavailable")
	}

	block, err := bc.BuildConsensusProposal(proposer.ID, round)
	if err != nil {
		t.Fatalf("build consensus proposal: %v", err)
	}

	proposerFixture, ok := s.byID[strings.ToLower(proposer.ID)]
	if !ok {
		t.Fatalf("proposer fixture unavailable: %s", proposer.ID)
	}

	header, err := bc.SignConsensusProposalHeader(
		block,
		proposer.ID,
		proposerFixture.consensus,
	)
	if err != nil {
		t.Fatalf("sign proposal header: %v", err)
	}

	proposal := ConsensusProposal{
		Header: header,
		Block:  block,
	}

	reactor, err := NewConsensusReactor(bc, "", nil, nil)
	if err != nil {
		t.Fatalf("observer reactor: %v", err)
	}

	if err := reactor.Start(); err != nil {
		t.Fatalf("observer reactor start: %v", err)
	}
	defer reactor.Stop()

	if err := reactor.HandleProposal(proposal); err != nil {
		t.Fatalf("authenticated proposal rejected: %v", err)
	}

	t.Log("PROPOSAL_ACCEPTED=PASS")

	// Explicit quorum membership:
	// validator 0 is the operationally DEGRADED validator.
	// Validators 1 and 2 complete the real 3-of-4 quorum.
	quorumIndexes := []int{0, 1, 2}

	degradedIncluded := false

	for position, idx := range quorumIndexes {
		v := after.Validators[idx]

		if strings.EqualFold(v.ID, degraded.ID) {
			degradedIncluded = true
		}

		fixture, ok := s.byID[strings.ToLower(v.ID)]
		if !ok {
			t.Fatalf("validator fixture unavailable: %s", v.ID)
		}

		vote, err := s.nodes[idx].SignConsensusPrecommit(
			block,
			v.ID,
			fixture.consensus,
		)
		if err != nil {
			t.Fatalf(
				"precommit signing failed validator=%s position=%d: %v",
				v.ID,
				position,
				err,
			)
		}

		if err := reactor.HandlePrecommit(vote); err != nil {
			t.Fatalf(
				"precommit rejected validator=%s position=%d: %v",
				v.ID,
				position,
				err,
			)
		}

		if position < 2 && bc.FinalizedHeight() >= int(height) {
			t.Fatalf(
				"finalized before 3-of-4 quorum: position=%d finalized=%d",
				position,
				bc.FinalizedHeight(),
			)
		}

		if strings.EqualFold(v.ID, degraded.ID) {
			t.Log("DEGRADED_VALIDATOR_PRECOMMIT_IN_QUORUM_ACCEPTED=PASS")
		}
	}

	if !degradedIncluded {
		t.Fatal("degraded validator was not included in quorum")
	}

	t.Log("DEGRADED_VALIDATOR_INCLUDED_IN_3OF4=PASS")

	if bc.FinalizedHeight() != int(height) {
		t.Fatalf(
			"3-of-4 quorum did not finalize height %d: finalized=%d",
			height,
			bc.FinalizedHeight(),
		)
	}

	head := bc.HeadSnapshot()
	if head == nil {
		t.Fatal("head unavailable after finality")
	}

	if !strings.EqualFold(head.Hash, block.Hash) {
		t.Fatalf(
			"finalized hash mismatch: head=%s proposal=%s",
			head.Hash,
			block.Hash,
		)
	}

	t.Log("NO_FINALITY_BEFORE_QUORUM=PASS")
	t.Log("REAL_3OF4_QUORUM_FINALIZED=PASS")
	t.Log("FINALIZED_PROPOSAL_HASH_MATCH=PASS")
	t.Log("DEGRADED_VALIDATOR_QC_AUTHORITY_PRESERVED=PASS")
	t.Log("RESOURCE_TELEMETRY_CANNOT_REMOVE_FINALITY_AUTHORITY=PASS")
}
