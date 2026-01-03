# nmcd Examples Guide

This guide provides detailed walkthroughs of the example applications included with nmcd. Each example demonstrates specific features of the library and follows Go best practices.

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [Example Applications](#example-applications)
   - [Simple Resolve](#1-simple-resolve)
   - [Embedded Client](#2-embedded-client)
   - [Register Name](#3-register-name)
   - [Update Name](#4-update-name)
   - [List Names](#5-list-names)
   - [Name Database](#6-name-database)
3. [Common Patterns](#common-patterns)
4. [Error Handling](#error-handling)
5. [Best Practices](#best-practices)
6. [Troubleshooting](#troubleshooting)

---

## Quick Start

All examples are located in the `examples/` directory and can be run directly with `go run`:

```bash
# Clone the repository
git clone https://github.com/opd-ai/nmcd.git
cd nmcd

# Run an example
go run ./examples/simple_resolve d/example

# Or navigate to the example directory
cd examples/simple_resolve
go run main.go d/example
```

**Prerequisites:**
- Go 1.24.11 or later
- No external dependencies required (nmcd uses only standard library and btcd)

---

## Example Applications

### 1. Simple Resolve

**Location:** `examples/simple_resolve/`

**Purpose:** Demonstrates basic name resolution using automatic mode detection.

**What You'll Learn:**
- Creating a client with auto-detection (daemon or embedded mode)
- Resolving Namecoin names
- Handling common errors (not found, expired)
- Displaying name record metadata

**Usage:**

```bash
go run ./examples/simple_resolve d/example
go run ./examples/simple_resolve id/alice
```

**Key Code Walkthrough:**

```go
// 1. Configure the client with auto-detection
cfg := &client.Config{
    Mode:    client.ModeAuto,    // Auto-detect daemon, fallback to embedded
    Network: "regtest",          // Use regtest for local testing
    DataDir: os.TempDir() + "/nmcd-simple-resolve",
}

// 2. Create the client
nc, err := client.NewClient(cfg)
if err != nil {
    log.Fatalf("Failed to create client: %v", err)
}
defer nc.Close()  // Always close to release resources

// 3. Resolve a name
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

record, err := nc.ResolveName(ctx, name)
```

**Error Handling Pattern:**

```go
switch {
case errors.Is(err, client.ErrNameNotFound):
    fmt.Printf("✗ Name not found: %s\n", name)
    
case errors.Is(err, client.ErrNameExpired):
    fmt.Printf("✗ Name expired: %s\n", name)
    
case err != nil:
    log.Fatalf("Failed to resolve name: %v", err)
    
default:
    // Success - display record
    fmt.Printf("Value: %s\nOwner: %s\n", record.Value, record.Address)
}
```

**Expected Output:**

```
Initializing Namecoin client...
Connected to regtest network in embedded mode (height: 0)

Resolving name: d/example
✓ Name resolved successfully!

Name Record:
  Name:       d/example
  Value:      {"ip":"1.2.3.4"}
  Owner:      N1abc...
  TX Hash:    abc123...
  Height:     100
  Expires At: 36100
  Expires In: 36000 blocks (~250.0 days)
  Updated:    2026-01-03T12:00:00Z
```

---

### 2. Embedded Client

**Location:** `examples/embedded_client/`

**Purpose:** Demonstrates using the embedded client explicitly (no daemon detection).

**What You'll Learn:**
- Creating an in-process embedded client
- Understanding embedded vs daemon mode differences
- Working with local blockchain data
- Proper resource management

**Usage:**

```bash
# Use default temp directory
go run ./examples/embedded_client

# Specify custom data directory
go run ./examples/embedded_client /path/to/data
```

**Key Code Walkthrough:**

```go
// 1. Force embedded mode (no daemon detection)
cfg := &client.Config{
    Mode:    client.ModeEmbedded,  // Explicitly use embedded mode
    DataDir: dataDir,
    Network: "regtest",
}

// 2. Create embedded client directly
nc, err := client.NewEmbeddedClient(cfg)
if err != nil {
    log.Fatalf("Failed to create client: %v", err)
}
defer nc.Close()

// 3. Get node information
ctx := context.Background()
info, err := nc.GetInfo(ctx)
if err != nil {
    log.Fatalf("Failed to get node info: %v", err)
}

// 4. Display node state
fmt.Printf("Version: %s\n", info.Version)
fmt.Printf("Network: %s\n", info.NetworkName)
fmt.Printf("Mode: %s\n", info.Mode)         // Will show "embedded"
fmt.Printf("Block Height: %d\n", info.BlockHeight)
```

**When to Use Embedded Mode:**
- ✅ Embedding nmcd in your application
- ✅ Running multiple isolated instances
- ✅ Testing without external dependencies
- ✅ Offline name resolution from local database
- ❌ Connecting to existing nmcd/Namecoin Core daemon
- ❌ Production deployment (use daemon mode for shared resources)

**Resource Management:**

The embedded client manages several resources that are automatically cleaned up on `Close()`:
- Name database (bbolt)
- Block database (ffldb)
- Wallet file
- Network connections (if network enabled)

Always use `defer nc.Close()` immediately after successful client creation.

---

### 3. Register Name

**Location:** `examples/register_name/`

**Purpose:** Demonstrates the two-step name registration process.

**What You'll Learn:**
- NAME_NEW commitment creation (prevents front-running)
- NAME_FIRSTUPDATE registration (reveals name and sets value)
- JSON value validation for namespaces
- Wallet integration for transaction signing

**Usage:**

```bash
go run ./examples/register_name d/mysite '{"ip":"1.2.3.4"}'
go run ./examples/register_name id/alice '{"name":"Alice","email":"alice@example.com"}'
```

**Key Code Walkthrough:**

```go
// 1. Validate name format
if !strings.HasPrefix(name, "d/") && 
   !strings.HasPrefix(name, "id/") && 
   !strings.HasPrefix(name, "p/") {
    fmt.Println("Error: Name must start with d/, id/, or p/ namespace")
    os.Exit(1)
}

// 2. Validate JSON for d/ and id/ namespaces
if strings.HasPrefix(name, "d/") || strings.HasPrefix(name, "id/") {
    var js json.RawMessage
    if err := json.Unmarshal([]byte(value), &js); err != nil {
        fmt.Printf("Invalid JSON: %v\n", err)
        os.Exit(1)
    }
}

// 3. Create embedded client with wallet enabled
cfg := &client.Config{
    Mode:          client.ModeEmbedded,
    Network:       "regtest",
    DataDir:       os.TempDir() + "/nmcd-register-example",
    DisableWallet: false,  // Wallet required for registration
}

// 4. Check name availability
existingRecord, err := nc.ResolveName(ctx, name)
if err == nil {
    fmt.Printf("✗ Name already registered!\n")
    os.Exit(1)
}

// 5. Register the name
opts := &client.RegisterOpts{
    WaitForConfirmation: false,  // Return immediately after NAME_NEW
    FeeRate:             1,      // 1 satoshi per byte
}

result, err := nc.RegisterName(ctx, name, value, opts)
```

**Registration Process:**

```
Step 1: NAME_NEW (Commitment Phase)
├─ Create commitment: Hash(name || random salt || chain ID)
├─ Submit commitment transaction to blockchain
├─ Wait 12 blocks (prevents front-running)
└─ Returns: TxHash of NAME_NEW transaction

Step 2: NAME_FIRSTUPDATE (Reveal Phase)
├─ Reveal name and salt from commitment
├─ Set initial value
├─ Establish ownership address
└─ Returns: TxHash of NAME_FIRSTUPDATE transaction
```

**Expected Output:**

```
Initializing Namecoin client...
Connected to regtest network (height: 0)

Checking if name 'd/mysite' is available...
✓ Name 'd/mysite' is available

Registering name...
  Name:  d/mysite
  Value: {"ip":"1.2.3.4"}

✓ Registration initiated!

Transaction Result:
  TX Hash:  abc123...
  Name:     d/mysite
  Status:   pending

Note: NAME_NEW transaction created.
After 12 blocks, NAME_FIRSTUPDATE will complete the registration.
Use WaitForConfirmation: true to wait for full registration.
```

**Synchronous Registration (Wait for Completion):**

```go
opts := &client.RegisterOpts{
    WaitForConfirmation: true,   // Block until NAME_FIRSTUPDATE is confirmed
    Confirmations:       1,      // Wait for 1 confirmation
    FeeRate:            1,
}

result, err := nc.RegisterName(ctx, name, value, opts)
// result.Status will be TxStatusConfirmed when returned
```

---

### 4. Update Name

**Location:** `examples/update_name/`

**Purpose:** Demonstrates updating an existing name's value.

**What You'll Learn:**
- NAME_UPDATE transaction creation
- Ownership verification (must have private key)
- Expiration extension (adds 36,000 blocks)
- Value updates while preserving ownership

**Usage:**

```bash
go run ./examples/update_name d/mysite '{"ip":"5.6.7.8"}'
go run ./examples/update_name id/alice '{"name":"Alice","verified":true}'
```

**Key Code Walkthrough:**

```go
// 1. Validate new value (same as registration)
if strings.HasPrefix(name, "d/") || strings.HasPrefix(name, "id/") {
    var js json.RawMessage
    if err := json.Unmarshal([]byte(newValue), &js); err != nil {
        fmt.Printf("Invalid JSON: %v\n", err)
        os.Exit(1)
    }
}

// 2. Get current name record
record, err := nc.ResolveName(ctx, name)
if errors.Is(err, client.ErrNameNotFound) {
    fmt.Printf("✗ Name not found: %s\n", name)
    os.Exit(1)
}

// 3. Display current state
fmt.Printf("Current Value: %s\n", record.Value)
fmt.Printf("Owner: %s\n", record.Address)
fmt.Printf("Expires In: %d blocks\n", record.ExpiresIn)

// 4. Update the name
opts := &client.UpdateOpts{
    WaitForConfirmation: false,  // Return immediately
    FeeRate:             1,
}

result, err := nc.UpdateName(ctx, name, newValue, opts)
```

**Update Effects:**

When a NAME_UPDATE is confirmed:
1. ✅ Name value is updated to new value
2. ✅ Expiration extended by 36,000 blocks (~250 days)
3. ✅ Block height updated to update transaction height
4. ❌ Ownership unchanged (stays at current address)

**Transfer with Update (Advanced):**

```go
opts := &client.UpdateOpts{
    TransferTo:          "N2xyz...",  // New owner address
    WaitForConfirmation: true,
    Confirmations:       1,
}

result, err := nc.UpdateName(ctx, name, newValue, opts)
// Name now owned by N2xyz...
```

**Expected Output:**

```
Initializing Namecoin client...
Connected to regtest network (height: 100)

Looking up current record for 'd/mysite'...
✓ Name found

Current Record:
  Value:      {"ip":"1.2.3.4"}
  Owner:      N1abc...
  Expires In: 35900 blocks (~249.3 days)

Updating name...
  New Value: {"ip":"5.6.7.8"}

✓ Update transaction created!

Transaction Result:
  TX Hash:  def456...
  Name:     d/mysite
  Status:   pending

Note: Transaction is pending. Once confirmed:
  - The name's value will be updated
  - The expiration will be extended by 36,000 blocks (~250 days)
```

---

### 5. List Names

**Location:** `examples/list_names/`

**Purpose:** Demonstrates filtering and pagination for name listings.

**What You'll Learn:**
- Filtering by namespace (d/, id/, p/)
- Filtering by owner address
- Pattern matching (prefix-based)
- Pagination with limit and offset
- Including/excluding expired names

**Usage:**

```bash
# List all names (default: 100 limit, active only)
go run ./examples/list_names

# Filter by namespace
go run ./examples/list_names --namespace=d/

# Filter by owner
go run ./examples/list_names --address=N1abc...

# Pattern matching with limit
go run ./examples/list_names --pattern=d/example --limit=10

# Include expired names
go run ./examples/list_names --include-expired

# Pagination
go run ./examples/list_names --limit=50 --offset=100
```

**Key Code Walkthrough:**

```go
// 1. Parse command-line flags
namespace := flag.String("namespace", "", "Filter by namespace (d/, id/, p/)")
address := flag.String("address", "", "Filter by owner address")
pattern := flag.String("pattern", "", "Filter by name prefix pattern")
includeExpired := flag.Bool("include-expired", false, "Include expired names")
limit := flag.Int("limit", 100, "Maximum number of results")
offset := flag.Int("offset", 0, "Number of results to skip")
flag.Parse()

// 2. Build filter
filter := &client.ListFilter{
    Namespace:      *namespace,
    Address:        *address,
    NamePattern:    *pattern,
    IncludeExpired: *includeExpired,
    Limit:          *limit,
    Offset:         *offset,
}

// 3. List names
records, err := nc.ListNames(ctx, filter)

// 4. Display results
for _, record := range records {
    fmt.Printf("%-30s %-40s %-15s %s\n", 
        record.Name, 
        truncate(record.Value, 40),
        formatExpiration(record.ExpiresIn),
        truncate(record.Address, 12))
}
```

**Filter Combinations:**

```go
// All d/ names owned by a specific address
filter := &client.ListFilter{
    Namespace: "d/",
    Address:   "N1abc...",
    Limit:     100,
}

// All names starting with "d/example" including expired
filter := &client.ListFilter{
    NamePattern:    "d/example",
    IncludeExpired: true,
    Limit:          1000,
}

// Second page of id/ namespace (pagination)
filter := &client.ListFilter{
    Namespace: "id/",
    Limit:     50,
    Offset:    50,  // Skip first 50 results
}
```

**Expected Output:**

```
Initializing Namecoin client...
Connected to regtest network in embedded mode (height: 100)

Filter Settings:
  Namespace:       d/
  Include Expired: false
  Limit:           100

Listing names...
Found 3 name(s):

NAME                           VALUE (truncated)                        EXPIRES IN      OWNER
----                           -----                                    ----------      -----
d/example                      {"ip":"1.2.3.4"}                         36000 blocks    N1abc...
d/mysite                       {"ip":"5.6.7.8"}                         35900 blocks    N2def...
d/test                         {"data":"test"}                          35800 blocks    N3ghi...
```

---

### 6. Name Database

**Location:** `examples/namedb/`

**Purpose:** Demonstrates low-level name database operations.

**What You'll Learn:**
- Direct database access (without blockchain)
- Storing and retrieving name records
- Expiration checking
- History management

**Usage:**

```bash
go run ./examples/namedb
```

**Key Code Walkthrough:**

```go
// 1. Open name database directly
db, err := namedb.NewNameDatabase("/tmp/example-names.db")
if err != nil {
    log.Fatal(err)
}
defer db.Close()

// 2. Create a name record
record := &namedb.NameRecord{
    Name:      "d/example",
    Value:     `{"ip":"1.2.3.4"}`,
    TxHash:    "abc123...",
    Height:    100,
    ExpiresAt: 36100,
    Address:   "N1abc...",
}

// 3. Store the record
err = db.PutName("d/example", record)

// 4. Retrieve the record
retrieved, err := db.GetName("d/example")
fmt.Printf("Name: %s\nValue: %s\n", retrieved.Name, retrieved.Value)

// 5. Check expiration
currentHeight := int32(200)
isExpired := retrieved.ExpiresAt <= currentHeight
```

**When to Use Direct Database Access:**

- ✅ Testing name database functionality
- ✅ Offline name resolution from pre-populated database
- ✅ Database migration or import tools
- ✅ Analytics and reporting on name data
- ❌ Production applications (use client library instead)
- ❌ Name registration/updates (requires blockchain)

---

## Common Patterns

### Pattern 1: Context with Timeout

Always use contexts with timeouts for network operations:

```go
// Set timeout appropriate for the operation
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

record, err := client.ResolveName(ctx, name)
if ctx.Err() == context.DeadlineExceeded {
    fmt.Println("Operation timed out")
}
```

### Pattern 2: Client Configuration

**Auto-Detection (Recommended for Applications):**

```go
cfg := &client.Config{
    Mode:    client.ModeAuto,  // Try daemon, fallback to embedded
    Network: "mainnet",
}
nc, err := client.NewClient(cfg)
```

**Explicit Embedded (Libraries and Services):**

```go
cfg := &client.Config{
    Mode:    client.ModeEmbedded,
    DataDir: "/var/lib/myapp/nmcd",
    Network: "mainnet",
}
nc, err := client.NewEmbeddedClient(cfg)
```

**Explicit Daemon (Shared Infrastructure):**

```go
cfg := &client.Config{
    Mode:        client.ModeDaemon,
    RPCAddr:     "http://localhost:8336",
    RPCUser:     "user",
    RPCPassword: "pass",
}
nc, err := client.NewDaemonClient(cfg)
```

### Pattern 3: Error Handling

Use typed errors for common cases:

```go
record, err := nc.ResolveName(ctx, name)

switch {
case errors.Is(err, client.ErrNameNotFound):
    // Name doesn't exist - handle gracefully
    
case errors.Is(err, client.ErrNameExpired):
    // Name expired - offer re-registration
    
case errors.Is(err, client.ErrNoWallet):
    // Wallet required for operation
    
case errors.Is(err, client.ErrInsufficientFunds):
    // Not enough NMC for transaction
    
case err != nil:
    // Unexpected error - log and fail
    log.Fatalf("Unexpected error: %v", err)
}
```

### Pattern 4: Graceful Shutdown

Handle cleanup properly in long-running applications:

```go
func main() {
    nc, err := client.NewClient(nil)
    if err != nil {
        log.Fatal(err)
    }
    
    // Setup signal handling
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    
    // Run application
    go runApplication(nc)
    
    // Wait for shutdown signal
    <-sigChan
    fmt.Println("Shutting down...")
    
    // Close client (releases all resources)
    if err := nc.Close(); err != nil {
        log.Printf("Error closing client: %v", err)
    }
}
```

### Pattern 5: Concurrent Operations

All client methods are thread-safe:

```go
var wg sync.WaitGroup
names := []string{"d/example1", "d/example2", "d/example3"}

for _, name := range names {
    wg.Add(1)
    go func(n string) {
        defer wg.Done()
        
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()
        
        record, err := nc.ResolveName(ctx, n)
        if err != nil {
            log.Printf("Failed to resolve %s: %v", n, err)
            return
        }
        fmt.Printf("%s = %s\n", n, record.Value)
    }(name)
}

wg.Wait()
```

---

## Error Handling

### Common Errors

#### ErrNameNotFound

**Meaning:** Name doesn't exist in the blockchain or local database.

**Handling:**

```go
if errors.Is(err, client.ErrNameNotFound) {
    // For read operations: return default or error
    return "", fmt.Errorf("name %s not found", name)
    
    // For UI: offer registration
    fmt.Printf("Name available! Register at: ...")
}
```

#### ErrNameExpired

**Meaning:** Name exists but has expired (> 36,000 blocks since last update).

**Handling:**

```go
if errors.Is(err, client.ErrNameExpired) {
    // Name is available for re-registration
    fmt.Printf("Name expired. You can re-register it.\n")
}
```

#### ErrInsufficientFunds

**Meaning:** Wallet doesn't have enough NMC for transaction fees.

**Handling:**

```go
if errors.Is(err, client.ErrInsufficientFunds) {
    // Show wallet balance and required fee
    fmt.Printf("Insufficient funds. Need at least %d satoshis.\n", requiredFee)
}
```

#### Context Errors

**Timeout:**

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

_, err := nc.ResolveName(ctx, name)
if ctx.Err() == context.DeadlineExceeded {
    fmt.Println("Operation timed out")
}
```

**Cancellation:**

```go
ctx, cancel := context.WithCancel(context.Background())

// Cancel from another goroutine
go func() {
    time.Sleep(2 * time.Second)
    cancel()
}()

_, err := nc.RegisterName(ctx, name, value, nil)
if ctx.Err() == context.Canceled {
    fmt.Println("Operation canceled")
}
```

---

## Best Practices

### 1. Always Close Clients

```go
nc, err := client.NewClient(cfg)
if err != nil {
    log.Fatal(err)
}
defer nc.Close()  // ✅ Always defer immediately after creation
```

### 2. Use Appropriate Timeouts

```go
// Quick lookups: 10-30 seconds
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
record, err := nc.ResolveName(ctx, name)

// Registration/updates: 1-5 minutes
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()
result, err := nc.RegisterName(ctx, name, value, opts)
```

### 3. Validate Input Early

```go
// Validate name format
if len(name) == 0 || len(name) > 255 {
    return fmt.Errorf("invalid name length: %d", len(name))
}

// Validate value size
if len(value) > 1023 {
    return fmt.Errorf("value too large: %d bytes (max 1023)", len(value))
}

// Validate JSON for d/ and id/ namespaces
if strings.HasPrefix(name, "d/") || strings.HasPrefix(name, "id/") {
    if !json.Valid([]byte(value)) {
        return fmt.Errorf("invalid JSON value")
    }
}
```

### 4. Handle Expiration Proactively

```go
record, err := nc.ResolveName(ctx, name)
if err != nil {
    return err
}

// Warn if expiring soon (< 1000 blocks ≈ 7 days)
if record.ExpiresIn < 1000 {
    log.Printf("WARNING: %s expires in %d blocks", name, record.ExpiresIn)
    
    // Auto-renew by updating with same value
    _, err := nc.UpdateName(ctx, name, record.Value, nil)
    if err != nil {
        log.Printf("Failed to renew: %v", err)
    }
}
```

### 5. Use Structured Logging

```go
import "log/slog"

logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

record, err := nc.ResolveName(ctx, name)
if err != nil {
    logger.Error("name resolution failed",
        "name", name,
        "error", err,
    )
    return err
}

logger.Info("name resolved",
    "name", record.Name,
    "owner", record.Address,
    "height", record.Height,
)
```

---

## Troubleshooting

### Database Locked Error

**Problem:**

```
failed to open database: timeout
```

**Cause:** Another process has the database open (bbolt allows only one writer).

**Solutions:**

1. Check for running nmcd processes:
   ```bash
   ps aux | grep nmcd
   killall nmcd
   ```

2. Use different data directories:
   ```go
   cfg := &client.Config{
       DataDir: "/tmp/myapp-nmcd",  // Separate from daemon
   }
   ```

3. Use daemon mode instead:
   ```go
   cfg := &client.Config{
       Mode: client.ModeDaemon,  // Connect to existing daemon
   }
   ```

### Name Not Found (Expected to Exist)

**Problem:** `ResolveName` returns `ErrNameNotFound` for a name you expect to exist.

**Causes & Solutions:**

1. **Blockchain not synced:**
   ```go
   info, _ := nc.GetInfo(ctx)
   fmt.Printf("Current height: %d\n", info.BlockHeight)
   // If height is low, blockchain is still syncing
   ```

2. **Wrong network:**
   ```go
   cfg := &client.Config{
       Network: "mainnet",  // Make sure this matches your expectation
   }
   ```

3. **Name expired:**
   ```go
   // Try including expired names
   records, _ := nc.ListNames(ctx, &client.ListFilter{
       NamePattern:    name,
       IncludeExpired: true,
   })
   ```

### Transaction Not Confirming

**Problem:** `WaitForConfirmation` times out or transaction stays pending.

**Causes & Solutions:**

1. **Insufficient fee:**
   ```go
   opts := &client.RegisterOpts{
       FeeRate: 10,  // Increase from 1 to 10 satoshis/byte
   }
   ```

2. **Network not connected:**
   ```go
   info, _ := nc.GetInfo(ctx)
   if info.Connections == 0 {
       fmt.Println("No network connections - transactions won't propagate")
   }
   ```

3. **Regtest mode (no automatic mining):**
   ```go
   // In regtest, you must manually mine blocks for confirmations
   // Use daemon mode and mine blocks with namecoin-cli or btcd
   ```

### Memory Usage Growing

**Problem:** Application memory usage increases over time.

**Causes & Solutions:**

1. **Not closing clients:**
   ```go
   // ❌ Bad: creating clients in a loop without closing
   for {
       nc, _ := client.NewClient(nil)
       // ... use nc ...
       // Missing: nc.Close()
   }
   
   // ✅ Good: reuse single client
   nc, _ := client.NewClient(nil)
   defer nc.Close()
   for {
       // ... use nc ...
   }
   ```

2. **Large result sets without pagination:**
   ```go
   // ❌ Bad: fetching all names at once
   all, _ := nc.ListNames(ctx, &client.ListFilter{
       Limit: 10000,
   })
   
   // ✅ Good: paginate results
   offset := 0
   for {
       batch, _ := nc.ListNames(ctx, &client.ListFilter{
           Limit:  100,
           Offset: offset,
       })
       // Process batch...
       if len(batch) < 100 {
           break
       }
       offset += 100
   }
   ```

---

## Additional Resources

- **API Reference:** See [API.md](API.md) for complete API documentation
- **Embedding Guide:** See [EMBEDDING.md](EMBEDDING.md) for integration patterns
- **Mode Comparison:** See [MODES.md](MODES.md) for embedded vs daemon tradeoffs
- **Performance Guide:** See [PERFORMANCE.md](PERFORMANCE.md) for optimization tips
- **Source Code:** Browse [examples/](../examples/) for complete, runnable code

---

## Contributing Examples

Have a useful example to share? We welcome contributions!

**Guidelines:**
1. Example should demonstrate a single concept clearly
2. Include comprehensive error handling
3. Add comments explaining key decisions
4. Test on all supported networks (mainnet, testnet, regtest)
5. Update this guide with your example walkthrough

**Submitting:**
1. Create example in `examples/<example-name>/`
2. Add `main.go` with package documentation
3. Update `examples/README.md` with quick description
4. Update this guide with detailed walkthrough
5. Submit pull request with clear description

---

## License

All examples are provided under the same license as nmcd. See [LICENSE](../LICENSE) for details.
