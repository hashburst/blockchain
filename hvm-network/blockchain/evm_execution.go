package blockchain

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
	execution "hashburst/evm-execution"
)

// EVMConfig is a consensus change, never inferred from a library upgrade.
// Nil preserves the existing network and its configuration digest.
// BaseFeeWei is fixed for this initial protocol; EIP-1559 adjustment is not implied.
type EVMConfig struct {
	ActivationHeight uint64 `json:"activation_height"`
	GasLimit         uint64 `json:"gas_limit"`
	BaseFeeWei       uint64 `json:"base_fee_wei"`
}

func (c ProtocolV2Config) EVMEnabledAt(h int) bool {
	return c.EVM != nil && h >= 0 && uint64(h) >= c.EVM.ActivationHeight
}
func (c ProtocolV2Config) validateEVMConfig() error {
	if c.EVM == nil {
		return nil
	}
	if _, err := execution.Config(c.ChainID); err != nil {
		return err
	}
	if c.EVM.ActivationHeight == 0 || c.EVM.ActivationHeight < c.ConsensusActivationHeight || c.EVM.ActivationHeight == DisabledActivationHeight {
		return fmt.Errorf("EVM requires explicit validator-consensus activation height")
	}
	if c.EVM.GasLimit < 21000 || c.EVM.GasLimit > 30_000_000 || c.EVM.BaseFeeWei == 0 {
		return fmt.Errorf("invalid EVM gas configuration")
	}
	return nil
}

// Projections are rebuilt from the canonical HashBurst block store. There is no
// second chain database, independently writable balance or replacement genesis.
type evmProjection struct {
	db                 *state.StateDB
	accounts           map[common.Address]struct{}
	root, receiptsRoot string
	gasUsed            uint64
	history            *ethereumReceipts
}
type ethereumReceipts struct {
	previous     *ethereumReceipts
	receipts     types.Receipts
	transactions []*types.Transaction
}

