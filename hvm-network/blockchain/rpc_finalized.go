package blockchain

import (
	"encoding/json"
	"hashburst/consensus"
)

// FinalizedCommitment identifies immutable state at one explicit BFT height.
// Certificates may contain different valid signer subsets; compare commitments,
// not their byte encodings, across nodes.
type FinalizedCommitment struct {
	ChainID          uint64                       `json:"chain_id"`
	Height           int                          `json:"height"`
	Hash             string                       `json:"hash"`
	ParentHash       string                       `json:"parent_hash"`
	HBTStateRoot     string                       `json:"hbt_state_root"`
	HVMStateRoot     string                       `json:"hvm_state_root"`
	ReceiptsRoot     string                       `json:"receipts_root"`
	ValidatorSetRoot string                       `json:"validator_set_root"`
	Certificate      *consensus.QuorumCertificate `json:"certificate"`
}

func (h *RPCHandler) getFinalizedCommitment(params []json.RawMessage) (interface{}, *rpcError) {
	if len(params) != 1 {
		return nil, &rpcError{Code: -32602, Message: "one explicit height required"}
	}
	var height uint64
	if json.Unmarshal(params[0], &height) != nil || height > uint64(^uint(0)>>1) {
		return nil, &rpcError{Code: -32602, Message: "height must be a nonnegative integer"}
	}
	h.bc.mu.RLock()
	defer h.bc.mu.RUnlock()
	if height >= uint64(len(h.bc.Blocks)) {
		return nil, &rpcError{Code: -32001, Message: "finalized block unavailable"}
	}
	b := h.bc.Blocks[int(height)]
	if b == nil || b.Index != int(height) || !h.bc.v2Config.ConsensusEnabledAt(b.Index) || b.FinalityCertificate == nil {
		return nil, &rpcError{Code: -32001, Message: "BFT finalized block unavailable"}
	}
	// Deep clone while locked: the response must not alias mutable chain state.
	copy := cloneBlockForConsensus(b)
	return FinalizedCommitment{copy.ProtocolChainID, copy.Index, copy.Hash, copy.PrevHash,
		copy.HBTStateRoot, copy.HVMStateRoot, copy.ReceiptsRoot, copy.ValidatorSetRoot,
		copy.FinalityCertificate}, nil
}
