package hvm

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const MaxStakeholderWallets = 128

// StakeholderProfile stores only commitments and pseudonymous references. The
// source APIKEY and raw wallet addresses from list.json never enter HVM state.
// Selective disclosure is possible later by revealing the original value plus
// its derived per-value salt to an auditor. The namespace key itself stays
// off-chain and is never disclosed to HVM or to the auditor.
type StakeholderProfile struct {
	UserRef                string `json:"user_ref"`
	APIKeyCommitment       string `json:"apikey_commitment"`
	WalletSetRoot          string `json:"wallet_set_root"`
	WalletCount            uint32 `json:"wallet_count"`
	ProfileVersion         uint64 `json:"profile_version"`
	SourceRecordCommitment string `json:"source_record_commitment"`
	UpdatedAt              uint64 `json:"updated_at"`
}

type StakeholderWalletCommitment struct {
	WalletRef         string `json:"wallet_ref"`
	Coin              string `json:"coin"`
	Network           string `json:"network"`
	AddressScheme     string `json:"address_scheme"`
	AddressCommitment string `json:"address_commitment"`
}

type StakeholderProfileRequest struct {
	UserRef                string                        `json:"user_ref"`
	APIKeyCommitment       string                        `json:"apikey_commitment"`
	ProfileVersion         uint64                        `json:"profile_version"`
	SourceRecordCommitment string                        `json:"source_record_commitment"`
	UpdatedAt              uint64                        `json:"updated_at"`
	Wallets                []StakeholderWalletCommitment `json:"wallets"`
}

func validateStakeholderWallet(w StakeholderWalletCommitment) error {
	if err := validateHex32("wallet_ref", w.WalletRef, false); err != nil {
		return err
	}
	if err := validateHex32("address_commitment", w.AddressCommitment, false); err != nil {
		return err
	}
	if strings.TrimSpace(w.Coin) == "" || len(w.Coin) > 32 {
		return fmt.Errorf("wallet coin required")
	}
	if strings.TrimSpace(w.Network) == "" || len(w.Network) > 32 {
		return fmt.Errorf("wallet network required")
	}
	if strings.TrimSpace(w.AddressScheme) == "" || len(w.AddressScheme) > 64 {
		return fmt.Errorf("wallet address_scheme required")
	}
	return nil
}

func stakeholderWalletLeaf(w StakeholderWalletCommitment) [32]byte {
	var b bytes.Buffer
	putPrivacyString(&b, "HASHBURST_STAKEHOLDER_WALLET_V1")
	putPrivacyString(&b, strings.ToLower(w.WalletRef))
	putPrivacyString(&b, strings.ToUpper(strings.TrimSpace(w.Coin)))
	putPrivacyString(&b, strings.ToLower(strings.TrimSpace(w.Network)))
	putPrivacyString(&b, strings.ToUpper(strings.TrimSpace(w.AddressScheme)))
	putPrivacyString(&b, strings.ToLower(w.AddressCommitment))
	return sha256.Sum256(b.Bytes())
}

// StakeholderWalletSetRoot is a deterministic binary Merkle root over sorted
// wallet commitment records. Sorting makes list.json key/order changes unable
// to change the committed wallet set.
func StakeholderWalletSetRoot(wallets []StakeholderWalletCommitment) (string, error) {
	if len(wallets) > MaxStakeholderWallets {
		return "", fmt.Errorf("wallet count must be 0-%d", MaxStakeholderWallets)
	}
	if len(wallets) == 0 {
		h := sha256.Sum256([]byte("HASHBURST_STAKEHOLDER_EMPTY_WALLET_SET_V1"))
		return "0x" + hex.EncodeToString(h[:]), nil
	}
	copyWallets := append([]StakeholderWalletCommitment(nil), wallets...)
	seen := make(map[string]struct{}, len(copyWallets))
	for _, w := range copyWallets {
		if err := validateStakeholderWallet(w); err != nil {
			return "", err
		}
		id := strings.ToLower(w.WalletRef)
		if _, dup := seen[id]; dup {
			return "", fmt.Errorf("duplicate wallet_ref %s", w.WalletRef)
		}
		seen[id] = struct{}{}
	}
	sort.Slice(copyWallets, func(i, j int) bool {
		a, b := copyWallets[i], copyWallets[j]
		ka := strings.ToUpper(a.Coin) + "\x00" + strings.ToLower(a.Network) + "\x00" + strings.ToLower(a.WalletRef)
		kb := strings.ToUpper(b.Coin) + "\x00" + strings.ToLower(b.Network) + "\x00" + strings.ToLower(b.WalletRef)
		return ka < kb
	})
	level := make([][32]byte, len(copyWallets))
	for i, w := range copyWallets {
		level[i] = stakeholderWalletLeaf(w)
	}
	for len(level) > 1 {
		next := make([][32]byte, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			left := level[i]
			right := left
			if i+1 < len(level) {
				right = level[i+1]
			}
			var pair [64]byte
			copy(pair[:32], left[:])
			copy(pair[32:], right[:])
			next = append(next, sha256.Sum256(pair[:]))
		}
		level = next
	}
	return "0x" + hex.EncodeToString(level[0][:]), nil
}

