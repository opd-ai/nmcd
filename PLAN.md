# Embedded Namecoin Library API - Implementation Plan

**Status:** ⏳ In Progress (Phase 1 Complete ✅)  
**Created:** 2026-01-02  
**Updated:** 2026-01-02  
**Target:** Embedded Go library for Namecoin name resolution and registration  
**Priority:** Post-PROTOCOL_COMPLIANCE_AUDIT.md resolution (AUDIT COMPLETE ✅)

---

## EXECUTIVE SUMMARY

This document outlines the implementation plan for transforming nmcd from a standalone daemon into a dual-purpose library that supports both:

1. **Embedded mode** (default): In-process namedb, blockchain validation, and peer networking without external daemon
2. **Daemon mode** (fallback): Automatic detection and use of existing nmcd/Namecoin Core daemon via RPC when available on localhost

**Key Design Principle:** Library-first architecture with the existing nmcd CLI as a consumer of the library, not the other way around.

**Implementation Complexity:** Medium (4-6 weeks)  
**Breaking Changes:** Minimal (existing CLI interface preserved, library adds new capabilities)

---

## 1. ARCHITECTURE OVERVIEW

### 1.1 Current Architecture (nmcd as Daemon)

```
┌─────────────────────────────────────────────────────────────┐
│                         nmcd Binary                          │
│  ┌──────────┐  ┌────────┐  ┌────────┐  ┌────────┐  ┌──────┐│
│  │cmd/nmcd/ │→ │ chain/ │→ │namedb/ │← │network/│← │ rpc/ ││
│  │  main    │  │blockchain│  │bbolt DB│  │ peer   │  │HTTP ││
│  │  flags   │  │validate │  │ names  │  │ mgmt   │  │JSON ││
│  └──────────┘  └────────┘  └────────┘  └────────┘  └──────┘│
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
                    JSON-RPC over HTTP
                              │
                              ▼
                    External RPC Clients
```

**Characteristics:**
- Monolithic binary with all components tightly coupled
- Main entry point initializes everything in sequence
- RPC server as external interface only
- No programmatic Go API for embedding

### 1.2 Target Architecture (Library + Daemon)

```
┌───────────────────────────────────────────────────────────────────┐
│                     nmcd Library Package                          │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │              NameClient Interface (Public API)             │  │
│  │  • ResolveName(name) → NameRecord                          │  │
│  │  • RegisterName(name, value, opts) → TxHash                │  │
│  │  • UpdateName(name, value, opts) → TxHash                  │  │
│  │  • ListNames(filter) → []NameRecord                        │  │
│  └────────────────────────────────────────────────────────────┘  │
│            ▲                                    ▲                  │
│            │                                    │                  │
│  ┌─────────┴─────────┐              ┌──────────┴──────────┐      │
│  │  EmbeddedClient   │              │   DaemonClient      │      │
│  │  (in-process)     │              │   (RPC to daemon)   │      │
│  └─────────┬─────────┘              └──────────┬──────────┘      │
│            │                                    │                  │
│  ┌─────────▼─────────────────────────┐         │                  │
│  │  Embedded Components               │         │                  │
│  │  ┌────────┐ ┌────────┐ ┌────────┐│         │                  │
│  │  │ chain/ │ │namedb/ │ │network/││         │                  │
│  │  │validate│ │bbolt DB│ │ peer   ││         │                  │
│  │  └────────┘ └────────┘ └────────┘│         │                  │
│  └───────────────────────────────────┘         │                  │
└───────────────────────────────────────────────────────────────────┘
            ▲                                     │
            │                                     │
┌───────────┴───────────┐           ┌─────────────▼──────────────┐
│   Application Code    │           │      nmcd Daemon           │
│   (embedded usage)    │           │   (existing or Core)       │
│                       │           │   RPC Server on :8336      │
└───────────────────────┘           └────────────────────────────┘
```

**Key Changes:**
1. **New `client/` package**: Public API with `NameClient` interface
2. **EmbeddedClient**: In-process implementation using existing components
3. **DaemonClient**: RPC client implementation (fallback mode)
4. **Auto-detection**: Probes localhost:8336 for daemon, falls back to embedded
5. **Refactored `cmd/nmcd`**: Uses library instead of direct component access

### 1.3 Mode Selection Logic

```go
// Automatic mode selection (recommended)
client, err := nmcd.NewClient(nil) // Auto-detects daemon, falls back to embedded

// Explicit embedded mode
client, err := nmcd.NewClient(&nmcd.Config{
    Mode: nmcd.ModeEmbedded,
    DataDir: "~/.nmcd",
})

// Explicit daemon mode
client, err := nmcd.NewClient(&nmcd.Config{
    Mode: nmcd.ModeDaemon,
    RPCAddr: "http://localhost:8336",
    RPCUser: "user",
    RPCPassword: "pass",
})
```

**Decision Flow:**
1. If `Mode` is set explicitly, use that mode
2. If `Mode` is auto (default):
   - Try to connect to daemon on `RPCAddr` (default localhost:8336)
   - If daemon responds and passes health check → DaemonClient
   - If daemon unreachable → EmbeddedClient
3. Return error only if explicitly requested mode fails

---

## 2. PUBLIC API DESIGN

### 2.1 Core Interfaces

