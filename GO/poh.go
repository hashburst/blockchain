package blockchain

import (
	"math/rand"
	"time"
)

// PoH generates a proof of history value (simulating cryptographic proof of elapsed time)
func PoH(prevPoH int64) int64 {
	rand.Seed(time.Now().UnixNano())
	// Simulate time-stamping function
	return prevPoH + rand.Int63n(1000)
}

// ValidatePoH verifies the PoH value
func ValidatePoH(proof int64) bool {
	// For simplicity, assume that the PoH is valid if it's greater than the previous value
	return proof > 0
}
