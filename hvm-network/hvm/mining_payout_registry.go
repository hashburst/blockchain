package hvm

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"hashburst/wallet"
)

const MiningPayoutRegistryType = "HBT-MINING-PAYOUT-REGISTRY/V3"
const MaxPayoutItemsPerBatch = 200
const MaxReportBatches = 10_000

type PayoutStatus string

const (
	StatusAuditing  PayoutStatus = "AUDITING"
	StatusPassed    PayoutStatus = "PASSED"
	StatusSent      PayoutStatus = "SENT"
	StatusCancelled PayoutStatus = "CANCELLED"
)

type MiningPayoutRegistry struct{}

func NewMiningPayoutRegistry() *MiningPayoutRegistry { return &MiningPayoutRegistry{} }
func (m *MiningPayoutRegistry) TypeID() string       { return MiningPayoutRegistryType }

type MiningPayoutRegistryInit struct {
	Admin           string   `json:"admin,omitempty"`
	Recorders       []string `json:"recorders,omitempty"`
	ProfileWriters  []string `json:"profile_writers,omitempty"`
	LegacyImporters []string `json:"legacy_importers,omitempty"`
}

type PayoutItem struct {
	PayoutID                  string `json:"payout_id"`
	UserRef                   string `json:"user_ref"`
	StakeholderProfileVersion uint64 `json:"stakeholder_profile_version,omitempty"`
	WalletRef                 string `json:"wallet_ref,omitempty"`
	AddressCommitment         string `json:"address_commitment,omitempty"`
	ExternalAddressRaw        string `json:"external_address_raw,omitempty"`
	ExternalAddressScheme     string `json:"external_address_scheme"`
	LegacyProxy               string `json:"legacy_proxy,omitempty"`
	LegacyProxyScheme         string `json:"legacy_proxy_scheme,omitempty"`
	AuditedAmountAtomic       string `json:"audited_amount_atomic"`
	ContributionNumerator     uint64 `json:"contribution_numerator"`
	ContributionDenominator   uint64 `json:"contribution_denominator"`
}

type AuditBatchRequest struct {
	ReportID         string       `json:"report_id"`
	BatchID          string       `json:"batch_id"`
	BatchIndex       uint32       `json:"batch_index"`
	BatchCount       uint32       `json:"batch_count"`
	Pool             string       `json:"pool"`
	Currency         string       `json:"currency"`
	CurrencyDecimals uint8        `json:"currency_decimals"`
	SourceTimestamp  uint64       `json:"source_timestamp"`
	EvidenceHash     string       `json:"evidence_hash"`
	LegacySource     string       `json:"legacy_source,omitempty"`
	LegacyFilename   string       `json:"legacy_filename,omitempty"`
	Items            []PayoutItem `json:"items"`
}

type ExternalTransfer struct {
	TransferID        string `json:"transfer_id"`
	ExternalChain     string `json:"external_chain"`
	Network           string `json:"network"`
	TxID              string `json:"txid"`
	BlockHeight       uint64 `json:"block_height,omitempty"`
	BlockHash         string `json:"block_hash,omitempty"`
	ObservedTimestamp uint64 `json:"observed_timestamp"`
	EvidenceHash      string `json:"evidence_hash"`
}

type TransitionItem struct {
	PayoutID             string `json:"payout_id"`
	ObservedAmountAtomic string `json:"observed_amount_atomic,omitempty"`
}

type TransitionBatchRequest struct {
	Status            PayoutStatus      `json:"status"`
	EvidenceHash      string            `json:"evidence_hash"`
	ObservedTimestamp uint64            `json:"observed_timestamp"`
	ExternalTransfer  *ExternalTransfer `json:"external_transfer,omitempty"`
	ReasonCode        string            `json:"reason_code,omitempty"`
	ReasonHash        string            `json:"reason_hash,omitempty"`
	Items             []TransitionItem  `json:"items"`
}

type PayoutCurrent struct {
	PayoutID                  string       `json:"payout_id"`
	UserRef                   string       `json:"user_ref"`
	StakeholderProfileVersion uint64       `json:"stakeholder_profile_version,omitempty"`
	WalletRef                 string       `json:"wallet_ref,omitempty"`
	AddressCommitment         string       `json:"address_commitment,omitempty"`
	Pool                      string       `json:"pool"`
	Currency                  string       `json:"currency"`
	CurrencyDecimals          uint8        `json:"currency_decimals"`
	ExternalAddressRaw        string       `json:"external_address_raw"`
	ExternalAddressScheme     string       `json:"external_address_scheme"`
	LegacyProxy               string       `json:"legacy_proxy,omitempty"`
	LegacyProxyScheme         string       `json:"legacy_proxy_scheme,omitempty"`
	AuditedAmountAtomic       string       `json:"audited_amount_atomic"`
	ObservedAmountAtomic      string       `json:"observed_amount_atomic,omitempty"`
	ContributionNumerator     uint64       `json:"contribution_numerator"`
	ContributionDenominator   uint64       `json:"contribution_denominator"`
	Status                    PayoutStatus `json:"status"`
	ReportID                  string       `json:"report_id"`
	LastEventID               string       `json:"last_event_id"`
	ExternalTransferID        string       `json:"external_transfer_id,omitempty"`
	SourceTimestamp           uint64       `json:"source_timestamp"`
	UpdatedAt                 uint64       `json:"updated_at"`
}

