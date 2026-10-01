package blockchain

import (
 "errors"
 "strings"
 "testing"
 "hashburst/consensus"
)

func TestLockConflictBufferedProposalTimeoutPreservesLock(t *testing.T) {
 s, r, locked := recoveryFixture(t)
 defer r.Stop()
 set := r.bc.CurrentValidatorSet(7)
 var buffered ConsensusProposal
 for round := uint64(1); round < 5; round++ {
  proposer, _ := set.Proposer(7, round)
  v := s.byID[strings.ToLower(proposer.ID)]
  b, err := r.bc.BuildConsensusProposal(v.id, round)
  if err != nil { t.Fatal(err) }
  if b.Hash == locked.Block.Hash { continue }
  h, err := consensus.NewSignedProposalHeader(s.cfg.ChainID, 7, round, b.Hash, b.ValidatorSetRoot, v.id, b.ValidRound, v.consensus)
  if err != nil { t.Fatal(err) }
  buffered = ConsensusProposal{h,b}
  break
 }
 if buffered.Block == nil { t.Fatal("fixture needs conflicting future proposal") }
 if err := r.HandleProposal(buffered); err != nil { t.Fatal(err) }
 applyRecoveryQC(t,s,r,locked)
 // Reproduce timeout entering a previously accepted future proposal after
 // a lock was established at the current view.
 r.round = buffered.Header.Round-1
 r.step = consensus.StepPrecommit
 if err := r.HandleTimeout(); err != nil { t.Fatalf("lock refusal killed timeout: %v",err) }
 if !r.Running() || r.lockedHash != locked.Block.Hash || r.lockedRound != 0 || r.step != consensus.StepProposal || r.deadline.IsZero() { t.Fatal("lost lock or pacemaker") }
 if err := r.HandleProposal(buffered); !errors.Is(err,ErrProposalLockConflict) { t.Fatalf("network conflict not rejected: %v",err) }
 if err := r.HandleTimeout(); err != nil { t.Fatal(err) }
 vote, ok := r.seenPrevotes[r.round][strings.ToLower(r.validatorID)]
 if !ok || vote.BlockHash != consensus.NilBlockHash { t.Fatal("expected nil prevote, never conflicting value") }
 next := reopenRecovery(t,r)
 defer next.Stop()
 if err := next.Start(); err != nil { t.Fatal(err) }
 if next.lockedHash != locked.Block.Hash || next.lockedRound != 0 { t.Fatal("restart lost lock") }
}

func TestLockConflictMissingValidBlockReproposesRetainedLock(t *testing.T) {
 s,r,p := recoveryFixture(t)
 defer r.Stop()
 applyRecoveryQC(t,s,r,p)
 // A QC without its block must not trigger a new conflicting proposal.
 r.validBlock = nil
 r.validQC = nil
 set := r.bc.CurrentValidatorSet(7)
 var round uint64
 for round=1; round<8; round++ {
  v,_ := set.Proposer(7,round)
  if strings.EqualFold(v.ID,r.validatorID) { break }
 }
 if round==8 { t.Fatal("no scheduled local proposer") }
 r.round=round-1
 r.step=consensus.StepPrecommit
 if err:=r.HandleTimeout();err!=nil { t.Fatal(err) }
 proposed,ok:=r.proposals[round]
 if !ok || proposed.Block.Hash!=p.Block.Hash || proposed.Block.ValidPrevoteCertificate==nil { t.Fatal("did not repropose certified lock") }
}

func TestLockConflictPersistenceErrorStillFatal(t *testing.T) {
 _,r,_:=recoveryFixture(t)
 defer r.Stop()
 failure:=errors.New("disk write failed")
 r.recoveryErr=failure
 if err:=r.HandleTimeout();!errors.Is(err,failure) { t.Fatalf("persistence error swallowed: %v",err) }
}
