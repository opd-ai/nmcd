package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mockRPCServer creates a mock JSON-RPC server for testing
func mockRPCServer(t *testing.T, handler func(method string, params json.RawMessage) (interface{}, *rpcError)) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check method
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check content type
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "Invalid content type", http.StatusBadRequest)
			return
		}

		// Parse request
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeRPCError(w, 1, -32700, "Parse error")
			return
		}

		// Handle request
		var paramsRaw json.RawMessage
		if req.Params != nil {
			paramsRaw, _ = json.Marshal(req.Params)
		}
		result, rpcErr := handler(req.Method, paramsRaw)

		// Write response
		resp := rpcResponse{
			Jsonrpc: "2.0",
			ID:      req.ID,
		}

		if rpcErr != nil {
			resp.Error = rpcErr
		} else {
			resultBytes, _ := json.Marshal(result)
			resp.Result = resultBytes
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func writeRPCError(w http.ResponseWriter, id, code int, message string) {
	resp := rpcResponse{
		Jsonrpc: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: message,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func TestNewDaemonClient(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			cfg:     nil,
			wantErr: false,
		},
		{
			name: "custom RPC address",
			cfg: &Config{
				RPCAddr: "http://custom:8336",
			},
			wantErr: false,
		},
		{
			name: "with authentication",
			cfg: &Config{
				RPCAddr:     "http://localhost:8336",
				RPCUser:     "testuser",
				RPCPassword: "testpass",
			},
			wantErr: false,
		},
		{
			name: "empty RPC address uses default",
			cfg: &Config{
				RPCAddr: "",
			},
			wantErr: false,
		},
		{
			name: "incomplete auth - only user",
			cfg: &Config{
				RPCAddr: "http://localhost:8336",
				RPCUser: "testuser",
			},
			wantErr: true,
		},
		{
			name: "incomplete auth - only password",
			cfg: &Config{
				RPCAddr:     "http://localhost:8336",
				RPCPassword: "testpass",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewDaemonClient(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewDaemonClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("NewDaemonClient() returned nil client")
			}
			if client != nil {
				client.Close()
			}
		})
	}
}

func TestDaemonClient_Ping(t *testing.T) {
	tests := []struct {
		name    string
		handler func(method string, params json.RawMessage) (interface{}, *rpcError)
		wantErr bool
	}{
		{
			name: "healthy daemon",
			handler: func(method string, params json.RawMessage) (interface{}, *rpcError) {
				if method == "getinfo" {
					return map[string]interface{}{
						"version":     "0.1.0",
						"blocks":      100,
						"connections": 5,
					}, nil
				}
				if method == "getbestblockhash" {
					return "0000000000000000000000000000000000000000000000000000000000000000", nil
				}
				return nil, &rpcError{Code: -32601, Message: "Method not found"}
			},
			wantErr: false,
		},
		{
			name: "daemon error",
			handler: func(method string, params json.RawMessage) (interface{}, *rpcError) {
				return nil, &rpcError{Code: -1, Message: "Internal error"}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockRPCServer(t, tt.handler)
			defer server.Close()

			client, err := NewDaemonClient(&Config{RPCAddr: server.URL})
			if err != nil {
				t.Fatalf("NewDaemonClient() error = %v", err)
			}
			defer client.Close()

			ctx := context.Background()
			err = client.Ping(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Ping() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDaemonClient_ResolveName(t *testing.T) {
	tests := []struct {
		name       string
		lookupName string
		handler    func(method string, params json.RawMessage) (interface{}, *rpcError)
		wantErr    error
		wantName   string
		wantValue  string
	}{
		{
			name:       "name found",
			lookupName: "d/example",
			handler: func(method string, params json.RawMessage) (interface{}, *rpcError) {
				if method == "name_show" {
					return map[string]interface{}{
						"name":       "d/example",
						"value":      `{"ip":"1.2.3.4"}`,
						"txid":       "abc123",
						"height":     1000,
						"expires_in": 5000,
						"address":    "N1abc",
					}, nil
				}
				return nil, &rpcError{Code: -32601, Message: "Method not found"}
			},
			wantErr:   nil,
			wantName:  "d/example",
			wantValue: `{"ip":"1.2.3.4"}`,
		},
		{
			name:       "name not found",
			lookupName: "d/notexist",
			handler: func(method string, params json.RawMessage) (interface{}, *rpcError) {
				return nil, &rpcError{Code: -5, Message: "Name not found: d/notexist"}
			},
			wantErr: ErrNameNotFound,
		},
		{
			name:       "name expired",
			lookupName: "d/expired",
			handler: func(method string, params json.RawMessage) (interface{}, *rpcError) {
				return map[string]interface{}{
					"name":       "d/expired",
					"value":      `{}`,
					"txid":       "abc123",
					"height":     1000,
					"expires_in": -100, // Negative means expired
					"address":    "N1abc",
				}, nil
			},
			wantErr: ErrNameExpired,
		},
		{
			name:       "empty name",
			lookupName: "",
			handler:    nil, // Won't be called
			wantErr:    ErrInvalidName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.handler != nil {
				server = mockRPCServer(t, tt.handler)
				defer server.Close()
			} else {
				server = mockRPCServer(t, func(method string, params json.RawMessage) (interface{}, *rpcError) {
					return nil, &rpcError{Code: -1, Message: "Unexpected call"}
				})
				defer server.Close()
			}

			client, err := NewDaemonClient(&Config{RPCAddr: server.URL})
			if err != nil {
				t.Fatalf("NewDaemonClient() error = %v", err)
			}
			defer client.Close()

			ctx := context.Background()
			record, err := client.ResolveName(ctx, tt.lookupName)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("ResolveName() expected error %v, got nil", tt.wantErr)
					return
				}
				if !errors.Is(err, tt.wantErr) && !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Errorf("ResolveName() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("ResolveName() unexpected error = %v", err)
				return
			}

			if record.Name != tt.wantName {
				t.Errorf("ResolveName() name = %v, want %v", record.Name, tt.wantName)
			}
			if record.Value != tt.wantValue {
				t.Errorf("ResolveName() value = %v, want %v", record.Value, tt.wantValue)
			}
		})
	}
}

func TestDaemonClient_UpdateName(t *testing.T) {
	tests := []struct {
		name       string
		updateName string
		value      string
		opts       *UpdateOpts
		handler    func(method string, params json.RawMessage) (interface{}, *rpcError)
		wantErr    bool
		wantTxHash string
	}{
		{
			name:       "successful update",
			updateName: "d/example",
			value:      `{"ip":"5.6.7.8"}`,
			opts:       nil,
			handler: func(method string, params json.RawMessage) (interface{}, *rpcError) {
				if method == "name_update" {
					return map[string]interface{}{
						"txid":   "tx123",
						"name":   "d/example",
						"value":  `{"ip":"5.6.7.8"}`,
						"status": "mempool",
					}, nil
				}
				return nil, &rpcError{Code: -32601, Message: "Method not found"}
			},
			wantErr:    false,
			wantTxHash: "tx123",
		},
		{
			name:       "name not found",
			updateName: "d/notexist",
			value:      `{}`,
			handler: func(method string, params json.RawMessage) (interface{}, *rpcError) {
				return nil, &rpcError{Code: -4, Message: "Name not found: d/notexist"}
			},
			wantErr: true,
		},
		{
			name:       "invalid name - empty",
			updateName: "",
			value:      `{}`,
			handler:    nil,
			wantErr:    true,
		},
		{
			name:       "invalid value - too long",
			updateName: "d/example",
			value:      string(make([]byte, 1024)), // > 1023 bytes
			handler:    nil,
			wantErr:    true,
		},
		{
			name:       "with transfer",
			updateName: "d/example",
			value:      `{"ip":"5.6.7.8"}`,
			opts: &UpdateOpts{
				TransferTo: "N1newaddress",
			},
			handler: func(method string, params json.RawMessage) (interface{}, *rpcError) {
				if method == "name_update" {
					// Verify params include address
					var p []string
					json.Unmarshal(params, &p)
					if len(p) < 3 || p[2] != "N1newaddress" {
						return nil, &rpcError{Code: -1, Message: "Expected transfer address"}
					}
					return map[string]interface{}{
						"txid":    "tx456",
						"name":    "d/example",
						"value":   `{"ip":"5.6.7.8"}`,
						"status":  "mempool",
						"address": "N1newaddress",
					}, nil
				}
				return nil, &rpcError{Code: -32601, Message: "Method not found"}
			},
			wantErr:    false,
			wantTxHash: "tx456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.handler != nil {
				server = mockRPCServer(t, tt.handler)
				defer server.Close()
			} else {
				server = mockRPCServer(t, func(method string, params json.RawMessage) (interface{}, *rpcError) {
					return nil, &rpcError{Code: -1, Message: "Unexpected call"}
				})
				defer server.Close()
			}

			client, err := NewDaemonClient(&Config{RPCAddr: server.URL})
			if err != nil {
				t.Fatalf("NewDaemonClient() error = %v", err)
			}
			defer client.Close()

			ctx := context.Background()
			result, err := client.UpdateName(ctx, tt.updateName, tt.value, tt.opts)

			if tt.wantErr {
				if err == nil {
					t.Errorf("UpdateName() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("UpdateName() unexpected error = %v", err)
				return
			}

			if result.TxHash != tt.wantTxHash {
				t.Errorf("UpdateName() txHash = %v, want %v", result.TxHash, tt.wantTxHash)
			}
		})
	}
}

func TestDaemonClient_ListNames(t *testing.T) {
	handler := func(method string, params json.RawMessage) (interface{}, *rpcError) {
		if method == "name_list" {
			return []map[string]interface{}{
				{
					"name":       "d/example1",
					"value":      `{}`,
					"txid":       "tx1",
					"height":     100,
					"expires_in": 5000,
					"address":    "N1a",
				},
				{
					"name":       "d/example2",
					"value":      `{}`,
					"txid":       "tx2",
					"height":     200,
					"expires_in": 4000,
					"address":    "N1b",
				},
				{
					"name":       "id/alice",
					"value":      `{}`,
					"txid":       "tx3",
					"height":     300,
					"expires_in": 3000,
					"address":    "N1a",
				},
				{
					"name":       "d/expired",
					"value":      `{}`,
					"txid":       "tx4",
					"height":     50,
					"expires_in": -100, // expired
					"address":    "N1c",
				},
			}, nil
		}
		return nil, &rpcError{Code: -32601, Message: "Method not found"}
	}

	tests := []struct {
		name      string
		filter    *ListFilter
		wantCount int
		wantFirst string
	}{
		{
			name:      "no filter",
			filter:    nil,
			wantCount: 3, // Excludes expired by default
			wantFirst: "d/example1",
		},
		{
			name: "namespace filter d/",
			filter: &ListFilter{
				Namespace: "d/",
			},
			wantCount: 2, // d/example1, d/example2 (not expired d/expired)
			wantFirst: "d/example1",
		},
		{
			name: "namespace filter id/",
			filter: &ListFilter{
				Namespace: "id/",
			},
			wantCount: 1,
			wantFirst: "id/alice",
		},
		{
			name: "address filter",
			filter: &ListFilter{
				Address: "N1a",
			},
			wantCount: 2, // d/example1 and id/alice
		},
		{
			name: "include expired",
			filter: &ListFilter{
				IncludeExpired: true,
			},
			wantCount: 4,
		},
		{
			name: "with limit",
			filter: &ListFilter{
				Limit: 1,
			},
			wantCount: 1,
		},
		{
			name: "with offset",
			filter: &ListFilter{
				Offset: 1,
			},
			wantCount: 2, // 3 total - 1 offset = 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockRPCServer(t, handler)
			defer server.Close()

			client, err := NewDaemonClient(&Config{RPCAddr: server.URL})
			if err != nil {
				t.Fatalf("NewDaemonClient() error = %v", err)
			}
			defer client.Close()

			ctx := context.Background()
			records, err := client.ListNames(ctx, tt.filter)
			if err != nil {
				t.Fatalf("ListNames() error = %v", err)
			}

			if len(records) != tt.wantCount {
				t.Errorf("ListNames() count = %v, want %v", len(records), tt.wantCount)
			}

			if tt.wantFirst != "" && len(records) > 0 && records[0].Name != tt.wantFirst {
				t.Errorf("ListNames() first name = %v, want %v", records[0].Name, tt.wantFirst)
			}
		})
	}
}

func TestDaemonClient_GetNameHistory(t *testing.T) {
	tests := []struct {
		name       string
		lookupName string
		handler    func(method string, params json.RawMessage) (interface{}, *rpcError)
		wantErr    bool
		wantCount  int
	}{
		{
			name:       "history found",
			lookupName: "d/example",
			handler: func(method string, params json.RawMessage) (interface{}, *rpcError) {
				if method == "name_history" {
					return []map[string]interface{}{
						{
							"name":       "d/example",
							"value":      `{"v":1}`,
							"txid":       "tx1",
							"height":     100,
							"expires_at": 36100,
							"address":    "N1a",
						},
						{
							"name":       "d/example",
							"value":      `{"v":2}`,
							"txid":       "tx2",
							"height":     200,
							"expires_at": 36200,
							"address":    "N1a",
						},
					}, nil
				}
				return nil, &rpcError{Code: -32601, Message: "Method not found"}
			},
			wantErr:   false,
			wantCount: 2,
		},
		{
			name:       "empty history",
			lookupName: "d/new",
			handler: func(method string, params json.RawMessage) (interface{}, *rpcError) {
				if method == "name_history" {
					return []map[string]interface{}{}, nil
				}
				return nil, &rpcError{Code: -32601, Message: "Method not found"}
			},
			wantErr:   false,
			wantCount: 0,
		},
		{
			name:       "empty name",
			lookupName: "",
			handler:    nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.handler != nil {
				server = mockRPCServer(t, tt.handler)
				defer server.Close()
			} else {
				server = mockRPCServer(t, func(method string, params json.RawMessage) (interface{}, *rpcError) {
					return nil, &rpcError{Code: -1, Message: "Unexpected call"}
				})
				defer server.Close()
			}

			client, err := NewDaemonClient(&Config{RPCAddr: server.URL})
			if err != nil {
				t.Fatalf("NewDaemonClient() error = %v", err)
			}
			defer client.Close()

			ctx := context.Background()
			records, err := client.GetNameHistory(ctx, tt.lookupName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetNameHistory() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetNameHistory() unexpected error = %v", err)
				return
			}

			if len(records) != tt.wantCount {
				t.Errorf("GetNameHistory() count = %v, want %v", len(records), tt.wantCount)
			}
		})
	}
}

func TestDaemonClient_GetInfo(t *testing.T) {
	handler := func(method string, params json.RawMessage) (interface{}, *rpcError) {
		switch method {
		case "getinfo":
			return map[string]interface{}{
				"version":     "0.1.0",
				"blocks":      12345,
				"connections": 8,
				"difficulty":  12345.67,
			}, nil
		case "getbestblockhash":
			return "0000000000000000abcdef1234567890abcdef1234567890abcdef1234567890", nil
		}
		return nil, &rpcError{Code: -32601, Message: "Method not found"}
	}

	server := mockRPCServer(t, handler)
	defer server.Close()

	client, err := NewDaemonClient(&Config{RPCAddr: server.URL})
	if err != nil {
		t.Fatalf("NewDaemonClient() error = %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	info, err := client.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo() error = %v", err)
	}

	if info.Version != "0.1.0" {
		t.Errorf("GetInfo() version = %v, want 0.1.0", info.Version)
	}
	if info.BlockHeight != 12345 {
		t.Errorf("GetInfo() blockHeight = %v, want 12345", info.BlockHeight)
	}
	if info.Connections != 8 {
		t.Errorf("GetInfo() connections = %v, want 8", info.Connections)
	}
	if info.Mode != "daemon" {
		t.Errorf("GetInfo() mode = %v, want daemon", info.Mode)
	}
}

func TestDaemonClient_Close(t *testing.T) {
	client, err := NewDaemonClient(nil)
	if err != nil {
		t.Fatalf("NewDaemonClient() error = %v", err)
	}

	// First close should succeed
	if err := client.Close(); err != nil {
		t.Errorf("Close() first call error = %v", err)
	}

	// Second close should be idempotent
	if err := client.Close(); err != nil {
		t.Errorf("Close() second call error = %v", err)
	}

	// Operations after close should fail
	ctx := context.Background()
	_, err = client.ResolveName(ctx, "d/test")
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Errorf("ResolveName() after close should fail with 'closed' error, got %v", err)
	}
}

func TestDaemonClient_ContextCancellation(t *testing.T) {
	// Create a handler that delays
	handler := func(method string, params json.RawMessage) (interface{}, *rpcError) {
		time.Sleep(500 * time.Millisecond)
		return map[string]interface{}{"version": "0.1.0"}, nil
	}

	server := mockRPCServer(t, handler)
	defer server.Close()

	client, err := NewDaemonClient(&Config{RPCAddr: server.URL})
	if err != nil {
		t.Fatalf("NewDaemonClient() error = %v", err)
	}
	defer client.Close()

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = client.ResolveName(ctx, "d/test")
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "context") {
		t.Errorf("ResolveName() with canceled context should fail, got %v", err)
	}
}

