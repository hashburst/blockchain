package blockchain

import (
	"sort"
	"strings"
	"sync"

	"hashburst/protocolv2"
)

type Mempool struct {
	transactions   map[string]*Transaction
	transactionsV2 map[string]*protocolv2.TransactionV2
	mutex          sync.Mutex
}

func NewMempool() *Mempool {
	return &Mempool{
		transactions:   make(map[string]*Transaction),
		transactionsV2: make(map[string]*protocolv2.TransactionV2),
	}
}

func (m *Mempool) AddTransaction(tx *Transaction) {
	if tx == nil {
		return
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if _, exists := m.transactions[tx.ID]; exists {
		return
	}
	m.transactions[tx.ID] = tx
}

func (m *Mempool) AddTransactionOnce(tx *Transaction) bool {
	if tx == nil {
		return false
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, existing := range m.transactions {
		if existing.Receiver == tx.Receiver && existing.Sender == tx.Sender {
			return false
		}
	}
	m.transactions[tx.ID] = tx
	return true
}

func (m *Mempool) AddTransactionV2(tx *protocolv2.TransactionV2) bool {
	if tx == nil {
		return false
	}
	id := tx.HashHex()
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if _, exists := m.transactionsV2[id]; exists {
		return false
	}
	m.transactionsV2[id] = tx.Clone()
	return true
}

func (m *Mempool) RemoveTransaction(id string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.transactions, id)
}

func (m *Mempool) HasTransactionV2(id string) bool {
	id = strings.TrimPrefix(strings.ToLower(id), "0x")
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for key := range m.transactionsV2 {
		if strings.TrimPrefix(strings.ToLower(key), "0x") == id {
			return true
		}
	}
	return false
}

func (m *Mempool) RemoveTransactionV2(id string) {
	id = strings.TrimPrefix(strings.ToLower(id), "0x")
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for key := range m.transactionsV2 {
		if strings.TrimPrefix(strings.ToLower(key), "0x") == id {
			delete(m.transactionsV2, key)
			return
		}
	}
}

// SnapshotTransactions returns a deterministic copy WITHOUT removing entries.
// Phase 3B deliberately fixes the old "read then clear" behavior: transactions
// are removed only after a block has been accepted/persisted.
func (m *Mempool) SnapshotTransactions() []*Transaction {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	ids := make([]string, 0, len(m.transactions))
	for id := range m.transactions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*Transaction, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.transactions[id])
	}
	return out
}

// GetTransactions remains as a compatibility alias, but no longer clears the
// mempool. Callers relying on the old destructive behavior must remove only
// transactions actually included in an accepted block.
func (m *Mempool) GetTransactions() []*Transaction { return m.SnapshotTransactions() }

func (m *Mempool) SnapshotTransactionsV2() []*protocolv2.TransactionV2 {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	out := make([]*protocolv2.TransactionV2, 0, len(m.transactionsV2))
	for _, tx := range m.transactionsV2 {
		out = append(out, tx.Clone())
	}
	// Deterministic proposer order also preserves same-account sequence order.
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Sender), strings.ToLower(out[j].Sender)
		if a != b {
			return a < b
		}
		if out[i].Sequence != out[j].Sequence {
			return out[i].Sequence < out[j].Sequence
		}
		return out[i].HashHex() < out[j].HashHex()
	})
	return out
}

func (m *Mempool) PendingV2ForSender(sender string) []*protocolv2.TransactionV2 {
	sender = strings.ToLower(sender)
	m.mutex.Lock()
	defer m.mutex.Unlock()
	var out []*protocolv2.TransactionV2
	for _, tx := range m.transactionsV2 {
		if strings.ToLower(tx.Sender) == sender {
			out = append(out, tx.Clone())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out
}

func (m *Mempool) Size() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return len(m.transactions) + len(m.transactionsV2)
}

func (m *Mempool) SizeV2() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return len(m.transactionsV2)
}
