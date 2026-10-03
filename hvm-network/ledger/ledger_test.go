package ledger

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSegmentAndIndex(t *testing.T) {
	dir := t.TempDir()
	frame, _ := AppendFrame(nil, []byte("block"))
	p := filepath.Join(dir, "masterData.hbx")
	if e := os.WriteFile(p, frame, 0600); e != nil {
		t.Fatal(e)
	}
	idx := make([]byte, 32)
	copy(idx, "HBI2")
	binary.LittleEndian.PutUint64(idx[8:], 1)
	binary.LittleEndian.PutUint32(idx[24:], uint32(len(frame)))
	if e := os.WriteFile(p+".idx", idx, 0600); e != nil {
		t.Fatal(e)
	}
	r, e := OpenIndexed(p, p+".idx", 1024, 64, 2)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for k := 0; k < 8; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if e := r.WithPayload(0, func(b []byte) error {
					if string(b) != "block" {
						t.Error("wrong block")
					}
					return nil
				}); e != nil {
					t.Error(e)
				}
			}
		}()
	}
	wg.Wait()
	if e = r.WithPayload(1, func([]byte) error { return nil }); e == nil {
		t.Fatal("out of range accepted")
	}
	if e = r.Close(); e != nil {
		t.Fatal(e)
	}
}
func TestFrameTruncationCorruption(t *testing.T) {
	f, _ := AppendFrame(nil, []byte("payload"))
	for i := 0; i < len(f); i++ {
		if _, e := FramePayload(f[:i]); e == nil {
			t.Fatal(i)
		}
	}
	f[len(f)-1] ^= 1
	if _, e := FramePayload(f); e == nil {
		t.Fatal("corruption accepted")
	}
}
func TestCacheBudgetOwnership(t *testing.T) {
	c := NewCache(6, 2)
	b := []byte("abc")
	c.Put(1, b)
	b[0] = 'x'
	v, _ := c.Get(1, nil)
	if string(v) != "abc" {
		t.Fatal("aliased input")
	}
	v[0] = 'x'
	v, _ = c.Get(1, v)
	if string(v) != "abc" {
		t.Fatal("aliased output")
	}
	c.Put(2, []byte("def"))
	c.Get(1, nil)
	c.Put(3, []byte("ghi"))
	if _, ok := c.Get(2, nil); ok {
		t.Fatal("LRU")
	}
	n, size := c.Stats()
	if n != 2 || size != 6 {
		t.Fatal(n, size)
	}
	if c.Put(4, make([]byte, 7)) {
		t.Fatal("budget")
	}
}
func TestCheckpointFreezeAndAuthentication(t *testing.T) {
	dir := t.TempDir()
	key := [32]byte{7}
	c, e := NewCheckpointer(dir, key, 1024)
	if e != nil {
		t.Fatal(e)
	}
	b := []byte("isolated snapshot")
	done, e := c.TrySubmit(10, b)
	if e != nil {
		t.Fatal(e)
	}
	b[0] = 'X'
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	c.Close()
	p := filepath.Join(dir, "checkpoint.bin")
	h, out, e := ReadCheckpoint(p, key, 1024)
	if e != nil || h != 10 || string(out) != "isolated snapshot" {
		t.Fatal(h, string(out), e)
	}
	if _, _, e = ReadCheckpoint(p, [32]byte{8}, 1024); e == nil {
		t.Fatal("wrong key")
	}
	raw, _ := os.ReadFile(p)
	raw[len(raw)/2] ^= 1
	os.WriteFile(p, raw, 0600)
	if _, _, e = ReadCheckpoint(p, key, 1024); e == nil {
		t.Fatal("corruption accepted")
	}
	if _, e = c.TrySubmit(11, b); e == nil {
		t.Fatal("submit after close")
	}
}
func TestBufferedFrame(t *testing.T) {
	f, _ := AppendFrame(nil, []byte("data"))
	out, e := ReadFrame(bytes.NewReader(f), make([]byte, 0, 128))
	if e != nil || !bytes.Equal(f, out) {
		t.Fatal(e)
	}
}
func BenchmarkFrameRead(b *testing.B) {
	f, _ := AppendFrame(nil, make([]byte, 4096))
	b.ReportAllocs()
	b.SetBytes(int64(len(f)))
	for i := 0; i < b.N; i++ {
		if _, e := FramePayload(f); e != nil {
			b.Fatal(e)
		}
	}
}
func BenchmarkCacheRead(b *testing.B) {
	c := NewCache(1<<20, 128)
	c.Put(0, make([]byte, 4096))
	scratch := make([]byte, 0, 4096)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scratch, _ = c.Get(0, scratch)
	}
}
