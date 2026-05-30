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
//
// LOCK ORDERING: sm.mu (SyncManager.mu) must be acquired before pm.mu (PeerManager.mu).
// This ordering is enforced in findReplacementPeer, which is called with sm.mu held.
// Code must not acquire locks in the reverse order to prevent deadlock.
type SyncManager struct {
	pm         *PeerManager
	blockchain *chain.BlockChain

	// Sync state
	mu               sync.RWMutex
	syncPeer         *peer.Peer                   // Current peer we're syncing from
	headersFirstMode bool                         // True during IBD, false during normal operation
	requestedBlocks  map[chainhash.Hash]time.Time // Track block requests to avoid duplicates

	// Best known height from peers
	bestHeight  int32
	bestPeer    *peer.Peer
	peerHeights map[*peer.Peer]int32

	quit     chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once
}

// NewSyncManager creates a new sync manager
func NewSyncManager(pm *PeerManager) *SyncManager {
	sm := &SyncManager{
		pm:               pm,
		blockchain:       pm.blockchain,
		headersFirstMode: true, // Start in IBD mode
		requestedBlocks:  make(map[chainhash.Hash]time.Time),
		peerHeights:      make(map[*peer.Peer]int32),
		quit:             make(chan struct{}),
	}

	// Start the sync loop
	sm.wg.Add(1)
	go sm.syncLoop()

	return sm
}

// Stop stops the sync manager.
// Safe to call multiple times; only the first call has effect.
func (sm *SyncManager) Stop() {
	sm.stopOnce.Do(func() { close(sm.quit) })
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

// syncTick runs periodically to maintain sync state.
func (sm *SyncManager) syncTick() {
	sm.mu.RLock()
	blockchain := sm.blockchain
	sm.mu.RUnlock()
	if blockchain == nil {
		return
	}

	ourHeight := blockchain.BestSnapshot().Height
	var peerToSync *peer.Peer
	var peerHeight int32

	sm.mu.Lock()
	if sm.bestHeight > ourHeight {
		if sm.syncPeer == nil {
			if sm.bestPeer == nil {
				sm.bestPeer = sm.findReplacementPeer(nil)
			}
			if sm.bestPeer != nil {
				sm.syncPeer = sm.bestPeer
				peerToSync = sm.syncPeer
				peerHeight = sm.bestHeight
			}
		}
	} else if sm.headersFirstMode {
		log.Printf("Initial Block Download complete. Synced to height %d", ourHeight)
		sm.headersFirstMode = false
		sm.syncPeer = nil
	}
	sm.cleanupOldRequestsLocked(time.Now())
	sm.mu.Unlock()

	if peerToSync != nil {
		log.Printf("Starting sync with peer %s (height: %d)", peerToSync.Addr(), peerHeight)
		sm.requestHeaders(peerToSync)
	}
}

// startSync begins syncing from the specified peer.
func (sm *SyncManager) startSync(p *peer.Peer) {
	if p == nil {
		return
	}

	sm.mu.Lock()
	sm.syncPeer = p
	peerHeight := sm.bestHeight
	sm.mu.Unlock()

	log.Printf("Starting sync with peer %s (height: %d)", p.Addr(), peerHeight)
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

// HandleHeaders processes a headers message from a peer.
func (sm *SyncManager) HandleHeaders(p *peer.Peer, msg *wire.MsgHeaders) {
	headerCount := len(msg.Headers)
	if headerCount == 0 {
		logEmptyHeaders(p)
		return
	}

	blockchain, headers, ok := sm.prepareHeaders(p, msg)
	if !ok {
		return
	}

	log.Printf("Received %d headers from %s", headerCount, p.Addr())
	sm.requestMissingHeaderBlocks(blockchain, p, headers)
	if headerCount == wire.MaxBlockHeadersPerMsg {
		sm.requestHeaders(p)
	}
}

func logEmptyHeaders(p *peer.Peer) {
	if p != nil {
		log.Printf("Received empty headers message from %s", p.Addr())
		return
	}
	log.Printf("Received empty headers message")
}

func (sm *SyncManager) prepareHeaders(p *peer.Peer, msg *wire.MsgHeaders) (*chain.BlockChain, []*wire.BlockHeader, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.blockchain == nil || p == nil {
		return nil, nil, false
	}
	if !sm.acceptsHeadersFromPeer(p) {
		return nil, nil, false
	}
	return sm.blockchain, append([]*wire.BlockHeader(nil), msg.Headers...), true
}

func (sm *SyncManager) acceptsHeadersFromPeer(p *peer.Peer) bool {
	if !sm.headersFirstMode {
		return true
	}
	if sm.syncPeer == nil {
		log.Printf("Ignoring headers from %s: no active sync peer", p.Addr())
		return false
	}
	if p.Addr() != sm.syncPeer.Addr() {
		log.Printf("Ignoring headers from non-sync peer %s (sync peer is %s)", p.Addr(), sm.syncPeer.Addr())
		return false
	}
	return true
}

func (sm *SyncManager) requestMissingHeaderBlocks(blockchain *chain.BlockChain, p *peer.Peer, headers []*wire.BlockHeader) {
	for _, header := range headers {
		blockHash := header.BlockHash()
		if !sm.shouldRequestBlock(blockchain, &blockHash) {
			continue
		}
		sm.requestBlock(p, &blockHash)
	}
}

func (sm *SyncManager) shouldRequestBlock(blockchain *chain.BlockChain, blockHash *chainhash.Hash) bool {
	sm.mu.RLock()
	_, exists := sm.requestedBlocks[*blockHash]
	sm.mu.RUnlock()
	if exists {
		return false
	}
	_, err := blockchain.BlockByHash(blockHash)
	return err != nil
}

// requestBlock requests a full block from a peer.
func (sm *SyncManager) requestBlock(p *peer.Peer, hash *chainhash.Hash) {
	if p == nil || hash == nil {
		return
	}

	sm.mu.Lock()
	if _, exists := sm.requestedBlocks[*hash]; exists {
		sm.mu.Unlock()
		return
	}
	sm.requestedBlocks[*hash] = time.Now()
	sm.mu.Unlock()

	msg := wire.NewMsgGetData()
	msg.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, hash))
	p.QueueMessage(msg, nil)
}

