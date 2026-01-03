package network

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
)

// TestNewMempool tests mempool creation
func TestNewMempool(t *testing.T) {
	mp := NewMempool()
	if mp == nil {
		t.Fatal("NewMempool returned nil")
	}
	if mp.Count() != 0 {
		t.Errorf("Expected empty mempool, got count %d", mp.Count())
	}
}

// TestMempoolAddTx tests adding transactions to the mempool
func TestMempoolAddTx(t *testing.T) {
	mp := NewMempool()

	// Create a test transaction
	tx := wire.NewMsgTx(1)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0,
		},
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    100,
		PkScript: []byte{0x76, 0xa9},
	})

	// Add transaction
	err := mp.AddTx(tx)
	if err != nil {
		t.Fatalf("Failed to add transaction: %v", err)
	}

	// Verify count
	if mp.Count() != 1 {
		t.Errorf("Expected count 1, got %d", mp.Count())
	}

	// Verify we can retrieve it
	txHash := tx.TxHash()
	retrieved, exists := mp.GetTx(&txHash)
	if !exists {
		t.Error("Transaction not found in mempool")
	}
	if retrieved.TxHash() != txHash {
		t.Error("Retrieved transaction hash mismatch")
	}
}

// TestMempoolAddNilTx tests adding nil transaction
func TestMempoolAddNilTx(t *testing.T) {
	mp := NewMempool()
	err := mp.AddTx(nil)
	if err == nil {
		t.Error("AddTx(nil) should return an error")
	}
	if mp.Count() != 0 {
		t.Errorf("Expected count 0 after adding nil, got %d", mp.Count())
	}
}

// TestMempoolRemoveTx tests removing transactions
func TestMempoolRemoveTx(t *testing.T) {
	mp := NewMempool()

	// Add a transaction
	tx := wire.NewMsgTx(1)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0,
		},
	})
	mp.AddTx(tx)

	txHash := tx.TxHash()

	// Verify it exists
	if mp.Count() != 1 {
		t.Fatalf("Expected count 1, got %d", mp.Count())
	}

	// Remove it
	mp.RemoveTx(&txHash)

	// Verify it's gone
	if mp.Count() != 0 {
		t.Errorf("Expected count 0 after removal, got %d", mp.Count())
	}

	_, exists := mp.GetTx(&txHash)
	if exists {
		t.Error("Transaction still exists after removal")
	}
}

// TestMempoolGetAll tests retrieving all transactions
func TestMempoolGetAll(t *testing.T) {
	mp := NewMempool()

	// Add multiple transactions
	for i := 0; i < 3; i++ {
		tx := wire.NewMsgTx(1)
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: uint32(i),
			},
		})
		mp.AddTx(tx)
	}

	// Get all transactions
	all := mp.GetAll()
	if len(all) != 3 {
		t.Errorf("Expected 3 transactions, got %d", len(all))
	}
}

// TestMempoolClear tests clearing the mempool
func TestMempoolClear(t *testing.T) {
	mp := NewMempool()

	// Add some transactions
	for i := 0; i < 5; i++ {
		tx := wire.NewMsgTx(1)
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: uint32(i),
			},
		})
		mp.AddTx(tx)
	}

	if mp.Count() != 5 {
		t.Fatalf("Expected count 5, got %d", mp.Count())
	}

	// Clear the mempool
	mp.Clear()

	if mp.Count() != 0 {
		t.Errorf("Expected count 0 after clear, got %d", mp.Count())
	}
}

// TestMempoolConcurrency tests concurrent access to mempool
func TestMempoolConcurrency(t *testing.T) {
	mp := NewMempool()

	// Concurrently add transactions
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(index int) {
			tx := wire.NewMsgTx(1)
			tx.AddTxIn(&wire.TxIn{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: uint32(index),
				},
			})
			mp.AddTx(tx)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify count
	if mp.Count() != 10 {
		t.Errorf("Expected count 10, got %d", mp.Count())
	}
}

// TestSyncBlocks tests that SyncBlocks sends getheaders requests
func TestSyncBlocks(t *testing.T) {
	// Create a PeerManager with no blockchain (should handle gracefully)
	pm := &PeerManager{
		peers:      make(map[string]*peer.Peer),
		blockchain: nil,
		mempool:    NewMempool(),
		quit:       make(chan struct{}),
	}

	// Should not panic when blockchain is nil
	pm.SyncBlocks()

	// In a real test with a blockchain, we would verify that getheaders
	// messages are sent to peers. For this minimal test, we just ensure
	// it doesn't crash.
}

// TestGetMempoolMethod tests the GetMempool accessor
func TestGetMempoolMethod(t *testing.T) {
	pm := &PeerManager{
		peers:   make(map[string]*peer.Peer),
		mempool: NewMempool(),
		quit:    make(chan struct{}),
	}

	mp := pm.GetMempool()
	if mp == nil {
		t.Fatal("GetMempool returned nil")
	}

	// Add a transaction and verify it's accessible
	tx := wire.NewMsgTx(1)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0,
		},
	})
	mp.AddTx(tx)

	if mp.Count() != 1 {
		t.Errorf("Expected mempool count 1, got %d", mp.Count())
	}
}
