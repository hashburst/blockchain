// hvm-mainnet-identity creates local keys and exports a public signed enrollment.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"hashburst/blockchain"
	"hashburst/internal/mainnetidentity"
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
		return fmt.Errorf("use generate or verify")
	}
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	config := f.String("protocol", "", "complete reviewed protocol JSON")
	out := f.String("out", "", "new local private directory; existing destination refused")
	public := f.String("public", "", "public enrollment JSON for verification")
	node := f.String("node-id", "", "unique mainnet node ID")
	ip := f.String("ip", "", "advertised IPv4")
	port := f.Int("p2p-port", 31317, "mainnet P2P port")
	tep := f.String("tep-public-key", "", "actual host X25519 public key, hex")
	if e := f.Parse(args[1:]); e != nil {
		return e
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	var c blockchain.ProtocolV2Config
	if e := read(*config, &c); e != nil {
		return e
	}
	switch args[0] {
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
