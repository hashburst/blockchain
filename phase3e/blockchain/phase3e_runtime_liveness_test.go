package blockchain

import (
	"testing"
	"time"
)

func TestPhase3EReactorTryStatusDoesNotBlockOnConsensusMutex(t *testing.T) {
	r := &ConsensusReactor{}
	r.running = true
	r.runningState.Store(true)

	r.mu.Lock()
	start := time.Now()
	status, fresh := r.TryStatus()
	elapsed := time.Since(start)
	r.mu.Unlock()

	if fresh {
		t.Fatal("TryStatus unexpectedly acquired a deliberately held consensus mutex")
	}
	if !status.Running {
		t.Fatal("TryStatus lost the lock-free reactor running state")
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("TryStatus blocked control-plane status for %s", elapsed)
	}
}

func TestPhase3EConsensusOutboundQueueCopiesWithoutInlineIO(t *testing.T) {
	n := &Libp2pConsensusNetwork{outbound: make(chan consensusOutboundFrame, 1)}
	payload := []byte("phase3e-consensus-frame")

	start := time.Now()
	n.queueRelay(payload, "")
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("queueRelay blocked for %s", elapsed)
	}

	payload[0] = 'X'
	select {
	case frame := <-n.outbound:
		if string(frame.payload) != "phase3e-consensus-frame" {
			t.Fatalf("queued payload was not isolated from caller mutation: %q", frame.payload)
		}
	default:
		t.Fatal("consensus frame was not queued")
	}
}
