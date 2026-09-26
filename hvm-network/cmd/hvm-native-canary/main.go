// hvm-native-canary performs a resumable, testnet-only funded native contract test.
package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hashburst/hvm"
	"hashburst/protocolv2"
	"hashburst/wallet"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const chain uint64 = 4735490
const stateDir = "/root/hvm-native-canary-v1"
const rpcURL = "http://127.0.0.1:18009/rpc"
const fundingAddress = "0xE1647931177416cB27c9c19b75bAD09E2A3B2a97"

var client = &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil}}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func encode(v any) []byte { b, e := json.Marshal(v); must(e); return b }
func rpc(method string, params any, result any) error {
	req, e := http.NewRequest("POST", rpcURL, bytes.NewReader(encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	res, e := client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("RPC HTTP %d", res.StatusCode)
	}
	var body struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&body); e != nil {
		return e
	}
	if len(body.Error) > 0 && string(body.Error) != "null" {
		return fmt.Errorf("RPC %s: %s", method, body.Error)
	}
	return json.Unmarshal(body.Result, result)
}
func quantity(method string, params any) uint64 {
	var s string
	must(rpc(method, params, &s))
	n, e := strconv.ParseUint(strings.TrimPrefix(s, "0x"), 16, 64)
	must(e)
	return n
}
func balance(addr string) *big.Int {
	var s string
	must(rpc("eth_getBalance", []any{addr, "latest"}, &s))
	n, ok := new(big.Int).SetString(strings.TrimPrefix(s, "0x"), 16)
	if !ok {
		panic("invalid balance")
	}
	return n.Div(n, big.NewInt(10000000000))
}

