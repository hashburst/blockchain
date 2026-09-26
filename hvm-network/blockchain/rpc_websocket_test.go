package blockchain

import (
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadOnlyWebSocket(t *testing.T) {
	h := NewRPCHandler(&Blockchain{}, nil, 4735490)
	s := httptest.NewServer(http.HandlerFunc(h.ServeReadOnlyWebSocket))
	defer s.Close()
	c, _, e := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(s.URL, "http"), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	for _, method := range []string{"eth_chainId", "hb_sendRawTransactionV2", "eth_subscribe"} {
		if e = c.WriteJSON(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": method, "params": []interface{}{}}); e != nil {
			t.Fatal(e)
		}
		var r map[string]interface{}
		if e = c.ReadJSON(&r); e != nil {
			t.Fatal(e)
		}
		if method == "eth_chainId" {
			if r["result"] != "0x484202" {
				t.Fatal(r)
			}
		} else if r["error"] == nil {
			t.Fatal("write/subscription accepted")
		}
	}
}
func TestWebSocketRejectsForeignOrigin(t *testing.T) {
	h := NewRPCHandler(&Blockchain{}, nil, 4735490)
	s := httptest.NewServer(http.HandlerFunc(h.ServeReadOnlyWebSocket))
	defer s.Close()
	c, r, e := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(s.URL, "http"), http.Header{"Origin": []string{"https://foreign.invalid"}})
	if c != nil {
		c.Close()
	}
	if e == nil || r == nil || r.StatusCode != 403 {
		t.Fatal("foreign origin accepted")
	}
}
