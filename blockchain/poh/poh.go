package poh

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type PoHEvent struct {
	Sequence  int64  `json:"sequence"`
	Timestamp int64  `json:"timestamp"`
	PrevHash  string `json:"prev_hash"`
	Hash      string `json:"hash"`
}

type PoHGenerator struct {
	events       []PoHEvent
	mu           sync.Mutex
	interval     time.Duration
	verifyWindow int
}

func New(intervalMs, verifyWindow int) *PoHGenerator {
	return &PoHGenerator{
		events: []PoHEvent{{
			Sequence:  0,
			Timestamp: time.Now().Unix(),
			PrevHash:  "0",
			Hash:      "0",
		}},
		interval:     time.Duration(intervalMs) * time.Millisecond,
		verifyWindow: verifyWindow,
	}
}

func (p *PoHGenerator) Generate() {
	sequence := int64(1)
	for {
		time.Sleep(p.interval)

		p.mu.Lock()
		prev := p.events[len(p.events)-1]
		data := []byte(fmt.Sprintf("%d%d%s", sequence, time.Now().UnixNano(), prev.Hash))
		hash := sha256.Sum256(data)

		p.events = append(p.events, PoHEvent{
			Sequence:  sequence,
			Timestamp: time.Now().UnixNano(),
			PrevHash:  prev.Hash,
			Hash:      hex.EncodeToString(hash[:]),
		})
		sequence++
		p.mu.Unlock()
	}
}

func (p *PoHGenerator) GetLastHash() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.events) == 0 {
		return "0"
	}
	return p.events[len(p.events)-1].Hash
}

func (p *PoHGenerator) GetEvents() []PoHEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.events
}
