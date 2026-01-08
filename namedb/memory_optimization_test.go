package namedb

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// TestMemoryOptimization_EncodeAllocationReduction verifies that our buffer pool
// optimization reduces allocations during encoding.
func TestMemoryOptimization_EncodeAllocationReduction(t *testing.T) {
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	record := &NameRecord{
		Name:          "d/test",
		Value:         `{"ip":"192.168.1.1"}`,
		TxHash:        *txHash,
		OutIndex:      0,
		Height:        100,
		ExpiresAt:     36100,
		Address:       "NTestAddress",
		UpdatedAt:     time.Now(),
		NameNewHeight: 88,
	}

	// Warm up the buffer pool
	for i := 0; i < 100; i++ {
		_ = encodeNameRecord(record)
	}

	// Measure allocations
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	const iterations = 10000
	for i := 0; i < iterations; i++ {
		data := encodeNameRecord(record)
		_ = data
	}

	runtime.ReadMemStats(&m2)

	allocsPerOp := float64(m2.Mallocs-m1.Mallocs) / float64(iterations)
	bytesPerOp := float64(m2.TotalAlloc-m1.TotalAlloc) / float64(iterations)

	t.Logf("Memory usage per encode operation:")
	t.Logf("  Allocations: %.2f allocs/op", allocsPerOp)
	t.Logf("  Bytes: %.0f B/op", bytesPerOp)

	// With buffer pool, we expect significantly fewer allocations
	// Target: <= 2 allocs/op (one for the result slice, potentially one for pool management)
	if allocsPerOp > 3.0 {
		t.Errorf("Too many allocations per encode: %.2f > 3.0 (buffer pool not effective)", allocsPerOp)
	}
}

// TestMemoryOptimization_BulkOperations verifies memory usage during bulk operations.
func TestMemoryOptimization_BulkOperations(t *testing.T) {
	db, err := NewNameDatabase(t.TempDir() + "/bulk_test.db")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// Perform bulk writes
	const numRecords = 1000
	for i := 0; i < numRecords; i++ {
		txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
		record := &NameRecord{
			Name:          fmt.Sprintf("d/bulk%d", i),
			Value:         `{"ip":"192.168.1.1"}`,
			TxHash:        *txHash,
			OutIndex:      uint32(i),
			Height:        int32(i),
			ExpiresAt:     int32(36000 + i),
			Address:       "NTestAddress",
			UpdatedAt:     time.Now(),
		}
		if err := db.PutName(record.Name, record); err != nil {
			t.Fatalf("PutName failed: %v", err)
		}
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	bytesPerOp := float64(m2.TotalAlloc-m1.TotalAlloc) / float64(numRecords)
	t.Logf("Memory usage during bulk operations:")
	t.Logf("  Average per operation: %.0f B/op", bytesPerOp)
	t.Logf("  Total allocated: %d MB", (m2.TotalAlloc-m1.TotalAlloc)/1024/1024)

	// Verify cache is working (should be under capacity)
	cacheLen := db.cache.Len()
	t.Logf("  Cache entries: %d / %d", cacheLen, db.cache.capacity)
	if cacheLen > db.cache.capacity {
		t.Errorf("Cache exceeded capacity: %d > %d", cacheLen, db.cache.capacity)
	}
}

// BenchmarkMemoryOptimization_BeforeAfter compares memory usage of key operations.
// This benchmark helps track memory optimization progress over time.
func BenchmarkMemoryOptimization_BeforeAfter(b *testing.B) {
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")

	b.Run("Encode", func(b *testing.B) {
		record := &NameRecord{
			Name:          "d/benchmark",
			Value:         `{"ip":"192.168.1.1","ns":["ns1.example.com"]}`,
			TxHash:        *txHash,
			OutIndex:      0,
			Height:        100,
			ExpiresAt:     36100,
			Address:       "NTestAddress",
			UpdatedAt:     time.Now(),
			NameNewHeight: 88,
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			data := encodeNameRecord(record)
			_ = data
		}
	})

	b.Run("Decode", func(b *testing.B) {
		record := &NameRecord{
			Name:          "d/benchmark",
			Value:         `{"ip":"192.168.1.1","ns":["ns1.example.com"]}`,
			TxHash:        *txHash,
			OutIndex:      0,
			Height:        100,
			ExpiresAt:     36100,
			Address:       "NTestAddress",
			UpdatedAt:     time.Now(),
			NameNewHeight: 88,
		}
		data := encodeNameRecord(record)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			decoded, err := decodeNameRecord(data)
			if err != nil {
				b.Fatalf("Decode failed: %v", err)
			}
			_ = decoded
		}
	})

	b.Run("BufferPoolReuse", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := getBuffer()
			buf.Write([]byte("test data"))
			putBuffer(buf)
		}
	})
}
