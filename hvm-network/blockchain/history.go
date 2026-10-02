package blockchain

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"hashburst/ledger"
)

// Blocks is retained only for legacy constructors and fixtures. Persistent HVM
// instances use indexedHistory; callers must hold bc.mu for the helpers below.
type indexedHistory struct {
	reader *ledger.LiveReader
	head   *Block
	hashes [257]*Block
}

func (bc *Blockchain) blockCountLocked() int {
	if bc.history != nil {
		return bc.history.head.Index + 1
	}
	return len(bc.Blocks)
}
func (bc *Blockchain) headLocked() *Block {
	if bc.history != nil {
		return bc.history.head
	}
	if len(bc.Blocks) == 0 {
		return nil
	}
	return bc.Blocks[len(bc.Blocks)-1]
}
func (bc *Blockchain) blockAtLocked(n int) (*Block, error) {
	if n < 0 || n >= bc.blockCountLocked() {
		return nil, fmt.Errorf("block %d unavailable", n)
	}
	if bc.history == nil {
		return bc.Blocks[n], nil
	}
	return readHistoryBlock(bc.history.reader, n)
}
func readHistoryBlock(r *ledger.LiveReader, n int) (*Block, error) {
	var b *Block
	err := r.WithPayload(uint64(n), func(format ledger.PayloadFormat, p []byte) error {
		if format == ledger.BinaryPayload {
			b = new(Block)
			return DecodeLedgerBlockInto(p, b)
		}
		var d blockOnDisk
		if e := gob.NewDecoder(bytes.NewReader(p)).Decode(&d); e != nil {
			return e
		}
		b = blockFromOnDisk(&d)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("history block %d: %w", n, err)
	}
	if b.Index != n {
		return nil, fmt.Errorf("history payload ordinal mismatch")
	}
	return b, nil
}

// Publication follows successful durable dat+idx append; it never writes disk.
func (bc *Blockchain) appendHistoryLocked(b *Block) error {
	if bc.history == nil {
		bc.Blocks = append(bc.Blocks, b)
		return nil
	}
	if b.Index != bc.blockCountLocked() {
		return fmt.Errorf("non-sequential history publication")
	}
	if err := bc.history.reader.Publish(uint64(b.Index)); err != nil {
		bc.storage.mu.Lock()
		bc.storage.writeErr = err
		bc.storage.mu.Unlock()
		return err
	}
	bc.history.head = cloneBlockForConsensus(b)
	bc.history.hashes[b.Index%257] = &Block{Index: b.Index, Hash: b.Hash}
	if bc.confirmedNodes != nil {
		applyNodeRegistrations(bc.confirmedNodes, b, bc.v2Config.ChainID)
	}
	return nil
}
func (bc *Blockchain) ancestorWindowLocked(height int) ([]*Block, error) {
	start := height - 256
	if start < 0 {
		start = 0
	}
	out := make([]*Block, 0, height-start)
	for n := start; n < height; n++ {
		var b *Block
		if bc.history != nil {
			b = bc.history.hashes[n%257]
			if b != nil && b.Index != n {
				b = nil
			}
			if b != nil {
				out = append(out, b)
				continue
			}
		}
		if b == nil {
			var e error
			b, e = bc.blockAtLocked(n)
			if e != nil {
				return nil, e
			}
		}
		out = append(out, &Block{Index: b.Index, Hash: b.Hash})
	}
	return out, nil
}
func (bc *Blockchain) nodeProjectionLocked() (map[string]ConfirmedNodeIdentity, error) {
	if bc.history != nil {
		return cloneNodeIdentityMap(bc.confirmedNodes), nil
	}
	return nodeIdentityProjection(bc.Blocks, bc.v2Config.ChainID), nil
}

func (bc *Blockchain) CloseHistory() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	if bc.history != nil {
		return bc.history.reader.Close()
	}
	return nil
}

func (bc *Blockchain) executeCurrentBlock(b *Block) (*blockExecutionV2, error) {
	nodes, e := bc.nodeProjectionLocked()
	if e != nil {
		return nil, e
	}
	ancestors, e := bc.ancestorWindowLocked(b.Index)
	if e != nil {
		return nil, e
	}
	return bc.executeBlockV2(bc.state, bc.hvmEngine, bc.validators, nodes, b, ancestors)
}

// BlockAt returns an owned block and propagates storage/decode failures.
func (bc *Blockchain) BlockAt(n int) (*Block, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	b, e := bc.blockAtLocked(n)
	if e != nil {
		return nil, e
	}
	return cloneBlockForConsensus(b), nil
}
