package hvm

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"sync"
)

// StateDB is a deterministic key/value projection of HVM state. It is not a
// second blockchain database: the intended production model rebuilds it from
// accepted HashBurst contract transactions, just as native HBT balances are a
// projection of the chain.
type StateDB struct {
	mu sync.RWMutex
	kv map[string][]byte
}

func NewStateDB() *StateDB {
	return &StateDB{kv: make(map[string][]byte)}
}

func (s *StateDB) Get(key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.kv[key]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), v...), true
}

func (s *StateDB) Set(key string, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kv[key] = append([]byte(nil), value...)
}

func (s *StateDB) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.kv, key)
}

func (s *StateDB) Clone() *StateDB {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := NewStateDB()
	for k, v := range s.kv {
		out.kv[k] = append([]byte(nil), v...)
	}
	return out
}

func (s *StateDB) ReplaceWith(other *StateDB) {
	other.mu.RLock()
	defer other.mu.RUnlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kv = make(map[string][]byte, len(other.kv))
	for k, v := range other.kv {
		s.kv[k] = append([]byte(nil), v...)
	}
}

// Root computes SHA-256 over sorted, length-prefixed key/value pairs. The root
// format is versioned so it can later coexist with a separate EVM state root.
func (s *StateDB) Root() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.kv))
	for k := range s.kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b bytes.Buffer
	putStateBytes(&b, []byte("HASHBURST_HVM_STATE_V1"))
	for _, k := range keys {
		putStateBytes(&b, []byte(k))
		putStateBytes(&b, s.kv[k])
	}
	sum := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(sum[:])
}

func putStateBytes(b *bytes.Buffer, p []byte) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(p)))
	b.Write(n[:])
	b.Write(p)
}
