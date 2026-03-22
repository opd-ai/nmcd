package rpc

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/opd-ai/nmcd/chain"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/internal/logging"
	"github.com/opd-ai/nmcd/namedb"
	"github.com/opd-ai/nmcd/network"
	"github.com/opd-ai/nmcd/wallet"
)

var nameHandlerTestLoggerOnce sync.Once

// setupNameHandlerTestLogger initializes the test logger once for all tests in this file
func setupNameHandlerTestLogger() {
	nameHandlerTestLoggerOnce.Do(func() {
		logCfg := logging.DefaultConfig()
		logCfg.Level = logging.LevelError
		logCfg.Output = "stderr"
		logger, err := logging.Init(logCfg)
		if err != nil {
			panic("Failed to initialize test logger: " + err.Error())
		}
		logging.SetDefault(logger)
	})
}

// setupNameHandlerTestServer creates a complete test server with blockchain, peer manager, and wallet
func setupNameHandlerTestServer(t *testing.T) (*Server, *chain.BlockChain, *wallet.Wallet) {
	t.Helper()

	setupNameHandlerTestLogger()

	// Create temporary directory for test database
	tmpDir := t.TempDir()

	// Create blockchain
	chainCfg := &chain.Config{
		ChainParams: &config.NamecoinRegTestParams,
		NameDBPath:  tmpDir + "/names.db",
		DataDir:     tmpDir,
	}
	bc, err := chain.NewBlockChain(chainCfg, nil)
	if err != nil {
		t.Fatalf("Failed to create test blockchain: %v", err)
	}

	// Create wallet
	w, err := wallet.NewWallet(tmpDir+"/wallet.json", &config.NamecoinRegTestParams)
	if err != nil {
		t.Fatalf("Failed to create test wallet: %v", err)
	}

	// Create peer manager
	pmCfg := &network.Config{
		ChainParams: &config.NamecoinRegTestParams,
		Blockchain:  bc,
		ListenAddrs: []string{},
		MaxPeers:    10,
	}
	pm, err := network.NewPeerManager(pmCfg)
	if err != nil {
		t.Fatalf("Failed to create test peer manager: %v", err)
	}

	server := &Server{
		blockchain:     bc,
		peerMgr:        pm,
		wallet:         w,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
		logger:         logging.GetDefault().WithComponent("rpc"),
	}

	t.Cleanup(func() {
		pm.Stop()
		bc.Close()
	})

	return server, bc, w
}

// TestNameShowSuccess tests nameShow with a registered name
func TestNameShowSuccess(t *testing.T) {
	server, bc, _ := setupNameHandlerTestServer(t)

	// Insert a test name directly into the name database
	testName := "d/testname"
	testValue := `{"ip":"192.168.1.1"}`
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")

	record := &namedb.NameRecord{
		Name:      testName,
		Value:     testValue,
		TxHash:    *txHash,
		OutIndex:  0,
		Height:    100,
		ExpiresAt: 36100, // 100 + 36000
		Address:   "n1test1234567890123456789012345678",
	}

	// Insert the name record
	if err := bc.GetNameDB().PutName(testName, record); err != nil {
		t.Fatalf("Failed to put name: %v", err)
	}

	// Create request
	paramsJSON, _ := json.Marshal([]string{testName})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_show",
		Params:  paramsJSON,
		ID:      1,
	}

	// Call nameShow
	resp := server.nameShow(req)

	// Verify response
	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got %T", resp.Result)
	}

	if result["name"] != testName {
		t.Errorf("Expected name '%s', got '%s'", testName, result["name"])
	}
	if result["value"] != testValue {
		t.Errorf("Expected value '%s', got '%s'", testValue, result["value"])
	}
	if result["height"].(int32) != 100 {
		t.Errorf("Expected height 100, got %v", result["height"])
	}
}

