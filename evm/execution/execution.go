// Package execution is the unactivated Ethereum execution adapter for HVM Network.
// It never modifies the native ledger or opens a network endpoint.
package execution

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
)

const TestnetID uint64 = 4735490
const MainnetID uint64 = 4735489
const MaxRawBytes = 128 * 1024

// Config freezes Cancun execution rules. Subsequent forks require a new protocol
// activation; upgrading a library must not silently change consensus rules.
func Config(chainID uint64) (*params.ChainConfig, error) {
	if chainID != TestnetID && chainID != MainnetID {
		return nil, fmt.Errorf("unsupported HVM chain ID")
	}
	zero := func() *big.Int { return new(big.Int) }
	timestamp := uint64(0)
	return &params.ChainConfig{ChainID: new(big.Int).SetUint64(chainID), HomesteadBlock: zero(), EIP150Block: zero(), EIP155Block: zero(), EIP158Block: zero(), ByzantiumBlock: zero(), ConstantinopleBlock: zero(), PetersburgBlock: zero(), IstanbulBlock: zero(), BerlinBlock: zero(), LondonBlock: zero(), TerminalTotalDifficulty: zero(), ShanghaiTime: &timestamp, CancunTime: &timestamp}, nil
}

// Decode rejects unprotected legacy transactions and unsupported envelope types.
// Blob and authorization-list transactions are deliberately not activated.
func Decode(raw []byte, chainID uint64) (*types.Transaction, common.Address, error) {
	var sender common.Address
	cfg, err := Config(chainID)
	if err != nil {
		return nil, sender, err
	}
	if len(raw) == 0 || len(raw) > MaxRawBytes {
		return nil, sender, fmt.Errorf("raw transaction size")
	}
	tx := new(types.Transaction)
	if err = tx.UnmarshalBinary(raw); err != nil {
		return nil, sender, err
	}
	if tx.Type() != types.LegacyTxType && tx.Type() != types.AccessListTxType && tx.Type() != types.DynamicFeeTxType {
		return nil, sender, fmt.Errorf("transaction type not activated")
	}
	if !tx.Protected() || tx.ChainId().Cmp(cfg.ChainID) != 0 {
		return nil, sender, fmt.Errorf("transaction chain mismatch or unprotected signature")
	}
	sender, err = types.Sender(types.NewCancunSigner(cfg.ChainID), tx)
	if err != nil {
		return nil, sender, err
	}
	return tx, sender, nil
}

// Block contains only consensus-supplied values. No local clock or random input
// may influence execution. HashAt must resolve finalized ancestor hashes.
type Block struct {
	Number     uint64
	Time       uint64
	Hash       common.Hash
	ParentHash common.Hash
	Coinbase   common.Address
	Random     common.Hash
	GasLimit   uint64
	BaseFee    *big.Int
	HashAt     func(uint64) common.Hash
}
type Result struct {
	State        *state.StateDB
	Receipts     types.Receipts
	StateRoot    common.Hash
	ReceiptsRoot common.Hash
	GasUsed      uint64
}

// ApplyBlock executes on a copy. Rejected transactions invalidate the whole
// candidate without mutating parent; EVM REVERT is instead a failed receipt,
// consumes gas and advances the sender nonce according to Ethereum rules.
func ApplyBlock(ctx context.Context, parent *state.StateDB, chainID uint64, b Block, rawTxs [][]byte) (*Result, error) {
	cfg, err := Config(chainID)
	if err != nil {
		return nil, err
	}
	if parent == nil || b.BaseFee == nil || b.BaseFee.Sign() < 0 || b.BaseFee.BitLen() > 256 || b.GasLimit == 0 || b.HashAt == nil {
		return nil, fmt.Errorf("incomplete block context")
	}
	st := parent.Copy()
	number := new(big.Int).SetUint64(b.Number)
	hashAt := func(n uint64) common.Hash {
		if n >= b.Number || b.Number-n > 256 {
			return common.Hash{}
		}
		return b.HashAt(n)
	}
	env := vm.NewEVM(vm.BlockContext{CanTransfer: core.CanTransfer, Transfer: core.Transfer, GetHash: hashAt, Coinbase: b.Coinbase, GasLimit: b.GasLimit, BlockNumber: number, Time: b.Time, Difficulty: new(big.Int), BaseFee: new(big.Int).Set(b.BaseFee), BlobBaseFee: big.NewInt(1), Random: &b.Random}, st, cfg, vm.Config{})
	defer env.Release()
	gp := core.NewGasPool(b.GasLimit)
	receipts := make(types.Receipts, 0, len(rawTxs))
	for i, raw := range rawTxs {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		tx, _, err := Decode(raw, chainID)
		if err != nil {
			return nil, fmt.Errorf("transaction %d: %w", i, err)
		}
		msg, err := core.TransactionToMessage(tx, types.NewCancunSigner(cfg.ChainID), b.BaseFee)
		if err != nil {
			return nil, err
		}
		st.SetTxContext(tx.Hash(), i, uint32(i+1))
		receipt, _, err := core.ApplyTransactionWithEVM(ctx, msg, gp, st, number, b.Hash, b.Time, tx, env)
		if err != nil {
			return nil, fmt.Errorf("transaction %d: %w", i, err)
		}
		receipt.EffectiveGasPrice = msg.GasPrice.ToBig()
		receipts = append(receipts, receipt)
	}
	root := st.IntermediateRoot(cfg.Rules(number, true, b.Time))
	return &Result{State: st, Receipts: receipts, StateRoot: root, ReceiptsRoot: types.DeriveSha(receipts, trie.NewStackTrie(nil)), GasUsed: gp.CumulativeUsed()}, nil
}
