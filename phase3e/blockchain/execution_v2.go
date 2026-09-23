package blockchain

import (
	"encoding/json"
	"fmt"
	"strings"

	"hashburst/consensus"
	"hashburst/hvm"
	"hashburst/protocolv2"
)

type blockExecutionV2 struct {
	state        *State
	hvm          *hvm.Engine
	validators   *consensus.Registry
	validatorSet consensus.ValidatorSet
	receipts     []hvm.Receipt
}

type contractCallPayload struct {
	Method string          `json:"method"`
	Args   json.RawMessage `json:"args"`
}

type validatorIDPayload struct {
	ValidatorID string `json:"validator_id"`
}

const (
	computeValidatorRegister = uint64(80_000)
	computeValidatorUnbond   = uint64(25_000)
	computeValidatorWithdraw = uint64(35_000)
	computeValidatorEvidence = uint64(120_000)
)

func (bc *Blockchain) executeBlockV2(baseState *State, baseHVM *hvm.Engine, baseValidators *consensus.Registry, confirmedNodes map[string]ConfirmedNodeIdentity, b *Block) (*blockExecutionV2, error) {
	if baseState == nil || baseHVM == nil || baseValidators == nil {
		return nil, fmt.Errorf("missing execution projection")
	}
	stateShadow := baseState.Clone()
	hvmShadow := baseHVM.Clone()
	validatorShadow := baseValidators.Clone()
	validatorShadow.AdvanceHeight(uint64(b.Index))
	preSet := validatorShadow.ActiveSet(uint64(b.Index))

	// V1 transactions (including the compatibility system reward) are still
	// applied first. In consensus mode the reward address is constrained by the
	// scheduled proposer before this execution is accepted.
	if err := stateShadow.ApplyBlock(b); err != nil {
		return nil, err
	}

	receipts := make([]hvm.Receipt, 0, len(b.TransactionsV2))
	for i, tx := range b.TransactionsV2 {
		if err := bc.validateV2Transaction(tx); err != nil {
			return nil, fmt.Errorf("v2 tx %d: %w", i, err)
		}
		if err := stateShadow.CanStartV2(tx); err != nil {
			return nil, fmt.Errorf("v2 tx %d precondition: %w", i, err)
		}

		ctx := hvm.ExecutionContext{
			TxID:         tx.HashHex(),
			ChainID:      tx.ChainID,
			Sender:       tx.Sender,
			ValueUnits:   tx.ValueUnits,
			BlockHeight:  uint64(b.Index),
			BlockTime:    uint64(b.Timestamp.Unix()),
			ComputeLimit: tx.ComputeLimit,
		}

		var receipt hvm.Receipt
		valueRecipient := ""
		applyValue := false

		switch tx.Type {
		case protocolv2.TxHBTTransfer:
			fee, err := bc.v2Config.FeePolicy.ComputeFee(hvm.ComputeBaseTransfer)
			if err != nil {
				return nil, fmt.Errorf("v2 tx %d transfer fee: %w", i, err)
			}
			receipt = hvm.Receipt{TxID: tx.HashHex(), Success: true, ComputeUsed: hvm.ComputeBaseTransfer, FeeUnits: fee}
			valueRecipient = tx.To
			applyValue = true

		case protocolv2.TxContractDeploy:
			var req hvm.DeployRequest
			if err := json.Unmarshal(tx.Data, &req); err != nil {
				return nil, fmt.Errorf("v2 tx %d deploy payload: %w", i, err)
			}
			receipt = hvmShadow.Deploy(ctx, req)
			if receipt.Success {
				valueRecipient = receipt.Contract
				applyValue = true
			}

		case protocolv2.TxContractCall:
			var payload contractCallPayload
			if err := json.Unmarshal(tx.Data, &payload); err != nil {
				return nil, fmt.Errorf("v2 tx %d call payload: %w", i, err)
			}
			if strings.TrimSpace(payload.Method) == "" {
				return nil, fmt.Errorf("v2 tx %d call method required", i)
			}
			receipt = hvmShadow.Call(ctx, hvm.CallRequest{Address: tx.To, Method: payload.Method, Args: payload.Args})
			if receipt.Success {
				valueRecipient = tx.To
				applyValue = true
			}

		case protocolv2.TxValidatorRegister:
			fee, err := bc.v2Config.FeePolicy.ComputeFee(computeValidatorRegister)
			if err != nil {
				return nil, err
			}
			receipt = hvm.Receipt{TxID: tx.HashHex(), ComputeUsed: computeValidatorRegister, FeeUnits: fee}
			var req consensus.RegisterRequest
			if err := json.Unmarshal(tx.Data, &req); err != nil {
				receipt.RevertReason = "invalid validator registration payload"
				break
			}
			if req.BondUnits != tx.ValueUnits {
				receipt.RevertReason = "validator bond must equal transaction value_units"
				break
			}
			if err := validateValidatorNodeBinding(tx.Sender, req, confirmedNodes, bc.v2Config.ChainID, bc.v2Config.RequireNodeRegistrationV2); err != nil {
				receipt.RevertReason = err.Error()
				break
			}
			candidate := validatorShadow.Clone()
			v, err := candidate.Register(tx.Sender, req, uint64(b.Index))
			if err != nil {
				receipt.RevertReason = err.Error()
				break
			}
			validatorShadow = candidate
			receipt.Success = true
			valueRecipient = v.EscrowAddress
			applyValue = true
			receipt.ReturnData = mustConsensusJSON(v)
			receipt.Events = []hvm.Event{{Name: "ValidatorRegistered", Topics: []string{v.ID, v.NodeID, v.OperatorAddress}, Data: receipt.ReturnData}}

		case protocolv2.TxValidatorUnbond:
			fee, err := bc.v2Config.FeePolicy.ComputeFee(computeValidatorUnbond)
			if err != nil {
				return nil, err
			}
			receipt = hvm.Receipt{TxID: tx.HashHex(), ComputeUsed: computeValidatorUnbond, FeeUnits: fee}
			var req validatorIDPayload
			if err := json.Unmarshal(tx.Data, &req); err != nil {
				receipt.RevertReason = "invalid validator unbond payload"
				break
			}
			candidate := validatorShadow.Clone()
			v, err := candidate.BeginUnbond(req.ValidatorID, tx.Sender, uint64(b.Index))
			if err != nil {
				receipt.RevertReason = err.Error()
				break
			}
			validatorShadow = candidate
			receipt.Success = true
			receipt.ReturnData = mustConsensusJSON(v)
			receipt.Events = []hvm.Event{{Name: "ValidatorUnbonding", Topics: []string{v.ID}, Data: receipt.ReturnData}}

		case protocolv2.TxValidatorWithdraw:
			fee, err := bc.v2Config.FeePolicy.ComputeFee(computeValidatorWithdraw)
			if err != nil {
				return nil, err
			}
			receipt = hvm.Receipt{TxID: tx.HashHex(), ComputeUsed: computeValidatorWithdraw, FeeUnits: fee}
			var req validatorIDPayload
			if err := json.Unmarshal(tx.Data, &req); err != nil {
				receipt.RevertReason = "invalid validator withdraw payload"
				break
			}
			candidate := validatorShadow.Clone()
			v, amount, err := candidate.Withdraw(req.ValidatorID, tx.Sender, uint64(b.Index))
			if err != nil {
				receipt.RevertReason = err.Error()
				break
			}
			if err := stateShadow.ProtocolTransfer(v.EscrowAddress, v.OperatorAddress, amount); err != nil {
				return nil, fmt.Errorf("validator withdrawal state mismatch: %w", err)
			}
			validatorShadow = candidate
			receipt.Success = true
			receipt.ReturnData = mustConsensusJSON(map[string]interface{}{"validator": v, "withdrawn_units": amount})
			receipt.Events = []hvm.Event{{Name: "ValidatorWithdrawn", Topics: []string{v.ID}, Data: receipt.ReturnData}}

		case protocolv2.TxValidatorEvidence:
			fee, err := bc.v2Config.FeePolicy.ComputeFee(computeValidatorEvidence)
			if err != nil {
				return nil, err
			}
			receipt = hvm.Receipt{TxID: tx.HashHex(), ComputeUsed: computeValidatorEvidence, FeeUnits: fee}
			candidate := validatorShadow.Clone()

			// Phase 3D evidence is same-height, same-round, same-step equivocation.
			// Keep legacy Phase 3C evidence decoding for historical test blocks, but
			// never classify legitimate cross-round view-change votes as slashable.
			var probe struct {
				Step consensus.BFTStep `json:"step"`
			}
			if err := json.Unmarshal(tx.Data, &probe); err != nil {
				receipt.RevertReason = "invalid validator evidence payload"
				break
			}
			var v consensus.Validator
			var slashed int64
			if probe.Step != "" {
				var ev consensus.BFTDoubleSignEvidence
				if err := json.Unmarshal(tx.Data, &ev); err != nil {
					receipt.RevertReason = "invalid BFT evidence payload"
					break
				}
				v, slashed, err = candidate.ApplyBFTDoubleSignEvidence(ev, uint64(b.Index))
			} else {
				var ev consensus.DoubleVoteEvidence
				if err := json.Unmarshal(tx.Data, &ev); err != nil {
					receipt.RevertReason = "invalid legacy validator evidence payload"
					break
				}
				v, slashed, err = candidate.ApplyDoubleVoteEvidence(ev, uint64(b.Index))
			}
			if err != nil {
				receipt.RevertReason = err.Error()
				break
			}
			if err := stateShadow.ProtocolTransfer(v.EscrowAddress, bc.v2Config.SlashCollector, slashed); err != nil {
				return nil, fmt.Errorf("validator slash state mismatch: %w", err)
			}
			validatorShadow = candidate
			receipt.Success = true
			receipt.ReturnData = mustConsensusJSON(map[string]interface{}{"validator": v, "slashed_units": slashed, "evidence_step": probe.Step})
			receipt.Events = []hvm.Event{{Name: "ValidatorSlashed", Topics: []string{v.ID, string(probe.Step)}, Data: receipt.ReturnData}}

		default:
			return nil, fmt.Errorf("v2 tx %d type %s not executable in Phase 3C", i, tx.Type.String())
		}

		// A reverted HVM/protocol transaction is still a valid chain transaction:
		// it consumes sequence and actual fee but not ValueUnits or reverted state.
		if err := stateShadow.SettleV2(tx, receipt.FeeUnits, bc.v2Config.FeeCollector, valueRecipient, applyValue && receipt.Success); err != nil {
			return nil, fmt.Errorf("v2 tx %d settlement: %w", i, err)
		}
		receipts = append(receipts, receipt)
	}

	return &blockExecutionV2{state: stateShadow, hvm: hvmShadow, validators: validatorShadow, validatorSet: preSet, receipts: receipts}, nil
}

func mustConsensusJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

func (bc *Blockchain) validateV2Transaction(tx *protocolv2.TransactionV2) error {
	if tx == nil {
		return fmt.Errorf("nil transaction")
	}
	if len(tx.Data) > bc.v2Config.MaxTxDataBytes {
		return fmt.Errorf("data too large: %d > %d", len(tx.Data), bc.v2Config.MaxTxDataBytes)
	}
	if err := tx.Verify(bc.v2Config.ChainID); err != nil {
		return err
	}
	if tx.IsSystem() {
		return fmt.Errorf("V2 system transaction types are reserved for a later consensus phase")
	}
	if !isProtocolV2ExecutableTxType(tx.Type) {
		return fmt.Errorf("transaction type %s is reserved but not active in Protocol V2", tx.Type.String())
	}
	maxRequired, err := bc.v2Config.FeePolicy.MaxFeeForLimit(tx.ComputeLimit)
	if err != nil {
		return err
	}
	switch tx.Type {
	case protocolv2.TxHBTTransfer:
		maxRequired, err = bc.v2Config.FeePolicy.ComputeFee(hvm.ComputeBaseTransfer)
	case protocolv2.TxValidatorRegister:
		maxRequired, err = bc.v2Config.FeePolicy.ComputeFee(computeValidatorRegister)
	case protocolv2.TxValidatorUnbond:
		maxRequired, err = bc.v2Config.FeePolicy.ComputeFee(computeValidatorUnbond)
	case protocolv2.TxValidatorWithdraw:
		maxRequired, err = bc.v2Config.FeePolicy.ComputeFee(computeValidatorWithdraw)
	case protocolv2.TxValidatorEvidence:
		maxRequired, err = bc.v2Config.FeePolicy.ComputeFee(computeValidatorEvidence)
	}
	if err != nil {
		return err
	}
	if tx.MaxFeeUnits < maxRequired {
		return fmt.Errorf("max_fee_units %d below required reservation %d", tx.MaxFeeUnits, maxRequired)
	}
	return nil
}

