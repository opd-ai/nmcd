package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/opd-ai/nmcd/chain"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/namedb"
	"github.com/opd-ai/nmcd/wallet"
)

// EmbeddedClient provides an in-process Namecoin client with local database
// and blockchain validation. It implements the NameClient interface for
// embedded usage without requiring an external daemon.
//
// Thread-safety: All methods are safe for concurrent use.
type EmbeddedClient struct {
	chain   *chain.BlockChain
	nameDB  *namedb.NameDatabase
	wallet  *wallet.Wallet
	network string
	dataDir string

	// mu protects client state during initialization and shutdown
	mu sync.RWMutex

	// stopCh signals shutdown to background goroutines
	stopCh chan struct{}

	// wg tracks background goroutines for graceful shutdown
	wg sync.WaitGroup

	// closed tracks whether client has been closed
	closed bool
}

// NewEmbeddedClient creates a new embedded Namecoin client with the given configuration.
// It initializes the local blockchain database, name database, and wallet.
//
// The client operates independently without requiring an external daemon, making it
// suitable for applications that need direct blockchain access.
//
// Parameters:
//   - cfg: Client configuration. If nil, uses default configuration.
//
// Returns:
//   - *EmbeddedClient: Initialized client ready for use
//   - error: Initialization error, or nil on success
//
// Example:
//
//	cfg := &Config{
//	    DataDir: "/path/to/data",
//	    Network: "mainnet",
//	}
//	client, err := NewEmbeddedClient(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
func NewEmbeddedClient(cfg *Config) (*EmbeddedClient, error) {
	if cfg == nil {
		cfg = defaultConfig()
	}

	// Set defaults
	if cfg.DataDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		cfg.DataDir = filepath.Join(homeDir, ".nmcd")
	}

	if cfg.Network == "" {
		cfg.Network = "mainnet"
	}

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Get network parameters
	var chainParams *chaincfg.Params
	switch cfg.Network {
	case "mainnet":
		chainParams = &config.NamecoinMainNetParams
	case "testnet":
		chainParams = &config.NamecoinTestNetParams
	case "regtest":
		chainParams = &config.NamecoinRegTestParams
	default:
		return nil, fmt.Errorf("unknown network: %s", cfg.Network)
	}

	// Initialize name database directly for now
	// In later phases, we'll integrate with full blockchain
	dbPath := filepath.Join(cfg.DataDir, "names.db")
	nameDB, err := namedb.NewNameDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open name database: %w", err)
	}

	// For Phase 2, we create a minimal blockchain wrapper
	// Full blockchain initialization will be added in later phases
	bc := &chain.BlockChain{}
	// TODO: Initialize full blockchain in future phases
	// For now, we'll just use the nameDB directly

	// Initialize wallet (if not disabled)
	var w *wallet.Wallet
	if !cfg.DisableWallet {
		w, err = wallet.NewWallet(cfg.DataDir, chainParams)
		if err != nil {
			nameDB.Close()
			return nil, fmt.Errorf("failed to initialize wallet: %w", err)
		}
	}

	client := &EmbeddedClient{
		chain:   bc,
		nameDB:  nameDB,
		wallet:  w,
		network: cfg.Network,
		dataDir: cfg.DataDir,
		stopCh:  make(chan struct{}),
		closed:  false,
	}

	return client, nil
}

// ResolveName retrieves the current value and metadata for a name.
// Returns ErrNameNotFound if the name doesn't exist or has expired.
//
// The lookup is performed against the local name database, which contains
// all name registrations from the blockchain.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - name: Name to resolve (e.g., "d/example", "id/alice")
//
// Returns:
//   - *NameRecord: Name record with current value and metadata
//   - error: ErrNameNotFound if name doesn't exist, or other errors
//
// Example:
//
//	record, err := client.ResolveName(ctx, "d/example")
//	if err == ErrNameNotFound {
//	    fmt.Println("Name not registered")
//	} else if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Value: %s\nOwner: %s\n", record.Value, record.Address)
func (c *EmbeddedClient) ResolveName(ctx context.Context, name string) (*NameRecord, error) {
	// Check context
	select {
	case <-ctx.Done():
		return nil, ErrContextCanceled
	default:
	}

	// Check if client is closed
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client is closed")
	}
	c.mu.RUnlock()

	// Validate name format
	if name == "" {
		return nil, ErrInvalidName
	}

	// Get name from database
	// nameDB.GetName already uses RLock internally for thread safety
	record, err := c.nameDB.GetName(name)
	if err != nil {
		// Check if error message indicates name not found
		if err.Error() == "name not found" {
			return nil, ErrNameNotFound
		}
		return nil, fmt.Errorf("failed to get name: %w", err)
	}

	// Get current blockchain height for expiration calculation
	// For Phase 2, we use height 0 as placeholder
	// Full blockchain integration will be added in later phases
	bestHeight := int32(0)
	// TODO: Get from blockchain: bestSnapshot := c.chain.BestSnapshot()

	// Check if name has expired
	if record.ExpiresAt <= bestHeight {
		return nil, ErrNameExpired
	}

	// Convert to client NameRecord format
	clientRecord := &NameRecord{
		Name:      record.Name,
		Value:     record.Value,
		TxHash:    record.TxHash.String(),
		Height:    record.Height,
		ExpiresAt: record.ExpiresAt,
		ExpiresIn: record.ExpiresAt - bestHeight,
		Address:   record.Address,
		UpdatedAt: record.UpdatedAt,
	}

	return clientRecord, nil
}

