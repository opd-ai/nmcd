package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Network != "mainnet" {
		t.Errorf("Expected network 'mainnet', got '%s'", cfg.Network)
	}

	if cfg.RPCAddr != "127.0.0.1:8336" {
		t.Errorf("Expected RPC addr '127.0.0.1:8336', got '%s'", cfg.RPCAddr)
	}

	if len(cfg.ListenAddrs) == 0 {
		t.Error("Expected at least one listen address")
	}
}

func TestChainParams(t *testing.T) {
	tests := []struct {
		network      string
		expectedName string
	}{
		{"mainnet", chaincfg.MainNetParams.Name},
		{"testnet", chaincfg.TestNet3Params.Name},
		{"regtest", chaincfg.RegressionNetParams.Name},
	}

	for _, tt := range tests {
		t.Run(tt.network, func(t *testing.T) {
			cfg := &Config{Network: tt.network}
			params := cfg.ChainParams()
			if params.Name != tt.expectedName {
				t.Errorf("Expected %s, got %s", tt.expectedName, params.Name)
			}
		})
	}
}

func TestNameDBPath(t *testing.T) {
	cfg := &Config{
		DataDir: "/tmp/nmcd-test",
	}

	expected := filepath.Join("/tmp/nmcd-test", "names.db")
	result := cfg.NameDBPath()

	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestEnsureDataDir(t *testing.T) {
	testDir := "/tmp/nmcd-test-data"
	defer os.RemoveAll(testDir)

	cfg := &Config{
		DataDir: testDir,
	}

	err := cfg.EnsureDataDir()
	if err != nil {
		t.Fatalf("Failed to ensure data dir: %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(testDir)
	if err != nil {
		t.Fatalf("Data dir not created: %v", err)
	}

	if !info.IsDir() {
		t.Error("Data dir is not a directory")
	}
}
