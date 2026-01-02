package server

import (
	"fmt"
	"log"
	"strings"

	"github.com/opd-ai/nmcd/chain"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/network"
	"github.com/opd-ai/nmcd/rpc"
	"github.com/opd-ai/nmcd/wallet"
)

// Server represents the nmcd daemon server with all its components.
type Server struct {
	config    *config.Config
	chain     *chain.BlockChain
	peerMgr   *network.PeerManager
	wallet    *wallet.Wallet
	rpcServer *rpc.Server
}

// NewServer creates and initializes a new nmcd server instance.
// It sets up the blockchain, network, wallet, and RPC server components.
func NewServer(cfg *config.Config) (*Server, error) {
	// Ensure data directory exists
	if err := cfg.EnsureDataDir(); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Create blockchain
	chainCfg := &chain.Config{
		ChainParams: cfg.ChainParams(),
		NameDBPath:  cfg.NameDBPath(),
		DataDir:     cfg.DataDir,
	}

	bc, err := chain.NewBlockChain(chainCfg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create blockchain: %w", err)
	}

	log.Printf("Blockchain initialized")

	// Create peer manager
	netCfg := &network.Config{
		ChainParams: cfg.ChainParams(),
		Blockchain:  bc,
		ListenAddrs: cfg.ListenAddrs,
		MaxPeers:    cfg.MaxPeers,
	}

	peerMgr, err := network.NewPeerManager(netCfg)
	if err != nil {
		bc.Close() // Clean up blockchain on failure
		return nil, fmt.Errorf("failed to create peer manager: %w", err)
	}

	log.Printf("Network listening on %v", cfg.ListenAddrs)

	// Create wallet
	w, err := wallet.NewWallet(cfg.DataDir, cfg.ChainParams())
	if err != nil {
		log.Printf("Warning: Failed to initialize wallet: %v", err)
		log.Printf("Wallet functionality will be disabled")
		// Note: wallet is optional, so we continue with nil wallet
	} else {
		log.Printf("Wallet initialized")
	}

	// Create RPC server
	rpcCfg := &rpc.Config{
		Blockchain:  bc,
		PeerMgr:     peerMgr,
		Wallet:      w,
		ListenAddr:  cfg.RPCAddr,
		RPCUser:     cfg.RPCUser,
		RPCPassword: cfg.RPCPassword,
	}

	// Warn if only one of rpcuser/rpcpassword is set
	if (cfg.RPCUser != "" && cfg.RPCPassword == "") || (cfg.RPCUser == "" && cfg.RPCPassword != "") {
		log.Printf("Warning: Both -rpcuser and -rpcpassword must be set for RPC authentication. Authentication is disabled.")
	}

	// Security warning about command-line credentials
	if cfg.RPCUser != "" && cfg.RPCPassword != "" {
		log.Printf("Warning: RPC credentials passed via command-line are visible in process listings. For production, consider using environment variables or a config file.")
	}

	rpcServer, err := rpc.NewServer(rpcCfg)
	if err != nil {
		peerMgr.Stop() // Clean up peer manager on failure
		bc.Close()     // Clean up blockchain on failure
		return nil, fmt.Errorf("failed to create RPC server: %w", err)
	}

	return &Server{
		config:    cfg,
		chain:     bc,
		peerMgr:   peerMgr,
		wallet:    w,
		rpcServer: rpcServer,
	}, nil
}

// Start starts the server components.
// It connects to initial peers and starts the RPC server.
// Returns a channel that will receive the first error from the RPC server, or nil on clean shutdown.
func (s *Server) Start() (<-chan error, error) {
	// Connect to initial peers (from -addpeer flag or DNS seeds)
	peersToConnect := s.config.AddPeers
	if len(peersToConnect) == 0 {
		// No peers specified, try DNS seed discovery
		seeds := config.DNSSeeds(s.config.Network)
		if len(seeds) > 0 {
			log.Printf("No peers specified, resolving DNS seeds for %s...", s.config.Network)
			seedAddrs := network.ResolveSeedNodes(seeds, config.DefaultPort(s.config.Network))
			if len(seedAddrs) > 0 {
				log.Printf("Discovered %d peer addresses from DNS seeds", len(seedAddrs))
				peersToConnect = seedAddrs
			} else {
				log.Printf("Warning: No peers discovered from DNS seeds")
			}
		}
	}

	for _, addr := range peersToConnect {
		if err := s.peerMgr.ConnectPeer(addr); err != nil {
			log.Printf("Failed to connect to %s: %v", addr, err)
		} else {
			log.Printf("Connected to peer %s", addr)
		}
	}

	// Start RPC server
	rpcErrCh := s.rpcServer.Start()
	log.Printf("RPC server listening on %s", s.config.RPCAddr)

	return rpcErrCh, nil
}

// Stop gracefully shuts down the server and all its components.
func (s *Server) Stop() error {
	log.Printf("Shutting down server...")

	// Stop components in reverse order of creation
	if s.rpcServer != nil {
		s.rpcServer.Stop()
	}

	if s.peerMgr != nil {
		s.peerMgr.Stop()
	}

	if s.chain != nil {
		if err := s.chain.Close(); err != nil {
			return fmt.Errorf("failed to close blockchain: %w", err)
		}
	}

	log.Printf("Server stopped")
	return nil
}

// Config returns the server's configuration.
func (s *Server) Config() *config.Config {
	return s.config
}

// Blockchain returns the server's blockchain instance.
func (s *Server) Blockchain() *chain.BlockChain {
	return s.chain
}

// PeerManager returns the server's peer manager instance.
func (s *Server) PeerManager() *network.PeerManager {
	return s.peerMgr
}

// Wallet returns the server's wallet instance (may be nil).
func (s *Server) Wallet() *wallet.Wallet {
	return s.wallet
}

// RPCServer returns the server's RPC server instance.
func (s *Server) RPCServer() *rpc.Server {
	return s.rpcServer
}

// SplitAndTrim splits a comma-separated string and trims whitespace from each element.
// Empty elements are filtered out. This is a utility function for parsing command-line arguments.
func SplitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
