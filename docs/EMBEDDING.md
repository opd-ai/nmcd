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
