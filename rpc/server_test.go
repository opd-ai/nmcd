package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/opd-ai/nmcd/wallet"
)

// testServer creates a minimal Server for testing authentication.
// The blockchain and peerMgr are nil since we only test auth logic.
func testServer(user, pass string) *Server {
	return &Server{
		rpcUser:        user,
		rpcPassword:    pass,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}
}

func TestCheckAuth(t *testing.T) {
	tests := []struct {
		name          string
		serverUser    string
		serverPass    string
		requestUser   string
		requestPass   string
		setBasicAuth  bool
		expectedValid bool
	}{
		{
			name:          "valid credentials",
			serverUser:    "testuser",
			serverPass:    "testpass",
			requestUser:   "testuser",
			requestPass:   "testpass",
			setBasicAuth:  true,
			expectedValid: true,
		},
		{
			name:          "invalid username",
			serverUser:    "testuser",
			serverPass:    "testpass",
			requestUser:   "wronguser",
			requestPass:   "testpass",
			setBasicAuth:  true,
			expectedValid: false,
		},
		{
			name:          "invalid password",
			serverUser:    "testuser",
			serverPass:    "testpass",
			requestUser:   "testuser",
			requestPass:   "wrongpass",
			setBasicAuth:  true,
			expectedValid: false,
		},
		{
			name:          "no auth header",
			serverUser:    "testuser",
			serverPass:    "testpass",
			requestUser:   "",
			requestPass:   "",
			setBasicAuth:  false,
			expectedValid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer(tc.serverUser, tc.serverPass)

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			if tc.setBasicAuth {
				req.SetBasicAuth(tc.requestUser, tc.requestPass)
			}

			result := s.checkAuth(req)
			if result != tc.expectedValid {
				t.Errorf("checkAuth() = %v, want %v", result, tc.expectedValid)
			}
		})
	}
}

func TestHandleRequestAuth(t *testing.T) {
	tests := []struct {
		name           string
		serverUser     string
		serverPass     string
		requestUser    string
		requestPass    string
		setBasicAuth   bool
		expectedStatus int
	}{
		{
			name:           "auth configured - valid credentials",
			serverUser:     "admin",
			serverPass:     "secret",
			requestUser:    "admin",
			requestPass:    "secret",
			setBasicAuth:   true,
			expectedStatus: http.StatusOK, // Request processed successfully
		},
		{
			name:           "auth configured - no credentials provided",
			serverUser:     "admin",
			serverPass:     "secret",
			requestUser:    "",
			requestPass:    "",
			setBasicAuth:   false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "auth configured - wrong credentials",
			serverUser:     "admin",
			serverPass:     "secret",
			requestUser:    "hacker",
			requestPass:    "guess",
			setBasicAuth:   true,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "only user configured - auth not enforced (need both)",
			serverUser:     "admin",
			serverPass:     "",
			requestUser:    "",
			requestPass:    "",
			setBasicAuth:   false,
			expectedStatus: http.StatusOK, // Auth not enforced when only one credential is set
		},
		{
			name:           "only password configured - auth not enforced (need both)",
			serverUser:     "",
			serverPass:     "secret",
			requestUser:    "",
			requestPass:    "",
			setBasicAuth:   false,
			expectedStatus: http.StatusOK, // Auth not enforced when only one credential is set
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer(tc.serverUser, tc.serverPass)

			// Create a valid JSON-RPC request body
			rpcReq := Request{
				Jsonrpc: "2.0",
				Method:  "unknown_method", // Use unknown method to avoid nil pointer dereference
				ID:      1,
			}
			body, _ := json.Marshal(rpcReq)

			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tc.setBasicAuth {
				req.SetBasicAuth(tc.requestUser, tc.requestPass)
			}

			rr := httptest.NewRecorder()
			s.handleRequest(rr, req)

			if rr.Code != tc.expectedStatus {
				t.Errorf("handleRequest() status = %d, want %d", rr.Code, tc.expectedStatus)
			}

			// Check WWW-Authenticate header on 401
			if tc.expectedStatus == http.StatusUnauthorized {
				authHeader := rr.Header().Get("WWW-Authenticate")
				if authHeader == "" {
					t.Error("Expected WWW-Authenticate header on 401 response")
				}
			}
		})
	}
}

func TestHandleRequestMethodNotAllowed(t *testing.T) {
	s := testServer("", "")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	s.handleRequest(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleRequest() status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestGetNewAddressNoWallet(t *testing.T) {
	s := &Server{}
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getnewaddress",
		ID:      1,
	}

	resp := s.getNewAddress(req)

	if resp.Error == nil {
		t.Error("expected error when wallet is nil")
	}
	if resp.Error.Code != -1 {
		t.Errorf("expected error code -1, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "Wallet not initialized. Start the node with wallet enabled." {
		t.Errorf("unexpected error message: %s", resp.Error.Message)
	}
}

func TestListAddressesNoWallet(t *testing.T) {
	s := &Server{}
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "listaddresses",
		ID:      1,
	}

	resp := s.listAddresses(req)

	if resp.Error == nil {
		t.Error("expected error when wallet is nil")
	}
	if resp.Error.Code != -1 {
		t.Errorf("expected error code -1, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "Wallet not initialized. Start the node with wallet enabled." {
		t.Errorf("unexpected error message: %s", resp.Error.Message)
	}
}

func TestGetNewAddressWithWallet(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := wallet.NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	s := &Server{wallet: w}
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getnewaddress",
		ID:      1,
	}

	resp := s.getNewAddress(req)

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Error("expected result, got nil")
	}

	address, ok := resp.Result.(string)
	if !ok {
		t.Errorf("expected string result, got %T", resp.Result)
	}
	if address == "" {
		t.Error("expected non-empty address")
	}

	// Verify the address was added to the wallet
	if !w.HasKey(address) {
		t.Error("generated address not found in wallet")
	}
}

func TestListAddressesWithWallet(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := wallet.NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	s := &Server{wallet: w}

	// Test with empty wallet
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "listaddresses",
		ID:      1,
	}

	resp := s.listAddresses(req)
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	addresses, ok := resp.Result.([]string)
	if !ok {
		t.Errorf("expected []string result, got %T", resp.Result)
	}
	if len(addresses) != 0 {
		t.Errorf("expected empty list, got %d addresses", len(addresses))
	}

	// Generate some addresses and verify they appear in the list
	addr1, err := w.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	addr2, err := w.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	resp = s.listAddresses(req)
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	addresses, ok = resp.Result.([]string)
	if !ok {
		t.Errorf("expected []string result, got %T", resp.Result)
	}
	if len(addresses) != 2 {
		t.Errorf("expected 2 addresses, got %d", len(addresses))
	}

	// Verify both addresses are present
	found := make(map[string]bool)
	for _, addr := range addresses {
		found[addr] = true
	}
	if !found[addr1] {
		t.Errorf("address %s not found in list", addr1)
	}
	if !found[addr2] {
		t.Errorf("address %s not found in list", addr2)
	}
}
