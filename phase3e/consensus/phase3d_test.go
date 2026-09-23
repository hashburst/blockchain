package consensus

import (
	"strings"
	"testing"
)

func TestPhase3DPrevoteQCAndSafeLockRule(t *testing.T) {
	r := NewRegistry(testConsensusConfig())
	fixtures := make([]validatorFixture, 4)
	for i := range fixtures {
		fixtures[i] = registerFixture(t, r, i, 2000)
	}
	set := r.ActiveSet(42)
	root := set.Root()
	blockA := "0x" + strings.Repeat("a1", 32)
	blockB := "0x" + strings.Repeat("b2", 32)

	votesA := make([]Prevote, 0, 3)
	for i := 0; i < 3; i++ {
		v, err := NewSignedPrevote(1337, 42, 0, blockA, root, fixtures[i].validator.ID, fixtures[i].consensus)
		if err != nil {
			t.Fatal(err)
		}
		votesA = append(votesA, v)
	}
	qcA, err := BuildPrevoteCertificate(1337, 42, 0, blockA, root, set, votesA)
	if err != nil {
		t.Fatalf("3-of-4 prevote QC rejected: %v", err)
	}
	if qcA.SignedPower != 3 || qcA.TotalPower != 4 {
		t.Fatalf("unexpected prevote power %d/%d", qcA.SignedPower, qcA.TotalPower)
	}
	if err := SafeProposal(0, blockA, blockB, -1, nil, set); err == nil {
		t.Fatal("conflicting proposal without proof unexpectedly passed local lock")
	}

	votesB := make([]Prevote, 0, 3)
	for i := 1; i < 4; i++ {
		v, err := NewSignedPrevote(1337, 42, 1, blockB, root, fixtures[i].validator.ID, fixtures[i].consensus)
		if err != nil {
			t.Fatal(err)
		}
		votesB = append(votesB, v)
	}
	qcB, err := BuildPrevoteCertificate(1337, 42, 1, blockB, root, set, votesB)
	if err != nil {
		t.Fatal(err)
	}
	if err := SafeProposal(0, blockA, blockB, 1, &qcB, set); err != nil {
		t.Fatalf("newer +2/3 proof did not safely unlock: %v", err)
	}
}

func TestPhase3DRoundChangeThresholdAndProposalSignature(t *testing.T) {
	r := NewRegistry(testConsensusConfig())
	fixtures := make([]validatorFixture, 4)
	for i := range fixtures {
		fixtures[i] = registerFixture(t, r, i, 1000)
	}
	set := r.ActiveSet(51)
	if FaultThreshold(set.TotalPower()) != 1 || CatchupThreshold(set.TotalPower()) != 2 {
		t.Fatalf("unexpected thresholds f=%d catchup=%d", FaultThreshold(set.TotalPower()), CatchupThreshold(set.TotalPower()))
	}
	changes := make([]RoundChange, 0, 2)
	for i := 0; i < 2; i++ {
		rc, err := NewSignedRoundChange(1337, 51, 1, set.Root(), fixtures[i].validator.ID, fixtures[i].consensus)
		if err != nil {
			t.Fatal(err)
		}
		changes = append(changes, rc)
	}
	cert := RoundChangeCertificate{ChainID: 1337, Height: 51, NextRound: 1, ValidatorSetRoot: set.Root(), Changes: changes}
	if err := cert.Verify(set, CatchupThreshold(set.TotalPower())); err != nil {
		t.Fatalf("f+1 round-change catchup proof rejected: %v", err)
	}
	if cert.SignedPower != 2 {
		t.Fatalf("round-change catchup power=%d want 2", cert.SignedPower)
	}
	if _, err := BuildRoundChangeCertificate(1337, 51, 1, set.Root(), set, changes); err == nil {
		t.Fatal("2-of-4 unexpectedly formed quorum round-change certificate")
	}
	proposer, _ := set.Proposer(51, 0)
	var signer validatorFixture
	for _, f := range fixtures {
		if strings.EqualFold(f.validator.ID, proposer.ID) {
			signer = f
			break
		}
	}
	h, err := NewSignedProposalHeader(1337, 51, 0, "0x"+strings.Repeat("c3", 32), set.Root(), proposer.ID, -1, signer.consensus)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyProposalHeader(h, proposer); err != nil {
		t.Fatalf("signed proposal header rejected: %v", err)
	}
}

func TestPhase3DBFTJournalAllowsViewChangeButRejectsSameRoundEquivocation(t *testing.T) {
	j, err := OpenBFTSignJournal(t.TempDir() + "/bft.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	root := "0x" + strings.Repeat("11", 32)
	validator := "0x" + strings.Repeat("22", 32)
	a := BFTSignRecord{Step: StepPrevote, ChainID: 1337, Height: 9, Round: 0, ValidatorID: validator, BlockHash: "0x" + strings.Repeat("aa", 32), ValidatorSetRoot: root, Signature: "sig-a"}
	if err := j.Record(a); err != nil {
		t.Fatal(err)
	}
	later := a
	later.Round = 1
	later.BlockHash = "0x" + strings.Repeat("bb", 32)
	later.Signature = "sig-b"
	if err := j.Record(later); err != nil {
		t.Fatalf("legitimate later-round vote rejected: %v", err)
	}
	conflict := a
	conflict.BlockHash = "0x" + strings.Repeat("cc", 32)
	conflict.Signature = "sig-c"
	if err := j.Record(conflict); err == nil {
		t.Fatal("same-round conflicting prevote unexpectedly recorded")
	}
}

func TestPhase3DSameRoundEquivocationSlashes(t *testing.T) {
	r := NewRegistry(testConsensusConfig())
	f := registerFixture(t, r, 0, 2000)
	set := r.ActiveSet(61)
	root := set.Root()
	a, err := NewSignedPrevote(1337, 61, 2, "0x"+strings.Repeat("31", 32), root, f.validator.ID, f.consensus)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSignedPrevote(1337, 61, 2, "0x"+strings.Repeat("32", 32), root, f.validator.ID, f.consensus)
	if err != nil {
		t.Fatal(err)
	}
	ev := BFTDoubleSignEvidence{Step: StepPrevote, Prevote: &PrevoteEquivocationEvidence{VoteA: a, VoteB: b}}
	v, slashed, err := r.ApplyBFTDoubleSignEvidence(ev, 62)
	if err != nil {
		t.Fatal(err)
	}
	if slashed != 100 || v.Status != ValidatorJailed || v.BondUnits != 1900 {
		t.Fatalf("unexpected phase3d slash result: %+v slash=%d", v, slashed)
	}
}
