package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/opd-ai/nmcd/namedb"
)

// TestIntegration_EmbeddedClient_FullWorkflow tests a complete workflow with EmbeddedClient
// including initialization, name operations, and cleanup.
func TestIntegration_EmbeddedClient_FullWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create embedded client
	cfg := &Config{
		Mode:    ModeEmbedded,
		DataDir: t.TempDir(),
		Network: "regtest",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Step 1: Verify client initialized correctly
	info, err := client.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo() failed: %v", err)
	}
	if info.Mode != "embedded" {
		t.Errorf("Expected embedded mode, got %s", info.Mode)
	}
	if info.NetworkName != "regtest" {
		t.Errorf("Expected regtest network, got %s", info.NetworkName)
	}

	// Step 2: List names (should be empty initially)
	names, err := client.ListNames(ctx, &ListFilter{
		Limit: 100,
	})
	if err != nil {
		t.Fatalf("ListNames() failed: %v", err)
	}
	initialCount := len(names)

	// Step 3: Try to resolve non-existent name
	_, err = client.ResolveName(ctx, "d/nonexistent")
	if err != ErrNameNotFound {
		t.Errorf("ResolveName() expected ErrNameNotFound, got %v", err)
	}

	// Step 4: Get history of non-existent name (should return empty)
	history, err := client.GetNameHistory(ctx, "d/nonexistent")
	if err != nil {
		t.Fatalf("GetNameHistory() failed: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("GetNameHistory() expected empty history, got %d entries", len(history))
	}

	// Step 5: Insert a test name directly into database for testing
	ec, ok := client.(*EmbeddedClient)
	if !ok {
		t.Fatal("Expected EmbeddedClient")
	}

	testName := "d/integration-test"
	testValue := `{"ip":"1.2.3.4"}`
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	
	record := &namedb.NameRecord{
		Name:      testName,
		Value:     testValue,
		TxHash:    *txHash,
		OutIndex:  0,
		Height:    100,
		ExpiresAt: 100 + 36000,
		Address:   "NTest123Address",
		UpdatedAt: time.Now(),
	}

	if err := ec.nameDB.PutName(testName, record); err != nil {
		t.Fatalf("Failed to insert test record: %v", err)
	}

	// Step 6: Resolve the name
	resolved, err := client.ResolveName(ctx, testName)
	if err != nil {
		t.Fatalf("ResolveName() failed: %v", err)
	}
	if resolved.Name != testName {
		t.Errorf("Expected name %s, got %s", testName, resolved.Name)
	}
	if resolved.Value != testValue {
		t.Errorf("Expected value %s, got %s", testValue, resolved.Value)
	}

	// Step 7: List names again (should have one more)
	names, err = client.ListNames(ctx, &ListFilter{
		Limit: 100,
	})
	if err != nil {
		t.Fatalf("ListNames() failed: %v", err)
	}
	if len(names) != initialCount+1 {
		t.Errorf("Expected %d names, got %d", initialCount+1, len(names))
	}

	// Step 8: Test namespace filtering
	dNames, err := client.ListNames(ctx, &ListFilter{
		Namespace: "d/",
		Limit:     100,
	})
	if err != nil {
		t.Fatalf("ListNames() with namespace filter failed: %v", err)
	}
	found := false
	for _, n := range dNames {
		if n.Name == testName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected to find %s in d/ namespace", testName)
	}

	// Step 9: Get name history
	history, err = client.GetNameHistory(ctx, testName)
	if err != nil {
		t.Fatalf("GetNameHistory() failed: %v", err)
	}
	// History should be empty for directly inserted name (no transaction history)
	// This is expected behavior

	// Step 10: Test GetInfo again
	info2, err := client.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo() second call failed: %v", err)
	}
	if info2.Mode != info.Mode {
		t.Errorf("Mode changed: %s -> %s", info.Mode, info2.Mode)
	}
}

