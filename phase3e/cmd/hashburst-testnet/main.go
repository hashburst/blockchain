package main

import (
	"context"
	"flag"
	"hashburst/internal/testnet"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() { os.Exit(run()) }
func run() int {
	path := flag.String("config", "", "required testnet configuration JSON")
	provision := flag.Bool("provision", false, "pin an existing prepared testnet checkpoint; do not start")
	check := flag.Bool("check", false, "validate pinned state and keys; do not start")
	flag.Parse()
	if *path == "" || (*provision && *check) {
		log.Print("--config required; --provision and --check are exclusive")
		return 2
	}
	c, e := testnet.Load(*path)
	if e != nil {
		log.Printf("config: %v", e)
		return 1
	}
	s, e := testnet.Prepare(c, *provision)
	if e != nil {
		log.Printf("state: %v", e)
		return 1
	}
	defer s.Close()
	if *provision || *check {
		log.Printf("TESTNET_STATE_OK digest=%s height=%d", c.Pin(), s.Chain.Height())
		return 0
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if e = testnet.Run(ctx, s); e != nil {
		log.Printf("runtime: %v", e)
		return 1
	}
	return 0
}
