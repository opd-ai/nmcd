// Command loadtest provides a command-line interface for running load and stress tests against nmcd.
//
// Usage:
//
//	loadtest [flags]
//
// Flags:
//
//	-rpcurl string      RPC endpoint URL (default "http://localhost:8336")
//	-rpcuser string     RPC username
//	-rpcpass string     RPC password
//	-duration duration  Test duration (default 60s)
//	-concurrency int    Number of concurrent clients (default 10)
//	-ratelimit int      Requests per second limit (0 = unlimited) (default 0)
//	-test string        Test to run: rpc, memory, continuous, all (default "all")
//	-namecount int      Number of names to process (continuous test only) (default 0)
//
// Examples:
//
//	# Run all tests with default settings
//	loadtest
//
//	# Run RPC load test with 500 concurrent clients for 5 minutes
//	loadtest -test rpc -concurrency 500 -duration 5m
//
//	# Run memory leak detection for 24 hours
//	loadtest -test memory -duration 24h
//
//	# Run continuous operation test with 100k operations
//	loadtest -test continuous -namecount 100000 -duration 72h
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/opd-ai/nmcd/loadtest"
)

const version = "0.1.0"

func main() {
	rpcURL := flag.String("rpcurl", "http://localhost:8336", "RPC endpoint URL")
	rpcUser := flag.String("rpcuser", "", "RPC username")
	rpcPass := flag.String("rpcpass", "", "RPC password")
	duration := flag.Duration("duration", 60*time.Second, "Test duration")
	concurrency := flag.Int("concurrency", 10, "Number of concurrent clients")
	rateLimit := flag.Int("ratelimit", 0, "Requests per second limit (0 = unlimited)")
	testType := flag.String("test", "all", "Test to run: rpc, memory, continuous, all")
	nameCount := flag.Int("namecount", 0, "Number of names to process (continuous test only)")
	showVersion := flag.Bool("version", false, "Show version information")

	flag.Parse()

	if *showVersion {
		fmt.Printf("nmcd loadtest version %s\n", version)
		os.Exit(0)
	}

	printBanner(*rpcURL, *duration, *concurrency, *rateLimit, *testType)

	cfg := loadtest.LoadTestConfig{
		RPCURL:      *rpcURL,
		RPCUser:     *rpcUser,
		RPCPassword: *rpcPass,
		Duration:    *duration,
		Concurrency: *concurrency,
		RateLimit:   *rateLimit,
	}

	var allResults []*loadtest.TestResult
	var hasFailure bool

	if *testType == "rpc" || *testType == "all" {
		result, failed := runRPCTest(cfg)
		allResults = appendResult(allResults, result)
		hasFailure = hasFailure || failed
	}

	if *testType == "memory" || *testType == "all" {
		result, failed := runMemoryTest(cfg)
		allResults = appendResult(allResults, result)
		hasFailure = hasFailure || failed
	}

	if *testType == "continuous" || *testType == "all" {
		result, failed := runContinuousTest(cfg, *nameCount)
		allResults = appendResult(allResults, result)
		hasFailure = hasFailure || failed
	}

	printSummary(allResults, hasFailure)
}

func printBanner(rpcURL string, duration time.Duration, concurrency, rateLimit int, testType string) {
	fmt.Printf("=== nmcd Load & Stress Testing v%s ===\n", version)
	fmt.Printf("RPC URL:      %s\n", rpcURL)
	fmt.Printf("Duration:     %v\n", duration)
	fmt.Printf("Concurrency:  %d\n", concurrency)
	fmt.Printf("Rate Limit:   %d req/s\n", rateLimit)
	fmt.Printf("Test Type:    %s\n\n", testType)
}

func appendResult(results []*loadtest.TestResult, r *loadtest.TestResult) []*loadtest.TestResult {
	if r != nil {
		return append(results, r)
	}
	return results
}

