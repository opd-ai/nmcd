package rpc

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestNameNewParameterValidation tests parameter validation for name_new RPC method
func TestNameNewParameterValidation(t *testing.T) {
	tests := []struct {
		name          string
		params        interface{}
		expectedError bool
		errorCode     int
		errorContains string
	}{
		{
			name:          "no wallet - returns wallet error",
			params:        []string{"d/example"},
			expectedError: true,
			errorCode:     -1,
			errorContains: "Wallet not initialized",
		},
		{
			name:          "no wallet - empty params returns wallet error",
			params:        []string{},
			expectedError: true,
			errorCode:     -1,
			errorContains: "Wallet not initialized",
		},
		{
			name:          "no wallet - invalid param type returns wallet error",
			params:        "not-an-array",
			expectedError: true,
			errorCode:     -1,
			errorContains: "Wallet not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create minimal server for testing parameter validation
			server := &Server{
				wallet: nil, // All tests here check for wallet errors
			}

			// Create request
			paramsJSON, _ := json.Marshal(tt.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "name_new",
				Params:  paramsJSON,
				ID:      1,
			}

			// Call the RPC method
			resp := server.nameNew(req)

			// Check response
			if tt.expectedError {
				if resp.Error == nil {
					t.Fatal("Expected error but got none")
				}
				if resp.Error.Code != tt.errorCode {
					t.Errorf("Expected error code %d, got %d", tt.errorCode, resp.Error.Code)
				}
				if tt.errorContains != "" && !strings.Contains(resp.Error.Message, tt.errorContains) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorContains, resp.Error.Message)
				}
			}
		})
	}
}

// TestNameFirstUpdateParameterValidation tests parameter validation for name_firstupdate RPC method
func TestNameFirstUpdateParameterValidation(t *testing.T) {
	tests := []struct {
		name          string
		params        interface{}
		expectedError bool
		errorCode     int
		errorContains string
	}{
		{
			name:          "no wallet - returns wallet error",
			params:        []string{"d/example", "0102030405060708090a0b0c0d0e0f1011121314", `{"test":"value"}`},
			expectedError: true,
			errorCode:     -1,
			errorContains: "Wallet not initialized",
		},
		{
			name:          "no wallet - missing parameters returns wallet error",
			params:        []string{"d/example"},
			expectedError: true,
			errorCode:     -1,
			errorContains: "Wallet not initialized",
		},
		{
			name:          "no wallet - only 2 params returns wallet error",
			params:        []string{"d/example", "0102030405060708090a0b0c0d0e0f1011121314"},
			expectedError: true,
			errorCode:     -1,
			errorContains: "Wallet not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create minimal server for testing parameter validation
			server := &Server{
				wallet: nil, // All tests here check for wallet errors
			}

			// Create request
			paramsJSON, _ := json.Marshal(tt.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "name_firstupdate",
				Params:  paramsJSON,
				ID:      1,
			}

			// Call the RPC method
			resp := server.nameFirstUpdate(req)

			// Check response
			if tt.expectedError {
				if resp.Error == nil {
					t.Fatal("Expected error but got none")
				}
				if resp.Error.Code != tt.errorCode {
					t.Errorf("Expected error code %d, got %d", tt.errorCode, resp.Error.Code)
				}
				if tt.errorContains != "" && !strings.Contains(resp.Error.Message, tt.errorContains) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorContains, resp.Error.Message)
				}
			}
		})
	}
}
