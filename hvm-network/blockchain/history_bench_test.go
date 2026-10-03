package blockchain

import (
	"fmt"
	"hashburst/ledger"
	"path/filepath"
	"testing"
	"time"
)

// This measures indexed access/decoding, not consensus validation, RSS or Joules.
func BenchmarkIndexedHistoryRead(b *testing.B) {
	dir := filepath.Join(b.TempDir(), "generation")
	_, e := ledger.WriteGeneration(dir, 4096, func(n uint64) ([]byte, error) {
		return EncodeLedgerBlock(nil, &Block{Index: int(n), Timestamp: time.Unix(0, 0), Hash: fmt.Sprintf("%064x", n+1), PrevHash: fmt.Sprintf("%064x", n)})
	})
	if e != nil {
		b.Fatal(e)
	}
	for _, budget := range []int{0, 1 << 20} {
		b.Run(fmt.Sprintf("cache_%d", budget), func(b *testing.B) {
			r, e := ledger.OpenLiveReader(dir, budget, 512)
			if e != nil {
				b.Fatal(e)
			}
			defer r.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				if _, e := readHistoryBlock(r, n%256); e != nil {
					b.Fatal(e)
				}
			}
		})
	}
}
