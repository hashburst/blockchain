package ledger

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// LiveReader uses positional reads on growing files. Mapping a growing/truncated
// inode is deliberately avoided. The directory must be exclusively owned by one
// runtime; finalized records may only be appended, never overwritten or truncated.
// Cache limits cover encoded bytes, not decoded values, projections or process RSS.
type LiveReader struct {
	mu          sync.RWMutex
	data, index *os.File
	count, end  uint64
	format      PayloadFormat
	closed      bool
	cache       *Cache
	slots       chan struct{}
	scratch     sync.Pool
}

func OpenLiveReader(dir string, cacheBytes, cacheEntries int) (r *LiveReader, err error) {
	r = &LiveReader{cache: NewCache(cacheBytes, cacheEntries), slots: make(chan struct{}, 4)}
	for _, name := range []string{"blockchain.dat", "blockchain.idx"} {
		st, e := os.Lstat(filepath.Join(dir, name))
		if e != nil {
			return nil, e
		}
		if !st.Mode().IsRegular() {
			return nil, errors.New("ledger requires regular files")
		}
	}
	r.data, err = os.Open(filepath.Join(dir, "blockchain.dat"))
	if err != nil {
		return nil, err
	}
	r.index, err = os.Open(filepath.Join(dir, "blockchain.idx"))
	if err != nil {
		r.data.Close()
		return nil, err
	}
	defer func(owner *LiveReader) {
		if err != nil {
			owner.Close()
		}
	}(r)
	st, err := r.index.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() == 0 || st.Size()%IndexRecordSize != 0 {
		return nil, errors.New("empty/truncated index")
	}
	var magic [4]byte
	if _, err = r.data.ReadAt(magic[:], 0); err != nil {
		return nil, err
	}
	if string(magic[:]) == "HBX2" {
		r.format = BinaryPayload
	}
	count := uint64(st.Size() / IndexRecordSize)
	for n := uint64(0); n < count; n++ {
		if err = r.publish(n); err != nil {
			return nil, err
		}
	}
	ds, err := r.data.Stat()
	if err != nil {
		return nil, err
	}
	if uint64(ds.Size()) != r.end {
		return nil, errors.New("unindexed data tail")
	}
	return r, nil
}
func (r *LiveReader) Count() uint64 { r.mu.RLock(); defer r.mu.RUnlock(); return r.count }
func (r *LiveReader) record(n uint64) (IndexRecord, error) {
	var raw [IndexRecordSize]byte
	if n > uint64(^uint64(0)>>1)/IndexRecordSize {
		return IndexRecord{}, errors.New("index overflow")
	}
	if _, e := r.index.ReadAt(raw[:], int64(n*IndexRecordSize)); e != nil {
		return IndexRecord{}, e
	}
	rec, e := DecodeIndexRecord(raw[:])
	if e != nil {
		return rec, e
	}
	min, max := uint32(4), uint32(64<<20)
	if r.format == BinaryPayload {
		min, max = FrameHeader, MaxRecord+FrameHeader
	}
	if rec.BlockIndex != n || rec.BlockSize < min || rec.BlockSize > max || rec.FileOffset > uint64(^uint64(0)>>1)-uint64(rec.BlockSize) {
		return rec, errors.New("invalid index position/size")
	}
	return rec, nil
}
func (r *LiveReader) publish(n uint64) error {
	if n != r.count {
		return errors.New("nonsequential publication")
	}
	rec, e := r.record(n)
	if e != nil {
		return e
	}
	if rec.FileOffset != r.end {
		return errors.New("noncontiguous ledger")
	}
	ds, e := r.data.Stat()
	if e != nil {
		return e
	}
	end := rec.FileOffset + uint64(rec.BlockSize)
	if end > uint64(ds.Size()) {
		return io.ErrUnexpectedEOF
	}
	var h [FrameHeader]byte
	length := 4
	if r.format == BinaryPayload {
		length = FrameHeader
	}
	if _, e = r.data.ReadAt(h[:length], int64(rec.FileOffset)); e != nil {
		return e
	}
	if r.format == BinaryPayload {
		if string(h[:4]) != "HBX2" || binary.LittleEndian.Uint32(h[4:8]) != rec.BlockSize-FrameHeader || binary.LittleEndian.Uint32(h[12:]) != 0 {
			return errors.New("invalid binary frame header")
		}
	} else if string(h[:4]) == "HBX2" || binary.BigEndian.Uint32(h[:4]) != rec.BlockSize-4 {
		return errors.New("invalid gob frame header")
	}
	r.count++
	r.end = end
	return nil
}

// Publish is called only after the writer has synced both files. It validates
// the next record before making it visible; failure leaves the count unchanged.
func (r *LiveReader) Publish(n uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return os.ErrClosed
	}
	return r.publish(n)
}

// WithPayload lends bytes only during fn. Do not retain the slice or call
// Close/Publish/WithPayload recursively from fn. Cache storage never aliases it.
func (r *LiveReader) WithPayload(n uint64, fn func(PayloadFormat, []byte) error) error {
	if fn == nil {
		return errors.New("nil callback")
	}
	r.slots <- struct{}{}
	defer func() { <-r.slots }()
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return os.ErrClosed
	}
	if n >= r.count {
		return fmt.Errorf("block %d out of range", n)
	}
	var buf []byte
	if x := r.scratch.Get(); x != nil {
		buf = x.([]byte)
	}
	defer func() {
		if cap(buf) <= 64<<10 {
			r.scratch.Put(buf[:0])
		}
	}()
	if b, ok := r.cache.Get(n, buf); ok {
		buf = b
		return fn(r.format, b)
	}
	rec, e := r.record(n)
	if e != nil {
		return e
	}
	if rec.FileOffset+uint64(rec.BlockSize) > r.end {
		return errors.New("index outside published data")
	}
	if cap(buf) < int(rec.BlockSize) {
		buf = make([]byte, rec.BlockSize)
	} else {
		buf = buf[:rec.BlockSize]
	}
	if _, e = r.data.ReadAt(buf, int64(rec.FileOffset)); e != nil {
		return e
	}
	payload := buf[4:]
	if r.format == BinaryPayload {
		payload, e = FramePayload(buf)
		if e != nil {
			return e
		}
	} else if binary.BigEndian.Uint32(buf[:4]) != rec.BlockSize-4 {
		return errors.New("frame size mismatch")
	}
	r.cache.Put(n, payload)
	return fn(r.format, payload)
}
func (r *LiveReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return errors.Join(r.data.Close(), r.index.Close())
}

func (r *LiveReader) Format() PayloadFormat { return r.format }
