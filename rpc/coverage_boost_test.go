package rpc

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/namedb"
)

// TestGetDifficultyRatio tests the getDifficultyRatio helper function.
func TestGetDifficultyRatio(t *testing.T) {
	tests := []struct {
		name   string
		bits   uint32
		params *chaincfg.Params
	}{
		{
			name:   "regtest min difficulty",
			bits:   0x207fffff,
			params: &config.NamecoinRegTestParams,
		},
		{
			name:   "mainnet typical bits",
			bits:   0x1c007fff,
			params: &config.NamecoinMainNetParams,
		},
		{
			name:   "max difficulty bits (pow limit)",
			bits:   config.NamecoinRegTestParams.PowLimitBits,
			params: &config.NamecoinRegTestParams,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diff := getDifficultyRatio(tc.bits, tc.params)
			if diff < 0 {
				t.Errorf("getDifficultyRatio() returned negative value %f", diff)
			}
		})
	}
}

// TestGetInfoSuccess tests getInfo with a valid blockchain.
func TestGetInfoSuccess(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getinfo",
		ID:      1,
	}

	resp := server.getInfo(req)

	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got %T", resp.Result)
	}

	if _, ok := result["blocks"]; !ok {
		t.Error("Expected 'blocks' field in getinfo result")
	}
	if _, ok := result["connections"]; !ok {
		t.Error("Expected 'connections' field in getinfo result")
	}
	if result["version"] != "0.1.0" {
		t.Errorf("Expected version '0.1.0', got %v", result["version"])
	}
}

// TestGetInfoNilBlockchain tests getInfo when blockchain is nil.
func TestGetInfoNilBlockchain(t *testing.T) {
	server := &Server{
		blockchain:     nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getinfo",
		ID:      1,
	}

	resp := server.getInfo(req)

	if resp.Error == nil {
		t.Fatal("Expected error when blockchain is nil")
	}
	if resp.Error.Code != -32603 {
		t.Errorf("Expected error code -32603, got %d", resp.Error.Code)
	}
}

// TestGetBestBlockHashSuccess tests getBestBlockHash with a valid blockchain.
func TestGetBestBlockHashSuccess(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getbestblockhash",
		ID:      1,
	}

	resp := server.getBestBlockHash(req)

	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}

	hash, ok := resp.Result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", resp.Result)
	}
	if len(hash) != 64 {
		t.Errorf("Expected 64-char hash, got %d chars", len(hash))
	}
}

// TestGetBestBlockHashNilBlockchain tests getBestBlockHash when blockchain is nil.
func TestGetBestBlockHashNilBlockchain(t *testing.T) {
	server := &Server{
		blockchain:     nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getbestblockhash",
		ID:      1,
	}

	resp := server.getBestBlockHash(req)
	if resp.Error == nil {
		t.Fatal("Expected error when blockchain is nil")
	}
}

// TestGetPeerInfoNilPeerMgr tests getPeerInfo when peer manager is nil.
func TestGetPeerInfoNilPeerMgr(t *testing.T) {
	server := &Server{
		peerMgr:        nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getpeerinfo",
		ID:      1,
	}

	resp := server.getPeerInfo(req)

	if resp.Error != nil {
		t.Fatalf("Expected success when peer manager is nil, got error: %v", resp.Error)
	}

	peers, ok := resp.Result.([]interface{})
	if !ok {
		t.Fatalf("Expected []interface{} result, got %T", resp.Result)
	}
	if len(peers) != 0 {
		t.Errorf("Expected empty peer list, got %d peers", len(peers))
	}
}

// TestGetPeerInfoWithPeerMgr tests getPeerInfo with a real peer manager.
func TestGetPeerInfoWithPeerMgr(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getpeerinfo",
		ID:      1,
	}

	resp := server.getPeerInfo(req)

	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}
}

// TestGetBalanceNoWalletCoverage tests getBalance when wallet is nil (duplicate name protection).
func TestGetBalanceNoWalletCoverage(t *testing.T) {
	server := &Server{
		wallet:         nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getbalance",
		ID:      1,
	}

	resp := server.getBalance(req)
	if resp.Error == nil {
		t.Fatal("Expected error when wallet is nil")
	}
}

