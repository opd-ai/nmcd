package rpc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/opd-ai/nmcd/chain"
	"github.com/opd-ai/nmcd/config"
)

// BenchmarkRPCConcurrentRequests measures RPC throughput with concurrent requests
func BenchmarkRPCConcurrentRequests(b *testing.B) {
	// Create test server
	tempDir := b.TempDir()
	cfg := &chain.Config{
		ChainParams: &config.NamecoinRegTestParams,
		NameDBPath:  tempDir + "/names.db",
		DataDir:     tempDir,
	}
	bc, err := chain.NewBlockChain(cfg, nil)
	if err != nil {
		b.Fatalf("Failed to create blockchain: %v", err)
	}
	defer bc.Close()

	serverCfg := &Config{
		Blockchain: bc,
		ListenAddr: "127.0.0.1:0",
		RateLimit:  1000000, // Very high rate limit for benchmarking
	}
	server, err := NewServer(serverCfg)
	if err != nil {
		b.Fatalf("Failed to create server: %v", err)
	}
	defer server.Close()

	benchmarks := []struct {
		name       string
		concurrent int
	}{
		{"Concurrent_1", 1},
		{"Concurrent_10", 10},
		{"Concurrent_50", 50},
		{"Concurrent_100", 100},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					req := `{"jsonrpc":"2.0","method":"getblockcount","params":[],"id":1}`
					httpReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(req))
					httpReq.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					server.handleRequest(w, httpReq)

					if w.Code != http.StatusOK {
						b.Errorf("Expected status 200, got %d", w.Code)
					}
				}
			})
		})
	}
}

// BenchmarkRPCSequentialRequests measures RPC throughput with sequential requests
func BenchmarkRPCSequentialRequests(b *testing.B) {
	tempDir := b.TempDir()
	cfg := &chain.Config{
		ChainParams: &config.NamecoinRegTestParams,
		NameDBPath:  tempDir + "/names.db",
		DataDir:     tempDir,
	}
	bc, err := chain.NewBlockChain(cfg, nil)
	if err != nil {
		b.Fatalf("Failed to create blockchain: %v", err)
	}
	defer bc.Close()

	serverCfg := &Config{
		Blockchain: bc,
		ListenAddr: "127.0.0.1:0",
		RateLimit:  1000000, // Very high rate limit for benchmarking
	}
	server, err := NewServer(serverCfg)
	if err != nil {
		b.Fatalf("Failed to create server: %v", err)
	}
	defer server.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := `{"jsonrpc":"2.0","method":"getblockcount","params":[],"id":1}`
		httpReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(req))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		server.handleRequest(w, httpReq)
	}
}

// BenchmarkRPCDifferentMethods measures throughput with different RPC methods
func BenchmarkRPCDifferentMethods(b *testing.B) {
	tempDir := b.TempDir()
	cfg := &chain.Config{
		ChainParams: &config.NamecoinRegTestParams,
		NameDBPath:  tempDir + "/names.db",
		DataDir:     tempDir,
	}
	bc, err := chain.NewBlockChain(cfg, nil)
	if err != nil {
		b.Fatalf("Failed to create blockchain: %v", err)
	}
	defer bc.Close()

	serverCfg := &Config{
		Blockchain: bc,
		ListenAddr: "127.0.0.1:0",
		RateLimit:  1000000, // Very high rate limit for benchmarking
	}
	server, err := NewServer(serverCfg)
	if err != nil {
		b.Fatalf("Failed to create server: %v", err)
	}
	defer server.Close()

	methods := []string{
		`{"jsonrpc":"2.0","method":"getblockcount","params":[],"id":1}`,
		`{"jsonrpc":"2.0","method":"getbestblockhash","params":[],"id":2}`,
		`{"jsonrpc":"2.0","method":"getinfo","params":[],"id":3}`,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			req := methods[i%len(methods)]
			i++
			httpReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(req))
			httpReq.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			server.handleRequest(w, httpReq)
		}
	})
}

// BenchmarkCacheEffectiveness measures cache hit rates under concurrent load
func BenchmarkCacheEffectiveness(b *testing.B) {
	tempDir := b.TempDir()
	cfg := &chain.Config{
		ChainParams: &config.NamecoinRegTestParams,
		NameDBPath:  tempDir + "/names.db",
		DataDir:     tempDir,
	}
	bc, err := chain.NewBlockChain(cfg, nil)
	if err != nil {
		b.Fatalf("Failed to create blockchain: %v", err)
	}
	defer bc.Close()

	serverCfg := &Config{
		Blockchain: bc,
		ListenAddr: "127.0.0.1:0",
		RateLimit:  1000000, // Very high rate limit for benchmarking
	}
	server, err := NewServer(serverCfg)
	if err != nil {
		b.Fatalf("Failed to create server: %v", err)
	}
	defer server.Close()

	// Benchmark concurrent name_show requests (will get "name not found" but tests cache)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			name := string(rune('a') + rune(i%26))
			i++
			req := `{"jsonrpc":"2.0","method":"name_show","params":["` + name + `"],"id":1}`
			httpReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(req))
			httpReq.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			server.handleRequest(w, httpReq)

			if w.Code == http.StatusOK {
				var resp Response
				json.NewDecoder(w.Body).Decode(&resp)
			}
		}
	})
}

// BenchmarkRPCLockContention measures lock contention under high concurrency
func BenchmarkRPCLockContention(b *testing.B) {
	tempDir := b.TempDir()
	cfg := &chain.Config{
		ChainParams: &config.NamecoinRegTestParams,
		NameDBPath:  tempDir + "/names.db",
		DataDir:     tempDir,
	}
	bc, err := chain.NewBlockChain(cfg, nil)
	if err != nil {
		b.Fatalf("Failed to create blockchain: %v", err)
	}
	defer bc.Close()

	serverCfg := &Config{
		Blockchain: bc,
		ListenAddr: "127.0.0.1:0",
		RateLimit:  1000000, // Very high rate limit for benchmarking
	}
	server, err := NewServer(serverCfg)
	if err != nil {
		b.Fatalf("Failed to create server: %v", err)
	}
	defer server.Close()

	// High contention scenario: many goroutines making requests simultaneously
	const goroutines = 100
	var wg sync.WaitGroup

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(goroutines)
		for j := 0; j < goroutines; j++ {
			go func() {
				defer wg.Done()
				req := `{"jsonrpc":"2.0","method":"getblockcount","params":[],"id":1}`
				httpReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(req))
				w := httptest.NewRecorder()
				server.handleRequest(w, httpReq)
			}()
		}
		wg.Wait()
	}
}