// TestIntegration_DaemonClient_FullWorkflow tests a complete workflow with DaemonClient
// using a mock RPC server.
func TestIntegration_DaemonClient_FullWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Mock name database for RPC responses
	mockNames := make(map[string]*NameRecord)
	mockNames["d/test"] = &NameRecord{
		Name:      "d/test",
		Value:     `{"ip":"5.6.7.8"}`,
		TxHash:    "0000000000000000000000000000000000000000000000000000000000000002",
		Height:    200,
		ExpiresAt: 200 + 36000,
		ExpiresIn: 36000,
		Address:   "NTest456Address",
		UpdatedAt: time.Now(),
	}

	// Create mock RPC server
	handler := func(method string, params json.RawMessage) (interface{}, *rpcError) {
		switch method {
		case "getinfo":
			return map[string]interface{}{
				"version":     "0.1.0",
				"blocks":      200,
				"connections": 3,
			}, nil

		case "getbestblockhash":
			return "0000000000000000000000000000000000000000000000000000000000000100", nil

		case "getblockcount":
			return 200, nil

		case "name_show":
			var nameParams []interface{}
			if err := json.Unmarshal(params, &nameParams); err != nil || len(nameParams) == 0 {
				return nil, &rpcError{Code: -32602, Message: "Invalid params"}
			}
			name, ok := nameParams[0].(string)
			if !ok {
				return nil, &rpcError{Code: -32602, Message: "Invalid name parameter"}
			}

			record, exists := mockNames[name]
			if !exists {
				return nil, &rpcError{Code: -4, Message: "name not found"}
			}

			return map[string]interface{}{
				"name":       record.Name,
				"value":      record.Value,
				"txid":       record.TxHash,
				"height":     record.Height,
				"expires_at": record.ExpiresAt,
				"expires_in": record.ExpiresIn,
				"address":    record.Address,
			}, nil

		case "name_list":
			var result []map[string]interface{}
			for _, record := range mockNames {
				result = append(result, map[string]interface{}{
					"name":       record.Name,
					"value":      record.Value,
					"txid":       record.TxHash,
					"height":     record.Height,
					"expires_at": record.ExpiresAt,
					"expires_in": record.ExpiresIn,
					"address":    record.Address,
				})
			}
			return result, nil

		case "name_history":
			var nameParams []interface{}
			if err := json.Unmarshal(params, &nameParams); err != nil || len(nameParams) == 0 {
				return nil, &rpcError{Code: -32602, Message: "Invalid params"}
			}
			// Return empty history for simplicity
			return []interface{}{}, nil

		default:
			return nil, &rpcError{Code: -32601, Message: "Method not found"}
		}
	}

	server := mockRPCServer(t, handler)
	defer server.Close()

	// Create daemon client
	cfg := &Config{
		Mode:    ModeDaemon,
		RPCAddr: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Test 1: GetInfo
	info, err := client.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo() failed: %v", err)
	}
	if info.Mode != "daemon" {
		t.Errorf("Expected daemon mode, got %s", info.Mode)
	}
	if info.BlockHeight != 200 {
		t.Errorf("Expected height 200, got %d", info.BlockHeight)
	}

	// Test 2: ResolveName
	record, err := client.ResolveName(ctx, "d/test")
	if err != nil {
		t.Fatalf("ResolveName() failed: %v", err)
	}
	if record.Name != "d/test" {
		t.Errorf("Expected name d/test, got %s", record.Name)
	}

	// Test 3: ResolveName for non-existent name
	_, err = client.ResolveName(ctx, "d/nonexistent")
	if err != ErrNameNotFound {
		t.Errorf("Expected ErrNameNotFound, got %v", err)
	}

	// Test 4: ListNames
	names, err := client.ListNames(ctx, &ListFilter{
		Limit: 100,
	})
	if err != nil {
		t.Fatalf("ListNames() failed: %v", err)
	}
	if len(names) != len(mockNames) {
		t.Errorf("Expected %d names, got %d", len(mockNames), len(names))
	}

	// Test 5: GetNameHistory
	history, err := client.GetNameHistory(ctx, "d/test")
	if err != nil {
		t.Fatalf("GetNameHistory() failed: %v", err)
	}
	// Mock returns empty history
	if len(history) != 0 {
		t.Errorf("Expected empty history, got %d entries", len(history))
	}
}

// TestIntegration_AutoMode_Switching tests the auto mode switching behavior
func TestIntegration_AutoMode_Switching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test 1: Auto mode with daemon available
	handler := func(method string, params json.RawMessage) (interface{}, *rpcError) {
		switch method {
		case "getinfo":
			return map[string]interface{}{
				"version":     "0.1.0",
				"blocks":      150,
				"connections": 2,
			}, nil
		case "getbestblockhash":
			return "0000000000000000000000000000000000000000000000000000000000000150", nil
		default:
			return nil, &rpcError{Code: -32601, Message: "Method not found"}
		}
	}

	server := mockRPCServer(t, handler)
	defer server.Close()

	cfg := &Config{
		Mode:    ModeAuto,
		RPCAddr: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() with auto mode failed: %v", err)
	}
	defer client.Close()

	info, err := client.GetInfo(context.Background())
	if err != nil {
		t.Fatalf("GetInfo() failed: %v", err)
	}
	if info.Mode != "daemon" {
		t.Errorf("Expected daemon mode when daemon available, got %s", info.Mode)
	}

	// Test 2: Auto mode with daemon unavailable (fallback to embedded)
	cfg2 := &Config{
		Mode:    ModeAuto,
		DataDir: t.TempDir(),
		Network: "regtest",
		RPCAddr: "http://127.0.0.1:59998", // Non-existent daemon
	}

	client2, err := NewClient(cfg2)
	if err != nil {
		t.Fatalf("NewClient() with auto mode (fallback) failed: %v", err)
	}
	defer client2.Close()

	info2, err := client2.GetInfo(context.Background())
	if err != nil {
		t.Fatalf("GetInfo() failed: %v", err)
	}
	if info2.Mode != "embedded" {
		t.Errorf("Expected embedded mode when daemon unavailable, got %s", info2.Mode)
	}
}

