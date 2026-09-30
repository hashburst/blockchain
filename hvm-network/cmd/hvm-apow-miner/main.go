// hvm-apow-miner mines one parent-bound proof; it never holds validator keys.
package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"hashburst/blockchain"
	"hashburst/wallet"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func run() error {
	endpoint := flag.String("endpoint", "http://127.0.0.1:18009/apow", "loopback miner endpoint (use an SSH tunnel)")
	key := flag.String("key-file", "", "miner private key, hex file with mode 0600; not a validator key")
	reward := flag.String("beneficiary", "", "native HBT reward address; defaults to miner")
	timeout := flag.Duration("timeout", time.Minute, "bounded one-job mining deadline")
	loop := flag.Bool("loop", false, "fetch fresh work continuously; retry transient job errors")
	chain := flag.Uint64("chain-id", 4735490, "expected chain ID; mismatched work is never signed")
	generate := flag.Bool("generate-key", false, "create a new miner-only key; never overwrite an existing key")
	identityOnly := flag.Bool("identity-only", false, "print only the public miner address")
	flag.Parse()
	if *generate {
		return generateKey(*key)
	}
	if *timeout <= 0 || *chain == 0 {
		return fmt.Errorf("positive timeout and explicit chain required")
	}
	u, err := url.Parse(*endpoint)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") {
		return fmt.Errorf("literal loopback HTTP endpoint required")
	}
	info, err := os.Lstat(*key)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("private key must be a regular owner-only file")
	}
	raw, err := os.ReadFile(*key)
	if err != nil {
		return err
	}
	signer, err := wallet.FromPrivateKeyHex(strings.TrimSpace(string(raw)))
	if err != nil {
		return err
	}
	if *identityOnly {
		fmt.Printf("APOW_MINER_ADDRESS=%s\n", signer.Address())
		return nil
	}
	if *reward == "" {
		*reward = signer.Address()
	}
	if !wallet.IsValidAddress(*reward) || strings.EqualFold(*reward, "0x0000000000000000000000000000000000000000") {
		return fmt.Errorf("valid nonzero beneficiary required")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	for {
		err := mineJob(ctx, *endpoint, signer, *reward, *chain, *timeout)
		if !*loop {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "APOW_WAIT: %v\n", err)
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func generateKey(path string) error {
	w, err := wallet.NewWallet()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.WriteString(hex.EncodeToString(w.PrivateKeyBytes()) + "\n"); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	if err = directory.Sync(); err != nil {
		return err
	}
	fmt.Printf("APOW_MINER_IDENTITY_CREATED address=%s NO_SERVICE_STARTED\n", w.Address())
	return nil
}

func mineJob(parent context.Context, endpoint string, signer *wallet.Wallet, reward string, chain uint64, timeout time.Duration) error {
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Get(endpoint)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("job HTTP %d", res.StatusCode)
	}
	var job blockchain.APoWProof
	if err = json.NewDecoder(io.LimitReader(res.Body, 2048)).Decode(&job); err != nil {
		return err
	}
	if job.ChainID != chain {
		return fmt.Errorf("job chain ID %d differs from expected %d", job.ChainID, chain)
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	proof, err := blockchain.MineAPoW(ctx, job, signer, reward)
	if err != nil {
		return err
	}
	body, err := json.Marshal(proof)
	if err != nil {
		return err
	}
	res2, err := client.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer res2.Body.Close()
	if res2.StatusCode != 200 {
		return fmt.Errorf("submission HTTP %d; fetch a fresh job before retry", res2.StatusCode)
	}
	fmt.Printf("APOW_SUBMITTED height=%d author=%s beneficiary=%s; reward requires BFT finality\n", proof.Height, proof.Author, proof.Beneficiary)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
