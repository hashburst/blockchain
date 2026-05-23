package blockchain

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
)

type Transaction struct {
	ID        string
	Sender    string
	Receiver  string
	Amount    float64
	Signature string
}

func NewTransaction(sender, receiver string, amount float64) *Transaction {
	t := &Transaction{Sender: sender, Receiver: receiver, Amount: amount}
	t.ID = t.HashTransaction()
	return t
}

func (t *Transaction) HashTransaction() string {
	record := t.Sender + t.Receiver + string(rune(int(t.Amount*1e8)))
	h := sha256.New()
	h.Write([]byte(record))
	return hex.EncodeToString(h.Sum(nil))
}

func (t *Transaction) SignTransaction(privKey *ecdsa.PrivateKey) error {
	r, s, err := ecdsa.Sign(rand.Reader, privKey, []byte(t.HashTransaction()))
	if err != nil {
		return err
	}
	t.Signature = hex.EncodeToString(append(r.Bytes(), s.Bytes()...))
	return nil
}

func (t *Transaction) VerifyTransaction(pubKey *ecdsa.PublicKey) bool {
	sig, err := hex.DecodeString(t.Signature)
	if err != nil || len(sig) < 2 {
		return false
	}
	r := new(big.Int).SetBytes(sig[:len(sig)/2])
	s := new(big.Int).SetBytes(sig[len(sig)/2:])
	return ecdsa.Verify(pubKey, []byte(t.HashTransaction()), r, s)
}
