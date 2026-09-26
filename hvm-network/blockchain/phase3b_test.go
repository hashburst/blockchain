package blockchain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"hashburst/hvm"
	"hashburst/protocolv2"
	"hashburst/wallet"
)

func p3bHex32(label string) string {
	h := sha256.Sum256([]byte(label))
	return "0x" + hex.EncodeToString(h[:])
}

func newPhase3BChain(t *testing.T, activation uint64) (*Blockchain, *Mempool, *wallet.Wallet) {
	t.Helper()
	admin, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultProtocolV2Config()
	cfg.ActivationHeight = activation
	cfg.LegacyPoWDifficulty = 1
	cfg.PoHTicksPerBlock = 4_000
	bc := NewBlockchainWithDirAndV2Config(t.TempDir(), cfg)
	mp := NewMempool()
	bc.SetMempool(mp)
	return bc, mp, admin
}

func signAndAdmitV2(t *testing.T, bc *Blockchain, w *wallet.Wallet, tx *protocolv2.TransactionV2) {
	t.Helper()
	if err := tx.Sign(w); err != nil {
		t.Fatal(err)
	}
	if err := bc.AdmitTransactionV2(tx); err != nil {
		t.Fatal(err)
	}
}

func mineSnapshots(t *testing.T, bc *Blockchain, mp *Mempool, miner string) {
	t.Helper()
	bc.SetPendingTransactions(mp.SnapshotTransactions(), mp.SnapshotTransactionsV2())
	if err := bc.AddBlock(miner); err != nil {
		t.Fatal(err)
	}
}

func TestPhase3BV2MiningPayoutRegistryEndToEnd(t *testing.T) {
	bc, mp, admin := newPhase3BChain(t, 2)

	// Height 1 remains byte-compatible V1 and gives the recorder native HBT.
	bc.SetPendingTransactions(nil, nil)
	if err := bc.AddBlock(admin.Address()); err != nil {
		t.Fatal(err)
	}
	if bc.Height() != 1 || bc.Blocks[1].EffectiveVersion() != BlockVersionLegacy {
		t.Fatalf("expected legacy block at height 1")
	}

	cfg := bc.ProtocolV2Config()
	deployLimit := uint64(500_000)
	deployMaxFee, _ := cfg.FeePolicy.MaxFeeForLimit(deployLimit)
	init, _ := json.Marshal(hvm.MiningPayoutRegistryInit{Admin: admin.Address(), LegacyImporters: []string{admin.Address()}})
	deployData, _ := json.Marshal(hvm.DeployRequest{ContractType: hvm.MiningPayoutRegistryType, Init: init})
	deploy := protocolv2.NewTransactionV2(cfg.ChainID, protocolv2.TxContractDeploy, admin.Address(), "", 0, 0, deployLimit, deployMaxFee, deployData)
	signAndAdmitV2(t, bc, admin, deploy)
	mineSnapshots(t, bc, mp, admin.Address())

	if bc.Blocks[2].EffectiveVersion() != BlockVersionV2 {
		t.Fatalf("height 2 is not V2")
	}
	if !isHex32(bc.Blocks[2].HBTStateRoot) || !isHex32(bc.Blocks[2].HVMStateRoot) || !isHex32(bc.Blocks[2].ReceiptsRoot) {
		t.Fatalf("missing V2 commitments")
	}
	deployReceipt, ok := bc.Receipt(deploy.HashHex())
	if !ok || !deployReceipt.Success || deployReceipt.Contract == "" {
		t.Fatalf("deploy receipt invalid: %+v", deployReceipt)
	}
	contract := deployReceipt.Contract

	payoutID := p3bHex32("doge-payout-68928894")
	auditReq := hvm.AuditBatchRequest{
		ReportID: p3bHex32("legacy-report"), BatchID: p3bHex32("audit-batch"), BatchIndex: 0, BatchCount: 1,
		Pool: "ViaBTC", Currency: "DOGE", CurrencyDecimals: 8, SourceTimestamp: 1749186443,
		EvidenceHash: p3bHex32("auditing-json"), LegacySource: "polygon-public-record-payout", LegacyFilename: "68928894_1749186443.json",
		Items: []hvm.PayoutItem{{
			PayoutID: payoutID, UserRef: p3bHex32("user-ref"), ExternalAddressRaw: "D72BB3tUN1P8xEKKS7FxJ9odmJPRPM9umC",
			ExternalAddressScheme: "DOGE_BASE58", LegacyProxy: "0xf39868f6289852d19ab64f65e00dbb10274656c2", LegacyProxyScheme: "SHA1_TRUNC20",
			AuditedAmountAtomic: "1500000000", ContributionNumerator: 1000, ContributionDenominator: 1000000,
		}},
	}
	auditArgs, _ := json.Marshal(auditReq)
	auditPayload, _ := json.Marshal(contractCallPayload{Method: "recordAuditBatch", Args: auditArgs})
	callLimit := uint64(700_000)
	callMaxFee, _ := cfg.FeePolicy.MaxFeeForLimit(callLimit)
	auditTx := protocolv2.NewTransactionV2(cfg.ChainID, protocolv2.TxContractCall, admin.Address(), contract, 0, 1, callLimit, callMaxFee, auditPayload)
	signAndAdmitV2(t, bc, admin, auditTx)
	mineSnapshots(t, bc, mp, admin.Address())
	if r, ok := bc.Receipt(auditTx.HashHex()); !ok || !r.Success {
		t.Fatalf("audit receipt failed: %+v", r)
	}

	transfer := hvm.ExternalTransfer{
		TransferID: hvm.ExternalTransferID("DOGE", "mainnet", "e2095e7f78df436114dfb5455a33a5f438a280cc21da88fc11ba69c8d23b1358"), ExternalChain: "DOGE", Network: "mainnet",
		TxID:              "e2095e7f78df436114dfb5455a33a5f438a280cc21da88fc11ba69c8d23b1358",
		ObservedTimestamp: 1749186443, EvidenceHash: p3bHex32("viabtc-api-response"),
	}
	sentReq := hvm.TransitionBatchRequest{
		Status: hvm.StatusSent, EvidenceHash: transfer.EvidenceHash, ObservedTimestamp: 1749186443,
		ExternalTransfer: &transfer, Items: []hvm.TransitionItem{{PayoutID: payoutID, ObservedAmountAtomic: "1500000000"}},
	}
	sentArgs, _ := json.Marshal(sentReq)
	sentPayload, _ := json.Marshal(contractCallPayload{Method: "recordTransitionBatch", Args: sentArgs})
	sentTx := protocolv2.NewTransactionV2(cfg.ChainID, protocolv2.TxContractCall, admin.Address(), contract, 0, 2, callLimit, callMaxFee, sentPayload)
	signAndAdmitV2(t, bc, admin, sentTx)
	mineSnapshots(t, bc, mp, admin.Address())
	if r, ok := bc.Receipt(sentTx.HashHex()); !ok || !r.Success {
		t.Fatalf("sent receipt failed: %+v", r)
	}

	query, _ := json.Marshal(map[string]string{"payout_id": payoutID})
	readReceipt, err := bc.SimulateHVMCall(admin.Address(), contract, "getPayout", query, 100_000)
	if err != nil || !readReceipt.Success {
		t.Fatalf("getPayout: %v %+v", err, readReceipt)
	}
	var current hvm.PayoutCurrent
	if err := json.Unmarshal(readReceipt.ReturnData, &current); err != nil {
		t.Fatal(err)
	}
	if current.Status != hvm.StatusSent || current.AuditedAmountAtomic != "1500000000" || current.ObservedAmountAtomic != "1500000000" {
		t.Fatalf("unexpected payout state: %+v", current)
	}
	if current.ContributionNumerator != 1000 || current.ContributionDenominator != 1000000 {
		t.Fatalf("unexpected contribution ratio")
	}
	if bc.Sequence(admin.Address()) != 3 {
		t.Fatalf("sequence=%d want 3", bc.Sequence(admin.Address()))
	}
}

