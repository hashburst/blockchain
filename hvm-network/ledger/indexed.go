package ledger

import (
	"encoding/binary"
	"errors"
	"sync"
)

// IndexedReader opens one sealed HBX2 segment and its HBI2 index. Split long
// histories into bounded segments; the runtime should retain only a small set of
// these handles. Index: 16-byte header (HBI2, reserved=0, count uint64), then
// count entries (offset uint64, frame length uint32, reserved uint32=0).
type IndexedReader struct {
	mu          sync.RWMutex
	closed      bool
	slots       chan struct{}
	data, index *Segment
	count       uint64
	cache       *Cache
	scratch     sync.Pool
}

func OpenIndexed(dataPath, indexPath string, maxSegmentBytes int64, cacheBytes, cacheEntries int) (*IndexedReader, error) {
	d, e := OpenSegment(dataPath, maxSegmentBytes)
	if e != nil {
		return nil, e
	}
	i, e := OpenSegment(indexPath, maxSegmentBytes)
	if e != nil {
		d.Close()
		return nil, e
	}
	r := &IndexedReader{data: d, index: i, cache: NewCache(cacheBytes, cacheEntries), slots: make(chan struct{}, 8)}
	e = i.View(0, 16, func(b []byte) error {
		if string(b[:4]) != "HBI2" || binary.LittleEndian.Uint32(b[4:8]) != 0 {
			return errors.New("invalid index")
		}
		r.count = binary.LittleEndian.Uint64(b[8:])
		if r.count > uint64((maxSegmentBytes-16)/16) {
			return errors.New("index count exceeds budget")
		}
		return nil
	})
	if e != nil {
		r.Close()
		return nil, e
	}
	return r, nil
}

// WithPayload lazily reads a record by ordinal without materializing history.
// The callback lifetime and immutability contract is identical to Segment.View.
func (r *IndexedReader) WithPayload(ordinal uint64, fn func([]byte) error) error {
	r.slots <- struct{}{}
	defer func() { <-r.slots }()
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return errors.New("indexed reader closed")
	}
	if ordinal >= r.count {
		return errors.New("block ordinal out of bounds")
	}
	var scratch []byte
	if x := r.scratch.Get(); x != nil {
		scratch = x.([]byte)
	}
	defer func() {
		if cap(scratch) <= 64<<10 {
			r.scratch.Put(scratch[:0])
		}
	}()
	if b, ok := r.cache.Get(ordinal, scratch); ok {
		scratch = b
		return fn(b)
	}
	var offset uint64
	var length uint32
	if e := r.index.View(int64(16+ordinal*16), 16, func(b []byte) error {
		offset = binary.LittleEndian.Uint64(b)
		length = binary.LittleEndian.Uint32(b[8:])
		if offset > uint64(^uint64(0)>>1) || length > MaxRecord+FrameHeader || binary.LittleEndian.Uint32(b[12:]) != 0 {
			return errors.New("invalid index entry")
		}
		return nil
	}); e != nil {
		return e
	}
	return r.data.View(int64(offset), int(length), func(frame []byte) error {
		b, e := FramePayload(frame)
		if e != nil {
			return e
		}
		r.cache.Put(ordinal, b)
		return fn(b)
	})
}
func (r *IndexedReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	a := r.data.Close()
	b := r.index.Close()
	return errors.Join(a, b)
}
