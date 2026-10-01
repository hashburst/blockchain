package blockchain

import (
	"sync"
	"testing"
)

func TestPoHMemoExactInputsAndEviction(t *testing.T) {
	var m pohMemo
	for repeat := 0; repeat < 2; repeat++ {
		for prev := int64(-40); prev < 40; prev++ {
			for _, ticks := range []int{0, 1, 8, 17} {
				want := computePoHWithTicks(prev, ticks)
				if got := m.calculate(prev, ticks); got != want {
					t.Fatalf("prev=%d ticks=%d got=%d want=%d", prev, ticks, got, want)
				}
			}
		}
	}
}
func TestPoHMemoConcurrent(t *testing.T) {
	var m pohMemo
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(prev int64) {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				if m.calculate(prev, 32) != computePoHWithTicks(prev, 32) {
					t.Error("memo changed PoH")
				}
			}
		}(int64(i))
	}
	wg.Wait()
}
func BenchmarkPoHRepeatedParent(b *testing.B) {
	b.Run("uncached", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			computePoHWithTicks(123, 4000)
		}
	})
	b.Run("memo", func(b *testing.B) {
		var m pohMemo
		m.calculate(123, 4000)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			m.calculate(123, 4000)
		}
	})
}
