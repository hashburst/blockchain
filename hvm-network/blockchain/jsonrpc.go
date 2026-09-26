package blockchain

// jsonrpc.go — HashBurst native JSON-RPC with a small Ethereum-read
// compatibility surface. Phase 3B adds signed TransactionV2 submission and HVM
// read/estimation methods; it still does NOT claim Ethereum RLP compatibility.

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"

	"hashburst/protocolv2"
)

type rpcRequest struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      json.RawMessage   `json:"id"`
	Method  string            `json:"method"`
	Params  []json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type TxV2Broadcaster interface {
	GossipTxV2(tx *protocolv2.TransactionV2)
}

type RPCHandler struct {
	bc            *Blockchain
	mp            *Mempool
	chainID       int64
	v2Broadcaster TxV2Broadcaster
}

func NewRPCHandler(bc *Blockchain, mp *Mempool, chainID int64) *RPCHandler {
	return &RPCHandler{bc: bc, mp: mp, chainID: chainID}
}

func (h *RPCHandler) SetV2Broadcaster(b TxV2Broadcaster) { h.v2Broadcaster = b }

func (h *RPCHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "solo POST", http.StatusMethodNotAllowed)
		return
	}

	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPCError(w, nil, -32700, "parse error")
		return
	}
	result, rpcErr := h.dispatch(&req)
	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	if rpcErr != nil {
		resp.Error = rpcErr
	} else {
		resp.Result = result
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *RPCHandler) dispatch(req *rpcRequest) (interface{}, *rpcError) {
	switch req.Method {
	case "eth_chainId":
		return hexUint(uint64(h.chainID)), nil
	case "net_version":
		return fmt.Sprintf("%d", h.chainID), nil
	case "eth_blockNumber":
		return hexUint(uint64(h.bc.Height())), nil
	case "eth_gasPrice":
		// HVM compute pricing is not Ethereum gas pricing. Keep this honest until
		// the EVM compatibility runtime defines a conversion layer.
		return "0x0", nil
	case "eth_getTransactionCount", "hb_getTransactionCount":
		return h.getTransactionCount(req.Params)
	case "eth_getBalance":
		return h.getBalance(req.Params)
	case "hb_sendTransaction":
		return h.sendTransaction(req.Params)
	case "hb_sendRawTransactionV2":
		return h.sendRawTransactionV2(req.Params)
	case "hb_getTransactionReceipt":
		return h.getTransactionReceipt(req.Params)
	case "hb_getHVMStateRoot":
		return h.bc.HVMStateRoot(), nil
	case "hb_getHBTStateRoot":
		return h.bc.HBTStateRoot(), nil
	case "hb_feePolicy":
		return h.bc.ProtocolV2Config(), nil
	case "hb_call":
		return h.callHVM(req.Params, false)
	case "hb_estimateCompute":
		return h.callHVM(req.Params, true)
	case "hb_getStandards":
		return h.bc.HVMEngine().Standards().List(), nil
	case "hb_getValidators":
		return h.getValidators(req.Params)
	case "hb_getValidatorSet":
		return h.getValidatorSet(req.Params)
	case "hb_getProposer":
		return h.getProposer(req.Params)
	case "hb_getFinalizedCommitment":
		return h.getFinalizedCommitment(req.Params)
	case "hb_getFinalizedHeight":
		return hexUint(uint64(h.bc.FinalizedHeight())), nil
	case "hb_getConsensusStatus":
		return h.getConsensusStatus(req.Params)
	case "hb_getConsensusNetworkStatus":
		return h.getConsensusNetworkStatus(req.Params)
	case "hb_getConsensusEvidence":
		return h.getConsensusEvidence(req.Params)
	case "web3_clientVersion":
		return "HashBurst/phase3d", nil
	default:
		return nil, &rpcError{Code: -32601, Message: "metodo non supportato: " + req.Method}
	}
}

func (h *RPCHandler) getBalance(params []json.RawMessage) (interface{}, *rpcError) {
	if len(params) < 1 {
		return nil, &rpcError{Code: -32602, Message: "manca l'indirizzo"}
	}
	var addr string
	if err := json.Unmarshal(params[0], &addr); err != nil {
		return nil, &rpcError{Code: -32602, Message: "indirizzo non valido"}
	}
	units := h.bc.BalanceUnits(addr)
	// Ethereum-read compatibility: 1 native HBT = 1e8 units, MetaMask displays
	// 18 decimals, so expose balance as a 1e18-scaled view only.
	wei := new(big.Int).Mul(big.NewInt(units), big.NewInt(10_000_000_000))
	return "0x" + wei.Text(16), nil
}

