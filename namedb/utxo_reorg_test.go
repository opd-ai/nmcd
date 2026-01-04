package namedb

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// TestSpentUTXOStorage tests storing and retrieving spent UTXOs
func TestSpentUTXOStorage(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	ndb, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	// Create test UTXO
	txHash := chainhash.HashH([]byte("test transaction"))
	utxo := &UTXO{
		TxHash:   txHash,
		OutIndex: 0,
		Value:    100000,
		Address:  "NTestAddress123",
		PkScript: []byte{0x76, 0xa9, 0x14}, // Simplified P2PKH
		Height:   100,
	}

	// Store spent UTXO
	spentAtHeight := int32(200)
	err = ndb.StoreSpentUTXO(utxo, spentAtHeight)
	if err != nil {
		t.Fatalf("Failed to store spent UTXO: %v", err)
	}

	// Verify we can retrieve it by restoring UTXOs for that height
	// First remove it from active set (simulating it was spent)
	// Note: It wasn't in active set, so this is just for consistency

	// Restore spent UTXOs for the block
	err = ndb.RestoreSpentUTXOsForBlock(spentAtHeight)
	if err != nil {
		t.Fatalf("Failed to restore spent UTXOs: %v", err)
	}

	// Now the UTXO should be in the active set
	restoredUTXO, err := ndb.GetUTXO(&txHash, 0)
	if err != nil {
		t.Fatalf("Failed to get restored UTXO: %v", err)
	}

	// Verify data matches
	if restoredUTXO.Value != utxo.Value {
		t.Errorf("Value mismatch: got %d, want %d", restoredUTXO.Value, utxo.Value)
	}
	if restoredUTXO.Address != utxo.Address {
		t.Errorf("Address mismatch: got %s, want %s", restoredUTXO.Address, utxo.Address)
	}
	if restoredUTXO.Height != utxo.Height {
		t.Errorf("Height mismatch: got %d, want %d", restoredUTXO.Height, utxo.Height)
	}
}

// TestMultipleSpentUTXOsPerBlock tests restoring multiple UTXOs spent in same block
func TestMultipleSpentUTXOsPerBlock(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	ndb, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	spentAtHeight := int32(300)
	numUTXOs := 5

	// Create and store multiple spent UTXOs
	utxos := make([]*UTXO, numUTXOs)
	for i := 0; i < numUTXOs; i++ {
		txHash := chainhash.HashH([]byte{byte(i)})
		utxo := &UTXO{
			TxHash:   txHash,
			OutIndex: uint32(i),
			Value:    int64(100000 + i*1000),
			Address:  "NTestAddress",
			PkScript: []byte{0x76, 0xa9, 0x14, byte(i)},
			Height:   100 + int32(i),
		}
		utxos[i] = utxo

		err = ndb.StoreSpentUTXO(utxo, spentAtHeight)
		if err != nil {
			t.Fatalf("Failed to store spent UTXO %d: %v", i, err)
		}
	}

	// Restore all UTXOs for the block
	err = ndb.RestoreSpentUTXOsForBlock(spentAtHeight)
	if err != nil {
		t.Fatalf("Failed to restore spent UTXOs: %v", err)
	}

	// Verify all UTXOs were restored
	for i := 0; i < numUTXOs; i++ {
		utxo := utxos[i]
		restoredUTXO, err := ndb.GetUTXO(&utxo.TxHash, utxo.OutIndex)
		if err != nil {
			t.Errorf("Failed to get restored UTXO %d: %v", i, err)
			continue
		}

		if restoredUTXO.Value != utxo.Value {
			t.Errorf("UTXO %d value mismatch: got %d, want %d", i, restoredUTXO.Value, utxo.Value)
		}
	}
}

// TestSpentUTXOCleanup tests cleanup of old spent UTXOs
func TestSpentUTXOCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	ndb, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	// Store spent UTXOs at different heights
	heights := []int32{100, 200, 300, 400, 500}
	utxos := make(map[int32]*UTXO)

	for _, height := range heights {
		txHash := chainhash.HashH([]byte{byte(height)})
		utxo := &UTXO{
			TxHash:   txHash,
			OutIndex: 0,
			Value:    int64(height * 1000),
			Address:  "NTestAddress",
			PkScript: []byte{0x76, 0xa9, 0x14},
			Height:   height - 10,
		}
		utxos[height] = utxo

		err = ndb.StoreSpentUTXO(utxo, height)
		if err != nil {
			t.Fatalf("Failed to store spent UTXO at height %d: %v", height, err)
		}
	}

	// Cleanup UTXOs older than height 300
	keepFromHeight := int32(300)
	err = ndb.CleanupOldSpentUTXOs(keepFromHeight)
	if err != nil {
		t.Fatalf("Failed to cleanup old spent UTXOs: %v", err)
	}

	// Try to restore UTXOs from different heights
	// Heights 100, 200 should be cleaned up (not restorable)
	// Heights 300, 400, 500 should still exist

	for _, height := range heights {
		err = ndb.RestoreSpentUTXOsForBlock(height)
		if err != nil {
			t.Fatalf("Failed to restore spent UTXOs for height %d: %v", height, err)
		}

		utxo := utxos[height]
		_, err := ndb.GetUTXO(&utxo.TxHash, utxo.OutIndex)

		if height < keepFromHeight {
			// Should NOT be restored (cleaned up)
			if err == nil {
				t.Errorf("UTXO at height %d should have been cleaned up but was restored", height)
			}
		} else {
			// Should be restored
			if err != nil {
				t.Errorf("UTXO at height %d should be restorable but got error: %v", height, err)
			}
		}
	}
}

