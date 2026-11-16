package consensus

import (
	"strings"
	"testing"

	"hashburst/wallet"
)

func TestR6SuspendRemovesValidatorFromActiveSet(t *testing.T) {
	r := NewRegistry(testConsensusConfig())
	f := registerFixture(t, r, 0, 2000)

	before := r.Root()

	v, err := r.applySuspend(f.validator.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != ValidatorSuspended {
		t.Fatalf("status=%s want SUSPENDED", v.Status)
	}
	if v.ID != f.validator.ID {
		t.Fatal("validator identity changed during suspension")
	}
	if r.Root() == before {
		t.Fatal("registry root unchanged after suspension")
	}
	if _, _, ok := r.ActiveSet(10).Find(v.ID); ok {
		t.Fatal("suspended validator remained active")
	}

	r.AdvanceHeight(1000000)
	got, _ := r.Get(v.ID)
	if got.Status != ValidatorSuspended {
		t.Fatalf("AdvanceHeight reactivated suspended validator: %s", got.Status)
	}
}

func TestR6RevokeIsTerminalForConsensusEligibility(t *testing.T) {
	r := NewRegistry(testConsensusConfig())
	f := registerFixture(t, r, 0, 2000)

	v, err := r.applyRevoke(f.validator.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != ValidatorRevoked {
		t.Fatalf("status=%s want REVOKED", v.Status)
	}
	if _, _, ok := r.ActiveSet(10).Find(v.ID); ok {
		t.Fatal("revoked validator remained active")
	}

	r.AdvanceHeight(1000000)
	got, _ := r.Get(v.ID)
	if got.Status != ValidatorRevoked {
		t.Fatalf("AdvanceHeight changed revoked validator: %s", got.Status)
	}

	if _, err := r.applyConsensusKeyRotation(v.ID, f.consensus.PublicKeyHexCompressed()); err == nil {
		t.Fatal("revoked validator unexpectedly rotated key")
	}
}

func TestR6ConsensusKeyRotationPreservesLogicalIdentity(t *testing.T) {
	r := NewRegistry(testConsensusConfig())
	f := registerFixture(t, r, 0, 2000)

	replacement, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	beforeID := f.validator.ID
	beforeEscrow := f.validator.EscrowAddress
	beforeRoot := r.Root()

	v, err := r.applyConsensusKeyRotation(beforeID, replacement.PublicKeyHexCompressed())
	if err != nil {
		t.Fatal(err)
	}

	if v.ID != beforeID {
		t.Fatalf("validator ID changed: %s -> %s", beforeID, v.ID)
	}
	if v.EscrowAddress != beforeEscrow {
		t.Fatalf("escrow changed: %s -> %s", beforeEscrow, v.EscrowAddress)
	}
	if !wallet.AddressEqual(v.ConsensusAddress, replacement.Address()) {
		t.Fatalf("consensus address=%s want=%s", v.ConsensusAddress, replacement.Address())
	}
	if r.Root() == beforeRoot {
		t.Fatal("registry root unchanged after key rotation")
	}
}

func TestR6OldConsensusKeyRejectedAfterRotation(t *testing.T) {
	r := NewRegistry(testConsensusConfig())
	f := registerFixture(t, r, 0, 2000)

	replacement, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	v, err := r.applyConsensusKeyRotation(f.validator.ID, replacement.PublicKeyHexCompressed())
	if err != nil {
		t.Fatal(err)
	}

	set := r.ActiveSet(42)
	root := set.Root()
	blockHash := "0x" + strings.Repeat("42", 32)

	oldVote, err := NewSignedVote(1337, 42, 0, blockHash, root, v.ID, f.consensus)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyVote(oldVote, v); err == nil {
		t.Fatal("superseded consensus key unexpectedly verified")
	}

	newVote, err := NewSignedVote(1337, 42, 0, blockHash, root, v.ID, replacement)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyVote(newVote, v); err != nil {
		t.Fatalf("replacement consensus key rejected: %v", err)
	}
}

func TestR6LifecycleCheckpointRestorePreservesStateAndKey(t *testing.T) {
	r := NewRegistry(testConsensusConfig())
	f0 := registerFixture(t, r, 0, 2000)
	f1 := registerFixture(t, r, 1, 2000)

	replacement, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	rotated, err := r.applyConsensusKeyRotation(f0.validator.ID, replacement.PublicKeyHexCompressed())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.applySuspend(f1.validator.ID); err != nil {
		t.Fatal(err)
	}

	root := r.Root()

	restored, err := RestoreCheckpoint(r.Config(), r.Snapshot(), root)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Root() != root {
		t.Fatal("restored registry root mismatch")
	}

	got0, ok := restored.Get(rotated.ID)
	if !ok {
		t.Fatal("rotated validator missing after restore")
	}
	if got0.ID != rotated.ID ||
		!strings.EqualFold(got0.ConsensusPubKey, rotated.ConsensusPubKey) ||
		!wallet.AddressEqual(got0.ConsensusAddress, rotated.ConsensusAddress) {
		t.Fatal("rotated key binding not preserved after restore")
	}

	got1, ok := restored.Get(f1.validator.ID)
	if !ok || got1.Status != ValidatorSuspended {
		t.Fatal("suspension not preserved after restore")
	}
}

func TestR6LifecycleRejectsDuplicateConsensusKey(t *testing.T) {
	r := NewRegistry(testConsensusConfig())
	f0 := registerFixture(t, r, 0, 2000)
	f1 := registerFixture(t, r, 1, 2000)

	if _, err := r.applyConsensusKeyRotation(
		f0.validator.ID,
		f1.consensus.PublicKeyHexCompressed(),
	); err == nil {
		t.Fatal("duplicate consensus key unexpectedly accepted")
	}
}

func TestR6OperationalTelemetryHasNoConsensusAuthority(t *testing.T) {
	// Consensus lifecycle primitives deliberately accept only validator identity
	// and protocol lifecycle/key material. Resource readiness, congestion,
	// DEGRADED routing state and mining telemetry are not inputs.
	r := NewRegistry(testConsensusConfig())
	f := registerFixture(t, r, 0, 2000)

	before := r.Root()
	r.AdvanceHeight(10)

	if r.Root() != before {
		t.Fatal("ordinary height advance unexpectedly changed validator state")
	}

	if _, _, ok := r.ActiveSet(10).Find(f.validator.ID); !ok {
		t.Fatal("healthy active validator unexpectedly lost eligibility")
	}
}
