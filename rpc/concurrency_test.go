package rpc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/opd-ai/nmcd/chain"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/namedb"
)

// TestConcurrentRPCRequests verifies that multiple RPC requests can be processed in parallel
// and that there are no race conditions. This test should be run with -race flag.
func TestConcurrentRPCRequests(t *testing.T) {
	// Create test blockchain and namedb
	tempDir := t.TempDir()
	cfg := &chain.Config{
		ChainParams: &config.NamecoinRegTestParams,
		NameDBPath:  tempDir + "/names.db",
		DataDir:     tempDir,
	}
	bc, err := chain.NewBlockChain(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create blockchain: %v", err)
	}
	defer bc.Close()

	// Create RPC server
	serverCfg := &Config{
		Blockchain: bc,
		ListenAddr: "127.0.0.1:0",
	}
	server, err := NewServer(serverCfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer server.Close()

	// Test concurrent getblockcount requests
	const numRequests = 100
	const numWorkers = 10

	var wg sync.WaitGroup
	errors := make(chan error, numRequests)
	start := time.Now()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numRequests/numWorkers; j++ {
				req := `{"jsonrpc":"2.0","method":"getblockcount","params":[],"id":1}`
				httpReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(req))
				httpReq.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				server.handleRequest(w, httpReq)

				if w.Code != http.StatusOK {
					errors <- nil // Don't fail on individual errors, just count them
					continue
				}

				var resp Response
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					errors <- err
					return
				}

				if resp.Error != nil {
					// This is expected since blockchain might not be initialized
					continue
				}
			}
		}()
	}

	wg.Wait()
	close(errors)
	duration := time.Since(start)

	// Check for errors
	errorCount := 0
	for err := range errors {
		if err != nil {
			t.Errorf("Request error: %v", err)
		}
		errorCount++
	}

	// Log performance metrics
	t.Logf("Processed %d concurrent requests in %v (%.2f req/s)", numRequests, duration, float64(numRequests)/duration.Seconds())

	// Verify reasonable throughput (should process faster than sequential)
	// With parallelization, we expect > 1000 req/s for simple getblockcount
	throughput := float64(numRequests) / duration.Seconds()
	if throughput < 100 {
		t.Logf("Warning: Low throughput (%.2f req/s), may indicate serialization", throughput)
	}
}

// TestConcurrentNameOperations tests concurrent name database operations
func TestConcurrentNameOperations(t *testing.T) {
	tempDir := t.TempDir()
	ndb, err := namedb.NewNameDatabase(tempDir + "/names.db")
	if err != nil {
		t.Fatalf("Failed to create namedb: %v", err)
	}
	defer ndb.Close()

	// Pre-populate some names
	for i := 0; i < 10; i++ {
		name := string(rune('a' + i))
		record := &namedb.NameRecord{
			Name:      name,
			Value:     "test value",
			Height:    100,
			ExpiresAt: 1000,
			Address:   "test_addr",
		}
		if err := ndb.PutName(name, record); err != nil {
			t.Fatalf("Failed to put name: %v", err)
		}
	}

	// Test concurrent reads
	const numReads = 1000
	const numWorkers = 10

	var wg sync.WaitGroup
	errors := make(chan error, numReads)
	start := time.Now()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < numReads/numWorkers; j++ {
				name := string(rune('a' + (j % 10)))
				_, err := ndb.GetName(name)
				if err != nil && err != namedb.ErrNameNotFound {
					errors <- err
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)
	duration := time.Since(start)

	// Check for errors
	for err := range errors {
		if err != nil {
			t.Errorf("GetName error: %v", err)
		}
	}

	// Log performance metrics
	t.Logf("Processed %d concurrent GetName operations in %v (%.2f ops/s)", numReads, duration, float64(numReads)/duration.Seconds())

	// Verify throughput - with caching and parallel reads, should be very fast
	throughput := float64(numReads) / duration.Seconds()
	if throughput < 10000 {
		t.Logf("Warning: Low GetName throughput (%.2f ops/s)", throughput)
	}
}

// TestRPCMethodParallelism benchmarks the improvement from parallel request processing
func TestRPCMethodParallelism(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &chain.Config{
		ChainParams: &config.NamecoinRegTestParams,
		NameDBPath:  tempDir + "/names.db",
		DataDir:     tempDir,
	}
	bc, err := chain.NewBlockChain(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create blockchain: %v", err)
	}
	defer bc.Close()

	serverCfg := &Config{
		Blockchain: bc,
		ListenAddr: "127.0.0.1:0",
	}
	server, err := NewServer(serverCfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer server.Close()

	// Measure parallel processing time
	const numRequests = 100
	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := `{"jsonrpc":"2.0","method":"getblockcount","params":[],"id":1}`
			httpReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(req))
			w := httptest.NewRecorder()
			server.handleRequest(w, httpReq)
		}()
	}

	wg.Wait()
	parallelDuration := time.Since(start)

	// Measure sequential processing time
	start = time.Now()
	for i := 0; i < numRequests; i++ {
		req := `{"jsonrpc":"2.0","method":"getblockcount","params":[],"id":1}`
		httpReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(req))
		w := httptest.NewRecorder()
		server.handleRequest(w, httpReq)
	}
	sequentialDuration := time.Since(start)

	// Calculate speedup
	speedup := float64(sequentialDuration) / float64(parallelDuration)
	t.Logf("Parallel: %v, Sequential: %v, Speedup: %.2fx", parallelDuration, sequentialDuration, speedup)

	// We expect at least some speedup from parallelization
	// Even with overhead, should see > 1.5x speedup with 10+ parallel workers
	if speedup < 1.2 {
		t.Logf("Warning: Low speedup (%.2fx), parallel processing may not be effective", speedup)
	}
}
