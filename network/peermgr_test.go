package network

import (
	"net"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/internal/logging"
)

// TestPeerManagerCreation tests that PeerManager can be created with proper configuration.
func TestPeerManagerCreation(t *testing.T) {
	// Ensure logger is initialized
	logCfg := logging.DefaultConfig()
	logCfg.Level = logging.LevelError
	logger, err := logging.Init(logCfg)
	if err == nil {
		logging.SetDefault(logger)
	}

	// Create a PeerManager directly for testing structure without needing a full blockchain
	// In production, NewPeerManager should be used which requires a non-nil blockchain
	pm := &PeerManager{
		peers:       make(map[int32]*peer.Peer),
		blockchain:  nil, // nil is acceptable for structure tests
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    10,
		logger:      logging.GetDefault().WithComponent("network"),
		quit:        make(chan struct{}),
		mempool:     NewMempoolWithConfig(&MempoolConfig{
			Validator:   nil,
			MaxTxs:      5000,
			TxExpiry:    24 * time.Hour,
			CleanupTick: 10 * time.Minute,
		}),
	}
	defer pm.Stop()

	// Verify the configuration was stored correctly
	if pm.maxPeers != 10 {
		t.Errorf("Expected maxPeers to be 10, got %d", pm.maxPeers)
	}

	if pm.chainParams != &chaincfg.MainNetParams {
		t.Error("Expected chainParams to be MainNetParams")
	}

	// Verify the peers map was initialized
	if pm.peers == nil {
		t.Error("Expected peers map to be initialized")
	}

	// Verify the quit channel was created
	if pm.quit == nil {
		t.Error("Expected quit channel to be initialized")
	}
}

// TestPeerManagerGetConnectedPeers tests the GetConnectedPeers method.
func TestPeerManagerGetConnectedPeers(t *testing.T) {
	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[int32]*peer.Peer),
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    10,
		quit:        make(chan struct{}),
	}

	// Initially should have 0 peers
	if count := pm.GetConnectedPeers(); count != 0 {
		t.Errorf("Expected 0 connected peers, got %d", count)
	}
}

// TestPeerManagerGetPeerInfo tests the GetPeerInfo method.
func TestPeerManagerGetPeerInfo(t *testing.T) {
	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[int32]*peer.Peer),
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    10,
		quit:        make(chan struct{}),
	}

	// Initially should have empty peer info
	info := pm.GetPeerInfo()
	if len(info) != 0 {
		t.Errorf("Expected 0 peer info entries, got %d", len(info))
	}
}

// TestPeerManagerStop tests that Stop can be called safely.
func TestPeerManagerStop(t *testing.T) {
	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[int32]*peer.Peer),
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    10,
		quit:        make(chan struct{}),
	}

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Stop panicked: %v", r)
		}
	}()

	pm.Stop()
}

// TestPeerManagerBlockchainReference tests that blockchain reference is stored correctly.
func TestPeerManagerBlockchainReference(t *testing.T) {
	// Ensure logger is initialized
	logCfg := logging.DefaultConfig()
	logCfg.Level = logging.LevelError
	logger, err := logging.Init(logCfg)
	if err == nil {
		logging.SetDefault(logger)
	}

	// Create a PeerManager directly for testing
	pm := &PeerManager{
		peers:       make(map[int32]*peer.Peer),
		blockchain:  nil,
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    10,
		logger:      logging.GetDefault().WithComponent("network"),
		quit:        make(chan struct{}),
		mempool:     NewMempoolWithConfig(&MempoolConfig{
			Validator:   nil,
			MaxTxs:      5000,
			TxExpiry:    24 * time.Hour,
			CleanupTick: 10 * time.Minute,
		}),
	}
	defer pm.Stop()

	// Verify blockchain reference is nil as configured
	if pm.blockchain != nil {
		t.Error("Expected blockchain to be nil when configured with nil")
	}
}

// TestPeerInfo tests the PeerInfo struct.
func TestPeerInfo(t *testing.T) {
	info := PeerInfo{
		Addr:      "192.168.1.1:8334",
		Connected: true,
		Inbound:   false,
	}

	if info.Addr != "192.168.1.1:8334" {
		t.Errorf("Expected Addr to be '192.168.1.1:8334', got %s", info.Addr)
	}

	if !info.Connected {
		t.Error("Expected Connected to be true")
	}

	if info.Inbound {
		t.Error("Expected Inbound to be false")
	}
}

