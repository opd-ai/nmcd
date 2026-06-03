# nmcd

Pure Go Namecoin library and daemon using btcd as dependencies (not forks).

## Overview

nmcd is a **library-first** Namecoin implementation built using btcd libraries. It can be embedded directly into your Go applications for in-process name resolution and registration, or run as a standalone daemon for traditional RPC access.

**Key Highlights:**
- 🔧 **Library-First Design**: Import and use directly in your Go code
- 🚀 **Embedded or Daemon Mode**: Choose in-process or external daemon
- 🧩 **Composition over Reimplementation**: Built on btcd's battle-tested components
- 🔒 **Thread-Safe**: All operations safe for concurrent use
- 📦 **Pure Go**: No C dependencies, cross-platform support

## Quick Start (Library Usage)

### Installation

```bash
go get github.com/opd-ai/nmcd
```

### Basic Example: Resolve a Name

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/opd-ai/nmcd/client"
)

func main() {
    // Create embedded client (runs in-process)
    nc, err := client.NewClient(&client.Config{
        Mode:    client.ModeAuto,  // Auto-detect daemon or use embedded
        Network: "mainnet",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer nc.Close()
    
    // Resolve a Namecoin name
    ctx := context.Background()
    record, err := nc.ResolveName(ctx, "d/example")
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Name: %s\nValue: %s\nOwner: %s\n",
        record.Name, record.Value, record.Address)
}
```

### Register or Update Names

```go
// Register a new name
result, err := nc.RegisterName(ctx, "d/mysite", `{"ip":"1.2.3.4"}`, nil)
fmt.Printf("Registration TX: %s\n", result.TxHash)

// Update existing name
result, err = nc.UpdateName(ctx, "d/mysite", `{"ip":"5.6.7.8"}`, nil)
fmt.Printf("Update TX: %s\n", result.TxHash)

// List names with filters
names, err := nc.ListNames(ctx, &client.ListFilter{
    Namespace: "d/",
    Limit:     100,
})
```

**📚 See [docs/EXAMPLES.md](docs/EXAMPLES.md) for detailed walkthroughs and patterns.**

## Library Features

- **Name Resolution**: Look up Namecoin names with expiration checking
- **Name Registration**: Two-step NAME_NEW → NAME_FIRSTUPDATE process
- **Name Updates**: Update values and extend expiration (36,000 blocks)
- **Name Listing**: Filter by namespace, address, or pattern with pagination
- **Embedded Mode**: In-process blockchain and database (no external daemon)
- **Daemon Mode**: Connect to existing nmcd or Namecoin Core via RPC
- **Auto-Detection**: Automatically choose daemon or embedded based on availability
- **Thread-Safe**: All methods safe for concurrent goroutines
- **Context Support**: Timeouts and cancellation for all operations

## Daemon Features

When run as a standalone daemon, nmcd provides:

- **Pure Go**: Built entirely in Go using standard library and btcd
- **NameDatabase**: bbolt-backed storage for name operations
- **Blockchain Integration**: Embeds btcd's blockchain.BlockChain with name validation hooks
- **Block Synchronization**: Automatic Initial Block Download (IBD) and ongoing sync with the network via headers-first protocol
- **Network Layer**: Uses btcd/peer for P2P networking with interface-based connections (net.Conn)
- **Transaction Mempool**: Validates and relays unconfirmed transactions with automatic expiration
- **JSON-RPC Server**: Standard library net/http for RPC interface
- **Thread-Safe**: Mutex protection for all shared state
- **Focused Implementation**: ~10,500 logical lines (non-comment, non-blank lines in non-test Go files); ~21,400 raw lines (all lines in non-test Go files)

### Block Synchronization

The daemon automatically synchronizes with the Namecoin network using a headers-first sync protocol:

1. **Initial Block Download (IBD)**: Downloads block headers from peers, validates the chain, then fetches full blocks
2. **Ongoing Sync**: Continuously monitors peers for new blocks and processes them as they arrive
3. **Peer Selection**: Tracks peer reliability and latency to choose the best sync sources
4. **Health Monitoring**: Use the `/ready` endpoint to check if sync is complete

The embedded client mode automatically connects to the network and syncs blocks when MaxPeers > 0 (default: 8). By default, it uses DNS seed discovery to find peers. To disable automatic network sync, set MaxPeers to 0 or provide custom BootstrapPeers.

## Documentation

- **[Installation Guide](docs/INSTALLATION.md)**: Platform-specific installation instructions
- **[Operations Guide](docs/OPERATIONS.md)**: Configuration, monitoring, backup, and troubleshooting
- **[Quick Start Guide](#quick-start-library-usage)**: Library usage and setup
- **[API Reference](docs/API.md)**: Complete API documentation with examples
- **[CHANGELOG](CHANGELOG.md)**: Version history and API stability policy
- **[Embedding Guide](docs/EMBEDDING.md)**: Integration patterns for applications
- **[Examples Guide](docs/EXAMPLES.md)**: Detailed walkthroughs of example code
- **[Mode Comparison](docs/MODES.md)**: Embedded vs daemon tradeoffs
- **[Performance Guide](docs/PERFORMANCE.md)**: Benchmarks and optimization tips

## API Stability

Starting with v1.0.0, nmcd follows [Semantic Versioning](https://semver.org/) and provides strong backward compatibility guarantees:

- **Stable API**: The `client` package interface is stable and will maintain backward compatibility
- **Breaking changes**: Only in MAJOR version releases (e.g., v1.x → v2.0)
- **New features**: Added in MINOR releases (e.g., v1.0 → v1.1) without breaking existing code
- **Bug fixes**: In PATCH releases (e.g., v1.0.0 → v1.0.1)

See [CHANGELOG.md](CHANGELOG.md) for:
- Complete version history
- Detailed semantic versioning policy
- Deprecation process
- Migration guides between versions

**Current Status**: v0.1.0 (development) → Working towards v1.0.0 production release


## Mode Selection

nmcd supports three operational modes:

### Auto Mode (Recommended)

Automatically detects if a daemon is running on localhost:8336 and uses it; otherwise runs in embedded mode.

```go
nc, err := client.NewClient(&client.Config{
    Mode: client.ModeAuto,  // Default
})
```

**Use When:**
- Building applications that work with or without a daemon
- Want flexibility without configuration
- Development and testing

### Embedded Mode

Runs the full blockchain, database, and network stack in-process. No external daemon required.

```go
nc, err := client.NewEmbeddedClient(&client.Config{
    Mode:    client.ModeEmbedded,
    DataDir: "/path/to/data",
    Network: "mainnet",
})
```

**Network Connectivity:**

By default, embedded mode automatically connects to the Namecoin network using DNS seed discovery (MaxPeers defaults to 8). To customize network behavior:

```go
// Disable automatic network sync (offline mode)
nc, err := client.NewEmbeddedClient(&client.Config{
    Mode:     client.ModeEmbedded,
    MaxPeers: 0,  // No peer connections
})

// Use custom bootstrap peers (skip DNS seeds)
nc, err := client.NewEmbeddedClient(&client.Config{
    Mode: client.ModeEmbedded,
    BootstrapPeers: []string{
        "peer1.example.com:8334",
        "peer2.example.com:8334",
    },
})
```

**Use When:**
- Embedding nmcd in your application
- Running multiple isolated instances
- Offline name resolution from local database
- No dependency on external services

**Pros:**
- ✅ No external dependencies
- ✅ Full control over resources
- ✅ Isolated data and state
- ✅ Simpler deployment
- ✅ Automatic network sync by default

**Cons:**
- ❌ Higher memory usage (~250MB UTXO cache)
- ❌ Each instance syncs independently
- ❌ Database locked to single process

### Daemon Mode

Connects to an existing nmcd or Namecoin Core daemon via RPC.

```go
nc, err := client.NewDaemonClient(&client.Config{
    Mode:        client.ModeDaemon,
    RPCAddr:     "http://localhost:8336",
    RPCUser:     "user",
    RPCPassword: "pass",
})
```

**Use When:**
- Shared blockchain state across applications
- Multiple applications need name resolution
- Centralized infrastructure
- Production deployments with managed daemon

**Pros:**
- ✅ Shared blockchain sync
- ✅ Lower per-application memory
- ✅ Centralized monitoring
- ✅ Multiple clients, one sync

**Cons:**
- ❌ Requires running daemon
- ❌ Network dependency
- ❌ RPC authentication needed

**📚 See [docs/MODES.md](docs/MODES.md) for detailed comparison and recommendations.**

## Architecture

### Library Architecture

```
┌───────────────────────────────────────────────────────────────┐
│                    Your Go Application                         │
├───────────────────────────────────────────────────────────────┤
│                    nmcd Library (client/)                      │
│  ┌────────────────────────────────────────────────────────┐  │
│  │         NameClient Interface (Public API)              │  │
│  │  • ResolveName(name) → NameRecord                      │  │
│  │  • RegisterName(name, value, opts) → TxHash            │  │
│  │  • UpdateName(name, value, opts) → TxHash              │  │
│  │  • ListNames(filter) → []NameRecord                    │  │
│  └────────────────────────────────────────────────────────┘  │
│            ▲                              ▲                    │
│            │                              │                    │
│  ┌─────────┴─────────┐        ┌──────────┴──────────┐        │
│  │  EmbeddedClient   │        │   DaemonClient      │        │
│  │  (in-process)     │        │   (RPC to daemon)   │        │
│  └─────────┬─────────┘        └─────────────────────┘        │
│            │                                                   │
│  ┌─────────▼─────────────────────────┐                       │
│  │  Embedded Components               │                       │
│  │  ┌────────┐ ┌────────┐ ┌────────┐│                       │
│  │  │ chain/ │ │namedb/ │ │network/││                       │
│  │  │validate│ │bbolt DB│ │ peer   ││                       │
│  │  └────────┘ └────────┘ └────────┘│                       │
│  └───────────────────────────────────┘                       │
└───────────────────────────────────────────────────────────────┘
```

### Daemon Component Architecture

1. **namedb**: Name database with bbolt storage
   - Stores name records with expiration tracking
   - Historical operation tracking
   - Thread-safe with RWMutex

2. **chain**: Blockchain wrapper
   - Embeds btcd's blockchain.BlockChain
   - Extends validation with name operation hooks
   - Manages name expiration (36000 blocks ~250 days)

3. **network**: P2P networking
   - Uses btcd/peer for peer management
   - Interface-based connections (net.Conn)
   - Handles block/tx propagation

4. **rpc**: JSON-RPC server
   - Standard library net/http
   - Name-specific RPC methods
   - Thread-safe access

5. **config**: Configuration management
   - Network selection (mainnet/testnet/regtest)
   - Data directory management

---

## Using as a Daemon

While nmcd is primarily a library, it can also run as a standalone daemon for traditional RPC access.

### Building the Daemon

```bash
go build -v ./cmd/nmcd
```

### Running the Daemon

```bash
# Run with defaults (auto-discovers peers via DNS seeds)
./nmcd

# Custom data directory
./nmcd -datadir=/path/to/data

# Testnet
./nmcd -network=testnet

# Custom RPC port
./nmcd -rpcaddr=127.0.0.1:18336

# Connect to specific peer (bypasses DNS seed discovery)
./nmcd -addpeer=peer.example.com:8334

# Enable RPC authentication (recommended for security)
./nmcd -rpcuser=myuser -rpcpassword=mypassword
```

**Note:** When using nmcd as a library, you don't need to run the daemon unless you want to share blockchain state across multiple applications.

### Peer Discovery (Daemon Mode)

nmcd supports automatic peer discovery via DNS seeds. When started without the `-addpeer` flag, the node will:

1. Query official Namecoin DNS seed servers
2. Resolve IP addresses of active network nodes
3. Connect to discovered peers automatically

**Mainnet DNS Seeds:**
- `nmc.seed.quisquis.de`
- `seed.nmc.markasoftware.com`
- `dnsseed1.nmc.dotbit.zone`
- `dnsseed2.nmc.dotbit.zone`
- `dnsseed.nmc.testls.space`
- `namecoin.seed.cypherstack.com`

**Testnet DNS Seeds:**
- `dnsseed.test.namecoin.webbtc.com`

To bypass DNS seed discovery and connect to specific peers, use the `-addpeer` flag.

### RPC API (Daemon Mode)

When running nmcd as a daemon, it exposes a JSON-RPC interface for external access. **If you're using nmcd as a library**, use the Go API instead of RPC for better performance and type safety.

The RPC server supports HTTP Basic Authentication. When both `-rpcuser` and `-rpcpassword` flags are set, all RPC requests must include valid credentials.

**Security Considerations:**
- Use strong, unique passwords for RPC authentication
- Command-line flags are visible in process listings (`ps`, `top`). For production use, consider environment variables or a configuration file
- HTTP Basic Auth transmits credentials in base64 encoding (not encrypted). Only use RPC over localhost or with proper network security (firewall, VPN, or reverse proxy with HTTPS)

```bash
# With authentication enabled
curl -X POST http://127.0.0.1:8336 \
  -u myuser:mypassword \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"getinfo","params":[],"id":1}'
