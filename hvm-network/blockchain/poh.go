package blockchain

import (
	"crypto/sha512"
	"encoding/binary"
	"sync"
)

// PoHTicks is the historical/production number of sequential SHA-512 rounds
// per block. Existing/default configurations always use this value.
const PoHTicks = 400_000

// poHWithTicks is the common deterministic PoH primitive. A fixed 8-byte
// state avoids one heap allocation per SHA-512 round while preserving the
// exact historical byte sequence and therefore every existing PoH vector.
// Only deterministic results computed by this process are memoized. No disk,
// peer or checkpoint result can seed this cache. Exact input equality is used.
var recentPoH pohMemo

type pohMemoEntry struct {
	prev    int64
	ticks   int
	result  int64
	present bool
}
type pohMemo struct {
	mu      sync.Mutex
	entries [32]pohMemoEntry
	next    uint
}

func (m *pohMemo) calculate(prev int64, ticks int) int64 {
	m.mu.Lock()
	for _, e := range m.entries {
		if e.present && e.prev == prev && e.ticks == ticks {
			m.mu.Unlock()
			return e.result
		}
	}
	m.mu.Unlock()
	// Do not serialize unrelated cold checks behind an expensive hash chain.
	result := computePoHWithTicks(prev, ticks)
	m.mu.Lock()
	m.entries[m.next] = pohMemoEntry{prev, ticks, result, true}
	m.next = (m.next + 1) % uint(len(m.entries))
	m.mu.Unlock()
	return result
}
func poHWithTicks(prev int64, ticks int) int64 {
	return recentPoH.calculate(prev, ticks)
}
func computePoHWithTicks(prev int64, ticks int) int64 {
	var state [8]byte
	binary.LittleEndian.PutUint64(state[:], uint64(prev))
	for i := 0; i < ticks; i++ {
		sum := sha512.Sum512(state[:])
		copy(state[:], sum[:8])
	}
	return int64(binary.LittleEndian.Uint64(state[:]))
}

// PoH preserves the historical production rule exactly: 400,000 sequential
// SHA-512 rounds derived from the previous block's ProofOfTime.
func PoH(prev int64) int64 { return poHWithTicks(prev, PoHTicks) }

// validatePoHWithTicks validates the deterministic PoH chain at the configured
// per-block tick count. HVM Network isolated fixtures may select fewer ticks to
// keep acceptance tests bounded; production/default configs remain at PoHTicks.
func validatePoHWithTicks(prev, poh int64, ticks int) bool {
	return poHWithTicks(prev, ticks) == poh
}

// ValidatePoH preserves the historical/default validation rule.
func ValidatePoH(prev, poh int64) bool { return validatePoHWithTicks(prev, poh, PoHTicks) }
