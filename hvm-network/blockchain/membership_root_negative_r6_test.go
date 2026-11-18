package blockchain

import (
	"strings"
	"testing"
)

func TestR6MembershipStateRootDynamicTamperRejected(t *testing.T) {

	bc, _, admin := newPhase3BChain(t, 1)
	bc.state.balances[stateKey(admin.Address())] = AmountToUnits(100)
	prev := bc.Blocks[len(bc.Blocks)-1]
	b := NewBlockV2([]*Transaction{NewSystemReward(admin.Address(), bc.MiningReward)}, nil, prev.Hash, 1, poHWithTicks(prev.ProofOfTime, bc.v2Config.EffectivePoHTicks()), bc.v2Config.ChainID)
	prepared, err := bc.prepareV2Commitments(b)
	if err != nil {
		t.Fatal(err)
	}

	if prepared == nil {
		t.Fatal("prepareV2Commitments returned nil result")
	}

	if b.MembershipStateRoot == "" {
		t.Fatal("prepared V2 block has empty MembershipStateRoot")
	}

	validRoot := b.MembershipStateRoot

	// Baseline: the exact prepared block must validate.
	if _, err := bc.validateV2Commitments(b); err != nil {
		t.Fatalf(
			"prepared V2 block rejected before tamper: %v",
			err,
		)
	}

	// Tamper exactly the membership-state commitment.
	const tamperA = "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	const tamperB = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

	b.MembershipStateRoot = tamperA
	if b.MembershipStateRoot == validRoot {
		b.MembershipStateRoot = tamperB
	}

	if b.MembershipStateRoot == validRoot {
		t.Fatal("failed to create distinct MembershipStateRoot tamper")
	}

	_, err = bc.validateV2Commitments(b)
	if err == nil {
		t.Fatal(
			"tampered MembershipStateRoot unexpectedly accepted",
		)
	}

	if !strings.Contains(
		err.Error(),
		"membership state root mismatch",
	) {
		t.Fatalf(
			"wrong rejection for tampered MembershipStateRoot: %v",
			err,
		)
	}

	t.Logf(
		"R3F4C_DYNAMIC_REJECTION_OK valid=%s tampered=%s err=%v",
		validRoot,
		b.MembershipStateRoot,
		err,
	)

}
