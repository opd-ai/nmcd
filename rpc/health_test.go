package rpc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/opd-ai/nmcd/chain"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/network"
)

// createTestBlockchain creates a real blockchain instance for testing
// This is needed because the Server struct uses concrete types
func createTestBlockchain(t *testing.T) *chain.BlockChain {
	t.Helper()

	// Create temporary directory for test database
	tmpDir := t.TempDir()

	cfg := &chain.Config{
		ChainParams: &config.NamecoinRegTestParams,
		NameDBPath:  tmpDir + "/names.db",
		DataDir:     tmpDir,
	}

	bc, err := chain.NewBlockChain(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create test blockchain: %v", err)
	}

	// Register cleanup
	t.Cleanup(func() {
		bc.Close()
	})

	return bc
}

// createTestPeerManager creates a real peer manager instance for testing
func createTestPeerManager(t *testing.T, bc *chain.BlockChain) *network.PeerManager {
	t.Helper()

	cfg := &network.Config{
		ChainParams: &config.NamecoinRegTestParams,
		Blockchain:  bc,
		ListenAddrs: []string{}, // Don't listen on any ports for tests
		MaxPeers:    10,
	}

	pm, err := network.NewPeerManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create test peer manager: %v", err)
	}

	// Register cleanup
	t.Cleanup(func() {
		pm.Stop()
	})

	return pm
}

func TestHandleHealth_Initializing(t *testing.T) {
	// Create a server without blockchain (initializing state)
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
}

func TestHandleHealth_Healthy(t *testing.T) {
	// Create a server with initialized blockchain
	bc := createTestBlockchain(t)
	pm := createTestPeerManager(t, bc)

	s := &Server{
		blockchain:     bc,
		peerMgr:        pm,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	s.handleHealth(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	// Parse JSON response
	var healthResp HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	// Verify response fields
	if healthResp.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", healthResp.Status)
	}
	// Block height should be 0 for a fresh blockchain
	if healthResp.BlockHeight != 0 {
		t.Errorf("Expected block_height 0, got %d", healthResp.BlockHeight)
	}
	// Peers should be 0 since we didn't add any
	if healthResp.Peers != 0 {
		t.Errorf("Expected peers 0, got %d", healthResp.Peers)
	}
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

func TestHandleReady_Ready(t *testing.T) {
	// Create a server that is fully synced and ready
	// A fresh blockchain with no peers is considered "ready" (not syncing)
	bc := createTestBlockchain(t)
	pm := createTestPeerManager(t, bc)

	// Give it a moment to initialize
	time.Sleep(100 * time.Millisecond)

	s := &Server{
		blockchain:     bc,
		peerMgr:        pm,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	s.handleReady(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Check status code - should be 200 OK (not syncing with no peers)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	// Parse JSON response
	var healthResp HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	// Verify response fields
	if healthResp.Status != "ready" {
		t.Errorf("Expected status 'ready', got '%s'", healthResp.Status)
	}
	if healthResp.BlockHeight != 0 {
		t.Errorf("Expected block_height 0, got %d", healthResp.BlockHeight)
	}
	if healthResp.Peers != 0 {
		t.Errorf("Expected peers 0, got %d", healthResp.Peers)
	}
	if healthResp.Syncing != false {
		t.Errorf("Expected syncing false, got %v", healthResp.Syncing)
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
