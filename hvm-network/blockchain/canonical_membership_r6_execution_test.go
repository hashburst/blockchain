package blockchain

import (
	"fmt"
	"strings"
	"testing"

	"hashburst/consensus"
	"hashburst/wallet"
)

type r6CanonicalValidatorFixture struct {
	operator  *wallet.Wallet
	reward    *wallet.Wallet
	consensus *wallet.Wallet
	validator consensus.Validator
}

type r6CanonicalMembershipFixture struct {
	cfg        ProtocolV2Config
	registry   *consensus.Registry
	membership *consensus.MembershipState
	set        consensus.ValidatorSet
	vals       []r6CanonicalValidatorFixture
}

func newR6CanonicalValidatorFixture(
	t *testing.T,
	r *consensus.Registry,
	index int,
	bond int64,
) r6CanonicalValidatorFixture {
	t.Helper()

	operator, err := wallet.NewWallet()
	if err != nil {
		t.Fatalf("operator wallet %d: %v", index, err)
	}

	reward, err := wallet.NewWallet()
	if err != nil {
		t.Fatalf("reward wallet %d: %v", index, err)
	}

	signer, err := wallet.NewWallet()
	if err != nil {
		t.Fatalf("consensus wallet %d: %v", index, err)
	}

	v, err := r.Register(
		operator.Address(),
		consensus.RegisterRequest{
			NodeID:          fmt.Sprintf("node-%c", rune('a'+index)),
			PeerID:          fmt.Sprintf("peer-%c", rune('a'+index)),
			ConsensusPubKey: signer.PublicKeyHexCompressed(),
			RewardAddress:   reward.Address(),
			BondUnits:       bond,
		},
		1,
	)
	if err != nil {
		t.Fatalf("register fixture %d: %v", index, err)
	}

	return r6CanonicalValidatorFixture{
		operator:  operator,
		reward:    reward,
		consensus: signer,
		validator: v,
	}
}

func newR6CanonicalMembershipFixture(
	t *testing.T,
) *r6CanonicalMembershipFixture {
	t.Helper()

	cfg := phase3CTestConfig()
	registry := consensus.NewRegistry(cfg.Validator)

	vals := make([]r6CanonicalValidatorFixture, 4)
	for i := range vals {
		vals[i] = newR6CanonicalValidatorFixture(
			t,
			registry,
			i,
			cfg.Validator.MinBondUnits,
		)
	}

	registry.AdvanceHeight(10)
	set := registry.ActiveSet(10)

	if len(set.Validators) != 4 {
		t.Fatalf(
			"active validators=%d want=4",
			len(set.Validators),
		)
	}

	if set.TotalPower() != 4 {
		t.Fatalf(
			"active power=%d want=4",
			set.TotalPower(),
		)
	}

	if got := consensus.QuorumThreshold(set.TotalPower()); got != 3 {
		t.Fatalf(
			"quorum threshold=%d want=3",
			got,
		)
	}

	return &r6CanonicalMembershipFixture{
		cfg:        cfg,
		registry:   registry,
		membership: consensus.NewMembershipState(0),
		set:        set,
		vals:       vals,
	}
}

func (f *r6CanonicalMembershipFixture) proposal(
	target int,
	action string,
) consensus.MembershipProposal {
	return consensus.MembershipProposal{
		ChainID:         f.cfg.ChainID,
		Version:         1,
		PreviousVersion: 0,
		PreviousSetRoot: f.set.Root(),
		ValidatorID:     f.vals[target].validator.ID,
		Action:          action,
	}
}

func (f *r6CanonicalMembershipFixture) votes(
	t *testing.T,
	p consensus.MembershipProposal,
	count int,
) []consensus.MembershipVote {
	t.Helper()

	if count < 0 || count > len(f.vals) {
		t.Fatalf("invalid signer count %d", count)
	}

	votes := make([]consensus.MembershipVote, 0, count)

	for i := 0; i < count; i++ {
		vote, err := consensus.NewMembershipVote(
			p,
			f.vals[i].validator.ID,
			f.vals[i].consensus,
		)
		if err != nil {
			t.Fatalf("membership vote %d: %v", i, err)
		}

		votes = append(votes, vote)
	}

	return votes
}

func (f *r6CanonicalMembershipFixture) certificate(
	t *testing.T,
	p consensus.MembershipProposal,
	count int,
) consensus.MembershipCertificate {
	t.Helper()

	return consensus.MembershipCertificate{
		Proposal: p,
		Votes:    f.votes(t, p, count),
	}
}

func TestR6CanonicalMembershipTwoOfFourRejectedWithoutMutation(
	t *testing.T,
) {
	f := newR6CanonicalMembershipFixture(t)

	beforeRoot := f.registry.Root()
	beforeMembership := f.membership.Snapshot()

	p := f.proposal(3, consensus.MembershipActionSuspend)
	cert := f.certificate(t, p, 2)

	if _, err := f.membership.Apply(
		f.registry,
		f.set,
		cert,
	); err == nil {
		t.Fatal("2-of-4 membership certificate unexpectedly accepted")
	}

	if got := f.registry.Root(); !strings.EqualFold(got, beforeRoot) {
		t.Fatalf(
			"registry mutated after rejected certificate: got=%s want=%s",
			got,
			beforeRoot,
		)
	}

	afterMembership := f.membership.Snapshot()

	if afterMembership.Version != beforeMembership.Version {
		t.Fatalf(
			"membership version mutated after rejection: got=%d want=%d",
			afterMembership.Version,
			beforeMembership.Version,
		)
	}

	if len(afterMembership.Consumed) != len(beforeMembership.Consumed) {
		t.Fatalf(
			"consumed set mutated after rejection: got=%d want=%d",
			len(afterMembership.Consumed),
			len(beforeMembership.Consumed),
		)
	}
}

