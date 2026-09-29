// Testnet-only, resumable funding of a user-supplied MetaMask account.
package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"hashburst/wallet"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const endpoint = "http://127.0.0.1:18009/evm"
const chainID = 4735490
const operator = "0xE1647931177416cB27c9c19b75bAD09E2A3B2a97"

var client = &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil}}

func rpc(method string, params any, out any) error {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	r, e := client.Post(endpoint, "application/json", bytes.NewReader(b))
	if e != nil {
		return e
	}
	defer r.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if e != nil {
		return e
	}
	if r.StatusCode != 200 {
		return fmt.Errorf("RPC HTTP %d", r.StatusCode)
	}
	var env struct {
		Result json.RawMessage
		Error  json.RawMessage
	}
	if e = json.Unmarshal(raw, &env); e != nil {
		return e
	}
	if len(env.Error) > 0 && string(env.Error) != "null" {
		return fmt.Errorf("RPC %s: %s", method, env.Error)
	}
	return json.Unmarshal(env.Result, out)
}
func quantity(s string) (*big.Int, error) {
	n, ok := new(big.Int).SetString(strings.TrimPrefix(s, "0x"), 16)
	if !ok {
		return nil, fmt.Errorf("invalid quantity")
	}
	return n, nil
}
func run() error {
	toArg := flag.String("to", "", "testnet MetaMask recipient; sends exactly 1 tHBT once per address")
	flag.Parse()
	if !common.IsHexAddress(*toArg) || common.HexToAddress(*toArg) == (common.Address{}) {
		return fmt.Errorf("valid nonzero --to address required")
	}
	to := common.HexToAddress(*toArg)
	var chain string
	if e := rpc("eth_chainId", []any{}, &chain); e != nil {
		return e
	}
	if chain != "0x484202" {
		return fmt.Errorf("testnet required")
	}
	var b map[string]any
	if e := rpc("eth_getBlockByNumber", []any{"finalized", false}, &b); e != nil {
		return e
	}
	height, e := quantity(fmt.Sprint(b["number"]))
	if e != nil || height.Uint64() < 53303 {
		return fmt.Errorf("EVM activation must be finalized")
	}
	dir := "/root/hvm-evm-metamask-funding"
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	lock, e := os.OpenFile(filepath.Join(dir, "fund.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return e
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	path := filepath.Join(dir, strings.ToLower(to.Hex())+".json")
	var saved struct {
		Raw  string `json:"raw"`
		Hash string `json:"hash"`
	}
	raw, e := os.ReadFile(path)
	signer := types.LatestSignerForChainID(big.NewInt(chainID))
	var tx *types.Transaction
	if os.IsNotExist(e) {
		keyPath := "/root/hvm-testnet-identity/operator.key"
		st, e := os.Lstat(keyPath)
		if e != nil {
			return e
		}
		if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("private key permissions")
		}
		key, e := os.ReadFile(keyPath)
		if e != nil {
			return e
		}
		w, e := wallet.FromPrivateKeyHex(strings.TrimSpace(string(key)))
		if e != nil {
			return e
		}
		if !wallet.AddressEqual(w.Address(), operator) {
			return fmt.Errorf("unexpected funding identity")
		}
		var nonceS, balanceS string
		if e = rpc("eth_getTransactionCount", []any{operator, "pending"}, &nonceS); e != nil {
			return e
		}
		nonce, e := quantity(nonceS)
		if e != nil || !nonce.IsUint64() {
			return fmt.Errorf("nonce")
		}
		if e = rpc("eth_getBalance", []any{operator, "latest"}, &balanceS); e != nil {
			return e
		}
		balance, e := quantity(balanceS)
		if e != nil {
			return e
		}
		value := big.NewInt(1000000000000000000)
		if balance.Cmp(new(big.Int).Add(value, big.NewInt(42000))) < 0 {
			return fmt.Errorf("operator has insufficient testnet funds")
		}
		tx = types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(chainID), Nonce: nonce.Uint64(), GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2), Gas: 21000, To: &to, Value: value})
		sig, e := w.Sign(signer.Hash(tx).Bytes())
		if e != nil {
			return e
		}
		tx, e = tx.WithSignature(signer, sig)
		if e != nil {
			return e
		}
		encoded, e := tx.MarshalBinary()
		if e != nil {
			return e
		}
		saved.Raw = "0x" + hex.EncodeToString(encoded)
		saved.Hash = tx.Hash().Hex()
		out, _ := json.MarshalIndent(saved, "", "  ")
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(out)
		if e == nil {
			e = f.Sync()
		}
		f.Close()
		if e != nil {
			return e
		}
		d, e := os.Open(dir)
		if e != nil {
			return e
		}
		e = d.Sync()
		d.Close()
		if e != nil {
			return e
		}
	} else {
		if e != nil {
			return e
		}
		if e = json.Unmarshal(raw, &saved); e != nil {
			return e
		}
		encoded, e := hex.DecodeString(strings.TrimPrefix(saved.Raw, "0x"))
		if e != nil {
			return e
		}
		tx = new(types.Transaction)
		if e = tx.UnmarshalBinary(encoded); e != nil {
			return e
		}
	}
	from, e := types.Sender(signer, tx)
	if e != nil {
		return e
	}
	if from != common.HexToAddress(operator) || tx.ChainId().Uint64() != chainID || tx.To() == nil || *tx.To() != to || tx.Value().Cmp(big.NewInt(1000000000000000000)) != 0 || tx.Hash().Hex() != saved.Hash || tx.Gas() != 21000 || len(tx.Data()) != 0 || tx.GasFeeCap().Cmp(big.NewInt(2)) > 0 {
		return fmt.Errorf("saved transaction differs")
	}
	var receipt map[string]any
	if e = rpc("eth_getTransactionReceipt", []any{saved.Hash}, &receipt); e != nil {
		return e
	}
	if receipt == nil {
		var got string
		if e = rpc("eth_sendRawTransaction", []any{saved.Raw}, &got); e != nil {
			fmt.Println("SUBMISSION_UNCERTAIN_POLLING_SAVED_HASH")
		} else if got != saved.Hash {
			return fmt.Errorf("submission hash mismatch")
		}
	}
	until := time.Now().Add(5 * time.Minute)
	for receipt == nil && time.Now().Before(until) {
		time.Sleep(3 * time.Second)
		if e = rpc("eth_getTransactionReceipt", []any{saved.Hash}, &receipt); e != nil {
			fmt.Println("WAIT_RECEIPT")
		}
	}
	if receipt == nil {
		return fmt.Errorf("receipt timeout; saved transaction retained; same command resumes")
	}
	if receipt["status"] != "0x1" || receipt["transactionHash"] != saved.Hash {
		return fmt.Errorf("funding failed")
	}
	out, _ := json.MarshalIndent(receipt, "", "  ")
	if e = os.WriteFile(path+".receipt.json", out, 0600); e != nil {
		return e
	}
	fmt.Printf("METAMASK_TESTNET_FUNDED recipient=%s amount=1_tHBT tx=%s\n", to.Hex(), saved.Hash)
	return nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "STOP: "+e.Error())
		os.Exit(1)
	}
}
