package execution

import (
	"context"
	"encoding/hex"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentReplayAndPinnedAnchor(t *testing.T) {
	_, sender, b := fixture(t)
	dir := filepath.Join(t.TempDir(), "evm")
	a := Anchor{ChainID: TestnetID, Hash: common.HexToHash("0x42"), Balances: map[common.Address]string{sender: "1000000000"}}
	s, e := OpenStore(dir, a)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if another, e := OpenStore(dir, a); e == nil {
		another.Close()
		t.Fatal("double open allowed")
	}
	st, _, _ := s.Snapshot()
	to := common.HexToAddress("0x1234")
	txs := [][]byte{sign(t, 0, &to, nil, 21000, TestnetID)}
	b.ParentHash = a.Hash
	result, e := ApplyBlock(context.Background(), st, TestnetID, b, txs)
	if e != nil {
		t.Fatal(e)
	}
	r := Record{Number: b.Number, Time: b.Time, Hash: b.Hash, ParentHash: b.ParentHash, GasLimit: b.GasLimit, BaseFee: b.BaseFee.String(), Transactions: txs, StateRoot: result.StateRoot, ReceiptsRoot: result.ReceiptsRoot, GasUsed: result.GasUsed}
	bad := r
	bad.StateRoot = common.Hash{}
	if s.Append(context.Background(), bad) == nil {
		t.Fatal("wrong root accepted")
	}
	if e = s.Append(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	if s.Append(context.Background(), r) == nil {
		t.Fatal("duplicate height accepted")
	}
	s.Close()
	recovered, e := OpenStore(dir, a)
	if e != nil {
		t.Fatal(e)
	}
	restored, height, hash := recovered.Snapshot()
	if height != 1 || hash != r.Hash || restored.GetNonce(sender) != 1 || restored.GetBalance(to).Uint64() != 7 {
		t.Fatal("replay mismatch")
	}
	recovered.Close()
	a.ChainID = MainnetID
	if reopened, e := OpenStore(dir, a); e == nil {
		reopened.Close()
		t.Fatal("changed anchor accepted")
	}
	a.ChainID = TestnetID
	if e = os.WriteFile(filepath.Join(dir, "00000000000000000001.json"), []byte(`{"truncated":`), 0600); e != nil {
		t.Fatal(e)
	}
	if reopened, e := OpenStore(dir, a); e == nil {
		reopened.Close()
		t.Fatal("corrupt record accepted")
	}
}
func TestExactWeiAccounting(t *testing.T) {
	for _, n := range []int64{0, 1, 100000000, 9223372036854775807} {
		wei, e := NativeToWei(n)
		if e != nil {
			t.Fatal(e)
		}
		wei.Add(wei, big.NewInt(123))
		q, r, e := SplitWei(wei)
		if e != nil || q != n || r != 123 {
			t.Fatal("conversion loses dust")
		}
	}
	if _, _, e := SplitWei(big.NewInt(-1)); e == nil {
		t.Fatal("negative accepted")
	}
	if _, _, e := SplitWei(new(big.Int).Lsh(big.NewInt(1), 256)); e == nil {
		t.Fatal("overflow accepted")
	}
}
func TestActivationRequiresAncestorHashes(t *testing.T) {
	_, sender, _ := fixture(t)
	a := Anchor{ChainID: TestnetID, Height: 7, Hash: common.HexToHash("0x42"), Balances: map[common.Address]string{sender: "1"}}
	if s, e := OpenStore(filepath.Join(t.TempDir(), "store"), a); e == nil {
		s.Close()
		t.Fatal("missing BLOCKHASH history accepted")
	}
}

func TestContractStateSurvivesReplay(t *testing.T) {
	_, sender, b := fixture(t)
	a := Anchor{ChainID: TestnetID, Hash: common.HexToHash("0x42"), Balances: map[common.Address]string{sender: "1000000000"}}
	dir := filepath.Join(t.TempDir(), "evm")
	store, e := OpenStore(dir, a)
	if e != nil {
		t.Fatal(e)
	}
	runtime, _ := hex.DecodeString("602a60005560006000a000")
	init := append([]byte{0x60, byte(len(runtime)), 0x60, 12, 0x60, 0, 0x39, 0x60, byte(len(runtime)), 0x60, 0, 0xf3}, runtime...)
	contract := crypto.CreateAddress(sender, 0)
	txs := [][]byte{sign(t, 0, nil, init, 200000, TestnetID), sign(t, 1, &contract, nil, 100000, TestnetID)}
	st, _, _ := store.Snapshot()
	b.ParentHash = a.Hash
	r, e := ApplyBlock(context.Background(), st, TestnetID, b, txs)
	if e != nil {
		t.Fatal(e)
	}
	record := Record{Number: b.Number, Time: b.Time, Hash: b.Hash, ParentHash: b.ParentHash, GasLimit: b.GasLimit, BaseFee: b.BaseFee.String(), Transactions: txs, StateRoot: r.StateRoot, ReceiptsRoot: r.ReceiptsRoot, GasUsed: r.GasUsed}
	if e = store.Append(context.Background(), record); e != nil {
		t.Fatal(e)
	}
	store.Close()
	reopened, e := OpenStore(dir, a)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	st, _, _ = reopened.Snapshot()
	if st.GetNonce(sender) != 2 || len(st.GetCode(contract)) != len(runtime) || st.GetState(contract, common.Hash{}) != common.HexToHash("0x2a") {
		t.Fatal("contract state lost on restart")
	}
}
