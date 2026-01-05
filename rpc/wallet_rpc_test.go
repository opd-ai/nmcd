package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/opd-ai/nmcd/wallet"
)

func setupTestServerWithWallet(t *testing.T) (*Server, func()) {
	tmpDir := t.TempDir()

	// Create wallet
	w, err := wallet.NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	// Create RPC server (minimal setup for wallet testing)
	server := &Server{
		wallet: w,
	}

	cleanup := func() {
		// Nothing to clean up beyond temp dir
	}

	return server, cleanup
}

func makeRPCRequest(t *testing.T, server *Server, method string, params interface{}) *Response {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal params: %v", err)
	}

	req := &Request{
		Jsonrpc: "2.0",
		Method:  method,
		Params:  paramsJSON,
		ID:      1,
	}

	return server.processRequest(req)
}

func TestEncryptWallet(t *testing.T) {
	server, cleanup := setupTestServerWithWallet(t)
	defer cleanup()

	// Verify wallet is not encrypted initially
	if server.wallet.IsEncrypted() {
		t.Fatal("wallet should not be encrypted initially")
	}

	// Encrypt wallet
	resp := makeRPCRequest(t, server, "encryptwallet", []string{"TestPassword123"})
	if resp.Error != nil {
		t.Fatalf("encryptwallet failed: %v", resp.Error.Message)
	}

	// Verify wallet is now encrypted
	if !server.wallet.IsEncrypted() {
		t.Error("wallet should be encrypted after encryptwallet")
	}

	// Try to encrypt again (should fail)
	resp = makeRPCRequest(t, server, "encryptwallet", []string{"AnotherPassword456"})
	if resp.Error == nil {
		t.Error("encrypting already encrypted wallet should fail")
	}
}

func TestEncryptWalletWeakPassword(t *testing.T) {
	server, cleanup := setupTestServerWithWallet(t)
	defer cleanup()

	// Try to encrypt with weak password
	resp := makeRPCRequest(t, server, "encryptwallet", []string{"weak"})
	if resp.Error == nil {
		t.Error("encryptwallet should reject weak password")
	}
	if resp.Error.Code != -8 {
		t.Errorf("expected error code -8, got %d", resp.Error.Code)
	}
}

func TestWalletPassphrase(t *testing.T) {
	server, cleanup := setupTestServerWithWallet(t)
	defer cleanup()

	// Encrypt wallet first
	password := "TestPassword123"
	resp := makeRPCRequest(t, server, "encryptwallet", []string{password})
	if resp.Error != nil {
		t.Fatalf("encryptwallet failed: %v", resp.Error.Message)
	}

	// Lock the wallet
	if err := server.wallet.Lock(); err != nil {
		t.Fatalf("failed to lock wallet: %v", err)
	}

	// Verify wallet is locked
	if !server.wallet.IsLocked() {
		t.Fatal("wallet should be locked")
	}

	// Unlock with walletpassphrase
	resp = makeRPCRequest(t, server, "walletpassphrase", []interface{}{password, 60})
	if resp.Error != nil {
		t.Fatalf("walletpassphrase failed: %v", resp.Error.Message)
	}

	// Verify wallet is unlocked
	if server.wallet.IsLocked() {
		t.Error("wallet should be unlocked after walletpassphrase")
	}
}

func TestWalletPassphraseWrongPassword(t *testing.T) {
	server, cleanup := setupTestServerWithWallet(t)
	defer cleanup()

	// Encrypt wallet
	correctPassword := "CorrectPassword123"
	resp := makeRPCRequest(t, server, "encryptwallet", []string{correctPassword})
	if resp.Error != nil {
		t.Fatalf("encryptwallet failed: %v", resp.Error.Message)
	}

	// Lock wallet
	if err := server.wallet.Lock(); err != nil {
		t.Fatalf("failed to lock wallet: %v", err)
	}

	// Try to unlock with wrong password
	resp = makeRPCRequest(t, server, "walletpassphrase", []interface{}{"WrongPassword456", 60})
	if resp.Error == nil {
		t.Error("walletpassphrase should fail with wrong password")
	}
	if resp.Error.Code != -14 {
		t.Errorf("expected error code -14, got %d", resp.Error.Code)
	}

	// Verify wallet is still locked
	if !server.wallet.IsLocked() {
		t.Error("wallet should remain locked after failed unlock")
	}
}

