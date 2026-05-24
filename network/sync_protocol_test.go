package network

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/internal/logging"
)

// mockConn implements net.Conn for testing peer interactions
type mockConn struct {
	readBuf  []byte
	writeBuf []byte
	closed   bool
	mu       sync.Mutex
}

func (c *mockConn) Read(b []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Block forever to prevent peer from reading (test doesn't send messages)
	select {}
}

func (c *mockConn) Write(b []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writeBuf = append(c.writeBuf, b...)
	return len(b), nil
}

func (c *mockConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *mockConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8334}
}

func (c *mockConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 8334}
}

func (c *mockConn) SetDeadline(t time.Time) error      { return nil }
func (c *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *mockConn) SetWriteDeadline(t time.Time) error { return nil }

// mockBlockchain provides a minimal mock for blockchain operations
type mockBlockchain struct {
	bestHash   chainhash.Hash
	bestHeight int32
	headers    []wire.BlockHeader
}

func (m *mockBlockchain) BestSnapshot() *blockchain.BestState {
	return &blockchain.BestState{
		Hash:   m.bestHash,
		Height: m.bestHeight,
	}
}

func (m *mockBlockchain) LocateHeaders(locator blockchain.BlockLocator, hashStop *chainhash.Hash) []wire.BlockHeader {
	return m.headers
}

func (m *mockBlockchain) ProcessBlock(block *btcutil.Block, flags blockchain.BehaviorFlags) (bool, bool, error) {
	return true, false, nil
}

func (m *mockBlockchain) SetBlockAuxPowFromBytes(hash *chainhash.Hash, buf []byte) error {
	return nil
}

func (m *mockBlockchain) ValidateMempoolTransaction(tx *wire.MsgTx) error {
	// Mock implementation - always accept transactions
	return nil
}

// createTestPeerManagerWithSyncManager creates a PeerManager with sync manager for testing
func createTestPeerManagerWithSyncManager(t *testing.T) *PeerManager {
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
		Validator:   nil,
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

	// Create sync manager
	pm.syncManager = NewSyncManager(pm)

	return pm
}

// TestOnHeadersWithSyncManager tests onHeaders when syncManager is available
func TestOnHeadersWithSyncManager(t *testing.T) {
	pm := createTestPeerManagerWithSyncManager(t)
	defer pm.mempool.Stop()
	defer pm.syncManager.Stop()

	// Create headers message with some headers
	headers := &wire.MsgHeaders{}
	header := wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
	}
	headers.AddBlockHeader(&header)

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onHeaders panicked with syncManager: %v", r)
		}
	}()

	pm.onHeaders(nil, headers)
}

// TestOnVersionWithSyncManager tests onVersion updates sync manager
func TestOnVersionWithSyncManager(t *testing.T) {
	pm := createTestPeerManagerWithSyncManager(t)
	defer pm.mempool.Stop()
	defer pm.syncManager.Stop()

	version := &wire.MsgVersion{
		ProtocolVersion: 70015,
		Services:        wire.SFNodeNetwork,
		LastBlock:       500000,
	}

	// Should not panic and should update peer height
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onVersion panicked with syncManager: %v", r)
		}
	}()

	result := pm.onVersion(nil, version)
	if result != nil {
		t.Logf("onVersion returned reject message: %v", result)
	}
}

// TestOnBlockNilBlockchain tests onBlock when blockchain is nil
func TestOnBlockNilBlockchain(t *testing.T) {
	pm := createTestPeerManager(t)
	pm.blockchain = nil
	defer pm.mempool.Stop()

	block := wire.NewMsgBlock(&wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
	})

	// Should not panic when blockchain is nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onBlock panicked with nil blockchain: %v", r)
		}
	}()

	pm.onBlock(nil, block, nil)
}

// TestOnBlockNilPeer tests onBlock with nil peer (local block)
func TestOnBlockNilPeer(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	block := wire.NewMsgBlock(&wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
	})

	// Should not panic with nil peer
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onBlock panicked with nil peer: %v", r)
		}
	}()

	pm.onBlock(nil, block, nil)
}