```go
// Package: github.com/opd-ai/nmcd/client

// NameClient is the primary interface for interacting with the Namecoin network.
// It provides methods for name resolution, registration, and management.
// Implementations include EmbeddedClient (in-process) and DaemonClient (RPC).
//
// Thread-safety: All methods are safe for concurrent use.
type NameClient interface {
    // ResolveName retrieves the current value and metadata for a name.
    // Returns ErrNameNotFound if the name doesn't exist or has expired.
    ResolveName(ctx context.Context, name string) (*NameRecord, error)

    // RegisterName creates a new name registration with the given value.
    // This is a two-step process:
    //   1. Issues NAME_NEW commitment (returns immediately with commitment TX hash)
    //   2. After MinBlocksBeforeFirstUpdate (12 blocks), issues NAME_FIRSTUPDATE
    //
    // Opts.WaitForConfirmation can be set to wait for both steps to complete.
    // Returns the final transaction hash once registration is complete.
    RegisterName(ctx context.Context, name, value string, opts *RegisterOpts) (*TxResult, error)

    // UpdateName updates an existing name's value.
    // The wallet must contain the private key for the address that owns the name.
    // Returns the transaction hash of the NAME_UPDATE operation.
    UpdateName(ctx context.Context, name, value string, opts *UpdateOpts) (*TxResult, error)

    // ListNames returns all registered names, optionally filtered.
    // Filter can match name patterns, namespaces (d/, id/, p/), or addresses.
    ListNames(ctx context.Context, filter *ListFilter) ([]*NameRecord, error)

    // GetNameHistory returns the full history of operations for a name.
    // Includes all NAME_FIRSTUPDATE and NAME_UPDATE operations.
    GetNameHistory(ctx context.Context, name string) ([]*NameRecord, error)

    // WaitForConfirmation waits for a transaction to be confirmed in a block.
    // Blocks until the transaction appears in the blockchain or context is canceled.
    WaitForConfirmation(ctx context.Context, txHash string, confirmations int) error

    // GetInfo returns general information about the node/network state.
    GetInfo(ctx context.Context) (*NodeInfo, error)

    // Close releases resources held by the client.
    // For EmbeddedClient, this includes closing the database and stopping network threads.
    // For DaemonClient, this closes the HTTP connection pool.
    Close() error
}

// NameRecord represents a name registration with its current state.
type NameRecord struct {
    Name       string    // Name identifier (e.g., "d/example")
    Value      string    // Current value (typically JSON for d/ and id/ namespaces)
    TxHash     string    // Transaction hash of last operation
    Height     int32     // Block height where last operation occurred
    ExpiresAt  int32     // Block height when name expires
    ExpiresIn  int32     // Blocks remaining until expiration
    Address    string    // Current owner address
    UpdatedAt  time.Time // Timestamp of last update
}

// RegisterOpts configures name registration behavior.
type RegisterOpts struct {
    // FromAddress specifies which wallet address to use for the registration.
    // If empty, uses the first available address or generates a new one.
    FromAddress string

    // WaitForConfirmation blocks until NAME_FIRSTUPDATE is confirmed.
    // Default: false (returns immediately after NAME_NEW is broadcast)
    WaitForConfirmation bool

    // Confirmations is the number of block confirmations to wait for.
    // Only used if WaitForConfirmation is true.
    // Default: 1
    Confirmations int

    // FeeRate in satoshis per byte for transaction fees.
    // Default: 1 (1 satoshi/byte = 1000 satoshis/KB)
    FeeRate int64
}

// UpdateOpts configures name update behavior.
type UpdateOpts struct {
    // TransferTo transfers the name to a new address.
    // If empty, name stays at current address.
    TransferTo string

    // WaitForConfirmation blocks until NAME_UPDATE is confirmed.
    // Default: false (returns immediately after broadcast)
    WaitForConfirmation bool

    // Confirmations is the number of block confirmations to wait for.
    // Default: 1
    Confirmations int

    // FeeRate in satoshis per byte for transaction fees.
    // Default: 1
    FeeRate int64
}

// TxResult contains the result of a name operation transaction.
type TxResult struct {
    TxHash        string    // Transaction hash
    Name          string    // Name that was operated on
    Status        TxStatus  // Transaction status (pending, confirmed, failed)
    Confirmations int       // Number of confirmations (0 if pending)
    BlockHeight   int32     // Block height where confirmed (0 if pending)
    BlockHash     string    // Block hash where confirmed (empty if pending)
}

// TxStatus represents the status of a transaction.
type TxStatus string

const (
    TxStatusPending   TxStatus = "pending"   // In mempool, not yet confirmed
    TxStatusConfirmed TxStatus = "confirmed" // Included in a block
    TxStatusFailed    TxStatus = "failed"    // Rejected by network or reorged out
)

// ListFilter configures name list filtering.
type ListFilter struct {
    // NamePattern matches name prefix or glob pattern.
    // Examples: "d/example*", "id/*", "*"
    NamePattern string

    // Namespace filters by namespace prefix.
    // Examples: "d/", "id/", "p/"
    Namespace string

    // Address filters names owned by a specific address.
    Address string

    // IncludeExpired includes expired names in results.
    // Default: false (only active names)
    IncludeExpired bool

    // Limit limits the number of results returned.
    // Default: 100, Max: 10000
    Limit int

    // Offset for pagination.
    // Default: 0
    Offset int
}

// NodeInfo contains general information about the node state.
type NodeInfo struct {
    Version         string // nmcd version
    ProtocolVersion int    // Network protocol version
    BlockHeight     int32  // Current blockchain height
    BestBlockHash   string // Hash of the best block
    Connections     int    // Number of peer connections
    NetworkName     string // Network name (mainnet, testnet, regtest)
    Mode            string // Client mode (embedded, daemon)
}

// Config configures client behavior and mode selection.
type Config struct {
    // Mode explicitly sets the client mode.
    // If ModeAuto (default), automatically detects daemon or uses embedded.
    Mode ClientMode

    // DataDir is the data directory for embedded mode.
    // Default: ~/.nmcd
    DataDir string

    // Network specifies the network to use (mainnet, testnet, regtest).
    // Default: mainnet
    Network string

    // RPCAddr is the daemon RPC address for daemon mode.
    // Default: http://localhost:8336
    RPCAddr string

    // RPCUser is the RPC authentication username.
    // Required if daemon has authentication enabled.
    RPCUser string

    // RPCPassword is the RPC authentication password.
    // Required if daemon has authentication enabled.
    RPCPassword string

    // MaxPeers is the maximum number of peer connections for embedded mode.
    // Default: 8
    MaxPeers int

    // BootstrapPeers are initial peers to connect to in embedded mode.
    // If empty, uses DNS seed discovery.
    BootstrapPeers []string

    // DisableWallet disables wallet functionality.
    // Default: false
    DisableWallet bool

    // Dialer is a custom dialer for outgoing network connections in embedded mode.
    // Allows routing traffic through anonymous networks like Tor or I2P.
    // The dialer should return net.Conn compatible connections.
    // If nil, uses standard net.Dial for clearnet connections.
    //
    // Example (Tor):
    //   Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
    //       proxy, _ := proxy.SOCKS5("tcp", "127.0.0.1:9050", nil, proxy.Direct)
    //       return proxy.Dial(network, addr)
    //   }
    //
    // Example (I2P):
    //   Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
    //       return i2p.Dial(network, addr) // Using custom I2P dialer
    //   }
    Dialer func(ctx context.Context, network, addr string) (net.Conn, error)

    // Listener is a custom listener for incoming network connections in embedded mode.
    // Allows accepting connections through anonymous networks like Tor or I2P.
    // The listener should return net.Listener compatible listener.
    // If nil, uses standard net.Listen for clearnet connections.
    //
    // Example (Tor hidden service):
    //   Listener: func(network, addr string) (net.Listener, error) {
    //       return tor.Listen(network, addr) // Using Tor hidden service
    //   }
    //
    // Example (I2P):
    //   Listener: func(network, addr string) (net.Listener, error) {
    //       return i2p.Listen(network, addr) // Using I2P SAM listener
    //   }
    Listener func(network, addr string) (net.Listener, error)

    // Logger is a custom logger for client operations.
    // If nil, uses default logger.
    Logger *log.Logger
}

// ClientMode specifies the client operation mode.
type ClientMode int

const (
    ModeAuto     ClientMode = iota // Auto-detect daemon, fallback to embedded
    ModeEmbedded                   // Force embedded mode (in-process)
    ModeDaemon                     // Force daemon mode (RPC only)
)

// Errors returned by the client.
var (
    ErrNameNotFound      = errors.New("name not found")
    ErrNameExpired       = errors.New("name has expired")
    ErrNameExists        = errors.New("name already registered")
    ErrInvalidName       = errors.New("invalid name format")
    ErrInvalidValue      = errors.New("invalid value format")
    ErrInsufficientFunds = errors.New("insufficient funds for operation")
    ErrNoWallet          = errors.New("wallet not initialized")
    ErrDaemonUnavailable = errors.New("daemon unavailable")
    ErrContextCanceled   = errors.New("context canceled")
)
```

### 2.2 Initialization Patterns

```go
// Example 1: Simple auto-detection (recommended for most applications)
client, err := nmcd.NewClient(nil)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Example 2: Embedded mode with custom data directory
client, err := nmcd.NewClient(&nmcd.Config{
    Mode:    nmcd.ModeEmbedded,
    DataDir: "/var/lib/myapp/namecoin",
    Network: "mainnet",
})

// Example 3: Daemon mode with authentication
client, err := nmcd.NewClient(&nmcd.Config{
    Mode:        nmcd.ModeDaemon,
    RPCAddr:     "http://localhost:8336",
    RPCUser:     "rpcuser",
    RPCPassword: "rpcpassword",
})

// Example 4: Embedded mode with custom peers (no DNS seeds)
client, err := nmcd.NewClient(&nmcd.Config{
    Mode: nmcd.ModeEmbedded,
    BootstrapPeers: []string{
        "peer1.example.com:8334",
        "peer2.example.com:8334",
    },
})

// Example 5: Embedded mode over Tor network (anonymous connections)
import "golang.org/x/net/proxy"

torDialer, err := proxy.SOCKS5("tcp", "127.0.0.1:9050", nil, proxy.Direct)
if err != nil {
    log.Fatal(err)
}

client, err := nmcd.NewClient(&nmcd.Config{
    Mode:    nmcd.ModeEmbedded,
    Network: "mainnet",
    // Route all outgoing connections through Tor
    Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
        return torDialer.Dial(network, addr)
    },
    // Accept incoming connections as Tor hidden service
    Listener: func(network, addr string) (net.Listener, error) {
        // Use onion service listener (implementation depends on Tor library)
        return torOnionListener.Listen(network, addr)
    },
    BootstrapPeers: []string{
        // Onion addresses of Namecoin peers
        "nmc2exampleonion.onion:8334",
    },
})

// Example 6: Embedded mode over I2P network (anonymous peer-to-peer)
import "github.com/eyedeekay/sam3"

samConn, err := sam3.NewSAM("127.0.0.1:7656")
if err != nil {
    log.Fatal(err)
}

client, err := nmcd.NewClient(&nmcd.Config{
    Mode:    nmcd.ModeEmbedded,
    Network: "mainnet",
    // Route connections through I2P
    Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
        // Convert clearnet address to I2P destination if needed
        return samConn.Dial(network, addr)
    },
    Listener: func(network, addr string) (net.Listener, error) {
        // Create I2P destination and listen
        return samConn.Listen(network, addr)
    },
    BootstrapPeers: []string{
        // I2P addresses of Namecoin peers
        "nmc.i2p:8334",
    },
})

// Example 7: Hybrid mode - Tor for outgoing, clearnet for incoming
client, err := nmcd.NewClient(&nmcd.Config{
    Mode: nmcd.ModeEmbedded,
    // Tor for outgoing (privacy)
    Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
        return torDialer.Dial(network, addr)
    },
    // Standard clearnet for incoming (availability)
    Listener: func(network, addr string) (net.Listener, error) {
        return net.Listen(network, addr)
    },
})
```

