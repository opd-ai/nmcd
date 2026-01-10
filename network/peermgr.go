package network

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/chain"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/internal/logging"
	"github.com/opd-ai/nmcd/metrics"
)

// PeerManager manages network peers using btcd/peer
type PeerManager struct {
	peers       map[string]*peer.Peer
	listeners   []net.Listener
	blockchain  *chain.BlockChain
	mempool     *Mempool
	syncManager *SyncManager
	chainParams *chaincfg.Params
	maxPeers    int
	logger      *logging.Logger
	mu          sync.RWMutex
	quit        chan struct{}
	wg          sync.WaitGroup
}

// Config holds network configuration
type Config struct {
	ChainParams *chaincfg.Params
	Blockchain  *chain.BlockChain
	ListenAddrs []string
	MaxPeers    int
	AddPeers    []string // Initial peers to connect to (empty = no auto-connect)
}

// NewPeerManager creates a new peer manager
func NewPeerManager(cfg *Config) (*PeerManager, error) {
	// Get logger
	logger := logging.GetDefault().WithComponent("network")

	// Create mempool with validation
	mempoolCfg := &MempoolConfig{
		Validator:   cfg.Blockchain, // BlockChain implements TxValidator
		MaxTxs:      5000,
		TxExpiry:    24 * time.Hour,
		CleanupTick: 10 * time.Minute,
	}

	pm := &PeerManager{
		peers:       make(map[string]*peer.Peer),
		blockchain:  cfg.Blockchain,
		mempool:     NewMempoolWithConfig(mempoolCfg),
		chainParams: cfg.ChainParams,
		maxPeers:    cfg.MaxPeers,
		logger:      logger,
		quit:        make(chan struct{}),
	}

	// Create sync manager for Initial Block Download
	pm.syncManager = NewSyncManager(pm)

	// Start listeners
	for _, addr := range cfg.ListenAddrs {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			pm.Stop()
			return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
		}
		pm.listeners = append(pm.listeners, listener)

		pm.wg.Add(1)
		go pm.listenLoop(listener)
	}

	// Connect to initial peers if configured
	for _, addr := range cfg.AddPeers {
		pm.wg.Add(1)
		go func(peerAddr string) {
			defer pm.wg.Done()

			// Wait briefly for peer manager to be ready
			select {
			case <-time.After(time.Second):
				// Proceed with connection
			case <-pm.quit:
				// Shutdown requested during wait
				return
			}

			if err := pm.ConnectPeer(peerAddr); err != nil {
				logger.Warn("failed to connect to initial peer",
					"address", peerAddr,
					"error", err,
				)
			}
		}(addr)
	}

	return pm, nil
}

// listenLoop accepts incoming connections
func (pm *PeerManager) listenLoop(listener net.Listener) {
	defer pm.wg.Done()

	// Use buffered channels to prevent goroutine leaks when the main loop exits
	// via the quit signal before the accept goroutine sends.
	acceptCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case errCh <- err:
				case <-pm.quit:
					// Main loop has exited, stop sending
				}
				return
			}
			select {
			case acceptCh <- conn:
			case <-pm.quit:
				// Main loop has exited, close the connection
				conn.Close()
				return
			}
		}
	}()

	for {
		select {
		case <-pm.quit:
			return
		case conn := <-acceptCh:
			pm.wg.Add(1)
			go pm.handleInboundPeer(conn)
		case err := <-errCh:
			// Accept error, likely due to listener closure.
			// The accept goroutine has already stopped, so we should exit too.
			pm.logger.Warn("accept error, listener closing", "error", err)
			return
		}
	}
}

