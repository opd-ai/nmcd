package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/opd-ai/nmcd/namedb"
)

// Example demonstrates basic name database operations
func main() {
	// Create temporary database
	dbPath := "/tmp/example-names.db"
	defer os.Remove(dbPath)

	// Open name database
	db, err := namedb.NewNameDatabase(dbPath)
	if err != nil {
		log.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	fmt.Println("=== Name Database Example ===")
	fmt.Println()

	// Create a name record
	hash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	record := &namedb.NameRecord{
		Name:      "d/example",
		Value:     `{"ip":"192.168.1.1","info":"Example domain"}`,
		TxHash:    *hash,
		Height:    1000,
		ExpiresAt: 37000, // Expires at height 37000
		Address:   "N1ExampleAddress",
		UpdatedAt: time.Now(),
	}

	// Store the name
	fmt.Printf("Storing name: %s\n", record.Name)
	err = db.PutName("d/example", record)
	if err != nil {
		log.Fatalf("Failed to store name: %v", err)
	}

	// Retrieve the name
	fmt.Println("\nRetrieving name...")
	retrieved, err := db.GetName("d/example")
	if err != nil {
		log.Fatalf("Failed to retrieve name: %v", err)
	}

	fmt.Printf("  Name: %s\n", retrieved.Name)
	fmt.Printf("  Value: %s\n", retrieved.Value)
	fmt.Printf("  Height: %d\n", retrieved.Height)
	fmt.Printf("  Expires At: %d\n", retrieved.ExpiresAt)
	fmt.Printf("  Address: %s\n", retrieved.Address)
	fmt.Printf("  Updated: %s\n", retrieved.UpdatedAt.Format(time.RFC3339))

	// Add another name that will expire sooner
	hash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000002")
	record2 := &namedb.NameRecord{
		Name:      "d/temporary",
		Value:     `{"status":"temporary"}`,
		TxHash:    *hash2,
		Height:    1000,
		ExpiresAt: 2000,
		UpdatedAt: time.Now(),
	}
	db.PutName("d/temporary", record2)

	// Check for expired names
	fmt.Println("\nChecking for expired names at height 2500...")
	expired, err := db.GetExpiredNames(2500)
	if err != nil {
		log.Fatalf("Failed to get expired names: %v", err)
	}

	if len(expired) > 0 {
		fmt.Printf("Found %d expired name(s):\n", len(expired))
		for _, name := range expired {
			fmt.Printf("  - %s\n", name)
		}
	} else {
		fmt.Println("No expired names found")
	}

	// Add to history
	fmt.Println("\nAdding to history...")
	err = db.AddHistory(*hash, record)
	if err != nil {
		log.Fatalf("Failed to add history: %v", err)
	}
	fmt.Println("History recorded successfully")

	fmt.Println("\n=== Example Complete ===")
}
