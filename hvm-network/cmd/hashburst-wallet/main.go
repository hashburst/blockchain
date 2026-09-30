// hashburst-wallet is an offline custody and native-transfer signer. No RPC client.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hashburst/protocolv2"
	"hashburst/wallet"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func readPrivate(path string) ([]byte, error) {
	st, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > 65536 {
		return nil, errors.New("owner-only regular file required, max 64 KiB")
	}
	return os.ReadFile(path)
}
func writeNew(path string, b []byte) error {
	st, e := os.Lstat(filepath.Dir(path))
	if e != nil {
		return e
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return errors.New("output directory must be private (0700)")
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func decodeTransfer(raw []byte, chain uint64, address string) (*protocolv2.TransactionV2, error) {
	var tx protocolv2.TransactionV2
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if e := d.Decode(&tx); e != nil {
		return nil, e
	}
	var extra interface{}
	if e := d.Decode(&extra); e != io.EOF {
		return nil, errors.New("trailing JSON")
	}
	if chain != 4735490 && chain != 4735489 {
		return nil, errors.New("unsupported chain")
	}
	if tx.ChainID != chain || tx.Version != 2 || tx.Type != protocolv2.TxHBTTransfer || !strings.EqualFold(tx.Sender, address) || !wallet.IsValidAddress(tx.To) || strings.EqualFold(tx.To, "0x0000000000000000000000000000000000000000") || tx.ValueUnits <= 0 || tx.MaxFeeUnits < 0 || tx.ComputeLimit == 0 || len(tx.Data) != 0 || tx.Signature != "" || tx.ID != "" {
		return nil, errors.New("only unsigned plain native HBT transfers for the expected account and chain are accepted")
	}
	return &tx, nil
}
func run() error {
	if len(os.Args) < 2 {
		return errors.New("commands: encrypt, verify, sign-transfer; use command --help")
	}
	cmd := os.Args[1]
	f := flag.NewFlagSet(cmd, flag.ContinueOnError)
	in := f.String("in", "", "private source file")
	out := f.String("out", "", "new output file, never overwritten")
	expected := f.String("address", "", "expected public address")
	chain := f.Uint64("chain-id", 0, "expected chain ID")
	draft := f.String("draft", "", "native transfer draft JSON")
	if e := f.Parse(os.Args[2:]); e != nil {
		return e
	}
	if cmd != "encrypt" && cmd != "verify" && cmd != "sign-transfer" {
		return errors.New("unknown command")
	}
	if !wallet.IsValidAddress(*expected) || (*chain != 4735490 && *chain != 4735489) {
		return errors.New("explicit valid address and supported chain ID required")
	}
	// Password delivered via anonymous stdin pipe by prompt.py, never argv/environment.
	password, e := bufio.NewReader(io.LimitReader(os.Stdin, 4097)).ReadString('\n')
	if e != nil {
		return errors.New("password input missing")
	}
	password = strings.TrimSuffix(password, "\n")
	if len(password) < 12 || len(password) > 4096 {
		return errors.New("password must contain 12 to 4096 bytes")
	}
	raw, e := readPrivate(*in)
	if e != nil {
		return e
	}
	var w *wallet.Wallet
	if cmd == "encrypt" {
		w, e = wallet.FromPrivateKeyHex(strings.TrimSpace(string(raw)))
	} else {
		w, e = wallet.DecryptV3(raw, password)
	}
	if e != nil {
		return e
	}
	if !strings.EqualFold(w.Address(), *expected) {
		return errors.New("key does not match expected address")
	}
	switch cmd {
	case "encrypt":
		b, e := w.EncryptV3(password, wallet.StandardScryptN, wallet.StandardScryptR, wallet.StandardScryptP)
		if e != nil {
			return e
		}
		check, e := wallet.DecryptV3(b, password)
		if e != nil || check.Address() != w.Address() {
			return errors.New("keystore roundtrip failed")
		}
		if e = writeNew(*out, b); e != nil {
			return e
		}
		saved, e := readPrivate(*out)
		if e != nil {
			return e
		}
		check, e = wallet.DecryptV3(saved, password)
		if e != nil || check.Address() != w.Address() {
			return errors.New("saved keystore verification failed")
		}
		fmt.Println("ENCRYPTED_KEYSTORE_VERIFIED_PLAINTEXT_RETAINED")
	case "verify":
		fmt.Println("KEYSTORE_UNLOCK_AND_ADDRESS_OK")
	case "sign-transfer":
		b, e := readPrivate(*draft)
		if e != nil {
			return e
		}
		tx, e := decodeTransfer(b, *chain, w.Address())
		if e != nil {
			return e
		}
		if e = tx.Sign(w); e != nil {
			return e
		}
		b, e = json.MarshalIndent(tx, "", "  ")
		if e != nil {
			return e
		}
		if e = writeNew(*out, append(b, '\n')); e != nil {
			return e
		}
		fmt.Println("NATIVE_TRANSFER_SIGNED_OFFLINE_NO_TRANSACTION_SENT id=" + tx.ID)
	}
	fmt.Printf("ADDRESS=%s CHAIN_ID=%d\n", w.Address(), *chain)
	return nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "STOP:", e)
		os.Exit(1)
	}
}