// handleInboundPeer handles an inbound peer connection
func (pm *PeerManager) handleInboundPeer(conn net.Conn) {
	defer pm.wg.Done()

	// Check if max peers reached
	pm.mu.RLock()
	peerCount := len(pm.peers)
	pm.mu.RUnlock()

	if pm.maxPeers > 0 && peerCount >= pm.maxPeers {
		conn.Close()
		return
	}

	// Create peer configuration
	peerCfg := &peer.Config{
		UserAgentName:    "nmcd",
		UserAgentVersion: "0.1.0",
		ChainParams:      pm.chainParams,
		Services:         wire.SFNodeNetwork,
		ProtocolVersion:  config.NamecoinProtocolVersion, // Use Namecoin-specific protocol version
		TrickleInterval:  time.Second * 10,
		Listeners: peer.MessageListeners{
			OnVersion:    pm.onVersion,
			OnVerAck:     pm.onVerAck,
			OnInv:        pm.onInv,
			OnBlock:      pm.onBlock,
			OnTx:         pm.onTx,
			OnGetData:    pm.onGetData,
			OnHeaders:    pm.onHeaders,
			OnGetHeaders: pm.onGetHeaders,
			OnGetBlocks:  pm.onGetBlocks,
		},
	}

	p := peer.NewInboundPeer(peerCfg)
	p.AssociateConnection(conn)

	// Add to peer list
	pm.mu.Lock()
	pm.peers[p.Addr()] = p
	// Update peer count metrics
	pm.updatePeerMetrics()
	pm.mu.Unlock()

	// Wait for disconnect
	p.WaitForDisconnect()

	// Remove from peer list
	pm.mu.Lock()
	delete(pm.peers, p.Addr())
	// Update peer count metrics and record disconnect
	pm.updatePeerMetrics()
	metrics.Get().RecordPeerDisconnect()
	pm.mu.Unlock()
}

// ConnectPeer connects to an outbound peer
func (pm *PeerManager) ConnectPeer(addr string) error {
	// Check if max peers reached
	pm.mu.RLock()
	peerCount := len(pm.peers)
	pm.mu.RUnlock()

	if pm.maxPeers > 0 && peerCount >= pm.maxPeers {
		return fmt.Errorf("max peers limit (%d) reached", pm.maxPeers)
	}

	// Dial the peer
	conn, err := net.DialTimeout("tcp", addr, time.Second*30)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	// Create peer configuration
	peerCfg := &peer.Config{
		UserAgentName:    "nmcd",
		UserAgentVersion: "0.1.0",
		ChainParams:      pm.chainParams,
		Services:         wire.SFNodeNetwork,
		ProtocolVersion:  config.NamecoinProtocolVersion, // Use Namecoin-specific protocol version
		TrickleInterval:  time.Second * 10,
		Listeners: peer.MessageListeners{
			OnVersion:    pm.onVersion,
			OnVerAck:     pm.onVerAck,
			OnInv:        pm.onInv,
			OnBlock:      pm.onBlock,
			OnTx:         pm.onTx,
			OnGetData:    pm.onGetData,
			OnHeaders:    pm.onHeaders,
			OnGetHeaders: pm.onGetHeaders,
			OnGetBlocks:  pm.onGetBlocks,
		},
	}

	p, err := peer.NewOutboundPeer(peerCfg, addr)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create outbound peer: %w", err)
	}
	p.AssociateConnection(conn)

	// Add to peer list
	pm.mu.Lock()
	pm.peers[p.Addr()] = p
	// Update peer count metrics
	pm.updatePeerMetrics()
	pm.mu.Unlock()

	pm.wg.Add(1)
	go func() {
		defer pm.wg.Done()
		p.WaitForDisconnect()

		pm.mu.Lock()
		delete(pm.peers, p.Addr())
		// Update peer count metrics and record disconnect
		pm.updatePeerMetrics()
		metrics.Get().RecordPeerDisconnect()
		pm.mu.Unlock()
	}()

	return nil
}

// Message handlers
func (pm *PeerManager) onVersion(p *peer.Peer, msg *wire.MsgVersion) *wire.MsgReject {
	// Handle version message - update sync manager with peer's best height
	if pm.syncManager != nil {
		pm.syncManager.UpdatePeerHeight(p, msg.LastBlock)
	}
	return nil
}

func (pm *PeerManager) onVerAck(p *peer.Peer, msg *wire.MsgVerAck) {
	// Handle verack message
}

