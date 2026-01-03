// Package main demonstrates basic Namecoin name resolution using the nmcd client library.
//
// This example shows how to:
//   - Create a client with automatic mode detection
//   - Resolve a Namecoin name to get its value and metadata
//   - Handle common error cases (name not found, expired)
//
// Usage:
//
//	go run simple_resolve.go <name>
//
// Examples:
//
//	go run simple_resolve.go d/example
//	go run simple_resolve.go id/alice
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/opd-ai/nmcd/client"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: simple_resolve <name>")
		fmt.Println("Example: simple_resolve d/example")
		os.Exit(1)
	}

	name := os.Args[1]

	// Create client with auto-detection
	// Will use daemon if running, otherwise embedded mode
	cfg := &client.Config{
		Mode:    client.ModeAuto,
		Network: "regtest", // Use regtest for local testing
		DataDir: os.TempDir() + "/nmcd-simple-resolve",
	}

	fmt.Println("Initializing Namecoin client...")
	nc, err := client.NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer nc.Close()

	// Get node info to show connection mode
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := nc.GetInfo(ctx)
	if err != nil {
		log.Fatalf("Failed to get node info: %v", err)
	}
	fmt.Printf("Connected to %s network in %s mode (height: %d)\n\n", info.NetworkName, info.Mode, info.BlockHeight)

	// Resolve the name
	fmt.Printf("Resolving name: %s\n", name)
	record, err := nc.ResolveName(ctx, name)

	switch {
	case errors.Is(err, client.ErrNameNotFound):
		fmt.Printf("✗ Name not found: %s\n", name)
		fmt.Println("\nThe name either doesn't exist or hasn't been registered yet.")
		os.Exit(1)

	case errors.Is(err, client.ErrNameExpired):
		fmt.Printf("✗ Name expired: %s\n", name)
		fmt.Println("\nThe name was registered but has expired and is now available for re-registration.")
		os.Exit(1)

	case err != nil:
		log.Fatalf("Failed to resolve name: %v", err)

	default:
		// Success
		fmt.Printf("✓ Name resolved successfully!\n\n")
		fmt.Println("Name Record:")
		fmt.Printf("  Name:       %s\n", record.Name)
		fmt.Printf("  Value:      %s\n", record.Value)
		fmt.Printf("  Owner:      %s\n", record.Address)
		fmt.Printf("  TX Hash:    %s\n", record.TxHash)
		fmt.Printf("  Height:     %d\n", record.Height)
		fmt.Printf("  Expires At: %d\n", record.ExpiresAt)
		fmt.Printf("  Expires In: %d blocks (~%.1f days)\n", record.ExpiresIn, float64(record.ExpiresIn)*10/60/24)
		fmt.Printf("  Updated:    %s\n", record.UpdatedAt.Format(time.RFC3339))
	}
}
