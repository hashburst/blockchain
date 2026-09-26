package blockchain

// state.go — native HBT/account state projected deterministically from chain
// history. V1 balances remain fully backward compatible; Phase 3B adds a
// monotonic per-account Sequence for TransactionV2 and a deterministic root.

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"

	"hashburst/protocolv2"
	"hashburst/wallet"
)

// State is the native HBT projection. Internal address keys are lowercase
// without 0x. sequences stores the NEXT expected TransactionV2 sequence.
type State struct {
	mu        sync.RWMutex
	balances  map[string]int64  // address -> HBT atomic units (1 HBT = 1e8)
	sequences map[string]uint64 // address -> next expected V2 sequence
}

func NewState() *State {
	return &State{
		balances:  make(map[string]int64),
		sequences: make(map[string]uint64),
	}
}

func stateKey(addr string) string { return normalizeAddr(addr) }

func normalizeAddr(addr string) string {
	s := addr
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		s = s[2:]
	}
	return toLower(s)
}

func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func (s *State) Clone() *State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := NewState()
	for k, v := range s.balances {
		out.balances[k] = v
	}
	for k, v := range s.sequences {
		out.sequences[k] = v
	}
	return out
}

func (s *State) ReplaceWith(other *State) {
	if other == nil {
		return
	}
	other.mu.RLock()
	defer other.mu.RUnlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.balances = make(map[string]int64, len(other.balances))
	for k, v := range other.balances {
		s.balances[k] = v
	}
	s.sequences = make(map[string]uint64, len(other.sequences))
	for k, v := range other.sequences {
		s.sequences[k] = v
	}
}

// BalanceUnits returns native HBT atomic units.
func (s *State) BalanceUnits(addr string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.balances[stateKey(addr)]
}

func (s *State) Balance(addr string) float64 { return UnitsToAmount(s.BalanceUnits(addr)) }

// Sequence returns the NEXT valid TransactionV2 sequence for addr.
func (s *State) Sequence(addr string) uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sequences[stateKey(addr)]
}

// applyTx applies one V1 transaction. V1 semantics are intentionally unchanged.
func (s *State) applyTx(t *Transaction) error {
	amount := AmountToUnits(t.Amount)
	if amount < 0 {
		return fmt.Errorf("importo negativo")
	}
	if t.IsSystem() {
		to := stateKey(t.Receiver)
		if addOverflowsInt64(s.balances[to], amount) {
			return fmt.Errorf("saldo reward overflow")
		}
		s.balances[to] += amount
		return nil
	}
	if amount == 0 {
		return nil
	}
	from := stateKey(t.Sender)
	if s.balances[from] < amount {
		return fmt.Errorf("fondi insufficienti: %s ha %d, richiesti %d", t.Sender, s.balances[from], amount)
	}
	to := stateKey(t.Receiver)
	if addOverflowsInt64(s.balances[to], amount) {
		return fmt.Errorf("saldo destinatario overflow")
	}
	s.balances[from] -= amount
	s.balances[to] += amount
	return nil
}

