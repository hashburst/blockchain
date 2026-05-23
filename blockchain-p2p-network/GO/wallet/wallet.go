package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

// Wallet holds an ECDSA key pair (secp256r1 / P-256)
type Wallet struct {
	PrivateKey *ecdsa.PrivateKey
	PublicKey  *ecdsa.PublicKey
}

// NewWallet generates a new wallet with a random key pair
func NewWallet() *Wallet {
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	return &Wallet{PrivateKey: privKey, PublicKey: &privKey.PublicKey}
}

// Address generates a wallet address from the public key (SHA-256 based)
func (w *Wallet) Address() string {
	pubKeyBytes := append(w.PublicKey.X.Bytes(), w.PublicKey.Y.Bytes()...)
	hash := sha256.Sum256(pubKeyBytes)
	return fmt.Sprintf("%x", hash[:])
}
