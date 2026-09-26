package blockchain

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"hashburst/consensus"
	"io"
	"net"
	"testing"
	"time"
)

func TestDeliverySlowPeerDoesNotBlockHealthyPeer(t *testing.T) {
	blocked := make(chan struct{})
	entered := make(chan struct{}, 1)
	healthy := make(chan string, 4)
	slow := make(chan string, 4)
	d := &consensusDelivery{send: func(id peer.ID, p []byte) {
		if id == "slow" {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-blocked
			slow <- string(p)
		} else {
			healthy <- string(p)
		}
	}}
	defer close(blocked)
	if err := d.enqueue("slow", []byte("first")); err != nil {
		t.Fatal(err)
	}
	<-entered
	for _, msg := range []string{"first", "second", "third"} {
		if err := d.enqueue("healthy", []byte(msg)); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{"first", "second", "third"} {
		select {
		case got := <-healthy:
			if got != want {
				t.Fatalf("order %s != %s", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("healthy destination blocked behind slow peer")
		}
	}
}

func TestDeliveryBoundedQueueAndRecovery(t *testing.T) {
	release := make(chan struct{})
	done := make(chan struct{}, deliveryMaxFrames+1)
	d := &consensusDelivery{send: func(peer.ID, []byte) { <-release; done <- struct{}{} }}
	for i := 0; i < deliveryMaxFrames; i++ {
		if err := d.enqueue("slow", []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.enqueue("slow", []byte("overflow")); err == nil {
		t.Fatal("unbounded peer queue")
	}
	pending, dropped := d.status()
	if pending != deliveryMaxFrames || dropped != 1 {
		t.Fatal(pending, dropped)
	}
	close(release)
	for i := 0; i < deliveryMaxFrames; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("worker stuck")
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		pending, _ = d.status()
		if pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker retained")
		}
		time.Sleep(time.Millisecond)
	}
	if err := d.enqueue("slow", []byte("reconnected")); err != nil {
		t.Fatal(err)
	}
	<-done
}

type deadlinePipe struct {
	net.Conn
	reset bool
}

func (p *deadlinePipe) Reset() error { p.reset = true; return p.Conn.Close() }
func TestDeliveryBlockedWriteHasDeadline(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	s := &deadlinePipe{Conn: a}
	n := &Libp2pConsensusNetwork{cfg: consensus.DefaultNetworkConfig()}
	start := time.Now()
	err := n.sendOnStream(s, []byte("frame"), start.Add(30*time.Millisecond))
	if err == nil || !s.reset {
		t.Fatal("blocked stream was not reset", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("write deadline ineffective")
	}
}

type shortConsensusWriter struct{}

func (shortConsensusWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestDeliveryRejectsShortWrite(t *testing.T) {
	n := &Libp2pConsensusNetwork{cfg: consensus.DefaultNetworkConfig()}
	if err := n.writePayload(shortConsensusWriter{}, []byte("x")); err != io.ErrShortWrite {
		t.Fatal(err)
	}
}