type ReportCurrent struct {
	ReportID         string `json:"report_id"`
	Pool             string `json:"pool"`
	Currency         string `json:"currency"`
	CurrencyDecimals uint8  `json:"currency_decimals"`
	BatchCount       uint32 `json:"batch_count"`
	RecordedBatches  uint32 `json:"recorded_batches"`
	ItemCount        uint64 `json:"item_count"`
	Complete         bool   `json:"complete"`
	FirstTimestamp   uint64 `json:"first_timestamp"`
	LastTimestamp    uint64 `json:"last_timestamp"`
}

type AuditEventRecord struct {
	EventID              string       `json:"event_id"`
	PayoutID             string       `json:"payout_id"`
	PreviousStatus       PayoutStatus `json:"previous_status,omitempty"`
	Status               PayoutStatus `json:"status"`
	ReportID             string       `json:"report_id,omitempty"`
	EvidenceHash         string       `json:"evidence_hash"`
	ExternalTransferID   string       `json:"external_transfer_id,omitempty"`
	ObservedAmountAtomic string       `json:"observed_amount_atomic,omitempty"`
	ReasonCode           string       `json:"reason_code,omitempty"`
	ReasonHash           string       `json:"reason_hash,omitempty"`
	Timestamp            uint64       `json:"timestamp"`
	HVMBlockHeight       uint64       `json:"hvm_block_height"`
	HVMTransactionID     string       `json:"hvm_transaction_id"`
}

func (m *MiningPayoutRegistry) Deploy(ctx ExecutionContext, state *StateDB, contractAddress string, initRaw json.RawMessage, meter *Meter) ([]Event, error) {
	var init MiningPayoutRegistryInit
	if len(initRaw) != 0 && string(initRaw) != "null" {
		if err := json.Unmarshal(initRaw, &init); err != nil {
			return nil, fmt.Errorf("decode init: %w", err)
		}
	}
	admin := init.Admin
	if admin == "" {
		admin = ctx.Sender
	}
	if !wallet.IsValidAddress(admin) {
		return nil, fmt.Errorf("invalid admin address")
	}
	state.Set(m.key(contractAddress, "role/admin/"+normalizeAddress(admin)), []byte{1})
	if err := meter.Consume(ComputeStateWrite); err != nil {
		return nil, err
	}
	// Admin is also a recorder so the canary can be used immediately without a
	// second privileged transaction.
	state.Set(m.key(contractAddress, "role/recorder/"+normalizeAddress(admin)), []byte{1})
	if err := meter.Consume(ComputeStateWrite); err != nil {
		return nil, err
	}
	state.Set(m.key(contractAddress, "role/profile/"+normalizeAddress(admin)), []byte{1})
	if err := meter.Consume(ComputeStateWrite); err != nil {
		return nil, err
	}
	for _, r := range init.Recorders {
		if !wallet.IsValidAddress(r) {
			return nil, fmt.Errorf("invalid recorder address %q", r)
		}
		state.Set(m.key(contractAddress, "role/recorder/"+normalizeAddress(r)), []byte{1})
		if err := meter.Consume(ComputeStateWrite); err != nil {
			return nil, err
		}
	}
	for _, r := range init.ProfileWriters {
		if !wallet.IsValidAddress(r) {
			return nil, fmt.Errorf("invalid profile writer address %q", r)
		}
		state.Set(m.key(contractAddress, "role/profile/"+normalizeAddress(r)), []byte{1})
		if err := meter.Consume(ComputeStateWrite); err != nil {
			return nil, err
		}
	}
	for _, r := range init.LegacyImporters {
		if !wallet.IsValidAddress(r) {
			return nil, fmt.Errorf("invalid legacy importer address %q", r)
		}
		state.Set(m.key(contractAddress, "role/legacy/"+normalizeAddress(r)), []byte{1})
		if err := meter.Consume(ComputeStateWrite); err != nil {
			return nil, err
		}
	}
	ev := Event{Contract: contractAddress, Name: "MiningPayoutRegistryDeployed", Topics: []string{normalizeAddress(admin)}}
	if err := meter.Consume(ComputeEventBase); err != nil {
		return nil, err
	}
	return []Event{ev}, nil
}

