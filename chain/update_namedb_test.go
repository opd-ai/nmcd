package chain

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/namedb"
)

// createTestBlockWithHeight creates a basic block with a coinbase at the given height.
func createTestBlockWithHeight(height int32) *btcutil.Block {
	coinbaseTx := wire.NewMsgTx(1)
	coinbaseTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0xffffffff,
		},
		SignatureScript: []byte{0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04},
		Sequence:        0xffffffff,
	})
	coinbaseTx.AddTxOut(&wire.TxOut{
		Value:    50 * config.CoinValue,
		PkScript: makeP2PKHScript(),
	})

	header := wire.BlockHeader{
		Version:   1,
		PrevBlock: chainhash.Hash{},
		Timestamp: time.Now(),
		Bits:      0x207fffff,
		Nonce:     0,
	}

	msgBlock := wire.NewMsgBlock(&header)
	msgBlock.AddTransaction(coinbaseTx)

	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(height)

	return block
}

// TestUpdateNameDatabaseEmptyBlock tests processing an empty block
func TestUpdateNameDatabaseEmptyBlock(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	block := createTestBlockWithHeight(1)

	err := bc.updateNameDatabase(block)
	if err != nil {
		t.Errorf("updateNameDatabase failed on empty block: %v", err)
	}
}

// TestUpdateNameDatabaseWithCoinbaseOnly tests processing a block with only coinbase
func TestUpdateNameDatabaseWithCoinbaseOnly(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	block := createTestBlockWithHeight(100)

	err := bc.updateNameDatabase(block)
	if err != nil {
		t.Errorf("updateNameDatabase failed on coinbase-only block: %v", err)
	}
}

// TestUpdateNameDatabaseNameNew tests processing a NAME_NEW operation
func TestUpdateNameDatabaseNameNew(t *testing.T) {
	bc, ndb, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// Create a commitment hash (simulating a salted name hash)
	commitHash := make([]byte, 20)
	for i := range commitHash {
		commitHash[i] = byte(i)
	}

	// Create a NAME_NEW transaction
	nameNewTx := wire.NewMsgTx(1)
	nameNewTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{1, 2, 3},
			Index: 0,
		},
		SignatureScript: []byte{0x01},
		Sequence:        0xffffffff,
	})
	nameNewTx.AddTxOut(&wire.TxOut{
		Value:    config.MinNameOperationFee,
		PkScript: buildNameNewScript(commitHash),
	})

	// Create coinbase transaction
	coinbaseTx := wire.NewMsgTx(1)
	coinbaseTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0xffffffff,
		},
		SignatureScript: []byte{0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04},
		Sequence:        0xffffffff,
	})
	coinbaseTx.AddTxOut(&wire.TxOut{
		Value:    50 * config.CoinValue,
		PkScript: makeP2PKHScript(),
	})

	// Create block
	header := wire.BlockHeader{
		Version:   1,
		PrevBlock: chainhash.Hash{},
		Timestamp: time.Now(),
		Bits:      0x207fffff,
		Nonce:     0,
	}
	msgBlock := wire.NewMsgBlock(&header)
	msgBlock.AddTransaction(coinbaseTx)
	msgBlock.AddTransaction(nameNewTx)

	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(100)

	err := bc.updateNameDatabase(block)
	if err != nil {
		t.Fatalf("updateNameDatabase failed: %v", err)
	}

	// Verify NAME_NEW was stored
	nameNewRecord, err := ndb.GetNameNew(commitHash)
	if err != nil {
		t.Errorf("Failed to get NAME_NEW record: %v", err)
	}
	if nameNewRecord == nil {
		t.Error("Expected NAME_NEW record to be stored")
	} else if nameNewRecord.Height != 100 {
		t.Errorf("Expected NAME_NEW height 100, got %d", nameNewRecord.Height)
	}
}

// TestUpdateNameDatabaseExpiredNames tests the cleanup of expired names
func TestUpdateNameDatabaseExpiredNames(t *testing.T) {
	bc, ndb, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// Create a name that will expire
	testName := "d/expiretest"
	record := &namedb.NameRecord{
		Name:      testName,
		Value:     `{"ip": "192.168.1.1"}`,
		Height:    100,
		ExpiresAt: 200, // Will expire at height 200
		Address:   "mtest123",
	}
	if err := ndb.PutName(testName, record); err != nil {
		t.Fatalf("Failed to create test name: %v", err)
	}

	// Process a block at height 201 (after expiration)
	block := createTestBlockWithHeight(201)

	err := bc.updateNameDatabase(block)
	if err != nil {
		t.Fatalf("updateNameDatabase failed: %v", err)
	}

	// Verify the name was deleted due to expiration
	_, err = ndb.GetName(testName)
	if err == nil {
		t.Error("Expected expired name to be deleted")
	}
}