```

### Standard Methods

- `getinfo` - Get general information
- `getblockcount` - Get current block height
- `getbestblockhash` - Get best block hash
- `getconnectioncount` - Get peer connection count
- `getpeerinfo` - Get connected peer information

### Name Methods

- `name_new` - Create a NAME_NEW transaction to pre-register a name commitment
  ```bash
  curl -X POST http://127.0.0.1:8336 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"name_new","params":["d/example"],"id":1}'
  ```
  
  This is the first step in the two-phase name registration process. It creates a commitment hash to prevent front-running. The response includes a `rand` value (hex-encoded random bytes) that **must be saved** for the NAME_FIRSTUPDATE step.
  
  Returns:
  ```json
  {
    "txid": "transaction_hash",
    "name": "d/example",
    "rand": "hex_encoded_random_bytes",
    "status": "broadcasted"
  }
  ```
  
  **Important:** Save the `rand` value! You'll need it for `name_firstupdate`.

- `name_firstupdate` - Complete name registration with NAME_FIRSTUPDATE
  ```bash
  curl -X POST http://127.0.0.1:8336 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"name_firstupdate","params":["d/example","hex_rand_from_name_new","{\"ip\":\"1.2.3.4\"}"],"id":1}'
  ```
  
  Parameters: `["name", "rand", "value"]`
  
  This is the second step in the two-phase registration process. Requirements:
  - Must be called at least 12 blocks after `name_new`
  - Must be called within 36,000 blocks of `name_new`
  - The `rand` parameter must match the value returned by `name_new`
  - The `value` must be valid UTF-8 (max 1023 bytes), JSON for `d/` and `id/` namespaces
  
  Returns:
  ```json
  {
    "txid": "transaction_hash",
    "name": "d/example",
    "value": "{\"ip\":\"1.2.3.4\"}",
    "status": "broadcasted"
  }
  ```

- `name_show` - Show name information
  ```bash
  curl -X POST http://127.0.0.1:8336 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"name_show","params":["d/example"],"id":1}'
  ```

- `name_list` - List all registered names
  ```bash
  curl -X POST http://127.0.0.1:8336 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"name_list","params":[],"id":1}'
  ```

- `name_history` - Get name history
  ```bash
  curl -X POST http://127.0.0.1:8336 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"name_history","params":["d/example"],"id":1}'
  ```

- `name_update` - Update an existing name's value
  ```bash
  curl -X POST http://127.0.0.1:8336 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"name_update","params":["d/example","new_value"],"id":1}'
  ```
  
  Parameters: `["name", "value"]` or `["name", "value", "address"]`
  
  The wallet must have the private key for the address that owns the name. If no address is specified, the name stays at its current address.

### Wallet Methods

- `getnewaddress` - Generate a new address in the wallet
  ```bash
  curl -X POST http://127.0.0.1:8336 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"getnewaddress","params":[],"id":1}'
  ```
  
  Returns a new address string. The address is persisted to the wallet file.

- `listaddresses` - List all addresses in the wallet
  ```bash
  curl -X POST http://127.0.0.1:8336 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"listaddresses","params":[],"id":1}'
  ```
  
  Returns an array of address strings currently stored in the wallet.

#### Wallet Encryption

**Security Warning:** By default, nmcd wallets store private keys **unencrypted** in `wallet.json` with file permissions `0600`. For production use, always encrypt your wallet.

- `encryptwallet` - Encrypt the wallet with a password (one-time operation)
  ```bash
  curl -X POST http://127.0.0.1:8336 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"encryptwallet","params":["YourSecurePassword123"],"id":1}'
  ```
  
  **Parameters:** `["password"]`
  - Password must be at least 8 characters with 2+ character types (lowercase, uppercase, digits, special)
  - Encrypts all private keys using AES-256-GCM with scrypt key derivation (N=32768)
  - Wallet remains unlocked immediately after encryption
  - **This cannot be undone** - backup your wallet before encrypting
  
  **Returns:** Success message with backup reminder

- `walletpassphrase` - Unlock the encrypted wallet temporarily
  ```bash
  curl -X POST http://127.0.0.1:8336 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"walletpassphrase","params":["YourSecurePassword123",300],"id":1}'
  ```
  
  **Parameters:** `["password", timeout]` or `["password"]`
  - `password` - Your wallet password
  - `timeout` - Seconds to keep wallet unlocked (default: 60)
  
  Unlocks the wallet for the specified duration. The wallet will automatically lock after the timeout expires. While unlocked, you can:
  - Generate new addresses
  - Send transactions
  - Register/update names
  
  **Security:** Keys are loaded into memory while unlocked and cleared on lock.

- `walletlock` - Lock the wallet immediately
  ```bash
  curl -X POST http://127.0.0.1:8336 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"walletlock","params":[],"id":1}'
  ```
  
  Locks an encrypted wallet, removing all private keys from memory. The wallet must be unlocked again with `walletpassphrase` before performing operations that require private keys.
  
  **Security Best Practices:**
  - Use `walletlock` immediately after completing sensitive operations
  - Set short timeouts (1-5 minutes) when using `walletpassphrase`
  - Never store passwords in scripts or command history
  - Use strong passwords (12+ characters recommended)
  - Backup encrypted wallet file regularly

### Additional Methods

Beyond the standard, name, and wallet methods above, nmcd also provides:

**Blockchain Methods:**
- `getblock` - Get block information by hash (verbose and raw modes; use `getblockhash` for height lookup)
- `getblockhash` - Get block hash at a specified height
- `getrawtransaction` - Get transaction data in raw hex or verbose JSON (`verbose=true`)
- `sendrawtransaction` - Broadcast a raw transaction to the network

**Name Query Methods:**
- `name_scan` - Scan names by prefix (`start`) with pagination (`count`, default 500)
- `name_pending` - Get transactions pending in the mempool related to names

**Utility Methods:**
- `getbalance` - Get total balance across all wallet addresses
- `listunspent` - List unspent transaction outputs (UTXOs) available for spending
- `getmetrics` - Get internal metrics snapshot as JSON (Prometheus format is available at `/metrics`)

**📚 See [docs/API.md](docs/API.md) for complete method documentation, parameters, and examples.**

### Health and Readiness Endpoints

nmcd provides HTTP health check endpoints suitable for Kubernetes liveness and readiness probes, load balancers, and monitoring systems.

**Health Endpoint (`/health`):**

```bash
curl http://127.0.0.1:8336/health
```

Returns HTTP 200 OK when the daemon is running and initialized, or 503 Service Unavailable when initializing.

Response:
```json
{
  "status": "healthy",
  "block_height": 500000,
  "peers": 8,
  "syncing": true
}
```

The `syncing` field is optional (omitempty) and appears when the node is initializing or syncing blocks.

Use this endpoint for **liveness probes** to detect if the process is alive and responsive.

**Readiness Endpoint (`/ready`):**

```bash
curl http://127.0.0.1:8336/ready
```

Returns HTTP 200 OK when the daemon is ready to serve requests (sync complete), or 503 Service Unavailable when syncing or initializing.

Response:
```json
{
  "status": "ready",
  "block_height": 500000,
  "peers": 8,
  "syncing": false
}
```

Use this endpoint for **readiness probes** to detect if the node is ready to handle traffic.

**Kubernetes Example:**

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8336
  initialDelaySeconds: 30
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /ready
    port: 8336
  initialDelaySeconds: 5
  periodSeconds: 5
```

