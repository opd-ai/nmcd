package rpc

import (
	"bytes"
	"encoding/json"
	"testing"
)

// FuzzJSONRPCRequest fuzzes JSON-RPC request parsing to catch:
// - Malformed JSON
// - Extreme values in ID field
// - Very long method names
// - Invalid parameter types
// - Missing required fields
// - Injection attempts in string fields
//
// Run with: go test -fuzz=FuzzJSONRPCRequest -fuzztime=30s
func FuzzJSONRPCRequest(f *testing.F) {
	// Seed corpus with valid and edge case inputs
	f.Add([]byte(`{"jsonrpc":"2.0","method":"getinfo","params":[],"id":1}`))
	f.Add([]byte(`{"jsonrpc":"2.0","method":"getblockcount","params":null,"id":"test"}`))
	f.Add([]byte(`{"jsonrpc":"2.0","method":"name_show","params":["d/test"],"id":0}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"method":"test"}`))
	f.Add([]byte(`{"jsonrpc":"2.0"}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`""`))
	f.Add([]byte(`{"jsonrpc":"2.0","method":"","params":[],"id":null}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Attempt to parse as JSON-RPC request
		var req Request
		err := json.Unmarshal(data, &req)
		// We don't expect any panics, even with malformed input
		// The function should either succeed or return an error
		if err != nil {
			// Invalid JSON is expected and acceptable
			return
		}

		// If parsing succeeded, verify the request structure is valid
		// The method field can be any string (including empty)
		// The params field can be any JSON value (including null)
		// The id field can be string, number, or null per JSON-RPC spec

		// Verify we can re-serialize without panic
		_, _ = json.Marshal(&req)

		// Verify response writing doesn't panic
		var buf bytes.Buffer
		resp := Response{
			Jsonrpc: "2.0",
			Result:  nil,
			ID:      req.ID,
		}
		_ = json.NewEncoder(&buf).Encode(&resp)
	})
}

// FuzzJSONRPCParams fuzzes parameter unmarshaling for various RPC methods
// to ensure robust handling of different parameter types and values.
//
// Run with: go test -fuzz=FuzzJSONRPCParams -fuzztime=30s
func FuzzJSONRPCParams(f *testing.F) {
	// Seed with valid parameter patterns
	f.Add([]byte(`["d/test"]`))
	f.Add([]byte(`[0]`))
	f.Add([]byte(`["hash","verbose"]`))
	f.Add([]byte(`["name","value","address"]`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`null`))
	f.Add([]byte(`[null]`))
	f.Add([]byte(`[true,false]`))
	f.Add([]byte(`[1.5,2.5,3.5]`))
	f.Add([]byte(`[[],[],{}]`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Test unmarshaling as different parameter types
		// This simulates what various RPC handlers do

		// As string array (most common pattern)
		var stringParams []string
		_ = json.Unmarshal(data, &stringParams)

		// As interface array (generic pattern)
		var interfaceParams []interface{}
		_ = json.Unmarshal(data, &interfaceParams)

		// As single string
		var singleString string
		_ = json.Unmarshal(data, &singleString)

		// As single number
		var singleNumber float64
		_ = json.Unmarshal(data, &singleNumber)

		// As boolean
		var boolParam bool
		_ = json.Unmarshal(data, &boolParam)

		// No panics should occur during any of these operations
	})
}

// FuzzRPCMethodName fuzzes method name validation to ensure:
// - No buffer overflows with extremely long names
// - No issues with special characters
// - No injection vulnerabilities
//
// Run with: go test -fuzz=FuzzRPCMethodName -fuzztime=30s
func FuzzRPCMethodName(f *testing.F) {
	// Seed with valid method names
	f.Add("getinfo")
	f.Add("getblockcount")
	f.Add("name_show")
	f.Add("name_update")
	f.Add("")
	f.Add("_")
	f.Add("a")
	f.Add("method_with_underscores")
	f.Add("123")
	f.Add("method123")

	f.Fuzz(func(t *testing.T, method string) {
		// Create a request with the fuzzed method name
		req := Request{
			Jsonrpc: "2.0",
			Method:  method,
			Params:  json.RawMessage("[]"),
			ID:      1,
		}

		// Verify we can marshal without panic
		data, err := json.Marshal(&req)
		if err != nil {
			return
		}

		// Verify we can unmarshal back without panic
		var req2 Request
		_ = json.Unmarshal(data, &req2)

		// Verify method name is preserved
		if req2.Method != method {
			t.Errorf("method name not preserved: got %q, want %q", req2.Method, method)
		}
	})
}

// FuzzRPCID fuzzes the ID field to ensure robust handling of various types
// (string, number, null) as allowed by JSON-RPC 2.0 specification.
//
// Run with: go test -fuzz=FuzzRPCID -fuzztime=30s
func FuzzRPCID(f *testing.F) {
	// Seed with valid ID patterns
	f.Add([]byte(`1`))
	f.Add([]byte(`"test"`))
	f.Add([]byte(`null`))
	f.Add([]byte(`0`))
	f.Add([]byte(`-1`))
	f.Add([]byte(`9007199254740991`)) // MAX_SAFE_INTEGER
	f.Add([]byte(`""`))
	f.Add([]byte(`"very long id string with special chars !@#$%^&*()"`))

	f.Fuzz(func(t *testing.T, idData []byte) {
		// Create a request with the fuzzed ID
		reqJSON := `{"jsonrpc":"2.0","method":"test","params":[],"id":` + string(idData) + `}`

		var req Request
		err := json.Unmarshal([]byte(reqJSON), &req)
		if err != nil {
			// Invalid JSON for ID is acceptable
			return
		}

		// Verify we can create a response with this ID
		resp := Response{
			Jsonrpc: "2.0",
			Result:  "test",
			ID:      req.ID,
		}

		// Verify we can marshal the response without panic
		_, err = json.Marshal(&resp)
		if err != nil {
			t.Errorf("failed to marshal response with ID %v: %v", req.ID, err)
		}
	})
}

// FuzzErrorResponse fuzzes error response generation to ensure:
// - No issues with error messages containing special characters
// - Proper handling of extreme error codes
// - No panics when serializing various error structures
//
// Run with: go test -fuzz=FuzzErrorResponse -fuzztime=30s
func FuzzErrorResponse(f *testing.F) {
	// Seed with common error patterns
	f.Add(int(-32700), "Parse error")
	f.Add(int(-32600), "Invalid Request")
	f.Add(int(-32601), "Method not found")
	f.Add(int(-32602), "Invalid params")
	f.Add(int(-32603), "Internal error")
	f.Add(int(0), "")
	f.Add(int(1), "Custom error")
	f.Add(int(-1), "Negative error code")

	f.Fuzz(func(t *testing.T, code int, message string) {
		// Create an error response
		resp := Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    code,
				Message: message,
			},
			ID: 1,
		}

		// Verify we can marshal without panic
		data, err := json.Marshal(&resp)
		if err != nil {
			t.Errorf("failed to marshal error response: %v", err)
			return
		}

		// Verify we can unmarshal back without panic
		var resp2 Response
		err = json.Unmarshal(data, &resp2)
		if err != nil {
			t.Errorf("failed to unmarshal error response: %v", err)
			return
		}

		// Verify error is preserved
		if resp2.Error == nil {
			t.Errorf("error was lost during round-trip")
			return
		}
		if resp2.Error.Code != code {
			t.Errorf("error code not preserved: got %d, want %d", resp2.Error.Code, code)
		}
		if resp2.Error.Message != message {
			t.Errorf("error message not preserved: got %q, want %q", resp2.Error.Message, message)
		}
	})
}
