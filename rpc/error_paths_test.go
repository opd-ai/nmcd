package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/opd-ai/nmcd/internal/logging"
)

// TestRPCErrorPaths tests error handling for various RPC methods with nil dependencies
func TestRPCErrorPaths(t *testing.T) {
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
		wallet:         nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
		logger:         logging.GetDefault().WithComponent("rpc"),
	}

	tests := []struct {
		name            string
		method          string
		params          interface{}
		expectError     bool
		expectErrorMsg  string
		expectErrorCode int
	}{
		// name_scan tests
		{
			name:            "name_scan without blockchain",
			method:          "name_scan",
			params:          nil,
			expectError:     true,
			expectErrorMsg:  "Blockchain not initialized",
			expectErrorCode: -32603,
		},
		// name_new tests
		{
			name:            "name_new without wallet",
			method:          "name_new",
			params:          []string{"d/testname"},
			expectError:     true,
			expectErrorMsg:  "Wallet not initialized",
			expectErrorCode: -1,
		},
		// name_firstupdate tests
		{
			name:            "name_firstupdate without wallet",
			method:          "name_firstupdate",
			params:          []string{"d/testname", "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20", "{}"},
			expectError:     true,
			expectErrorMsg:  "Wallet not initialized",
			expectErrorCode: -1,
		},
		// getblock tests
		{
			name:            "getblock without blockchain",
			method:          "getblock",
			params:          []string{"0000000000000000000000000000000000000000000000000000000000000000"},
			expectError:     true,
			expectErrorMsg:  "Blockchain not initialized",
			expectErrorCode: -32603,
		},
		// getblockhash tests
		{
			name:            "getblockhash without blockchain",
			method:          "getblockhash",
			params:          []int{0},
			expectError:     true,
			expectErrorMsg:  "Blockchain not initialized",
			expectErrorCode: -32603,
		},
		// getrawtransaction tests
		{
			name:            "getrawtransaction without blockchain",
			method:          "getrawtransaction",
			params:          []string{"0000000000000000000000000000000000000000000000000000000000000000"},
			expectError:     true,
			expectErrorMsg:  "Blockchain not initialized",
			expectErrorCode: -32603,
		},
		// sendrawtransaction tests - tests TX decode error path with invalid hex
		{
			name:            "sendrawtransaction with invalid tx hex",
			method:          "sendrawtransaction",
			params:          []string{"0100000001"},
			expectError:     true,
			expectErrorMsg:  "TX decode failed",
			expectErrorCode: -22,
		},
		// getnewaddress tests
		{
			name:            "getnewaddress without wallet",
			method:          "getnewaddress",
			params:          nil,
			expectError:     true,
			expectErrorMsg:  "Wallet not initialized",
			expectErrorCode: -1,
		},
		// listaddresses tests
		{
			name:            "listaddresses without wallet",
			method:          "listaddresses",
			params:          nil,
			expectError:     true,
			expectErrorMsg:  "Wallet not initialized",
			expectErrorCode: -1,
		},
		// walletpassphrase tests
		{
			name:            "walletpassphrase without wallet",
			method:          "walletpassphrase",
			params:          []interface{}{"password", 300},
			expectError:     true,
			expectErrorMsg:  "Wallet not initialized",
			expectErrorCode: -1,
		},
		// walletlock tests
		{
			name:            "walletlock without wallet",
			method:          "walletlock",
			params:          nil,
			expectError:     true,
			expectErrorMsg:  "Wallet not initialized",
			expectErrorCode: -1,
		},
		// encryptwallet tests
		{
			name:            "encryptwallet without wallet",
			method:          "encryptwallet",
			params:          []string{"password"},
			expectError:     true,
			expectErrorMsg:  "Wallet not initialized",
			expectErrorCode: -1,
		},
		// getbalance tests
		{
			name:            "getbalance without wallet",
			method:          "getbalance",
			params:          nil,
			expectError:     true,
			expectErrorMsg:  "Wallet not initialized",
			expectErrorCode: -1,
		},
		// listunspent tests
		{
			name:            "listunspent without wallet",
			method:          "listunspent",
			params:          nil,
			expectError:     true,
			expectErrorMsg:  "Wallet not initialized",
			expectErrorCode: -1,
		},
		// getmetrics test - should succeed even without blockchain
		{
			name:        "getmetrics without blockchain",
			method:      "getmetrics",
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paramsJSON, _ := json.Marshal(tc.params)
			reqBody := Request{
				Jsonrpc: "2.0",
				Method:  tc.method,
				Params:  paramsJSON,
				ID:      1,
			}
			body, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("Failed to marshal request: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			req.Header.Set("Content-Length", "1000")
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
					t.Errorf("Expected error for %s, got none", tc.method)
				} else {
					if !strings.Contains(resp.Error.Message, tc.expectErrorMsg) {
						t.Errorf("Expected error message containing '%s', got '%s'", tc.expectErrorMsg, resp.Error.Message)
					}
					if tc.expectErrorCode != 0 && resp.Error.Code != tc.expectErrorCode {
						t.Errorf("Expected error code %d, got %d", tc.expectErrorCode, resp.Error.Code)
					}
				}
			} else {
				if resp.Error != nil {
					t.Errorf("Expected no error for %s, got: %v", tc.method, resp.Error)
				}
			}
		})
	}
}

