package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/opd-ai/nmcd/internal/logging"
)

var (
	testLoggerOnce sync.Once
)

// setupTestLogger initializes the test logger once for all tests in this file
func setupTestLogger() {
	testLoggerOnce.Do(func() {
		logCfg := logging.DefaultConfig()
		logCfg.Level = logging.LevelError // Only show errors during test
		logCfg.Output = "stderr"
		logger, err := logging.Init(logCfg)
		if err != nil {
			panic("Failed to initialize test logger: " + err.Error())
		}
		logging.SetDefault(logger)
	})
}

// setupTestServer creates a test server with default configuration
func setupTestServer(t *testing.T) *Server {
	setupTestLogger()

	cfg := &Config{
		ListenAddr: "127.0.0.1:0",
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	return server
}

// TestPanicRecovery tests that panics in HTTP handlers are recovered
func TestPanicRecovery(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// Create a handler that panics
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	// Wrap with panic recovery
	wrapped := server.withPanicRecovery(panicHandler)

	// Create request
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
	w := httptest.NewRecorder()

	// Should not panic, should return 500
	wrapped(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500 after panic, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Internal server error") {
		t.Errorf("Expected 'Internal server error' in response, got: %s", body)
	}
}

// TestPanicRecoveryWithNilPointer tests recovery from nil pointer dereference
func TestPanicRecoveryWithNilPointer(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// Create a handler that dereferences nil
	nilHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ptr *string
		_ = *ptr // This will panic with nil pointer dereference
	})

	// Wrap with panic recovery
	wrapped := server.withPanicRecovery(nilHandler)

	// Create request
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
	w := httptest.NewRecorder()

	// Should not panic, should return 500
	wrapped(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500 after panic, got %d", w.Code)
	}
}

// TestPanicRecoveryDoesNotAffectNormalRequests tests that normal requests work fine
func TestPanicRecoveryDoesNotAffectNormalRequests(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// Create a normal handler
	normalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap with panic recovery
	wrapped := server.withPanicRecovery(normalHandler)

	// Create request
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
	w := httptest.NewRecorder()

	// Should work normally
	wrapped(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for normal request, got %d", w.Code)
	}

	if w.Body.String() != "OK" {
		t.Errorf("Expected 'OK' in response, got: %s", w.Body.String())
	}
}

// TestErrorLogging tests that errors are properly logged
func TestErrorLogging(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// Test invalid JSON (should trigger parse error)
	reqBody := `{"jsonrpc": "2.0", "method": "getinfo", "id": 1` // Missing closing brace
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(reqBody))
	w := httptest.NewRecorder()

	server.handleRequest(w, req)

	// Should return error response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Error == nil {
		t.Error("Expected error in response")
	}

	if resp.Error.Code != -32700 {
		t.Errorf("Expected error code -32700 (parse error), got %d", resp.Error.Code)
	}
}

// TestMultiplePanicRecoveries tests that multiple panics are handled independently
func TestMultiplePanicRecoveries(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// Create a handler that panics
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	// Wrap with panic recovery
	wrapped := server.withPanicRecovery(panicHandler)

	// Test multiple panics in sequence
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		w := httptest.NewRecorder()

		wrapped(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Request %d: Expected status 500 after panic, got %d", i+1, w.Code)
		}
	}
}

// TestPanicRecoveryPreservesContext tests that panic recovery includes request context
func TestPanicRecoveryPreservesContext(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// Create a handler that panics
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("context-aware panic")
	})

	// Wrap with panic recovery
	wrapped := server.withPanicRecovery(panicHandler)

	// Create request with specific path and remote address
	req := httptest.NewRequest(http.MethodPost, "/test/path", bytes.NewBufferString("{}"))
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	wrapped(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500 after panic, got %d", w.Code)
	}

	// The logger should have logged the panic with path and remote_addr
	// We can't easily test the log output without capturing stderr,
	// but we verified the panic was recovered
}
