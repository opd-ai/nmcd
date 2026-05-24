package network

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/chain"
	"github.com/opd-ai/nmcd/internal/logging"
)

// TestSyncManagerCreation tests that a sync manager can be created
func TestSyncManagerCreation(t *testing.T) {
	// Create a minimal peer manager for testing
	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[string]*peer.Peer),
		blockchain:  nil, // Can be nil for this test
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    8,
		quit:        make(chan struct{}),
	}

	// Create sync manager
	sm := NewSyncManager(pm)
	if sm == nil {
		t.Fatal("NewSyncManager returned nil")
	}

	// Check initial state
	if !sm.headersFirstMode {
		t.Error("Expected sync manager to start in headers-first mode")
	}

	if sm.IsSyncing() {
		t.Error("Expected sync manager to not be syncing initially")
	}

	// Clean up
	sm.Stop()
}

// TestSyncManagerUpdatePeerHeight tests updating peer height
func TestSyncManagerUpdatePeerHeight(t *testing.T) {
	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[string]*peer.Peer),
		blockchain:  nil,
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    8,
		quit:        make(chan struct{}),
	}

	sm := NewSyncManager(pm)
	defer sm.Stop()

	// Create a mock peer (nil is okay for this test)
	var mockPeer *peer.Peer = nil

	// Update with a height
	sm.UpdatePeerHeight(mockPeer, 100)

	// Check that best height was updated
	sm.mu.RLock()
	if sm.bestHeight != 100 {
		t.Errorf("Expected best height 100, got %d", sm.bestHeight)
	}
	sm.mu.RUnlock()

	// Update with a higher height
	sm.UpdatePeerHeight(mockPeer, 200)

	sm.mu.RLock()
	if sm.bestHeight != 200 {
		t.Errorf("Expected best height 200, got %d", sm.bestHeight)
	}
	sm.mu.RUnlock()

	// Update with a lower height (should not change)
	sm.UpdatePeerHeight(mockPeer, 150)

	sm.mu.RLock()
	if sm.bestHeight != 200 {
		t.Errorf("Expected best height to remain 200, got %d", sm.bestHeight)
	}
	sm.mu.RUnlock()
}

// TestSyncManagerBlockReceived tests block request tracking
func TestSyncManagerBlockReceived(t *testing.T) {
	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[string]*peer.Peer),
		blockchain:  nil,
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    8,
		quit:        make(chan struct{}),
	}

	sm := NewSyncManager(pm)
	defer sm.Stop()

	// Create a test block hash
	hash := chainhash.Hash{}
	hash[0] = 1

	// Add to requested blocks manually
	sm.mu.Lock()
	sm.requestedBlocks[hash] = time.Now()
	sm.mu.Unlock()

	// Verify it's in the map
	sm.mu.RLock()
	if _, exists := sm.requestedBlocks[hash]; !exists {
		t.Error("Expected block to be in requested blocks map")
	}
	sm.mu.RUnlock()

	// Notify that block was received
	sm.BlockReceived(&hash)

	// Verify it was removed
	sm.mu.RLock()
	if _, exists := sm.requestedBlocks[hash]; exists {
		t.Error("Expected block to be removed from requested blocks map")
	}
	sm.mu.RUnlock()
}

// TestSyncManagerCleanupOldRequests tests old request cleanup
func TestSyncManagerCleanupOldRequests(t *testing.T) {
	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[string]*peer.Peer),
		blockchain:  nil,
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    8,
		quit:        make(chan struct{}),
	}

	sm := NewSyncManager(pm)
	defer sm.Stop()

	// Create test hashes
	oldHash := chainhash.Hash{}
	oldHash[0] = 1
	newHash := chainhash.Hash{}
	newHash[0] = 2

	// Add old and new requests
	sm.mu.Lock()
	sm.requestedBlocks[oldHash] = time.Now().Add(-3 * time.Minute) // Old request
	sm.requestedBlocks[newHash] = time.Now()                       // New request
	sm.mu.Unlock()

	// Run cleanup
	sm.mu.Lock()
	sm.cleanupOldRequests()
	sm.mu.Unlock()

	// Check that old request was removed and new request remains
	sm.mu.RLock()
	if _, exists := sm.requestedBlocks[oldHash]; exists {
		t.Error("Expected old block request to be cleaned up")
	}
	if _, exists := sm.requestedBlocks[newHash]; !exists {
		t.Error("Expected new block request to remain")
	}
	sm.mu.RUnlock()
}