### 2.3 Usage Examples

```go
// Example 1: Resolve a domain name
record, err := client.ResolveName(ctx, "d/example")
if err == nmcd.ErrNameNotFound {
    fmt.Println("Name not registered")
} else if err != nil {
    log.Fatal(err)
}
fmt.Printf("Domain: %s\nValue: %s\nOwner: %s\n", 
    record.Name, record.Value, record.Address)

// Example 2: Register a new domain
result, err := client.RegisterName(ctx, "d/mysite", `{"ip":"1.2.3.4"}`, &nmcd.RegisterOpts{
    WaitForConfirmation: true,
    Confirmations:       6, // Wait for 6 confirmations
})
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Registered! TX: %s\n", result.TxHash)

// Example 3: Update existing name
result, err := client.UpdateName(ctx, "d/mysite", `{"ip":"5.6.7.8"}`, &nmcd.UpdateOpts{
    WaitForConfirmation: true,
})

// Example 4: List all domains
records, err := client.ListNames(ctx, &nmcd.ListFilter{
    Namespace: "d/",
    Limit:     100,
})
for _, record := range records {
    fmt.Printf("%s: %s (expires in %d blocks)\n", 
        record.Name, record.Value, record.ExpiresIn)
}

// Example 5: Get name history
history, err := client.GetNameHistory(ctx, "d/example")
for i, record := range history {
    fmt.Printf("Operation %d: Height=%d, Value=%s\n", 
        i+1, record.Height, record.Value)
}
```

### 2.4 Error Handling and Retries

```go
// Built-in retry logic with exponential backoff
type RetryConfig struct {
    MaxAttempts int           // Default: 3
    InitialDelay time.Duration // Default: 100ms
    MaxDelay    time.Duration // Default: 5s
    Multiplier  float64       // Default: 2.0
}

// Example: Custom retry configuration
client, err := nmcd.NewClient(&nmcd.Config{
    Mode: nmcd.ModeEmbedded,
    RetryConfig: &nmcd.RetryConfig{
        MaxAttempts:  5,
        InitialDelay: 200 * time.Millisecond,
        MaxDelay:     10 * time.Second,
        Multiplier:   2.0,
    },
})

// Network errors (connection failures, timeouts) are automatically retried
// Application errors (ErrNameNotFound, ErrNameExpired) are NOT retried
// Context cancellation immediately aborts retries
```

---

## 3. IMPLEMENTATION PHASES

### Phase 1: Extract Reusable Components ✅ **COMPLETE** (2026-01-02)

**Goal:** Reorganize existing code to support library usage without breaking CLI

**Tasks:**
1. ✅ Create `client/` package with interface definitions
2. ✅ Move shared types from `cmd/nmcd/main.go` to `internal/server/`
3. ✅ Extract initialization logic into `internal/server/server.go`
4. ✅ Update `cmd/nmcd/main.go` to use server package (no functionality change)
5. ✅ Add integration tests to verify CLI still works

**Deliverables:**
- ✅ `client/types.go` - Public interface definitions (240 lines)
- ✅ `internal/server/server.go` - Daemon server implementation (205 lines)
- ✅ `internal/server/server_test.go` - Unit tests
- ✅ Updated `cmd/nmcd/main.go` using server package (reduced from 188 to 84 lines)
- ✅ Passing tests: `make test` succeeds

**Breaking Changes:** None (internal refactoring only)

**Completed:** 2026-01-02  
**Commit:** 55524fe - Phase 1 complete: Extract reusable components into library structure

**Validation:**
```bash
# Build and run CLI (should work identically) ✅
make build
./nmcd -datadir=/tmp/test-nmcd

# Run tests ✅
make test  # All tests passing
```

### Phase 2: Implement EmbeddedClient (Week 2-3) ⏳ **IN PROGRESS**

**Goal:** Create in-process client implementation

**Status:** Foundation complete + ListNames + GetNameHistory implemented (2026-01-02) ✅
- ✅ Task 1: Implement `client/embedded.go` with NameClient interface (partial - ResolveName, GetInfo, Close, ListNames, GetNameHistory)
- ✅ Task 1.1: Created EmbeddedClient struct with nameDB, wallet, and placeholder blockchain
- ✅ Task 1.2: Implemented NewEmbeddedClient() with configuration support (mainnet, testnet, regtest)
- ✅ Task 1.3: Implemented ResolveName() method for reading names from local database
- ✅ Task 1.4: Implemented GetInfo() method for node information
- ✅ Task 1.5: Implemented Close() method with resource cleanup
- ✅ Task 1.6: Added comprehensive tests (12 test functions, all passing)
- ✅ Task 1.7: Thread-safety verified with concurrent operations test
- ✅ Task 1.8: Implemented ListNames() method with filtering and pagination support (2026-01-02)
- ✅ Task 1.9: Implemented GetNameHistory() method for retrieving operation history (2026-01-02)
- ⏳ Task 2: Initialize full blockchain with btcd integration (deferred to next phase)
- ⏳ Task 3: Implement RegisterName with NAME_NEW → NAME_FIRSTUPDATE flow (deferred)
- ⏳ Task 4: Add support for custom Dialer and Listener for anonymous networks (deferred)
- ⏳ Task 5: Implement UpdateName, WaitForConfirmation (deferred)

**Completed Deliverables:**
- ✅ `client/embedded.go` - EmbeddedClient with ResolveName, GetInfo, Close, ListNames, GetNameHistory (534 lines)
- ✅ `client/embedded_test.go` - Comprehensive test suite (886 lines, 12 test functions)
- ✅ `chain/blockchain.go` - Added GetNameDB() method for database access
- ✅ All tests passing (100% success rate)

**Phase 2 Foundation Notes:**
- Simplified implementation focuses on read-only operations (ResolveName, ListNames, GetNameHistory)
- Full blockchain integration deferred to allow stepwise development
- Placeholder blockchain (height 0) used for expiration checks
- Database operations fully functional with proper thread safety
- Design allows easy integration of full blockchain in next iteration
- ListNames implemented with comprehensive filtering (2026-01-02):
  - Namespace filtering (d/, id/, p/)
  - Name pattern matching (prefix-based)
  - Address-based filtering
  - Expiration status filtering
  - Pagination with limit (max 10,000) and offset
  - Context cancellation support
  - Thread-safe concurrent operations
- GetNameHistory implemented with full operation history (2026-01-02):
  - Returns chronological history of all name operations
  - Supports all operation types (NAME_FIRSTUPDATE, NAME_UPDATE)
  - Context cancellation support
  - Thread-safe concurrent operations
  - Empty history handling for non-existent names

**Next Steps for Phase 2 Completion:**
1. Integrate full btcd blockchain with block database
2. Implement RegisterName and UpdateName operations
3. Add NAME_NEW tracker for pending registrations
4. Implement WaitForConfirmation
5. Add custom Dialer/Listener support for Tor/I2P

**Tasks:**
1. ✅ Implement `client/embedded.go` with NameClient interface (foundation + ListNames + GetNameHistory complete)
2. ⏳ Initialize blockchain, namedb, network components in-process
3. ✅ Implement thread-safe access to shared state (blockchain, namedb)
4. ✅ Add graceful shutdown with resource cleanup
5. ⏳ Implement `RegisterName` with NAME_NEW → NAME_FIRSTUPDATE flow
6. ⏳ Add support for custom Dialer and Listener (anonymous networks)
7. ✅ Add unit tests for EmbeddedClient methods
8. ✅ Implement `ListNames` with filtering and pagination (2026-01-02)
9. ✅ Implement `GetNameHistory` for operation history retrieval (2026-01-02)

**Technical Challenges:**

