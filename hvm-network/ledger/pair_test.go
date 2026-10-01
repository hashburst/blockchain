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

func TestPairGeneration(t *testing.T) {
	parent := t.TempDir()
	os.Chmod(parent, 0700)
	dst := filepath.Join(parent, "generation")
	proof, e := WriteGeneration(dst, 3, func(n uint64) ([]byte, error) { return []byte{byte(n), 8, 9}, nil })
	if e != nil || proof.Count != 3 {
		t.Fatal(proof, e)
	}
	r, e := OpenPair(dst, BinaryPayload, 1<<20, 32, 2)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.ValidateLayout(); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := uint64(0); n < 3; n++ {
				if e := r.WithPayload(n, func(p []byte) error {
					if !bytes.Equal(p, []byte{byte(n), 8, 9}) {
						return errors.New("payload mismatch")
					}
					return nil
				}); e != nil {
					t.Error(e)
				}
			}
		}()
	}
	wg.Wait()
	r.Close()
	if e = r.WithPayload(0, func([]byte) error { return nil }); e == nil {
		t.Fatal("read after close")
	}
	if _, e = WriteGeneration(dst, 1, func(uint64) ([]byte, error) { return nil, nil }); e == nil {
		t.Fatal("overwrite accepted")
	}
}
func TestPairIndexAndFrames(t *testing.T) {
	for _, format := range []PayloadFormat{GobPayload, BinaryPayload} {
		dir := t.TempDir()
		raw := []byte("payload")
		var frame []byte
		if format == GobPayload {
			frame = make([]byte, 4)
			binary.BigEndian.PutUint32(frame, 7)
			frame = append(frame, raw...)
		} else {
			frame, _ = AppendFrame(nil, raw)
		}
		idx := IndexRecord{0, 0, uint32(len(frame))}.Encode()
		os.WriteFile(filepath.Join(dir, "blockchain.dat"), frame, 0600)
		os.WriteFile(filepath.Join(dir, "blockchain.idx"), idx[:], 0600)
		r, e := OpenPair(dir, format, 1<<20, 0, 0)
		if e != nil {
			t.Fatal(e)
		}
		if e = r.ValidateLayout(); e != nil {
			t.Fatal(e)
		}
		if e = r.WithPayload(0, func(p []byte) error {
			if !bytes.Equal(p, raw) {
				return errors.New("payload")
			}
			return nil
		}); e != nil {
			t.Fatal(e)
		}
		r.Close()
		for _, bad := range []IndexRecord{{1, 0, uint32(len(frame))}, {0, 1, uint32(len(frame))}, {0, ^uint64(0), 5}, {0, 0, 0}} {
			b := bad.Encode()
			os.WriteFile(filepath.Join(dir, "blockchain.idx"), b[:], 0600)
			r, e = OpenPair(dir, format, 1<<20, 0, 0)
			if e != nil {
				t.Fatal(e)
			}
			if e = r.ValidateLayout(); e == nil {
				t.Fatal("bad layout accepted", bad)
			}
			r.Close()
		}
		os.WriteFile(filepath.Join(dir, "blockchain.idx"), idx[:19], 0600)
		if r, e = OpenPair(dir, format, 1<<20, 0, 0); e == nil {
			r.Close()
			t.Fatal("truncated idx accepted")
		}
	}
}
func TestGenerationFailureDoesNotPublish(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	dst := filepath.Join(dir, "out")
	_, e := WriteGeneration(dst, 3, func(n uint64) ([]byte, error) {
		if n == 1 {
			return nil, errors.New("validation failed")
		}
		return []byte{1}, nil
	})
	if e == nil {
		t.Fatal("failure ignored")
	}
	if _, e = os.Stat(dst); !os.IsNotExist(e) {
		t.Fatal("partial generation published")
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".hvm-generation-*"))
	if len(matches) != 0 {
		t.Fatal("temporary generation retained", matches)
	}
}
func TestPairCorruptionRejected(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	dst := filepath.Join(dir, "out")
	if _, e := WriteGeneration(dst, 1, func(uint64) ([]byte, error) { return []byte("payload"), nil }); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(dst, "blockchain.dat")
	b, _ := os.ReadFile(p)
	b[len(b)-1] ^= 1
	os.WriteFile(p, b, 0600)
	r, e := OpenPair(dst, BinaryPayload, 1<<20, 0, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	if e = r.WithPayload(0, func([]byte) error { return nil }); e == nil {
		t.Fatal("CRC corruption accepted")
	}
}

// The existing gob storage permits larger frames than the new 16 MiB codec.
func TestPairLegacyFrameBudget(t *testing.T) {
	dir := t.TempDir()
	f, e := os.Create(filepath.Join(dir, "blockchain.dat"))
	if e != nil {
		t.Fatal(e)
	}
	size := uint32(17 << 20)
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], size-4)
	if _, e = f.Write(h[:]); e != nil {
		t.Fatal(e)
	}
	if e = f.Truncate(int64(size)); e != nil {
		t.Fatal(e)
	}
	f.Close()
	idx := IndexRecord{0, 0, size}.Encode()
	os.WriteFile(filepath.Join(dir, "blockchain.idx"), idx[:], 0600)
	r, e := OpenPair(dir, GobPayload, 32<<20, 0, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	if e = r.ValidateLayout(); e != nil {
		t.Fatal(e)
	}
	if e = r.WithPayload(0, func(b []byte) error {
		if len(b) != int(size-4) {
			return errors.New("size")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
}
