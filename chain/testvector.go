package chain

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MainnetTestVector represents a blockchain test case from real Namecoin mainnet data
type MainnetTestVector struct {
	Description string `json:"description"` // Human-readable description
	Network     string `json:"network"`     // mainnet, testnet, or regtest
	Type        string `json:"type"`        // "block" for block vectors
	Height      int32  `json:"height"`      // Block height
	Hash        string `json:"hash"`        // Block hash (hex)
	Hex         string `json:"hex"`         // Serialized block data (hex)
	Valid       bool   `json:"valid"`       // Expected validation result
	Notes       string `json:"notes"`       // Additional context
}

// LoadMainnetTestVector loads a single mainnet test vector from a JSON file
func LoadMainnetTestVector(path string) (*MainnetTestVector, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read test vector file %s: %w", path, err)
	}

	var vector MainnetTestVector
	if err := json.Unmarshal(data, &vector); err != nil {
		return nil, fmt.Errorf("failed to parse test vector JSON from %s: %w", path, err)
	}

	// Validate required fields
	if vector.Type == "" {
		return nil, fmt.Errorf("test vector missing 'type' field")
	}
	if vector.Hex == "" {
		return nil, fmt.Errorf("test vector missing 'hex' field")
	}

	return &vector, nil
}

// LoadMainnetTestVectors loads all mainnet test vectors from a directory matching a glob pattern
// Example: LoadMainnetTestVectors("testdata/blocks", "block_*.json")
func LoadMainnetTestVectors(dir string, pattern string) ([]*MainnetTestVector, error) {
	globPattern := filepath.Join(dir, pattern)
	matches, err := filepath.Glob(globPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob pattern %s: %w", globPattern, err)
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no test vectors found matching pattern %s", globPattern)
	}

	// Sort for deterministic ordering
	sort.Strings(matches)

	vectors := make([]*MainnetTestVector, 0, len(matches))
	for _, path := range matches {
		vector, err := LoadMainnetTestVector(path)
		if err != nil {
			return nil, fmt.Errorf("failed to load test vector %s: %w", path, err)
		}
		vectors = append(vectors, vector)
	}

	return vectors, nil
}

// DecodeHex decodes the hex-encoded block data
func (v *MainnetTestVector) DecodeHex() ([]byte, error) {
	// Remove any whitespace
	hexStr := strings.ReplaceAll(v.Hex, " ", "")
	hexStr = strings.ReplaceAll(hexStr, "\n", "")
	hexStr = strings.ReplaceAll(hexStr, "\t", "")

	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hex data for block %s (height %d): %w",
			v.Hash, v.Height, err)
	}

	return data, nil
}

// IsMainnet returns true if this test vector is from mainnet
func (v *MainnetTestVector) IsMainnet() bool {
	return v.Network == "mainnet"
}

// IsTestnet returns true if this test vector is from testnet
func (v *MainnetTestVector) IsTestnet() bool {
	return v.Network == "testnet"
}

// IsRegtest returns true if this test vector is from regtest
func (v *MainnetTestVector) IsRegtest() bool {
	return v.Network == "regtest"
}

// IsBlock returns true if this is a block test vector
func (v *MainnetTestVector) IsBlock() bool {
	return v.Type == "block"
}

// String returns a human-readable representation of the test vector
func (v *MainnetTestVector) String() string {
	return fmt.Sprintf("TestVector{height=%d, hash=%s, network=%s, description=\"%s\"}",
		v.Height, v.Hash, v.Network, v.Description)
}
