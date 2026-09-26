package blockchain

import (
	"encoding/json"
	"testing"
)

func TestRPCNullableResultAndErrorEnvelope(t *testing.T) {
	for _, r := range []rpcResponse{{JSONRPC: "2.0", ID: json.RawMessage("1")}, {JSONRPC: "2.0", ID: json.RawMessage("1"), Error: &rpcError{Code: -32601, Message: "unsupported"}}} {
		b, e := json.Marshal(r)
		if e != nil {
			t.Fatal(e)
		}
		var obj map[string]json.RawMessage
		if e = json.Unmarshal(b, &obj); e != nil {
			t.Fatal(e)
		}
		if r.Error == nil {
			if string(obj["result"]) != "null" {
				t.Fatalf("missing explicit null: %s", b)
			}
		} else {
			if _, ok := obj["result"]; ok {
				t.Fatalf("error must not contain result: %s", b)
			}
		}
	}
}
