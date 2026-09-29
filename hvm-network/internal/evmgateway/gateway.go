// Package evmgateway limits public RPC to signed admission, queries and bounded
// finalized subscriptions. Node administrative and native signing APIs stay local.
package evmgateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const MaxRequest = 270000

var methods = map[string]bool{}

func init() {
	for _, m := range strings.Fields("eth_chainId net_version net_listening web3_clientVersion eth_blockNumber eth_getBalance eth_getTransactionCount eth_getCode eth_getStorageAt eth_call eth_estimateGas eth_getTransactionReceipt eth_getTransactionByHash eth_getBlockByNumber eth_getBlockByHash eth_gasPrice eth_maxPriorityFeePerGas eth_feeHistory eth_getLogs eth_sendRawTransaction") {
		methods[m] = true
	}
}

type request struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      json.RawMessage   `json:"id"`
	Method  string            `json:"method"`
	Params  []json.RawMessage `json:"params"`
}

func validate(raw []byte, ws bool) (request, error) {
	var r request
	if len(raw) > MaxRequest || json.Unmarshal(raw, &r) != nil || r.JSONRPC != "2.0" || len(r.ID) == 0 || len(r.ID) > 130 || bytes.Equal(r.ID, []byte("null")) {
		return r, fmt.Errorf("invalid JSON-RPC request; batches/notifications unsupported")
	}
	var id any
	if json.Unmarshal(r.ID, &id) != nil {
		return r, fmt.Errorf("invalid id")
	}
	switch id.(type) {
	case string, float64:
	default:
		return r, fmt.Errorf("invalid id")
	}
	if ws && r.Method == "eth_subscribe" {
		var kind string
		if len(r.Params) < 1 || len(r.Params) > 2 || json.Unmarshal(r.Params[0], &kind) != nil || (kind != "newHeads" && kind != "logs") {
			return r, fmt.Errorf("unsupported subscription")
		}
		return r, nil
	}
	if ws && r.Method == "eth_unsubscribe" && len(r.Params) == 1 {
		return r, nil
	}
	if !methods[r.Method] || (ws && r.Method == "eth_sendRawTransaction") {
		return r, fmt.Errorf("method unavailable")
	}
	return r, nil
}

type Gateway struct {
	HTTPURL, WSURL, Origin string
	slots                  chan struct{}
	client                 *http.Client
}

func New(httpURL, wsURL, origin string) *Gateway {
	return &Gateway{httpURL, wsURL, origin, make(chan struct{}, 16), &http.Client{Timeout: 12 * time.Second, Transport: &http.Transport{Proxy: nil, MaxConnsPerHost: 16}}}
}
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	origin := r.Header.Get("Origin")
	if r.URL.Path == "/ws" && origin != "" && origin != g.Origin {
		http.Error(w, "origin denied", 403)
		return
	}
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Vary", "Origin")
	}
	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(204)
		return
	}
	select {
	case g.slots <- struct{}{}:
		defer func() { <-g.slots }()
	default:
		http.Error(w, "busy", 503)
		return
	}
	if r.URL.Path == "/ws" && r.Method == "GET" {
		g.websocket(w, r)
		return
	}
	if r.URL.Path != "/rpc" || r.Method != "POST" {
		http.NotFound(w, r)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "JSON required", 415)
		return
	}
	raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxRequest))
	if e != nil {
		http.Error(w, "body too large", 413)
		return
	}
	req, e := validate(raw, false)
	if e != nil {
		w.Header().Set("Content-Type", "application/json")
		id := req.ID
		if len(id) == 0 || !json.Valid(id) {
			id = json.RawMessage("null")
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32601, "message": e.Error()}})
		return
	}
	upstream, e := http.NewRequestWithContext(r.Context(), "POST", g.HTTPURL, bytes.NewReader(raw))
	if e != nil {
		http.Error(w, "upstream", 502)
		return
	}
	upstream.Header.Set("Content-Type", "application/json")
	response, e := g.client.Do(upstream)
	if e != nil {
		http.Error(w, "upstream unavailable", 502)
		return
	}
	defer response.Body.Close()
	body, e := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
	if e != nil || len(body) > 2*1024*1024 || !json.Valid(body) {
		http.Error(w, "invalid upstream response", 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)
	w.Write(body)
}
func (g *Gateway) websocket(w http.ResponseWriter, r *http.Request) {
	upstream, _, e := websocket.DefaultDialer.Dial(g.WSURL, http.Header{"Origin": []string{g.Origin}})
	if e != nil {
		http.Error(w, "upstream unavailable", 502)
		return
	}
	defer upstream.Close()
	up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == "" || r.Header.Get("Origin") == g.Origin }, HandshakeTimeout: 5 * time.Second}
	client, e := up.Upgrade(w, r, nil)
	if e != nil {
		return
	}
	defer client.Close()
	client.SetReadLimit(MaxRequest)
	upstream.SetReadLimit(2 * 1024 * 1024)
	done := make(chan struct{})
	var mu sync.Mutex
	pending := map[string]request{}
	subscriptions := map[string]bool{}
	// One writer for each data direction. Bounded lifetime prevents abandoned
	// subscriptions retaining observer resources indefinitely; clients reconnect.
	timer := time.AfterFunc(10*time.Minute, func() { client.Close(); upstream.Close() })
	defer timer.Stop()
	go func() {
		defer close(done)
		defer client.Close()
		defer upstream.Close()
		for {
			kind, raw, e := upstream.ReadMessage()
			if e != nil {
				return
			}
			var reply struct {
				ID     json.RawMessage `json:"id"`
				Result json.RawMessage `json:"result"`
			}
			if json.Unmarshal(raw, &reply) != nil {
				return
			}
			mu.Lock()
			if req, ok := pending[string(reply.ID)]; ok {
				delete(pending, string(reply.ID))
				if req.Method == "eth_subscribe" {
					var id string
					if json.Unmarshal(reply.Result, &id) == nil && id != "" {
						subscriptions[id] = true
					}
				}
				if req.Method == "eth_unsubscribe" && string(reply.Result) == "true" {
					var id string
					json.Unmarshal(req.Params[0], &id)
					delete(subscriptions, id)
				}
			}
			mu.Unlock()
			client.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if client.WriteMessage(kind, raw) != nil {
				return
			}
		}
	}()
	tokens := 10.0
	last := time.Now()
	for {
		kind, raw, e := client.ReadMessage()
		if e != nil {
			break
		}
		if kind != websocket.TextMessage {
			break
		}
		now := time.Now()
		tokens += now.Sub(last).Seconds() * 5
		if tokens > 10 {
			tokens = 10
		}
		last = now
		if tokens < 1 {
			break
		}
		tokens--
		req, e := validate(raw, true)
		if e != nil {
			break
		}
		mu.Lock()
		_, duplicate := pending[string(req.ID)]
		allowed := !duplicate && len(pending) < 16
		if req.Method == "eth_subscribe" {
			count := len(subscriptions)
			for _, p := range pending {
				if p.Method == "eth_subscribe" {
					count++
				}
			}
			allowed = allowed && count < 8
		}
		if req.Method == "eth_unsubscribe" {
			var id string
			allowed = allowed && json.Unmarshal(req.Params[0], &id) == nil && subscriptions[id]
		}
		if allowed {
			pending[string(req.ID)] = req
		}
		mu.Unlock()
		if !allowed {
			break
		}
		upstream.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if upstream.WriteMessage(kind, raw) != nil {
			break
		}
	}
	client.Close()
	upstream.Close()
	<-done
}