**Security Note:** These endpoints do **not** require authentication and are intended for health checking systems. Bind the RPC server to localhost (default) or use network-level controls to restrict access.

### Prometheus Metrics Endpoint

nmcd can optionally expose metrics in Prometheus format for monitoring and observability. The metrics are served on a separate HTTP endpoint from the RPC server.

**Enable Prometheus metrics:**

```bash
# Enable on default port 9100
./nmcd -prometheusaddr=127.0.0.1:9100

# Or use custom port
./nmcd -prometheusaddr=127.0.0.1:9999
```

Once enabled, metrics are available at `http://<address>/metrics` in Prometheus text format.

**Security Considerations:**

The Prometheus metrics endpoint does **not** implement authentication and exposes operational data about the node (e.g., block processing statistics and name operation counts). For production deployments, you should:

- Prefer binding `-prometheusaddr` to `127.0.0.1` (localhost only), or
- Place the endpoint behind a reverse proxy that provides authentication and TLS.

If you bind the metrics endpoint to a non-localhost network interface, use network-level controls (firewall rules, VPN, or restricted subnets) to allow access only from trusted monitoring systems.

**Available Metrics:**

Block Processing:
- `nmcd_blocks_processed_total` - Total blocks processed
- `nmcd_blocks_accepted_total` - Blocks accepted on main chain
- `nmcd_blocks_orphaned_total` - Orphaned blocks received
- `nmcd_blocks_rejected_total` - Blocks rejected due to validation errors
- `nmcd_last_block_height` - Height of last processed block
- `nmcd_avg_block_process_time_seconds` - Average block processing time

