# nmcd Client API Reference

This document provides a complete API reference for the nmcd client library, enabling programmatic access to Namecoin name resolution, registration, and management.

## Table of Contents

- [Quick Start](#quick-start)
- [Client Creation](#client-creation)
- [NameClient Interface](#nameclient-interface)
- [Types](#types)
- [Error Handling](#error-handling)
- [Client Modes](#client-modes)
- [Configuration Options](#configuration-options)
- [Thread Safety](#thread-safety)

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/opd-ai/nmcd/client"
)

func main() {
    // Create client with auto-detection (daemon if available, else embedded)
    nc, err := client.NewClient(nil)
    if err != nil {
        log.Fatal(err)
    }
    defer nc.Close()

    // Resolve a name
    ctx := context.Background()
    record, err := nc.ResolveName(ctx, "d/example")
    if errors.Is(err, client.ErrNameNotFound) {
        fmt.Println("Name not found")
        return
    } else if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Name: %s\nValue: %s\nOwner: %s\n",
        record.Name, record.Value, record.Address)
}
```

## Client Creation

### NewClient

```go
func NewClient(cfg *Config) (NameClient, error)
```

Creates a new NameClient based on the configuration. Automatically selects between embedded and daemon modes.

**Mode Selection Logic:**
- `ModeAuto` (default): Tries daemon first (localhost:8336), falls back to embedded
- `ModeEmbedded`: Forces in-process embedded mode
- `ModeDaemon`: Forces daemon mode (RPC only)

**Parameters:**
- `cfg`: Client configuration. If nil, uses default configuration with ModeAuto.

**Returns:**
- `NameClient`: Initialized client (EmbeddedClient or DaemonClient)
- `error`: Initialization error, or nil on success

**Examples:**

```go
// Auto-detection (recommended for most applications)
client, err := client.NewClient(nil)

// Explicit embedded mode
client, err := client.NewClient(&client.Config{
    Mode:    client.ModeEmbedded,
    DataDir: "/path/to/data",
    Network: "mainnet",
})

// Explicit daemon mode with authentication
client, err := client.NewClient(&client.Config{
    Mode:        client.ModeDaemon,
    RPCAddr:     "http://localhost:8336",
    RPCUser:     "user",
    RPCPassword: "pass",
})
```

### NewEmbeddedClient

```go
func NewEmbeddedClient(cfg *Config) (*EmbeddedClient, error)
```

Creates a new embedded Namecoin client with local database and blockchain validation.

**Parameters:**
- `cfg`: Client configuration. If nil, uses default configuration.

**Returns:**
- `*EmbeddedClient`: Initialized embedded client
- `error`: Initialization error, or nil on success

### NewDaemonClient

```go
func NewDaemonClient(cfg *Config) (*DaemonClient, error)
```

Creates a new daemon client that connects to an external nmcd or Namecoin Core daemon via JSON-RPC.

**Parameters:**
- `cfg`: Client configuration. If nil, uses default configuration.

**Returns:**
- `*DaemonClient`: Initialized daemon client
- `error`: Initialization error, or nil on success

## NameClient Interface

```go
type NameClient interface {
    ResolveName(ctx context.Context, name string) (*NameRecord, error)
    RegisterName(ctx context.Context, name, value string, opts *RegisterOpts) (*TxResult, error)
    UpdateName(ctx context.Context, name, value string, opts *UpdateOpts) (*TxResult, error)
    ListNames(ctx context.Context, filter *ListFilter) ([]*NameRecord, error)
    GetNameHistory(ctx context.Context, name string) ([]*NameRecord, error)
    WaitForConfirmation(ctx context.Context, txHash string, confirmations int) error
    GetInfo(ctx context.Context) (*NodeInfo, error)
    Close() error
}
```

All implementations are thread-safe and support context cancellation.

### ResolveName

```go
func ResolveName(ctx context.Context, name string) (*NameRecord, error)
```

Retrieves the current value and metadata for a name.

**Parameters:**
- `ctx`: Context for cancellation and timeout
- `name`: Name to resolve (e.g., "d/example", "id/alice")

**Returns:**
- `*NameRecord`: Name record with current value and metadata
- `error`: `ErrNameNotFound` if name doesn't exist, `ErrNameExpired` if expired

**Example:**

```go
record, err := client.ResolveName(ctx, "d/example")
if errors.Is(err, client.ErrNameNotFound) {
    fmt.Println("Name not registered")
    return
}
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Value: %s\n", record.Value)
fmt.Printf("Owner: %s\n", record.Address)
fmt.Printf("Expires in: %d blocks\n", record.ExpiresIn)
```

### RegisterName

```go
func RegisterName(ctx context.Context, name, value string, opts *RegisterOpts) (*TxResult, error)
```

Creates a new name registration with the given value. Implements Namecoin's two-phase registration process (NAME_NEW → NAME_FIRSTUPDATE).

**Parameters:**
- `ctx`: Context for cancellation and timeout
- `name`: Name to register (1-255 characters)
- `value`: Initial value (max 1023 bytes, typically JSON)
- `opts`: Registration options

**Returns:**
- `*TxResult`: Result containing transaction hash and status
- `error`: Any error encountered

**Registration Options:**

```go
type RegisterOpts struct {
    FromAddress         string  // Wallet address to use (optional)
    WaitForConfirmation bool    // Wait for confirmation (default: false)
    Confirmations       int     // Number of confirmations (default: 1)
    FeeRate             int64   // Satoshis per byte (default: 1)
}
```

**Example:**

```go
result, err := client.RegisterName(ctx, "d/mysite", `{"ip":"1.2.3.4"}`, &client.RegisterOpts{
    FeeRate: 1,
})
if err != nil {
    log.Fatal(err)
}
fmt.Printf("NAME_NEW tx: %s\n", result.TxHash)
```

**Note:** The full two-phase registration (waiting for 12 blocks, then NAME_FIRSTUPDATE) requires network integration, which is in development.

### UpdateName

```go
func UpdateName(ctx context.Context, name, value string, opts *UpdateOpts) (*TxResult, error)
```

Updates an existing name's value using NAME_UPDATE.

**Parameters:**
- `ctx`: Context for cancellation and timeout
- `name`: Name to update (must exist and not be expired)
- `value`: New value (max 1023 bytes)
- `opts`: Update options

**Returns:**
- `*TxResult`: Result containing transaction hash and status
- `error`: Any error encountered

**Update Options:**

```go
type UpdateOpts struct {
    TransferTo          string  // Transfer to new address (optional)
    WaitForConfirmation bool    // Wait for confirmation (default: false)
    Confirmations       int     // Number of confirmations (default: 1)
    FeeRate             int64   // Satoshis per byte (default: 1)
}
```

**Example:**

```go
result, err := client.UpdateName(ctx, "d/mysite", `{"ip":"5.6.7.8"}`, nil)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("NAME_UPDATE tx: %s\n", result.TxHash)
```

### ListNames

```go
func ListNames(ctx context.Context, filter *ListFilter) ([]*NameRecord, error)
```

Returns all registered names, optionally filtered.

**Parameters:**
- `ctx`: Context for cancellation and timeout
- `filter`: Optional filter configuration

**Returns:**
- `[]*NameRecord`: Slice of matching name records
- `error`: Error retrieving names

**Filter Options:**

```go
type ListFilter struct {
    NamePattern    string  // Prefix to match (e.g., "d/example")
    Namespace      string  // Namespace prefix (e.g., "d/", "id/")
    Address        string  // Filter by owner address
    IncludeExpired bool    // Include expired names (default: false)
    Limit          int     // Max results (default: 100, max: 10000)
    Offset         int     // Pagination offset (default: 0)
}
```

**Example:**

```go
// List all domain names
records, err := client.ListNames(ctx, &client.ListFilter{
    Namespace: "d/",
    Limit:     100,
})
if err != nil {
    log.Fatal(err)
}

for _, record := range records {
    fmt.Printf("%s: %s\n", record.Name, record.Value)
}
```

### GetNameHistory

```go
func GetNameHistory(ctx context.Context, name string) ([]*NameRecord, error)
```

Returns the full history of operations for a name.

**Parameters:**
- `ctx`: Context for cancellation and timeout
- `name`: Name to retrieve history for

**Returns:**
- `[]*NameRecord`: Chronological history (oldest first)
- `error`: Error retrieving history

**Example:**

```go
history, err := client.GetNameHistory(ctx, "d/example")
if err != nil {
    log.Fatal(err)
}

for i, record := range history {
    fmt.Printf("Operation %d: Height=%d, Value=%s\n",
        i+1, record.Height, record.Value)
}
```

### WaitForConfirmation

```go
func WaitForConfirmation(ctx context.Context, txHash string, confirmations int) error
```

Waits for a transaction to be confirmed in a block.

**Parameters:**
- `ctx`: Context for cancellation and timeout
- `txHash`: Transaction hash (hex string)
- `confirmations`: Number of confirmations to wait for (min: 1)

**Returns:**
- `error`: `ErrContextCanceled` if cancelled, or other errors

**Note:** Full implementation requires blockchain notification system (in development).

### GetInfo

```go
func GetInfo(ctx context.Context) (*NodeInfo, error)
```

Returns general information about the node/network state.

**Returns:**
- `*NodeInfo`: Node information
- `error`: Error retrieving information

**Example:**

```go
info, err := client.GetInfo(ctx)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Network: %s\n", info.NetworkName)
fmt.Printf("Mode: %s\n", info.Mode)
fmt.Printf("Height: %d\n", info.BlockHeight)
fmt.Printf("Connections: %d\n", info.Connections)
```

### Close

```go
func Close() error
```

Releases resources held by the client. After Close is called, the client cannot be reused.

- **EmbeddedClient**: Closes database and stops background goroutines
- **DaemonClient**: Closes HTTP connection pool

## Types

### NameRecord

```go
type NameRecord struct {
    Name      string    // Name identifier (e.g., "d/example")
    Value     string    // Current value (typically JSON)
    TxHash    string    // Transaction hash of last operation
    Height    int32     // Block height of last operation
    ExpiresAt int32     // Block height when name expires
    ExpiresIn int32     // Blocks remaining until expiration
    Address   string    // Current owner address
    UpdatedAt time.Time // Timestamp of last update
}
```

### TxResult

```go
type TxResult struct {
    TxHash        string   // Transaction hash
    Name          string   // Name operated on
    Status        TxStatus // pending, confirmed, or failed
    Confirmations int      // Number of confirmations
    BlockHeight   int32    // Block height (if confirmed)
    BlockHash     string   // Block hash (if confirmed)
}
```

### TxStatus

```go
const (
    TxStatusPending   TxStatus = "pending"
    TxStatusConfirmed TxStatus = "confirmed"
    TxStatusFailed    TxStatus = "failed"
)
```

### NodeInfo

```go
type NodeInfo struct {
    Version         string // nmcd version
    ProtocolVersion int    // Network protocol version
    BlockHeight     int32  // Current blockchain height
    BestBlockHash   string // Hash of best block
    Connections     int    // Number of peer connections
    NetworkName     string // Network name (mainnet, testnet, regtest)
    Mode            string // Client mode (embedded, daemon)
}
```

## Error Handling

The client package defines standard errors for common conditions:

```go
var (
    ErrNameNotFound      = errors.New("name not found")
    ErrNameExpired       = errors.New("name has expired")
    ErrNameExists        = errors.New("name already registered")
    ErrInvalidName       = errors.New("invalid name format")
    ErrInvalidValue      = errors.New("invalid value format")
    ErrInsufficientFunds = errors.New("insufficient funds for operation")
    ErrNoWallet          = errors.New("wallet not initialized")
    ErrDaemonUnavailable = errors.New("daemon unavailable")
    ErrContextCanceled   = context.Canceled
)
```

**Usage with errors.Is:**

```go
record, err := client.ResolveName(ctx, name)
switch {
case errors.Is(err, client.ErrNameNotFound):
    fmt.Println("Name not registered")
case errors.Is(err, client.ErrNameExpired):
    fmt.Println("Name has expired")
case err != nil:
    log.Fatal(err)
default:
    fmt.Printf("Found: %s\n", record.Value)
}
```

## Client Modes

### ModeAuto (Default)

Automatically detects whether a daemon is running and uses it if available, otherwise falls back to embedded mode.

```go
client, err := client.NewClient(nil) // Uses ModeAuto
```

### ModeEmbedded

In-process client with local database and blockchain validation. No external daemon required.

```go
client, err := client.NewClient(&client.Config{
    Mode:    client.ModeEmbedded,
    DataDir: "/path/to/data",
    Network: "mainnet",
})
```

**Advantages:**
- No external dependencies
- Full control over data
- Works offline (after initial sync)

**Use Cases:**
- Standalone applications
- IoT devices
- Testing and development

### ModeDaemon

RPC client that connects to an external nmcd or Namecoin Core daemon.

```go
client, err := client.NewClient(&client.Config{
    Mode:        client.ModeDaemon,
    RPCAddr:     "http://localhost:8336",
    RPCUser:     "user",
    RPCPassword: "pass",
})
```

**Advantages:**
- Lightweight client
- Shared blockchain data
- Works with existing daemon installations

**Use Cases:**
- Web applications
- Microservices
- Shared infrastructure

## Configuration Options

```go
type Config struct {
    Mode           ClientMode // ModeAuto, ModeEmbedded, or ModeDaemon
    DataDir        string     // Data directory (default: ~/.nmcd)
    Network        string     // mainnet, testnet, or regtest
    RPCAddr        string     // Daemon RPC address (default: http://localhost:8336)
    RPCUser        string     // RPC username
    RPCPassword    string     // RPC password
    MaxPeers       int        // Max peer connections (default: 8)
    BootstrapPeers []string   // Initial peers (default: DNS seeds)
    DisableWallet  bool       // Disable wallet functionality
    Logger         *logging.Logger // Custom logger for client operations (default: uses internal/logging default logger)
}
```

### Custom Logger Configuration

The client package supports custom logging via the `Logger` field in the configuration. This allows applications to control log output format, destination, and verbosity level.

**Example: File Logging with JSON Format**

```go
import (
    "github.com/opd-ai/nmcd/client"
    "github.com/opd-ai/nmcd/internal/logging"
)

// Create custom logger
loggerCfg := &logging.Config{
    Level:     logging.LevelDebug,  // DEBUG, INFO, WARN, ERROR
    Format:    "json",               // "json" or "text"
    Output:    "/var/log/nmcd/client.log", // File path, "stdout", or "stderr"
    Component: "my-app",
}

logger, err := logging.Init(loggerCfg)
if err != nil {
    log.Fatal(err)
}
defer logger.Close()

// Use logger with client
cfg := &client.Config{
    Mode:   client.ModeEmbedded,
    Logger: logger,
}

client, err := client.NewClient(cfg)
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

**Available Log Levels:**
- `logging.LevelDebug` - Detailed debug information
- `logging.LevelInfo` - General informational messages
- `logging.LevelWarn` - Warning messages (e.g., broadcast failures)
- `logging.LevelError` - Error messages

**Output Formats:**
- `"json"` - Structured JSON logging (recommended for production)
- `"text"` - Human-readable text format (useful for development)

**Note:** If `Logger` is nil, the client uses the default logger from the `internal/logging` package with INFO level and text format to stdout.

## Thread Safety

All NameClient implementations are fully thread-safe:

- **EmbeddedClient**: Uses `sync.RWMutex` for state protection
- **DaemonClient**: Uses `sync.RWMutex` and thread-safe HTTP client

You can safely call methods from multiple goroutines:

```go
var wg sync.WaitGroup
names := []string{"d/example1", "d/example2", "d/example3"}

for _, name := range names {
    wg.Add(1)
    go func(n string) {
        defer wg.Done()
        record, err := client.ResolveName(ctx, n)
        if err != nil {
            return
        }
        fmt.Printf("%s: %s\n", record.Name, record.Value)
    }(name)
}

wg.Wait()
```

---

## See Also

- [EMBEDDING.md](EMBEDDING.md) - Guide for embedding nmcd in applications
- [examples/](../examples/) - Example applications
- [README.md](../README.md) - Project overview
