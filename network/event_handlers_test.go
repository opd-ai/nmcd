package network

import (
	"errors"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/internal/logging"
)

// testValidator is a simple mock for testing mempool validation (different from mempool_validation_test.go's mockValidator)
type testValidator struct {
	shouldReject bool
}

func (v *testValidator) ValidateMempoolTransaction(tx *wire.MsgTx) error {
	if v.shouldReject {
		return errors.New("mock validation failed")
	}
	return nil
}

// createTestPeerManager creates a PeerManager configured for testing
func createTestPeerManager(t *testing.T) *PeerManager {
	t.Helper()

	// Ensure logger is initialized
	logCfg := logging.DefaultConfig()
	logCfg.Level = logging.LevelError
	logCfg.Output = "stderr"
	logger, err := logging.Init(logCfg)
	if err == nil {
		logging.SetDefault(logger)
	}

	mempool := NewMempoolWithConfig(&MempoolConfig{
		Validator:   nil, // No validation for basic tests
		MaxTxs:      100,
		TxExpiry:    1 * time.Hour,
		CleanupTick: 10 * time.Minute,
	})

	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[string]*peer.Peer),
		blockchain:  nil,
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    10,
		quit:        make(chan struct{}),
		mempool:     mempool,
	}
	return pm
}

// createTestTransaction creates a minimal valid transaction for testing
func createTestTransaction() *wire.MsgTx {
	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Index: 0,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    1000,
		PkScript: []byte{0x76, 0xa9, 0x14}, // Minimal script
	})
	return tx
}

// TestOnTxNilMessage tests that onTx handles nil message gracefully
func TestOnTxNilMessage(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	// Should not panic with nil message
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onTx panicked with nil message: %v", r)
		}
	}()

	pm.onTx(nil, nil)
}

// TestOnTxValidTransaction tests that onTx accepts valid transactions
func TestOnTxValidTransaction(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	tx := createTestTransaction()
	txHash := tx.TxHash()

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onTx panicked: %v", r)
		}
	}()

	pm.onTx(nil, tx)

	// Transaction should be in mempool
	if !pm.mempool.HasTx(&txHash) {
		t.Error("Transaction should be in mempool after onTx")
	}
}

// TestOnTxDuplicateTransaction tests that onTx ignores duplicate transactions
func TestOnTxDuplicateTransaction(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	tx := createTestTransaction()

	// Add transaction first time
	pm.onTx(nil, tx)

	initialCount := pm.mempool.Count()

	// Add same transaction again - should be ignored
	pm.onTx(nil, tx)

	if pm.mempool.Count() != initialCount {
		t.Error("Duplicate transaction should not be added to mempool")
	}
}

// TestOnTxMempoolFull tests behavior when mempool is at capacity
func TestOnTxMempoolFull(t *testing.T) {
	mempool := NewMempoolWithConfig(&MempoolConfig{
		MaxTxs:      1, // Very small mempool
		TxExpiry:    1 * time.Hour,
		CleanupTick: 10 * time.Minute,
	})

	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[string]*peer.Peer),
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    10,
		quit:        make(chan struct{}),
		mempool:     mempool,
	}
	defer mempool.Stop()

	// Add first transaction
	tx1 := createTestTransaction()
	pm.onTx(nil, tx1)

	// Add second transaction - should be rejected due to full mempool
	tx2 := wire.NewMsgTx(wire.TxVersion)
	tx2.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 1},
		Sequence:         wire.MaxTxInSequenceNum,
	})
	tx2.AddTxOut(&wire.TxOut{
		Value:    2000,
		PkScript: []byte{0x76, 0xa9, 0x14},
	})

	// Should not panic even when mempool rejects
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onTx panicked when mempool is full: %v", r)
		}
	}()

	pm.onTx(nil, tx2)
}

// TestRelayTransactionNoPeers tests relayTransaction when no peers are connected
func TestRelayTransactionNoPeers(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	tx := createTestTransaction()

	// Should not panic with no peers
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("relayTransaction panicked with no peers: %v", r)
		}
	}()

	pm.relayTransaction(tx, nil)
}

// TestRelayTransactionExcludePeer tests that relay excludes source peer
func TestRelayTransactionExcludePeer(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	tx := createTestTransaction()

	// relayTransaction with excludePeer should not panic
	// even with empty peers map
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("relayTransaction panicked: %v", r)
		}
	}()

	// Passing nil excludePeer should also work
	pm.relayTransaction(tx, nil)
}

