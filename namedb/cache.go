package namedb

import (
	"container/list"
	"sync"
)

// cacheEntry represents a single entry in the LRU cache
type cacheEntry struct {
	key   string
	value *NameRecord
}

// lruCache implements a thread-safe LRU (Least Recently Used) cache
// for name records. It provides O(1) lookups and evictions.
type lruCache struct {
	mu       sync.RWMutex
	capacity int
	items    map[string]*list.Element // key -> list element
	list     *list.List               // doubly linked list for LRU ordering
}

// newLRUCache creates a new LRU cache with the specified capacity.
// Capacity must be > 0.
func newLRUCache(capacity int) *lruCache {
	if capacity <= 0 {
		capacity = 10000 // Default as per PLAN.md
	}
	return &lruCache{
		capacity: capacity,
		items:    make(map[string]*list.Element, capacity),
		list:     list.New(),
	}
}

// Get retrieves a value from the cache.
// Returns the value and true if found, nil and false if not found.
// Moves the accessed item to the front (most recently used).
func (c *lruCache) Get(key string) (*NameRecord, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		// Move to front (most recently used)
		c.list.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		return entry.value, true
	}
	return nil, false
}

// Put adds or updates a value in the cache.
// If the cache is at capacity, the least recently used item is evicted.
// Returns true if an item was evicted.
func (c *lruCache) Put(key string, value *NameRecord) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry
	if elem, ok := c.items[key]; ok {
		c.list.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		entry.value = value
		return false
	}

	// Add new entry
	entry := &cacheEntry{key: key, value: value}
	elem := c.list.PushFront(entry)
	c.items[key] = elem

	// Evict if over capacity
	if c.list.Len() > c.capacity {
		c.evictOldest()
		return true
	}
	return false
}

// Delete removes an entry from the cache.
// Returns true if the entry existed.
func (c *lruCache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.removeElement(elem)
		return true
	}
	return false
}

// Clear removes all entries from the cache.
func (c *lruCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element, c.capacity)
	c.list.Init()
}

// Len returns the current number of items in the cache.
func (c *lruCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.list.Len()
}

// evictOldest removes the least recently used item from the cache.
// Must be called with lock held.
func (c *lruCache) evictOldest() {
	elem := c.list.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

// removeElement removes an element from both the list and map.
// Must be called with lock held.
func (c *lruCache) removeElement(elem *list.Element) {
	c.list.Remove(elem)
	entry := elem.Value.(*cacheEntry)
	delete(c.items, entry.key)
}
