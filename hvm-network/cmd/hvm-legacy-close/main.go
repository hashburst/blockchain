// This offline signer creates a separate terminal generation; never a live ledger.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hashburst/blockchain"
	"hashburst/wallet"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const sourceDat = "4d958c23c65b42f95b976d134a1752425a40696f7241b1495a379f4300cdc2d5"
const sourceIdx = "71fc170294373382ec2f938899d8d50815ca3fd16cbb09fad0198654e57e1a03"

func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func read(p string, limit int64) ([]byte, error) {
	st, e := os.Lstat(p)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Size() > limit {
		return nil, errors.New("invalid bounded regular file")
	}
	f, e := os.Open(p)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	after, e := f.Stat()
	if e != nil || !os.SameFile(st, after) {
		return nil, errors.New("file changed")
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(b)) > limit {
		return nil, errors.New("oversize file")
	}
	return b, e
}
func write(p string, b []byte) error {
	f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if _, e = f.Write(b); e != nil {
		return e
	}
	return f.Sync()
}
func run() error {
	source := flag.String("source", "", "existing approved legacy directory")
	out := flag.String("out", "", "NEW generation directory outside live storage")
	ks := flag.String("keystore", "", "existing legacy-owner V3 key; password on stdin")
	authorize := flag.Bool("authorize-terminal-transfer-450", false, "authorize zero-issuance terminal transfer to pinned founder; no network submission")
	check := flag.Bool("check-source", false, "verify pinned source bytes, signatures, PoH and supply without signing")
	flag.Parse()
	if *source == "" || (!*check && (!*authorize || *out == "" || *ks == "")) {
		return errors.New("explicit source/out/keystore and authorization flag required")
	}
	src, e := filepath.EvalSymlinks(*source)
	if e != nil {
		return e
	}
	originals := map[string][]byte{}
	for name, want := range map[string]string{"blockchain.dat": sourceDat, "blockchain.idx": sourceIdx} {
		b, e := read(filepath.Join(src, name), 16<<20)
		if e != nil || hash(b) != want {
			return fmt.Errorf("unapproved source bytes: %s", name)
		}
		originals[name] = b
	}
	prefix, e := blockchain.ReadLegacyPair(src, 10)
	if e != nil {
		return e
	}
	if _, e = blockchain.VerifyLegacyPrefix(prefix); e != nil {
		return e
	}
	if *check {
		fmt.Println("APPROVED_LEGACY_PREFIX_SIGNATURES_POH_AND_450_HBT_OK")
		return nil
	}
	dest, e := filepath.Abs(*out)
	if e != nil {
		return e
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(dest))
	if e != nil || parent != filepath.Dir(dest) {
		return errors.New("canonical existing output parent required")
	}
	if dest == src || strings.HasPrefix(dest, src+string(os.PathSeparator)) {
		return errors.New("output must be outside live storage")
	}
	if _, e = os.Lstat(dest); !os.IsNotExist(e) {
		return errors.New("output already exists or inaccessible; inspect it instead of signing again")
	}
	keyInfo, e := os.Lstat(*ks)
	if e != nil {
		return e
	}
	if keyInfo.Mode().Perm()&0077 != 0 {
		return errors.New("keystore must be owner-only")
	}
	raw, e := read(*ks, 65536)
	if e != nil {
		return e
	}
	defer clear(raw)
	pw, e := bufio.NewReader(io.LimitReader(os.Stdin, 4097)).ReadString('\n')
	if e != nil || len(pw) > 4096 {
		return errors.New("newline-terminated password required via stdin")
	}
	w, e := wallet.DecryptV3(raw, strings.TrimSuffix(pw, "\n"))
	if e != nil {
		return errors.New("keystore unlock failed")
	}
	blocks, e := blockchain.BuildLegacyTerminal(prefix, w, time.Now().UTC())
	if e != nil {
		return e
	}
	lock, e := os.OpenFile(dest+".lock", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	lock.Close()
	defer os.Remove(dest + ".lock")
	stage, e := os.MkdirTemp(parent, ".legacy-terminal-")
	if e != nil {
		return e
	}
	// Retain failed generations for inspection; never remove evidence automatically.
	fmt.Fprintln(os.Stderr, "STAGING="+stage)
	for name, b := range originals {
		if e = write(filepath.Join(stage, name), b); e != nil {
			return e
		}
	}
	if e = blockchain.NewChainStorage(stage).SaveBlock(blocks[10]); e != nil {
		return e
	}
	for _, name := range []string{"blockchain.dat", "blockchain.idx"} {
		f, e := os.OpenFile(filepath.Join(stage, name), os.O_RDWR, 0)
		if e != nil {
			return e
		}
		e = f.Sync()
		f.Close()
		if e != nil {
			return e
		}
	}
	reread, e := blockchain.ReadLegacyPair(stage, 11)
	if e != nil {
		return e
	}
	state, e := blockchain.VerifyLegacyTerminal(reread)
	if e != nil {
		return e
	}
	sums := map[string]string{}
	for name, before := range originals {
		after, e := read(filepath.Join(src, name), 16<<20)
		if e != nil || hash(after) != hash(before) {
			return errors.New("source changed during operation; do not activate staging")
		}
		b, e := read(filepath.Join(stage, name), 16<<20)
		if e != nil {
			return e
		}
		sums[name] = hash(b)
	}
	manifest := map[string]any{"schema": "hashburst-legacy-terminal-candidate-v1", "source_chain_id": 1337, "target_chain_id": 4735489, "source_height": 9, "source_hash": blockchain.LegacyCloseAnchor, "terminal_height": 10, "terminal_hash": blocks[10].Hash, "state_root": state.Root(), "file_sha256": sums, "imported_units": blockchain.LegacyCloseUnits, "new_issuance_units": 0, "recipient": blockchain.LegacyCloseRecipient, "balances_units": state.Snapshot(), "fleet_freeze_verified": false, "mainnet_import_executed": false, "activation_allowed": false}
	encoded, e := json.MarshalIndent(manifest, "", "  ")
	if e != nil {
		return e
	}
	if e = write(filepath.Join(stage, "manifest.json"), append(encoded, '\n')); e != nil {
		return e
	}
	d, e := os.Open(stage)
	if e != nil {
		return e
	}
	e = d.Sync()
	d.Close()
	if e != nil {
		return e
	}
	if _, e = os.Lstat(dest); !os.IsNotExist(e) {
		return errors.New("destination appeared; staging retained")
	}
	if e = os.Rename(stage, dest); e != nil {
		return e
	}
	d, e = os.Open(parent)
	if e != nil {
		return e
	}
	e = d.Sync()
	d.Close()
	if e != nil {
		return e
	}
	fmt.Println("TERMINAL_CANDIDATE_CREATED_NO_ACTIVE_LEDGER_CHANGED=" + dest)
	return nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "STOP:", e)
		os.Exit(1)
	}
}
