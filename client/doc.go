// Package client provides a high-level interface for interacting with the Namecoin network.
//
// The client package offers a unified API for name resolution, registration, and
// management that works in both embedded (in-process) and daemon (RPC) modes.
// This enables applications to seamlessly switch between running their own
// Namecoin node or connecting to an external one.
//
// # Operation Modes
//
// Three client modes are supported:
//
//   - ModeAuto: Automatically detects daemon, falls back to embedded
//   - ModeEmbedded: Runs blockchain in-process (no external dependencies)
//   - ModeDaemon: Connects to external nmcd or Namecoin Core via RPC
//
// ModeAuto is recommended for most applications as it provides flexibility
// without code changes.
//
// # NameClient Interface
//
// The NameClient interface defines the primary API:
//
//   - ResolveName: Look up a name's current value
//   - RegisterName: Two-step NAME_NEW + NAME_FIRSTUPDATE registration
//   - UpdateName: Update an existing name's value
//   - ListNames: List names with optional filtering
//   - GetNameHistory: Retrieve complete history of a name
//
// All methods accept a context.Context for timeout and cancellation support.
//
// # Embedded Mode
//
// EmbeddedClient runs a complete Namecoin node in-process:
//
//   - No external daemon required
//   - Full blockchain validation
//   - Local name database storage
//   - Suitable for applications needing guaranteed availability
//
// Note: Embedded mode requires significant disk space for blockchain data
// and initial sync time.
//
// # Daemon Mode
//
// DaemonClient connects to an external node via JSON-RPC:
//
//   - Lower resource usage
//   - Shared node across applications
//   - Requires running nmcd or Namecoin Core
//   - Network latency for each operation
//
// # Name Registration
//
// Name registration is a two-step process:
//
//  1. NAME_NEW: Creates a commitment hash to prevent front-running
//  2. NAME_FIRSTUPDATE: Reveals the name and sets initial value
//
// The RegisterName method handles both steps automatically with appropriate
// timing (minimum 12 blocks between steps).
//
// # Thread Safety
//
// Both EmbeddedClient and DaemonClient are safe for concurrent use.
// Multiple goroutines can call any method simultaneously.
//
// # Error Handling
//
// Common errors:
//
//   - ErrNameNotFound: Name doesn't exist or has expired
//   - ErrNameExpired: Name exists but has expired
//   - ErrNameExists: Cannot register a name that already exists
//   - ErrInvalidName: Name violates protocol rules
//   - ErrInvalidValue: Value too large or invalid format
//
// # Example Usage
//
// Creating a client:
//
//	// Auto-detect mode (recommended)
//	client, err := client.NewClient(&client.Config{
//	    Mode:    client.ModeAuto,
//	    Network: "mainnet",
//	    DataDir: "/path/to/data",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
// Resolving a name:
//
//	ctx := context.Background()
//	record, err := client.ResolveName(ctx, "d/example")
//	if err == client.ErrNameNotFound {
//	    fmt.Println("Name not registered")
//	} else if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Value: %s\n", record.Value)
//
// Registering a name:
//
//	result, err := client.RegisterName(ctx, "d/mysite", `{"ip":"1.2.3.4"}`, nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Registration TX: %s\n", result.TxHash)
//
// Listing names:
//
//	names, err := client.ListNames(ctx, &client.ListFilter{
//	    Namespace: "d/",
//	    Limit:     100,
//	})
//	for _, name := range names {
//	    fmt.Printf("%s = %s\n", name.Name, name.Value)
//	}
//
// # Configuration
//
// Client configuration options:
//
//   - Mode: Operation mode (ModeAuto, ModeEmbedded, ModeDaemon)
//   - Network: Network to connect to ("mainnet", "testnet", "regtest")
//   - DataDir: Data directory for embedded mode
//   - RPCAddr: Daemon RPC address for daemon mode
//   - RPCUser/RPCPassword: Authentication credentials
//   - Timeout: Default timeout for operations
//
// # Semantic Versioning
//
// The client package follows semantic versioning:
//
//   - MAJOR: Breaking API changes
//   - MINOR: New features, backward compatible
//   - PATCH: Bug fixes only
package client
