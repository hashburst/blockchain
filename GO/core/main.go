// cmd/hashburstcore/main.go

package main

import (
    "fmt"
    "os"
    "hashburstcore/mining"
    "hashburstcore/wallet"
)

func main() {
    if len(os.Args) < 2 {
        fmt.Println("Usage: hashburstcore [mine|wallet]")
        return
    }
    switch os.Args[1] {
    case "mine":
        mining.StartMining()
    case "wallet":
        wallet.GenerateWallet()
    default:
        fmt.Println("Unknown command")
    }
}
