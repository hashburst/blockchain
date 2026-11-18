package blockchain

import (
	"strings"
	"testing"

	execution "hashburst/evm-execution"
)

// This test isolates the production replay commitment layer.
//
// Persistence/codec survival is tested separately by the HBX2 and on-disk
// round-trip tests. The runtime open-path also independently rejects a
// persisted MembershipStateRoot tamper through the canonical block hash.
//
// Here we prove the second fail-closed layer: deterministic replay itself
// recomputes POST-execution membership state and rejects a false committed
// MembershipStateRoot even when canonical-hash validation is deliberately
// outside the scope of this unit boundary.
func TestR6MembershipStateRootTamperRejectedByReplayCommitment(t *testing.T) {
	cfg := phase3CTestConfig()
	cfg.ChainID = execution.TestnetID
	cfg.EVM = &EVMConfig{
		ActivationHeight: 7,
		GasLimit:         1000000,
		BaseFeeWei:       1,
	}

	s := setupPhase3DChainsWithConfig(t, cfg)
	n := s.nodes[0]

	for _, node := range s.nodes {
		node.SetMempool(NewMempool())
	}

	finalizeEVMFixture(t, s)

	if len(n.Blocks) < 2 {
		t.Fatalf("fixture chain too short: %d", len(n.Blocks))
	}

	// First prove that the untouched canonical chain is replayable.
	if _, _, _, _, _, _, err := n.computeProjections(n.Blocks); err != nil {
		t.Fatalf("untouched fixture rejected by replay: %v", err)
	}

	targetIndex := -1

	for i, b := range n.Blocks {
		if b != nil &&
			b.EffectiveVersion() >= BlockVersionV2 &&
			b.MembershipStateRoot != "" {
			targetIndex = i
			break
		}
	}

	if targetIndex < 0 {
		t.Fatal("no V2 block with MembershipStateRoot found")
	}

	original := n.Blocks[targetIndex]

	validRoot := original.MembershipStateRoot

	if validRoot == "" {
		t.Fatal("target MembershipStateRoot is empty")
	}

	// Clone only the target block. Do not mutate the fixture chain.
	tampered := *original

	const tamperA = "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	const tamperB = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

	tampered.MembershipStateRoot = tamperA

	if strings.EqualFold(tampered.MembershipStateRoot, validRoot) {
		tampered.MembershipStateRoot = tamperB
	}

	if strings.EqualFold(tampered.MembershipStateRoot, validRoot) {
		t.Fatal("unable to construct distinct membership-root tamper")
	}

	// Construct a replay prefix ending exactly at the tampered block.
	// This avoids any descendant PrevHash dependency and exercises
	// computeProjections' deterministic state-commitment verification.
	prefix := make([]*Block, targetIndex+1)

	copy(prefix, n.Blocks[:targetIndex])
	prefix[targetIndex] = &tampered

	_, _, _, _, _, _, err :=
		n.computeProjections(prefix)

	if err == nil {
		t.Fatal(
			"tampered MembershipStateRoot unexpectedly accepted by replay",
		)
	}

	want := "V2 state/receipt/validator commitment mismatch"

	if !strings.Contains(err.Error(), want) {
		t.Fatalf(
			"wrong replay rejection for MembershipStateRoot tamper: %v",
			err,
		)
	}

	// Prove the original fixture itself was never changed.
	if n.Blocks[targetIndex].MembershipStateRoot != validRoot {
		t.Fatal("canonical fixture was mutated")
	}

	t.Logf(
		"R3F4D_REPLAY_MEMBERSHIP_COMMITMENT_REJECTION_OK block=%d valid=%s tampered=%s err=%v",
		original.Index,
		validRoot,
		tampered.MembershipStateRoot,
		err,
	)
}
