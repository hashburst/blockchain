package ledger

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

// ReadCheckpoint authenticates before decompression and bounds expanded bytes.
// The payload must additionally bind chain ID, configuration, finalized block and
// state roots, and node identity. This local HMAC is not peer bootstrap authority.
func ReadCheckpoint(path string, key [32]byte, maxBytes int) (uint64, []byte, error) {
	if maxBytes <= 0 || maxBytes > 64<<20 {
		return 0, nil, errors.New("invalid budget")
	}
	f, e := os.Open(path)
	if e != nil {
		return 0, nil, e
	}
	defer f.Close()
	// Allocate once from bounded file size instead of geometric ReadAll growth.
	st, e := f.Stat()
	if e != nil {
		return 0, nil, e
	}
	if !st.Mode().IsRegular() || st.Size() < 52 || st.Size() > int64(maxBytes)+(1<<20)+52 {
		return 0, nil, errors.New("invalid checkpoint file size/type")
	}
	data := make([]byte, int(st.Size()))
	_, e = io.ReadFull(f, data)
	if e != nil {
		return 0, nil, e
	}
	var extra [1]byte
	if n, err := f.Read(extra[:]); n != 0 || err != io.EOF {
		return 0, nil, errors.New("checkpoint changed while reading")
	}
	if len(data) < 52 || len(data) > maxBytes+(1<<20)+52 || string(data[:4]) != "HBC2" {
		return 0, nil, errors.New("invalid checkpoint frame")
	}
	body, tag := data[:len(data)-32], data[len(data)-32:]
	m := hmac.New(sha256.New, key[:])
	m.Write(body)
	if !hmac.Equal(m.Sum(nil), tag) {
		return 0, nil, errors.New("checkpoint authentication failed")
	}
	n := binary.LittleEndian.Uint64(body[12:20])
	if n > uint64(maxBytes) {
		return 0, nil, errors.New("checkpoint expansion exceeds budget")
	}
	z, e := gzip.NewReader(bytes.NewReader(body[20:]))
	if e != nil {
		return 0, nil, e
	}
	defer z.Close()
	// HMAC and authenticated expansion bound have already been checked.
	b := make([]byte, int(n))
	_, e = io.ReadFull(z, b)
	if e != nil {
		return 0, nil, e
	}
	// Read through the gzip trailer even when the declared size is exact.
	// This validates its CRC and rejects extra uncompressed bytes/members.
	if count, err := z.Read(extra[:]); count != 0 || err != io.EOF {
		return 0, nil, errors.New("checkpoint size mismatch")
	}
	return binary.LittleEndian.Uint64(body[4:12]), b, nil
}