// TestUpdateNameDatabaseMultipleTransactions tests processing multiple transactions in one block
func TestUpdateNameDatabaseMultipleTransactions(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// Create multiple NAME_NEW transactions
	coinbaseTx := wire.NewMsgTx(1)
	coinbaseTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0xffffffff,
		},
		SignatureScript: []byte{0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04},
		Sequence:        0xffffffff,
	})
	coinbaseTx.AddTxOut(&wire.TxOut{
		Value:    50 * config.CoinValue,
		PkScript: makeP2PKHScript(),
	})

	msgBlock := wire.NewMsgBlock(&wire.BlockHeader{
		Version:   1,
		PrevBlock: chainhash.Hash{},
		Timestamp: time.Now(),
		Bits:      0x207fffff,
	})
	msgBlock.AddTransaction(coinbaseTx)

	// Add 3 regular transactions (not name ops, just with P2PKH outputs)
	for i := 0; i < 3; i++ {
		tx := wire.NewMsgTx(1)
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{byte(i + 1)},
				Index: 0,
			},
			SignatureScript: []byte{0x01},
			Sequence:        0xffffffff,
		})
		tx.AddTxOut(&wire.TxOut{
			Value:    1000000,
			PkScript: makeP2PKHScript(),
		})
		msgBlock.AddTransaction(tx)
	}

	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(50)

	err := bc.updateNameDatabase(block)
	if err != nil {
		t.Errorf("updateNameDatabase failed with multiple transactions: %v", err)
	}
}

// TestUpdateNameDatabaseNameFirstUpdate tests processing a NAME_FIRSTUPDATE operation
func TestUpdateNameDatabaseNameFirstUpdate(t *testing.T) {
	bc, ndb, cleanup := setupTestBlockChain(t)
	defer cleanup()

	testName := "d/testname"
	randBytes := []byte("randomsalt1234567890") // 20 bytes
	testValue := []byte(`{"ip": "10.0.0.1"}`)

	// First, store a NAME_NEW commitment (simulating a prior block)
	// Note: We need to compute the commit hash the same way the code does
	commitHash := computeCommitHash(randBytes, testName)
	if err := ndb.PutNameNew(commitHash, 90); err != nil {
		t.Fatalf("Failed to store NAME_NEW: %v", err)
	}

	// Create NAME_FIRSTUPDATE transaction
	firstUpdateTx := wire.NewMsgTx(1)
	firstUpdateTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{1, 2, 3},
			Index: 0,
		},
		SignatureScript: []byte{0x01},
		Sequence:        0xffffffff,
	})
	firstUpdateTx.AddTxOut(&wire.TxOut{
		Value:    config.MinNameOperationFee,
		PkScript: buildNameFirstUpdateScript([]byte(testName), randBytes, testValue),
	})

	// Create block with coinbase and NAME_FIRSTUPDATE
	coinbaseTx := wire.NewMsgTx(1)
	coinbaseTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0xffffffff,
		},
		SignatureScript: []byte{0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04},
		Sequence:        0xffffffff,
	})
	coinbaseTx.AddTxOut(&wire.TxOut{
		Value:    50 * config.CoinValue,
		PkScript: makeP2PKHScript(),
	})

	header := wire.BlockHeader{
		Version:   1,
		PrevBlock: chainhash.Hash{},
		Timestamp: time.Now(),
		Bits:      0x207fffff,
	}
	msgBlock := wire.NewMsgBlock(&header)
	msgBlock.AddTransaction(coinbaseTx)
	msgBlock.AddTransaction(firstUpdateTx)

	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(100) // 10 blocks after NAME_NEW at 90

	err := bc.updateNameDatabase(block)
	if err != nil {
		t.Fatalf("updateNameDatabase failed: %v", err)
	}

	// Verify the name was registered
	record, err := ndb.GetName(testName)
	if err != nil {
		t.Fatalf("Failed to get name: %v", err)
	}
	if record == nil {
		t.Fatal("Expected name to be registered")
	}
	if record.Height != 100 {
		t.Errorf("Expected height 100, got %d", record.Height)
	}
	if string(record.Value) != string(testValue) {
		t.Errorf("Expected value %s, got %s", testValue, record.Value)
	}

	// Verify NAME_NEW commitment was deleted
	nameNewRecord, _ := ndb.GetNameNew(commitHash)
	if nameNewRecord != nil {
		t.Error("Expected NAME_NEW commitment to be deleted after NAME_FIRSTUPDATE")
	}
}

