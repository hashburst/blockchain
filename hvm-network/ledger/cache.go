package ledger

import (
	"container/list"
	"sync"
)

type cached struct {
	key   uint64
	bytes []byte
}

// Cache bounds owned encoded payload bytes and entry count, not process RSS.
// Encoded values avoid returning mutable pooled Block pointers to callers.
type Cache struct {
	mu                         sync.RWMutex
	maxBytes, maxEntries, used int
	items                      map[uint64]*list.Element
	order                      *list.List
}

func NewCache(maxBytes, maxEntries int) *Cache {
	if maxBytes < 0 {
		maxBytes = 0
	}
	if maxEntries < 0 {
		maxEntries = 0
	}
	return &Cache{maxBytes: maxBytes, maxEntries: maxEntries, items: make(map[uint64]*list.Element), order: list.New()}
}

// Put copies before retaining. Budget is checked before allocating.
func (c *Cache) Put(key uint64, b []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.items[key]; e != nil {
		c.remove(e)
	}
	if len(b) > c.maxBytes || c.maxEntries == 0 {
		return false
	}
	for c.used > c.maxBytes-len(b) || len(c.items) >= c.maxEntries {
		c.remove(c.order.Back())
	}
	v := cached{key: key, bytes: append([]byte(nil), b...)}
	c.items[key] = c.order.PushFront(v)
	c.used += len(b)
	return true
}
func (c *Cache) remove(e *list.Element) {
	v := e.Value.(cached)
	c.used -= len(v.bytes)
	delete(c.items, v.key)
	c.order.Remove(e)
}

// Get copies into caller-owned scratch, reusable across reads. No alias survives
// eviction. LRU hits mutate order, so they require Lock rather than RLock.
func (c *Cache) Get(key uint64, dst []byte) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.items[key]
	if e == nil {
		return dst, false
	}
	c.order.MoveToFront(e)
	return append(dst[:0], e.Value.(cached).bytes...), true
}
func (c *Cache) Stats() (entries, bytes int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items), c.used
}
