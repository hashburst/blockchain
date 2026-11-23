package blockchain

import (
	"bytes"
	"testing"
)

func TestR8E14LegacyHBB2RoundTripCompatibility(t *testing.T) {
	original := &Block{
		ValidatorSetRoot:   "validator-set-root",
		ValidatorStateRoot: "validator-state-root",
		AuthorValidatorID:  "validator-author",
		ProposerID:         "validator-proposer",
	}

	wire, err := encodeLedgerBlockLegacy(nil, original)
	if err != nil {
		t.Fatalf("legacy encode: %v", err)
	}

	var decoded Block
	if err := DecodeLedgerBlockInto(wire, &decoded); err != nil {
		t.Fatalf("legacy decode: %v", err)
	}

	if decoded.MembershipStateRoot != "" {
		t.Fatalf("legacy MembershipStateRoot must be empty, got %q",
			decoded.MembershipStateRoot)
	}

	again, err := encodeLedgerBlockLegacy(nil, &decoded)
	if err != nil {
		t.Fatalf("legacy re-encode: %v", err)
	}

	if !bytes.Equal(wire, again) {
		t.Fatal("legacy roundtrip is not byte-exact")
	}
}

func TestR8E14CurrentHBB2MembershipRoundTrip(t *testing.T) {
	original := &Block{
		ValidatorSetRoot:    "validator-set-root",
		ValidatorStateRoot:  "validator-state-root",
		MembershipStateRoot: "membership-state-root",
		AuthorValidatorID:   "validator-author",
		ProposerID:          "validator-proposer",
	}

	wire, err := EncodeLedgerBlock(nil, original)
	if err != nil {
		t.Fatalf("current encode: %v", err)
	}

	var decoded Block
	if err := DecodeLedgerBlockInto(wire, &decoded); err != nil {
		t.Fatalf("current decode: %v", err)
	}

	if decoded.MembershipStateRoot != original.MembershipStateRoot {
		t.Fatalf("membership root mismatch: got %q want %q",
			decoded.MembershipStateRoot,
			original.MembershipStateRoot)
	}
}

func TestR8E14LegacyHBB2RejectsTrailingBytes(t *testing.T) {
	original := &Block{
		ValidatorSetRoot:   "validator-set-root",
		ValidatorStateRoot: "validator-state-root",
		AuthorValidatorID:  "validator-author",
		ProposerID:         "validator-proposer",
	}

	wire, err := encodeLedgerBlockLegacy(nil, original)
	if err != nil {
		t.Fatalf("legacy encode: %v", err)
	}

	wire = append(wire, 0x00)

	var decoded Block
	if err := DecodeLedgerBlockInto(wire, &decoded); err == nil {
		t.Fatal("decoder accepted legacy record with trailing byte")
	}
}

func TestR8E14LegacyHBB2RejectsCorruption(t *testing.T) {
	original := &Block{
		ValidatorSetRoot:   "validator-set-root",
		ValidatorStateRoot: "validator-state-root",
		AuthorValidatorID:  "validator-author",
		ProposerID:         "validator-proposer",
	}

	wire, err := encodeLedgerBlockLegacy(nil, original)
	if err != nil {
		t.Fatalf("legacy encode: %v", err)
	}

	if len(wire) < 8 {
		t.Fatal("unexpectedly short legacy record")
	}

	// Damage the HBB2 magic. Neither current nor legacy schema may accept it.
	wire[0] ^= 0xff

	var decoded Block
	if err := DecodeLedgerBlockInto(wire, &decoded); err == nil {
		t.Fatal("decoder accepted corrupted HBB2 record")
	}
}
