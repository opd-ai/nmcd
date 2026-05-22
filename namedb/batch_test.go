package namedb

import (
	"fmt"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// TestBatchWriter_BasicOperations tests basic batch writer functionality
func TestBatchWriter_BasicOperations(t *testing.T) {
	db, err := NewNameDatabase(t.TempDir() + "/batch_basic.db")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	batch := db.NewBatchWriter(0) // No auto-commit

	// Add some names
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("d/test%d", i)
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", i))
		record := &NameRecord{
			Name:      name,
			Value:     fmt.Sprintf(`{"id":%d}`, i),
			TxHash:    *txHash,
			Height:    int32(i),
			ExpiresAt: int32(i + 36000),
			UpdatedAt: time.Now(),
		}
		if err := batch.PutName(name, record); err != nil {
			t.Fatalf("Failed to add name to batch: %v", err)
		}
	}

	// Verify batch size
	if batch.Size() != 10 {
		t.Errorf("Expected batch size 10, got %d", batch.Size())
	}

	// Names should not exist yet (not committed)
	if _, err := db.GetName("d/test0"); err == nil {
		t.Error("Expected name not to exist before commit")
	}

	// Commit batch
	if err := batch.Commit(); err != nil {
		t.Fatalf("Failed to commit batch: %v", err)
	}

	// Verify batch is cleared
	if batch.Size() != 0 {
		t.Errorf("Expected batch size 0 after commit, got %d", batch.Size())
	}

	// Names should now exist
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("d/test%d", i)
		record, err := db.GetName(name)
		if err != nil {
			t.Errorf("Failed to get name %s after commit: %v", name, err)
		}
		if record.Value != fmt.Sprintf(`{"id":%d}`, i) {
			t.Errorf("Wrong value for %s: got %s", name, record.Value)
		}
	}
}

// TestBatchWriter_AutoCommit tests automatic batch commit when size limit reached
func TestBatchWriter_AutoCommit(t *testing.T) {
	db, err := NewNameDatabase(t.TempDir() + "/batch_auto.db")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	batch := db.NewBatchWriter(5) // Auto-commit at 5 operations

	// Add 5 names (should trigger auto-commit)
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("d/test%d", i)
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", i))
		record := &NameRecord{
			Name:      name,
			Value:     fmt.Sprintf(`{"id":%d}`, i),
			TxHash:    *txHash,
			Height:    int32(i),
			ExpiresAt: int32(i + 36000),
			UpdatedAt: time.Now(),
		}
		if err := batch.PutName(name, record); err != nil {
			t.Fatalf("Failed to add name to batch: %v", err)
		}
	}

	// Batch should be empty after auto-commit
	if batch.Size() != 0 {
		t.Errorf("Expected batch size 0 after auto-commit, got %d", batch.Size())
	}

	// First 5 names should exist
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("d/test%d", i)
		if _, err := db.GetName(name); err != nil {
			t.Errorf("Failed to get auto-committed name %s: %v", name, err)
		}
	}
}

// TestBatchWriter_MixedOperations tests putting, deleting, and history in one batch
func TestBatchWriter_MixedOperations(t *testing.T) {
	db, err := NewNameDatabase(t.TempDir() + "/batch_mixed.db")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Pre-populate with some names
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("d/existing%d", i)
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", i))
		record := &NameRecord{
			Name:      name,
			Value:     "old",
			TxHash:    *txHash,
			Height:    int32(i),
			ExpiresAt: int32(i + 36000),
			UpdatedAt: time.Now(),
		}
		db.PutName(name, record)
	}

	batch := db.NewBatchWriter(0)

	// Update existing name
	txHash1, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000100")
	batch.PutName("d/existing0", &NameRecord{
		Name:      "d/existing0",
		Value:     "updated",
		TxHash:    *txHash1,
		Height:    100,
		ExpiresAt: 36100,
		UpdatedAt: time.Now(),
	})

	// Add new name
	txHash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000200")
	batch.PutName("d/new", &NameRecord{
		Name:      "d/new",
		Value:     "new value",
		TxHash:    *txHash2,
		Height:    101,
		ExpiresAt: 36101,
		UpdatedAt: time.Now(),
	})

	// Delete existing name
	batch.DeleteName("d/existing1")

	// Add history
	txHash3, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000300")
	batch.AddHistory(*txHash3, &NameRecord{
		Name:      "d/existing0",
		Value:     "history entry",
		TxHash:    *txHash3,
		Height:    50,
		ExpiresAt: 36050,
		UpdatedAt: time.Now(),
	})

	if err := batch.Commit(); err != nil {
		t.Fatalf("Failed to commit mixed batch: %v", err)
	}

	// Verify update
	record, err := db.GetName("d/existing0")
	if err != nil || record.Value != "updated" {
		t.Errorf("Update failed: err=%v, value=%v", err, record)
	}

	// Verify new name
	record, err = db.GetName("d/new")
	if err != nil || record.Value != "new value" {
		t.Errorf("New name failed: err=%v, value=%v", err, record)
	}

	// Verify delete
	if _, err := db.GetName("d/existing1"); err == nil {
		t.Error("Expected deleted name to not exist")
	}

	// Verify history
	history, err := db.GetHistory("d/existing0")
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	if len(history) != 1 {
		t.Errorf("Expected 1 history entry, got %d", len(history))
	}
	if len(history) > 0 && history[0].Value != "history entry" {
		t.Errorf("Wrong history value: %s", history[0].Value)
	}
}

