// hvm-wallet-proof performs an offline custody check. It never creates transactions.
package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hashburst/wallet"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const domain = "HASHBURST-CUSTODY-CHECK-v1:NO-TRANSFER:NO-MIGRATION-AUTHORIZATION:"

type proof struct {
	Message   string `json:"message"`
	Digest    string `json:"sha256"`
	Signature string `json:"signature"`
	Address   string `json:"address"`
	Verified  bool   `json:"signature_verified"`
}

func prove(w *wallet.Wallet, expected string) (proof, error) {
	var p proof
	if !wallet.IsValidAddress(expected) || !strings.EqualFold(w.Address(), expected) {
		return p, errors.New("key does not match expected address")
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return p, err
	}
	p.Message = domain + w.Address() + ":" + hex.EncodeToString(nonce)
	digest := sha256.Sum256([]byte(p.Message))
	sig, err := w.Sign(digest[:])
	if err != nil {
		return p, err
	}
	recovered, err := wallet.RecoverAddress(digest[:], sig)
	if err != nil || !strings.EqualFold(recovered, expected) {
		return p, errors.New("signature recovery failed")
	}
	p.Digest, p.Signature, p.Address, p.Verified = hex.EncodeToString(digest[:]), hex.EncodeToString(sig), recovered, true
	return p, nil
}

func readPrivate(path string) ([]byte, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil || real != abs {
		return nil, errors.New("canonical private file required")
	}
	before, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0 || before.Size() > 65536 {
		return nil, errors.New("regular owner-only private file up to 64 KiB required")
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		return nil, errors.New("private file changed")
	}
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if len(b) > 65536 {
		return nil, errors.New("private file too large")
	}
	return b, err
}

func run() error {
	key := flag.String("key", "", "existing raw wallet.key; never created")
	ks := flag.String("keystore", "", "existing encrypted V3 keystore; password from stdin pipe")
	expected := flag.String("address", "", "expected public address")
	flag.Parse()
	if (*key == "") == (*ks == "") || !wallet.IsValidAddress(*expected) {
		return errors.New("choose exactly one of --key/--keystore and supply --address")
	}
	path := *key
	if *ks != "" {
		path = *ks
	}
	raw, err := readPrivate(path)
	if err != nil {
		return err
	}
	defer clear(raw)
	var w *wallet.Wallet
	if *key != "" {
		w, err = wallet.FromPrivateKeyHex(strings.TrimSpace(string(raw)))
	} else {
		password, e := bufio.NewReader(io.LimitReader(os.Stdin, 4097)).ReadString('\n')
		if e != nil || len(password) > 4096 {
			return errors.New("password must arrive via stdin, newline terminated, at most 4095 bytes")
		}
		w, err = wallet.DecryptV3(raw, strings.TrimSuffix(password, "\n"))
	}
	if err != nil {
		return errors.New("private key load or keystore unlock failed")
	}
	p, err := prove(w, *expected)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(p)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "STOP:", err)
		os.Exit(1)
	}
}
