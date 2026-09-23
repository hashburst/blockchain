package hvm

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"hashburst/protocolv2"
	"hashburst/wallet"
)

func testHex32(label string) string {
	return hashID("TEST", label)
}

func deployTestRegistry(t *testing.T) (*Engine, *wallet.Wallet, string) {
	t.Helper()
	admin, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(NewStateDB(), protocolv2.FeePolicy{BaseTxUnits: 1, FeeRateUnitsPerMillion: 2})
	init, _ := json.Marshal(MiningPayoutRegistryInit{Admin: admin.Address(), LegacyImporters: []string{admin.Address()}})
	ctx := ExecutionContext{
		TxID: testHex32("deploy"), ChainID: 1337, Sender: admin.Address(),
		BlockHeight: 9, BlockTime: 1787000000, ComputeLimit: 500000,
	}
	receipt := engine.Deploy(ctx, DeployRequest{ContractType: MiningPayoutRegistryType, Init: init})
	if !receipt.Success {
		t.Fatalf("deploy failed: %s", receipt.RevertReason)
	}
	return engine, admin, receipt.Contract
}

func TestMiningPayoutRegistryAuditingToSent(t *testing.T) {
	engine, admin, contract := deployTestRegistry(t)
	root0 := engine.State().Root()

	payoutID := testHex32("doge-payout-68928894")
	reportID := testHex32("legacy-report")
	audit := AuditBatchRequest{
		ReportID:         reportID,
		BatchID:          testHex32("audit-batch-0"),
		BatchIndex:       0,
		BatchCount:       1,
		Pool:             "ViaBTC",
		Currency:         "DOGE",
		CurrencyDecimals: 8,
		SourceTimestamp:  1749186443,
		EvidenceHash:     testHex32("auditing-json"),
		LegacySource:     "polygon-public-record-payout",
		LegacyFilename:   "68928894_1749186443.json",
		Items: []PayoutItem{{
			PayoutID:                payoutID,
			UserRef:                 testHex32("user-ref"),
			ExternalAddressRaw:      "D72BB3tUN1P8xEKKS7FxJ9odmJPRPM9umC",
			ExternalAddressScheme:   "DOGE_BASE58",
			LegacyProxy:             "0xf39868f6289852d19ab64f65e00dbb10274656c2",
			LegacyProxyScheme:       "SHA1_TRUNC20",
			AuditedAmountAtomic:     "1500000000",
			ContributionNumerator:   1000,
			ContributionDenominator: 1000000,
		}},
	}
	args, _ := json.Marshal(audit)
	r1 := engine.Call(ExecutionContext{
		TxID: testHex32("audit-tx"), ChainID: 1337, Sender: admin.Address(),
		BlockHeight: 10, BlockTime: 1749186443, ComputeLimit: 700000,
	}, CallRequest{Address: contract, Method: "recordAuditBatch", Args: args})
	if !r1.Success {
		t.Fatalf("audit failed: %s", r1.RevertReason)
	}
	if engine.State().Root() == root0 {
		t.Fatal("state root did not change after audit")
	}

	transfer := ExternalTransfer{
		TransferID: ExternalTransferID("DOGE", "mainnet", "e2095e7f78df436114dfb5455a33a5f438a280cc21da88fc11ba69c8d23b1358"), ExternalChain: "DOGE", Network: "mainnet",
		TxID:              "e2095e7f78df436114dfb5455a33a5f438a280cc21da88fc11ba69c8d23b1358",
		ObservedTimestamp: 1749186443, EvidenceHash: testHex32("viabtc-api-response"),
	}
	transition := TransitionBatchRequest{
		Status: StatusSent, EvidenceHash: transfer.EvidenceHash, ObservedTimestamp: 1749186443,
		ExternalTransfer: &transfer,
		Items:            []TransitionItem{{PayoutID: payoutID, ObservedAmountAtomic: "1500000000"}},
	}
	transitionArgs, _ := json.Marshal(transition)
	r2 := engine.Call(ExecutionContext{
		TxID: testHex32("sent-tx"), ChainID: 1337, Sender: admin.Address(),
		BlockHeight: 11, BlockTime: 1749186450, ComputeLimit: 700000,
	}, CallRequest{Address: contract, Method: "recordTransitionBatch", Args: transitionArgs})
	if !r2.Success {
		t.Fatalf("sent transition failed: %s", r2.RevertReason)
	}

	q, _ := json.Marshal(map[string]string{"payout_id": payoutID})
	get := engine.Call(ExecutionContext{
		TxID: testHex32("query"), ChainID: 1337, Sender: admin.Address(),
		BlockHeight: 11, BlockTime: 1749186451, ComputeLimit: 100000,
	}, CallRequest{Address: contract, Method: "getPayout", Args: q})
	if !get.Success {
		t.Fatalf("get payout failed: %s", get.RevertReason)
	}
	var current PayoutCurrent
	if err := json.Unmarshal(get.ReturnData, &current); err != nil {
		t.Fatal(err)
	}
	if current.Status != StatusSent {
		t.Fatalf("status=%s want SENT", current.Status)
	}
	if current.ExternalTransferID != transfer.TransferID {
		t.Fatalf("transfer id mismatch")
	}
	if current.AuditedAmountAtomic != "1500000000" || current.ObservedAmountAtomic != "1500000000" {
		t.Fatalf("amount mismatch: audited=%s observed=%s", current.AuditedAmountAtomic, current.ObservedAmountAtomic)
	}
	if current.ContributionNumerator != 1000 || current.ContributionDenominator != 1000000 {
		t.Fatalf("contribution ratio mismatch")
	}

	// SENT is terminal: a later CANCELLED transition must fail atomically.
	cancelArgs, _ := json.Marshal(TransitionBatchRequest{
		Status: StatusCancelled, EvidenceHash: testHex32("cancel"), ObservedTimestamp: 1749186500,
		ReasonCode: "TEST", Items: []TransitionItem{{PayoutID: payoutID}},
	})
	r3 := engine.Call(ExecutionContext{
		TxID: testHex32("cancel-tx"), ChainID: 1337, Sender: admin.Address(),
		BlockHeight: 12, BlockTime: 1749186500, ComputeLimit: 200000,
	}, CallRequest{Address: contract, Method: "recordTransitionBatch", Args: cancelArgs})
	if r3.Success {
		t.Fatal("terminal SENT payout unexpectedly transitioned to CANCELLED")
	}
}

