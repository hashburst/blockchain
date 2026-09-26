package hvm

import (
	"encoding/hex"
	"fmt"
	"strings"
)

type Runtime string

const (
	RuntimeNative Runtime = "hvm-native"
	RuntimeEVM    Runtime = "evm"
)

type StandardStatus string

const (
	StandardAvailable  StandardStatus = "AVAILABLE"
	StandardActive     StandardStatus = "ACTIVE"
	StandardDeprecated StandardStatus = "DEPRECATED"
)

type StandardDescriptor struct {
	ID               string         `json:"id"`
	Origin           string         `json:"origin"`
	Version          string         `json:"version"`
	Runtime          Runtime        `json:"runtime"`
	ABIHash          string         `json:"abi_hash,omitempty"`
	CodeHash         string         `json:"code_hash,omitempty"`
	Capabilities     []string       `json:"capabilities,omitempty"`
	Dependencies     []string       `json:"dependencies,omitempty"`
	Status           StandardStatus `json:"status"`
	ActivationHeight uint64         `json:"activation_height,omitempty"`
	ConsensusFeature bool           `json:"consensus_feature"`
}

type Event struct {
	Contract string   `json:"contract"`
	Name     string   `json:"name"`
	Topics   []string `json:"topics,omitempty"`
	Data     []byte   `json:"data,omitempty"`
}

type Receipt struct {
	TxID         string  `json:"txid"`
	Success      bool    `json:"success"`
	ComputeUsed  uint64  `json:"compute_used"`
	FeeUnits     int64   `json:"fee_units"`
	Contract     string  `json:"contract,omitempty"`
	ReturnData   []byte  `json:"return_data,omitempty"`
	RevertReason string  `json:"revert_reason,omitempty"`
	Events       []Event `json:"events,omitempty"`
}

type ExecutionContext struct {
	TxID         string
	ChainID      uint64
	Sender       string
	ValueUnits   int64
	BlockHeight  uint64
	BlockTime    uint64
	ComputeLimit uint64
}

func normalizeAddress(addr string) string {
	return strings.ToLower(addr)
}

func validateHex32(name, value string, allowZero bool) error {
	v := strings.TrimPrefix(strings.TrimSpace(value), "0x")
	if len(v) != 64 {
		return fmt.Errorf("%s must be 32 bytes hex", name)
	}
	b, err := hex.DecodeString(v)
	if err != nil {
		return fmt.Errorf("%s is not hex: %w", name, err)
	}
	if !allowZero {
		zero := true
		for _, x := range b {
			if x != 0 {
				zero = false
				break
			}
		}
		if zero {
			return fmt.Errorf("%s must not be zero", name)
		}
	}
	return nil
}
