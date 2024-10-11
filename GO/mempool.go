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

// AddTransaction aggiunge una nuova transazione alla Mempool
func (m *Mempool) AddTransaction(tx Transaction) {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    m.transactions[tx.ID] = tx
}

// RemoveTransaction rimuove una transazione dalla Mempool
func (m *Mempool) RemoveTransaction(txID string) {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    delete(m.transactions, txID)
}

// GetTransactions restituisce tutte le transazioni nella Mempool
func (m *Mempool) GetTransactions() []Transaction {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    txs := []Transaction{}
    for _, tx := range m.transactions {
        txs = append(txs, tx)
    }
    return txs
}
