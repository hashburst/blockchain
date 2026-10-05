package blockchain

import (
	"fmt"
	"log"
	"strings"

	"hashburst/consensus"
	"hashburst/hvm"
)

func (bc *Blockchain) computeProjections(blocks []*Block) (*State, *hvm.Engine, *consensus.Registry, map[string]hvm.Receipt, *evmReadHistory, error) {
	return bc.computeProjectionsFrom(len(blocks), func(n int) (*Block, error) { return blocks[n], nil })
}
func (bc *Blockchain) computeProjectionsFrom(count int, get func(int) (*Block, error)) (*State, *hvm.Engine, *consensus.Registry, map[string]hvm.Receipt, *evmReadHistory, error) {
	st := initialProtocolState(bc.v2Config)
	history := &evmReadHistory{}
	engine := hvm.NewEngine(nil, bc.v2Config.FeePolicy)
	validators := consensus.NewRegistry(bc.v2Config.Validator)
	receipts := make(map[string]hvm.Receipt)
	confirmedNodes := make(map[string]ConfirmedNodeIdentity)
	start := 0
	if seed := bc.startupSeed; seed != nil {
		st, engine, validators, receipts = seed.state, seed.engine, seed.validators, seed.receipts
		start = seed.height + 1
		confirmedNodes = cloneNodeIdentityMap(seed.nodes)
	}
	ancestors := make([]*Block, 0, 257)
	for n := max(0, start-256); n < start; n++ {
		b, e := get(n)
		if e != nil {
			return nil, nil, nil, nil, nil, e
		}
		ancestors = append(ancestors, &Block{Index: b.Index, Hash: b.Hash})
	}
	for n := start; n < count; n++ {
		b, e := get(n)
		if e != nil {
			return nil, nil, nil, nil, nil, e
		}
		if n == 0 && bc.v2Config.GenesisImport != nil {
			if !validProtocolGenesis(b, bc.v2Config) {
				return nil, nil, nil, nil, nil, fmt.Errorf("mainnet import genesis mismatch")
			}
			ancestors = appendAncestor(ancestors, b)
			continue
		}
		if b.Index > 0 && b.Index%5000 == 0 {
			log.Printf("HVM_REPLAY_PROGRESS height=%d total=%d", b.Index, count)
		}
		if b.EffectiveVersion() < BlockVersionV2 {
			if err := st.ApplyBlock(b); err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("block #%d native state: %w", b.Index, err)
			}
			applyNodeRegistrations(confirmedNodes, b, bc.v2Config.ChainID)
			ancestors = appendAncestor(ancestors, b)
			continue
		}
		result, err := bc.executeBlockV2(st, engine, validators, cloneNodeIdentityMap(confirmedNodes), b, ancestors)
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
		if b.Index >= count-evmReadHistoryLimit {
			history.remember(b, st)
		}
		engine = result.hvm
		validators = result.validators
		applyNodeRegistrations(confirmedNodes, b, bc.v2Config.ChainID)
		for _, r := range result.receipts {
			receipts[strings.ToLower(strings.TrimPrefix(r.TxID, "0x"))] = r
		}
		ancestors = appendAncestor(ancestors, b)
		if bc.checkpointStartup && bc.startupSeed == nil && b.Index == count-evmReadHistoryLimit-1 {
			if err := bc.saveRecoveryCheckpoint(b.Index, st, engine, validators, receipts, confirmedNodes); err != nil {
				log.Printf("HVM_CHECKPOINT_WRITE_SKIPPED reason=%v", err)
			}
		}
	}
	if bc.history != nil {
		bc.confirmedNodes = confirmedNodes
	}
	return st, engine, validators, receipts, history, nil
}

func appendAncestor(window []*Block, b *Block) []*Block {
	b = &Block{Index: b.Index, Hash: b.Hash}
	if len(window) == 256 {
		copy(window, window[1:])
		window[255] = b
		return window
	}
	return append(window, b)
}
