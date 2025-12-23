package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// testServer creates a minimal Server for testing authentication.
// The blockchain and peerMgr are nil since we only test auth logic.
func testServer(user, pass string) *Server {
	return &Server{
		rpcUser:     user,
		rpcPassword: pass,
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
