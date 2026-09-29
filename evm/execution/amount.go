package execution

import (
	"fmt"
	"math/big"
)

const WeiPerNativeUnit int64 = 10000000000

// NativeToWei is exact and never uses floating point.
func NativeToWei(units int64) (*big.Int, error) {
	if units < 0 {
		return nil, fmt.Errorf("negative native amount")
	}
	return new(big.Int).Mul(big.NewInt(units), big.NewInt(WeiPerNativeUnit)), nil
}

// SplitWei retains sub-native-unit dust explicitly: q*1e10+r always equals wei.
// It is a conversion primitive, not authority to credit either ledger.
func SplitWei(wei *big.Int) (int64, int64, error) {
	if wei == nil || wei.Sign() < 0 {
		return 0, 0, fmt.Errorf("invalid wei amount")
	}
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(wei, big.NewInt(WeiPerNativeUnit), r)
	if !q.IsInt64() {
		return 0, 0, fmt.Errorf("native amount overflow")
	}
	return q.Int64(), r.Int64(), nil
}
