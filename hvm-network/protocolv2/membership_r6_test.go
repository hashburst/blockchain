package protocolv2

import (
	"bytes"
	"testing"

	"hashburst/wallet"
)

func TestR6CanonicalMembershipTransactionEncoding(t *testing.T) {
	signer, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	data := []byte(`{"membership":"canonical"}`)

	tx := NewTransactionV2(
		4735489,
		TxValidatorMembership,
		signer.Address(),
		"",
		0,
		7,
		90000,
		123,
		data,
	)

	if tx.Type.String() != "VALIDATOR_MEMBERSHIP" {
		t.Fatalf("type string=%q", tx.Type.String())
	}

	before := append([]byte(nil), tx.CanonicalBytes()...)

	if err := tx.Sign(signer); err != nil {
		t.Fatal(err)
	}
	if err := tx.Verify(4735489); err != nil {
		t.Fatalf("canonical membership transaction rejected: %v", err)
	}

	raw, err := tx.EncodeRaw()
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeRawTransactionV2(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := decoded.Verify(4735489); err != nil {
		t.Fatalf("decoded membership transaction rejected: %v", err)
	}

	if !bytes.Equal(before, decoded.CanonicalBytes()) {
		t.Fatal("canonical bytes changed across encode/decode")
	}
	if decoded.HashHex() != tx.HashHex() {
		t.Fatal("transaction hash changed across encode/decode")
	}
}

func TestR6CanonicalMembershipWrongChainIDRejected(t *testing.T) {
	signer, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	tx := NewTransactionV2(
		4735489,
		TxValidatorMembership,
		signer.Address(),
		"",
		0,
		0,
		90000,
		123,
		[]byte(`{"membership":"chain-bound"}`),
	)

	if err := tx.Sign(signer); err != nil {
		t.Fatal(err)
	}

	if err := tx.Verify(4735490); err == nil {
		t.Fatal("foreign chain membership transaction unexpectedly accepted")
	}
}