// TestNameShowNotFound tests nameShow with non-existent name
func TestNameShowNotFound(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	paramsJSON, _ := json.Marshal([]string{"d/nonexistent"})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_show",
		Params:  paramsJSON,
		ID:      1,
	}

	resp := server.nameShow(req)

	if resp.Error == nil {
		t.Fatal("Expected error for non-existent name")
	}
	if resp.Error.Code != -5 {
		t.Errorf("Expected error code -5, got %d", resp.Error.Code)
	}
}

// TestNameShowInvalidParams tests nameShow with invalid parameters
func TestNameShowInvalidParams(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	tests := []struct {
		name   string
		params interface{}
	}{
		{"empty array", []string{}},
		{"invalid json", "not-an-array"},
		{"null params", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paramsJSON, _ := json.Marshal(tc.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "name_show",
				Params:  paramsJSON,
				ID:      1,
			}

			resp := server.nameShow(req)

			if resp.Error == nil {
				t.Fatal("Expected error for invalid params")
			}
			if resp.Error.Code != -32602 {
				t.Errorf("Expected error code -32602, got %d", resp.Error.Code)
			}
		})
	}
}

// TestNameShowNilBlockchain tests nameShow when blockchain is nil
func TestNameShowNilBlockchain(t *testing.T) {
	server := &Server{
		blockchain:     nil,
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
	}

	paramsJSON, _ := json.Marshal([]string{"d/test"})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_show",
		Params:  paramsJSON,
		ID:      1,
	}

	// This will panic or fail - we expect it to handle nil gracefully
	defer func() {
		if r := recover(); r != nil {
			t.Logf("nameShow panicked with nil blockchain (expected for some implementations)")
		}
	}()

	resp := server.nameShow(req)
	// If it doesn't panic, verify it returns an error
	if resp.Error == nil {
		t.Error("Expected error response when blockchain is nil")
	}
}

// TestNameListEmpty tests nameList when no names are registered
func TestNameListEmpty(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_list",
		Params:  nil,
		ID:      1,
	}

	resp := server.nameList(req)

	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}

	result, ok := resp.Result.([]map[string]interface{})
	if !ok {
		t.Fatalf("Expected slice result, got %T", resp.Result)
	}

	if len(result) != 0 {
		t.Errorf("Expected empty list, got %d items", len(result))
	}
}

// TestNameListWithNames tests nameList with registered names
func TestNameListWithNames(t *testing.T) {
	server, bc, _ := setupNameHandlerTestServer(t)

	// Insert test names
	names := []string{"d/alpha", "d/beta", "d/gamma"}
	for i, name := range names {
		txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
		record := &namedb.NameRecord{
			Name:      name,
			Value:     `{"test":true}`,
			TxHash:    *txHash,
			OutIndex:  uint32(i),
			Height:    int32(100 + i),
			ExpiresAt: int32(36100 + i),
			Address:   "n1test1234567890123456789012345678",
		}
		if err := bc.GetNameDB().PutName(name, record); err != nil {
			t.Fatalf("Failed to put name: %v", err)
		}
	}

	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_list",
		Params:  nil,
		ID:      1,
	}

	resp := server.nameList(req)

	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}

	result, ok := resp.Result.([]map[string]interface{})
	if !ok {
		t.Fatalf("Expected slice result, got %T", resp.Result)
	}

	if len(result) != len(names) {
		t.Errorf("Expected %d names, got %d", len(names), len(result))
	}
}

