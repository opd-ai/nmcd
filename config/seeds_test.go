package config

import (
	"testing"
)

func TestDNSSeeds(t *testing.T) {
	tests := []struct {
		network     string
		expectEmpty bool
		minCount    int
	}{
		{"mainnet", false, 6},  // MainNet should have at least 6 seeds
		{"testnet", false, 1},  // TestNet should have at least 1 seed
		{"regtest", true, 0},   // RegTest should have no seeds
		{"unknown", false, 6},  // Unknown defaults to mainnet
	}

	for _, tt := range tests {
		t.Run(tt.network, func(t *testing.T) {
			seeds := DNSSeeds(tt.network)
			if tt.expectEmpty && len(seeds) != 0 {
				t.Errorf("Expected empty seeds for %s, got %d", tt.network, len(seeds))
			}
			if !tt.expectEmpty && len(seeds) < tt.minCount {
				t.Errorf("Expected at least %d seeds for %s, got %d", tt.minCount, tt.network, len(seeds))
			}
		})
	}
}

func TestDefaultPort(t *testing.T) {
	tests := []struct {
		network      string
		expectedPort string
	}{
		{"mainnet", MainNetDefaultPort},
		{"testnet", TestNetDefaultPort},
		{"regtest", RegTestDefaultPort},
		{"unknown", MainNetDefaultPort},
	}

	for _, tt := range tests {
		t.Run(tt.network, func(t *testing.T) {
			port := DefaultPort(tt.network)
			if port != tt.expectedPort {
				t.Errorf("Expected port %s for %s, got %s", tt.expectedPort, tt.network, port)
			}
		})
	}
}

func TestMainNetDNSSeedsNotEmpty(t *testing.T) {
	if len(MainNetDNSSeeds) == 0 {
		t.Error("MainNetDNSSeeds should not be empty")
	}

	// Verify seeds are valid hostnames (not empty strings)
	for i, seed := range MainNetDNSSeeds {
		if seed == "" {
			t.Errorf("Seed at index %d is empty", i)
		}
	}
}

func TestTestNetDNSSeedsNotEmpty(t *testing.T) {
	if len(TestNetDNSSeeds) == 0 {
		t.Error("TestNetDNSSeeds should not be empty")
	}

	// Verify seeds are valid hostnames (not empty strings)
	for i, seed := range TestNetDNSSeeds {
		if seed == "" {
			t.Errorf("Seed at index %d is empty", i)
		}
	}
}

func TestRegTestDNSSeedsEmpty(t *testing.T) {
	// RegTest should not have DNS seeds since it's for local testing
	if len(RegTestDNSSeeds) != 0 {
		t.Errorf("RegTestDNSSeeds should be empty, got %d seeds", len(RegTestDNSSeeds))
	}
}
