package blockchain

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	execution "hashburst/evm-execution"
)

type ethereumNetworkAPI struct{ chain uint64 }

func (a *ethereumNetworkAPI) Version() string { return strconv.FormatUint(a.chain, 10) }
func (a *ethereumNetworkAPI) Listening() bool { return true }

type ethereumClientAPI struct{}

func (*ethereumClientAPI) ClientVersion() string { return "HashBurst/HVM-Network/EVM-Cancun" }

func ethereumTransactionObject(tx *types.Transaction, b *Block, index int) (map[string]any, error) {
	data, e := json.Marshal(tx)
	if e != nil {
		return nil, e
	}
	var obj map[string]any
	if e = json.Unmarshal(data, &obj); e != nil {
		return nil, e
	}
	sender, e := types.Sender(types.NewCancunSigner(tx.ChainId()), tx)
	if e != nil {
		return nil, e
	}
	obj["from"] = sender
	obj["blockHash"] = nil
	obj["blockNumber"] = nil
	obj["transactionIndex"] = nil
	if b != nil {
		obj["blockHash"] = common.HexToHash(b.Hash)
		obj["blockNumber"] = hexutil.Uint64(b.Index)
		obj["transactionIndex"] = hexutil.Uint64(index)
	}
	return obj, nil
}
func (a *EthereumNodeAPI) GetTransactionByHash(hash common.Hash) (map[string]any, error) {
	a.bc.mu.RLock()
	defer a.bc.mu.RUnlock()
	receipt, tx := a.bc.ethereumReceiptLocked(hash)
	if tx != nil {
		b, e := a.bc.blockAtLocked(int(receipt.BlockNumber.Int64()))
		if e != nil {
			return nil, e
		}
		return ethereumTransactionObject(tx, b, int(receipt.TransactionIndex))
	}
	if a.bc.mempool != nil {
		for _, raw := range a.bc.mempool.snapshotEthereum() {
			tx, _, err := execution.Decode(raw, a.bc.v2Config.ChainID)
			if err == nil && tx.Hash() == hash {
				return ethereumTransactionObject(tx, nil, 0)
			}
		}
	}
	return nil, nil
}
func (a *EthereumNodeAPI) GetBlockByHash(hash common.Hash, full bool) (map[string]any, error) {
	a.bc.mu.RLock()
	height := -1
	for n := 0; n < a.bc.blockCountLocked(); n++ {
		b, e := a.bc.blockAtLocked(n)
		if e != nil {
			a.bc.mu.RUnlock()
			return nil, e
		}
		if common.HexToHash(b.Hash) == hash {
			height = b.Index
			break
		}
	}
	a.bc.mu.RUnlock()
	if height < 0 {
		return nil, nil
	}
	return a.GetBlockByNumber(rpc.BlockNumber(height), full)
}
func (a *EthereumNodeAPI) FeeHistory(count hexutil.Uint64, newest rpc.BlockNumber, percentiles []float64) (map[string]any, error) {
	if count == 0 || count > 1024 || len(percentiles) > 100 {
		return nil, fmt.Errorf("fee history limits")
	}
	for i, p := range percentiles {
		if math.IsNaN(p) || p < 0 || p > 100 || (i > 0 && p < percentiles[i-1]) {
			return nil, fmt.Errorf("invalid percentiles")
		}
	}
	a.bc.mu.RLock()
	defer a.bc.mu.RUnlock()
	end := int64(newest)
	if newest == rpc.LatestBlockNumber || newest == rpc.PendingBlockNumber || newest == rpc.FinalizedBlockNumber || newest == rpc.SafeBlockNumber {
		end = int64(a.bc.blockCountLocked() - 1)
	}
	start := end - int64(count) + 1
	activation := int64(a.bc.v2Config.EVM.ActivationHeight)
	if start < activation {
		start = activation
	}
	if end < start || end >= int64(a.bc.blockCountLocked()) {
		return nil, fmt.Errorf("EVM fee history unavailable")
	}
	fees := make([]hexutil.Big, 0, end-start+2)
	ratios := make([]float64, 0, end-start+1)
	rewards := make([][]hexutil.Big, 0, end-start+1)
	for h := start; h <= end; h++ {
		b, e := a.bc.blockAtLocked(int(h))
		if e != nil {
			return nil, e
		}
		base := new(big.Int).SetUint64(a.bc.v2Config.EVM.BaseFeeWei)
		fees = append(fees, hexutil.Big(*base))
		ratios = append(ratios, float64(b.EVMGasUsed)/float64(a.bc.v2Config.EVMGasLimitAt(b.Index)))
		type weighted struct {
			tip *big.Int
			gas uint64
		}
		items := []weighted{}
		for _, raw := range b.EthereumTransactions {
			tx, _, err := execution.Decode(raw, a.bc.v2Config.ChainID)
			if err != nil {
				return nil, err
			}
			r, _ := a.bc.ethereumReceiptLocked(tx.Hash())
			if r == nil {
				return nil, fmt.Errorf("receipt missing")
			}
			items = append(items, weighted{new(big.Int).Sub(r.EffectiveGasPrice, base), r.GasUsed})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].tip.Cmp(items[j].tip) < 0 })
		row := make([]hexutil.Big, len(percentiles))
		for i, p := range percentiles {
			tip := new(big.Int)
			threshold := uint64(float64(b.EVMGasUsed) * p / 100)
			var cumulative uint64
			for _, v := range items {
				tip.Set(v.tip)
				cumulative += v.gas
				if cumulative >= threshold {
					break
				}
			}
			row[i] = hexutil.Big(*tip)
		}
		rewards = append(rewards, row)
	}
	fees = append(fees, hexutil.Big(*new(big.Int).SetUint64(a.bc.v2Config.EVM.BaseFeeWei)))
	out := map[string]any{"oldestBlock": hexutil.Uint64(start), "baseFeePerGas": fees, "gasUsedRatio": ratios}
	if len(percentiles) > 0 {
		out["reward"] = rewards
	}
	return out, nil
}

