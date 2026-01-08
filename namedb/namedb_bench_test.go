package namedb

import (
	"fmt"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// setupBenchmarkDB creates a database with pre-populated test data for benchmarking.
func setupBenchmarkDB(b *testing.B) *NameDatabase {
	b.Helper()

	db, err := NewNameDatabase(b.TempDir() + "/bench.db")
	if err != nil {
		b.Fatalf("Failed to create database: %v", err)
	}

	// Pre-populate database with test names for realistic benchmarks
	populateBenchmarkData(b, db, 10000) // 10,000 names

	return db
}

// populateBenchmarkData adds N test names to the database.
func populateBenchmarkData(b *testing.B, db *NameDatabase, count int) {
	b.Helper()

	for i := 0; i < count; i++ {
		name := fmt.Sprintf("d/benchmark%d", i)
		value := fmt.Sprintf(`{"ip":"192.168.%d.%d"}`, i/256, i%256)

		txHashStr := fmt.Sprintf("%064x", i)
		txHash, err := chainhash.NewHashFromStr(txHashStr)
		if err != nil {
			b.Fatalf("Failed to create tx hash for benchmark data (i=%d, hash=%s): %v", i, txHashStr, err)
		}
		record := &NameRecord{
			Name:      name,
			Value:     value,
			TxHash:    *txHash,
			OutIndex:  0,
			Height:    int32(i),
			ExpiresAt: int32(i + 36000),
			Address:   fmt.Sprintf("N%dTestAddress", i),
			UpdatedAt: time.Now(),
		}

		if err := db.PutName(name, record); err != nil {
			b.Fatalf("Failed to populate test data: %v", err)
		}
	}
}

// BenchmarkGetName measures the performance of retrieving a single name.
// Target: < 1ms for cached lookups
func BenchmarkGetName(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Lookup existing names across the database
		name := fmt.Sprintf("d/benchmark%d", i%10000)
		_, err := db.GetName(name)
		if err != nil {
			b.Fatalf("GetName failed: %v", err)
		}
	}
}

// BenchmarkGetNameConcurrent measures concurrent read performance.
// Target: High throughput with minimal lock contention
func BenchmarkGetNameConcurrent(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			name := fmt.Sprintf("d/benchmark%d", i%10000)
			_, err := db.GetName(name)
			if err != nil {
				b.Fatalf("GetName failed: %v", err)
			}
			i++
		}
	})
}

// BenchmarkGetNameNotFound measures lookup performance for non-existent names.
// Target: Similar to existing names (early termination)
func BenchmarkGetNameNotFound(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("d/nonexistent%d", i)
		_, err := db.GetName(name)
		if err == nil {
			b.Fatal("Expected error for non-existent name")
		}
	}
}

// BenchmarkPutName measures the performance of writing a name record.
// Target: < 10ms for single write (includes disk I/O)
func BenchmarkPutName(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("d/newname%d", i)
		value := fmt.Sprintf(`{"ip":"10.0.0.%d"}`, i%256)

		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", 10000+i))
		record := &NameRecord{
			Name:      name,
			Value:     value,
			TxHash:    *txHash,
			OutIndex:  0,
			Height:    int32(10000 + i),
			ExpiresAt: int32(46000 + i),
			Address:   "NTestAddress",
			UpdatedAt: time.Now(),
		}

		if err := db.PutName(name, record); err != nil {
			b.Fatalf("PutName failed: %v", err)
		}
	}
}

// BenchmarkPutNameUpdate measures the performance of updating an existing name.
// Target: < 10ms for update (includes read + write)
func BenchmarkPutNameUpdate(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	// Use same name for all updates to simulate typical usage
	name := "d/updatetest"
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	record := &NameRecord{
		Name:      name,
		Value:     `{"ip":"1.2.3.4"}`,
		TxHash:    *txHash,
		OutIndex:  0,
		Height:    100,
		ExpiresAt: 36100,
		Address:   "NTestAddress",
		UpdatedAt: time.Now(),
	}
	db.PutName(name, record)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		record.Value = fmt.Sprintf(`{"ip":"10.0.0.%d"}`, i%256)
		record.Height = int32(100 + i)
		record.ExpiresAt = int32(36100 + i)
		record.UpdatedAt = time.Now()

		if err := db.PutName(name, record); err != nil {
			b.Fatalf("PutName failed: %v", err)
		}
	}
}

// BenchmarkListNames measures the performance of listing all names.
// Target: < 100ms for 10,000 names
func BenchmarkListNames(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		names, err := db.ListNames()
		if err != nil {
			b.Fatalf("ListNames failed: %v", err)
		}
		if len(names) != 10000 {
			b.Fatalf("Expected 10000 names, got %d", len(names))
		}
	}
}

