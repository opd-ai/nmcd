// Package client provides a high-level interface for interacting with the Namecoin network.
//
// This package offers both embedded (in-process) and daemon (RPC) client modes with
// automatic mode detection. It is designed for applications that need to resolve,
// register, or manage Namecoin names programmatically.
//
// # API Stability (v1.0.0+)
//
// Starting with v1.0.0, this package follows semantic versioning and provides
// backward compatibility guarantees:
//
//   - All exported types, functions, and interfaces are stable
//   - Breaking changes only in MAJOR version releases (e.g., v1.x → v2.0)
//   - New features added in MINOR releases (e.g., v1.0 → v1.1)
//   - Bug fixes in PATCH releases (e.g., v1.0.0 → v1.0.1)
//
// # Thread Safety
//
// All client implementations (EmbeddedClient, DaemonClient) are safe for concurrent
// use by multiple goroutines. No external synchronization is required.
//
// # Basic Usage
//
// Auto-detection mode (recommended):
//
//	client, err := client.NewClient(nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	name, err := client.ResolveName(context.Background(), "d/example")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Value: %s\n", name.Value)
//
// Explicit embedded mode:
//
//	cfg := &client.Config{
//	    Mode:    client.ModeEmbedded,
//	    DataDir: "/path/to/data",
//	    Network: "mainnet",
//	}
//	client, err := client.NewClient(cfg)
//
// Explicit daemon mode:
//
//	cfg := &client.Config{
//	    Mode:        client.ModeDaemon,
//	    RPCAddr:     "http://localhost:8336",
//	    RPCUser:     "user",
//	    RPCPassword: "pass",
//	}
//	client, err := client.NewClient(cfg)
package client

import (
	"context"
	"errors"
	"time"

	"github.com/opd-ai/nmcd/internal/logging"
)

