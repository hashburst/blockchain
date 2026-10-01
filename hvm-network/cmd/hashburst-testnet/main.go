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
	migration := flag.String("migrate-evm", "", "offline: migrate pinned testnet to this candidate config; safely resume the same transition")
	apowMigration := flag.String("migrate-apow", "", "offline: schedule APoW and optional future gas capacity; preserve existing state")
	flag.Parse()
	if *path == "" || (*provision && *check) {
		log.Print("--config required; --provision and --check are exclusive")
		return 2
	}
	if *apowMigration != "" {
		if *migration != "" || *provision || *check {
			log.Print("APoW migration excludes other actions")
			return 2
		}
		if e := testnet.MigrateAPoW(*path, *apowMigration); e != nil {
			log.Printf("APoW migration: %v", e)
			return 1
		}
		log.Print("HVM_APOW_CONFIG_MIGRATION_OK_NO_SERVICE_STARTED")
		return 0
	}
	if *migration != "" {
		if *provision || *check {
			log.Print("migration excludes provision/check")
			return 2
		}
		if e := testnet.MigrateEVM(*path, *migration); e != nil {
			log.Printf("migration: %v", e)
			return 1
		}
		log.Print("HVM_EVM_CONFIG_MIGRATION_OK")
		return 0
	}
	c, e := testnet.Load(*path)
	if e != nil {
		log.Printf("config: %v", e)
		return 1
	}
	if c.Network != "testnet" {
		log.Print("testnet executable refuses another network")
		return 1
	}
	var s *testnet.State
 if *provision || *check { s,e=testnet.Prepare(c,*provision) } else { s,e=testnet.PrepareRuntime(c) }
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