**Challenge 1: Database Locking**
- **Problem:** bbolt allows only one writer at a time
- **Solution:** Use read-write locks; batch write operations
- **Code Pattern:**
```go
type EmbeddedClient struct {
    chain   *chain.BlockChain
    nameDB  *namedb.NameDatabase // Already has RWMutex
    network *network.PeerManager
    mu      sync.RWMutex         // Protects client state
}

func (c *EmbeddedClient) ResolveName(ctx context.Context, name string) (*NameRecord, error) {
    // nameDB.GetName already uses RLock internally
    record, err := c.chain.GetName(name)
    // ... convert to client.NameRecord
}
```

**Challenge 2: Blockchain Sync Strategy**
- **Problem:** Full blockchain sync is slow and resource-intensive
- **Solution:** Implement simplified sync modes
  - **Mode 1 (Default):** Headers-only sync + name transactions only
  - **Mode 2 (Full):** Full blockchain sync (like current daemon)
  - **Mode 3 (Name-only):** Trust checkpoints, validate name operations only
- **Implementation:**
```go
type SyncMode int

const (
    SyncModeHeadersOnly SyncMode = iota // Fast: headers + name TXs only
    SyncModeFull                        // Slow: full blockchain validation
    SyncModeNameOnly                    // Fastest: trust checkpoints, names only
)

// Config.SyncMode determines blockchain sync strategy
```

**Challenge 3: Concurrent NAME_NEW Management**
- **Problem:** NAME_NEW requires waiting 12 blocks before NAME_FIRSTUPDATE
- **Solution:** Background goroutine tracks pending registrations
- **Code Pattern:**
```go
type pendingRegistration struct {
    Name         string
    Value        string
    CommitTxHash string
    CommitHeight int32
    WaitChan     chan error
}

// Background goroutine in EmbeddedClient
func (c *EmbeddedClient) watchPendingRegistrations() {
    ticker := time.NewTicker(30 * time.Second) // Check every ~3 blocks
    for {
        select {
        case <-ticker.C:
            c.processPendingRegistrations()
        case <-c.stopCh:
            return
        }
    }
}
```

**Deliverables:**
- ✅ `client/embedded.go` - Full EmbeddedClient implementation
- ✅ `client/embedded_test.go` - Comprehensive unit tests
- ✅ Background NAME_NEW tracker with tests
- ✅ Resource cleanup on Close()

**Breaking Changes:** None (new functionality)

**Validation:**
```bash
# Run embedded client tests
go test -v ./client -run TestEmbedded

# Manual test: embed in example program
cd examples
go run embedded_client_example.go
```

### Phase 3: Implement DaemonClient (Week 3-4)

**Goal:** Create RPC client implementation for daemon fallback

**Tasks:**
1. Implement `client/daemon.go` with NameClient interface
2. Create RPC client using existing RPC server endpoints
3. Add daemon health check and auto-reconnection
4. Implement RPC request/response with retries
5. Add unit tests with mock RPC server

**Technical Considerations:**

**RPC Client Design:**
```go
type DaemonClient struct {
    httpClient *http.Client
    baseURL    string
    auth       *basicAuth
    mu         sync.RWMutex
}

type basicAuth struct {
    username string
    password string
}

func (c *DaemonClient) rpcCall(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
    req := &rpc.Request{
        Jsonrpc: "2.0",
        Method:  method,
        Params:  params,
        ID:      1,
    }
    
    // Marshal request
    body, err := json.Marshal(req)
    // ... HTTP POST with context, auth, retry logic
}

// Health check: probes daemon availability
func (c *DaemonClient) Ping(ctx context.Context) error {
    _, err := c.rpcCall(ctx, "getinfo", nil)
    return err
}
```

**Deliverables:**
- ✅ `client/daemon.go` - DaemonClient implementation
- ✅ `client/daemon_test.go` - Tests with mock server
- ✅ Retry logic with exponential backoff
- ✅ Auto-reconnection on connection failure

**Breaking Changes:** None (new functionality)

**Validation:**
```bash
# Start daemon
./nmcd -datadir=/tmp/daemon-test

# Test daemon client in another terminal
go test -v ./client -run TestDaemon
```

### Phase 4: Auto-Detection and Integration (Week 4-5)

**Goal:** Implement mode auto-detection and finalize public API

**Tasks:**
1. Implement `client/client.go` with NewClient factory
2. Add daemon auto-detection logic
3. Create `examples/` demonstrating all use cases
4. Add integration tests covering both modes
5. Update documentation (README, API docs)

**Auto-Detection Logic:**
```go
func NewClient(cfg *Config) (NameClient, error) {
    if cfg == nil {
        cfg = defaultConfig()
    }

    switch cfg.Mode {
    case ModeEmbedded:
        return newEmbeddedClient(cfg)
    
    case ModeDaemon:
        return newDaemonClient(cfg)
    
    case ModeAuto:
        // Try daemon first
        daemonClient, err := newDaemonClient(cfg)
        if err == nil {
            // Daemon available - use it
            if err := daemonClient.Ping(context.Background()); err == nil {
                return daemonClient, nil
            }
        }
        // Daemon unavailable - fallback to embedded
        return newEmbeddedClient(cfg)
    
    default:
        return nil, fmt.Errorf("unknown client mode: %v", cfg.Mode)
    }
}
```

**Deliverables:**
- ✅ `client/client.go` - NewClient factory with auto-detection
- ✅ `examples/simple_resolve.go` - Basic name resolution
- ✅ `examples/register_name.go` - Name registration flow
- ✅ `examples/update_name.go` - Name update flow
- ✅ `examples/list_names.go` - Name listing and filtering
- ✅ Integration tests: `client/integration_test.go`
- ✅ Updated README with library usage examples

**Breaking Changes:** None (additive only)

**Validation:**
```bash
# Run all examples
cd examples
go run simple_resolve.go
go run register_name.go d/test '{"ip":"1.2.3.4"}'
go run update_name.go d/test '{"ip":"5.6.7.8"}'
go run list_names.go --namespace=d/

# Run integration tests
go test -v ./client -run TestIntegration
```

### Phase 5: Documentation and Examples (Week 5-6)

**Goal:** Comprehensive documentation and real-world examples

**Tasks:**
1. Generate godoc for all public APIs
2. Write usage guides for common scenarios
3. Create example applications (DNS resolver, domain registry UI)
4. Add benchmarks for performance testing
5. Update README with library-first approach

**Documentation Structure:**
```
docs/
├── API.md              # Full API reference with examples
├── EMBEDDING.md        # Guide for embedding nmcd in applications
├── MODES.md            # Detailed comparison of embedded vs daemon modes
├── PERFORMANCE.md      # Benchmarks and optimization guide
└── EXAMPLES.md         # Annotated example applications
```

**Example Applications:**
1. **DNS Bridge:** Resolves Namecoin domains via DNS protocol
2. **Name Registry CLI:** Command-line tool using library (like `namecoin-cli`)
3. **Web Dashboard:** Simple web UI for name management
4. **Domain Monitor:** Background service monitoring name expirations
5. **Tor/I2P Bridge:** Anonymous Namecoin client over Tor or I2P networks

**Deliverables:**
- ✅ Complete API documentation
- ✅ Four example applications
- ✅ Benchmarks for key operations
- ✅ Updated README and migration guide

**Breaking Changes:** None (documentation only)

---

## 4. TECHNICAL CONSIDERATIONS

### 4.1 Thread Safety

**Requirement:** All public API methods must be safe for concurrent use.

**Implementation Strategy:**

1. **EmbeddedClient Concurrency:**
```go
type EmbeddedClient struct {
    chain      *chain.BlockChain     // Has internal RWMutex
    nameDB     *namedb.NameDatabase  // Has internal RWMutex
    network    *network.PeerManager  // Has internal sync primitives
    wallet     *wallet.Wallet        // Has internal Mutex
    
    pendingRegs map[string]*pendingRegistration // Protected by pendingMu
    pendingMu   sync.RWMutex
    
    stopCh      chan struct{}        // Signals shutdown
    wg          sync.WaitGroup       // Tracks background goroutines
}

// Thread-safe registration tracking
func (c *EmbeddedClient) addPendingRegistration(reg *pendingRegistration) {
    c.pendingMu.Lock()
    defer c.pendingMu.Unlock()
    c.pendingRegs[reg.Name] = reg
}
```

