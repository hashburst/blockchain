package blockchain

import (
	"encoding/json"
	"strings"
	"testing"

	"hashburst/consensus"
	"hashburst/protocolv2"
	"hashburst/wallet"
)

func r6CanonicalMembershipTx(
	t *testing.T,
	cfg ProtocolV2Config,
	sender *wallet.Wallet,
	cert consensus.MembershipCertificate,
	sequence uint64,
) *protocolv2.TransactionV2 {
	t.Helper()

	raw, err := json.Marshal(cert)
	if err != nil {
		t.Fatalf("marshal membership certificate: %v", err)
	}

	fee, err := cfg.FeePolicy.ComputeFee(computeValidatorMembership)
	if err != nil {
		t.Fatalf("membership fee: %v", err)
	}

	tx := protocolv2.NewTransactionV2(
		cfg.ChainID,
		protocolv2.TxValidatorMembership,
		sender.Address(),
		"",
		0,
		sequence,
		computeValidatorMembership,
		fee,
		raw,
	)

	if err := tx.Sign(sender); err != nil {
		t.Fatalf("sign membership transaction: %v", err)
	}

	return tx
}

func r6CanonicalMembershipExecutionBlock(
	t *testing.T,
	bc *Blockchain,
	tx *protocolv2.TransactionV2,
	height int,
) *Block {
	t.Helper()

	miner, err := wallet.NewWallet()
	if err != nil {
		t.Fatalf("miner wallet: %v", err)
	}

	prev := bc.Blocks[len(bc.Blocks)-1]

	return NewBlockV2(
		[]*Transaction{
			NewSystemReward(miner.Address(), bc.MiningReward),
		},
		[]*protocolv2.TransactionV2{tx},
		prev.Hash,
		height,
		poHWithTicks(
			prev.ProofOfTime,
			bc.v2Config.EffectivePoHTicks(),
		),
		bc.v2Config.ChainID,
	)
}

func TestR6CanonicalMembershipTxE2ETwoOfFourRevertsAtomically(
	t *testing.T,
) {
	f := newR6CanonicalMembershipFixture(t)

	bc := NewBlockchainWithDirAndV2Config(
		t.TempDir(),
		f.cfg,
	)

	sender, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	baseState := initialProtocolState(f.cfg)

	// Fund only the transaction sender. The transaction has zero ValueUnits,
	// therefore the balance is needed only for the reserved/actual fee.
	baseState.balances[stateKey(sender.Address())] = AmountToUnits(100)

	beforeRegistryRoot := f.registry.Root()
	beforeMembership := f.membership.Snapshot()
	beforeSequence := baseState.Sequence(sender.Address())

	p := f.proposal(
		3,
		consensus.MembershipActionSuspend,
	)

	cert := f.certificate(t, p, 2)

	tx := r6CanonicalMembershipTx(
		t,
		f.cfg,
		sender,
		cert,
		beforeSequence,
	)

	b := r6CanonicalMembershipExecutionBlock(
		t,
		bc,
		tx,
		10,
	)

	result, err := bc.executeBlockV2(
		baseState,
		bc.hvmEngine,
		f.registry,
		f.membership,
		nil,
		b,
		nil,
	)
	if err != nil {
		t.Fatalf("execute 2-of-4 membership tx: %v", err)
	}

	if len(result.receipts) != 1 {
		t.Fatalf(
			"receipt count=%d want=1",
			len(result.receipts),
		)
	}

	receipt := result.receipts[0]

	if receipt.Success {
		t.Fatal("2-of-4 membership transaction unexpectedly succeeded")
	}

	if strings.TrimSpace(receipt.RevertReason) == "" {
		t.Fatal("2-of-4 reverted without revert reason")
	}

	if got := result.validators.Root(); !strings.EqualFold(
		got,
		beforeRegistryRoot,
	) {
		t.Fatalf(
			"registry mutated on 2-of-4 revert: got=%s want=%s",
			got,
			beforeRegistryRoot,
		)
	}

	afterMembership := result.membership.Snapshot()

	if afterMembership.Version != beforeMembership.Version {
		t.Fatalf(
			"membership version mutated on revert: got=%d want=%d",
			afterMembership.Version,
			beforeMembership.Version,
		)
	}

	if len(afterMembership.Consumed) != len(beforeMembership.Consumed) {
		t.Fatalf(
			"membership consumed set mutated on revert: got=%d want=%d",
			len(afterMembership.Consumed),
			len(beforeMembership.Consumed),
		)
	}

	// Settlement semantics are inherited from SettleV2:
	// reverted protocol transactions still consume their sequence and fee.
	if got := result.state.Sequence(sender.Address()); got != beforeSequence+1 {
		t.Fatalf(
			"sequence after reverted membership tx=%d want=%d",
			got,
			beforeSequence+1,
		)
	}

	if len(result.validatorSet.Validators) != 4 {
		t.Fatalf(
			"block-N preSet validators=%d want=4",
			len(result.validatorSet.Validators),
		)
	}

	if result.validatorSet.TotalPower() != 4 {
		t.Fatalf(
			"block-N preSet power=%d want=4",
			result.validatorSet.TotalPower(),
		)
	}

	if !strings.EqualFold(
		result.validatorSet.Root(),
		f.set.Root(),
	) {
		t.Fatalf(
			"block-N preSet root=%s want=%s",
			result.validatorSet.Root(),
			f.set.Root(),
		)
	}
}