// BenchmarkListNamesFiltered measures filtering performance.
// Target: < 50ms for filtered results
func BenchmarkListNamesFiltered(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// List names with specific prefix pattern
		names, err := db.ListNames()
		if err != nil {
			b.Fatalf("ListNames failed: %v", err)
		}

		// Filter in-memory (database doesn't support SQL-like queries)
		count := 0
		for _, record := range names {
			if len(record.Name) >= 12 && record.Name[:12] == "d/benchmark1" {
				count++
			}
		}

		// Should match d/benchmark1, d/benchmark10-19, d/benchmark100-199, etc.
		if count == 0 {
			b.Fatal("Expected matching names, got 0")
		}
	}
}

// BenchmarkDeleteName measures the performance of deleting a name.
// Target: < 5ms for single delete
func BenchmarkDeleteName(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	// Add extra names for deletion
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("d/todelete%d", i)
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", 20000+i))
		record := &NameRecord{
			Name:      name,
			Value:     `{"temp":"data"}`,
			TxHash:    *txHash,
			OutIndex:  0,
			Height:    100,
			ExpiresAt: 36100,
			Address:   "NTestAddress",
			UpdatedAt: time.Now(),
		}
		db.PutName(name, record)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("d/todelete%d", i)
		if err := db.DeleteName(name); err != nil {
			b.Fatalf("DeleteName failed: %v", err)
		}
	}
}

// BenchmarkGetExpiredNames measures expiration query performance.
// Target: < 50ms for scanning 10,000 names
func BenchmarkGetExpiredNames(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	// Set current height such that some names are expired
	currentHeight := int32(40000) // Names from 0-4000 will be expired

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		expired, err := db.GetExpiredNames(currentHeight)
		if err != nil {
			b.Fatalf("GetExpiredNames failed: %v", err)
		}

		// Should have ~4000 expired names
		if len(expired) < 3000 || len(expired) > 5000 {
			b.Fatalf("Expected ~4000 expired names, got %d", len(expired))
		}
	}
}

// BenchmarkAddHistory measures the performance of adding history entries.
// Target: < 5ms for single history entry
func BenchmarkAddHistory(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	name := "d/benchmark0"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", 20000+i))
		record := &NameRecord{
			Name:      name,
			Value:     fmt.Sprintf(`{"version":%d}`, i),
			TxHash:    *txHash,
			OutIndex:  0,
			Height:    int32(20000 + i),
			ExpiresAt: int32(56000 + i),
			Address:   "NTestAddress",
			UpdatedAt: time.Now(),
		}

		if err := db.AddHistory(*txHash, record); err != nil {
			b.Fatalf("AddHistory failed: %v", err)
		}
	}
}

// BenchmarkGetHistory measures the performance of retrieving name history.
// Target: < 10ms for retrieving full history
func BenchmarkGetHistory(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	// Add history entries for benchmark0
	name := "d/benchmark0"
	for i := 0; i < 100; i++ {
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", 20000+i))
		record := &NameRecord{
			Name:      name,
			Value:     fmt.Sprintf(`{"version":%d}`, i),
			TxHash:    *txHash,
			OutIndex:  0,
			Height:    int32(20000 + i),
			ExpiresAt: int32(56000 + i),
			Address:   "NTestAddress",
			UpdatedAt: time.Now(),
		}
		db.AddHistory(*txHash, record)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		history, err := db.GetHistory(name)
		if err != nil {
			b.Fatalf("GetHistory failed: %v", err)
		}
		if len(history) != 100 {
			b.Fatalf("Expected 100 history entries, got %d", len(history))
		}
	}
}

