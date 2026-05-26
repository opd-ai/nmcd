package rpc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/namedb"
)

// buildMinimalTx builds a syntactically valid but semantically trivial transaction
// sufficient for RPC serialization and mempool tests.
func buildMinimalTx() *wire.MsgTx {
	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 0},
		Sequence:         wire.MaxTxInSequenceNum,
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    1000,
		PkScript: []byte{0x51}, // OP_1 (trivially spendable)
	})
	return tx
}

// serializeTxHex serializes a wire.MsgTx to a hex string for use in RPC params.
func serializeTxHex(t *testing.T, tx *wire.MsgTx) string {
	t.Helper()
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		t.Fatalf("failed to serialize tx: %v", err)
	}
	return hex.EncodeToString(buf.Bytes())
}

// TestGetBlockVerboseMode tests getBlock with verbose=true using the genesis block.
func TestGetBlockVerboseMode(t *testing.T) {
	server, bc, _ := setupNameHandlerTestServer(t)

	// Use the best block hash from the live blockchain (regtest starts at height 0)
	best := bc.BestSnapshot()
	bestHash := best.Hash.String()

	params, _ := json.Marshal([]interface{}{bestHash, true})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getblock",
		Params:  params,
		ID:      1,
	}

	resp := server.getBlock(req)

	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got %T", resp.Result)
	}

	if result["hash"] != bestHash {
		t.Errorf("Expected hash %s, got %v", bestHash, result["hash"])
	}
	if _, ok := result["tx"]; !ok {
		t.Error("Expected 'tx' field in verbose block result")
	}
	if _, ok := result["merkleroot"]; !ok {
		t.Error("Expected 'merkleroot' field in verbose block result")
	}
	if _, ok := result["time"]; !ok {
		t.Error("Expected 'time' field in verbose block result")
	}
}

// TestGetBlockNonVerboseMode tests getBlock with verbose=false (hex output).
func TestGetBlockNonVerboseMode(t *testing.T) {
	server, bc, _ := setupNameHandlerTestServer(t)

	best := bc.BestSnapshot()
	bestHash := best.Hash.String()

	params, _ := json.Marshal([]interface{}{bestHash, false})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getblock",
		Params:  params,
		ID:      1,
	}

	resp := server.getBlock(req)

	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}

	hexStr, ok := resp.Result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", resp.Result)
	}
	if len(hexStr) == 0 {
		t.Error("Expected non-empty hex string for block data")
	}
}

// TestGetRawTransactionNotFound tests getRawTransaction with a hash not in any block.
func TestGetRawTransactionNotFound(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	// Use a hash that cannot possibly be found
	fakeTxid := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	params, _ := json.Marshal([]interface{}{fakeTxid, false})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getrawtransaction",
		Params:  params,
		ID:      1,
	}

	resp := server.getRawTransaction(req)

	if resp.Error == nil {
		t.Fatal("Expected error for nonexistent transaction")
	}
	if resp.Error.Code != -5 {
		t.Errorf("Expected error code -5, got %d: %s", resp.Error.Code, resp.Error.Message)
	}
}

// TestGetRawTransactionNilBlockchain tests getRawTransaction when blockchain is nil.
func TestGetRawTransactionNilBlockchain(t *testing.T) {
	s := &Server{}

	params, _ := json.Marshal([]interface{}{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getrawtransaction",
		Params:  params,
		ID:      1,
	}

	resp := s.getRawTransaction(req)

	if resp.Error == nil {
		t.Fatal("Expected error for nil blockchain")
	}
	if resp.Error.Code != -32603 {
		t.Errorf("Expected error code -32603, got %d", resp.Error.Code)
	}
}

// TestCalculateConfirmations tests the calculateConfirmations helper directly.
func TestCalculateConfirmations(t *testing.T) {
	s := &Server{}

	tests := []struct {
		name       string
		utxoHeight int32
		bestHeight int32
		want       int
	}{
		{"confirmed at tip", 10, 10, 1},
		{"1 conf below tip", 9, 10, 2},
		{"unconfirmed (height 0)", 0, 100, 0},
		{"genesis block", 1, 100, 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := s.calculateConfirmations(tc.utxoHeight, tc.bestHeight)
			if got != tc.want {
				t.Errorf("calculateConfirmations(%d, %d) = %d, want %d",
					tc.utxoHeight, tc.bestHeight, got, tc.want)
			}
		})
	}
}

