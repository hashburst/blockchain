package blockchain

import (
	"strings"
	"testing"
)

func TestHVMNetworkDefaultLegacyPoWDifficultyUnchanged(t *testing.T) {
	cfg := DefaultProtocolV2Config()
	if cfg.LegacyDifficulty() != Difficulty {
		t.Fatalf("default legacy difficulty=%d want %d", cfg.LegacyDifficulty(), Difficulty)
	}
	var zeroField ProtocolV2Config
	if zeroField.LegacyDifficulty() != Difficulty {
		t.Fatalf("zero-value legacy difficulty=%d want compatibility default %d", zeroField.LegacyDifficulty(), Difficulty)
	}
}

func TestHVMNetworkConsensusProposalUsesCanonicalZeroPoW(t *testing.T) {
	s := setupPhase3DChains(t)
	set := s.nodes[0].CurrentValidatorSet(7)
	proposer, ok := set.Proposer(7, 0)
	if !ok {
		t.Fatal("missing proposer")
	}
	proposal, err := s.nodes[0].BuildConsensusProposal(proposer.ID, 0)
	if err != nil {
		t.Fatalf("build consensus proposal: %v", err)
	}
	if proposal.ProofOfWork != 0 {
		t.Fatalf("consensus proposal PoW=%d want canonical zero", proposal.ProofOfWork)
	}
	if proposal.Hash != proposal.GenerateHash() {
		t.Fatal("consensus proposal hash is not deterministic with canonical zero PoW")
	}

	mutated := cloneBlockForConsensus(proposal)
	mutated.ProofOfWork = 1
	mutated.Hash = mutated.GenerateHash()
	if err := s.nodes[0].ValidateConsensusProposalForVote(mutated); err == nil || !strings.Contains(err.Error(), "ProofOfWork=0") {
		t.Fatalf("non-zero consensus PoW was not rejected canonically: %v", err)
	}
}
