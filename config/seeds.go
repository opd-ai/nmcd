package config

// DNS seed nodes for Namecoin network peer discovery.
// These are official DNS seeds that return IP addresses of active nodes.

// MainNetDNSSeeds contains DNS seed hostnames for Namecoin mainnet.
// These seeds return IPv4/IPv6 addresses of full nodes when queried.
// Source: https://github.com/namecoin/namecoin-core
var MainNetDNSSeeds = []string{
	"nmc.seed.quisquis.de",            // Peter Conrad
	"seed.nmc.markasoftware.com",      // Mark Polyakov
	"dnsseed1.nmc.dotbit.zone",        // Stefan Stere
	"dnsseed2.nmc.dotbit.zone",        // Stefan Stere
	"dnsseed.nmc.testls.space",        // mjgill89
	"namecoin.seed.cypherstack.com",   // Dan Miller
}

// TestNetDNSSeeds contains DNS seed hostnames for Namecoin testnet.
var TestNetDNSSeeds = []string{
	"dnsseed.test.namecoin.webbtc.com", // Marius Hanne
}

// RegTestDNSSeeds contains DNS seed hostnames for Namecoin regtest.
// Regtest has no DNS seeds since it's a local/private network.
var RegTestDNSSeeds = []string{}

// DNSSeeds returns the appropriate DNS seeds for the given network.
func DNSSeeds(network string) []string {
	switch network {
	case "testnet":
		return TestNetDNSSeeds
	case "regtest":
		return RegTestDNSSeeds
	default:
		return MainNetDNSSeeds
	}
}

// DefaultPort returns the default P2P port for the given network.
func DefaultPort(network string) string {
	switch network {
	case "testnet":
		return TestNetDefaultPort
	case "regtest":
		return RegTestDefaultPort
	default:
		return MainNetDefaultPort
	}
}