func (p *evmProjection) clone() *evmProjection {
	if p == nil {
		return nil
	}
	q := *p
	q.db = p.db.Copy()
	q.accounts = make(map[common.Address]struct{}, len(p.accounts))
	for a := range p.accounts {
		q.accounts[a] = struct{}{}
	}
	// History is immutable after execution, shared across speculative clones.
	return &q
}
func cloneRawTransactions(raws [][]byte) [][]byte {
	out := make([][]byte, len(raws))
	for i, r := range raws {
		out[i] = append([]byte(nil), r...)
	}
	return out
}
func validateEVMEnvelope(b *Block, c ProtocolV2Config) error {
	if !c.EVMEnabledAt(b.Index) {
		if len(b.EthereumTransactions) != 0 || b.EVMStateRoot != "" || b.EVMReceiptsRoot != "" || b.EVMGasUsed != 0 {
			return fmt.Errorf("EVM payload before activation")
		}
		return nil
	}
	if b.EffectiveVersion() != BlockVersionEVM {
		return fmt.Errorf("EVM block version required")
	}
	size := 0
	for _, raw := range b.EthereumTransactions {
		size += len(raw)
		if len(raw) == 0 || len(raw) > execution.MaxRawBytes {
			return fmt.Errorf("invalid Ethereum envelope size")
		}
	}
	if size > 1<<20 || len(b.EthereumTransactions) > 1024 {
		return fmt.Errorf("Ethereum block payload exceeds limit")
	}
	return nil
}
func checkEVMCommitments(b *Block, s *State) error {
	if b.EffectiveVersion() < BlockVersionEVM {
		return nil
	}
	p := s.evm
	if p == nil || p.root != b.EVMStateRoot || p.receiptsRoot != b.EVMReceiptsRoot || p.gasUsed != b.EVMGasUsed {
		return fmt.Errorf("block #%d EVM execution commitments mismatch", b.Index)
	}
	return nil
}
func (bc *Blockchain) ethereumContext(b *Block, ancestors []*Block) execution.Block {
	coinbase := common.Address{}
	for _, t := range b.Transactions {
		if t != nil && t.IsSystem() {
			coinbase = common.HexToAddress(t.Receiver)
			break
		}
	}
	return execution.Block{Number: uint64(b.Index), Time: uint64(b.Timestamp.Unix()), Hash: common.HexToHash(b.Hash), ParentHash: common.HexToHash(b.PrevHash), Coinbase: coinbase, Random: common.HexToHash(b.PrevHash), GasLimit: bc.v2Config.EVM.GasLimit, BaseFee: new(big.Int).SetUint64(bc.v2Config.EVM.BaseFeeWei), HashAt: func(h uint64) common.Hash {
		if h >= uint64(len(ancestors)) {
			return common.Hash{}
		}
		return common.HexToHash(ancestors[h].Hash)
	}}
}
func (bc *Blockchain) executeEthereum(base, next *State, b *Block, ancestors []*Block) error {
	if err := validateEVMEnvelope(b, bc.v2Config); err != nil {
		return err
	}
	if !bc.v2Config.EVMEnabledAt(b.Index) {
		return nil
	}
	p := next.evm
	if p == nil {
		if uint64(b.Index) != bc.v2Config.EVM.ActivationHeight {
			return fmt.Errorf("missing prior EVM projection")
		}
		db, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
		if err != nil {
			return err
		}
		p = &evmProjection{db: db, accounts: make(map[common.Address]struct{})}
		next.evm = p
	}
	// Native transfers, fees, bonding and block rewards have already executed.
	// Synchronize the integer HBT projection while retaining every fractional wei.
	for k, units := range next.balances {
		if !common.IsHexAddress(k) {
			continue
		}
		a := common.HexToAddress(k)
		p.accounts[a] = struct{}{}
		if units < 0 {
			return fmt.Errorf("negative native balance")
		}
		_, remainder, err := execution.SplitWei(p.db.GetBalance(a).ToBig())
		if err != nil {
			return err
		}
		amount, err := execution.NativeToWei(units)
		if err != nil {
			return err
		}
		amount.Add(amount, big.NewInt(remainder))
		p.db.SetBalance(a, uint256.MustFromBig(amount), tracing.BalanceChangeUnspecified)
	}
	for k, n := range next.sequences {
		if common.IsHexAddress(k) {
			a := common.HexToAddress(k)
			p.accounts[a] = struct{}{}
			p.db.SetNonce(a, n, tracing.NonceChangeUnspecified)
		}
	}
	result, err := execution.ApplyBlock(context.Background(), p.db, bc.v2Config.ChainID, bc.ethereumContext(b, ancestors), b.EthereumTransactions)
	if err != nil {
		return fmt.Errorf("EVM execution: %w", err)
	}
	p.db = result.State
	for a := range result.Touched {
		p.accounts[a] = struct{}{}
	}
	for a := range p.accounts {
		units, _, err := execution.SplitWei(p.db.GetBalance(a).ToBig())
		if err != nil {
			return fmt.Errorf("EVM balance out of native range: %w", err)
		}
		next.balances[stateKey(a.Hex())] = units
		next.sequences[stateKey(a.Hex())] = p.db.GetNonce(a)
	}
	p.root = result.StateRoot.Hex()
	p.receiptsRoot = result.ReceiptsRoot.Hex()
	p.gasUsed = result.GasUsed
	if len(b.EthereumTransactions) > 0 {
		txs := make([]*types.Transaction, len(b.EthereumTransactions))
		for i, raw := range b.EthereumTransactions {
			txs[i], _, err = execution.Decode(raw, bc.v2Config.ChainID)
			if err != nil {
				return err
			}
		}
		p.history = &ethereumReceipts{previous: p.history, receipts: result.Receipts, transactions: txs}
	}
	return nil
}

func (bc *Blockchain) ethereumReceiptLocked(hash common.Hash) (*types.Receipt, *types.Transaction) {
	if bc.state.evm == nil {
		return nil, nil
	}
	for h := bc.state.evm.history; h != nil; h = h.previous {
		for i, tx := range h.transactions {
			if tx.Hash() == hash {
				return h.receipts[i], tx
			}
		}
	}
	return nil, nil
}
func sameEthereumSender(raw []byte, chain uint64, sender string) bool {
	_, a, e := execution.Decode(raw, chain)
	return e == nil && strings.EqualFold(stateKey(a.Hex()), stateKey(sender))
}

func (c ProtocolV2Config) detached() ProtocolV2Config {
	if c.EVM != nil {
		copy := *c.EVM
		c.EVM = &copy
	}
	return c
}
