package protocolv2

import (
	"fmt"
	"math"
	"math/big"
)

// FeePolicy expresses HVM compute pricing in native HBT atomic units without
// forcing Ethereum's 18-decimal gas accounting into the native ledger.
// FeeRateUnitsPerMillion means: HBT atomic units charged per 1,000,000 compute units.
type FeePolicy struct {
	FeeRateUnitsPerMillion int64 `json:"fee_rate_units_per_million"`
	BaseTxUnits            int64 `json:"base_tx_units"`
}

func (p FeePolicy) Validate() error {
	if p.FeeRateUnitsPerMillion < 0 || p.BaseTxUnits < 0 {
		return fmt.Errorf("negative fee policy")
	}
	return nil
}

func (p FeePolicy) ComputeFee(computeUsed uint64) (int64, error) {
	if err := p.Validate(); err != nil {
		return 0, err
	}

	// ceil(computeUsed * rate / 1,000,000), calculated with big.Int so fee
	// arithmetic itself can never wrap and accidentally undercharge consensus.
	used := new(big.Int).SetUint64(computeUsed)
	rate := big.NewInt(p.FeeRateUnitsPerMillion)
	variable := new(big.Int).Mul(used, rate)
	if variable.Sign() != 0 {
		variable.Add(variable, big.NewInt(999_999))
	}
	variable.Div(variable, big.NewInt(1_000_000))
	variable.Add(variable, big.NewInt(p.BaseTxUnits))
	if !variable.IsInt64() || variable.Int64() < 0 || variable.Int64() > math.MaxInt64 {
		return 0, fmt.Errorf("fee overflow")
	}
	return variable.Int64(), nil
}

func (p FeePolicy) MaxFeeForLimit(computeLimit uint64) (int64, error) {
	return p.ComputeFee(computeLimit)
}
