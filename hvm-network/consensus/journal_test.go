package consensus

import (
	"path/filepath"
	"testing"
)

func journalVote(height, round uint64, block, validator string) Vote {
	return Vote{
		ChainID:          1337,
		Height:           height,
		Round:            round,
		BlockHash:        block,
		ValidatorSetRoot: "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		ValidatorID:      validator,
		Signature:        "journal-does-not-validate-crypto",
	}
}

func TestVoteJournalPersistsAndRejectsCrossRoundDoubleSign(t *testing.T) {
	path := filepath.Join(t.TempDir(), "validator-votes.jsonl")
	validator := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	blockA := "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	blockB := "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

	j, err := OpenVoteJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	first := journalVote(7, 0, blockA, validator)
	if err := j.Record(first); err != nil {
		t.Fatal(err)
	}
	if err := j.Record(first); err != nil {
		t.Fatalf("idempotent vote rejected: %v", err)
	}
	if err := j.Record(journalVote(7, 1, blockB, validator)); err == nil {
		t.Fatal("cross-round double-sign unexpectedly allowed")
	}

	reopened, err := OpenVoteJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := reopened.Get(1337, 7, validator); !ok || got.BlockHash != blockA || got.Round != 0 {
		t.Fatalf("journal did not survive restart: %+v ok=%v", got, ok)
	}
	if err := reopened.Record(journalVote(7, 2, blockB, validator)); err == nil {
		t.Fatal("restart bypassed double-sign protection")
	}
}