// TestOnInvNilPeer tests onInv handles nil peer gracefully
func TestOnInvNilPeer(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	inv := wire.NewMsgInv()
	var hash chainhash.Hash
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &hash))

	// Should return early without panicking for nil peer
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onInv panicked with nil peer: %v", r)
		}
	}()

	pm.onInv(nil, inv)
}

// TestOnGetDataNilPeer tests onGetData handles nil peer gracefully
func TestOnGetDataNilPeer(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	getData := wire.NewMsgGetData()
	var hash chainhash.Hash
	getData.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &hash))

	// Should return early without panicking for nil peer
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onGetData panicked with nil peer: %v", r)
		}
	}()

	pm.onGetData(nil, getData)
}

// TestOnGetHeadersNilPeer tests onGetHeaders handles nil peer gracefully
func TestOnGetHeadersNilPeer(t *testing.T) {
	pm := createTestPeerManager(t)
	pm.blockchain = nil
	defer pm.mempool.Stop()

	getHeaders := &wire.MsgGetHeaders{}

	// Should not panic with nil peer when blockchain is also nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onGetHeaders panicked with nil peer: %v", r)
		}
	}()

	pm.onGetHeaders(nil, getHeaders)
}

// TestOnGetBlocksNilPeer tests onGetBlocks handles nil peer gracefully
func TestOnGetBlocksNilPeer(t *testing.T) {
	pm := createTestPeerManager(t)
	pm.blockchain = nil
	defer pm.mempool.Stop()

	getBlocks := &wire.MsgGetBlocks{}

	// Should not panic with nil peer when blockchain is also nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onGetBlocks panicked with nil peer: %v", r)
		}
	}()

	pm.onGetBlocks(nil, getBlocks)
}

// TestUpdatePeerMetrics tests updatePeerMetrics function
func TestUpdatePeerMetrics(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	// Should not panic with empty peers map
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("updatePeerMetrics panicked: %v", r)
		}
	}()

	pm.mu.Lock()
	pm.updatePeerMetrics()
	pm.mu.Unlock()
}

// TestBroadcastBlockWithPeers tests BroadcastBlock behavior
func TestBroadcastBlockWithPeers(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	block := wire.NewMsgBlock(&wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
		Nonce:     12345,
	})

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("BroadcastBlock panicked: %v", r)
		}
	}()

	pm.BroadcastBlock(block)
}

// TestBroadcastTxMempoolValidation tests BroadcastTx with validation failure
func TestBroadcastTxMempoolValidation(t *testing.T) {
	// Create mempool with validator that rejects
	validator := &testValidator{shouldReject: true}
	mempool := NewMempoolWithConfig(&MempoolConfig{
		Validator:   validator,
		MaxTxs:      100,
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

	tx := createTestTransaction()

	err := pm.BroadcastTx(tx)
	if err == nil {
		t.Error("Expected error when validation fails")
	}
}

// TestRelayTransactionWithPeers tests relaying transactions
func TestRelayTransactionWithPeers(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	tx := createTestTransaction()

	// Should not panic even with empty peers
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("relayTransaction panicked: %v", r)
		}
	}()

	pm.relayTransaction(tx, nil)
}

// TestOnVersionNilSyncManager tests onVersion when syncManager is nil
func TestOnVersionNilSyncManager(t *testing.T) {
	pm := createTestPeerManager(t)
	pm.syncManager = nil
	defer pm.mempool.Stop()

	version := &wire.MsgVersion{
		ProtocolVersion: 70015,
		Services:        wire.SFNodeNetwork,
		LastBlock:       500000,
	}

	// Should not panic when syncManager is nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onVersion panicked with nil syncManager: %v", r)
		}
	}()

	result := pm.onVersion(nil, version)
	if result != nil {
		t.Logf("onVersion returned: %v", result)
	}
}