func TestMiningPayoutRegistryRejectsDuplicateBatch(t *testing.T) {
	engine, admin, contract := deployTestRegistry(t)
	audit := AuditBatchRequest{
		ReportID: testHex32("r"), BatchID: testHex32("b"), BatchIndex: 0, BatchCount: 1,
		Pool: "ViaBTC", Currency: "DOGE", CurrencyDecimals: 8, SourceTimestamp: 1,
		EvidenceHash: testHex32("e"),
		Items: []PayoutItem{{PayoutID: testHex32("p"), UserRef: testHex32("u"),
			ExternalAddressRaw: "Dabc", ExternalAddressScheme: "DOGE_BASE58",
			AuditedAmountAtomic: "1", ContributionNumerator: 1, ContributionDenominator: 1000}},
	}
	args, _ := json.Marshal(audit)
	ctx := ExecutionContext{TxID: testHex32("a1"), ChainID: 1337, Sender: admin.Address(), BlockHeight: 1, BlockTime: 1, ComputeLimit: 500000}
	first := engine.Call(ctx, CallRequest{Address: contract, Method: "recordAuditBatch", Args: args})
	if !first.Success {
		t.Fatalf("first batch failed: %s", first.RevertReason)
	}
	ctx.TxID = testHex32("a2")
	second := engine.Call(ctx, CallRequest{Address: contract, Method: "recordAuditBatch", Args: args})
	if second.Success {
		t.Fatal("duplicate batch unexpectedly succeeded")
	}
}

func TestReceiptsRootDeterministic(t *testing.T) {
	rs := []Receipt{{TxID: "a", Success: true, ComputeUsed: 10, FeeUnits: 1, Events: []Event{{Contract: "c", Name: "E", Topics: []string{"x"}, Data: []byte("d")}}}}
	a := ReceiptsRoot(rs)
	b := ReceiptsRoot(rs)
	if a != b {
		t.Fatalf("receipts root not deterministic")
	}
}

