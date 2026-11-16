package blockchain

import (
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// PeerOperationalState is deliberately NOT a consensus validator state.
//
// A peer may be operationally degraded and immediately deprioritized for
// chain sync, RPC or other non-critical traffic. This state MUST NOT change
// validator eligibility, signatures, quorum, finality or validator-set
// membership.
type PeerOperationalState string

const (
	PeerOperationalReady    PeerOperationalState = "READY"
	PeerOperationalDegraded PeerOperationalState = "DEGRADED"
)

type adaptivePeerState struct {
	State          PeerOperationalState
	Failures       uint32
	Successes      uint32
	ConsecutiveOK  uint32
	LastLatency    time.Duration
	DegradedSince  time.Time
	LastObservedAt time.Time
}

// AdaptiveRoutingPolicy contains only local operational routing state.
//
// It is intentionally independent from consensus.Registry and from the BFT
// transport. Local telemetry can influence operational peer preference, but
// cannot unilaterally exclude a validator from consensus.
type AdaptiveRoutingPolicy struct {
	mu sync.RWMutex

	peers map[peer.ID]adaptivePeerState

	failureThreshold uint32
	recoverySuccess  uint32
	latencyThreshold time.Duration
}

func NewAdaptiveRoutingPolicy() *AdaptiveRoutingPolicy {
	return &AdaptiveRoutingPolicy{
		peers:            make(map[peer.ID]adaptivePeerState),
		failureThreshold: 3,
		recoverySuccess:  2,
		latencyThreshold: 2 * time.Second,
	}
}

func (p *AdaptiveRoutingPolicy) ObserveSuccess(id peer.ID, latency time.Duration) {
	if p == nil || id == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	s := p.peers[id]
	s.Successes++
	s.ConsecutiveOK++
	s.LastLatency = latency
	s.LastObservedAt = time.Now()

	if latency > p.latencyThreshold {
		s.Failures++
		s.ConsecutiveOK = 0
		if s.Failures >= p.failureThreshold {
			s.State = PeerOperationalDegraded
			if s.DegradedSince.IsZero() {
				s.DegradedSince = s.LastObservedAt
			}
		}
	} else {
		if s.State == "" {
			s.State = PeerOperationalReady
		}
		if s.State == PeerOperationalDegraded && s.ConsecutiveOK >= p.recoverySuccess {
			s.State = PeerOperationalReady
			s.Failures = 0
			s.DegradedSince = time.Time{}
		}
	}

	p.peers[id] = s
}

func (p *AdaptiveRoutingPolicy) ObserveFailure(id peer.ID) {
	if p == nil || id == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	s := p.peers[id]
	s.Failures++
	s.ConsecutiveOK = 0
	s.LastObservedAt = time.Now()

	if s.Failures >= p.failureThreshold {
		s.State = PeerOperationalDegraded
		if s.DegradedSince.IsZero() {
			s.DegradedSince = s.LastObservedAt
		}
	} else if s.State == "" {
		s.State = PeerOperationalReady
	}

	p.peers[id] = s
}

func (p *AdaptiveRoutingPolicy) OperationalState(id peer.ID) PeerOperationalState {
	if p == nil {
		return PeerOperationalReady
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	s, ok := p.peers[id]
	if !ok || s.State == "" {
		return PeerOperationalReady
	}
	return s.State
}

// Prefer orders peers for non-consensus traffic.
//
// READY peers are preferred. DEGRADED peers are retained as fallback instead
// of being disconnected, preserving liveness when every available route is
// degraded.
//
// This function MUST NOT be used for consensus proposal/vote/QC delivery.
func (p *AdaptiveRoutingPolicy) Prefer(ids []peer.ID) []peer.ID {
	out := append([]peer.ID(nil), ids...)
	if p == nil || len(out) < 2 {
		return out
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	sort.SliceStable(out, func(i, j int) bool {
		a := p.peers[out[i]]
		b := p.peers[out[j]]

		aDegraded := a.State == PeerOperationalDegraded
		bDegraded := b.State == PeerOperationalDegraded

		if aDegraded != bDegraded {
			return !aDegraded
		}

		if a.LastLatency != b.LastLatency {
			if a.LastLatency == 0 {
				return false
			}
			if b.LastLatency == 0 {
				return true
			}
			return a.LastLatency < b.LastLatency
		}

		return string(out[i]) < string(out[j])
	})

	return out
}
