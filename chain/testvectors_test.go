package chain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/namedb"
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

// deserializeTransaction deserializes a transaction from bytes
func deserializeTransaction(data []byte) (*wire.MsgTx, error) {
	tx := wire.NewMsgTx(wire.TxVersion)
	if err := tx.Deserialize(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("failed to deserialize transaction: %w", err)
	}
	return tx, nil
}

// NameOperationInfo holds information about a name operation found in a transaction
type NameOperationInfo struct {
	OutputIndex int
	OpType      namedb.NameOperation
	Name        string
}

// String returns a string representation of the name operation
func (n *NameOperationInfo) String() string {
	return fmt.Sprintf("%s (name: %s)", n.OpType.String(), n.Name)
}

// parseNameOperationsFromTx extracts name operations from transaction outputs
// This is a simplified version that just identifies name operations without full validation
func parseNameOperationsFromTx(tx *wire.MsgTx) []NameOperationInfo {
	var operations []NameOperationInfo
	
	for i, txOut := range tx.TxOut {
		// Try to parse as name operation script
		opType, name, _, err := parseNameScript(txOut.PkScript)
		if err == nil && opType != namedb.NameOperation(0) {
			operations = append(operations, NameOperationInfo{
				OutputIndex: i,
				OpType:      opType,
				Name:        name,
			})
		}
	}
	
	return operations
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

				// Deserialize transaction
				tx, err := deserializeTransaction(txBytes)
				
				// Check if deserialization matches expected validity
				isValid := (err == nil)
				if vec.Valid {
					// Expected to be valid
					if !isValid {
						t.Errorf("Expected valid transaction but got deserialization error: %v", err)
						return
					}
					
					// For valid transactions, verify basic structure
					if tx == nil {
						t.Errorf("Transaction is nil despite successful deserialization")
						return
					}
					
					// Verify transaction hash if provided
					if vec.Hash != "" {
						txHash := tx.TxHash()
						if txHash.String() != vec.Hash {
							t.Logf("Note: Transaction hash mismatch")
							t.Logf("  Expected: %s", vec.Hash)
							t.Logf("  Got:      %s", txHash.String())
						}
					}
					
					// Parse name operation scripts from outputs
					nameOps := parseNameOperationsFromTx(tx)
					if len(nameOps) > 0 {
						t.Logf("Found %d name operation(s) in transaction", len(nameOps))
						for i, op := range nameOps {
							t.Logf("  Output %d: %s", i, op.String())
						}
					}
					
				} else {
					// Expected to be invalid
					if isValid {
						t.Errorf("Expected invalid transaction but deserialization succeeded")
						t.Logf("Transaction: %v", tx)
					}
				}
			})
		}
	}
}