func TestDaemonClient_Authentication(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authentication
		user, pass, ok := r.BasicAuth()
		if !ok || user != "testuser" || pass != "testpass" {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Parse request to handle different methods
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Return success based on method
		resp := rpcResponse{
			Jsonrpc: "2.0",
			ID:      req.ID,
		}

		var resultBytes []byte
		switch req.Method {
		case "getinfo":
			resultBytes, _ = json.Marshal(map[string]interface{}{
				"version":     "0.1.0",
				"blocks":      100,
				"connections": 5,
			})
		case "getbestblockhash":
			resultBytes, _ = json.Marshal("0000000000000000000000000000000000000000000000000000000000000000")
		default:
			resultBytes, _ = json.Marshal(nil)
		}
		resp.Result = resultBytes
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	tests := []struct {
		name     string
		user     string
		password string
		wantErr  bool
	}{
		{
			name:     "correct credentials",
			user:     "testuser",
			password: "testpass",
			wantErr:  false,
		},
		{
			name:     "wrong username",
			user:     "wronguser",
			password: "testpass",
			wantErr:  true,
		},
		{
			name:     "wrong password",
			user:     "testuser",
			password: "wrongpass",
			wantErr:  true,
		},
		{
			name:     "no credentials",
			user:     "",
			password: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewDaemonClient(&Config{
				RPCAddr:     server.URL,
				RPCUser:     tt.user,
				RPCPassword: tt.password,
			})
			if err != nil {
				t.Fatalf("NewDaemonClient() error = %v", err)
			}
			defer client.Close()

			ctx := context.Background()
			err = client.Ping(ctx)

			if tt.wantErr && err == nil {
				t.Errorf("Ping() expected error for %s", tt.name)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Ping() unexpected error for %s: %v", tt.name, err)
			}
		})
	}
}

