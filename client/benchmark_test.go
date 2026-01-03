package client

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/opd-ai/nmcd/namedb"
)

// setupBenchmarkClient creates a client with pre-populated test data for benchmarking.
func setupBenchmarkClient(b *testing.B) *EmbeddedClient {
	b.Helper()

	cfg := &Config{
		DataDir: b.TempDir(),
		Network: "regtest",
	}

	client, err := NewEmbeddedClient(cfg)
	if err != nil {
		b.Fatalf("Failed to create client: %v", err)
	}

	// Pre-populate database with test names for realistic benchmarks
	populateTestData(b, client, 1000) // 1000 names

	return client
}

// populateTestData adds N test names to the database
func populateTestData(b *testing.B, client *EmbeddedClient, count int) {
	b.Helper()

	for i := 0; i < count; i++ {
		name := fmt.Sprintf("d/benchmark%d", i)
		value := fmt.Sprintf(`{"ip":"192.168.%d.%d"}`, i/256, i%256)

		// Create a name record
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", i))
		record := &namedb.NameRecord{
			Name:      name,
			Value:     value,
			TxHash:    *txHash,
			OutIndex:  0,
			Height:    int32(i),
			ExpiresAt: int32(i + 36000),
			Address:   fmt.Sprintf("N%dTestAddress", i),
			UpdatedAt: time.Now(),
		}

		if err := client.nameDB.PutName(name, record); err != nil {
			b.Fatalf("Failed to populate test data: %v", err)
		}
	}
}

// BenchmarkResolveName measures the performance of name resolution from local database.
func BenchmarkResolveName(b *testing.B) {
	client := setupBenchmarkClient(b)
	defer client.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Benchmark lookup of existing names (worst case: end of database)
		name := fmt.Sprintf("d/benchmark%d", i%1000)
		_, err := client.ResolveName(ctx, name)
		if err != nil {
			b.Fatalf("ResolveName failed: %v", err)
		}
	}
}

// BenchmarkResolveNameConcurrent measures concurrent name resolution performance.
func BenchmarkResolveNameConcurrent(b *testing.B) {
	client := setupBenchmarkClient(b)
	defer client.Close()

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			name := fmt.Sprintf("d/benchmark%d", i%1000)
			_, err := client.ResolveName(ctx, name)
			if err != nil {
				b.Fatalf("ResolveName failed: %v", err)
			}
			i++
		}
	})
}

// BenchmarkResolveNameNotFound measures lookup performance for non-existent names.
func BenchmarkResolveNameNotFound(b *testing.B) {
	client := setupBenchmarkClient(b)
	defer client.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Lookup names that don't exist
		name := fmt.Sprintf("d/nonexistent%d", i)
		_, err := client.ResolveName(ctx, name)
		if err != ErrNameNotFound {
			b.Fatalf("Expected ErrNameNotFound, got: %v", err)
		}
	}
}

// BenchmarkListNames measures the performance of listing names without filters.
func BenchmarkListNames(b *testing.B) {
	client := setupBenchmarkClient(b)
	defer client.Close()

	ctx := context.Background()
	filter := &ListFilter{
		Limit: 100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		names, err := client.ListNames(ctx, filter)
		if err != nil {
			b.Fatalf("ListNames failed: %v", err)
		}
		if len(names) == 0 {
			b.Fatal("Expected names, got empty list")
		}
	}
}

// BenchmarkListNamesWithNamespace measures listing with namespace filter.
func BenchmarkListNamesWithNamespace(b *testing.B) {
	client := setupBenchmarkClient(b)
	defer client.Close()

	ctx := context.Background()
	filter := &ListFilter{
		Namespace: "d/",
		Limit:     100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		names, err := client.ListNames(ctx, filter)
		if err != nil {
			b.Fatalf("ListNames failed: %v", err)
		}
		if len(names) == 0 {
			b.Fatal("Expected names, got empty list")
		}
	}
}

// BenchmarkListNamesWithPattern measures listing with name pattern filter.
func BenchmarkListNamesWithPattern(b *testing.B) {
	client := setupBenchmarkClient(b)
	defer client.Close()

	ctx := context.Background()
	filter := &ListFilter{
		NamePattern: "d/benchmark1",
		Limit:       100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		names, err := client.ListNames(ctx, filter)
		if err != nil {
			b.Fatalf("ListNames failed: %v", err)
		}
		// Should match d/benchmark1, d/benchmark10, d/benchmark11, etc.
		if len(names) == 0 {
			b.Fatal("Expected names, got empty list")
		}
	}
}

// BenchmarkListNamesLargeResult measures listing with large result sets.
func BenchmarkListNamesLargeResult(b *testing.B) {
	client := setupBenchmarkClient(b)
	defer client.Close()

	ctx := context.Background()
	filter := &ListFilter{
		Limit: 1000, // All 1000 names
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		names, err := client.ListNames(ctx, filter)
		if err != nil {
			b.Fatalf("ListNames failed: %v", err)
		}
		if len(names) != 1000 {
			b.Fatalf("Expected 1000 names, got %d", len(names))
		}
	}
}

