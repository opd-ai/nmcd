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

## Additional Methods
See source code documentation for complete RPC method reference.
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
