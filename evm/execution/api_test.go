package execution

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"math/big"
	"testing"
)

type testBackend struct {
	s        *state.StateDB
	b        Block
	admitErr error
	admitted bool
	tx       *types.Transaction
	receipt  *types.Receipt
}

func (b *testBackend) Snapshot(context.Context, rpc.BlockNumber) (*state.StateDB, Block, error) {
	return b.s.Copy(), b.b, nil
}
func (b *testBackend) Receipt(context.Context, common.Hash) (*types.Receipt, error) {
	return b.receipt, nil
}
func (b *testBackend) Admit(_ context.Context, raw []byte) (common.Hash, error) {
	b.admitted = true
	if b.admitErr != nil {
		return common.Hash{}, b.admitErr
	}
	tx, _, e := Decode(raw, TestnetID)
	if e != nil {
		return common.Hash{}, e
	}
	return tx.Hash(), nil
}
func TestEthereumRPCSimulationAndAdmission(t *testing.T) {
	s, from, b := fixture(t)
	backend := &testBackend{s: s, b: b}
	api, e := NewAPI(TestnetID, backend)
	if e != nil {
		t.Fatal(e)
	}
	server := rpc.NewServer()
	defer server.Stop()
	if e = server.RegisterName("eth", api); e != nil {
		t.Fatal(e)
	}
	client := rpc.DialInProc(server)
	defer client.Close()
	ctx := context.Background()
	var id string
	if e = client.CallContext(ctx, &id, "eth_chainId"); e != nil || id != "0x484202" {
		t.Fatalf("chain: %s %v", id, e)
	}
	to := common.HexToAddress("0x1234")
	code, _ := hex.DecodeString("602a60005260206000f3")
	s.SetCode(to, code, tracing.CodeChangeUnspecified)
	var output hexutil.Bytes
	if e = client.CallContext(ctx, &output, "eth_call", CallArgs{From: &from, To: &to}, "latest"); e != nil || len(output) != 32 || output[31] != 42 {
		t.Fatalf("call: %x %v", output, e)
	}
	plain := common.HexToAddress("0x9999")
	var gas hexutil.Uint64
	if e = client.CallContext(ctx, &gas, "eth_estimateGas", CallArgs{From: &from, To: &plain}); e != nil || gas != 21000 {
		t.Fatalf("estimate: %d %v", gas, e)
	}
	if s.GetNonce(from) != 0 || s.GetBalance(from).Uint64() != 1000000000 {
		t.Fatal("simulation mutated canonical state")
	}
	var receipt *types.Receipt
	if e = client.CallContext(ctx, &receipt, "eth_getTransactionReceipt", common.Hash{}); e != nil || receipt != nil {
		t.Fatal("unknown receipt must be null")
	}
	var slot common.Hash
	if e = client.CallContext(ctx, &slot, "eth_getStorageAt", to, "0x0", "latest"); e != nil {
		t.Fatal(e)
	}
	raw := sign(t, 0, &plain, nil, 21000, TestnetID)
	backend.admitErr = errors.New("mempool unavailable")
	var hash common.Hash
	if e = client.CallContext(ctx, &hash, "eth_sendRawTransaction", hexutil.Bytes(raw)); e == nil || !backend.admitted {
		t.Fatal("unavailable mempool reported success")
	}
	backend.admitErr = nil
	if e = client.CallContext(ctx, &hash, "eth_sendRawTransaction", hexutil.Bytes(raw)); e != nil {
		t.Fatal(e)
	}
	tx, _, _ := Decode(raw, TestnetID)
	if hash != tx.Hash() {
		t.Fatal("hash mismatch")
	}
}
func TestCallRevertAndCancellation(t *testing.T) {
	s, from, b := fixture(t)
	to := common.HexToAddress("0x1234")
	s.SetCode(to, []byte{0x60, 0, 0x60, 0, 0xfd}, tracing.CodeChangeUnspecified)
	api, _ := NewAPI(TestnetID, &testBackend{s: s, b: b})
	_, e := api.Call(context.Background(), CallArgs{From: &from, To: &to}, rpc.LatestBlockNumber)
	var revert *revertError
	if !errors.As(e, &revert) || revert.ErrorCode() != 3 {
		t.Fatal("missing Ethereum revert error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = api.Call(ctx, CallArgs{From: &from, To: &to}, rpc.LatestBlockNumber); !errors.Is(e, context.Canceled) {
		t.Fatalf("cancellation: %v", e)
	}
	negative := hexutil.Big(*big.NewInt(-1))
	if _, e = api.Call(context.Background(), CallArgs{From: &from, To: &to, Value: &negative}, rpc.LatestBlockNumber); e == nil {
		t.Fatal("negative value accepted")
	}
}

func (b *testBackend) Transaction(context.Context, common.Hash) (*types.Transaction, error) {
	return b.tx, nil
}

func TestReceiptEthereumFields(t *testing.T) {
	s, from, b := fixture(t)
	to := common.HexToAddress("0x9999")
	raw := sign(t, 0, &to, nil, 21000, TestnetID)
	result, e := ApplyBlock(context.Background(), s, TestnetID, b, [][]byte{raw})
	if e != nil {
		t.Fatal(e)
	}
	tx, _, _ := Decode(raw, TestnetID)
	backend := &testBackend{s: s, b: b, tx: tx, receipt: result.Receipts[0]}
	api, _ := NewAPI(TestnetID, backend)
	value, e := api.GetTransactionReceipt(context.Background(), tx.Hash())
	if e != nil {
		t.Fatal(e)
	}
	obj := value.(map[string]interface{})
	if obj["from"] != from || obj["contractAddress"] != nil || obj["status"] != "0x1" || obj["effectiveGasPrice"] != "0x2" {
		t.Fatalf("receipt fields: %+v", obj)
	}
	backend.tx = nil
	if _, e = api.GetTransactionReceipt(context.Background(), tx.Hash()); e == nil {
		t.Fatal("receipt without transaction accepted")
	}
}

func TestCallAccessListAndGasPrice(t *testing.T) {
	s, from, b := fixture(t)
	api, _ := NewAPI(TestnetID, &testBackend{s: s, b: b})
	to := common.HexToAddress("0x9876")
	gas, err := api.EstimateGas(context.Background(), CallArgs{From: &from, To: &to, AccessList: types.AccessList{{Address: to, StorageKeys: []common.Hash{{}}}}})
	if err != nil || gas != 25300 {
		t.Fatalf("access list gas %d: %v", gas, err)
	}
	s.SetCode(to, []byte{0x3a, 0x60, 0, 0x52, 0x60, 0x20, 0x60, 0, 0xf3}, tracing.CodeChangeUnspecified)
	price := hexutil.Big(*big.NewInt(9))
	out, err := api.Call(context.Background(), CallArgs{From: &from, To: &to, GasPrice: &price}, rpc.LatestBlockNumber)
	if err != nil || new(big.Int).SetBytes(out).Uint64() != 9 {
		t.Fatalf("GASPRICE %x: %v", out, err)
	}
	if _, err = api.Call(context.Background(), CallArgs{GasPrice: &price, MaxFeePerGas: &price}, rpc.LatestBlockNumber); err == nil {
		t.Fatal("conflicting fee styles accepted")
	}
}