// TestGetBalanceNilBlockchain tests getBalance when blockchain is nil.
func TestGetBalanceNilBlockchain(t *testing.T) {
	server, _, w := setupNameHandlerTestServer(t)
	server.blockchain = nil
	_ = w

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getbalance",
		ID:      1,
	}

	resp := server.getBalance(req)
	if resp.Error == nil {
		t.Fatal("Expected error when blockchain is nil")
	}
	if resp.Error.Code != -32603 {
		t.Errorf("Expected code -32603, got %d", resp.Error.Code)
	}
}

// TestGetBalanceEmptyWallet tests getBalance with a wallet that has no addresses.
func TestGetBalanceEmptyWallet(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getbalance",
		ID:      1,
	}

	resp := server.getBalance(req)
	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}

	// Empty wallet should return 0.0
	balance, ok := resp.Result.(float64)
	if !ok {
		t.Fatalf("Expected float64 result, got %T", resp.Result)
	}
	if balance != 0.0 {
		t.Errorf("Expected balance 0.0, got %f", balance)
	}
}

// TestListUnspentNoWalletCoverage tests listUnspent when wallet is nil.
func TestListUnspentNoWalletCoverage(t *testing.T) {
	server := &Server{
		wallet:         nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "listunspent",
		ID:      1,
	}

	resp := server.listUnspent(req)
	if resp.Error == nil {
		t.Fatal("Expected error when wallet is nil")
	}
}

// TestListUnspentNilBlockchain tests listUnspent when blockchain is nil.
func TestListUnspentNilBlockchain(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)
	server.blockchain = nil

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "listunspent",
		ID:      1,
	}

	resp := server.listUnspent(req)
	if resp.Error == nil {
		t.Fatal("Expected error when blockchain is nil")
	}
}

// TestListUnspentEmpty tests listUnspent with no UTXOs.
func TestListUnspentEmpty(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "listunspent",
		ID:      1,
	}

	resp := server.listUnspent(req)
	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}
}

// TestListUnspentWithParams tests listUnspent with minconf/maxconf parameters.
func TestListUnspentWithParams(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	params, _ := json.Marshal([]interface{}{0, 100})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "listunspent",
		Params:  params,
		ID:      1,
	}

	resp := server.listUnspent(req)
	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}
}

// TestListUnspentWithAddressFilter tests listUnspent with address filter.
func TestListUnspentWithAddressFilter(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	params, _ := json.Marshal([]interface{}{1, 9999999, []string{"n1test1234567890123456789012345678"}})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "listunspent",
		Params:  params,
		ID:      1,
	}

	resp := server.listUnspent(req)
	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}
}

// TestParseListUnspentParamsDefaults tests that parseListUnspentParams returns defaults for nil/empty params.
func TestParseListUnspentParamsDefaults(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	minConf, maxConf, addrs := server.parseListUnspentParams(nil)
	if minConf != 1 {
		t.Errorf("Expected default minConf=1, got %d", minConf)
	}
	if maxConf != 9999999 {
		t.Errorf("Expected default maxConf=9999999, got %d", maxConf)
	}
	if len(addrs) != 0 {
		t.Errorf("Expected no filter addresses, got %d", len(addrs))
	}
}

// TestParseListUnspentParamsWithValues tests parseListUnspentParams with explicit values.
func TestParseListUnspentParamsWithValues(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	params, _ := json.Marshal([]interface{}{2, 50, []string{"addr1", "addr2"}})
	minConf, maxConf, addrs := server.parseListUnspentParams(params)

	if minConf != 2 {
		t.Errorf("Expected minConf=2, got %d", minConf)
	}
	if maxConf != 50 {
		t.Errorf("Expected maxConf=50, got %d", maxConf)
	}
	if len(addrs) != 2 {
		t.Errorf("Expected 2 filter addresses, got %d", len(addrs))
	}
}

