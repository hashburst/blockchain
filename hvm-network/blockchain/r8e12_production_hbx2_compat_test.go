package blockchain

import (
    "os"
    "path/filepath"
    "testing"

    "hashburst/ledger"
)

func TestR8E12ProductionHBX2BlockZeroCompatibility(t *testing.T) {
    dir := os.Getenv("HASHBURST_R8E12_FIXTURE")
    if dir == "" {
        t.Skip("HASHBURST_R8E12_FIXTURE not set")
    }

    r, err := ledger.OpenLiveReader(dir, 8<<20, 8)
    if err != nil {
        t.Fatalf("OpenLiveReader(%s): %v", filepath.Base(dir), err)
    }
    defer r.Close()

    if got := r.Format(); got != ledger.BinaryPayload {
        t.Fatalf("format=%v want BinaryPayload", got)
    }

    if r.Count() == 0 {
        t.Fatal("production fixture contains no blocks")
    }

    if _, err := readHistoryBlock(r, 0); err != nil {
        t.Fatalf("production HBX2 block 0 compatibility: %v", err)
    }
}