Name Operations:
- `nmcd_name_operations_total` - Total name operations processed
- `nmcd_name_new_total` - NAME_NEW operations
- `nmcd_name_firstupdate_total` - NAME_FIRSTUPDATE operations
- `nmcd_name_update_total` - NAME_UPDATE operations
- `nmcd_names_expired_total` - Names expired during processing

Network:
- `nmcd_peers_connected` - Currently connected peers
- `nmcd_inbound_peers` - Current inbound peers
- `nmcd_outbound_peers` - Current outbound peers
- `nmcd_peer_disconnects_total` - Total peer disconnections

Transactions:
- `nmcd_txs_processed_total` - Total transactions processed
- `nmcd_txs_in_mempool` - Current transactions in mempool

Validation Errors:
- `nmcd_validation_errors_total` - Total validation errors
- `nmcd_auxpow_errors_total` - AuxPoW validation errors
- `nmcd_subsidy_errors_total` - Block subsidy validation errors
- `nmcd_name_theft_attempts_total` - Name theft attempts detected
- `nmcd_double_spend_attempts_total` - Double-spend attempts detected

Database Performance (New):
- `nmcd_namedb_size_bytes` - Size of name database in bytes
- `nmcd_namedb_read_latency_seconds` - Average database read latency
- `nmcd_namedb_write_latency_seconds` - Average database write latency

