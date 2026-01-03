package network

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// mockValidator is a simple test validator
type mockValidator struct {
	shouldAccept bool
	errorMsg     string
}

func (m *mockValidator) ValidateMempoolTransaction(tx *wire.MsgTx) error {
	if !m.shouldAccept {
		return &validationError{msg: m.errorMsg}
	}
	return nil
}

type validationError struct {
	msg string
}

func (e *validationError) Error() string {
	return e.msg
}

// TestMempoolWithValidator tests mempool with transaction validation
func TestMempoolWithValidator(t *testing.T) {
	// Create accepting validator
	acceptValidator := &mockValidator{
		shouldAccept: true,
	}

	cfg := &MempoolConfig{
		Validator:   acceptValidator,
		MaxTxs:      10,
		TxExpiry:    1 * time.Hour,
		CleanupTick: 10 * time.Minute,
	}

	mp := NewMempoolWithConfig(cfg)
	defer mp.Stop()

	// Create test transaction
	tx := wire.NewMsgTx(1)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{1, 2, 3},
			Index: 0,
		},
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    1000,
		PkScript: []byte{0x76, 0xa9},
	})

	// Should accept valid transaction
	err := mp.AddTx(tx)
	if err != nil {
		t.Fatalf("Failed to add valid transaction: %v", err)
	}

	if mp.Count() != 1 {
		t.Errorf("Expected 1 transaction, got %d", mp.Count())
	}
}

// TestMempoolRejectsInvalidTransaction tests rejection of invalid transactions
func TestMempoolRejectsInvalidTransaction(t *testing.T) {
	// Create rejecting validator
	rejectValidator := &mockValidator{
		shouldAccept: false,
		errorMsg:     "invalid transaction",
	}

	cfg := &MempoolConfig{
		Validator:   rejectValidator,
		MaxTxs:      10,
		TxExpiry:    1 * time.Hour,
		CleanupTick: 10 * time.Minute,
	}

	mp := NewMempoolWithConfig(cfg)
	defer mp.Stop()

	// Create test transaction
	tx := wire.NewMsgTx(1)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{1, 2, 3},
			Index: 0,
		},
	})

	// Should reject invalid transaction
	err := mp.AddTx(tx)
	if err == nil {
		t.Fatal("Expected error for invalid transaction, got nil")
	}

	if mp.Count() != 0 {
		t.Errorf("Expected 0 transactions after rejection, got %d", mp.Count())
	}
}

// TestMempoolCapacityLimit tests mempool capacity enforcement
func TestMempoolCapacityLimit(t *testing.T) {
	cfg := &MempoolConfig{
		Validator:   nil, // No validation for this test
		MaxTxs:      5,   // Small limit for testing
		TxExpiry:    1 * time.Hour,
		CleanupTick: 10 * time.Minute,
	}

	mp := NewMempoolWithConfig(cfg)
	defer mp.Stop()

	// Fill mempool to capacity
	for i := 0; i < 5; i++ {
		tx := wire.NewMsgTx(1)
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{byte(i)},
				Index: 0,
			},
		})
		tx.AddTxOut(&wire.TxOut{Value: 100})

		if err := mp.AddTx(tx); err != nil {
			t.Fatalf("Failed to add transaction %d: %v", i, err)
		}
	}

	// Verify mempool is full
	if mp.Count() != 5 {
		t.Errorf("Expected 5 transactions, got %d", mp.Count())
	}

	// Try to add one more - should be rejected
	tx := wire.NewMsgTx(1)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{99},
			Index: 0,
		},
	})
	tx.AddTxOut(&wire.TxOut{Value: 100})

	err := mp.AddTx(tx)
	if err == nil {
		t.Error("Expected error when exceeding mempool capacity")
	}

	if mp.Count() != 5 {
		t.Errorf("Expected mempool to stay at 5 transactions, got %d", mp.Count())
	}
}

