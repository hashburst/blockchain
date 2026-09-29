package blockchain

import (
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"
	"math/big"
)

func (bc *Blockchain) ethereumHeader(b *Block) *types.Header {
	txs := make(types.Transactions, 0, len(b.EthereumTransactions))
	for _, raw := range b.EthereumTransactions {
		tx := new(types.Transaction)
		if tx.UnmarshalBinary(raw) == nil {
			txs = append(txs, tx)
		}
	}
	bloom := types.Bloom{}
	if bc.state.evm != nil {
		for h := bc.state.evm.history; h != nil; h = h.previous {
			if len(h.receipts) > 0 && h.receipts[0].BlockNumber.Uint64() == uint64(b.Index) {
				for _, r := range h.receipts {
					for i, v := range r.Bloom {
						bloom[i] |= v
					}
				}
				break
			}
		}
	}
	return &types.Header{ParentHash: common.HexToHash(b.PrevHash), UncleHash: types.EmptyUncleHash, Coinbase: bc.ethereumContext(b, bc.Blocks).Coinbase, Root: common.HexToHash(b.EVMStateRoot), TxHash: types.DeriveSha(txs, trie.NewStackTrie(nil)), ReceiptHash: common.HexToHash(b.EVMReceiptsRoot), Bloom: bloom, Difficulty: new(big.Int), Number: big.NewInt(int64(b.Index)), GasLimit: bc.v2Config.EVM.GasLimit, GasUsed: b.EVMGasUsed, Time: uint64(b.Timestamp.Unix()), Extra: []byte("HVM Network"), BaseFee: new(big.Int).SetUint64(bc.v2Config.EVM.BaseFeeWei)}
}
func (bc *Blockchain) publishEthereumFinalizedLocked() {
	if bc.evmSubscriptions == nil || bc.state.evm == nil || len(bc.Blocks) == 0 {
		return
	}
	b := bc.Blocks[len(bc.Blocks)-1]
	if b.Version < BlockVersionEVM || b.FinalityCertificate == nil {
		return
	}
	var logs []*types.Log
	h := bc.state.evm.history
	if h != nil && len(h.receipts) > 0 && h.receipts[0].BlockNumber.Uint64() == uint64(b.Index) {
		for _, r := range h.receipts {
			logs = append(logs, r.Logs...)
		}
	}
	_ = bc.evmSubscriptions.PublishCanonicalFinalized(bc.ethereumHeader(b), common.HexToHash(b.Hash), logs)
}

// EthereumNodeAPI provides canonical block metadata. It never invents an
// Ethereum header hash that differs from the hash actually signed by validators.
type EthereumNodeAPI struct{ bc *Blockchain }

func (a *EthereumNodeAPI) GasPrice() hexutil.Big {
	return hexutil.Big(*new(big.Int).Add(new(big.Int).SetUint64(a.bc.v2Config.EVM.BaseFeeWei), big.NewInt(1)))
}
func (a *EthereumNodeAPI) MaxPriorityFeePerGas() hexutil.Big { return hexutil.Big(*big.NewInt(1)) }
func (a *EthereumNodeAPI) GetBlockByNumber(n rpc.BlockNumber, full bool) (map[string]any, error) {
	a.bc.mu.RLock()
	defer a.bc.mu.RUnlock()
	index := int64(n)
	if n == rpc.LatestBlockNumber || n == rpc.FinalizedBlockNumber || n == rpc.SafeBlockNumber {
		index = int64(len(a.bc.Blocks) - 1)
	}
	if index < 0 || index >= int64(len(a.bc.Blocks)) {
		return nil, nil
	}
	b := a.bc.Blocks[index]
	if b.Version < BlockVersionEVM {
		return nil, fmt.Errorf("block predates EVM activation")
	}
	data, err := json.Marshal(a.bc.ethereumHeader(b))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err = json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	out["hash"] = common.HexToHash(b.Hash)
	out["uncles"] = []common.Hash{}
	txs := make([]any, 0, len(b.EthereumTransactions))
	for i, raw := range b.EthereumTransactions {
		tx := new(types.Transaction)
		if err = tx.UnmarshalBinary(raw); err != nil {
			return nil, err
		}
		if !full {
			txs = append(txs, tx.Hash())
			continue
		}
		encoded, _ := json.Marshal(tx)
		var obj map[string]any
		_ = json.Unmarshal(encoded, &obj)
		sender, e := types.Sender(types.NewCancunSigner(tx.ChainId()), tx)
		if e != nil {
			return nil, e
		}
		obj["from"] = sender
		obj["blockHash"] = common.HexToHash(b.Hash)
		obj["blockNumber"] = hexutil.Uint64(b.Index)
		obj["transactionIndex"] = hexutil.Uint64(i)
		txs = append(txs, obj)
	}
	out["transactions"] = txs
	return out, nil
}