RPC Performance (New):
- `nmcd_rpc_requests_total{method="METHOD"}` - Total RPC requests by method
- `nmcd_rpc_duration_seconds{method="METHOD"}` - Average RPC duration by method

Error Breakdown (New):
- `nmcd_errors_total{type="validation"}` - Validation errors
- `nmcd_errors_total{type="network"}` - Network errors
- `nmcd_errors_total{type="database"}` - Database errors

Go Runtime (New):
- `nmcd_go_goroutines` - Number of goroutines
- `nmcd_go_memstats_alloc_bytes` - Bytes allocated and in use
- `nmcd_go_memstats_heap_alloc_bytes` - Heap bytes allocated and in use
- `nmcd_go_memstats_heap_idle_bytes` - Heap bytes waiting to be used
- `nmcd_go_memstats_heap_inuse_bytes` - Heap bytes that are in use

**Example Prometheus Query:**

```bash
curl http://127.0.0.1:9100/metrics
```

**Prometheus Configuration:**

```yaml
scrape_configs:
  - job_name: 'nmcd'
    static_configs:
      - targets: ['localhost:9100']
```

**Grafana Dashboard:**

The metrics can be visualized in Grafana. Key panels to consider:
- Block processing rate (rate of `nmcd_blocks_processed_total`)
- Peer count trends (`nmcd_peers_connected`)
- Name operation activity (rate of `nmcd_name_*_total` metrics)
- Validation error rates (rate of `nmcd_*_errors_total` metrics)
- Mempool size over time (`nmcd_txs_in_mempool`)
- Database performance (`nmcd_namedb_read_latency_seconds`, `nmcd_namedb_write_latency_seconds`)
- RPC performance by method (`rate(nmcd_rpc_requests_total[5m])`, `nmcd_rpc_duration_seconds`)
- Error breakdown by category (`nmcd_errors_total` grouped by type label)
- Memory usage (`nmcd_go_memstats_alloc_bytes`, `nmcd_go_memstats_heap_inuse_bytes`)
- Goroutine count (`nmcd_go_goroutines`)