func putPrivacyString(b *bytes.Buffer, s string) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(s)))
	b.Write(n[:])
	b.WriteString(s)
}

func (m *MiningPayoutRegistry) registerStakeholderProfile(ctx ExecutionContext, state *StateDB, contractAddress string, req StakeholderProfileRequest, meter *Meter) ([]Event, error) {
	if err := validateHex32("user_ref", req.UserRef, false); err != nil {
		return nil, err
	}
	if err := validateHex32("apikey_commitment", req.APIKeyCommitment, false); err != nil {
		return nil, err
	}
	if err := validateHex32("source_record_commitment", req.SourceRecordCommitment, false); err != nil {
		return nil, err
	}
	if req.ProfileVersion == 0 {
		return nil, fmt.Errorf("profile_version must be >=1")
	}
	if req.UpdatedAt == 0 {
		return nil, fmt.Errorf("updated_at required")
	}
	root, err := StakeholderWalletSetRoot(req.Wallets)
	if err != nil {
		return nil, err
	}
	user := strings.ToLower(req.UserRef)
	currentKey := m.key(contractAddress, "stakeholder/"+user+"/current")
	var previous StakeholderProfile
	if b, exists := state.Get(currentKey); exists {
		if err := json.Unmarshal(b, &previous); err != nil {
			return nil, fmt.Errorf("corrupt stakeholder profile: %w", err)
		}
		if req.ProfileVersion != previous.ProfileVersion+1 {
			return nil, fmt.Errorf("profile_version %d: expected %d", req.ProfileVersion, previous.ProfileVersion+1)
		}
		if req.UpdatedAt < previous.UpdatedAt {
			return nil, fmt.Errorf("stakeholder updated_at cannot move backwards")
		}
	} else if req.ProfileVersion != 1 {
		return nil, fmt.Errorf("first profile version must be 1")
	}
	if err := meter.Consume(ComputeStateRead); err != nil {
		return nil, err
	}

	profile := StakeholderProfile{
		UserRef: req.UserRef, APIKeyCommitment: req.APIKeyCommitment,
		WalletSetRoot: root, WalletCount: uint32(len(req.Wallets)),
		ProfileVersion: req.ProfileVersion, SourceRecordCommitment: req.SourceRecordCommitment, UpdatedAt: req.UpdatedAt,
	}
	versionPrefix := fmt.Sprintf("stakeholder/%s/version/%020d/", user, req.ProfileVersion)
	profileKey := m.key(contractAddress, versionPrefix+"profile")
	if _, exists := state.Get(profileKey); exists {
		return nil, fmt.Errorf("stakeholder profile version already exists")
	}
	state.Set(profileKey, mustJSON(profile))
	state.Set(currentKey, mustJSON(profile))
	if err := meter.Consume(2 * ComputeStateWrite); err != nil {
		return nil, err
	}
	for _, w := range req.Wallets {
		walletKey := m.key(contractAddress, versionPrefix+"wallet/"+strings.ToLower(w.WalletRef))
		state.Set(walletKey, mustJSON(w))
		if err := meter.Consume(ComputeStateWrite); err != nil {
			return nil, err
		}
	}
	data := mustJSON(profile)
	if err := meter.Consume(ComputeEventBase + uint64(len(data))*ComputePerPayloadByte); err != nil {
		return nil, err
	}
	return []Event{{Contract: contractAddress, Name: "StakeholderProfileCommitted", Topics: []string{req.UserRef, req.APIKeyCommitment, root}, Data: data}}, nil
}

