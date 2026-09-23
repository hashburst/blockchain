package blockchain

import (
	"bytes"
	"encoding/json"
	"hashburst/consensus"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func recoveryFixture(t *testing.T) (phase3DSetup, *ConsensusReactor, ConsensusProposal) {
	t.Helper()
	s := setupPhase3DChains(t)
	bc := s.nodes[0]
	bc.storage.durable = true
	set := bc.CurrentValidatorSet(7)
	proposer, _ := set.Proposer(7, 0)
	val := s.byID[strings.ToLower(proposer.ID)]
	b, err := bc.BuildConsensusProposal(val.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	header, err := consensus.NewSignedProposalHeader(s.cfg.ChainID, 7, 0, b.Hash, b.ValidatorSetRoot, val.id, b.ValidRound, val.consensus)
	if err != nil {
		t.Fatal(err)
	}
	// Choose a non-proposer: Start must not sign or build before our injection.
	local := s.vals[0]
	if strings.EqualFold(local.id, val.id) {
		local = s.vals[1]
	}
	r, err := NewConsensusReactor(bc, local.id, local.consensus, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Start(); err != nil {
		t.Fatal(err)
	}
	return s, r, ConsensusProposal{header, b}
}
func reopenRecovery(t *testing.T, r *ConsensusReactor) *ConsensusReactor {
	t.Helper()
	bc := r.bc
	r.Stop()
	fresh, err := OpenExistingBlockchain(bc.storage.dir, bc.v2Config, bc.Blocks[0].Hash, 6, bc.Blocks[6].Hash)
	if err != nil {
		t.Fatal(err)
	}
	if err = fresh.CheckValidatorRestart(r.validatorID); err != nil {
		t.Fatal(err)
	}
	next, err := NewConsensusReactor(fresh, r.validatorID, r.signer, nil)
	if err != nil {
		t.Fatal(err)
	}
	return next
}
func applyRecoveryQC(t *testing.T, s phase3DSetup, r *ConsensusReactor, p ConsensusProposal) {
	t.Helper()
	if err := r.HandleProposal(p); err != nil {
		t.Fatal(err)
	}
	for _, v := range s.vals {
		pv, err := consensus.NewSignedPrevote(s.cfg.ChainID, 7, 0, p.Block.Hash, p.Block.ValidatorSetRoot, v.id, v.consensus)
		if err != nil {
			t.Fatal(err)
		}
		if err = r.HandlePrevote(pv); err != nil {
			t.Fatal(err)
		}
	}
	if r.lockedRound != 0 || r.step != consensus.StepPrecommit {
		t.Fatalf("lock not established: %+v", r.Status())
	}
}
func TestPersistentRecoveryPrevoteAndPrecommit(t *testing.T) {
	for _, locked := range []bool{false, true} {
		t.Run(map[bool]string{false: "prevote", true: "precommit"}[locked], func(t *testing.T) {
			s, r, p := recoveryFixture(t)
			if locked {
				applyRecoveryQC(t, s, r, p)
			} else if err := r.HandleProposal(p); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(r.bc.storage.dir, "consensus-bft-signatures.jsonl")
			before, _ := os.ReadFile(path)
			next := reopenRecovery(t, r)
			if err := next.Start(); err != nil {
				t.Fatal(err)
			}
			defer next.Stop()
			if next.Status().Round < 1 {
				t.Fatal("reused signed round")
			}
			if locked && (next.lockedRound != 0 || next.lockedHash != p.Block.Hash || next.lockedQC == nil || next.validBlock == nil) {
				t.Fatal("lost certified lock")
			}
			after, _ := os.ReadFile(path)
			if !bytes.HasPrefix(after, before) {
				t.Fatal("journal rewritten")
			}
			// A second crash/restart must again reserve a fresh view, never replay votes.
			again := reopenRecovery(t, next)
			oldRound := next.round
			if err := again.Start(); err != nil {
				t.Fatal(err)
			}
			defer again.Stop()
			if again.round <= oldRound {
				t.Fatal("repeated restart reused a view")
			}
		})
	}
}
func TestPersistentRecoveryMissingCorruptOrOlderSnapshotRefusesSigning(t *testing.T) {
	for _, mode := range []string{"missing", "corrupt", "older", "identity", "qc", "future-journal"} {
		t.Run(mode, func(t *testing.T) {
			s, r, p := recoveryFixture(t)
			if err := r.HandleProposal(p); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(r.bc.storage.dir, recoveryFile)
			older, _ := os.ReadFile(path)
			applyRecoveryQC(t, s, r, p)
			r.Stop()
			switch mode {
			case "missing":
				os.Remove(path)
			case "corrupt":
				os.WriteFile(path, []byte("{broken"), 0600)
			case "older":
				os.WriteFile(path, older, 0600)
			case "identity", "qc":
				snap, err := r.bc.readRecovery(r.validatorID)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "identity" {
					snap.ChainID++
				} else {
					snap.LockedQC.Votes[0].Signature = "bad"
				}
				if err = writeRecovery(path, snap); err != nil {
					t.Fatal(err)
				}
			case "future-journal":
				recs, err := r.bc.bftJournal.PendingRecords(s.cfg.ChainID, 7, r.validatorID)
				if err != nil || len(recs) == 0 {
					t.Fatal(err)
				}
				rec := recs[0]
				rec.Height++
				if err = r.bc.bftJournal.Record(rec); err != nil {
					t.Fatal(err)
				}
			}
			journal := filepath.Join(r.bc.storage.dir, "consensus-bft-signatures.jsonl")
			before, _ := os.ReadFile(journal)
			if err := r.bc.CheckValidatorRestart(r.validatorID); err == nil {
				t.Fatal("accepted invalid recovery")
			}
			next, err := NewConsensusReactor(r.bc, r.validatorID, r.signer, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err = next.Start(); err == nil || next.Running() {
				t.Fatal("started without valid recovery")
			}
			after, _ := os.ReadFile(journal)
			if !bytes.Equal(before, after) {
				t.Fatal("signed on failed recovery")
			}
		})
	}
}
func TestPersistentRecoveryWriteFailureStopsSigning(t *testing.T) {
	_, r, p := recoveryFixture(t)
	path := filepath.Join(r.bc.storage.dir, recoveryFile)
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(r.bc.storage.dir, "consensus-bft-signatures.jsonl")
	before, _ := os.ReadFile(journal)
	if err := r.HandleProposal(p); err == nil || r.Running() {
		t.Fatal("disk failure did not stop reactor")
	}
	os.Remove(path)
	if err := r.Start(); err == nil {
		t.Fatal("latched recovery error cleared without restart")
	}
	after, _ := os.ReadFile(journal)
	if !bytes.Equal(before, after) {
		t.Fatal("signed after failed durability")
	}
}
func TestPersistentRecoveryReservationBeforeSignature(t *testing.T) {
	_, r, _ := recoveryFixture(t)
	// Crash immediately after durable round reservation, before signing it.
	if err := r.persistRecoveryLocked(3); err != nil {
		t.Fatal(err)
	}
	next := reopenRecovery(t, r)
	if err := next.Start(); err != nil {
		t.Fatal(err)
	}
	defer next.Stop()
	if next.round != 4 {
		t.Fatalf("expected fresh round 4, got %d", next.round)
	}
}
func TestPersistentRecoveryFourValidatorsColdRestartLocked(t *testing.T) {
	s := setupPhase3DChains(t)
	set := s.nodes[0].CurrentValidatorSet(7)
	proposer, _ := set.Proposer(7, 0)
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
		r, err := NewConsensusReactor(bc, s.vals[i].id, s.vals[i].consensus, nil)
		if err != nil {
			t.Fatal(err)
		}
		// Exact persisted precommit crash point without scheduling an extra proposal.
		r.height = 7
		r.running = true
		r.runningState.Store(true)
		r.step = consensus.StepProposal
		applyRecoveryQC(t, s, r, p)
		rs[i] = reopenRecovery(t, r)
		s.nodes[i] = rs[i].bc
	}
	bus := &testConsensusBus{reactors: rs, active: []bool{true, true, true, true}}
	for i, r := range rs {
		r.SetTransport(&testConsensusEndpoint{bus: bus, self: i})
		defer r.Stop()
	}
	for _, r := range rs {
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
	for attempt := 0; attempt < 10 && !done(); attempt++ {
		bus.drainUntil(20000, done)
		if !done() {
			for _, r := range rs {
				if err := r.HandleTimeout(); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if !done() {
		for _, r := range rs {
			t.Logf("status %+v", r.Status())
		}
		t.Fatal("cold restart did not finalize locked value")
	}
	for _, bc := range s.nodes {
		if bc.Blocks[7].Hash != b.Hash {
			t.Fatal("finalized conflicting value")
		}
	}
	t.Log("FOUR_VALIDATOR_COLD_RESTART_LOCK_QC_FINALITY_OK")
}

// Keep encoding/json exercised on the persisted representation, not Go pointers.
func TestPersistentRecoveryEncodingRejectsTrailingData(t *testing.T) {
	b, _ := json.Marshal(consensusRecovery{Version: 1})
	if strictRecoveryJSON(append(b, []byte(" {}")...), new(consensusRecovery)) == nil {
		t.Fatal("accepted trailing JSON")
	}
}

func TestPersistentRecoveryProposalAndNilVote(t *testing.T) {
	for _, mode := range []string{"proposal", "nil-prevote"} {
		t.Run(mode, func(t *testing.T) {
			s, r, _ := recoveryFixture(t)
			if mode == "proposal" {
				r.Stop()
				proposer, _ := r.bc.CurrentValidatorSet(7).Proposer(7, 0)
				v := s.byID[strings.ToLower(proposer.ID)]
				var err error
				r, err = NewConsensusReactor(r.bc, v.id, v.consensus, nil)
				if err != nil {
					t.Fatal(err)
				}
				if err = r.Start(); err != nil {
					t.Fatal(err)
				}
			} else if err := r.HandleTimeout(); err != nil {
				t.Fatal(err)
			}
			next := reopenRecovery(t, r)
			if err := next.Start(); err != nil {
				t.Fatal(err)
			}
			defer next.Stop()
			if next.round < 1 || next.lockedRound != -1 {
				t.Fatal("incorrect proposal/nil-vote recovery")
			}
		})
	}
}

func TestPersistentRecoveryRoundLimitFailsClosed(t *testing.T) {
	_, r, _ := recoveryFixture(t)
	if err := r.persistRecoveryLocked(r.cfg.MaxRound); err != nil {
		t.Fatal(err)
	}
	r.Stop()
	if err := r.bc.CheckValidatorRestart(r.validatorID); err == nil {
		t.Fatal("exhausted round budget accepted")
	}
}

func TestPersistentRecoveryRejectsConflictingValueAfterRestart(t *testing.T) {
	s, r, p := recoveryFixture(t)
	applyRecoveryQC(t, s, r, p)
	next := reopenRecovery(t, r)
	if err := next.Start(); err != nil {
		t.Fatal(err)
	}
	defer next.Stop()
	proposer, _ := next.bc.CurrentValidatorSet(7).Proposer(7, next.round)
	v := s.byID[strings.ToLower(proposer.ID)]
	other, err := next.bc.BuildConsensusProposal(v.id, next.round)
	if err != nil {
		t.Fatal(err)
	}
	if other.Hash == p.Block.Hash {
		t.Fatal("fixture needs a conflicting value")
	}
	h, err := consensus.NewSignedProposalHeader(s.cfg.ChainID, 7, next.round, other.Hash, other.ValidatorSetRoot, v.id, -1, v.consensus)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(next.bc.storage.dir, "consensus-bft-signatures.jsonl")
	before, _ := os.ReadFile(path)
	if err = next.HandleProposal(ConsensusProposal{h, other}); err == nil {
		t.Fatal("lost lock across restart")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("signed conflicting recovery proposal")
	}
}
