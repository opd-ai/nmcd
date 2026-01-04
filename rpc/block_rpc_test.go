package rpc

import (
	"encoding/json"
	"testing"
)

func TestGetBlockHashInvalidParams(t *testing.T) {
	s := &Server{}

	tests := []struct {
		name      string
		params    interface{}
		errorCode int
	}{
		{
			name:      "no params",
			params:    []interface{}{},
			errorCode: -32602,
		},
		{
			name:      "string param instead of number",
			params:    []interface{}{"abc"},
			errorCode: -32602,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params, _ := json.Marshal(tc.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "getblockhash",
				Params:  params,
				ID:      1,
			}

			resp := s.getBlockHash(req)

			if resp.Error == nil {
				t.Error("expected error but got none")
			}
			if resp.Error.Code != tc.errorCode {
				t.Errorf("expected error code %d, got %d", tc.errorCode, resp.Error.Code)
			}
		})
	}
}

func TestGetBlockHashNegative(t *testing.T) {
	s := &Server{}

	params, _ := json.Marshal([]interface{}{-1})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getblockhash",
		Params:  params,
		ID:      1,
	}

	resp := s.getBlockHash(req)

	if resp.Error == nil {
		t.Error("expected error for negative height")
	}
	if resp.Error.Code != -8 {
		t.Errorf("expected error code -8, got %d", resp.Error.Code)
	}
}

func TestGetBlockInvalidParams(t *testing.T) {
	s := &Server{}

	tests := []struct {
		name      string
		params    interface{}
		errorCode int
	}{
		{
			name:      "no params",
			params:    []interface{}{},
			errorCode: -32602,
		},
		{
			name:      "number param instead of string",
			params:    []interface{}{123},
			errorCode: -32602,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params, _ := json.Marshal(tc.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "getblock",
				Params:  params,
				ID:      1,
			}

			resp := s.getBlock(req)

			if resp.Error == nil {
				t.Error("expected error but got none")
			}
			if resp.Error.Code != tc.errorCode {
				t.Errorf("expected error code %d, got %d", tc.errorCode, resp.Error.Code)
			}
		})
	}
}

func TestGetBlockInvalidHash(t *testing.T) {
	s := &Server{}

	params, _ := json.Marshal([]interface{}{"invalid_hash", false})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getblock",
		Params:  params,
		ID:      1,
	}

	resp := s.getBlock(req)

	if resp.Error == nil {
		t.Error("expected error for invalid hash")
	}
	if resp.Error.Code != -5 {
		t.Errorf("expected error code -5, got %d", resp.Error.Code)
	}
}

func TestGetRawTransactionInvalidParams(t *testing.T) {
	s := &Server{}

	tests := []struct {
		name      string
		params    interface{}
		errorCode int
	}{
		{
			name:      "no params",
			params:    []interface{}{},
			errorCode: -32602,
		},
		{
			name:      "number param instead of string",
			params:    []interface{}{123},
			errorCode: -32602,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params, _ := json.Marshal(tc.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "getrawtransaction",
				Params:  params,
				ID:      1,
			}

			resp := s.getRawTransaction(req)

			if resp.Error == nil {
				t.Error("expected error but got none")
			}
			if resp.Error.Code != tc.errorCode {
				t.Errorf("expected error code %d, got %d", tc.errorCode, resp.Error.Code)
			}
		})
	}
}

func TestGetRawTransactionInvalidTxid(t *testing.T) {
	s := &Server{}

	params, _ := json.Marshal([]interface{}{"invalid_txid", false})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getrawtransaction",
		Params:  params,
		ID:      1,
	}

	resp := s.getRawTransaction(req)

	if resp.Error == nil {
		t.Error("expected error for invalid txid")
	}
	if resp.Error.Code != -5 {
		t.Errorf("expected error code -5, got %d", resp.Error.Code)
	}
}

