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
	"io"
	"os"
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
	signed := f.String("signed-funding", "", "signed founder transfer array")
	public := f.String("public", "", "public enrollment JSON for verification")
	node := f.String("node-id", "", "unique mainnet node ID")
	ip := f.String("ip", "", "advertised IPv4")
	port := f.Int("p2p-port", 31317, "mainnet P2P port")
	tep := f.String("tep-public-key", "", "actual host X25519 public key, hex")
	if e := f.Parse(args[1:]); e != nil {
		return e
	}
	if f.NArg() != 0 && args[0] != "funding-plan" && args[0] != "verify-funding" {
		return fmt.Errorf("unexpected arguments")
	}
	var c blockchain.ProtocolV2Config
	if e := read(*config, &c); e != nil {
		return e
	}
	switch args[0] {
	case "funding-plan", "verify-funding":
		var identities []mainnetidentity.Identity
		for _, path := range f.Args() {
			var v mainnetidentity.Identity
			if e := read(path, &v); e != nil {
				return e
			}
			identities = append(identities, v)
		}
		if args[0] == "verify-funding" {
			var txs []*protocolv2.TransactionV2
			if e := read(*signed, &txs); e != nil {
				return e
			}
			if e := mainnetidentity.VerifyFunding(c, identities, txs); e != nil {
				return e
			}
			fmt.Println("FUNDING_SIGNATURES_AND_PLAN_VERIFIED_NO_STATE_CHANGED")
			return nil
		}
		plan, e := mainnetidentity.PlanFunding(c, identities)
		if e != nil {
			return e
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
