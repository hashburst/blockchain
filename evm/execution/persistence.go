package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

// Anchor must be derived from an approved, finalized activation snapshot.
// It does not create or replace a HashBurst genesis. Balance import policy is
// the responsibility of the consensus integration, not this storage primitive.
type Anchor struct {
	ChainID        uint64
	Height         uint64
	Hash           common.Hash
	Balances       map[common.Address]string
	PreviousHashes map[uint64]common.Hash
}
type Record struct {
	Number       uint64
	Time         uint64
	Hash         common.Hash
	ParentHash   common.Hash
	Coinbase     common.Address
	Random       common.Hash
	GasLimit     uint64
	BaseFee      string
	Transactions [][]byte
	StateRoot    common.Hash
	ReceiptsRoot common.Hash
	GasUsed      uint64
}
type Store struct {
	mu       sync.Mutex
	dir      string
	lock     *os.File
	anchor   Anchor
	state    *state.StateDB
	height   uint64
	hash     common.Hash
	hashes   map[uint64]common.Hash
	poisoned bool
}

func writeDurable(path string, data []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".pending-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	// Link provides no-replace publication. Existing finalized records are immutable.
	if e = os.Link(name, path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func OpenStore(dir string, anchor Anchor) (out *Store, err error) {
	if _, err = Config(anchor.ChainID); err != nil {
		return nil, err
	}
	if anchor.Hash == (common.Hash{}) {
		return nil, fmt.Errorf("missing finalized anchor")
	}
	// Detach caller-owned maps, so config cannot change after pinning.
	data, err := json.Marshal(anchor)
	if err != nil {
		return nil, err
	}
	var copyAnchor Anchor
	if err = json.Unmarshal(data, &copyAnchor); err != nil {
		return nil, err
	}
	anchor = copyAnchor
	start := uint64(0)
	if anchor.Height > 255 {
		start = anchor.Height - 255
	}
	for n := start; n < anchor.Height; n++ {
		if anchor.PreviousHashes[n] == (common.Hash{}) {
			return nil, fmt.Errorf("missing activation ancestor hash %d", n)
		}
	}

	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("unsafe store directory")
	}
	lock, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, err
	}
	defer func() {
		if err != nil {
			lock.Close()
		}
	}()
	digest := sha256.Sum256(data)
	pin := []byte(hex.EncodeToString(digest[:]))
	pinPath := filepath.Join(dir, "anchor.pin")
	existing, e := os.ReadFile(pinPath)
	if os.IsNotExist(e) {
		entries, e := os.ReadDir(dir)
		if e != nil {
			return nil, e
		}
		for _, entry := range entries {
			if entry.Name() != "lock" {
				return nil, fmt.Errorf("refusing unpinned nonempty store")
			}
		}
		if err = writeDurable(pinPath, pin); err != nil {
			return nil, err
		}
	} else if e != nil {
		return nil, e
	} else if string(existing) != string(pin) {
		return nil, fmt.Errorf("activation anchor mismatch")
	}
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		return nil, err
	}
	for address, value := range anchor.Balances {
		n, ok := new(big.Int).SetString(value, 10)
		if !ok || n.Sign() < 0 || n.BitLen() > 256 {
			return nil, fmt.Errorf("invalid anchor balance")
		}
		st.SetBalance(address, uint256.MustFromBig(n), tracing.BalanceChangeUnspecified)
	}
	s := &Store{dir: dir, lock: lock, anchor: anchor, state: st, height: anchor.Height, hash: anchor.Hash, hashes: map[uint64]common.Hash{anchor.Height: anchor.Hash}}
	for n, h := range anchor.PreviousHashes {
		if n < anchor.Height {
			s.hashes[n] = h
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		expected := fmt.Sprintf("%020d.json", s.height+1)
		if entry.Name() != expected {
			return nil, fmt.Errorf("record gap or unexpected record %s", entry.Name())
		}
		b, e := os.ReadFile(filepath.Join(dir, entry.Name()))
		if e != nil {
			return nil, e
		}
		var record Record
		if e = json.Unmarshal(b, &record); e != nil {
			return nil, fmt.Errorf("corrupt finalized record: %w", e)
		}
		result, e := s.execute(context.Background(), record)
		if e != nil {
			return nil, e
		}
		s.accept(record, result)
	}
	return s, nil
}
func (s *Store) execute(ctx context.Context, r Record) (*Result, error) {
	if s.height == ^uint64(0) || r.Number != s.height+1 || r.ParentHash != s.hash || r.Hash == (common.Hash{}) {
		return nil, fmt.Errorf("finalized record does not extend head")
	}
	fee, ok := new(big.Int).SetString(r.BaseFee, 10)
	if !ok {
		return nil, fmt.Errorf("invalid base fee")
	}
	result, e := ApplyBlock(ctx, s.state, s.anchor.ChainID, Block{Number: r.Number, Time: r.Time, Hash: r.Hash, ParentHash: r.ParentHash, Coinbase: r.Coinbase, Random: r.Random, GasLimit: r.GasLimit, BaseFee: fee, HashAt: func(n uint64) common.Hash { return s.hashes[n] }}, r.Transactions)
	if e != nil {
		return nil, e
	}
	if result.StateRoot != r.StateRoot || result.ReceiptsRoot != r.ReceiptsRoot || result.GasUsed != r.GasUsed {
		return nil, fmt.Errorf("finalized EVM commitments mismatch")
	}
	return result, nil
}
func (s *Store) accept(r Record, result *Result) {
	s.state = result.State
	s.height = r.Number
	s.hash = r.Hash
	s.hashes[r.Number] = r.Hash
	if r.Number > 256 {
		delete(s.hashes, r.Number-256)
	}
}

// Append verifies consensus-supplied commitments, writes and fsyncs the record,
// then advances memory. Any ambiguous write error poisons the handle: reopen
// and replay rather than attempting another append against uncertain disk state.
func (s *Store) Append(ctx context.Context, r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.poisoned || s.lock == nil {
		return fmt.Errorf("store must be reopened")
	}
	result, e := s.execute(ctx, r)
	if e != nil {
		return e
	}
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	if e = writeDurable(filepath.Join(s.dir, fmt.Sprintf("%020d.json", r.Number)), b); e != nil {
		s.poisoned = true
		return e
	}
	s.accept(r, result)
	return nil
}
func (s *Store) Snapshot() (*state.StateDB, uint64, common.Hash) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Copy(), s.height, s.hash
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil
	}
	e := s.lock.Close()
	s.lock = nil
	return e
}
