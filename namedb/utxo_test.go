package namedb

import (
	"os"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

func TestUTXOOperations(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test_utxo.db"
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create test UTXO
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	utxo := &UTXO{
		TxHash:   *txHash,
		OutIndex: 0,
		Value:    50000000,
		Address:  "N123456789",
		PkScript: []byte{0x76, 0xa9, 0x14, 0x89, 0xab, 0xcd, 0xef},
		Height:   100,
	}

	// Test AddUTXO
	err = db.AddUTXO(utxo)
	if err != nil {
		t.Fatalf("AddUTXO failed: %v", err)
	}

	// Test GetUTXO
	retrieved, err := db.GetUTXO(txHash, 0)
	if err != nil {
		t.Fatalf("GetUTXO failed: %v", err)
	}

	if retrieved.Value != utxo.Value {
		t.Errorf("Value mismatch: got %d, want %d", retrieved.Value, utxo.Value)
	}
	if retrieved.Address != utxo.Address {
		t.Errorf("Address mismatch: got %s, want %s", retrieved.Address, utxo.Address)
	}
	if retrieved.Height != utxo.Height {
		t.Errorf("Height mismatch: got %d, want %d", retrieved.Height, utxo.Height)
	}

	// Test GetUTXOsForAddress
	utxos, err := db.GetUTXOsForAddress("N123456789")
	if err != nil {
		t.Fatalf("GetUTXOsForAddress failed: %v", err)
	}
	if len(utxos) != 1 {
		t.Fatalf("Expected 1 UTXO, got %d", len(utxos))
	}
	if utxos[0].Value != 50000000 {
		t.Errorf("UTXO value mismatch: got %d, want 50000000", utxos[0].Value)
	}

	// Test RemoveUTXO
	err = db.RemoveUTXO(txHash, 0)
	if err != nil {
		t.Fatalf("RemoveUTXO failed: %v", err)
	}

	// Verify UTXO is removed
	_, err = db.GetUTXO(txHash, 0)
	if err == nil {
		t.Error("Expected error for removed UTXO, got nil")
	}

	// Verify address index is cleaned up
	utxos, err = db.GetUTXOsForAddress("N123456789")
	if err != nil {
		t.Fatalf("GetUTXOsForAddress failed: %v", err)
	}
	if len(utxos) != 0 {
		t.Errorf("Expected 0 UTXOs after removal, got %d", len(utxos))
	}
}

func TestMultipleUTXOsPerAddress(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test_multi_utxo.db"
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	address := "N987654321"

	// Add multiple UTXOs for the same address
	for i := 0; i < 3; i++ {
		txHash, _ := chainhash.NewHashFromStr("000000000000000000000000000000000000000000000000000000000000000" + string(rune('1'+i)))
		utxo := &UTXO{
			TxHash:   *txHash,
			OutIndex: uint32(i),
			Value:    int64((i + 1) * 10000000),
			Address:  address,
			PkScript: []byte{0x76, 0xa9},
			Height:   int32(100 + i),
		}
		if err := db.AddUTXO(utxo); err != nil {
			t.Fatalf("AddUTXO %d failed: %v", i, err)
		}
	}

	// Retrieve all UTXOs for the address
	utxos, err := db.GetUTXOsForAddress(address)
	if err != nil {
		t.Fatalf("GetUTXOsForAddress failed: %v", err)
	}

	if len(utxos) != 3 {
		t.Fatalf("Expected 3 UTXOs, got %d", len(utxos))
	}

	// Verify values
	totalValue := int64(0)
	for _, utxo := range utxos {
		totalValue += utxo.Value
		if utxo.Address != address {
			t.Errorf("Unexpected address: %s", utxo.Address)
		}
	}

	expectedTotal := int64(10000000 + 20000000 + 30000000)
	if totalValue != expectedTotal {
		t.Errorf("Total value mismatch: got %d, want %d", totalValue, expectedTotal)
	}
}

func TestGetNameUTXO(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test_name_utxo.db"
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Register a name
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000042")
	name := "d/example"
	record := &NameRecord{
		Name:      name,
		Value:     `{"ip":"1.2.3.4"}`,
		TxHash:    *txHash,
		Height:    200,
		ExpiresAt: 36200,
		Address:   "NNameOwner123",
	}

	if err := db.PutName(name, record); err != nil {
		t.Fatalf("PutName failed: %v", err)
	}

	// Add the UTXO for this name (output index 0)
	utxo := &UTXO{
		TxHash:   *txHash,
		OutIndex: 0, // Name is always at output 0
		Value:    5000000,
		Address:  "NNameOwner123",
		PkScript: []byte{0x76, 0xa9, 0x14},
		Height:   200,
	}

	if err := db.AddUTXO(utxo); err != nil {
		t.Fatalf("AddUTXO failed: %v", err)
	}

	// Test GetNameUTXO
	nameUTXO, err := db.GetNameUTXO(name)
	if err != nil {
		t.Fatalf("GetNameUTXO failed: %v", err)
	}

	if nameUTXO.Value != 5000000 {
		t.Errorf("Name UTXO value mismatch: got %d, want 5000000", nameUTXO.Value)
	}
	if nameUTXO.Address != "NNameOwner123" {
		t.Errorf("Name UTXO address mismatch: got %s, want NNameOwner123", nameUTXO.Address)
	}
	if nameUTXO.OutIndex != 0 {
		t.Errorf("Name UTXO output index mismatch: got %d, want 0", nameUTXO.OutIndex)
	}
}

func TestRemoveNonexistentUTXO(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test_remove_nonexistent.db"
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Try to remove a UTXO that doesn't exist
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000099")
	err = db.RemoveUTXO(txHash, 0)
	// Should not error - just be a no-op
	if err != nil {
		t.Errorf("RemoveUTXO should not error for nonexistent UTXO, got: %v", err)
	}
}

func TestUTXOWithLargeScript(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test_large_script.db"
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create UTXO with large script (typical name operation script)
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000055")
	largeScript := make([]byte, 500) // Name scripts can be several hundred bytes
	for i := range largeScript {
		largeScript[i] = byte(i % 256)
	}

	utxo := &UTXO{
		TxHash:   *txHash,
		OutIndex: 0,
		Value:    100000,
		Address:  "NLargeScript",
		PkScript: largeScript,
		Height:   150,
	}

	if err := db.AddUTXO(utxo); err != nil {
		t.Fatalf("AddUTXO with large script failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := db.GetUTXO(txHash, 0)
	if err != nil {
		t.Fatalf("GetUTXO failed: %v", err)
	}

	if len(retrieved.PkScript) != len(largeScript) {
		t.Errorf("Script length mismatch: got %d, want %d", len(retrieved.PkScript), len(largeScript))
	}

	for i := range retrieved.PkScript {
		if retrieved.PkScript[i] != largeScript[i] {
			t.Errorf("Script byte %d mismatch: got %x, want %x", i, retrieved.PkScript[i], largeScript[i])
			break
		}
	}
}
