package namedb

import (
	"fmt"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// TestExpirationIndex tests that the expiration index is properly maintained
func TestExpirationIndex(t *testing.T) {
	db, err := NewNameDatabase(t.TempDir() + "/expiration_index.db")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Add names with different expiration heights
	names := []struct {
		name      string
		expiresAt int32
	}{
		{"d/expires100", 100},
		{"d/expires200", 200},
		{"d/expires150", 150},
		{"d/expires50", 50},
		{"d/expires300", 300},
	}

	for i, n := range names {
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", i))
		record := &NameRecord{
			Name:      n.name,
			Value:     fmt.Sprintf(`{"id":%d}`, i),
			TxHash:    *txHash,
			Height:    int32(i),
			ExpiresAt: n.expiresAt,
			UpdatedAt: time.Now(),
		}
		if err := db.PutName(n.name, record); err != nil {
			t.Fatalf("Failed to add name %s: %v", n.name, err)
		}
	}

	// Test expiration at different heights
	tests := []struct {
		height   int32
		expected []string
	}{
		{
			height:   50,
			expected: []string{}, // No names expired yet (expires AT 50, not before)
		},
		{
			height:   51,
			expected: []string{"d/expires50"},
		},
		{
			height:   101,
			expected: []string{"d/expires50", "d/expires100"},
		},
		{
			height:   151,
			expected: []string{"d/expires50", "d/expires100", "d/expires150"},
		},
		{
			height:   201,
			expected: []string{"d/expires50", "d/expires100", "d/expires150", "d/expires200"},
		},
		{
			height:   301,
			expected: []string{"d/expires50", "d/expires100", "d/expires150", "d/expires200", "d/expires300"},
		},
	}

	for _, tt := range tests {
		expired, err := db.GetExpiredNames(tt.height)
		if err != nil {
			t.Fatalf("GetExpiredNames(%d) failed: %v", tt.height, err)
		}

		if len(expired) != len(tt.expected) {
			t.Errorf("GetExpiredNames(%d): expected %d names, got %d", tt.height, len(tt.expected), len(expired))
			t.Logf("  Expected: %v", tt.expected)
			t.Logf("  Got: %v", expired)
			continue
		}

		// Convert to map for easier comparison
		expiredMap := make(map[string]bool)
		for _, name := range expired {
			expiredMap[name] = true
		}

		for _, expectedName := range tt.expected {
			if !expiredMap[expectedName] {
				t.Errorf("GetExpiredNames(%d): expected %s to be expired", tt.height, expectedName)
			}
		}
	}
}

// TestExpirationIndexUpdate tests that updating a name's expiration updates the index
func TestExpirationIndexUpdate(t *testing.T) {
	db, err := NewNameDatabase(t.TempDir() + "/expiration_update.db")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Add a name expiring at height 100
	txHash1, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	db.PutName("d/test", &NameRecord{
		Name:      "d/test",
		Value:     "version1",
		TxHash:    *txHash1,
		Height:    10,
		ExpiresAt: 100,
		UpdatedAt: time.Now(),
	})

	// Verify it's not expired at height 50
	expired, _ := db.GetExpiredNames(50)
	if len(expired) != 0 {
		t.Errorf("Expected no expired names at height 50, got %v", expired)
	}

	// Verify it's expired at height 101
	expired, _ = db.GetExpiredNames(101)
	if len(expired) != 1 || expired[0] != "d/test" {
		t.Errorf("Expected d/test to be expired at height 101, got %v", expired)
	}

	// Update the name to expire at height 200
	txHash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000002")
	db.PutName("d/test", &NameRecord{
		Name:      "d/test",
		Value:     "version2",
		TxHash:    *txHash2,
		Height:    20,
		ExpiresAt: 200,
		UpdatedAt: time.Now(),
	})

	// Verify it's NOT expired at height 101 anymore
	expired, _ = db.GetExpiredNames(101)
	if len(expired) != 0 {
		t.Errorf("Expected no expired names at height 101 after update, got %v", expired)
	}

	// Verify it's now expired at height 201
	expired, _ = db.GetExpiredNames(201)
	if len(expired) != 1 || expired[0] != "d/test" {
		t.Errorf("Expected d/test to be expired at height 201, got %v", expired)
	}
}

// TestExpirationIndexDelete tests that deleting a name removes it from the expiration index
func TestExpirationIndexDelete(t *testing.T) {
	db, err := NewNameDatabase(t.TempDir() + "/expiration_delete.db")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Add names
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("d/test%d", i)
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", i))
		db.PutName(name, &NameRecord{
			Name:      name,
			Value:     fmt.Sprintf(`{"id":%d}`, i),
			TxHash:    *txHash,
			Height:    int32(i),
			ExpiresAt: 100,
			UpdatedAt: time.Now(),
		})
	}

	// Verify all 5 names are expired at height 101
	expired, _ := db.GetExpiredNames(101)
	if len(expired) != 5 {
		t.Errorf("Expected 5 expired names, got %d", len(expired))
	}

	// Delete test2
	db.DeleteName("d/test2")

	// Verify only 4 names are expired now
	expired, _ = db.GetExpiredNames(101)
	if len(expired) != 4 {
		t.Errorf("Expected 4 expired names after delete, got %d", len(expired))
	}

	// Verify test2 is not in the list
	for _, name := range expired {
		if name == "d/test2" {
			t.Error("d/test2 should not be in expired list after deletion")
		}
	}
}