func TestWalletPassphraseAutoLock(t *testing.T) {
	server, cleanup := setupTestServerWithWallet(t)
	defer cleanup()

	// Encrypt wallet
	password := "TestPassword123"
	resp := makeRPCRequest(t, server, "encryptwallet", []string{password})
	if resp.Error != nil {
		t.Fatalf("encryptwallet failed: %v", resp.Error.Message)
	}

	// Lock wallet
	if err := server.wallet.Lock(); err != nil {
		t.Fatalf("failed to lock wallet: %v", err)
	}

	// Unlock with short timeout (1 second)
	resp = makeRPCRequest(t, server, "walletpassphrase", []interface{}{password, 1})
	if resp.Error != nil {
		t.Fatalf("walletpassphrase failed: %v", resp.Error.Message)
	}

	// Verify wallet is unlocked
	if server.wallet.IsLocked() {
		t.Error("wallet should be unlocked immediately after walletpassphrase")
	}

	// Wait for auto-lock
	time.Sleep(1500 * time.Millisecond)

	// Verify wallet is locked again
	if !server.wallet.IsLocked() {
		t.Error("wallet should be locked after timeout")
	}
}

func TestWalletPassphraseManualLockBeforeTimeout(t *testing.T) {
	server, cleanup := setupTestServerWithWallet(t)
	defer cleanup()

	// Encrypt wallet
	password := "TestPassword123"
	resp := makeRPCRequest(t, server, "encryptwallet", []string{password})
	if resp.Error != nil {
		t.Fatalf("encryptwallet failed: %v", resp.Error.Message)
	}

	// Lock wallet
	if err := server.wallet.Lock(); err != nil {
		t.Fatalf("failed to lock wallet: %v", err)
	}

	// Unlock with longer timeout (5 seconds)
	resp = makeRPCRequest(t, server, "walletpassphrase", []interface{}{password, 5})
	if resp.Error != nil {
		t.Fatalf("walletpassphrase failed: %v", resp.Error.Message)
	}

	// Verify wallet is unlocked
	if server.wallet.IsLocked() {
		t.Error("wallet should be unlocked immediately after walletpassphrase")
	}

	// Manually lock before timeout
	resp = makeRPCRequest(t, server, "walletlock", nil)
	if resp.Error != nil {
		t.Fatalf("walletlock failed: %v", resp.Error.Message)
	}

	// Verify wallet is locked
	if !server.wallet.IsLocked() {
		t.Error("wallet should be locked after manual lock")
	}

	// Wait past the original timeout to ensure no issues with timer
	time.Sleep(6 * time.Second)

	// Verify wallet is still locked (timer shouldn't cause issues)
	if !server.wallet.IsLocked() {
		t.Error("wallet should remain locked after timeout")
	}
}

func TestWalletLock(t *testing.T) {
	server, cleanup := setupTestServerWithWallet(t)
	defer cleanup()

	// Encrypt wallet
	password := "TestPassword123"
	resp := makeRPCRequest(t, server, "encryptwallet", []string{password})
	if resp.Error != nil {
		t.Fatalf("encryptwallet failed: %v", resp.Error.Message)
	}

	// Wallet should be unlocked after encryption
	if server.wallet.IsLocked() {
		t.Fatal("wallet should be unlocked after encryption")
	}

	// Lock the wallet
	resp = makeRPCRequest(t, server, "walletlock", nil)
	if resp.Error != nil {
		t.Fatalf("walletlock failed: %v", resp.Error.Message)
	}

	// Verify wallet is locked
	if !server.wallet.IsLocked() {
		t.Error("wallet should be locked after walletlock")
	}
}

func TestWalletLockUnencrypted(t *testing.T) {
	server, cleanup := setupTestServerWithWallet(t)
	defer cleanup()

	// Try to lock unencrypted wallet (should fail)
	resp := makeRPCRequest(t, server, "walletlock", nil)
	if resp.Error == nil {
		t.Error("walletlock should fail for unencrypted wallet")
	}
	if resp.Error.Code != -13 {
		t.Errorf("expected error code -13, got %d", resp.Error.Code)
	}
}