// Durable writes precede submission. A resumed run reuses the exact signed bytes.
func save(path string, v any) {
	f, e := os.OpenFile(path+".tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	must(e)
	_, e = f.Write(encode(v))
	must(e)
	must(f.Sync())
	must(f.Close())
	must(os.Rename(path+".tmp", path))
	d, e := os.Open(filepath.Dir(path))
	must(e)
	must(d.Sync())
	must(d.Close())
}
func key(path string) *wallet.Wallet {
	s, e := os.Lstat(path)
	must(e)
	if !s.Mode().IsRegular() || s.Mode().Perm()&0077 != 0 {
		panic("private key permissions invalid")
	}
	b, e := os.ReadFile(path)
	must(e)
	w, e := wallet.FromPrivateKeyHex(string(b))
	must(e)
	return w
}

type stage struct {
	Tx            *protocolv2.TransactionV2 `json:"transaction"`
	Raw           string                    `json:"raw"`
	BalanceBefore string                    `json:"balance_before"`
}

func checkReceipt(s stage, r *hvm.Receipt, p protocolv2.FeePolicy) error {
	if r == nil {
		return fmt.Errorf("receipt missing")
	}
	if !strings.EqualFold(strings.TrimPrefix(r.TxID, "0x"), s.Tx.HashHex()) || !r.Success {
		return fmt.Errorf("transaction failed or receipt ID mismatch: %s", r.RevertReason)
	}
	fee, e := p.ComputeFee(r.ComputeUsed)
	if e != nil {
		return e
	}
	if r.ComputeUsed > s.Tx.ComputeLimit || r.FeeUnits != fee || fee > s.Tx.MaxFeeUnits {
		return fmt.Errorf("receipt fee/compute mismatch")
	}
	return nil
}
func transact(name string, w *wallet.Wallet, to string, value int64, typ protocolv2.TxType, data []byte, p protocolv2.FeePolicy) *hvm.Receipt {
	path := filepath.Join(stateDir, name+".json")
	var s stage
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		seq := quantity("hb_getTransactionCount", []any{w.Address(), "pending"})
		fee, e := p.MaxFeeForLimit(600000)
		must(e)
		if fee > 1000000 {
			panic("fee exceeds canary cap")
		}
		before := balance(w.Address())
		if before.Cmp(big.NewInt(value+fee)) < 0 {
			panic("insufficient testnet balance")
		}
		tx := protocolv2.NewTransactionV2(chain, typ, w.Address(), to, value, seq, 600000, fee, data)
		must(tx.Sign(w))
		raw, e := tx.EncodeRaw()
		must(e)
		s = stage{tx, raw, before.String()}
		save(path, s)
	} else {
		must(e)
		must(json.Unmarshal(b, &s))
		if s.Tx == nil {
			panic("invalid saved stage")
		}
		raw, e := s.Tx.EncodeRaw()
		must(e)
		if raw != s.Raw || s.Tx.ChainID != chain || s.Tx.Type != typ || !wallet.AddressEqual(s.Tx.Sender, w.Address()) || s.Tx.To != to || s.Tx.ValueUnits != value || !bytes.Equal(s.Tx.Data, data) {
			panic("saved transaction does not match requested canary")
		}
	}
	must(s.Tx.Verify(chain))
	hash := "0x" + s.Tx.HashHex()
	var receipt *hvm.Receipt
	must(rpc("hb_getTransactionReceipt", []any{hash}, &receipt))
	if receipt == nil {
		var submitted string
		if e := rpc("hb_sendRawTransactionV2", []any{s.Raw}, &submitted); e != nil {
			fmt.Println("SUBMISSION_UNCERTAIN: polling saved transaction; no replacement")
		} else if !strings.EqualFold(strings.TrimPrefix(submitted, "0x"), s.Tx.HashHex()) {
			panic("submission hash mismatch")
		}
	}
	deadline := time.Now().Add(5 * time.Minute)
	for receipt == nil && time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		if e := rpc("hb_getTransactionReceipt", []any{hash}, &receipt); e != nil {
			fmt.Println("WAIT_RECEIPT=" + name)
		}
	}
	must(checkReceipt(s, receipt, p))
	if quantity("hb_getTransactionCount", []any{w.Address(), "latest"}) <= s.Tx.Sequence {
		panic("confirmed nonce did not advance")
	}
	save(filepath.Join(stateDir, name+"-receipt.json"), receipt)
	fmt.Printf("NATIVE_RECEIPT_OK stage=%s tx=%s fee=%d\n", name, hash, receipt.FeeUnits)
	return receipt
}
func run() {
	if os.Geteuid() != 0 {
		panic("run on validator v1 as root")
	}
	if quantity("eth_chainId", []any{}) != chain {
		panic("wrong chain: testnet 4735490 required")
	}
	res, e := client.Get("http://127.0.0.1:18009/health")
	must(e)
	defer res.Body.Close()
	var health struct {
		Node    string `json:"node_id"`
		Role    string `json:"role"`
		Network string `json:"network"`
	}
	must(json.NewDecoder(res.Body).Decode(&health))
	if health.Node != "hvm-testnet-v1" || health.Role != "validator" || health.Network != "testnet" {
		panic("requires hvm-testnet-v1 testnet validator")
	}
	must(os.MkdirAll(stateDir, 0700))
	st, e := os.Lstat(stateDir)
	must(e)
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		panic("unsafe canary directory")
	}
	lock, e := os.OpenFile(filepath.Join(stateDir, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	must(e)
	defer lock.Close()
	must(syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB))
	accountPath := filepath.Join(stateDir, "account.key")
	if _, e := os.Lstat(accountPath); os.IsNotExist(e) {
		w, e := wallet.NewWallet()
		must(e)
		f, e := os.OpenFile(accountPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		must(e)
		_, e = f.WriteString(hex.EncodeToString(w.PrivateKeyBytes()))
		must(e)
		must(f.Sync())
		must(f.Close())
	} else {
		must(e)
	}
	account := key(accountPath)
	funder := key("/root/hvm-testnet-identity/operator.key")
	if !wallet.AddressEqual(funder.Address(), fundingAddress) {
		panic("unexpected funding account")
	}
	var cfg struct {
		ChainID uint64               `json:"chain_id"`
		Fee     protocolv2.FeePolicy `json:"fee_policy"`
	}
	must(rpc("hb_feePolicy", []any{}, &cfg))
	if cfg.ChainID != chain {
		panic("fee policy chain mismatch")
	}
	must(cfg.Fee.Validate())
	transact("fund", funder, account.Address(), 100000000, protocolv2.TxHBTTransfer, nil, cfg.Fee)
	init := encode(hvm.MiningPayoutRegistryInit{Admin: account.Address(), Recorders: []string{account.Address()}, ProfileWriters: []string{account.Address()}})
	deploy := transact("deploy", account, "", 0, protocolv2.TxContractDeploy, encode(hvm.DeployRequest{ContractType: hvm.MiningPayoutRegistryType, Init: init}), cfg.Fee)
	if !wallet.IsValidAddress(deploy.Contract) || len(deploy.Events) == 0 {
		panic("deployment contract/event missing")
	}
	// Commitments are synthetic canary data, never personal or external payment data.
	ref := func(s string) string { return "0x" + hex.EncodeToString(wallet.Keccak256([]byte(account.Address()+s))) }
	profile := hvm.StakeholderProfileRequest{UserRef: ref("user"), APIKeyCommitment: ref("api"), SourceRecordCommitment: ref("source"), ProfileVersion: 1, UpdatedAt: 1, Wallets: []hvm.StakeholderWalletCommitment{}}
	call := transact("profile", account, deploy.Contract, 0, protocolv2.TxContractCall, encode(map[string]any{"method": "registerStakeholderProfile", "args": profile}), cfg.Fee)
	if len(call.Events) == 0 {
		panic("profile event missing")
	}
	var read hvm.Receipt
	must(rpc("hb_call", []any{map[string]any{"sender": account.Address(), "address": deploy.Contract, "method": "getStakeholderProfile", "args": map[string]any{"user_ref": profile.UserRef}, "compute_limit": 200000}}, &read))
	if !read.Success {
		panic("profile read failed")
	}
	var got hvm.StakeholderProfile
	must(json.Unmarshal(read.ReturnData, &got))
	if got.UserRef != profile.UserRef || got.ProfileVersion != 1 || got.APIKeyCommitment != profile.APIKeyCommitment {
		panic("profile readback mismatch")
	}
	var deployStage stage
	b, e := os.ReadFile(filepath.Join(stateDir, "deploy.json"))
	must(e)
	must(json.Unmarshal(b, &deployStage))
	initial, ok := new(big.Int).SetString(deployStage.BalanceBefore, 10)
	if !ok {
		panic("invalid recorded balance")
	}
	expected := new(big.Int).Sub(initial, big.NewInt(deploy.FeeUnits+call.FeeUnits))
	if balance(account.Address()).Cmp(expected) != 0 {
		panic("canary balance does not match executed fees")
	}
	height := quantity("hb_getFinalizedHeight", []any{})
	var commitment map[string]any
	must(rpc("hb_getFinalizedCommitment", []any{height}, &commitment))
	if commitment["certificate"] == nil {
		panic("finalized certificate missing")
	}
	save(filepath.Join(stateDir, "public-proof.json"), map[string]any{"chain_id": chain, "account": account.Address(), "contract": deploy.Contract, "deploy_receipt": deploy, "call_receipt": call, "readback": got, "commitment": commitment, "evm_compatibility": false})
	fmt.Println("HVM_NATIVE_FUNDED_CANARY_OK")
	fmt.Println("PROOF=" + filepath.Join(stateDir, "public-proof.json"))
}
func main() {
	defer func() {
		if e := recover(); e != nil {
			fmt.Fprintln(os.Stderr, "STOP: canary state retained:", e)
			os.Exit(1)
		}
	}()
	run()
}