// BenchmarkUTXOOperations measures UTXO tracking performance.
// Target: < 5ms for UTXO add/get/delete
func BenchmarkUTXOOperations(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	b.Run("AddUTXO", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", 30000+i))
			utxo := &UTXO{
				TxHash:   *txHash,
				OutIndex: 0,
				Address:  "NTestAddress",
				Value:    100000000, // 1 NMC
			}
			if err := db.AddUTXO(utxo); err != nil {
				b.Fatalf("AddUTXO failed: %v", err)
			}
		}
	})

	// Add some UTXOs for GetUTXO test
	for i := 0; i < 100; i++ {
		txHashStr := fmt.Sprintf("%064x", 40000+i)
		txHash, err := chainhash.NewHashFromStr(txHashStr)
		if err != nil {
			b.Fatalf("Failed to create tx hash for UTXO setup (i=%d, hash=%s): %v", i, txHashStr, err)
		}
		utxo := &UTXO{
			TxHash:   *txHash,
			OutIndex: 0,
			Address:  "NTestAddress",
			Value:    100000000,
		}
		if err := db.AddUTXO(utxo); err != nil {
			b.Fatalf("Failed to add UTXO during setup (i=%d): %v", i, err)
		}
	}

	b.Run("GetUTXO", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", 40000+(i%100)))
			_, err := db.GetUTXO(txHash, 0)
			if err != nil {
				b.Fatalf("GetUTXO failed: %v", err)
			}
		}
	})

	b.Run("GetUTXOsForAddress", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			utxos, err := db.GetUTXOsForAddress("NTestAddress")
			if err != nil {
				b.Fatalf("GetUTXOsForAddress failed: %v", err)
			}
			if len(utxos) == 0 {
				b.Fatal("Expected UTXOs, got 0")
			}
		}
	})

	b.Run("RemoveUTXO", func(b *testing.B) {
		// Need to add UTXOs first
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			txHashStr := fmt.Sprintf("%064x", 50000+i)
			txHash, err := chainhash.NewHashFromStr(txHashStr)
			if err != nil {
				b.Fatalf("Failed to create tx hash for RemoveUTXO benchmark (i=%d, hash=%s): %v", i, txHashStr, err)
			}
			utxo := &UTXO{
				TxHash:   *txHash,
				OutIndex: 0,
				Address:  "NTestAddress",
				Value:    100000000,
			}
			if err := db.AddUTXO(utxo); err != nil {
				b.Fatalf("Failed to add UTXO for RemoveUTXO benchmark (i=%d): %v", i, err)
			}

			b.StartTimer()
			if err := db.RemoveUTXO(txHash, 0); err != nil {
				b.Fatalf("RemoveUTXO failed: %v", err)
			}
		}
	})
}

// BenchmarkMemoryUsage measures memory allocation patterns.
func BenchmarkMemoryUsage(b *testing.B) {
	b.ReportAllocs()

	db := setupBenchmarkDB(b)
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("d/benchmark%d", i%10000)
		_, _ = db.GetName(name)
	}
}

// BenchmarkConcurrentReadWrite measures performance under concurrent load.
// Target: Graceful degradation, no deadlocks
func BenchmarkConcurrentReadWrite(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// Mix of reads (90%) and writes (10%)
			if i%10 == 0 {
				// Write operation
				name := fmt.Sprintf("d/concurrent%d", i)
				txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", 60000+i))
				record := &NameRecord{
					Name:      name,
					Value:     `{"ip":"192.168.1.1"}`,
					TxHash:    *txHash,
					OutIndex:  0,
					Height:    int32(60000 + i),
					ExpiresAt: int32(96000 + i),
					Address:   "NTestAddress",
					UpdatedAt: time.Now(),
				}
				_ = db.PutName(name, record)
			} else {
				// Read operation
				name := fmt.Sprintf("d/benchmark%d", i%10000)
				_, _ = db.GetName(name)
			}
			i++
		}
	})
}

// BenchmarkEncodeNameRecord measures serialization performance with buffer pool.
// This benchmark validates the memory optimization from using sync.Pool.
func BenchmarkEncodeNameRecord(b *testing.B) {
	// Use a constant hash for benchmarking to avoid error handling overhead
	txHash, err := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	if err != nil {
		b.Fatalf("Failed to create test hash: %v", err)
	}
	record := &NameRecord{
		Name:          "d/benchmark",
		Value:         `{"ip":"192.168.1.1","ns":["ns1.example.com","ns2.example.com"]}`,
		TxHash:        *txHash,
		OutIndex:      0,
		Height:        100000,
		ExpiresAt:     136000,
		Address:       "NTestBenchmarkAddress123",
		UpdatedAt:     time.Now(),
		NameNewHeight: 99988,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data := encodeNameRecord(record)
		_ = data
	}
}

// BenchmarkDecodeNameRecord measures deserialization performance.
func BenchmarkDecodeNameRecord(b *testing.B) {
	txHash, err := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	if err != nil {
		b.Fatalf("Failed to create test hash: %v", err)
	}
	record := &NameRecord{
		Name:          "d/benchmark",
		Value:         `{"ip":"192.168.1.1","ns":["ns1.example.com","ns2.example.com"]}`,
		TxHash:        *txHash,
		OutIndex:      0,
		Height:        100000,
		ExpiresAt:     136000,
		Address:       "NTestBenchmarkAddress123",
		UpdatedAt:     time.Now(),
		NameNewHeight: 99988,
	}

	// Encode once
	data := encodeNameRecord(record)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decoded, err := decodeNameRecord(data)
		if err != nil {
			b.Fatalf("decodeNameRecord failed: %v", err)
		}
		_ = decoded
	}
}
