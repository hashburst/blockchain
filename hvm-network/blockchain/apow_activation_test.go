package blockchain

import "testing"

func TestAPoWGasScheduleAndRecoveryBoundary(t *testing.T) {
	old := phase3CTestConfig()
	old.ChainID = 4735490
	old.EVM = &EVMConfig{ActivationHeight: 7, GasLimit: 200000, BaseFeeWei: 1}
	next := old.detached()
	next.APoW = &APoWConfig{ActivationHeight: 10000, InitialBits: 4, MinBits: 1, MaxBits: 16, Window: 4, TargetSeconds: 5, GasLimit: 2000000}
	if e := next.Validate(); e != nil {
		t.Fatal(e)
	}
	for _, h := range []int{7, 9999} {
		if next.EVMGasLimitAt(h) != old.EVMGasLimitAt(h) {
			t.Fatal("historic gas changed")
		}
	}
	if next.EVMGasLimitAt(10000) != 2000000 {
		t.Fatal("new gas not activated")
	}
	bc := &Blockchain{v2Config: next}
	snap := &consensusRecovery{Height: 9999, ConfigHash: recoveryProtocolHash(old)}
	if !bc.recoveryConfigMatches(snap) {
		t.Fatal("old lock not accepted before activation")
	}
	snap.Height = 10000
	if bc.recoveryConfigMatches(snap) {
		t.Fatal("old rules accepted at activation")
	}
	snap.Height = 9999
	different := old.detached()
	different.EVM.GasLimit++
	snap.ConfigHash = recoveryProtocolHash(different)
	if bc.recoveryConfigMatches(snap) {
		t.Fatal("arbitrary predecessor accepted")
	}
	next.APoW.GasLimit = 100000
	if next.Validate() == nil {
		t.Fatal("gas reduction accepted")
	}
	next.APoW.GasLimit = 30000001
	if next.Validate() == nil {
		t.Fatal("oversized gas accepted")
	}
}
