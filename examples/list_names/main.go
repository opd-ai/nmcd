// Package main demonstrates Namecoin name listing and filtering using the nmcd client library.
//
// This example shows how to:
//   - Create a client and list all registered names
//   - Filter names by namespace (d/, id/, p/)
//   - Filter names by owner address
//   - Use pagination for large result sets
//   - Include or exclude expired names
//
// Usage:
//
//	go run list_names.go [options]
//
// Options:
//
//	--namespace=<ns>   Filter by namespace (d/, id/, p/)
//	--address=<addr>   Filter by owner address
//	--pattern=<prefix> Filter by name prefix
//	--include-expired  Include expired names
//	--limit=<n>        Limit number of results (default: 100)
//	--offset=<n>       Skip first n results (for pagination)
//
// Examples:
//
//	go run list_names.go
//	go run list_names.go --namespace=d/
//	go run list_names.go --address=N1abc...
//	go run list_names.go --pattern=d/example --limit=10
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/opd-ai/nmcd/client"
)

func main() {
	// Parse command-line flags
	namespace := flag.String("namespace", "", "Filter by namespace (d/, id/, p/)")
	address := flag.String("address", "", "Filter by owner address")
	pattern := flag.String("pattern", "", "Filter by name prefix pattern")
	includeExpired := flag.Bool("include-expired", false, "Include expired names")
	limit := flag.Int("limit", 100, "Maximum number of results")
	offset := flag.Int("offset", 0, "Number of results to skip (pagination)")
	flag.Parse()

	// Validate namespace if provided
	if *namespace != "" && *namespace != "d/" && *namespace != "id/" && *namespace != "p/" {
		fmt.Println("Error: Invalid namespace. Must be d/, id/, or p/")
		os.Exit(1)
	}

	// Validate limit
	if *limit < 1 || *limit > 10000 {
		fmt.Println("Error: Limit must be between 1 and 10000")
		os.Exit(1)
	}

	// Create client with auto-detection
	cfg := &client.Config{
		Mode:    client.ModeAuto,
		Network: "regtest", // Use regtest for local testing
		DataDir: os.TempDir() + "/nmcd-list-example",
	}

	fmt.Println("Initializing Namecoin client...")
	nc, err := client.NewClient(cfg)
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
	fmt.Printf("Connected to %s network in %s mode (height: %d)\n\n", info.NetworkName, info.Mode, info.BlockHeight)

	// Build filter
	filter := &client.ListFilter{
		Namespace:      *namespace,
		Address:        *address,
		NamePattern:    *pattern,
		IncludeExpired: *includeExpired,
		Limit:          *limit,
		Offset:         *offset,
	}

	// Print filter settings
	fmt.Println("Filter Settings:")
	if filter.Namespace != "" {
		fmt.Printf("  Namespace:       %s\n", filter.Namespace)
	}
	if filter.Address != "" {
		fmt.Printf("  Address:         %s\n", filter.Address)
	}
	if filter.NamePattern != "" {
		fmt.Printf("  Pattern:         %s*\n", filter.NamePattern)
	}
	fmt.Printf("  Include Expired: %v\n", filter.IncludeExpired)
	fmt.Printf("  Limit:           %d\n", filter.Limit)
	if filter.Offset > 0 {
		fmt.Printf("  Offset:          %d\n", filter.Offset)
	}
	fmt.Println()

	// List names
	fmt.Println("Listing names...")
	records, err := nc.ListNames(ctx, filter)
	if err != nil {
		log.Fatalf("Failed to list names: %v", err)
	}

	if len(records) == 0 {
		fmt.Println("No names found matching the filter criteria.")
		if !filter.IncludeExpired {
			fmt.Println("\nTip: Use --include-expired to include expired names.")
		}
		return
	}

	fmt.Printf("Found %d name(s):\n\n", len(records))

	// Display results in a formatted table
	fmt.Printf("%-30s %-40s %-15s %s\n", "NAME", "VALUE (truncated)", "EXPIRES IN", "OWNER")
	fmt.Printf("%-30s %-40s %-15s %s\n", "----", "-----", "----------", "-----")

	for _, record := range records {
		// Truncate value for display
		value := record.Value
		if len(value) > 37 {
			value = value[:37] + "..."
		}

		// Calculate expiration status
		var expiresStr string
		if record.ExpiresIn <= 0 {
			expiresStr = "EXPIRED"
		} else {
			days := float64(record.ExpiresIn) * 10 / 60 / 24
			expiresStr = fmt.Sprintf("%d blocks (~%.0fd)", record.ExpiresIn, days)
		}

		// Truncate address for display
		addr := record.Address
		if len(addr) > 12 {
			addr = addr[:6] + "..." + addr[len(addr)-4:]
		}

		fmt.Printf("%-30s %-40s %-15s %s\n", record.Name, value, expiresStr, addr)
	}

	// Pagination info
	if len(records) == filter.Limit {
		fmt.Printf("\n(Showing %d results. Use --offset=%d to see more.)\n", filter.Limit, filter.Offset+filter.Limit)
	}
}
