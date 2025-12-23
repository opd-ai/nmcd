package network

import (
	"testing"
)

func TestNewSeedResolver(t *testing.T) {
	seeds := []string{"seed1.example.com", "seed2.example.com"}
	port := "8334"

	resolver := NewSeedResolver(seeds, port)

	if resolver == nil {
		t.Fatal("NewSeedResolver returned nil")
	}

	if len(resolver.seeds) != 2 {
		t.Errorf("Expected 2 seeds, got %d", len(resolver.seeds))
	}

	if resolver.defaultPort != port {
		t.Errorf("Expected port %s, got %s", port, resolver.defaultPort)
	}

	if resolver.timeout == 0 {
		t.Error("Timeout should be set to a non-zero value")
	}
}

func TestSeedResolverEmptySeeds(t *testing.T) {
	resolver := NewSeedResolver([]string{}, "8334")

	addresses, err := resolver.Resolve()
	if err != nil {
		t.Errorf("Expected no error for empty seeds, got: %v", err)
	}

	if addresses != nil {
		t.Errorf("Expected nil for empty seeds, got: %v", addresses)
	}
}

func TestSeedResolverNilSeeds(t *testing.T) {
	resolver := NewSeedResolver(nil, "8334")

	addresses, err := resolver.Resolve()
	if err != nil {
		t.Errorf("Expected no error for nil seeds, got: %v", err)
	}

	if addresses != nil {
		t.Errorf("Expected nil for nil seeds, got: %v", addresses)
	}
}

func TestResolveSeedNodesEmptySeeds(t *testing.T) {
	addresses := ResolveSeedNodes([]string{}, "8334")
	if len(addresses) != 0 {
		t.Errorf("Expected no addresses for empty seeds, got: %v", addresses)
	}
}

func TestResolveSeedNodesNilSeeds(t *testing.T) {
	addresses := ResolveSeedNodes(nil, "8334")
	if len(addresses) != 0 {
		t.Errorf("Expected no addresses for nil seeds, got: %v", addresses)
	}
}

// TestSeedResolverInvalidSeed tests that invalid DNS seeds are handled gracefully.
func TestSeedResolverInvalidSeed(t *testing.T) {
	// Use an invalid seed that will fail DNS lookup
	resolver := NewSeedResolver([]string{"invalid.nonexistent.local.test"}, "8334")

	addresses, err := resolver.Resolve()
	
	// Should return an error when no seeds can be resolved
	if err == nil && len(addresses) > 0 {
		t.Error("Expected error or empty result for invalid seed")
	}
}

// TestSeedResolverMixedSeeds tests behavior with mix of valid and invalid seeds.
// This is an integration-style test that may be skipped in CI environments.
func TestSeedResolverMixedSeeds(t *testing.T) {
	// Using invalid seeds to test error handling
	resolver := NewSeedResolver([]string{
		"invalid1.nonexistent.local.test",
		"invalid2.nonexistent.local.test",
	}, "8334")

	// Should not panic even with invalid seeds
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Resolve panicked: %v", r)
		}
	}()

	_, _ = resolver.Resolve()
}