func TestMiningPayoutRegistryReportCompletenessAndDuplicateIndex(t *testing.T) {
	engine, admin, contract := deployTestRegistry(t)
	reportID := testHex32("report-two-batches")
	mkBatch := func(batchID string, index uint32, payoutLabel string) AuditBatchRequest {
		return AuditBatchRequest{
			ReportID: reportID, BatchID: testHex32(batchID), BatchIndex: index, BatchCount: 2,
			Pool: "ViaBTC", Currency: "DOGE", CurrencyDecimals: 8, SourceTimestamp: 1749186443 + uint64(index),
			EvidenceHash: testHex32("evidence-" + batchID),
			Items: []PayoutItem{{
				PayoutID: testHex32(payoutLabel), UserRef: testHex32("user-" + payoutLabel),
				ExternalAddressRaw: "D" + payoutLabel, ExternalAddressScheme: "DOGE_BASE58",
				AuditedAmountAtomic: "100000000", ContributionNumerator: 1000, ContributionDenominator: 1000000,
			}},
		}
	}
	call := func(txLabel string, req AuditBatchRequest) Receipt {
		args, _ := json.Marshal(req)
		return engine.Call(ExecutionContext{
			TxID: testHex32(txLabel), ChainID: 1337, Sender: admin.Address(), BlockHeight: 20, BlockTime: req.SourceTimestamp, ComputeLimit: 700000,
		}, CallRequest{Address: contract, Method: "recordAuditBatch", Args: args})
	}

	first := call("batch0-tx", mkBatch("batch0", 0, "p0"))
	if !first.Success {
		t.Fatalf("first report batch failed: %s", first.RevertReason)
	}
	q, _ := json.Marshal(map[string]string{"report_id": reportID})
	get := engine.Call(ExecutionContext{TxID: testHex32("get-report-1"), ChainID: 1337, Sender: admin.Address(), BlockHeight: 20, BlockTime: 1749186443, ComputeLimit: 100000}, CallRequest{Address: contract, Method: "getReport", Args: q})
	if !get.Success {
		t.Fatalf("getReport failed: %s", get.RevertReason)
	}
	var report ReportCurrent
	if err := json.Unmarshal(get.ReturnData, &report); err != nil {
		t.Fatal(err)
	}
	if report.RecordedBatches != 1 || report.BatchCount != 2 || report.Complete || report.ItemCount != 1 {
		t.Fatalf("unexpected partial report: %+v", report)
	}

	// A different batch_id cannot occupy an already-recorded index.
	dupIndex := call("dup-index-tx", mkBatch("different-batch-same-index", 0, "p-dup"))
	if dupIndex.Success {
		t.Fatal("duplicate report batch index unexpectedly succeeded")
	}

	second := call("batch1-tx", mkBatch("batch1", 1, "p1"))
	if !second.Success {
		t.Fatalf("second report batch failed: %s", second.RevertReason)
	}
	get = engine.Call(ExecutionContext{TxID: testHex32("get-report-2"), ChainID: 1337, Sender: admin.Address(), BlockHeight: 21, BlockTime: 1749186444, ComputeLimit: 100000}, CallRequest{Address: contract, Method: "getReport", Args: q})
	if !get.Success {
		t.Fatalf("getReport after completion failed: %s", get.RevertReason)
	}
	if err := json.Unmarshal(get.ReturnData, &report); err != nil {
		t.Fatal(err)
	}
	if report.RecordedBatches != 2 || !report.Complete || report.ItemCount != 2 {
		t.Fatalf("report did not become complete: %+v", report)
	}
}