func TestR6CanonicalMembershipThreeOfFourApplied(
	t *testing.T,
) {
	f := newR6CanonicalMembershipFixture(t)

	p := f.proposal(3, consensus.MembershipActionSuspend)
	cert := f.certificate(t, p, 3)

	updated, err := f.membership.Apply(
		f.registry,
		f.set,
		cert,
	)
	if err != nil {
		t.Fatalf("3-of-4 membership certificate rejected: %v", err)
	}

	if updated.Status != consensus.ValidatorSuspended {
		t.Fatalf(
			"updated status=%s want=%s",
			updated.Status,
			consensus.ValidatorSuspended,
		)
	}

	if f.membership.Version() != 1 {
		t.Fatalf(
			"membership version=%d want=1",
			f.membership.Version(),
		)
	}

	snapshot := f.membership.Snapshot()
	if len(snapshot.Consumed) != 1 {
		t.Fatalf(
			"consumed proposals=%d want=1",
			len(snapshot.Consumed),
		)
	}

	got, ok := f.registry.Get(f.vals[3].validator.ID)
	if !ok {
		t.Fatal("suspended validator missing from registry")
	}

	if got.Status != consensus.ValidatorSuspended {
		t.Fatalf(
			"registry status=%s want=%s",
			got.Status,
			consensus.ValidatorSuspended,
		)
	}

	next := f.registry.ActiveSet(11)
	if len(next.Validators) != 3 {
		t.Fatalf(
			"post-transition active validators=%d want=3",
			len(next.Validators),
		)
	}
}

func TestR6CanonicalMembershipFailedTxAtomic(
	t *testing.T,
) {
	f := newR6CanonicalMembershipFixture(t)

	beforeRoot := f.registry.Root()
	beforeMembership := f.membership.Snapshot()

	p := f.proposal(3, consensus.MembershipActionSuspend)

	// The proposal is signed correctly but is bound to a foreign
	// previous-set root. Apply must fail without mutating either
	// registry or membership anti-replay state.
	p.PreviousSetRoot = "0x" + strings.Repeat("00", 32)

	cert := f.certificate(t, p, 3)

	if _, err := f.membership.Apply(
		f.registry,
		f.set,
		cert,
	); err == nil {
		t.Fatal("wrong previous-set root unexpectedly accepted")
	}

	if got := f.registry.Root(); !strings.EqualFold(got, beforeRoot) {
		t.Fatalf(
			"registry changed after failed transition: got=%s want=%s",
			got,
			beforeRoot,
		)
	}

	afterMembership := f.membership.Snapshot()

	if afterMembership.Version != beforeMembership.Version {
		t.Fatalf(
			"membership version changed after failure: got=%d want=%d",
			afterMembership.Version,
			beforeMembership.Version,
		)
	}

	if len(afterMembership.Consumed) != len(beforeMembership.Consumed) {
		t.Fatalf(
			"consumed set changed after failure: got=%d want=%d",
			len(afterMembership.Consumed),
			len(beforeMembership.Consumed),
		)
	}
}

func TestR6CanonicalMembershipCurrentBlockUsesPreviousSet(
	t *testing.T,
) {
	f := newR6CanonicalMembershipFixture(t)

	preSet := f.set
	preSetRoot := preSet.Root()

	if len(preSet.Validators) != 4 {
		t.Fatalf(
			"pre-transition set=%d want=4",
			len(preSet.Validators),
		)
	}

	p := f.proposal(3, consensus.MembershipActionSuspend)
	cert := f.certificate(t, p, 3)

	if _, err := f.membership.Apply(
		f.registry,
		preSet,
		cert,
	); err != nil {
		t.Fatalf("membership transition rejected: %v", err)
	}

	// The immutable value captured before transaction execution remains
	// the authority for block N.
	if got := preSet.Root(); !strings.EqualFold(got, preSetRoot) {
		t.Fatalf(
			"captured previous set root changed: got=%s want=%s",
			got,
			preSetRoot,
		)
	}

	if len(preSet.Validators) != 4 {
		t.Fatalf(
			"block-N previous set count=%d want=4",
			len(preSet.Validators),
		)
	}

	if preSet.TotalPower() != 4 {
		t.Fatalf(
			"block-N previous power=%d want=4",
			preSet.TotalPower(),
		)
	}
}

func TestR6CanonicalMembershipNextBlockUsesNewSet(
	t *testing.T,
) {
	f := newR6CanonicalMembershipFixture(t)

	oldSet := f.set

	p := f.proposal(3, consensus.MembershipActionSuspend)
	cert := f.certificate(t, p, 3)

	if _, err := f.membership.Apply(
		f.registry,
		oldSet,
		cert,
	); err != nil {
		t.Fatalf("membership transition rejected: %v", err)
	}

	nextSet := f.registry.ActiveSet(oldSet.Height + 1)

	if len(nextSet.Validators) != 3 {
		t.Fatalf(
			"next-block active validators=%d want=3",
			len(nextSet.Validators),
		)
	}

	if nextSet.TotalPower() != 3 {
		t.Fatalf(
			"next-block total power=%d want=3",
			nextSet.TotalPower(),
		)
	}

	if strings.EqualFold(nextSet.Root(), oldSet.Root()) {
		t.Fatal("next-block validator set root did not change")
	}

	if _, _, ok := nextSet.Find(f.vals[3].validator.ID); ok {
		t.Fatal("suspended validator remained in next active set")
	}

	if got := consensus.QuorumThreshold(nextSet.TotalPower()); got != 3 {
		t.Fatalf(
			"next-block quorum threshold=%d want=3",
			got,
		)
	}
}
