package blockchain

import (
	"fmt"
	"log"
	"strings"

	"hashburst/consensus"
	"hashburst/hvm"
)

func (bc *Blockchain) computeProjections(blocks []*Block) (*State, *hvm.Engine, *consensus.Registry, map[string]hvm.Receipt, *evmReadHistory, error) {
	st := NewState()
	history := &evmReadHistory{}
	engine := hvm.NewEngine(nil, bc.v2Config.FeePolicy)
	validators := consensus.NewRegistry(bc.v2Config.Validator)
	receipts := make(map[string]hvm.Receipt)
	confirmedNodes := make(map[string]ConfirmedNodeIdentity)
 start := 0
 if seed := bc.startupSeed; seed != nil {
  st, engine, validators, receipts = seed.state, seed.engine, seed.validators, seed.receipts
  start = seed.height+1
  confirmedNodes = nodeIdentityProjection(blocks[:start], bc.v2Config.ChainID)
 }
 for _, b := range blocks[start:] {
		if b.Index > 0 && b.Index%5000 == 0 {
			log.Printf("HVM_REPLAY_PROGRESS height=%d total=%d", b.Index, len(blocks))
		}
		if b.EffectiveVersion() < BlockVersionV2 {
			if err := st.ApplyBlock(b); err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("block #%d native state: %w", b.Index, err)
			}
			applyNodeRegistrations(confirmedNodes, b, bc.v2Config.ChainID)
			continue
		}
		result, err := bc.executeBlockV2(st, engine, validators, cloneNodeIdentityMap(confirmedNodes), b, blocks[:b.Index])
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("block #%d V2 execution: %w", b.Index, err)
		}
		if err := checkEVMCommitments(b, result.state); err != nil {
			return nil, nil, nil, nil, nil, err
		}
		if !strings.EqualFold(result.state.Root(), b.HBTStateRoot) ||
			!strings.EqualFold(result.hvm.State().Root(), b.HVMStateRoot) ||
			!strings.EqualFold(hvm.ReceiptsRoot(result.receipts), b.ReceiptsRoot) ||
			!strings.EqualFold(result.validatorSet.Root(), b.ValidatorSetRoot) ||
			!strings.EqualFold(result.validators.Root(), b.ValidatorStateRoot) {
			return nil, nil, nil, nil, nil, fmt.Errorf("block #%d V2 state/receipt/validator commitment mismatch", b.Index)
		}
		if bc.v2Config.ConsensusEnabledAt(b.Index) {
			if err := bc.validateConsensusProposal(b, result.validatorSet, true); err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("block #%d finality: %w", b.Index, err)
			}
		}
		st = result.state
		if b.Index >= len(blocks)-evmReadHistoryLimit {
			history.remember(b, st)
		}
		engine = result.hvm
		validators = result.validators
		applyNodeRegistrations(confirmedNodes, b, bc.v2Config.ChainID)
		for _, r := range result.receipts {
			receipts[strings.ToLower(strings.TrimPrefix(r.TxID, "0x"))] = r
		}
  if bc.checkpointStartup && b.Index == len(blocks)-evmReadHistoryLimit-1 {
   if err := bc.saveRecoveryCheckpoint(b.Index, st, engine, validators, receipts); err != nil { log.Printf("HVM_CHECKPOINT_WRITE_SKIPPED reason=%v", err) }
  }
 }
 return st, engine, validators, receipts, history, nil
}
