package chain

import (
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/namedb"
)

// TestValidateMempoolTransactionNil tests nil transaction validation
func TestValidateMempoolTransactionNil(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	err := bc.ValidateMempoolTransaction(nil)
	if err == nil {
		t.Error("Expected error for nil transaction")
	}
	if err.Error() != "transaction is nil" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestValidateMempoolTransactionEmptyTx tests transaction with no inputs/outputs
func TestValidateMempoolTransactionEmptyTx(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	tx := wire.NewMsgTx(wire.TxVersion)

	err := bc.ValidateMempoolTransaction(tx)
	if err == nil {
		t.Error("Expected error for empty transaction")
	}
}

// TestValidateMempoolTransactionNoOutputs tests transaction with inputs but no outputs
func TestValidateMempoolTransactionNoOutputs(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	tx := wire.NewMsgTx(wire.TxVersion)
	// Add an input
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{0x01},
			Index: 0,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})

	err := bc.ValidateMempoolTransaction(tx)
	if err == nil {
		t.Error("Expected error for transaction with no outputs")
	}
}

// TestValidateMempoolTransactionNoInputs tests transaction with outputs but no inputs
func TestValidateMempoolTransactionNoInputs(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	tx := wire.NewMsgTx(wire.TxVersion)
	// Add an output
	tx.AddTxOut(&wire.TxOut{
		Value:    1000,
		PkScript: []byte{0x76, 0xa9, 0x14},
	})

	err := bc.ValidateMempoolTransaction(tx)
	if err == nil {
		t.Error("Expected error for transaction with no inputs")
	}
}

// TestValidateMempoolTransactionCoinbase tests coinbase transactions are rejected
func TestValidateMempoolTransactionCoinbase(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	tx := wire.NewMsgTx(wire.TxVersion)
	// Coinbase input has null hash and 0xFFFFFFFF index
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: wire.MaxPrevOutIndex,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    5000000000, // 50 NMC
		PkScript: []byte{0x76, 0xa9, 0x14},
	})

	err := bc.ValidateMempoolTransaction(tx)
	if err == nil {
		t.Error("Expected error for coinbase transaction")
	}
}

// TestValidateMempoolTransactionValid tests a valid regular transaction
func TestValidateMempoolTransactionValid(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	tx := wire.NewMsgTx(wire.TxVersion)
	// Add a normal input (non-coinbase)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{0x01, 0x02, 0x03},
			Index: 0,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})
	// Add an output
	tx.AddTxOut(&wire.TxOut{
		Value:    1000,
		PkScript: []byte{0x76, 0xa9, 0x14},
	})

	err := bc.ValidateMempoolTransaction(tx)
	if err != nil {
		t.Errorf("Unexpected error for valid transaction: %v", err)
	}
}

// TestValidateMempoolTransactionNameNewDuplicate tests NAME_NEW with existing commitment
func TestValidateMempoolTransactionNameNewDuplicate(t *testing.T) {
	bc, ndb, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// First, add a NAME_NEW record to the database
	commitment := make([]byte, 20)
	commitment[0] = 0xAA
	err := ndb.PutNameNew(commitment, 100, chainhash.Hash{0x01}, 0)
	if err != nil {
		t.Fatalf("Failed to add NAME_NEW: %v", err)
	}

	// Create a transaction with NAME_NEW that has the same commitment
	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{0x01, 0x02, 0x03},
			Index: 0,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})

	// Create NAME_NEW script: OP_NAME_NEW <commitment> OP_2DROP <pubkey script>
	// Opcode 0xd0 = NAME_NEW
	nameNewScript := make([]byte, 0, 50)
	nameNewScript = append(nameNewScript, 0xd0)             // OP_NAME_NEW
	nameNewScript = append(nameNewScript, byte(len(commitment))) // Push commitment length
	nameNewScript = append(nameNewScript, commitment...)    // Commitment
	nameNewScript = append(nameNewScript, 0x6d)             // OP_2DROP
	nameNewScript = append(nameNewScript, 0x76, 0xa9, 0x14) // Regular P2PKH

	tx.AddTxOut(&wire.TxOut{
		Value:    config.MinNameFee,
		PkScript: nameNewScript,
	})

	err = bc.ValidateMempoolTransaction(tx)
	if err == nil {
		t.Error("Expected error for duplicate NAME_NEW commitment")
	}
}

