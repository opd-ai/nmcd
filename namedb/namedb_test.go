package namedb

import (
	"os"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

func TestNameDatabase(t *testing.T) {
	// Create temporary database
	dbPath := "/tmp/test-namedb.db"
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Test PutName
	hash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	record := &NameRecord{
		Name:      "d/example",
		Value:     `{"ip":"1.2.3.4"}`,
		TxHash:    *hash,
		Height:    100,
		ExpiresAt: 36100,
		Address:   "N1234567890",
		UpdatedAt: time.Now(),
	}

	err = db.PutName("d/example", record)
	if err != nil {
		t.Fatalf("Failed to put name: %v", err)
	}

	// Test GetName
	retrieved, err := db.GetName("d/example")
	if err != nil {
		t.Fatalf("Failed to get name: %v", err)
	}

	if retrieved.Name != "d/example" {
		t.Errorf("Expected name 'd/example', got '%s'", retrieved.Name)
	}

	if retrieved.Value != record.Value {
		t.Errorf("Expected value '%s', got '%s'", record.Value, retrieved.Value)
	}

	if retrieved.Height != 100 {
		t.Errorf("Expected height 100, got %d", retrieved.Height)
	}

	// Test DeleteName
	err = db.DeleteName("d/example")
	if err != nil {
		t.Fatalf("Failed to delete name: %v", err)
	}

	// Verify deletion
	_, err = db.GetName("d/example")
	if err == nil {
		t.Error("Expected error for non-existent name")
	}
}

func TestNameExpiration(t *testing.T) {
	dbPath := "/tmp/test-expiration.db"
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	hash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000002")
	
	// Add expired name
	record1 := &NameRecord{
		Name:      "d/expired",
		Value:     "old",
		TxHash:    *hash,
		Height:    100,
		ExpiresAt: 200,
		UpdatedAt: time.Now(),
	}
	db.PutName("d/expired", record1)

	// Add valid name
	record2 := &NameRecord{
		Name:      "d/valid",
		Value:     "current",
		TxHash:    *hash,
		Height:    100,
		ExpiresAt: 500,
		UpdatedAt: time.Now(),
	}
	db.PutName("d/valid", record2)

	// Check for expired names at height 250
	expired, err := db.GetExpiredNames(250)
	if err != nil {
		t.Fatalf("Failed to get expired names: %v", err)
	}

	if len(expired) != 1 {
		t.Errorf("Expected 1 expired name, got %d", len(expired))
	}

	if len(expired) > 0 && expired[0] != "d/expired" {
		t.Errorf("Expected 'd/expired', got '%s'", expired[0])
	}
}
