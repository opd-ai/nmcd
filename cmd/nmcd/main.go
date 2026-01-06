package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/internal/logging"
	"github.com/opd-ai/nmcd/internal/server"
)

func main() {
	// Parse command line flags
	cfg := parseFlags()

	// Initialize structured logging
	logCfg := logging.DefaultConfig()
	logCfg.Level = logging.LogLevel(cfg.LogLevel)
	logCfg.Format = cfg.LogFormat
	logCfg.Output = cfg.LogOutput
	logCfg.Component = "nmcd"
	logCfg.EnableRotation = cfg.LogRotation
	logCfg.MaxSizeMB = cfg.LogMaxSizeMB

	logger, err := logging.Init(logCfg)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	// Set as default logger for the application
	logging.SetDefault(logger)

	logger.Info("nmcd starting",
		"version", "0.1.0",
		"network", cfg.Network,
		"data_dir", cfg.DataDir,
	)

	// Create and initialize server
	srv, err := server.NewServer(cfg)
	if err != nil {
		logger.Error("Failed to create server", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := srv.Stop(); err != nil {
			logger.Warn("Failed to stop server cleanly", "error", err)
		}
	}()

	// Start server components
	rpcErrCh, err := srv.Start()
	if err != nil {
		logger.Error("Failed to start server", "error", err)
		os.Exit(1)
	}

	logger.Info("nmcd started successfully",
		"rpc_addr", cfg.RPCAddr,
		"prometheus_addr", cfg.PrometheusAddr,
		"listen_addrs", cfg.ListenAddrs,
	)

	// Wait for shutdown signal or RPC error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Info("Shutdown signal received", "signal", sig)
	case err := <-rpcErrCh:
		if err != nil {
			logger.Error("RPC server error", "error", err)
		}
	}

	logger.Info("nmcd shutting down...")
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

	// Logging flags
	var logLevelFlag string
	flag.StringVar(&logLevelFlag, "loglevel", "", "Log level: DEBUG, INFO, WARN, ERROR")
	var logFormatFlag string
	flag.StringVar(&logFormatFlag, "logformat", "", "Log format: text, json")
	var logOutputFlag string
	flag.StringVar(&logOutputFlag, "logoutput", "", "Log output: stdout, stderr, or file path")

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
	if logLevelFlag != "" {
		cfg.LogLevel = logLevelFlag
	}
	if logFormatFlag != "" {
		cfg.LogFormat = logFormatFlag
	}
	if logOutputFlag != "" {
		cfg.LogOutput = logOutputFlag
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