func TestMiningPayoutRegistryExternalTransferMaySettleMultipleBatches(t *testing.T) {
	engine, admin, contract := deployTestRegistry(t)
	reportID := testHex32("shared-transfer-report")
	payouts := []string{testHex32("shared-payout-a"), testHex32("shared-payout-b")}
	for i, pid := range payouts {
		audit := AuditBatchRequest{
			ReportID: reportID, BatchID: testHex32("shared-audit-" + strconv.Itoa(i)), BatchIndex: uint32(i), BatchCount: 2,
			Pool: "ViaBTC", Currency: "DOGE", CurrencyDecimals: 8, SourceTimestamp: 1749186443,
			EvidenceHash: testHex32("shared-audit-evidence"),
			Items: []PayoutItem{{PayoutID: pid, UserRef: testHex32("shared-user-" + strconv.Itoa(i)),
				ExternalAddressRaw: "DShared" + strconv.Itoa(i), ExternalAddressScheme: "DOGE_BASE58",
				AuditedAmountAtomic: "750000000", ContributionNumerator: 1000, ContributionDenominator: 1000000}},
		}
		args, _ := json.Marshal(audit)
		r := engine.Call(ExecutionContext{TxID: testHex32("shared-audit-tx-" + strconv.Itoa(i)), ChainID: 1337, Sender: admin.Address(), BlockHeight: 30 + uint64(i), BlockTime: 1749186443, ComputeLimit: 700000}, CallRequest{Address: contract, Method: "recordAuditBatch", Args: args})
		if !r.Success {
			t.Fatalf("audit %d failed: %s", i, r.RevertReason)
		}
	}

	txid := "e2095e7f78df436114dfb5455a33a5f438a280cc21da88fc11ba69c8d23b1358"
	transfer := ExternalTransfer{
		TransferID: ExternalTransferID("DOGE", "mainnet", txid), ExternalChain: "DOGE", Network: "mainnet", TxID: txid,
		ObservedTimestamp: 1749186443, EvidenceHash: testHex32("shared-viabtc-evidence"),
	}
	for i, pid := range payouts {
		req := TransitionBatchRequest{
			Status: StatusSent, EvidenceHash: transfer.EvidenceHash, ObservedTimestamp: transfer.ObservedTimestamp,
			ExternalTransfer: &transfer, Items: []TransitionItem{{PayoutID: pid, ObservedAmountAtomic: "750000000"}},
		}
		args, _ := json.Marshal(req)
		r := engine.Call(ExecutionContext{TxID: testHex32("shared-sent-tx-" + strconv.Itoa(i)), ChainID: 1337, Sender: admin.Address(), BlockHeight: 40 + uint64(i), BlockTime: 1749186450, ComputeLimit: 700000}, CallRequest{Address: contract, Method: "recordTransitionBatch", Args: args})
		if !r.Success {
			t.Fatalf("shared transfer transition %d failed: %s", i, r.RevertReason)
		}
	}

	q, _ := json.Marshal(map[string]string{"transfer_id": transfer.TransferID})
	get := engine.Call(ExecutionContext{TxID: testHex32("get-shared-transfer"), ChainID: 1337, Sender: admin.Address(), BlockHeight: 42, BlockTime: 1749186451, ComputeLimit: 100000}, CallRequest{Address: contract, Method: "getExternalTransfer", Args: q})
	if !get.Success {
		t.Fatalf("shared transfer lookup failed: %s", get.RevertReason)
	}
	var stored ExternalTransfer
	if err := json.Unmarshal(get.ReturnData, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.TransferID != transfer.TransferID || stored.TxID != txid {
		t.Fatalf("unexpected shared transfer state: %+v", stored)
	}
}

func TestExternalTransferIDVectorMatchesLegacyNormalizer(t *testing.T) {
	const txid = "e2095e7f78df436114dfb5455a33a5f438a280cc21da88fc11ba69c8d23b1358"
	const want = "0x437751cf78233ee0d84c233e2fc847ef9d1a261cca3f1b5d1a4e3b7c1fdcc27b"
	if got := ExternalTransferID("DOGE", "mainnet", txid); got != want {
		t.Fatalf("external transfer id=%s want %s", got, want)
	}
}

func TestMiningPayoutRegistryAdminRotationRevokesOldRecorder(t *testing.T) {
	engine, admin, contract := deployTestRegistry(t)
	newAdmin, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	// Self-transfer must not accidentally delete the only admin role.
	selfArgs, _ := json.Marshal(map[string]string{"address": admin.Address()})
	self := engine.Call(ExecutionContext{TxID: testHex32("admin-self"), ChainID: 1337, Sender: admin.Address(), BlockHeight: 50, BlockTime: 1, ComputeLimit: 100000}, CallRequest{Address: contract, Method: "transferAdmin", Args: selfArgs})
	if self.Success {
		t.Fatal("admin self-transfer unexpectedly succeeded")
	}

	args, _ := json.Marshal(map[string]string{"address": newAdmin.Address()})
	rotated := engine.Call(ExecutionContext{TxID: testHex32("admin-rotate"), ChainID: 1337, Sender: admin.Address(), BlockHeight: 51, BlockTime: 2, ComputeLimit: 100000}, CallRequest{Address: contract, Method: "transferAdmin", Args: args})
	if !rotated.Success {
		t.Fatalf("admin rotation failed: %s", rotated.RevertReason)
	}

	// Old key must no longer be able to record payouts after rotation. Use the
	// native privacy path so legacy-import authority remains a separate role.
	userRef := testHex32("role-user")
	walletRef := testHex32("role-wallet")
	addrCommit := testHex32("role-address")
	profileArgs, _ := json.Marshal(StakeholderProfileRequest{
		UserRef: userRef, APIKeyCommitment: testHex32("role-api"), ProfileVersion: 1, SourceRecordCommitment: testHex32("role-profile-evidence"), UpdatedAt: 3,
		Wallets: []StakeholderWalletCommitment{{WalletRef: walletRef, Coin: "DOGE", Network: "mainnet", AddressScheme: "DOGE_BASE58", AddressCommitment: addrCommit}},
	})
	profileCall := engine.Call(ExecutionContext{TxID: testHex32("new-profile-writer"), ChainID: 1337, Sender: newAdmin.Address(), BlockHeight: 52, BlockTime: 3, ComputeLimit: 500000}, CallRequest{Address: contract, Method: "registerStakeholderProfile", Args: profileArgs})
	if !profileCall.Success {
		t.Fatalf("new admin did not receive profile-writer authority: %s", profileCall.RevertReason)
	}
	audit := AuditBatchRequest{
		ReportID: testHex32("role-report"), BatchID: testHex32("role-batch"), BatchIndex: 0, BatchCount: 1,
		Pool: "ViaBTC", Currency: "DOGE", CurrencyDecimals: 8, SourceTimestamp: 3, EvidenceHash: testHex32("role-evidence"),
		Items: []PayoutItem{{PayoutID: testHex32("role-payout"), UserRef: userRef, StakeholderProfileVersion: 1, WalletRef: walletRef, AddressCommitment: addrCommit, ExternalAddressScheme: "DOGE_BASE58", AuditedAmountAtomic: "1", ContributionNumerator: 1, ContributionDenominator: 1}},
	}
	auditArgs, _ := json.Marshal(audit)
	oldCall := engine.Call(ExecutionContext{TxID: testHex32("old-recorder"), ChainID: 1337, Sender: admin.Address(), BlockHeight: 52, BlockTime: 3, ComputeLimit: 500000}, CallRequest{Address: contract, Method: "recordAuditBatch", Args: auditArgs})
	if oldCall.Success {
		t.Fatal("old admin retained recorder authority after rotation")
	}
	newCall := engine.Call(ExecutionContext{TxID: testHex32("new-recorder"), ChainID: 1337, Sender: newAdmin.Address(), BlockHeight: 52, BlockTime: 3, ComputeLimit: 500000}, CallRequest{Address: contract, Method: "recordAuditBatch", Args: auditArgs})
	if !newCall.Success {
		t.Fatalf("new admin did not receive recorder authority: %s", newCall.RevertReason)
	}
}

func TestMiningPayoutRegistryReportTimestampsAreMinMax(t *testing.T) {
	engine, admin, contract := deployTestRegistry(t)
	reportID := testHex32("report-timestamp-order")
	callBatch := func(label string, index uint32, ts uint64) {
		req := AuditBatchRequest{
			ReportID: reportID, BatchID: testHex32("ts-batch-" + label), BatchIndex: index, BatchCount: 2,
			Pool: "ViaBTC", Currency: "DOGE", CurrencyDecimals: 8, SourceTimestamp: ts,
			EvidenceHash: testHex32("ts-evidence-" + label),
			Items: []PayoutItem{{
				PayoutID: testHex32("ts-payout-" + label), UserRef: testHex32("ts-user-" + label),
				ExternalAddressRaw: "D" + label, ExternalAddressScheme: "DOGE_BASE58",
				AuditedAmountAtomic: "100000000", ContributionNumerator: 1, ContributionDenominator: 1000,
			}},
		}
		args, _ := json.Marshal(req)
		r := engine.Call(ExecutionContext{
			TxID: testHex32("ts-tx-" + label), ChainID: 1337, Sender: admin.Address(), BlockHeight: 70 + uint64(index), BlockTime: ts, ComputeLimit: 700000,
		}, CallRequest{Address: contract, Method: "recordAuditBatch", Args: args})
		if !r.Success {
			t.Fatalf("timestamp batch %s failed: %s", label, r.RevertReason)
		}
	}

	// Record the later source batch first to prove timestamps describe the
	// report data, not transaction insertion order.
	callBatch("late", 1, 200)
	callBatch("early", 0, 100)

	q, _ := json.Marshal(map[string]string{"report_id": reportID})
	get := engine.Call(ExecutionContext{TxID: testHex32("ts-get"), ChainID: 1337, Sender: admin.Address(), BlockHeight: 72, BlockTime: 201, ComputeLimit: 100000}, CallRequest{Address: contract, Method: "getReport", Args: q})
	if !get.Success {
		t.Fatalf("getReport failed: %s", get.RevertReason)
	}
	var report ReportCurrent
	if err := json.Unmarshal(get.ReturnData, &report); err != nil {
		t.Fatal(err)
	}
	if report.FirstTimestamp != 100 || report.LastTimestamp != 200 || !report.Complete {
		t.Fatalf("report timestamps/completeness mismatch: %+v", report)
	}
}

func TestStakeholderProfileCommitmentsHideAPIKeyAndWallet(t *testing.T) {
	engine, admin, contract := deployTestRegistry(t)
	const rawAPIKey = "TEST_APIKEY_DO_NOT_USE_0123456789abcdef"
	const rawWallet = "D72BB3tUN1P8xEKKS7FxJ9odmJPRPM9umC"
	userRef := testHex32("private-user-ref")
	walletRef := testHex32("private-wallet-ref")
	addrCommit := testHex32("private-address-commitment")
	profileReq := StakeholderProfileRequest{
		UserRef: userRef, APIKeyCommitment: testHex32("private-apikey-commitment"), ProfileVersion: 1,
		SourceRecordCommitment: testHex32("private-list-record"), UpdatedAt: 1787000000,
		Wallets: []StakeholderWalletCommitment{{WalletRef: walletRef, Coin: "DOGE", Network: "mainnet", AddressScheme: "DOGE_BASE58", AddressCommitment: addrCommit}},
	}
	args, _ := json.Marshal(profileReq)
	r := engine.Call(ExecutionContext{TxID: testHex32("profile-register"), ChainID: 1337, Sender: admin.Address(), BlockHeight: 8, BlockTime: 1787000000, ComputeLimit: 500000}, CallRequest{Address: contract, Method: "registerStakeholderProfile", Args: args})
	if !r.Success {
		t.Fatalf("profile registration failed: %s", r.RevertReason)
	}

	// The raw APIKEY and external wallet are deliberately not present in any
	// committed StateDB key/value nor in the emitted profile event.
	engine.State().mu.RLock()
	for k, v := range engine.State().kv {
		if strings.Contains(k, rawAPIKey) || strings.Contains(string(v), rawAPIKey) {
			engine.State().mu.RUnlock()
			t.Fatal("raw APIKEY leaked into HVM state")
		}
		if strings.Contains(k, rawWallet) || strings.Contains(string(v), rawWallet) {
			engine.State().mu.RUnlock()
			t.Fatal("raw external wallet leaked into HVM state")
		}
	}
	engine.State().mu.RUnlock()
	for _, ev := range r.Events {
		if strings.Contains(string(ev.Data), rawAPIKey) || strings.Contains(string(ev.Data), rawWallet) {
			t.Fatal("raw private identifier leaked into HVM event")
		}
	}

	audit := AuditBatchRequest{
		ReportID: testHex32("privacy-report"), BatchID: testHex32("privacy-batch"), BatchIndex: 0, BatchCount: 1,
		Pool: "ViaBTC", Currency: "DOGE", CurrencyDecimals: 8, SourceTimestamp: 1787000001, EvidenceHash: testHex32("privacy-audit-evidence"),
		Items: []PayoutItem{{PayoutID: testHex32("privacy-payout"), UserRef: userRef, StakeholderProfileVersion: 1,
			WalletRef: walletRef, AddressCommitment: addrCommit, ExternalAddressScheme: "DOGE_BASE58",
			AuditedAmountAtomic: "1500000000", ContributionNumerator: 1000, ContributionDenominator: 1000000}},
	}
	auditArgs, _ := json.Marshal(audit)
	auditReceipt := engine.Call(ExecutionContext{TxID: testHex32("privacy-audit-tx"), ChainID: 1337, Sender: admin.Address(), BlockHeight: 9, BlockTime: 1787000001, ComputeLimit: 800000}, CallRequest{Address: contract, Method: "recordAuditBatch", Args: auditArgs})
	if !auditReceipt.Success {
		t.Fatalf("private payout binding failed: %s", auditReceipt.RevertReason)
	}

	q, _ := json.Marshal(map[string]string{"payout_id": audit.Items[0].PayoutID})
	get := engine.Call(ExecutionContext{TxID: testHex32("privacy-get"), ChainID: 1337, Sender: admin.Address(), BlockHeight: 9, BlockTime: 1787000001, ComputeLimit: 100000}, CallRequest{Address: contract, Method: "getPayout", Args: q})
	if !get.Success {
		t.Fatalf("get private payout: %s", get.RevertReason)
	}
	var current PayoutCurrent
	if err := json.Unmarshal(get.ReturnData, &current); err != nil {
		t.Fatal(err)
	}
	if current.ExternalAddressRaw != "" || current.WalletRef != walletRef || current.AddressCommitment != addrCommit || current.StakeholderProfileVersion != 1 {
		t.Fatalf("private payout state mismatch: %+v", current)
	}
}

func TestStakeholderWalletCoinBindingAndProfileVersion(t *testing.T) {
	engine, admin, contract := deployTestRegistry(t)
	userRef := testHex32("versioned-user")
	walletV1 := StakeholderWalletCommitment{WalletRef: testHex32("wallet-v1"), Coin: "DOGE", Network: "mainnet", AddressScheme: "DOGE_BASE58", AddressCommitment: testHex32("addr-v1")}
	register := func(version uint64, ts uint64, wallets []StakeholderWalletCommitment) {
		req := StakeholderProfileRequest{UserRef: userRef, APIKeyCommitment: testHex32("api-version-" + strconv.FormatUint(version, 10)), ProfileVersion: version, SourceRecordCommitment: testHex32("evidence-version-" + strconv.FormatUint(version, 10)), UpdatedAt: ts, Wallets: wallets}
		args, _ := json.Marshal(req)
		r := engine.Call(ExecutionContext{TxID: testHex32("profile-version-" + strconv.FormatUint(version, 10)), ChainID: 1337, Sender: admin.Address(), BlockHeight: ts, BlockTime: ts, ComputeLimit: 500000}, CallRequest{Address: contract, Method: "registerStakeholderProfile", Args: args})
		if !r.Success {
			t.Fatalf("profile v%d failed: %s", version, r.RevertReason)
		}
	}
	register(1, 100, []StakeholderWalletCommitment{walletV1})
	walletV2 := StakeholderWalletCommitment{WalletRef: testHex32("wallet-v2"), Coin: "BTC", Network: "mainnet", AddressScheme: "BTC_BECH32", AddressCommitment: testHex32("addr-v2")}
	register(2, 200, []StakeholderWalletCommitment{walletV2})

	// Historical payout can still bind to immutable profile version 1 even when
	// the stakeholder's current profile is version 2.
	good := AuditBatchRequest{ReportID: testHex32("historic-report"), BatchID: testHex32("historic-batch"), BatchIndex: 0, BatchCount: 1,
		Pool: "ViaBTC", Currency: "DOGE", CurrencyDecimals: 8, SourceTimestamp: 150, EvidenceHash: testHex32("historic-evidence"),
		Items: []PayoutItem{{PayoutID: testHex32("historic-payout"), UserRef: userRef, StakeholderProfileVersion: 1, WalletRef: walletV1.WalletRef, AddressCommitment: walletV1.AddressCommitment, ExternalAddressScheme: walletV1.AddressScheme, AuditedAmountAtomic: "1", ContributionNumerator: 1, ContributionDenominator: 1}}}
	args, _ := json.Marshal(good)
	r := engine.Call(ExecutionContext{TxID: testHex32("historic-tx"), ChainID: 1337, Sender: admin.Address(), BlockHeight: 201, BlockTime: 201, ComputeLimit: 800000}, CallRequest{Address: contract, Method: "recordAuditBatch", Args: args})
	if !r.Success {
		t.Fatalf("historical profile binding failed: %s", r.RevertReason)
	}

	bad := good
	bad.ReportID = testHex32("bad-coin-report")
	bad.BatchID = testHex32("bad-coin-batch")
	bad.Currency = "BTC"
	bad.Items = append([]PayoutItem(nil), good.Items...)
	bad.Items[0].PayoutID = testHex32("bad-coin-payout")
	badArgs, _ := json.Marshal(bad)
	badReceipt := engine.Call(ExecutionContext{TxID: testHex32("bad-coin-tx"), ChainID: 1337, Sender: admin.Address(), BlockHeight: 202, BlockTime: 202, ComputeLimit: 800000}, CallRequest{Address: contract, Method: "recordAuditBatch", Args: badArgs})
	if badReceipt.Success {
		t.Fatal("DOGE wallet commitment unexpectedly accepted for BTC payout")
	}
}

func TestStakeholderProfileMayHaveNoWallets(t *testing.T) {
	engine, admin, contract := deployTestRegistry(t)
	req := StakeholderProfileRequest{UserRef: testHex32("empty-wallet-user"), APIKeyCommitment: testHex32("empty-wallet-api"), ProfileVersion: 1, SourceRecordCommitment: testHex32("empty-wallet-evidence"), UpdatedAt: 1, Wallets: nil}
	args, _ := json.Marshal(req)
	r := engine.Call(ExecutionContext{TxID: testHex32("empty-wallet-profile"), ChainID: 1337, Sender: admin.Address(), BlockHeight: 1, BlockTime: 1, ComputeLimit: 300000}, CallRequest{Address: contract, Method: "registerStakeholderProfile", Args: args})
	if !r.Success {
		t.Fatalf("zero-wallet stakeholder profile rejected: %s", r.RevertReason)
	}
	q, _ := json.Marshal(map[string]interface{}{"user_ref": req.UserRef, "version": 1})
	get := engine.Call(ExecutionContext{TxID: testHex32("empty-wallet-get"), ChainID: 1337, Sender: admin.Address(), BlockHeight: 1, BlockTime: 1, ComputeLimit: 100000}, CallRequest{Address: contract, Method: "getStakeholderProfile", Args: q})
	if !get.Success {
		t.Fatal(get.RevertReason)
	}
	var profile StakeholderProfile
	if err := json.Unmarshal(get.ReturnData, &profile); err != nil {
		t.Fatal(err)
	}
	if profile.WalletCount != 0 {
		t.Fatalf("wallet_count=%d want 0", profile.WalletCount)
	}
	want, _ := StakeholderWalletSetRoot(nil)
	if profile.WalletSetRoot != want {
		t.Fatalf("empty wallet root=%s want %s", profile.WalletSetRoot, want)
	}
}

func TestStakeholderWalletSetRootMatchesPythonCommitmentTool(t *testing.T) {
	wallets := []StakeholderWalletCommitment{
		{WalletRef: "0x" + strings.Repeat("11", 32), Coin: "DOGE", Network: "mainnet", AddressScheme: "DOGE_BASE58", AddressCommitment: "0x" + strings.Repeat("aa", 32)},
		{WalletRef: "0x" + strings.Repeat("22", 32), Coin: "BTC", Network: "mainnet", AddressScheme: "BTC_BECH32", AddressCommitment: "0x" + strings.Repeat("bb", 32)},
	}
	got, err := StakeholderWalletSetRoot(wallets)
	if err != nil {
		t.Fatal(err)
	}
	const want = "0xd78f22cc37dfdb821e407dd52512e02406db516c887e29431b7be8b66fb8d0bb"
	if got != want {
		t.Fatalf("wallet set root=%s want %s", got, want)
	}
}
