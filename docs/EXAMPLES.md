# nmcd Examples Guide

Walkthroughs of example applications demonstrating nmcd library features and Go best practices.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Example Applications](#example-applications)
3. [Common Patterns](#common-patterns)
4. [Error Handling](#error-handling)
5. [Best Practices](#best-practices)
6. [Troubleshooting](#troubleshooting)

---

## Quick Start

Run examples from `examples/` directory:

```bash
git clone https://github.com/opd-ai/nmcd.git
cd nmcd
go run ./examples/simple_resolve d/example
```

**Prerequisites:** Go 1.24.11+

---

## Example Applications

### 1. Simple Resolve

**Location:** `examples/simple_resolve/`

Demonstrates basic name resolution with auto-detection.

**Usage:**
```bash
go run ./examples/simple_resolve d/example
```

**Key code:**
```go
cfg := &client.Config{
    Mode:    client.ModeAuto,
    Network: "regtest",
    DataDir: os.TempDir() + "/nmcd-simple",
}

nc, err := client.NewClient(cfg)
if err != nil {
    log.Fatalf("Failed to create client: %v", err)
}
defer nc.Close()

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

record, err := nc.ResolveName(ctx, name)
if err != nil {
    if errors.Is(err, client.ErrNameNotFound) {
        fmt.Printf("Name not found: %s\n", name)
        return
    }
    log.Fatal(err)
}

fmt.Printf("Name: %s\nValue: %s\nHeight: %d\nExpires: %d\n",
    record.Name, record.Value, record.Height, record.ExpiresAt)
```

---

### 2. Embedded Client

**Location:** `examples/embedded_client/`

Shows embedded mode without external daemon.

**Usage:**
```bash
go run ./examples/embedded_client
```

**Key features:**
- Embedded blockchain and name database
- No external dependencies
- Automatic data directory creation
- Context cancellation support

```go
cfg := &client.Config{
    Mode:        client.ModeEmbedded,
    Network:     "regtest",
    DataDir:     "/tmp/nmcd-embedded",
    WalletEnabled: true,
}

nc, err := client.NewClient(cfg)
defer nc.Close()

// Resolve names
record, err := nc.ResolveName(ctx, "d/example")

// Get node info
info, err := nc.GetInfo(ctx)
fmt.Printf("Block height: %d\n", info.Blocks)
```

---

### 3. Register Name

**Location:** `examples/register_name/`

Demonstrates two-phase name registration (NAME_NEW → NAME_FIRSTUPDATE).

**Usage:**
```bash
go run ./examples/register_name d/myname '{"ip":"1.2.3.4"}'
```

**Registration flow:**
```go
// Phase 1: Pre-register (NAME_NEW)
salt := make([]byte, 20)
rand.Read(salt)

commitTxHash, err := nc.RegisterNameCommitment(ctx, name, salt)
fmt.Printf("Commitment TX: %s\n", commitTxHash)

// Wait 12 blocks minimum
time.Sleep(12 * 10 * time.Minute)

// Phase 2: Reveal and register (NAME_FIRSTUPDATE)
txHash, err := nc.RegisterName(ctx, name, value, salt)
fmt.Printf("Registration TX: %s\n", txHash)
```

---

### 4. Update Name

**Location:** `examples/update_name/`

Shows updating existing name values.

**Usage:**
```bash
go run ./examples/update_name d/myname '{"ip":"5.6.7.8"}'
```

**Key code:**
```go
txHash, err := nc.UpdateName(ctx, name, newValue)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Update TX: %s\n", txHash)
fmt.Printf("Expiration extended +36,000 blocks\n")
```

---

### 5. List Names

**Location:** `examples/list_names/`

Lists all registered names with filtering.

**Usage:**
```bash
go run ./examples/list_names
go run ./examples/list_names --namespace d/
```

**Key code:**
```go
names, err := nc.ListNames(ctx, &client.NameFilter{
    Namespace: "d/",
    Limit:     100,
})

for _, name := range names {
    fmt.Printf("%s: %s (expires: %d)\n",
        name.Name, name.Value, name.ExpiresAt)
}
```

---

### 6. Name Database

**Location:** `examples/name_database/`

Direct database interaction for advanced use cases.

**Usage:**
```bash
go run ./examples/name_database
```

**Key operations:**
```go
db, err := namedb.NewNameDatabase(dataDir, network)
defer db.Close()

// Get name
record, err := db.GetName("d/example")

// List all names
names, err := db.ListNames()

// Get history
history, err := db.GetNameHistory("d/example")
```

---

## Common Patterns

### Context Usage

Always use contexts with timeouts:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

record, err := nc.ResolveName(ctx, name)
```

### Resource Cleanup

```go
nc, err := client.NewClient(cfg)
if err != nil {
    return err
}
defer nc.Close()  // Always close
```

### Mode Selection

```go
// Production: Auto-detection (daemon preferred)
Mode: client.ModeAuto

// Development: Embedded (no daemon needed)
Mode: client.ModeEmbedded

// CI/Testing: Daemon with known endpoint
Mode: client.ModeDaemon
RPCAddress: "localhost:18336"
```

### Wallet Management

```go
cfg := &client.Config{
    WalletEnabled: true,
    WalletPath:    "/secure/location/wallet.json",
    WalletPassword: os.Getenv("WALLET_PASSWORD"),
}
```

### Network Selection

```go
// Mainnet (production)
Network: "mainnet"

// Testnet (testing)
Network: "testnet"

// Regtest (local development)
Network: "regtest"
```

---

## Error Handling

### Standard Errors

```go
record, err := nc.ResolveName(ctx, name)
if err != nil {
    switch {
    case errors.Is(err, client.ErrNameNotFound):
        // Name doesn't exist
    case errors.Is(err, client.ErrNameExpired):
        // Name expired
    case errors.Is(err, context.DeadlineExceeded):
        // Timeout
    case errors.Is(err, client.ErrNotConnected):
        // No daemon connection
    default:
        // Other error
    }
}
```

### Registration Errors

```go
_, err := nc.RegisterName(ctx, name, value, salt)
if err != nil {
    switch {
    case errors.Is(err, client.ErrNameAlreadyExists):
        // Name taken
    case errors.Is(err, client.ErrInvalidName):
        // Invalid format
    case errors.Is(err, client.ErrInvalidValue):
        // Value too large or invalid JSON
    case errors.Is(err, client.ErrInsufficientFunds):
        // Need more NMC
    }
}
```

### Best Practices

- Check specific errors first (Is, As)
- Log errors with context
- Return wrapped errors: `fmt.Errorf("resolve failed: %w", err)`
- Don't ignore errors from Close()

---

## Best Practices

### 1. Connection Pooling

```go
// Reuse client across requests
var nc *client.Client

func init() {
    var err error
    nc, err = client.NewClient(&client.Config{Mode: client.ModeAuto})
    if err != nil {
        log.Fatal(err)
    }
}

// Use in handlers
func handler(w http.ResponseWriter, r *http.Request) {
    record, err := nc.ResolveName(r.Context(), name)
    // ...
}
```

### 2. Graceful Shutdown

```go
sig := make(chan os.Signal, 1)
signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

go func() {
    <-sig
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    nc.Close()
    os.Exit(0)
}()
```

### 3. Metrics and Monitoring

```go
import "github.com/prometheus/client_golang/prometheus"

var resolveCounter = prometheus.NewCounterVec(
    prometheus.CounterOpts{Name: "name_resolves_total"},
    []string{"status"},
)

record, err := nc.ResolveName(ctx, name)
if err != nil {
    resolveCounter.WithLabelValues("error").Inc()
} else {
    resolveCounter.WithLabelValues("success").Inc()
}
```

### 4. Testing

```go
func TestNameResolution(t *testing.T) {
    cfg := &client.Config{
        Mode:    client.ModeEmbedded,
        Network: "regtest",
        DataDir: t.TempDir(),
    }
    
    nc, err := client.NewClient(cfg)
    require.NoError(t, err)
    defer nc.Close()
    
    // Test resolution
    _, err = nc.ResolveName(context.Background(), "d/example")
    assert.Error(t, err)
}
```

---

## Troubleshooting

### Cannot connect to daemon

**Error:** `failed to connect to daemon`

**Solution:**
```bash
# Check daemon is running
ps aux | grep nmcd

# Use embedded mode instead
cfg := &client.Config{Mode: client.ModeEmbedded}
```

### Name not found

**Error:** `name not found: d/example`

**Cause:** Name doesn't exist or expired

**Solution:**
```bash
# Check name status
nmcd-cli name_show d/example

# Register if needed
go run ./examples/register_name d/example '{"ip":"1.2.3.4"}'
```

### Registration fails

**Error:** `name already exists`

**Cause:** Name taken or commitment not found

**Solution:**
- Choose different name
- Wait 12 blocks after NAME_NEW
- Verify commitment TX confirmed

### High memory usage

**Cause:** Large embedded blockchain

**Solution:**
```go
// Use daemon mode for production
cfg := &client.Config{
    Mode:       client.ModeDaemon,
    RPCAddress: "localhost:8336",
}
```

### Context deadline exceeded

**Cause:** Operation timeout

**Solution:**
```go
// Increase timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
```

---

## Additional Resources

- [API Documentation](API.md)
- [Embedding Guide](EMBEDDING.md)
- [Operations Guide](OPERATIONS.md)
- [Go Package Documentation](https://pkg.go.dev/github.com/opd-ai/nmcd)

## Contributing Examples

Submit examples via pull request to `examples/` directory. Include:
- Clear README with purpose and usage
- Well-commented code
- Error handling
- Resource cleanup

## License

Examples are MIT licensed. See LICENSE file.