// TestValidateMempoolTransactionNameFirstUpdateNoNameNew tests NAME_FIRSTUPDATE without NAME_NEW
func TestValidateMempoolTransactionNameFirstUpdateNoNameNew(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{0x01, 0x02, 0x03},
			Index: 0,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})

	// Create NAME_FIRSTUPDATE script
	// Opcode 0xd1 = NAME_FIRSTUPDATE
	name := "d/test"
	value := `{"ip":"1.2.3.4"}`
	salt := make([]byte, 20)
	salt[0] = 0xBB

	nameFirstUpdateScript := make([]byte, 0, 100)
	nameFirstUpdateScript = append(nameFirstUpdateScript, 0xd1)           // OP_NAME_FIRSTUPDATE
	nameFirstUpdateScript = append(nameFirstUpdateScript, byte(len(name))) // Push name length
	nameFirstUpdateScript = append(nameFirstUpdateScript, []byte(name)...) // Name
	nameFirstUpdateScript = append(nameFirstUpdateScript, byte(len(salt))) // Push salt length
	nameFirstUpdateScript = append(nameFirstUpdateScript, salt...)         // Salt (rand for NAME_NEW)
	nameFirstUpdateScript = append(nameFirstUpdateScript, byte(len(value))) // Push value length
	nameFirstUpdateScript = append(nameFirstUpdateScript, []byte(value)...) // Value
	nameFirstUpdateScript = append(nameFirstUpdateScript, 0x6d, 0x6d, 0x6d) // OP_2DROP x3
	nameFirstUpdateScript = append(nameFirstUpdateScript, 0x76, 0xa9, 0x14) // Regular P2PKH

	tx.AddTxOut(&wire.TxOut{
		Value:    config.MinNameFee,
		PkScript: nameFirstUpdateScript,
	})

	err := bc.ValidateMempoolTransaction(tx)
	if err == nil {
		t.Error("Expected error for NAME_FIRSTUPDATE without NAME_NEW")
	}
}

// TestValidateMempoolTransactionNameUpdateNoName tests NAME_UPDATE for non-existent name
func TestValidateMempoolTransactionNameUpdateNoName(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{0x01, 0x02, 0x03},
			Index: 0,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})

	// Create NAME_UPDATE script
	// Opcode 0xd2 = NAME_UPDATE
	name := "d/nonexistent"
	value := `{"ip":"5.6.7.8"}`

	nameUpdateScript := make([]byte, 0, 100)
	nameUpdateScript = append(nameUpdateScript, 0xd2)            // OP_NAME_UPDATE
	nameUpdateScript = append(nameUpdateScript, byte(len(name))) // Push name length
	nameUpdateScript = append(nameUpdateScript, []byte(name)...) // Name
	nameUpdateScript = append(nameUpdateScript, byte(len(value))) // Push value length
	nameUpdateScript = append(nameUpdateScript, []byte(value)...) // Value
	nameUpdateScript = append(nameUpdateScript, 0x6d, 0x6d)       // OP_2DROP x2
	nameUpdateScript = append(nameUpdateScript, 0x76, 0xa9, 0x14) // Regular P2PKH

	tx.AddTxOut(&wire.TxOut{
		Value:    config.MinNameFee,
		PkScript: nameUpdateScript,
	})

	err := bc.ValidateMempoolTransaction(tx)
	if err == nil {
		t.Error("Expected error for NAME_UPDATE of non-existent name")
	}
}