func (m *MiningPayoutRegistry) getStakeholderProfile(state *StateDB, contractAddress string, args json.RawMessage, meter *Meter) ([]byte, error) {
	var q struct {
		UserRef string `json:"user_ref"`
		Version uint64 `json:"version,omitempty"`
	}
	if err := json.Unmarshal(args, &q); err != nil {
		return nil, err
	}
	if err := validateHex32("user_ref", q.UserRef, false); err != nil {
		return nil, err
	}
	key := "stakeholder/" + strings.ToLower(q.UserRef) + "/current"
	if q.Version != 0 {
		key = fmt.Sprintf("stakeholder/%s/version/%020d/profile", strings.ToLower(q.UserRef), q.Version)
	}
	b, ok := state.Get(m.key(contractAddress, key))
	if !ok {
		return nil, fmt.Errorf("stakeholder profile not found")
	}
	if err := meter.Consume(ComputeStateRead); err != nil {
		return nil, err
	}
	return b, nil
}

func (m *MiningPayoutRegistry) getStakeholderWallet(state *StateDB, contractAddress string, args json.RawMessage, meter *Meter) ([]byte, error) {
	var q struct {
		UserRef   string `json:"user_ref"`
		Version   uint64 `json:"version"`
		WalletRef string `json:"wallet_ref"`
	}
	if err := json.Unmarshal(args, &q); err != nil {
		return nil, err
	}
	if err := validateHex32("user_ref", q.UserRef, false); err != nil {
		return nil, err
	}
	if err := validateHex32("wallet_ref", q.WalletRef, false); err != nil {
		return nil, err
	}
	if q.Version == 0 {
		return nil, fmt.Errorf("version required for wallet lookup")
	}
	key := fmt.Sprintf("stakeholder/%s/version/%020d/wallet/%s", strings.ToLower(q.UserRef), q.Version, strings.ToLower(q.WalletRef))
	b, ok := state.Get(m.key(contractAddress, key))
	if !ok {
		return nil, fmt.Errorf("stakeholder wallet commitment not found")
	}
	if err := meter.Consume(ComputeStateRead); err != nil {
		return nil, err
	}
	return b, nil
}

func (m *MiningPayoutRegistry) validatePayoutPrivacyBinding(state *StateDB, contractAddress, currency string, item PayoutItem, meter *Meter) error {
	if item.WalletRef == "" {
		return nil // legacy-import mode
	}
	if item.StakeholderProfileVersion == 0 {
		return fmt.Errorf("stakeholder_profile_version required for private payout binding")
	}
	profileKey := m.key(contractAddress, fmt.Sprintf("stakeholder/%s/version/%020d/profile", strings.ToLower(item.UserRef), item.StakeholderProfileVersion))
	b, ok := state.Get(profileKey)
	if !ok {
		return fmt.Errorf("stakeholder profile version %d not registered for user_ref %s", item.StakeholderProfileVersion, item.UserRef)
	}
	var profile StakeholderProfile
	if err := json.Unmarshal(b, &profile); err != nil {
		return fmt.Errorf("corrupt stakeholder profile: %w", err)
	}
	walletKey := m.key(contractAddress, fmt.Sprintf("stakeholder/%s/version/%020d/wallet/%s", strings.ToLower(item.UserRef), item.StakeholderProfileVersion, strings.ToLower(item.WalletRef)))
	wb, ok := state.Get(walletKey)
	if !ok {
		return fmt.Errorf("wallet_ref %s is not in current stakeholder profile", item.WalletRef)
	}
	var w StakeholderWalletCommitment
	if err := json.Unmarshal(wb, &w); err != nil {
		return fmt.Errorf("corrupt stakeholder wallet commitment: %w", err)
	}
	if !strings.EqualFold(w.Coin, currency) {
		return fmt.Errorf("wallet_ref coin %s does not match payout currency %s", w.Coin, currency)
	}
	if !strings.EqualFold(w.AddressCommitment, item.AddressCommitment) {
		return fmt.Errorf("payout address commitment does not match registered wallet_ref")
	}
	if !strings.EqualFold(w.AddressScheme, item.ExternalAddressScheme) {
		return fmt.Errorf("payout address scheme does not match registered wallet_ref")
	}
	return meter.Consume(2 * ComputeStateRead)
}