func (m *MiningPayoutRegistry) Call(ctx ExecutionContext, state *StateDB, contractAddress, method string, args json.RawMessage, meter *Meter) ([]byte, []Event, error) {
	switch method {
	case "transferAdmin":
		if !m.hasRole(state, contractAddress, "admin", ctx.Sender) {
			return nil, nil, fmt.Errorf("unauthorized: admin role required")
		}
		var a struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal(args, &a); err != nil || !wallet.IsValidAddress(a.Address) {
			return nil, nil, fmt.Errorf("invalid admin address")
		}
		if wallet.AddressEqual(a.Address, ctx.Sender) {
			return nil, nil, fmt.Errorf("new admin must differ from current admin")
		}
		state.Set(m.key(contractAddress, "role/admin/"+normalizeAddress(a.Address)), []byte{1})
		state.Set(m.key(contractAddress, "role/recorder/"+normalizeAddress(a.Address)), []byte{1})
		state.Set(m.key(contractAddress, "role/profile/"+normalizeAddress(a.Address)), []byte{1})
		state.Delete(m.key(contractAddress, "role/admin/"+normalizeAddress(ctx.Sender)))
		// Admin rotation is a security boundary: the old administrator must not
		// silently retain recorder authority. If desired, the new admin can grant
		// it back explicitly in a separate auditable transaction.
		state.Delete(m.key(contractAddress, "role/recorder/"+normalizeAddress(ctx.Sender)))
		state.Delete(m.key(contractAddress, "role/profile/"+normalizeAddress(ctx.Sender)))
		state.Delete(m.key(contractAddress, "role/legacy/"+normalizeAddress(ctx.Sender)))
		if err := meter.Consume(6*ComputeStateWrite + ComputeEventBase); err != nil {
			return nil, nil, err
		}
		return nil, []Event{{Contract: contractAddress, Name: "AdminTransferred", Topics: []string{normalizeAddress(ctx.Sender), normalizeAddress(a.Address)}}}, nil

	case "grantRecorder":
		if !m.hasRole(state, contractAddress, "admin", ctx.Sender) {
			return nil, nil, fmt.Errorf("unauthorized: admin role required")
		}
		var a struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal(args, &a); err != nil || !wallet.IsValidAddress(a.Address) {
			return nil, nil, fmt.Errorf("invalid recorder address")
		}
		state.Set(m.key(contractAddress, "role/recorder/"+normalizeAddress(a.Address)), []byte{1})
		if err := meter.Consume(ComputeStateWrite + ComputeEventBase); err != nil {
			return nil, nil, err
		}
		return nil, []Event{{Contract: contractAddress, Name: "RecorderGranted", Topics: []string{normalizeAddress(a.Address)}}}, nil

	case "revokeRecorder":
		if !m.hasRole(state, contractAddress, "admin", ctx.Sender) {
			return nil, nil, fmt.Errorf("unauthorized: admin role required")
		}
		var a struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal(args, &a); err != nil || !wallet.IsValidAddress(a.Address) {
			return nil, nil, fmt.Errorf("invalid recorder address")
		}
		if wallet.AddressEqual(a.Address, ctx.Sender) {
			return nil, nil, fmt.Errorf("admin cannot revoke its own recorder role")
		}
		state.Delete(m.key(contractAddress, "role/recorder/"+normalizeAddress(a.Address)))
		if err := meter.Consume(ComputeStateWrite + ComputeEventBase); err != nil {
			return nil, nil, err
		}
		return nil, []Event{{Contract: contractAddress, Name: "RecorderRevoked", Topics: []string{normalizeAddress(a.Address)}}}, nil

	case "grantProfileWriter":
		if !m.hasRole(state, contractAddress, "admin", ctx.Sender) {
			return nil, nil, fmt.Errorf("unauthorized: admin role required")
		}
		var a struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal(args, &a); err != nil || !wallet.IsValidAddress(a.Address) {
			return nil, nil, fmt.Errorf("invalid profile writer address")
		}
		state.Set(m.key(contractAddress, "role/profile/"+normalizeAddress(a.Address)), []byte{1})
		if err := meter.Consume(ComputeStateWrite + ComputeEventBase); err != nil {
			return nil, nil, err
		}
		return nil, []Event{{Contract: contractAddress, Name: "ProfileWriterGranted", Topics: []string{normalizeAddress(a.Address)}}}, nil

	case "revokeProfileWriter":
		if !m.hasRole(state, contractAddress, "admin", ctx.Sender) {
			return nil, nil, fmt.Errorf("unauthorized: admin role required")
		}
		var a struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal(args, &a); err != nil || !wallet.IsValidAddress(a.Address) {
			return nil, nil, fmt.Errorf("invalid profile writer address")
		}
		if wallet.AddressEqual(a.Address, ctx.Sender) {
			return nil, nil, fmt.Errorf("admin cannot revoke its own profile role")
		}
		state.Delete(m.key(contractAddress, "role/profile/"+normalizeAddress(a.Address)))
		if err := meter.Consume(ComputeStateWrite + ComputeEventBase); err != nil {
			return nil, nil, err
		}
		return nil, []Event{{Contract: contractAddress, Name: "ProfileWriterRevoked", Topics: []string{normalizeAddress(a.Address)}}}, nil

	case "grantLegacyImporter":
		if !m.hasRole(state, contractAddress, "admin", ctx.Sender) {
			return nil, nil, fmt.Errorf("unauthorized: admin role required")
		}
		var a struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, nil, err
		}
		if !wallet.IsValidAddress(a.Address) {
			return nil, nil, fmt.Errorf("invalid legacy importer address")
		}
		state.Set(m.key(contractAddress, "role/legacy/"+normalizeAddress(a.Address)), []byte{1})
		if err := meter.Consume(ComputeStateWrite + ComputeEventBase); err != nil {
			return nil, nil, err
		}
		return nil, []Event{{Contract: contractAddress, Name: "LegacyImporterGranted", Topics: []string{normalizeAddress(a.Address)}}}, nil

	case "revokeLegacyImporter":
		if !m.hasRole(state, contractAddress, "admin", ctx.Sender) {
			return nil, nil, fmt.Errorf("unauthorized: admin role required")
		}
		var a struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, nil, err
		}
		if !wallet.IsValidAddress(a.Address) {
			return nil, nil, fmt.Errorf("invalid legacy importer address")
		}
		state.Delete(m.key(contractAddress, "role/legacy/"+normalizeAddress(a.Address)))
		if err := meter.Consume(ComputeStateWrite + ComputeEventBase); err != nil {
			return nil, nil, err
		}
		return nil, []Event{{Contract: contractAddress, Name: "LegacyImporterRevoked", Topics: []string{normalizeAddress(a.Address)}}}, nil

	case "registerStakeholderProfile":
		if !m.hasRole(state, contractAddress, "profile", ctx.Sender) {
			return nil, nil, fmt.Errorf("unauthorized: profile writer role required")
		}
		var req StakeholderProfileRequest
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, nil, fmt.Errorf("decode stakeholder profile: %w", err)
		}
		events, err := m.registerStakeholderProfile(ctx, state, contractAddress, req, meter)
		return nil, events, err

	case "getStakeholderProfile":
		b, err := m.getStakeholderProfile(state, contractAddress, args, meter)
		return b, nil, err

	case "getStakeholderWallet":
		b, err := m.getStakeholderWallet(state, contractAddress, args, meter)
		return b, nil, err

	case "recordAuditBatch":
		if !m.hasRole(state, contractAddress, "recorder", ctx.Sender) {
			return nil, nil, fmt.Errorf("unauthorized: recorder role required")
		}
		var req AuditBatchRequest
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, nil, fmt.Errorf("decode audit batch: %w", err)
		}
		events, err := m.recordAuditBatch(ctx, state, contractAddress, req, meter)
		return nil, events, err

	case "recordTransitionBatch":
		if !m.hasRole(state, contractAddress, "recorder", ctx.Sender) {
			return nil, nil, fmt.Errorf("unauthorized: recorder role required")
		}
		var req TransitionBatchRequest
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, nil, fmt.Errorf("decode transition batch: %w", err)
		}
		events, err := m.recordTransitionBatch(ctx, state, contractAddress, req, meter)
		return nil, events, err

	case "getPayout":
		var q struct {
			PayoutID string `json:"payout_id"`
		}
		if err := json.Unmarshal(args, &q); err != nil {
			return nil, nil, err
		}
		if err := validateHex32("payout_id", q.PayoutID, false); err != nil {
			return nil, nil, err
		}
		b, ok := state.Get(m.key(contractAddress, "payout/"+strings.ToLower(q.PayoutID)+"/current"))
		if !ok {
			return nil, nil, fmt.Errorf("payout not found")
		}
		if err := meter.Consume(ComputeStateRead); err != nil {
			return nil, nil, err
		}
		return b, nil, nil

	case "getPayoutEventCount":
		var q struct {
			PayoutID string `json:"payout_id"`
		}
		if err := json.Unmarshal(args, &q); err != nil {
			return nil, nil, err
		}
		count := m.eventCount(state, contractAddress, q.PayoutID)
		if err := meter.Consume(ComputeStateRead); err != nil {
			return nil, nil, err
		}
		return []byte(strconv.FormatUint(count, 10)), nil, nil

	case "getPayoutEvent":
		var q struct {
			PayoutID string `json:"payout_id"`
			Index    uint64 `json:"index"`
		}
		if err := json.Unmarshal(args, &q); err != nil {
			return nil, nil, err
		}
		b, ok := state.Get(m.key(contractAddress, fmt.Sprintf("payout/%s/event/%020d", strings.ToLower(q.PayoutID), q.Index)))
		if !ok {
			return nil, nil, fmt.Errorf("payout event not found")
		}
		if err := meter.Consume(ComputeStateRead); err != nil {
			return nil, nil, err
		}
		return b, nil, nil

	case "getReport":
		var q struct {
			ReportID string `json:"report_id"`
		}
		if err := json.Unmarshal(args, &q); err != nil {
			return nil, nil, err
		}
		if err := validateHex32("report_id", q.ReportID, false); err != nil {
			return nil, nil, err
		}
		b, ok := state.Get(m.key(contractAddress, "report/"+strings.ToLower(q.ReportID)+"/current"))
		if !ok {
			return nil, nil, fmt.Errorf("report not found")
		}
		if err := meter.Consume(ComputeStateRead); err != nil {
			return nil, nil, err
		}
		return b, nil, nil

	case "getExternalTransfer":
		var q struct {
			TransferID string `json:"transfer_id"`
		}
		if err := json.Unmarshal(args, &q); err != nil {
			return nil, nil, err
		}
		if err := validateHex32("transfer_id", q.TransferID, false); err != nil {
			return nil, nil, err
		}
		b, ok := state.Get(m.key(contractAddress, "transfer/"+strings.ToLower(q.TransferID)))
		if !ok {
			return nil, nil, fmt.Errorf("external transfer not found")
		}
		if err := meter.Consume(ComputeStateRead); err != nil {
			return nil, nil, err
		}
		return b, nil, nil

	default:
		return nil, nil, fmt.Errorf("unknown MiningPayoutRegistry method %q", method)
	}
}