// TestValidateMempoolTransactionNameUpdateExpired tests NAME_UPDATE for expired name
func TestValidateMempoolTransactionNameUpdateExpired(t *testing.T) {
	bc, ndb, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// Add an expired name to the database
	testName := &namedb.NameRecord{
		Name:      "d/expired",
		Value:     `{"ip":"1.1.1.1"}`,
		Height:    1,
		ExpiresAt: 0, // Already expired at genesis
		Address:   "N1234567890",
		TxHash:    chainhash.Hash{0x11, 0x22, 0x33},
		OutIndex:  0,
	}
	if err := ndb.PutName(testName.Name, testName); err != nil {
		t.Fatalf("Failed to add test name: %v", err)
	}

	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  testName.TxHash,
			Index: testName.OutIndex,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})

	// Create NAME_UPDATE script
	name := "d/expired"
	value := `{"ip":"2.2.2.2"}`

	nameUpdateScript := make([]byte, 0, 100)
	nameUpdateScript = append(nameUpdateScript, 0xd2)            // OP_NAME_UPDATE
	nameUpdateScript = append(nameUpdateScript, byte(len(name))) // Push name length
	nameUpdateScript = append(nameUpdateScript, []byte(name)...) // Name
	nameUpdateScript = append(nameUpdateScript, byte(len(value))) // Push value length
	nameUpdateScript = append(nameUpdateScript, []byte(value)...) // Value
	nameUpdateScript = append(nameUpdateScript, 0x6d, 0x6d)       // OP_2DROP x2
	nameUpdateScript = append(nameUpdateScript, 0x76, 0xa9, 0x14) // Regular P2PKH

	tx.AddTxOut(&wire.TxOut{
		Value:    config.MinNameFee,
		PkScript: nameUpdateScript,
	})

	err := bc.ValidateMempoolTransaction(tx)
	if err == nil {
		t.Error("Expected error for NAME_UPDATE of expired name")
	}
}

// TestValidateMempoolTransactionNameUpdateWrongUTXO tests NAME_UPDATE that doesn't spend current UTXO
func TestValidateMempoolTransactionNameUpdateWrongUTXO(t *testing.T) {
	bc, ndb, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// Add a name with a specific UTXO
	testName := &namedb.NameRecord{
		Name:      "d/testutxo",
		Value:     `{"ip":"1.1.1.1"}`,
		Height:    1,
		ExpiresAt: 100000, // Not expired
		Address:   "N1234567890",
		TxHash:    chainhash.Hash{0x11, 0x22, 0x33},
		OutIndex:  0,
	}
	if err := ndb.PutName(testName.Name, testName); err != nil {
		t.Fatalf("Failed to add test name: %v", err)
	}

	tx := wire.NewMsgTx(wire.TxVersion)
	// Use WRONG UTXO - not the one the name points to
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{0xAA, 0xBB, 0xCC}, // Wrong hash
			Index: 0,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})

	// Create NAME_UPDATE script
	name := "d/testutxo"
	value := `{"ip":"2.2.2.2"}`

	nameUpdateScript := make([]byte, 0, 100)
	nameUpdateScript = append(nameUpdateScript, 0xd2)            // OP_NAME_UPDATE
	nameUpdateScript = append(nameUpdateScript, byte(len(name))) // Push name length
	nameUpdateScript = append(nameUpdateScript, []byte(name)...) // Name
	nameUpdateScript = append(nameUpdateScript, byte(len(value))) // Push value length
	nameUpdateScript = append(nameUpdateScript, []byte(value)...) // Value
	nameUpdateScript = append(nameUpdateScript, 0x6d, 0x6d)       // OP_2DROP x2
	nameUpdateScript = append(nameUpdateScript, 0x76, 0xa9, 0x14) // Regular P2PKH

	tx.AddTxOut(&wire.TxOut{
		Value:    config.MinNameFee,
		PkScript: nameUpdateScript,
	})

	err := bc.ValidateMempoolTransaction(tx)
	if err == nil {
		t.Error("Expected error for NAME_UPDATE with wrong UTXO (name theft)")
	}
}

