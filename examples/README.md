# nmcd Examples

This directory contains example programs demonstrating how to use nmcd components.

## Name Database Example

The `namedb_example.go` demonstrates basic name database operations:

- Creating and opening a name database
- Storing name records
- Retrieving name information
- Checking for expired names
- Adding to history

Run it with:

```bash
go run examples/namedb_example.go
```

## Using nmcd Components

The examples show how to use nmcd as a library in your own Go programs.

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
