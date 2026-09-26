package blockchain

// chainops.go — fork-choice and projection-safe chain replacement.

import (
	"fmt"
	"log"

	"hashburst/consensus"
	"hashburst/hvm"
)

func (bc *Blockchain) TryExtendOrAdopt(incoming []*Block) (applied bool, err error) {
	if len(incoming) == 0 {
		return false, nil
	}

	bc.mu.Lock()
	// Consensus reactor reconciliation belongs to the authoritative chain
	// transition, not to an individual network caller. The chain lock must be
	// released first because SyncToFinalizedHead may enter a new proposer round
	// and read blockchain state again.
	defer func() {
		bc.mu.Unlock()
		if applied {
			if syncErr := bc.syncConsensusReactorToHead(); syncErr != nil && err == nil {
				err = fmt.Errorf("sync consensus reactor after chain advance: %w", syncErr)
			}
		}
	}()

	head := bc.Blocks[len(bc.Blocks)-1]
	first := incoming[0]
	if first.Index == head.Index+1 && first.PrevHash == head.Hash {
		return bc.extendLocked(incoming)
	}

	// Once validator finality is active, a finalized head is not replaced by a
	// longer-PoH branch. Any competing branch must be rejected rather than fed
	// into the legacy fork-choice rule.
	if bc.v2Config.ConsensusEnabledAt(head.Index) || bc.v2Config.ConsensusEnabledAt(first.Index) {
		return false, fmt.Errorf("finalized validator-consensus chain does not reorganize below head #%d", head.Index)
	}

	candidate, err := bc.buildCandidateChain(incoming)
	if err != nil {
		return false, fmt.Errorf("ramo non ricostruibile: %w", err)
	}
	ticksPerBlock := int64(bc.v2Config.EffectivePoHTicks())
	candidateTicks := int64(len(candidate)-1) * ticksPerBlock
	ourTicks := int64(len(bc.Blocks)-1) * ticksPerBlock
	if candidateTicks <= ourTicks {
		return false, nil
	}

	if err := validateFullChainWithConfig(candidate, bc.v2Config, bc.MiningReward); err != nil {
		return false, fmt.Errorf("ramo piu' lungo ma invalido: %w", err)
	}
	newState, newHVM, newValidators, newReceipts, err := bc.computeProjections(candidate)
	if err != nil {
		return false, fmt.Errorf("ramo con proiezioni invalide: %w", err)
	}
	if err := bc.replaceLocked(candidate, newState, newHVM, newValidators, newReceipts); err != nil {
		return false, err
	}
	log.Printf("fork-choice: adottato ramo con %d tick PoH (nostro: %d)", candidateTicks, ourTicks)
	return true, nil
}

func (bc *Blockchain) extendLocked(blocks []*Block) (bool, error) {
	appliedAny := false
	for _, b := range blocks {
		head := bc.Blocks[len(bc.Blocks)-1]
		if b.Index <= head.Index {
			continue
		}
		if err := ValidateBlockAgainstConfig(head, b, bc.MiningReward, bc.v2Config); err != nil {
			return appliedAny, fmt.Errorf("blocco #%d rifiutato: %w", b.Index, err)
		}

		var ex *blockExecutionV2
		var err error
		if b.EffectiveVersion() >= BlockVersionV2 {
			ex, err = bc.validateV2Commitments(b)
			if err != nil {
				return appliedAny, fmt.Errorf("blocco #%d V2: %w", b.Index, err)
			}
			if bc.v2Config.ConsensusEnabledAt(b.Index) {
				if err := bc.validateConsensusProposal(b, ex.validatorSet, true); err != nil {
					return appliedAny, fmt.Errorf("blocco #%d finality: %w", b.Index, err)
				}
			}
		} else if bc.state != nil {
			if err := bc.state.CanApplyBlock(b); err != nil {
				return appliedAny, fmt.Errorf("blocco #%d, fondi: %w", b.Index, err)
			}
		}

		if err := bc.storage.SaveBlock(b); err != nil {
			return appliedAny, fmt.Errorf("sync: persistenza blocco #%d: %w", b.Index, err)
		}
		bc.Blocks = append(bc.Blocks, b)
		if ex != nil {
			bc.commitV2Execution(ex)
		} else if bc.state != nil {
			_ = bc.state.ApplyBlock(b)
		}
		bc.removeMinedFromMempool(b)
		appliedAny = true
	}
	return appliedAny, nil
}

func (bc *Blockchain) buildCandidateChain(incoming []*Block) ([]*Block, error) {
	first := incoming[0]
	if first.Index == 0 {
		return nil, fmt.Errorf("il ramo riparte dal genesis: non gestito")
	}
	forkPoint := -1
	for i, b := range bc.Blocks {
		if b.Hash == first.PrevHash {
			forkPoint = i
			break
		}
	}
	if forkPoint < 0 {
		return nil, fmt.Errorf("nessun antenato comune per il blocco #%d", first.Index)
	}
	candidate := make([]*Block, 0, forkPoint+1+len(incoming))
	candidate = append(candidate, bc.Blocks[:forkPoint+1]...)
	candidate = append(candidate, incoming...)
	return candidate, nil
}

func (bc *Blockchain) replaceLocked(chain []*Block, newState *State, newHVM *hvm.Engine, newValidators *consensus.Registry, newReceipts map[string]hvm.Receipt) error {
	if err := bc.storage.Rewrite(chain); err != nil {
		return fmt.Errorf("riscrittura storage: %w", err)
	}
	bc.Blocks = chain
	bc.state = newState
	bc.hvmEngine = newHVM
	bc.validators = newValidators
	bc.receipts = newReceipts
	return nil
}

func (bc *Blockchain) removeMinedFromMempool(b *Block) {
	if bc.mempool == nil {
		return
	}
	for _, tx := range b.Transactions {
		if tx != nil {
			bc.mempool.RemoveTransaction(tx.ID)
		}
	}
	for _, tx := range b.TransactionsV2 {
		if tx != nil {
			bc.mempool.RemoveTransactionV2(tx.HashHex())
		}
	}
}

func validateFullChain(chain []*Block) error {
	return validateFullChainWithConfig(chain, DefaultProtocolV2Config(), DefaultMiningReward)
}

func validateFullChainWithConfig(chain []*Block, cfg ProtocolV2Config, reward float64) error {
	if len(chain) == 0 {
		return fmt.Errorf("catena vuota")
	}
	if chain[0].Hash != NewGenesisBlock().Hash {
		return fmt.Errorf("genesis non corrisponde")
	}
	for i := 1; i < len(chain); i++ {
		if err := ValidateBlockAgainstConfig(chain[i-1], chain[i], reward, cfg); err != nil {
			return fmt.Errorf("blocco #%d: %w", i, err)
		}
	}
	return nil
}

func (bc *Blockchain) SnapshotBlocksFrom(fromIndex, limit int) []*Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	var out []*Block
	for _, b := range bc.Blocks {
		if b.Index >= fromIndex {
			out = append(out, b)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}