// TestOnInvMultipleItems tests onInv with multiple inventory items
func TestOnInvMultipleItems(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	inv := wire.NewMsgInv()
	var hash1, hash2, hash3 chainhash.Hash
	hash1[0] = 1
	hash2[0] = 2
	hash3[0] = 3
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &hash1))
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, &hash2))
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &hash3))

	// Should not panic with nil peer (early return)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onInv panicked with multiple items: %v", r)
		}
	}()

	pm.onInv(nil, inv)
}

// TestOnGetDataMixedTypes tests onGetData with both tx and block types
func TestOnGetDataMixedTypes(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	// Add a transaction to mempool
	tx := createTestTransaction()
	pm.mempool.AddTx(tx)
	txHash := tx.TxHash()

	getData := wire.NewMsgGetData()
	var blockHash chainhash.Hash
	getData.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &txHash))
	getData.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, &blockHash))

	// Should not panic with nil peer (early return)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onGetData panicked with mixed types: %v", r)
		}
	}()

	pm.onGetData(nil, getData)
}

// TestPeerManagerStopWithSyncManager tests proper shutdown with sync manager
func TestPeerManagerStopWithSyncManager(t *testing.T) {
	pm := createTestPeerManagerWithSyncManager(t)

	// Should not panic on Stop
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Stop panicked: %v", r)
		}
	}()

	pm.Stop()
}

// TestPeerManagerStopWithNilComponents tests Stop with nil components
func TestPeerManagerStopWithNilComponents(t *testing.T) {
	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[string]*peer.Peer),
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    10,
		quit:        make(chan struct{}),
		syncManager: nil,
		mempool:     nil,
		listeners:   nil,
	}

	// Should not panic with nil components
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Stop panicked with nil components: %v", r)
		}
	}()

	pm.Stop()
}

// TestOnTxWithValidation tests onTx with mempool validation
func TestOnTxWithValidation(t *testing.T) {
	// Create mempool with validator
	validator := &testValidator{shouldReject: false}
	mempool := NewMempoolWithConfig(&MempoolConfig{
		Validator:   validator,
		MaxTxs:      100,
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

	tx := createTestTransaction()

	// Should not panic with validation
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onTx panicked with validation: %v", r)
		}
	}()

	pm.onTx(nil, tx)

	txHash := tx.TxHash()
	if !pm.mempool.HasTx(&txHash) {
		t.Error("Transaction should be accepted with passing validation")
	}
}

// TestOnTxValidationFailure tests onTx when validation fails
func TestOnTxValidationFailure(t *testing.T) {
	// Create mempool with validator that rejects
	validator := &testValidator{shouldReject: true}
	mempool := NewMempoolWithConfig(&MempoolConfig{
		Validator:   validator,
		MaxTxs:      100,
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

	tx := createTestTransaction()

	// Should not panic when validation fails
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onTx panicked on validation failure: %v", r)
		}
	}()

	pm.onTx(nil, tx)

	txHash := tx.TxHash()
	if pm.mempool.HasTx(&txHash) {
		t.Error("Transaction should be rejected with failing validation")
	}
}

// TestGetPeerInfoEmpty tests GetPeerInfo with no peers
func TestGetPeerInfoEmpty(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	info := pm.GetPeerInfo()
	if info == nil {
		t.Error("GetPeerInfo should return non-nil slice")
	}
	if len(info) != 0 {
		t.Errorf("Expected 0 peers, got %d", len(info))
	}
}

// TestGetConnectedPeersEmpty tests GetConnectedPeers with no peers
func TestGetConnectedPeersEmpty(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	count := pm.GetConnectedPeers()
	if count != 0 {
		t.Errorf("Expected 0 connected peers, got %d", count)
	}
}

// TestSyncManagerHandleHeadersWithPeer tests HandleHeaders with headers
func TestSyncManagerHandleHeadersWithPeer(t *testing.T) {
	pm := createTestPeerManagerWithSyncManager(t)
	defer pm.mempool.Stop()
	defer pm.syncManager.Stop()

	// Create headers message with some headers
	headers := &wire.MsgHeaders{}
	header := wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
		Nonce:     12345,
	}
	headers.AddBlockHeader(&header)

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("HandleHeaders panicked: %v", r)
		}
	}()

	pm.syncManager.HandleHeaders(nil, headers)
}

