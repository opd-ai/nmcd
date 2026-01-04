// Package main demonstrates using the mail router to resolve .bit email addresses.
//
// This example shows:
//   - Creating a Namecoin bridge resolver
//   - Setting up a mail router with caching
//   - Resolving .bit addresses to real email addresses
//   - Handling errors and edge cases
//
// Usage:
//
//	go run examples/mail_router/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/opd-ai/nmcd/bridge"
	"github.com/opd-ai/nmcd/client"
	"github.com/opd-ai/nmcd/mail"
)

func main() {
	fmt.Println("Mail Router Example")
	fmt.Println("===================")
	fmt.Println()

	// 1. Create a Namecoin client
	// In a real application, this would connect to an actual Namecoin node.
	// For this example, we'll use a mock client for demonstration purposes.
	fmt.Println("Step 1: Creating Namecoin client...")
	nc := newMockNamecoinClient()
	defer nc.Close()
	fmt.Println("✓ Client created")
	fmt.Println()

	// 2. Create bridge resolver
	// The bridge translates Namecoin name records into mail configurations
	fmt.Println("Step 2: Creating bridge resolver...")
	resolver := bridge.NewNamecoinBridge(nc)
	fmt.Println("✓ Bridge resolver created")
	fmt.Println()

	// 3. Create mail router with 1 hour cache TTL
	// Caching reduces lookup overhead for frequently accessed addresses
	fmt.Println("Step 3: Creating mail router (TTL: 1 hour)...")
	router := mail.NewRouter(resolver, time.Hour)
	fmt.Println("✓ Router created")
	fmt.Println()

	// 4. Route some .bit addresses
	fmt.Println("Step 4: Routing .bit addresses...")
	fmt.Println()

	ctx := context.Background()
	addresses := []string{
		"alice@mail.bit",
		"bob@inbox.bit",
		"charlie@company.bit",
		"invalid@example.com",  // Not a .bit address
		"nonexistent@mail.bit", // Doesn't exist
	}

	for _, addr := range addresses {
		fmt.Printf("Routing: %s\n", addr)
		realEmail, err := router.Route(ctx, addr)
		if err != nil {
			fmt.Printf("  ✗ Error: %v\n", err)
		} else {
			fmt.Printf("  ✓ Forwards to: %s\n", realEmail)
		}
		fmt.Println()
	}

	// 5. Demonstrate cache effectiveness
	fmt.Println("Step 5: Demonstrating cache (routing alice@mail.bit again)...")
	start := time.Now()
	realEmail, err := router.Route(ctx, "alice@mail.bit")
	elapsed := time.Since(start)
	if err != nil {
		log.Printf("Cache test failed: %v", err)
	} else {
		fmt.Printf("  ✓ Cached result: %s (took %v)\n", realEmail, elapsed)
	}
	fmt.Println()

	fmt.Println("Example completed successfully!")
}

// mockNamecoinClient is a simple mock implementation for demonstration
type mockNamecoinClient struct {
	names map[string]*client.NameRecord
}

func newMockNamecoinClient() client.NameClient {
	return &mockNamecoinClient{
		names: map[string]*client.NameRecord{
			"alice": {
				Name:  "alice",
				Value: `{"email": "alice@gmail.com", "backup": ["alice-backup@proton.me"]}`,
			},
			"bob": {
				Name:  "bob",
				Value: `{"email": "bob@proton.me"}`,
			},
			"charlie": {
				Name:  "charlie",
				Value: `{"email": "charlie@company.com", "backup": ["charlie@personal.com", "c.smith@backup.net"]}`,
			},
		},
	}
}

func (m *mockNamecoinClient) ResolveName(ctx context.Context, name string) (*client.NameRecord, error) {
	if record, ok := m.names[name]; ok {
		return record, nil
	}
	return nil, client.ErrNameNotFound
}

func (m *mockNamecoinClient) RegisterName(ctx context.Context, name, value string, opts *client.RegisterOpts) (*client.TxResult, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *mockNamecoinClient) UpdateName(ctx context.Context, name, value string, opts *client.UpdateOpts) (*client.TxResult, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *mockNamecoinClient) ListNames(ctx context.Context, filter *client.ListFilter) ([]*client.NameRecord, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *mockNamecoinClient) GetNameHistory(ctx context.Context, name string) ([]*client.NameRecord, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *mockNamecoinClient) WaitForConfirmation(ctx context.Context, txHash string, confirmations int) error {
	return fmt.Errorf("not implemented in mock")
}

func (m *mockNamecoinClient) GetInfo(ctx context.Context) (*client.NodeInfo, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *mockNamecoinClient) Close() error {
	return nil
}
