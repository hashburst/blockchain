package blockchain

import (
	"fmt"
	"hashburst/protocolv2"
	"hashburst/wallet"
)

func (c ProtocolV2Config) bootstrapAt(height int) bool {
	return c.ChainID == MainnetChainID && c.GenesisImport != nil && c.MainnetBootstrapEnd != 0 && height > 0 && uint64(height) <= c.MainnetBootstrapEnd
}

// The explicit, protocol-hash-bound bootstrap interval mints zero units.
// Normal rewards and consensus rules resume immediately after its last block.
func validateBootstrapPayload(b *Block, cfg ProtocolV2Config) error {
	for _, tx := range b.Transactions {
		if tx == nil {
			return fmt.Errorf("nil bootstrap transaction")
		}
		if tx.IsSystem() {
			if tx.Amount != 0 || !wallet.AddressEqual(tx.Receiver, cfg.GenesisImport.Recipient) {
				return fmt.Errorf("bootstrap reward must be zero to founder")
			}
		} else if b.Index != 1 || tx.Receiver != RegistryAddress || tx.Amount != 0 {
			return fmt.Errorf("only node registrations allowed in first bootstrap block")
		}
	}
	for _, tx := range b.TransactionsV2 {
		if tx == nil {
			return fmt.Errorf("nil bootstrap v2 transaction")
		}
		switch b.Index {
		case 1:
			if tx.Type != protocolv2.TxHBTTransfer || !wallet.AddressEqual(tx.Sender, cfg.GenesisImport.Recipient) {
				return fmt.Errorf("bootstrap funding requires founder transfers")
			}
		case 2:
			if tx.Type != protocolv2.TxValidatorRegister {
				return fmt.Errorf("bootstrap second block requires registrations")
			}
		default:
			return fmt.Errorf("bootstrap activation delay blocks must be empty")
		}
	}
	return nil
}
