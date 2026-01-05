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
	// Start with defaults
	cfg := config.DefaultConfig()

	// Parse command-line flags (to get datadir early for config file path)
	var dataDirFlag string
	flag.StringVar(&dataDirFlag, "datadir", cfg.DataDir, "Data directory")
	var configFileFlag string
	flag.StringVar(&configFileFlag, "config", "", "Path to configuration file (default: <datadir>/nmcd.conf)")

	// Parse other flags
	var networkFlag string
	flag.StringVar(&networkFlag, "network", "", "Network to use (mainnet, testnet, regtest)")
	var rpcAddrFlag string
	flag.StringVar(&rpcAddrFlag, "rpcaddr", "", "RPC server address")
	var rpcUserFlag string
	flag.StringVar(&rpcUserFlag, "rpcuser", "", "RPC authentication username")
	var rpcPasswordFlag string
	flag.StringVar(&rpcPasswordFlag, "rpcpassword", "", "RPC authentication password")
	var prometheusAddrFlag string
	flag.StringVar(&prometheusAddrFlag, "prometheusaddr", "", "Prometheus metrics HTTP endpoint address (empty = disabled)")

	var listenAddrs string
	flag.StringVar(&listenAddrs, "listen", "", "Network listen addresses (comma-separated)")

	var addPeers string
	flag.StringVar(&addPeers, "addpeer", "", "Peers to connect to (comma-separated)")

	var maxPeersFlag int
	flag.IntVar(&maxPeersFlag, "maxpeers", 0, "Maximum number of peers")

	flag.Parse()

	// Apply datadir from flag if provided
	if dataDirFlag != "" && dataDirFlag != cfg.DataDir {
		cfg.DataDir = dataDirFlag
	}

	// Load configuration file
	// Priority order: 1. Config file, 2. Environment variables, 3. Command-line flags
	configPath := configFileFlag
	if configPath == "" {
		configPath = config.ConfigPath(cfg.DataDir)
	}

	fileConfig, err := config.LoadConfigFile(configPath)
	if err != nil {
		log.Printf("Warning: Failed to load config file %s: %v", configPath, err)
		log.Printf("Continuing with defaults and command-line/environment configuration...")
	} else {
		// Apply config file settings
		cfg.ApplyFileConfig(fileConfig)
	}

	// Apply environment variables (override config file)
	cfg.ApplyEnvironmentVariables()

	// Apply command-line flags (highest priority - override everything)
	// Only apply if the flag was explicitly set (not just default value)
	if networkFlag != "" {
		cfg.Network = networkFlag
	}
	if rpcAddrFlag != "" {
		cfg.RPCAddr = rpcAddrFlag
	}
	if rpcUserFlag != "" {
		cfg.RPCUser = rpcUserFlag
	}
	if rpcPasswordFlag != "" {
		cfg.RPCPassword = rpcPasswordFlag
	}
	if prometheusAddrFlag != "" {
		cfg.PrometheusAddr = prometheusAddrFlag
	}
	if maxPeersFlag > 0 {
		cfg.MaxPeers = maxPeersFlag
	}

	// Parse listen addresses (comma-separated)
	if listenAddrs != "" {
		cfg.ListenAddrs = server.SplitAndTrim(listenAddrs)
	}

	// Parse add peers (comma-separated)
	if addPeers != "" {
		cfg.AddPeers = server.SplitAndTrim(addPeers)
	}

	// Security warning if credentials provided via command-line
	if rpcUserFlag != "" || rpcPasswordFlag != "" {
		log.Printf("Warning: RPC credentials passed via command-line flags are visible in process listings.")
		log.Printf("For production use, consider using a config file (%s) or environment variables (NMCD_RPC_USER, NMCD_RPC_PASSWORD).", configPath)
	}

	return cfg
}

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	fmt.Println("nmcd - Pure Go Namecoin using btcd")
	fmt.Println("Version 0.1.0")
	fmt.Println()
}
