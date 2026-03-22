package main

import (
	"context"
	"fmt"
	"log"

	"github.com/opd-ai/nmcd/bridge"
	"github.com/opd-ai/nmcd/client"
)

func main() {
	cfg := &client.Config{
		Mode:    client.ModeEmbedded,
		Network: "regtest",
	}

	nc, err := client.NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create Namecoin client: %v", err)
	}
	defer nc.Close()

	resolver := bridge.NewNamecoinBridge(nc)
	ctx := context.Background()

	demonstrateLookup(resolver, ctx)
	demonstrateErrorHandling(resolver, ctx)
	demonstrateResolverInterface(resolver)
}

func demonstrateLookup(resolver bridge.Resolver, ctx context.Context) {
	fmt.Println("Example 1: Looking up mail configuration")
	fmt.Println("=========================================")

	config, err := resolver.LookupMail(ctx, "alice")
	if err == client.ErrNameNotFound {
		printNotFoundHelp()
	} else if err == bridge.ErrInvalidMailConfig {
		printInvalidConfigHelp()
	} else if err != nil {
		log.Fatalf("Error looking up mail config: %v", err)
	} else {
		printMailConfig(config)
	}

	fmt.Println()
}

func printNotFoundHelp() {
	fmt.Println("Name 'alice' not found in blockchain")
	fmt.Println("To use this example:")
	fmt.Println("  1. Start nmcd daemon: ./nmcd --network=regtest")
	fmt.Println("  2. Register a name with mail config:")
	fmt.Println(`     curl -X POST http://localhost:18336 -d '{"jsonrpc":"2.0","id":1,"method":"name_new","params":["alice"]}'`)
	fmt.Println(`     # Wait 12 blocks...`)
	fmt.Println(`     curl -X POST http://localhost:18336 -d '{"jsonrpc":"2.0","id":1,"method":"name_firstupdate","params":["alice","{\"email\":\"alice@example.com\"}"]}'`)
	fmt.Println("  3. Run this example again")
}

func printInvalidConfigHelp() {
	fmt.Println("Name exists but doesn't contain valid mail configuration")
	fmt.Println("Expected JSON format:")
	fmt.Println(`  {"email": "user@example.com", "backup": ["backup@example.com"], "pubkey": "base64key..."}`)
}

func printMailConfig(config bridge.MailConfig) {
	fmt.Printf("✓ Mail configuration found for 'alice':\n")
	fmt.Printf("  Forward to: %s\n", config.ForwardTo)

	if len(config.BackupAddrs) > 0 {
		fmt.Printf("  Backup addresses:\n")
		for i, addr := range config.BackupAddrs {
			fmt.Printf("    %d. %s\n", i+1, addr)
		}
	}

	if len(config.PublicKey) > 0 {
		fmt.Printf("  Public key: %d bytes\n", len(config.PublicKey))
	}
}

func demonstrateErrorHandling(resolver bridge.Resolver, ctx context.Context) {
	fmt.Println("Example 2: Error handling")
	fmt.Println("=========================")

	for _, name := range []string{"nonexistent", "alice"} {
		_, err := resolver.LookupMail(ctx, name)
		switch {
		case err == client.ErrNameNotFound:
			fmt.Printf("✗ Name '%s': not found\n", name)
		case err == bridge.ErrInvalidMailConfig:
			fmt.Printf("✗ Name '%s': invalid mail config\n", name)
		case err != nil:
			fmt.Printf("✗ Name '%s': error: %v\n", name, err)
		default:
			fmt.Printf("✓ Name '%s': valid mail config\n", name)
		}
	}

	fmt.Println()
}

// demonstrateResolverInterface shows how the Resolver interface enables
// loose coupling between the mail routing logic and Namecoin implementation
func demonstrateResolverInterface(resolver bridge.Resolver) {
	fmt.Println("This function accepts any Resolver implementation")
	fmt.Println("(NamecoinBridge, MockResolver for testing, etc.)")

	// In a real mail router, this would be used to resolve .bit addresses
	// For example: alice@mail.bit -> alice@gmail.com
	// Context would be passed from the SMTP handler for cancellation support

	fmt.Println("Resolver interface enables:")
	fmt.Println("  • Testability (mock resolvers for unit tests)")
	fmt.Println("  • Flexibility (swap Namecoin implementations)")
	fmt.Println("  • Decoupling (mail logic doesn't depend on Namecoin internals)")
}
