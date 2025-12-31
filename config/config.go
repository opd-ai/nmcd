package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/btcsuite/btcd/chaincfg"
)

// Namecoin protocol constants
const (
	// NameExpirationBlocks is the number of blocks until a name expires (~250 days)
	NameExpirationBlocks = 36000

	// MinBlocksBeforeFirstUpdate is the minimum blocks between name_new and name_firstupdate
	MinBlocksBeforeFirstUpdate = 12

	// MaxBlocksBeforeFirstUpdate is the maximum blocks between name_new and name_firstupdate
	// After this period, the NAME_NEW commitment expires and the name becomes available
	MaxBlocksBeforeFirstUpdate = 36000

	// MaxNameLength is the maximum length of a name in bytes
	MaxNameLength = 255

	// MaxValueLength is the maximum length of a value in bytes
	MaxValueLength = 1023

	// DustLimit is the minimum output value for name operations (in satoshis).
	// This follows Bitcoin's standard dust limit for P2PKH outputs (546 satoshis).
	// Name operation outputs (NAME_NEW, NAME_FIRSTUPDATE, and NAME_UPDATE) below
	// this limit are considered dust and will be rejected to prevent spam and
	// uneconomical UTXO creation.
	DustLimit = 546
)

// ValidNamespaces defines the allowed namespace prefixes for Namecoin names
// Per Namecoin protocol specification:
// - d/ : Domain names (DNS, .bit TLD)
// - id/ : Identity/OpenID records
// - p/ : Personal namespace
var ValidNamespaces = []string{
	"d/",  // Domain names
	"id/", // Identity records
	"p/",  // Personal namespace
}

// Config holds all configuration for nmcd
type Config struct {
	DataDir     string
	Network     string
	RPCAddr     string
	RPCUser     string
	RPCPassword string
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
		RPCUser:     "",
		RPCPassword: "",
		ListenAddrs: []string{"0.0.0.0:8334"},
		MaxPeers:    125,
		AddPeers:    []string{},
	}
}

// ChainParams returns the Namecoin chain parameters for the network
func (c *Config) ChainParams() *chaincfg.Params {
	switch c.Network {
	case "testnet":
		return &NamecoinTestNetParams
	case "regtest":
		return &NamecoinRegTestParams
	default:
		return &NamecoinMainNetParams
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

// IsValidNamespace checks if a name starts with a valid namespace prefix
func IsValidNamespace(name string) bool {
	for _, ns := range ValidNamespaces {
		if strings.HasPrefix(name, ns) {
			return true
		}
	}
	return false
}
