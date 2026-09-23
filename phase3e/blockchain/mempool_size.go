package blockchain

// SizeLegacy reports only V1 transactions. Size() in mempool.go reports the
// total V1+V2 population and is used for general metrics.
func (m *Mempool) SizeLegacy() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return len(m.transactions)
}
