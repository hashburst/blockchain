package consensus

import (
	"strings"
	"testing"

	"hashburst/wallet"
)

const r6MembershipChainID uint64 = 1337

func r6MembershipFixture(t *testing.T) (*Registry, []validatorFixture, ValidatorSet) {
	t.Helper()

	r := NewRegistry(testConsensusConfig())
	fixtures := make([]validatorFixture, 4)
	for i := range fixtures {
		fixtures[i] = registerFixture(t, r, i, 2000)
	}

	set := r.ActiveSet(10)
	if len(set.Validators) != 4 {
		t.Fatalf("active validators=%d want=4", len(set.Validators))
	}
	return r, fixtures, set
}

func r6MembershipProposal(
	set ValidatorSet,
	version uint64,
	previousVersion uint64,
	validatorID string,
	action string,
	consensusPubKey string,
) MembershipProposal {
	return MembershipProposal{
		ChainID:         r6MembershipChainID,
		Version:         version,
		PreviousVersion: previousVersion,
		PreviousSetRoot: set.Root(),
		ValidatorID:     validatorID,
		Action:          action,
		ConsensusPubKey: consensusPubKey,
	}
}

func r6MembershipVotes(
	t *testing.T,
	p MembershipProposal,
	fixtures []validatorFixture,
	indexes ...int,
) []MembershipVote {
	t.Helper()

	out := make([]MembershipVote, 0, len(indexes))
	for _, i := range indexes {
		vote, err := NewMembershipVote(
			p,
			fixtures[i].validator.ID,
			fixtures[i].consensus,
		)
		if err != nil {
			t.Fatalf("membership vote %d: %v", i, err)
		}
		out = append(out, vote)
	}
	return out
}

func r6MembershipCertificate(
	t *testing.T,
	p MembershipProposal,
	set ValidatorSet,
	fixtures []validatorFixture,
	indexes ...int,
) MembershipCertificate {
	t.Helper()

	cert, err := BuildMembershipCertificate(
		p,
		set,
		r6MembershipVotes(t, p, fixtures, indexes...),
	)
	if err != nil {
		t.Fatalf("build membership certificate: %v", err)
	}
	return cert
}

func TestR6MembershipTwoOfFourRejected(t *testing.T) {
	_, fixtures, set := r6MembershipFixture(t)

	p := r6MembershipProposal(
		set, 1, 0,
		fixtures[3].validator.ID,
		MembershipActionSuspend,
		"",
	)

	_, err := BuildMembershipCertificate(
		p,
		set,
		r6MembershipVotes(t, p, fixtures, 0, 1),
	)
	if err == nil {
		t.Fatal("2-of-4 membership certificate unexpectedly accepted")
	}
	if !strings.Contains(err.Error(), "insufficient membership quorum") {
		t.Fatalf("unexpected 2-of-4 error: %v", err)
	}
}

func TestR6MembershipThreeOfFourAccepted(t *testing.T) {
	r, fixtures, set := r6MembershipFixture(t)
	state := NewMembershipState(0)

	p := r6MembershipProposal(
		set, 1, 0,
		fixtures[3].validator.ID,
		MembershipActionSuspend,
		"",
	)
	cert := r6MembershipCertificate(t, p, set, fixtures, 0, 1, 2)

	updated, err := state.Apply(r, set, cert)
	if err != nil {
		t.Fatalf("3-of-4 membership apply: %v", err)
	}
	if updated.Status != ValidatorSuspended {
		t.Fatalf("status=%s want=%s", updated.Status, ValidatorSuspended)
	}
	if state.Version() != 1 {
		t.Fatalf("membership version=%d want=1", state.Version())
	}
	if got := len(r.ActiveSet(set.Height).Validators); got != 3 {
		t.Fatalf("active validators=%d want=3", got)
	}
}

func TestR6MembershipOutsiderRejected(t *testing.T) {
	_, fixtures, set := r6MembershipFixture(t)

	p := r6MembershipProposal(
		set, 1, 0,
		fixtures[3].validator.ID,
		MembershipActionSuspend,
		"",
	)

	outsider, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	outsiderVote, err := NewMembershipVote(
		p,
		"0x"+strings.Repeat("aa", 32),
		outsider,
	)
	if err != nil {
		t.Fatal(err)
	}

	votes := r6MembershipVotes(t, p, fixtures, 0, 1)
	votes = append(votes, outsiderVote)

	_, err = BuildMembershipCertificate(p, set, votes)
	if err == nil {
		t.Fatal("membership certificate containing outsider unexpectedly accepted")
	}
	if !strings.Contains(err.Error(), "non-active validator") {
		t.Fatalf("unexpected outsider error: %v", err)
	}
}

func TestR6MembershipPreviousSetRootBinding(t *testing.T) {
	_, fixtures, set := r6MembershipFixture(t)

	p := r6MembershipProposal(
		set, 1, 0,
		fixtures[3].validator.ID,
		MembershipActionSuspend,
		"",
	)
	p.PreviousSetRoot = "0x" + strings.Repeat("11", 32)

	_, err := BuildMembershipCertificate(
		p,
		set,
		r6MembershipVotes(t, p, fixtures, 0, 1, 2),
	)
	if err == nil {
		t.Fatal("certificate with foreign previous-set root unexpectedly accepted")
	}
	if !strings.Contains(err.Error(), "previous validator set root mismatch") {
		t.Fatalf("unexpected previous-set-root error: %v", err)
	}
}

