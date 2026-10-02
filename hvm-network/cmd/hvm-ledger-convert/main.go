// hvm-ledger-convert validates an existing offline HVM state and creates an
// independent binary-codec generation. It never installs it into a live runtime.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"hashburst/blockchain"
	"hashburst/internal/testnet"
	"hashburst/ledger"
	"io"
	"log"
	"os"
	"path/filepath"
)

func digest(p string) (string, error) {
	f, e := os.Open(p)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func main() {
	if e := run(); e != nil {
		log.Printf("STOP: original state retained; generation not activated: %v", e)
		os.Exit(1)
	}
}
func run() error {
	config := flag.String("config", "", "existing pinned HVM configuration; node must be offline")
	output := flag.String("output", "", "new absolute generation directory, outside data directory")
	expectedID := flag.Uint64("expected-chain-id", 0, "required exact chain ID")
	expectedPin := flag.String("expected-config-digest", "", "required reviewed configuration digest")
	flag.Parse()
	if *config == "" || *output == "" || *expectedID == 0 || *expectedPin == "" || flag.NArg() != 0 {
		return fmt.Errorf("config, output, expected-chain-id and expected-config-digest required")
	}
	c, e := testnet.Load(*config)
	if e != nil {
		return e
	}
	if c.Protocol.ChainID != *expectedID || c.Pin() != *expectedPin {
		return fmt.Errorf("chain/config identity mismatch")
	}
	if !filepath.IsAbs(*output) || filepath.Clean(*output) != *output {
		return fmt.Errorf("canonical absolute output required")
	}
	realParent, e := filepath.EvalSymlinks(filepath.Dir(*output))
	if e != nil {
		return e
	}
	candidate := filepath.Join(realParent, filepath.Base(*output))
	if _, err := os.Lstat(candidate); !os.IsNotExist(err) {
		return fmt.Errorf("destination exists or cannot be inspected")
	}
	rel, e := filepath.Rel(c.DataDir, candidate)
	if e != nil {
		return e
	}
	if rel == "." || (rel != ".." && !bytes.HasPrefix([]byte(rel), []byte(".."+string(filepath.Separator)))) {
		return fmt.Errorf("output must be outside source state directory")
	}
	// This existing runtime path acquires the exclusive state lock, validates keys
	// and pin, fully verifies versioned consensus and replays economic projections.
	// It still uses the old full-memory loader; no bounded-memory migration claim.
	s, e := testnet.Prepare(c, false)
	if e != nil {
		return e
	}
	defer s.Close()
	names := []string{"blockchain.dat", "blockchain.idx", "runtime.pin", "consensus-votes.jsonl", "consensus-bft-signatures.jsonl"}
	before := map[string]string{}
	for _, name := range names {
		before[name], e = digest(filepath.Join(c.DataDir, name))
		if e != nil {
			return e
		}
	}
	count := s.Chain.Height() + 1
	if count == 0 {
		return fmt.Errorf("empty verified chain")
	}
	proof, e := ledger.WriteGeneration(candidate, uint64(count), func(n uint64) ([]byte, error) {
		b, e := s.Chain.BlockAt(int(n))
		if e != nil {
			return nil, e
		}
		if b == nil || b.Index != int(n) {
			return nil, fmt.Errorf("block ordinal mismatch %d", n)
		}
		encoded, e := blockchain.EncodeLedgerBlock(nil, b)
		if e != nil {
			return nil, e
		}
		var decoded blockchain.Block
		if e = blockchain.DecodeLedgerBlockInto(encoded, &decoded); e != nil {
			return nil, e
		}
		again, e := blockchain.EncodeLedgerBlock(nil, &decoded)
		if e != nil {
			return nil, e
		}
		if !bytes.Equal(encoded, again) {
			return nil, fmt.Errorf("codec roundtrip differs %d", n)
		}
		return encoded, nil
	})
	if e != nil {
		return e
	}
	ds, e := os.Stat(filepath.Join(candidate, "blockchain.dat"))
	if e != nil {
		return e
	}
	is, e := os.Stat(filepath.Join(candidate, "blockchain.idx"))
	if e != nil {
		return e
	}
	if ds.Size() > int64(1<<63-1)-is.Size() {
		return fmt.Errorf("mapping size overflow")
	}
	r, e := ledger.OpenPair(candidate, ledger.BinaryPayload, ds.Size()+is.Size(), 0, 0)
	if e != nil {
		return e
	}
	defer r.Close()
	if e = r.ValidateLayout(); e != nil {
		return e
	}
	for n := uint64(0); n < r.Count(); n++ {
		e = r.WithPayload(n, func(raw []byte) error {
			b, e := s.Chain.BlockAt(int(n))
			if e != nil {
				return e
			}
			want, e := blockchain.EncodeLedgerBlock(nil, b)
			if e != nil {
				return e
			}
			if !bytes.Equal(raw, want) {
				return fmt.Errorf("persisted payload differs %d", n)
			}
			return nil
		})
		if e != nil {
			return e
		}
	}
	for _, name := range names {
		after, e := digest(filepath.Join(c.DataDir, name))
		if e != nil {
			return e
		}
		if before[name] != after {
			return fmt.Errorf("source changed during conversion: %s", name)
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "OFFLINE_GENERATION_VERIFIED_NOT_ACTIVATED", "chain_id": c.Protocol.ChainID, "node_id": c.NodeID, "configuration_digest": c.Pin(), "head_height": s.Chain.Height(), "generation": proof, "source_sha256": before, "legacy_freeze_verified": false, "binary_runtime_supported": true, "active_generation_replaced": false})
}
