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
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/opd-ai/nmcd/client"
	"github.com/opd-ai/nmcd/examples/shared"
)

func main() {
	if len(os.Args) < 3 {
		printRegisterUsage()
		os.Exit(1)
	}

	name := os.Args[1]
	value := os.Args[2]

	validateRegisterInputs(name, value)

	nc := initRegisterClient()
	defer nc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	printRegisterInfo(nc, ctx)
	checkNameAvailability(nc, ctx, name)

	fmt.Println("Registering name...")
	fmt.Printf("  Name:  %s\n", name)
	fmt.Printf("  Value: %s\n", value)

	result, err := nc.RegisterName(ctx, name, value, &client.RegisterOpts{
		WaitForConfirmation: false,
		FeeRate:             1,
	})
	if err != nil {
		handleRegisterError(err)
	}

	printRegisterResult(result)
}

func printRegisterUsage() {
	fmt.Println("Usage: register_name <name> <value>")
	fmt.Println("Examples:")
	fmt.Println("  register_name d/mysite '{\"ip\":\"1.2.3.4\"}'")
	fmt.Println("  register_name id/alice '{\"name\":\"Alice\"}'")
}

func validateRegisterInputs(name, value string) {
	if !strings.HasPrefix(name, "d/") && !strings.HasPrefix(name, "id/") && !strings.HasPrefix(name, "p/") {
		fmt.Println("Error: Name must start with d/, id/, or p/ namespace")
		fmt.Println("Examples: d/example, id/alice, p/notes")
		os.Exit(1)
	}

	if strings.HasPrefix(name, "d/") || strings.HasPrefix(name, "id/") {
		var js json.RawMessage
		if err := json.Unmarshal([]byte(value), &js); err != nil {
			fmt.Printf("Error: Value must be valid JSON for %s namespace\n", strings.Split(name, "/")[0]+"/")
			fmt.Printf("Invalid JSON: %v\n", err)
			os.Exit(1)
		}
	}
}

func initRegisterClient() *client.EmbeddedClient {
	cfg := &client.Config{
		Mode:          client.ModeEmbedded,
		Network:       "regtest",
		DataDir:       os.TempDir() + "/nmcd-register-example",
		DisableWallet: false,
	}

	fmt.Println("Initializing Namecoin client...")
	nc, err := client.NewEmbeddedClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	return nc
}

func printRegisterInfo(nc *client.EmbeddedClient, ctx context.Context) {
	info, err := nc.GetInfo(ctx)
	if err != nil {
		log.Fatalf("Failed to get node info: %v", err)
	}
	fmt.Printf("Connected to %s network (height: %d)\n\n", info.NetworkName, info.BlockHeight)
}

func checkNameAvailability(nc *client.EmbeddedClient, ctx context.Context, name string) {
	fmt.Printf("Checking if name '%s' is available...\n", name)
	existingRecord, err := nc.ResolveName(ctx, name)
	if err == nil {
		fmt.Printf("✗ Name already registered!\n")
		fmt.Printf("  Owner: %s\n", existingRecord.Address)
		fmt.Printf("  Expires in: %d blocks\n", existingRecord.ExpiresIn)
		os.Exit(1)
	} else if !errors.Is(err, client.ErrNameNotFound) && !errors.Is(err, client.ErrNameExpired) {
		log.Fatalf("Failed to check name: %v", err)
	}
	fmt.Printf("✓ Name '%s' is available\n\n", name)
}

func handleRegisterError(err error) {
	switch {
	case errors.Is(err, client.ErrNameExists):
		fmt.Println("✗ Name already exists (race condition)")
	case errors.Is(err, client.ErrInsufficientFunds):
		fmt.Println("✗ Insufficient funds for registration")
		fmt.Println("  The wallet needs NMC to pay for the registration fee")
	case errors.Is(err, client.ErrNoWallet):
		fmt.Println("✗ Wallet not initialized")
	case errors.Is(err, client.ErrInvalidName):
		fmt.Println("✗ Invalid name format")
	case errors.Is(err, client.ErrInvalidValue):
		fmt.Println("✗ Invalid value format")
	default:
		log.Fatalf("Failed to register name: %v", err)
	}
	os.Exit(1)
}

func printRegisterResult(result *client.TxResult) {
	shared.PrintTxResult(result,
		"✓ Registration initiated!",
		"Note: NAME_NEW transaction created.\nAfter 12 blocks, NAME_FIRSTUPDATE will complete the registration.\nUse WaitForConfirmation: true to wait for full registration.",
	)
}
