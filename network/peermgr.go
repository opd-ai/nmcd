package network

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/chain"
)

// PeerManager manages network peers using btcd/peer
type PeerManager struct {
	peers       map[string]*peer.Peer
	listeners   []net.Listener
	blockchain  *chain.BlockChain
	chainParams *chaincfg.Params
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
}

// NewPeerManager creates a new peer manager
func NewPeerManager(cfg *Config) (*PeerManager, error) {
	pm := &PeerManager{
		peers:       make(map[string]*peer.Peer),
		blockchain:  cfg.Blockchain,
		chainParams: cfg.ChainParams,
		quit:        make(chan struct{}),
	}

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

	return pm, nil
}

// listenLoop accepts incoming connections
func (pm *PeerManager) listenLoop(listener net.Listener) {
	defer pm.wg.Done()

	for {
		select {
		case <-pm.quit:
			return
		default:
		}

		// Set accept deadline to allow checking quit channel
		if tcpListener, ok := listener.(*net.TCPListener); ok {
			tcpListener.SetDeadline(time.Now().Add(time.Second))
		}

		conn, err := listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			continue
		}

		pm.wg.Add(1)
		go pm.handleInboundPeer(conn)
	}
}

// handleInboundPeer handles an inbound peer connection
func (pm *PeerManager) handleInboundPeer(conn net.Conn) {
	defer pm.wg.Done()

	// Create peer configuration
	peerCfg := &peer.Config{
		UserAgentName:    "nmcd",
		UserAgentVersion: "0.1.0",
		ChainParams:      pm.chainParams,
		Services:         wire.SFNodeNetwork,
		TrickleInterval:  time.Second * 10,
		Listeners: peer.MessageListeners{
			OnVersion: pm.onVersion,
			OnVerAck:  pm.onVerAck,
			OnInv:     pm.onInv,
			OnBlock:   pm.onBlock,
			OnTx:      pm.onTx,
			OnGetData: pm.onGetData,
		},
	}

	p := peer.NewInboundPeer(peerCfg)
	p.AssociateConnection(conn)

	// Add to peer list
	pm.mu.Lock()
	pm.peers[p.Addr()] = p
	pm.mu.Unlock()

	// Wait for disconnect
	p.WaitForDisconnect()

	// Remove from peer list
	pm.mu.Lock()
	delete(pm.peers, p.Addr())
	pm.mu.Unlock()
}

// ConnectPeer connects to an outbound peer
func (pm *PeerManager) ConnectPeer(addr string) error {
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
		TrickleInterval:  time.Second * 10,
		Listeners: peer.MessageListeners{
			OnVersion: pm.onVersion,
			OnVerAck:  pm.onVerAck,
			OnInv:     pm.onInv,
			OnBlock:   pm.onBlock,
			OnTx:      pm.onTx,
			OnGetData: pm.onGetData,
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
	pm.mu.Unlock()

	pm.wg.Add(1)
	go func() {
		defer pm.wg.Done()
		p.WaitForDisconnect()

		pm.mu.Lock()
		delete(pm.peers, p.Addr())
		pm.mu.Unlock()
	}()

	return nil
}

// Message handlers
func (pm *PeerManager) onVersion(p *peer.Peer, msg *wire.MsgVersion) *wire.MsgReject {
	// Handle version message
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
	// Handle block message - would process with blockchain
	// This is where we'd call blockchain.ProcessBlock
}

func (pm *PeerManager) onTx(p *peer.Peer, msg *wire.MsgTx) {
	// Handle transaction message
}

func (pm *PeerManager) onGetData(p *peer.Peer, msg *wire.MsgGetData) {
	// Handle getdata message - send requested blocks/transactions
	for _, inv := range msg.InvList {
		switch inv.Type {
		case wire.InvTypeBlock:
			// Would fetch and send block
		case wire.InvTypeTx:
			// Would fetch and send transaction
		}
	}
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
func (pm *PeerManager) BroadcastTx(tx *wire.MsgTx) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	inv := wire.NewMsgInv()
	txHash := tx.TxHash()
	inv.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &txHash))

	for _, p := range pm.peers {
		p.QueueMessage(inv, nil)
	}
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

// PeerInfo contains information about a peer
type PeerInfo struct {
	Addr      string
	Connected bool
	Inbound   bool
}

// Stop stops the peer manager
func (pm *PeerManager) Stop() {
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
