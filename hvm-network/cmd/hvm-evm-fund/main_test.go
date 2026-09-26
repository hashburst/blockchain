package main

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"hashburst/wallet"
	"math/big"
	"testing"
)

func TestFundingSignedEnvelopeAndChainSeparation(t *testing.T) {
	w, e := wallet.NewWallet()
	if e != nil {
		t.Fatal(e)
	}
	to := common.HexToAddress("0x1234567890123456789012345678901234567890")
	tx := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(chainID), Nonce: 7, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2), Gas: 21000, To: &to, Value: big.NewInt(1000000000000000000)})
	signer := types.LatestSignerForChainID(big.NewInt(chainID))
	sig, e := w.Sign(signer.Hash(tx).Bytes())
	if e != nil {
		t.Fatal(e)
	}
	signed, e := tx.WithSignature(signer, sig)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := signed.MarshalBinary()
	if e != nil {
		t.Fatal(e)
	}
	restored := new(types.Transaction)
	if e = restored.UnmarshalBinary(raw); e != nil {
		t.Fatal(e)
	}
	sender, e := types.Sender(signer, restored)
	if e != nil || sender != common.HexToAddress(w.Address()) || restored.Hash() != signed.Hash() {
		t.Fatalf("saved envelope invalid: %v", e)
	}
	if _, e = types.Sender(types.LatestSignerForChainID(big.NewInt(4735489)), restored); e == nil {
		t.Fatal("mainnet accepted testnet signature")
	}
}
