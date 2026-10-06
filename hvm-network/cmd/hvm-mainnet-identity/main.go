// hvm-mainnet-identity creates local keys and exports a public signed enrollment.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"hashburst/blockchain"
	"hashburst/internal/mainnetidentity"
	"hashburst/protocolv2"
	"hashburst/wallet"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func read(path string, out any) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if e != nil {
		return e
	}
	if len(b) > 1<<20 {
		return fmt.Errorf("input exceeds 1 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(out); e != nil {
		return e
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}
func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("use generate, verify, funding-plan or verify-funding")
	}
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	config := f.String("protocol", "", "complete reviewed protocol JSON")
	out := f.String("out", "", "new local private directory; existing destination refused")
	key := f.String("key", "", "existing founder raw wallet.key, read only on signing host")
	economic := f.String("economic-ledger", "", "verified pristine economic checkpoint ledger")
	signed := f.String("signed-funding", "", "signed founder transfer array")
	public := f.String("public", "", "public enrollment JSON for verification")
	node := f.String("node-id", "", "unique mainnet node ID")
	ip := f.String("ip", "", "advertised IPv4")
	port := f.Int("p2p-port", 31317, "mainnet P2P port")
	tep := f.String("tep-public-key", "", "actual host X25519 public key, hex")
	if e := f.Parse(args[1:]); e != nil {
		return e
	}
	if f.NArg() != 0 && args[0] != "funding-plan" && args[0] != "verify-funding" && args[0] != "assemble" && args[0] != "sign-funding" {
		return fmt.Errorf("unexpected arguments")
	}
	var c blockchain.ProtocolV2Config
	if e := read(*config, &c); e != nil {
		return e
	}
	switch args[0] {
	case "funding-plan", "verify-funding", "assemble", "sign-funding":
		var identities []mainnetidentity.Identity
		for _, path := range f.Args() {
			var v mainnetidentity.Identity
			if e := read(path, &v); e != nil {
				return e
			}
			identities = append(identities, v)
		}
		if args[0] == "verify-funding" || args[0] == "assemble" {
			var txs []*protocolv2.TransactionV2
			if e := read(*signed, &txs); e != nil {
				return e
			}
			if e := mainnetidentity.VerifyFunding(c, identities, txs); e != nil {
				return e
			}
			if args[0] == "assemble" {
				if e := mainnetidentity.Assemble(*out, *economic, c, identities, txs); e != nil {
					return e
				}
				fmt.Println("OFFLINE_VALIDATOR_CHECKPOINT_REPLAY_VERIFIED_RUNTIME_ACCEPTANCE_REQUIRED")
				return nil
			}
			fmt.Println("FUNDING_SIGNATURES_AND_PLAN_VERIFIED_NO_STATE_CHANGED")
			return nil
		}
		plan, e := mainnetidentity.PlanFunding(c, identities)
		if e != nil {
			return e
		}
		if args[0] == "sign-funding" {
			b, e := readPrivate(*key)
			if e != nil {
				return e
			}
			defer func() {
				for i := range b {
					b[i] = 0
				}
			}()
			w, e := wallet.FromPrivateKeyHex(strings.TrimSpace(string(b)))
			if e != nil {
				return e
			}
			if !wallet.AddressEqual(w.Address(), plan.Founder) {
				return fmt.Errorf("private key does not match founder")
			}
			for _, tx := range plan.Transfers {
				if e = tx.Sign(w); e != nil {
					return e
				}
			}
			if e = mainnetidentity.VerifyFunding(c, identities, plan.Transfers); e != nil {
				return e
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(plan.Transfers)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(plan)
	case "generate":
		if e := mainnetidentity.Generate(*out, mainnetidentity.Options{Config: c, NodeID: *node, IP: *ip, P2PPort: *port, TEPPublicKey: *tep}); e != nil {
			return e
		}
		fmt.Println("MAINNET_ENROLLMENT_CREATED transfer_only=public.json NO_FUNDING_NO_SERVICE_STARTED")
	case "verify":
		var v mainnetidentity.Identity
		if e := read(*public, &v); e != nil {
			return e
		}
		if e := mainnetidentity.Verify(c, v); e != nil {
			return e
		}
		fmt.Println("MAINNET_ENROLLMENT_SIGNATURES_OK NO_CHECKPOINT_OR_ACTIVATION")
	default:
		return fmt.Errorf("unknown command")
	}
	return nil
}
func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}

func readPrivate(path string) ([]byte, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil || real != abs {
		return nil, fmt.Errorf("canonical private file required")
	}
	before, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0 || before.Size() > 65536 {
		return nil, fmt.Errorf("regular owner-only private file up to 64 KiB required")
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		return nil, fmt.Errorf("private file changed")
	}
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if len(b) > 65536 {
		return nil, fmt.Errorf("private file too large")
	}
	return b, err
}