func (pm *PeerManager) onInv(p *peer.Peer, msg *wire.MsgInv) {
	// Handle inventory message
	// Request blocks/transactions we don't have
	gdmsg := wire.NewMsgGetData()
	for _, inv := range msg.InvList {
		gdmsg.AddInvVect(inv)
	}
	if len(gdmsg.InvList) > 0 {
		p.QueueMessage(gdmsg, nil)
	}
}

func (pm *PeerManager) onBlock(p *peer.Peer, msg *wire.MsgBlock, buf []byte) {
	// Check if blockchain is available for processing
	if pm.blockchain == nil {
		pm.logger.Warn("cannot process block: blockchain not initialized",
			"block_hash", msg.BlockHash().String())
		return
	}

	// Convert wire.MsgBlock to btcutil.Block for processing
	block := btcutil.NewBlock(msg)
	blockHash := block.Hash()

	// If buf is provided and block has AuxPow version bit, parse AuxPow data
	// The buf parameter contains the complete serialized block including AuxPow (if present).
	// We need to extract and store the AuxPow for later validation.
	if buf != nil && len(buf) > 0 {
		if err := pm.blockchain.SetBlockAuxPowFromBytes(blockHash, buf); err != nil {
			pm.logger.Warn("failed to parse AuxPow data, continuing anyway",
				"block_hash", msg.BlockHash().String(),
				"error", err)
			// Don't return - continue with validation, which will catch AuxPow issues
		}
	}

	// Process the block through the blockchain
	// BFNone means no special behavior flags
	isMainChain, isOrphan, err := pm.blockchain.ProcessBlock(block, blockchain.BFNone)
	if err != nil {
		pm.logger.Error("failed to process block",
			"block_hash", msg.BlockHash().String(),
			"peer_id", p.Addr(),
			"error", err)
		return
	}

	// Notify sync manager that block was received
	if pm.syncManager != nil {
		pm.syncManager.BlockReceived(blockHash)
	}

	// If block was accepted on the main chain, remove confirmed transactions from mempool
	if isMainChain {
		// Collect transaction hashes from the block
		txHashes := make([]chainhash.Hash, 0, len(msg.Transactions))
		for _, tx := range msg.Transactions {
			txHashes = append(txHashes, tx.TxHash())
		}

		// Remove confirmed transactions from mempool
		pm.mempool.RemoveTxs(txHashes)
	}

	// Log the result for debugging
	if isOrphan {
		pm.logger.Debug("received orphan block",
			"block_hash", msg.BlockHash().String(),
			"peer_id", p.Addr())
	} else if isMainChain {
		pm.logger.Info("accepted block on main chain",
			"block_hash", msg.BlockHash().String(),
			"peer_id", p.Addr())
	} else {
		pm.logger.Info("accepted block on side chain",
			"block_hash", msg.BlockHash().String(),
			"peer_id", p.Addr())
	}
}

func (pm *PeerManager) onTx(p *peer.Peer, msg *wire.MsgTx) {
	// Handle transaction message by validating and adding to mempool
	if msg == nil {
		return
	}

	txHash := msg.TxHash()

	// Check if we already have this transaction in mempool
	if pm.mempool.HasTx(&txHash) {
		// Already have this transaction, no need to process again
		return
	}

	// Add transaction to mempool (includes validation)
	err := pm.mempool.AddTx(msg)
	if err != nil {
		pm.logger.Warn("rejected transaction from peer",
			"tx_hash", txHash.String(),
			"peer_id", p.Addr(),
			"error", err)
		return
	}

	pm.logger.Info("accepted transaction",
		"tx_hash", txHash.String(),
		"peer_id", p.Addr(),
		"mempool_size", pm.mempool.Count())

	// Relay transaction to other peers (transaction propagation)
	// This implements the critical transaction relay functionality
	pm.relayTransaction(msg, p)
}