// CanStartV2 checks sequence and the sender's declared worst-case reservation.
// The reservation is not committed here. A contract revert still consumes the
// sequence and actual fee, but does not transfer ValueUnits.
func (s *State) CanStartV2(t *protocolv2.TransactionV2) error {
	if t == nil {
		return fmt.Errorf("nil v2 transaction")
	}
	if t.IsSystem() {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	from := stateKey(t.Sender)
	expected := s.sequences[from]
	if t.Sequence != expected {
		return fmt.Errorf("sequence %d: expected %d for %s", t.Sequence, expected, t.Sender)
	}
	required, err := checkedAddInt64(t.ValueUnits, t.MaxFeeUnits)
	if err != nil {
		return fmt.Errorf("reservation overflow: %w", err)
	}
	if s.balances[from] < required {
		return fmt.Errorf("fondi insufficienti per value+maxfee: %s ha %d, richiesti %d", t.Sender, s.balances[from], required)
	}
	return nil
}

// SettleV2 commits one V2 account transition after deterministic execution.
// applyValue=false is used for reverted contract calls: sequence and actual fee
// are consumed, while ValueUnits remains with the sender.
func (s *State) SettleV2(t *protocolv2.TransactionV2, actualFee int64, feeCollector, valueRecipient string, applyValue bool) error {
	if t == nil {
		return fmt.Errorf("nil v2 transaction")
	}
	if t.IsSystem() {
		return fmt.Errorf("system V2 settlement is not enabled in Phase 3B")
	}
	if t.ValueUnits < 0 {
		return fmt.Errorf("negative value")
	}
	if actualFee < 0 || actualFee > t.MaxFeeUnits {
		return fmt.Errorf("actual fee %d exceeds max fee %d", actualFee, t.MaxFeeUnits)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	from := stateKey(t.Sender)
	if t.Sequence != s.sequences[from] {
		return fmt.Errorf("sequence %d: expected %d for %s", t.Sequence, s.sequences[from], t.Sender)
	}

	charge := actualFee
	if applyValue {
		var err error
		charge, err = checkedAddInt64(charge, t.ValueUnits)
		if err != nil {
			return fmt.Errorf("settlement charge overflow: %w", err)
		}
	}
	if s.balances[from] < charge {
		return fmt.Errorf("fondi insufficienti al settlement: %s ha %d, richiesti %d", t.Sender, s.balances[from], charge)
	}
	if applyValue && t.ValueUnits > 0 && valueRecipient == "" {
		return fmt.Errorf("value recipient required")
	}
	if actualFee > 0 && feeCollector == "" {
		return fmt.Errorf("fee collector required")
	}
	if s.sequences[from] == ^uint64(0) {
		return fmt.Errorf("account sequence overflow")
	}

	// Compute net account deltas first. This matters when sender, fee collector,
	// and/or value recipient are the same address (for example a transfer sent
	// directly to the fee collector). Checking credits independently can miss a
	// combined int64 overflow or reject a harmless self-transfer.
	deltas := make(map[string]int64, 3)
	addDelta := func(key string, delta int64) error {
		if delta == 0 {
			return nil
		}
		v, err := checkedAddInt64(deltas[key], delta)
		if err != nil {
			return err
		}
		deltas[key] = v
		return nil
	}
	if err := addDelta(from, -charge); err != nil {
		return fmt.Errorf("sender delta overflow: %w", err)
	}
	if actualFee > 0 {
		if err := addDelta(stateKey(feeCollector), actualFee); err != nil {
			return fmt.Errorf("fee delta overflow: %w", err)
		}
	}
	if applyValue && t.ValueUnits > 0 {
		if err := addDelta(stateKey(valueRecipient), t.ValueUnits); err != nil {
			return fmt.Errorf("value delta overflow: %w", err)
		}
	}

	finals := make(map[string]int64, len(deltas))
	for key, delta := range deltas {
		v, err := checkedAddInt64(s.balances[key], delta)
		if err != nil {
			return fmt.Errorf("balance overflow for %s: %w", key, err)
		}
		if v < 0 {
			return fmt.Errorf("negative balance after settlement for %s", key)
		}
		finals[key] = v
	}

	// All checks passed: commit all balance deltas and the sequence atomically
	// under the same lock.
	for key, value := range finals {
		s.balances[key] = value
	}
	s.sequences[from]++
	return nil
}

func checkedAddInt64(a, b int64) (int64, error) {
	if b > 0 && a > int64(^uint64(0)>>1)-b {
		return 0, fmt.Errorf("int64 overflow")
	}
	if b < 0 && a < -int64(^uint64(0)>>1)-1-b {
		return 0, fmt.Errorf("int64 underflow")
	}
	return a + b, nil
}

func addOverflowsInt64(a, b int64) bool {
	_, err := checkedAddInt64(a, b)
	return err != nil
}

// ApplyBlock applies V1 transactions atomically. V2 transactions are executed
// by Blockchain's Phase 3B executor because their fees depend on HVM receipts.
func (s *State) ApplyBlock(b *Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	backupBalances := make(map[string]int64, len(s.balances))
	for k, v := range s.balances {
		backupBalances[k] = v
	}
	for i, tx := range b.Transactions {
		if err := s.applyTx(tx); err != nil {
			s.balances = backupBalances
			return fmt.Errorf("tx %d nel blocco #%d: %w", i, b.Index, err)
		}
	}
	return nil
}

func (s *State) CanApplyBlock(b *Block) error {
	shadow := s.Clone()
	for i, tx := range b.Transactions {
		if err := shadow.applyTx(tx); err != nil {
			return fmt.Errorf("tx %d: %w", i, err)
		}
	}
	return nil
}

func (s *State) Snapshot() map[string]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]int64)
	for k, v := range s.balances {
		if v != 0 {
			out["0x"+k] = v
		}
	}
	return out
}