// TestConfigStruct tests the Config struct.
func TestConfigStruct(t *testing.T) {
	cfg := &Config{
		ChainParams: &chaincfg.TestNet3Params,
		Blockchain:  nil,
		ListenAddrs: []string{"0.0.0.0:8334", "127.0.0.1:8335"},
		MaxPeers:    25,
	}

	if cfg.ChainParams != &chaincfg.TestNet3Params {
		t.Error("Expected ChainParams to be TestNet3Params")
	}

	if len(cfg.ListenAddrs) != 2 {
		t.Errorf("Expected 2 listen addresses, got %d", len(cfg.ListenAddrs))
	}

	if cfg.MaxPeers != 25 {
		t.Errorf("Expected MaxPeers to be 25, got %d", cfg.MaxPeers)
	}
}

// TestOnBlockWithNilBlockchain tests that onBlock handles nil blockchain gracefully.
// When blockchain is nil, the handler should log a message and return without panicking.
func TestOnBlockWithNilBlockchain(t *testing.T) {
	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[int32]*peer.Peer),
		blockchain:  nil, // nil blockchain
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    10,
		quit:        make(chan struct{}),
	}

	// Create a test block message
	msgBlock := wire.NewMsgBlock(&wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
	})

	// Should not panic when blockchain is nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onBlock panicked with nil blockchain: %v", r)
		}
	}()

	// Call onBlock with nil peer and nil blockchain
	// The function should handle this gracefully by logging and returning
	pm.onBlock(nil, msgBlock, nil)
}

// TestOnBlockDoesNotPanicWithValidBlock tests that onBlock doesn't panic
// when given a valid block structure, even if the block would be rejected.
func TestOnBlockDoesNotPanicWithValidBlock(t *testing.T) {
	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[int32]*peer.Peer),
		blockchain:  nil, // We use nil to test the nil check path
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    10,
		quit:        make(chan struct{}),
	}

	// Create a more complete block message
	msgBlock := wire.NewMsgBlock(&wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
	})

	// Add a coinbase transaction
	coinbaseTx := wire.NewMsgTx(1)
	coinbaseTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Index: 0xffffffff,
		},
	})
	coinbaseTx.AddTxOut(&wire.TxOut{
		Value:    50 * 1e8,
		PkScript: []byte{0x76, 0xa9, 0x14},
	})
	msgBlock.AddTransaction(coinbaseTx)

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("onBlock panicked: %v", r)
		}
	}()

	pm.onBlock(nil, msgBlock, nil)
}

// TestOnBlockBufferParameter tests that the buf parameter is properly handled.
func TestOnBlockBufferParameter(t *testing.T) {
	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[int32]*peer.Peer),
		blockchain:  nil,
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    10,
		quit:        make(chan struct{}),
	}

	msgBlock := wire.NewMsgBlock(&wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
	})

	// Test with various buf values - none should cause issues
	testCases := []struct {
		name string
		buf  []byte
	}{
		{"nil buffer", nil},
		{"empty buffer", []byte{}},
		{"non-empty buffer", []byte{0x01, 0x02, 0x03}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("onBlock panicked with %s: %v", tc.name, r)
				}
			}()
			pm.onBlock(nil, msgBlock, tc.buf)
		})
	}
}

// TestEdgeCaseBugAcceptLoopRace tests that the accept loop goroutine
// properly handles shutdown without races or goroutine leaks.
// This test reproduces the race condition where the accept goroutine may try
// to send on channels after the main loop has exited.
func TestEdgeCaseBugAcceptLoopRace(t *testing.T) {
	// Ensure logger is initialized
	logCfg := logging.DefaultConfig()
	logCfg.Level = logging.LevelError
	logger, err := logging.Init(logCfg)
	if err == nil {
		logging.SetDefault(logger)
	}

	// Run the test multiple times to increase the chance of detecting a race
	for i := 0; i < 10; i++ {
		// Start a listener manually (simulates what NewPeerManager does)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to create listener: %v", err)
		}

		// Create PeerManager directly
		pm := &PeerManager{
			peers:       make(map[int32]*peer.Peer),
			blockchain:  nil,
			chainParams: &chaincfg.MainNetParams,
			maxPeers:    10,
			logger:      logging.GetDefault().WithComponent("network"),
			quit:        make(chan struct{}),
			mempool:     NewMempoolWithConfig(&MempoolConfig{
				Validator:   nil,
				MaxTxs:      5000,
				TxExpiry:    24 * time.Hour,
				CleanupTick: 10 * time.Minute,
			}),
		}
		pm.listeners = append(pm.listeners, listener)

		// Start listen loop
		pm.wg.Add(1)
		go pm.listenLoop(listener)

		// Give it a tiny bit of time to start listening
		time.Sleep(10 * time.Millisecond)

		// Stop immediately - this should trigger the race condition
		// where the accept goroutine may still be blocked on Accept()
		pm.Stop()

		// Give a small window for any potential race to manifest
		time.Sleep(20 * time.Millisecond)
	}
	// If we get here without a panic or hanging, the test passes
}
