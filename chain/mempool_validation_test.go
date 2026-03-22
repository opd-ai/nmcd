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

// TestValidateMempoolTransactionMultipleOutputs tests transaction with multiple outputs
func TestValidateMempoolTransactionMultipleOutputs(t *testing.T) {
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
	// Add multiple regular outputs
	tx.AddTxOut(&wire.TxOut{
		Value:    1000,
		PkScript: []byte{0x76, 0xa9, 0x14, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x88, 0xac},
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    2000,
		PkScript: []byte{0x76, 0xa9, 0x14, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x88, 0xac},
	})

	err := bc.ValidateMempoolTransaction(tx)
	if err != nil {
		t.Errorf("Unexpected error for valid transaction with multiple outputs: %v", err)
	}
}

// TestValidateMempoolTransactionWithEmptyScript tests transaction with empty output script
func TestValidateMempoolTransactionWithEmptyScript(t *testing.T) {
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
	// Empty script is technically valid for basic validation
	tx.AddTxOut(&wire.TxOut{
		Value:    1000,
		PkScript: []byte{},
	})

	// Should pass basic validation (empty script is skipped by name parser)
	err := bc.ValidateMempoolTransaction(tx)
	if err != nil {
		t.Errorf("Unexpected error for transaction with empty script: %v", err)
	}
}

// TestValidateMempoolTransactionConcurrency tests thread safety
func TestValidateMempoolTransactionConcurrency(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// Create a valid transaction
	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{0x01, 0x02, 0x03},
			Index: 0,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    1000,
		PkScript: []byte{0x76, 0xa9, 0x14},
	})

	// Run validation concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			err := bc.ValidateMempoolTransaction(tx)
			if err != nil {
				t.Errorf("Unexpected error in concurrent validation: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestValidateMempoolTransactionLargeOutput tests transaction with large output value
func TestValidateMempoolTransactionLargeOutput(t *testing.T) {
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
	// Very large output value (21 million NMC equivalent)
	tx.AddTxOut(&wire.TxOut{
		Value:    2100000000000000, // 21 million * 100 million satoshis
		PkScript: []byte{0x76, 0xa9, 0x14},
	})

	// Basic validation doesn't check total supply, so this should pass
	err := bc.ValidateMempoolTransaction(tx)
	if err != nil {
		t.Errorf("Unexpected error for transaction with large output: %v", err)
	}
}

// TestValidateMempoolTransactionMultipleInputs tests transaction with multiple inputs
func TestValidateMempoolTransactionMultipleInputs(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	tx := wire.NewMsgTx(wire.TxVersion)
	// Add multiple inputs
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{0x01, 0x02, 0x03},
			Index: 0,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{0x04, 0x05, 0x06},
			Index: 1,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    1000,
		PkScript: []byte{0x76, 0xa9, 0x14},
	})

	err := bc.ValidateMempoolTransaction(tx)
	if err != nil {
		t.Errorf("Unexpected error for transaction with multiple inputs: %v", err)
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