// TestNameScanParamValidation tests parameter validation for name_scan
func TestNameScanParamValidation(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	tests := []struct {
		name           string
		params         interface{}
		expectError    bool
		expectErrorMsg string
	}{
		{
			name:        "valid empty params",
			params:      nil,
			expectError: false,
		},
		{
			name:        "valid with start",
			params:      []interface{}{"d/"},
			expectError: false,
		},
		{
			name:        "valid with start and count",
			params:      []interface{}{"d/", 100.0},
			expectError: false,
		},
		{
			name:           "invalid start type",
			params:         []interface{}{123},
			expectError:    true,
			expectErrorMsg: "start must be a string",
		},
		{
			name:           "invalid count type",
			params:         []interface{}{"d/", "notanumber"},
			expectError:    true,
			expectErrorMsg: "count must be a number",
		},
		{
			name:           "count too small",
			params:         []interface{}{"d/", 0.0},
			expectError:    true,
			expectErrorMsg: "count must be between 1 and 10000",
		},
		{
			name:           "count too large",
			params:         []interface{}{"d/", 10001.0},
			expectError:    true,
			expectErrorMsg: "count must be between 1 and 10000",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paramsJSON, _ := json.Marshal(tc.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "name_scan",
				Params:  paramsJSON,
				ID:      1,
			}

			resp := server.nameScan(req)

			if tc.expectError {
				if resp.Error == nil {
					t.Fatal("Expected error but got none")
				}
				if !strings.Contains(resp.Error.Message, tc.expectErrorMsg) {
					t.Errorf("Expected error message containing '%s', got '%s'", tc.expectErrorMsg, resp.Error.Message)
				}
			} else {
				if resp.Error != nil {
					t.Errorf("Expected no error, got: %v", resp.Error)
				}
			}
		})
	}
}

// TestGetBlockParamValidation tests parameter validation for getblock
func TestGetBlockParamValidation(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	tests := []struct {
		name           string
		params         interface{}
		expectError    bool
		expectErrorMsg string
	}{
		{
			name:           "empty params",
			params:         []string{},
			expectError:    true,
			expectErrorMsg: "Invalid params",
		},
		{
			name:           "invalid hash format",
			params:         []string{"notahash"},
			expectError:    true,
			expectErrorMsg: "Invalid block hash",
		},
		{
			name:           "non-existent block hash",
			params:         []string{"0000000000000000000000000000000000000000000000000000000000000001"},
			expectError:    true,
			expectErrorMsg: "Block not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paramsJSON, _ := json.Marshal(tc.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "getblock",
				Params:  paramsJSON,
				ID:      1,
			}

			resp := server.getBlock(req)

			if tc.expectError {
				if resp.Error == nil {
					t.Fatal("Expected error but got none")
				}
				if !strings.Contains(resp.Error.Message, tc.expectErrorMsg) {
					t.Errorf("Expected error message containing '%s', got '%s'", tc.expectErrorMsg, resp.Error.Message)
				}
			} else {
				if resp.Error != nil {
					t.Errorf("Expected no error, got: %v", resp.Error)
				}
			}
		})
	}
}

