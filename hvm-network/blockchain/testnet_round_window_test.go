package blockchain

import (
	"context"
	"hashburst/consensus"
	"strings"
	"testing"
	"time"
)

func TestTestnetPacemakerPasses64WithoutQuorum(t *testing.T) {
	cfg := phase3CTestConfig()
	cfg.ChainID = 4735490
	s := setupPhase3DChainsWithConfig(t, cfg)
	s.nodes[0].storage.durable = true
	r, e := NewConsensusReactor(s.nodes[0], s.vals[0].id, s.vals[0].consensus, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Start(); e != nil {
		t.Fatal(e)
	}
	defer r.Stop()
	for r.round < 130 {
		if e = r.HandleTimeout(); e != nil {
			t.Fatal(e)
		}
	}
	if r.height != 7 || r.bc.FinalizedHeight() != 6 {
		t.Fatal("finality without quorum")
	}
	if len(r.roundChanges) > int(r.cfg.MaxRound)+1 || len(r.proposals) > int(r.cfg.MaxRound)+1 {
		t.Fatal("unbounded round cache")
	}
	next := reopenRecovery(t, r)
	if e = next.Start(); e != nil {
		t.Fatal(e)
	}
	defer next.Stop()
	if next.round <= 130 {
		t.Fatal("restart reused signed round")
	}
	if next.acceptsRound(next.round + next.cfg.MaxRound + 1) {
		t.Fatal("unbounded future vote admission")
	}
}

func TestConsensusTimeoutErrorTerminatesRun(t *testing.T) {
	s := setupPhase3DChains(t)
	r, e := NewConsensusReactor(s.nodes[0], "", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	r.height = 7
	r.round = r.roundLimit() - 1
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	for !r.Running() && ctx.Err() == nil {
		time.Sleep(time.Millisecond)
	}
	r.mu.Lock()
	r.step = consensus.StepPrevote
	r.deadline = time.Now().Add(-time.Second)
	r.mu.Unlock()
	e = <-done
	if e == nil || !strings.Contains(e.Error(), "exhausted") || r.Running() {
		t.Fatalf("terminal error hidden: %v", e)
	}
}

func TestTestnetCatchupNeedsTwoDistinctAuthenticatedValidators(t *testing.T) {
	cfg := phase3CTestConfig()
	cfg.ChainID = 4735490
	s := setupPhase3DChainsWithConfig(t, cfg)
	s.nodes[0].storage.durable = true
	r, e := NewConsensusReactor(s.nodes[0], "", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Start(); e != nil {
		t.Fatal(e)
	}
	defer r.Stop()
	for round := uint64(100); round <= 300; round++ {
		v := s.vals[0]
		rc, e := consensus.NewSignedRoundChange(cfg.ChainID, 7, round, s.nodes[0].CurrentValidatorSet(7).Root(), v.id, v.consensus)
		if e != nil {
			t.Fatal(e)
		}
		if e = r.HandleRoundChange(rc); e != nil {
			t.Fatal(e)
		}
	}
	if r.round != 0 || len(r.roundChanges) != 1 {
		t.Fatal("single signer moved round or grew cache")
	}
	v := s.vals[1]
	rc, e := consensus.NewSignedRoundChange(cfg.ChainID, 7, 300, s.nodes[0].CurrentValidatorSet(7).Root(), v.id, v.consensus)
	if e != nil {
		t.Fatal(e)
	}
	rc.Signature = "bad"
	if r.HandleRoundChange(rc) == nil {
		t.Fatal("invalid signature accepted")
	}
	rc, e = consensus.NewSignedRoundChange(cfg.ChainID, 7, 300, s.nodes[0].CurrentValidatorSet(7).Root(), v.id, v.consensus)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.HandleRoundChange(rc); e != nil {
		t.Fatal(e)
	}
	if r.round != 300 {
		t.Fatal("authenticated catchup failed")
	}
}
