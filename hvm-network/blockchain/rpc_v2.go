package blockchain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"hashburst/consensus"
	"hashburst/hvm"
)

func (bc *Blockchain) SimulateHVMCall(sender, address, method string, args json.RawMessage, computeLimit uint64) (hvm.Receipt, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if computeLimit == 0 {
		computeLimit = 5_000_000
	}
	seed, _ := json.Marshal(struct {
		Sender  string          `json:"sender"`
		Address string          `json:"address"`
		Method  string          `json:"method"`
		Args    json.RawMessage `json:"args"`
	}{sender, address, method, args})
	sum := sha256.Sum256(seed)
	engine := bc.hvmEngine.Clone()
	head := bc.Blocks[len(bc.Blocks)-1]
	r := engine.Call(hvm.ExecutionContext{
		TxID:         hex.EncodeToString(sum[:]),
		ChainID:      bc.v2Config.ChainID,
		Sender:       sender,
		BlockHeight:  uint64(head.Index + 1),
		BlockTime:    uint64(head.Timestamp.Unix()),
		ComputeLimit: computeLimit,
	}, hvm.CallRequest{Address: address, Method: method, Args: args})
	return r, nil
}

func (bc *Blockchain) EstimateHVMCall(sender, address, method string, args json.RawMessage) (hvm.Receipt, error) {
	r, err := bc.SimulateHVMCall(sender, address, method, args, 10_000_000)
	if err != nil {
		return r, err
	}
	if r.ComputeUsed == 0 && r.RevertReason == "" {
		return r, fmt.Errorf("estimation returned zero compute")
	}
	return r, nil
}

func (h *RPCHandler) getValidators(params []json.RawMessage) (interface{}, *rpcError) {
	return h.bc.ValidatorRegistry().Snapshot(), nil
}

func (h *RPCHandler) getValidatorSet(params []json.RawMessage) (interface{}, *rpcError) {
	height := uint64(h.bc.Height() + 1)
	if len(params) > 0 {
		var raw interface{}
		if err := json.Unmarshal(params[0], &raw); err != nil {
			return nil, &rpcError{Code: -32602, Message: "validator set height non valido"}
		}
		v, err := rpcUint64(raw)
		if err != nil {
			return nil, &rpcError{Code: -32602, Message: err.Error()}
		}
		height = v
	}
	set := h.bc.CurrentValidatorSet(height)
	return map[string]interface{}{
		"height": height, "validator_set_root": set.Root(), "total_power": set.TotalPower(),
		"quorum_threshold": consensus.QuorumThreshold(set.TotalPower()), "validators": set.Validators, "power": set.Power,
	}, nil
}

func (h *RPCHandler) getProposer(params []json.RawMessage) (interface{}, *rpcError) {
	height := uint64(h.bc.Height() + 1)
	round := uint64(0)
	for i := 0; i < len(params) && i < 2; i++ {
		var raw interface{}
		if err := json.Unmarshal(params[i], &raw); err != nil {
			return nil, &rpcError{Code: -32602, Message: "height/round non valido"}
		}
		v, err := rpcUint64(raw)
		if err != nil {
			return nil, &rpcError{Code: -32602, Message: err.Error()}
		}
		if i == 0 {
			height = v
		} else {
			round = v
		}
	}
	set := h.bc.CurrentValidatorSet(height)
	p, ok := set.Proposer(height, round)
	if !ok {
		return nil, &rpcError{Code: -32000, Message: "nessun validator attivo"}
	}
	return map[string]interface{}{"height": height, "round": round, "validator_set_root": set.Root(), "proposer": p}, nil
}

func (h *RPCHandler) getConsensusStatus(params []json.RawMessage) (interface{}, *rpcError) {
	cfg := h.bc.ProtocolV2Config()
	head := h.bc.Height()
	next := head + 1
	set := h.bc.CurrentValidatorSet(uint64(next))
	result := map[string]interface{}{
		"protocol_v2_activation_height": cfg.ActivationHeight,
		"consensus_activation_height":   cfg.ConsensusActivationHeight,
		"consensus_active_next_height":  cfg.ConsensusEnabledAt(next),
		"head_height":                   head,
		"finalized_height":              h.bc.FinalizedHeight(),
		"validator_registry_root":       h.bc.ValidatorRegistry().Root(),
		"next_validator_set_root":       set.Root(),
		"next_validator_count":          len(set.Validators),
		"quorum_threshold":              consensus.QuorumThreshold(set.TotalPower()),
		"bft_sign_journal_healthy":      h.bc.bftJournalErr == nil && h.bc.bftJournal != nil,
		"double_sign_guard":             "persistent-per-height-round-step",
		"node_registration_v2_required": cfg.RequireNodeRegistrationV2,
	}
	if reactor, network, ok := h.bc.ConsensusRuntimeStatus(); ok {
		result["reactor"] = reactor
		result["network"] = network
	} else {
		result["reactor_attached"] = false
	}
	return result, nil
}

func (h *RPCHandler) getConsensusNetworkStatus(params []json.RawMessage) (interface{}, *rpcError) {
	reactor, network, ok := h.bc.ConsensusRuntimeStatus()
	if !ok {
		return map[string]interface{}{
			"attached":            false,
			"enabled_next_height": h.bc.ProtocolV2Config().ConsensusEnabledAt(h.bc.Height() + 1),
		}, nil
	}
	return map[string]interface{}{
		"attached": true,
		"reactor":  reactor,
		"network":  network,
	}, nil
}

func (h *RPCHandler) getConsensusEvidence(params []json.RawMessage) (interface{}, *rpcError) {
	return h.bc.ConsensusEvidence(), nil
}

func rpcUint64(v interface{}) (uint64, error) {
	switch x := v.(type) {
	case float64:
		if x < 0 || x != float64(uint64(x)) {
			return 0, fmt.Errorf("valore intero uint64 richiesto")
		}
		return uint64(x), nil
	case string:
		s := strings.TrimSpace(x)
		base := 10
		if strings.HasPrefix(strings.ToLower(s), "0x") {
			base = 16
			s = s[2:]
		}
		if s == "" {
			return 0, fmt.Errorf("valore vuoto")
		}
		v, err := strconv.ParseUint(s, base, 64)
		if err != nil {
			return 0, fmt.Errorf("uint64 non valido")
		}
		return v, nil
	default:
		return 0, fmt.Errorf("uint64 deve essere numero o stringa hex/decimale")
	}
}
