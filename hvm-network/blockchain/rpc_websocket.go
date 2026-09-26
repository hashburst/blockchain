package blockchain

import (
	"github.com/gorilla/websocket"
	"net/http"
	"time"
)

// ServeReadOnlyWebSocket exposes a bounded request/response transport.
// Ethereum subscriptions and transaction submission are deliberately unsupported.
func (h *RPCHandler) ServeReadOnlyWebSocket(w http.ResponseWriter, r *http.Request) {
	u := websocket.Upgrader{HandshakeTimeout: 3 * time.Second}
	c, err := u.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	c.SetReadLimit(64 << 10)
	for count := 0; count < 256; count++ {
		_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))
		var req rpcRequest
		if c.ReadJSON(&req) != nil {
			return
		}
		resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
		if req.JSONRPC != "2.0" || len(req.ID) == 0 {
			resp.Error = &rpcError{Code: -32600, Message: "JSON-RPC 2.0 request with id required"}
		} else {
			switch req.Method {
			case "eth_chainId", "net_version", "eth_blockNumber", "hb_getFinalizedHeight", "hb_getFinalizedCommitment", "web3_clientVersion":
				resp.Result, resp.Error = h.dispatch(&req)
			default:
				resp.Error = &rpcError{Code: -32601, Message: "method unavailable on read-only WebSocket"}
			}
		}
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if c.WriteJSON(resp) != nil {
			return
		}
	}
}
