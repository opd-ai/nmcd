# nmcd Examples

This directory contains example programs demonstrating how to use nmcd components.

## Quick Start Examples

### Simple Name Resolution (`simple_resolve/`)

Demonstrates basic Namecoin name resolution:

```bash
go run ./examples/simple_resolve d/example
go run ./examples/simple_resolve id/alice
```

Features:
- Auto-detection of daemon or embedded mode
- Error handling for name not found and expired names
- Display of complete name record information

### Name Registration (`register_name/`)

Demonstrates registering a new Namecoin name:

```bash
go run ./examples/register_name d/mysite '{"ip":"1.2.3.4"}'
go run ./examples/register_name id/alice '{"name":"Alice"}'
```

Features:
- NAME_NEW + NAME_FIRSTUPDATE two-step registration process
- JSON value validation for d/ and id/ namespaces
- Wallet integration for signing transactions

### Name Update (`update_name/`)

Demonstrates updating an existing name's value:

```bash
go run ./examples/update_name d/mysite '{"ip":"5.6.7.8"}'
go run ./examples/update_name id/alice '{"name":"Alice","verified":true}'
```

Features:
- NAME_UPDATE transaction creation
- Current record lookup before update
- Expiration extension (36,000 blocks added)

### Name Listing (`list_names/`)

Demonstrates listing and filtering names:

```bash
go run ./examples/list_names
go run ./examples/list_names --namespace=d/
go run ./examples/list_names --address=N1abc...
go run ./examples/list_names --pattern=d/example --limit=10
```

Options:
- `--namespace=<ns>`: Filter by namespace (d/, id/, p/)
- `--address=<addr>`: Filter by owner address
- `--pattern=<prefix>`: Filter by name prefix
- `--include-expired`: Include expired names
- `--limit=<n>`: Limit results (default: 100)
- `--offset=<n>`: Pagination offset

---

## Mail System Examples

### Bridge Adapter (`bridge_adapter/`)

Demonstrates the bridge between Namecoin and the mail routing system:

```bash
go run ./examples/bridge_adapter
```

Features:
- MailConfig extraction from Namecoin name records
- JSON parsing of email forwarding configuration
- Base64 public key decoding
- Error handling for invalid configurations

### Mail Router (`mail_router/`)

Demonstrates .bit email address routing:

```bash
go run ./examples/mail_router
```

Features:
- Resolution of .bit addresses to real email addresses
- Caching with configurable TTL
- Thread-safe concurrent operations
- Mock resolver for testing

### SMTP Relay (`smtp_relay/`)

Complete SMTP relay server that forwards .bit addresses to real email:

```bash
go run ./examples/smtp_relay \
  -listen=":2525" \
  -upstream-host="smtp.gmail.com" \
  -upstream-port=587 \
  -upstream-user="your-email@gmail.com" \
  -upstream-pass="your-app-password"
```

Features:
- Full SMTP protocol implementation
- .bit address validation and routing
- Upstream SMTP forwarding
- Graceful shutdown
- Production-ready with systemd integration

See [smtp_relay/README.md](smtp_relay/README.md) for detailed configuration and deployment instructions.

---

## Prometheus Metrics Example

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
