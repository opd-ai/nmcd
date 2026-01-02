package client

import (
	"context"
	"errors"
	"time"
)

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

	// Logger is a custom logger for client operations.
	// If nil, uses default logger.
	// Logger *log.Logger // TODO: Implement in Phase 2
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
