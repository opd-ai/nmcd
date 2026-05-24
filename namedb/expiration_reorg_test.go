package namedb

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// TestStoreAndRestoreExpiredName tests storing and restoring a single expired name
func TestStoreAndRestoreExpiredName(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	ndb, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	// Create a name record
	txHash, _ := chainhash.NewHashFromStr("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
	record := &NameRecord{
		Name:          "d/test",
		Value:         `{"ip":"1.2.3.4"}`,
		TxHash:        *txHash,
		OutIndex:      0,
		Height:        100,
		ExpiresAt:     200,
		Address:       "NTestAddress123",
		UpdatedAt:     time.Now(),
		NameNewHeight: 95,
	}

	// Store the name
	err = ndb.PutName(record.Name, record)
	if err != nil {
		t.Fatalf("Failed to store name: %v", err)
	}

	// Verify name exists
	retrieved, err := ndb.GetName("d/test")
	if err != nil {
		t.Fatalf("Failed to retrieve name: %v", err)
	}
	if retrieved.Value != record.Value {
		t.Errorf("Value mismatch: got %s, want %s", retrieved.Value, record.Value)
	}

	// Store as expired at height 201
	expiredAtHeight := int32(201)
	err = ndb.StoreExpiredName(record, expiredAtHeight)
	if err != nil {
		t.Fatalf("Failed to store expired name: %v", err)
	}

	// Delete the name (simulating expiration)
	err = ndb.DeleteName("d/test")
	if err != nil {
		t.Fatalf("Failed to delete name: %v", err)
	}

	// Verify name is deleted
	_, err = ndb.GetName("d/test")
	if err != ErrNameNotFound {
		t.Errorf("Expected ErrNameNotFound, got %v", err)
	}

	// Restore expired names for that height (simulating reorg)
	err = ndb.RestoreExpiredNamesForBlock(expiredAtHeight)
	if err != nil {
		t.Fatalf("Failed to restore expired names: %v", err)
	}

	// Verify name is restored
	restored, err := ndb.GetName("d/test")
	if err != nil {
		t.Fatalf("Failed to retrieve restored name: %v", err)
	}

	// Verify data integrity
	if restored.Name != record.Name {
		t.Errorf("Name mismatch: got %s, want %s", restored.Name, record.Name)
	}
	if restored.Value != record.Value {
		t.Errorf("Value mismatch: got %s, want %s", restored.Value, record.Value)
	}
	if restored.Height != record.Height {
		t.Errorf("Height mismatch: got %d, want %d", restored.Height, record.Height)
	}
	if restored.ExpiresAt != record.ExpiresAt {
		t.Errorf("ExpiresAt mismatch: got %d, want %d", restored.ExpiresAt, record.ExpiresAt)
	}
	if restored.Address != record.Address {
		t.Errorf("Address mismatch: got %s, want %s", restored.Address, record.Address)
	}
}

// TestRestoreMultipleExpiredNames tests restoring multiple names expired at the same height
func TestRestoreMultipleExpiredNames(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	ndb, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	expiredAtHeight := int32(300)
	numNames := 5

	// Create and store multiple names
	var records []*NameRecord
	for i := 0; i < numNames; i++ {
		txHash, _ := chainhash.NewHashFromStr("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
		txHash[0] = byte(i) // Make each hash unique

		record := &NameRecord{
			Name:          "d/test" + string(rune('0'+i)),
			Value:         `{"ip":"1.2.3.` + string(rune('0'+i)) + `"}`,
			TxHash:        *txHash,
			OutIndex:      0,
			Height:        200,
			ExpiresAt:     299,
			Address:       "NTestAddress" + string(rune('0'+i)),
			UpdatedAt:     time.Now(),
			NameNewHeight: 195,
		}

		// Store the name
		err = ndb.PutName(record.Name, record)
		if err != nil {
			t.Fatalf("Failed to store name %d: %v", i, err)
		}

		// Store as expired
		err = ndb.StoreExpiredName(record, expiredAtHeight)
		if err != nil {
			t.Fatalf("Failed to store expired name %d: %v", i, err)
		}

		records = append(records, record)
	}

	// Delete all names (simulating expiration)
	for _, record := range records {
		err = ndb.DeleteName(record.Name)
		if err != nil {
			t.Fatalf("Failed to delete name %s: %v", record.Name, err)
		}
	}

	// Verify all names are deleted
	for _, record := range records {
		_, err = ndb.GetName(record.Name)
		if err != ErrNameNotFound {
			t.Errorf("Expected ErrNameNotFound for %s, got %v", record.Name, err)
		}
	}

	// Restore all expired names for that height
	err = ndb.RestoreExpiredNamesForBlock(expiredAtHeight)
	if err != nil {
		t.Fatalf("Failed to restore expired names: %v", err)
	}

	// Verify all names are restored
	for i, record := range records {
		restored, err := ndb.GetName(record.Name)
		if err != nil {
			t.Errorf("Failed to retrieve restored name %d (%s): %v", i, record.Name, err)
			continue
		}

		if restored.Value != record.Value {
			t.Errorf("Name %d value mismatch: got %s, want %s", i, restored.Value, record.Value)
		}
		if restored.Height != record.Height {
			t.Errorf("Name %d height mismatch: got %d, want %d", i, restored.Height, record.Height)
		}
	}
}

// TestCleanupOldExpiredNames tests cleanup of old expired name backups
func TestCleanupOldExpiredNames(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	ndb, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	// Create names expired at different heights
	heights := []int32{100, 200, 300, 400, 500}
	records := make(map[int32]*NameRecord)

	for _, height := range heights {
		txHash, _ := chainhash.NewHashFromStr("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
		txHash[0] = byte(height) // Make each hash unique

		record := &NameRecord{
			Name:          "d/test" + string(rune('0'+(height/100))),
			Value:         `{"height":` + string(rune('0'+(height/100))) + `}`,
			TxHash:        *txHash,
			OutIndex:      0,
			Height:        height - 100,
			ExpiresAt:     height - 1,
			Address:       "NTestAddr",
			UpdatedAt:     time.Now(),
			NameNewHeight: height - 105,
		}

		// Store the name
		err = ndb.PutName(record.Name, record)
		if err != nil {
			t.Fatalf("Failed to store name at height %d: %v", height, err)
		}

		// Store as expired
		err = ndb.StoreExpiredName(record, height)
		if err != nil {
			t.Fatalf("Failed to store expired name at height %d: %v", height, err)
		}

		// Delete (simulate expiration)
		err = ndb.DeleteName(record.Name)
		if err != nil {
			t.Fatalf("Failed to delete name at height %d: %v", height, err)
		}

		records[height] = record
	}

	// Cleanup old expired names (keep from height 300 onwards)
	keepFromHeight := int32(300)
	err = ndb.CleanupOldExpiredNames(keepFromHeight)
	if err != nil {
		t.Fatalf("Failed to cleanup old expired names: %v", err)
	}

	// Try to restore names from different heights
	for _, height := range heights {
		err = ndb.RestoreExpiredNamesForBlock(height)
		if err != nil {
			t.Fatalf("Failed to restore expired names for height %d: %v", height, err)
		}

		record := records[height]
		_, err := ndb.GetName(record.Name)

		if height < keepFromHeight {
			// Should NOT be restored (cleaned up)
			if err == nil {
				t.Errorf("Name at height %d should have been cleaned up but was restored", height)
			}
		} else {
			// Should be restored
			if err != nil {
				t.Errorf("Name at height %d should have been restored but wasn't: %v", height, err)
			}
		}
	}
}

// TestExpiredNameReorgScenario tests a realistic reorg scenario with expiration
func TestExpiredNameReorgScenario(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	ndb, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	// Scenario: Name registered at height 100, expires at 36100
	txHash, _ := chainhash.NewHashFromStr("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
	record := &NameRecord{
		Name:          "d/important",
		Value:         `{"domain":"example.com"}`,
		TxHash:        *txHash,
		OutIndex:      0,
		Height:        100,
		ExpiresAt:     36100,
		Address:       "NOwnerAddress",
		UpdatedAt:     time.Now(),
		NameNewHeight: 95,
	}

	// Store the name
	err = ndb.PutName(record.Name, record)
	if err != nil {
		t.Fatalf("Failed to store name: %v", err)
	}

	// Chain advances to height 36101, name expires
	expirationHeight := int32(36101)

	// Store before expiration
	err = ndb.StoreExpiredName(record, expirationHeight)
	if err != nil {
		t.Fatalf("Failed to store expired name: %v", err)
	}

	// Delete name and history (simulating expiration processing)
	err = ndb.DeleteName("d/important")
	if err != nil {
		t.Fatalf("Failed to delete name: %v", err)
	}

	// Verify name is gone
	_, err = ndb.GetName("d/important")
	if err != ErrNameNotFound {
		t.Errorf("Expected name to be deleted, got error: %v", err)
	}

	// Reorg happens: block 36101 is disconnected
	// Restore expired names from that block
	err = ndb.RestoreExpiredNamesForBlock(expirationHeight)
	if err != nil {
		t.Fatalf("Failed to restore expired names during reorg: %v", err)
	}

	// Verify name is restored
	restored, err := ndb.GetName("d/important")
	if err != nil {
		t.Fatalf("Failed to retrieve restored name after reorg: %v", err)
	}

	// Verify all data is intact
	if restored.Name != "d/important" {
		t.Errorf("Restored name mismatch: got %s, want d/important", restored.Name)
	}
	if restored.Value != record.Value {
		t.Errorf("Restored value mismatch: got %s, want %s", restored.Value, record.Value)
	}
	if restored.Height != record.Height {
		t.Errorf("Restored height mismatch: got %d, want %d", restored.Height, record.Height)
	}
	if restored.ExpiresAt != record.ExpiresAt {
		t.Errorf("Restored ExpiresAt mismatch: got %d, want %d", restored.ExpiresAt, record.ExpiresAt)
	}

	// The name should now be valid again at height 36100
	// (it can be re-expired in a future block)
}
