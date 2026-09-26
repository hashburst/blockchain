package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/holiman/uint256"
)

// Backend must return a detached state and consensus-derived block context.
// Admit must validate and reserve nonce/funds in the real node mempool, gossip
// the original signed bytes, and must not execute an independent chain.
type Backend interface {
	Snapshot(context.Context, rpc.BlockNumber) (*state.StateDB, Block, error)
	Admit(context.Context, []byte) (common.Hash, error)
	Receipt(context.Context, common.Hash) (*types.Receipt, error)
	Transaction(context.Context, common.Hash) (*types.Transaction, error)
}
type API struct {
	backend Backend
	chain   uint64
}

func NewAPI(chain uint64, b Backend) (*API, error) {
	if _, e := Config(chain); e != nil {
		return nil, e
	}
	if b == nil {
		return nil, errors.New("consensus backend required")
	}
	return &API{backend: b, chain: chain}, nil
}
func (a *API) ChainId() hexutil.Uint64 { return hexutil.Uint64(a.chain) }
func (a *API) BlockNumber(ctx context.Context) (hexutil.Uint64, error) {
	_, b, e := a.backend.Snapshot(ctx, rpc.LatestBlockNumber)
	return hexutil.Uint64(b.Number), e
}
func (a *API) GetBalance(ctx context.Context, address common.Address, tag rpc.BlockNumber) (*hexutil.Big, error) {
	s, _, e := a.backend.Snapshot(ctx, tag)
	if e != nil {
		return nil, e
	}
	n := s.GetBalance(address).ToBig()
	return (*hexutil.Big)(n), nil
}
func (a *API) GetTransactionCount(ctx context.Context, address common.Address, tag rpc.BlockNumber) (hexutil.Uint64, error) {
	s, _, e := a.backend.Snapshot(ctx, tag)
	if e != nil {
		return 0, e
	}
	return hexutil.Uint64(s.GetNonce(address)), nil
}
func (a *API) GetCode(ctx context.Context, address common.Address, tag rpc.BlockNumber) (hexutil.Bytes, error) {
	s, _, e := a.backend.Snapshot(ctx, tag)
	if e != nil {
		return nil, e
	}
	return append(hexutil.Bytes{}, s.GetCode(address)...), nil
}
func (a *API) GetStorageAt(ctx context.Context, address common.Address, key hexutil.Big, tag rpc.BlockNumber) (common.Hash, error) {
	s, _, e := a.backend.Snapshot(ctx, tag)
	if e != nil {
		return common.Hash{}, e
	}
	n := (*big.Int)(&key)
	if n.Sign() < 0 || n.BitLen() > 256 {
		return common.Hash{}, errors.New("invalid storage position")
	}
	return s.GetState(address, common.BigToHash(n)), nil
}
func (a *API) GetTransactionReceipt(ctx context.Context, hash common.Hash) (interface{}, error) {
	receipt, e := a.backend.Receipt(ctx, hash)
	if e != nil {
		return nil, e
	}
	if receipt == nil {
		return nil, nil
	}
	tx, e := a.backend.Transaction(ctx, hash)
	if e != nil {
		return nil, e
	}
	if tx == nil || tx.Hash() != hash || receipt.TxHash != hash {
		return nil, errors.New("receipt transaction unavailable or inconsistent")
	}
	sender, e := types.Sender(types.NewCancunSigner(new(big.Int).SetUint64(a.chain)), tx)
	if e != nil {
		return nil, e
	}
	data, e := json.Marshal(receipt)
	if e != nil {
		return nil, e
	}
	var out map[string]interface{}
	if e = json.Unmarshal(data, &out); e != nil {
		return nil, e
	}
	out["from"] = sender
	out["to"] = tx.To()
	if receipt.ContractAddress == (common.Address{}) {
		out["contractAddress"] = nil
	}
	if receipt.Logs == nil {
		out["logs"] = []*types.Log{}
	}
	return out, nil
}
func (a *API) SendRawTransaction(ctx context.Context, raw hexutil.Bytes) (common.Hash, error) {
	tx, _, e := Decode(raw, a.chain)
	if e != nil {
		return common.Hash{}, e
	}
	hash, e := a.backend.Admit(ctx, append([]byte(nil), raw...))
	if e != nil {
		return common.Hash{}, e
	}
	if hash != tx.Hash() {
		return common.Hash{}, errors.New("consensus admission hash mismatch")
	}
	return hash, nil
}

type CallArgs struct {
	From  *common.Address `json:"from"`
	To    *common.Address `json:"to"`
	Gas   *hexutil.Uint64 `json:"gas"`
	Value *hexutil.Big    `json:"value"`
	Data  *hexutil.Bytes  `json:"data"`
	Input *hexutil.Bytes  `json:"input"`
}
type revertError struct{ data []byte }