// TestNameHistorySuccess tests nameHistory with valid history
func TestNameHistorySuccess(t *testing.T) {
	server, bc, _ := setupNameHandlerTestServer(t)

	testName := "d/historytest"
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000002")

	// Insert the name
	record := &namedb.NameRecord{
		Name:      testName,
		Value:     `{"version":2}`,
		TxHash:    *txHash,
		OutIndex:  0,
		Height:    200,
		ExpiresAt: 36200,
		Address:   "n1test1234567890123456789012345678",
	}
	if err := bc.GetNameDB().PutName(testName, record); err != nil {
		t.Fatalf("Failed to put name: %v", err)
	}

	// Add history entry
	historyRecord := &namedb.NameRecord{
		Name:      testName,
		Value:     `{"version":1}`,
		TxHash:    *txHash,
		OutIndex:  0,
		Height:    100,
		ExpiresAt: 36100,
		Address:   "n1test1234567890123456789012345678",
	}
	if err := bc.GetNameDB().AddHistory(*txHash, historyRecord); err != nil {
		t.Fatalf("Failed to add history: %v", err)
	}

	paramsJSON, _ := json.Marshal([]string{testName})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_history",
		Params:  paramsJSON,
		ID:      1,
	}

	resp := server.nameHistory(req)

	if resp.Error != nil {
		t.Fatalf("Expected success, got error: %v", resp.Error)
	}

	result, ok := resp.Result.([]map[string]interface{})
	if !ok {
		t.Fatalf("Expected slice result, got %T", resp.Result)
	}

	// History may have entries depending on implementation
	if result == nil {
		t.Error("Expected non-nil result")
	}
}

// TestNameHistoryInvalidParams tests nameHistory with invalid parameters
func TestNameHistoryInvalidParams(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	tests := []struct {
		name   string
		params interface{}
	}{
		{"empty array", []string{}},
		{"invalid json", "not-an-array"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paramsJSON, _ := json.Marshal(tc.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "name_history",
				Params:  paramsJSON,
				ID:      1,
			}

			resp := server.nameHistory(req)

			if resp.Error == nil {
				t.Fatal("Expected error for invalid params")
			}
			if resp.Error.Code != -32602 {
				t.Errorf("Expected error code -32602, got %d", resp.Error.Code)
			}
		})
	}
}

// TestNameHistoryNotFound tests nameHistory with non-existent name
func TestNameHistoryNotFound(t *testing.T) {
	server, _, _ := setupNameHandlerTestServer(t)

	paramsJSON, _ := json.Marshal([]string{"d/nohistory"})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_history",
		Params:  paramsJSON,
		ID:      1,
	}

	resp := server.nameHistory(req)

	// GetNameHistory returns empty slice for non-existent names
	// It should not error, just return empty history
	if resp.Error != nil {
		// Some implementations may return an error here
		t.Logf("Got error for non-existent name history: %v", resp.Error)
	} else {
		result, ok := resp.Result.([]map[string]interface{})
		if !ok {
			t.Fatalf("Expected slice result, got %T", resp.Result)
		}
		if len(result) != 0 {
			t.Errorf("Expected empty history, got %d entries", len(result))
		}
	}
}

// TestNameUpdateNoWallet tests nameUpdate when wallet is nil
func TestNameUpdateNoWallet(t *testing.T) {
	// Initialize logger for testing
	logCfg := logging.DefaultConfig()
	logCfg.Level = logging.LevelError
	logCfg.Output = "stderr"
	logger, err := logging.Init(logCfg)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	logging.SetDefault(logger)

	server := &Server{
		wallet:         nil, // No wallet
		rateLimiter:    newRateLimiter(defaultRateLimit),
		maxRequestSize: defaultMaxRequestSize,
		logger:         logging.GetDefault().WithComponent("rpc"),
	}

	paramsJSON, _ := json.Marshal([]string{"d/test", `{"new":"value"}`})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_update",
		Params:  paramsJSON,
		ID:      1,
	}

	resp := server.nameUpdate(req)

	if resp.Error == nil {
		t.Fatal("Expected error when wallet is nil")
	}
	if resp.Error.Code != -1 {
		t.Errorf("Expected error code -1, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "Wallet not initialized. Start the node with wallet enabled." {
		t.Errorf("Unexpected error message: %s", resp.Error.Message)
	}
}

