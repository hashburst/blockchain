package blockchain

import (
	"encoding/json"
	"testing"

	"hashburst/consensus"
	"hashburst/protocolv2"
	"hashburst/wallet"
)

func TestR6CanonicalMembershipTransactionIsExecutable(t *testing.T) {
	if !isProtocolV2ExecutableTxType(protocolv2.TxValidatorMembership) {
		t.Fatal("canonical validator membership transaction is not executable")
	}
}

func TestR6CanonicalMembershipComputeRequirement(t *testing.T) {
	cfg := phase3CTestConfig()
	bc := NewBlockchainWithDirAndV2Config(t.TempDir(), cfg)

	signer, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	fee, err := cfg.FeePolicy.ComputeFee(computeValidatorMembership)
	if err != nil {
		t.Fatal(err)
	}

	tx := protocolv2.NewTransactionV2(
		cfg.ChainID,
		protocolv2.TxValidatorMembership,
		signer.Address(),
		"",
		0,
		0,
		computeValidatorMembership,
		fee,
		nil,
	)
	if err := tx.Sign(signer); err != nil {
		t.Fatal(err)
	}

	if err := bc.validateV2Transaction(tx); err != nil {
		t.Fatalf("membership execution envelope rejected: %v", err)
	}
}

func TestR6CanonicalMembershipCertificatePayloadRoundTrip(t *testing.T) {
	cert := consensus.MembershipCertificate{
		Proposal: consensus.MembershipProposal{
			ChainID:         4735489,
			Version:         2,
			PreviousVersion: 1,
			PreviousSetRoot: "0x1234",
			ValidatorID:     "0xabcd",
			Action:          consensus.MembershipActionSuspend,
		},
		Votes: []consensus.MembershipVote{
			{
				ProposalHash: "0x5678",
				ValidatorID:  "0xvalidator",
				Signature:    "0xsignature",
			},
		},
	}

	raw, err := json.Marshal(cert)
	if err != nil {
		t.Fatal(err)
	}

	var decoded consensus.MembershipCertificate
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Proposal.ChainID != cert.Proposal.ChainID ||
		decoded.Proposal.Version != cert.Proposal.Version ||
		decoded.Proposal.PreviousVersion != cert.Proposal.PreviousVersion ||
		decoded.Proposal.Action != cert.Proposal.Action ||
		len(decoded.Votes) != 1 {
		t.Fatal("membership certificate payload changed across JSON round-trip")
	}
}

func TestR6CanonicalMembershipStateCanBeShadowed(t *testing.T) {
	base := consensus.NewMembershipState(7)

	snap := base.Snapshot()

	shadow, err := consensus.RestoreMembershipState(snap)
	if err != nil {
		t.Fatal(err)
	}

	if shadow == base {
		t.Fatal("membership shadow aliases base state")
	}

	if shadow.Version() != base.Version() {
		t.Fatalf(
			"shadow version=%d base=%d",
			shadow.Version(),
			base.Version(),
		)
	}
}
