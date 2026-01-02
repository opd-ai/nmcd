package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/opd-ai/nmcd/namedb"
)

func TestNewEmbeddedClient(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *Config
		wantError bool
		errorMsg  string
	}{
		{
			name:      "nil config uses defaults",
			cfg:       nil,
			wantError: false,
		},
		{
			name: "custom data directory",
			cfg: &Config{
				DataDir: t.TempDir(),
				Network: "regtest",
			},
			wantError: false,
		},
		{
			name: "mainnet configuration",
			cfg: &Config{
				DataDir: t.TempDir(),
				Network: "mainnet",
			},
			wantError: false,
		},
		{
			name: "testnet configuration",
			cfg: &Config{
				DataDir: t.TempDir(),
				Network: "testnet",
			},
			wantError: false,
		},
		{
			name: "invalid network",
			cfg: &Config{
				DataDir: t.TempDir(),
				Network: "invalid",
			},
			wantError: true,
			errorMsg:  "unknown network",
		},
		{
			name: "wallet disabled",
			cfg: &Config{
				DataDir:       t.TempDir(),
				Network:       "regtest",
				DisableWallet: true,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use temp directory for nil config test
			if tt.cfg == nil {
				tt.cfg = &Config{
					DataDir: t.TempDir(),
					Network: "regtest",
				}
			}

			client, err := NewEmbeddedClient(tt.cfg)
			if tt.wantError {
				if err == nil {
					t.Errorf("NewEmbeddedClient() expected error containing %q, got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("NewEmbeddedClient() error = %v, want error containing %q", err, tt.errorMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewEmbeddedClient() unexpected error: %v", err)
			}
			defer client.Close()

			// Verify client state
			if client.chain == nil {
				t.Error("chain is nil")
			}
			if client.nameDB == nil {
				t.Error("nameDB is nil")
			}
			if !tt.cfg.DisableWallet && client.wallet == nil {
				t.Error("wallet is nil but should be initialized")
			}
			if tt.cfg.DisableWallet && client.wallet != nil {
				t.Error("wallet is not nil but should be disabled")
			}
			if client.network != tt.cfg.Network {
				t.Errorf("network = %v, want %v", client.network, tt.cfg.Network)
			}
			if client.dataDir != tt.cfg.DataDir {
				t.Errorf("dataDir = %v, want %v", client.dataDir, tt.cfg.DataDir)
			}
			if client.closed {
				t.Error("client should not be closed after initialization")
			}
		})
	}
}

func TestEmbeddedClient_ResolveName(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()

	// Initialize client
	cfg := &Config{
		DataDir: tmpDir,
		Network: "regtest",
	}
	client, err := NewEmbeddedClient(cfg)
	if err != nil {
		t.Fatalf("NewEmbeddedClient() error: %v", err)
	}
	defer client.Close()

	// Insert test name into database
	testName := "d/test"
	testValue := `{"ip":"1.2.3.4"}`
	testAddr := "NTest1234567890abcdefghijklmno"
	txHash, _ := chainhash.NewHashFromStr("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")

	// For Phase 2, height is always 0 (blockchain not fully integrated yet)
	currentHeight := int32(0)
	expiresAt := currentHeight + 36000

	nameRecord := &namedb.NameRecord{
		Value:     testValue,
		TxHash:    *txHash,
		Height:    currentHeight,
		ExpiresAt: expiresAt,
		Address:   testAddr,
		UpdatedAt: time.Now(),
	}

	if err := client.nameDB.PutName(testName, nameRecord); err != nil {
		t.Fatalf("PutName() error: %v", err)
	}

	tests := []struct {
		name      string
		ctx       context.Context
		nameLookup string
		want      *NameRecord
		wantError error
	}{
		{
			name:       "resolve existing name",
			ctx:        context.Background(),
			nameLookup: testName,
			want: &NameRecord{
				Name:      testName,
				Value:     testValue,
				TxHash:    txHash.String(),
				Height:    currentHeight,
				ExpiresAt: expiresAt,
				ExpiresIn: expiresAt - currentHeight,
				Address:   testAddr,
			},
			wantError: nil,
		},
		{
			name:       "name not found",
			ctx:        context.Background(),
			nameLookup: "d/nonexistent",
			want:       nil,
			wantError:  ErrNameNotFound,
		},
		{
			name:       "empty name",
			ctx:        context.Background(),
			nameLookup: "",
			want:       nil,
			wantError:  ErrInvalidName,
		},
		{
			name:       "context canceled",
			ctx:        canceledContext(),
			nameLookup: testName,
			want:       nil,
			wantError:  ErrContextCanceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.ResolveName(tt.ctx, tt.nameLookup)

			if tt.wantError != nil {
				if err != tt.wantError {
					t.Errorf("ResolveName() error = %v, want %v", err, tt.wantError)
				}
				return
			}

			if err != nil {
				t.Fatalf("ResolveName() unexpected error: %v", err)
			}

			// Verify record fields (excluding UpdatedAt which may vary slightly)
			if got.Name != tt.want.Name {
				t.Errorf("Name = %v, want %v", got.Name, tt.want.Name)
			}
			if got.Value != tt.want.Value {
				t.Errorf("Value = %v, want %v", got.Value, tt.want.Value)
			}
			if got.TxHash != tt.want.TxHash {
				t.Errorf("TxHash = %v, want %v", got.TxHash, tt.want.TxHash)
			}
			if got.Height != tt.want.Height {
				t.Errorf("Height = %v, want %v", got.Height, tt.want.Height)
			}
			if got.ExpiresAt != tt.want.ExpiresAt {
				t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, tt.want.ExpiresAt)
			}
			if got.ExpiresIn != tt.want.ExpiresIn {
				t.Errorf("ExpiresIn = %v, want %v", got.ExpiresIn, tt.want.ExpiresIn)
			}
			if got.Address != tt.want.Address {
				t.Errorf("Address = %v, want %v", got.Address, tt.want.Address)
			}
		})
	}
}

