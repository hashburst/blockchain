package blockchain

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"

	"hashburst/consensus"
)

func TestR6ResourceDegradationDoesNotChangeConsensusAuthority(t *testing.T) {
	s := setupPhase3DChains(t)
	bc := s.nodes[0]

	height := uint64(bc.Height() + 1)
	round := uint64(0)

	before := bc.CurrentValidatorSet(height)
	if len(before.Validators) == 0 {
		t.Fatal("fixture has no active validators")
	}

	beforeRoot := before.Root()
	beforeCount := len(before.Validators)
	beforePower := before.TotalPower()
	beforeQuorum := consensus.QuorumThreshold(beforePower)

	beforeProposer, ok := before.Proposer(height, round)
	if !ok {
		t.Fatal("fixture has no proposer")
	}

	target := before.Validators[0]
	targetPeer := peer.ID(target.PeerID)
	if targetPeer == "" {
		targetPeer = peer.ID("operational-peer-for-validator")
	}

	p := NewAdaptiveRoutingPolicy()

	for i := 0; i < 3; i++ {
		p.ObserveFailure(targetPeer)
	}

	if got := p.OperationalState(targetPeer); got != PeerOperationalDegraded {
		t.Fatalf("resource mutation did not degrade target peer: got=%s", got)
	}

	after := bc.CurrentValidatorSet(height)

	if len(after.Validators) != beforeCount {
		t.Fatalf(
			"resource degradation changed validator count: before=%d after=%d",
			beforeCount,
			len(after.Validators),
		)
	}

	if after.Root() != beforeRoot {
		t.Fatalf(
			"resource degradation changed validator-set root: before=%s after=%s",
			beforeRoot,
			after.Root(),
		)
	}

	if after.TotalPower() != beforePower {
		t.Fatalf(
			"resource degradation changed total voting power: before=%d after=%d",
			beforePower,
			after.TotalPower(),
		)
	}

	afterQuorum := consensus.QuorumThreshold(after.TotalPower())
	if afterQuorum != beforeQuorum {
		t.Fatalf(
			"resource degradation changed quorum threshold: before=%d after=%d",
			beforeQuorum,
			afterQuorum,
		)
	}

	afterProposer, ok := after.Proposer(height, round)
	if !ok {
		t.Fatal("proposer disappeared after resource degradation")
	}

	if afterProposer.ID != beforeProposer.ID {
		t.Fatalf(
			"resource degradation changed proposer: before=%s after=%s",
			beforeProposer.ID,
			afterProposer.ID,
		)
	}

	if _, _, ok := after.Find(target.ID); !ok {
		t.Fatalf(
			"operationally degraded validator lost consensus authority: validator=%s",
			target.ID,
		)
	}
}
