package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opd-ai/nmcd/internal/logging"
)

// TestGracefulDegradationNoBlockchain tests that RPC methods handle missing blockchain gracefully
func TestGracefulDegradationNoBlockchain(t *testing.T) {
	// Initialize logger for testing
	logCfg := logging.DefaultConfig()
	logCfg.Level = logging.LevelError
	logCfg.Output = "stderr"
	logger, err := logging.Init(logCfg)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	logging.SetDefault(logger)

	// Server with no blockchain or peer manager
	server := &Server{
		blockchain:     nil,
		peerMgr:        nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
		logger:         logging.GetDefault().WithComponent("rpc"),
	}

	tests := []struct {
		name           string
		method         string
		expectError    bool
		expectErrorMsg string
	}{
		{
			name:           "getinfo without blockchain",
			method:         "getinfo",
			expectError:    true,
			expectErrorMsg: "Blockchain not initialized",
		},
		{
			name:           "getblockcount without blockchain",
			method:         "getblockcount",
			expectError:    true,
			expectErrorMsg: "Blockchain not initialized",
		},
		{
			name:           "getbestblockhash without blockchain",
			method:         "getbestblockhash",
			expectError:    true,
			expectErrorMsg: "Blockchain not initialized",
		},
		{
			name:        "getconnectioncount without peermgr",
			method:      "getconnectioncount",
			expectError: false, // Should return 0, not error
		},
		{
			name:        "getpeerinfo without peermgr",
			method:      "getpeerinfo",
			expectError: false, // Should return empty array, not error
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reqBody := Request{
				Jsonrpc: "2.0",
				Method:  tc.method,
				ID:      1,
			}
			body, _ := json.Marshal(reqBody)

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

			if tc.expectError {
				if resp.Error == nil {
					t.Errorf("Expected error in response for %s, got none", tc.method)
				} else if resp.Error.Message != tc.expectErrorMsg {
					t.Errorf("Expected error message '%s', got '%s'", tc.expectErrorMsg, resp.Error.Message)
				}
			} else {
				if resp.Error != nil {
					t.Errorf("Expected no error for %s, got: %v", tc.method, resp.Error)
				}
				if resp.Result == nil {
					t.Errorf("Expected result for %s, got nil", tc.method)
				}
			}
		})
	}
}

// TestGracefulDegradationWallet tests wallet methods handle missing wallet gracefully
func TestGracefulDegradationWallet(t *testing.T) {
	// Initialize logger for testing
	logCfg := logging.DefaultConfig()
	logCfg.Level = logging.LevelError
	logCfg.Output = "stderr"
	logger, err := logging.Init(logCfg)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	logging.SetDefault(logger)

	// Server with no wallet
	server := &Server{
		wallet:         nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
		logger:         logging.GetDefault().WithComponent("rpc"),
	}

	// Test getnewaddress without wallet
	reqBody := Request{
		Jsonrpc: "2.0",
		Method:  "getnewaddress",
		ID:      1,
	}
	body, _ := json.Marshal(reqBody)

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

	if resp.Error == nil {
		t.Error("Expected error when wallet is nil")
	}
}

// TestDatabaseErrorRecovery tests that database errors are handled gracefully
func TestDatabaseErrorRecovery(t *testing.T) {
	// This is a placeholder test demonstrating the pattern
	// In a real scenario, we would mock the blockchain to return errors
	t.Skip("Requires blockchain mocking - demonstrates graceful degradation pattern")
}