// relayTransaction broadcasts a transaction to all peers except the source
func (pm *PeerManager) relayTransaction(tx *wire.MsgTx, excludePeer *peer.Peer) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if len(pm.peers) == 0 {
		return
	}

	txHash := tx.TxHash()

	// Create inventory message for the transaction
	inv := wire.NewMsgInv()
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &txHash))

	// Broadcast to all connected peers except the source
	relayCount := 0
	for _, targetPeer := range pm.peers {
		// Skip the peer we received this from to avoid relay loops
		// Use pointer equality for reliable peer identity comparison
		if excludePeer != nil && targetPeer == excludePeer {
			continue
		}

		if targetPeer.Connected() {
			targetPeer.QueueMessage(inv, nil)
			relayCount++
		}
	}

	if relayCount > 0 {
		pm.logger.Debug("relayed transaction to peers",
			"tx_hash", txHash.String(),
			"peer_count", relayCount)
	}
}

func (pm *PeerManager) onGetData(p *peer.Peer, msg *wire.MsgGetData) {
	// Handle getdata message - send requested blocks/transactions
	for _, inv := range msg.InvList {
		switch inv.Type {
		case wire.InvTypeBlock:
			// Block requests are handled but not fully implemented
			// This would require fetching blocks from the blockchain database
			pm.logger.Debug("received block request (not implemented)",
				"peer_id", p.Addr(),
				"block_hash", inv.Hash.String())

		case wire.InvTypeTx:
			// Send transaction from mempool if we have it
			tx, exists := pm.mempool.GetTx(&inv.Hash)
			if exists {
				p.QueueMessage(tx, nil)
				pm.logger.Debug("sent transaction to peer",
					"tx_hash", inv.Hash.String(),
					"peer_id", p.Addr())
			} else {
				// Transaction not found in mempool
				// Could also check blockchain for confirmed transactions
				pm.logger.Debug("transaction not found in mempool",
					"tx_hash", inv.Hash.String(),
					"requested_by", p.Addr())
			}
		}
	}
}

// onHeaders handles incoming headers messages for block synchronization
func (pm *PeerManager) onHeaders(p *peer.Peer, msg *wire.MsgHeaders) {
	// Forward to sync manager for processing
	if pm.syncManager != nil {
		pm.syncManager.HandleHeaders(p, msg)
	}
}

// onGetHeaders handles getheaders requests for block synchronization
func (pm *PeerManager) onGetHeaders(p *peer.Peer, msg *wire.MsgGetHeaders) {
	// Handle getheaders message - respond with block headers for sync
	if pm.blockchain == nil {
		pm.logger.Warn("cannot process getheaders: blockchain not initialized")
		return
	}

	// Use btcd's LocateHeaders to find headers to send
	headers := pm.blockchain.LocateHeaders(msg.BlockLocatorHashes, &msg.HashStop)

	pm.logger.Debug("received getheaders request",
		"peer_id", p.Addr(),
		"header_count", len(headers))

	// Create and send headers message
	headersMsg := wire.NewMsgHeaders()
	for i := range headers {
		// Add header with empty transaction count (headers-only)
		headersMsg.AddBlockHeader(&headers[i])
	}
	p.QueueMessage(headersMsg, nil)
}

// onGetBlocks handles getblocks requests for block synchronization
func (pm *PeerManager) onGetBlocks(p *peer.Peer, msg *wire.MsgGetBlocks) {
	// Handle getblocks message - respond with block inventory for sync
	if pm.blockchain == nil {
		pm.logger.Warn("cannot process getblocks: blockchain not initialized")
		return
	}

	// Get the best block hash
	bestHash := pm.blockchain.BestSnapshot().Hash

	// In a full implementation, this would:
	// 1. Find the common ancestor with msg.BlockLocatorHashes
	// 2. Send inventory message with block hashes from that point
	// For now, just log that we received the request
	pm.logger.Debug("received getblocks request",
		"peer_id", p.Addr(),
		"best_hash", bestHash.String())

	// Create inv message (empty for minimal implementation)
	invMsg := wire.NewMsgInv()
	p.QueueMessage(invMsg, nil)
}

// BroadcastBlock broadcasts a block to all peers
func (pm *PeerManager) BroadcastBlock(block *wire.MsgBlock) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	inv := wire.NewMsgInv()
	blockHash := block.BlockHash()
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, &blockHash))

	for _, p := range pm.peers {
		p.QueueMessage(inv, nil)
	}
}