// TestSpentUTXOReorgScenario tests a realistic reorg scenario
func TestSpentUTXOReorgScenario(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	ndb, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	// Simulate block connection: create UTXO at height 100
	txHash1 := chainhash.HashH([]byte("transaction1"))
	utxo1 := &UTXO{
		TxHash:   txHash1,
		OutIndex: 0,
		Value:    100000,
		Address:  "NTestAddress1",
		PkScript: []byte{0x76, 0xa9, 0x14, 0x01},
		Height:   100,
	}
	err = ndb.AddUTXO(utxo1)
	if err != nil {
		t.Fatalf("Failed to add UTXO: %v", err)
	}

	// Verify UTXO is in active set
	_, err = ndb.GetUTXO(&txHash1, 0)
	if err != nil {
		t.Fatalf("UTXO should be in active set: %v", err)
	}

	// Simulate block connection at height 150: spend utxo1
	// Store spent UTXO before removing
	err = ndb.StoreSpentUTXO(utxo1, 150)
	if err != nil {
		t.Fatalf("Failed to store spent UTXO: %v", err)
	}

	// Remove from active set
	err = ndb.RemoveUTXO(&txHash1, 0)
	if err != nil {
		t.Fatalf("Failed to remove UTXO: %v", err)
	}

	// Verify UTXO is no longer in active set
	_, err = ndb.GetUTXO(&txHash1, 0)
	if err == nil {
		t.Fatalf("UTXO should not be in active set after spending")
	}

	// Simulate reorg: disconnect block 150
	err = ndb.RestoreSpentUTXOsForBlock(150)
	if err != nil {
		t.Fatalf("Failed to restore spent UTXOs during reorg: %v", err)
	}

	// Verify UTXO is back in active set
	restoredUTXO, err := ndb.GetUTXO(&txHash1, 0)
	if err != nil {
		t.Fatalf("UTXO should be restored to active set: %v", err)
	}

	// Verify data integrity
	if restoredUTXO.Value != utxo1.Value {
		t.Errorf("Restored UTXO value mismatch: got %d, want %d", restoredUTXO.Value, utxo1.Value)
	}
	if restoredUTXO.Address != utxo1.Address {
		t.Errorf("Restored UTXO address mismatch: got %s, want %s", restoredUTXO.Address, utxo1.Address)
	}
}

// TestSpentUTXOMultipleOutputsPerTransaction tests handling multiple outputs from same tx
func TestSpentUTXOMultipleOutputsPerTransaction(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	ndb, err := NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	// Create transaction with multiple outputs
	txHash := chainhash.HashH([]byte("multi-output-tx"))
	numOutputs := 3

	// Add all outputs to active set
	for i := 0; i < numOutputs; i++ {
		utxo := &UTXO{
			TxHash:   txHash,
			OutIndex: uint32(i),
			Value:    int64(10000 * (i + 1)),
			Address:  "NTestAddress",
			PkScript: []byte{0x76, 0xa9, 0x14, byte(i)},
			Height:   100,
		}
		err = ndb.AddUTXO(utxo)
		if err != nil {
			t.Fatalf("Failed to add UTXO %d: %v", i, err)
		}
	}

	// Spend output 1 at height 150
	utxo1, _ := ndb.GetUTXO(&txHash, 1)
	err = ndb.StoreSpentUTXO(utxo1, 150)
	if err != nil {
		t.Fatalf("Failed to store spent UTXO: %v", err)
	}
	err = ndb.RemoveUTXO(&txHash, 1)
	if err != nil {
		t.Fatalf("Failed to remove UTXO: %v", err)
	}

	// Verify output 0 and 2 are still in active set
	_, err = ndb.GetUTXO(&txHash, 0)
	if err != nil {
		t.Errorf("Output 0 should still be in active set: %v", err)
	}
	_, err = ndb.GetUTXO(&txHash, 2)
	if err != nil {
		t.Errorf("Output 2 should still be in active set: %v", err)
	}

	// Verify output 1 is not in active set
	_, err = ndb.GetUTXO(&txHash, 1)
	if err == nil {
		t.Errorf("Output 1 should not be in active set")
	}

	// Restore spent UTXOs
	err = ndb.RestoreSpentUTXOsForBlock(150)
	if err != nil {
		t.Fatalf("Failed to restore spent UTXOs: %v", err)
	}

	// Verify all outputs are back in active set
	for i := 0; i < numOutputs; i++ {
		_, err = ndb.GetUTXO(&txHash, uint32(i))
		if err != nil {
			t.Errorf("Output %d should be in active set after restoration: %v", i, err)
		}
	}
}
