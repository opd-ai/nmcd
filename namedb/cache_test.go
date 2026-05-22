package namedb

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// TestLRUCache_BasicOperations tests basic cache operations
func TestLRUCache_BasicOperations(t *testing.T) {
	cache := newLRUCache(3)

	// Test Put and Get
	record1 := &NameRecord{Name: "test1", Value: "value1", Height: 100}
	cache.Put("test1", record1)

	if val, ok := cache.Get("test1"); !ok || val.Value != "value1" {
		t.Errorf("Expected to get test1, got ok=%v, val=%v", ok, val)
	}

	// Test non-existent key
	if _, ok := cache.Get("nonexistent"); ok {
		t.Error("Expected false for non-existent key")
	}
}

// TestLRUCache_Eviction tests that LRU eviction works correctly
func TestLRUCache_Eviction(t *testing.T) {
	cache := newLRUCache(2) // Capacity of 2

	record1 := &NameRecord{Name: "test1", Value: "value1"}
	record2 := &NameRecord{Name: "test2", Value: "value2"}
	record3 := &NameRecord{Name: "test3", Value: "value3"}

	cache.Put("test1", record1)
	cache.Put("test2", record2)

	// Cache is now full: [test2, test1] (most recent first)

	// Access test1 to make it most recent
	cache.Get("test1")
	// Cache order: [test1, test2]

	// Add test3, should evict test2 (least recently used)
	evicted := cache.Put("test3", record3)
	if !evicted {
		t.Error("Expected eviction when cache is full")
	}

	// test2 should be evicted
	if _, ok := cache.Get("test2"); ok {
		t.Error("Expected test2 to be evicted")
	}

	// test1 and test3 should still be present
	if _, ok := cache.Get("test1"); !ok {
		t.Error("Expected test1 to still be in cache")
	}
	if _, ok := cache.Get("test3"); !ok {
		t.Error("Expected test3 to be in cache")
	}
}

// TestLRUCache_Update tests updating an existing entry
func TestLRUCache_Update(t *testing.T) {
	cache := newLRUCache(2)

	record1 := &NameRecord{Name: "test1", Value: "value1"}
	cache.Put("test1", record1)

	// Update with new value
	record1Updated := &NameRecord{Name: "test1", Value: "updated"}
	evicted := cache.Put("test1", record1Updated)

	if evicted {
		t.Error("Expected no eviction when updating existing entry")
	}

	val, ok := cache.Get("test1")
	if !ok || val.Value != "updated" {
		t.Errorf("Expected updated value, got ok=%v, val=%v", ok, val)
	}

	if cache.Len() != 1 {
		t.Errorf("Expected cache length 1, got %d", cache.Len())
	}
}

// TestLRUCache_Immutability tests that cached values are isolated from caller mutation.
func TestLRUCache_Immutability(t *testing.T) {
	cache := newLRUCache(2)

	original := &NameRecord{Name: "test1", Value: "value1", Height: 100}
	cache.Put("test1", original)

	original.Value = "mutated after put"

	cached, ok := cache.Get("test1")
	if !ok {
		t.Fatal("Expected cached record to exist")
	}
	if cached.Value != "value1" {
		t.Fatalf("Expected cached value to remain unchanged after Put mutation, got %q", cached.Value)
	}

	cached.Value = "mutated after get"

	cachedAgain, ok := cache.Get("test1")
	if !ok {
		t.Fatal("Expected cached record to still exist")
	}
	if cachedAgain.Value != "value1" {
		t.Fatalf("Expected cached value to remain unchanged after Get mutation, got %q", cachedAgain.Value)
	}
}

