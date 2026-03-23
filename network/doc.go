// Package network provides P2P networking for Namecoin nodes.
//
// The network package implements peer-to-peer communication using btcd's peer
// package, handling peer discovery, connection management, block and transaction
// relay, and Initial Block Download (IBD). It provides the networking layer
// that connects nmcd to the Namecoin network.
//
// # Architecture
//
// The package is organized around several key components:
//
//   - PeerManager: Manages peer connections, discovery, and lifecycle
//   - SyncManager: Handles Initial Block Download and ongoing synchronization
//   - Mempool: Stores unconfirmed transactions awaiting block inclusion
//   - PeerScoring: Rates peers based on behavior for connection prioritization
//
// # Peer Management
//
// PeerManager handles:
//
//   - Inbound and outbound peer connections
//   - DNS seed resolution for peer discovery
//   - Connection limiting and load balancing
//   - Peer message handling and routing
//   - Graceful shutdown with connection draining
//
// # Initial Block Download (IBD)
//
// The SyncManager implements headers-first synchronization:
//
//  1. Download block headers to determine chain structure
//  2. Validate header chain (PoW, timestamps, difficulty)
//  3. Request blocks in parallel from multiple peers
//  4. Process blocks and update name database
//
// This approach allows efficient sync even with limited memory by not
// requiring all block data to be held simultaneously.
//
// # Mempool
//
// The Mempool stores unconfirmed transactions with:
//
//   - Configurable size limits and expiration
//   - Name operation validation before acceptance
//   - Automatic cleanup of expired transactions
//   - Fee-based prioritization for relay
//
// # Peer Scoring
//
// Peers are scored based on behavior to prioritize reliable connections:
//
//   - Block validity: Invalid blocks decrease score
//   - Response time: Slow peers are deprioritized
//   - Data quality: Peers providing bad data are penalized
//   - Connection stability: Frequent disconnects lower score
//
// # Thread Safety
//
// All types in this package are safe for concurrent use. PeerManager uses
// sync.RWMutex for peer map access, and message handlers are designed for
// concurrent invocation.
//
// # Interface-Based Design
//
// Network connections use interface types for testability:
//
//   - net.Conn instead of *net.TCPConn
//   - net.Listener instead of *net.TCPListener
//   - net.Addr instead of concrete address types
//
// This allows mock connections in tests without actual network I/O.
//
// # Example Usage
//
// Creating a peer manager:
//
//	cfg := &network.Config{
//	    ChainParams: &config.NamecoinMainNetParams,
//	    Blockchain:  blockchain,
//	    ListenAddrs: []string{":8334"},
//	    MaxPeers:    125,
//	    AddPeers:    []string{"192.168.1.1:8334"},
//	}
//	pm, err := network.NewPeerManager(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Start peer discovery and sync
//	err = pm.Start()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer pm.Stop()
//
//	// Get connected peer count
//	count := pm.ConnectedCount()
//	fmt.Printf("Connected to %d peers\n", count)
//
// # DNS Seeds
//
// Peer discovery uses DNS seeds configured in the config package. Seeds are
// queried in parallel with fallback to hardcoded addresses if DNS fails.
//
// # Buffer Pools
//
// The package includes buffer pools (bufpool) to reduce allocation pressure
// during high-throughput message processing. Buffers are automatically
// returned to the pool after use.
package network
