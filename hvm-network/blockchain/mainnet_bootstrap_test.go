package blockchain

import "testing"

func bootstrapFixture() ProtocolV2Config {
	c := importFixtureConfig()
	c.ActivationHeight = 1
	c.MainnetBootstrapEnd = 3
	c.ConsensusActivationHeight = 4
	c.Validator.ActivationDelay = 1
	c.LegacyPoWDifficulty = 1
	c.PoHTicksPerBlock = 4000
	c.EVM = nil
	c.APoW = nil
	return c
}
func TestMainnetBootstrapZeroIssuanceReplayAndBoundary(t *testing.T) {
	c := bootstrapFixture()
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	bc := NewBlockchainWithDirAndV2Config(dir, c)
	g, e := bc.BlockAt(0)
	if e != nil {
		t.Fatal(e)
	}
	initial := bc.BalanceUnits(c.GenesisImport.Recipient)
	for i := 1; i <= 3; i++ {
		if e = bc.AddBlock(c.GenesisImport.Recipient); e != nil {
			t.Fatal(e)
		}
	}
	if bc.BalanceUnits(c.GenesisImport.Recipient) != initial {
		t.Fatal("bootstrap issued coins")
	}
	b, e := bc.BlockAt(3)
	if e != nil {
		t.Fatal(e)
	}
	root := bc.HBTStateRoot()
	if e = bc.AddBlock(c.GenesisImport.Recipient); e == nil {
		t.Fatal("consensus activation bypassed")
	}
	bc.CloseHistory()
	reopened, e := OpenExistingBlockchain(dir, c, g.Hash, 3, b.Hash)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.CloseHistory()
	if reopened.HBTStateRoot() != root || reopened.BalanceUnits(c.GenesisImport.Recipient) != initial {
		t.Fatal("replay changed balances")
	}
	bad := *b
	bad.Transactions = append([]*Transaction(nil), b.Transactions...)
	bad.Transactions[0] = NewSystemReward(c.GenesisImport.Recipient, 50)
	prev, e := reopened.BlockAt(2)
	if e != nil {
		t.Fatal(e)
	}
	if ValidateBlockAgainstConfig(prev, &bad, DefaultMiningReward, c) == nil {
		t.Fatal("bootstrap emission accepted")
	}
	changed := c
	changed.ChainID = 4735490
	if changed.Validate() == nil {
		t.Fatal("testnet policy accepted")
	}
	changed = c
	changed.ConsensusActivationHeight++
	if changed.Validate() == nil {
		t.Fatal("unbounded gap accepted")
	}
	changed = c
	changed.MainnetBootstrapEnd = 4097
	if changed.Validate() == nil {
		t.Fatal("unbounded bootstrap accepted")
	}
}