// TestGetBlockHashParamValidation tests parameter validation for getblockhash
func TestGetBlockHashParamValidation(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	tests := []struct {
		name           string
		params         interface{}
		expectError    bool
		expectErrorMsg string
	}{
		{
			name:           "empty params",
			params:         []int{},
			expectError:    true,
			expectErrorMsg: "Invalid params",
		},
		{
			name:           "negative height",
			params:         []interface{}{-1.0},
			expectError:    true,
			expectErrorMsg: "Block height out of range",
		},
		{
			name:           "height too high",
			params:         []interface{}{1000000.0},
			expectError:    true,
			expectErrorMsg: "", // Will fail because block doesn't exist
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paramsJSON, _ := json.Marshal(tc.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "getblockhash",
				Params:  paramsJSON,
				ID:      1,
			}

			resp := server.getBlockHash(req)

			if tc.expectError {
				if resp.Error == nil {
					t.Fatal("Expected error but got none")
				}
				if tc.expectErrorMsg != "" && !strings.Contains(resp.Error.Message, tc.expectErrorMsg) {
					t.Errorf("Expected error message containing '%s', got '%s'", tc.expectErrorMsg, resp.Error.Message)
				}
			} else {
				if resp.Error != nil {
					t.Errorf("Expected no error, got: %v", resp.Error)
				}
			}
		})
	}
}

// TestGetRawTransactionParamValidation tests parameter validation for getrawtransaction
func TestGetRawTransactionParamValidation(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	tests := []struct {
		name           string
		params         interface{}
		expectError    bool
		expectErrorMsg string
	}{
		{
			name:           "empty params",
			params:         []string{},
			expectError:    true,
			expectErrorMsg: "Invalid params",
		},
		{
			name:           "invalid txid format",
			params:         []string{"notahash"},
			expectError:    true,
			expectErrorMsg: "Invalid transaction ID",
		},
		{
			name:           "non-existent transaction",
			params:         []string{"0000000000000000000000000000000000000000000000000000000000000001"},
			expectError:    true,
			expectErrorMsg: "Transaction not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paramsJSON, _ := json.Marshal(tc.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "getrawtransaction",
				Params:  paramsJSON,
				ID:      1,
			}

			resp := server.getRawTransaction(req)

			if tc.expectError {
				if resp.Error == nil {
					t.Fatal("Expected error but got none")
				}
				if tc.expectErrorMsg != "" && !strings.Contains(resp.Error.Message, tc.expectErrorMsg) {
					t.Errorf("Expected error message containing '%s', got '%s'", tc.expectErrorMsg, resp.Error.Message)
				}
			} else {
				if resp.Error != nil {
					t.Errorf("Expected no error, got: %v", resp.Error)
				}
			}
		})
	}
}

// TestSendRawTransactionParamValidation tests parameter validation for sendrawtransaction
func TestSendRawTransactionParamValidation(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	tests := []struct {
		name           string
		params         interface{}
		expectError    bool
		expectErrorMsg string
	}{
		{
			name:           "empty params",
			params:         []string{},
			expectError:    true,
			expectErrorMsg: "Invalid params",
		},
		{
			name:           "invalid hex",
			params:         []string{"notvalidhex!@#$"},
			expectError:    true,
			expectErrorMsg: "TX decode failed",
		},
		{
			name:           "truncated transaction",
			params:         []string{"0100000001"},
			expectError:    true,
			expectErrorMsg: "TX decode failed", // Will fail to deserialize
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paramsJSON, _ := json.Marshal(tc.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "sendrawtransaction",
				Params:  paramsJSON,
				ID:      1,
			}

			resp := server.sendRawTransaction(req)

			if tc.expectError {
				if resp.Error == nil {
					t.Fatal("Expected error but got none")
				}
				if tc.expectErrorMsg != "" && !strings.Contains(resp.Error.Message, tc.expectErrorMsg) {
					t.Errorf("Expected error message containing '%s', got '%s'", tc.expectErrorMsg, resp.Error.Message)
				}
			} else {
				if resp.Error != nil {
					t.Errorf("Expected no error, got: %v", resp.Error)
				}
			}
		})
	}
}

