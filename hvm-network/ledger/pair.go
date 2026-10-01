package ledger

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const IndexRecordSize = 20

type PayloadFormat uint8

const (
	GobPayload    PayloadFormat = iota // existing uint32 BE length + gob payload
	BinaryPayload                      // HBX2 frame containing the manual HBB2 Block codec
)

// IndexRecord is the EXISTING HVM on-disk index layout, big endian, no padding.
// Size includes the entire data frame, including its length/header.
type IndexRecord struct {
	BlockIndex uint64
	FileOffset uint64
	BlockSize  uint32
}

func (r IndexRecord) Encode() [IndexRecordSize]byte {
	var b [IndexRecordSize]byte
	binary.BigEndian.PutUint64(b[0:8], r.BlockIndex)
	binary.BigEndian.PutUint64(b[8:16], r.FileOffset)
	binary.BigEndian.PutUint32(b[16:20], r.BlockSize)
	return b
}
func DecodeIndexRecord(b []byte) (IndexRecord, error) {
	if len(b) != IndexRecordSize {
		return IndexRecord{}, errors.New("index record size")
	}
	return IndexRecord{binary.BigEndian.Uint64(b[:8]), binary.BigEndian.Uint64(b[8:16]), binary.BigEndian.Uint32(b[16:20])}, nil
}

// PairReader reads an exclusively sealed generation. No process may truncate or
// overwrite either mapped inode. Appending/live remapping is NOT supported here.
// maxMapBytes bounds virtual mappings, not resident memory or the OS page cache.
type PairReader struct {
	mu          sync.RWMutex
	closed      bool
	data, index *Segment
	count       uint64
	dataSize    int64
	format      PayloadFormat
	cache       *Cache
	slots       chan struct{}
	scratch     sync.Pool
}

func OpenPair(dir string, format PayloadFormat, maxMapBytes int64, cacheBytes, cacheEntries int) (*PairReader, error) {
	if format != GobPayload && format != BinaryPayload {
		return nil, errors.New("explicit known payload format required")
	}
	if maxMapBytes <= 0 {
		return nil, errors.New("invalid mapping budget")
	}
	ds, e := os.Lstat(filepath.Join(dir, "blockchain.dat"))
	if e != nil {
		return nil, e
	}
	is, e := os.Lstat(filepath.Join(dir, "blockchain.idx"))
	if e != nil {
		return nil, e
	}
	if !ds.Mode().IsRegular() || !is.Mode().IsRegular() || ds.Size() <= 0 || is.Size() <= 0 || is.Size()%IndexRecordSize != 0 || ds.Size() > maxMapBytes || is.Size() > maxMapBytes-ds.Size() {
		return nil, errors.New("invalid pair sizes/type/budget")
	}
	d, e := OpenSegment(filepath.Join(dir, "blockchain.dat"), ds.Size())
	if e != nil {
		return nil, e
	}
	i, e := OpenSegment(filepath.Join(dir, "blockchain.idx"), is.Size())
	if e != nil {
		d.Close()
		return nil, e
	}
	return &PairReader{data: d, index: i, count: uint64(is.Size() / IndexRecordSize), dataSize: ds.Size(), format: format, cache: NewCache(cacheBytes, cacheEntries), slots: make(chan struct{}, 8)}, nil
}
func (r *PairReader) Count() uint64 { return r.count }
func (r *PairReader) record(n uint64) (rec IndexRecord, err error) {
	if n >= r.count {
		return rec, errors.New("block index out of range")
	}
	err = r.index.View(int64(n*IndexRecordSize), IndexRecordSize, func(b []byte) error {
		var e error
		rec, e = DecodeIndexRecord(b)
		if e != nil {
			return e
		}
		min := uint32(4)
		max := uint32(MaxRecord + 4)
		if r.format == BinaryPayload {
			min = FrameHeader
			max = MaxRecord + FrameHeader
		}
		if rec.BlockIndex != n || rec.BlockSize < min || rec.BlockSize > max || rec.FileOffset > uint64(r.dataSize) || uint64(rec.BlockSize) > uint64(r.dataSize)-rec.FileOffset {
			return errors.New("invalid index position/size")
		}
		return nil
	})
	return
}

// ValidateLayout scans fixed index records without retaining history. Full payload
// authentication, consensus and economic verification belong to the runtime.
func (r *PairReader) ValidateLayout() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return errors.New("pair closed")
	}
	var end uint64
	for n := uint64(0); n < r.count; n++ {
		rec, e := r.record(n)
		if e != nil {
			return e
		}
		if rec.FileOffset != end {
			return fmt.Errorf("non-contiguous index %d", n)
		}
		end += uint64(rec.BlockSize)
	}
	if end != uint64(r.dataSize) {
		return errors.New("unindexed data tail")
	}
	return nil
}

// WithPayload lends immutable bytes only within fn. Mmap misses lend directly;
// cache hits copy to owned scratch. This is not zero-copy decoding of a Go Block.
// Never retain/mutate the slice, call Close or recursively read inside fn.
func (r *PairReader) WithPayload(n uint64, fn func([]byte) error) error {
	if fn == nil {
		return errors.New("nil callback")
	}
	r.slots <- struct{}{}
	defer func() { <-r.slots }()
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return errors.New("pair closed")
	}
	rec, e := r.record(n)
	if e != nil {
		return e
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
	if p, ok := r.cache.Get(n, scratch); ok {
		scratch = p
		return fn(p)
	}
	return r.data.View(int64(rec.FileOffset), int(rec.BlockSize), func(frame []byte) error {
		var p []byte
		if r.format == BinaryPayload {
			var err error
			p, err = FramePayload(frame)
			if err != nil {
				return err
			}
		} else {
			if uint64(binary.BigEndian.Uint32(frame[:4]))+4 != uint64(len(frame)) {
				return errors.New("gob frame length differs from index")
			}
			p = frame[4:]
		}
		r.cache.Put(n, p)
		return fn(p)
	})
}
func (r *PairReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return errors.Join(r.data.Close(), r.index.Close())
}
