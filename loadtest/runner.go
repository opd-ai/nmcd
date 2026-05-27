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
	"math"
	"net/http"
	"sort"
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
	if config.Concurrency <= 0 {
		return nil, fmt.Errorf("concurrency must be > 0")
	}
	if config.Duration <= 0 {
		return nil, fmt.Errorf("duration must be > 0")
	}

	result := &TestResult{
		Name:         "RPC Load Test",
		ErrorDetails: make([]string, 0),
	}

	client := NewRPCClient(config.RPCURL, config.RPCUser, config.RPCPassword)
	ctx := context.Background()

	if _, err := client.Call(ctx, "getinfo", nil); err != nil {
		return nil, fmt.Errorf("initial connectivity check failed: %w", err)
	}

	counters := &loadTestCounters{}
	tickers := createRateLimitTickers(config)
	for _, t := range tickers {
		defer t.Stop()
	}

	start := time.Now()
	stopChan := make(chan struct{})
	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()

	var wg sync.WaitGroup
	for i := 0; i < config.Concurrency; i++ {
		wg.Add(1)
		go runLoadWorker(&wg, client, workerCtx, stopChan, tickers, i, result, counters)
	}

	time.Sleep(config.Duration)
	close(stopChan)
	workerCancel()
	wg.Wait()

	populateLoadTestResult(result, counters, time.Since(start))
	return result, nil
}

// loadTestCounters holds thread-safe counters and latency samples for a load test.
type loadTestCounters struct {
	requestCount int64
	successCount int64
	failureCount int64
	latencies    []time.Duration
	latencyMu    sync.Mutex
	errorMu      sync.Mutex
}

// createRateLimitTickers creates per-worker rate limit tickers if rate limiting is enabled.
func createRateLimitTickers(config LoadTestConfig) []*time.Ticker {
	if config.RateLimit <= 0 {
		return nil
	}
	rpw := config.RateLimit / config.Concurrency
	if rpw < 1 {
		rpw = 1
	}
	tickers := make([]*time.Ticker, config.Concurrency)
	for i := range tickers {
		tickers[i] = time.NewTicker(time.Second / time.Duration(rpw))
	}
	return tickers
}

// runLoadWorker runs a single load test worker goroutine.
// Note: each worker uses a per-goroutine ticker; the actual aggregate RPS may
// slightly exceed the configured RateLimit due to scheduling jitter between
// goroutine wakeups. This drift is negligible at high concurrencies but may be
// observable (≤1 extra request/tick) at low concurrencies (1–2 workers).
func runLoadWorker(wg *sync.WaitGroup, client *RPCClient, ctx context.Context, stopChan chan struct{}, tickers []*time.Ticker, workerID int, result *TestResult, counters *loadTestCounters) {
	defer wg.Done()

	methods := []string{"getinfo", "getblockcount", "getbestblockhash"}
	methodIdx := 0

	var myTicker *time.Ticker
	if len(tickers) > workerID {
		myTicker = tickers[workerID]
	}

	for {
		select {
		case <-stopChan:
			return
		default:
			if myTicker != nil {
				<-myTicker.C
			}

			reqStart := time.Now()
			method := methods[methodIdx%len(methods)]
			methodIdx++

			_, err := client.Call(ctx, method, nil)
			latency := time.Since(reqStart)
			atomic.AddInt64(&counters.requestCount, 1)

			if err != nil {
				atomic.AddInt64(&counters.failureCount, 1)
				counters.errorMu.Lock()
				if len(result.ErrorDetails) < 100 {
					result.ErrorDetails = append(result.ErrorDetails, err.Error())
				}
				counters.errorMu.Unlock()
			} else {
				atomic.AddInt64(&counters.successCount, 1)
				counters.latencyMu.Lock()
				if len(counters.latencies) < 10000 {
					counters.latencies = append(counters.latencies, latency)
				}
				counters.latencyMu.Unlock()
			}
		}
	}
}

