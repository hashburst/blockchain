package blockchain

import "strings"

// ContainsTx checks confirmed V1 transaction IDs.
func (bc *Blockchain) ContainsTx(txID string) bool {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	for _, b := range bc.Blocks {
		for _, tx := range b.Transactions {
			if tx != nil && tx.ID == txID {
				return true
			}
		}
	}
	return false
}

func (bc *Blockchain) ContainsTxV2(txID string) bool {
	txID = strings.ToLower(strings.TrimPrefix(txID, "0x"))
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	for _, b := range bc.Blocks {
		for _, tx := range b.TransactionsV2 {
			if tx != nil && strings.ToLower(strings.TrimPrefix(tx.HashHex(), "0x")) == txID {
				return true
			}
		}
	}
	return false
}