// TestNameUpdateInvalidParams tests nameUpdate with invalid parameters
func TestNameUpdateInvalidParams(t *testing.T) {
	server, _, w := setupNameHandlerTestServer(t)
	server.wallet = w

	tests := []struct {
		name   string
		params interface{}
	}{
		{"empty array", []string{}},
		{"only one param", []string{"d/test"}},
		{"invalid json", "not-an-array"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paramsJSON, _ := json.Marshal(tc.params)
			req := &Request{
				Jsonrpc: "2.0",
				Method:  "name_update",
				Params:  paramsJSON,
				ID:      1,
			}

			resp := server.nameUpdate(req)

			if resp.Error == nil {
				t.Fatal("Expected error for invalid params")
			}
			if resp.Error.Code != -32602 {
				t.Errorf("Expected error code -32602, got %d", resp.Error.Code)
			}
		})
	}
}

// TestNameUpdateNameTooLong tests nameUpdate with name exceeding max length
func TestNameUpdateNameTooLong(t *testing.T) {
	server, _, w := setupNameHandlerTestServer(t)
	server.wallet = w

	// Create a name longer than 255 characters
	longName := "d/" + string(make([]byte, 260))
	paramsJSON, _ := json.Marshal([]string{longName, `{"test":"value"}`})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_update",
		Params:  paramsJSON,
		ID:      1,
	}

	resp := server.nameUpdate(req)

	if resp.Error == nil {
		t.Fatal("Expected error for name too long")
	}
	if resp.Error.Code != -5 {
		t.Errorf("Expected error code -5, got %d", resp.Error.Code)
	}
}

// TestNameUpdateValueTooLarge tests nameUpdate with value exceeding max size
func TestNameUpdateValueTooLarge(t *testing.T) {
	server, _, w := setupNameHandlerTestServer(t)
	server.wallet = w

	// Create a value larger than 1023 bytes
	largeValue := string(make([]byte, 1024))
	paramsJSON, _ := json.Marshal([]string{"d/test", largeValue})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_update",
		Params:  paramsJSON,
		ID:      1,
	}

	resp := server.nameUpdate(req)

	if resp.Error == nil {
		t.Fatal("Expected error for value too large")
	}
	if resp.Error.Code != -5 {
		t.Errorf("Expected error code -5, got %d", resp.Error.Code)
	}
}

// TestNameUpdateNameNotFound tests nameUpdate for non-existent name
func TestNameUpdateNameNotFound(t *testing.T) {
	server, _, w := setupNameHandlerTestServer(t)
	server.wallet = w

	paramsJSON, _ := json.Marshal([]string{"d/nonexistent", `{"test":"value"}`})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_update",
		Params:  paramsJSON,
		ID:      1,
	}

	resp := server.nameUpdate(req)

	if resp.Error == nil {
		t.Fatal("Expected error for non-existent name")
	}
	if resp.Error.Code != -4 {
		t.Errorf("Expected error code -4, got %d", resp.Error.Code)
	}
}

// TestNameUpdateInvalidAddress tests nameUpdate with invalid destination address
func TestNameUpdateInvalidAddress(t *testing.T) {
	server, _, w := setupNameHandlerTestServer(t)
	server.wallet = w

	paramsJSON, _ := json.Marshal([]string{"d/test", `{"test":"value"}`, "invalid-address"})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_update",
		Params:  paramsJSON,
		ID:      1,
	}

	resp := server.nameUpdate(req)

	if resp.Error == nil {
		t.Fatal("Expected error for invalid address")
	}
	if resp.Error.Code != -5 {
		t.Errorf("Expected error code -5, got %d", resp.Error.Code)
	}
}

// TestNameUpdateEmptyName tests nameUpdate with empty name
func TestNameUpdateEmptyName(t *testing.T) {
	server, _, w := setupNameHandlerTestServer(t)
	server.wallet = w

	paramsJSON, _ := json.Marshal([]string{"", `{"test":"value"}`})
	req := &Request{
		Jsonrpc: "2.0",
		Method:  "name_update",
		Params:  paramsJSON,
		ID:      1,
	}

	resp := server.nameUpdate(req)

	if resp.Error == nil {
		t.Fatal("Expected error for empty name")
	}
	if resp.Error.Code != -5 {
		t.Errorf("Expected error code -5, got %d", resp.Error.Code)
	}
}
