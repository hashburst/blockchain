//go:build linux || darwin

package ledger

import (
	"errors"
	"os"
	"sync"
	"syscall"
)

// Segment maps an immutable, sealed file. Never truncate or overwrite a mapped
// inode; publish replacements by rename. File permissions are not a trust proof.
type Segment struct {
	mu     sync.RWMutex
	data   []byte
	closed bool
}

// OpenSegment bounds each mapping; use fixed-size sealed segments for large ledgers.
func OpenSegment(path string, maxBytes int64) (*Segment, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Size() <= 0 || st.Size() > maxBytes || uint64(st.Size()) > uint64(^uint(0)>>1) {
		return nil, errors.New("invalid segment size/type")
	}
	b, e := syscall.Mmap(int(f.Fd()), 0, int(st.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if e != nil {
		return nil, e
	}
	return &Segment{data: b}, nil
}

// View lends zero-copy encoded bytes only for fn's duration. fn must not mutate,
// retain, or pass the slice to another goroutine, or call Close. Decode into owned
// values before returning. Close waits for current views; concurrent views are safe.
func (s *Segment) View(offset int64, length int, fn func([]byte) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return errors.New("segment closed")
	}
	if offset < 0 || length < 0 || offset > int64(len(s.data)) || int64(length) > int64(len(s.data))-offset {
		return errors.New("segment bounds")
	}
	return fn(s.data[int(offset) : int(offset)+length : int(offset)+length])
}
func (s *Segment) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	if e := syscall.Munmap(s.data); e != nil {
		return e
	}
	s.closed = true
	s.data = nil
	return nil
}
