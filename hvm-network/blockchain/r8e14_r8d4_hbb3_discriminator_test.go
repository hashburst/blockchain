package blockchain

import (
	"reflect"
	"testing"
)

func TestR8E14R8D4CurrentHBB3RejectsLegacyShapedTruncation(t *testing.T) {
	var b Block
	fillLedgerFixture(reflect.ValueOf(&b).Elem())
	b.Version = 0
	b.Hash = b.GenerateHash()

	data, err := EncodeLedgerBlock(nil, &b)
	if err != nil {
		t.Fatal(err)
	}

	if len(data) < 4 || string(data[:4]) != "HBB3" {
		t.Fatalf("current magic=%q want HBB3", data[:min(4, len(data))])
	}

	const boundary = 606

	if len(data) <= boundary {
		t.Fatalf("current length=%d <= ambiguity boundary", len(data))
	}

	var full Block
	if err := DecodeLedgerBlockInto(data, &full); err != nil {
		t.Fatalf("full HBB3 rejected: %v", err)
	}

	var truncated Block
	if err := DecodeLedgerBlockInto(data[:boundary], &truncated); err == nil {
		t.Fatalf("truncated HBB3 accepted at %d", boundary)
	}
}

func TestR8E14R8D4PreMigrationCurrentHBB2StillReadable(t *testing.T) {
	var b Block
	fillLedgerFixture(reflect.ValueOf(&b).Elem())
	b.Version = 2
	b.Hash = b.GenerateHash()

	data, err := EncodeLedgerBlock(nil, &b)
	if err != nil {
		t.Fatal(err)
	}

	if len(data) < 4 || string(data[:4]) != "HBB3" {
		t.Fatal("current encoder did not emit HBB3")
	}

	oldCurrent := append([]byte(nil), data...)
	copy(oldCurrent[:4], "HBB2")

	var restored Block
	if err := DecodeLedgerBlockInto(oldCurrent, &restored); err != nil {
		t.Fatalf("pre-migration current HBB2 rejected: %v", err)
	}

	if !reflect.DeepEqual(b, restored) {
		t.Fatal("pre-migration current HBB2 roundtrip mismatch")
	}
}