2. **DaemonClient Concurrency:**
```go
type DaemonClient struct {
    httpClient *http.Client // net/http.Client is thread-safe
    baseURL    string
    auth       *basicAuth
    
    // No internal state to protect - stateless RPC calls
}
```

3. **Lock Ordering Rules (to prevent deadlocks):**
```
Global order (outer to inner):
1. EmbeddedClient.pendingMu (pending registrations)
2. chain.BlockChain.mu (blockchain state)
3. namedb.NameDatabase.mu (database operations)

Never acquire locks in reverse order!
```

### 4.2 Database Sharing

**Challenge:** Can embedded client and daemon share the same database?

**Answer:** **No, by design.** bbolt (BoltDB) allows only one writer process at a time.

**Solutions:**

**Option 1: Separate Data Directories (Recommended)**
```go
// Embedded mode: ~/.nmcd/embedded/
// Daemon mode: ~/.nmcd/daemon/

cfg := &nmcd.Config{
    Mode:    nmcd.ModeEmbedded,
    DataDir: filepath.Join(homeDir, ".nmcd", "embedded"),
}
```

**Option 2: Use Daemon Mode When Daemon is Running**
```go
// Auto-detection handles this:
// If daemon is running, use DaemonClient (RPC)
// If daemon is not running, use EmbeddedClient (separate DB)

client, err := nmcd.NewClient(nil) // Auto mode
```

**Option 3: File Locking for Mutual Exclusion**
```go
// Check if daemon is holding the lock
func tryLockDataDir(dataDir string) (*os.File, error) {
    lockFile := filepath.Join(dataDir, ".lock")
    f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0600)
    if err != nil {
        return nil, err
    }
    
    // Try to acquire exclusive lock
    if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
        f.Close()
        return nil, ErrDataDirLocked
    }
    
    return f, nil
}
```

**Recommendation:** Use Option 2 (auto-detection) for most cases. Only expert users should manage database locks manually.

### 4.3 Network Abstraction for Anonymous Networks

**Requirement:** Support for anonymous networks (Tor, I2P) and custom network implementations via pluggable Dialer and Listener interfaces.

**Design Principle:** Use interface types (net.Conn, net.Listener) consistently, never concrete types (*net.TCPConn, *net.UDPConn). This enables transparent network abstraction.

**Implementation Strategy:**

**1. Dialer Interface for Outgoing Connections:**

```go
// EmbeddedClient uses custom dialer if provided, falls back to net.Dialer
type EmbeddedClient struct {
    dialer func(ctx context.Context, network, addr string) (net.Conn, error)
    // ... other fields
}

// Connect to peer using custom dialer
func (c *EmbeddedClient) connectPeer(ctx context.Context, addr string) (net.Conn, error) {
    if c.dialer != nil {
        // Use custom dialer (Tor, I2P, etc.)
        return c.dialer(ctx, "tcp", addr)
    }
    
    // Fallback to standard clearnet dialer
    var d net.Dialer
    return d.DialContext(ctx, "tcp", addr)
}
```

**2. Listener Interface for Incoming Connections:**

```go
// Accept incoming peer connections using custom listener
func (c *EmbeddedClient) startListener(network, addr string) (net.Listener, error) {
    if c.listener != nil {
        // Use custom listener (Tor hidden service, I2P SAM, etc.)
        return c.listener(network, addr)
    }
    
    // Fallback to standard clearnet listener
    return net.Listen(network, addr)
}
```

**3. Integration with btcd/peer:**

The btcd/peer package already uses interface types for network connections, making it compatible with custom dialers:

```go
// btcd/peer.Config accepts net.Conn, not concrete TCP connections
peerCfg := &peer.Config{
    // ... configuration
}

// Connect using custom dialer
conn, err := c.connectPeer(ctx, peerAddr)
if err != nil {
    return err
}

// btcd/peer.NewOutboundPeer accepts any net.Conn
p, err := peer.NewOutboundPeer(peerCfg, peerAddr)
p.AssociateConnection(conn)
```

**4. DNS Resolution for Anonymous Networks:**

Different networks have different naming conventions:
- **Clearnet:** DNS names (peer.example.com)
- **Tor:** Onion addresses (xyz.onion)
- **I2P:** Base32 addresses (xyz.b32.i2p) or addressbook names (peer.i2p)

```go
// Network-aware peer resolution
func (c *EmbeddedClient) resolvePeer(addr string) ([]string, error) {
    if c.dialer == nil {
        // Clearnet: use DNS
        return net.LookupHost(addr)
    }
    
    // Anonymous network: assume address is already resolved
    // (Tor .onion, I2P .i2p addresses don't use DNS)
    return []string{addr}, nil
}
```

**5. Bootstrap Peers for Anonymous Networks:**

Allow mixed clearnet and anonymous bootstrap peers:

```go
cfg := &nmcd.Config{
    Mode: nmcd.ModeEmbedded,
    Dialer: torDialer, // Routes through Tor
    BootstrapPeers: []string{
        // Mix of clearnet and onion addresses
        "peer1.example.com:8334",           // Clearnet (accessed via Tor exit)
        "nmc2exampleonion.onion:8334",      // Tor hidden service
        "nmci2pexample.b32.i2p:8334",       // I2P eepsite
    },
}
```

**6. Network Type Detection:**

Automatically detect network type from address format:

```go
func detectNetworkType(addr string) string {
    switch {
    case strings.HasSuffix(addr, ".onion"):
        return "tor"
    case strings.HasSuffix(addr, ".i2p"):
        return "i2p"
    default:
        return "clearnet"
    }
}
```

**7. Thread Safety for Network Operations:**

All network operations must respect context cancellation:

```go
func (c *EmbeddedClient) connectPeer(ctx context.Context, addr string) (net.Conn, error) {
    // Check context before dialing
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    
    // Dial with context (respects timeout and cancellation)
    conn, err := c.dialer(ctx, "tcp", addr)
    if err != nil {
        return nil, err
    }
    
    return conn, nil
}
```

**8. Testing with Mock Networks:**

Custom dialers enable comprehensive testing without real network:

```go
// Test with in-memory pipe network
func TestEmbeddedClientWithMockNetwork(t *testing.T) {
    server, client := net.Pipe()
    
    cfg := &nmcd.Config{
        Mode: nmcd.ModeEmbedded,
        Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
            return client, nil
        },
    }
    
    nc, err := nmcd.NewClient(cfg)
    // ... test network operations
}
```

**Security Considerations:**

- **DNS Leaks:** When using Tor/I2P, ensure DNS resolution also goes through the network (use SOCKS5 DNS or local resolution)
- **Connection Metadata:** Custom dialers should preserve connection privacy (no clearnet fallback)
- **Peer Discovery:** Disable DNS seed discovery when using anonymous networks (use manual bootstrap peers only)
- **Network Isolation:** Consider separate data directories for different network modes to prevent correlation

**Example: Complete Tor Configuration**

```go
import (
    "context"
    "net"
    "golang.org/x/net/proxy"
)

// Create Tor SOCKS5 dialer with DNS resolution through Tor
torDialer, err := proxy.SOCKS5("tcp", "127.0.0.1:9050", nil, proxy.Direct)
if err != nil {
    log.Fatal(err)
}

client, err := nmcd.NewClient(&nmcd.Config{
    Mode:    nmcd.ModeEmbedded,
    Network: "mainnet",
    DataDir: "~/.nmcd-tor", // Separate data directory
    
    // All outgoing connections through Tor
    Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
        return torDialer.Dial(network, addr)
    },
    
    // Listen as Tor hidden service (requires torrc configuration)
    Listener: func(network, addr string) (net.Listener, error) {
        // Tor control library creates hidden service
        return torControl.CreateHiddenService(addr)
    },
    
    // Use only onion bootstrap peers (no clearnet, no DNS seeds)
    BootstrapPeers: []string{
        "nmc1exampleonion.onion:8334",
        "nmc2exampleonion.onion:8334",
    },
    
    // Disable DNS seeds (would leak to clearnet)
    MaxPeers: 8,
})
```

### 4.4 Blockchain Sync Strategy

**Challenge:** Full blockchain sync takes hours/days. Embedded clients need faster startup.

**Sync Modes:**

**1. Headers-Only Sync (Default for Embedded)**
- Download block headers only (~80 bytes/block = ~6 MB for 75k blocks)
- Download transactions containing name operations only
- Validate name operations against lightweight SPV proofs
- **Startup Time:** ~5 minutes
- **Disk Usage:** ~50 MB
- **Security:** SPV-level (trusts longest chain)

