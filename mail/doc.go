// Package mail provides stateless email routing logic for .bit addresses.
//
// The mail package implements the core routing functionality for the Permamail system,
// enabling resolution of blockchain-based .bit email addresses to real email addresses.
// It is designed to be completely independent of Namecoin, using dependency injection
// to interact with the blockchain through the Resolver interface.
//
// # Architecture
//
// The package follows a clean separation of concerns:
//
//   - Resolver interface: Abstracts mail config lookups (implemented by bridge package)
//   - Router: Stateless routing logic with optional caching
//   - Address parsing: Validates .bit email address format
//
// # Usage
//
// Basic usage with the Namecoin bridge:
//
//	import (
//	    "context"
//	    "time"
//	    "github.com/opd-ai/nmcd/bridge"
//	    "github.com/opd-ai/nmcd/client"
//	    "github.com/opd-ai/nmcd/mail"
//	)
//
//	// Create Namecoin client
//	nc, err := client.NewClient(&client.Config{
//	    Mode:    client.ModeEmbedded,
//	    Network: "mainnet",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer nc.Close()
//
//	// Create bridge resolver
//	resolver := bridge.NewNamecoinBridge(nc)
//
//	// Create router with 1 hour cache
//	router := mail.NewRouter(resolver, time.Hour)
//
//	// Route a .bit address
//	ctx := context.Background()
//	realEmail, err := router.Route(ctx, "alice@mail.bit")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Forward to: %s\n", realEmail)
//
// # Address Format
//
// .bit addresses follow the email format: localpart@domain.bit
//
//   - The localpart is used for Namecoin name lookup
//   - The domain must end with .bit
//   - Examples: alice@mail.bit, bob@company.bit, user@inbox.bit
//
// # Caching
//
// The router implements TTL-based caching to reduce lookup overhead:
//
//   - Cache entries expire after the configured TTL
//   - Thread-safe concurrent access
//   - Set TTL to 0 to disable caching
//   - Recommended TTL: 1 hour for production
//
// # Thread Safety
//
// All types in this package are safe for concurrent use by multiple goroutines.
// The Router uses sync.RWMutex to protect its cache map.
//
// # Testing
//
// The Resolver interface enables testing without a real Namecoin node:
//
//	type mockResolver struct {
//	    responses map[string]bridge.MailConfig
//	}
//
//	func (m *mockResolver) LookupMail(ctx context.Context, name string) (bridge.MailConfig, error) {
//	    if config, ok := m.responses[name]; ok {
//	        return config, nil
//	    }
//	    return bridge.MailConfig{}, fmt.Errorf("name not found")
//	}
//
//	// Test with mock
//	mock := &mockResolver{
//	    responses: map[string]bridge.MailConfig{
//	        "alice": {ForwardTo: "alice@gmail.com"},
//	    },
//	}
//	router := mail.NewRouter(mock, 0)
//	result, _ := router.Route(context.Background(), "alice@mail.bit")
//	// result == "alice@gmail.com"
package mail
