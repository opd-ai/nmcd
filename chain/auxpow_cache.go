package chain

import (
	"container/list"
	"sync"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// auxPowCacheEntry represents a single entry in the AuxPow LRU cache
type auxPowCacheEntry struct {
	key   chainhash.Hash
	value *AuxPow
}

// auxPowLRUCache implements a thread-safe LRU (Least Recently Used) cache
// for AuxPow data. It provides O(1) lookups and evictions with bounded memory usage.
//
// This cache is used to temporarily store AuxPow data for blocks during validation.
// AuxPow data is set when blocks arrive from the network (SetBlockAuxPowFromBytes)
// and cleared after validation (clearBlockAuxPow). The LRU eviction policy ensures
// that memory usage remains bounded even under adversarial conditions where many
// blocks with AuxPow arrive simultaneously.
type auxPowLRUCache struct {
	mu       sync.RWMutex
	capacity int
	items    map[chainhash.Hash]*list.Element // hash -> list element
	list     *list.List                       // doubly linked list for LRU ordering
}

// DefaultAuxPowCacheSize is the default maximum number of AuxPow entries to cache.
// This value is chosen to be large enough to handle normal operation (blocks are
// processed sequentially) while providing protection against adversarial scenarios.
// Each AuxPow entry is approximately 1KB, so 100 entries use ~100KB memory.
const DefaultAuxPowCacheSize = 100

// newAuxPowLRUCache creates a new AuxPow LRU cache with the specified capacity.
// Capacity must be > 0; if not, defaults to DefaultAuxPowCacheSize.
func newAuxPowLRUCache(capacity int) *auxPowLRUCache {
	if capacity <= 0 {
		capacity = DefaultAuxPowCacheSize
	}
	return &auxPowLRUCache{
		capacity: capacity,
		items:    make(map[chainhash.Hash]*list.Element, capacity),
		list:     list.New(),
	}
}

// Get retrieves AuxPow data from the cache by block hash.
// Returns the AuxPow and true if found, nil and false if not found.
// Moves the accessed item to the front (most recently used).
//
// Note: Get uses a full mutex lock (not RLock) because MoveToFront modifies
// the linked list structure. This is intentional - the LRU ordering update
// is a write operation even though the map lookup is conceptually a read.
func (c *auxPowLRUCache) Get(hash *chainhash.Hash) (*AuxPow, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[*hash]; ok {
		// Move to front (most recently used)
		c.list.MoveToFront(elem)
		entry := elem.Value.(*auxPowCacheEntry)
		return entry.value, true
	}
	return nil, false
}

// Put adds or updates AuxPow data in the cache.
// If the cache is at capacity, the least recently used item is evicted.
// Returns true if an item was evicted.
func (c *auxPowLRUCache) Put(hash *chainhash.Hash, auxPow *AuxPow) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry
	if elem, ok := c.items[*hash]; ok {
		c.list.MoveToFront(elem)
		entry := elem.Value.(*auxPowCacheEntry)
		entry.value = auxPow
		return false
	}

	// Add new entry
	entry := &auxPowCacheEntry{key: *hash, value: auxPow}
	elem := c.list.PushFront(entry)
	c.items[*hash] = elem

	// Evict if over capacity
	if c.list.Len() > c.capacity {
		c.evictOldest()
		return true
	}
	return false
}

// Delete removes an entry from the cache by block hash.
// Returns true if the entry existed.
func (c *auxPowLRUCache) Delete(hash *chainhash.Hash) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[*hash]; ok {
		c.removeElement(elem)
		return true
	}
	return false
}

// Len returns the current number of items in the cache.
func (c *auxPowLRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.list.Len()
}

// Clear removes all entries from the cache.
func (c *auxPowLRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Delete all entries without reallocating the map to avoid unnecessary allocations
	for k := range c.items {
		delete(c.items, k)
	}
	c.list.Init()
}

// evictOldest removes the least recently used item from the cache.
// Must be called with lock held.
func (c *auxPowLRUCache) evictOldest() {
	elem := c.list.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

// removeElement removes an element from both the list and map.
// Must be called with lock held.
func (c *auxPowLRUCache) removeElement(elem *list.Element) {
	c.list.Remove(elem)
	entry := elem.Value.(*auxPowCacheEntry)
	delete(c.items, entry.key)
}
