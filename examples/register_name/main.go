// Package main demonstrates Namecoin name registration using the nmcd client library.
//
// This example shows how to:
//   - Create an embedded client with wallet support
//   - Register a new Namecoin name with a JSON value
//   - Handle registration errors and name conflicts
//
// Usage:
//
//	go run register_name.go <name> <value>
//
// Examples:
//
//	go run register_name.go d/mysite '{"ip":"1.2.3.4"}'
//	go run register_name.go id/alice '{"name":"Alice","email":"alice@example.com"}'
//
// Note: Name registration is a two-step process (NAME_NEW + NAME_FIRSTUPDATE).
// The NAME_NEW creates a commitment, and after 12 blocks, NAME_FIRSTUPDATE
// reveals the name and sets its initial value.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/opd-ai/nmcd/client"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: register_name <name> <value>")
		fmt.Println("Examples:")
		fmt.Println("  register_name d/mysite '{\"ip\":\"1.2.3.4\"}'")
		fmt.Println("  register_name id/alice '{\"name\":\"Alice\"}'")
		os.Exit(1)
	}

	name := os.Args[1]
	value := os.Args[2]

	// Validate name format
	if !strings.HasPrefix(name, "d/") && !strings.HasPrefix(name, "id/") && !strings.HasPrefix(name, "p/") {
		fmt.Println("Error: Name must start with d/, id/, or p/ namespace")
		fmt.Println("Examples: d/example, id/alice, p/notes")
		os.Exit(1)
	}

	// Validate value is valid JSON for d/ and id/ namespaces
	if strings.HasPrefix(name, "d/") || strings.HasPrefix(name, "id/") {
		var js json.RawMessage
		if err := json.Unmarshal([]byte(value), &js); err != nil {
			fmt.Printf("Error: Value must be valid JSON for %s namespace\n", strings.Split(name, "/")[0]+"/")
			fmt.Printf("Invalid JSON: %v\n", err)
			os.Exit(1)
		}
	}

	// Create embedded client with wallet enabled
	cfg := &client.Config{
		Mode:          client.ModeEmbedded,
		Network:       "regtest", // Use regtest for local testing
		DataDir:       os.TempDir() + "/nmcd-register-example",
		DisableWallet: false, // Wallet required for registration
	}

	fmt.Println("Initializing Namecoin client...")
	nc, err := client.NewEmbeddedClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer nc.Close()

	// Get node info
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	info, err := nc.GetInfo(ctx)
	if err != nil {
		log.Fatalf("Failed to get node info: %v", err)
	}
	fmt.Printf("Connected to %s network (height: %d)\n\n", info.NetworkName, info.BlockHeight)

	// Check if name already exists
	fmt.Printf("Checking if name '%s' is available...\n", name)
	existingRecord, err := nc.ResolveName(ctx, name)
	if err == nil {
		fmt.Printf("✗ Name already registered!\n")
		fmt.Printf("  Owner: %s\n", existingRecord.Address)
		fmt.Printf("  Expires in: %d blocks\n", existingRecord.ExpiresIn)
		os.Exit(1)
	} else if err != client.ErrNameNotFound && err != client.ErrNameExpired {
		log.Fatalf("Failed to check name: %v", err)
	}

	fmt.Printf("✓ Name '%s' is available\n\n", name)

	// Register the name
	fmt.Println("Registering name...")
	fmt.Printf("  Name:  %s\n", name)
	fmt.Printf("  Value: %s\n", value)

	opts := &client.RegisterOpts{
		WaitForConfirmation: false, // Don't wait for confirmation in this example
		FeeRate:             1,     // 1 satoshi per byte
	}

	result, err := nc.RegisterName(ctx, name, value, opts)
	if err != nil {
		switch {
		case err == client.ErrNameExists:
			fmt.Println("✗ Name already exists (race condition)")
		case err == client.ErrInsufficientFunds:
			fmt.Println("✗ Insufficient funds for registration")
			fmt.Println("  The wallet needs NMC to pay for the registration fee")
		case err == client.ErrNoWallet:
			fmt.Println("✗ Wallet not initialized")
		case err == client.ErrInvalidName:
			fmt.Println("✗ Invalid name format")
		case err == client.ErrInvalidValue:
			fmt.Println("✗ Invalid value format")
		default:
			log.Fatalf("Failed to register name: %v", err)
		}
		os.Exit(1)
	}

	fmt.Println("\n✓ Registration initiated!")
	fmt.Println("\nTransaction Result:")
	fmt.Printf("  TX Hash:  %s\n", result.TxHash)
	fmt.Printf("  Name:     %s\n", result.Name)
	fmt.Printf("  Status:   %s\n", result.Status)

	if result.Status == client.TxStatusPending {
		fmt.Println("\nNote: NAME_NEW transaction created.")
		fmt.Println("After 12 blocks, NAME_FIRSTUPDATE will complete the registration.")
		fmt.Println("Use WaitForConfirmation: true to wait for full registration.")
	} else if result.Status == client.TxStatusConfirmed {
		fmt.Printf("  Block:    %d\n", result.BlockHeight)
		fmt.Printf("  Confirms: %d\n", result.Confirmations)
	}
}
