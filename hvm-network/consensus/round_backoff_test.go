package consensus

import (
	"testing"
	"time"
)

func TestRoundBackoffCapAndOverflow(t *testing.T) {
	c := DefaultNetworkConfig()
	for _, r := range []uint64{65, 1000000, 1<<63 - 1} {
		if c.TimeoutFor(StepPrevote, r) != c.TimeoutFor(StepPrevote, c.MaxRound) {
			t.Fatal("backoff unbounded")
		}
	}
	c.MaxRound = 1<<63 - 1
	c.RoundTimeoutDelta = time.Second
	if c.TimeoutFor(StepPrevote, c.MaxRound) != time.Duration(1<<63-1) {
		t.Fatal("duration overflow")
	}
}