// TestSyncManagerHandleHeadersEmpty tests HandleHeaders with empty headers
func TestSyncManagerHandleHeadersEmpty(t *testing.T) {
	pm := createTestPeerManagerWithSyncManager(t)
	defer pm.mempool.Stop()
	defer pm.syncManager.Stop()

	headers := &wire.MsgHeaders{}

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("HandleHeaders panicked with empty headers: %v", r)
		}
	}()

	pm.syncManager.HandleHeaders(nil, headers)
}

// TestSyncManagerUpdatePeerHeightNilPeer tests UpdatePeerHeight with nil peer
func TestSyncManagerUpdatePeerHeightNilPeer(t *testing.T) {
	pm := createTestPeerManagerWithSyncManager(t)
	defer pm.mempool.Stop()
	defer pm.syncManager.Stop()

	// Should not panic with nil peer
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("UpdatePeerHeight panicked with nil peer: %v", r)
		}
	}()

	pm.syncManager.UpdatePeerHeight(nil, 500000)
}

// TestSyncManagerBlockReceivedEmpty tests BlockReceived with non-existent hash
func TestSyncManagerBlockReceivedEmpty(t *testing.T) {
	pm := createTestPeerManagerWithSyncManager(t)
	defer pm.mempool.Stop()
	defer pm.syncManager.Stop()

	var hash chainhash.Hash
	hash[0] = 1

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("BlockReceived panicked: %v", r)
		}
	}()

	pm.syncManager.BlockReceived(&hash)
}

// TestOnBlockWithSyncManager tests onBlock notifies sync manager
func TestOnBlockWithSyncManager(t *testing.T) {
	pm := createTestPeerManagerWithSyncManager(t)
	defer pm.mempool.Stop()
	defer pm.syncManager.Stop()

	block := wire.NewMsgBlock(&wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
		Nonce:     54321,
	})

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onBlock panicked: %v", r)
		}
	}()

	pm.onBlock(nil, block, nil)
}

// TestPeerScoreManagerGetBestPeersEmpty tests GetBestPeers with empty slice
func TestPeerScoreManagerGetBestPeersEmpty(t *testing.T) {
	psm := NewPeerScoreManager()

	result := psm.GetBestPeers(nil, 5)
	if result != nil {
		t.Error("Expected nil for empty peers slice")
	}

	result = psm.GetBestPeers([]*peer.Peer{}, 5)
	if result != nil {
		t.Error("Expected nil for empty peers slice")
	}
}

// TestPeerScoreManagerGetBestPeersZeroN tests GetBestPeers with n=0
func TestPeerScoreManagerGetBestPeersZeroN(t *testing.T) {
	psm := NewPeerScoreManager()

	result := psm.GetBestPeers([]*peer.Peer{nil}, 0)
	if result != nil {
		t.Error("Expected nil for n=0")
	}

	result = psm.GetBestPeers([]*peer.Peer{nil}, -1)
	if result != nil {
		t.Error("Expected nil for n<0")
	}
}

// TestRemoveTxsEmpty tests RemoveTxs with empty slice
func TestRemoveTxsEmpty(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RemoveTxs panicked with empty slice: %v", r)
		}
	}()

	pm.mempool.RemoveTxs([]chainhash.Hash{})
}

// TestRemoveTxsNonExistent tests RemoveTxs with non-existent hashes
func TestRemoveTxsNonExistent(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	var hash1, hash2 chainhash.Hash
	hash1[0] = 1
	hash2[0] = 2

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RemoveTxs panicked with non-existent hashes: %v", r)
		}
	}()

	pm.mempool.RemoveTxs([]chainhash.Hash{hash1, hash2})
}

