// internal/wallet/wallet.go

package wallet

import (
    "fmt"
    "crypto/ecdsa"
    "crypto/rand"
    "crypto/elliptic"
)

func GenerateWallet() {
    priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    fmt.Printf("New Wallet: Private Key: %x\n", priv.D)
}
