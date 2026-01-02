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

	// MinNameOperationFee is the minimum transaction fee for NAME_FIRSTUPDATE
	// and NAME_UPDATE operations (in satoshis).
	// Per Namecoin protocol, these operations require a 0.01 NMC fee to prevent spam.
	// This fee is destroyed (burned) as the difference between inputs and outputs,
	// not paid to miners. 0.01 NMC = 1,000,000 satoshis (1 NMC = 100,000,000 satoshis)
	MinNameOperationFee = 1000000 // 0.01 NMC in satoshis

	// MinRelayTxFee is the minimum transaction fee for NAME_NEW and standard
	// transactions (in satoshis). This follows Bitcoin's minimum relay fee.
	// NAME_NEW operations only need to pay standard transaction fees to miners.
	MinRelayTxFee = 1000 // Standard minimum relay fee

	// AuxPow (Auxiliary Proof of Work) constants
	// Namecoin switched to merged mining at these block heights

	// AuxPowVersionBit is the bit that must be set in block version for AuxPow blocks
	// Per Namecoin Core: nVersion & 0x100 must be non-zero for blocks >= AuxPowActivationHeight
	AuxPowVersionBit = 0x100

	// MainNetAuxPowActivationHeight is the block height where AuxPow became mandatory on mainnet
	// After this block, all blocks must have the AuxPow version bit set
	MainNetAuxPowActivationHeight = 19200

	// TestNetAuxPowActivationHeight is the block height where AuxPow became mandatory on testnet
	// Testnet follows the same activation height as mainnet
	TestNetAuxPowActivationHeight = 19200

	// RegTestAuxPowActivationHeight is the block height where AuxPow becomes mandatory on regtest
	// For regtest, we set this very high since AuxPow is not typically used in local testing
	// This allows regtest to operate without AuxPow indefinitely for development purposes
	RegTestAuxPowActivationHeight = 999999999
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

// GetAuxPowActivationHeight returns the block height at which AuxPow becomes
// mandatory for the given network. Blocks at or above this height must have
// the AuxPow version bit (0x100) set in their version field.
//
// Returns:
//   - mainnet: 19,200 (circa 2011, when Namecoin activated merged mining)
//   - testnet: 19,200 (same as mainnet)
//   - regtest: 999,999,999 (effectively never for local testing)
func GetAuxPowActivationHeight(chainParams *chaincfg.Params) int32 {
	switch chainParams.Net {
	case MainNetMagic:
		return MainNetAuxPowActivationHeight
	case TestNetMagic:
		return TestNetAuxPowActivationHeight
	case RegTestMagic:
		return RegTestAuxPowActivationHeight
	default:
		// Unknown network - assume mainnet behavior for safety
		return MainNetAuxPowActivationHeight
	}
}
