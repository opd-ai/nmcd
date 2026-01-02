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
			name:     "namespace prefix only - no content after slash",
			input:    "d/",
			expected: true, // IsValidNamespace only checks prefix; full validation in validateNameFormat
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

// TestGetAuxPowActivationHeight tests the helper function that returns
// the block height at which AuxPow (merged mining) becomes mandatory
// for each network.
func TestGetAuxPowActivationHeight(t *testing.T) {
	tests := []struct {
		name           string
		network        string
		expectedHeight int32
		description    string
	}{
		{
			name:           "mainnet activation height",
			network:        "mainnet",
			expectedHeight: 19200,
			description:    "Mainnet activated AuxPow at block 19,200 (circa 2011)",
		},
		{
			name:           "testnet activation height",
			network:        "testnet",
			expectedHeight: 19200,
			description:    "Testnet uses same activation height as mainnet",
		},
		{
			name:           "regtest activation height",
			network:        "regtest",
			expectedHeight: 999999999,
			description:    "Regtest has very high activation for testing flexibility",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get chain params for the network
			cfg := &Config{Network: tt.network}
			params := cfg.ChainParams()

			// Get activation height
			height := GetAuxPowActivationHeight(params)

			// Verify it matches expected height
			if height != tt.expectedHeight {
				t.Errorf("%s: expected activation height %d, got %d",
					tt.description, tt.expectedHeight, height)
			}
		})
	}
}

// TestAuxPowConstants validates the AuxPow-related constants are correct
func TestAuxPowConstants(t *testing.T) {
	// Test AuxPow version bit constant
	if AuxPowVersionBit != 0x100 {
		t.Errorf("AuxPowVersionBit should be 0x100, got 0x%x", AuxPowVersionBit)
	}

	// Test mainnet activation height
	if MainNetAuxPowActivationHeight != 19200 {
		t.Errorf("MainNetAuxPowActivationHeight should be 19200, got %d", MainNetAuxPowActivationHeight)
	}

	// Test testnet activation height
	if TestNetAuxPowActivationHeight != 19200 {
		t.Errorf("TestNetAuxPowActivationHeight should be 19200, got %d", TestNetAuxPowActivationHeight)
	}

	// Test regtest activation height is very high (for testing)
	if RegTestAuxPowActivationHeight < 1000000 {
		t.Errorf("RegTestAuxPowActivationHeight should be very high for testing, got %d", RegTestAuxPowActivationHeight)
	}

	// Verify that the version bit is within valid range
	// Block version is int32, so bit must be < 31
	bitPosition := 0
	for i := 0; i < 32; i++ {
		if (AuxPowVersionBit >> uint(i)) == 1 {
			bitPosition = i
			break
		}
	}
	if bitPosition >= 31 {
		t.Errorf("AuxPowVersionBit position %d is too high for int32", bitPosition)
	}
}
