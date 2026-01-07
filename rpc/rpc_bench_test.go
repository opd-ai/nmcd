package rpc

import (
	"encoding/json"
	"testing"
)

// BenchmarkJSONRequestParsing measures JSON request parsing performance.
// Target: < 100μs for typical request
func BenchmarkJSONRequestParsing(b *testing.B) {
	reqBody := `{"jsonrpc":"2.0","method":"getinfo","params":null,"id":1}`

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var req Request
		if err := json.Unmarshal([]byte(reqBody), &req); err != nil {
			b.Fatalf("Failed to parse request: %v", err)
		}
	}
}

// BenchmarkJSONResponseEncoding measures JSON response encoding performance.
// Target: < 100μs for typical response
func BenchmarkJSONResponseEncoding(b *testing.B) {
	response := Response{
		Jsonrpc: "2.0",
		Result: map[string]interface{}{
			"version":     "0.1.0",
			"blockheight": 12345,
			"peers":       5,
			"names":       1000,
		},
		ID: 1,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(response)
		if err != nil {
			b.Fatalf("Failed to encode response: %v", err)
		}
	}
}

// BenchmarkRateLimiting measures rate limiting overhead.
// Target: < 10μs overhead per request
func BenchmarkRateLimiting(b *testing.B) {
	limiter := newRateLimiter(1000)
	ip := "192.168.1.1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !limiter.allow(ip) {
			b.Fatal("Rate limit should not trigger")
		}
	}
}

// BenchmarkMemoryUsage measures memory allocation patterns for RPC handling.
func BenchmarkMemoryUsage(b *testing.B) {
	b.ReportAllocs()

	reqBody := `{"jsonrpc":"2.0","method":"getinfo","params":null,"id":1}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var req Request
		_ = json.Unmarshal([]byte(reqBody), &req)

		response := Response{
			Jsonrpc: "2.0",
			Result:  map[string]interface{}{"version": "0.1.0"},
			ID:      req.ID,
		}
		_, _ = json.Marshal(response)
	}
}