func (s *State) SequenceSnapshot() map[string]uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]uint64)
	for k, v := range s.sequences {
		if v != 0 {
			out["0x"+k] = v
		}
	}
	return out
}

// Root commits balances and next sequences. Zero-only accounts are omitted so
// the root depends on logical state, not incidental map insertion history.
func (s *State) Root() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keysMap := make(map[string]struct{}, len(s.balances)+len(s.sequences))
	for k, v := range s.balances {
		if v != 0 || s.sequences[k] != 0 {
			keysMap[k] = struct{}{}
		}
	}
	for k, v := range s.sequences {
		if v != 0 || s.balances[k] != 0 {
			keysMap[k] = struct{}{}
		}
	}
	keys := make([]string, 0, len(keysMap))
	for k := range keysMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b bytes.Buffer
	putNativeRootBytes(&b, []byte("HASHBURST_HBT_STATE_V2"))
	for _, k := range keys {
		putNativeRootBytes(&b, []byte(k))
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(s.balances[k]))
		b.Write(n[:])
		binary.BigEndian.PutUint64(n[:], s.sequences[k])
		b.Write(n[:])
	}
	sum := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(sum[:])
}

func putNativeRootBytes(b *bytes.Buffer, p []byte) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(p)))
	b.Write(n[:])
	b.Write(p)
}

// ComputeState rebuilds the V1-compatible projection. Full V2/HVM replay is
// handled by Blockchain.rebuildProjections because it also needs HVM receipts.
func ComputeState(blocks []*Block) (*State, error) {
	st := NewState()
	for _, b := range blocks {
		if err := st.ApplyBlock(b); err != nil {
			return nil, fmt.Errorf("ricostruzione stato al blocco #%d: %w", b.Index, err)
		}
	}
	return st, nil
}

func validAddrForState(addr string) bool {
	return addr == SystemSender || addr == RegistryAddress || wallet.IsValidAddress(addr)
}

// ProtocolTransfer moves already-issued native HBT between protocol-owned
// accounts without consuming an account sequence. It is used only by
// consensus-controlled state transitions such as validator bond withdrawal
// and slashing. Callers execute against a cloned State, so any error aborts the
// whole block projection before commit.
func (s *State) ProtocolTransfer(fromAddr, toAddr string, amount int64) error {
	if amount < 0 {
		return fmt.Errorf("negative protocol transfer")
	}
	if amount == 0 {
		return nil
	}
	if fromAddr == "" || toAddr == "" {
		return fmt.Errorf("protocol transfer requires both addresses")
	}
	from := stateKey(fromAddr)
	to := stateKey(toAddr)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.balances[from] < amount {
		return fmt.Errorf("protocol source %s has %d, requires %d", fromAddr, s.balances[from], amount)
	}
	if from == to {
		return nil
	}
	if addOverflowsInt64(s.balances[to], amount) {
		return fmt.Errorf("protocol destination balance overflow")
	}
	s.balances[from] -= amount
	s.balances[to] += amount
	return nil
}