// TestBuildUTXOResult tests buildUTXOResult filtering and field construction.
func TestBuildUTXOResult(t *testing.T) {
	s := &Server{}

	txHash, _ := chainhash.NewHashFromStr("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	utxo := &namedb.UTXO{
		TxHash:   *txHash,
		OutIndex: 0,
		Value:    100000000, // 1 NMC
		Address:  "testaddr",
		PkScript: []byte{0x76, 0xa9},
		Height:   5,
	}

	t.Run("within conf range", func(t *testing.T) {
		result := s.buildUTXOResult(utxo, 10, 1, 999)
		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if result["amount"] != 1.0 {
			t.Errorf("Expected amount 1.0, got %v", result["amount"])
		}
		if result["address"] != "testaddr" {
			t.Errorf("Expected address 'testaddr', got %v", result["address"])
		}
		if result["spendable"] != true {
			t.Errorf("Expected spendable=true, got %v", result["spendable"])
		}
	})

	t.Run("below min conf", func(t *testing.T) {
		result := s.buildUTXOResult(utxo, 10, 100, 999)
		if result != nil {
			t.Error("Expected nil result when below minConf")
		}
	})

	t.Run("above max conf", func(t *testing.T) {
		result := s.buildUTXOResult(utxo, 10, 1, 3)
		if result != nil {
			t.Error("Expected nil result when above maxConf")
		}
	})

	t.Run("no pkscript", func(t *testing.T) {
		utxoNoPk := &namedb.UTXO{
			TxHash:   *txHash,
			OutIndex: 0,
			Value:    50000000,
			Address:  "testaddr2",
			Height:   5,
		}
		result := s.buildUTXOResult(utxoNoPk, 10, 1, 999)
		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if _, hasScript := result["scriptPubKey"]; hasScript {
			t.Error("Expected no scriptPubKey field when PkScript is empty")
		}
	})
}

// TestConvertAndFindNameUTXO tests the pure helper function directly.
func TestConvertAndFindNameUTXO(t *testing.T) {
	nameHash, _ := chainhash.NewHashFromStr("1111111111111111111111111111111111111111111111111111111111111111")
	otherHash, _ := chainhash.NewHashFromStr("2222222222222222222222222222222222222222222222222222222222222222")

	dbUTXOs := []*namedb.UTXO{
		{TxHash: *otherHash, OutIndex: 0, Value: 1000},
		{TxHash: *nameHash, OutIndex: 1, Value: 2000},
	}

	t.Run("name utxo found", func(t *testing.T) {
		utxos, idx := convertAndFindNameUTXO(dbUTXOs, nameHash, 1)
		if idx != 1 {
			t.Errorf("Expected name UTXO at index 1, got %d", idx)
		}
		if len(utxos) != 2 {
			t.Errorf("Expected 2 UTXOs, got %d", len(utxos))
		}
	})

	t.Run("name utxo not found - wrong hash", func(t *testing.T) {
		notFound, _ := chainhash.NewHashFromStr("3333333333333333333333333333333333333333333333333333333333333333")
		_, idx := convertAndFindNameUTXO(dbUTXOs, notFound, 0)
		if idx != -1 {
			t.Errorf("Expected -1, got %d", idx)
		}
	})

	t.Run("name utxo not found - wrong vout", func(t *testing.T) {
		_, idx := convertAndFindNameUTXO(dbUTXOs, nameHash, 99)
		if idx != -1 {
			t.Errorf("Expected -1 for wrong vout, got %d", idx)
		}
	})

	t.Run("empty utxo list", func(t *testing.T) {
		utxos, idx := convertAndFindNameUTXO(nil, nameHash, 0)
		if idx != -1 {
			t.Errorf("Expected -1 for empty list, got %d", idx)
		}
		if len(utxos) != 0 {
			t.Errorf("Expected empty utxo slice, got %d", len(utxos))
		}
	})
}

// TestBroadcastAndRespond tests broadcastAndRespond with a peer manager.
func TestBroadcastAndRespond(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	// Wait a moment for peer manager to start
	time.Sleep(50 * time.Millisecond)

	// Build a minimal (invalid for network, but valid for RPC) transaction
	tx := buildMinimalTx()

	result := map[string]interface{}{
		"txid": tx.TxHash().String(),
	}

	resp := server.broadcastAndRespond(tx, 1, result)

	// The local peer manager won't have peers but BroadcastTx adds to mempool first.
	// Either success (added to mempool) or a mempool validation error is acceptable.
	// What we must NOT get is a nil response.
	if resp == nil {
		t.Fatal("Expected non-nil response from broadcastAndRespond")
	}
}

// TestBroadcastAndRespondNilPeerMgr tests broadcastAndRespond when peerMgr is nil.
func TestBroadcastAndRespondNilPeerMgr(t *testing.T) {
	s := &Server{}

	tx := buildMinimalTx()
	resp := s.broadcastAndRespond(tx, 1, map[string]interface{}{})

	if resp.Error == nil {
		t.Fatal("Expected error when peer manager is nil")
	}
}

// TestSendRawTransactionNilBlockchain tests that sendRawTransaction with valid tx but nil
// peer manager returns the expected error.
func TestSendRawTransactionNilPeerMgrExplicit(t *testing.T) {
	s := &Server{}

	tx := buildMinimalTx()
	txHex := serializeTxHex(t, tx)

	params, _ := json.Marshal([]interface{}{txHex})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "sendrawtransaction",
		Params:  params,
		ID:      1,
	}

	resp := s.sendRawTransaction(req)

	if resp.Error == nil {
		t.Fatal("Expected error for nil blockchain/peerMgr")
	}
	if resp.Error.Code != -1 {
		t.Errorf("Expected code -1 (network not available), got %d: %s",
			resp.Error.Code, resp.Error.Message)
	}
}
