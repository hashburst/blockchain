package protocolv2

import (
	"bytes"
	"testing"

	"hashburst/wallet"
)

func TestTransactionV2SignVerifyAndMutation(t *testing.T) {
	w, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	to, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	tx := NewTransactionV2(1337, TxContractCall, w.Address(), to.Address(), 0, 7, 100000, 10, []byte(`{"method":"ping"}`))
	if err := tx.Sign(w); err != nil {
		t.Fatal(err)
	}
	if err := tx.Verify(1337); err != nil {
		t.Fatalf("signed tx did not verify: %v", err)
	}

	originalHash := append([]byte(nil), tx.SigningHash()...)
	tx.Sequence++
	if bytes.Equal(originalHash, tx.SigningHash()) {
		t.Fatal("sequence mutation did not change signing hash")
	}
	if err := tx.Verify(1337); err == nil {
		t.Fatal("mutated transaction unexpectedly verified")
	}
}

func TestFeePolicy(t *testing.T) {
	p := FeePolicy{BaseTxUnits: 2, FeeRateUnitsPerMillion: 5}
	fee, err := p.ComputeFee(1_500_000)
	if err != nil {
		t.Fatal(err)
	}
	if fee != 10 { // 2 + ceil(7.5)
		t.Fatalf("fee=%d, want 10", fee)
	}
}

func TestTransactionV2RawRoundTrip(t *testing.T) {
	w, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	to, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	tx := NewTransactionV2(1337, TxHBTTransfer, w.Address(), to.Address(), 123456789, 3, 0, 101, []byte("memo"))
	if err := tx.Sign(w); err != nil {
		t.Fatal(err)
	}
	raw, err := tx.EncodeRaw()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRawTransactionV2(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.HashHex() != tx.HashHex() || decoded.Signature != tx.Signature || decoded.ID != tx.ID {
		t.Fatalf("raw round trip mismatch: got=%+v want=%+v", decoded, tx)
	}
	if err := decoded.Verify(1337); err != nil {
		t.Fatalf("decoded raw transaction did not verify: %v", err)
	}
}

func TestFeePolicyRejectsOverflow(t *testing.T) {
	p := FeePolicy{BaseTxUnits: 1, FeeRateUnitsPerMillion: 1 << 62}
	if _, err := p.ComputeFee(^uint64(0)); err == nil {
		t.Fatal("overflowing fee calculation unexpectedly succeeded")
	}
}
