package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/opd-ai/nmcd/chain"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/network"
	"github.com/opd-ai/nmcd/rpc"
)

func main() {
	// Parse command line flags
	cfg := parseFlags()

	// Ensure data directory exists
	if err := cfg.EnsureDataDir(); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Create blockchain
	chainCfg := &chain.Config{
		ChainParams: cfg.ChainParams(),
		NameDBPath:  cfg.NameDBPath(),
		DataDir:     cfg.DataDir,
	}

	bc, err := chain.NewBlockChain(chainCfg, nil)
	if err != nil {
		log.Fatalf("Failed to create blockchain: %v", err)
	}
	defer bc.Close()

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
		log.Fatalf("Failed to create peer manager: %v", err)
	}
	defer peerMgr.Stop()

	log.Printf("Network listening on %v", cfg.ListenAddrs)

	// Connect to initial peers
	for _, addr := range cfg.AddPeers {
		if err := peerMgr.ConnectPeer(addr); err != nil {
			log.Printf("Failed to connect to %s: %v", addr, err)
		} else {
			log.Printf("Connected to peer %s", addr)
		}
	}

	// Create RPC server
	rpcCfg := &rpc.Config{
		Blockchain: bc,
		PeerMgr:    peerMgr,
		ListenAddr: cfg.RPCAddr,
	}

	rpcServer, err := rpc.NewServer(rpcCfg)
	if err != nil {
		log.Fatalf("Failed to create RPC server: %v", err)
	}
	defer rpcServer.Stop()

	rpcServer.Start()
	log.Printf("RPC server listening on %s", cfg.RPCAddr)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Printf("Shutting down...")
}

func parseFlags() *config.Config {
	cfg := config.DefaultConfig()

	flag.StringVar(&cfg.DataDir, "datadir", cfg.DataDir, "Data directory")
	flag.StringVar(&cfg.Network, "network", cfg.Network, "Network to use (mainnet, testnet, regtest)")
	flag.StringVar(&cfg.RPCAddr, "rpcaddr", cfg.RPCAddr, "RPC server address")

	var listenAddrs string
	flag.StringVar(&listenAddrs, "listen", "0.0.0.0:8334", "Network listen addresses (comma-separated)")

	var addPeers string
	flag.StringVar(&addPeers, "addpeer", "", "Peers to connect to (comma-separated)")

	flag.IntVar(&cfg.MaxPeers, "maxpeers", cfg.MaxPeers, "Maximum number of peers")

	flag.Parse()

	// Parse listen addresses
	if listenAddrs != "" {
		cfg.ListenAddrs = []string{listenAddrs}
	}

	// Parse add peers
	if addPeers != "" {
		cfg.AddPeers = []string{addPeers}
	}

	return cfg
}

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	fmt.Println("nmcd - Pure Go Namecoin using btcd")
	fmt.Println("Version 0.1.0")
	fmt.Println()
}
