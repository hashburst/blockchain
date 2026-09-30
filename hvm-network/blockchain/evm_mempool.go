package blockchain

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	execution "hashburst/evm-execution"
	"math/big"
	"sort"
	"time"
)

func (m *Mempool) snapshotEthereum() [][]byte {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	raws := make([][]byte, 0, len(m.ethereum))
	for _, r := range m.ethereum {
		raws = append(raws, append([]byte(nil), r...))
	}
	// Sort by recovered sender then nonce. Chain-domain checks happen on admission.
	sort.Slice(raws, func(i, j int) bool {
		a, b := new(types.Transaction), new(types.Transaction)
		_ = a.UnmarshalBinary(raws[i])
		_ = b.UnmarshalBinary(raws[j])
		sa, _ := types.Sender(types.NewCancunSigner(a.ChainId()), a)
		sb, _ := types.Sender(types.NewCancunSigner(b.ChainId()), b)
		if sa != sb {
			return sa.Hex() < sb.Hex()
		}
		if a.Nonce() != b.Nonce() {
			return a.Nonce() < b.Nonce()
		}
		return a.Hash().Hex() < b.Hash().Hex()
	})
	return raws
}
func (m *Mempool) removeEthereum(raws [][]byte) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, r := range raws {
		tx := new(types.Transaction)
		if tx.UnmarshalBinary(r) == nil {
			delete(m.ethereum, tx.Hash().Hex())
		}
	}
}

// AdmitEthereum serializes reservations with native V2 admission under bc.mu.
// It never signs, mines, changes balances or writes a validator journal.
func (bc *Blockchain) AdmitEthereum(raw []byte) (common.Hash, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	if bc.mempool == nil {
		return common.Hash{}, fmt.Errorf("mempool unavailable")
	}
	head := bc.Blocks[len(bc.Blocks)-1]
	if !bc.v2Config.EVMEnabledAt(head.Index) || bc.state.evm == nil {
		return common.Hash{}, fmt.Errorf("EVM activation block not finalized")
	}
	tx, sender, err := execution.Decode(raw, bc.v2Config.ChainID)
	if err != nil {
		return common.Hash{}, err
	}
	if r, _ := bc.ethereumReceiptLocked(tx.Hash()); r != nil {
		return common.Hash{}, fmt.Errorf("already finalized")
	}
	if len(bc.mempool.PendingV2ForSender(sender.Hex())) > 0 {
		return common.Hash{}, fmt.Errorf("native transaction already reserves sender nonce")
	}
	pending := bc.mempool.snapshotEthereum()
	expected := bc.state.Sequence(sender.Hex())
	reserved := new(big.Int)
	size := len(raw)
	for _, r := range pending {
		t, a, e := execution.Decode(r, bc.v2Config.ChainID)
		if e != nil {
			return common.Hash{}, e
		}
		size += len(r)
		if t.Hash() == tx.Hash() {
			return tx.Hash(), nil
		}
		if a == sender {
			if t.Nonce() != expected {
				return common.Hash{}, fmt.Errorf("pending nonce gap")
			}
			expected++
			reserved.Add(reserved, t.Cost())
		}
	}
	if len(pending) >= 1024 || size > 1<<20 {
		return common.Hash{}, fmt.Errorf("Ethereum mempool full")
	}
	if tx.Nonce() != expected {
		return common.Hash{}, fmt.Errorf("nonce %d expected %d", tx.Nonce(), expected)
	}
	reserved.Add(reserved, tx.Cost())
	if bc.state.evm.db.GetBalance(sender).ToBig().Cmp(reserved) < 0 {
		return common.Hash{}, fmt.Errorf("insufficient balance for pending Ethereum reservations")
	}
	b := &Block{Version: BlockVersionEVM, Index: head.Index + 1, Timestamp: time.Unix(head.Timestamp.Unix()+1, 0), PrevHash: head.Hash}
	// Validate intrinsic gas, fee and execution envelope on a detached state.
	raws := append(pending, append([]byte(nil), raw...))
	if _, err := execution.ApplyBlock(context.Background(), bc.state.evm.db, bc.v2Config.ChainID, bc.ethereumContext(b, bc.Blocks), raws); err != nil {
		return common.Hash{}, err
	}
	bc.mempool.mutex.Lock()
	bc.mempool.ethereum[tx.Hash().Hex()] = append([]byte(nil), raw...)
	bc.mempool.mutex.Unlock()
	return tx.Hash(), nil
}

// selectEthereum removes stale/invalid entries from consideration without
// deleting mempool entries until finality. One stale nonce cannot stall BFT.
func (bc *Blockchain) selectEthereum(b *Block) {
	candidates := b.EthereumTransactions
	b.EthereumTransactions = nil
	// Execute native effects once, then consider each envelope once. Replaying
	// the entire pending prefix for every entry would allow quadratic gas work.
	base, err := bc.executeBlockV2(bc.state, bc.hvmEngine, bc.validators, nodeIdentityProjection(bc.Blocks, bc.v2Config.ChainID), b, bc.Blocks)
	if err != nil {
		return
	} // prepareV2Commitments reports the native execution error.
	st := base.state.evm.db
	remaining := bc.v2Config.EVMGasLimitAt(b.Index)
	for _, raw := range candidates {
		tx, _, err := execution.Decode(raw, bc.v2Config.ChainID)
		if err != nil || tx.Gas() > remaining {
			continue
		}
		result, err := execution.ApplyBlock(context.Background(), st, bc.v2Config.ChainID, bc.ethereumContext(b, bc.Blocks), [][]byte{raw})
		if err != nil {
			continue
		}
		representable := true
		for addr := range result.Touched {
			if _, _, err := execution.SplitWei(result.State.GetBalance(addr).ToBig()); err != nil {
				representable = false
				break
			}
		}
		if !representable {
			continue
		}
		st = result.State
		remaining -= result.GasUsed
		b.EthereumTransactions = append(b.EthereumTransactions, raw)
	}
}

// A transaction finalized through another proposer may consume a pending nonce
// with different signed bytes. Remove that stale reservation after finality.
func (bc *Blockchain) pruneConsumedEthereumNonces() {
	m := bc.mempool
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for id, raw := range m.ethereum {
		tx, sender, err := execution.Decode(raw, bc.v2Config.ChainID)
		if err != nil || tx.Nonce() < bc.state.Sequence(sender.Hex()) {
			delete(m.ethereum, id)
		}
	}
	for id, tx := range m.transactionsV2 {
		if tx.Sequence < bc.state.Sequence(tx.Sender) {
			delete(m.transactionsV2, id)
		}
	}
}
