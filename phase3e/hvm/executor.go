package hvm

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"hashburst/protocolv2"
	"hashburst/wallet"
)

type NativeContract interface {
	TypeID() string
	Deploy(ctx ExecutionContext, state *StateDB, contractAddress string, init json.RawMessage, meter *Meter) ([]Event, error)
	Call(ctx ExecutionContext, state *StateDB, contractAddress, method string, args json.RawMessage, meter *Meter) ([]byte, []Event, error)
}

type Engine struct {
	state     *StateDB
	standards *StandardsRegistry
	natives   map[string]NativeContract
	feePolicy protocolv2.FeePolicy
}

func NewEngine(state *StateDB, feePolicy protocolv2.FeePolicy) *Engine {
	if state == nil {
		state = NewStateDB()
	}
	e := &Engine{
		state:     state,
		standards: NewStandardsRegistry(),
		natives:   make(map[string]NativeContract),
		feePolicy: feePolicy,
	}
	e.RegisterNative(NewMiningPayoutRegistry())
	return e
}

func (e *Engine) State() *StateDB                 { return e.state }
func (e *Engine) Standards() *StandardsRegistry   { return e.standards }
func (e *Engine) FeePolicy() protocolv2.FeePolicy { return e.feePolicy }

// Clone creates an isolated execution engine over a cloned state. Contract
// implementations are stateless descriptors, so they may safely be shared.
func (e *Engine) Clone() *Engine {
	out := &Engine{
		state:     e.state.Clone(),
		standards: e.standards,
		natives:   make(map[string]NativeContract, len(e.natives)),
		feePolicy: e.feePolicy,
	}
	for k, v := range e.natives {
		out.natives[k] = v
	}
	return out
}

func (e *Engine) ReplaceStateWith(other *Engine) {
	if other == nil {
		return
	}
	e.state.ReplaceWith(other.state)
}

func (e *Engine) RegisterNative(c NativeContract) {
	if c == nil {
		return
	}
	e.natives[strings.ToUpper(c.TypeID())] = c
}

func (e *Engine) HasContract(address string) bool {
	_, ok := e.state.Get(contractMetaKey(address, "type"))
	return ok
}

type DeployRequest struct {
	ContractType string          `json:"contract_type"`
	Salt         string          `json:"salt,omitempty"`
	Init         json.RawMessage `json:"init"`
}

func (e *Engine) Deploy(ctx ExecutionContext, req DeployRequest) Receipt {
	meter := NewMeter(ctx.ComputeLimit)
	receipt := Receipt{TxID: ctx.TxID, Success: false}
	finish := func() Receipt {
		receipt.ComputeUsed = meter.Used
		fee, err := e.feePolicy.ComputeFee(meter.Used)
		if err != nil {
			receipt.Success = false
			if receipt.RevertReason == "" {
				receipt.RevertReason = err.Error()
			}
		} else {
			receipt.FeeUnits = fee
		}
		return receipt
	}

	if err := meter.Consume(ComputeBaseDeploy + uint64(len(req.Init))*ComputePerPayloadByte); err != nil {
		receipt.RevertReason = err.Error()
		return finish()
	}
	c, ok := e.natives[strings.ToUpper(strings.TrimSpace(req.ContractType))]
	if !ok {
		receipt.RevertReason = "unknown native contract type"
		return finish()
	}
	address := GenerateContractAddress(ctx.ChainID, ctx.Sender, ctx.TxID, req.Salt)
	receipt.Contract = address
	if _, exists := e.state.Get(contractMetaKey(address, "type")); exists {
		receipt.RevertReason = "contract address already deployed"
		return finish()
	}

	shadow := e.state.Clone()
	shadow.Set(contractMetaKey(address, "type"), []byte(c.TypeID()))
	shadow.Set(contractMetaKey(address, "creator"), []byte(normalizeAddress(ctx.Sender)))
	if err := meter.Consume(2 * ComputeStateWrite); err != nil {
		receipt.RevertReason = err.Error()
		return finish()
	}
	events, err := c.Deploy(ctx, shadow, address, req.Init, meter)
	if err != nil {
		receipt.RevertReason = err.Error()
		return finish()
	}
	e.state.ReplaceWith(shadow)
	receipt.Success = true
	receipt.Events = events
	receipt.ReturnData, _ = json.Marshal(map[string]string{"address": address})
	return finish()
}

type CallRequest struct {
	Address string          `json:"address"`
	Method  string          `json:"method"`
	Args    json.RawMessage `json:"args"`
}

func (e *Engine) Call(ctx ExecutionContext, req CallRequest) Receipt {
	meter := NewMeter(ctx.ComputeLimit)
	receipt := Receipt{TxID: ctx.TxID, Contract: req.Address, Success: false}
	finish := func() Receipt {
		receipt.ComputeUsed = meter.Used
		fee, err := e.feePolicy.ComputeFee(meter.Used)
		if err != nil {
			receipt.Success = false
			if receipt.RevertReason == "" {
				receipt.RevertReason = err.Error()
			}
		} else {
			receipt.FeeUnits = fee
		}
		return receipt
	}

	if err := meter.Consume(ComputeBaseCall + uint64(len(req.Args))*ComputePerPayloadByte); err != nil {
		receipt.RevertReason = err.Error()
		return finish()
	}
	typeBytes, ok := e.state.Get(contractMetaKey(req.Address, "type"))
	if !ok {
		receipt.RevertReason = "contract not found"
		return finish()
	}
	c, ok := e.natives[strings.ToUpper(string(typeBytes))]
	if !ok {
		receipt.RevertReason = "contract runtime not available"
		return finish()
	}

	shadow := e.state.Clone()
	ret, events, err := c.Call(ctx, shadow, req.Address, req.Method, req.Args, meter)
	if err != nil {
		receipt.RevertReason = err.Error()
		return finish()
	}
	e.state.ReplaceWith(shadow)
	receipt.Success = true
	receipt.Events = events
	receipt.ReturnData = ret
	return finish()
}

func contractMetaKey(address, name string) string {
	return "contract/" + normalizeAddress(address) + "/meta/" + name
}

func GenerateContractAddress(chainID uint64, creator, txID, salt string) string {
	h := sha256.New()
	h.Write([]byte("HASHBURST_HVM_CONTRACT_V1"))
	var x [8]byte
	binary.BigEndian.PutUint64(x[:], chainID)
	h.Write(x[:])
	h.Write([]byte(normalizeAddress(creator)))
	h.Write([]byte(strings.ToLower(strings.TrimPrefix(txID, "0x"))))
	h.Write([]byte(salt))
	sum := h.Sum(nil)
	return wallet.ToChecksumAddress(sum[:20])
}

func hashID(domain string, parts ...string) string {
	h := sha256.New()
	h.Write([]byte(domain))
	for _, p := range parts {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(p)))
		h.Write(n[:])
		h.Write([]byte(p))
	}
	return "0x" + hex.EncodeToString(h.Sum(nil))
}

func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("json marshal invariant failed: %v", err))
	}
	return b
}