func isProtocolV2ExecutableTxType(t protocolv2.TxType) bool {
	switch t {
	case protocolv2.TxHBTTransfer,
		protocolv2.TxContractDeploy,
		protocolv2.TxContractCall,
		protocolv2.TxValidatorRegister,
		protocolv2.TxValidatorUnbond,
		protocolv2.TxValidatorWithdraw,
		protocolv2.TxValidatorEvidence:
		return true
	default:
		return false
	}
}

func (bc *Blockchain) prepareV2Commitments(b *Block) (*blockExecutionV2, error) {
	result, err := bc.executeBlockV2(bc.state, bc.hvmEngine, bc.validators, nodeIdentityProjection(bc.Blocks, bc.v2Config.ChainID), b)
	if err != nil {
		return nil, err
	}
	b.HBTStateRoot = result.state.Root()
	b.HVMStateRoot = result.hvm.State().Root()
	b.ReceiptsRoot = hvm.ReceiptsRoot(result.receipts)
	b.ValidatorSetRoot = result.validatorSet.Root()
	b.ValidatorStateRoot = result.validators.Root()
	return result, nil
}

func (bc *Blockchain) validateV2Commitments(b *Block) (*blockExecutionV2, error) {
	result, err := bc.executeBlockV2(bc.state, bc.hvmEngine, bc.validators, nodeIdentityProjection(bc.Blocks, bc.v2Config.ChainID), b)
	if err != nil {
		return nil, err
	}
	if got := result.state.Root(); !strings.EqualFold(got, b.HBTStateRoot) {
		return nil, fmt.Errorf("HBT state root mismatch: got %s want %s", got, b.HBTStateRoot)
	}
	if got := result.hvm.State().Root(); !strings.EqualFold(got, b.HVMStateRoot) {
		return nil, fmt.Errorf("HVM state root mismatch: got %s want %s", got, b.HVMStateRoot)
	}
	if got := hvm.ReceiptsRoot(result.receipts); !strings.EqualFold(got, b.ReceiptsRoot) {
		return nil, fmt.Errorf("receipts root mismatch: got %s want %s", got, b.ReceiptsRoot)
	}
	if got := result.validatorSet.Root(); !strings.EqualFold(got, b.ValidatorSetRoot) {
		return nil, fmt.Errorf("validator set root mismatch: got %s want %s", got, b.ValidatorSetRoot)
	}
	if got := result.validators.Root(); !strings.EqualFold(got, b.ValidatorStateRoot) {
		return nil, fmt.Errorf("validator state root mismatch: got %s want %s", got, b.ValidatorStateRoot)
	}
	return result, nil
}