func (h *RPCHandler) getTransactionCount(params []json.RawMessage) (interface{}, *rpcError) {
	if len(params) < 1 {
		return nil, &rpcError{Code: -32602, Message: "manca l'indirizzo"}
	}
	var addr string
	if err := json.Unmarshal(params[0], &addr); err != nil {
		return nil, &rpcError{Code: -32602, Message: "indirizzo non valido"}
	}
	sequence := h.bc.Sequence(addr)
	if len(params) >= 2 {
		var tag string
		if err := json.Unmarshal(params[1], &tag); err != nil {
			return nil, &rpcError{Code: -32602, Message: "block tag non valido"}
		}
		switch strings.ToLower(strings.TrimSpace(tag)) {
		case "", "latest", "confirmed":
			// confirmed state only
		case "pending":
			// Admission guarantees same-sender pending sequences are contiguous.
			// Re-check here instead of blindly adding len() so RPC remains safe if
			// the mempool is ever populated by another internal path.
			expected := sequence
			for _, tx := range h.mp.PendingV2ForSender(addr) {
				if tx.Sequence != expected {
					break
				}
				expected++
			}
			sequence = expected
		default:
			return nil, &rpcError{Code: -32602, Message: "block tag supportato: latest|confirmed|pending"}
		}
	}
	return hexUint(sequence), nil
}

func (h *RPCHandler) sendTransaction(params []json.RawMessage) (interface{}, *rpcError) {
	if len(params) < 1 {
		return nil, &rpcError{Code: -32602, Message: "manca la transazione"}
	}
	var tx Transaction
	if err := json.Unmarshal(params[0], &tx); err != nil {
		return nil, &rpcError{Code: -32602, Message: "transazione non decodificabile"}
	}
	if err := tx.Verify(); err != nil {
		return nil, &rpcError{Code: -32000, Message: "firma non valida: " + err.Error()}
	}
	if !tx.IsSystem() && AmountToUnits(tx.Amount) > 0 && h.bc.BalanceUnits(tx.Sender) < AmountToUnits(tx.Amount) {
		return nil, &rpcError{Code: -32000, Message: "fondi insufficienti"}
	}
	h.mp.AddTransaction(&tx)
	return tx.ID, nil
}

func (h *RPCHandler) sendRawTransactionV2(params []json.RawMessage) (interface{}, *rpcError) {
	if len(params) < 1 {
		return nil, &rpcError{Code: -32602, Message: "manca la raw transaction V2"}
	}
	var raw string
	if err := json.Unmarshal(params[0], &raw); err != nil {
		return nil, &rpcError{Code: -32602, Message: "raw transaction V2 deve essere una stringa hex"}
	}
	tx, err := protocolv2.DecodeRawTransactionV2(raw)
	if err != nil {
		return nil, &rpcError{Code: -32602, Message: err.Error()}
	}
	if err := h.bc.AdmitTransactionV2(tx); err != nil {
		return nil, &rpcError{Code: -32000, Message: err.Error()}
	}
	if h.v2Broadcaster != nil {
		h.v2Broadcaster.GossipTxV2(tx)
	}
	return "0x" + tx.HashHex(), nil
}

func (h *RPCHandler) getTransactionReceipt(params []json.RawMessage) (interface{}, *rpcError) {
	if len(params) < 1 {
		return nil, &rpcError{Code: -32602, Message: "manca txid"}
	}
	var txid string
	if err := json.Unmarshal(params[0], &txid); err != nil {
		return nil, &rpcError{Code: -32602, Message: "txid non valido"}
	}
	r, ok := h.bc.Receipt(txid)
	if !ok {
		return nil, nil
	}
	return r, nil
}

type hvmCallRPC struct {
	Sender       string          `json:"sender"`
	Address      string          `json:"address"`
	Method       string          `json:"method"`
	Args         json.RawMessage `json:"args"`
	ComputeLimit uint64          `json:"compute_limit,omitempty"`
}

func (h *RPCHandler) callHVM(params []json.RawMessage, estimate bool) (interface{}, *rpcError) {
	if len(params) < 1 {
		return nil, &rpcError{Code: -32602, Message: "manca call object"}
	}
	var c hvmCallRPC
	if err := json.Unmarshal(params[0], &c); err != nil {
		return nil, &rpcError{Code: -32602, Message: "call object non valido"}
	}
	if strings.TrimSpace(c.Sender) == "" {
		c.Sender = "0x0000000000000000000000000000000000000000"
	}
	var r interface{}
	if estimate {
		receipt, err := h.bc.EstimateHVMCall(c.Sender, c.Address, c.Method, c.Args)
		if err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		r = map[string]interface{}{
			"success": receipt.Success, "compute_used": receipt.ComputeUsed,
			"fee_units": receipt.FeeUnits, "revert_reason": receipt.RevertReason,
		}
	} else {
		receipt, err := h.bc.SimulateHVMCall(c.Sender, c.Address, c.Method, c.Args, c.ComputeLimit)
		if err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		r = receipt
	}
	return r, nil
}

func hexUint(v uint64) string { return fmt.Sprintf("0x%x", v) }

func writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}})
}