// BenchmarkGetNameHistory measures the performance of retrieving name history.
func BenchmarkGetNameHistory(b *testing.B) {
	client := setupBenchmarkClient(b)
	defer client.Close()

	ctx := context.Background()

	// Add some history entries for benchmark0
	for i := 0; i < 10; i++ {
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", 1000+i))
		record := &namedb.NameRecord{
			Name:      "d/benchmark0",
			Value:     fmt.Sprintf(`{"version":%d}`, i),
			TxHash:    *txHash,
			OutIndex:  0,
			Height:    int32(1000 + i),
			ExpiresAt: int32(37000 + i),
			Address:   "NTestAddress",
			UpdatedAt: time.Now(),
		}
		if err := client.nameDB.AddHistory(*txHash, record); err != nil {
			b.Fatalf("Failed to add history: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		history, err := client.GetNameHistory(ctx, "d/benchmark0")
		if err != nil {
			b.Fatalf("GetNameHistory failed: %v", err)
		}
		if len(history) == 0 {
			b.Fatal("Expected history entries, got empty list")
		}
	}
}

// BenchmarkGetInfo measures the performance of retrieving node information.
func BenchmarkGetInfo(b *testing.B) {
	client := setupBenchmarkClient(b)
	defer client.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		info, err := client.GetInfo(ctx)
		if err != nil {
			b.Fatalf("GetInfo failed: %v", err)
		}
		if info == nil {
			b.Fatal("Expected info, got nil")
		}
	}
}

// BenchmarkRegisterName measures the performance of creating a NAME_NEW transaction.
// Note: This only benchmarks transaction creation, not network broadcast.
func BenchmarkRegisterName(b *testing.B) {
	client := setupBenchmarkClient(b)
	defer client.Close()

	ctx := context.Background()

	// Generate a wallet address for the benchmark
	addrStr, err := client.wallet.GenerateKey()
	if err != nil {
		b.Fatalf("Failed to generate address: %v", err)
	}

	// Add a UTXO for the address
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	utxo := &namedb.UTXO{
		TxHash:   *txHash,
		OutIndex: 0,
		Address:  addrStr,
		Value:    100000000, // 1 NMC
	}
	if err := client.nameDB.AddUTXO(utxo); err != nil {
		b.Fatalf("Failed to add UTXO: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("d/newname%d", i)
		value := fmt.Sprintf(`{"ip":"10.0.0.%d"}`, i%256)

		_, err := client.RegisterName(ctx, name, value, &RegisterOpts{
			FromAddress:         addrStr,
			WaitForConfirmation: false,
		})
		if err != nil {
			b.Fatalf("RegisterName failed: %v", err)
		}
	}
}

// BenchmarkUpdateName measures the performance of creating a NAME_UPDATE transaction.
// Note: This only benchmarks transaction creation, not network broadcast.
func BenchmarkUpdateName(b *testing.B) {
	client := setupBenchmarkClient(b)
	defer client.Close()

	ctx := context.Background()

	// Generate a wallet address
	addrStr, err := client.wallet.GenerateKey()
	if err != nil {
		b.Fatalf("Failed to generate address: %v", err)
	}

	// Create a name that we can update
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	record := &namedb.NameRecord{
		Name:      "d/updatetest",
		Value:     `{"ip":"1.2.3.4"}`,
		TxHash:    *txHash,
		OutIndex:  0,
		Height:    100,
		ExpiresAt: 36100,
		Address:   addrStr,
		UpdatedAt: time.Now(),
	}
	if err := client.nameDB.PutName("d/updatetest", record); err != nil {
		b.Fatalf("Failed to create test name: %v", err)
	}

	// Add a UTXO for the name
	utxo := &namedb.UTXO{
		TxHash:   *txHash,
		OutIndex: 0,
		Address:  addrStr,
		Value:    100000000, // 1 NMC
	}
	if err := client.nameDB.AddUTXO(utxo); err != nil {
		b.Fatalf("Failed to add UTXO: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value := fmt.Sprintf(`{"ip":"10.0.0.%d"}`, i%256)

		_, err := client.UpdateName(ctx, "d/updatetest", value, &UpdateOpts{
			WaitForConfirmation: false,
		})
		if err != nil {
			b.Fatalf("UpdateName failed: %v", err)
		}
	}
}

// BenchmarkClientMemoryUsage measures memory allocation patterns.
func BenchmarkClientMemoryUsage(b *testing.B) {
	b.ReportAllocs()

	client := setupBenchmarkClient(b)
	defer client.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("d/benchmark%d", i%1000)
		_, _ = client.ResolveName(ctx, name)
	}
}
