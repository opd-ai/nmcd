package config

import (
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/chaincfg"
)

// Config holds all configuration for nmcd
type Config struct {
	DataDir     string
	Network     string
	RPCAddr     string
	ListenAddrs []string
	MaxPeers    int
	AddPeers    []string
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	dataDir := filepath.Join(homeDir, ".nmcd")

	return &Config{
		DataDir:     dataDir,
		Network:     "mainnet",
		RPCAddr:     "127.0.0.1:8336",
		ListenAddrs: []string{"0.0.0.0:8334"},
		MaxPeers:    125,
		AddPeers:    []string{},
	}
}

// ChainParams returns the chain parameters for the network
func (c *Config) ChainParams() *chaincfg.Params {
	switch c.Network {
	case "testnet":
		return &chaincfg.TestNet3Params
	case "regtest":
		return &chaincfg.RegressionNetParams
	default:
		return &chaincfg.MainNetParams
	}
}

// NameDBPath returns the path to the name database
func (c *Config) NameDBPath() string {
	return filepath.Join(c.DataDir, "names.db")
}

// EnsureDataDir creates the data directory if it doesn't exist
func (c *Config) EnsureDataDir() error {
	return os.MkdirAll(c.DataDir, 0700)
}
