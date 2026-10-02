package blockchain

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	execution "hashburst/evm-execution"
	"math/big"
	"time"
)

// EthereumBackend binds the Ethereum API to this node's canonical projection
// and mempool. The observer may relay signed envelopes but owns no signing key.
type EthereumBackend struct {
	Chain  *Blockchain
	Gossip func([]byte)
}

func (a *EthereumBackend) Snapshot(ctx context.Context, tag rpc.BlockNumber) (*state.StateDB, execution.Block, error) {
	if err := ctx.Err(); err != nil {
		return nil, execution.Block{}, err
	}
	bc := a.Chain
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	head := bc.headLocked()
	if bc.state.evm == nil {
		return nil, execution.Block{}, fmt.Errorf("EVM not active")
	}
	if tag != rpc.LatestBlockNumber && tag != rpc.FinalizedBlockNumber && tag != rpc.SafeBlockNumber && tag != rpc.PendingBlockNumber && int64(tag) != int64(head.Index) {
		if tag < 0 || int64(tag) > int64(head.Index) {
			return nil, execution.Block{}, fmt.Errorf("unknown EVM block")
		}
		b, readErr := bc.blockAtLocked(int(tag))
		if readErr != nil {
			return nil, execution.Block{}, readErr
		}
		if !bc.v2Config.EVMEnabledAt(b.Index) {
			return nil, execution.Block{}, fmt.Errorf("EVM not active at requested block")
		}
		st, err := bc.evmReadHistory.snapshot(b)
		if err != nil {
			return nil, execution.Block{}, err
		}
		ancestors, readErr := bc.ancestorWindowLocked(b.Index)
		if readErr != nil {
			return nil, execution.Block{}, readErr
		}
		return st, bc.ethereumContext(b, ancestors), nil
	}
	st := bc.state.evm.db.Copy()
	ancestors, readErr := bc.ancestorWindowLocked(head.Index)
	if readErr != nil {
		return nil, execution.Block{}, readErr
	}
	block := bc.ethereumContext(head, ancestors)
	if tag == rpc.PendingBlockNumber {
		next := &Block{Version: BlockVersionEVM, Index: head.Index + 1, Timestamp: time.Unix(head.Timestamp.Unix()+1, 0), PrevHash: head.Hash}
		ancestors, readErr = bc.ancestorWindowLocked(next.Index)
		if readErr != nil {
			return nil, execution.Block{}, readErr
		}
		block = bc.ethereumContext(next, ancestors)
		if bc.mempool != nil {
			result, err := execution.ApplyBlock(ctx, st, bc.v2Config.ChainID, block, bc.mempool.snapshotEthereum())
			if err != nil {
				return nil, block, err
			}
			st = result.State
		}
	}
	return st, block, nil
}
func (a *EthereumBackend) Admit(ctx context.Context, raw []byte) (common.Hash, error) {
	if err := ctx.Err(); err != nil {
		return common.Hash{}, err
	}
	hash, err := a.Chain.AdmitEthereum(raw)
	if err == nil && a.Gossip != nil {
		a.Gossip(append([]byte(nil), raw...))
	}
	return hash, err
}
func (a *EthereumBackend) Receipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	bc := a.Chain
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	r, _ := bc.ethereumReceiptLocked(hash)
	if r == nil {
		return nil, nil
	}
	// Do not expose canonical log pointers to API consumers.
	out := *r
	out.PostState = append([]byte(nil), r.PostState...)
	if r.BlockNumber != nil {
		out.BlockNumber = new(big.Int).Set(r.BlockNumber)
	}
	if r.EffectiveGasPrice != nil {
		out.EffectiveGasPrice = new(big.Int).Set(r.EffectiveGasPrice)
	}
	out.Logs = make([]*types.Log, len(r.Logs))
	for i, l := range r.Logs {
		c := *l
		c.Topics = append([]common.Hash{}, l.Topics...)
		c.Data = append([]byte(nil), l.Data...)
		out.Logs[i] = &c
	}
	return &out, nil
}
func (a *EthereumBackend) Transaction(ctx context.Context, hash common.Hash) (*types.Transaction, error) {
	bc := a.Chain
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	_, tx := bc.ethereumReceiptLocked(hash)
	return tx, nil
}
func (bc *Blockchain) NewEthereumRPC(gossip func([]byte)) (*rpc.Server, error) {
	if bc.v2Config.EVM == nil {
		return nil, fmt.Errorf("EVM configuration required")
	}
	api, err := execution.NewAPI(bc.v2Config.ChainID, &EthereumBackend{Chain: bc, Gossip: gossip})
	if err != nil {
		return nil, err
	}
	bc.mu.Lock()
	if bc.evmSubscriptions == nil {
		bc.evmSubscriptions = execution.NewSubscriptions()
	}
	hub := bc.evmSubscriptions
	bc.mu.Unlock()
	server := rpc.NewServer()
	if err = server.RegisterName("eth", api); err != nil {
		server.Stop()
		return nil, err
	}
	if err = server.RegisterName("eth", hub.API()); err != nil {
		server.Stop()
		return nil, err
	}
	if err = server.RegisterName("eth", &EthereumNodeAPI{bc}); err != nil {
		server.Stop()
		return nil, err
	}
	if err = server.RegisterName("net", &ethereumNetworkAPI{bc.v2Config.ChainID}); err != nil {
		server.Stop()
		return nil, err
	}
	if err = server.RegisterName("web3", &ethereumClientAPI{}); err != nil {
		server.Stop()
		return nil, err
	}
	return server, nil
}
