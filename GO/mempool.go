package core

import (
    "sync"
    "time"
)

// Transaction rappresenta una singola transazione
type Transaction struct {
    ID        string
    Timestamp time.Time
    Data      string
    Fee       int
}

// Mempool è la struttura che mantiene le transazioni non confermate
type Mempool struct {
    transactions map[string]Transaction
    mutex        sync.Mutex
}

// NewMempool crea una nuova istanza della Mempool
func NewMempool() *Mempool {
    return &Mempool{
        transactions: make(map[string]Transaction),
    }
}