func TestPhase3BRevertConsumesSequenceAndFeeNotValue(t *testing.T) {
	bc, mp, admin := newPhase3BChain(t, 2)
	bc.SetPendingTransactions(nil, nil)
	if err := bc.AddBlock(admin.Address()); err != nil {
		t.Fatal(err)
	}
	cfg := bc.ProtocolV2Config()
	limit := uint64(500_000)
	maxFee, _ := cfg.FeePolicy.MaxFeeForLimit(limit)
	init, _ := json.Marshal(hvm.MiningPayoutRegistryInit{Admin: admin.Address()})
	deployData, _ := json.Marshal(hvm.DeployRequest{ContractType: hvm.MiningPayoutRegistryType, Init: init})
	deploy := protocolv2.NewTransactionV2(cfg.ChainID, protocolv2.TxContractDeploy, admin.Address(), "", 0, 0, limit, maxFee, deployData)
	signAndAdmitV2(t, bc, admin, deploy)
	mineSnapshots(t, bc, mp, admin.Address())
	dr, _ := bc.Receipt(deploy.HashHex())

	beforeSender := bc.BalanceUnits(admin.Address())
	beforeContract := bc.BalanceUnits(dr.Contract)
	badArgs, _ := json.Marshal(contractCallPayload{Method: "methodThatDoesNotExist", Args: json.RawMessage(`{}`)})
	bad := protocolv2.NewTransactionV2(cfg.ChainID, protocolv2.TxContractCall, admin.Address(), dr.Contract, 12345, 1, limit, maxFee, badArgs)
	signAndAdmitV2(t, bc, admin, bad)
	mineSnapshots(t, bc, mp, admin.Address())
	r, ok := bc.Receipt(bad.HashHex())
	if !ok || r.Success || r.FeeUnits <= 0 {
		t.Fatalf("expected reverted receipt with fee: %+v", r)
	}
	if bc.Sequence(admin.Address()) != 2 {
		t.Fatalf("reverted tx did not consume sequence")
	}
	if bc.BalanceUnits(dr.Contract) != beforeContract {
		t.Fatalf("reverted tx transferred value")
	}
	// Each mined block also credits a 50 HBT reward to admin. Net sender change
	// is therefore +reward-fee, with no 12,345-unit value debit.
	expected := beforeSender + AmountToUnits(bc.MiningReward) - r.FeeUnits
	if bc.BalanceUnits(admin.Address()) != expected {
		t.Fatalf("sender balance=%d want %d", bc.BalanceUnits(admin.Address()), expected)
	}
}

