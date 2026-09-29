// hvm-apow-miner mines one parent-bound proof; it never holds validator keys.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"hashburst/blockchain"
	"hashburst/wallet"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func run() error {
	endpoint := flag.String("endpoint", "http://127.0.0.1:18009/apow", "loopback miner endpoint (use an SSH tunnel)")
	key := flag.String("key-file", "", "miner private key, hex file with mode 0600; not a validator key")
	reward := flag.String("beneficiary", "", "native HBT reward address; defaults to miner")
	timeout := flag.Duration("timeout", time.Minute, "bounded one-job mining deadline")
	flag.Parse()
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
	if *reward == "" {
		*reward = signer.Address()
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Get(*endpoint)
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
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	proof, err := blockchain.MineAPoW(ctx, job, signer, *reward)
	if err != nil {
		return err
	}
	body, err := json.Marshal(proof)
	if err != nil {
		return err
	}
	res2, err := client.Post(*endpoint, "application/json", bytes.NewReader(body))
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
