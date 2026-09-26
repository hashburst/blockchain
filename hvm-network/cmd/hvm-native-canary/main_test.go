package main

import (
	"encoding/json"
	"hashburst/hvm"
	"hashburst/protocolv2"
	"hashburst/wallet"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeDeployProfileReadback(t *testing.T) {
	w, e := wallet.NewWallet()
	if e != nil {
		t.Fatal(e)
	}
	p := protocolv2.FeePolicy{BaseTxUnits: 100, FeeRateUnitsPerMillion: 1000}
	engine := hvm.NewEngine(nil, p)
	ctx := hvm.ExecutionContext{TxID: strings.Repeat("1", 64), ChainID: chain, Sender: w.Address(), BlockHeight: 50000, BlockTime: 1790000000, ComputeLimit: 600000}
	d := engine.Deploy(ctx, hvm.DeployRequest{ContractType: hvm.MiningPayoutRegistryType, Init: encode(hvm.MiningPayoutRegistryInit{Admin: w.Address(), ProfileWriters: []string{w.Address()}})})
	if !d.Success || len(d.Events) == 0 {
		t.Fatalf("deploy: %+v", d)
	}
	profile := hvm.StakeholderProfileRequest{UserRef: "0x" + strings.Repeat("1", 64), APIKeyCommitment: "0x" + strings.Repeat("2", 64), SourceRecordCommitment: "0x" + strings.Repeat("3", 64), ProfileVersion: 1, UpdatedAt: 1, Wallets: []hvm.StakeholderWalletCommitment{}}
	ctx.TxID = strings.Repeat("4", 64)
	r := engine.Call(ctx, hvm.CallRequest{Address: d.Contract, Method: "registerStakeholderProfile", Args: encode(profile)})
	if !r.Success || len(r.Events) != 1 {
		t.Fatalf("profile: %+v", r)
	}
	read := engine.Call(ctx, hvm.CallRequest{Address: d.Contract, Method: "getStakeholderProfile", Args: encode(map[string]any{"user_ref": profile.UserRef})})
	var got hvm.StakeholderProfile
	if !read.Success || json.Unmarshal(read.ReturnData, &got) != nil || got.UserRef != profile.UserRef || got.ProfileVersion != 1 {
		t.Fatalf("readback: %+v", read)
	}
}
func TestReceiptRejectsWrongHashFeeAndFailure(t *testing.T) {
	tx := protocolv2.NewTransactionV2(chain, protocolv2.TxHBTTransfer, "a", "b", 1, 0, 600000, 700, nil)
	s := stage{Tx: tx}
	p := protocolv2.FeePolicy{BaseTxUnits: 100, FeeRateUnitsPerMillion: 1000}
	r := hvm.Receipt{TxID: tx.HashHex(), Success: true, ComputeUsed: 1000, FeeUnits: 101}
	if e := checkReceipt(s, &r, p); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []hvm.Receipt{{TxID: "bad", Success: true}, {TxID: tx.HashHex(), Success: false}, {TxID: tx.HashHex(), Success: true, ComputeUsed: 1000, FeeUnits: 0}} {
		if checkReceipt(s, &bad, p) == nil {
			t.Fatal("accepted invalid receipt")
		}
	}
}
func TestSavedSignedTransactionRoundTrip(t *testing.T) {
	w, _ := wallet.NewWallet()
	tx := protocolv2.NewTransactionV2(chain, protocolv2.TxHBTTransfer, w.Address(), w.Address(), 1, 3, 600000, 700, nil)
	if e := tx.Sign(w); e != nil {
		t.Fatal(e)
	}
	raw, _ := tx.EncodeRaw()
	path := filepath.Join(t.TempDir(), "signed.json")
	save(path, stage{Tx: tx, Raw: raw})
	b, _ := os.ReadFile(path)
	var s stage
	if e := json.Unmarshal(b, &s); e != nil {
		t.Fatal(e)
	}
	if e := s.Tx.Verify(chain); e != nil {
		t.Fatal(e)
	}
	again, _ := s.Tx.EncodeRaw()
	if again != raw {
		t.Fatal("signed bytes changed")
	}
	if s.Tx.Verify(1337) == nil {
		t.Fatal("wrong chain accepted")
	}
}

func TestDeployedMissingReceiptResult(t *testing.T) {
	for _, body := range []string{`{"jsonrpc":"2.0","id":1}`, `{"jsonrpc":"2.0","id":1,"result":null}`} {
		r := &hvm.Receipt{Success: true}
		if err := decodeRPC("hb_getTransactionReceipt", []byte(body), &r); err != nil || r != nil {
			t.Fatalf("nullable receipt: %v %+v", err, r)
		}
	}
	for _, body := range []string{``, `{}`, `{"jsonrpc":"2.0","id":2}`, `{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"failed"}}`} {
		var r *hvm.Receipt
		if decodeRPC("hb_getTransactionReceipt", []byte(body), &r) == nil {
			t.Fatalf("accepted malformed/error envelope: %s", body)
		}
	}
	var value string
	if decodeRPC("eth_chainId", []byte(`{"jsonrpc":"2.0","id":1}`), &value) == nil {
		t.Fatal("accepted missing chain result")
	}
}