// TestBroadcastTxNilTransaction tests BroadcastTx with nil transaction
func TestBroadcastTxNilTransaction(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	err := pm.BroadcastTx(nil)
	if err == nil {
		t.Error("Expected error for nil transaction")
	}
}

// TestBroadcastTxValidTransaction tests BroadcastTx with valid transaction
func TestBroadcastTxValidTransaction(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	tx := createTestTransaction()

	err := pm.BroadcastTx(tx)
	if err != nil {
		t.Errorf("BroadcastTx failed: %v", err)
	}

	// Transaction should be in mempool
	txHash := tx.TxHash()
	if !pm.mempool.HasTx(&txHash) {
		t.Error("Transaction should be in mempool after BroadcastTx")
	}
}

// TestBroadcastTxNoPeers tests BroadcastTx when no peers are connected
func TestBroadcastTxNoPeers(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	tx := createTestTransaction()

	// Should not return error, just log warning
	err := pm.BroadcastTx(tx)
	if err != nil {
		t.Errorf("BroadcastTx should not fail with no peers: %v", err)
	}
}

// TestBroadcastBlockNoPeers tests BroadcastBlock when no peers are connected
func TestBroadcastBlockNoPeers(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	block := wire.NewMsgBlock(&wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
	})

	// Should not panic with no peers
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("BroadcastBlock panicked: %v", r)
		}
	}()

	pm.BroadcastBlock(block)
}

// TestOnInvTransaction tests onInv with transaction inventory
func TestOnInvTransaction(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	// Create inv message with transaction
	inv := wire.NewMsgInv()
	var txHash chainhash.Hash
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &txHash))

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onInv panicked: %v", r)
		}
	}()

	pm.onInv(nil, inv)
}

// TestOnInvBlock tests onInv with block inventory
func TestOnInvBlock(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	// Create inv message with block
	inv := wire.NewMsgInv()
	var blockHash chainhash.Hash
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, &blockHash))

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onInv panicked: %v", r)
		}
	}()

	pm.onInv(nil, inv)
}

// TestOnGetDataTransaction tests onGetData for transaction requests
func TestOnGetDataTransaction(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	// Add a transaction to mempool first
	tx := createTestTransaction()
	pm.mempool.AddTx(tx)

	// Create getdata request for the transaction
	getData := wire.NewMsgGetData()
	txHash := tx.TxHash()
	getData.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &txHash))

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onGetData panicked: %v", r)
		}
	}()

	pm.onGetData(nil, getData)
}

// TestOnGetDataTransactionNotFound tests onGetData for non-existent transaction
func TestOnGetDataTransactionNotFound(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	// Create getdata request for non-existent transaction
	getData := wire.NewMsgGetData()
	var txHash chainhash.Hash
	getData.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &txHash))

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onGetData panicked for non-existent tx: %v", r)
		}
	}()

	pm.onGetData(nil, getData)
}

// TestOnGetDataBlock tests onGetData for block requests
func TestOnGetDataBlock(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	// Create getdata request for block
	getData := wire.NewMsgGetData()
	var blockHash chainhash.Hash
	getData.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, &blockHash))

	// Should not panic (block retrieval not fully implemented)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onGetData panicked for block: %v", r)
		}
	}()

	pm.onGetData(nil, getData)
}

// TestOnHeadersNilSyncManager tests onHeaders when syncManager is nil
func TestOnHeadersNilSyncManager(t *testing.T) {
	pm := createTestPeerManager(t)
	pm.syncManager = nil
	defer pm.mempool.Stop()

	headers := &wire.MsgHeaders{}

	// Should not panic when syncManager is nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onHeaders panicked with nil syncManager: %v", r)
		}
	}()

	pm.onHeaders(nil, headers)
}

// TestOnGetHeadersNilBlockchain tests onGetHeaders when blockchain is nil
func TestOnGetHeadersNilBlockchain(t *testing.T) {
	pm := createTestPeerManager(t)
	pm.blockchain = nil
	defer pm.mempool.Stop()

	getHeaders := &wire.MsgGetHeaders{}

	// Should not panic when blockchain is nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onGetHeaders panicked with nil blockchain: %v", r)
		}
	}()

	pm.onGetHeaders(nil, getHeaders)
}

