package chain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMainnetTestVector(t *testing.T) {
	// Create a temporary test vector file
	tmpDir := t.TempDir()
	vectorPath := filepath.Join(tmpDir, "test_vector.json")

	// Write a valid test vector
	validVector := `{
  "description": "Test Block",
  "network": "mainnet",
  "type": "block",
  "height": 100,
  "hash": "abc123",
  "hex": "0102030405",
  "valid": true,
  "notes": "Test notes"
}`

	if err := os.WriteFile(vectorPath, []byte(validVector), 0644); err != nil {
		t.Fatalf("Failed to write test vector file: %v", err)
	}

	// Test loading
	vector, err := LoadMainnetTestVector(vectorPath)
	if err != nil {
		t.Fatalf("LoadMainnetTestVector failed: %v", err)
	}

	// Validate fields
	if vector.Description != "Test Block" {
		t.Errorf("Expected description 'Test Block', got '%s'", vector.Description)
	}
	if vector.Network != "mainnet" {
		t.Errorf("Expected network 'mainnet', got '%s'", vector.Network)
	}
	if vector.Type != "block" {
		t.Errorf("Expected type 'block', got '%s'", vector.Type)
	}
	if vector.Height != 100 {
		t.Errorf("Expected height 100, got %d", vector.Height)
	}
	if vector.Hash != "abc123" {
		t.Errorf("Expected hash 'abc123', got '%s'", vector.Hash)
	}
	if vector.Hex != "0102030405" {
		t.Errorf("Expected hex '0102030405', got '%s'", vector.Hex)
	}
	if !vector.Valid {
		t.Error("Expected valid=true, got false")
	}
	if vector.Notes != "Test notes" {
		t.Errorf("Expected notes 'Test notes', got '%s'", vector.Notes)
	}
}

func TestLoadMainnetTestVector_MissingFile(t *testing.T) {
	_, err := LoadMainnetTestVector("/nonexistent/path.json")
	if err == nil {
		t.Error("Expected error for missing file, got nil")
	}
}