// TestBatchWriter_UTXOOperations tests UTXO batch operations
func TestBatchWriter_UTXOOperations(t *testing.T) {
	db, err := NewNameDatabase(t.TempDir() + "/batch_utxo.db")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	batch := db.NewBatchWriter(0)

	// Add UTXOs
	for i := 0; i < 10; i++ {
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", i))
		utxo := &UTXO{
			TxHash:   *txHash,
			OutIndex: 0,
			Value:    int64(i * 100000000),
			Address:  fmt.Sprintf("NAddr%d", i),
			Height:   int32(i),
		}
		if err := batch.AddUTXO(utxo); err != nil {
			t.Fatalf("Failed to add UTXO to batch: %v", err)
		}
	}

	if err := batch.Commit(); err != nil {
		t.Fatalf("Failed to commit UTXO batch: %v", err)
	}

	// Verify UTXOs exist
	for i := 0; i < 10; i++ {
		txHash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", i))
		utxo, err := db.GetUTXO(txHash, 0)
		if err != nil {
			t.Errorf("Failed to get UTXO %d: %v", i, err)
		}
		if utxo.Value != int64(i*100000000) {
			t.Errorf("Wrong UTXO value for %d: got %d", i, utxo.Value)
		}
	}

	// Test UTXO removal in batch
	batch2 := db.NewBatchWriter(0)
	txHash0, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", 0))
	txHash1, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", 1))
	batch2.RemoveUTXO(txHash0, 0)
	batch2.RemoveUTXO(txHash1, 0)

	if err := batch2.Commit(); err != nil {
		t.Fatalf("Failed to commit UTXO removal batch: %v", err)
	}

	// Verify UTXOs removed
	if _, err := db.GetUTXO(txHash0, 0); err == nil {
		t.Error("Expected UTXO 0 to be removed")
	}
	if _, err := db.GetUTXO(txHash1, 0); err == nil {
		t.Error("Expected UTXO 1 to be removed")
	}

	// Verify others still exist
	txHash2, _ := chainhash.NewHashFromStr(fmt.Sprintf("%064x", 2))
	if _, err := db.GetUTXO(txHash2, 0); err != nil {
		t.Errorf("UTXO 2 should still exist: %v", err)
	}
}

