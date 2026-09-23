package consensus

import (
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
