//go:build !linux && !darwin

package ledger

import (
	"errors"
	"os"
	"sync"
)

// Segment uses bounded ReadAt on platforms without the Unix mmap implementation.
// This fallback is deliberately not advertised as zero-copy.
type Segment struct {
	mu   sync.RWMutex
	file *os.File
	size int64
}

func OpenSegment(path string, maxBytes int64) (*Segment, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	st, e := f.Stat()
	if e != nil {
		f.Close()
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Size() <= 0 || st.Size() > maxBytes {
		f.Close()
		return nil, errors.New("invalid segment size/type")
	}
	return &Segment{file: f, size: st.Size()}, nil
}
func (s *Segment) View(offset int64, length int, fn func([]byte) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.file == nil {
		return errors.New("segment closed")
	}
	if offset < 0 || length < 0 || offset > s.size || int64(length) > s.size-offset {
		return errors.New("segment bounds")
	}
	b := make([]byte, length)
	if _, e := s.file.ReadAt(b, offset); e != nil {
		return e
	}
	return fn(b)
}
func (s *Segment) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	e := s.file.Close()
	s.file = nil
	return e
}