**2. Full Sync (Default for Daemon)**
- Download and validate all blocks and transactions
- Full blockchain validation including UTXO set
- **Startup Time:** Hours to days (depending on network)
- **Disk Usage:** ~2-5 GB
- **Security:** Full node security

**3. Name-Only Sync (Fast Mode)**
- Trust hardcoded checkpoints (validated by developers)
- Download only name operations after latest checkpoint
- **Startup Time:** ~1 minute
- **Disk Usage:** ~10 MB
- **Security:** Trusts checkpoint; validates names after checkpoint

**Configuration:**
```go
cfg := &nmcd.Config{
    Mode:     nmcd.ModeEmbedded,
    SyncMode: nmcd.SyncModeHeadersOnly, // or SyncModeFull, SyncModeNameOnly
}
```

**Implementation Notes:**
- Headers-Only and Name-Only modes require adding SPV validation (not in current implementation)
- Phase 1-4 will use Full Sync mode only
- SPV modes can be added as enhancement in future phases

### 4.5 Resource Management

**Requirements:**
1. Graceful shutdown releases all resources
2. Database writes are flushed before Close()
3. Network connections are closed cleanly
4. Background goroutines are stopped

**Implementation:**

```go
func (c *EmbeddedClient) Close() error {
    // Signal shutdown to background goroutines
    close(c.stopCh)
    
    // Wait for background goroutines to finish
    // Timeout after 30 seconds to prevent hanging
    done := make(chan struct{})
    go func() {
        c.wg.Wait()
        close(done)
    }()
    
    select {
    case <-done:
        // Clean shutdown
    case <-time.After(30 * time.Second):
        return fmt.Errorf("shutdown timeout: background goroutines did not finish")
    }
    
    // Stop network connections
    if c.network != nil {
        c.network.Stop()
    }
    
    // Close blockchain (flushes caches)
    if c.chain != nil {
        if err := c.chain.Close(); err != nil {
            return fmt.Errorf("failed to close blockchain: %w", err)
        }
    }
    
    // Close database (final flush)
    if c.nameDB != nil {
        if err := c.nameDB.Close(); err != nil {
            return fmt.Errorf("failed to close database: %w", err)
        }
    }
    
    return nil
}

// Context-aware operations
func (c *EmbeddedClient) RegisterName(ctx context.Context, name, value string, opts *RegisterOpts) (*TxResult, error) {
    // Check context before expensive operations
    select {
    case <-ctx.Done():
        return nil, ErrContextCanceled
    default:
    }
    
    // Submit NAME_NEW
    tx, err := c.createNameNew(name, value)
    if err != nil {
        return nil, err
    }
    
    // If waiting for confirmation, respect context cancellation
    if opts.WaitForConfirmation {
        if err := c.waitForBlocks(ctx, opts.Confirmations); err != nil {
            return nil, err
        }
    }
    
    return &TxResult{TxHash: tx.TxHash().String()}, nil
}
```

### 4.6 btcd Composition Principles

**Design Principle:** Compose with btcd libraries, don't reimplement or fork.

**Current Implementation:**
```go
// Good: Composition via embedding
type BlockChain struct {
    *blockchain.BlockChain  // Embed btcd blockchain
    nameDB      *namedb.NameDatabase
    chainParams *chaincfg.Params
    mu          sync.RWMutex
}

// Good: Extend btcd types with Namecoin-specific validation
func (bc *BlockChain) ProcessBlock(block *btcutil.Block, flags blockchain.BehaviorFlags) (bool, bool, error) {
    // Validate Namecoin name operations
    if err := bc.validateNameOperations(block); err != nil {
        return false, false, err
    }
    
    // Delegate to btcd for Bitcoin-compatible validation
    return bc.BlockChain.ProcessBlock(block, flags)
}
```

**Library Integration:**
- ✅ Use `btcd/blockchain` for blockchain management
- ✅ Use `btcd/peer` for P2P networking
- ✅ Use `btcd/wire` for message serialization
- ✅ Use `btcd/chaincfg` for chain parameters
- ❌ Don't fork btcd packages
- ❌ Don't copy btcd code into nmcd

**Future Considerations:**
- If btcd API changes, update composition points (not internals)
- If btcd adds features we need, use them via composition
- Contribute Namecoin-specific features to btcd if they benefit both projects

---

## 5. BREAKING CHANGES & MIGRATION PATH

### 5.1 Breaking Changes

**None.** This implementation is 100% additive.

**CLI Compatibility:**
```bash
# Old CLI (still works)
./nmcd -datadir=/tmp/test

# New CLI (identical behavior, different internal implementation)
./nmcd -datadir=/tmp/test
```

**RPC Compatibility:**
- All existing RPC methods preserved
- Request/response formats unchanged
- Authentication mechanisms unchanged

### 5.2 Migration Path for Daemon Users

**No migration required.** Daemon continues to work as before.

**Optional Migration:** Use library for programmatic access instead of RPC.

**Before (RPC client):**
```go
// External RPC client
resp, err := http.Post("http://localhost:8336", "application/json", body)
// ... parse JSON response
```

**After (library):**
```go
// Use library in daemon mode
client, err := nmcd.NewClient(&nmcd.Config{
    Mode:        nmcd.ModeDaemon,
    RPCAddr:     "http://localhost:8336",
    RPCUser:     "user",
    RPCPassword: "pass",
})
record, err := client.ResolveName(ctx, "d/example")
```

**Benefits:**
- Type-safe API (no manual JSON parsing)
- Automatic retries and error handling
- Context support for cancellation
- Consistent interface across embedded/daemon modes

### 5.3 Migration Path for Embedded Users

**New capability - no existing users to migrate.**

**Example:** Embed Namecoin in existing Go application.

```go
// In your application code
import "github.com/opd-ai/nmcd/client"

func main() {
    // Initialize embedded Namecoin client
    nc, err := nmcd.NewClient(&nmcd.Config{
        Mode:    nmcd.ModeEmbedded,
        DataDir: "/var/lib/myapp/namecoin",
        Network: "mainnet",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer nc.Close()
    
    // Use Namecoin for decentralized name resolution
    record, err := nc.ResolveName(ctx, "d/myapp")
    if err != nil {
        log.Printf("Name not found: %v", err)
        return
    }
    
    // Parse name value (typically JSON)
    var config AppConfig
    if err := json.Unmarshal([]byte(record.Value), &config); err != nil {
        log.Printf("Invalid name value: %v", err)
        return
    }
    
    // Use configuration from blockchain
    log.Printf("Connecting to: %s", config.ServerURL)
}
```

### 5.4 Deprecation Policy

**No deprecations.** All existing functionality is preserved.

**Future Deprecations (if any):**
1. Announce deprecation in release notes
2. Maintain for at least 2 major versions
3. Provide migration guide
4. Remove only after 6 months minimum

---

## 6. SUCCESS CRITERIA

### 6.1 Functional Requirements

**Must Have (MVP):**
- ✅ `NewClient()` with auto-detection works reliably
- ✅ EmbeddedClient can resolve names
- ✅ EmbeddedClient can register names (NAME_NEW + NAME_FIRSTUPDATE)
- ✅ EmbeddedClient can update names (NAME_UPDATE)
- ✅ DaemonClient works with nmcd daemon
- ✅ DaemonClient works with Namecoin Core daemon
- ✅ All examples run without errors
- ✅ Existing nmcd CLI functionality unchanged

**Should Have (Polish):**
- ✅ WaitForConfirmation works with context cancellation
- ✅ ListNames filtering (namespace, address, patterns)
- ✅ GetNameHistory returns full operation history
- ✅ Graceful shutdown releases all resources
- ✅ Retry logic with exponential backoff
- ✅ Custom logger support

**Nice to Have (Future):**
- ⏳ Headers-only sync mode (SPV)
- ⏳ Name-only sync mode (checkpoint trust)
- ⏳ Multi-signature name ownership
- ⏳ Bulk name operations

### 6.2 Non-Functional Requirements

**Performance:**
- ✅ Embedded client startup < 10 seconds (full sync mode)
- ✅ Name resolution < 100ms (from local DB)
- ✅ Name registration < 30 seconds (with 1 confirmation)
- ✅ List 1000 names < 500ms