// TestValidateMempoolTransactionNameUpdateValid tests valid NAME_UPDATE with correct UTXO
func TestValidateMempoolTransactionNameUpdateValid(t *testing.T) {
	bc, ndb, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// Add a name with a specific UTXO
	testName := &namedb.NameRecord{
		Name:      "d/validupdate",
		Value:     `{"ip":"1.1.1.1"}`,
		Height:    1,
		ExpiresAt: 100000, // Not expired
		Address:   "N1234567890",
		TxHash:    chainhash.Hash{0x11, 0x22, 0x33},
		OutIndex:  0,
	}
	if err := ndb.PutName(testName.Name, testName); err != nil {
		t.Fatalf("Failed to add test name: %v", err)
	}

	tx := wire.NewMsgTx(wire.TxVersion)
	// Use CORRECT UTXO
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  testName.TxHash,
			Index: testName.OutIndex,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})

	// Create NAME_UPDATE script
	name := "d/validupdate"
	value := `{"ip":"2.2.2.2"}`

	nameUpdateScript := make([]byte, 0, 100)
	nameUpdateScript = append(nameUpdateScript, 0xd2)            // OP_NAME_UPDATE
	nameUpdateScript = append(nameUpdateScript, byte(len(name))) // Push name length
	nameUpdateScript = append(nameUpdateScript, []byte(name)...) // Name
	nameUpdateScript = append(nameUpdateScript, byte(len(value))) // Push value length
	nameUpdateScript = append(nameUpdateScript, []byte(value)...) // Value
	nameUpdateScript = append(nameUpdateScript, 0x6d, 0x6d)       // OP_2DROP x2
	nameUpdateScript = append(nameUpdateScript, 0x76, 0xa9, 0x14) // Regular P2PKH

	tx.AddTxOut(&wire.TxOut{
		Value:    config.MinNameFee,
		PkScript: nameUpdateScript,
	})

	// Note: This may still fail due to fee validation, which is tested separately
	err := bc.ValidateMempoolTransaction(tx)
	// We expect either success or a fee validation error (not a name validation error)
	if err != nil && (err.Error() == "name not found for update: d/validupdate" ||
		err.Error() == "name_update does not spend current name UTXO: name theft attempt for d/validupdate") {
		t.Errorf("Unexpected name validation error: %v", err)
	}
}

// TestValidateMempoolTransactionMultipleNameOps tests transaction with multiple name operations
func TestValidateMempoolTransactionMultipleNameOps(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{0x01, 0x02, 0x03},
			Index: 0,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})

	// Create two NAME_NEW scripts
	commitment1 := make([]byte, 20)
	commitment1[0] = 0xAA
	commitment2 := make([]byte, 20)
	commitment2[0] = 0xBB

	nameNewScript1 := make([]byte, 0, 50)
	nameNewScript1 = append(nameNewScript1, 0xd0)
	nameNewScript1 = append(nameNewScript1, byte(len(commitment1)))
	nameNewScript1 = append(nameNewScript1, commitment1...)
	nameNewScript1 = append(nameNewScript1, 0x6d)
	nameNewScript1 = append(nameNewScript1, 0x76, 0xa9, 0x14)

	nameNewScript2 := make([]byte, 0, 50)
	nameNewScript2 = append(nameNewScript2, 0xd0)
	nameNewScript2 = append(nameNewScript2, byte(len(commitment2)))
	nameNewScript2 = append(nameNewScript2, commitment2...)
	nameNewScript2 = append(nameNewScript2, 0x6d)
	nameNewScript2 = append(nameNewScript2, 0x76, 0xa9, 0x14)

	tx.AddTxOut(&wire.TxOut{
		Value:    config.MinNameFee,
		PkScript: nameNewScript1,
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    config.MinNameFee,
		PkScript: nameNewScript2,
	})

	err := bc.ValidateMempoolTransaction(tx)
	if err == nil {
		t.Error("Expected error for transaction with multiple name operations")
	}
}