func (e *revertError) Error() string          { return "execution reverted" }
func (e *revertError) ErrorCode() int         { return 3 }
func (e *revertError) ErrorData() interface{} { return hexutil.Encode(e.data) }
func simulate(ctx context.Context, s *state.StateDB, b Block, chain uint64, args CallArgs, gas uint64) (*core.ExecutionResult, error) {
	cfg, e := Config(chain)
	if e != nil {
		return nil, e
	}
	if s == nil || b.HashAt == nil || b.BaseFee == nil || gas > b.GasLimit {
		return nil, errors.New("invalid simulation context")
	}
	var from common.Address
	if args.From != nil {
		from = *args.From
	}
	value := new(uint256.Int)
	if args.Value != nil {
		v := (*big.Int)(args.Value)
		if v.Sign() < 0 || v.BitLen() > 256 {
			return nil, errors.New("invalid value")
		}
		value.SetFromBig(v)
	}
	data := []byte{}
	if args.Input != nil {
		data = *args.Input
	}
	if args.Data != nil {
		if args.Input != nil && string(*args.Data) != string(data) {
			return nil, errors.New("data and input differ")
		}
		data = *args.Data
	}
	if len(data) > MaxRawBytes {
		return nil, errors.New("call data exceeds limit")
	}
	// eth_call runs on a copy and does not charge gas to the canonical ledger.
	st := s.Copy()
	env := vm.NewEVM(vm.BlockContext{CanTransfer: core.CanTransfer, Transfer: core.Transfer, GetHash: func(n uint64) common.Hash {
		if n >= b.Number || b.Number-n > 256 {
			return common.Hash{}
		}
		return b.HashAt(n)
	}, Coinbase: b.Coinbase, GasLimit: b.GasLimit, BlockNumber: new(big.Int).SetUint64(b.Number), Time: b.Time, Difficulty: new(big.Int), BaseFee: new(big.Int).Set(b.BaseFee), Random: &b.Random, BlobBaseFee: big.NewInt(1)}, st, cfg, vm.Config{NoBaseFee: true})
	defer env.Release()
	done := make(chan struct{})
	finished := make(chan struct{})
	defer func() { close(done); <-finished }()
	go func() {
		defer close(finished)
		select {
		case <-ctx.Done():
			env.Cancel()
		case <-done:
		}
	}()
	result, e := core.ApplyMessage(env, &core.Message{From: from, To: args.To, Value: value, GasLimit: gas, GasPrice: new(uint256.Int), GasFeeCap: new(uint256.Int), GasTipCap: new(uint256.Int), Data: data, SkipNonceChecks: true, SkipTransactionChecks: true}, core.NewGasPool(gas))
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return result, e
}
func (a *API) Call(ctx context.Context, args CallArgs, tag rpc.BlockNumber) (hexutil.Bytes, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	s, b, e := a.backend.Snapshot(ctx, tag)
	if e != nil {
		return nil, e
	}
	gas := b.GasLimit
	if args.Gas != nil && uint64(*args.Gas) < gas {
		gas = uint64(*args.Gas)
	}
	result, e := simulate(ctx, s, b, a.chain, args, gas)
	if e != nil {
		return nil, e
	}
	if result.Err != nil {
		if errors.Is(result.Err, vm.ErrExecutionReverted) {
			return nil, &revertError{data: result.Revert()}
		}
		return nil, result.Err
	}
	return append(hexutil.Bytes{}, result.Return()...), nil
}
func (a *API) EstimateGas(ctx context.Context, args CallArgs) (hexutil.Uint64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	s, b, e := a.backend.Snapshot(ctx, rpc.PendingBlockNumber)
	if e != nil {
		return 0, e
	}
	high := b.GasLimit
	if args.Gas != nil && uint64(*args.Gas) < high {
		high = uint64(*args.Gas)
	}
	r, e := simulate(ctx, s, b, a.chain, args, high)
	if e != nil {
		return 0, e
	}
	if r.Err != nil {
		if errors.Is(r.Err, vm.ErrExecutionReverted) {
			return 0, &revertError{data: r.Revert()}
		}
		return 0, r.Err
	}
	low := uint64(20999)
	if high <= low {
		return 0, fmt.Errorf("gas below intrinsic minimum")
	}
	for high-low > 1 {
		mid := low + (high-low)/2
		result, err := simulate(ctx, s, b, a.chain, args, mid)
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if err != nil || result.Failed() {
			low = mid
		} else {
			high = mid
		}
	}
	return hexutil.Uint64(high), nil
}