**Example Queries:**
```promql
# Top 5 slowest RPC methods (with method label aggregation)
topk(5, avg by (method) (nmcd_rpc_duration_seconds))

# Error rate by category (per second, grouped by type)
sum by (type) (rate(nmcd_errors_total[5m]))

# Database read/write latency comparison
nmcd_namedb_read_latency_seconds
nmcd_namedb_write_latency_seconds

# RPC request rate by method
sum by (method) (rate(nmcd_rpc_requests_total[5m]))
```

---

## Examples

The `examples/` directory contains several working examples demonstrating library usage:

- **[simple_resolve](examples/simple_resolve/)**: Basic name resolution with auto-detection
- **[embedded_client](examples/embedded_client/)**: Explicit embedded mode usage
- **[register_name](examples/register_name/)**: Two-step name registration (NAME_NEW → NAME_FIRSTUPDATE)
- **[update_name](examples/update_name/)**: Update existing name values
- **[list_names](examples/list_names/)**: Filter and paginate name listings
- **[namedb](examples/namedb/)**: Direct database operations

**Run an example:**

```bash
go run ./examples/simple_resolve d/example
go run ./examples/list_names --namespace=d/
```

**📚 For detailed walkthroughs, see [docs/EXAMPLES.md](docs/EXAMPLES.md)**

---

## Wallet

nmcd includes basic wallet functionality for managing name operations. The wallet stores private keys in `wallet.json` within the data directory.

**Security Note:** The wallet file contains unencrypted private keys. Ensure proper file permissions (0600) and secure the data directory.

**Library Usage:** When using nmcd as a library, the wallet is automatically initialized in the data directory. Use `DisableWallet: true` in the config to disable wallet functionality if you only need name resolution.

---

## Use Cases

### Library Mode (Embedded)

**Best For:**
- 🔍 DNS resolvers and proxies
- 🌐 Web applications with Namecoin integration
- 🤖 Bots and monitoring tools
- 📱 Mobile/desktop applications (future)
- 🔬 Research and experimentation

**Example Applications:**
- DNS bridge resolving Namecoin domains to IP addresses
- Identity verification service using id/ namespace
- Domain monitoring service with expiration alerts
- Decentralized website hosting with d/ namespace

### Daemon Mode

**Best For:**
- 🖥️ Shared infrastructure serving multiple applications
- 🔧 Development and testing environments
- 📊 Blockchain explorers and analytics
- 🔌 Legacy applications using RPC interface

**Example Setups:**
- Central Namecoin node serving multiple microservices
- Development server for application testing
- Network monitoring and statistics collection

---

## Dependencies

- **github.com/btcsuite/btcd/blockchain** - Blockchain management
- **github.com/btcsuite/btcd/peer** - P2P peer management
- **github.com/btcsuite/btcd/wire** - Wire protocol
- **go.etcd.io/bbolt** - Embedded database

## Design Principles

1. **Composition over Reimplementation**: Use btcd libraries directly
2. **Standard Library**: net/http for RPC, no web frameworks
3. **Interface Types**: net.Conn not concrete *net.TCPConn
4. **Thread Safety**: Mutex protection for all shared state
5. **Minimal Code**: Focus on name-specific functionality only

## Name Operations

The implementation supports three name operations:

1. **NAME_NEW**: Pre-register a name (prevents front-running)
2. **NAME_FIRSTUPDATE**: First registration of a name
3. **NAME_UPDATE**: Update existing name value

Names expire after 36000 blocks (~250 days) and must be renewed.

### Name Deletion

Namecoin does not have a separate NAME_DELETE operation. To delete a name, use **NAME_UPDATE with an empty value**:

```go
// Delete a name by setting empty value
result, err := nc.UpdateName(ctx, "d/mysite", "", nil)
```

This effectively removes the name's data while maintaining ownership on the blockchain. The name will still expire after 36,000 blocks unless renewed.

## Project Structure

