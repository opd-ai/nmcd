package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/internal/server"
)

func main() {
	// Parse command line flags
	cfg := parseFlags()

	// Create and initialize server
	srv, err := server.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	defer func() {
		if err := srv.Stop(); err != nil {
			log.Printf("Failed to stop server: %v", err)
		}
	}()

	// Start server components
	rpcErrCh, err := srv.Start()
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Wait for shutdown signal or RPC error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Printf("Shutdown signal received...")
	case err := <-rpcErrCh:
		if err != nil {
			log.Printf("RPC server error: %v", err)
		}
	}
}

func parseFlags() *config.Config {
	cfg := config.DefaultConfig()

	flag.StringVar(&cfg.DataDir, "datadir", cfg.DataDir, "Data directory")
	flag.StringVar(&cfg.Network, "network", cfg.Network, "Network to use (mainnet, testnet, regtest)")
	flag.StringVar(&cfg.RPCAddr, "rpcaddr", cfg.RPCAddr, "RPC server address")
	flag.StringVar(&cfg.RPCUser, "rpcuser", cfg.RPCUser, "RPC authentication username")
	flag.StringVar(&cfg.RPCPassword, "rpcpassword", cfg.RPCPassword, "RPC authentication password")

	var listenAddrs string
	flag.StringVar(&listenAddrs, "listen", "0.0.0.0:8334", "Network listen addresses (comma-separated)")

	var addPeers string
	flag.StringVar(&addPeers, "addpeer", "", "Peers to connect to (comma-separated)")

	flag.IntVar(&cfg.MaxPeers, "maxpeers", cfg.MaxPeers, "Maximum number of peers")

	flag.Parse()

	// Parse listen addresses (comma-separated)
	if listenAddrs != "" {
		cfg.ListenAddrs = server.SplitAndTrim(listenAddrs)
	}

	// Parse add peers (comma-separated)
	if addPeers != "" {
		cfg.AddPeers = server.SplitAndTrim(addPeers)
	}

	return cfg
}

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	fmt.Println("nmcd - Pure Go Namecoin using btcd")
	fmt.Println("Version 0.1.0")
	fmt.Println()
}
