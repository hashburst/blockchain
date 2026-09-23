package blockchain

import (
	"fmt"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

const deliveryMaxPeers = 64
const deliveryMaxFrames = 128
const deliveryMaxBytes = 8 << 20

type deliveryQueue struct {
	frames [][]byte
	bytes  int
}

// One worker per destination; no network I/O while holding mu. Workers retire
// as soon as their queue drains. Limits include the in-flight frame.
type consensusDelivery struct {
	mu      sync.Mutex
	queues  map[peer.ID]*deliveryQueue
	send    func(peer.ID, []byte)
	dropped uint64
}

func (d *consensusDelivery) enqueue(id peer.ID, payload []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.queues == nil {
		d.queues = make(map[peer.ID]*deliveryQueue)
	}
	q, exists := d.queues[id]
	if (!exists && len(d.queues) >= deliveryMaxPeers) || len(payload) > deliveryMaxBytes {
		d.dropped++
		return fmt.Errorf("consensus delivery capacity exceeded for %s", id)
	}
	if !exists {
		q = &deliveryQueue{}
	}
	if len(q.frames) >= deliveryMaxFrames || q.bytes+len(payload) > deliveryMaxBytes {
		d.dropped++
		return fmt.Errorf("consensus peer queue full for %s", id)
	}
	// payload is the immutable copy owned by the outbound frame.
	q.frames = append(q.frames, payload)
	q.bytes += len(payload)
	if !exists {
		d.queues[id] = q
		go d.drain(id, q)
	}
	return nil
}

func (d *consensusDelivery) drain(id peer.ID, q *deliveryQueue) {
	for {
		d.mu.Lock()
		payload := q.frames[0]
		d.mu.Unlock()
		d.send(id, payload)
		d.mu.Lock()
		q.bytes -= len(payload)
		q.frames[0] = nil
		q.frames = q.frames[1:]
		if len(q.frames) == 0 {
			delete(d.queues, id)
			d.mu.Unlock()
			return
		}
		d.mu.Unlock()
	}
}

func (d *consensusDelivery) status() (int, uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, q := range d.queues {
		n += len(q.frames)
	}
	return n, d.dropped
}
