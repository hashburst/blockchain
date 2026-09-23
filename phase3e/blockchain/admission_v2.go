package blockchain

import (
	"fmt"

	"hashburst/protocolv2"
)

// AdmitTransactionV2 validates a locally/RPC submitted V2 transaction against
// the current account state plus same-sender pending transactions, then adds it
// to the mempool without exposing or accepting any private key.
func (bc *Blockchain) AdmitTransactionV2(tx *protocolv2.TransactionV2) error {
	if bc.mempool == nil {
		return fmt.Errorf("mempool not configured")
	}
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	nextHeight := bc.Blocks[len(bc.Blocks)-1].Index + 1
	if !bc.v2Config.EnabledAt(nextHeight) {
		return fmt.Errorf("Protocol V2 is not active at next block height %d", nextHeight)
	}
	if err := bc.validateV2Transaction(tx); err != nil {
		return err
	}
	if bc.containsTxV2Locked(tx.HashHex()) {
		return fmt.Errorf("transaction already confirmed")
	}

	pending := bc.mempool.PendingV2ForSender(tx.Sender)
	expected := bc.state.Sequence(tx.Sender)
	reserved := int64(0)
	for _, p := range pending {
		if p.Sequence != expected {
			return fmt.Errorf("mempool sequence gap for %s: got %d expected %d", tx.Sender, p.Sequence, expected)
		}
		var err error
		reserved, err = checkedAddInt64(reserved, p.ValueUnits)
		if err != nil {
			return fmt.Errorf("pending reservation overflow")
		}
		reserved, err = checkedAddInt64(reserved, p.MaxFeeUnits)
		if err != nil {
			return fmt.Errorf("pending fee reservation overflow")
		}
		expected++
	}
	if tx.Sequence != expected {
		return fmt.Errorf("sequence %d: expected %d considering pending transactions", tx.Sequence, expected)
	}
	var err error
	reserved, err = checkedAddInt64(reserved, tx.ValueUnits)
	if err != nil {
		return fmt.Errorf("reservation overflow")
	}
	reserved, err = checkedAddInt64(reserved, tx.MaxFeeUnits)
	if err != nil {
		return fmt.Errorf("fee reservation overflow")
	}
	if bc.state.BalanceUnits(tx.Sender) < reserved {
		return fmt.Errorf("insufficient HBT for pending value+max fees: balance=%d reserved=%d", bc.state.BalanceUnits(tx.Sender), reserved)
	}
	if !bc.mempool.AddTransactionV2(tx) {
		return fmt.Errorf("transaction already in mempool")
	}
	return nil
}

func (bc *Blockchain) containsTxV2Locked(txID string) bool {
	for _, b := range bc.Blocks {
		for _, tx := range b.TransactionsV2 {
			if tx != nil && tx.HashHex() == txID {
				return true
			}
		}
	}
	return false
}