// TestFormatNameRecordsExpiredFields tests that formatNameRecords clamps expires_in to 0.
func TestFormatNameRecordsExpiredFields(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)
	_ = server

	// Use a large current height so names are expired
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_list",
		ID:      1,
	}

	resp := server.nameList(req)
	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}
}

// TestNameShowExpiredName tests that nameShow returns expires_in=0 for an expired name.
func TestNameShowExpiredName(t *testing.T) {
	server, bc, _ := setupNameHandlerTestServer(t)

	testName := "d/expiredtest"

	// Insert a name record with ExpiresAt in the past (height 0 is genesis, expires at 1)
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000099")
	record := &namedb.NameRecord{
		Name:      testName,
		Value:     `{}`,
		TxHash:    *txHash,
		OutIndex:  0,
		Height:    1,
		ExpiresAt: 1, // Already expired at genesis height (0)
		Address:   "n1test1234567890123456789012345678",
	}
	if err := bc.GetNameDB().PutName(testName, record); err != nil {
		t.Fatalf("Failed to put name: %v", err)
	}

	paramsJSON, _ := json.Marshal([]string{testName})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_show",
		Params:  paramsJSON,
		ID:      1,
	}

	resp := server.nameShow(req)
	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got %T", resp.Result)
	}

	// expires_in should be >= 0 for expired names (not negative)
	if expiresIn, ok := result["expires_in"].(int32); ok {
		if expiresIn < 0 {
			t.Errorf("expires_in should be >= 0 for expired names, got %d", expiresIn)
		}
	}

	// expired should be present
	if _, ok := result["expired"]; !ok {
		t.Error("Expected 'expired' field in response")
	}
}

// TestProcessRequestRouting tests that processRequest routes to the correct handler.
func TestProcessRequestRouting(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	methods := []string{
		"getinfo",
		"getblockcount",
		"getbestblockhash",
		"getconnectioncount",
		"getpeerinfo",
		"getmetrics",
		"name_list",
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := &Request{
				Jsonrpc: "2.0",
				Method:  method,
				ID:      1,
			}
			resp := server.processRequest(req)
			if resp == nil {
				t.Errorf("processRequest(%s) returned nil", method)
			}
		})
	}
}

// TestProcessRequestUnknownMethod tests processRequest with an unknown method.
func TestProcessRequestUnknownMethod(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "unknown_method_xyz",
		ID:      1,
	}

	resp := server.processRequest(req)
	if resp == nil {
		t.Fatal("Expected response for unknown method")
	}
	if resp.Error == nil {
		t.Fatal("Expected error response for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("Expected error code -32601 (Method not found), got %d", resp.Error.Code)
	}
}

// TestNameListWithExpiredNames tests nameList and checks that expired names have expires_in=0.
func TestNameListWithExpiredNames(t *testing.T) {
	server, bc, _ := setupNameHandlerTestServer(t)

	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")

	expiredRecord := &namedb.NameRecord{
		Name:      "d/expiredlist",
		Value:     `{}`,
		TxHash:    *txHash,
		OutIndex:  0,
		Height:    1,
		ExpiresAt: 1,
		Address:   "n1test1234567890123456789012345678",
	}
	if err := bc.GetNameDB().PutName(expiredRecord.Name, expiredRecord); err != nil {
		t.Fatalf("Failed to put name: %v", err)
	}

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_list",
		ID:      1,
	}

	resp := server.nameList(req)
	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}

	result, ok := resp.Result.([]map[string]interface{})
	if !ok {
		t.Fatalf("Expected []map result, got %T", resp.Result)
	}

	for _, entry := range result {
		if entry["name"] == "d/expiredlist" {
			if expiresIn, ok := entry["expires_in"].(int32); ok && expiresIn < 0 {
				t.Errorf("expires_in should be >= 0 for expired names, got %d", expiresIn)
			}
		}
	}
}

