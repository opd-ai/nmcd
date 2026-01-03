# nmcd Examples

This directory contains example programs demonstrating how to use nmcd components.

## Embedded Client Example (NEW)

The `embedded_client/` directory demonstrates using the embedded Namecoin client:

- Creating an in-process embedded client
- Resolving names from the local database
- Getting node information
- Proper resource cleanup

Run it with:

```bash
go run examples/embedded_client/main.go [datadir]
# or
cd examples/embedded_client && go run main.go [datadir]
```

**Example output:**
```
Using data directory: /tmp/nmcd-example

Initializing embedded Namecoin client...
✓ Client initialized successfully

Node Information:
  Version: 0.1.0
  Network: regtest
  Mode: embedded
  Block Height: 0
  ...
```

**Phase 2 Foundation Note:** The current implementation supports read-only operations (ResolveName, GetInfo). Full blockchain sync, name registration (RegisterName), and name updates (UpdateName) will be added in future phases.

## Name Database Example

The `namedb/` directory demonstrates basic name database operations:

- Creating and opening a name database
- Storing name records
- Retrieving name information
- Checking for expired names
- Adding to history

Run it with:

```bash
go run examples/namedb/main.go
# or
cd examples/namedb && go run main.go
```

## Using nmcd Components

The examples show how to use nmcd as a library in your own Go programs.

### Embedded Client (Recommended)

```go
import "github.com/opd-ai/nmcd/client"

// Create embedded client
cfg := &client.Config{
    Mode:    client.ModeEmbedded,
    DataDir: "/path/to/data",
    Network: "mainnet",
}

nc, err := client.NewEmbeddedClient(cfg)
if err != nil {
    log.Fatal(err)
}
defer nc.Close()

// Resolve a name
ctx := context.Background()
record, err := nc.ResolveName(ctx, "d/example")
if err == client.ErrNameNotFound {
    fmt.Println("Name not found")
} else if err != nil {
    log.Fatal(err)
} else {
    fmt.Printf("Value: %s\nOwner: %s\n", record.Value, record.Address)
}

// Get node info
info, err := nc.GetInfo(ctx)
fmt.Printf("Network: %s\nHeight: %d\n", info.NetworkName, info.BlockHeight)
```

### Import Packages

```go
import (
    "github.com/opd-ai/nmcd/namedb"
    "github.com/opd-ai/nmcd/chain"
    "github.com/opd-ai/nmcd/config"
)
```

### Name Database Operations

```go
// Open database
db, err := namedb.NewNameDatabase("/path/to/names.db")
if err != nil {
    log.Fatal(err)
}
defer db.Close()

// Store a name
record := &namedb.NameRecord{
    Name:      "d/example",
    Value:     `{"ip":"1.2.3.4"}`,
    Height:    100,
    ExpiresAt: 36100,
}
err = db.PutName("d/example", record)

// Retrieve a name
retrieved, err := db.GetName("d/example")
```

### Configuration

```go
// Create default config
cfg := config.DefaultConfig()

// Or custom config
cfg := &config.Config{
    DataDir: "/var/lib/nmcd",
    Network: "testnet",
    RPCAddr: "127.0.0.1:18336",
}

// Get chain parameters
params := cfg.ChainParams()
```

## Notes

- The examples use temporary databases that are cleaned up automatically
- Name records expire after 36,000 blocks (~250 days)
- All operations are thread-safe with mutex protection