// NameClient is the primary interface for interacting with the Namecoin network.
// It provides methods for name resolution, registration, and management.
// Implementations include EmbeddedClient (in-process) and DaemonClient (RPC).
//
// # Thread Safety
//
// All methods are safe for concurrent use by multiple goroutines.
//
// # API Stability Guarantee (v1.0.0+)
//
// This interface is part of the stable v1.0.0+ API contract. Changes to this interface
// will only occur in MAJOR version releases. Specifically:
//
//   - Method signatures on this interface will not change or be removed in MINOR or PATCH releases
//   - New methods will not be added to this interface in MINOR or PATCH releases; adding a method
//     to NameClient requires a MAJOR release
//   - New standalone exported functions, types, or helper interfaces in this package may be added
//     in MINOR releases, provided they do not break existing code
//   - Behavior changes that break existing usage patterns require a MAJOR release
//
// # Context Support
//
// All methods that accept a context.Context parameter support:
//   - Cancellation via ctx.Done()
//   - Timeouts via context.WithTimeout/WithDeadline
//   - Request-scoped values via context.WithValue
//
// Methods return context.Canceled or context.DeadlineExceeded when appropriate.
//
// # Error Handling
//
// Methods return well-defined errors (ErrNameNotFound, ErrNameExpired, etc.) that
// can be checked with errors.Is(). Internal errors are wrapped with context using
// fmt.Errorf("%w", err) to preserve error chains.
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
//
// # API Stability (v1.0.0+)
//
// This type is part of the stable API. Fields may be added in MINOR releases
// but existing fields will not be removed or have their types changed except
// in MAJOR releases.
type NameRecord struct {
	Name      string    // Name identifier (e.g., "d/example")
	Value     string    // Current value (typically JSON for d/ and id/ namespaces)
	TxHash    string    // Transaction hash of last operation
	Height    int32     // Block height where last operation occurred
	ExpiresAt int32     // Block height when name expires
	ExpiresIn int32     // Blocks remaining until expiration
	Address   string    // Current owner address
	UpdatedAt time.Time // Timestamp of last update
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
	TxHash        string   // Transaction hash
	Name          string   // Name that was operated on
	Status        TxStatus // Transaction status (pending, confirmed, failed)
	Confirmations int      // Number of confirmations (0 if pending)
	BlockHeight   int32    // Block height where confirmed (0 if pending)
	BlockHash     string   // Block hash where confirmed (empty if pending)
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
	// NamePattern performs string prefix matching on the entire name.
	// A name matches if it starts with the exact NamePattern string (character-by-character).
	// Note: Only simple string prefix matching is supported (not glob patterns or regex).
	// Examples:
	//   - Pattern "d/example" matches: "d/example" (exact), "d/example1", "d/examplefoo"
	//   - Pattern "d/example" does NOT match: "d/other", "d/ex", "example"
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
//
// # API Stability (v1.0.0+)
//
// This type is part of the stable API. New optional fields may be added in MINOR
// releases. Existing fields will maintain their semantics except in MAJOR releases.
//
// # Field Defaults
//
// Zero values for fields trigger default behavior:
//   - Mode: ModeAuto (auto-detect daemon, fallback to embedded)
//   - DataDir: ~/.nmcd
//   - Network: "mainnet"
//   - RPCAddr: http://localhost:8336
//   - MaxPeers: 8
//
// # Network Configuration
//
// The Network field must match the network that the daemon is running on (for
// daemon mode) or the network to use for embedded mode. Valid values:
//   - "mainnet" (default)
//   - "testnet"
//   - "regtest"
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
	// If empty and MaxPeers > 0, automatically uses DNS seed discovery to find peers.
	// Set to an empty slice explicitly if you want to disable automatic peer discovery.
	// Example custom peers: []string{"peer1.example.com:8334", "peer2.example.com:8334"}
	BootstrapPeers []string

	// DisableWallet disables wallet functionality.
	// Default: false
	DisableWallet bool

	// Logger is a custom logger for client operations.
	// If nil, uses default logger from internal/logging package.
	// This allows applications to customize logging behavior, output format,
	// and destination for client operations.
	//
	// Example:
	//   logger, _ := logging.Init(&logging.Config{
	//       Level: logging.LevelDebug,
	//       Format: "json",
	//       Output: "/var/log/nmcd/client.log",
	//   })
	//   cfg := &client.Config{
	//       Logger: logger,
	//   }
	Logger *logging.Logger
}

// ClientMode specifies the client operation mode.
type ClientMode int

const (
	ModeAuto     ClientMode = iota // Auto-detect daemon, fallback to embedded
	ModeEmbedded                   // Force embedded mode (in-process)
	ModeDaemon                     // Force daemon mode (RPC only)
)

// Errors returned by the client.
//
// # API Stability (v1.0.0+)
//
// These error variables are part of the stable API contract. They will not be
// removed or have their values changed except in MAJOR releases. New error types
// may be added in MINOR releases.
//
// # Error Checking
//
// Use errors.Is() to check for specific errors:
//
//	name, err := client.ResolveName(ctx, "d/example")
//	if errors.Is(err, client.ErrNameNotFound) {
//	    // Handle name not found
//	}
//
// # Error Wrapping
//
// Client methods may wrap these errors with additional context using fmt.Errorf.
// Always use errors.Is() instead of direct equality checks (==).
var (
	ErrNameNotFound      = errors.New("name not found")
	ErrNameExpired       = errors.New("name has expired")
	ErrNameExists        = errors.New("name already registered")
	ErrInvalidName       = errors.New("invalid name format")
	ErrInvalidValue      = errors.New("invalid value format")
	ErrInsufficientFunds = errors.New("insufficient funds for operation")
	ErrNoWallet          = errors.New("wallet not initialized")
	ErrDaemonUnavailable = errors.New("daemon unavailable")
	ErrContextCanceled   = context.Canceled // Use standard library error for context cancellation
)
