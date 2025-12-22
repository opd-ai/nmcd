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

// TestNameExpirationEdgeCase verifies that a name is valid at its ExpiresAt height
// and only considered expired one block after. This tests the boundary condition.
func TestNameExpirationEdgeCase(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test-expiration-edge.db")
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	hash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000003")

	// Add name that expires at height 100
	record := &NameRecord{
		Name:      "d/edge",
		Value:     "test",
		TxHash:    *hash,
		Height:    50,
		ExpiresAt: 100,
		UpdatedAt: time.Now(),
	}
	db.PutName("d/edge", record)

	// At height 99, name should NOT be expired (ExpiresAt=100 > 99)
	expired, err := db.GetExpiredNames(99)
	if err != nil {
		t.Fatalf("Failed to get expired names at height 99: %v", err)
	}
	if len(expired) != 0 {
		t.Errorf("Expected 0 expired names at height 99 (before ExpiresAt), got %d: %v", len(expired), expired)
	}

	// At height 100 (ExpiresAt), name should still be valid (not expired yet)
	expired, err = db.GetExpiredNames(100)
	if err != nil {
		t.Fatalf("Failed to get expired names at height 100: %v", err)
	}
	if len(expired) != 0 {
		t.Errorf("Expected 0 expired names at height 100 (at ExpiresAt), got %d: %v", len(expired), expired)
	}

	// At height 101, name should be expired (ExpiresAt=100 < 101)
	expired, err = db.GetExpiredNames(101)
	if err != nil {
		t.Fatalf("Failed to get expired names at height 101: %v", err)
	}
	if len(expired) != 1 {
		t.Errorf("Expected 1 expired name at height 101 (after ExpiresAt), got %d", len(expired))
	}
	if len(expired) > 0 && expired[0] != "d/edge" {
		t.Errorf("Expected 'd/edge', got '%s'", expired[0])
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

func TestNameNew(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test-namenew.db")
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Test PutNameNew and GetNameNew
	commitHash := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}
	height := int32(100)

	err = db.PutNameNew(commitHash, height)
	if err != nil {
		t.Fatalf("Failed to put name_new: %v", err)
	}

	record, err := db.GetNameNew(commitHash)
	if err != nil {
		t.Fatalf("Failed to get name_new: %v", err)
	}

	if record.Height != height {
		t.Errorf("Expected height %d, got %d", height, record.Height)
	}

	// Test that duplicate commitment is rejected
	err = db.PutNameNew(commitHash, height+10)
	if err == nil {
		t.Error("Expected error for duplicate name_new commitment, got nil")
	}

	// Verify original height is preserved
	record, err = db.GetNameNew(commitHash)
	if err != nil {
		t.Fatalf("Failed to get name_new after duplicate attempt: %v", err)
	}
	if record.Height != height {
		t.Errorf("Original height should be preserved: expected %d, got %d", height, record.Height)
	}
}

func TestNameNewNotFound(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test-namenew-notfound.db")
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Try to get a non-existent commitment
	commitHash := []byte{0xff, 0xff, 0xff, 0xff}
	_, err = db.GetNameNew(commitHash)
	if err == nil {
		t.Error("Expected error for non-existent name_new, got nil")
	}
}

func TestDeleteNameNew(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test-namenew-delete.db")
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	commitHash := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}
	height := int32(100)

	// Add a NAME_NEW
	err = db.PutNameNew(commitHash, height)
	if err != nil {
		t.Fatalf("Failed to put name_new: %v", err)
	}

	// Verify it exists
	_, err = db.GetNameNew(commitHash)
	if err != nil {
		t.Fatalf("Expected name_new to exist: %v", err)
	}

	// Delete it
	err = db.DeleteNameNew(commitHash)
	if err != nil {
		t.Fatalf("Failed to delete name_new: %v", err)
	}

	// Verify it's gone
	_, err = db.GetNameNew(commitHash)
	if err == nil {
		t.Error("Expected error after deleting name_new, got nil")
	}
}

