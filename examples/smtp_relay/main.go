package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/opd-ai/nmcd/bridge"
	"github.com/opd-ai/nmcd/client"
	"github.com/opd-ai/nmcd/mail"
)

func main() {
	// Parse command-line flags
	listenAddr := flag.String("listen", ":2525", "SMTP relay listen address")
	upstreamHost := flag.String("upstream-host", "", "Upstream SMTP server hostname (required)")
	upstreamPort := flag.Int("upstream-port", 587, "Upstream SMTP server port")
	upstreamUser := flag.String("upstream-user", "", "Upstream SMTP username (optional)")
	upstreamPass := flag.String("upstream-pass", "", "Upstream SMTP password (optional)")
	cacheTTL := flag.Duration("cache-ttl", time.Hour, "Mail routing cache TTL")
	dataDir := flag.String("datadir", "~/.nmcd", "Namecoin data directory")
	network := flag.String("network", "mainnet", "Network to use (mainnet, testnet, regtest)")

	flag.Parse()

	// Validate required flags
	if *upstreamHost == "" {
		fmt.Println("Error: -upstream-host is required")
		flag.Usage()
		os.Exit(1)
	}

	// Create Namecoin client
	log.Println("Connecting to Namecoin...")
	nc, err := client.NewClient(&client.Config{
		Mode:    client.ModeEmbedded,
		Network: *network,
		DataDir: *dataDir,
	})
	if err != nil {
		log.Fatalf("Failed to create Namecoin client: %v", err)
	}
	defer nc.Close()

	// Create bridge adapter
	bridge := bridge.NewNamecoinBridge(nc)
	log.Println("Namecoin bridge initialized")

	// Create mail router
	router := mail.NewRouter(bridge, *cacheTTL)
	log.Printf("Mail router initialized (cache TTL: %s)", *cacheTTL)

	// Configure relay
	config := mail.DefaultRelayConfig()
	config.ListenAddr = *listenAddr
	config.UpstreamHost = *upstreamHost
	config.UpstreamPort = *upstreamPort

	// Configure upstream authentication if provided
	if *upstreamUser != "" && *upstreamPass != "" {
		config.UpstreamAuth = smtp.PlainAuth("", *upstreamUser, *upstreamPass, *upstreamHost)
		log.Println("Upstream SMTP authentication configured")
	}

	// Create and start relay
	relay := mail.NewRelay(router, config)
	if err := relay.Start(); err != nil {
		log.Fatalf("Failed to start SMTP relay: %v", err)
	}

	log.Printf("SMTP relay started on %s", *listenAddr)
	log.Printf("Forwarding to %s:%d", *upstreamHost, *upstreamPort)
	log.Println("Ready to accept .bit email addresses")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	if err := relay.Stop(); err != nil {
		log.Printf("Error stopping relay: %v", err)
	}
	log.Println("Goodbye!")
}

// Example usage demonstrating testing the relay
func exampleTestRelay() {
	// This function demonstrates how to test the relay without external SMTP server
	log.Println("\n=== Testing SMTP Relay ===")

	// Create a mock Namecoin client for testing
	nc := &mockNameClient{
		names: map[string]string{
			"alice": `{"email": "alice@example.com"}`,
			"bob":   `{"email": "bob@protonmail.com", "backup": ["bob.backup@gmail.com"]}`,
		},
	}

	// Create bridge and router
	bridgeAdapter := bridge.NewNamecoinBridge(nc)
	router := mail.NewRouter(bridgeAdapter, 0) // No caching for testing

	// Test routing resolution
	ctx := context.Background()
	resolved, err := router.Route(ctx, "alice@mail.bit")
	if err != nil {
		log.Printf("Failed to route alice@mail.bit: %v", err)
		return
	}
	log.Printf("alice@mail.bit resolves to: %s", resolved)

	resolved, err = router.Route(ctx, "bob@mail.bit")
	if err != nil {
		log.Printf("Failed to route bob@mail.bit: %v", err)
		return
	}
	log.Printf("bob@mail.bit resolves to: %s", resolved)
}

// mockNameClient is a simple mock for testing
type mockNameClient struct {
	names map[string]string
}

func (m *mockNameClient) ResolveName(ctx context.Context, name string) (*client.NameRecord, error) {
	value, ok := m.names[name]
	if !ok {
		return nil, client.ErrNameNotFound
	}
	return &client.NameRecord{
		Name:  name,
		Value: value,
	}, nil
}

func (m *mockNameClient) RegisterName(ctx context.Context, name, value string, opts *client.RegisterOpts) (*client.TxResult, error) {
	return nil, nil
}

func (m *mockNameClient) UpdateName(ctx context.Context, name, value string, opts *client.UpdateOpts) (*client.TxResult, error) {
	return nil, nil
}

func (m *mockNameClient) ListNames(ctx context.Context, filter *client.ListFilter) ([]*client.NameRecord, error) {
	return nil, nil
}

func (m *mockNameClient) GetNameHistory(ctx context.Context, name string) ([]*client.NameRecord, error) {
	return nil, nil
}

func (m *mockNameClient) WaitForConfirmation(ctx context.Context, txHash string, confirmations int) error {
	return nil
}

func (m *mockNameClient) GetInfo(ctx context.Context) (*client.NodeInfo, error) {
	return &client.NodeInfo{
		Version:     "test",
		NetworkName: "regtest",
		Mode:        "mock",
	}, nil
}

func (m *mockNameClient) Close() error {
	return nil
}