```
nmcd/
├── client/          # 🔧 Library public API (import this!)
│   ├── types.go           # NameClient interface and types
│   ├── embedded.go        # In-process embedded client
│   ├── daemon.go          # RPC daemon client
│   └── *_test.go          # Comprehensive test suite
│
├── cmd/nmcd/        # Daemon binary entry point
│   └── main.go            # CLI flags and daemon setup
│
├── cmd/permamail/   # 📧 Permamail CLI tool
│   └── main.go            # Email forwarding management
│
├── internal/        # Internal packages (not importable)
│   └── server/            # Daemon server implementation
│
├── bridge/          # Email forwarding bridge adapter
├── mail/            # SMTP routing and relay
├── namedb/          # Name database (bbolt)
├── chain/           # Blockchain wrapper (btcd integration)
├── network/         # P2P networking (btcd/peer)
├── rpc/             # JSON-RPC server (daemon mode)
├── wallet/          # Basic wallet functionality
├── config/          # Network configuration
│
├── docs/            # 📚 Documentation
│   ├── API.md             # Complete API reference
│   ├── EMBEDDING.md       # Integration guide
│   ├── EXAMPLES.md        # Example walkthroughs
│   ├── MODES.md           # Mode comparison
│   └── PERFORMANCE.md     # Optimization tips
│
├── examples/        # 🎯 Working code examples
│   ├── simple_resolve/    # Basic name resolution
│   ├── embedded_client/   # Embedded mode demo
│   ├── register_name/     # Name registration
│   ├── update_name/       # Name updates
│   ├── list_names/        # Name filtering
│   ├── bridge_adapter/    # Email bridge usage
│   ├── mail_router/       # Email routing example
│   ├── smtp_relay/        # SMTP relay deployment
│   └── namedb/            # Direct database access
│
└── README.md        # This file
```

**Import Paths:**

```go
// Primary library interface
import "github.com/opd-ai/nmcd/client"

// For advanced usage (typically not needed)
import "github.com/opd-ai/nmcd/namedb"
import "github.com/opd-ai/nmcd/config"
```

---

## Permamail: Decentralized Email Forwarding

nmcd includes **permamail**, a command-line tool for managing decentralized email forwarding via Namecoin. It allows you to register .bit email addresses that forward to your real email address.

### Installation

```bash
# Build both nmcd and permamail
make build

# Or build permamail only
go build -o permamail ./cmd/permamail
```

### Usage

```bash
# Register a new .bit email address (prepares config; completion requires nmcd RPC)
permamail register alice --forward user@gmail.com

# Update forwarding configuration (prepares config; completion requires nmcd RPC)
permamail update alice --forward newemail@proton.me --backup backup@proton.me

# Look up current configuration
permamail lookup alice

# Start SMTP relay server
# Note: Avoid passing SMTP passwords directly on the command line, as they
# may be visible to other local users via process listings (ps, /proc).
# Consider using environment variables or a config file with restricted permissions.
permamail serve --upstream smtp.sendgrid.net --upstreamport 587 \
                --smtpuser apikey --smtppass <api-key>
```

### Architecture

Permamail consists of three main components:

1. **Bridge Adapter** (`bridge/`): Translates Namecoin name records to email forwarding configuration
2. **Mail Router** (`mail/router.go`): Routes .bit addresses to real email addresses with caching
3. **SMTP Relay** (`mail/smtp.go`): Accepts mail to .bit addresses and forwards to real inboxes

The permamail CLI integrates these components into a single tool for managing email forwarding.

### Email Configuration Format

Email forwarding is stored in Namecoin name values as JSON:

```json
{
  "email": "user@gmail.com",
  "backup": ["backup@proton.me"],
  "pubkey": "base64_encoded_public_key"
}
```

### Running an SMTP Relay

To run a production SMTP relay that forwards .bit emails:

```bash
# Start relay on port 2525, forwarding via Gmail
permamail serve --listen :2525 \
                --upstream smtp.gmail.com \
                --upstreamport 587 \
                --smtpuser your.email@gmail.com \
                --smtppass your-app-password

# Users can now send email to alice@mail.bit
# The relay will resolve alice -> user@gmail.com and forward
```

**Note**: Currently, name registration and updates require a running nmcd node with wallet enabled. The `register` and `update` commands prepare the configuration and display instructions for completing the operation via nmcd RPC. Full integrated wallet support is planned for future releases.

### Examples

See `examples/smtp_relay/` for a complete production-ready SMTP relay deployment with systemd integration.

---

## Security

### RPC Security Features

**Authentication:**
- Use `-rpcuser` and `-rpcpassword` to require HTTP Basic Authentication for all RPC requests
- Constant-time comparison prevents timing attacks on credentials

**Rate Limiting:**
- Per-IP rate limiting using token bucket algorithm (default: 100 tokens/minute refill rate)
- Allows burst of up to 100 requests, then continuous refill at 100 tokens/minute
- Configurable via `Config.RateLimit` when using as a library
- Automatic cleanup of stale IP tracking entries

**Request Size Limits:**
- Maximum request body size enforced (default: 1MB)
- Configurable via `Config.MaxRequestSize` when using as a library
- Early rejection prevents memory exhaustion attacks

**Security Headers:**
- `X-Content-Type-Options: nosniff` - Prevents MIME type sniffing
- `X-Frame-Options: DENY` - Prevents clickjacking attacks
- `Content-Security-Policy: default-src 'none'` - Restricts resource loading

