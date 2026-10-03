package blockchain

import (
	"log"
	"strings"
)

func (bc *Blockchain) containsTxLocked(id string, v2 bool) (bool, error) {
	if v2 {
		id = strings.ToLower(strings.TrimPrefix(id, "0x"))
		if bc.history != nil {
			_, found := bc.receipts[id]
			return found, nil
		}
	}
	for n := 0; n < bc.blockCountLocked(); n++ {
		b, e := bc.blockAtLocked(n)
		if e != nil {
			return false, e
		}
		if v2 {
			for _, tx := range b.TransactionsV2 {
				if tx != nil && strings.ToLower(strings.TrimPrefix(tx.HashHex(), "0x")) == id {
					return true, nil
				}
			}
		} else {
			for _, tx := range b.Transactions {
				if tx != nil && tx.ID == id {
					return true, nil
				}
			}
		}
	}
	return false, nil
}
func (bc *Blockchain) ContainsTxChecked(id string, v2 bool) (bool, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.containsTxLocked(id, v2)
}

// Legacy boolean callers fail closed on unreadable history. Network uses checked API.
func (bc *Blockchain) ContainsTx(id string) bool {
	v, e := bc.ContainsTxChecked(id, false)
	if e != nil {
		log.Printf("history lookup failed: %v", e)
	}
	return v || e != nil
}
func (bc *Blockchain) ContainsTxV2(id string) bool {
	v, e := bc.ContainsTxChecked(id, true)
	if e != nil {
		log.Printf("history lookup failed: %v", e)
	}
	return v || e != nil
}