func TestDaemonClient_RetryConfig(t *testing.T) {
	client, err := NewDaemonClient(nil)
	if err != nil {
		t.Fatalf("NewDaemonClient() error = %v", err)
	}
	defer client.Close()

	// Set custom retry config
	client.SetRetryConfig(RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   1.5,
	})

	if client.retryConfig.MaxAttempts != 5 {
		t.Errorf("SetRetryConfig() MaxAttempts = %v, want 5", client.retryConfig.MaxAttempts)
	}
	if client.retryConfig.InitialDelay != 50*time.Millisecond {
		t.Errorf("SetRetryConfig() InitialDelay = %v, want 50ms", client.retryConfig.InitialDelay)
	}

	// Test default values for invalid config
	client.SetRetryConfig(RetryConfig{
		MaxAttempts:  -1,
		InitialDelay: -1,
		MaxDelay:     -1,
		Multiplier:   -1,
	})

	if client.retryConfig.MaxAttempts != 3 {
		t.Errorf("SetRetryConfig() with invalid MaxAttempts should use default 3, got %v", client.retryConfig.MaxAttempts)
	}
}

func TestDaemonClient_RegisterName(t *testing.T) {
	client, err := NewDaemonClient(nil)
	if err != nil {
		t.Fatalf("NewDaemonClient() error = %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// RegisterName is not yet supported in daemon mode
	_, err = client.RegisterName(ctx, "d/test", `{}`, nil)
	if err == nil || !strings.Contains(err.Error(), "not yet supported") {
		t.Errorf("RegisterName() should return 'not supported' error, got %v", err)
	}

	// Test validation still works
	_, err = client.RegisterName(ctx, "", `{}`, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Errorf("RegisterName() with empty name should fail with validation error, got %v", err)
	}
}

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "rpc error",
			err:  &rpcError{Code: -1, Message: "error"},
			want: false,
		},
		{
			name: "connection refused",
			err:  fmt.Errorf("connection refused"),
			want: true,
		},
		{
			name: "timeout",
			err:  fmt.Errorf("request timeout"),
			want: true,
		},
		{
			name: "EOF",
			err:  fmt.Errorf("unexpected EOF"),
			want: true,
		},
		{
			name: "connection reset",
			err:  fmt.Errorf("connection reset by peer"),
			want: true,
		},
		{
			name: "auth error",
			err:  fmt.Errorf("authentication failed"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransientError(tt.err)
			if got != tt.want {
				t.Errorf("isTransientError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestDaemonClient_WaitForConfirmation(t *testing.T) {
	tests := []struct {
		name          string
		txHash        string
		confirmations int
		handler       func(method string, params json.RawMessage) (interface{}, *rpcError)
		wantErr       bool
		errType       error
	}{
		{
			name:          "transaction already confirmed",
			txHash:        "abc123",
			confirmations: 1,
			handler: func(method string, params json.RawMessage) (interface{}, *rpcError) {
				if method == "getrawtransaction" {
					return map[string]interface{}{
						"txid":          "abc123",
						"confirmations": 1,
						"blockhash":     "block123",
						"blockheight":   1000,
					}, nil
				}
				return nil, &rpcError{Code: -32601, Message: "Method not found"}
			},
			wantErr: false,
		},
		{
			name:          "transaction confirmed after polling",
			txHash:        "def456",
			confirmations: 3,
			handler: func() func(method string, params json.RawMessage) (interface{}, *rpcError) {
				// Track call count and return increasing confirmations
				// Call 1: 0 confirmations (not yet confirmed)
				// Call 2: 1 confirmation (partially confirmed)
				// Call 3: 3 confirmations (fully confirmed - success!)
				callCount := 0
				expectedConfirmations := []int{0, 1, 3}
				
				return func(method string, params json.RawMessage) (interface{}, *rpcError) {
					if method == "getrawtransaction" {
						confs := 0
						if callCount < len(expectedConfirmations) {
							confs = expectedConfirmations[callCount]
						}
						callCount++
						
						return map[string]interface{}{
							"txid":          "def456",
							"confirmations": confs,
							"blockhash":     "block456",
							"blockheight":   2000,
						}, nil
					}
					return nil, &rpcError{Code: -32601, Message: "Method not found"}
				}
			}(),
			wantErr: false,
		},
		{
			name:          "transaction not found initially then confirmed",
			txHash:        "ghi789",
			confirmations: 1,
			handler: func() func(method string, params json.RawMessage) (interface{}, *rpcError) {
				callCount := 0
				return func(method string, params json.RawMessage) (interface{}, *rpcError) {
					if method == "getrawtransaction" {
						callCount++
						// First two calls: transaction not found
						if callCount <= 2 {
							return nil, &rpcError{Code: -5, Message: "Transaction not found"}
						}
						// Third call: transaction confirmed
						return map[string]interface{}{
							"txid":          "ghi789",
							"confirmations": 1,
							"blockhash":     "block789",
							"blockheight":   3000,
						}, nil
					}
					return nil, &rpcError{Code: -32601, Message: "Method not found"}
				}
			}(),
			wantErr: false,
		},
		{
			name:          "invalid confirmations count",
			txHash:        "jkl012",
			confirmations: 0,
			handler:       nil, // Won't be called
			wantErr:       true,
		},
		{
			name:          "negative confirmations count",
			txHash:        "mno345",
			confirmations: -1,
			handler:       nil, // Won't be called
			wantErr:       true,
		},
		{
			name:          "context canceled before confirmation",
			txHash:        "pqr678",
			confirmations: 1,
			handler: func(method string, params json.RawMessage) (interface{}, *rpcError) {
				if method == "getrawtransaction" {
					// Return 0 confirmations - will never reach 1
					return map[string]interface{}{
						"txid":          "pqr678",
						"confirmations": 0,
						"blockhash":     "blockpqr",
						"blockheight":   4000,
					}, nil
				}
				return nil, &rpcError{Code: -32601, Message: "Method not found"}
			},
			wantErr: true,
			errType: ErrContextCanceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For tests with handler, create mock server
			var client *DaemonClient
			if tt.handler != nil {
				server := mockRPCServer(t, tt.handler)
				defer server.Close()

				var err error
				client, err = NewDaemonClient(&Config{RPCAddr: server.URL})
				if err != nil {
					t.Fatalf("NewDaemonClient() error = %v", err)
				}
				defer client.Close()
			} else {
				// For validation tests (invalid confirmations), create client with dummy server
				server := mockRPCServer(t, func(method string, params json.RawMessage) (interface{}, *rpcError) {
					return nil, &rpcError{Code: -1, Message: "Unexpected call"}
				})
				defer server.Close()

				var err error
				client, err = NewDaemonClient(&Config{RPCAddr: server.URL})
				if err != nil {
					t.Fatalf("NewDaemonClient() error = %v", err)
				}
				defer client.Close()
			}

			ctx := context.Background()
			var cancel context.CancelFunc

			// For context cancellation test, use a short timeout
			if tt.errType == ErrContextCanceled {
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}

			err := client.WaitForConfirmation(ctx, tt.txHash, tt.confirmations)

			if tt.wantErr {
				if err == nil {
					t.Errorf("WaitForConfirmation() expected error but got nil")
				}
				if tt.errType != nil && !errors.Is(err, tt.errType) {
					t.Errorf("WaitForConfirmation() error = %v, want error type %v", err, tt.errType)
				}
			} else {
				if err != nil {
					t.Errorf("WaitForConfirmation() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestDaemonClient_getRawTransaction(t *testing.T) {
	tests := []struct {
		name              string
		txHash            string
		handler           func(method string, params json.RawMessage) (interface{}, *rpcError)
		wantErr           bool
		wantConfirmations int
		wantBlockHeight   int32
	}{
		{
			name:   "transaction found",
			txHash: "abc123",
			handler: func(method string, params json.RawMessage) (interface{}, *rpcError) {
				if method == "getrawtransaction" {
					return map[string]interface{}{
						"txid":          "abc123",
						"confirmations": 5,
						"blockhash":     "block123",
						"blockheight":   1000,
					}, nil
				}
				return nil, &rpcError{Code: -32601, Message: "Method not found"}
			},
			wantErr:           false,
			wantConfirmations: 5,
			wantBlockHeight:   1000,
		},
		{
			name:   "transaction not found",
			txHash: "notfound",
			handler: func(method string, params json.RawMessage) (interface{}, *rpcError) {
				if method == "getrawtransaction" {
					return nil, &rpcError{Code: -5, Message: "Transaction not found"}
				}
				return nil, &rpcError{Code: -32601, Message: "Method not found"}
			},
			wantErr: true,
		},
		{
			name:   "unconfirmed transaction",
			txHash: "unconfirmed",
			handler: func(method string, params json.RawMessage) (interface{}, *rpcError) {
				if method == "getrawtransaction" {
					return map[string]interface{}{
						"txid":          "unconfirmed",
						"confirmations": 0,
					}, nil
				}
				return nil, &rpcError{Code: -32601, Message: "Method not found"}
			},
			wantErr:           false,
			wantConfirmations: 0,
			wantBlockHeight:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockRPCServer(t, tt.handler)
			defer server.Close()

			client, err := NewDaemonClient(&Config{RPCAddr: server.URL})
			if err != nil {
				t.Fatalf("NewDaemonClient() error = %v", err)
			}
			defer client.Close()

			ctx := context.Background()
			info, err := client.getRawTransaction(ctx, tt.txHash)

			if tt.wantErr {
				if err == nil {
					t.Errorf("getRawTransaction() expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("getRawTransaction() unexpected error = %v", err)
				}
				if info == nil {
					t.Fatal("getRawTransaction() returned nil info")
				}
				if info.Confirmations != tt.wantConfirmations {
					t.Errorf("getRawTransaction() confirmations = %d, want %d", info.Confirmations, tt.wantConfirmations)
				}
				if info.BlockHeight != tt.wantBlockHeight {
					t.Errorf("getRawTransaction() blockheight = %d, want %d", info.BlockHeight, tt.wantBlockHeight)
				}
				if info.TxID != tt.txHash {
					t.Errorf("getRawTransaction() txid = %s, want %s", info.TxID, tt.txHash)
				}
			}
		})
	}
}
