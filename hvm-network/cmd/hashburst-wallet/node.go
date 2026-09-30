package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hashburst/hvm"
	"hashburst/protocolv2"
	"hashburst/wallet"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type nodeClient struct {
	endpoint string
	client   *http.Client
}

func newNodeClient(endpoint string) (*nodeClient, error) {
	u, e := url.Parse(endpoint)
	if e != nil {
		return nil, e
	}
	ip := net.ParseIP(u.Hostname())
	if u.User != nil || u.Fragment != "" || u.Host == "" || (u.Scheme != "https" && (u.Scheme != "http" || ip == nil || !ip.IsLoopback())) {
		return nil, errors.New("use HTTPS or an explicit loopback HTTP address, without credentials or fragment")
	}
	return &nodeClient{endpoint, &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("RPC redirect refused") }}}, nil
}
func (n *nodeClient) rpc(method string, params []any, out any) error {
	b, e := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if e != nil {
		return e
	}
	r, e := n.client.Post(n.endpoint, "application/json", bytes.NewReader(b))
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("RPC HTTP %d", r.StatusCode)
	}
	b, e = io.ReadAll(io.LimitReader(r.Body, 2097153))
	if e != nil {
		return e
	}
	if len(b) > 2097152 {
		return errors.New("RPC response too large")
	}
	var v struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	if e = json.Unmarshal(b, &v); e != nil {
		return e
	}
	if v.JSONRPC != "2.0" || v.ID != 1 {
		return errors.New("RPC response identity mismatch")
	}
	if len(v.Error) > 0 && string(v.Error) != "null" {
		return fmt.Errorf("RPC error: %s", v.Error)
	}
	if len(v.Result) == 0 {
		return errors.New("RPC result missing")
	}
	return json.Unmarshal(v.Result, out)
}
func (n *nodeClient) checkChain(chain uint64) error {
	if chain != 4735490 && chain != 4735489 {
		return errors.New("unsupported HashBurst network")
	}
	var s string
	if e := n.rpc("eth_chainId", []any{}, &s); e != nil {
		return e
	}
	v, e := quantity(s)
	if e != nil || v != chain {
		return errors.New("RPC chain mismatch")
	}
	return nil
}
func quantity(s string) (uint64, error) {
	if !strings.HasPrefix(s, "0x") || len(s) < 3 {
		return 0, errors.New("invalid hex quantity")
	}
	return strconv.ParseUint(s[2:], 16, 64)
}
func signedTransfer(path string, chain uint64) (*protocolv2.TransactionV2, error) {
	b, e := readPrivate(path)
	if e != nil {
		return nil, e
	}
	var tx protocolv2.TransactionV2
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(&tx); e != nil {
		return nil, e
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return nil, errors.New("trailing transaction data")
	}
	if e = tx.Verify(chain); e != nil {
		return nil, e
	}
	if tx.Type != protocolv2.TxHBTTransfer || len(tx.Data) != 0 || tx.ValueUnits <= 0 {
		return nil, errors.New("native plain transfer required")
	}
	return &tx, nil
}
func (n *nodeClient) receipt(tx *protocolv2.TransactionV2) (*hvm.Receipt, error) {
	var r *hvm.Receipt
	if e := n.rpc("hb_getTransactionReceipt", []any{"0x" + tx.HashHex()}, &r); e != nil {
		return nil, e
	}
	if r == nil {
		return nil, nil
	}
	if !strings.EqualFold(strings.TrimPrefix(r.TxID, "0x"), tx.HashHex()) || !r.Success || r.FeeUnits < 0 || r.FeeUnits > tx.MaxFeeUnits || r.ComputeUsed > tx.ComputeLimit {
		return nil, errors.New("receipt failed or differs from signed intent")
	}
	return r, nil
}
func (n *nodeClient) submit(tx *protocolv2.TransactionV2, marker, confirm string) error {
	hash := "0x" + tx.HashHex()
	if confirm != hash {
		return errors.New("--confirm-tx must equal the locally computed transaction hash")
	}
	if r, e := n.receipt(tx); e != nil {
		return e
	} else if r != nil {
		fmt.Println("NATIVE_RECEIPT_VERIFIED_NO_RESUBMISSION tx=" + hash)
		return nil
	}
	// Persist before POST: a lost response must not cause automatic retransmission.
	b, _ := json.Marshal(map[string]any{"tx": hash, "chain_id": tx.ChainID, "endpoint": n.endpoint, "state": "submission_attempt_recorded"})
	if e := writeNew(marker, append(b, '\n')); e != nil {
		return fmt.Errorf("submission marker unavailable; no send: %w", e)
	}
	raw, e := tx.EncodeRaw()
	if e != nil {
		return e
	}
	var returned string
	if e = n.rpc("hb_sendRawTransactionV2", []any{raw}, &returned); e != nil {
		return fmt.Errorf("SUBMISSION_UNCERTAIN tx=%s; retain marker and use receipt: %w", hash, e)
	}
	if !strings.EqualFold(returned, hash) {
		return fmt.Errorf("submission hash mismatch; retain marker; expected %s", hash)
	}
	fmt.Println("NATIVE_TRANSACTION_ADMITTED_NOT_YET_RECEIPTED tx=" + hash)
	return nil
}
func nodeCommand(command string, args []string) error {
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	endpoint := f.String("rpc", "", "native node RPC HTTPS or loopback URL")
	chain := f.Uint64("chain-id", 0, "expected HashBurst chain ID")
	in := f.String("in", "", "signed native transaction JSON")
	out := f.String("out", "", "exclusive private draft or attempt marker output")
	from := f.String("from", "", "public sender")
	to := f.String("to", "", "public recipient")
	value := f.Int64("value-units", 0, "native HBT atomic units (1 HBT=100000000)")
	limit := f.Uint64("compute-limit", 0, "maximum compute")
	cap := f.Int64("fee-cap-units", -1, "user maximum allowed fee in native units")
	confirm := f.String("confirm-tx", "", "explicit transaction hash to submit")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	n, e := newNodeClient(*endpoint)
	if e != nil {
		return e
	}
	if e = n.checkChain(*chain); e != nil {
		return e
	}
	if command == "prepare-transfer" {
		if !wallet.IsValidAddress(*from) || !wallet.IsValidAddress(*to) || strings.EqualFold(*to, "0x0000000000000000000000000000000000000000") || *value <= 0 || *limit == 0 || *cap < 0 {
			return errors.New("valid transfer amount, addresses, compute limit and fee cap required")
		}
		var config struct {
			ChainID   uint64                `json:"chain_id"`
			FeePolicy *protocolv2.FeePolicy `json:"fee_policy"`
		}
		if e = n.rpc("hb_feePolicy", []any{}, &config); e != nil {
			return e
		}
		if config.ChainID != *chain || config.FeePolicy == nil {
			return errors.New("native fee policy missing or wrong chain")
		}
		fee, e := config.FeePolicy.MaxFeeForLimit(*limit)
		if e != nil {
			return e
		}
		if fee > *cap {
			return errors.New("node fee estimate exceeds user's fee cap")
		}
		var s string
		if e = n.rpc("hb_getTransactionCount", []any{*from, "pending"}, &s); e != nil {
			return e
		}
		seq, e := quantity(s)
		if e != nil {
			return e
		}
		tx := protocolv2.NewTransactionV2(*chain, protocolv2.TxHBTTransfer, *from, *to, *value, seq, *limit, fee, nil)
		b, e := json.MarshalIndent(tx, "", "  ")
		if e != nil {
			return e
		}
		if e = writeNew(*out, append(b, '\n')); e != nil {
			return e
		}
		fmt.Println("UNSIGNED_NATIVE_DRAFT_CREATED_NO_TRANSACTION_SENT")
		return nil
	}
	tx, e := signedTransfer(*in, *chain)
	if e != nil {
		return e
	}
	if command == "submit-transfer" {
		return n.submit(tx, *out, *confirm)
	}
	if command == "receipt" {
		r, e := n.receipt(tx)
		if e != nil {
			return e
		}
		if r == nil {
			return fmt.Errorf("receipt not yet available tx=0x%s; no transaction sent", tx.HashHex())
		}
		b, _ := json.MarshalIndent(r, "", "  ")
		fmt.Println(string(b))
		fmt.Println("NATIVE_RECEIPT_VERIFIED_NODE_REPORTED_NO_TRANSACTION_SENT")
		return nil
	}
	return errors.New("unknown node command")
}
