package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opd-ai/nmcd/internal/logging"
)

// TestNameScanRPCNoBlockchain tests name_scan with no blockchain
func TestNameScanRPCNoBlockchain(t *testing.T) {
	// Initialize logger for testing
	logCfg := logging.DefaultConfig()
	logCfg.Level = logging.LevelError
	logCfg.Output = "stderr"
	logger, err := logging.Init(logCfg)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	logging.SetDefault(logger)

	// Server with no blockchain
	server := &Server{
		blockchain:     nil,
		peerMgr:        nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
		logger:         logging.GetDefault().WithComponent("rpc"),
	}

	reqBody := Request{
		Jsonrpc: "2.0",
		Method:  "name_scan",
		Params:  json.RawMessage(`["d/", 100]`),
		ID:      1,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleRequest(w, req)

	var resp Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Error == nil {
		t.Error("Expected error for name_scan without blockchain")
	}
	if resp.Error != nil && resp.Error.Message != "Blockchain not initialized" {
		t.Errorf("Expected 'Blockchain not initialized' error, got '%s'", resp.Error.Message)
	}
}

// TestNameScanRPCInvalidParams tests name_scan parameter validation
// Note: Blockchain check happens first, so these tests document expected behavior
// when a blockchain is available. Without blockchain, the error is "Blockchain not initialized".
func TestNameScanRPCInvalidParams(t *testing.T) {
	// Initialize logger for testing
	logCfg := logging.DefaultConfig()
	logCfg.Level = logging.LevelError
	logCfg.Output = "stderr"
	logger, err := logging.Init(logCfg)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	logging.SetDefault(logger)

	// Server with no blockchain - param validation happens after blockchain check
	// so these tests verify the blockchain nil check takes precedence
	server := &Server{
		blockchain:     nil,
		peerMgr:        nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
		logger:         logging.GetDefault().WithComponent("rpc"),
	}

	// Test that blockchain check happens before param validation
	tests := []struct {
		name   string
		params json.RawMessage
	}{
		{
			name:   "invalid start type - blocked by blockchain check",
			params: json.RawMessage(`[123, 100]`),
		},
		{
			name:   "invalid count type - blocked by blockchain check",
			params: json.RawMessage(`["d/", "abc"]`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reqBody := Request{
				Jsonrpc: "2.0",
				Method:  "name_scan",
				Params:  tc.params,
				ID:      1,
			}
			body, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("Failed to marshal request: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			w := httptest.NewRecorder()

			server.handleRequest(w, req)

			var resp Response
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// When blockchain is nil, all requests return blockchain error first
			if resp.Error == nil {
				t.Error("Expected error in response")
				return
			}
			if resp.Error.Message != "Blockchain not initialized" {
				t.Errorf("Expected 'Blockchain not initialized' error, got '%s'", resp.Error.Message)
			}
		})
	}
}

// TestNamePendingRPC tests the name_pending RPC method
func TestNamePendingRPC(t *testing.T) {
	// Initialize logger for testing
	logCfg := logging.DefaultConfig()
	logCfg.Level = logging.LevelError
	logCfg.Output = "stderr"
	logger, err := logging.Init(logCfg)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	logging.SetDefault(logger)

	// Server with no blockchain or mempool (name_pending should return empty list)
	server := &Server{
		blockchain:     nil,
		peerMgr:        nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
		logger:         logging.GetDefault().WithComponent("rpc"),
	}

	tests := []struct {
		name   string
		params json.RawMessage
	}{
		{
			name:   "no params",
			params: json.RawMessage(`[]`),
		},
		{
			name:   "with name filter",
			params: json.RawMessage(`["d/example"]`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reqBody := Request{
				Jsonrpc: "2.0",
				Method:  "name_pending",
				Params:  tc.params,
				ID:      1,
			}
			body, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("Failed to marshal request: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			w := httptest.NewRecorder()

			server.handleRequest(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}

			var resp Response
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// name_pending should return empty list (no pending operations)
			if resp.Error != nil {
				t.Errorf("Expected no error, got: %s", resp.Error.Message)
				return
			}

			results, ok := resp.Result.([]interface{})
			if !ok {
				t.Fatalf("Expected array result, got %T", resp.Result)
			}
			if len(results) != 0 {
				t.Errorf("Expected empty result (no pending names), got %d", len(results))
			}
		})
	}
}