func TestLoadMainnetTestVector_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	vectorPath := filepath.Join(tmpDir, "invalid.json")

	// Write invalid JSON
	if err := os.WriteFile(vectorPath, []byte("{invalid json}"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := LoadMainnetTestVector(vectorPath)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestLoadMainnetTestVector_MissingRequiredFields(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "missing type",
			content: `{"hex": "010203"}`,
		},
		{
			name:    "missing hex",
			content: `{"type": "block"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vectorPath := filepath.Join(tmpDir, tc.name+".json")
			if err := os.WriteFile(vectorPath, []byte(tc.content), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			_, err := LoadMainnetTestVector(vectorPath)
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestLoadMainnetTestVectors(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test vector files
	vectors := []struct {
		filename string
		content  string
	}{
		{
			"block_100.json",
			`{"description": "Block 100", "network": "mainnet", "type": "block", "height": 100, "hash": "hash100", "hex": "01", "valid": true}`,
		},
		{
			"block_200.json",
			`{"description": "Block 200", "network": "mainnet", "type": "block", "height": 200, "hash": "hash200", "hex": "02", "valid": true}`,
		},
		{
			"block_300.json",
			`{"description": "Block 300", "network": "mainnet", "type": "block", "height": 300, "hash": "hash300", "hex": "03", "valid": true}`,
		},
	}

	for _, v := range vectors {
		path := filepath.Join(tmpDir, v.filename)
		if err := os.WriteFile(path, []byte(v.content), 0644); err != nil {
			t.Fatalf("Failed to write test vector %s: %v", v.filename, err)
		}
	}

	// Load all vectors
	loaded, err := LoadMainnetTestVectors(tmpDir, "block_*.json")
	if err != nil {
		t.Fatalf("LoadMainnetTestVectors failed: %v", err)
	}

	// Verify count
	if len(loaded) != 3 {
		t.Errorf("Expected 3 vectors, got %d", len(loaded))
	}

	// Verify ordering (should be sorted by filename)
	expectedHeights := []int32{100, 200, 300}
	for i, vec := range loaded {
		if vec.Height != expectedHeights[i] {
			t.Errorf("Vector %d: expected height %d, got %d", i, expectedHeights[i], vec.Height)
		}
	}
}

func TestLoadMainnetTestVectors_NoMatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Try to load from empty directory
	_, err := LoadMainnetTestVectors(tmpDir, "*.json")
	if err == nil {
		t.Error("Expected error when no vectors found, got nil")
	}
}

func TestMainnetTestVector_DecodeHex(t *testing.T) {
	tests := []struct {
		name        string
		hex         string
		expected    []byte
		expectError bool
	}{
		{
			name:        "valid hex",
			hex:         "0102030405",
			expected:    []byte{0x01, 0x02, 0x03, 0x04, 0x05},
			expectError: false,
		},
		{
			name:        "hex with whitespace",
			hex:         "01 02 03\n04\t05",
			expected:    []byte{0x01, 0x02, 0x03, 0x04, 0x05},
			expectError: false,
		},
		{
			name:        "empty hex",
			hex:         "",
			expected:    []byte{},
			expectError: false,
		},
		{
			name:        "invalid hex (odd length)",
			hex:         "0102030",
			expectError: true,
		},
		{
			name:        "invalid hex (non-hex chars)",
			hex:         "01020g0405",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vector := &MainnetTestVector{
				Hex:    tc.hex,
				Height: 123,
				Hash:   "test",
			}

			decoded, err := vector.DecodeHex()
			if tc.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(decoded) != len(tc.expected) {
				t.Errorf("Expected %d bytes, got %d", len(tc.expected), len(decoded))
				return
			}

			for i := range decoded {
				if decoded[i] != tc.expected[i] {
					t.Errorf("Byte %d: expected 0x%02x, got 0x%02x", i, tc.expected[i], decoded[i])
				}
			}
		})
	}
}

func TestMainnetTestVector_NetworkChecks(t *testing.T) {
	tests := []struct {
		network   string
		isMainnet bool
		isTestnet bool
		isRegtest bool
	}{
		{"mainnet", true, false, false},
		{"testnet", false, true, false},
		{"regtest", false, false, true},
		{"unknown", false, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.network, func(t *testing.T) {
			vector := &MainnetTestVector{Network: tc.network}

			if vector.IsMainnet() != tc.isMainnet {
				t.Errorf("IsMainnet: expected %v, got %v", tc.isMainnet, vector.IsMainnet())
			}
			if vector.IsTestnet() != tc.isTestnet {
				t.Errorf("IsTestnet: expected %v, got %v", tc.isTestnet, vector.IsTestnet())
			}
			if vector.IsRegtest() != tc.isRegtest {
				t.Errorf("IsRegtest: expected %v, got %v", tc.isRegtest, vector.IsRegtest())
			}
		})
	}
}

func TestMainnetTestVector_IsBlock(t *testing.T) {
	tests := []struct {
		typ      string
		expected bool
	}{
		{"block", true},
		{"transaction", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.typ, func(t *testing.T) {
			vector := &MainnetTestVector{Type: tc.typ}
			if vector.IsBlock() != tc.expected {
				t.Errorf("IsBlock: expected %v, got %v", tc.expected, vector.IsBlock())
			}
		})
	}
}

func TestMainnetTestVector_String(t *testing.T) {
	vector := &MainnetTestVector{
		Height:      19200,
		Hash:        "abc123def456",
		Network:     "mainnet",
		Description: "AuxPoW Activation",
	}

	str := vector.String()

	// Verify string contains key information
	if !contains(str, "19200") {
		t.Error("String should contain height")
	}
	if !contains(str, "abc123def456") {
		t.Error("String should contain hash")
	}
	if !contains(str, "mainnet") {
		t.Error("String should contain network")
	}
	if !contains(str, "AuxPoW Activation") {
		t.Error("String should contain description")
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
