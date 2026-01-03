# Embedding nmcd in Applications

This guide explains how to embed the nmcd Namecoin client directly in your Go applications, enabling decentralized name resolution without requiring an external daemon.

## Table of Contents

- [Why Embed nmcd?](#why-embed-nmcd)
- [Quick Start](#quick-start)
- [Architecture Overview](#architecture-overview)
- [Basic Usage](#basic-usage)
- [Advanced Configuration](#advanced-configuration)
- [Resource Management](#resource-management)
- [Common Patterns](#common-patterns)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Why Embed nmcd?

Embedding nmcd provides several advantages over connecting to an external daemon:

| Feature | Embedded Mode | Daemon Mode |
|---------|---------------|-------------|
| External dependencies | None | Requires running daemon |
| Startup time | Instant (local DB) | Instant (RPC connection) |
| Data control | Full control | Shared with daemon |
| Offline operation | Yes (after sync) | No |
| Resource usage | Higher (runs blockchain) | Lower (RPC only) |
| Deployment | Single binary | Daemon + client |

**Use embedded mode when:**
- Building standalone applications
- Need offline operation after initial sync
- Want full control over data directory
- Deploying to environments without existing daemon
- Testing and development

## Quick Start

### Installation

```bash
go get github.com/opd-ai/nmcd
```

### Minimal Example

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/opd-ai/nmcd/client"
)

func main() {
    // Create embedded client
    nc, err := client.NewEmbeddedClient(&client.Config{
        DataDir: "./data",
        Network: "regtest", // Use regtest for development
    })
    if err != nil {
        log.Fatal(err)
    }
    defer nc.Close()

    // Get node info
    ctx := context.Background()
    info, err := nc.GetInfo(ctx)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Network: %s\n", info.NetworkName)
    fmt.Printf("Height: %d\n", info.BlockHeight)
}
```

## Architecture Overview

When you embed nmcd, your application includes these components:

```
┌─────────────────────────────────────────────────────────────┐
│                   Your Application                           │
│  ┌────────────────────────────────────────────────────────┐ │
│  │                  EmbeddedClient                         │ │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────────────┐│ │
│  │  │ Blockchain │  │  NameDB    │  │      Wallet        ││ │
│  │  │ (btcd)     │  │  (bbolt)   │  │   (wallet.json)    ││ │
│  │  └────────────┘  └────────────┘  └────────────────────┘│ │
│  └────────────────────────────────────────────────────────┘ │
│                              │                               │
│                              ▼                               │
│                    Data Directory (~/.nmcd)                  │
│                    ├── blocks/                               │
│                    ├── names.db                              │
│                    └── wallet.json                           │
└─────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

- **Blockchain**: Block validation, chain management, UTXO tracking
- **NameDB**: Name records, history, expiration tracking
- **Wallet**: Private key storage, transaction signing

## Basic Usage

### Name Resolution

```go
// Resolve a domain name
record, err := nc.ResolveName(ctx, "d/example")
if errors.Is(err, client.ErrNameNotFound) {
    fmt.Println("Name not registered")
    return
}
if err != nil {
    log.Fatal(err)
}

// Parse JSON value for domain names
var dns struct {
    IP  string `json:"ip"`
    IP6 string `json:"ip6"`
}
if err := json.Unmarshal([]byte(record.Value), &dns); err != nil {
    log.Printf("Invalid DNS value: %v", err)
}

fmt.Printf("IPv4: %s\n", dns.IP)
fmt.Printf("IPv6: %s\n", dns.IP6)
```

### Listing Names

```go
// List all domain names
records, err := nc.ListNames(ctx, &client.ListFilter{
    Namespace: "d/",
    Limit:     100,
})
if err != nil {
    log.Fatal(err)
}

for _, record := range records {
    fmt.Printf("%s expires in %d blocks\n", record.Name, record.ExpiresIn)
}
```

### Name History

```go
// Get complete history of name operations
history, err := nc.GetNameHistory(ctx, "d/example")
if err != nil {
    log.Fatal(err)
}

for i, record := range history {
    fmt.Printf("%d. Height %d: %s\n", i+1, record.Height, record.Value)
}
```

### Name Registration (Foundation)

```go
// Create NAME_NEW transaction
result, err := nc.RegisterName(ctx, "d/mysite", `{"ip":"1.2.3.4"}`, nil)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("NAME_NEW created: %s\n", result.TxHash)
// Note: Full registration flow (waiting + NAME_FIRSTUPDATE) requires network integration
```

### Name Updates

```go
// Update an existing name
result, err := nc.UpdateName(ctx, "d/mysite", `{"ip":"5.6.7.8"}`, nil)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("NAME_UPDATE created: %s\n", result.TxHash)
```

## Advanced Configuration

### Network Selection

```go
// Mainnet (production)
cfg := &client.Config{
    Network: "mainnet",
    DataDir: "/var/lib/myapp/namecoin",
}

// Testnet (testing with real-ish conditions)
cfg := &client.Config{
    Network: "testnet",
    DataDir: "/tmp/nmcd-testnet",
}

// Regtest (local development, instant blocks)
cfg := &client.Config{
    Network: "regtest",
    DataDir: "/tmp/nmcd-regtest",
}
```

### Custom Data Directory

```go
// Use application-specific data directory
cfg := &client.Config{
    DataDir: filepath.Join(os.Getenv("HOME"), ".myapp", "namecoin"),
}
```

### Disable Wallet

If you only need read operations (name resolution), disable the wallet for reduced resource usage:

```go
cfg := &client.Config{
    DisableWallet: true, // No wallet.json, no signing capability
}
```

### Custom Bootstrap Peers

```go
cfg := &client.Config{
    BootstrapPeers: []string{
        "peer1.example.com:8334",
        "peer2.example.com:8334",
    },
    MaxPeers: 4,
}
```

## Resource Management

### Proper Cleanup

Always close the client to release resources:

```go
nc, err := client.NewEmbeddedClient(cfg)
if err != nil {
    log.Fatal(err)
}
defer nc.Close() // Always defer Close()

// Use client...
```

### Graceful Shutdown

Handle OS signals for clean shutdown:

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "github.com/opd-ai/nmcd/client"
)

func main() {
    nc, err := client.NewEmbeddedClient(nil)
    if err != nil {
        log.Fatal(err)
    }

    // Handle shutdown signals
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    // Run application
    go runApplication(nc)

    // Wait for shutdown signal
    <-sigCh
    log.Println("Shutting down...")

    // Clean shutdown
    if err := nc.Close(); err != nil {
        log.Printf("Error closing client: %v", err)
    }
}
```

### Context Cancellation

All operations support context cancellation:

```go
// With timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

record, err := nc.ResolveName(ctx, "d/example")
if errors.Is(err, context.Canceled) {
    log.Println("Operation cancelled")
}
```

## Common Patterns

### DNS Resolver

Build a DNS resolver using embedded nmcd:

```go
type NamecoinResolver struct {
    client client.NameClient
}

func (r *NamecoinResolver) Resolve(domain string) ([]net.IP, error) {
    // Convert domain to Namecoin name (e.g., "example.bit" -> "d/example")
    name := "d/" + strings.TrimSuffix(domain, ".bit")

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    record, err := r.client.ResolveName(ctx, name)
    if err != nil {
        return nil, err
    }

    // Parse DNS value
    var dns struct {
        IP  string   `json:"ip"`
        IP6 string   `json:"ip6"`
        NS  []string `json:"ns"`
    }
    if err := json.Unmarshal([]byte(record.Value), &dns); err != nil {
        return nil, err
    }

    var ips []net.IP
    if dns.IP != "" {
        ips = append(ips, net.ParseIP(dns.IP))
    }
    if dns.IP6 != "" {
        ips = append(ips, net.ParseIP(dns.IP6))
    }

    return ips, nil
}
```

### Name Expiration Monitor

Monitor names approaching expiration:

```go
func monitorExpirations(nc client.NameClient, threshold int32) {
    ctx := context.Background()

    records, err := nc.ListNames(ctx, &client.ListFilter{
        Namespace: "d/",
        Limit:     10000,
    })
    if err != nil {
        log.Printf("Error listing names: %v", err)
        return
    }

    for _, record := range records {
        if record.ExpiresIn < threshold {
            log.Printf("WARNING: %s expires in %d blocks (~%.1f days)",
                record.Name,
                record.ExpiresIn,
                float64(record.ExpiresIn)*10/60/24)
        }
    }
}
```

### Singleton Client

For applications needing a single shared client:

```go
var (
    clientOnce sync.Once
    sharedClient client.NameClient
    clientErr error
)

func GetClient() (client.NameClient, error) {
    clientOnce.Do(func() {
        sharedClient, clientErr = client.NewEmbeddedClient(&client.Config{
            DataDir: "/var/lib/myapp/nmcd",
            Network: "mainnet",
        })
    })
    return sharedClient, clientErr
}

// Call at application shutdown
func CloseClient() error {
    if sharedClient != nil {
        return sharedClient.Close()
    }
    return nil
}
```

## Best Practices

### 1. Use Context Timeouts

Always set appropriate timeouts for operations:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
```

### 2. Handle All Errors

Check for specific error types:

```go
switch {
case errors.Is(err, client.ErrNameNotFound):
    // Handle not found
case errors.Is(err, client.ErrNameExpired):
    // Handle expired
case errors.Is(err, context.Canceled):
    // Handle cancellation
case err != nil:
    // Handle unexpected error
}
```

### 3. Close Resources

Always close the client, even on error:

```go
nc, err := client.NewEmbeddedClient(cfg)
if err != nil {
    return err
}
defer nc.Close()
```

### 4. Secure Wallet Files

The wallet stores unencrypted private keys:

```go
// Ensure proper permissions on data directory
os.MkdirAll(dataDir, 0700)

// Check permissions before use
info, _ := os.Stat(filepath.Join(dataDir, "wallet.json"))
if info.Mode().Perm() != 0600 {
    log.Warning("Wallet file has insecure permissions")
}
```

### 5. Use Regtest for Testing

Use regtest network for unit and integration tests:

```go
func TestMyApp(t *testing.T) {
    nc, err := client.NewEmbeddedClient(&client.Config{
        DataDir: t.TempDir(), // Auto-cleanup
        Network: "regtest",
    })
    if err != nil {
        t.Fatal(err)
    }
    defer nc.Close()

    // Run tests...
}
```

## Troubleshooting

### Database Lock Errors

**Problem:** "database is locked" or "another process has the file open"

**Cause:** Only one process can open the bbolt database at a time.

**Solution:**
1. Ensure only one embedded client is running per data directory
2. Close the client before starting another
3. Use different data directories for concurrent instances

### Insufficient Funds

**Problem:** "insufficient funds for operation"

**Cause:** No UTXOs available for the wallet address.

**Solution:**
1. Fund the wallet address with NMC
2. Ensure transactions are confirmed before use
3. Check UTXO availability with debug logging

### Network Errors

**Problem:** "failed to connect to peers"

**Cause:** Network connectivity issues or firewall blocking.

**Solution:**
1. Check network connectivity
2. Ensure port 8334 (mainnet) is accessible
3. Use custom bootstrap peers if DNS seeds are blocked

### Memory Usage

**Problem:** High memory usage

**Cause:** Blockchain caching and UTXO set.

**Solution:**
1. Use daemon mode for resource-constrained environments
2. Configure lower UTXO cache size (advanced)
3. Run periodic garbage collection

---

## See Also

- [API.md](API.md) - Complete API reference
- [examples/](../examples/) - Example applications
- [examples/embedded_client/](../examples/embedded_client/) - Embedded client example
