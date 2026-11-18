package blockchain

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// Membership state is consensus state. Once it has a canonical root, the block
// carrying that state transition must commit to it in the same canonical block
// object that already commits HBT, HVM, receipts and validator state.
//
// This test intentionally precedes the production field. The expected RED
// state is a compile failure because MembershipStateRoot does not exist yet.
func TestR6BlockMembershipStateRootAffectsCanonicalHash(t *testing.T) {
	base := &Block{
		Version:            BlockVersionV2,
		ProtocolChainID:    4735489,
		Index:              17,
		Timestamp:          time.Unix(1837000000, 123456789).UTC(),
		PrevHash:           strings.Repeat("11", 32),
		ProofOfWork:        7,
		ProofOfTime:        19,
		HBTStateRoot:       strings.Repeat("22", 32),
		HVMStateRoot:       strings.Repeat("33", 32),
		ReceiptsRoot:       strings.Repeat("44", 32),
		ValidatorSetRoot:   strings.Repeat("55", 32),
		ValidatorStateRoot: strings.Repeat("66", 32),
		AuthorValidatorID:  strings.Repeat("77", 20),
	}

	a := *base
	b := *base

	a.MembershipStateRoot = strings.Repeat("88", 32)
	b.MembershipStateRoot = strings.Repeat("99", 32)

	if a.GenerateHash() == b.GenerateHash() {
		t.Fatal("membership state root is not committed by canonical block hash")
	}
}

// HBX2 is the current versioned binary ledger representation. The membership
// commitment must survive the same codec boundary as the validator roots.
func TestR6MembershipStateRootSurvivesHBX2RoundTrip(t *testing.T) {
	want := strings.Repeat("ab", 32)

	in := &Block{
		Version:             BlockVersionV2,
		ProtocolChainID:     4735489,
		Index:               23,
		Timestamp:           time.Unix(1837000100, 987654321).UTC(),
		PrevHash:            strings.Repeat("10", 32),
		Hash:                strings.Repeat("20", 32),
		ProofOfWork:         11,
		ProofOfTime:         29,
		HBTStateRoot:        strings.Repeat("30", 32),
		HVMStateRoot:        strings.Repeat("40", 32),
		ReceiptsRoot:        strings.Repeat("50", 32),
		ValidatorSetRoot:    strings.Repeat("60", 32),
		ValidatorStateRoot:  strings.Repeat("70", 32),
		MembershipStateRoot: want,
		AuthorValidatorID:   strings.Repeat("80", 20),
	}

	encoded, err := EncodeLedgerBlock(nil, in)
	if err != nil {
		t.Fatalf("EncodeLedgerBlock: %v", err)
	}

	if len(encoded) < 4 || !bytes.Equal(encoded[:4], []byte("HBB2")) {
		t.Fatalf("unexpected HBX2 prefix: %x", encoded)
	}

	var out Block
	if err := DecodeLedgerBlockInto(encoded, &out); err != nil {
		t.Fatalf("DecodeLedgerBlockInto: %v", err)
	}

	if out.MembershipStateRoot != want {
		t.Fatalf(
			"membership state root after HBX2 round trip=%q want=%q",
			out.MembershipStateRoot,
			want,
		)
	}
}

// Sync is a separate block reconstruction boundary. The commitment transmitted
// by a peer must remain byte-for-byte represented by the reconstructed Block.
func TestR6MembershipStateRootSurvivesWireRoundTrip(t *testing.T) {
	want := strings.Repeat("cd", 32)

	in := &Block{
		Version:             BlockVersionV2,
		ProtocolChainID:     4735489,
		Index:               31,
		Timestamp:           time.Unix(1837000200, 0).UTC(),
		PrevHash:            strings.Repeat("12", 32),
		Hash:                strings.Repeat("34", 32),
		ValidatorSetRoot:    strings.Repeat("56", 32),
		ValidatorStateRoot:  strings.Repeat("78", 32),
		MembershipStateRoot: want,
	}

	wire := blockToWire(in)

	if wire.MembershipStateRoot != want {
		t.Fatalf(
			"wire membership state root=%q want=%q",
			wire.MembershipStateRoot,
			want,
		)
	}

	out := wire.toBlock()

	if out.MembershipStateRoot != want {
		t.Fatalf(
			"block membership state root after wire round trip=%q want=%q",
			out.MembershipStateRoot,
			want,
		)
	}
}

// blockOnDisk is another real Block reconstruction boundary found by R3F-2A.
// The commitment must not disappear when crossing it.
func TestR6MembershipStateRootSurvivesOnDiskRoundTrip(t *testing.T) {
	want := strings.Repeat("ef", 32)

	in := &Block{
		Version:             BlockVersionV2,
		ProtocolChainID:     4735489,
		Index:               37,
		Timestamp:           time.Unix(1837000300, 0).UTC(),
		PrevHash:            strings.Repeat("13", 32),
		Hash:                strings.Repeat("35", 32),
		ValidatorSetRoot:    strings.Repeat("57", 32),
		ValidatorStateRoot:  strings.Repeat("79", 32),
		MembershipStateRoot: want,
	}

	disk := blockToOnDisk(in)

	if disk.MembershipStateRoot != want {
		t.Fatalf(
			"on-disk membership state root=%q want=%q",
			disk.MembershipStateRoot,
			want,
		)
	}

	out := blockFromOnDisk(&disk)

	if out.MembershipStateRoot != want {
		t.Fatalf(
			"block membership state root after on-disk round trip=%q want=%q",
			out.MembershipStateRoot,
			want,
		)
	}
}
