// Package main demonstrates Namecoin name updates using the nmcd client library.
//
// This example shows how to:
//   - Create an embedded client with wallet support
//   - Update an existing Namecoin name's value
//   - Handle update errors and ownership verification
//
// Usage:
//
//	go run update_name.go <name> <new_value>
//
// Examples:
//
//	go run update_name.go d/mysite '{"ip":"5.6.7.8"}'
//	go run update_name.go id/alice '{"name":"Alice","verified":true}'
//
// Note: You must own the name (have the private key for its address)
// in your wallet to update it. Updates also extend the name's expiration.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/opd-ai/nmcd/client"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: update_name <name> <new_value>")
		fmt.Println("Examples:")
		fmt.Println("  update_name d/mysite '{\"ip\":\"5.6.7.8\"}'")
		fmt.Println("  update_name id/alice '{\"name\":\"Alice\",\"verified\":true}'")
		os.Exit(1)
	}

	name := os.Args[1]
	newValue := os.Args[2]

	// Validate name format
	if !strings.HasPrefix(name, "d/") && !strings.HasPrefix(name, "id/") && !strings.HasPrefix(name, "p/") {
		fmt.Println("Error: Name must start with d/, id/, or p/ namespace")
		os.Exit(1)
	}

	// Validate value is valid JSON for d/ and id/ namespaces
	if strings.HasPrefix(name, "d/") || strings.HasPrefix(name, "id/") {
		var js json.RawMessage
		if err := json.Unmarshal([]byte(newValue), &js); err != nil {
			fmt.Printf("Error: Value must be valid JSON for %s namespace\n", strings.Split(name, "/")[0]+"/")
			fmt.Printf("Invalid JSON: %v\n", err)
			os.Exit(1)
		}
	}

	// Create embedded client with wallet enabled
	cfg := &client.Config{
		Mode:          client.ModeEmbedded,
		Network:       "regtest", // Use regtest for local testing
		DataDir:       os.TempDir() + "/nmcd-update-example",
		DisableWallet: false, // Wallet required for updates
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

	// Get current name record
	fmt.Printf("Looking up current record for '%s'...\n", name)
	record, err := nc.ResolveName(ctx, name)
	if errors.Is(err, client.ErrNameNotFound) {
		fmt.Printf("✗ Name not found: %s\n", name)
		fmt.Println("\nThe name must be registered before it can be updated.")
		fmt.Println("Use register_name.go to register a new name.")
		os.Exit(1)
	} else if errors.Is(err, client.ErrNameExpired) {
		fmt.Printf("✗ Name expired: %s\n", name)
		fmt.Println("\nThe name has expired. You can re-register it as a new name.")
		os.Exit(1)
	} else if err != nil {
		log.Fatalf("Failed to lookup name: %v", err)
	}

	fmt.Println("✓ Name found")
	fmt.Println("\nCurrent Record:")
	fmt.Printf("  Value:      %s\n", record.Value)
	fmt.Printf("  Owner:      %s\n", record.Address)
	fmt.Printf("  Expires In: %d blocks (~%.1f days)\n", record.ExpiresIn, float64(record.ExpiresIn)*10/60/24)

	// Update the name
	fmt.Println("\nUpdating name...")
	fmt.Printf("  New Value: %s\n", newValue)

	opts := &client.UpdateOpts{
		WaitForConfirmation: false, // Don't wait for confirmation in this example
		FeeRate:             1,     // 1 satoshi per byte
	}

	result, err := nc.UpdateName(ctx, name, newValue, opts)
	if err != nil {
		switch {
		case errors.Is(err, client.ErrNameNotFound):
			fmt.Println("✗ Name not found")
		case errors.Is(err, client.ErrNameExpired):
			fmt.Println("✗ Name expired while updating")
		case errors.Is(err, client.ErrInsufficientFunds):
			fmt.Println("✗ Insufficient funds for update")
			fmt.Println("  The wallet needs NMC to pay for the update fee")
		case errors.Is(err, client.ErrNoWallet):
			fmt.Println("✗ Wallet not initialized")
		case errors.Is(err, client.ErrInvalidValue):
			fmt.Println("✗ Invalid value format")
		default:
			log.Fatalf("Failed to update name: %v", err)
		}
		os.Exit(1)
	}

	fmt.Println("\n✓ Update transaction created!")
	fmt.Println("\nTransaction Result:")
	fmt.Printf("  TX Hash:  %s\n", result.TxHash)
	fmt.Printf("  Name:     %s\n", result.Name)
	fmt.Printf("  Status:   %s\n", result.Status)

	if result.Status == client.TxStatusPending {
		fmt.Println("\nNote: Transaction is pending. Once confirmed:")
		fmt.Println("  - The name's value will be updated")
		fmt.Println("  - The expiration will be extended by 36,000 blocks (~250 days)")
	} else if result.Status == client.TxStatusConfirmed {
		fmt.Printf("  Block:    %d\n", result.BlockHeight)
		fmt.Printf("  Confirms: %d\n", result.Confirmations)
	}
}