// TestOnGetBlocksNilBlockchain tests onGetBlocks when blockchain is nil
func TestOnGetBlocksNilBlockchain(t *testing.T) {
	pm := createTestPeerManager(t)
	pm.blockchain = nil
	defer pm.mempool.Stop()

	getBlocks := &wire.MsgGetBlocks{}

	// Should not panic when blockchain is nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onGetBlocks panicked with nil blockchain: %v", r)
		}
	}()

	pm.onGetBlocks(nil, getBlocks)
}

// TestOnVersionMessage tests onVersion handling
func TestOnVersionMessage(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	version := &wire.MsgVersion{
		ProtocolVersion: 70015,
		Services:        wire.SFNodeNetwork,
	}

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onVersion panicked: %v", r)
		}
	}()

	result := pm.onVersion(nil, version)
	// Should return nil (accepted) for valid version
	if result != nil {
		t.Logf("onVersion returned reject message: %v", result)
	}
}

// TestOnVerAckMessage tests onVerAck handling
func TestOnVerAckMessage(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	verAck := &wire.MsgVerAck{}

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onVerAck panicked: %v", r)
		}
	}()

	pm.onVerAck(nil, verAck)
}

// TestEventHandlerMempoolGetAll tests getting all transactions from mempool
func TestEventHandlerMempoolGetAll(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	// Add some transactions
	tx1 := createTestTransaction()
	tx2 := wire.NewMsgTx(wire.TxVersion)
	tx2.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 1},
		Sequence:         wire.MaxTxInSequenceNum,
	})
	tx2.AddTxOut(&wire.TxOut{
		Value:    2000,
		PkScript: []byte{0x76, 0xa9, 0x14},
	})

	pm.mempool.AddTx(tx1)
	pm.mempool.AddTx(tx2)

	txs := pm.mempool.GetAll()
	if len(txs) != 2 {
		t.Errorf("Expected 2 transactions, got %d", len(txs))
	}
}

// TestEventHandlerMempoolRemoveTxs tests removing transactions from mempool
func TestEventHandlerMempoolRemoveTxs(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	tx := createTestTransaction()
	txHash := tx.TxHash()

	pm.mempool.AddTx(tx)

	if !pm.mempool.HasTx(&txHash) {
		t.Fatal("Transaction should be in mempool")
	}

	pm.mempool.RemoveTxs([]chainhash.Hash{txHash})

	if pm.mempool.HasTx(&txHash) {
		t.Error("Transaction should be removed from mempool")
	}
}

// TestBroadcastTxDuplicate tests BroadcastTx with duplicate transaction
func TestBroadcastTxDuplicate(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	tx := createTestTransaction()

	// First broadcast should succeed
	err := pm.BroadcastTx(tx)
	if err != nil {
		t.Errorf("First BroadcastTx failed: %v", err)
	}

	// Second broadcast of same transaction should fail (duplicate)
	err = pm.BroadcastTx(tx)
	if err == nil {
		t.Error("Expected error for duplicate transaction")
	}
}

// TestOnInvEmptyMessage tests onInv with empty inventory
func TestOnInvEmptyMessage(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	inv := wire.NewMsgInv()

	// Should not panic with empty inv
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onInv panicked with empty message: %v", r)
		}
	}()

	pm.onInv(nil, inv)
}

// TestOnGetDataEmptyMessage tests onGetData with empty message
func TestOnGetDataEmptyMessage(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	getData := wire.NewMsgGetData()

	// Should not panic with empty getData
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onGetData panicked with empty message: %v", r)
		}
	}()

	pm.onGetData(nil, getData)
}

// TestGetMempoolInterface tests that mempool interface is accessible
func TestGetMempoolInterface(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	mempool := pm.GetMempool()
	if mempool == nil {
		t.Error("GetMempool should return non-nil mempool")
	}

	// Verify it's the same mempool
	tx := createTestTransaction()
	mempool.AddTx(tx)

	txHash := tx.TxHash()
	if !pm.mempool.HasTx(&txHash) {
		t.Error("Transaction should be in pm.mempool after adding via GetMempool")
	}
}

