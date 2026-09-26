package blockchain

// canonical.go — buffer per la codifica canonica, usato da block.go e
// transaction.go. Sostituisce le funzioni libere putInt64/putString/putUint64
// che erano in hashing.go: stessa semantica, forma riusabile.
//
// REGOLE (invariate): interi big-endian a 8 byte, stringhe con prefisso di
// lunghezza a 8 byte. Il prefisso e' cio' che rende la codifica non ambigua —
// senza, "ab"+"c" e "a"+"bc" produrrebbero gli stessi byte.

import (
	"bytes"
	"encoding/binary"
)

type canonicalBuffer struct {
	buf *bytes.Buffer
}

func newCanonicalBuffer() *canonicalBuffer {
	return &canonicalBuffer{buf: new(bytes.Buffer)}
}

func (c *canonicalBuffer) putInt64(v int64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	c.buf.Write(b[:])
}

func (c *canonicalBuffer) putUint64(v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	c.buf.Write(b[:])
}

func (c *canonicalBuffer) putString(s string) {
	c.putUint64(uint64(len(s)))
	c.buf.WriteString(s)
}

func (c *canonicalBuffer) bytes() []byte {
	return c.buf.Bytes()
}
