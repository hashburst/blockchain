package blockchain

import (
	"context"
	"testing"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestAdaptiveRoutingFailureHysteresis(t *testing.T) {
	p := NewAdaptiveRoutingPolicy()
	id := peer.ID("peer-a")

	p.ObserveFailure(id)
	p.ObserveFailure(id)

	if got := p.OperationalState(id); got != PeerOperationalReady {
		t.Fatalf("peer degraded before threshold: got=%s", got)
	}

	p.ObserveFailure(id)

	if got := p.OperationalState(id); got != PeerOperationalDegraded {
		t.Fatalf("peer not degraded at threshold: got=%s", got)
	}
}

func TestAdaptiveRoutingLatencyHysteresis(t *testing.T) {
	p := NewAdaptiveRoutingPolicy()
	id := peer.ID("peer-latency")

	for i := 0; i < 2; i++ {
		p.ObserveSuccess(id, 3*time.Second)
	}

	if got := p.OperationalState(id); got != PeerOperationalReady {
		t.Fatalf("peer degraded before latency threshold count: got=%s", got)
	}

	p.ObserveSuccess(id, 3*time.Second)

	if got := p.OperationalState(id); got != PeerOperationalDegraded {
		t.Fatalf("slow peer not degraded after repeated latency: got=%s", got)
	}
}

func TestAdaptiveRoutingRecoveryRequiresConsecutiveSuccesses(t *testing.T) {
	p := NewAdaptiveRoutingPolicy()
	id := peer.ID("peer-recovery")

	for i := 0; i < 3; i++ {
		p.ObserveFailure(id)
	}

	if got := p.OperationalState(id); got != PeerOperationalDegraded {
		t.Fatalf("setup failed: got=%s", got)
	}

	p.ObserveSuccess(id, 10*time.Millisecond)

	if got := p.OperationalState(id); got != PeerOperationalDegraded {
		t.Fatalf("peer recovered without hysteresis: got=%s", got)
	}

	p.ObserveSuccess(id, 10*time.Millisecond)

	if got := p.OperationalState(id); got != PeerOperationalReady {
		t.Fatalf("peer did not recover after consecutive successes: got=%s", got)
	}
}

func TestAdaptiveRoutingPreferReadyButRetainDegradedFallback(t *testing.T) {
	p := NewAdaptiveRoutingPolicy()

	degraded := peer.ID("peer-degraded")
	readyFast := peer.ID("peer-ready-fast")
	readySlow := peer.ID("peer-ready-slow")

	for i := 0; i < 3; i++ {
		p.ObserveFailure(degraded)
	}

	p.ObserveSuccess(readySlow, 400*time.Millisecond)
	p.ObserveSuccess(readyFast, 20*time.Millisecond)

	got := p.Prefer([]peer.ID{degraded, readySlow, readyFast})

	if len(got) != 3 {
		t.Fatalf("routing policy removed fallback peer: got=%d want=3", len(got))
	}

	if got[0] != readyFast {
		t.Fatalf("fast ready peer not preferred: got=%s", got[0])
	}

	if got[1] != readySlow {
		t.Fatalf("second ready peer ordering wrong: got=%s", got[1])
	}

	if got[2] != degraded {
		t.Fatalf("degraded peer not retained as fallback: got=%s", got[2])
	}
}

func TestAdaptiveRoutingAllDegradedStillRetained(t *testing.T) {
	p := NewAdaptiveRoutingPolicy()

	ids := []peer.ID{
		peer.ID("peer-a"),
		peer.ID("peer-b"),
		peer.ID("peer-c"),
	}

	for _, id := range ids {
		for i := 0; i < 3; i++ {
			p.ObserveFailure(id)
		}
	}

	got := p.Prefer(ids)

	if len(got) != len(ids) {
		t.Fatalf("all-degraded routing lost peers: got=%d want=%d", len(got), len(ids))
	}

	seen := make(map[peer.ID]bool)
	for _, id := range got {
		seen[id] = true
	}

	for _, id := range ids {
		if !seen[id] {
			t.Fatalf("degraded fallback peer disappeared: %s", id)
		}
	}
}

// This is an architectural regression test.
//
// AdaptiveRoutingPolicy is local operational state only. Marking a libp2p
// peer DEGRADED must not alter the validator registry, ActiveSet or voting
// power. Consensus exclusion requires a protocol-level validator transition.
func TestAdaptiveRoutingCongestionDoesNotChangeValidatorActiveSet(t *testing.T) {
	s := setupPhase3DChains(t)
	bc := s.nodes[0]

	height := uint64(bc.Height() + 1)

	before := bc.CurrentValidatorSet(height)

	if len(before.Validators) == 0 {
		t.Fatal("fixture has no active validators")
	}

	targetPeer := peer.ID(before.Validators[0].PeerID)
	if targetPeer == "" {
		targetPeer = peer.ID("operational-peer-for-validator")
	}

	p := NewAdaptiveRoutingPolicy()

	for i := 0; i < 3; i++ {
		p.ObserveFailure(targetPeer)
	}

	if got := p.OperationalState(targetPeer); got != PeerOperationalDegraded {
		t.Fatalf("operational degradation failed: got=%s", got)
	}

	after := bc.CurrentValidatorSet(height)

	if len(after.Validators) != len(before.Validators) {
		t.Fatalf(
			"operational congestion changed validator count: before=%d after=%d",
			len(before.Validators),
			len(after.Validators),
		)
	}

	if before.Root() != after.Root() {
		t.Fatalf(
			"operational congestion changed validator-set root: before=%s after=%s",
			before.Root(),
			after.Root(),
		)
	}

	if before.TotalPower() != after.TotalPower() {
		t.Fatalf(
			"operational congestion changed consensus power: before=%d after=%d",
			before.TotalPower(),
			after.TotalPower(),
		)
	}
}

func TestAdaptiveRoutingRealLibp2pSyncFeedbackAndFallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client, err := libp2p.New()
	if err != nil {
		t.Fatalf("create client host: %v", err)
	}
	defer client.Close()

	healthy, err := libp2p.New()
	if err != nil {
		t.Fatalf("create healthy host: %v", err)
	}
	defer healthy.Close()

	slow, err := libp2p.New()
	if err != nil {
		t.Fatalf("create slow host: %v", err)
	}
	defer slow.Close()

	installResponder := func(delay time.Duration, h network.StreamHandler) network.StreamHandler {
		return func(s network.Stream) {
			if delay > 0 {
				time.Sleep(delay)
			}
			h(s)
		}
	}

	respondEmptyBlocks := func(s network.Stream) {
		defer s.Close()

		msg, err := readFrame(s)
		if err != nil {
			return
		}
		if msg.Type != MsgGetBlocks {
			return
		}

		_ = writeFrame(s, &syncMessage{
			Type:   MsgBlocks,
			Blocks: nil,
		})
	}

	healthy.SetStreamHandler(
		ProtocolID,
		installResponder(0, respondEmptyBlocks),
	)

	slow.SetStreamHandler(
		ProtocolID,
		installResponder(2100*time.Millisecond, respondEmptyBlocks),
	)

	connect := func(h peer.ID, addrs []peer.AddrInfo) {
		_ = h
		_ = addrs
	}

	_ = connect

	for _, remote := range []struct {
		id    peer.ID
		addrs []peer.AddrInfo
	}{
		{
			id: healthy.ID(),
			addrs: []peer.AddrInfo{{
				ID:    healthy.ID(),
				Addrs: healthy.Addrs(),
			}},
		},
		{
			id: slow.ID(),
			addrs: []peer.AddrInfo{{
				ID:    slow.ID(),
				Addrs: slow.Addrs(),
			}},
		},
	} {
		if len(remote.addrs) != 1 {
			t.Fatalf("invalid addr fixture for %s", remote.id)
		}
		if err := client.Connect(ctx, remote.addrs[0]); err != nil {
			t.Fatalf("connect %s: %v", remote.id, err)
		}
	}

	sy := NewSyncer(nil, nil, client)

	if got := sy.routing.OperationalState(healthy.ID()); got != PeerOperationalReady {
		t.Fatalf("healthy peer initial state=%s", got)
	}
	if got := sy.routing.OperationalState(slow.ID()); got != PeerOperationalReady {
		t.Fatalf("slow peer initial state=%s", got)
	}

	healthyStarted := time.Now()
	if err := sy.requestBlocks(healthy.ID(), 0); err != nil {
		t.Fatalf("healthy requestBlocks: %v", err)
	}
	healthyElapsed := time.Since(healthyStarted)

	if got := sy.routing.OperationalState(healthy.ID()); got != PeerOperationalReady {
		t.Fatalf("healthy peer unexpectedly degraded: %s", got)
	}

	for i := 0; i < 3; i++ {
		started := time.Now()

		if err := sy.requestBlocks(slow.ID(), 0); err != nil {
			t.Fatalf("slow requestBlocks iteration=%d: %v", i+1, err)
		}

		elapsed := time.Since(started)
		if elapsed <= 2*time.Second {
			t.Fatalf(
				"slow peer iteration=%d did not cross latency threshold: %s",
				i+1,
				elapsed,
			)
		}
	}

	if got := sy.routing.OperationalState(slow.ID()); got != PeerOperationalDegraded {
		t.Fatalf(
			"real Syncer feedback did not degrade slow peer: got=%s",
			got,
		)
	}

	ordered := sy.routing.Prefer([]peer.ID{
		slow.ID(),
		healthy.ID(),
	})

	if len(ordered) != 2 {
		t.Fatalf("routing removed peer: got=%d want=2", len(ordered))
	}

	if ordered[0] != healthy.ID() {
		t.Fatalf(
			"healthy peer not preferred after real feedback: first=%s healthy=%s",
			ordered[0],
			healthy.ID(),
		)
	}

	if ordered[1] != slow.ID() {
		t.Fatalf(
			"degraded peer not retained as fallback: second=%s slow=%s",
			ordered[1],
			slow.ID(),
		)
	}

	allDegraded := NewAdaptiveRoutingPolicy()
	for _, id := range []peer.ID{healthy.ID(), slow.ID()} {
		for i := 0; i < 3; i++ {
			allDegraded.ObserveFailure(id)
		}
	}

	fallback := allDegraded.Prefer([]peer.ID{
		healthy.ID(),
		slow.ID(),
	})

	if len(fallback) != 2 {
		t.Fatalf(
			"all-degraded fallback lost connectivity candidates: got=%d want=2",
			len(fallback),
		)
	}

	t.Logf(
		"REAL_LIBP2P_FEEDBACK_OK healthy_latency=%s slow_state=%s fallback_peers=%d",
		healthyElapsed,
		sy.routing.OperationalState(slow.ID()),
		len(fallback),
	)
}
