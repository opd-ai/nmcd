package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/opd-ai/nmcd/client"
)

// This example demonstrates using the embedded Namecoin client to resolve names.
// The embedded client runs in-process without requiring an external daemon.
//
// Usage:
//   go run embedded_client_example.go [datadir]
//
// If datadir is not provided, uses a temporary directory.
func main() {
	// Get data directory from command line or use temp dir
	dataDir := getTempDataDir()
	if len(os.Args) > 1 {
		dataDir = os.Args[1]
	}

	fmt.Printf("Using data directory: %s\n\n", dataDir)

	// Create embedded client configuration
	cfg := &client.Config{
		Mode:    client.ModeEmbedded, // Force embedded mode
		DataDir: dataDir,
		Network: "regtest", // Use regtest for local testing
	}

	// Initialize embedded client
	fmt.Println("Initializing embedded Namecoin client...")
	nc, err := client.NewEmbeddedClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer nc.Close()

	fmt.Println("✓ Client initialized successfully\n")

	// Get node information
	ctx := context.Background()
	info, err := nc.GetInfo(ctx)
	if err != nil {
		log.Fatalf("Failed to get node info: %v", err)
	}

	fmt.Println("Node Information:")
	fmt.Printf("  Version: %s\n", info.Version)
	fmt.Printf("  Network: %s\n", info.NetworkName)
	fmt.Printf("  Mode: %s\n", info.Mode)
	fmt.Printf("  Block Height: %d\n", info.BlockHeight)
	fmt.Printf("  Best Block: %s\n", info.BestBlockHash)
	fmt.Printf("  Connections: %d\n\n", info.Connections)

	// Try to resolve a name
	// Note: In Phase 2 foundation, this will only work if the name exists in the database
	// Full blockchain sync will be added in future phases
	testName := "d/example"
	fmt.Printf("Attempting to resolve name: %s\n", testName)

	record, err := nc.ResolveName(ctx, testName)
	if err == client.ErrNameNotFound {
		fmt.Printf("✗ Name not found: %s\n", testName)
		fmt.Println("\nNote: In Phase 2 foundation, the embedded client can only resolve")
		fmt.Println("names that already exist in the local database. Full blockchain sync")
		fmt.Println("and name registration will be added in future phases.")
	} else if err == client.ErrNameExpired {
		fmt.Printf("✗ Name has expired: %s\n", testName)
	} else if err != nil {
		log.Fatalf("Failed to resolve name: %v", err)
	} else {
		fmt.Printf("✓ Name resolved successfully!\n\n")
		fmt.Println("Name Record:")
		fmt.Printf("  Name: %s\n", record.Name)
		fmt.Printf("  Value: %s\n", record.Value)
		fmt.Printf("  Owner: %s\n", record.Address)
		fmt.Printf("  TX Hash: %s\n", record.TxHash)
		fmt.Printf("  Height: %d\n", record.Height)
		fmt.Printf("  Expires At: %d\n", record.ExpiresAt)
		fmt.Printf("  Expires In: %d blocks\n", record.ExpiresIn)
		fmt.Printf("  Updated: %s\n", record.UpdatedAt)
	}

	fmt.Println("\n✓ Example completed successfully")
}

func getTempDataDir() string {
	tmpDir := os.TempDir()
	return filepath.Join(tmpDir, "nmcd-example")
}
