package network

import (
	"log"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/chain"
)

// SyncManager handles Initial Block Download (IBD) and ongoing block synchronization.
// It implements the Bitcoin/Namecoin sync protocol:
// 1. Request headers from peers using getheaders
// 2. Process received headers to learn about the chain
// 3. Request full blocks for headers we don't have
// 4. Transition from IBD to normal operation when caught up
type SyncManager struct {
	pm         *PeerManager
	blockchain *chain.BlockChain

	// Sync state
	mu               sync.RWMutex
	syncPeer         *peer.Peer                   // Current peer we're syncing from
	headersFirstMode bool                         // True during IBD, false during normal operation
	requestedBlocks  map[chainhash.Hash]time.Time // Track block requests to avoid duplicates

	// Best known height from peers
	bestHeight int32
	bestPeer   *peer.Peer

	quit chan struct{}
	wg   sync.WaitGroup
}

// NewSyncManager creates a new sync manager
func NewSyncManager(pm *PeerManager) *SyncManager {
	sm := &SyncManager{
		pm:               pm,
		blockchain:       pm.blockchain,
		headersFirstMode: true, // Start in IBD mode
		requestedBlocks:  make(map[chainhash.Hash]time.Time),
		quit:             make(chan struct{}),
	}

	// Start the sync loop
	sm.wg.Add(1)
	go sm.syncLoop()

	return sm
}

// Stop stops the sync manager
func (sm *SyncManager) Stop() {
	close(sm.quit)
	sm.wg.Wait()
}

// syncLoop is the main sync orchestration loop
func (sm *SyncManager) syncLoop() {
	defer sm.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sm.quit:
			return
		case <-ticker.C:
			sm.syncTick()
		}
	}
}

// syncTick runs periodically to maintain sync state
func (sm *SyncManager) syncTick() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Skip if blockchain is not initialized
	if sm.blockchain == nil {
		return
	}

	// Get our current best block height
	bestSnapshot := sm.blockchain.BestSnapshot()
	ourHeight := bestSnapshot.Height

	// Check if we need to sync
	if sm.bestHeight > ourHeight {
		// We're behind, start syncing
		if sm.syncPeer == nil && sm.bestPeer != nil {
			sm.startSync(sm.bestPeer)
		}
	} else if sm.headersFirstMode {
		// We're caught up, exit IBD mode
		log.Printf("Initial Block Download complete. Synced to height %d", ourHeight)
		sm.headersFirstMode = false
		sm.syncPeer = nil
	}

	// Clean up old block requests (>2 minutes old)
	sm.cleanupOldRequests()
}

// startSync begins syncing from the specified peer
func (sm *SyncManager) startSync(p *peer.Peer) {
	if p == nil {
		return
	}
	sm.syncPeer = p
	log.Printf("Starting sync with peer %s (height: %d)", p.Addr(), sm.bestHeight)

	// Request headers from this peer
	sm.requestHeaders(p)
}

// requestHeaders sends a getheaders request to a peer
func (sm *SyncManager) requestHeaders(p *peer.Peer) {
	// Ensure blockchain is initialized before using it
	if sm.blockchain == nil {
		if p != nil {
			log.Printf("Cannot request headers from %s: blockchain is not initialized", p.Addr())
		} else {
			log.Printf("Cannot request headers: blockchain is not initialized")
		}
		return
	}

	if p == nil {
		log.Printf("Cannot request headers: no peer provided")
		return
	}

	// Get our latest block locator
	locator, err := sm.blockchain.LatestBlockLocator()
	if err != nil {
		log.Printf("Failed to get block locator: %v", err)
		return
	}

	// Create getheaders message
	msg := wire.NewMsgGetHeaders()
	msg.BlockLocatorHashes = locator
	msg.HashStop = chainhash.Hash{} // Empty hash means "send all you have"

	// Send to peer
	p.QueueMessage(msg, nil)
	log.Printf("Sent getheaders request to %s", p.Addr())
}

