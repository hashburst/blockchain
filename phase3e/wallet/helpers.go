package wallet

// helpers.go — utilità esposte ad altri package (blockchain), per non
// duplicare né la codifica né le conversioni esadecimali.

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"strings"
)

// RandomInt63 restituisce un int64 positivo casuale (nonce delle transazioni).
func RandomInt63() int64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return int64(binary.BigEndian.Uint64(b[:]) >> 1)
}

// HexToBytes decodifica hex con o senza prefisso 0x.
func HexToBytes(s string) ([]byte, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "0x")
	return hex.DecodeString(s)
}

// AddressEqual confronta due indirizzi ignorando 0x e maiuscole/minuscole.
// Il checksum EIP-55 è solo un rilevatore di errori di battitura: due scritture
// dello stesso indirizzo con case diverso restano lo stesso indirizzo.
func AddressEqual(a, b string) bool {
	na := strings.ToLower(strings.TrimPrefix(a, "0x"))
	nb := strings.ToLower(strings.TrimPrefix(b, "0x"))
	return na == nb
}
