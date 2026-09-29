package bootstrap

import "testing"

func TestTestnetBootstrapRejectsMainnet(t *testing.T) {
	if CheckChain(4735489) == nil {
		t.Fatal("testnet bootstrap accepted mainnet")
	}
	if err := CheckChain(4735490); err != nil {
		t.Fatal(err)
	}
}