func (m *MiningPayoutRegistry) recordAuditBatch(ctx ExecutionContext, state *StateDB, contractAddress string, req AuditBatchRequest, meter *Meter) ([]Event, error) {
	if err := validateHex32("report_id", req.ReportID, false); err != nil {
		return nil, err
	}
	if err := validateHex32("batch_id", req.BatchID, false); err != nil {
		return nil, err
	}
	if err := validateHex32("evidence_hash", req.EvidenceHash, false); err != nil {
		return nil, err
	}
	if req.BatchCount == 0 || req.BatchCount > MaxReportBatches || req.BatchIndex >= req.BatchCount {
		return nil, fmt.Errorf("invalid batch index/count")
	}
	if req.SourceTimestamp == 0 {
		return nil, fmt.Errorf("source_timestamp required")
	}
	if strings.TrimSpace(req.Pool) == "" || len(req.Pool) > 128 {
		return nil, fmt.Errorf("invalid pool")
	}
	if strings.TrimSpace(req.Currency) == "" || len(req.Currency) > 32 {
		return nil, fmt.Errorf("invalid currency")
	}
	if len(req.Items) == 0 || len(req.Items) > MaxPayoutItemsPerBatch {
		return nil, fmt.Errorf("invalid item count: 1-%d required", MaxPayoutItemsPerBatch)
	}

	reportID := strings.ToLower(req.ReportID)
	reportKey := m.key(contractAddress, "report/"+reportID+"/current")
	batchIndexKey := m.key(contractAddress, fmt.Sprintf("report/%s/batch_index/%010d", reportID, req.BatchIndex))
	if _, exists := state.Get(batchIndexKey); exists {
		return nil, fmt.Errorf("report batch index already recorded")
	}
	if err := meter.Consume(ComputeStateRead); err != nil {
		return nil, err
	}
	var report ReportCurrent
	if b, exists := state.Get(reportKey); exists {
		if err := json.Unmarshal(b, &report); err != nil {
			return nil, fmt.Errorf("corrupt report state: %w", err)
		}
		if report.BatchCount != req.BatchCount || report.Pool != req.Pool || !strings.EqualFold(report.Currency, req.Currency) || report.CurrencyDecimals != req.CurrencyDecimals {
			return nil, fmt.Errorf("report metadata mismatch")
		}
		if report.Complete || report.RecordedBatches >= report.BatchCount {
			return nil, fmt.Errorf("report already complete")
		}
	} else {
		report = ReportCurrent{
			ReportID: req.ReportID, Pool: req.Pool, Currency: strings.ToUpper(req.Currency), CurrencyDecimals: req.CurrencyDecimals,
			BatchCount: req.BatchCount, FirstTimestamp: req.SourceTimestamp, LastTimestamp: req.SourceTimestamp,
		}
	}

	batchKey := m.key(contractAddress, "batch/"+strings.ToLower(req.BatchID))
	if _, exists := state.Get(batchKey); exists {
		return nil, fmt.Errorf("batch already recorded")
	}
	if err := meter.Consume(ComputeStateRead); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(req.Items))
	events := make([]Event, 0, len(req.Items)+1)
	for _, item := range req.Items {
		if item.WalletRef == "" && !m.hasRole(state, contractAddress, "legacy", ctx.Sender) {
			return nil, fmt.Errorf("legacy-import payout requires dedicated legacy importer role")
		}
		if err := validatePayoutItem(item); err != nil {
			return nil, err
		}
		if err := m.validatePayoutPrivacyBinding(state, contractAddress, req.Currency, item, meter); err != nil {
			return nil, err
		}
		pid := strings.ToLower(item.PayoutID)
		if _, dup := seen[pid]; dup {
			return nil, fmt.Errorf("duplicate payout_id in batch: %s", item.PayoutID)
		}
		seen[pid] = struct{}{}
		currentKey := m.key(contractAddress, "payout/"+pid+"/current")
		if _, exists := state.Get(currentKey); exists {
			return nil, fmt.Errorf("payout already exists: %s", item.PayoutID)
		}
		if err := meter.Consume(ComputePayoutItem + ComputeStateRead); err != nil {
			return nil, err
		}

		eventID := hashID("HVM_MPR_EVENT_V3", pid, string(StatusAuditing), strings.ToLower(req.EvidenceHash), strconv.FormatUint(req.SourceTimestamp, 10))
		current := PayoutCurrent{
			PayoutID: item.PayoutID, UserRef: item.UserRef, StakeholderProfileVersion: item.StakeholderProfileVersion, WalletRef: item.WalletRef, AddressCommitment: item.AddressCommitment, Pool: req.Pool, Currency: strings.ToUpper(req.Currency),
			CurrencyDecimals: req.CurrencyDecimals, ExternalAddressRaw: item.ExternalAddressRaw,
			ExternalAddressScheme: item.ExternalAddressScheme, LegacyProxy: item.LegacyProxy,
			LegacyProxyScheme: item.LegacyProxyScheme, AuditedAmountAtomic: canonicalUint(item.AuditedAmountAtomic),
			ContributionNumerator: item.ContributionNumerator, ContributionDenominator: item.ContributionDenominator,
			Status: StatusAuditing, ReportID: req.ReportID, LastEventID: eventID,
			SourceTimestamp: req.SourceTimestamp, UpdatedAt: req.SourceTimestamp,
		}
		state.Set(currentKey, mustJSON(current))
		if err := meter.Consume(ComputeStateWrite); err != nil {
			return nil, err
		}
		rec := AuditEventRecord{EventID: eventID, PayoutID: item.PayoutID, Status: StatusAuditing,
			ReportID: req.ReportID, EvidenceHash: req.EvidenceHash, Timestamp: req.SourceTimestamp,
			HVMBlockHeight: ctx.BlockHeight, HVMTransactionID: ctx.TxID}
		if err := m.appendPayoutEvent(state, contractAddress, item.PayoutID, rec, meter); err != nil {
			return nil, err
		}
		data := mustJSON(rec)
		events = append(events, Event{Contract: contractAddress, Name: "PayoutAuditRecorded", Topics: []string{item.PayoutID, string(StatusAuditing), req.ReportID}, Data: data})
		if err := meter.Consume(ComputeEventBase + uint64(len(data))*ComputePerPayloadByte); err != nil {
			return nil, err
		}
	}

	state.Set(batchKey, mustJSON(struct {
		ReportID   string `json:"report_id"`
		BatchIndex uint32 `json:"batch_index"`
		BatchCount uint32 `json:"batch_count"`
		ItemCount  int    `json:"item_count"`
	}{req.ReportID, req.BatchIndex, req.BatchCount, len(req.Items)}))
	if err := meter.Consume(ComputeStateWrite); err != nil {
		return nil, err
	}
	state.Set(batchIndexKey, []byte(strings.ToLower(req.BatchID)))
	report.RecordedBatches++
	report.ItemCount += uint64(len(req.Items))
	if report.FirstTimestamp == 0 || req.SourceTimestamp < report.FirstTimestamp {
		report.FirstTimestamp = req.SourceTimestamp
	}
	if req.SourceTimestamp > report.LastTimestamp {
		report.LastTimestamp = req.SourceTimestamp
	}
	report.Complete = report.RecordedBatches == report.BatchCount
	state.Set(reportKey, mustJSON(report))
	if err := meter.Consume(2 * ComputeStateWrite); err != nil {
		return nil, err
	}
	events = append(events, Event{Contract: contractAddress, Name: "AuditBatchRecorded", Topics: []string{req.ReportID, req.BatchID}, Data: mustJSON(req)})
	if report.Complete {
		events = append(events, Event{Contract: contractAddress, Name: "AuditReportCompleted", Topics: []string{req.ReportID}, Data: mustJSON(report)})
		if err := meter.Consume(ComputeEventBase); err != nil {
			return nil, err
		}
	}
	return events, nil
}

