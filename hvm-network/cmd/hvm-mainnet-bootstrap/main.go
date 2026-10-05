// hvm-mainnet-bootstrap prepares an offline economic checkpoint only.
// It cannot provision identities, start networking, or import into a live node.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"hashburst/blockchain"
	"hashburst/ledger"
	"io"
	"os"
	"path/filepath"
)

const freezeHash = "d0f2246fc26e9e72c6117ac0d6f6805c7427a8cc1b106613c822a1cbb7e0a6f6"

func readBounded(path string) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Size() > 16<<20 {
		return nil, fmt.Errorf("bounded regular file required")
	}
	b, e := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	if len(b) > 16<<20 {
		return nil, fmt.Errorf("input too large")
	}
	return b, e
}
func hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func writeJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(append(b, '\n'))
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}
func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func prepare(protocol, freeze, source, out string) error {
	raw, e := readBounded(protocol)
	if e != nil {
		return e
	}
	var cfg blockchain.ProtocolV2Config
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if e = dec.Decode(&cfg); e != nil {
		return e
	}
	if dec.Decode(new(any)) != io.EOF {
		return fmt.Errorf("trailing protocol data")
	}
	g, e := blockchain.MainnetEconomicGenesis(cfg)
	if e != nil {
		return e
	}
	report, e := readBounded(freeze)
	if e != nil {
		return e
	}
	if hash(report) != freezeHash {
		return fmt.Errorf("freeze report does not match accepted evidence")
	}
	blocks, e := blockchain.ReadLegacyPair(source, 11)
	if e != nil {
		return e
	}
	state, e := blockchain.VerifyLegacyTerminal(blocks)
	if e != nil {
		return e
	}
	if blocks[10].Hash != cfg.GenesisImport.SourceHash || state.BalanceUnits(cfg.GenesisImport.Recipient) != cfg.GenesisImport.Units {
		return fmt.Errorf("source terminal commitment mismatch")
	}
	return publishCheckpoint(cfg, g, out)
}

func publishCheckpoint(cfg blockchain.ProtocolV2Config, g *blockchain.Block, out string) error {
	parent := filepath.Dir(out)
	real, e := filepath.EvalSymlinks(parent)
	if e != nil {
		return e
	}
	if !filepath.IsAbs(out) || filepath.Clean(out) != out || real != parent {
		return fmt.Errorf("new canonical absolute output required")
	}
	st, e := os.Stat(parent)
	if e != nil {
		return e
	}
	if st.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("private parent required")
	}
	lock, e := os.OpenFile(out+".bootstrap-lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer func() { lock.Close(); os.Remove(out + ".bootstrap-lock") }()
	if _, e = os.Lstat(out); !os.IsNotExist(e) {
		return fmt.Errorf("output already exists or inaccessible; never overwrite")
	}
	tmp, e := os.MkdirTemp(parent, ".mainnet-economic-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	proof, e := ledger.WriteGeneration(filepath.Join(tmp, "ledger"), 1, func(uint64) ([]byte, error) { return blockchain.EncodeLedgerBlock(nil, g) })
	if e != nil {
		return e
	}
	reopened, e := blockchain.OpenExistingBlockchain(filepath.Join(tmp, "ledger"), cfg, g.Hash, 0, g.Hash)
	if e != nil {
		return e
	}
	if reopened.HBTStateRoot() != g.HBTStateRoot || reopened.BalanceUnits(cfg.GenesisImport.Recipient) != cfg.GenesisImport.Units+cfg.GenesisImport.FounderUnits {
		reopened.CloseHistory()
		return fmt.Errorf("persisted genesis mismatch")
	}
	if e = reopened.CloseHistory(); e != nil {
		return e
	}
	if e = writeJSON(filepath.Join(tmp, "protocol.json"), cfg); e != nil {
		return e
	}
	manifest := map[string]any{"schema": "hashburst-mainnet-economic-checkpoint-v1", "activation_allowed": false, "production_import_executed": false, "chain_id": cfg.ChainID, "checkpoint_height": 0, "checkpoint_hash": g.Hash, "native_state_root": g.HBTStateRoot, "source_nullifier": cfg.GenesisImport.Nullifier(), "legacy_units": cfg.GenesisImport.Units, "founder_units": cfg.GenesisImport.FounderUnits, "migration_new_issuance_units": 0, "total_initial_units": cfg.GenesisImport.Units + cfg.GenesisImport.FounderUnits, "freeze_evidence_sha256": freezeHash, "ledger": proof, "protocol_commitment": g.PrevHash, "validators": []any{}, "blockers": []string{"approved fresh signed validator identities and complete protocol policy", "validator bootstrap without unaccounted bootstrap subsidies", "mainnet runtime and crash-recovery acceptance"}}
	if e = writeJSON(filepath.Join(tmp, "CHECKPOINT.json"), manifest); e != nil {
		return e
	}
	if e = syncDir(tmp); e != nil {
		return e
	}
	if _, e = os.Lstat(out); !os.IsNotExist(e) {
		return fmt.Errorf("output appeared; refusing overwrite")
	}
	if e = os.Rename(tmp, out); e != nil {
		return e
	}
	if e = syncDir(parent); e != nil {
		return e
	}
	fmt.Println("ECONOMIC_CHECKPOINT_PREPARED_NOT_ACTIVATABLE=" + out)
	return nil
}
func main() {
	p := flag.String("protocol", "", "reviewed protocol JSON including genesis_import")
	f := flag.String("freeze-report", "", "accepted unchanged fleet freeze report")
	s := flag.String("source", "", "offline terminal generation")
	o := flag.String("out", "", "new absolute offline output")
	flag.Parse()
	if *p == "" || *f == "" || *s == "" || *o == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "all four flags required")
		os.Exit(2)
	}
	if e := prepare(*p, *f, *s, *o); e != nil {
		fmt.Fprintln(os.Stderr, "STOP: no service changed:", e)
		os.Exit(1)
	}
}
