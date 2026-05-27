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
	"github.com/opd-ai/nmcd/internal/version"
)

// version may be overridden via ldflags during build (e.g. -ldflags "-X main.appVersion=...")
var appVersion = version.Version

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
		"version", appVersion,
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
	defer signal.Stop(sigChan)

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
	cfg := config.DefaultConfig()
	flags := registerFlags(cfg)
	flag.Parse()

	applyDataDir(cfg, flags)
	loadAndApplyConfigFile(cfg, flags.configFile)
	cfg.ApplyEnvironmentVariables()
	applyFlagOverrides(cfg, flags)

	if flags.rpcUser != "" || flags.rpcPassword != "" {
		log.Printf("Warning: RPC credentials passed via command-line flags are visible in process listings.")
		configPath := flags.configFile
		if configPath == "" {
			configPath = config.ConfigPath(cfg.DataDir)
		}
		log.Printf("For production use, consider using a config file (%s) or environment variables (NMCD_RPC_USER, NMCD_RPC_PASSWORD).", configPath)
	}

	return cfg
}

// cliFlags holds all command-line flag values.
type cliFlags struct {
	dataDir        string
	configFile     string
	network        string
	rpcAddr        string
	rpcUser        string
	rpcPassword    string
	prometheusAddr string
	listenAddrs    string
	addPeers       string
	maxPeers       int
	logLevel       string
	logFormat      string
	logOutput      string
}

// registerFlags registers all CLI flags and returns the flags struct.
func registerFlags(cfg *config.Config) *cliFlags {
	f := &cliFlags{}
	flag.StringVar(&f.dataDir, "datadir", cfg.DataDir, "Data directory")
	flag.StringVar(&f.configFile, "config", "", "Path to configuration file (default: <datadir>/nmcd.conf)")
	flag.StringVar(&f.network, "network", "", "Network to use (mainnet, testnet, regtest)")
	flag.StringVar(&f.rpcAddr, "rpcaddr", "", "RPC server address")
	flag.StringVar(&f.rpcUser, "rpcuser", "", "RPC authentication username")
	flag.StringVar(&f.rpcPassword, "rpcpassword", "", "RPC authentication password")
	flag.StringVar(&f.prometheusAddr, "prometheusaddr", "", "Prometheus metrics HTTP endpoint address (empty = disabled)")
	flag.StringVar(&f.listenAddrs, "listen", "", "Network listen addresses (comma-separated)")
	flag.StringVar(&f.addPeers, "addpeer", "", "Peers to connect to (comma-separated)")
	flag.IntVar(&f.maxPeers, "maxpeers", 0, "Maximum number of peers")
	flag.StringVar(&f.logLevel, "loglevel", "", "Log level: DEBUG, INFO, WARN, ERROR")
	flag.StringVar(&f.logFormat, "logformat", "", "Log format: text, json")
	flag.StringVar(&f.logOutput, "logoutput", "", "Log output: stdout, stderr, or file path")
	return f
}

// applyDataDir applies the data directory flag if explicitly set.
func applyDataDir(cfg *config.Config, f *cliFlags) {
	if f.dataDir != "" && f.dataDir != cfg.DataDir {
		cfg.DataDir = f.dataDir
	}
}

// loadAndApplyConfigFile loads and applies settings from the config file.
func loadAndApplyConfigFile(cfg *config.Config, configFileFlag string) {
	configPath := configFileFlag
	if configPath == "" {
		configPath = config.ConfigPath(cfg.DataDir)
	}
	fileConfig, err := config.LoadConfigFile(configPath)
	if err != nil {
		log.Printf("Warning: Failed to load config file %s: %v", configPath, err)
		log.Printf("Continuing with defaults and command-line/environment configuration...")
		return
	}
	cfg.ApplyFileConfig(fileConfig)
}

// applyFlagOverrides applies command-line flags that were explicitly set (non-empty/non-zero).
// A table-driven approach maps each flag to its setter, keeping complexity flat.
func applyFlagOverrides(cfg *config.Config, f *cliFlags) {
	applyStringFlagOverrides(cfg, f)
	if f.maxPeers > 0 {
		cfg.MaxPeers = f.maxPeers
	}
}

// stringFlagOverride pairs a source string flag value with the config field to write.
type stringFlagOverride struct {
	src string
	dst *string
}

// applyStringFlagOverrides copies non-empty CLI string flags into cfg.
func applyStringFlagOverrides(cfg *config.Config, f *cliFlags) {
	for _, sf := range []stringFlagOverride{
		{f.network, &cfg.Network},
		{f.rpcAddr, &cfg.RPCAddr},
		{f.rpcUser, &cfg.RPCUser},
		{f.rpcPassword, &cfg.RPCPassword},
		{f.prometheusAddr, &cfg.PrometheusAddr},
		{f.logLevel, &cfg.LogLevel},
		{f.logFormat, &cfg.LogFormat},
		{f.logOutput, &cfg.LogOutput},
	} {
		if sf.src != "" {
			*sf.dst = sf.src
		}
	}
	if f.listenAddrs != "" {
		cfg.ListenAddrs = server.SplitAndTrim(f.listenAddrs)
	}
	if f.addPeers != "" {
		cfg.AddPeers = server.SplitAndTrim(f.addPeers)
	}
}

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	fmt.Println("nmcd - Pure Go Namecoin using btcd")
	fmt.Printf("Version %s\n", appVersion)
	fmt.Println()
}