func (m *MiningPayoutRegistry) recordTransitionBatch(ctx ExecutionContext, state *StateDB, contractAddress string, req TransitionBatchRequest, meter *Meter) ([]Event, error) {
	if req.Status != StatusPassed && req.Status != StatusSent && req.Status != StatusCancelled {
		return nil, fmt.Errorf("transition status must be PASSED, SENT, or CANCELLED")
	}
	if err := validateHex32("evidence_hash", req.EvidenceHash, false); err != nil {
		return nil, err
	}
	if len(req.Items) == 0 || len(req.Items) > MaxPayoutItemsPerBatch {
		return nil, fmt.Errorf("invalid transition item count")
	}
	if req.ObservedTimestamp == 0 {
		return nil, fmt.Errorf("observed_timestamp required")
	}

	transferID := ""
	if req.Status == StatusSent {
		if req.ExternalTransfer == nil {
			return nil, fmt.Errorf("SENT requires external_transfer")
		}
		if err := validateExternalTransfer(*req.ExternalTransfer); err != nil {
			return nil, err
		}
		transferID = req.ExternalTransfer.TransferID
		transferKey := m.key(contractAddress, "transfer/"+strings.ToLower(transferID))
		transferJSON := mustJSON(req.ExternalTransfer)
		if existing, exists := state.Get(transferKey); exists {
			if string(existing) != string(transferJSON) {
				return nil, fmt.Errorf("external transfer id already exists with different evidence")
			}
			if err := meter.Consume(ComputeStateRead); err != nil {
				return nil, err
			}
		} else {
			state.Set(transferKey, transferJSON)
			if err := meter.Consume(ComputeStateRead + ComputeStateWrite); err != nil {
				return nil, err
			}
		}
	} else if req.ExternalTransfer != nil {
		return nil, fmt.Errorf("external_transfer is only valid for SENT")
	}
	if req.Status == StatusCancelled && strings.TrimSpace(req.ReasonCode) == "" {
		return nil, fmt.Errorf("CANCELLED requires reason_code")
	}
	if req.ReasonHash != "" {
		if err := validateHex32("reason_hash", req.ReasonHash, true); err != nil {
			return nil, err
		}
	}

	seen := make(map[string]struct{}, len(req.Items))
	events := make([]Event, 0, len(req.Items)+1)
	for _, item := range req.Items {
		if err := validateHex32("payout_id", item.PayoutID, false); err != nil {
			return nil, err
		}
		pid := strings.ToLower(item.PayoutID)
		if _, dup := seen[pid]; dup {
			return nil, fmt.Errorf("duplicate payout_id in transition")
		}
		seen[pid] = struct{}{}
		currentKey := m.key(contractAddress, "payout/"+pid+"/current")
		b, ok := state.Get(currentKey)
		if !ok {
			return nil, fmt.Errorf("payout not found: %s", item.PayoutID)
		}
		var current PayoutCurrent
		if err := json.Unmarshal(b, &current); err != nil {
			return nil, fmt.Errorf("corrupt payout state: %w", err)
		}
		if !allowedTransition(current.Status, req.Status) {
			return nil, fmt.Errorf("invalid status transition %s -> %s for %s", current.Status, req.Status, item.PayoutID)
		}
		if req.Status == StatusSent {
			if !strings.EqualFold(current.Currency, req.ExternalTransfer.ExternalChain) {
				return nil, fmt.Errorf("external chain %s does not match payout currency %s", req.ExternalTransfer.ExternalChain, current.Currency)
			}
			if strings.TrimSpace(item.ObservedAmountAtomic) == "" {
				return nil, fmt.Errorf("SENT requires observed_amount_atomic for %s", item.PayoutID)
			}
		}
		if item.ObservedAmountAtomic != "" {
			if _, err := parseUint(item.ObservedAmountAtomic, true); err != nil {
				return nil, fmt.Errorf("observed amount: %w", err)
			}
			current.ObservedAmountAtomic = canonicalUint(item.ObservedAmountAtomic)
		}
		current.Status = req.Status
		current.ExternalTransferID = transferID
		current.UpdatedAt = req.ObservedTimestamp
		eventID := hashID("HVM_MPR_EVENT_V3", pid, string(req.Status), strings.ToLower(req.EvidenceHash), strings.ToLower(transferID), strconv.FormatUint(req.ObservedTimestamp, 10))
		prevStatus := PayoutStatus("")
		// Re-read previous from the event chain only for the event metadata.
		var previous PayoutCurrent
		_ = json.Unmarshal(b, &previous)
		prevStatus = previous.Status
		current.LastEventID = eventID
		state.Set(currentKey, mustJSON(current))
		if err := meter.Consume(ComputeTransitionItem + ComputeStateRead + ComputeStateWrite); err != nil {
			return nil, err
		}

		rec := AuditEventRecord{EventID: eventID, PayoutID: item.PayoutID, PreviousStatus: prevStatus,
			Status: req.Status, EvidenceHash: req.EvidenceHash, ExternalTransferID: transferID,
			ObservedAmountAtomic: current.ObservedAmountAtomic, ReasonCode: req.ReasonCode,
			ReasonHash: req.ReasonHash, Timestamp: req.ObservedTimestamp, HVMBlockHeight: ctx.BlockHeight,
			HVMTransactionID: ctx.TxID}
		if err := m.appendPayoutEvent(state, contractAddress, item.PayoutID, rec, meter); err != nil {
			return nil, err
		}
		data := mustJSON(rec)
		events = append(events, Event{Contract: contractAddress, Name: "PayoutStatusRecorded", Topics: []string{item.PayoutID, string(req.Status), transferID}, Data: data})
		if err := meter.Consume(ComputeEventBase + uint64(len(data))*ComputePerPayloadByte); err != nil {
			return nil, err
		}
	}
	if req.Status == StatusSent && req.ExternalTransfer != nil {
		data := mustJSON(req.ExternalTransfer)
		events = append(events, Event{Contract: contractAddress, Name: "ExternalTransferRecorded", Topics: []string{req.ExternalTransfer.TransferID, strings.ToUpper(req.ExternalTransfer.ExternalChain), req.ExternalTransfer.TxID}, Data: data})
		if err := meter.Consume(ComputeEventBase + uint64(len(data))*ComputePerPayloadByte); err != nil {
			return nil, err
		}
	}
	return events, nil
}