// TestProcessRequestNewMethods verifies that the new RPC methods are registered
func TestProcessRequestNewMethods(t *testing.T) {
	s := &Server{}

	methods := []string{"getblock", "getblockhash", "getrawtransaction"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := &Request{
				Jsonrpc: "2.0",
				Method:  method,
				Params:  json.RawMessage(`[]`),
				ID:      1,
			}

			resp := s.processRequest(req)

			// Should not get "Method not found" error
			if resp.Error != nil && resp.Error.Code == -32601 {
				t.Errorf("method %s not found in processRequest", method)
			}

			// We expect a different error (invalid params) since we passed empty params
			// This proves the method is registered
			if resp.Error != nil && resp.Error.Code != -32601 {
				// This is expected - method exists but params are invalid
			}
		})
	}
}

// TestGetBlockVerboseParamValidation tests that verbose parameter must be boolean
func TestGetBlockVerboseParamValidation(t *testing.T) {
s := &Server{}

tests := []struct {
name      string
params    interface{}
errorCode int
}{
{
name:      "verbose as string",
params:    []interface{}{"0000000000000000000000000000000000000000000000000000000000000001", "true"},
errorCode: -32602,
},
{
name:      "verbose as number",
params:    []interface{}{"0000000000000000000000000000000000000000000000000000000000000001", 1},
errorCode: -32602,
},
}

for _, tc := range tests {
t.Run(tc.name, func(t *testing.T) {
params, _ := json.Marshal(tc.params)
req := &Request{
Jsonrpc: "2.0",
Method:  "getblock",
Params:  params,
ID:      1,
}

resp := s.getBlock(req)

if resp.Error == nil {
t.Error("expected error but got none")
}
if resp.Error.Code != tc.errorCode {
t.Errorf("expected error code %d, got %d", tc.errorCode, resp.Error.Code)
}
if resp.Error.Message != "Invalid params: verbose must be a boolean" {
t.Errorf("unexpected error message: %s", resp.Error.Message)
}
})
}
}

// TestGetRawTransactionVerboseParamValidation tests that verbose parameter must be boolean
func TestGetRawTransactionVerboseParamValidation(t *testing.T) {
s := &Server{}

tests := []struct {
name      string
params    interface{}
errorCode int
}{
{
name:      "verbose as string",
params:    []interface{}{"0000000000000000000000000000000000000000000000000000000000000001", "true"},
errorCode: -32602,
},
{
name:      "verbose as number",
params:    []interface{}{"0000000000000000000000000000000000000000000000000000000000000001", 1},
errorCode: -32602,
},
}

for _, tc := range tests {
t.Run(tc.name, func(t *testing.T) {
params, _ := json.Marshal(tc.params)
req := &Request{
Jsonrpc: "2.0",
Method:  "getrawtransaction",
Params:  params,
ID:      1,
}

resp := s.getRawTransaction(req)

if resp.Error == nil {
t.Error("expected error but got none")
}
if resp.Error.Code != tc.errorCode {
t.Errorf("expected error code %d, got %d", tc.errorCode, resp.Error.Code)
}
if resp.Error.Message != "Invalid params: verbose must be a boolean" {
t.Errorf("unexpected error message: %s", resp.Error.Message)
}
})
}
}

// TestGetBlockHashOverflow tests that height overflow is detected
func TestGetBlockHashOverflow(t *testing.T) {
s := &Server{}

tests := []struct {
name      string
height    float64
errorCode int
}{
{
name:      "overflow positive",
height:    3000000000.0,
errorCode: -32602,
},
{
name:      "overflow negative",
height:    -3000000000.0,
errorCode: -32602,
},
}

for _, tc := range tests {
t.Run(tc.name, func(t *testing.T) {
params, _ := json.Marshal([]interface{}{tc.height})
req := &Request{
Jsonrpc: "2.0",
Method:  "getblockhash",
Params:  params,
ID:      1,
}

resp := s.getBlockHash(req)

if resp.Error == nil {
t.Error("expected error but got none")
}
if resp.Error.Code != tc.errorCode {
t.Errorf("expected error code %d, got %d", tc.errorCode, resp.Error.Code)
}
})
}
}
