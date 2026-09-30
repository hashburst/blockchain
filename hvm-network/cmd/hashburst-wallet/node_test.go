package main

import (
	"encoding/json"
	"hashburst/protocolv2"
	"hashburst/wallet"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleSigned(t *testing.T) *protocolv2.TransactionV2 {
	t.Helper()
	w, e := wallet.NewWallet()
	if e != nil {
		t.Fatal(e)
	}
	to, e := wallet.NewWallet()
	if e != nil {
		t.Fatal(e)
	}
	tx := protocolv2.NewTransactionV2(4735490, protocolv2.TxHBTTransfer, w.Address(), to.Address(), 100, 0, 10000, 101, nil)
	if e = tx.Sign(w); e != nil {
		t.Fatal(e)
	}
	return tx
}
func rpcFixture(t *testing.T, fn func(string) any) (*nodeClient, func()) {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Method string `json:"method"`
		}
		if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
			t.Error(e)
		}
		result := fn(q.Method)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
	}))
	n, e := newNodeClient(s.URL)
	if e != nil {
		t.Fatal(e)
	}
	return n, s.Close
}
func TestRPCNetworkAndRedirectGuards(t *testing.T) {
	for _, u := range []string{"http://example.com/rpc", "http://localhost/rpc", "https://user:secret@example.com", "file:///tmp/key"} {
		if _, e := newNodeClient(u); e == nil {
			t.Fatal("unsafe URL", u)
		}
	}
	n, close := rpcFixture(t, func(string) any { return "0x539" })
	defer close()
	if n.checkChain(4735490) == nil {
		t.Fatal("legacy endpoint accepted")
	}
	calls := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	n, _ = newNodeClient(redirect.URL)
	if n.checkChain(4735490) == nil || calls != 0 {
		t.Fatal("redirect followed")
	}
}
func TestSubmissionUncertainNeverAutomaticallyRepeated(t *testing.T) {
	tx := sampleSigned(t)
	posts := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&q)
		if q.Method == "hb_sendRawTransactionV2" {
			posts++
			http.Error(w, "upstream lost after admission", 502)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": nil})
	}))
	defer s.Close()
	n, _ := newNodeClient(s.URL)
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	marker := filepath.Join(dir, "attempt.json")
	hash := "0x" + tx.HashHex()
	if e := n.submit(tx, marker, hash); e == nil || !strings.Contains(e.Error(), "SUBMISSION_UNCERTAIN") {
		t.Fatal(e)
	}
	if _, e := os.Stat(marker); e != nil {
		t.Fatal("missing durable intent", e)
	}
	if e := n.submit(tx, marker, hash); e == nil {
		t.Fatal("repeat permitted")
	}
	if posts != 1 {
		t.Fatal("multiple submissions", posts)
	}
}
func TestReceiptMismatchAndExistingReceipt(t *testing.T) {
	tx := sampleSigned(t)
	hash := "0x" + tx.HashHex()
	calls := 0
	valid := false
	n, close := rpcFixture(t, func(method string) any {
		calls++
		if method != "hb_getTransactionReceipt" {
			t.Fatal(method)
		}
		id := hash
		if !valid {
			id = "wrong"
		}
		return map[string]any{"txid": id, "success": true, "compute_used": 100, "fee_units": 101}
	})
	defer close()
	if _, e := n.receipt(tx); e == nil {
		t.Fatal("wrong receipt accepted")
	}
	valid = true
	if e := n.submit(tx, "no-marker-needed", hash); e != nil {
		t.Fatal(e)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}
func TestPrepareNativeDraftUsesNodePolicyAndFeeCap(t *testing.T) {
	tx := sampleSigned(t)
	n, close := rpcFixture(t, func(method string) any {
		switch method {
		case "eth_chainId":
			return "0x484202"
		case "hb_feePolicy":
			return map[string]any{"chain_id": 4735490, "fee_policy": map[string]any{"base_tx_units": 100, "fee_rate_units_per_million": 1}}
		case "hb_getTransactionCount":
			return "0x7"
		}
		t.Fatal("unexpected method", method)
		return nil
	})
	defer close()
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	out := filepath.Join(dir, "draft.json")
	args := []string{"--rpc", n.endpoint, "--chain-id", "4735490", "--from", tx.Sender, "--to", tx.To, "--value-units", "100", "--compute-limit", "10000", "--fee-cap-units", "101", "--out", out}
	if e := nodeCommand("prepare-transfer", args); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(out)
	var draft protocolv2.TransactionV2
	json.Unmarshal(b, &draft)
	if draft.Sequence != 7 || draft.MaxFeeUnits != 101 || draft.Signature != "" {
		t.Fatal("draft differs", draft)
	}
	args[len(args)-3] = "100" // fee cap value
	args[len(args)-1] = filepath.Join(dir, "rejected.json")
	if e := nodeCommand("prepare-transfer", args); e == nil {
		t.Fatal("fee cap exceeded")
	}
}
