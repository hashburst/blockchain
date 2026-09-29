package testnet

import (
	"fmt"
	"reflect"
)

func validateAPoWTransition(old, next Config) error {
	if err := old.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if old.Network != "testnet" || old.Protocol.ChainID != 4735490 || old.Protocol.EVM == nil || old.Protocol.APoW != nil || next.Protocol.APoW == nil {
		return fmt.Errorf("only first APoW activation on EVM testnet is supported")
	}
	stripped := next
	stripped.Protocol.APoW = nil
	if !reflect.DeepEqual(old, stripped) {
		return fmt.Errorf("APoW migration may only add protocol.apow; historical EVM gas, identities, funds and all other rules must remain unchanged")
	}
	return nil
}
