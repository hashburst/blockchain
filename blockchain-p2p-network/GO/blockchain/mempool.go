package blockchain

import "sync"

type Mempool struct {
	transactions map[string]*Transaction
	mutex        sync.Mutex
}

func NewMempool() *Mempool {
	return &Mempool{transactions: make(map[string]*Transaction)}
}

func (m *Mempool) AddTransaction(tx *Transaction) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.transactions[tx.ID] = tx
}

func (m *Mempool) RemoveTransaction(id string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.transactions, id)
}

func (m *Mempool) GetTransactions() []*Transaction {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	txs := []*Transaction{}
	for _, tx := range m.transactions {
		txs = append(txs, tx)
	}
	return txs
}
