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
	if cfg == nil {
		cfg = defaultConfig()
	}

	switch cfg.Mode {
	case ModeEmbedded:
		return NewEmbeddedClient(cfg)

	case ModeDaemon:
		return NewDaemonClient(cfg)

	case ModeAuto:
		// Try daemon first
		daemonClient, err := NewDaemonClient(cfg)
		if err == nil {
			// Check if daemon is responsive
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := daemonClient.Ping(ctx); err == nil {
				// Daemon is available and responsive
				return daemonClient, nil
			}

			// Daemon not responsive, close and fall back to embedded
			daemonClient.Close()
		}

		// Fall back to embedded mode
		return NewEmbeddedClient(cfg)

	default:
		return nil, fmt.Errorf("invalid client mode: %d", cfg.Mode)
	}
}
