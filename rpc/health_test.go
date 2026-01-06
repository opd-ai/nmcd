package rpc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockBlockchainForHealth provides a minimal interface for health endpoint testing
// It's embedded in the Server struct for testing purposes
type mockBlockchainForHealth interface {
	BestSnapshot() interface{} // Returns a type with Height field
}

// mockPeerMgrForHealth provides a minimal interface for health endpoint testing
type mockPeerMgrForHealth interface {
	GetConnectedPeers() int
	IsSyncing() bool
}

func TestHandleHealth_Healthy(t *testing.T) {
	// Create a test server using the NewServer constructor with mux configured
	// We'll test via HTTP since the server struct has private fields
	
	// Create a minimal server for testing
	s := &Server{
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}
	
	// Set up HTTP mux like in NewServer
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Test /health endpoint when blockchain is nil (initializing)
	t.Run("Initializing", func(t *testing.T) {
		resp, err := http.Get(testServer.URL + "/health")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("Expected status 503 Service Unavailable, got %d", resp.StatusCode)
		}

		var healthResp HealthResponse
		if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
			t.Fatalf("Failed to decode JSON response: %v", err)
		}

		if healthResp.Status != "initializing" {
			t.Errorf("Expected status 'initializing', got '%s'", healthResp.Status)
		}
	})
}

func TestHandleHealth_MethodNotAllowed(t *testing.T) {
	s := &Server{
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	// Test POST request (should be rejected)
	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()

	s.handleHealth(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 Method Not Allowed, got %d", resp.StatusCode)
	}
}

func TestHandleReady_MethodNotAllowed(t *testing.T) {
	s := &Server{
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	// Test POST request (should be rejected)
	req := httptest.NewRequest(http.MethodPost, "/ready", nil)
	w := httptest.NewRecorder()

	s.handleReady(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 Method Not Allowed, got %d", resp.StatusCode)
	}
}

func TestHandleReady_Initializing(t *testing.T) {
	// Create a server without blockchain (initializing state)
	s := &Server{
		blockchain:     nil, // Not initialized
		peerMgr:        nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	s.handleReady(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Check status code - should be 503 Service Unavailable
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 Service Unavailable, got %d", resp.StatusCode)
	}

	// Parse JSON response
	var healthResp HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	// Verify response fields
	if healthResp.Status != "initializing" {
		t.Errorf("Expected status 'initializing', got '%s'", healthResp.Status)
	}
	if healthResp.Syncing != true {
		t.Errorf("Expected syncing true, got %v", healthResp.Syncing)
	}
}

// TestHealthEndpoint_ResponseStructure tests that health responses have correct JSON structure
func TestHealthEndpoint_ResponseStructure(t *testing.T) {
	s := &Server{
		blockchain:     nil,
		peerMgr:        nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	s.handleHealth(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	// Verify JSON can be decoded
	var healthResp HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	// Status field must be present
	if healthResp.Status == "" {
		t.Error("Status field should not be empty")
	}
}

// TestReadyEndpoint_ResponseStructure tests that ready responses have correct JSON structure
func TestReadyEndpoint_ResponseStructure(t *testing.T) {
	s := &Server{
		blockchain:     nil,
		peerMgr:        nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	s.handleReady(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	// Verify JSON can be decoded
	var healthResp HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	// Status field must be present
	if healthResp.Status == "" {
		t.Error("Status field should not be empty")
	}

	// When initializing, Syncing should be true
	if healthResp.Syncing != true {
		t.Error("Syncing field should be true when initializing")
	}
}