// HandleHeaders processes a headers message from a peer
// This is called by the PeerManager when headers are received
func (sm *SyncManager) HandleHeaders(p *peer.Peer, msg *wire.MsgHeaders) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	headerCount := len(msg.Headers)
	if headerCount == 0 {
		if p != nil {
			log.Printf("Received empty headers message from %s", p.Addr())
		} else {
			log.Printf("Received empty headers message")
		}
		return
	}

	// Skip processing if we don't have a blockchain or peer
	if sm.blockchain == nil || p == nil {
		return
	}

	// Only accept headers from the active sync peer to prevent spam.
	// During headers-first sync, reject all headers unless a sync peer is selected.
	if sm.headersFirstMode {
		if sm.syncPeer == nil {
			log.Printf("Ignoring headers from %s: no active sync peer", p.Addr())
			return
		}
		if p.Addr() != sm.syncPeer.Addr() {
			log.Printf("Ignoring headers from non-sync peer %s (sync peer is %s)", p.Addr(), sm.syncPeer.Addr())
			return
		}
	}

	if p != nil {
		log.Printf("Received %d headers from %s", headerCount, p.Addr())
	} else {
		log.Printf("Received %d headers", headerCount)
	}

	// Process each header by requesting the full block
	// In headers-first sync, we validate headers first, then download blocks
	for _, header := range msg.Headers {
		blockHash := header.BlockHash()

		// Skip if we already requested this block recently
		if _, exists := sm.requestedBlocks[blockHash]; exists {
			continue
		}

		// Check if we already have this block
		_, err := sm.blockchain.BlockByHash(&blockHash)
		if err == nil {
			// We already have this block, skip it
			continue
		}

		// Request the full block
		sm.requestBlock(p, &blockHash)
	}

	// If we received max headers (2000), there may be more available
	// Request more headers to continue the chain
	if headerCount == wire.MaxBlockHeadersPerMsg {
		sm.requestHeaders(p)
	}
}

// requestBlock requests a full block from a peer
// This method must be called while holding sm.mu lock
func (sm *SyncManager) requestBlock(p *peer.Peer, hash *chainhash.Hash) {
	if p == nil {
		return
	}

	// Mark as requested
	sm.requestedBlocks[*hash] = time.Now()

	// Create getdata message for the block
	msg := wire.NewMsgGetData()
	msg.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, hash))

	// Send to peer
	p.QueueMessage(msg, nil)
}

// UpdatePeerHeight updates the best known height from a peer
// This is called when we receive a version message from a peer
func (sm *SyncManager) UpdatePeerHeight(p *peer.Peer, height int32) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if height > sm.bestHeight {
		sm.bestHeight = height
		sm.bestPeer = p
		if p != nil {
			log.Printf("Updated best known height to %d from peer %s", height, p.Addr())
		} else {
			log.Printf("Updated best known height to %d", height)
		}
	}
}

// IsSyncing returns whether we're currently syncing
func (sm *SyncManager) IsSyncing() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.syncPeer != nil
}

// OnPeerDisconnected is called when a peer disconnects.
// If the disconnected peer was the sync peer, clear it so a new peer can be selected.
func (sm *SyncManager) OnPeerDisconnected(p *peer.Peer) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Clear sync peer if it disconnected
	if sm.syncPeer != nil && sm.syncPeer.Addr() == p.Addr() {
		log.Printf("Sync peer %s disconnected, will reselect", p.Addr())
		sm.syncPeer = nil
	}

	// Clear best peer if it disconnected
	if sm.bestPeer != nil && sm.bestPeer.Addr() == p.Addr() {
		log.Printf("Best peer %s disconnected, will reselect", p.Addr())
		sm.bestPeer = sm.findReplacementPeer(p)
	}
}

// findReplacementPeer selects another connected peer to continue syncing.
// This method must be called while holding sm.mu lock.
func (sm *SyncManager) findReplacementPeer(disconnected *peer.Peer) *peer.Peer {
	if sm.pm == nil {
		return nil
	}

	sm.pm.mu.RLock()
	defer sm.pm.mu.RUnlock()

	for _, candidate := range sm.pm.peers {
		if candidate == nil {
			continue
		}
		if disconnected != nil && candidate.Addr() == disconnected.Addr() {
			continue
		}
		return candidate
	}

	return nil
}

// cleanupOldRequests removes block requests older than 2 minutes
// This method must be called while holding sm.mu lock
func (sm *SyncManager) cleanupOldRequests() {
	now := time.Now()
	for hash, requestTime := range sm.requestedBlocks {
		if now.Sub(requestTime) > 2*time.Minute {
			delete(sm.requestedBlocks, hash)
		}
	}
}

// BlockReceived notifies the sync manager that a block was received
// This allows us to remove it from the requested blocks map
func (sm *SyncManager) BlockReceived(hash *chainhash.Hash) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.requestedBlocks, *hash)
}
