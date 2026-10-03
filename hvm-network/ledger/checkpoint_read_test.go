package ledger

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpointExactRead(t *testing.T) {
	key := [32]byte{1}
	dir := t.TempDir()
	path := filepath.Join(dir, "checkpoint.bin")
	build := func(payload []byte, declared uint64, badCRC bool) []byte {
		var raw bytes.Buffer
		header := make([]byte, 20)
		copy(header, "HBC2")
		binary.LittleEndian.PutUint64(header[12:], declared)
		raw.Write(header)
		z := gzip.NewWriter(&raw)
		z.Write(payload)
		z.Close()
		b := raw.Bytes()
		if badCRC {
			b[len(b)-8] ^= 1
		}
		mac := hmac.New(sha256.New, key[:])
		mac.Write(b)
		return append(b, mac.Sum(nil)...)
	}
	for _, tt := range []struct {
		payload         []byte
		n               uint64
		badCRC, wantErr bool
	}{
		{nil, 0, false, false}, {[]byte("abc"), 3, false, false}, {[]byte("abc"), 2, false, true},
		{[]byte("abc"), 4, false, true}, {[]byte("abc"), 3, true, true}, {[]byte("abc"), 0, false, true},
	} {
		raw := build(tt.payload, tt.n, tt.badCRC)
		if e := os.WriteFile(path, raw, 0600); e != nil {
			t.Fatal(e)
		}
		_, got, e := ReadCheckpoint(path, key, 1024)
		if (e != nil) != tt.wantErr || (!tt.wantErr && !bytes.Equal(got, tt.payload)) {
			t.Fatalf("%+v got %q %v", tt, got, e)
		}
	}
	raw := build([]byte("abc"), 3, false)
	for i := 0; i < len(raw); i++ {
		os.WriteFile(path, raw[:i], 0600)
		if _, _, e := ReadCheckpoint(path, key, 1024); e == nil {
			t.Fatalf("accepted truncation %d", i)
		}
	}
}
func BenchmarkCheckpointRead4MiB(b *testing.B) {
	payload := make([]byte, 4<<20)
	x := uint64(7)
	for i := range payload {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		payload[i] = byte(x)
	}
	dir := b.TempDir()
	key := [32]byte{7}
	c, e := NewCheckpointer(dir, key, len(payload))
	if e != nil {
		b.Fatal(e)
	}
	done, e := c.TrySubmit(1, payload)
	if e != nil {
		b.Fatal(e)
	}
	if e = <-done; e != nil {
		b.Fatal(e)
	}
	c.Close()
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, out, e := ReadCheckpoint(filepath.Join(dir, "checkpoint.bin"), key, len(payload))
		if e != nil || len(out) != len(payload) {
			b.Fatal(e)
		}
	}
}