// TestUpdateNameDatabaseNameUpdate tests processing a NAME_UPDATE operation
func TestUpdateNameDatabaseNameUpdate(t *testing.T) {
	bc, ndb, cleanup := setupTestBlockChain(t)
	defer cleanup()

	testName := "d/updatetest"
	originalValue := `{"ip": "1.1.1.1"}`
	updatedValue := []byte(`{"ip": "2.2.2.2"}`)

	// First, create an existing name record
	record := &namedb.NameRecord{
		Name:          testName,
		Value:         originalValue,
		Height:        50,
		ExpiresAt:     50 + config.NameExpirationBlocks,
		Address:       "mtest123",
		NameNewHeight: 45,
	}
	if err := ndb.PutName(testName, record); err != nil {
		t.Fatalf("Failed to create test name: %v", err)
	}

	// Create NAME_UPDATE transaction
	updateTx := wire.NewMsgTx(1)
	updateTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{1, 2, 3},
			Index: 0,
		},
		SignatureScript: []byte{0x01},
		Sequence:        0xffffffff,
	})
	updateTx.AddTxOut(&wire.TxOut{
		Value:    config.MinNameOperationFee,
		PkScript: buildNameUpdateScript([]byte(testName), updatedValue),
	})

	// Create block with coinbase and NAME_UPDATE
	coinbaseTx := wire.NewMsgTx(1)
	coinbaseTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0xffffffff,
		},
		SignatureScript: []byte{0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04},
		Sequence:        0xffffffff,
	})
	coinbaseTx.AddTxOut(&wire.TxOut{
		Value:    50 * config.CoinValue,
		PkScript: makeP2PKHScript(),
	})

	header := wire.BlockHeader{
		Version:   1,
		PrevBlock: chainhash.Hash{},
		Timestamp: time.Now(),
		Bits:      0x207fffff,
	}
	msgBlock := wire.NewMsgBlock(&header)
	msgBlock.AddTransaction(coinbaseTx)
	msgBlock.AddTransaction(updateTx)

	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(100)

	err := bc.updateNameDatabase(block)
	if err != nil {
		t.Fatalf("updateNameDatabase failed: %v", err)
	}

	// Verify the name was updated
	updatedRecord, err := ndb.GetName(testName)
	if err != nil {
		t.Fatalf("Failed to get updated name: %v", err)
	}
	if updatedRecord == nil {
		t.Fatal("Expected name to exist after update")
	}
	if updatedRecord.Height != 100 {
		t.Errorf("Expected height 100, got %d", updatedRecord.Height)
	}
	if string(updatedRecord.Value) != string(updatedValue) {
		t.Errorf("Expected value %s, got %s", updatedValue, updatedRecord.Value)
	}
	// Verify NameNewHeight was preserved
	if updatedRecord.NameNewHeight != 45 {
		t.Errorf("Expected NameNewHeight 45, got %d", updatedRecord.NameNewHeight)
	}
}

// TestUpdateNameDatabaseUTXOTracking tests UTXO creation and removal
func TestUpdateNameDatabaseUTXOTracking(t *testing.T) {
	bc, ndb, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// Create a block with a transaction that has multiple outputs
	coinbaseTx := wire.NewMsgTx(1)
	coinbaseTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0xffffffff,
		},
		SignatureScript: []byte{0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04},
		Sequence:        0xffffffff,
	})
	coinbaseTx.AddTxOut(&wire.TxOut{
		Value:    50 * config.CoinValue,
		PkScript: makeP2PKHScript(),
	})

	msgBlock := wire.NewMsgBlock(&wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
		Bits:      0x207fffff,
	})
	msgBlock.AddTransaction(coinbaseTx)

	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(1)

	err := bc.updateNameDatabase(block)
	if err != nil {
		t.Fatalf("updateNameDatabase failed: %v", err)
	}

	// Verify UTXOs were created for coinbase outputs
	coinbaseTxHash := coinbaseTx.TxHash()
	utxo, err := ndb.GetUTXO(&coinbaseTxHash, 0)
	if err != nil {
		t.Errorf("Failed to get UTXO: %v", err)
	}
	if utxo == nil {
		t.Error("Expected UTXO to be created for coinbase output")
	} else if utxo.Value != 50*config.CoinValue {
		t.Errorf("Expected UTXO value %d, got %d", 50*config.CoinValue, utxo.Value)
	}
}

