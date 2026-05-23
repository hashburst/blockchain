package blockchain

import (
	"testing"
	"time"
)

func TestAddBlockValidation(t *testing.T) {
	bc := NewBlockchain(4)
	
	tx := Transaction{
		Type:      "TEST",
		UserID:    "0001234",
		Timestamp: time.Now().Unix(),
	}
	
	if _, err := bc.AddBlock([]Transaction{tx}, "poh_hash"); err != nil {
		t.Errorf("Failed to add block: %v", err)
	}
}
