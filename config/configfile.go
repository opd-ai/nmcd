package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// FileConfig represents the structure of the nmcd.conf configuration file.
// This allows users to specify RPC credentials and other settings in a file
// rather than via command-line flags, which is more secure as credentials
// won't be visible in process listings.
type FileConfig struct {
	// RPC server settings
	RPC RPCConfig `toml:"rpc"`

	// Network settings
	Network NetworkConfig `toml:"network"`

	// General settings
	DataDir        string `toml:"datadir"`        // Data directory path
	PrometheusAddr string `toml:"prometheusaddr"` // Prometheus metrics endpoint address
	MaxPeers       int    `toml:"maxpeers"`       // Maximum number of peer connections
}

// RPCConfig holds RPC server configuration.
type RPCConfig struct {
	User     string `toml:"user"`     // RPC authentication username
	Password string `toml:"password"` // RPC authentication password
	Addr     string `toml:"addr"`     // RPC server listen address (e.g., "127.0.0.1:8336")
}

// NetworkConfig holds network configuration.
type NetworkConfig struct {
	Type    string   `toml:"type"`    // Network type: mainnet, testnet, or regtest
	Listen  []string `toml:"listen"`  // Network listen addresses
	AddPeer []string `toml:"addpeer"` // Peers to connect to on startup
}

// ConfigPath returns the default path to the nmcd.conf configuration file.
// The file is expected to be in the data directory (e.g., ~/.nmcd/nmcd.conf).
func ConfigPath(dataDir string) string {
	return filepath.Join(dataDir, "nmcd.conf")
}

// LoadConfigFile reads and parses the nmcd.conf configuration file.
// If the file doesn't exist, it returns an empty FileConfig with no error.
// This allows the daemon to work without a config file using defaults.
//
// If the config file contains an RPC password and has world-readable permissions
// (other-read bit set), LoadConfigFile returns an error to prevent credential
// exposure on shared hosts.
func LoadConfigFile(path string) (*FileConfig, error) {
	// Check if file exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		// File doesn't exist - return empty config (not an error)
		return &FileConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to stat config file %s: %w", path, err)
	}

	// Read and parse config file
	var cfg FileConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	// Reject world-readable config files that contain RPC credentials.
	if cfg.RPC.Password != "" {
		mode := info.Mode().Perm()
		if mode&0o004 != 0 {
			return nil, fmt.Errorf("config file %s contains RPC password but has world-readable permissions (%04o); set mode to 0600", path, mode)
		}
	}

	return &cfg, nil
}

// ApplyFileConfig updates the Config with values from the FileConfig.
// This should be called after loading the file config but before applying
// environment variables and command-line flags.
func (c *Config) ApplyFileConfig(fc *FileConfig) {
	applyString(&c.RPCUser, fc.RPC.User)
	applyString(&c.RPCPassword, fc.RPC.Password)
	applyString(&c.RPCAddr, fc.RPC.Addr)
	applyString(&c.Network, fc.Network.Type)
	applyStringSlice(&c.ListenAddrs, fc.Network.Listen)
	applyStringSlice(&c.AddPeers, fc.Network.AddPeer)
	applyString(&c.DataDir, fc.DataDir)
	applyString(&c.PrometheusAddr, fc.PrometheusAddr)
	applyPositiveInt(&c.MaxPeers, fc.MaxPeers)
}

func applyString(dst *string, value string) {
	if value != "" {
		*dst = value
	}
}

func applyStringSlice(dst *[]string, value []string) {
	if len(value) > 0 {
		*dst = value
	}
}

func applyPositiveInt(dst *int, value int) {
	if value > 0 {
		*dst = value
	}
}

// ApplyEnvironmentVariables reads and applies configuration from environment variables.
// Environment variables take precedence over config file but are overridden by command-line flags.
// Supported variables:
//   - NMCD_RPC_USER: RPC authentication username
//   - NMCD_RPC_PASSWORD: RPC authentication password
//   - NMCD_RPC_ADDR: RPC server listen address
//   - NMCD_PROMETHEUS_ADDR: Prometheus metrics endpoint address
//   - NMCD_NETWORK: Network type (mainnet, testnet, regtest)
//   - NMCD_DATADIR: Data directory path
func (c *Config) ApplyEnvironmentVariables() {
	if user := os.Getenv("NMCD_RPC_USER"); user != "" {
		c.RPCUser = user
	}
	if password := os.Getenv("NMCD_RPC_PASSWORD"); password != "" {
		c.RPCPassword = password
	}
	if addr := os.Getenv("NMCD_RPC_ADDR"); addr != "" {
		c.RPCAddr = addr
	}
	if prometheusAddr := os.Getenv("NMCD_PROMETHEUS_ADDR"); prometheusAddr != "" {
		c.PrometheusAddr = prometheusAddr
	}
	if network := os.Getenv("NMCD_NETWORK"); network != "" {
		c.Network = network
	}
	if dataDir := os.Getenv("NMCD_DATADIR"); dataDir != "" {
		c.DataDir = dataDir
	}
}