func TestMultipleNameNews(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test-namenew-multi.db")
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Add two different NAME_NEW commitments at different heights
	hash1 := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}
	hash2 := []byte{0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6, 0xa7, 0xa8,
		0xa9, 0xaa, 0xab, 0xac, 0xad, 0xae, 0xaf, 0xb0,
		0xb1, 0xb2, 0xb3, 0xb4}

	err = db.PutNameNew(hash1, 100)
	if err != nil {
		t.Fatalf("Failed to put name_new 1: %v", err)
	}
	err = db.PutNameNew(hash2, 150)
	if err != nil {
		t.Fatalf("Failed to put name_new 2: %v", err)
	}

	// Retrieve and verify each
	record1, err := db.GetNameNew(hash1)
	if err != nil {
		t.Fatalf("Failed to get name_new 1: %v", err)
	}
	if record1.Height != 100 {
		t.Errorf("Expected height 100, got %d", record1.Height)
	}

	record2, err := db.GetNameNew(hash2)
	if err != nil {
		t.Fatalf("Failed to get name_new 2: %v", err)
	}
	if record2.Height != 150 {
		t.Errorf("Expected height 150, got %d", record2.Height)
	}
}

func TestDecodeNameRecordCorruptData(t *testing.T) {
	testCases := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: "corrupt record: empty data",
		},
		{
			name:    "truncated at value length",
			data:    []byte{1, 0, 0}, // version byte + only 3 bytes for value length (needs 4)
			wantErr: "corrupt record: truncated at value length",
		},
		{
			name:    "truncated at value data",
			data:    []byte{1, 10, 0, 0, 0, 'a', 'b'}, // version + value length 10, but only 2 chars
			wantErr: "corrupt record: truncated at value data",
		},
		{
			name:    "truncated at txhash",
			data:    append([]byte{1, 5, 0, 0, 0}, append([]byte("hello"), make([]byte, 20)...)...), // version + value + partial txhash
			wantErr: "corrupt record: truncated at txhash",
		},
		{
			name: "truncated at height",
			data: append([]byte{1, 5, 0, 0, 0}, append([]byte("hello"), append(make([]byte, 32), []byte{0, 0}...)...)...), // version + value + full txhash + partial height
			wantErr: "corrupt record: truncated at height",
		},
		{
			name: "truncated at expires_at",
			data: append([]byte{1, 5, 0, 0, 0}, append([]byte("hello"), append(make([]byte, 32), []byte{100, 0, 0, 0, 0, 0}...)...)...), // version + value + full txhash + height + partial expires
			wantErr: "corrupt record: truncated at expires_at",
		},
		{
			name: "truncated at address length",
			// version(1) + value_len(4) + value(5) + txhash(32) + height(4) + expires(4) + partial addr_len(2)
			data: append([]byte{1, 5, 0, 0, 0}, append([]byte("hello"), append(make([]byte, 32), []byte{100, 0, 0, 0, 200, 0, 0, 0, 0, 0}...)...)...),
			wantErr: "corrupt record: truncated at address length",
		},
		{
			name: "truncated at address data",
			// version(1) + value_len(4) + value(5) + txhash(32) + height(4) + expires(4) + addr_len(4, value=10) + partial addr(2)
			data: append([]byte{1, 5, 0, 0, 0}, append([]byte("hello"), append(make([]byte, 32), []byte{100, 0, 0, 0, 200, 0, 0, 0, 10, 0, 0, 0, 'a', 'b'}...)...)...),
			wantErr: "corrupt record: truncated at address data",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			record, err := decodeNameRecord(tc.data)
			if err == nil {
				t.Errorf("expected error for %s, got nil (record: %+v)", tc.name, record)
				return
			}
			if err.Error() != tc.wantErr {
				t.Errorf("expected error %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestDecodeNameRecordValid(t *testing.T) {
	// Create a valid record and encode it
	record := &NameRecord{
		Name:      "test",
		Value:     "testvalue",
		Height:    12345,
		ExpiresAt: 67890,
		Address:   "N1234567890",
	}

	// Encode the record
	encoded := encodeNameRecord(record)

	// Decode it back
	decoded, err := decodeNameRecord(encoded)
	if err != nil {
		t.Fatalf("unexpected error decoding valid record: %v", err)
	}

	// Verify the decoded values match
	if decoded.Value != record.Value {
		t.Errorf("expected value %q, got %q", record.Value, decoded.Value)
	}
	if decoded.Height != record.Height {
		t.Errorf("expected height %d, got %d", record.Height, decoded.Height)
	}
	if decoded.ExpiresAt != record.ExpiresAt {
		t.Errorf("expected expires_at %d, got %d", record.ExpiresAt, decoded.ExpiresAt)
	}
	if decoded.Address != record.Address {
		t.Errorf("expected address %q, got %q", record.Address, decoded.Address)
	}
}

func TestRestoreNameNew(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test-restorenamenew.db")
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	commitHash := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}
	height := int32(100)

	// First, add a NAME_NEW
	err = db.PutNameNew(commitHash, height)
	if err != nil {
		t.Fatalf("Failed to put name_new: %v", err)
	}

	// Delete it (simulating NAME_FIRSTUPDATE consumption)
	err = db.DeleteNameNew(commitHash)
	if err != nil {
		t.Fatalf("Failed to delete name_new: %v", err)
	}

	// Verify it's gone
	_, err = db.GetNameNew(commitHash)
	if err == nil {
		t.Error("Expected error for deleted name_new, got nil")
	}

	// Restore it (simulating rollback)
	err = db.RestoreNameNew(commitHash, height)
	if err != nil {
		t.Fatalf("Failed to restore name_new: %v", err)
	}

	// Verify it's back
	record, err := db.GetNameNew(commitHash)
	if err != nil {
		t.Fatalf("Failed to get restored name_new: %v", err)
	}
	if record.Height != height {
		t.Errorf("Expected height %d, got %d", height, record.Height)
	}
}

func TestRestoreNameNewOverwrite(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test-restorenamenew-overwrite.db")
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	commitHash := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}

	// Add a NAME_NEW at height 100
	err = db.PutNameNew(commitHash, 100)
	if err != nil {
		t.Fatalf("Failed to put name_new: %v", err)
	}

	// RestoreNameNew should overwrite (unlike PutNameNew which rejects duplicates)
	err = db.RestoreNameNew(commitHash, 200)
	if err != nil {
		t.Fatalf("Failed to restore name_new: %v", err)
	}

	// Verify the height was updated
	record, err := db.GetNameNew(commitHash)
	if err != nil {
		t.Fatalf("Failed to get name_new: %v", err)
	}
	if record.Height != 200 {
		t.Errorf("Expected height 200, got %d", record.Height)
	}
}