**Example RPC Server Configuration:**
```go
server, err := rpc.NewServer(&rpc.Config{
    Blockchain:     blockchain,
    PeerMgr:        peerMgr,
    Wallet:         wallet,
    ListenAddr:     "127.0.0.1:8336",
    RPCUser:        "myuser",
    RPCPassword:    "mypassword",
    RateLimit:      100,        // requests per minute (0 = use default)
    MaxRequestSize: 1024 * 1024, // 1MB (0 = use default)
})
```

### Name Operation Security

- All name operations are validated before blockchain processing
- Names must be unique and unexpired
- Value size limits enforced (1023 bytes)
- Name length limits enforced (1-255 characters)

---

## Getting Help

- **📖 Documentation**: Start with [docs/EXAMPLES.md](docs/EXAMPLES.md) for guided walkthroughs
- **💡 API Reference**: See [docs/API.md](docs/API.md) for complete method signatures
- **🔧 Integration**: Check [docs/EMBEDDING.md](docs/EMBEDDING.md) for integration patterns
- **⚡ Performance**: Read [docs/PERFORMANCE.md](docs/PERFORMANCE.md) for optimization tips
- **🐛 Issues**: Report bugs on [GitHub Issues](https://github.com/opd-ai/nmcd/issues)

---

## Testing

nmcd has comprehensive test coverage across all packages:

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with race detector
go test -race ./...

# Generate HTML coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### Test Coverage Status

**Critical Packages:**
- chain: 79.8% ✅ (core blockchain and name validation)
- rpc: 81.0% ✅ (RPC server and methods)
- network: 75.9% ✅ (P2P networking and mempool)
- namedb: 81.9% ✅ (name database operations)

**Other Packages:**
- bridge: 100.0% ✅ (email bridge)
- client: 82.5% ✅ (client library)
- config: 94.9% ✅ (configuration)
- metrics: 83.9% ✅ (Prometheus metrics)
- wallet: 83.7% ✅ (wallet operations)

**Note:** Command-line entry points (cmd/*) and integration components have lower coverage by design, as they primarily wire together well-tested components.

📊 **Detailed Coverage Analysis**: See [docs/development/COVERAGE.md](docs/development/COVERAGE.md) for comprehensive coverage analysis and improvement roadmap.

---

## Building and Releases

### Building from Source

To build nmcd from source, you need Go 1.24 or later:

```bash
# Clone the repository
git clone https://github.com/opd-ai/nmcd
cd nmcd

# Build the binary
make build

# Or build with custom version
go build -ldflags="-X github.com/opd-ai/nmcd/internal/version.Version=1.0.0" ./cmd/nmcd
```

### Official Releases

nmcd provides automated binary releases for multiple platforms. Releases are triggered when a version tag is pushed:

**Supported Platforms:**
- Linux: amd64, arm64
- macOS: amd64 (Intel), arm64 (Apple Silicon)
- Windows: amd64

**Download Options:**

1. **Pre-built Binaries**: Download from [GitHub Releases](https://github.com/opd-ai/nmcd/releases)
   - All binaries include SHA256 checksums for verification
   - Extract and run directly (no installation required)

2. **Docker Images**: Multi-architecture images available on GitHub Container Registry
   ```bash
   # Pull latest version
   docker pull ghcr.io/opd-ai/nmcd:latest
   
   # Run nmcd in Docker, exposing RPC only on localhost
   docker run -p 127.0.0.1:8336:8336 -v nmcd-data:/data ghcr.io/opd-ai/nmcd:latest
   
   # Pull specific version
   docker pull ghcr.io/opd-ai/nmcd:1.0.0
   ```
   
   Docker images are optimized with multi-stage builds and compressed to <100MB.
   
   > **Security note:** The JSON-RPC interface is sensitive. Before exposing port `8336` beyond localhost
   > (for example by using `-p 8336:8336` or binding to a non-loopback address), configure strong
   > RPC credentials via `-rpcuser` and `-rpcpassword` (or enforce equivalent network access controls
   > such as a firewall, VPN, or TLS-terminating reverse proxy) to prevent unauthorized access.

**Verifying Downloads:**

```bash
# Verify SHA256 checksum (Linux/macOS)
sha256sum -c nmcd-linux-amd64.sha256

# Or manually compare
sha256sum nmcd-linux-amd64
cat nmcd-linux-amd64.sha256
```

---

## Contributing

Contributions are welcome! Areas of interest:

- **Examples**: Add new example applications showcasing library usage
- **Documentation**: Improve guides, add tutorials, fix typos
- **Testing**: Expand test coverage, add integration tests
- **Features**: Implement missing Namecoin protocol features
- **Performance**: Optimize hot paths, reduce memory usage

Please ensure:
1. Code follows Go best practices (`go fmt`, `go vet`)
2. All tests pass (`make test`)
3. Documentation is updated for API changes
4. Examples are updated if public API changes

---

## License

See LICENSE file for details.
