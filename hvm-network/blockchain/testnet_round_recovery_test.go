package blockchain

import (
	"bytes"
	"hashburst/consensus"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTestnetRoundExhaustionMixedLocksRecoverAndFinalize(t *testing.T) {
	cfg := phase3CTestConfig()
	cfg.ChainID = 4735490
	s := setupPhase3DChainsWithConfig(t, cfg)
	proposer, _ := s.nodes[0].CurrentValidatorSet(7).Proposer(7, 0)
	v := s.byID[strings.ToLower(proposer.ID)]
	b, err := s.nodes[0].BuildConsensusProposal(v.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	h, err := consensus.NewSignedProposalHeader(s.cfg.ChainID, 7, 0, b.Hash, b.ValidatorSetRoot, v.id, b.ValidRound, v.consensus)
	if err != nil {
		t.Fatal(err)
	}
	p := ConsensusProposal{h, b}
	rs := make([]*ConsensusReactor, 4)
	for i, bc := range s.nodes {
		bc.storage.durable = true
		r, e := NewConsensusReactor(bc, s.vals[i].id, s.vals[i].consensus, nil)
		if e != nil {
			t.Fatal(e)
		}
		r.height = 7
		r.running = true
		r.runningState.Store(true)
		r.step = consensus.StepProposal
		if i < 2 {
			applyRecoveryQC(t, s, r, p)
		}
		// Both unlocked validators ran out of the former lifetime budget
		// while locked validators were still replaying their chain.
		r.round = 64
		if i >= 2 {
			r.round = 256
		}
		r.step = consensus.StepProposal
		if e = r.HandleTimeout(); e != nil {
			t.Fatal(e)
		}
		if e = r.persistRecoveryLocked(r.round); e != nil {
			t.Fatal(e)
		}
		r.Stop()
		paths := []string{recoveryFile, "consensus-bft-signatures.jsonl", "blockchain.dat", "blockchain.idx"}
		before := map[string][]byte{}
		for _, name := range paths {
			before[name], e = os.ReadFile(filepath.Join(bc.storage.dir, name))
			if e != nil {
				t.Fatal(e)
			}
		}
		next := bc.v2Config.detached()
		next.EVM = &EVMConfig{ActivationHeight: 10000, GasLimit: 200000, BaseFeeWei: 1}
		fresh, e := OpenExistingBlockchain(bc.storage.dir, next, testHistoryBlock(t,bc,0).Hash, 6, testHistoryBlock(t,bc,6).Hash)
		if e != nil {
			t.Fatal(e)
		}
		if e = fresh.CheckValidatorRestart(r.validatorID); e != nil {
			t.Fatal(e)
		}
		for name, want := range before {
			got, _ := os.ReadFile(filepath.Join(bc.storage.dir, name))
			if !bytes.Equal(got, want) {
				t.Fatal("recovery check rewrote", name)
			}
		}
		rs[i], e = NewConsensusReactor(fresh, r.validatorID, r.signer, nil)
		if e != nil {
			t.Fatal(e)
		}
		s.nodes[i] = fresh
	}
	bus := &testConsensusBus{reactors: rs, active: []bool{true, true, true, true}}
	for i, r := range rs {
		r.SetTransport(&testConsensusEndpoint{bus: bus, self: i})
		defer r.Stop()
	}
	for i, r := range rs {
		if err := r.Start(); err != nil {
			t.Fatal(err)
		}
		if r.round <= 64 {
			t.Fatal("round reused")
		}
		if i < 2 && (r.lockedHash != b.Hash || r.lockedRound != 0 || r.lockedQC == nil) {
			t.Fatal("lost lock")
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
	for n := 0; n < 10 && !done(); n++ {
		bus.drainUntil(20000, done)
		if !done() {
			for _, r := range rs {
				if e := r.HandleTimeout(); e != nil {
					t.Fatal(e)
				}
			}
		}
	}
	if !done() {
		t.Fatal("no finality after recovery")
	}
	for _, bc := range s.nodes {
		if testHistoryBlock(t,bc,7).Hash != b.Hash {
			t.Fatal("conflicting finality")
		}
	}
}