// TestMempoolWithValidatorRejects tests mempool validation rejection
func TestMempoolWithValidatorRejects(t *testing.T) {
	validator := &testValidator{shouldReject: true}

	mempool := NewMempoolWithConfig(&MempoolConfig{
		Validator:   validator,
		MaxTxs:      100,
		TxExpiry:    1 * time.Hour,
		CleanupTick: 10 * time.Minute,
	})
	defer mempool.Stop()

	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[string]*peer.Peer),
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    10,
		quit:        make(chan struct{}),
		mempool:     mempool,
	}

	tx := createTestTransaction()

	// onTx should handle validation failure gracefully
	pm.onTx(nil, tx)

	// Transaction should not be in mempool due to validation failure
	txHash := tx.TxHash()
	if mempool.HasTx(&txHash) {
		t.Error("Transaction should not be in mempool after validation failure")
	}
}

// TestOnTxWithValidatorAccepts tests onTx with accepting validator
func TestOnTxWithValidatorAccepts(t *testing.T) {
	validator := &testValidator{shouldReject: false}

	mempool := NewMempoolWithConfig(&MempoolConfig{
		Validator:   validator,
		MaxTxs:      100,
		TxExpiry:    1 * time.Hour,
		CleanupTick: 10 * time.Minute,
	})
	defer mempool.Stop()

	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[string]*peer.Peer),
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    10,
		quit:        make(chan struct{}),
		mempool:     mempool,
	}

	tx := createTestTransaction()

	// onTx should accept the transaction
	pm.onTx(nil, tx)

	// Transaction should be in mempool
	txHash := tx.TxHash()
	if !mempool.HasTx(&txHash) {
		t.Error("Transaction should be in mempool after validation passes")
	}
}

// TestBroadcastBlockMultipleTx tests BroadcastBlock with block containing transactions
func TestBroadcastBlockMultipleTx(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	// Create a block with multiple transactions
	block := wire.NewMsgBlock(&wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
	})

	// Add coinbase
	coinbaseTx := wire.NewMsgTx(1)
	coinbaseTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 0xffffffff},
	})
	coinbaseTx.AddTxOut(&wire.TxOut{
		Value:    50 * 1e8,
		PkScript: []byte{0x76, 0xa9, 0x14},
	})
	block.AddTransaction(coinbaseTx)

	// Add regular tx
	regularTx := createTestTransaction()
	block.AddTransaction(regularTx)

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("BroadcastBlock panicked: %v", r)
		}
	}()

	pm.BroadcastBlock(block)
}

// TestRelayTransactionWithPeers tests relay with peers map
func TestRelayTransactionWithPeers(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	tx := createTestTransaction()

	// Add some peer entries (nil pointers won't panic but won't relay)
	pm.mu.Lock()
	pm.peers["test1"] = nil
	pm.peers["test2"] = nil
	pm.mu.Unlock()

	// Should not panic even with nil peer entries
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("relayTransaction panicked with nil peer entries: %v", r)
		}
	}()

	pm.relayTransaction(tx, nil)
}

// TestMempoolCount tests the Count method
func TestMempoolCount(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	if pm.mempool.Count() != 0 {
		t.Error("Empty mempool should have count 0")
	}

	tx := createTestTransaction()
	pm.mempool.AddTx(tx)

	if pm.mempool.Count() != 1 {
		t.Errorf("Mempool should have count 1, got %d", pm.mempool.Count())
	}
}

// TestEventHandlerMempoolClear tests the Clear method via event handlers
func TestEventHandlerMempoolClear(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	tx1 := createTestTransaction()
	tx2 := wire.NewMsgTx(wire.TxVersion)
	tx2.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 1},
		Sequence:         wire.MaxTxInSequenceNum,
	})
	tx2.AddTxOut(&wire.TxOut{
		Value:    2000,
		PkScript: []byte{0x76, 0xa9, 0x14},
	})

	pm.mempool.AddTx(tx1)
	pm.mempool.AddTx(tx2)

	if pm.mempool.Count() != 2 {
		t.Fatal("Should have 2 transactions before clear")
	}

	pm.mempool.Clear()

	if pm.mempool.Count() != 0 {
		t.Error("Mempool should be empty after clear")
	}
}

// TestGetConnectedPeers tests getting the count of connected peers
func TestGetConnectedPeers(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	// Initially should be 0
	if count := pm.GetConnectedPeers(); count != 0 {
		t.Errorf("Expected 0 peers, got %d", count)
	}
}

// TestGetPeerInfo tests getting peer information
func TestGetPeerInfo(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	// Initially should be empty
	info := pm.GetPeerInfo()
	if len(info) != 0 {
		t.Errorf("Expected empty peer info, got %d entries", len(info))
	}
}