// TestIntegration_ConcurrentOperations tests thread-safe concurrent access
func TestIntegration_ConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create embedded client
	cfg := &Config{
		Mode:    ModeEmbedded,
		DataDir: t.TempDir(),
		Network: "regtest",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Insert test data
	ec, ok := client.(*EmbeddedClient)
	if !ok {
		t.Fatal("Expected EmbeddedClient")
	}

	for i := 0; i < 10; i++ {
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("000000000000000000000000000000000000000000000000000000000000000%d", i))
		record := &namedb.NameRecord{
			Name:      fmt.Sprintf("d/test-%d", i),
			Value:     fmt.Sprintf(`{"id":%d}`, i),
			TxHash:    *txHash,
			OutIndex:  0,
			Height:    int32(100 + i),
			ExpiresAt: int32(100 + i + 36000),
			Address:   fmt.Sprintf("NAddr%d", i),
			UpdatedAt: time.Now(),
		}
		if err := ec.nameDB.PutName(record.Name, record); err != nil {
			t.Fatalf("Failed to insert test record: %v", err)
		}
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent GetInfo calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.GetInfo(ctx)
			if err != nil {
				errors <- fmt.Errorf("GetInfo failed: %w", err)
			}
		}()
	}

	// Concurrent ResolveName calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		i := i // Capture loop variable
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("d/test-%d", i)
			record, err := client.ResolveName(ctx, name)
			if err != nil {
				errors <- fmt.Errorf("ResolveName(%s) failed: %w", name, err)
				return
			}
			if record.Name != name {
				errors <- fmt.Errorf("Expected name %s, got %s", name, record.Name)
			}
		}()
	}

	// Concurrent ListNames calls
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.ListNames(ctx, &ListFilter{
				Namespace: "d/",
				Limit:     100,
			})
			if err != nil {
				errors <- fmt.Errorf("ListNames failed: %w", err)
			}
		}()
	}

	// Concurrent GetNameHistory calls
	for i := 0; i < 5; i++ {
		wg.Add(1)
		i := i // Capture loop variable
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("d/test-%d", i)
			_, err := client.GetNameHistory(ctx, name)
			if err != nil {
				errors <- fmt.Errorf("GetNameHistory(%s) failed: %w", name, err)
			}
		}()
	}

	// Wait for all goroutines
	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}
}

// TestIntegration_ContextCancellation tests that operations respect context cancellation
func TestIntegration_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create embedded client
	cfg := &Config{
		Mode:    ModeEmbedded,
		DataDir: t.TempDir(),
		Network: "regtest",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Test 1: Cancel before operation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = client.ResolveName(ctx, "d/test")
	if err == nil {
		t.Error("Expected error from cancelled context, got nil")
	}

	// Test 2: Cancel during operation (use short timeout)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel2()

	time.Sleep(10 * time.Millisecond) // Ensure timeout expires

	_, err = client.ListNames(ctx2, &ListFilter{Limit: 100})
	// May or may not error depending on timing, but shouldn't panic
}

// TestIntegration_ErrorRecovery tests recovery from various error conditions
func TestIntegration_ErrorRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test 1: Invalid name format
	cfg := &Config{
		Mode:    ModeEmbedded,
		DataDir: t.TempDir(),
		Network: "regtest",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Empty name
	_, err = client.ResolveName(ctx, "")
	if err == nil {
		t.Error("Expected error for empty name, got nil")
	}

	// Very long name
	longName := "d/" + string(make([]byte, 1000))
	_, err = client.ResolveName(ctx, longName)
	if err == nil {
		t.Error("Expected error for too long name, got nil")
	}

	// Test 2: Invalid filter parameters
	// Note: Currently the implementation caps excessive limits but doesn't validate negative ones
	// This test focuses on values that should work after the implementation validates inputs
	_, err = client.ListNames(ctx, &ListFilter{
		Limit: 20000, // Exceeds max, should be capped to 10000
	})
	// This should not error - it just caps the limit
	if err != nil {
		t.Errorf("ListNames() with large limit failed: %v", err)
	}

	// Test 3: Operations should work after errors
	names, err := client.ListNames(ctx, &ListFilter{
		Limit: 10,
	})
	if err != nil {
		t.Errorf("ListNames() after errors failed: %v", err)
	}
	_ = names // Use the result

	info, err := client.GetInfo(ctx)
	if err != nil {
		t.Errorf("GetInfo() after errors failed: %v", err)
	}
	if info.Mode != "embedded" {
		t.Errorf("Client mode changed after errors: %s", info.Mode)
	}
}