// TestMempoolDuplicateTransaction tests handling of duplicate transactions
func TestMempoolDuplicateTransaction(t *testing.T) {
	mp := NewMempool()
	defer mp.Stop()

	// Create test transaction
	tx := wire.NewMsgTx(1)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{1, 2, 3},
			Index: 0,
		},
	})
	tx.AddTxOut(&wire.TxOut{Value: 1000})

	// Add first time
	if err := mp.AddTx(tx); err != nil {
		t.Fatalf("Failed to add transaction: %v", err)
	}

	if mp.Count() != 1 {
		t.Errorf("Expected 1 transaction, got %d", mp.Count())
	}

	// Add same transaction again - should not error, just update timestamp
	if err := mp.AddTx(tx); err != nil {
		t.Errorf("Adding duplicate transaction returned error: %v", err)
	}

	// Count should still be 1
	if mp.Count() != 1 {
		t.Errorf("Expected 1 transaction after duplicate, got %d", mp.Count())
	}
}

// TestMempoolHasTx tests the HasTx method
func TestMempoolHasTx(t *testing.T) {
	mp := NewMempool()
	defer mp.Stop()

	// Create test transaction
	tx := wire.NewMsgTx(1)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{1, 2, 3},
			Index: 0,
		},
	})

	txHash := tx.TxHash()

	// Should not have transaction initially
	if mp.HasTx(&txHash) {
		t.Error("Mempool reports having transaction before it's added")
	}

	// Add transaction
	mp.AddTx(tx)

	// Should now have transaction
	if !mp.HasTx(&txHash) {
		t.Error("Mempool reports not having transaction after it's added")
	}

	// Remove transaction
	mp.RemoveTx(&txHash)

	// Should not have transaction anymore
	if mp.HasTx(&txHash) {
		t.Error("Mempool reports having transaction after it's removed")
	}
}

// TestMempoolRemoveTxs tests batch removal of transactions
func TestMempoolRemoveTxs(t *testing.T) {
	mp := NewMempool()
	defer mp.Stop()

	// Add multiple transactions
	var txHashes []chainhash.Hash
	for i := 0; i < 5; i++ {
		tx := wire.NewMsgTx(1)
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{byte(i)},
				Index: 0,
			},
		})
		mp.AddTx(tx)
		txHashes = append(txHashes, tx.TxHash())
	}

	if mp.Count() != 5 {
		t.Fatalf("Expected 5 transactions, got %d", mp.Count())
	}

	// Remove first 3 transactions
	mp.RemoveTxs(txHashes[:3])

	if mp.Count() != 2 {
		t.Errorf("Expected 2 transactions after batch removal, got %d", mp.Count())
	}

	// Verify correct transactions were removed
	for i, hash := range txHashes {
		if i < 3 {
			if mp.HasTx(&hash) {
				t.Errorf("Transaction %d should have been removed", i)
			}
		} else {
			if !mp.HasTx(&hash) {
				t.Errorf("Transaction %d should still exist", i)
			}
		}
	}
}

// TestMempoolExpiration tests transaction expiration
func TestMempoolExpiration(t *testing.T) {
	cfg := &MempoolConfig{
		Validator:   nil,
		MaxTxs:      100,
		TxExpiry:    100 * time.Millisecond, // Very short expiry for testing
		CleanupTick: 50 * time.Millisecond,  // Frequent cleanup for testing
	}

	mp := NewMempoolWithConfig(cfg)
	defer mp.Stop()

	// Add test transaction
	tx := wire.NewMsgTx(1)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{1},
			Index: 0,
		},
	})

	if err := mp.AddTx(tx); err != nil {
		t.Fatalf("Failed to add transaction: %v", err)
	}

	if mp.Count() != 1 {
		t.Fatalf("Expected 1 transaction, got %d", mp.Count())
	}

	// Wait for expiration and cleanup
	time.Sleep(200 * time.Millisecond)

	// Transaction should be expired and cleaned up
	if mp.Count() != 0 {
		t.Errorf("Expected 0 transactions after expiration, got %d", mp.Count())
	}
}