// BroadcastTx broadcasts a transaction to all peers
// This is used when locally creating transactions (e.g., from RPC calls)
func (pm *PeerManager) BroadcastTx(tx *wire.MsgTx) error {
	if tx == nil {
		return fmt.Errorf("cannot broadcast nil transaction")
	}

	// First, add to our own mempool (with validation)
	if err := pm.mempool.AddTx(tx); err != nil {
		return fmt.Errorf("failed to add transaction to mempool: %w", err)
	}

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if len(pm.peers) == 0 {
		pm.logger.Warn("no peers connected, transaction not relayed",
			"tx_hash", tx.TxHash().String())
		return nil
	}

	// Create inventory message
	inv := wire.NewMsgInv()
	txHash := tx.TxHash()
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &txHash))

	// Broadcast to all connected peers
	broadcastCount := 0
	for _, p := range pm.peers {
		if p.Connected() {
			p.QueueMessage(inv, nil)
			broadcastCount++
		}
	}

	pm.logger.Info("broadcast transaction",
		"tx_hash", txHash.String(),
		"peer_count", broadcastCount)
	return nil
}

// GetConnectedPeers returns the number of connected peers
func (pm *PeerManager) GetConnectedPeers() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.peers)
}

// GetPeerInfo returns information about connected peers
func (pm *PeerManager) GetPeerInfo() []PeerInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	info := make([]PeerInfo, 0, len(pm.peers))
	for _, p := range pm.peers {
		info = append(info, PeerInfo{
			Addr:      p.Addr(),
			Connected: p.Connected(),
			Inbound:   p.Inbound(),
		})
	}
	return info
}

// IsSyncing returns whether we're currently syncing with peers
func (pm *PeerManager) IsSyncing() bool {
	if pm.syncManager == nil {
		return false
	}
	return pm.syncManager.IsSyncing()
}

// GetMempool returns the mempool instance
func (pm *PeerManager) GetMempool() *Mempool {
	return pm.mempool
}

// SyncBlocks initiates block synchronization with peers
// This sends getheaders requests to connected peers to start syncing
func (pm *PeerManager) SyncBlocks() {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.blockchain == nil {
		pm.logger.Warn("cannot sync blocks: blockchain not initialized")
		return
	}

	// Get our best block hash to use as the starting point
	bestHash := pm.blockchain.BestSnapshot().Hash

	// Create a getheaders message
	getHeadersMsg := wire.NewMsgGetHeaders()
	getHeadersMsg.AddBlockLocatorHash(&bestHash)

	// Send to all connected peers
	for _, p := range pm.peers {
		if p.Connected() {
			p.QueueMessage(getHeadersMsg, nil)
			pm.logger.Debug("requesting headers from peer",
				"peer_id", p.Addr(),
				"our_best_hash", bestHash.String())
		}
	}
}

// PeerInfo contains information about a peer
type PeerInfo struct {
	Addr      string
	Connected bool
	Inbound   bool
}

// updatePeerMetrics updates peer count metrics
// Must be called with pm.mu held
func (pm *PeerManager) updatePeerMetrics() {
	total := uint32(len(pm.peers))
	var inbound, outbound uint32
	for _, p := range pm.peers {
		if p.Inbound() {
			inbound++
		} else {
			outbound++
		}
	}
	metrics.Get().UpdatePeerCount(total, inbound, outbound)
}

// Stop stops the peer manager
func (pm *PeerManager) Stop() {
	// Stop sync manager first
	if pm.syncManager != nil {
		pm.syncManager.Stop()
	}

	// Stop mempool cleanup
	if pm.mempool != nil {
		pm.mempool.Stop()
	}

	close(pm.quit)

	// Close all listeners
	for _, listener := range pm.listeners {
		listener.Close()
	}

	// Disconnect all peers
	pm.mu.Lock()
	for _, p := range pm.peers {
		p.Disconnect()
	}
	pm.mu.Unlock()

	pm.wg.Wait()
}
