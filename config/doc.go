// Package config provides Namecoin network configuration and protocol constants.
//
// The config package defines all network parameters, protocol constants, and
// configuration structures needed to operate an nmcd node. It includes chain
// parameters for mainnet, testnet, and regtest networks, as well as name
// operation limits, fee schedules, and AuxPow activation heights.
//
// # Network Parameters
//
// Three network configurations are provided:
//
//   - NamecoinMainNetParams: Production Namecoin network (chain ID 1)
//   - NamecoinTestNetParams: Test network for development
//   - NamecoinRegTestParams: Local regression testing network
//
// Each network defines genesis blocks, DNS seeds, default ports, and protocol
// activation heights.
//
// # Protocol Constants
//
// Key protocol constants include:
//
//   - NameExpirationBlocks (36,000): Blocks until a name expires (~250 days)
//   - MaxNameLength (255): Maximum name length in bytes
//   - MaxValueLength (1,023): Maximum value length in bytes (consensus)
//   - NameValueRelayLimit (520): Maximum value length for relay (policy)
//   - AuxPowVersionBit (0x100): Block version bit for AuxPow blocks
//   - MainNetAuxPowActivationHeight (19,200): AuxPow mandatory after this height
//
// # Configuration Files
//
// The package supports TOML configuration files for node setup:
//
//	[network]
//	type = "mainnet"
//
//	[rpc]
//	listen = "127.0.0.1:8336"
//	user = "rpcuser"
//	password = "rpcpassword"
//
//	[data]
//	dir = "/var/lib/nmcd"
//
// Use LoadConfig to read configuration files and ConfigPath to locate the
// default configuration file path.
//
// # Block Subsidy
//
// The package includes Namecoin's block subsidy calculation which follows
// Bitcoin's halving schedule:
//
//   - Initial subsidy: 50 NMC per block
//   - Halving interval: 210,000 blocks
//   - Current subsidy at height h: 50 >> (h / 210,000)
//
// # DNS Seeds
//
// DNS seed nodes for peer discovery are configured per network. MainNet seeds
// include:
//
//   - nmc.seed.quisquis.de
//   - seed.nmc.markasoftware.com
//   - dnsseed.namecoin.webbtc.com
//
// # Example Usage
//
// Getting network parameters:
//
//	// Get mainnet parameters
//	params := config.GetNamecoinParams("mainnet")
//
//	// Check AuxPow requirement
//	if height >= config.MainNetAuxPowActivationHeight {
//	    // Block requires AuxPow
//	}
//
//	// Calculate block subsidy
//	subsidy := config.CalcBlockSubsidy(height)
//
// Loading configuration:
//
//	cfg, err := config.LoadConfig(config.ConfigPath())
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Network: %s\n", cfg.Network)
package config
