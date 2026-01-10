# Client Modes: Embedded vs Daemon

This guide provides a comprehensive comparison of the two client modes available in nmcd: **Embedded Mode** (in-process) and **Daemon Mode** (RPC). Understanding the differences helps you choose the right mode for your application.

## Table of Contents

- [Overview](#overview)
- [Mode Comparison](#mode-comparison)
- [Embedded Mode](#embedded-mode)
- [Daemon Mode](#daemon-mode)
- [Auto-Detection](#auto-detection)
- [Decision Guide](#decision-guide)
- [Migration Between Modes](#migration-between-modes)
- [Configuration Reference](#configuration-reference)
- [Troubleshooting](#troubleshooting)

## Overview

nmcd provides two distinct client implementations, both implementing the same `NameClient` interface:

```
┌─────────────────────────────────────────────────────────────────┐
│                     NameClient Interface                         │
│  ResolveName | RegisterName | UpdateName | ListNames | GetInfo  │
└─────────────────────────────────────────────────────────────────┘
                    ▲                           ▲
                    │                           │
        ┌───────────┴───────────┐   ┌──────────┴──────────┐
        │   EmbeddedClient      │   │   DaemonClient      │
        │   (In-Process)        │   │   (JSON-RPC)        │
        │                       │   │                     │
        │  ┌─────────────────┐  │   │  ┌─────────────┐    │
        │  │ Local Blockchain│  │   │  │ HTTP Client │    │
        │  │ Local NameDB    │  │   │  │ RPC Calls   │    │
        │  │ Local Wallet    │  │   │  └─────────────┘    │
        │  └─────────────────┘  │   │         │          │
        └───────────────────────┘   └─────────┼──────────┘
                    │                         │
                    ▼                         ▼
            Local Database              External Daemon
              (~/.nmcd)             (nmcd or Namecoin Core)
```

## Mode Comparison

| Aspect | Embedded Mode | Daemon Mode |
|--------|---------------|-------------|
| **External Dependencies** | None | Requires running daemon |
| **Startup Time** | ~1-5 seconds (depends on sync state) | Instant (~50ms) |
| **Memory Usage** | Higher (100-500 MB) | Lower (~10 MB) |
| **CPU Usage** | Higher (blockchain validation) | Minimal |
| **Disk Usage** | Higher (2-5 GB for mainnet) | Minimal (~1 MB for config) |
| **Offline Operation** | Yes (after initial sync) | No |
| **Data Control** | Full ownership | Shared with daemon |
| **Deployment** | Single binary | Daemon + client |
| **Thread Safety** | Fully thread-safe | Fully thread-safe |
| **Network Access** | Direct P2P | Via daemon only |
| **Wallet Support** | Built-in | Via daemon RPC |
| **Best For** | Standalone apps, testing | Microservices, web apps |

## Embedded Mode

Embedded mode runs the full Namecoin node in-process, including blockchain validation, name database, and wallet functionality.

### How It Works

```
┌─────────────────────────────────────────────────────────────┐
│                     Your Application                         │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐ │
│  │                  EmbeddedClient                        │ │
│  │                                                       │ │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────────┐ │ │
│  │  │ Blockchain  │ │   NameDB    │ │      Wallet     │ │ │
│  │  │   (btcd)    │ │   (bbolt)   │ │  (wallet.json)  │ │ │
│  │  │             │ │             │ │                 │ │ │
│  │  │ • Block     │ │ • Names     │ │ • Private Keys  │ │ │
│  │  │   Validation│ │ • History   │ │ • Signing       │ │ │
│  │  │ • Chain     │ │ • Expiration│ │ • Addresses     │ │ │
│  │  │   Management│ │ • UTXOs     │ │                 │ │ │
│  │  └─────────────┘ └─────────────┘ └─────────────────┘ │ │
│  └───────────────────────────────────────────────────────┘ │
│                              │                              │
│                              ▼                              │
│                      Data Directory                         │
│                      ~/.nmcd/                               │
│                      ├── blocks/                            │
│                      ├── names.db                           │
│                      └── wallet.json                        │
└─────────────────────────────────────────────────────────────┘
```

### Initialization

```go
// Import the nmcd client package
import "github.com/opd-ai/nmcd/client"

// Create embedded client with custom configuration
nc, err := client.NewEmbeddedClient(&client.Config{
    DataDir: "/path/to/data",
    Network: "mainnet",
})
if err != nil {
    log.Fatal(err)
}
defer nc.Close()
```

### Advantages

1. **No External Dependencies**: Single binary deployment with no daemon required
2. **Full Data Control**: Complete ownership of blockchain and wallet data
3. **Offline Operation**: Works without network after initial synchronization
4. **Deterministic Testing**: Ideal for unit tests with regtest network
5. **Security**: Private keys never leave the process

### Disadvantages

1. **Higher Resource Usage**: Requires more memory, CPU, and disk space
2. **Sync Time**: Initial blockchain sync can take hours (mainnet)
3. **Database Locking**: Only one embedded client per data directory
4. **Complexity**: More moving parts to manage

### Use Cases

- **Standalone Applications**: Desktop apps, CLI tools
- **IoT Devices**: Devices with sufficient storage and processing
- **Testing & Development**: Unit tests, integration tests, local development
- **Privacy-Critical**: Applications requiring local data control
- **Offline-Capable**: Applications that need to work without network

## Daemon Mode

Daemon mode connects to an external nmcd or Namecoin Core daemon via JSON-RPC, delegating all blockchain operations to the daemon.

### How It Works

```
┌──────────────────────────────────────┐
│           Your Application            │
│                                      │
│  ┌────────────────────────────────┐  │
│  │        DaemonClient            │  │
│  │                                │  │
│  │  ┌──────────────────────────┐  │  │
│  │  │       HTTP Client        │  │  │
│  │  │                          │  │  │
│  │  │  • JSON-RPC requests     │  │  │
│  │  │  • Retry logic           │  │  │
│  │  │  • Authentication        │  │  │
│  │  └──────────────────────────┘  │  │
│  └────────────────────────────────┘  │
│                   │                   │
└───────────────────┼───────────────────┘
                    │ HTTP/JSON-RPC
                    ▼
┌──────────────────────────────────────┐
│      External Daemon                  │
│   (nmcd or Namecoin Core)            │
│                                      │
│  ┌────────┐ ┌────────┐ ┌──────────┐  │
│  │Blockchain│ │NameDB │ │  Wallet │  │
│  └────────┘ └────────┘ └──────────┘  │
│                                      │
│  localhost:8336 (mainnet)            │
│  localhost:18336 (testnet)           │
└──────────────────────────────────────┘
```

```go
// Before: Embedded mode
nc, err := client.NewClient(&client.Config{
    Mode:    client.ModeEmbedded,
    DataDir: "/path/to/data",
})

// After: Daemon mode
nc, err := client.NewClient(&client.Config{
    Mode:        client.ModeDaemon,
    RPCAddr:     "http://localhost:8336",
    RPCUser:     "user",
    RPCPassword: "pass",
})

// Application code remains unchanged
record, err := nc.ResolveName(ctx, "d/example")
```

### Environment-Based Configuration

Use environment variables for flexible deployment:

```go
func getClientConfig() *client.Config {
    cfg := &client.Config{
        Network: getEnv("NMCD_NETWORK", "mainnet"),
    }

    if rpcAddr := os.Getenv("NMCD_RPC_ADDR"); rpcAddr != "" {
        cfg.Mode = client.ModeDaemon
        cfg.RPCAddr = rpcAddr
        cfg.RPCUser = os.Getenv("NMCD_RPC_USER")
        cfg.RPCPassword = os.Getenv("NMCD_RPC_PASSWORD")
    } else if dataDir := os.Getenv("NMCD_DATA_DIR"); dataDir != "" {
        cfg.Mode = client.ModeEmbedded
        cfg.DataDir = dataDir
    } else {
        cfg.Mode = client.ModeAuto
    }

    return cfg
}

func getEnv(key, fallback string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return fallback
}
```

## Configuration Reference

### Embedded Mode Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Mode` | `ClientMode` | `ModeAuto` | Set to `ModeEmbedded` for explicit embedded mode |
| `DataDir` | `string` | `~/.nmcd` | Directory for blockchain and wallet data |
| `Network` | `string` | `mainnet` | Network: `mainnet`, `testnet`, or `regtest` |
| `MaxPeers` | `int` | `8` | Maximum P2P peer connections |
| `BootstrapPeers` | `[]string` | DNS seeds | Custom bootstrap peers |
| `DisableWallet` | `bool` | `false` | Disable wallet for read-only mode |

### Daemon Mode Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Mode` | `ClientMode` | `ModeAuto` | Set to `ModeDaemon` for explicit daemon mode |
| `RPCAddr` | `string` | `http://localhost:8336` | Daemon RPC endpoint |
| `RPCUser` | `string` | `""` | RPC authentication username |
| `RPCPassword` | `string` | `""` | RPC authentication password |
| `Network` | `string` | `mainnet` | Network for port defaults |

### Default Ports by Network

| Network | P2P Port | RPC Port |
|---------|----------|----------|
| mainnet | 8334 | 8336 |
| testnet | 18334 | 18336 |
| regtest | 18445 | 18443¹ |

¹ Regtest RPC port follows Namecoin Core convention. Check your daemon configuration.

## Troubleshooting

### Embedded Mode Issues

#### "database is locked"

**Problem:** Only one embedded client can open a database directory at a time.

**Solutions:**
1. Ensure only one instance is running
2. Use different data directories for multiple instances
3. Use daemon mode for shared access

```go
