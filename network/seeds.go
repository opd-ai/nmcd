package network

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"
)

// SeedResolver handles DNS seed resolution for peer discovery.
type SeedResolver struct {
	seeds       []string
	defaultPort string
	timeout     time.Duration
}

// NewSeedResolver creates a new seed resolver with the given DNS seeds.
func NewSeedResolver(seeds []string, defaultPort string) *SeedResolver {
	return &SeedResolver{
		seeds:       seeds,
		defaultPort: defaultPort,
		timeout:     10 * time.Second,
	}
}

// Resolve queries DNS seed nodes and returns a list of peer addresses.
// It attempts to resolve each seed and collects all returned IP addresses.
// Returns addresses in "host:port" format ready for connection.
func (sr *SeedResolver) Resolve() ([]string, error) {
	if len(sr.seeds) == 0 {
		return nil, nil
	}

	var addresses []string
	var lastErr error

	for _, seed := range sr.seeds {
		ips, err := sr.resolveSeed(seed)
		if err != nil {
			log.Printf("Failed to resolve seed %s: %v", seed, err)
			lastErr = err
			continue
		}

		for _, ip := range ips {
			addr := net.JoinHostPort(ip.String(), sr.defaultPort)
			addresses = append(addresses, addr)
		}

		log.Printf("Resolved %d addresses from seed %s", len(ips), seed)
	}

	if len(addresses) == 0 && lastErr != nil {
		return nil, fmt.Errorf("failed to resolve any seed nodes: %w", lastErr)
	}

	return addresses, nil
}

// resolveSeed resolves a single DNS seed and returns the IP addresses.
func (sr *SeedResolver) resolveSeed(seed string) ([]net.IP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sr.timeout)
	defer cancel()

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", seed)
	if err != nil {
		return nil, fmt.Errorf("DNS lookup failed for %s: %w", seed, err)
	}

	return ips, nil
}

// ResolveSeedNodes is a convenience function that resolves DNS seeds
// and returns peer addresses for the given network.
func ResolveSeedNodes(seeds []string, defaultPort string) []string {
	resolver := NewSeedResolver(seeds, defaultPort)
	addresses, err := resolver.Resolve()
	if err != nil {
		log.Printf("Seed resolution warning: %v", err)
	}
	return addresses
}
