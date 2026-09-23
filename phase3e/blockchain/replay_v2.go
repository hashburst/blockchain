package blockchain

import (
	"fmt"
	"strings"

	"hashburst/consensus"
	"hashburst/hvm"
)

func (bc *Blockchain) computeProjections(blocks []*Block) (*State, *hvm.Engine, *consensus.Registry, map[string]hvm.Receipt, error) {
	st := NewState()
	engine := hvm.NewEngine(nil, bc.v2Config.FeePolicy)
	validators := consensus.NewRegistry(bc.v2Config.Validator)
	receipts := make(map[string]hvm.Receipt)
	confirmedNodes := make(map[string]ConfirmedNodeIdentity)
	for _, b := range blocks {
		if b.EffectiveVersion() < BlockVersionV2 {
			if err := st.ApplyBlock(b); err != nil {
				return nil, nil, nil, nil, fmt.Errorf("block #%d native state: %w", b.Index, err)
			}
			applyNodeRegistrations(confirmedNodes, b, bc.v2Config.ChainID)
			continue
		}
		result, err := bc.executeBlockV2(st, engine, validators, cloneNodeIdentityMap(confirmedNodes), b)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("block #%d V2 execution: %w", b.Index, err)
		}
		if !strings.EqualFold(result.state.Root(), b.HBTStateRoot) ||
			!strings.EqualFold(result.hvm.State().Root(), b.HVMStateRoot) ||
			!strings.EqualFold(hvm.ReceiptsRoot(result.receipts), b.ReceiptsRoot) ||
			!strings.EqualFold(result.validatorSet.Root(), b.ValidatorSetRoot) ||
			!strings.EqualFold(result.validators.Root(), b.ValidatorStateRoot) {
			return nil, nil, nil, nil, fmt.Errorf("block #%d V2 state/receipt/validator commitment mismatch", b.Index)
		}
		if bc.v2Config.ConsensusEnabledAt(b.Index) {
			if err := bc.validateConsensusProposal(b, result.validatorSet, true); err != nil {
				return nil, nil, nil, nil, fmt.Errorf("block #%d finality: %w", b.Index, err)
			}
		}
		st = result.state
		engine = result.hvm
		validators = result.validators
		applyNodeRegistrations(confirmedNodes, b, bc.v2Config.ChainID)
		for _, r := range result.receipts {
			receipts[strings.ToLower(strings.TrimPrefix(r.TxID, "0x"))] = r
		}
	}
	return st, engine, validators, receipts, nil
}