func TestEmbeddedClient_ResolveExpiredName(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()

	// Initialize client
	cfg := &Config{
		DataDir: tmpDir,
		Network: "regtest",
	}
	client, err := NewEmbeddedClient(cfg)
	if err != nil {
		t.Fatalf("NewEmbeddedClient() error: %v", err)
	}
	defer client.Close()

	// Insert expired name into database
	testName := "d/expired"
	testValue := `{"ip":"1.2.3.4"}`
	testAddr := "NTest1234567890abcdefghijklmno"
	txHash, _ := chainhash.NewHashFromStr("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")

	// For Phase 2, height is always 0 (blockchain not fully integrated yet)
	// So we set expiration to negative to test expired names
	currentHeight := int32(0)
	expiresAt := int32(-100) // Expired

	nameRecord := &namedb.NameRecord{
		Value:     testValue,
		TxHash:    *txHash,
		Height:    currentHeight - 36100,
		ExpiresAt: expiresAt,
		Address:   testAddr,
		UpdatedAt: time.Now(),
	}

	if err := client.nameDB.PutName(testName, nameRecord); err != nil {
		t.Fatalf("PutName() error: %v", err)
	}

	// Try to resolve expired name
	ctx := context.Background()
	_, err = client.ResolveName(ctx, testName)
	if err != ErrNameExpired {
		t.Errorf("ResolveName() error = %v, want %v", err, ErrNameExpired)
	}
}

func TestEmbeddedClient_GetInfo(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()

	// Initialize client
	cfg := &Config{
		DataDir: tmpDir,
		Network: "regtest",
	}
	client, err := NewEmbeddedClient(cfg)
	if err != nil {
		t.Fatalf("NewEmbeddedClient() error: %v", err)
	}
	defer client.Close()

	tests := []struct {
		name      string
		ctx       context.Context
		wantError error
	}{
		{
			name:      "get info success",
			ctx:       context.Background(),
			wantError: nil,
		},
		{
			name:      "context canceled",
			ctx:       canceledContext(),
			wantError: ErrContextCanceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := client.GetInfo(tt.ctx)

			if tt.wantError != nil {
				if err != tt.wantError {
					t.Errorf("GetInfo() error = %v, want %v", err, tt.wantError)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetInfo() unexpected error: %v", err)
			}

			// Verify info fields
			if info.Version == "" {
				t.Error("Version is empty")
			}
			if info.ProtocolVersion == 0 {
				t.Error("ProtocolVersion is 0")
			}
			// For Phase 2, BlockHeight is always 0 (placeholder)
			if info.BlockHeight != 0 {
				t.Errorf("BlockHeight = %v, want 0 (Phase 2 placeholder)", info.BlockHeight)
			}
			if info.BestBlockHash == "" {
				t.Error("BestBlockHash is empty")
			}
			if info.NetworkName != cfg.Network {
				t.Errorf("NetworkName = %v, want %v", info.NetworkName, cfg.Network)
			}
			if info.Mode != "embedded" {
				t.Errorf("Mode = %v, want %v", info.Mode, "embedded")
			}
		})
	}
}

