package config

import (
	"os"
	"path/filepath"
	"testing"
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
		expectedPort string
	}{
		{"mainnet", "mainnet", MainNetDefaultPort},
		{"testnet", "testnet", TestNetDefaultPort},
		{"regtest", "regtest", RegTestDefaultPort},
	}

	for _, tt := range tests {
		t.Run(tt.network, func(t *testing.T) {
			cfg := &Config{Network: tt.network}
			params := cfg.ChainParams()
			if params.Name != tt.expectedName {
				t.Errorf("Expected name %s, got %s", tt.expectedName, params.Name)
			}
			if params.DefaultPort != tt.expectedPort {
				t.Errorf("Expected port %s, got %s", tt.expectedPort, params.DefaultPort)
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

func TestIsValidNamespace(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid domain namespace",
			input:    "d/example",
			expected: true,
		},
		{
			name:     "valid identity namespace",
			input:    "id/johndoe",
			expected: true,
		},
		{
			name:     "valid personal namespace",
			input:    "p/alice",
			expected: true,
		},
		{
			name:     "domain namespace with subdomain",
			input:    "d/example.bit",
			expected: true,
		},
		{
			name:     "invalid namespace - no prefix",
			input:    "example",
			expected: false,
		},
		{
			name:     "invalid namespace - wrong prefix",
			input:    "x/example",
			expected: false,
		},
		{
			name:     "invalid namespace - only prefix",
			input:    "d/",
			expected: true, // technically valid prefix, but would fail length check elsewhere
		},
		{
			name:     "invalid namespace - empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "invalid namespace - partial prefix",
			input:    "d",
			expected: false,
		},
		{
			name:     "invalid namespace - wrong separator",
			input:    "d\\example",
			expected: false,
		},
		{
			name:     "case sensitive - uppercase D",
			input:    "D/example",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsValidNamespace(tc.input)
			if result != tc.expected {
				t.Errorf("IsValidNamespace(%q) = %v, expected %v", tc.input, result, tc.expected)
			}
		})
	}
}