func validatePayoutItem(i PayoutItem) error {
	if err := validateHex32("payout_id", i.PayoutID, false); err != nil {
		return err
	}
	if err := validateHex32("user_ref", i.UserRef, false); err != nil {
		return err
	}
	if strings.TrimSpace(i.ExternalAddressScheme) == "" || len(i.ExternalAddressScheme) > 64 {
		return fmt.Errorf("external_address_scheme required")
	}
	if i.WalletRef != "" || i.AddressCommitment != "" {
		if i.StakeholderProfileVersion == 0 {
			return fmt.Errorf("stakeholder_profile_version required when wallet commitment is used")
		}
		if err := validateHex32("wallet_ref", i.WalletRef, false); err != nil {
			return err
		}
		if err := validateHex32("address_commitment", i.AddressCommitment, false); err != nil {
			return err
		}
		if i.ExternalAddressRaw != "" {
			return fmt.Errorf("privacy-preserving payout item must not include external_address_raw")
		}
	} else {
		// Legacy-import mode only. New payout pipelines should use wallet_ref +
		// address_commitment and keep the actual wallet in private evidence.
		if i.StakeholderProfileVersion != 0 {
			return fmt.Errorf("legacy payout item must not set stakeholder_profile_version")
		}
		if strings.TrimSpace(i.ExternalAddressRaw) == "" || len(i.ExternalAddressRaw) > 256 {
			return fmt.Errorf("legacy payout item requires external_address_raw <=256 bytes")
		}
	}
	if _, err := parseUint(i.AuditedAmountAtomic, false); err != nil {
		return fmt.Errorf("audited amount: %w", err)
	}
	if i.ContributionDenominator == 0 {
		return fmt.Errorf("contribution denominator must be >0")
	}
	if i.ContributionNumerator > i.ContributionDenominator {
		return fmt.Errorf("contribution numerator exceeds denominator")
	}
	if i.LegacyProxy != "" && len(i.LegacyProxy) > 256 {
		return fmt.Errorf("legacy_proxy too long")
	}
	return nil
}