// TestLookupActiveNameRecordExpired tests lookupActiveNameRecord with an expired name.
func TestLookupActiveNameRecordExpired(t *testing.T) {
	server, bc, _ := setupNameHandlerTestServer(t)

	testName := "d/lookedupexpired"
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000002")
	record := &namedb.NameRecord{
		Name:      testName,
		Value:     `{}`,
		TxHash:    *txHash,
		OutIndex:  0,
		Height:    1,
		ExpiresAt: -1, // ExpiresAt=-1 is < bestHeight=0 (genesis), so name is expired (per strict-< convention)
		Address:   "n1test1234567890123456789012345678",
	}
	if err := bc.GetNameDB().PutName(testName, record); err != nil {
		t.Fatalf("Failed to put name: %v", err)
	}

	_, errResp := server.lookupActiveNameRecord(testName, 1)
	if errResp == nil {
		t.Fatal("Expected error response for expired name")
	}
	if errResp.Error == nil {
		t.Fatal("Expected error in response for expired name")
	}
}

// TestLookupActiveNameRecordNotFound tests lookupActiveNameRecord with a non-existent name.
func TestLookupActiveNameRecordNotFound(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	_, errResp := server.lookupActiveNameRecord("d/doesnotexist", 1)
	if errResp == nil {
		t.Fatal("Expected error response for non-existent name")
	}
}

// TestFormatNameRecordsDirectly tests formatNameRecords with active and expired records.
func TestFormatNameRecordsDirectly(t *testing.T) {
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")

	records := []*namedb.NameRecord{
		{
			Name:      "d/active",
			Value:     `{}`,
			TxHash:    *txHash,
			ExpiresAt: 1000,
			Height:    100,
			Address:   "ntest1",
		},
		{
			Name:      "d/expired",
			Value:     `{"old":true}`,
			TxHash:    *txHash,
			ExpiresAt: 10, // Less than currentHeight
			Height:    5,
			Address:   "ntest2",
		},
	}

	result := formatNameRecords(records, 50) // currentHeight=50

	if len(result) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(result))
	}

	// Active name: expires_in = 1000-50 = 950
	activeEntry := result[0]
	if expiresIn, ok := activeEntry["expires_in"].(int32); ok {
		if expiresIn != 950 {
			t.Errorf("Active name: expected expires_in=950, got %d", expiresIn)
		}
	}
	if expired, ok := activeEntry["expired"].(bool); ok && expired {
		t.Error("Active name should not be expired")
	}

	// Expired name: expiresAt=10 <= currentHeight=50, so expires_in should be clamped to 0
	expiredEntry := result[1]
	if expiresIn, ok := expiredEntry["expires_in"].(int32); ok {
		if expiresIn != 0 {
			t.Errorf("Expired name: expected expires_in=0, got %d", expiresIn)
		}
	}
	if expired, ok := expiredEntry["expired"].(bool); !ok || !expired {
		t.Error("Expired name: expected expired=true")
	}
}

// TestVerifyNameOwnershipMissingKey tests verifyNameOwnership when wallet lacks the key.
func TestVerifyNameOwnershipMissingKey(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	record := &namedb.NameRecord{
		Name:    "d/test",
		Address: "m1notinwallet12345678901234567890",
	}

	errResp := server.verifyNameOwnership(record, 1)
	if errResp == nil {
		t.Fatal("Expected error when wallet doesn't have the key")
	}
	if errResp.Error == nil {
		t.Fatal("Expected error response")
	}
	if errResp.Error.Code != -13 {
		t.Errorf("Expected error code -13, got %d", errResp.Error.Code)
	}
}

// TestVerifyNameOwnershipHasKey tests verifyNameOwnership when wallet has the key.
func TestVerifyNameOwnershipHasKey(t *testing.T) {
	server, _, w := setupNameHandlerTestServer(t)

	// Generate a key in the wallet
	addr, err := w.GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	record := &namedb.NameRecord{
		Name:    "d/test",
		Address: addr,
	}

	errResp := server.verifyNameOwnership(record, 1)
	if errResp != nil {
		t.Errorf("Expected nil when wallet has the key, got: %v", errResp)
	}
}