// TestSyncManagerHandleHeaders tests header message handling
func TestSyncManagerHandleHeaders(t *testing.T) {
	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[string]*peer.Peer),
		blockchain:  nil,
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    8,
		quit:        make(chan struct{}),
	}

	sm := NewSyncManager(pm)
	defer sm.Stop()

	// Create empty headers message
	msg := wire.NewMsgHeaders()

	// Handle empty headers (should not panic)
	sm.HandleHeaders(nil, msg)

	// No assertions needed - test passes if no panic occurs
}

func TestSyncManagerHandleHeadersRequiresActiveSyncPeer(t *testing.T) {
	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[string]*peer.Peer),
		blockchain:  nil,
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    8,
		quit:        make(chan struct{}),
	}

	sm := NewSyncManager(pm)
	defer sm.Stop()
	sm.blockchain = &chain.BlockChain{}
	sm.headersFirstMode = true

	nonSyncPeer, err := peer.NewOutboundPeer(&peer.Config{
		UserAgentName:    "nmcd-test",
		UserAgentVersion: "0.1.0",
		ChainParams:      &chaincfg.MainNetParams,
	}, "127.0.0.1:8333")
	if err != nil {
		t.Fatalf("failed to create non-sync peer: %v", err)
	}
	syncPeer, err := peer.NewOutboundPeer(&peer.Config{
		UserAgentName:    "nmcd-test",
		UserAgentVersion: "0.1.0",
		ChainParams:      &chaincfg.MainNetParams,
	}, "127.0.0.1:8334")
	if err != nil {
		t.Fatalf("failed to create sync peer: %v", err)
	}

	msg := wire.NewMsgHeaders()
	header := wire.BlockHeader{Version: 1, Timestamp: time.Now()}
	msg.AddBlockHeader(&header)

	// No active sync peer: ignore all headers.
	sm.syncPeer = nil
	sm.HandleHeaders(nonSyncPeer, msg)
	sm.mu.RLock()
	requested := len(sm.requestedBlocks)
	sm.mu.RUnlock()
	if requested != 0 {
		t.Fatalf("expected no requested blocks without active sync peer, got %d", requested)
	}

	// Active sync peer exists: ignore headers from other peers.
	sm.syncPeer = syncPeer
	sm.HandleHeaders(nonSyncPeer, msg)
	sm.mu.RLock()
	requested = len(sm.requestedBlocks)
	sm.mu.RUnlock()
	if requested != 0 {
		t.Fatalf("expected no requested blocks from non-sync peer, got %d", requested)
	}
}

func TestSyncManagerOnPeerDisconnectedReselectsBestPeerWithoutResettingHeight(t *testing.T) {
	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[string]*peer.Peer),
		blockchain:  nil,
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    8,
		quit:        make(chan struct{}),
	}

	sm := NewSyncManager(pm)
	defer sm.Stop()

	disconnectedPeer, err := peer.NewOutboundPeer(&peer.Config{
		UserAgentName:    "nmcd-test",
		UserAgentVersion: "0.1.0",
		ChainParams:      &chaincfg.MainNetParams,
	}, "127.0.0.1:9333")
	if err != nil {
		t.Fatalf("failed to create disconnected peer: %v", err)
	}
	remainingPeer, err := peer.NewOutboundPeer(&peer.Config{
		UserAgentName:    "nmcd-test",
		UserAgentVersion: "0.1.0",
		ChainParams:      &chaincfg.MainNetParams,
	}, "127.0.0.1:9334")
	if err != nil {
		t.Fatalf("failed to create remaining peer: %v", err)
	}

	// PeerManager removes the disconnected peer before notifying SyncManager.
	pm.mu.Lock()
	pm.peers[remainingPeer.Addr()] = remainingPeer
	pm.mu.Unlock()

	sm.mu.Lock()
	sm.syncPeer = disconnectedPeer
	sm.bestPeer = disconnectedPeer
	sm.bestHeight = 321
	sm.mu.Unlock()

	sm.OnPeerDisconnected(disconnectedPeer)

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.syncPeer != nil {
		t.Fatal("expected sync peer to be cleared")
	}
	if sm.bestPeer == nil || sm.bestPeer.Addr() != remainingPeer.Addr() {
		t.Fatalf("expected best peer to be reselected to remaining peer %s", remainingPeer.Addr())
	}
	if sm.bestHeight != 321 {
		t.Fatalf("expected best height to remain 321, got %d", sm.bestHeight)
	}
}