func TestR6MembershipApplyAndReplayRejected(t *testing.T) {
	r, fixtures, set := r6MembershipFixture(t)
	state := NewMembershipState(0)

	p := r6MembershipProposal(
		set, 1, 0,
		fixtures[3].validator.ID,
		MembershipActionRotateKey,
		"",
	)

	replacement, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	p.ConsensusPubKey = replacement.PublicKeyHexCompressed()

	cert := r6MembershipCertificate(t, p, set, fixtures, 0, 1, 2)

	if _, err := state.Apply(r, set, cert); err != nil {
		t.Fatalf("initial membership apply: %v", err)
	}

	proposalHash, err := MembershipProposalHash(p)
	if err != nil {
		t.Fatal(err)
	}

	snapshot := state.Snapshot()
	if snapshot.Version != 1 {
		t.Fatalf("snapshot version=%d want=1", snapshot.Version)
	}
	if len(snapshot.Consumed) != 1 ||
		!strings.EqualFold(snapshot.Consumed[0], proposalHash) {
		t.Fatalf("consumed proposal snapshot=%v want=%s", snapshot.Consumed, proposalHash)
	}

	restored, err := RestoreMembershipState(snapshot)
	if err != nil {
		t.Fatalf("restore membership state: %v", err)
	}

	// A restored state must not accept the already-consumed transition.
	// The exact rejection may be version-first or replay-first; both are
	// fail-closed for an already consumed proposal.
	if _, err := restored.Apply(r, set, cert); err == nil {
		t.Fatal("replayed membership certificate unexpectedly accepted")
	}
}

func TestR6MembershipOldSetRejectedAfterMembershipChange(t *testing.T) {
	r, fixtures, oldSet := r6MembershipFixture(t)
	state := NewMembershipState(0)

	first := r6MembershipProposal(
		oldSet, 1, 0,
		fixtures[3].validator.ID,
		MembershipActionSuspend,
		"",
	)
	firstCert := r6MembershipCertificate(t, first, oldSet, fixtures, 0, 1, 2)

	if _, err := state.Apply(r, oldSet, firstCert); err != nil {
		t.Fatalf("first membership transition: %v", err)
	}

	if got := len(r.ActiveSet(oldSet.Height).Validators); got != 3 {
		t.Fatalf("post-transition active validators=%d want=3", got)
	}

	second := r6MembershipProposal(
		oldSet, 2, 1,
		fixtures[2].validator.ID,
		MembershipActionSuspend,
		"",
	)

	oldSetCert := r6MembershipCertificate(t, second, oldSet, fixtures, 0, 1, 2)

	_, err := state.Apply(r, oldSet, oldSetCert)
	if err == nil {
		t.Fatal("certificate authorized by obsolete validator set unexpectedly accepted")
	}
	if !strings.Contains(err.Error(), "previous validator set is not current") {
		t.Fatalf("unexpected obsolete-set error: %v", err)
	}
}

func TestR6MembershipKeyRotationAuthorizedByPreviousSet(t *testing.T) {
	r, fixtures, set := r6MembershipFixture(t)
	state := NewMembershipState(0)

	replacement, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	target := fixtures[3].validator
	p := r6MembershipProposal(
		set, 1, 0,
		target.ID,
		MembershipActionRotateKey,
		replacement.PublicKeyHexCompressed(),
	)
	cert := r6MembershipCertificate(t, p, set, fixtures, 0, 1, 2)

	updated, err := state.Apply(r, set, cert)
	if err != nil {
		t.Fatalf("authorized key rotation: %v", err)
	}

	if !strings.EqualFold(updated.ID, target.ID) {
		t.Fatalf("validator identity changed: got=%s want=%s", updated.ID, target.ID)
	}
	if !wallet.AddressEqual(updated.ConsensusAddress, replacement.Address()) {
		t.Fatalf(
			"rotated consensus address=%s want=%s",
			updated.ConsensusAddress,
			replacement.Address(),
		)
	}

	after, ok := r.Get(target.ID)
	if !ok {
		t.Fatal("rotated validator missing from registry")
	}
	if !wallet.AddressEqual(after.ConsensusAddress, replacement.Address()) {
		t.Fatal("registry did not retain rotated consensus key")
	}
}

func TestR6MembershipRestartContinuity(t *testing.T) {
	r, fixtures, set := r6MembershipFixture(t)
	state := NewMembershipState(0)

	p := r6MembershipProposal(
		set, 1, 0,
		fixtures[3].validator.ID,
		MembershipActionSuspend,
		"",
	)
	cert := r6MembershipCertificate(t, p, set, fixtures, 0, 1, 2)

	if _, err := state.Apply(r, set, cert); err != nil {
		t.Fatalf("membership transition: %v", err)
	}

	registrySnapshot := r.Snapshot()
	registryRoot := r.Root()
	membershipSnapshot := state.Snapshot()

	restoredRegistry, err := RestoreCheckpoint(
		r.Config(),
		registrySnapshot,
		registryRoot,
	)
	if err != nil {
		t.Fatalf("restore validator registry: %v", err)
	}

	restoredMembership, err := RestoreMembershipState(membershipSnapshot)
	if err != nil {
		t.Fatalf("restore membership state: %v", err)
	}

	if restoredRegistry.Root() != registryRoot {
		t.Fatalf(
			"restored registry root=%s want=%s",
			restoredRegistry.Root(),
			registryRoot,
		)
	}
	if restoredMembership.Version() != state.Version() {
		t.Fatalf(
			"restored membership version=%d want=%d",
			restoredMembership.Version(),
			state.Version(),
		)
	}

	restoredValidator, ok := restoredRegistry.Get(fixtures[3].validator.ID)
	if !ok {
		t.Fatal("suspended validator missing after restore")
	}
	if restoredValidator.Status != ValidatorSuspended {
		t.Fatalf(
			"restored status=%s want=%s",
			restoredValidator.Status,
			ValidatorSuspended,
		)
	}

	if len(restoredMembership.Snapshot().Consumed) != 1 {
		t.Fatal("consumed membership proposal not preserved across restore")
	}
}