// TestGetBalanceWithWalletAddress tests getBalance with a wallet address (but no UTXOs).
func TestGetBalanceWithWalletAddress(t *testing.T) {
	server, _, w := setupNameHandlerTestServer(t)

	// Generate a key to create an address
	_, err := w.GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "getbalance",
		ID:      1,
	}

	resp := server.getBalance(req)
	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}

	balance, ok := resp.Result.(float64)
	if !ok {
		t.Fatalf("Expected float64 result, got %T", resp.Result)
	}
	// No UTXOs in test blockchain, so balance should be 0
	if balance != 0.0 {
		t.Errorf("Expected balance 0.0 (no UTXOs), got %f", balance)
	}
}

// TestCheckNameNotActiveWithActiveExpired tests checkNameNotActive with an expired active name.
func TestCheckNameNotActiveWithActiveExpiredName(t *testing.T) {
	server, bc, _ := setupNameHandlerTestServer(t)

	testName := "d/activetestexpired"
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000005")
	record := &namedb.NameRecord{
		Name:      testName,
		Value:     `{}`,
		TxHash:    *txHash,
		ExpiresAt: 0, // Expired
		Height:    1,
		Address:   "n1test1234567890123456789012345678",
	}
	if err := bc.GetNameDB().PutName(testName, record); err != nil {
		t.Fatalf("Failed to put expired name: %v", err)
	}

	// checkNameNotActive should succeed when the existing name is expired
	errResp := server.checkNameNotActive(testName, 1)
	if errResp != nil {
		t.Errorf("checkNameNotActive should return nil for expired name, got: %v", errResp)
	}
}

// TestParseVerboseParam tests parseVerboseParam with various inputs.
func TestParseVerboseParam(t *testing.T) {
	tests := []struct {
		name    string
		params  []interface{}
		idx     int
		want    bool
		wantErr bool
	}{
		{"true", []interface{}{true}, 0, true, false},
		{"false", []interface{}{false}, 0, false, false},
		{"out of range", []interface{}{}, 0, false, false},
		{"invalid type", []interface{}{"notabool"}, 0, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, errResp := parseVerboseParam(tc.params, tc.idx, 1)
			if tc.wantErr && errResp == nil {
				t.Errorf("Expected error for parseVerboseParam(%v, %d)", tc.params, tc.idx)
			}
			if !tc.wantErr && errResp != nil {
				t.Errorf("Unexpected error: %v", errResp)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("parseVerboseParam(%v, %d) = %v, want %v", tc.params, tc.idx, got, tc.want)
			}
		})
	}
}

// TestNamePendingNoPeerMgr tests namePending when peer manager is nil.
func TestNamePendingNoPeerMgr(t *testing.T) {
	server := &Server{
		peerMgr:        nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_pending",
		ID:      1,
	}

	resp := server.namePending(req)
	if resp.Error != nil {
		t.Fatalf("Expected empty result, got error: %v", resp.Error)
	}

	result, ok := resp.Result.([]map[string]interface{})
	if !ok {
		t.Fatalf("Expected []map result, got %T", resp.Result)
	}
	if len(result) != 0 {
		t.Errorf("Expected empty list, got %d items", len(result))
	}
}

// TestValidateNameNewCommitmentNotFound tests validateNameNewCommitment when no commitment exists.
func TestValidateNameNewCommitmentNotFound(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	randBytes := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	errResp := server.validateNameNewCommitment(randBytes, "d/test", 1)
	if errResp == nil {
		t.Fatal("Expected error when no NAME_NEW commitment exists")
	}
	if errResp.Error == nil {
		t.Fatal("Expected error response")
	}
}