// RegisterName creates a new name registration with the given value.
// This is a placeholder implementation for Phase 2.
func (c *EmbeddedClient) RegisterName(ctx context.Context, name, value string, opts *RegisterOpts) (*TxResult, error) {
	return nil, fmt.Errorf("RegisterName not yet implemented in Phase 2")
}

// UpdateName updates an existing name's value.
// This is a placeholder implementation for Phase 2.
func (c *EmbeddedClient) UpdateName(ctx context.Context, name, value string, opts *UpdateOpts) (*TxResult, error) {
	return nil, fmt.Errorf("UpdateName not yet implemented in Phase 2")
}

// ListNames returns all registered names, optionally filtered.
// This is a placeholder implementation for Phase 2.
func (c *EmbeddedClient) ListNames(ctx context.Context, filter *ListFilter) ([]*NameRecord, error) {
	return nil, fmt.Errorf("ListNames not yet implemented in Phase 2")
}

// GetNameHistory returns the full history of operations for a name.
// This is a placeholder implementation for Phase 2.
func (c *EmbeddedClient) GetNameHistory(ctx context.Context, name string) ([]*NameRecord, error) {
	return nil, fmt.Errorf("GetNameHistory not yet implemented in Phase 2")
}

// WaitForConfirmation waits for a transaction to be confirmed in a block.
// This is a placeholder implementation for Phase 2.
func (c *EmbeddedClient) WaitForConfirmation(ctx context.Context, txHash string, confirmations int) error {
	return fmt.Errorf("WaitForConfirmation not yet implemented in Phase 2")
}

// GetInfo returns general information about the node/network state.
//
// Returns:
//   - *NodeInfo: Node information including version, height, network
//   - error: Error retrieving information, or nil on success
func (c *EmbeddedClient) GetInfo(ctx context.Context) (*NodeInfo, error) {
	// Check context
	select {
	case <-ctx.Done():
		return nil, ErrContextCanceled
	default:
	}

	// Check if client is closed
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client is closed")
	}
	c.mu.RUnlock()

	// For Phase 2, return placeholder info
	// Full blockchain integration will be added in later phases
	info := &NodeInfo{
		Version:         "0.1.0",
		ProtocolVersion: 70015,
		BlockHeight:     0, // TODO: Get from blockchain
		BestBlockHash:   "0000000000000000000000000000000000000000000000000000000000000000",
		Connections:     0,
		NetworkName:     c.network,
		Mode:            "embedded",
	}

	return info, nil
}

// Close releases resources held by the client.
// This includes closing the database and stopping background goroutines.
//
// After Close is called, the client cannot be reused.
// Any subsequent method calls will return an error.
//
// Returns:
//   - error: Error during cleanup, or nil on success
func (c *EmbeddedClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil // Already closed
	}

	// Signal shutdown to background goroutines
	close(c.stopCh)

	// Wait for background goroutines to finish (with timeout would be better in production)
	c.wg.Wait()

	// Close name database
	var errs []error
	if c.nameDB != nil {
		if err := c.nameDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close name database: %w", err))
		}
	}

	// Wallet doesn't have a Close method in current implementation
	// No need to close wallet or blockchain placeholder

	c.closed = true

	// Return first error if any
	if len(errs) > 0 {
		return errs[0]
	}

	return nil
}

// defaultConfig returns the default client configuration
func defaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		Mode:           ModeAuto,
		DataDir:        filepath.Join(homeDir, ".nmcd"),
		Network:        "mainnet",
		RPCAddr:        "http://localhost:8336",
		MaxPeers:       8,
		DisableWallet:  false,
		BootstrapPeers: []string{},
	}
}