func TestMempoolSnapshotDoesNotClear(t *testing.T) {
	mp := NewMempool()
	w, _ := wallet.NewWallet()
	to, _ := wallet.NewWallet()
	tx := NewTransaction(w.Address(), to.Address(), 0)
	if err := tx.Sign(w); err != nil {
		t.Fatal(err)
	}
	mp.AddTransaction(tx)
	if got := len(mp.GetTransactions()); got != 1 {
		t.Fatalf("first snapshot=%d", got)
	}
	if got := len(mp.GetTransactions()); got != 1 {
		t.Fatalf("mempool was cleared before block commit")
	}
	mp.RemoveTransaction(tx.ID)
	if got := len(mp.GetTransactions()); got != 0 {
		t.Fatalf("remove did not commit")
	}
}

func TestSettleV2HandlesOverlappingAccountsAtomically(t *testing.T) {
	sender := "0x1111111111111111111111111111111111111111"
	collector := "0x2222222222222222222222222222222222222222"

	st := NewState()
	st.balances[stateKey(sender)] = 1000
	tx := &protocolv2.TransactionV2{Sender: sender, To: collector, ValueUnits: 100, Sequence: 0, MaxFeeUnits: 50}
	if err := st.SettleV2(tx, 10, collector, collector, true); err != nil {
		t.Fatalf("overlapping collector/recipient settlement: %v", err)
	}
	if got := st.BalanceUnits(sender); got != 890 {
		t.Fatalf("sender balance=%d want 890", got)
	}
	if got := st.BalanceUnits(collector); got != 110 {
		t.Fatalf("collector balance=%d want 110", got)
	}

	// A self-transfer must only charge the fee: value leaves and returns to the
	// same account within one atomic delta set.
	st = NewState()
	st.balances[stateKey(sender)] = 1000
	tx = &protocolv2.TransactionV2{Sender: sender, To: sender, ValueUnits: 100, Sequence: 0, MaxFeeUnits: 50}
	if err := st.SettleV2(tx, 10, collector, sender, true); err != nil {
		t.Fatalf("self-transfer settlement: %v", err)
	}
	if got := st.BalanceUnits(sender); got != 990 {
		t.Fatalf("self-transfer sender balance=%d want 990", got)
	}
	if got := st.BalanceUnits(collector); got != 10 {
		t.Fatalf("self-transfer collector balance=%d want 10", got)
	}
}

func TestPhase3BRejectsReservedUnimplementedTxTypeAtAdmission(t *testing.T) {
	bc, _, admin := newPhase3BChain(t, 1)
	bc.state.balances[stateKey(admin.Address())] = AmountToUnits(100)
	cfg := bc.ProtocolV2Config()
	limit := uint64(100000)
	maxFee, err := cfg.FeePolicy.MaxFeeForLimit(limit)
	if err != nil {
		t.Fatal(err)
	}
	tx := protocolv2.NewTransactionV2(cfg.ChainID, protocolv2.TxValidatorRegister, admin.Address(), "", 0, 0, limit, maxFee, []byte(`{}`))
	if err := tx.Sign(admin); err != nil {
		t.Fatal(err)
	}
	if err := bc.AdmitTransactionV2(tx); err == nil {
		t.Fatal("reserved validator transaction unexpectedly entered Phase 3B mempool")
	}
}

func TestV2BlockCommitsNumericProtocolChainID(t *testing.T) {
	bc, _, admin := newPhase3BChain(t, 1)
	bc.state.balances[stateKey(admin.Address())] = AmountToUnits(100)
	prev := bc.Blocks[len(bc.Blocks)-1]
	b := NewBlockV2([]*Transaction{NewSystemReward(admin.Address(), bc.MiningReward)}, nil, prev.Hash, 1, poHWithTicks(prev.ProofOfTime, bc.v2Config.EffectivePoHTicks()), bc.v2Config.ChainID)
	prepared, err := bc.prepareV2Commitments(b)
	if err != nil {
		t.Fatal(err)
	}
	_ = prepared
	if err := b.MineBlock(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBlockAgainstConfig(prev, b, bc.MiningReward, bc.v2Config); err != nil {
		t.Fatalf("correct V2 chain id rejected: %v", err)
	}
	b.ProtocolChainID++
	if err := ValidateBlockAgainstConfig(prev, b, bc.MiningReward, bc.v2Config); err == nil {
		t.Fatal("mutated protocol chain id unexpectedly validated")
	}
}
