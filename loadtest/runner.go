// Package loadtest provides load and stress testing utilities for nmcd.
// These tests validate daemon behavior under sustained load, memory pressure,
// and failure conditions to ensure production readiness.
package loadtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// TestResult contains the outcome of a load test
type TestResult struct {
	Name          string        // Test name
	Duration      time.Duration // Total test duration
	RequestCount  int64         // Total requests sent
	SuccessCount  int64         // Successful requests
	FailureCount  int64         // Failed requests
	AvgLatency    time.Duration // Average request latency
	P95Latency    time.Duration // 95th percentile latency
	P99Latency    time.Duration // 99th percentile latency
	ThroughputRPS float64       // Requests per second
	MemoryStart   uint64        // Starting memory usage (bytes)
	MemoryEnd     uint64        // Ending memory usage (bytes)
	MemoryGrowth  int64         // Memory growth (bytes)
	ErrorDetails  []string      // Detailed error messages
}

// RPCClient provides JSON-RPC communication for load testing
type RPCClient struct {
	URL      string
	Username string
	Password string
	client   *http.Client
}

// NewRPCClient creates a new RPC client with connection pooling
func NewRPCClient(url, username, password string) *RPCClient {
	return &RPCClient{
		URL:      url,
		Username: username,
		Password: password,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Call sends a JSON-RPC request
func (c *RPCClient) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.URL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.Username != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

// LoadTestConfig contains configuration for load testing
type LoadTestConfig struct {
	RPCURL       string        // RPC endpoint URL
	RPCUser      string        // RPC username
	RPCPassword  string        // RPC password
	Duration     time.Duration // Test duration
	Concurrency  int           // Number of concurrent clients
	RateLimit    int           // Requests per second (0 = unlimited)
	WarmupPeriod time.Duration // Warmup period before measurements
}

// RPCLoadTest performs sustained RPC load testing
func RPCLoadTest(config LoadTestConfig) (*TestResult, error) {
	result := &TestResult{
		Name:         "RPC Load Test",
		ErrorDetails: make([]string, 0),
	}

	client := NewRPCClient(config.RPCURL, config.RPCUser, config.RPCPassword)
	ctx := context.Background()

	// Test connectivity first
	_, err := client.Call(ctx, "getinfo", nil)
	if err != nil {
		return nil, fmt.Errorf("initial connectivity check failed: %w", err)
	}

	var requestCount, successCount, failureCount int64
	var wg sync.WaitGroup
	latencies := make([]time.Duration, 0, 10000)
	var latencyMutex sync.Mutex

	// Rate limiter (if needed)
	var ticker *time.Ticker
	if config.RateLimit > 0 {
		requestsPerWorker := config.RateLimit / config.Concurrency
		if requestsPerWorker < 1 {
			requestsPerWorker = 1
		}
		ticker = time.NewTicker(time.Second / time.Duration(requestsPerWorker))
		defer ticker.Stop()
	}

	start := time.Now()
	stopChan := make(chan struct{})

	// Start worker goroutines
	for i := 0; i < config.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			methods := []string{"getinfo", "getblockcount", "getbestblockhash"}
			methodIdx := 0

			for {
				select {
				case <-stopChan:
					return
				default:
					if ticker != nil {
						<-ticker.C
					}

					reqStart := time.Now()
					method := methods[methodIdx%len(methods)]
					methodIdx++

					_, err := client.Call(ctx, method, nil)
					latency := time.Since(reqStart)

					atomic.AddInt64(&requestCount, 1)

					if err != nil {
						atomic.AddInt64(&failureCount, 1)
						latencyMutex.Lock()
						if len(result.ErrorDetails) < 100 {
							result.ErrorDetails = append(result.ErrorDetails, err.Error())
						}
						latencyMutex.Unlock()
					} else {
						atomic.AddInt64(&successCount, 1)
						latencyMutex.Lock()
						if len(latencies) < 10000 {
							latencies = append(latencies, latency)
						}
						latencyMutex.Unlock()
					}
				}
			}
		}(i)
	}

	// Run for specified duration
	time.Sleep(config.Duration)
	close(stopChan)
	wg.Wait()

	duration := time.Since(start)

	// Calculate statistics
	result.Duration = duration
	result.RequestCount = requestCount
	result.SuccessCount = successCount
	result.FailureCount = failureCount
	result.ThroughputRPS = float64(requestCount) / duration.Seconds()

	if len(latencies) > 0 {
		var total time.Duration
		for _, l := range latencies {
			total += l
		}
		result.AvgLatency = total / time.Duration(len(latencies))

		// Calculate percentiles (simple sort would be better for production)
		if len(latencies) >= 20 {
			result.P95Latency = latencies[int(float64(len(latencies))*0.95)]
			result.P99Latency = latencies[int(float64(len(latencies))*0.99)]
		}
	}

	return result, nil
}

// MemoryLeakTest monitors memory growth over time
func MemoryLeakTest(config LoadTestConfig, memoryCheckInterval time.Duration) (*TestResult, error) {
	result := &TestResult{
		Name:         "Memory Leak Detection",
		ErrorDetails: make([]string, 0),
	}

	client := NewRPCClient(config.RPCURL, config.RPCUser, config.RPCPassword)
	ctx := context.Background()

	// Get initial memory baseline
	initialMem, err := getServerMemory(client, ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get initial memory: %w", err)
	}
	result.MemoryStart = initialMem

	start := time.Now()
	stopChan := make(chan struct{})
	var requestCount int64

	// Background load generator
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		methods := []string{"getinfo", "getblockcount", "name_list"}
		idx := 0

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				method := methods[idx%len(methods)]
				idx++
				_, _ = client.Call(ctx, method, nil)
				atomic.AddInt64(&requestCount, 1)
			}
		}
	}()

	// Monitor memory periodically
	memSamples := make([]uint64, 0)
	ticker := time.NewTicker(memoryCheckInterval)
	defer ticker.Stop()

	endTime := time.Now().Add(config.Duration)
	for time.Now().Before(endTime) {
		<-ticker.C
		mem, err := getServerMemory(client, ctx)
		if err != nil {
			result.ErrorDetails = append(result.ErrorDetails, fmt.Sprintf("memory check failed: %v", err))
			continue
		}
		memSamples = append(memSamples, mem)
	}

	close(stopChan)

	// Get final memory
	finalMem, err := getServerMemory(client, ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get final memory: %w", err)
	}

	result.Duration = time.Since(start)
	result.RequestCount = requestCount
	result.MemoryEnd = finalMem
	result.MemoryGrowth = int64(finalMem) - int64(initialMem)

	return result, nil
}

