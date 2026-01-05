package rpc

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestRequestSizeLimit tests that requests exceeding the size limit are rejected
func TestRequestSizeLimit(t *testing.T) {
	// Create server with small max request size (1KB)
	cfg := &Config{
		ListenAddr:     "127.0.0.1:0",
		MaxRequestSize: 1024,
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer server.Stop()

	tests := []struct {
		name           string
		bodySize       int
		expectedStatus int
	}{
		{
			name:           "small request - allowed",
			bodySize:       512,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "exact limit - allowed",
			bodySize:       1024,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "oversized request - rejected",
			bodySize:       2048,
			expectedStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:           "very large request - rejected",
			bodySize:       10240,
			expectedStatus: http.StatusRequestEntityTooLarge,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create request with specified body size
			body := strings.Repeat("a", tc.bodySize)
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
			req.ContentLength = int64(tc.bodySize)

			w := httptest.NewRecorder()
			server.handleRequest(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, w.Code)
			}
		})
	}
}

// TestRateLimiting tests that rate limiting is enforced per IP
func TestRateLimiting(t *testing.T) {
	// Create server with low rate limit (5 requests per minute)
	cfg := &Config{
		ListenAddr: "127.0.0.1:0",
		RateLimit:  5,
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer server.Stop()

	// Use minimal valid JSON (will fail at method processing but that's ok)
	// We're testing rate limiting, not method handling
	reqBody := `{}`

	// First 5 requests should pass rate limiting (may fail later but not with 429)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(reqBody))
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()

		server.handleRequest(w, req)

		if w.Code == http.StatusTooManyRequests {
			t.Errorf("Request %d should not be rate limited", i+1)
		}
	}

	// 6th request should be rate limited
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(reqBody))
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	server.handleRequest(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429 (rate limited), got %d", w.Code)
	}
}

// TestRateLimitingPerIP tests that different IPs have independent rate limits
func TestRateLimitingPerIP(t *testing.T) {
	// Create server with low rate limit (3 requests per minute)
	cfg := &Config{
		ListenAddr: "127.0.0.1:0",
		RateLimit:  3,
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer server.Stop()

	reqBody := `{}`

	// Exhaust rate limit for IP1
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(reqBody))
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		server.handleRequest(w, req)
	}

	// IP1 should be rate limited
	req1 := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(reqBody))
	req1.RemoteAddr = "192.168.1.1:12345"
	w1 := httptest.NewRecorder()
	server.handleRequest(w1, req1)

	if w1.Code != http.StatusTooManyRequests {
		t.Error("IP1 should be rate limited")
	}

	// IP2 should still be allowed
	req2 := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(reqBody))
	req2.RemoteAddr = "192.168.1.2:12345"
	w2 := httptest.NewRecorder()
	server.handleRequest(w2, req2)

	if w2.Code == http.StatusTooManyRequests {
		t.Error("IP2 should not be rate limited")
	}
}

// TestSecurityHeaders tests that proper security headers are set
func TestSecurityHeaders(t *testing.T) {
	cfg := &Config{
		ListenAddr: "127.0.0.1:0",
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer server.Stop()

	reqBody := `{}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(reqBody))
	w := httptest.NewRecorder()

	server.handleRequest(w, req)

	// Check security headers
	expectedHeaders := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Content-Security-Policy": "default-src 'none'",
	}

	for header, expected := range expectedHeaders {
		actual := w.Header().Get(header)
		if actual != expected {
			t.Errorf("Header %s = %q, want %q", header, actual, expected)
		}
	}
}

// TestMethodNotAllowed tests that non-POST methods are rejected
func TestMethodNotAllowed(t *testing.T) {
	cfg := &Config{
		ListenAddr: "127.0.0.1:0",
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer server.Stop()

	methods := []string{
		http.MethodGet,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
		http.MethodHead,
		http.MethodOptions,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/", nil)
			w := httptest.NewRecorder()

			server.handleRequest(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("Method %s should return 405, got %d", method, w.Code)
			}
		})
	}
}

// TestMaxBytesReader tests that the MaxBytesReader prevents reading oversized bodies
func TestMaxBytesReader(t *testing.T) {
	cfg := &Config{
		ListenAddr:     "127.0.0.1:0",
		MaxRequestSize: 1024,
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer server.Stop()

	// Create a valid JSON body that is larger than the limit
	// Build a large params array
	largeBody := `{"jsonrpc":"2.0","method":"test","params":[` + strings.Repeat(`"aaaa",`, 200) + `"aaaa"],"id":1}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(largeBody))
	// Set ContentLength to actual size
	req.ContentLength = int64(len(largeBody))

	w := httptest.NewRecorder()
	server.handleRequest(w, req)

	// Should be rejected for being too large
	if w.Code == http.StatusOK {
		t.Error("Should not succeed with oversized body")
	}
}

