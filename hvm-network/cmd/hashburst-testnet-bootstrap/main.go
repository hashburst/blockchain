package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"hashburst/internal/bootstrap"
	"os"
)

func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, "BOOTSTRAP_FAILED:", e)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("use identity or assemble; no service is started")
	}
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	out := f.String("out", "", "new absolute output directory; parent must exist")
	chain := f.Uint64("chain-id", 0, "approved TESTNET ID; no default, legacy1337 forbidden")
	switch args[0] {
	case "identity":
		node := f.String("node-id", "", "unique testnet node ID")
		ip := f.String("ip", "", "advertised IPv4")
		port := f.Int("p2p-port", 31307, "dedicated P2P port")
		tep := f.String("tep-public-key", "", "existing TEP X25519 public key, hex")
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		if f.NArg() != 0 {
			return fmt.Errorf("unexpected arguments")
		}
		if e := bootstrap.Generate(*out, bootstrap.Options{ChainID: *chain, NodeID: *node, IP: *ip, P2PPort: *port, TEPPublicKey: *tep}); e != nil {
			return e
		}
		fmt.Println("TESTNET_IDENTITY_READY transfer_only=public.json NO_SERVICE_STARTED")
	case "assemble":
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		identities := []bootstrap.Identity{}
		for _, path := range f.Args() {
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			if len(b) > 65536 {
				return fmt.Errorf("identity too large")
			}
			var v bootstrap.Identity
			if e = json.Unmarshal(b, &v); e != nil {
				return e
			}
			identities = append(identities, v)
		}
		if e := bootstrap.Assemble(*out, *chain, identities); e != nil {
			return e
		}
		fmt.Println("TESTNET_BOOTSTRAP_READY NO_SERVICE_STARTED")
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	return nil
}