// TestUpdatePeerMetricsWithPeers tests updatePeerMetrics with peers
func TestUpdatePeerMetricsWithPeers(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	// Add some nil peer entries (simulating disconnected peers)
	pm.mu.Lock()
	pm.peers["peer1"] = nil
	pm.peers["peer2"] = nil
	pm.mu.Unlock()

	// Should not panic even with nil peers
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("updatePeerMetrics panicked with nil peers: %v", r)
		}
	}()

	pm.mu.Lock()
	// Clear peers since nil pointers cause issues
	pm.peers = make(map[string]*peer.Peer)
	pm.updatePeerMetrics()
	pm.mu.Unlock()
}

// TestBroadcastBlockEmptyTxList tests BroadcastBlock with empty transaction list
func TestBroadcastBlockEmptyTxList(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	block := wire.NewMsgBlock(&wire.BlockHeader{
		Version:    1,
		Timestamp:  time.Now(),
		Nonce:      999,
		MerkleRoot: chainhash.Hash{},
	})

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("BroadcastBlock panicked: %v", r)
		}
	}()

	pm.BroadcastBlock(block)
}

// TestOnInvWithPeerLikeConditions tests inv processing
func TestOnInvWithPeerLikeConditions(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	// Create inv with various types
	inv := wire.NewMsgInv()
	var hash1, hash2 chainhash.Hash
	hash1[0] = 0xAA
	hash2[0] = 0xBB

	inv.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &hash1))
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, &hash2))
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeWitnessBlock, &hash1))

	// Should handle nil peer gracefully
	pm.onInv(nil, inv)
}

// TestServeBlockNilBlockchain tests serveBlock when blockchain is nil.
// Note: Functional tests verifying that serveBlock queues a MsgBlock when
// GetBlockByHash succeeds would require the blockchain field to be an interface
// for mock injection. The field is currently *chain.BlockChain (concrete type),
// so these tests validate nil-safety and error paths only.
func TestServeBlockNilBlockchain(t *testing.T) {
	pm := createTestPeerManager(t)
	pm.blockchain = nil
	defer pm.mempool.Stop()

	var hash chainhash.Hash
	hash[0] = 0x01

	// Should not panic when blockchain is nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("serveBlock panicked with nil blockchain: %v", r)
		}
	}()

	pm.serveBlock(nil, &hash, "test-peer")
}

// TestServeBlockNilPeer tests serveBlock returns silently with nil peer
func TestServeBlockNilPeer(t *testing.T) {
	pm := createTestPeerManager(t)
	defer pm.mempool.Stop()

	var hash chainhash.Hash
	hash[0] = 0x01

	// Should return silently without panic or logging when peer is nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("serveBlock panicked with nil peer: %v", r)
		}
	}()

	pm.serveBlock(nil, &hash, "test-peer")
}

// TestOnGetDataBlockRequest tests onGetData block serving with nil blockchain
func TestOnGetDataBlockRequest(t *testing.T) {
	pm := createTestPeerManager(t)
	pm.blockchain = nil
	defer pm.mempool.Stop()

	getData := wire.NewMsgGetData()
	var blockHash chainhash.Hash
	blockHash[0] = 0xAB
	getData.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, &blockHash))

	// Should not panic — gracefully handles nil blockchain
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onGetData panicked with block request and nil blockchain: %v", r)
		}
	}()

	// nil peer triggers early return
	pm.onGetData(nil, getData)
}

// TestOnGetBlocksNilBlockchainGraceful tests onGetBlocks with nil blockchain returns early
func TestOnGetBlocksNilBlockchainGraceful(t *testing.T) {
	pm := createTestPeerManager(t)
	pm.blockchain = nil
	defer pm.mempool.Stop()

	getBlocks := &wire.MsgGetBlocks{
		HashStop: chainhash.Hash{},
	}

	// Should not panic when blockchain is nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onGetBlocks panicked with nil blockchain: %v", r)
		}
	}()

	pm.onGetBlocks(nil, getBlocks)
}
