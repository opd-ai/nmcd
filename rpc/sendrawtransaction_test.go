package rpc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/btcsuite/btcd/wire"
)

func TestSendRawTransactionInvalidParams(t *testing.T) {
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
			name:      "number instead of string",
			params:    []interface{}{12345},
			errorCode: -32602,
		},
		{
			name:      "invalid hex string",
			params:    []interface{}{"not_valid_hex"},
			errorCode: -22,
		},
		{
			name:      "valid hex but not a transaction",
			params:    []interface{}{"deadbeef"},
			errorCode: -22,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params, _ := json.Marshal(tc.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "sendrawtransaction",
				Params:  params,
				ID:      1,
			}

			resp := s.sendRawTransaction(req)

			if resp.Error == nil {
				t.Error("expected error but got none")
			}
			if resp.Error.Code != tc.errorCode {
				t.Errorf("expected error code %d, got %d: %s", tc.errorCode, resp.Error.Code, resp.Error.Message)
			}
		})
	}
}

func TestSendRawTransactionNoPeerManager(t *testing.T) {
	// Server without peer manager should fail
	s := &Server{}

	// Create a minimal valid transaction for testing
	// This transaction won't pass blockchain validation, but it's valid enough
	// for RPC deserialization testing - we only need to test the "no peer manager" error path
	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Index: 0,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})
	// Use a minimal script - full P2PKH validation is not the goal of this test
	tx.AddTxOut(&wire.TxOut{
		Value:    1000,
		PkScript: []byte{0x76, 0xa9, 0x14}, // Minimal script (OP_DUP OP_HASH160 OP_PUSHBYTES_20)
	})

	// Serialize the transaction using bytes.Buffer
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		t.Fatalf("failed to serialize tx: %v", err)
	}
	txHex := hex.EncodeToString(buf.Bytes())

	params, _ := json.Marshal([]interface{}{txHex})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "sendrawtransaction",
		Params:  params,
		ID:      1,
	}

	resp := s.sendRawTransaction(req)

	if resp.Error == nil {
		t.Error("expected error for missing peer manager but got none")
	}
	if resp.Error.Code != -1 {
		t.Errorf("expected error code -1 for network not available, got %d: %s", resp.Error.Code, resp.Error.Message)
	}
}

func TestSendRawTransactionMethodRouting(t *testing.T) {
	// Test that the method is correctly routed in processRequest
	s := &Server{}

	params, _ := json.Marshal([]interface{}{"deadbeef"})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "sendrawtransaction",
		Params:  params,
		ID:      1,
	}

	resp := s.processRequest(req)

	// Should not get "Method not found" error
	if resp.Error != nil && resp.Error.Code == -32601 {
		t.Error("sendrawtransaction method not found - routing not configured correctly")
	}
	// Should get a different error (invalid hex or tx decode error)
	if resp.Error == nil {
		t.Error("expected error for invalid transaction hex")
	}
	if resp.Error.Code != -22 {
		t.Errorf("expected error code -22 for TX decode failed, got %d", resp.Error.Code)
	}
}