func runRPCTest(cfg loadtest.LoadTestConfig) (*loadtest.TestResult, bool) {
	fmt.Println("Running RPC Load Test...")
	result, err := loadtest.RPCLoadTest(cfg)
	if err != nil {
		fmt.Printf("ERROR: RPC load test failed: %v\n", err)
		return nil, true
	}
	loadtest.PrintResult(result)
	if result.ThroughputRPS < 1000 {
		fmt.Printf("⚠️  WARNING: Throughput %.2f req/s is below target 1000 req/s\n\n", result.ThroughputRPS)
	}
	if result.RequestCount > 0 && result.FailureCount > result.RequestCount/100 {
		fmt.Printf("⚠️  WARNING: Failure rate %.2f%% is above 1%% threshold\n\n",
			float64(result.FailureCount)/float64(result.RequestCount)*100)
	}
	return result, false
}

func runMemoryTest(cfg loadtest.LoadTestConfig) (*loadtest.TestResult, bool) {
	fmt.Println("Running Memory Leak Detection Test...")
	memCheckInterval := cfg.Duration / 20
	if memCheckInterval < 10*time.Second {
		memCheckInterval = 10 * time.Second
	}

	result, err := loadtest.MemoryLeakTest(cfg, memCheckInterval)
	if err != nil {
		fmt.Printf("ERROR: Memory leak test failed: %v\n", err)
		return nil, true
	}
	loadtest.PrintResult(result)

	if durationHours := result.Duration.Hours(); durationHours >= 24 {
		growthMBPerHour := float64(result.MemoryGrowth) / durationHours / (1024 * 1024)
		if growthMBPerHour > 10 {
			fmt.Printf("⚠️  WARNING: Memory growth %.2f MB/hour exceeds 10 MB/hour threshold\n\n", growthMBPerHour)
		} else {
			fmt.Printf("✅ PASS: Memory growth %.2f MB/hour is within acceptable range\n\n", growthMBPerHour)
		}
	}
	return result, false
}

func runContinuousTest(cfg loadtest.LoadTestConfig, nameCount int) (*loadtest.TestResult, bool) {
	fmt.Println("Running Continuous Operation Test...")
	result, err := loadtest.ContinuousOperationTest(loadtest.ContinuousOperationConfig{
		RPCURL:        cfg.RPCURL,
		RPCUser:       cfg.RPCUser,
		RPCPassword:   cfg.RPCPassword,
		Duration:      cfg.Duration,
		NameCount:     nameCount,
		CheckInterval: 30 * time.Second,
	})
	if err != nil {
		fmt.Printf("ERROR: Continuous operation test failed: %v\n", err)
		return nil, true
	}
	loadtest.PrintResult(result)

	successRate := float64(result.SuccessCount) / float64(result.RequestCount) * 100
	if successRate < 99.9 {
		fmt.Printf("⚠️  WARNING: Success rate %.2f%% is below 99.9%% threshold\n\n", successRate)
	} else {
		fmt.Printf("✅ PASS: Success rate %.2f%% meets reliability requirements\n\n", successRate)
	}
	return result, false
}

func printSummary(allResults []*loadtest.TestResult, hasFailure bool) {
	fmt.Println("=== Test Summary ===")
	fmt.Printf("Tests Run:        %d\n", len(allResults))

	var totalRequests, totalSuccess, totalFailures int64
	for _, result := range allResults {
		totalRequests += result.RequestCount
		totalSuccess += result.SuccessCount
		totalFailures += result.FailureCount
	}

	fmt.Printf("Total Requests:   %d\n", totalRequests)
	fmt.Printf("Total Success:    %d\n", totalSuccess)
	fmt.Printf("Total Failures:   %d\n", totalFailures)

	if totalRequests > 0 {
		fmt.Printf("Overall Success:  %.2f%%\n", float64(totalSuccess)/float64(totalRequests)*100)
	}

	if hasFailure {
		fmt.Println("\n❌ FAILED: One or more tests encountered errors")
		os.Exit(1)
	} else {
		fmt.Println("\n✅ PASSED: All tests completed successfully")
		os.Exit(0)
	}
}