// TestNamePendingWithBlockchain tests name_pending with an initialized blockchain
func TestNamePendingWithBlockchain(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_pending",
		Params:  nil,
		ID:      1,
	}

	resp := server.namePending(req)

	// name_pending should return an empty array, not an error
	if resp.Error != nil {
		t.Errorf("Expected success, got error: %v", resp.Error)
	}

	result, ok := resp.Result.([]map[string]interface{})
	if !ok {
		t.Errorf("Expected array result, got %T", resp.Result)
	}

	if len(result) != 0 {
		t.Errorf("Expected empty array for empty mempool, got %d items", len(result))
	}
}

// TestNamePendingWithFilter tests name_pending with name filter parameter
func TestNamePendingWithFilter(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	paramsJSON, _ := json.Marshal([]string{"d/filter"})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_pending",
		Params:  paramsJSON,
		ID:      1,
	}

	resp := server.namePending(req)

	// Should return empty array, not an error
	if resp.Error != nil {
		t.Errorf("Expected success, got error: %v", resp.Error)
	}
}

// TestWalletPassphraseParamValidation tests parameter validation for walletpassphrase
func TestWalletPassphraseParamValidation(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	tests := []struct {
		name           string
		params         interface{}
		expectError    bool
		expectErrorMsg string
	}{
		{
			name:           "empty params",
			params:         []interface{}{},
			expectError:    true,
			expectErrorMsg: "", // Will fail with empty params or wallet not encrypted
		},
		{
			name:           "missing timeout",
			params:         []interface{}{"password"},
			expectError:    true,
			expectErrorMsg: "", // Wallet not encrypted error
		},
		{
			name:           "invalid timeout type",
			params:         []interface{}{"password", "notanumber"},
			expectError:    true,
			expectErrorMsg: "Invalid timeout", // Check for partial match
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paramsJSON, _ := json.Marshal(tc.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "walletpassphrase",
				Params:  paramsJSON,
				ID:      1,
			}

			resp := server.walletPassphrase(req)

			if tc.expectError {
				if resp.Error == nil {
					t.Fatal("Expected error but got none")
				}
				if !strings.Contains(resp.Error.Message, tc.expectErrorMsg) {
					t.Errorf("Expected error message containing '%s', got '%s'", tc.expectErrorMsg, resp.Error.Message)
				}
			}
		})
	}
}

// TestEncryptWalletParamValidation tests parameter validation for encryptwallet
func TestEncryptWalletParamValidation(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	tests := []struct {
		name           string
		params         interface{}
		expectError    bool
		expectErrorMsg string
	}{
		{
			name:           "empty params",
			params:         []string{},
			expectError:    true,
			expectErrorMsg: "Invalid params",
		},
		{
			name:           "empty password",
			params:         []string{""},
			expectError:    true,
			expectErrorMsg: "password", // Error message mentions password length
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paramsJSON, _ := json.Marshal(tc.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "encryptwallet",
				Params:  paramsJSON,
				ID:      1,
			}

			resp := server.encryptWallet(req)

			if tc.expectError {
				if resp.Error == nil {
					t.Fatal("Expected error but got none")
				}
				if !strings.Contains(resp.Error.Message, tc.expectErrorMsg) {
					t.Errorf("Expected error message containing '%s', got '%s'", tc.expectErrorMsg, resp.Error.Message)
				}
			}
		})
	}
}
