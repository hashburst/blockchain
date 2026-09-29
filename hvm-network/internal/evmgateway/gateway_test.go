package evmgateway

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPublicFilter(t *testing.T) {
	for _, tc := range []struct {
		raw    string
		ws, ok bool
	}{
		{`{"jsonrpc":"2.0","id":1,"method":"eth_sendRawTransaction","params":["0x01"]}`, false, true},
		{`{"jsonrpc":"2.0","id":1,"method":"eth_sendRawTransaction","params":["0x01"]}`, true, false},
		{`{"jsonrpc":"2.0","id":1,"method":"eth_sendTransaction","params":[]}`, false, false},
		{`{"jsonrpc":"2.0","id":1,"method":"debug_traceTransaction"}`, false, false},
		{`{"jsonrpc":"2.0","id":1,"method":"eth_subscribe","params":["newHeads"]}`, true, true},
		{`{"jsonrpc":"2.0","id":1,"method":"eth_subscribe","params":["newPendingTransactions"]}`, true, false},
		{`[{"jsonrpc":"2.0","id":1,"method":"eth_chainId"}]`, false, false},
		{`{"jsonrpc":"2.0","method":"eth_chainId"}`, false, false},
	} {
		_, e := validate([]byte(tc.raw), tc.ws)
		if (e == nil) != tc.ok {
			t.Fatalf("%s: %v", tc.raw, e)
		}
	}
}
func TestHTTPForwardAndOrigin(t *testing.T) {
	hits := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x484202"}`))
	}))
	defer up.Close()
	g := New(up.URL, "", "https://blockchainapi.one")
	for _, origin := range []string{"", "https://evil.example"} {
		r := httptest.NewRequest("POST", "/rpc", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
	if hits != 2 {
		t.Fatal(hits)
	}
}
func TestWebsocketEightSubscriptionsAndCleanup(t *testing.T) {
	closed := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		c, e := u.Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer c.Close()
		defer close(closed)
		for i := 0; ; i++ {
			_, raw, e := c.ReadMessage()
			if e != nil {
				return
			}
			var req request
			json.Unmarshal(raw, &req)
			c.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": string(rune('a' + i))})
		}
	}))
	defer up.Close()
	g := httptest.NewServer(New("", "ws"+strings.TrimPrefix(up.URL, "http"), "https://blockchainapi.one"))
	defer g.Close()
	c, _, e := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(g.URL, "http")+"/ws", nil)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	for i := 0; i < 9; i++ {
		c.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": i, "method": "eth_subscribe", "params": []string{"newHeads"}})
		_, _, e = c.ReadMessage()
		if i < 8 && e != nil {
			t.Fatal(e)
		}
		if i == 8 && e == nil {
			t.Fatal("ninth subscription accepted")
		}
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream retained")
	}
}
