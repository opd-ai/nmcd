package network

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/chain"
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
		mempool: NewMempoolWithConfig(&MempoolConfig{
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
		mempool: NewMempoolWithConfig(&MempoolConfig{
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

func TestNewPeerManagerRequiresBlockchain(t *testing.T) {
	cfg := &Config{
		ChainParams: &chaincfg.MainNetParams,
		Blockchain:  nil,
		MaxPeers:    8,
	}

	pm, err := NewPeerManager(cfg)
	if err == nil {
		t.Fatal("Expected error when Blockchain is nil")
	}
	if pm != nil {
		t.Fatal("Expected nil peer manager when Blockchain is nil")
	}
}

func TestNewPeerManagerWithBlockchain(t *testing.T) {
	cfg := &Config{
		ChainParams: &chaincfg.MainNetParams,
		Blockchain:  &chain.BlockChain{},
		MaxPeers:    8,
	}

	pm, err := NewPeerManager(cfg)
	if err != nil {
		t.Fatalf("NewPeerManager failed with blockchain: %v", err)
	}
	defer pm.Stop()

	if pm.blockchain != cfg.Blockchain {
		t.Fatal("Expected peer manager to retain configured blockchain")
	}
	if pm.mempool == nil {
		t.Fatal("Expected peer manager to initialize mempool")
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

func TestOnInvFiltersAndQueuesGetData(t *testing.T) {
	pm := &PeerManager{
		logger:      logging.GetDefault().WithComponent("test"),
		peers:       make(map[int32]*peer.Peer),
		chainParams: &chaincfg.MainNetParams,
		maxPeers:    10,
		quit:        make(chan struct{}),
		mempool: NewMempoolWithConfig(&MempoolConfig{
			MaxTxs:      100,
			TxExpiry:    time.Hour,
			CleanupTick: time.Minute,
		}),
	}
	defer pm.mempool.Stop()

	p, err := peer.NewOutboundPeer(&peer.Config{
		UserAgentName:    "nmcd-test",
		UserAgentVersion: "0.1.0",
		ChainParams:      &chaincfg.MainNetParams,
	}, "127.0.0.1:8334")
	if err != nil {
		t.Fatalf("Failed to create peer: %v", err)
	}
	peerConn, remoteConn := net.Pipe()
	p.AssociateConnection(peerConn)
	getDataCh := make(chan *wire.MsgGetData, 1)
	handshakeDone := make(chan struct{})
	helperErrCh := make(chan error, 1)
	go func() {
		defer remoteConn.Close()

		remoteVersion := wire.NewMsgVersion(
			wire.NewNetAddressIPPort(net.ParseIP("127.0.0.1"), 8334, wire.SFNodeNetwork),
			wire.NewNetAddressIPPort(net.ParseIP("127.0.0.1"), 8333, wire.SFNodeNetwork),
			1,
			0,
		)

		msg, _, err := wire.ReadMessage(remoteConn, wire.ProtocolVersion, chaincfg.MainNetParams.Net)
		if err != nil {
			helperErrCh <- err
			return
		}
		if _, ok := msg.(*wire.MsgVersion); !ok {
			helperErrCh <- fmt.Errorf("expected version message, got %T", msg)
			return
		}
		if err := wire.WriteMessage(remoteConn, remoteVersion, wire.ProtocolVersion, chaincfg.MainNetParams.Net); err != nil {
			helperErrCh <- err
			return
		}

		for {
			msg, _, err = wire.ReadMessage(remoteConn, wire.ProtocolVersion, chaincfg.MainNetParams.Net)
			if err != nil {
				helperErrCh <- err
				return
			}
			if _, ok := msg.(*wire.MsgVerAck); ok {
				break
			}
		}

		if err := wire.WriteMessage(remoteConn, wire.NewMsgVerAck(), wire.ProtocolVersion, chaincfg.MainNetParams.Net); err != nil {
			helperErrCh <- err
			return
		}
		close(handshakeDone)

		for {
			msg, _, err = wire.ReadMessage(remoteConn, wire.ProtocolVersion, chaincfg.MainNetParams.Net)
			if err != nil {
				helperErrCh <- err
				return
			}
			if gd, ok := msg.(*wire.MsgGetData); ok {
				getDataCh <- gd
				return
			}
		}
	}()
	defer func() {
		p.Disconnect()
		_ = peerConn.Close()
		_ = remoteConn.Close()
	}()

	select {
	case <-handshakeDone:
	case err := <-helperErrCh:
		t.Fatalf("Failed during peer handshake setup: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for peer handshake")
	}

	mempoolTx := wire.NewMsgTx(wire.TxVersion)
	if err := pm.mempool.AddTx(mempoolTx); err != nil {
		t.Fatalf("Failed to add mempool transaction: %v", err)
	}
	mempoolTxHash := mempoolTx.TxHash()

	var newTxHash, blockHash chainhash.Hash
	newTxHash[0] = 0x02
	blockHash[0] = 0x03

	inv := wire.NewMsgInv()
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &mempoolTxHash))       // should be skipped (already in mempool)
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &newTxHash))           // should be requested
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, &blockHash))        // should be requested
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeWitnessBlock, &blockHash)) // should be ignored

	pm.onInv(p, inv)
	var gotGetData *wire.MsgGetData
	select {
	case gotGetData = <-getDataCh:
	case err := <-helperErrCh:
		t.Fatalf("Peer helper failed while waiting for getdata: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for getdata message")
	}

	if gotGetData == nil {
		t.Fatal("Expected queued getdata message")
	}
	if len(gotGetData.InvList) != 2 {
		t.Fatalf("Expected 2 inventory vectors in getdata, got %d", len(gotGetData.InvList))
	}
	if gotGetData.InvList[0].Type != wire.InvTypeTx || gotGetData.InvList[0].Hash != newTxHash {
		t.Fatalf("Expected first inventory vector to be new tx %s, got type=%v hash=%s", newTxHash, gotGetData.InvList[0].Type, gotGetData.InvList[0].Hash)
	}
	if gotGetData.InvList[1].Type != wire.InvTypeBlock || gotGetData.InvList[1].Hash != blockHash {
		t.Fatalf("Expected second inventory vector to be block %s, got type=%v hash=%s", blockHash, gotGetData.InvList[1].Type, gotGetData.InvList[1].Hash)
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
			mempool: NewMempoolWithConfig(&MempoolConfig{
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