**Reliability:**
- ✅ No data corruption on ungraceful shutdown
- ✅ Automatic recovery from network failures
- ✅ No resource leaks (memory, file handles, goroutines)
- ✅ Thread-safe for concurrent operations

**Maintainability:**
- ✅ Godoc coverage > 90% for public APIs
- ✅ Unit test coverage > 80% for client package
- ✅ Integration tests for both modes
- ✅ Examples demonstrate all major use cases

**Compatibility:**
- ✅ Works with Namecoin Core RPC (tested against v24.0)
- ✅ No breaking changes to existing nmcd CLI
- ✅ Backward compatible with existing RPC clients

### 6.3 Testing Strategy

**Unit Tests:**
```bash
# Test individual components
go test -v ./client -run TestEmbedded
go test -v ./client -run TestDaemon
go test -v ./client -run TestAutoDetection
```

**Integration Tests:**
```bash
# Test against real daemon
./nmcd -datadir=/tmp/daemon-test &
DAEMON_PID=$!
go test -v ./client -run TestIntegration
kill $DAEMON_PID
```

**Example Tests:**
```bash
# Verify examples run correctly
cd examples
for example in *.go; do
    echo "Testing $example"
    go run "$example" || exit 1
done
```

**Benchmark Tests:**
```bash
# Measure performance
go test -v ./client -bench=. -benchmem
```

**Expected Benchmark Results:**
```
BenchmarkResolveName-8         10000    100000 ns/op    1024 B/op    10 allocs/op
BenchmarkRegisterName-8         1000   5000000 ns/op   10240 B/op   100 allocs/op
BenchmarkListNames-8            2000    500000 ns/op   20480 B/op   200 allocs/op
```

---

## 7. RISKS AND MITIGATIONS

### Risk 1: Database Locking Conflicts

**Risk:** Embedded client and daemon try to open same database simultaneously.

**Impact:** HIGH - Database corruption, application crashes

**Probability:** MEDIUM (if users don't understand data directory separation)

**Mitigation:**
1. Document data directory requirements clearly
2. Implement file locking with descriptive error messages
3. Auto-detection mode avoids conflict by using daemon when present
4. Add warning in logs when lock acquisition fails

**Example Error Message:**
```
Error: Cannot start embedded client - data directory is locked by another process.
This usually means nmcd daemon is already running.

Solutions:
1. Use daemon mode: nmcd.NewClient(&nmcd.Config{Mode: nmcd.ModeDaemon})
2. Stop the daemon: killall nmcd
3. Use a different data directory: nmcd.NewClient(&nmcd.Config{DataDir: "/tmp/nmcd-embedded"})
```

### Risk 2: Blockchain Sync Time

**Risk:** Embedded client takes hours to sync, poor user experience.

**Impact:** MEDIUM - Users abandon embedded mode

**Probability:** HIGH (mainnet has 500k+ blocks)

**Mitigation:**
1. Default to headers-only sync (future enhancement)
2. Document sync time expectations clearly
3. Add progress reporting during sync
4. Provide pre-synced database downloads (optional)

**Code:**
```go
// Progress callback
type SyncProgress struct {
    CurrentHeight int32
    TargetHeight  int32
    Percentage    float64
    ETA           time.Duration
}

cfg := &nmcd.Config{
    OnSyncProgress: func(progress *SyncProgress) {
        log.Printf("Sync: %d/%d (%.1f%%) - ETA: %v",
            progress.CurrentHeight, progress.TargetHeight,
            progress.Percentage, progress.ETA)
    },
}
```

### Risk 3: Memory Usage in Embedded Mode

**Risk:** Embedded client uses too much memory for resource-constrained applications.

**Impact:** MEDIUM - Cannot use on low-memory devices

**Probability:** MEDIUM (blockchain data structures can be large)

**Mitigation:**
1. Implement database result pagination
2. Use streaming for large queries
3. Add memory limit configuration
4. Document memory requirements

**Example:**
```go
cfg := &nmcd.Config{
    MaxMemoryMB: 256, // Limit memory usage to 256 MB
}
```

### Risk 4: API Stability During Development

**Risk:** API changes during phase 2-4, breaks early adopters.

**Impact:** LOW - Limited early adopters

**Probability:** MEDIUM (typical for new APIs)

**Mitigation:**
1. Mark API as "beta" until phase 5 complete
2. Use semantic versioning (v0.x.y = unstable)
3. Provide migration guides for any breaking changes
4. Stabilize API in v1.0.0 release

**Versioning:**
```
v0.1.0 - Phase 1 complete (internal refactoring)
v0.2.0 - Phase 2 complete (EmbeddedClient beta)
v0.3.0 - Phase 3 complete (DaemonClient beta)
v0.4.0 - Phase 4 complete (Auto-detection beta)
v0.5.0 - Phase 5 complete (Documentation beta)
v1.0.0 - Stable release (API frozen)
```

### Risk 5: Dependency on btcd Library Changes

**Risk:** btcd introduces breaking changes, requires nmcd updates.

**Impact:** MEDIUM - Code changes required

**Probability:** LOW (btcd is mature and stable)

**Mitigation:**
1. Pin btcd versions in go.mod
2. Test against btcd release candidates
3. Maintain composition wrappers (not tight coupling)
4. Contribute to btcd discussions for advance notice

---

## 8. FUTURE ENHANCEMENTS

These features are out of scope for the initial implementation but valuable for future versions:

### 8.1 SPV (Simplified Payment Verification) Mode

**Goal:** Fast startup with SPV-level security

**Implementation:**
- Download block headers only
- Validate name transactions with merkle proofs
- Trust longest chain for name ordering

**Benefits:**
- Startup time: ~1 minute
- Disk usage: ~50 MB
- Security: Same as SPV wallets

**Effort:** Medium (2-3 weeks)

### 8.2 WebAssembly (WASM) Support

**Goal:** Run embedded client in browser

**Implementation:**
- Compile to WASM target
- Use IndexedDB for storage
- WebRTC for P2P networking

**Benefits:**
- Fully decentralized web apps
- No server required for name resolution
- Privacy-preserving (no RPC to external server)

**Effort:** High (4-6 weeks)

**Example:**
```javascript
// JavaScript usage
import { NewClient } from 'nmcd.wasm';

const client = NewClient({
    mode: 'embedded',
    network: 'mainnet'
});

const record = await client.ResolveName('d/example');
console.log(record.value);
```

### 8.3 GraphQL API

**Goal:** Modern API for web/mobile applications

**Implementation:**
- Add GraphQL schema on top of client package
- Support subscriptions for real-time updates
- Pagination, filtering, sorting built-in

**Benefits:**
- Flexible querying
- Real-time name updates
- Better than REST for complex queries

**Effort:** Medium (2-3 weeks)

**Example:**
```graphql
query {
  name(id: "d/example") {
    value
    owner
    expiresAt
    history {
      height
      value
      timestamp
    }
  }
}

subscription {
  nameUpdated(pattern: "d/*") {
    name
    value
    blockHeight
  }
}
```

### 8.4 Multi-Signature Name Ownership

**Goal:** Shared name ownership with threshold signatures

**Implementation:**
- Support P2SH addresses for name ownership
- Multi-sig wallet support
- Partial signature collection and merging

**Benefits:**
- Shared domain management
- Enhanced security (no single point of failure)
- DAO-owned names

**Effort:** High (4-6 weeks)

### 8.5 Name Marketplace Integration

**Goal:** Built-in support for buying/selling names

**Implementation:**
- Atomic name transfer (swap NAME_UPDATE for payment)
- Escrow contract support
- Marketplace discovery protocol

**Benefits:**
- Decentralized name trading
- No need for trusted third party
- Integrated into client library

**Effort:** High (6-8 weeks)

---

## 9. DEPENDENCIES AND PREREQUISITES

### 9.1 External Dependencies

**Required (already in go.mod):**
- `github.com/btcsuite/btcd` v0.25.0 - Bitcoin core libraries
- `github.com/btcsuite/btcd/btcutil` v1.1.5 - Bitcoin utilities
- `go.etcd.io/bbolt` v1.4.3 - Embedded database
- Go 1.24.11+ - Language runtime

**New Dependencies (to be added):**
None. Implementation uses only standard library and existing dependencies.

**Optional Dependencies (for examples):**
- `github.com/spf13/cobra` - CLI framework for example applications
- `github.com/stretchr/testify` - Testing utilities

### 9.2 Development Prerequisites

**Required:**
- Go 1.24.11 or later
- Git for version control
- Make for build automation

**Recommended:**
- `golangci-lint` for code quality
- `godoc` for documentation preview
- `go-test-coverage` for coverage reports

**Testing Prerequisites:**
- nmcd daemon for integration tests (or Namecoin Core)
- At least 5 GB disk space for full blockchain sync
- Network access for P2P connections (or mock peers)

### 9.3 Compatibility Matrix

| Component | nmcd Library | nmcd Daemon | Namecoin Core |
|-----------|-------------|-------------|---------------|
| Go API    | ✅ Native    | ✅ Via RPC   | ✅ Via RPC     |
| RPC API   | N/A         | ✅ v2.0      | ✅ v24.0+      |
| Database  | bbolt       | bbolt       | LevelDB       |
| Network   | btcd/peer   | btcd/peer   | Bitcoin P2P   |
| Sync Mode | Full/SPV*   | Full        | Full          |

*SPV mode planned for future enhancement

---

## 10. APPENDIX

### A. Code Structure After Implementation

```
nmcd/
├── client/                  # NEW: Public library API
│   ├── client.go           # NewClient factory, auto-detection
│   ├── types.go            # Public types (NameRecord, Config, etc.)
│   ├── embedded.go         # EmbeddedClient implementation
│   ├── embedded_test.go    # EmbeddedClient unit tests
│   ├── daemon.go           # DaemonClient implementation
│   ├── daemon_test.go      # DaemonClient unit tests
│   ├── integration_test.go # Integration tests
│   └── errors.go           # Error types
│
├── internal/               # NEW: Internal packages (not exported)
│   └── server/            
│       ├── server.go       # Daemon server implementation
│       └── server_test.go  # Server unit tests
│
├── cmd/nmcd/              # MODIFIED: Uses library
│   ├── main.go            # CLI entry point (simplified)
│   └── main_test.go       # CLI tests
│
├── examples/              # MODIFIED: Library usage examples
│   ├── simple_resolve.go  # Basic name resolution
│   ├── register_name.go   # Name registration flow
│   ├── update_name.go     # Name update flow
│   ├── list_names.go      # Name listing
│   ├── dns_bridge.go      # DNS protocol bridge
│   ├── tor_client.go      # NEW: Tor anonymous client
│   ├── i2p_client.go      # NEW: I2P anonymous client
│   └── namedb_example.go  # Existing database example
│
├── namedb/                # UNCHANGED: Existing package
├── chain/                 # UNCHANGED: Existing package
├── network/               # UNCHANGED: Existing package
├── rpc/                   # UNCHANGED: Existing package
├── wallet/                # UNCHANGED: Existing package
├── config/                # UNCHANGED: Existing package
│
├── docs/                  # NEW: Documentation
│   ├── API.md
│   ├── EMBEDDING.md
│   ├── MODES.md
│   ├── ANONYMOUS_NETWORKS.md  # NEW: Tor/I2P configuration guide
│   └── PERFORMANCE.md
│
├── README.md              # MODIFIED: Library-first documentation
├── PLAN.md                # THIS FILE
└── go.mod                 # UNCHANGED: No new dependencies
```

### B. Example: Complete Application

**File: `examples/domain_monitor/main.go`**

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/opd-ai/nmcd/client"
)

