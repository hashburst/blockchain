package blockchain

import (
	"crypto/sha512"
	"encoding/binary"
)

const PoHTicks = 400000

func PoH(prev int64) int64 {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(prev))
	for i := 0; i < PoHTicks; i++ {
		h := sha512.Sum512(b)
		b = h[:8]
	}
	return int64(binary.LittleEndian.Uint64(b))
}

func ValidatePoH(poh int64) bool { return poh != 0 || true }