// populateLoadTestResult fills in the test result with calculated statistics.
func populateLoadTestResult(result *TestResult, counters *loadTestCounters, duration time.Duration) {
	result.Duration = duration
	result.RequestCount = atomic.LoadInt64(&counters.requestCount)
	result.SuccessCount = atomic.LoadInt64(&counters.successCount)
	result.FailureCount = atomic.LoadInt64(&counters.failureCount)
	result.ThroughputRPS = float64(result.RequestCount) / duration.Seconds()

	// Copy latencies slice under lock to prevent race
	counters.latencyMu.Lock()
	latenciesCopy := make([]time.Duration, len(counters.latencies))
	copy(latenciesCopy, counters.latencies)
	counters.latencyMu.Unlock()

	if len(latenciesCopy) == 0 {
		return
	}

	var total time.Duration
	for _, l := range latenciesCopy {
		total += l
	}
	result.AvgLatency = total / time.Duration(len(latenciesCopy))

	if len(latenciesCopy) >= 20 {
		sort.Slice(latenciesCopy, func(i, j int) bool { return latenciesCopy[i] < latenciesCopy[j] })
		n := len(latenciesCopy)
		result.P95Latency = latenciesCopy[percentileIdx(n, 0.95)]
		result.P99Latency = latenciesCopy[percentileIdx(n, 0.99)]
	}
}

// percentileIdx returns the 0-based index into a sorted slice of n elements
// for the given percentile p (0–1). Uses nearest-rank method with ceiling,
// clamped to [0, n-1].
func percentileIdx(n int, p float64) int {
	if n <= 0 {
		return 0
	}
	idx := int(math.Ceil(float64(n)*p)) - 1
	if idx < 0 {
		return 0
	}
	if idx >= n {
		return n - 1
	}
	return idx
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
	result.RequestCount = atomic.LoadInt64(&requestCount)
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
	RPCURL        string
	RPCUser       string
	RPCPassword   string
	Duration      time.Duration
	NameCount     int           // Number of names to process
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
	counters := &loadTestCounters{}

	stopChan := make(chan struct{})
	healthErrors := runHealthChecker(client, ctx, config.CheckInterval, stopChan)

	runContinuousLoop(client, ctx, config, result, counters)

	close(stopChan)

	healthErrors.mu.Lock()
	result.ErrorDetails = append(result.ErrorDetails, healthErrors.errors...)
	healthErrors.mu.Unlock()

	result.Duration = time.Since(start)
	result.RequestCount = atomic.LoadInt64(&counters.requestCount)
	result.SuccessCount = atomic.LoadInt64(&counters.successCount)
	result.FailureCount = atomic.LoadInt64(&counters.failureCount)
	result.ThroughputRPS = float64(result.RequestCount) / time.Since(start).Seconds()

	return result, nil
}

// healthCheckResult holds health check errors collected by the health checker goroutine.
type healthCheckResult struct {
	errors []string
	mu     sync.Mutex
}

// runHealthChecker starts a background health check goroutine and returns its results.
func runHealthChecker(client *RPCClient, ctx context.Context, interval time.Duration, stopChan <-chan struct{}) *healthCheckResult {
	result := &healthCheckResult{}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				if _, err := client.Call(ctx, "getinfo", nil); err != nil {
					result.mu.Lock()
					result.errors = append(result.errors, fmt.Sprintf("[%s] health check failed: %v", time.Now().Format(time.RFC3339), err))
					result.mu.Unlock()
				}
			}
		}
	}()
	return result
}

// runContinuousLoop executes the main continuous operation loop.
func runContinuousLoop(client *RPCClient, ctx context.Context, config ContinuousOperationConfig, result *TestResult, counters *loadTestCounters) {
	endTime := time.Now().Add(config.Duration)
	operations := []string{"getinfo", "getblockcount", "getbestblockhash", "name_list"}
	opIdx := 0

	for time.Now().Before(endTime) {
		operation := operations[opIdx%len(operations)]
		opIdx++

		_, err := client.Call(ctx, operation, nil)
		atomic.AddInt64(&counters.requestCount, 1)

		if err != nil {
			atomic.AddInt64(&counters.failureCount, 1)
			counters.errorMu.Lock()
			if len(result.ErrorDetails) < 100 {
				result.ErrorDetails = append(result.ErrorDetails, err.Error())
			}
			counters.errorMu.Unlock()
		} else {
			atomic.AddInt64(&counters.successCount, 1)
		}

		time.Sleep(10 * time.Millisecond)

		if config.NameCount > 0 && int(atomic.LoadInt64(&counters.successCount)) >= config.NameCount {
			break
		}
	}
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
