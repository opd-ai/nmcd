package namedb

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

func TestNameDatabase(t *testing.T) {
	// Create temporary database
	dbPath := filepath.Join(os.TempDir(), "test-namedb.db")
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
	dbPath := filepath.Join(os.TempDir(), "test-expiration.db")
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

func TestGetHistory(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test-history.db")
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create multiple history entries for the same name
	hash1, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	hash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000002")
	hash3, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000003")

	record1 := &NameRecord{
		Name:      "d/example",
		Value:     `{"ip":"1.2.3.4"}`,
		TxHash:    *hash1,
		Height:    100,
		ExpiresAt: 36100,
		Address:   "N1111111111",
		UpdatedAt: time.Now(),
	}

	record2 := &NameRecord{
		Name:      "d/example",
		Value:     `{"ip":"5.6.7.8"}`,
		TxHash:    *hash2,
		Height:    200,
		ExpiresAt: 36200,
		Address:   "N2222222222",
		UpdatedAt: time.Now(),
	}

	record3 := &NameRecord{
		Name:      "d/example",
		Value:     `{"ip":"9.10.11.12"}`,
		TxHash:    *hash3,
		Height:    300,
		ExpiresAt: 36300,
		Address:   "N3333333333",
		UpdatedAt: time.Now(),
	}

	// Add history entries
	if err := db.AddHistory(*hash1, record1); err != nil {
		t.Fatalf("Failed to add history 1: %v", err)
	}
	if err := db.AddHistory(*hash2, record2); err != nil {
		t.Fatalf("Failed to add history 2: %v", err)
	}
	if err := db.AddHistory(*hash3, record3); err != nil {
		t.Fatalf("Failed to add history 3: %v", err)
	}

	// Retrieve history
	history, err := db.GetHistory("d/example")
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}

	if len(history) != 3 {
		t.Errorf("Expected 3 history entries, got %d", len(history))
	}

	// Verify entries are in order (oldest first)
	if len(history) >= 3 {
		if history[0].Height != 100 {
			t.Errorf("Expected first entry height 100, got %d", history[0].Height)
		}
		if history[1].Height != 200 {
			t.Errorf("Expected second entry height 200, got %d", history[1].Height)
		}
		if history[2].Height != 300 {
			t.Errorf("Expected third entry height 300, got %d", history[2].Height)
		}
	}

	// Verify values
	if len(history) >= 3 {
		if history[0].Value != `{"ip":"1.2.3.4"}` {
			t.Errorf("Expected first value '{\"ip\":\"1.2.3.4\"}', got '%s'", history[0].Value)
		}
		if history[2].Value != `{"ip":"9.10.11.12"}` {
			t.Errorf("Expected third value '{\"ip\":\"9.10.11.12\"}', got '%s'", history[2].Value)
		}
	}
}

func TestGetHistoryEmpty(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test-history-empty.db")
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Get history for a name that has no history
	history, err := db.GetHistory("d/nonexistent")
	if err != nil {
		t.Fatalf("GetHistory should not error on empty history: %v", err)
	}

	if len(history) != 0 {
		t.Errorf("Expected 0 history entries, got %d", len(history))
	}
}

func TestGetHistoryMultipleNames(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test-history-multi.db")
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create history for two different names
	hash1, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	hash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000002")

	record1 := &NameRecord{
		Name:      "d/name1",
		Value:     "value1",
		TxHash:    *hash1,
		Height:    100,
		ExpiresAt: 36100,
		Address:   "N1111111111",
		UpdatedAt: time.Now(),
	}

	record2 := &NameRecord{
		Name:      "d/name2",
		Value:     "value2",
		TxHash:    *hash2,
		Height:    200,
		ExpiresAt: 36200,
		Address:   "N2222222222",
		UpdatedAt: time.Now(),
	}

	if err := db.AddHistory(*hash1, record1); err != nil {
		t.Fatalf("Failed to add history for name1: %v", err)
	}
	if err := db.AddHistory(*hash2, record2); err != nil {
		t.Fatalf("Failed to add history for name2: %v", err)
	}

	// Get history for name1 - should only contain its own entry
	history1, err := db.GetHistory("d/name1")
	if err != nil {
		t.Fatalf("Failed to get history for name1: %v", err)
	}
	if len(history1) != 1 {
		t.Errorf("Expected 1 history entry for name1, got %d", len(history1))
	}
	if len(history1) > 0 && history1[0].Value != "value1" {
		t.Errorf("Expected value 'value1', got '%s'", history1[0].Value)
	}

	// Get history for name2 - should only contain its own entry
	history2, err := db.GetHistory("d/name2")
	if err != nil {
		t.Fatalf("Failed to get history for name2: %v", err)
	}
	if len(history2) != 1 {
		t.Errorf("Expected 1 history entry for name2, got %d", len(history2))
	}
	if len(history2) > 0 && history2[0].Value != "value2" {
		t.Errorf("Expected value 'value2', got '%s'", history2[0].Value)
	}
}
