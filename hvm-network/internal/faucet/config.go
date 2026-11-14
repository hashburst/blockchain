package faucet

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"hashburst/wallet"
)

const (
	TestnetChainID uint64 = 4735490
	MainnetChainID uint64 = 4735489
)

type Config struct {
	ChainID          uint64        `json:"chain_id"`
	AmountUnits      int64         `json:"amount_units"`
	Cooldown         time.Duration `json:"-"`
	DailyCapUnits    int64         `json:"daily_cap_units"`
	GlobalReserve    int64         `json:"global_reserve_units"`
	Protected        []string      `json:"protected_addresses"`
}

func (c Config) Validate() error {
	if c.ChainID != TestnetChainID && c.ChainID != MainnetChainID {
		return errors.New("faucet requires chain 4735490 or 4735489")
	}
	if c.AmountUnits <= 0 || c.DailyCapUnits < c.AmountUnits || c.GlobalReserve < c.AmountUnits {
		return errors.New("invalid positive faucet amount/cap/reserve")
	}
	if c.Cooldown <= 0 || c.Cooldown > 30*24*time.Hour {
		return errors.New("invalid faucet cooldown")
	}
	seen := map[string]bool{}
	for _, a := range c.Protected {
		if !wallet.IsValidAddress(a) {
			return fmt.Errorf("invalid protected address %q", a)
		}
		k := strings.ToLower(a)
		if seen[k] {
			return errors.New("duplicate protected address")
		}
		seen[k] = true
	}
	return nil
}

func validRecipient(a string) bool {
	return wallet.IsValidAddress(a) && !strings.EqualFold(a, "0x0000000000000000000000000000000000000000")
}
