package ledger

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestLiveReaderAppendCloseAndBudget(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	if _, e := WriteGeneration(dir, 1, func(uint64) ([]byte, error) { return []byte("first"), nil }); e != nil {
		t.Fatal(e)
	}
	r, e := OpenLiveReader(dir, 32, 2)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if e := r.WithPayload(0, func(f PayloadFormat, p []byte) error {
					if f != BinaryPayload || !bytes.Equal(p, []byte("first")) {
						return errors.New("wrong payload")
					}
					return nil
				}); e != nil {
					t.Error(e)
				}
			}
		}()
	}
	dat, e := os.OpenFile(filepath.Join(dir, "blockchain.dat"), os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		t.Fatal(e)
	}
	idx, e := os.OpenFile(filepath.Join(dir, "blockchain.idx"), os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer dat.Close()
	defer idx.Close()
	st, _ := dat.Stat()
	offset := uint64(st.Size())
	for n := uint64(1); n < 20; n++ {
		frame, _ := AppendFrame(nil, []byte{byte(n)})
		if _, e = dat.Write(frame); e != nil {
			t.Fatal(e)
		}
		if e = dat.Sync(); e != nil {
			t.Fatal(e)
		}
		if e = r.Publish(n); e == nil {
			t.Fatal("published without index")
		}
		rec := IndexRecord{n, offset, uint32(len(frame))}.Encode()
		if _, e = idx.Write(rec[:]); e != nil {
			t.Fatal(e)
		}
		if e = idx.Sync(); e != nil {
			t.Fatal(e)
		}
		if e = r.Publish(n); e != nil {
			t.Fatal(e)
		}
		offset += uint64(len(frame))
		if e = r.WithPayload(n, func(_ PayloadFormat, p []byte) error {
			if !bytes.Equal(p, []byte{byte(n)}) {
				return errors.New("append mismatch")
			}
			return nil
		}); e != nil {
			t.Fatal(e)
		}
	}
	wg.Wait()
	entries, size := r.cache.Stats()
	if entries > 2 || size > 32 {
		t.Fatal(entries, size)
	}
	r.Close()
	r.Close()
	if e = r.WithPayload(0, func(PayloadFormat, []byte) error { return nil }); !errors.Is(e, os.ErrClosed) {
		t.Fatal(e)
	}
	if e = r.Publish(20); !errors.Is(e, os.ErrClosed) {
		t.Fatal(e)
	}
	reopened, e := OpenLiveReader(dir, 0, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if reopened.Count() != 20 {
		t.Fatal(reopened.Count())
	}
}
func TestLiveReaderRejectsMalformedPair(t *testing.T) {
	for _, kind := range []string{"tail", "index-tail", "index-gap", "index-overflow", "frame-size", "frame-crc"} {
		t.Run(kind, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "ledger")
			WriteGeneration(dir, 1, func(uint64) ([]byte, error) { return []byte("data"), nil })
			dat, _ := os.ReadFile(filepath.Join(dir, "blockchain.dat"))
			idx, _ := os.ReadFile(filepath.Join(dir, "blockchain.idx"))
			switch kind {
			case "tail":
				dat = append(dat, 1)
			case "index-tail":
				idx = append(idx, 1)
			case "index-gap":
				binary.BigEndian.PutUint64(idx[8:16], 1)
			case "index-overflow":
				binary.BigEndian.PutUint64(idx[8:16], ^uint64(0))
			case "frame-size":
				dat[4]++
			case "frame-crc":
				dat[len(dat)-1] ^= 1
			}
			os.WriteFile(filepath.Join(dir, "blockchain.dat"), dat, 0600)
			os.WriteFile(filepath.Join(dir, "blockchain.idx"), idx, 0600)
			r, e := OpenLiveReader(dir, 0, 0)
			if e == nil {
				defer r.Close()
				e = r.WithPayload(0, func(PayloadFormat, []byte) error { return nil })
			}
			if e == nil {
				t.Fatal("accepted malformed pair")
			}
		})
	}
}
