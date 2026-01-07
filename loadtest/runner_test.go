package loadtest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestRPCClient validates the RPC client implementation
func TestRPCClient(t *testing.T) {
	// Create a test server
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)

		// Verify request format
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		// Send valid JSON-RPC response
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]interface{}{
				"blocks": 1000,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewRPCClient(server.URL, "", "")
	ctx := context.Background()

	result, err := client.Call(ctx, "getinfo", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	if atomic.LoadInt32(&requestCount) != 1 {
		t.Errorf("Expected 1 request, got %d", requestCount)
	}
}

// TestRPCClientError validates error handling
func TestRPCClientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"error": map[string]interface{}{
				"code":    -32600,
				"message": "Invalid request",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewRPCClient(server.URL, "", "")
	ctx := context.Background()

	_, err := client.Call(ctx, "invalid_method", nil)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	expected := "RPC error -32600: Invalid request"
	if err.Error() != expected {
		t.Errorf("Expected error %q, got %q", expected, err.Error())
	}
}

// TestRPCLoadTestBasic validates basic load testing functionality
func TestRPCLoadTestBasic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)

		// Simulate small processing delay
		time.Sleep(5 * time.Millisecond)

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]interface{}{
				"blocks": 1000,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := LoadTestConfig{
		RPCURL:      server.URL,
		Duration:    2 * time.Second,
		Concurrency: 5,
	}

	result, err := RPCLoadTest(config)
	if err != nil {
		t.Fatalf("RPCLoadTest failed: %v", err)
	}

	if result.RequestCount == 0 {
		t.Error("Expected requests to be made")
	}

	if result.SuccessCount == 0 {
		t.Error("Expected successful requests")
	}

	if result.ThroughputRPS <= 0 {
		t.Error("Expected positive throughput")
	}

	t.Logf("Load test completed: %d requests, %.2f req/s", result.RequestCount, result.ThroughputRPS)
}

// TestRPCLoadTestConcurrency validates concurrent request handling
func TestRPCLoadTestConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrency test in short mode")
	}

	var requestCount int64
	var maxConcurrent int32
	var currentConcurrent int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&currentConcurrent, 1)
		defer atomic.AddInt32(&currentConcurrent, -1)

		// Track max concurrent requests
		for {
			max := atomic.LoadInt32(&maxConcurrent)
			if current <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
				break
			}
		}

		atomic.AddInt64(&requestCount, 1)
		time.Sleep(10 * time.Millisecond)

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  map[string]interface{}{"blocks": 1000},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := LoadTestConfig{
		RPCURL:      server.URL,
		Duration:    1 * time.Second,
		Concurrency: 10,
	}

	result, err := RPCLoadTest(config)
	if err != nil {
		t.Fatalf("RPCLoadTest failed: %v", err)
	}

	if result.RequestCount == 0 {
		t.Error("Expected requests to be made")
	}

	maxConcurrentObserved := atomic.LoadInt32(&maxConcurrent)
	t.Logf("Max concurrent requests: %d (config: %d)", maxConcurrentObserved, config.Concurrency)

	// We should see some concurrency (at least 2 concurrent requests)
	if maxConcurrentObserved < 2 {
		t.Errorf("Expected concurrent requests, got max %d", maxConcurrentObserved)
	}
}

// TestMemoryLeakDetection validates memory leak detection
func TestMemoryLeakDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory test in short mode")
	}

	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)

		// Simulate memory info response
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]interface{}{
				"blocks":       1000,
				"memory_usage": 10 * 1024 * 1024, // 10 MB
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := LoadTestConfig{
		RPCURL:   server.URL,
		Duration: 1 * time.Second,
	}

	result, err := MemoryLeakTest(config, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("MemoryLeakTest failed: %v", err)
	}

	if result.RequestCount == 0 {
		t.Error("Expected requests to be made")
	}

	t.Logf("Memory leak test: %d requests, growth: %d bytes", result.RequestCount, result.MemoryGrowth)
}