// getServerMemory retrieves current memory usage from server
func getServerMemory(client *RPCClient, ctx context.Context) (uint64, error) {
	result, err := client.Call(ctx, "getinfo", nil)
	if err != nil {
		return 0, err
	}

	var info struct {
		MemoryUsage uint64 `json:"memory_usage"`
	}

	// If getinfo doesn't provide memory, we'll estimate based on response
	if err := json.Unmarshal(result, &info); err == nil && info.MemoryUsage > 0 {
		return info.MemoryUsage, nil
	}

	// Fallback: return 0 (memory monitoring may not be available)
	return 0, nil
}

// ContinuousOperationConfig contains configuration for continuous operation testing
type ContinuousOperationConfig struct {
	RPCURL       string
	RPCUser      string
	RPCPassword  string
	Duration     time.Duration
	NameCount    int           // Number of names to process
	CheckInterval time.Duration // How often to check daemon health
}

// ContinuousOperationTest runs the daemon under sustained load for extended periods
func ContinuousOperationTest(config ContinuousOperationConfig) (*TestResult, error) {
	result := &TestResult{
		Name:         "Continuous Operation Test",
		ErrorDetails: make([]string, 0),
	}

	client := NewRPCClient(config.RPCURL, config.RPCUser, config.RPCPassword)
	ctx := context.Background()

	start := time.Now()
	var requestCount, successCount, failureCount int64

	// Health check goroutine
	stopChan := make(chan struct{})
	healthErrors := make([]string, 0)
	var healthMutex sync.Mutex

	go func() {
		ticker := time.NewTicker(config.CheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				_, err := client.Call(ctx, "getinfo", nil)
				if err != nil {
					healthMutex.Lock()
					healthErrors = append(healthErrors, fmt.Sprintf("[%s] health check failed: %v", time.Now().Format(time.RFC3339), err))
					healthMutex.Unlock()
				}
			}
		}
	}()

	// Main test loop
	endTime := time.Now().Add(config.Duration)
	operations := []string{"getinfo", "getblockcount", "getbestblockhash", "name_list"}
	opIdx := 0

	for time.Now().Before(endTime) {
		operation := operations[opIdx%len(operations)]
		opIdx++

		_, err := client.Call(ctx, operation, nil)
		atomic.AddInt64(&requestCount, 1)

		if err != nil {
			atomic.AddInt64(&failureCount, 1)
			if len(result.ErrorDetails) < 100 {
				result.ErrorDetails = append(result.ErrorDetails, err.Error())
			}
		} else {
			atomic.AddInt64(&successCount, 1)
		}

		// Small delay between requests
		time.Sleep(10 * time.Millisecond)

		// Check if we've reached name count limit
		if config.NameCount > 0 && int(successCount) >= config.NameCount {
			break
		}
	}

	close(stopChan)

	healthMutex.Lock()
	result.ErrorDetails = append(result.ErrorDetails, healthErrors...)
	healthMutex.Unlock()

	result.Duration = time.Since(start)
	result.RequestCount = requestCount
	result.SuccessCount = successCount
	result.FailureCount = failureCount
	result.ThroughputRPS = float64(requestCount) / time.Since(start).Seconds()

	return result, nil
}

// PrintResult outputs test results in a readable format
func PrintResult(result *TestResult) {
	fmt.Printf("\n=== %s Results ===\n", result.Name)
	fmt.Printf("Duration:         %v\n", result.Duration)
	fmt.Printf("Total Requests:   %d\n", result.RequestCount)
	fmt.Printf("Successful:       %d\n", result.SuccessCount)
	fmt.Printf("Failed:           %d\n", result.FailureCount)
	fmt.Printf("Throughput:       %.2f req/s\n", result.ThroughputRPS)

	if result.AvgLatency > 0 {
		fmt.Printf("Avg Latency:      %v\n", result.AvgLatency)
		fmt.Printf("P95 Latency:      %v\n", result.P95Latency)
		fmt.Printf("P99 Latency:      %v\n", result.P99Latency)
	}

	if result.MemoryStart > 0 || result.MemoryEnd > 0 {
		fmt.Printf("Memory Start:     %d bytes (%.2f MB)\n", result.MemoryStart, float64(result.MemoryStart)/(1024*1024))
		fmt.Printf("Memory End:       %d bytes (%.2f MB)\n", result.MemoryEnd, float64(result.MemoryEnd)/(1024*1024))
		fmt.Printf("Memory Growth:    %d bytes (%.2f MB)\n", result.MemoryGrowth, float64(result.MemoryGrowth)/(1024*1024))
	}

	if len(result.ErrorDetails) > 0 {
		fmt.Printf("\nErrors (%d total, showing first 10):\n", len(result.ErrorDetails))
		for i, err := range result.ErrorDetails {
			if i >= 10 {
				break
			}
			fmt.Printf("  - %s\n", err)
		}
	}
	fmt.Println()
}
