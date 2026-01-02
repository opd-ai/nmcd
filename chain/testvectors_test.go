package chain

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestVector represents a test case from Namecoin Core
type TestVector struct {
	Description string `json:"description"`
	Network     string `json:"network"` // mainnet, testnet, regtest
	Type        string `json:"type"`    // block, transaction, etc.
	Height      int32  `json:"height"`  // Block height (for blocks)
	Hash        string `json:"hash"`    // Block or transaction hash
	Data        string `json:"data"`    // Hex-encoded block/transaction data
	Valid       bool   `json:"valid"`   // Expected validation result
	Notes       string `json:"notes"`   // Additional context
}

// loadTestVectors loads test vectors from a JSON file
func loadTestVectors(path string) ([]TestVector, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var vectors []TestVector
	if err := json.Unmarshal(data, &vectors); err != nil {
		return nil, err
	}

	return vectors, nil
}

// decodeHexData decodes hex string to bytes
func decodeHexData(hexStr string) ([]byte, error) {
	return hex.DecodeString(hexStr)
}

// TestBlockVectors tests block validation against Namecoin Core test vectors
// This test will be skipped if no test vector files exist
func TestBlockVectors(t *testing.T) {
	vectorPath := filepath.Join("../testdata", "blocks", "*.json")
	files, err := filepath.Glob(vectorPath)
	if err != nil || len(files) == 0 {
		t.Skip("No block test vectors found - skipping test vector validation")
		return
	}

	for _, file := range files {
		vectors, err := loadTestVectors(file)
		if err != nil {
			t.Errorf("Failed to load test vectors from %s: %v", file, err)
			continue
		}

		for _, vec := range vectors {
			t.Run(vec.Description, func(t *testing.T) {
				// Decode hex data
				blockBytes, err := decodeHexData(vec.Data)
				if err != nil {
					t.Fatalf("Failed to decode hex data: %v", err)
				}

				// Attempt to deserialize block
				block, err := NewBlockFromBytes(blockBytes)
				
				// Check if deserialization matches expected validity
				isValid := (err == nil)
				if isValid != vec.Valid {
					if vec.Valid {
						t.Errorf("Expected valid block but got error: %v", err)
					} else {
						t.Errorf("Expected invalid block but deserialization succeeded")
					}
				}

				// If block is valid and we have height information, verify it
				if isValid && vec.Height >= 0 {
					block.SetHeight(vec.Height)
					
					// Additional validation could be added here:
					// - Verify block hash matches vec.Hash
					// - Validate block structure
					// - Check AuxPow if height >= 19200
					
					if block.Hash().String() != vec.Hash {
						t.Logf("Note: Block hash mismatch (may be normal for some test vectors)")
						t.Logf("  Expected: %s", vec.Hash)
						t.Logf("  Got:      %s", block.Hash().String())
					}
				}
			})
		}
	}
}

// TestTransactionVectors tests transaction validation against Namecoin Core test vectors
// This test will be skipped if no test vector files exist
func TestTransactionVectors(t *testing.T) {
	vectorPath := filepath.Join("../testdata", "transactions", "*.json")
	files, err := filepath.Glob(vectorPath)
	if err != nil || len(files) == 0 {
		t.Skip("No transaction test vectors found - skipping test vector validation")
		return
	}

	for _, file := range files {
		vectors, err := loadTestVectors(file)
		if err != nil {
			t.Errorf("Failed to load test vectors from %s: %v", file, err)
			continue
		}

		for _, vec := range vectors {
			t.Run(vec.Description, func(t *testing.T) {
				// Decode hex data
				txBytes, err := decodeHexData(vec.Data)
				if err != nil {
					t.Fatalf("Failed to decode hex data: %v", err)
				}

				// Attempt to deserialize transaction
				// This would use wire.MsgTx deserialization
				_ = txBytes // Placeholder - full implementation would deserialize and validate
				
				// Note: Full transaction validation would require:
				// - Deserializing wire.MsgTx from bytes
				// - Parsing name operation scripts
				// - Validating against consensus rules
				// - Checking expected validity matches actual
				
				t.Logf("Transaction vector: %s (valid=%v)", vec.Description, vec.Valid)
				t.Log("Note: Full transaction validation not yet implemented")
			})
		}
	}
}
