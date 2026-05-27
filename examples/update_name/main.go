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
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/opd-ai/nmcd/client"
	"github.com/opd-ai/nmcd/examples/shared"
)

func main() {
	if len(os.Args) < 3 {
		printUsage()
		os.Exit(1)
	}

	name := os.Args[1]
	newValue := os.Args[2]

	validateNameAndValue(name, newValue)

	nc := initEmbeddedClient()
	defer nc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	printInfo(nc, ctx)
	showCurrentRecord(nc, ctx, name)

	fmt.Println("\nUpdating name...")
	fmt.Printf("  New Value: %s\n", newValue)

	result, err := nc.UpdateName(ctx, name, newValue, &client.UpdateOpts{
		WaitForConfirmation: false,
		FeeRate:             1,
	})
	if err != nil {
		handleUpdateError(err)
	}

	printUpdateResult(result)
}

func printUsage() {
	fmt.Println("Usage: update_name <name> <new_value>")
	fmt.Println("Examples:")
	fmt.Println("  update_name d/mysite '{\"ip\":\"5.6.7.8\"}'")
	fmt.Println("  update_name id/alice '{\"name\":\"Alice\",\"verified\":true}'")
}

func validateNameAndValue(name, value string) {
	if err := shared.ValidateNameNamespace(name); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if err := shared.ValidateNameValue(name, value); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func initEmbeddedClient() *client.EmbeddedClient {
	cfg := &client.Config{
		Mode:          client.ModeEmbedded,
		Network:       "regtest",
		DataDir:       os.TempDir() + "/nmcd-update-example",
		DisableWallet: false,
	}

	fmt.Println("Initializing Namecoin client...")
	nc, err := client.NewEmbeddedClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	return nc
}

func printInfo(nc *client.EmbeddedClient, ctx context.Context) {
	info, err := nc.GetInfo(ctx)
	if err != nil {
		log.Fatalf("Failed to get node info: %v", err)
	}
	fmt.Printf("Connected to %s network (height: %d)\n\n", info.NetworkName, info.BlockHeight)
}

func showCurrentRecord(nc *client.EmbeddedClient, ctx context.Context, name string) {
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
}

func handleUpdateError(err error) {
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

func printUpdateResult(result *client.TxResult) {
	shared.PrintTxResult(result,
		"✓ Update transaction created!",
		"Note: Transaction is pending. Once confirmed:\n  - The name's value will be updated\n  - The expiration will be extended by 36,000 blocks (~250 days)",
	)
}