func TestRemoveLastHistoryEntry(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test-removelasthistory.db")
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

	// Remove the last entry and verify we get record2 back
	prevRecord, err := db.RemoveLastHistoryEntry("d/example")
	if err != nil {
		t.Fatalf("Failed to remove last history entry: %v", err)
	}

	if prevRecord == nil {
		t.Fatal("Expected previous record, got nil")
	}
	if prevRecord.Height != 200 {
		t.Errorf("Expected previous record height 200, got %d", prevRecord.Height)
	}
	if prevRecord.Value != `{"ip":"5.6.7.8"}` {
		t.Errorf("Expected previous value '{\"ip\":\"5.6.7.8\"}', got '%s'", prevRecord.Value)
	}

	// Verify history now has only 2 entries
	history, err := db.GetHistory("d/example")
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	if len(history) != 2 {
		t.Errorf("Expected 2 history entries after removal, got %d", len(history))
	}
}

func TestRemoveLastHistoryEntrySingleEntry(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test-removelasthistory-single.db")
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	hash1, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")

	record1 := &NameRecord{
		Name:      "d/example",
		Value:     `{"ip":"1.2.3.4"}`,
		TxHash:    *hash1,
		Height:    100,
		ExpiresAt: 36100,
		Address:   "N1111111111",
		UpdatedAt: time.Now(),
	}

	// Add a single history entry
	if err := db.AddHistory(*hash1, record1); err != nil {
		t.Fatalf("Failed to add history: %v", err)
	}

	// Remove the only entry - should return nil for previous record
	prevRecord, err := db.RemoveLastHistoryEntry("d/example")
	if err != nil {
		t.Fatalf("Failed to remove last history entry: %v", err)
	}

	// No previous record when removing the only entry
	if prevRecord != nil {
		t.Errorf("Expected nil previous record when removing only entry, got %+v", prevRecord)
	}

	// Verify history is now empty
	history, err := db.GetHistory("d/example")
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("Expected 0 history entries after removal, got %d", len(history))
	}
}

func TestRemoveLastHistoryEntryNoHistory(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test-removelasthistory-none.db")
	defer os.Remove(dbPath)

	db, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Try to remove history for a name that has no history
	prevRecord, err := db.RemoveLastHistoryEntry("d/nonexistent")
	if err != nil {
		t.Fatalf("RemoveLastHistoryEntry should not error on empty history: %v", err)
	}

	if prevRecord != nil {
		t.Errorf("Expected nil for non-existent name history, got %+v", prevRecord)
	}
}