// UpdatePeerHeight updates the best known height from a peer.
func (sm *SyncManager) UpdatePeerHeight(p *peer.Peer, height int32) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.peerHeights == nil {
		sm.peerHeights = make(map[*peer.Peer]int32)
	}
	if p != nil {
		sm.peerHeights[p] = height
	}
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
func (sm *SyncManager) OnPeerDisconnected(p *peer.Peer) {
	if p == nil {
		return
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.syncPeer != nil && sm.syncPeer == p {
		log.Printf("Sync peer %s disconnected, will reselect", p.Addr())
		sm.syncPeer = nil
	}

	delete(sm.peerHeights, p)
	if sm.bestPeer != nil && sm.bestPeer == p {
		log.Printf("Best peer %s disconnected, will reselect", p.Addr())
		sm.bestHeight = sm.maxPeerHeightLocked()
		sm.bestPeer = nil
	}
}

// findReplacementPeer selects another connected peer to continue syncing.
// This method must be called while holding sm.mu lock.
func (sm *SyncManager) findReplacementPeer(disconnected *peer.Peer) *peer.Peer {
	if sm.pm == nil {
		return nil
	}

	var fallback *peer.Peer
	var replacement *peer.Peer
	var bestHeight int32 = -1

	sm.pm.mu.RLock()
	defer sm.pm.mu.RUnlock()

	for _, candidate := range sm.pm.peers {
		if candidate == nil {
			continue
		}
		if disconnected != nil && candidate == disconnected {
			continue
		}
		if fallback == nil {
			fallback = candidate
		}
		if height, ok := sm.peerHeights[candidate]; ok && height > bestHeight {
			bestHeight = height
			replacement = candidate
		}
	}
	if replacement != nil {
		return replacement
	}
	return fallback
}

func (sm *SyncManager) maxPeerHeightLocked() int32 {
	var maxHeight int32
	for _, height := range sm.peerHeights {
		if height > maxHeight {
			maxHeight = height
		}
	}
	return maxHeight
}

// cleanupOldRequests removes block requests older than 2 minutes.
func (sm *SyncManager) cleanupOldRequests() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.cleanupOldRequestsLocked(time.Now())
}

// cleanupOldRequestsLocked removes block requests older than 2 minutes.
func (sm *SyncManager) cleanupOldRequestsLocked(now time.Time) {
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