// TestDefaultConfiguration tests default values are applied correctly
func TestDefaultConfiguration(t *testing.T) {
	cfg := &Config{
		ListenAddr: "127.0.0.1:0",
		// RateLimit and MaxRequestSize not set - should use defaults
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer server.Stop()

	if server.maxRequestSize != defaultMaxRequestSize {
		t.Errorf("maxRequestSize = %d, want %d", server.maxRequestSize, defaultMaxRequestSize)
	}

	if server.rateLimiter == nil {
		t.Error("rateLimiter should be initialized")
	}
}

// TestConcurrentRequests tests that rate limiting works correctly under concurrent load
func TestConcurrentRequests(t *testing.T) {
	cfg := &Config{
		ListenAddr: "127.0.0.1:0",
		RateLimit:  20, // Allow 20 requests per minute
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer server.Stop()

	reqBody := `{}`
	numRequests := 30
	results := make(chan int, numRequests)

	// Send 30 concurrent requests from the same IP
	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(reqBody))
			req.RemoteAddr = "192.168.1.1:12345"
			w := httptest.NewRecorder()
			server.handleRequest(w, req)
			results <- w.Code
		}()
	}

	// Collect results
	rateLimited := 0
	allowed := 0

	timeout := time.After(5 * time.Second)
	for i := 0; i < numRequests; i++ {
		select {
		case code := <-results:
			if code == http.StatusTooManyRequests {
				rateLimited++
			} else {
				allowed++
			}
		case <-timeout:
			t.Fatal("Test timeout")
		}
	}

	// Should have some rate limited requests (at least 10 out of 30)
	if rateLimited < 10 {
		t.Errorf("Expected at least 10 rate limited requests, got %d", rateLimited)
	}

	// Should have some allowed requests
	if allowed < 10 {
		t.Errorf("Expected at least 10 allowed requests, got %d", allowed)
	}
}

// TestAuthenticationWithRateLimiting tests that authentication and rate limiting work together
func TestAuthenticationWithRateLimiting(t *testing.T) {
	cfg := &Config{
		ListenAddr:  "127.0.0.1:0",
		RateLimit:   5,
		RPCUser:     "testuser",
		RPCPassword: "testpass",
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer server.Stop()

	reqBody := `{}`
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(reqBody))
		req.RemoteAddr = "192.168.1.1:12345"
		req.SetBasicAuth("testuser", "testpass")
		w := httptest.NewRecorder()
		server.handleRequest(w, req)

		if w.Code == http.StatusTooManyRequests {
			t.Errorf("Request %d should not be rate limited", i+1)
		}
	}

	// 6th request should be rate limited (before auth check)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(reqBody))
	req.RemoteAddr = "192.168.1.1:12345"
	req.SetBasicAuth("testuser", "testpass")
	w := httptest.NewRecorder()
	server.handleRequest(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429 (rate limited), got %d", w.Code)
	}
}

// BenchmarkHandleRequest benchmarks the handleRequest function
func BenchmarkHandleRequest(b *testing.B) {
	cfg := &Config{
		ListenAddr: "127.0.0.1:0",
		RateLimit:  10000, // High limit to avoid rate limiting in benchmark
	}

	server, err := NewServer(cfg)
	if err != nil {
		b.Fatalf("Failed to create server: %v", err)
	}
	defer server.Stop()

	reqBody := `{}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(reqBody))
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		server.handleRequest(w, req)
	}
}