// TestBatchWriter_Immutability tests that queued batch inputs are snapshotted at enqueue time.
func TestBatchWriter_Immutability(t *testing.T) {
	db, err := NewNameDatabase(t.TempDir() + "/batch_immutability.db")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	batch := db.NewBatchWriter(0)

	nameHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000111")
	nameRecord := &NameRecord{
		Name:      "d/test",
		Value:     "original",
		TxHash:    *nameHash,
		Height:    111,
		ExpiresAt: 36111,
		UpdatedAt: time.Now(),
	}
	if err := batch.PutName(nameRecord.Name, nameRecord); err != nil {
		t.Fatalf("Failed to queue name: %v", err)
	}

	historyHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000222")
	historyRecord := &NameRecord{
		Name:      "d/test",
		Value:     "history-original",
		TxHash:    *historyHash,
		Height:    100,
		ExpiresAt: 36100,
		UpdatedAt: time.Now(),
	}
	if err := batch.AddHistory(*historyHash, historyRecord); err != nil {
		t.Fatalf("Failed to queue history: %v", err)
	}

	utxoHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000333")
	utxo := &UTXO{
		TxHash:   *utxoHash,
		OutIndex: 1,
		Value:    12345,
		Address:  "NOriginal",
		PkScript: []byte{0x51, 0x52, 0x53},
		Height:   112,
	}
	if err := batch.AddUTXO(utxo); err != nil {
		t.Fatalf("Failed to queue UTXO: %v", err)
	}

	commitHash := []byte{0x01, 0x02, 0x03, 0x04}
	originalCommitHash := append([]byte(nil), commitHash...)
	if err := batch.PutNameNew(commitHash, 120); err != nil {
		t.Fatalf("Failed to queue name_new: %v", err)
	}

	nameRecord.Value = "mutated"
	historyRecord.Value = "history-mutated"
	utxo.Value = 99999
	utxo.Address = "NMutated"
	utxo.PkScript[0] = 0xff
	commitHash[0] = 0xaa

	if err := batch.Commit(); err != nil {
		t.Fatalf("Failed to commit batch: %v", err)
	}

	storedName, err := db.GetName("d/test")
	if err != nil {
		t.Fatalf("Failed to get stored name: %v", err)
	}
	if storedName.Value != "original" {
		t.Fatalf("Expected original name value, got %q", storedName.Value)
	}

	history, err := db.GetHistory("d/test")
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("Expected 1 history entry, got %d", len(history))
	}
	if history[0].Value != "history-original" {
		t.Fatalf("Expected original history value, got %q", history[0].Value)
	}

	storedUTXO, err := db.GetUTXO(utxoHash, 1)
	if err != nil {
		t.Fatalf("Failed to get stored UTXO: %v", err)
	}
	if storedUTXO.Value != 12345 {
		t.Fatalf("Expected original UTXO value, got %d", storedUTXO.Value)
	}
	if storedUTXO.Address != "NOriginal" {
		t.Fatalf("Expected original UTXO address, got %q", storedUTXO.Address)
	}
	if len(storedUTXO.PkScript) != 3 || storedUTXO.PkScript[0] != 0x51 {
		t.Fatalf("Expected original UTXO script, got %v", storedUTXO.PkScript)
	}

	nameNewRecord, err := db.GetNameNew(originalCommitHash)
	if err != nil {
		t.Fatalf("Failed to get stored name_new: %v", err)
	}
	if nameNewRecord.Height != 120 {
		t.Fatalf("Expected original name_new height, got %d", nameNewRecord.Height)
	}
	if _, err := db.GetNameNew(commitHash); err == nil {
		t.Fatal("Expected mutated commitment hash to be absent")
	}
}

// TestBatchWriter_EmptyCommit tests that committing an empty batch is safe
func TestBatchWriter_EmptyCommit(t *testing.T) {
	db, err := NewNameDatabase(t.TempDir() + "/batch_empty.db")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	batch := db.NewBatchWriter(0)

	// Commit empty batch (should be no-op)
	if err := batch.Commit(); err != nil {
		t.Errorf("Empty batch commit failed: %v", err)
	}

	if batch.Size() != 0 {
		t.Errorf("Expected batch size 0 after empty commit, got %d", batch.Size())
	}
}

// TestBatchWriter_CacheCoherence tests that batch commits update the cache
func TestBatchWriter_CacheCoherence(t *testing.T) {
	db, err := NewNameDatabase(t.TempDir() + "/batch_cache.db")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Put a name directly
	txHash1, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	db.PutName("d/test", &NameRecord{
		Name:      "d/test",
		Value:     "original",
		TxHash:    *txHash1,
		Height:    100,
		ExpiresAt: 36100,
		UpdatedAt: time.Now(),
	})

	// Read it to populate cache
	record, _ := db.GetName("d/test")
	if record.Value != "original" {
		t.Errorf("Wrong cached value: %s", record.Value)
	}

	// Update via batch
	batch := db.NewBatchWriter(0)
	txHash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000002")
	batch.PutName("d/test", &NameRecord{
		Name:      "d/test",
		Value:     "updated",
		TxHash:    *txHash2,
		Height:    101,
		ExpiresAt: 36101,
		UpdatedAt: time.Now(),
	})
	batch.Commit()

	// Cache should be updated
	record, _ = db.GetName("d/test")
	if record.Value != "updated" {
		t.Errorf("Cache not updated after batch commit: %s", record.Value)
	}

	// Delete via batch
	batch2 := db.NewBatchWriter(0)
	batch2.DeleteName("d/test")
	batch2.Commit()

	// Cache should be invalidated
	if _, err := db.GetName("d/test"); err == nil {
		t.Error("Cache not invalidated after batch delete")
	}
}
