package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
)

// Wallet holds a public-private key pair
type Wallet struct {
	PrivateKey *ecdsa.PrivateKey
	PublicKey  *ecdsa.PublicKey
}

// NewWallet generates a new wallet
func NewWallet() *Wallet {
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	return &Wallet{PrivateKey: privKey, PublicKey: &privKey.PublicKey}
}

// Address generates a wallet address from the public key (simplified)
func (w *Wallet) Address() string {
	pubKeyBytes := append(w.PublicKey.X.Bytes(), w.PublicKey.Y.Bytes()...)
	hash := sha256.Sum256(pubKeyBytes)
	return fmt.Sprintf("%x", hash[:])
}
