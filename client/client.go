package client

import (
	"context"
	"fmt"
	"time"
)

// NewClient creates a new NameClient based on the configuration.
// It automatically selects between embedded and daemon modes based on
// the Mode field in the configuration.
//
// Mode selection logic:
//   - ModeAuto (default): Tries to connect to daemon first, falls back to embedded
//   - ModeEmbedded: Forces embedded mode (in-process)
//   - ModeDaemon: Forces daemon mode (RPC only)
//
// Parameters:
//   - cfg: Client configuration. If nil, uses default configuration with ModeAuto.
//
// Returns:
//   - NameClient: Initialized client (either EmbeddedClient or DaemonClient)
//   - error: Initialization error, or nil on success
//
// Example:
//
//	// Auto-detection (recommended for most applications)
//	client, err := nmcd.NewClient(nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	// Explicit embedded mode
//	client, err := nmcd.NewClient(&nmcd.Config{
//	    Mode:    nmcd.ModeEmbedded,
//	    DataDir: "/path/to/data",
//	})
//
//	// Explicit daemon mode
//	client, err := nmcd.NewClient(&nmcd.Config{
//	    Mode:        nmcd.ModeDaemon,
//	    RPCAddr:     "http://localhost:8336",
//	    RPCUser:     "user",
//	    RPCPassword: "pass",
//	})
func NewClient(cfg *Config) (NameClient, error) {
	cfg = resolveClientConfig(cfg)

	switch cfg.Mode {
	case ModeEmbedded:
		return NewEmbeddedClient(cfg)
	case ModeDaemon:
		return NewDaemonClient(cfg)
	case ModeAuto:
		return newAutoClient(cfg)
	default:
		return nil, fmt.Errorf("invalid client mode: %d", cfg.Mode)
	}
}

func resolveClientConfig(cfg *Config) *Config {
	if cfg == nil {
		return defaultConfig()
	}
	return cfg
}

func newAutoClient(cfg *Config) (NameClient, error) {
	daemonClient, err := tryDaemonClient(cfg)
	if err != nil {
		return nil, err
	}
	if daemonClient != nil {
		return daemonClient, nil
	}
	return NewEmbeddedClient(cfg)
}

func tryDaemonClient(cfg *Config) (*DaemonClient, error) {
	daemonClient, err := NewDaemonClient(cfg)
	if err != nil {
		return nil, nil
	}

	ok, err := validateDaemonClient(cfg, daemonClient)
	if !ok || err != nil {
		daemonClient.Close()
		return nil, err
	}
	return daemonClient, nil
}

func validateDaemonClient(cfg *Config, daemonClient *DaemonClient) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := daemonClient.Ping(ctx); err != nil {
		return false, nil
	}
	return validateDaemonNetwork(ctx, cfg, daemonClient)
}

func validateDaemonNetwork(ctx context.Context, cfg *Config, daemonClient *DaemonClient) (bool, error) {
	detectedNetwork, err := daemonClient.DetectNetwork(ctx)
	if err != nil {
		return false, nil
	}

	expectedNetwork := cfg.Network
	if expectedNetwork == "" {
		expectedNetwork = "mainnet"
	}
	if detectedNetwork != expectedNetwork {
		return false, fmt.Errorf("network mismatch: daemon is running on %s but client configured for %s (ensure cfg.Network matches daemon's network or use explicit mode)", detectedNetwork, expectedNetwork)
	}
	return true, nil
}
