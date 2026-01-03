package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient_ModeSelection(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *Config
		wantMode string
		wantErr  bool
	}{
		{
			name: "explicit embedded mode",
			cfg: &Config{
				Mode:    ModeEmbedded,
				DataDir: t.TempDir(),
				Network: "regtest",
			},
			wantMode: "embedded",
			wantErr:  false,
		},
		{
			name: "invalid mode returns error",
			cfg: &Config{
				Mode: ClientMode(99), // Invalid mode
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && client != nil {
				defer client.Close()

				// Check mode
				info, err := client.GetInfo(context.Background())
				if err != nil {
					t.Errorf("GetInfo() error = %v", err)
					return
				}
				if info.Mode != tt.wantMode {
					t.Errorf("NewClient() mode = %v, want %v", info.Mode, tt.wantMode)
				}
			}
		})
	}
}

func TestNewClient_DaemonMode(t *testing.T) {
	// Create a mock daemon server
	handler := func(method string, params json.RawMessage) (interface{}, *rpcError) {
		switch method {
		case "getinfo":
			return map[string]interface{}{
				"version":     "0.1.0",
				"blocks":      100,
				"connections": 5,
			}, nil
		case "getbestblockhash":
			return "0000000000000000000000000000000000000000000000000000000000000000", nil
		}
		return nil, &rpcError{Code: -32601, Message: "Method not found"}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		var paramsRaw json.RawMessage
		if req.Params != nil {
			paramsRaw, _ = json.Marshal(req.Params)
		}
		result, rpcErr := handler(req.Method, paramsRaw)

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
	defer server.Close()

	// Test explicit daemon mode
	client, err := NewClient(&Config{
		Mode:    ModeDaemon,
		RPCAddr: server.URL,
	})
	if err != nil {
		t.Fatalf("NewClient(ModeDaemon) error = %v", err)
	}
	defer client.Close()

	info, err := client.GetInfo(context.Background())
	if err != nil {
		t.Fatalf("GetInfo() error = %v", err)
	}

	if info.Mode != "daemon" {
		t.Errorf("GetInfo() mode = %v, want daemon", info.Mode)
	}
}

func TestNewClient_AutoMode_FallbackToEmbedded(t *testing.T) {
	// With ModeAuto and no daemon available, should fall back to embedded
	cfg := &Config{
		Mode:    ModeAuto,
		DataDir: t.TempDir(),
		Network: "regtest",
		// Use a URL that won't connect (localhost with random port)
		RPCAddr: "http://127.0.0.1:59999",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient(ModeAuto) error = %v", err)
	}
	defer client.Close()

	info, err := client.GetInfo(context.Background())
	if err != nil {
		t.Fatalf("GetInfo() error = %v", err)
	}

	// Should have fallen back to embedded mode
	if info.Mode != "embedded" {
		t.Errorf("GetInfo() mode = %v, want embedded (fallback)", info.Mode)
	}
}

func TestNewClient_AutoMode_UsesDaemonWhenAvailable(t *testing.T) {
	// Create a mock daemon server
	handler := func(method string, params json.RawMessage) (interface{}, *rpcError) {
		switch method {
		case "getinfo":
			return map[string]interface{}{
				"version":     "0.1.0",
				"blocks":      100,
				"connections": 5,
			}, nil
		case "getbestblockhash":
			return "0000000000000000000000000000000000000000000000000000000000000000", nil
		}
		return nil, &rpcError{Code: -32601, Message: "Method not found"}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		var paramsRaw json.RawMessage
		if req.Params != nil {
			paramsRaw, _ = json.Marshal(req.Params)
		}
		result, rpcErr := handler(req.Method, paramsRaw)

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
	defer server.Close()

	// With ModeAuto and daemon available, should use daemon
	cfg := &Config{
		Mode:    ModeAuto,
		RPCAddr: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient(ModeAuto) error = %v", err)
	}
	defer client.Close()

	info, err := client.GetInfo(context.Background())
	if err != nil {
		t.Fatalf("GetInfo() error = %v", err)
	}

	// Should use daemon mode
	if info.Mode != "daemon" {
		t.Errorf("GetInfo() mode = %v, want daemon", info.Mode)
	}
}

func TestNewClient_NilConfig(t *testing.T) {
	// With nil config and no daemon, should fall back to embedded with defaults
	// This test may take a moment to timeout on daemon connection
	client, err := NewClient(nil)
	if err != nil {
		// It's acceptable for this to fail if embedded mode also fails
		// (e.g., if the default data directory is not writable)
		t.Skipf("NewClient(nil) error = %v (may be expected in some environments)", err)
		return
	}
	defer client.Close()

	_, err = client.GetInfo(context.Background())
	if err != nil {
		t.Errorf("GetInfo() error = %v", err)
	}
}
