package blockchain

import (
	"fmt"
	"math"

	"hashburst/consensus"
	"hashburst/protocolv2"
	"hashburst/wallet"
)

const (
	BlockVersionLegacy uint16 = 1
	BlockVersionV2     uint16 = 2

	// DisabledActivationHeight is deliberately impossible for a practical chain
	// height. Phase 3B ships Protocol V2 dark: production behavior remains V1
	// unless a later, explicit consensus release changes this value/config.
	DisabledActivationHeight uint64 = math.MaxUint64

	DefaultMaxV2DataBytes = 1 << 20 // 1 MiB hard admission guard, provisional.
)

// ProtocolV2Config contains consensus-relevant Phase 3B parameters. Economic
// values are provisional while ActivationHeight is disabled; before testnet or
// mainnet activation they must be frozen in a versioned protocol release.
type ProtocolV2Config struct {
	ChainID                   uint64                  `json:"chain_id"`
	ActivationHeight          uint64                  `json:"activation_height"`
	FeePolicy                 protocolv2.FeePolicy    `json:"fee_policy"`
	FeeCollector              string                  `json:"fee_collector"`
	MaxTxDataBytes            int                     `json:"max_tx_data_bytes"`
	LegacyPoWDifficulty       int                     `json:"legacy_pow_difficulty"`
	PoHTicksPerBlock          int                     `json:"poh_ticks_per_block"`
	ConsensusActivationHeight uint64                  `json:"consensus_activation_height"`
	Validator                 consensus.Config        `json:"validator"`
	ConsensusNetwork          consensus.NetworkConfig `json:"consensus_network"`
	RequireNodeRegistrationV2 bool                    `json:"require_node_registration_v2"`
	SlashCollector            string                  `json:"slash_collector"`
}

func DefaultProtocolV2Config() ProtocolV2Config {
	return ProtocolV2Config{
		ChainID:          1337,
		ActivationHeight: DisabledActivationHeight,
		// Provisional development pricing only. It has no production effect while
		// Protocol V2 is disabled.
		FeePolicy: protocolv2.FeePolicy{
			BaseTxUnits:            100,
			FeeRateUnitsPerMillion: 1_000,
		},
		FeeCollector:              "0x000000000000000000000000000000000000fee0",
		MaxTxDataBytes:            DefaultMaxV2DataBytes,
		LegacyPoWDifficulty:       Difficulty,
		PoHTicksPerBlock:          PoHTicks,
		ConsensusActivationHeight: DisabledActivationHeight,
		Validator: consensus.Config{
			MinBondUnits:       1000 * AmountScale,
			ActivationDelay:    1,
			UnbondingBlocks:    100,
			JailBlocks:         20,
			DoubleVoteSlashBPS: 500,
		},
		ConsensusNetwork:          consensus.DefaultNetworkConfig(),
		RequireNodeRegistrationV2: true,
		SlashCollector:            "0x00000000000000000000000000000000000051a5",
	}
}

func (c ProtocolV2Config) EnabledAt(height int) bool {
	return height >= 0 && uint64(height) >= c.ActivationHeight
}

func (c ProtocolV2Config) ConsensusEnabledAt(height int) bool {
	return height >= 0 && c.EnabledAt(height) && uint64(height) >= c.ConsensusActivationHeight
}

// LegacyDifficulty keeps older configs that predate this field compatible: a
// zero value means the historical production Difficulty constant.
func (c ProtocolV2Config) LegacyDifficulty() int {
	if c.LegacyPoWDifficulty == 0 {
		return Difficulty
	}
	return c.LegacyPoWDifficulty
}

// EffectivePoHTicks keeps configs serialized before this field was introduced
// compatible: zero means the historical production PoHTicks value. Phase 3E
// isolated test/devnet manifests explicitly select a lower value.
func (c ProtocolV2Config) EffectivePoHTicks() int {
	if c.PoHTicksPerBlock == 0 {
		return PoHTicks
	}
	return c.PoHTicksPerBlock
}

func (c ProtocolV2Config) Validate() error {
	if c.ChainID == 0 {
		return fmt.Errorf("protocol v2 chain id must be non-zero")
	}
	if err := c.FeePolicy.Validate(); err != nil {
		return fmt.Errorf("fee policy: %w", err)
	}
	if !wallet.IsValidAddress(c.FeeCollector) {
		return fmt.Errorf("invalid fee collector address")
	}
	if c.MaxTxDataBytes <= 0 {
		return fmt.Errorf("max v2 transaction data must be positive")
	}
	if err := validatePoWDifficulty(c.LegacyDifficulty()); err != nil {
		return fmt.Errorf("legacy pow difficulty: %w", err)
	}
	if ticks := c.EffectivePoHTicks(); ticks < 1 || ticks > PoHTicks {
		return fmt.Errorf("poh ticks per block must be between 1 and %d", PoHTicks)
	}
	if c.ConsensusActivationHeight < c.ActivationHeight {
		return fmt.Errorf("consensus activation cannot precede Protocol V2 activation")
	}
	if err := c.Validator.Validate(); err != nil {
		return fmt.Errorf("validator config: %w", err)
	}
	if err := c.ConsensusNetwork.Validate(); err != nil {
		return fmt.Errorf("consensus network config: %w", err)
	}
	if !wallet.IsValidAddress(c.SlashCollector) {
		return fmt.Errorf("invalid slash collector address")
	}
	return nil
}
