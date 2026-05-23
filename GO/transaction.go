package blockchain

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
)

// Transaction represents a transaction between two wallets
type Transaction struct {
	Sender    string  // Public Key (address) of the sender
	Receiver  string  // Public Key (address) of the receiver
	Amount    float64 // Amount being transferred
	Signature string  // Digital signature to verify the transaction
}

// NewTransaction creates a new unsigned transaction
func NewTransaction(sender, receiver string, amount float64) *Transaction {
	return &Transaction{Sender: sender, Receiver: receiver, Amount: amount}
}

// HashTransaction generates the hash of the transaction data
func (t *Transaction) HashTransaction() string {
	record := t.Sender + t.Receiver + string(t.Amount)
	h := sha256.New()
	h.Write([]byte(record))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}

// SignTransaction signs the transaction using the sender's private key
func (t *Transaction) SignTransaction(privKey *ecdsa.PrivateKey) {
	hash := t.HashTransaction()
	r, s, _ := ecdsa.Sign(nil, privKey, []byte(hash))
	signature := append(r.Bytes(), s.Bytes()...)
	t.Signature = hex.EncodeToString(signature)
}

// VerifyTransaction verifies the transaction using the sender's public key
func (t *Transaction) VerifyTransaction(pubKey *ecdsa.PublicKey) bool {
	hash := t.HashTransaction()
	signature, _ := hex.DecodeString(t.Signature)
	r := big.Int{}
	s := big.Int{}
	sigLen := len(signature)
	r.SetBytes(signature[:sigLen/2])
	s.SetBytes(signature[sigLen/2:])
	return ecdsa.Verify(pubKey, []byte(hash), &r, &s)
}