func TestWalletPassphraseUnencrypted(t *testing.T) {
	server, cleanup := setupTestServerWithWallet(t)
	defer cleanup()

	// Try to unlock unencrypted wallet (should fail)
	resp := makeRPCRequest(t, server, "walletpassphrase", []interface{}{"password", 60})
	if resp.Error == nil {
		t.Error("walletpassphrase should fail for unencrypted wallet")
	}
	if resp.Error.Code != -13 {
		t.Errorf("expected error code -13, got %d", resp.Error.Code)
	}
}

func TestWalletEncryptionHTTP(t *testing.T) {
	tmpDir := t.TempDir()

	// Create wallet
	w, err := wallet.NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	// Create full RPC server for HTTP testing
	server, err := NewServer(&Config{
		Wallet:     w,
		ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Start server
	errCh := server.Start()
	defer server.Stop()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Check for startup errors
	select {
	case err := <-errCh:
		t.Fatalf("server failed to start: %v", err)
	default:
	}

	// Get server address
	addr := server.listener.Addr().String()

	// Helper to make HTTP RPC request
	makeHTTPRequest := func(method string, params interface{}) map[string]interface{} {
		reqBody := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  method,
			"params":  params,
			"id":      1,
		}

		reqJSON, err := json.Marshal(reqBody)
		if err != nil {
			t.Fatalf("failed to marshal request: %v", err)
		}

		resp, err := http.Post("http://"+addr, "application/json", bytes.NewReader(reqJSON))
		if err != nil {
			t.Fatalf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		return result
	}

	// Encrypt wallet via HTTP
	result := makeHTTPRequest("encryptwallet", []string{"TestPassword123"})
	if errObj, ok := result["error"]; ok && errObj != nil {
		t.Fatalf("encryptwallet failed: %v", errObj)
	}

	// Lock wallet
	if err := server.wallet.Lock(); err != nil {
		t.Fatalf("failed to lock wallet: %v", err)
	}

	// Unlock wallet via HTTP
	result = makeHTTPRequest("walletpassphrase", []interface{}{"TestPassword123", 60})
	if errObj, ok := result["error"]; ok && errObj != nil {
		t.Fatalf("walletpassphrase failed: %v", errObj)
	}

	// Verify wallet is unlocked
	if server.wallet.IsLocked() {
		t.Error("wallet should be unlocked")
	}

	// Lock wallet via HTTP
	result = makeHTTPRequest("walletlock", nil)
	if errObj, ok := result["error"]; ok && errObj != nil {
		t.Fatalf("walletlock failed: %v", errObj)
	}

	// Verify wallet is locked
	if !server.wallet.IsLocked() {
		t.Error("wallet should be locked")
	}
}

func TestEncryptWalletNoParams(t *testing.T) {
	server, cleanup := setupTestServerWithWallet(t)
	defer cleanup()

	resp := makeRPCRequest(t, server, "encryptwallet", []string{})
	if resp.Error == nil {
		t.Error("encryptwallet should fail with no params")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("expected error code -32602, got %d", resp.Error.Code)
	}
}

func TestWalletPassphraseInvalidTimeout(t *testing.T) {
	server, cleanup := setupTestServerWithWallet(t)
	defer cleanup()

	// Encrypt wallet
	resp := makeRPCRequest(t, server, "encryptwallet", []string{"TestPassword123"})
	if resp.Error != nil {
		t.Fatalf("encryptwallet failed: %v", resp.Error.Message)
	}

	// Lock wallet
	if err := server.wallet.Lock(); err != nil {
		t.Fatalf("failed to lock wallet: %v", err)
	}

	// Try with invalid timeout (negative)
	resp = makeRPCRequest(t, server, "walletpassphrase", []interface{}{"TestPassword123", -1})
	if resp.Error == nil {
		t.Error("walletpassphrase should fail with negative timeout")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("expected error code -32602, got %d", resp.Error.Code)
	}
}