// TestExpirationIndexBatch tests that batch operations maintain the expiration index
func TestExpirationIndexBatch(t *testing.T) {
	db, err := NewNameDatabase(t.TempDir() + "/expiration_batch.db")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	batch := db.NewBatchWriter(0)

	// Add names via batch
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("d/batch%d", i)
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", i))
		record := &NameRecord{
			Name:      name,
			Value:     fmt.Sprintf(`{"id":%d}`, i),
			TxHash:    *txHash,
			Height:    int32(i),
			ExpiresAt: int32(100 + i*10), // Stagger expirations
			UpdatedAt: time.Now(),
		}
		batch.PutName(name, record)
	}

	if err := batch.Commit(); err != nil {
		t.Fatalf("Batch commit failed: %v", err)
	}

	// Verify expiration at different heights
	expired, _ := db.GetExpiredNames(111)
	if len(expired) != 2 { // batch0 (expires 100), batch1 (expires 110)
		t.Errorf("Expected 2 expired names at height 111, got %d: %v", len(expired), expired)
	}

	expired, _ = db.GetExpiredNames(151)
	if len(expired) != 6 { // batch0-5
		t.Errorf("Expected 6 expired names at height 151, got %d: %v", len(expired), expired)
	}

	// Update via batch
	batch2 := db.NewBatchWriter(0)
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000100")
	batch2.PutName("d/batch0", &NameRecord{
		Name:      "d/batch0",
		Value:     "updated",
		TxHash:    *txHash,
		Height:    200,
		ExpiresAt: 300, // Extend expiration
		UpdatedAt: time.Now(),
	})
	batch2.Commit()

	// batch0 should no longer be expired at height 151
	expired, _ = db.GetExpiredNames(151)
	if len(expired) != 5 { // batch1-5 only
		t.Errorf("Expected 5 expired names at height 151 after update, got %d: %v", len(expired), expired)
	}

	// Delete via batch
	batch3 := db.NewBatchWriter(0)
	batch3.DeleteName("d/batch1")
	batch3.Commit()

	// batch1 should be gone
	expired, _ = db.GetExpiredNames(151)
	if len(expired) != 4 { // batch2-5 only
		t.Errorf("Expected 4 expired names at height 151 after delete, got %d: %v", len(expired), expired)
	}
}