// TestValidateMempoolTransactionDustOutput tests name operation with dust output
func TestValidateMempoolTransactionDustOutput(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{0x01, 0x02, 0x03},
			Index: 0,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})

	// Create NAME_NEW script with dust value
	commitment := make([]byte, 20)
	commitment[0] = 0xCC

	nameNewScript := make([]byte, 0, 50)
	nameNewScript = append(nameNewScript, 0xd0)
	nameNewScript = append(nameNewScript, byte(len(commitment)))
	nameNewScript = append(nameNewScript, commitment...)
	nameNewScript = append(nameNewScript, 0x6d)
	nameNewScript = append(nameNewScript, 0x76, 0xa9, 0x14)

	tx.AddTxOut(&wire.TxOut{
		Value:    100, // Below dust limit (546)
		PkScript: nameNewScript,
	})

	err := bc.ValidateMempoolTransaction(tx)
	if err == nil {
		t.Error("Expected error for name operation with dust output")
	}
}

// TestValidateMempoolTransactionNameFirstUpdateExistingName tests NAME_FIRSTUPDATE for existing name
func TestValidateMempoolTransactionNameFirstUpdateExistingName(t *testing.T) {
	bc, ndb, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// Add an existing (non-expired) name
	existingName := &namedb.NameRecord{
		Name:      "d/existing",
		Value:     `{"ip":"1.1.1.1"}`,
		Height:    1,
		ExpiresAt: 100000, // Not expired
		Address:   "N1234567890",
	}
	if err := ndb.PutName(existingName.Name, existingName); err != nil {
		t.Fatalf("Failed to add existing name: %v", err)
	}

	// Also add a NAME_NEW for this name
	commitment := make([]byte, 20)
	commitment[0] = 0xDD
	if err := ndb.PutNameNew(commitment, 1, chainhash.Hash{0x01}, 0); err != nil {
		t.Fatalf("Failed to add NAME_NEW: %v", err)
	}

	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{0x01, 0x02, 0x03},
			Index: 0,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})

	// Create NAME_FIRSTUPDATE script for the existing name
	name := "d/existing"
	value := `{"ip":"2.2.2.2"}`
	salt := commitment // Use the same commitment we registered

	nameFirstUpdateScript := make([]byte, 0, 100)
	nameFirstUpdateScript = append(nameFirstUpdateScript, 0xd1)
	nameFirstUpdateScript = append(nameFirstUpdateScript, byte(len(name)))
	nameFirstUpdateScript = append(nameFirstUpdateScript, []byte(name)...)
	nameFirstUpdateScript = append(nameFirstUpdateScript, byte(len(salt)))
	nameFirstUpdateScript = append(nameFirstUpdateScript, salt...)
	nameFirstUpdateScript = append(nameFirstUpdateScript, byte(len(value)))
	nameFirstUpdateScript = append(nameFirstUpdateScript, []byte(value)...)
	nameFirstUpdateScript = append(nameFirstUpdateScript, 0x6d, 0x6d, 0x6d)
	nameFirstUpdateScript = append(nameFirstUpdateScript, 0x76, 0xa9, 0x14)

	tx.AddTxOut(&wire.TxOut{
		Value:    config.MinNameFee,
		PkScript: nameFirstUpdateScript,
	})

	err := bc.ValidateMempoolTransaction(tx)
	if err == nil {
		t.Error("Expected error for NAME_FIRSTUPDATE of existing name")
	}
}

// setupTestBlockChainWithPath creates a test blockchain with specific paths
func setupTestBlockChainWithPath(t *testing.T, dataDir string) (*BlockChain, *namedb.NameDatabase, func()) {
	cfg := &Config{
		ChainParams: &config.NamecoinRegTestParams,
		DataDir:     dataDir,
		BlockDBPath: filepath.Join(dataDir, "blocks.db"),
		NameDBPath:  filepath.Join(dataDir, "names.db"),
	}

	bc, err := NewBlockChain(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create blockchain: %v", err)
	}

	cleanup := func() {
		bc.Close()
	}

	return bc, bc.nameDB, cleanup
}