// TestLRUCache_Delete tests deleting entries
func TestLRUCache_Delete(t *testing.T) {
	cache := newLRUCache(3)

	record1 := &NameRecord{Name: "test1", Value: "value1"}
	cache.Put("test1", record1)

	// Delete existing entry
	if !cache.Delete("test1") {
		t.Error("Expected Delete to return true for existing entry")
	}

	// Verify it's gone
	if _, ok := cache.Get("test1"); ok {
		t.Error("Expected test1 to be deleted")
	}

	// Delete non-existent entry
	if cache.Delete("nonexistent") {
		t.Error("Expected Delete to return false for non-existent entry")
	}
}

// TestLRUCache_Clear tests clearing the cache
func TestLRUCache_Clear(t *testing.T) {
	cache := newLRUCache(3)

	record1 := &NameRecord{Name: "test1", Value: "value1"}
	record2 := &NameRecord{Name: "test2", Value: "value2"}
	cache.Put("test1", record1)
	cache.Put("test2", record2)

	if cache.Len() != 2 {
		t.Errorf("Expected cache length 2, got %d", cache.Len())
	}

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("Expected cache length 0 after clear, got %d", cache.Len())
	}

	if _, ok := cache.Get("test1"); ok {
		t.Error("Expected cache to be empty after clear")
	}
}

// TestLRUCache_Concurrent tests thread safety
func TestLRUCache_Concurrent(t *testing.T) {
	cache := newLRUCache(100)
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				record := &NameRecord{Name: key, Value: fmt.Sprintf("value-%d", j)}
				cache.Put(key, record)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j%10)
				cache.Get(key)
			}
		}(i)
	}

	wg.Wait()

	// Cache should not exceed capacity
	if cache.Len() > 100 {
		t.Errorf("Cache exceeded capacity: %d > 100", cache.Len())
	}
}

// TestLRUCache_DefaultCapacity tests default capacity when invalid value provided
func TestLRUCache_DefaultCapacity(t *testing.T) {
	cache := newLRUCache(0)
	if cache.capacity != 10000 {
		t.Errorf("Expected default capacity 10000, got %d", cache.capacity)
	}

	cache = newLRUCache(-1)
	if cache.capacity != 10000 {
		t.Errorf("Expected default capacity 10000, got %d", cache.capacity)
	}
}

// BenchmarkCache_Get measures cache hit performance
func BenchmarkCache_Get(b *testing.B) {
	cache := newLRUCache(10000)

	// Pre-populate cache
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("d/benchmark%d", i)
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", i))
		record := &NameRecord{
			Name:      key,
			Value:     fmt.Sprintf(`{"ip":"192.168.%d.%d"}`, i/256, i%256),
			TxHash:    *txHash,
			Height:    int32(i),
			ExpiresAt: int32(i + 36000),
			UpdatedAt: time.Now(),
		}
		cache.Put(key, record)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("d/benchmark%d", i%10000)
		cache.Get(key)
	}
}

// BenchmarkCache_Put measures cache write performance
func BenchmarkCache_Put(b *testing.B) {
	cache := newLRUCache(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("d/benchmark%d", i)
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", i))
		record := &NameRecord{
			Name:      key,
			Value:     fmt.Sprintf(`{"ip":"192.168.%d.%d"}`, i/256, i%256),
			TxHash:    *txHash,
			Height:    int32(i),
			ExpiresAt: int32(i + 36000),
			UpdatedAt: time.Now(),
		}
		cache.Put(key, record)
	}
}

// BenchmarkCache_Concurrent measures concurrent access performance
func BenchmarkCache_Concurrent(b *testing.B) {
	cache := newLRUCache(10000)

	// Pre-populate
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("d/benchmark%d", i)
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", i))
		record := &NameRecord{
			Name:      key,
			Value:     fmt.Sprintf(`{"ip":"192.168.%d.%d"}`, i/256, i%256),
			TxHash:    *txHash,
			Height:    int32(i),
			ExpiresAt: int32(i + 36000),
			UpdatedAt: time.Now(),
		}
		cache.Put(key, record)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("d/benchmark%d", i%10000)
			cache.Get(key)
			i++
		}
	})
}
