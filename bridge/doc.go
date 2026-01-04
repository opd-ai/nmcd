// Package bridge provides an adapter layer between Namecoin name resolution
// and email forwarding configuration. It translates Namecoin name records into
// mail configuration structures suitable for SMTP relay and routing.
//
// The bridge package implements the adapter pattern, decoupling the mail routing
// logic from the specific Namecoin client implementation. This allows the email
// system to work with any Namecoin client (embedded or daemon) without tight coupling.
//
// # Architecture
//
// The bridge package defines two primary abstractions:
//
//   - MailConfig: Email forwarding configuration extracted from Namecoin records
//   - Resolver: Interface for looking up mail configuration by name
//
// The NamecoinBridge type implements Resolver by querying a Namecoin client
// and parsing the name value as JSON to extract email configuration.
//
// # Name Record Format
//
// Namecoin name values must be valid JSON with the following structure:
//
//	{
//	    "email": "user@gmail.com",           // Required: primary forwarding address
//	    "backup": ["backup@proton.me"],      // Optional: fallback addresses
//	    "pubkey": "base64encodedkey..."      // Optional: public key for verification
//	}
//
// Only the "email" field is required. The "backup" and "pubkey" fields are optional.
//
// # Example Usage
//
// Basic mail config lookup:
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
//	// Create bridge adapter
//	bridge := bridge.NewNamecoinBridge(nc)
//
//	// Lookup mail config
//	ctx := context.Background()
//	config, err := bridge.LookupMail(ctx, "alice")
//	if err == client.ErrNameNotFound {
//	    fmt.Println("Name not registered")
//	} else if err == bridge.ErrInvalidMailConfig {
//	    fmt.Println("Invalid mail configuration")
//	} else if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Use configuration
//	fmt.Printf("Forward mail to: %s\n", config.ForwardTo)
//	if len(config.BackupAddrs) > 0 {
//	    fmt.Printf("Backup addresses: %v\n", config.BackupAddrs)
//	}
//
// # Error Handling
//
// The bridge package defines one package-specific error:
//
//   - ErrInvalidMailConfig: Returned when a name value cannot be parsed as valid mail config
//
// Other errors are passed through from the underlying Namecoin client, including:
//
//   - client.ErrNameNotFound: Name doesn't exist or has expired
//   - client.ErrNameExpired: Name exists but has expired
//
// # Thread Safety
//
// NamecoinBridge is safe for concurrent use. Multiple goroutines can call LookupMail
// simultaneously on the same bridge instance.
//
// # Design Principles
//
// The bridge package follows these design principles from the Permamail roadmap:
//
//  1. Zero imports from namecoin internals (uses client interface only)
//  2. Simple, stateless adapter (no caching, just translation)
//  3. Clear separation of concerns (bridge knows nothing about SMTP)
//  4. Interface-based design (Resolver interface for testability)
//  5. Minimal code footprint (~100 LOC excluding tests)
//
// # Future Extensions
//
// Potential future enhancements (out of scope for Phase 1):
//
//   - Caching layer for frequently accessed names
//   - Support for additional mail configuration fields (DKIM, SPF)
//   - Multiple namespace support (d/, id/, p/)
//   - Validation of email address format
//   - Support for mail routing rules beyond simple forwarding
package bridge
