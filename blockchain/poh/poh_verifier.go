package poh

import (
	"time"
)

func (p *PoHGenerator) VerifySequence() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	events := p.events
	if len(events) <= 1 {
		return true
	}

	for i := 1; i < len(events); i++ {
		current := events[i]
		previous := events[i-1]

		// Verifica la catena temporale
		if current.Timestamp <= previous.Timestamp {
			return false
		}

		// Verifica la relazione tra hash
		if current.PrevHash != previous.Hash {
			return false
		}

		// Verifica la finestra temporale
		if i >= p.verifyWindow {
			windowStart := i - p.verifyWindow
			expectedDuration := time.Duration(p.verifyWindow) * p.interval
			actualDuration := time.Duration(current.Timestamp-events[windowStart].Timestamp) * time.Nanosecond
			
			if actualDuration > expectedDuration*2 || actualDuration < expectedDuration/2 {
				return false
			}
		}
	}
	return true
}