func TestR6CanonicalMembershipTxE2EThreeOfFourAppliesWithPreviousSet(
	t *testing.T,
) {
	f := newR6CanonicalMembershipFixture(t)

	bc := NewBlockchainWithDirAndV2Config(
		t.TempDir(),
		f.cfg,
	)

	sender, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	baseState := initialProtocolState(f.cfg)
	baseState.balances[stateKey(sender.Address())] = AmountToUnits(100)

	beforeSequence := baseState.Sequence(sender.Address())

	p := f.proposal(
		3,
		consensus.MembershipActionSuspend,
	)

	cert := f.certificate(t, p, 3)

	tx := r6CanonicalMembershipTx(
		t,
		f.cfg,
		sender,
		cert,
		beforeSequence,
	)

	b := r6CanonicalMembershipExecutionBlock(
		t,
		bc,
		tx,
		10,
	)

	result, err := bc.executeBlockV2(
		baseState,
		bc.hvmEngine,
		f.registry,
		f.membership,
		nil,
		b,
		nil,
	)
	if err != nil {
		t.Fatalf("execute 3-of-4 membership tx: %v", err)
	}

	if len(result.receipts) != 1 {
		t.Fatalf(
			"receipt count=%d want=1",
			len(result.receipts),
		)
	}

	receipt := result.receipts[0]

	if !receipt.Success {
		t.Fatalf(
			"3-of-4 membership transaction reverted: %s",
			receipt.RevertReason,
		)
	}

	if strings.TrimSpace(receipt.RevertReason) != "" {
		t.Fatalf(
			"successful membership receipt has revert reason: %q",
			receipt.RevertReason,
		)
	}

	if receipt.ComputeUsed != computeValidatorMembership {
		t.Fatalf(
			"compute used=%d want=%d",
			receipt.ComputeUsed,
			computeValidatorMembership,
		)
	}

	if result.membership.Version() != 1 {
		t.Fatalf(
			"post membership version=%d want=1",
			result.membership.Version(),
		)
	}

	snapshot := result.membership.Snapshot()

	if len(snapshot.Consumed) != 1 {
		t.Fatalf(
			"post consumed proposals=%d want=1",
			len(snapshot.Consumed),
		)
	}

	target, ok := result.validators.Get(
		f.vals[3].validator.ID,
	)
	if !ok {
		t.Fatal("target validator missing after membership execution")
	}

	if target.Status != consensus.ValidatorSuspended {
		t.Fatalf(
			"target status=%s want=%s",
			target.Status,
			consensus.ValidatorSuspended,
		)
	}

	// The block being executed remains authorized by the set captured
	// before the membership transaction.
	if len(result.validatorSet.Validators) != 4 {
		t.Fatalf(
			"block-N preSet validators=%d want=4",
			len(result.validatorSet.Validators),
		)
	}

	if result.validatorSet.TotalPower() != 4 {
		t.Fatalf(
			"block-N preSet power=%d want=4",
			result.validatorSet.TotalPower(),
		)
	}

	if !strings.EqualFold(
		result.validatorSet.Root(),
		f.set.Root(),
	) {
		t.Fatalf(
			"block-N preSet root=%s want=%s",
			result.validatorSet.Root(),
			f.set.Root(),
		)
	}

	// The post-state registry is the authority available to the next
	// consensus height.
	nextSet := result.validators.ActiveSet(11)

	if len(nextSet.Validators) != 3 {
		t.Fatalf(
			"block-N+1 active validators=%d want=3",
			len(nextSet.Validators),
		)
	}

	if nextSet.TotalPower() != 3 {
		t.Fatalf(
			"block-N+1 total power=%d want=3",
			nextSet.TotalPower(),
		)
	}

	if _, _, ok := nextSet.Find(
		f.vals[3].validator.ID,
	); ok {
		t.Fatal(
			"suspended validator remained eligible in block-N+1 set",
		)
	}

	if got := consensus.QuorumThreshold(
		nextSet.TotalPower(),
	); got != 3 {
		t.Fatalf(
			"block-N+1 quorum=%d want=3",
			got,
		)
	}

	if got := result.state.Sequence(sender.Address()); got != beforeSequence+1 {
		t.Fatalf(
			"sequence after successful membership tx=%d want=%d",
			got,
			beforeSequence+1,
		)
	}
}
