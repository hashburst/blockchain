package consensus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestartGuardPersistsAndRefusesUnfinalizedSignatures(t *testing.T) {
	p := filepath.Join(t.TempDir(), "journal.jsonl")
	j, e := OpenBFTSignJournal(p)
	if e != nil {
		t.Fatal(e)
	}
	r := BFTSignRecord{Step: StepPrevote, ChainID: 1337, Height: 10, Round: 2, ValidatorID: "ab", BlockHash: strings.Repeat("aa", 32), ValidatorSetRoot: strings.Repeat("bb", 32), Signature: "record-test"}
	if e = j.Record(r); e != nil {
		t.Fatal(e)
	}
	j, e = OpenBFTSignJournal(p)
	if e != nil {
		t.Fatal(e)
	}
	for _, h := range []uint64{9, 10} {
		if j.CheckRestartHeight(1337, h, "ab") == nil {
			t.Fatal("pending signature accepted")
		}
	}
	if e = j.CheckRestartHeight(1337, 11, "ab"); e != nil {
		t.Fatal(e)
	}
}

func TestJournalPersistenceFailureIsLatched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "journal")
	j, err := OpenBFTSignJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	rec := BFTSignRecord{Step: StepPrevote, ChainID: 1337, Height: 10, ValidatorID: "ab", BlockHash: NilBlockHash, ValidatorSetRoot: strings.Repeat("bb", 32), Signature: "test"}
	if err = j.Record(rec); err == nil {
		t.Fatal("expected append failure")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if j.Record(rec) == nil || j.Healthy() == nil {
		t.Fatal("uncertain journal reused")
	}
	if _, err = j.PendingRecords(1337, 10, "ab"); err == nil {
		t.Fatal("unhealthy journal exposed as recoverable")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("latched journal wrote again")
	}
}