func ExternalTransferID(externalChain, network, txID string) string {
	return hashID("HVM_EXTERNAL_TRANSFER_V1", strings.ToUpper(strings.TrimSpace(externalChain)), strings.ToLower(strings.TrimSpace(network)), strings.ToLower(strings.TrimSpace(txID)))
}

func validateExternalTransfer(t ExternalTransfer) error {
	if err := validateHex32("transfer_id", t.TransferID, false); err != nil {
		return err
	}
	if strings.TrimSpace(t.ExternalChain) == "" || len(t.ExternalChain) > 32 {
		return fmt.Errorf("external_chain required")
	}
	if strings.TrimSpace(t.Network) == "" || len(t.Network) > 32 {
		return fmt.Errorf("network required")
	}
	if strings.TrimSpace(t.TxID) == "" || len(t.TxID) > 256 {
		return fmt.Errorf("txid required")
	}
	if !strings.EqualFold(t.TransferID, ExternalTransferID(t.ExternalChain, t.Network, t.TxID)) {
		return fmt.Errorf("transfer_id does not match external chain/network/txid")
	}
	if t.ObservedTimestamp == 0 {
		return fmt.Errorf("external transfer observed_timestamp required")
	}
	if err := validateHex32("external transfer evidence_hash", t.EvidenceHash, false); err != nil {
		return err
	}
	return nil
}

