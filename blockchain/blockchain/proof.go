package blockchain

func ValidateBlock(block *Block, difficulty int) bool {
	if block.Index < 0 {
		return false
	}
	
	if block.Hash != block.CalculateHash() {
		return false
	}
	
	if !isHashValid(block.Hash, difficulty) {
		return false
	}
	
	return true
}

func ValidateTransaction(tx Transaction) bool {
	if tx.Timestamp <= 0 {
		return false
	}
	
	switch tx.Type {
	case "DOCUMENT":
		if tx.Document == nil || tx.Document.Hash == "" {
			return false
		}
	case "USER":
		if tx.UserData == nil || tx.UserData.Field == "" {
			return false
		}
	case "WALLET":
		if tx.Wallet == nil || tx.Wallet.Address == "" {
			return false
		}
	default:
		return false
	}
	
	return true
}
