package blockchain

import (
	"encoding/json"
	"testing"

	"hashburst/hvm"
	"hashburst/protocolv2"
	"hashburst/wallet"
)

type testV2Broadcaster struct{ count int }

func (b *testV2Broadcaster) GossipTxV2(*protocolv2.TransactionV2) { b.count++ }

func rpcStringParam(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

func TestRPCV2RawSubmissionPendingSequenceAndReceipt(t *testing.T) {
	bc, mp, sender := newPhase3BChain(t, 2)
	bc.SetPendingTransactions(nil, nil)
	if err := bc.AddBlock(sender.Address()); err != nil {
		t.Fatal(err)
	}
	recipient, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	cfg := bc.ProtocolV2Config()
	fee, err := cfg.FeePolicy.ComputeFee(hvm.ComputeBaseTransfer)
	if err != nil {
		t.Fatal(err)
	}
	tx := protocolv2.NewTransactionV2(cfg.ChainID, protocolv2.TxHBTTransfer, sender.Address(), recipient.Address(), 123456789, 0, 0, fee, []byte("rpc-v2-test"))
	if err := tx.Sign(sender); err != nil {
		t.Fatal(err)
	}
	raw, err := tx.EncodeRaw()
	if err != nil {
		t.Fatal(err)
	}

	h := NewRPCHandler(bc, mp, 1337)
	broadcast := &testV2Broadcaster{}
	h.SetV2Broadcaster(broadcast)
	result, rpcErr := h.dispatch(&rpcRequest{Method: "hb_sendRawTransactionV2", Params: []json.RawMessage{rpcStringParam(raw)}})
	if rpcErr != nil {
		t.Fatalf("raw submission failed: %+v", rpcErr)
	}
	if result != "0x"+tx.HashHex() {
		t.Fatalf("submission result=%v want 0x%s", result, tx.HashHex())
	}
	if broadcast.count != 1 || mp.SizeV2() != 1 {
		t.Fatalf("broadcast=%d mempool=%d", broadcast.count, mp.SizeV2())
	}

	latest, rpcErr := h.dispatch(&rpcRequest{Method: "hb_getTransactionCount", Params: []json.RawMessage{rpcStringParam(sender.Address()), rpcStringParam("latest")}})
	if rpcErr != nil || latest != "0x0" {
		t.Fatalf("latest sequence=%v err=%+v", latest, rpcErr)
	}
	pending, rpcErr := h.dispatch(&rpcRequest{Method: "hb_getTransactionCount", Params: []json.RawMessage{rpcStringParam(sender.Address()), rpcStringParam("pending")}})
	if rpcErr != nil || pending != "0x1" {
		t.Fatalf("pending sequence=%v err=%+v", pending, rpcErr)
	}

	mineSnapshots(t, bc, mp, sender.Address())
	if mp.SizeV2() != 0 {
		t.Fatalf("mined V2 transaction remained in mempool")
	}
	if bc.BalanceUnits(recipient.Address()) != tx.ValueUnits {
		t.Fatalf("recipient balance=%d want %d", bc.BalanceUnits(recipient.Address()), tx.ValueUnits)
	}
	latest, rpcErr = h.dispatch(&rpcRequest{Method: "eth_getTransactionCount", Params: []json.RawMessage{rpcStringParam(sender.Address()), rpcStringParam("latest")}})
	if rpcErr != nil || latest != "0x1" {
		t.Fatalf("confirmed sequence after mine=%v err=%+v", latest, rpcErr)
	}

	receiptResult, rpcErr := h.dispatch(&rpcRequest{Method: "hb_getTransactionReceipt", Params: []json.RawMessage{rpcStringParam("0x" + tx.HashHex())}})
	if rpcErr != nil {
		t.Fatalf("receipt RPC failed: %+v", rpcErr)
	}
	receipt, ok := receiptResult.(hvm.Receipt)
	if !ok || !receipt.Success || receipt.TxID != tx.HashHex() {
		t.Fatalf("unexpected receipt: %#v", receiptResult)
	}
}

func TestRPCV2SubmissionRejectedWhileActivationDisabled(t *testing.T) {
	bc, mp, sender := newPhase3BChain(t, DisabledActivationHeight)
	bc.state.balances[stateKey(sender.Address())] = AmountToUnits(100)
	to, _ := wallet.NewWallet()
	cfg := bc.ProtocolV2Config()
	fee, _ := cfg.FeePolicy.ComputeFee(hvm.ComputeBaseTransfer)
	tx := protocolv2.NewTransactionV2(cfg.ChainID, protocolv2.TxHBTTransfer, sender.Address(), to.Address(), 1, 0, 0, fee, nil)
	if err := tx.Sign(sender); err != nil {
		t.Fatal(err)
	}
	raw, _ := tx.EncodeRaw()
	h := NewRPCHandler(bc, mp, 1337)
	_, rpcErr := h.dispatch(&rpcRequest{Method: "hb_sendRawTransactionV2", Params: []json.RawMessage{rpcStringParam(raw)}})
	if rpcErr == nil {
		t.Fatal("V2 RPC submission unexpectedly succeeded while activation is disabled")
	}
}