// DomainMonitor watches Namecoin domains and alerts on expiration
type DomainMonitor struct {
    client       client.NameClient
    domains      []string
    checkInterval time.Duration
}

func NewMonitor(domains []string) (*DomainMonitor, error) {
    // Initialize embedded Namecoin client
    nc, err := client.NewClient(&client.Config{
        Mode:    client.ModeAuto, // Auto-detect daemon or use embedded
        Network: "mainnet",
    })
    if err != nil {
        return nil, fmt.Errorf("failed to create client: %w", err)
    }

    return &DomainMonitor{
        client:       nc,
        domains:      domains,
        checkInterval: 1 * time.Hour,
    }, nil
}

func (m *DomainMonitor) Start(ctx context.Context) error {
    ticker := time.NewTicker(m.checkInterval)
    defer ticker.Stop()

    // Check immediately on start
    m.checkDomains(ctx)

    for {
        select {
        case <-ticker.C:
            m.checkDomains(ctx)
        case <-ctx.Done():
            return m.client.Close()
        }
    }
}

func (m *DomainMonitor) checkDomains(ctx context.Context) {
    for _, domain := range m.domains {
        record, err := m.client.ResolveName(ctx, domain)
        if err == client.ErrNameNotFound {
            log.Printf("⚠️  Domain not registered: %s", domain)
            continue
        } else if err != nil {
            log.Printf("❌ Error checking %s: %v", domain, err)
            continue
        }

        // Alert if expiring soon (< 1000 blocks = ~7 days)
        if record.ExpiresIn < 1000 {
            log.Printf("🚨 EXPIRING SOON: %s in %d blocks (~%d days)",
                domain, record.ExpiresIn, record.ExpiresIn/144)
            
            // Auto-renew if we own it
            if err := m.renewDomain(ctx, domain, record); err != nil {
                log.Printf("❌ Failed to renew %s: %v", domain, err)
            }
        } else {
            log.Printf("✅ %s: OK (%d blocks remaining)", domain, record.ExpiresIn)
        }
    }
}

func (m *DomainMonitor) renewDomain(ctx context.Context, name string, record *client.NameRecord) error {
    log.Printf("🔄 Renewing %s...", name)
    
    result, err := m.client.UpdateName(ctx, name, record.Value, &client.UpdateOpts{
        WaitForConfirmation: true,
        Confirmations:       6,
    })
    if err != nil {
        return err
    }

    log.Printf("✅ Renewed %s: TX %s", name, result.TxHash)
    return nil
}

func main() {
    domains := []string{
        "d/example",
        "d/mycompany",
        "id/alice",
    }

    monitor, err := NewMonitor(domains)
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()
    if err := monitor.Start(ctx); err != nil {
        log.Fatal(err)
    }
}
```

**Usage:**
```bash
go run examples/domain_monitor/main.go

# Output:
# ✅ d/example: OK (25000 blocks remaining)
# 🚨 EXPIRING SOON: d/mycompany in 500 blocks (~3 days)
# 🔄 Renewing d/mycompany...
# ✅ Renewed d/mycompany: TX abc123...
# ✅ id/alice: OK (30000 blocks remaining)
```

### C. Glossary

**Terms used in this document:**

- **Embedded Mode:** In-process Namecoin client with local database and P2P networking
- **Daemon Mode:** RPC client connecting to external nmcd or Namecoin Core daemon
- **NAME_NEW:** First step of name registration; creates commitment hash
- **NAME_FIRSTUPDATE:** Second step of name registration; reveals name and sets initial value
- **NAME_UPDATE:** Updates existing name value; extends expiration
- **SPV:** Simplified Payment Verification; lightweight blockchain validation
- **bbolt:** Embedded key-value database (formerly BoltDB)
- **btcd:** Go implementation of Bitcoin protocol; nmcd's primary dependency
- **UTXO:** Unspent Transaction Output; fundamental Bitcoin/Namecoin concept
- **P2P:** Peer-to-Peer networking protocol
- **RPC:** Remote Procedure Call; JSON-RPC in Namecoin's case
- **Namespace:** Name prefix indicating data type (d/=domain, id/=identity, p/=personal)

---

## SIGN-OFF

This plan has been created to guide the implementation of the embedded Namecoin library API for nmcd. It represents a comprehensive analysis of requirements, architecture, and implementation strategy.

**Next Steps:**
1. Review plan with stakeholders
2. Obtain approval for implementation phases
3. Begin Phase 1: Extract Reusable Components
4. Track progress via GitHub issues linked to each phase

**Estimated Timeline:**
- Phase 1: Week 1 (Extract components)
- Phase 2: Week 2-3 (EmbeddedClient)
- Phase 3: Week 3-4 (DaemonClient)
- Phase 4: Week 4-5 (Auto-detection & integration)
- Phase 5: Week 5-6 (Documentation & examples)
- **Total:** 6 weeks to MVP

**Success Metrics:**
- ✅ All tests pass
- ✅ Examples demonstrate real-world usage
- ✅ API documentation complete
- ✅ No breaking changes to existing CLI
- ✅ Performance meets requirements

---

**Document Version:** 1.0  
**Last Updated:** 2026-01-02  
**Status:** Ready for Review
