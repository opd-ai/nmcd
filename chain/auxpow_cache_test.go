package chain

import (
	"sync"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

func TestAuxPowLRUCache_BasicOperations(t *testing.T) {
	cache := newAuxPowLRUCache(3)

	// Test initial state
	if cache.Len() != 0 {
		t.Errorf("new cache should be empty, got %d", cache.Len())
	}

	// Create test hashes
	hash1 := chainhash.HashH([]byte("block1"))
	hash2 := chainhash.HashH([]byte("block2"))
	hash3 := chainhash.HashH([]byte("block3"))

	// Create test AuxPow entries (use ParentBlock.Version to differentiate)
	auxPow1 := &AuxPow{ParentBlock: wire.BlockHeader{Version: 1}}
	auxPow2 := &AuxPow{ParentBlock: wire.BlockHeader{Version: 2}}
	auxPow3 := &AuxPow{ParentBlock: wire.BlockHeader{Version: 3}}

	// Test Put
	evicted := cache.Put(&hash1, auxPow1)
	if evicted {
		t.Error("should not evict when cache not full")
	}
	if cache.Len() != 1 {
		t.Errorf("expected len 1, got %d", cache.Len())
	}

	// Test Get (hit)
	got, ok := cache.Get(&hash1)
	if !ok {
		t.Error("expected cache hit for hash1")
	}
	if got.ParentBlock.Version != 1 {
		t.Errorf("expected Version 1, got %d", got.ParentBlock.Version)
	}

	// Test Get (miss)
	_, ok = cache.Get(&hash2)
	if ok {
		t.Error("expected cache miss for hash2")
	}

	// Add more entries
	cache.Put(&hash2, auxPow2)
	cache.Put(&hash3, auxPow3)
	if cache.Len() != 3 {
		t.Errorf("expected len 3, got %d", cache.Len())
	}

	// Test Delete
	deleted := cache.Delete(&hash2)
	if !deleted {
		t.Error("expected delete to return true")
	}
	if cache.Len() != 2 {
		t.Errorf("expected len 2 after delete, got %d", cache.Len())
	}

	// Delete non-existent
	deleted = cache.Delete(&hash2)
	if deleted {
		t.Error("delete of non-existent should return false")
	}

	// Test Clear
	cache.Clear()
	if cache.Len() != 0 {
		t.Errorf("expected len 0 after clear, got %d", cache.Len())
	}
}

func TestAuxPowLRUCache_Eviction(t *testing.T) {
	cache := newAuxPowLRUCache(3)

	// Fill cache to capacity
	hashes := make([]chainhash.Hash, 4)
	auxPows := make([]*AuxPow, 4)
	for i := 0; i < 4; i++ {
		hashes[i] = chainhash.HashH([]byte{byte(i)})
		auxPows[i] = &AuxPow{ParentBlock: wire.BlockHeader{Version: int32(i)}}
	}

	// Add first 3 entries (no eviction)
	for i := 0; i < 3; i++ {
		evicted := cache.Put(&hashes[i], auxPows[i])
		if evicted {
			t.Errorf("entry %d should not cause eviction", i)
		}
	}

	// Verify all 3 are present
	for i := 0; i < 3; i++ {
		if _, ok := cache.Get(&hashes[i]); !ok {
			t.Errorf("entry %d should be in cache", i)
		}
	}

	// Add 4th entry (should evict oldest = hash[0])
	// Note: Get() on hash[0-2] above moves them to front, so hash[0] is now most recent
	// Let's reset and test eviction order properly

	cache.Clear()

	// Add in order: 0, 1, 2
	cache.Put(&hashes[0], auxPows[0])
	cache.Put(&hashes[1], auxPows[1])
	cache.Put(&hashes[2], auxPows[2])

	// Add 4th entry - should evict hash[0] (oldest)
	evicted := cache.Put(&hashes[3], auxPows[3])
	if !evicted {
		t.Error("adding 4th entry should cause eviction")
	}

	// hash[0] should be evicted
	if _, ok := cache.Get(&hashes[0]); ok {
		t.Error("hash[0] should have been evicted")
	}

	// hash[1], hash[2], hash[3] should still be present
	if _, ok := cache.Get(&hashes[1]); !ok {
		t.Error("hash[1] should still be in cache")
	}
	if _, ok := cache.Get(&hashes[2]); !ok {
		t.Error("hash[2] should still be in cache")
	}
	if _, ok := cache.Get(&hashes[3]); !ok {
		t.Error("hash[3] should still be in cache")
	}
}

func TestAuxPowLRUCache_LRUOrder(t *testing.T) {
	cache := newAuxPowLRUCache(3)

	// Create test hashes
	hashes := make([]chainhash.Hash, 4)
	auxPows := make([]*AuxPow, 4)
	for i := 0; i < 4; i++ {
		hashes[i] = chainhash.HashH([]byte{byte(i + 10)}) // Different seed
		auxPows[i] = &AuxPow{ParentBlock: wire.BlockHeader{Version: int32(i)}}
	}

	// Add 0, 1, 2
	cache.Put(&hashes[0], auxPows[0])
	cache.Put(&hashes[1], auxPows[1])
	cache.Put(&hashes[2], auxPows[2])

	// Access hash[0] to make it most recently used
	cache.Get(&hashes[0])

	// Add hash[3] - should evict hash[1] (now least recently used)
	cache.Put(&hashes[3], auxPows[3])

	// hash[1] should be evicted (LRU)
	if _, ok := cache.Get(&hashes[1]); ok {
		t.Error("hash[1] should have been evicted (was LRU)")
	}

	// hash[0] should still be present (was accessed, so not LRU)
	if _, ok := cache.Get(&hashes[0]); !ok {
		t.Error("hash[0] should still be in cache (was accessed recently)")
	}
}

func TestAuxPowLRUCache_Update(t *testing.T) {
	cache := newAuxPowLRUCache(3)

	hash := chainhash.HashH([]byte("test"))
	auxPow1 := &AuxPow{ParentBlock: wire.BlockHeader{Version: 1}}
	auxPow2 := &AuxPow{ParentBlock: wire.BlockHeader{Version: 2}}

	// Add entry
	cache.Put(&hash, auxPow1)
	got, _ := cache.Get(&hash)
	if got.ParentBlock.Version != 1 {
		t.Errorf("expected Version 1, got %d", got.ParentBlock.Version)
	}

	// Update entry
	evicted := cache.Put(&hash, auxPow2)
	if evicted {
		t.Error("update should not cause eviction")
	}
	if cache.Len() != 1 {
		t.Errorf("expected len 1 after update, got %d", cache.Len())
	}

	got, _ = cache.Get(&hash)
	if got.ParentBlock.Version != 2 {
		t.Errorf("expected Version 2 after update, got %d", got.ParentBlock.Version)
	}
}

func TestAuxPowLRUCache_DefaultCapacity(t *testing.T) {
	// Test with invalid capacities
	cache := newAuxPowLRUCache(0)
	if cache.capacity != DefaultAuxPowCacheSize {
		t.Errorf("expected default capacity %d for 0, got %d", DefaultAuxPowCacheSize, cache.capacity)
	}

	cache = newAuxPowLRUCache(-1)
	if cache.capacity != DefaultAuxPowCacheSize {
		t.Errorf("expected default capacity %d for -1, got %d", DefaultAuxPowCacheSize, cache.capacity)
	}

	cache = newAuxPowLRUCache(50)
	if cache.capacity != 50 {
		t.Errorf("expected capacity 50, got %d", cache.capacity)
	}
}

func TestAuxPowLRUCache_Concurrent(t *testing.T) {
	cache := newAuxPowLRUCache(100)

	// Create test data
	hashes := make([]chainhash.Hash, 200)
	auxPows := make([]*AuxPow, 200)
	for i := 0; i < 200; i++ {
		hashes[i] = chainhash.HashH([]byte{byte(i), byte(i >> 8)})
		auxPows[i] = &AuxPow{ParentBlock: wire.BlockHeader{Version: int32(i)}}
	}

	// Run concurrent operations
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				idx := (goroutineID*20 + j) % 200
				cache.Put(&hashes[idx], auxPows[idx])
				cache.Get(&hashes[idx])
				if j%5 == 0 {
					cache.Delete(&hashes[(idx+10)%200])
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify cache is still consistent
	if cache.Len() < 0 || cache.Len() > cache.capacity {
		t.Errorf("cache size out of bounds: %d (capacity: %d)", cache.Len(), cache.capacity)
	}
}

func BenchmarkAuxPowLRUCache_Put(b *testing.B) {
	cache := newAuxPowLRUCache(1000)
	hashes := make([]chainhash.Hash, b.N)
	auxPows := make([]*AuxPow, b.N)
	for i := 0; i < b.N; i++ {
		hashes[i] = chainhash.HashH([]byte{byte(i), byte(i >> 8), byte(i >> 16)})
		auxPows[i] = &AuxPow{ParentBlock: wire.BlockHeader{Version: int32(i)}}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Put(&hashes[i%len(hashes)], auxPows[i%len(auxPows)])
	}
}

func BenchmarkAuxPowLRUCache_Get(b *testing.B) {
	cache := newAuxPowLRUCache(1000)
	hashes := make([]chainhash.Hash, 1000)
	for i := 0; i < 1000; i++ {
		hashes[i] = chainhash.HashH([]byte{byte(i), byte(i >> 8)})
		cache.Put(&hashes[i], &AuxPow{ParentBlock: wire.BlockHeader{Version: int32(i)}})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(&hashes[i%1000])
	}
}
