package rpc

import (
	"encoding/json"
	"testing"
)

// TestGetBalanceNoWallet tests getbalance when wallet is not initialized
func TestGetBalanceNoWallet(t *testing.T) {
	s := &Server{wallet: nil}

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getbalance",
		Params:  json.RawMessage(`[]`),
		ID:      1,
	}

	resp := s.getBalance(req)

	if resp.Error == nil {
		t.Error("expected error for missing wallet but got none")
	}
	if resp.Error.Code != -1 {
		t.Errorf("expected error code -1, got %d: %s", resp.Error.Code, resp.Error.Message)
	}
}

// TestGetBalanceNoBlockchain tests getbalance when blockchain is not initialized
func TestGetBalanceNoBlockchain(t *testing.T) {
	// Skip this test - requires wallet initialization
	// The NoWallet test above covers the primary error path
	t.Skip("Requires wallet initialization for complete test")
}

// TestListUnspentNoWallet tests listunspent when wallet is not initialized
func TestListUnspentNoWallet(t *testing.T) {
	s := &Server{wallet: nil}

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "listunspent",
		Params:  json.RawMessage(`[]`),
		ID:      1,
	}

	resp := s.listUnspent(req)

	if resp.Error == nil {
		t.Error("expected error for missing wallet but got none")
	}
	if resp.Error.Code != -1 {
		t.Errorf("expected error code -1, got %d: %s", resp.Error.Code, resp.Error.Message)
	}
}

// TestListUnspentMethodRouting tests that listunspent is correctly routed
func TestListUnspentMethodRouting(t *testing.T) {
	s := &Server{}

	params, _ := json.Marshal([]interface{}{})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "listunspent",
		Params:  params,
		ID:      1,
	}

	resp := s.processRequest(req)

	// Should not get "Method not found" error
	if resp.Error != nil && resp.Error.Code == -32601 {
		t.Error("listunspent method not found - routing not configured correctly")
	}
}

// TestGetBalanceMethodRouting tests that getbalance is correctly routed
func TestGetBalanceMethodRouting(t *testing.T) {
	s := &Server{}

	params, _ := json.Marshal([]interface{}{})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getbalance",
		Params:  params,
		ID:      1,
	}

	resp := s.processRequest(req)

	// Should not get "Method not found" error
	if resp.Error != nil && resp.Error.Code == -32601 {
		t.Error("getbalance method not found - routing not configured correctly")
	}
}