// TestUpdateNameDatabaseConcurrency tests concurrent access to updateNameDatabase
func TestUpdateNameDatabaseConcurrency(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// Run multiple concurrent updates
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func(height int32) {
			block := createTestBlockWithHeight(height)
			_ = bc.updateNameDatabase(block)
			done <- true
		}(int32(i + 1))
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}
	// If we get here without deadlock, the test passes
}

// TestUpdateNameDatabaseSpentUTXOCleanup tests the periodic cleanup of spent UTXOs
func TestUpdateNameDatabaseSpentUTXOCleanup(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// Process a block at height that triggers cleanup (height > 1000 and height % 100 == 0)
	block := createTestBlockWithHeight(1100) // > 1000 and divisible by 100

	err := bc.updateNameDatabase(block)
	if err != nil {
		t.Errorf("updateNameDatabase failed at cleanup height: %v", err)
	}
}

// TestUpdateNameDatabaseInputSpending tests UTXO removal when inputs are spent
func TestUpdateNameDatabaseInputSpending(t *testing.T) {
	bc, ndb, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// First create a UTXO by processing a coinbase block
	coinbaseTx := wire.NewMsgTx(1)
	coinbaseTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0xffffffff,
		},
		SignatureScript: []byte{0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04},
		Sequence:        0xffffffff,
	})
	coinbaseTx.AddTxOut(&wire.TxOut{
		Value:    50 * config.CoinValue,
		PkScript: makeP2PKHScript(),
	})

	msgBlock1 := wire.NewMsgBlock(&wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
		Bits:      0x207fffff,
	})
	msgBlock1.AddTransaction(coinbaseTx)

	block1 := btcutil.NewBlock(msgBlock1)
	block1.SetHeight(1)

	if err := bc.updateNameDatabase(block1); err != nil {
		t.Fatalf("Failed to process first block: %v", err)
	}

	// Get the coinbase tx hash
	coinbaseTxHash := coinbaseTx.TxHash()

	// Verify UTXO exists
	utxo, err := ndb.GetUTXO(&coinbaseTxHash, 0)
	if err != nil || utxo == nil {
		t.Fatalf("UTXO not created: %v", err)
	}

	// Now create a second block that spends the coinbase UTXO
	spendTx := wire.NewMsgTx(1)
	spendTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  coinbaseTxHash,
			Index: 0,
		},
		SignatureScript: []byte{0x01},
		Sequence:        0xffffffff,
	})
	spendTx.AddTxOut(&wire.TxOut{
		Value:    49 * config.CoinValue,
		PkScript: makeP2PKHScript(),
	})

	coinbaseTx2 := wire.NewMsgTx(1)
	coinbaseTx2.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0xffffffff,
		},
		SignatureScript: []byte{0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04},
		Sequence:        0xffffffff,
	})
	coinbaseTx2.AddTxOut(&wire.TxOut{
		Value:    50 * config.CoinValue,
		PkScript: makeP2PKHScript(),
	})

	msgBlock2 := wire.NewMsgBlock(&wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
		Bits:      0x207fffff,
	})
	msgBlock2.AddTransaction(coinbaseTx2)
	msgBlock2.AddTransaction(spendTx)

	block2 := btcutil.NewBlock(msgBlock2)
	block2.SetHeight(2)

	if err := bc.updateNameDatabase(block2); err != nil {
		t.Fatalf("Failed to process second block: %v", err)
	}

	// Verify the original UTXO was removed
	utxo, err = ndb.GetUTXO(&coinbaseTxHash, 0)
	if err == nil && utxo != nil {
		t.Error("Expected UTXO to be removed after being spent")
	}
}