// TestContinuousOperation validates sustained operation
func TestContinuousOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping continuous operation test in short mode")
	}

	var requestCount int64
	var failureSimulated bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt64(&requestCount, 1)

		// Simulate occasional failure
		if !failureSimulated && count == 10 {
			failureSimulated = true
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  map[string]interface{}{"blocks": 1000},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := ContinuousOperationConfig{
		RPCURL:        server.URL,
		Duration:      2 * time.Second,
		CheckInterval: 500 * time.Millisecond,
	}

	result, err := ContinuousOperationTest(config)
	if err != nil {
		t.Fatalf("ContinuousOperationTest failed: %v", err)
	}

	if result.RequestCount == 0 {
		t.Error("Expected requests to be made")
	}

	if result.SuccessCount == 0 {
		t.Error("Expected successful requests")
	}

	// Should have detected the simulated failure
	if result.FailureCount == 0 {
		t.Error("Expected at least one failure (simulated)")
	}

	t.Logf("Continuous operation: %d requests, %d success, %d failures",
		result.RequestCount, result.SuccessCount, result.FailureCount)
}

// TestPrintResult validates result formatting
func TestPrintResult(t *testing.T) {
	result := &TestResult{
		Name:          "Test Result",
		Duration:      10 * time.Second,
		RequestCount:  1000,
		SuccessCount:  990,
		FailureCount:  10,
		AvgLatency:    5 * time.Millisecond,
		P95Latency:    10 * time.Millisecond,
		P99Latency:    15 * time.Millisecond,
		ThroughputRPS: 100.0,
		MemoryStart:   10 * 1024 * 1024,
		MemoryEnd:     11 * 1024 * 1024,
		MemoryGrowth:  1 * 1024 * 1024,
		ErrorDetails:  []string{"error 1", "error 2"},
	}

	// PrintResult should not panic
	PrintResult(result)
}

// TestRateLimiting validates rate limiting functionality
func TestRateLimiting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping rate limiting test in short mode")
	}

	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  map[string]interface{}{"blocks": 1000},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Test with rate limit of 10 req/s
	config := LoadTestConfig{
		RPCURL:      server.URL,
		Duration:    1 * time.Second,
		Concurrency: 2,
		RateLimit:   10, // 10 requests per second
	}

	result, err := RPCLoadTest(config)
	if err != nil {
		t.Fatalf("RPCLoadTest failed: %v", err)
	}

	// Should be around 10 requests (with some tolerance)
	if result.RequestCount < 5 || result.RequestCount > 20 {
		t.Errorf("Expected ~10 requests with rate limit, got %d", result.RequestCount)
	}

	t.Logf("Rate limited test: %d requests in %v", result.RequestCount, result.Duration)
}

// BenchmarkRPCClient benchmarks the RPC client
func BenchmarkRPCClient(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  map[string]interface{}{"blocks": 1000},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewRPCClient(server.URL, "", "")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.Call(ctx, "getinfo", nil)
		if err != nil {
			b.Fatalf("Call failed: %v", err)
		}
	}
}

// Example demonstrating RPC load testing
func ExampleRPCLoadTest() {
	// Note: This example won't actually run without a real daemon
	// It's here for documentation purposes

	config := LoadTestConfig{
		RPCURL:      "http://localhost:8336",
		RPCUser:     "user",
		RPCPassword: "pass",
		Duration:    10 * time.Second,
		Concurrency: 50,
		RateLimit:   1000, // 1000 req/s
	}

	result, err := RPCLoadTest(config)
	if err != nil {
		fmt.Printf("Load test failed: %v\n", err)
		return
	}

	PrintResult(result)

	// Check if performance meets requirements
	if result.ThroughputRPS < 1000 {
		fmt.Printf("WARNING: Throughput %.2f req/s is below target 1000 req/s\n", result.ThroughputRPS)
	}
}
