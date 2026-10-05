// hvm-legacy-archive has no miner, P2P, transaction pool or chain write API.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hashburst/blockchain"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func digest(p string) (string, error) {
	st, e := os.Lstat(p)
	if e != nil {
		return "", e
	}
	if !st.Mode().IsRegular() || st.Size() > 16<<20 {
		return "", errors.New("invalid bounded ledger file")
	}
	f, e := os.Open(p)
	if e != nil {
		return "", e
	}
	defer f.Close()
	actual, e := f.Stat()
	if e != nil || !os.SameFile(st, actual) {
		return "", errors.New("ledger changed")
	}
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, (16<<20)+1))
	if e != nil || n > 16<<20 {
		return "", errors.New("ledger read failed or too large")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func handler(blocks []*blockchain.Block, state *blockchain.State) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "mode": "legacy-terminal-archive", "chainId": 1337, "blockHeight": 11, "terminal_height": 10, "terminal_hash": blocks[10].Hash, "state_root": state.Root(), "spendable": false, "mining_enabled": false, "p2p_enabled": false})
	})
	mux.HandleFunc("/api/balances", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"balance_units": state.Snapshot(), "unit_decimals": 8, "spendable": false})
	})
	mux.HandleFunc("/api/blocks", func(w http.ResponseWriter, r *http.Request) {
		out := []map[string]any{}
		for _, b := range blocks {
			out = append(out, map[string]any{"number": b.Index, "hash": b.Hash, "parentHash": b.PrevHash, "timestamp": b.Timestamp.Unix(), "transactions": len(b.Transactions), "version": b.EffectiveVersion()})
		}
		json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("/api/transactions", func(w http.ResponseWriter, r *http.Request) {
		out := []map[string]any{}
		for _, b := range blocks {
			for _, tx := range b.Transactions {
				out = append(out, map[string]any{"hash": tx.ID, "from": tx.Sender, "to": tx.Receiver, "value": tx.Amount, "status": "archived", "version": 1})
			}
		}
		json.NewEncoder(w).Encode(out)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "legacy archive is immutable", http.StatusMethodNotAllowed)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func run() error {
	dir := flag.String("directory", "", "verified terminal generation directory")
	dat := flag.String("dat-sha256", "", "externally pinned data digest")
	idx := flag.String("idx-sha256", "", "externally pinned index digest")
	terminal := flag.String("terminal-hash", "", "externally pinned terminal block hash")
	listen := flag.String("listen", "127.0.0.1:8009", "loopback HTTP only")
	check := flag.Bool("check", false, "verify and exit without listening")
	flag.Parse()
	for _, h := range []string{*dat, *idx, *terminal} {
		b, e := hex.DecodeString(h)
		if e != nil || len(b) != 32 {
			return errors.New("all three external commitments required")
		}
	}
	verifyFiles := func() error {
		for name, want := range map[string]string{"blockchain.dat": *dat, "blockchain.idx": *idx} {
			got, e := digest(filepath.Join(*dir, name))
			if e != nil || got != want {
				return fmt.Errorf("ledger commitment mismatch: %s", name)
			}
		}
		return nil
	}
	if e := verifyFiles(); e != nil {
		return e
	}
	blocks, e := blockchain.ReadLegacyPair(*dir, 11)
	if e != nil {
		return e
	}
	state, e := blockchain.VerifyLegacyTerminal(blocks)
	if e != nil {
		return e
	}
	if e = verifyFiles(); e != nil {
		return e
	}
	if blocks[10].Hash != *terminal {
		return errors.New("terminal commitment mismatch")
	}
	if *check {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"terminal_verified": true, "state_root": state.Root(), "legacy_units": blockchain.LegacyCloseUnits, "new_issuance_units": 0, "mainnet_import_executed": false})
	}
	host, _, e := net.SplitHostPort(*listen)
	if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("loopback listener required")
	}
	s := http.Server{Addr: *listen, Handler: handler(blocks, state), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	return s.ListenAndServe()
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "STOP:", e)
		os.Exit(1)
	}
}