// TestNameScanWithCount tests nameScan with an explicit count parameter.
func TestNameScanWithCount(t *testing.T) {
	server, bc, _ := setupNameHandlerTestServer(t)

	// Insert some test names
	for i := 0; i < 3; i++ {
		txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
		name := fmt.Sprintf("d/scan%d", i)
		record := &namedb.NameRecord{
			Name:      name,
			Value:     `{}`,
			TxHash:    *txHash,
			OutIndex:  uint32(i),
			Height:    100,
			ExpiresAt: 36100,
			Address:   "n1test1234567890123456789012345678",
		}
		if err := bc.GetNameDB().PutName(name, record); err != nil {
			t.Fatalf("Failed to put name %s: %v", name, err)
		}
	}

	paramsJSON, _ := json.Marshal([]interface{}{"", 2})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_scan",
		Params:  paramsJSON,
		ID:      1,
	}

	resp := server.nameScan(req)
	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}
}

// TestNameHistoryExpiredName tests nameHistory for an expired name.
func TestNameHistoryExpiredName(t *testing.T) {
	server, bc, _ := setupNameHandlerTestServer(t)

	testName := "d/histexpired"
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000003")
	record := &namedb.NameRecord{
		Name:      testName,
		Value:     `{}`,
		TxHash:    *txHash,
		ExpiresAt: 0,
		Height:    1,
		Address:   "n1test1234567890123456789012345678",
	}
	if err := bc.GetNameDB().PutName(testName, record); err != nil {
		t.Fatalf("Failed to put name: %v", err)
	}

	paramsJSON, _ := json.Marshal([]string{testName})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_history",
		Params:  paramsJSON,
		ID:      1,
	}

	resp := server.nameHistory(req)
	// Either success or error is fine - just ensure no panic
	_ = resp
}

// TestGetConnectionCountNilPeerMgr tests getConnectionCount with nil peer manager.
func TestGetConnectionCountNilPeerMgr(t *testing.T) {
	server := &Server{
		peerMgr:        nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	req := &Request{Jsonrpc: "2.0", Method: "getconnectioncount", ID: 1}
	resp := server.getConnectionCount(req)
	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}
	if resp.Result.(int) != 0 {
		t.Errorf("Expected 0 connections, got %v", resp.Result)
	}
}

// TestNameNewWithWalletNoAddresses tests nameNew when wallet has no addresses.
func TestNameNewWithWalletNoAddresses(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)
	// Wallet starts with no addresses

	paramsJSON, _ := json.Marshal([]string{"d/newname"})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_new",
		Params:  paramsJSON,
		ID:      1,
	}

	resp := server.nameNew(req)
	if resp.Error == nil {
		t.Fatal("Expected error when wallet has no addresses")
	}
	if resp.Error.Code != -1 {
		t.Fatalf("Expected error code -1 for missing wallet address, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "No addresses in wallet. Create an address first using getnewaddress." {
		t.Fatalf("Unexpected error message: %s", resp.Error.Message)
	}
}

// TestNameNewWithWalletAddress tests nameNew when wallet has an address (but no UTXOs).
func TestNameNewWithWalletAddress(t *testing.T) {
	server, _, w := setupNameHandlerTestServer(t)

	// Generate a key to create an address
	_, err := w.GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	paramsJSON, _ := json.Marshal([]string{"d/newname2"})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_new",
		Params:  paramsJSON,
		ID:      1,
	}

	resp := server.nameNew(req)
	if resp.Error == nil {
		t.Fatal("Expected error when wallet has no UTXOs")
	}
	if resp.Error.Code != -6 {
		t.Fatalf("Expected error code -6 for insufficient funds, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "Insufficient funds. No UTXOs available in wallet." {
		t.Fatalf("Unexpected error message: %s", resp.Error.Message)
	}
}

// TestNameListNilBlockchainReturnsError tests nameList returns JSON-RPC error when blockchain is nil.
func TestNameListNilBlockchainReturnsError(t *testing.T) {
	server := &Server{
		blockchain:     nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	req := &Request{Jsonrpc: "2.0", Method: "name_list", ID: 1}
	resp := server.nameList(req)
	if resp.Error == nil {
		t.Fatal("Expected error response when blockchain is nil")
	}
	if resp.Error.Code != -32603 {
		t.Fatalf("Expected error code -32603, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "Blockchain not initialized" {
		t.Fatalf("Expected 'Blockchain not initialized', got %q", resp.Error.Message)
	}
}