func allowedTransition(from, to PayoutStatus) bool {
	switch from {
	case StatusAuditing:
		return to == StatusPassed || to == StatusSent || to == StatusCancelled
	case StatusPassed:
		return to == StatusSent || to == StatusCancelled
	default:
		return false
	}
}

func parseUint(s string, allowZero bool) (*big.Int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("amount required")
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() < 0 {
		return nil, fmt.Errorf("invalid unsigned integer %q", s)
	}
	if !allowZero && n.Sign() == 0 {
		return nil, fmt.Errorf("amount must be >0")
	}
	if n.BitLen() > 256 {
		return nil, fmt.Errorf("amount exceeds uint256")
	}
	return n, nil
}

func canonicalUint(s string) string {
	n, _ := parseUint(s, true)
	if n == nil {
		return "0"
	}
	return n.String()
}

func (m *MiningPayoutRegistry) hasRole(state *StateDB, contractAddress, role, addr string) bool {
	_, ok := state.Get(m.key(contractAddress, "role/"+role+"/"+normalizeAddress(addr)))
	return ok
}

func (m *MiningPayoutRegistry) appendPayoutEvent(state *StateDB, contractAddress, payoutID string, rec AuditEventRecord, meter *Meter) error {
	if _, exists := state.Get(m.key(contractAddress, "event/"+strings.ToLower(rec.EventID))); exists {
		return fmt.Errorf("audit event already recorded")
	}
	count := m.eventCount(state, contractAddress, payoutID)
	state.Set(m.key(contractAddress, fmt.Sprintf("payout/%s/event/%020d", strings.ToLower(payoutID), count)), mustJSON(rec))
	state.Set(m.key(contractAddress, "payout/"+strings.ToLower(payoutID)+"/event_count"), []byte(strconv.FormatUint(count+1, 10)))
	state.Set(m.key(contractAddress, "event/"+strings.ToLower(rec.EventID)), []byte{1})
	return meter.Consume(3 * ComputeStateWrite)
}

func (m *MiningPayoutRegistry) eventCount(state *StateDB, contractAddress, payoutID string) uint64 {
	b, ok := state.Get(m.key(contractAddress, "payout/"+strings.ToLower(payoutID)+"/event_count"))
	if !ok {
		return 0
	}
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

func (m *MiningPayoutRegistry) key(contractAddress, suffix string) string {
	return "contract/" + normalizeAddress(contractAddress) + "/mpr/" + suffix
}
