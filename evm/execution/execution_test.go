package execution

import (
	"context"
	"encoding/hex"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
	"math/big"
	"testing"
)

const privateTestKey = "0123456789012345678901234567890123456789012345678901234567890123"

func fixture(t *testing.T) (*state.StateDB, common.Address, Block) {
	t.Helper()
	s, e := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if e != nil {
		t.Fatal(e)
	}
	key, _ := crypto.HexToECDSA(privateTestKey)
	sender := crypto.PubkeyToAddress(key.PublicKey)
	s.SetBalance(sender, uint256.NewInt(1000000000), tracing.BalanceChangeUnspecified)
	return s, sender, Block{Number: 1, Time: 100, Hash: common.HexToHash("0x01"), GasLimit: 10000000, BaseFee: big.NewInt(1), HashAt: func(uint64) common.Hash { return common.Hash{} }}
}
func sign(t *testing.T, nonce uint64, to *common.Address, data []byte, gas uint64, chain uint64) []byte {
	t.Helper()
	key, _ := crypto.HexToECDSA(privateTestKey)
	tx := types.NewTx(&types.DynamicFeeTx{ChainID: new(big.Int).SetUint64(chain), Nonce: nonce, To: to, Value: big.NewInt(7), Gas: gas, GasFeeCap: big.NewInt(3), GasTipCap: big.NewInt(1), Data: data})
	tx, e := types.SignTx(tx, types.NewCancunSigner(new(big.Int).SetUint64(chain)), key)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := tx.MarshalBinary()
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
func TestSignedTransferAndReplay(t *testing.T) {
	s, from, b := fixture(t)
	to := common.HexToAddress("0x1234")
	raw := sign(t, 0, &to, nil, 21000, TestnetID)
	r, e := ApplyBlock(context.Background(), s, TestnetID, b, [][]byte{raw})
	if e != nil {
		t.Fatal(e)
	}
	if s.GetNonce(from) != 0 || s.GetBalance(to).Sign() != 0 {
		t.Fatal("parent mutated")
	}
	if r.State.GetNonce(from) != 1 || r.State.GetBalance(to).Uint64() != 7 || r.Receipts[0].GasUsed != 21000 || r.Receipts[0].Status != 1 {
		t.Fatal("transfer mismatch")
	}
	if r.State.GetBalance(from).Uint64() != 1000000000-7-42000 {
		t.Fatal("gas accounting")
	}
	if _, e = ApplyBlock(context.Background(), r.State, TestnetID, b, [][]byte{raw}); e == nil {
		t.Fatal("replay accepted")
	}
	if _, _, e = Decode(raw, MainnetID); e == nil {
		t.Fatal("wrong chain accepted")
	}
	r2, e := ApplyBlock(context.Background(), s, TestnetID, b, [][]byte{raw})
	if e != nil || r2.StateRoot != r.StateRoot || r2.ReceiptsRoot != r.ReceiptsRoot {
		t.Fatal("nondeterministic execution")
	}
}
func TestDeployStorageLogAndRevert(t *testing.T) {
	s, from, b := fixture(t)
	// Runtime stores 42 at slot 0 and emits LOG0 with empty data.
	runtime, _ := hex.DecodeString("602a60005560006000a000")
	init := append([]byte{0x60, byte(len(runtime)), 0x60, 12, 0x60, 0, 0x39, 0x60, byte(len(runtime)), 0x60, 0, 0xf3}, runtime...)
	contract := crypto.CreateAddress(from, 0)
	deploy := sign(t, 0, nil, init, 200000, TestnetID)
	call := sign(t, 1, &contract, nil, 100000, TestnetID)
	r, e := ApplyBlock(context.Background(), s, TestnetID, b, [][]byte{deploy, call})
	if e != nil {
		t.Fatal(e)
	}
	if r.Receipts[0].ContractAddress != contract || r.Receipts[1].Status != 1 || len(r.Receipts[1].Logs) != 1 || r.State.GetState(contract, common.Hash{}) != common.HexToHash("0x2a") {
		t.Fatal("contract state/log mismatch")
	}
	// REVERT discards value/storage, but retains nonce and gas payment.
	target := common.HexToAddress("0x5678")
	s.SetCode(target, []byte{0x60, 0, 0x60, 0, 0xfd}, tracing.CodeChangeUnspecified)
	r, e = ApplyBlock(context.Background(), s, TestnetID, b, [][]byte{sign(t, 0, &target, nil, 100000, TestnetID)})
	if e != nil {
		t.Fatal(e)
	}
	if r.Receipts[0].Status != 0 || r.State.GetNonce(from) != 1 || r.State.GetBalance(target).Sign() != 0 || r.State.GetBalance(from).Uint64() >= 1000000000 {
		t.Fatal("revert accounting mismatch")
	}
}
func TestInvalidBlockIsAtomic(t *testing.T) {
	s, from, b := fixture(t)
	to := common.HexToAddress("0x1234")
	if _, e := ApplyBlock(context.Background(), s, TestnetID, b, [][]byte{sign(t, 0, &to, nil, 21000, TestnetID), sign(t, 0, &to, nil, 21000, TestnetID)}); e == nil {
		t.Fatal("invalid nonce accepted")
	}
	if s.GetNonce(from) != 0 || s.GetBalance(from).Uint64() != 1000000000 {
		t.Fatal("rejected block modified parent")
	}
}
func TestDecodeRejectsMalformedAndUnprotected(t *testing.T) {
	key, _ := crypto.HexToECDSA(privateTestKey)
	tx, e := types.SignTx(types.NewTx(&types.LegacyTx{Gas: 21000, GasPrice: big.NewInt(1)}), types.HomesteadSigner{}, key)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := tx.MarshalBinary()
	for _, b := range [][]byte{nil, {1, 2, 3}, raw, make([]byte, MaxRawBytes+1)} {
		if _, _, e = Decode(b, TestnetID); e == nil {
			t.Fatal("invalid envelope accepted")
		}
	}
}