type EthereumLogQuery struct {
	From, To  rpc.BlockNumber
	BlockHash *common.Hash
	Filter    execution.LogFilter
}

func (q *EthereumLogQuery) UnmarshalJSON(data []byte) error {
	q.From = rpc.LatestBlockNumber
	q.To = rpc.LatestBlockNumber
	var fields map[string]json.RawMessage
	if e := json.Unmarshal(data, &fields); e != nil {
		return e
	}
	if raw, ok := fields["blockHash"]; ok {
		var h common.Hash
		if e := json.Unmarshal(raw, &h); e != nil {
			return e
		}
		q.BlockHash = &h
		delete(fields, "blockHash")
	}
	for _, key := range []string{"fromBlock", "toBlock"} {
		if raw, ok := fields[key]; ok {
			if q.BlockHash != nil {
				return fmt.Errorf("blockHash cannot combine with range")
			}
			dst := &q.From
			if key == "toBlock" {
				dst = &q.To
			}
			if e := json.Unmarshal(raw, dst); e != nil {
				return e
			}
			delete(fields, key)
		}
	}
	rest, _ := json.Marshal(fields)
	return json.Unmarshal(rest, &q.Filter)
}
func (a *EthereumNodeAPI) GetLogs(q EthereumLogQuery) ([]*types.Log, error) {
	a.bc.mu.RLock()
	defer a.bc.mu.RUnlock()
	out := make([]*types.Log, 0)
	head := a.bc.blockCountLocked() - 1
	resolve := func(n rpc.BlockNumber) int {
		if n == rpc.LatestBlockNumber || n == rpc.FinalizedBlockNumber || n == rpc.SafeBlockNumber {
			return head
		}
		return int(n)
	}
	start, end := resolve(q.From), resolve(q.To)
	if q.BlockHash != nil {
		start = -1
		for n := 0; n < a.bc.blockCountLocked(); n++ {
			b, e := a.bc.blockAtLocked(n)
			if e != nil {
				return nil, e
			}
			if common.HexToHash(b.Hash) == *q.BlockHash {
				start = b.Index
				break
			}
		}
		if start < 0 {
			return out, nil
		}
		end = start
	}
	if start < 0 || end < start || end > head || end-start > 2048 {
		return nil, fmt.Errorf("log range unavailable or exceeds 2049 blocks")
	}
	if a.bc.state.evm == nil {
		return out, nil
	}
	for h := a.bc.state.evm.history; h != nil; h = h.previous {
		for _, r := range h.receipts {
			height := int(r.BlockNumber.Int64())
			if height < start || height > end {
				continue
			}
			for _, l := range r.Logs {
				if execution.MatchesLog(l, q.Filter) {
					c := *l
					c.Topics = append([]common.Hash{}, l.Topics...)
					c.Data = append([]byte(nil), l.Data...)
					out = append(out, &c)
					if len(out) > 10000 {
						return nil, fmt.Errorf("too many logs")
					}
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].BlockNumber != out[j].BlockNumber {
			return out[i].BlockNumber < out[j].BlockNumber
		}
		return out[i].Index < out[j].Index
	})
	return out, nil
}
