package ledger

import (
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var ErrBusy = errors.New("checkpoint queue full; optional checkpoint skipped")

type checkpointJob struct {
	height  uint64
	payload []byte
	done    chan error
}

// Checkpointer writes derived local caches only. Consensus blocks and signature
// journals must already be durable. It is NOT a transaction WAL.
// One writer per directory; one active plus one pending snapshot bounds retained
// payload to 2*maxBytes. Temporary compression buffers are additional.
type Checkpointer struct {
	mu        sync.Mutex
	jobs      chan checkpointJob
	stopped   chan struct{}
	closed    bool
	dir       string
	key       [32]byte
	maxBytes  int
	last      uint64
	submitted bool
}

func NewCheckpointer(dir string, key [32]byte, maxBytes int) (*Checkpointer, error) {
	if maxBytes <= 0 || maxBytes > 64<<20 {
		return nil, errors.New("invalid checkpoint budget")
	}
	st, e := os.Stat(dir)
	if e != nil {
		return nil, e
	}
	if !st.IsDir() {
		return nil, errors.New("checkpoint directory required")
	}
	c := &Checkpointer{dir: dir, key: key, maxBytes: maxBytes, jobs: make(chan checkpointJob, 1), stopped: make(chan struct{})}
	go c.run()
	return c, nil
}

// TrySubmit freezes already-encoded state by copying it. Caller must create that
// state under its own snapshot/COW discipline; a concurrent live map is not safe.
// No disk I/O is done here, but copying costs O(len(payload)): not zero latency.
// ErrBusy is safe to ignore only for this optional cache, never for transactions.
func (c *Checkpointer) TrySubmit(height uint64, payload []byte) (<-chan error, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, errors.New("checkpointer closed")
	}
	if len(payload) > c.maxBytes {
		return nil, errors.New("checkpoint too large")
	}
	if c.submitted && height <= c.last {
		return nil, errors.New("checkpoint height must increase")
	}
	if len(c.jobs) == cap(c.jobs) {
		return nil, ErrBusy
	}
	j := checkpointJob{height: height, payload: append([]byte(nil), payload...), done: make(chan error, 1)}
	c.jobs <- j
	c.last = height
	c.submitted = true
	return j.done, nil
}
func (c *Checkpointer) run() {
	defer close(c.stopped)
	for j := range c.jobs {
		j.done <- c.write(j)
		close(j.done)
	}
}

// Close drains accepted work. Each returned completion channel carries its write
// error; callers must check those errors, including directory fsync failures.
func (c *Checkpointer) Close() {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.jobs)
	}
	c.mu.Unlock()
	<-c.stopped
}
func (c *Checkpointer) write(j checkpointJob) (err error) {
	f, e := os.CreateTemp(c.dir, ".checkpoint-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer func() { f.Close(); os.Remove(name) }()
	// HMAC covers header and compressed stream; the final 32 bytes hold its tag.
	mac := hmac.New(sha256.New, c.key[:])
	w := io.MultiWriter(f, mac)
	var header [20]byte
	copy(header[:], "HBC2")
	binary.LittleEndian.PutUint64(header[4:], j.height)
	binary.LittleEndian.PutUint64(header[12:], uint64(len(j.payload)))
	if _, e = w.Write(header[:]); e != nil {
		return e
	}
	z, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
	if _, e = z.Write(j.payload); e != nil {
		z.Close()
		return e
	}
	if e = z.Close(); e != nil {
		return e
	}
	if _, e = f.Write(mac.Sum(nil)); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	// Atomic inode replacement; never overwrite an inode held by an mmap reader.
	if e = os.Rename(name, filepath.Join(c.dir, "checkpoint.bin")); e != nil {
		return e
	}
	d, e := os.Open(c.dir)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