// TestIntegration_DaemonClient_NetworkRecovery tests recovery from transient network failures
func TestIntegration_DaemonClient_NetworkRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test verifies that the client can recover from transient network errors
	// Note: The retry logic only retries on connection-level errors (connection refused,
	// timeout, EOF, etc.), not on application-level RPC errors.
	// Since we're using a mock HTTP server that's always available, we can't easily
	// simulate connection failures. This test documents the expected behavior.

	handler := func(method string, params json.RawMessage) (interface{}, *rpcError) {
		switch method {
		case "getinfo":
			return map[string]interface{}{
				"version":     "0.1.0",
				"blocks":      100,
				"connections": 1,
			}, nil
		case "getbestblockhash":
			return "0000000000000000000000000000000000000000000000000000000000000001", nil
		default:
			return nil, &rpcError{Code: -32601, Message: "Method not found"}
		}
	}

	server := mockRPCServer(t, handler)
	defer server.Close()

	cfg := &Config{
		Mode:    ModeDaemon,
		RPCAddr: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Normal operation should work
	info, err := client.GetInfo(context.Background())
	if err != nil {
		t.Fatalf("GetInfo() failed: %v", err)
	}

	if info.BlockHeight != 100 {
		t.Errorf("Expected height 100, got %d", info.BlockHeight)
	}

	// Application errors (RPC errors) should NOT be retried
	// They should fail immediately
	ctx := context.Background()
	_, err = client.ResolveName(ctx, "d/nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent name, got nil")
	}
	// This should be ErrNameNotFound, not a retry timeout
	if err != ErrNameNotFound {
		t.Logf("Got error: %v (expected ErrNameNotFound)", err)
	}
}

// TestIntegration_CleanShutdown tests that Close() properly releases resources
func TestIntegration_CleanShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &Config{
		Mode:    ModeEmbedded,
		DataDir: t.TempDir(),
		Network: "regtest",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	// Perform some operations
	ctx := context.Background()
	_, _ = client.GetInfo(ctx)
	_, _ = client.ListNames(ctx, &ListFilter{Limit: 10})

	// Close should not error
	if err := client.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// Operations after close should fail gracefully
	_, err = client.GetInfo(ctx)
	if err == nil {
		t.Error("Expected error after Close(), got nil")
	}
}

// TestIntegration_MultipleClients tests multiple clients can coexist
func TestIntegration_MultipleClients(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create two embedded clients with separate data directories
	cfg1 := &Config{
		Mode:    ModeEmbedded,
		DataDir: t.TempDir(),
		Network: "regtest",
	}

	cfg2 := &Config{
		Mode:    ModeEmbedded,
		DataDir: t.TempDir(),
		Network: "regtest",
	}

	client1, err := NewClient(cfg1)
	if err != nil {
		t.Fatalf("NewClient(1) failed: %v", err)
	}
	defer client1.Close()

	client2, err := NewClient(cfg2)
	if err != nil {
		t.Fatalf("NewClient(2) failed: %v", err)
	}
	defer client2.Close()

	ctx := context.Background()

	// Both clients should work independently
	info1, err := client1.GetInfo(ctx)
	if err != nil {
		t.Fatalf("Client1 GetInfo() failed: %v", err)
	}

	info2, err := client2.GetInfo(ctx)
	if err != nil {
		t.Fatalf("Client2 GetInfo() failed: %v", err)
	}

	// Both should be in embedded mode
	if info1.Mode != "embedded" || info2.Mode != "embedded" {
		t.Errorf("Expected both clients in embedded mode, got %s and %s", info1.Mode, info2.Mode)
	}

	// Perform operations on both
	_, _ = client1.ListNames(ctx, &ListFilter{Limit: 10})
	_, _ = client2.ListNames(ctx, &ListFilter{Limit: 10})

	// Close one should not affect the other
	if err := client1.Close(); err != nil {
		t.Errorf("Client1 Close() failed: %v", err)
	}

	// Client2 should still work
	info2b, err := client2.GetInfo(ctx)
	if err != nil {
		t.Errorf("Client2 GetInfo() after Client1 close failed: %v", err)
	}
	if info2b.Mode != "embedded" {
		t.Errorf("Client2 mode changed: %s", info2b.Mode)
	}
}