func TestEmbeddedClient_Close(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()

	// Initialize client
	cfg := &Config{
		DataDir: tmpDir,
		Network: "regtest",
	}
	client, err := NewEmbeddedClient(cfg)
	if err != nil {
		t.Fatalf("NewEmbeddedClient() error: %v", err)
	}

	// Close client
	if err := client.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}

	// Verify client is closed
	if !client.closed {
		t.Error("client.closed = false, want true")
	}

	// Second close should not error
	if err := client.Close(); err != nil {
		t.Errorf("Second Close() error: %v", err)
	}

	// Operations after close should error
	ctx := context.Background()
	_, err = client.ResolveName(ctx, "d/test")
	if err == nil {
		t.Error("ResolveName() after Close() should error, got nil")
	}

	_, err = client.GetInfo(ctx)
	if err == nil {
		t.Error("GetInfo() after Close() should error, got nil")
	}
}

func TestEmbeddedClient_NotImplementedMethods(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()

	// Initialize client
	cfg := &Config{
		DataDir: tmpDir,
		Network: "regtest",
	}
	client, err := NewEmbeddedClient(cfg)
	if err != nil {
		t.Fatalf("NewEmbeddedClient() error: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Test not-yet-implemented methods return appropriate errors
	_, err = client.RegisterName(ctx, "d/test", "value", nil)
	if err == nil {
		t.Error("RegisterName() should return not implemented error")
	}

	_, err = client.UpdateName(ctx, "d/test", "value", nil)
	if err == nil {
		t.Error("UpdateName() should return not implemented error")
	}

	_, err = client.ListNames(ctx, nil)
	if err == nil {
		t.Error("ListNames() should return not implemented error")
	}

	_, err = client.GetNameHistory(ctx, "d/test")
	if err == nil {
		t.Error("GetNameHistory() should return not implemented error")
	}

	err = client.WaitForConfirmation(ctx, "txhash", 1)
	if err == nil {
		t.Error("WaitForConfirmation() should return not implemented error")
	}
}

func TestEmbeddedClient_DataDirectoryCreation(t *testing.T) {
	// Create a non-existent directory path
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "nested", "directory", "structure")

	cfg := &Config{
		DataDir: dataDir,
		Network: "regtest",
	}

	client, err := NewEmbeddedClient(cfg)
	if err != nil {
		t.Fatalf("NewEmbeddedClient() error: %v", err)
	}
	defer client.Close()

	// Verify directory was created
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Errorf("Data directory was not created: %s", dataDir)
	}

	// Verify files were created in the directory
	dbPath := filepath.Join(dataDir, "names.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Database file was not created: %s", dbPath)
	}
}

func TestEmbeddedClient_ThreadSafety(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()

	// Initialize client
	cfg := &Config{
		DataDir: tmpDir,
		Network: "regtest",
	}
	client, err := NewEmbeddedClient(cfg)
	if err != nil {
		t.Fatalf("NewEmbeddedClient() error: %v", err)
	}
	defer client.Close()

	// Insert test name
	testName := "d/concurrent"
	testValue := `{"test":"value"}`
	txHash, _ := chainhash.NewHashFromStr("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")

	// For Phase 2, height is always 0
	currentHeight := int32(0)
	nameRecord := &namedb.NameRecord{
		Value:     testValue,
		TxHash:    *txHash,
		Height:    currentHeight,
		ExpiresAt: currentHeight + 36000,
		Address:   "NTest1234567890abcdefghijklmno",
		UpdatedAt: time.Now(),
	}

	if err := client.nameDB.PutName(testName, nameRecord); err != nil {
		t.Fatalf("PutName() error: %v", err)
	}

	// Run concurrent ResolveName operations
	const numGoroutines = 10
	const numOperations = 100

	errCh := make(chan error, numGoroutines*numOperations)
	doneCh := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		go func() {
			ctx := context.Background()
			for j := 0; j < numOperations; j++ {
				_, err := client.ResolveName(ctx, testName)
				if err != nil {
					errCh <- err
					return
				}

				_, err = client.GetInfo(ctx)
				if err != nil {
					errCh <- err
					return
				}
			}
			doneCh <- struct{}{}
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		select {
		case err := <-errCh:
			t.Fatalf("Concurrent operation error: %v", err)
		case <-doneCh:
			// Goroutine completed successfully
		}
	}
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
