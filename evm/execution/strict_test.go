package execution

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"
	"os"
	"strings"
	"testing"
)

// Execute pinned Solidity artifacts through the same engine used by HVM blocks.
// This is an integration smoke gate, not a universal ERC conformance certificate.
func TestHBTStrictContractsOnHVM(t *testing.T) {
	raw, err := os.ReadFile("../strict/artifacts.json")
	if err != nil {
		t.Fatal(err)
	}
	var art struct {
		Contracts map[string]struct {
			ABI      json.RawMessage `json:"abi"`
			Bytecode string          `json:"bytecode"`
		}
	}
	if err = json.Unmarshal(raw, &art); err != nil {
		t.Fatal(err)
	}
	st, from, b := fixture(t)
	b.GasLimit = 2000000
	key, _ := crypto.HexToECDSA(privateTestKey)
	nonce := uint64(0)
	transact := func(to *common.Address, data []byte) *types.Receipt {
		t.Helper()
		tx := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(int64(TestnetID)), Nonce: nonce, To: to, Gas: 2000000, GasFeeCap: big.NewInt(3), GasTipCap: big.NewInt(1), Data: data})
		tx, e := types.SignTx(tx, types.NewCancunSigner(big.NewInt(int64(TestnetID))), key)
		if e != nil {
			t.Fatal(e)
		}
		wire, _ := tx.MarshalBinary()
		result, e := ApplyBlock(context.Background(), st, TestnetID, b, [][]byte{wire})
		if e != nil {
			t.Fatal(e)
		}
		r := result.Receipts[0]
		if r.Status != 1 {
			t.Fatalf("execution failed gas=%d", r.GasUsed)
		}
		st = result.State
		nonce++
		b.Number++
		return r
	}
	abis := map[string]abi.ABI{}
	addresses := map[string]common.Address{}
	for _, name := range []string{"HBT20Strict", "HBT721Strict", "HBT1155Strict", "HBT1271Strict", "HBT4626Strict"} {
		c := art.Contracts[name]
		a, e := abi.JSON(strings.NewReader(string(c.ABI)))
		if e != nil {
			t.Fatal(e)
		}
		abis[name] = a
		code, e := hex.DecodeString(c.Bytecode)
		if e != nil {
			t.Fatal(e)
		}
		if name == "HBT4626Strict" {
			args, e := a.Pack("", addresses["HBT20Strict"])
			if e != nil {
				t.Fatal(e)
			}
			code = append(code, args...)
		}
		r := transact(nil, code)
		addresses[name] = r.ContractAddress
		t.Logf("%s deployment_gas=%d current_200000_limit_supported=%t", name, r.GasUsed, r.GasUsed <= 200000)
	}
	read := func(name, method string, args ...any) []any {
		t.Helper()
		a := abis[name]
		data, e := a.Pack(method, args...)
		if e != nil {
			t.Fatal(e)
		}
		input := hexutil.Bytes(data)
		to := addresses[name]
		result, e := simulate(context.Background(), st.Copy(), b, TestnetID, CallArgs{From: &from, To: &to, Input: &input}, 1000000)
		if e != nil || result.Failed() {
			t.Fatalf("read %s: %v %+v", method, e, result)
		}
		values, e := a.Unpack(method, result.ReturnData)
		if e != nil {
			t.Fatal(e)
		}
		return values
	}
	send := func(name, method string, args ...any) *types.Receipt {
		t.Helper()
		data, e := abis[name].Pack(method, args...)
		if e != nil {
			t.Fatal(e)
		}
		to := addresses[name]
		return transact(&to, data)
	}
	recipient := common.HexToAddress("0x1234")
	r := send("HBT20Strict", "transfer", recipient, big.NewInt(10))
	if len(r.Logs) != 1 || r.Logs[0].Topics[0] != crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)")) {
		t.Fatal("ERC20 event changed")
	}
	if read("HBT20Strict", "balanceOf", recipient)[0].(*big.Int).Int64() != 10 {
		t.Fatal("ERC20 transfer")
	}
	for name, ids := range map[string][]string{"HBT721Strict": {"01ffc9a7", "80ac58cd", "5b5e139f", "49064906", "2a55205a"}, "HBT1155Strict": {"01ffc9a7", "d9b67a26", "0e89341c"}} {
		for _, h := range ids {
			bytes, _ := hex.DecodeString(h)
			var id [4]byte
			copy(id[:], bytes)
			if !read(name, "supportsInterface", id)[0].(bool) {
				t.Fatal("missing interface", name, h)
			}
		}
		if read(name, "supportsInterface", [4]byte{255, 255, 255, 255})[0].(bool) {
			t.Fatal("invalid interface accepted")
		}
	}
	send("HBT721Strict", "transferFrom", from, recipient, big.NewInt(1))
	if read("HBT721Strict", "ownerOf", big.NewInt(1))[0].(common.Address) != recipient {
		t.Fatal("NFT ownership")
	}
	send("HBT1155Strict", "safeTransferFrom", from, recipient, big.NewInt(1), big.NewInt(3), []byte{})
	if read("HBT1155Strict", "balanceOf", recipient, big.NewInt(1))[0].(*big.Int).Int64() != 3 {
		t.Fatal("multi token balance")
	}
	vault := addresses["HBT4626Strict"]
	send("HBT20Strict", "approve", vault, big.NewInt(100))
	send("HBT4626Strict", "deposit", big.NewInt(100), from)
	if read("HBT4626Strict", "totalAssets")[0].(*big.Int).Int64() != 100 {
		t.Fatal("vault accounting")
	}
	send("HBT4626Strict", "withdraw", big.NewInt(10), from, from)
	hash := crypto.Keccak256Hash([]byte("strict-test"))
	sig, _ := crypto.Sign(hash[:], key)
	sig[64] += 27
	if read("HBT1271Strict", "isValidSignature", hash, sig)[0].([4]byte) != ([4]byte{0x16, 0x26, 0xba, 0x7e}) {
		t.Fatal("1271 signature")
	}
	sig[0] ^= 1
	if read("HBT1271Strict", "isValidSignature", hash, sig)[0].([4]byte) != ([4]byte{255, 255, 255, 255}) {
		t.Fatal("1271 invalid signature")
	}
}
