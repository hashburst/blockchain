package consensus

import (
	"strings"
	"testing"

	"hashburst/wallet"
)

func testConsensusConfig() Config {
	return Config{MinBondUnits: 1000, ActivationDelay: 0, UnbondingBlocks: 3, JailBlocks: 2, DoubleVoteSlashBPS: 500}
}

type validatorFixture struct {
	operator  *wallet.Wallet
	reward    *wallet.Wallet
	consensus *wallet.Wallet
	validator Validator
}

func registerFixture(t *testing.T, r *Registry, index int, bond int64) validatorFixture {
	t.Helper()
	op, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	reward, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	v, err := r.Register(op.Address(), RegisterRequest{
		NodeID: "node-" + string(rune('a'+index)), PeerID: "peer-" + string(rune('a'+index)),
		ConsensusPubKey: signer.PublicKeyHexCompressed(), RewardAddress: reward.Address(), BondUnits: bond,
	}, 1)
	if err != nil {
		t.Fatalf("register fixture %d: %v", index, err)
	}
	return validatorFixture{op, reward, signer, v}
}

func TestPhase3CQuorumThreeOfFour(t *testing.T) {
	r := NewRegistry(testConsensusConfig())
	fixtures := make([]validatorFixture, 4)
	for i := range fixtures {
		fixtures[i] = registerFixture(t, r, i, 2000)
	}
	set := r.ActiveSet(10)
	if len(set.Validators) != 4 {
		t.Fatalf("active validators=%d want 4", len(set.Validators))
	}
	blockHash := "0x" + strings.Repeat("11", 32)
	setRoot := set.Root()
	votes := make([]Vote, 0, 4)
	for _, f := range fixtures {
		v, err := NewSignedVote(1337, set.Height, 0, blockHash, setRoot, f.validator.ID, f.consensus)
		if err != nil {
			t.Fatal(err)
		}
		votes = append(votes, v)
	}
	if _, err := BuildQuorumCertificate(1337, set.Height, 0, blockHash, setRoot, set, votes[:3]); err != nil {
		t.Fatalf("3-of-4 equal validators should form QC: %v", err)
	}
	if _, err := BuildQuorumCertificate(1337, set.Height, 0, blockHash, setRoot, set, votes[:2]); err == nil {
		t.Fatal("2-of-4 validators unexpectedly formed QC")
	}
}

func TestPhase3CQuorumEqualBondThresholdIsThreeOfFour(t *testing.T) {
	cfg := testConsensusConfig()
	r := NewRegistry(cfg)
	fixtures := make([]validatorFixture, 4)
	for i := range fixtures {
		fixtures[i] = registerFixture(t, r, i, cfg.MinBondUnits)
	}
	set := r.ActiveSet(10)
	if set.TotalPower() != 4 || QuorumThreshold(set.TotalPower()) != 3 {
		t.Fatalf("equal-bond quorum total=%d threshold=%d want total=4 threshold=3", set.TotalPower(), QuorumThreshold(set.TotalPower()))
	}
}

func TestPhase3CWrongConsensusSignerRejected(t *testing.T) {
	r := NewRegistry(testConsensusConfig())
	a := registerFixture(t, r, 0, 1000)
	b := registerFixture(t, r, 1, 1000)
	set := r.ActiveSet(9)
	vote, err := NewSignedVote(1337, set.Height, 0, "0x"+strings.Repeat("22", 32), set.Root(), a.validator.ID, b.consensus)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyVote(vote, a.validator); err == nil {
		t.Fatal("vote signed by another validator key unexpectedly verified")
	}
}

func TestPhase3CProposerScheduleDeterministic(t *testing.T) {
	r := NewRegistry(testConsensusConfig())
	for i := 0; i < 4; i++ {
		registerFixture(t, r, i, 1000)
	}
	set := r.ActiveSet(100)
	seen := map[string]bool{}
	for round := uint64(0); round < 4; round++ {
		p1, ok := set.Proposer(100, round)
		if !ok {
			t.Fatal("missing proposer")
		}
		p2, _ := set.Proposer(100, round)
		if p1.ID != p2.ID {
			t.Fatal("proposer schedule is not deterministic")
		}
		seen[p1.ID] = true
	}
	if len(seen) != 4 {
		t.Fatalf("round-robin covered %d validators want 4", len(seen))
	}
}

func TestPhase3CDoubleVoteSlashesAndJails(t *testing.T) {
	r := NewRegistry(testConsensusConfig())
	f := registerFixture(t, r, 0, 2000)
	set := r.ActiveSet(12)
	root := set.Root()
	v1, err := NewSignedVote(1337, 12, 0, "0x"+strings.Repeat("33", 32), root, f.validator.ID, f.consensus)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := NewSignedVote(1337, 12, 1, "0x"+strings.Repeat("44", 32), root, f.validator.ID, f.consensus)
	if err != nil {
		t.Fatal(err)
	}
	v, slashed, err := r.ApplyDoubleVoteEvidence(DoubleVoteEvidence{VoteA: v1, VoteB: v2}, 13)
	if err != nil {
		t.Fatal(err)
	}
	if slashed != 100 || v.BondUnits != 1900 || v.SlashedUnits != 100 {
		t.Fatalf("unexpected slash: v=%+v slashed=%d", v, slashed)
	}
	if v.Status != ValidatorJailed || v.JailedUntilHeight != 15 {
		t.Fatalf("validator not jailed correctly: %+v", v)
	}
	if _, _, err := r.ApplyDoubleVoteEvidence(DoubleVoteEvidence{VoteA: v1, VoteB: v2}, 14); err == nil {
		t.Fatal("duplicate evidence unexpectedly applied twice")
	}
	r.AdvanceHeight(15)
	got, _ := r.Get(v.ID)
	if got.Status != ValidatorActive {
		t.Fatalf("validator did not leave jail after period: %s", got.Status)
	}
}

func TestPhase3CUnbondAndWithdraw(t *testing.T) {
	r := NewRegistry(testConsensusConfig())
	f := registerFixture(t, r, 0, 2000)
	v, err := r.BeginUnbond(f.validator.ID, f.operator.Address(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != ValidatorUnbonding || v.UnbondingEndHeight != 23 {
		t.Fatalf("unexpected unbond state: %+v", v)
	}
	if _, _, err := r.Withdraw(v.ID, f.operator.Address(), 22); err == nil {
		t.Fatal("early withdraw unexpectedly succeeded")
	}
	v, amount, err := r.Withdraw(v.ID, f.operator.Address(), 23)
	if err != nil {
		t.Fatal(err)
	}
	if amount != 2000 || v.Status != ValidatorExited || v.BondUnits != 0 {
		t.Fatalf("withdraw mismatch: amount=%d validator=%+v", amount, v)
	}
}

func TestPhase3CConsensusKeySeparatedFromRewardAndOperator(t *testing.T) {
	r := NewRegistry(testConsensusConfig())
	op, _ := wallet.NewWallet()
	reward, _ := wallet.NewWallet()
	signer, _ := wallet.NewWallet()
	base := RegisterRequest{NodeID: "node-x", PeerID: "peer-x", ConsensusPubKey: signer.PublicKeyHexCompressed(), BondUnits: 1000}

	req := base
	req.RewardAddress = signer.Address()
	if _, err := r.Register(op.Address(), req, 1); err == nil {
		t.Fatal("consensus key == reward wallet unexpectedly accepted")
	}

	req = base
	req.RewardAddress = reward.Address()
	if _, err := r.Register(signer.Address(), req, 1); err == nil {
		t.Fatal("consensus key == operator wallet unexpectedly accepted")
	}
}
