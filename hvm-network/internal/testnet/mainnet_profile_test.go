package testnet

import (
	"hashburst/blockchain"
	"testing"
)

func mainnetFixture(t *testing.T) Config {
	c := fixture(t)
	c.Network = "mainnet"
	c.Protocol.ChainID = 4735489
	imp := blockchain.ApprovedMainnetGenesisImport()
	c.Protocol.GenesisImport = &imp
	c.CheckpointHeight = 8
	c.Protocol.ActivationHeight = 1
	c.Protocol.ConsensusActivationHeight = 8
	c.Protocol.EVM = &blockchain.EVMConfig{ActivationHeight: 100000, GasLimit: 200000, BaseFeeWei: 1}
	c.DataDir = "/var/lib/hashburst-hvm-mainnet-ingress"
	c.P2PKeyFile = "/etc/hashburst-hvm-mainnet-ingress/p2p.key"
	c.RPCListen = "127.0.0.1:18019"
	c.P2PPort = 31317
	return c
}
func TestMainnetProfileIsolation(t *testing.T) {
	c := mainnetFixture(t)
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	cases := map[string]func(*Config){
		"testnet id":            func(c *Config) { c.Protocol.ChainID = 4735490 },
		"legacy id":             func(c *Config) { c.Protocol.ChainID = 1337 },
		"economic genesis only": func(c *Config) { c.CheckpointHeight = 0 },
		"missing import":        func(c *Config) { c.Protocol.GenesisImport = nil },
		"no evm":                func(c *Config) { c.Protocol.EVM = nil },
		"testnet data":          func(c *Config) { c.DataDir = "/var/lib/hashburst-hvm-testnet" },
		"testnet key":           func(c *Config) { c.P2PKeyFile = "/etc/hashburst-hvm-testnet/p2p.key" },
		"traversal":             func(c *Config) { c.P2PKeyFile = "/etc/hashburst-hvm-mainnet-ingress/../hashburst-hvm-testnet/p2p.key" },
		"rpc port":              func(c *Config) { c.RPCListen = "127.0.0.1:18009" },
		"p2p port":              func(c *Config) { c.P2PPort = 31307 },
		"observer signing":      func(c *Config) { c.ConsensusKeyFile = "/etc/hashburst-hvm-mainnet-ingress/sign.key" },
		"validator data": func(c *Config) {
			c.Role = "validator"
			c.ValidatorID = "test"
			c.ConsensusKeyFile = "/etc/hashburst-hvm-mainnet/sign.key"
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			bad := c
			change(&bad)
			if bad.Validate() == nil {
				t.Fatal("accepted cross-network or invalid mainnet config")
			}
		})
	}
	v := c
	v.Role = "validator"
	v.DataDir = "/var/lib/hashburst-hvm-mainnet"
	v.P2PKeyFile = "/etc/hashburst-hvm-mainnet/p2p.key"
	v.ConsensusKeyFile = "/etc/hashburst-hvm-mainnet/consensus.key"
	v.ValidatorID = "test"
	if e := v.Validate(); e != nil {
		t.Fatal(e)
	}
	other := c
	other.Network = "testnet"
	if other.Validate() == nil {
		t.Fatal("mainnet EVM under testnet label")
	}
	if other.Pin() == c.Pin() {
		t.Fatal("network missing from pin")
	}
}

func TestMainnetBootstrapCheckpointBoundary(t *testing.T) {
	c := mainnetFixture(t)
	c.Protocol.MainnetBootstrapEnd = 3
	c.Protocol.ConsensusActivationHeight = 4
	c.Protocol.Validator.ActivationDelay = 1
	c.CheckpointHeight = 3
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	for _, height := range []int{0, 1, 2} {
		bad := c
		bad.CheckpointHeight = height
		if bad.Validate() == nil {
			t.Fatalf("accepted incomplete checkpoint %d", height)
		}
	}
	bad := c
	bad.Protocol.ConsensusActivationHeight = 5
	if bad.Validate() == nil {
		t.Fatal("accepted bootstrap/consensus gap")
	}
	c.Protocol.LegacyPoWDifficulty = 1
	c.Protocol.PoHTicksPerBlock = 4000
	bc := blockchain.NewBlockchainWithDirAndV2Config(t.TempDir(), c.Protocol)
	defer bc.CloseHistory()
	for i := 0; i < 3; i++ {
		if e := bc.AddBlock(c.Protocol.GenesisImport.Recipient); e != nil {
			t.Fatal(e)
		}
	}
	if validateBootstrapState(c, bc) == nil {
		t.Fatal("accepted replayed checkpoint without validators")
	}
}
