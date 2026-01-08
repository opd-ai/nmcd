package client

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/opd-ai/nmcd/chain"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/namedb"
	"github.com/opd-ai/nmcd/network"
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
	peerMgr *network.PeerManager
	network string
	dataDir string

	// mu protects client state during initialization and shutdown
	mu sync.RWMutex

	// stopCh signals shutdown to background goroutines.
	// Note: In the current Phase 2 foundation implementation, no background
	// goroutines are started yet. This field is reserved for future use when
	// implementing background tasks (e.g., NAME_NEW tracking, blockchain sync).
	stopCh chan struct{}

	// wg tracks background goroutines for graceful shutdown.
	// It is kept alongside stopCh to support future background workers.
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

	// Initialize blockchain with name database support
	// The blockchain provides block validation, name operation tracking,
	// and serves as the authoritative source for blockchain state.
	chainCfg := &chain.Config{
		ChainParams: chainParams,
		NameDBPath:  filepath.Join(cfg.DataDir, "names.db"),
		DataDir:     cfg.DataDir,
	}

	bc, err := chain.NewBlockChain(chainCfg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create blockchain: %w", err)
	}

	// Initialize wallet (if not disabled)
	var w *wallet.Wallet
	if !cfg.DisableWallet {
		w, err = wallet.NewWallet(cfg.DataDir, chainParams)
		if err != nil {
			bc.Close()
			return nil, fmt.Errorf("failed to initialize wallet: %w", err)
		}
	}

	// Get name database from blockchain for consistent access
	nameDB := bc.GetNameDB()

	// Set MaxPeers default if not configured
	if cfg.MaxPeers == 0 {
		cfg.MaxPeers = 8
	}

	// Initialize peer manager for network connectivity
	// ListenAddrs is empty for embedded clients (no incoming connections by default)
	netCfg := &network.Config{
		ChainParams: chainParams,
		Blockchain:  bc,
		ListenAddrs: []string{}, // Embedded clients don't listen for incoming connections
		MaxPeers:    cfg.MaxPeers,
	}

	peerMgr, err := network.NewPeerManager(netCfg)
	if err != nil {
		bc.Close()
		return nil, fmt.Errorf("failed to create peer manager: %w", err)
	}

	client := &EmbeddedClient{
		chain:   bc,
		nameDB:  nameDB,
		wallet:  w,
		peerMgr: peerMgr,
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
		// Check if error is the sentinel ErrNameNotFound
		if errors.Is(err, namedb.ErrNameNotFound) {
			return nil, ErrNameNotFound
		}
		return nil, fmt.Errorf("failed to get name: %w", err)
	}

	// Get current blockchain height for expiration calculation
	// Use the blockchain's best snapshot to get the current tip
	bestHeight := c.chain.BestSnapshot().Height

	// Check if name has expired
	// A name expires AFTER the block at ExpiresAt height, not during it
	// So ExpiresAt == bestHeight means ExpiresIn = 0, which is still valid
	if record.ExpiresAt < bestHeight {
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
// This implements the two-phase NAME_NEW → NAME_FIRSTUPDATE registration process.
//
// The registration process:
// 1. Creates a NAME_NEW transaction with a commitment hash (prevents front-running)
// 2. Broadcasts the NAME_NEW transaction
// 3. If WaitForConfirmation is true:
//   - Waits for at least 12 block confirmations (minimum protocol requirement)
//   - Automatically creates and broadcasts NAME_FIRSTUPDATE transaction
//   - Returns the NAME_FIRSTUPDATE transaction hash
//
// 4. If WaitForConfirmation is false:
//   - Returns immediately with NAME_NEW transaction hash
//   - Caller must manually complete NAME_FIRSTUPDATE after 12 blocks
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - name: Name to register (1-255 characters, e.g., "d/example")
//   - value: Initial value for the name (max 1023 bytes, typically JSON)
//   - opts: Registration options (address, confirmations, fee rate, etc.)
//
// Returns:
//   - *TxResult: Result containing transaction hash and status
//   - error: Any error encountered during registration
//
// Example:
//
//	result, err := client.RegisterName(ctx, "d/example", `{"ip":"1.2.3.4"}`, &RegisterOpts{
//	    WaitForConfirmation: true,  // Automatic NAME_FIRSTUPDATE after 12 blocks
//	    Confirmations:       12,    // Can request more confirmations if desired
//	})
func (c *EmbeddedClient) RegisterName(ctx context.Context, name, value string, opts *RegisterOpts) (*TxResult, error) {
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

	// Check if wallet is available
	if c.wallet == nil {
		return nil, ErrNoWallet
	}

	// Validate inputs
	if len(name) == 0 || len(name) > 255 {
		return nil, fmt.Errorf("%w: length %d (must be 1-255)", ErrInvalidName, len(name))
	}
	if len(value) > 1023 {
		return nil, fmt.Errorf("%w: length %d (max 1023)", ErrInvalidValue, len(value))
	}

	// Check if name already exists
	existing, err := c.nameDB.GetName(name)
	if err == nil {
		// Name exists, check if it's expired
		bestHeight := c.chain.BestSnapshot().Height
		if existing.ExpiresAt > bestHeight {
			return nil, fmt.Errorf("%w: %s", ErrNameExists, name)
		}
	}

	// Set default options
	if opts == nil {
		opts = &RegisterOpts{
			Confirmations: 1,
			FeeRate:       1,
		}
	}
	if opts.FeeRate == 0 {
		opts.FeeRate = 1
	}
	if opts.Confirmations == 0 {
		opts.Confirmations = 1
	}

	// Get or create wallet address
	var ownerAddr string
	if opts.FromAddress != "" {
		// Use provided address
		if !c.wallet.HasKey(opts.FromAddress) {
			return nil, fmt.Errorf("wallet does not have key for address: %s", opts.FromAddress)
		}
		ownerAddr = opts.FromAddress
	} else {
		// Get first address or generate new one
		addrs := c.wallet.GetAddresses()
		if len(addrs) == 0 {
			ownerAddr, err = c.wallet.GenerateKey()
			if err != nil {
				return nil, fmt.Errorf("failed to generate wallet address: %w", err)
			}
		} else {
			ownerAddr = addrs[0]
		}
	}

	// Get UTXOs for funding the transaction
	utxos, err := c.chain.GetUTXOsForAddress(ownerAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs: %w", err)
	}

	if len(utxos) == 0 {
		return nil, fmt.Errorf("%w: no UTXOs available for address %s", ErrInsufficientFunds, ownerAddr)
	}

	// Convert namedb UTXOs to wallet UTXOs
	walletUTXOs := make([]wallet.UTXO, len(utxos))
	for i, utxo := range utxos {
		walletUTXOs[i] = wallet.UTXO{
			TxHash:   utxo.TxHash,
			Vout:     utxo.OutIndex,
			Value:    utxo.Value,
			PkScript: utxo.PkScript,
			Address:  utxo.Address,
		}
	}

	// Parse owner address for transaction creation
	ownerBtcAddr, err := c.wallet.GetKey(ownerAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get owner key: %w", err)
	}

	// Generate random bytes for NAME_NEW commitment
	randBytes, err := wallet.GenerateRand()
	if err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Create NAME_NEW transaction
	nameNewTx, randUsed, err := c.wallet.CreateNameNewTx(
		randBytes,
		name,
		walletUTXOs,
		opts.FeeRate,
		ownerBtcAddr.Address,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create NAME_NEW transaction: %w", err)
	}

	// Broadcast NAME_NEW transaction to network
	nameNewTxHash := nameNewTx.TxHash()
	if c.peerMgr != nil {
		if err := c.peerMgr.BroadcastTx(nameNewTx); err != nil {
			// Log warning but don't fail - transaction is still valid locally
			// In offline mode or with no peers, transaction can be broadcast later
			log.Printf("Warning: failed to broadcast NAME_NEW transaction: %v", err)
		}
	}

	result := &TxResult{
		TxHash:        nameNewTxHash.String(),
		Name:          name,
		Status:        TxStatusPending,
		Confirmations: 0,
		BlockHeight:   0,
		BlockHash:     "",
	}

	// If WaitForConfirmation is false, return immediately with NAME_NEW tx hash
	if !opts.WaitForConfirmation {
		return result, nil
	}

	// Wait for NAME_NEW confirmation (minimum 12 blocks as per protocol)
	// The MinBlocksBeforeFirstUpdate (12) is the required delay before NAME_FIRSTUPDATE
	confirmations := opts.Confirmations
	if confirmations < 12 {
		confirmations = 12 // Protocol requires at least 12 blocks
	}

	if err := c.WaitForConfirmation(ctx, nameNewTxHash.String(), confirmations); err != nil {
		return nil, fmt.Errorf("failed to wait for NAME_NEW confirmation: %w", err)
	}

	// After waiting for 12+ confirmations, automatically create and broadcast NAME_FIRSTUPDATE
	// Get UTXOs including the NAME_NEW UTXO
	utxosForFirstUpdate, err := c.chain.GetUTXOsForAddress(ownerAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs for NAME_FIRSTUPDATE: %w", err)
	}

	// Find the NAME_NEW UTXO (always at output index 0)
	var nameNewUtxoIndex int = -1
	for i, utxo := range utxosForFirstUpdate {
		if utxo.TxHash.String() == nameNewTxHash.String() && utxo.OutIndex == 0 {
			nameNewUtxoIndex = i
			break
		}
	}

	if nameNewUtxoIndex == -1 {
		return nil, fmt.Errorf("NAME_NEW UTXO not found for NAME_FIRSTUPDATE")
	}

	// Convert namedb UTXOs to wallet UTXOs for NAME_FIRSTUPDATE
	walletUTXOsForFirstUpdate := make([]wallet.UTXO, len(utxosForFirstUpdate))
	for i, utxo := range utxosForFirstUpdate {
		walletUTXOsForFirstUpdate[i] = wallet.UTXO{
			TxHash:   utxo.TxHash,
			Vout:     utxo.OutIndex,
			Value:    utxo.Value,
			PkScript: utxo.PkScript,
			Address:  utxo.Address,
		}
	}

	// Convert randUsed bytes to hex string for NAME_FIRSTUPDATE
	randUsedHex := fmt.Sprintf("%x", randUsed)

	// Create NAME_FIRSTUPDATE transaction
	nameFirstUpdateTx, err := c.wallet.CreateNameFirstUpdateTx(
		name,
		randUsedHex,
		value,
		walletUTXOsForFirstUpdate,
		nameNewUtxoIndex,
		opts.FeeRate,
		ownerBtcAddr.Address,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create NAME_FIRSTUPDATE transaction: %w", err)
	}

	// Broadcast NAME_FIRSTUPDATE transaction
	nameFirstUpdateTxHash := nameFirstUpdateTx.TxHash()
	if c.peerMgr != nil {
		if err := c.peerMgr.BroadcastTx(nameFirstUpdateTx); err != nil {
			log.Printf("Warning: failed to broadcast NAME_FIRSTUPDATE transaction: %v", err)
		}
	}

	// Update result with NAME_FIRSTUPDATE transaction info. At this point
	// the transaction has only been broadcast and may still be in the mempool,
	// so we report it as pending with zero confirmations.
	result.TxHash = nameFirstUpdateTxHash.String()
	result.Status = TxStatusPending
	result.Confirmations = 0

	return result, nil
}

// UpdateName updates the value of an existing name using a NAME_UPDATE operation.
// It validates the client state and inputs, then constructs a transaction using the
// embedded wallet. The caller is responsible for broadcasting the transaction to the
// network and handling confirmations (network integration in Phase 3).
//
// The wallet must contain the private key for the address that currently owns the name.
// The name must exist in the database and must not be expired.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - name: Name to update (must exist and not be expired)
//   - value: New value for the name (max 1023 bytes, typically JSON)
//   - opts: Update options (transfer address, confirmations, fee rate, etc.)
//
// Returns:
//   - *TxResult: Result containing transaction hash and pending status
//   - error: Any error encountered during update
//
// Example:
//
//	result, err := client.UpdateName(ctx, "d/example", `{"ip":"5.6.7.8"}`, &UpdateOpts{
//	    FeeRate: 1,
//	})
func (c *EmbeddedClient) UpdateName(ctx context.Context, name, value string, opts *UpdateOpts) (*TxResult, error) {
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

	// Check if wallet is available
	if c.wallet == nil {
		return nil, ErrNoWallet
	}

	// Validate inputs
	if len(name) == 0 || len(name) > 255 {
		return nil, fmt.Errorf("%w: length %d (must be 1-255)", ErrInvalidName, len(name))
	}
	if len(value) > 1023 {
		return nil, fmt.Errorf("%w: length %d (max 1023)", ErrInvalidValue, len(value))
	}

	// Check if name exists and get current record
	nameRecord, err := c.nameDB.GetName(name)
	if err != nil {
		if errors.Is(err, namedb.ErrNameNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrNameNotFound, name)
		}
		return nil, fmt.Errorf("failed to get name record: %w", err)
	}

	// Check if name is expired
	// A name expires AFTER the block at ExpiresAt height, not during it
	// So ExpiresAt == bestHeight means ExpiresIn = 0, which is still valid
	bestHeight := c.chain.BestSnapshot().Height
	if nameRecord.ExpiresAt < bestHeight {
		return nil, fmt.Errorf("%w: %s (expired at height %d, current height %d)",
			ErrNameExpired, name, nameRecord.ExpiresAt, bestHeight)
	}

	// Verify wallet has key for the current owner address
	if !c.wallet.HasKey(nameRecord.Address) {
		return nil, fmt.Errorf("wallet does not have key for name owner address: %s", nameRecord.Address)
	}

	// Set default options
	if opts == nil {
		opts = &UpdateOpts{
			Confirmations: 1,
			FeeRate:       1,
		}
	}
	if opts.FeeRate == 0 {
		opts.FeeRate = 1
	}
	if opts.Confirmations == 0 {
		opts.Confirmations = 1
	}

	// Get UTXOs for the current owner address
	utxos, err := c.chain.GetUTXOsForAddress(nameRecord.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs: %w", err)
	}

	if len(utxos) == 0 {
		return nil, fmt.Errorf("%w: no UTXOs available for address %s", ErrInsufficientFunds, nameRecord.Address)
	}

	// Find the name UTXO (the one that holds the name)
	var nameUTXOIndex int
	found := false
	for i, utxo := range utxos {
		if utxo.TxHash.IsEqual(&nameRecord.TxHash) && utxo.OutIndex == nameRecord.OutIndex {
			nameUTXOIndex = i
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("name UTXO not found in available UTXOs (expected tx %s, output %d)",
			nameRecord.TxHash.String(), nameRecord.OutIndex)
	}

	// Convert namedb UTXOs to wallet UTXOs
	walletUTXOs := make([]wallet.UTXO, len(utxos))
	for i, utxo := range utxos {
		walletUTXOs[i] = wallet.UTXO{
			TxHash:   utxo.TxHash,
			Vout:     utxo.OutIndex,
			Value:    utxo.Value,
			PkScript: utxo.PkScript,
			Address:  utxo.Address,
		}
	}

	// Parse destination address if transfer is requested
	var destAddr btcutil.Address
	if opts.TransferTo != "" {
		// For now, only same-address "transfers" are allowed and treated as no-transfer
		// (destAddr remains nil, so ownership stays with the current address).
		// Any real transfer to a different address requires full network integration.
		if opts.TransferTo != nameRecord.Address {
			return nil, fmt.Errorf("name transfers (TransferTo) require network integration (coming in future phase)")
		}
		// Transferring to same address is redundant but allowed
		log.Printf("Warning: TransferTo address matches current owner (%s) - transfer is redundant and will be ignored", opts.TransferTo)
		destAddr = nil
	}

	// Create NAME_UPDATE transaction
	updateTx, err := c.wallet.CreateNameUpdateTx(
		name,
		value,
		walletUTXOs,
		nameUTXOIndex,
		opts.FeeRate,
		destAddr,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create NAME_UPDATE transaction: %w", err)
	}

	// Broadcast NAME_UPDATE transaction to network
	updateTxHash := updateTx.TxHash()
	if c.peerMgr != nil {
		if err := c.peerMgr.BroadcastTx(updateTx); err != nil {
			// Log warning but don't fail - transaction is still valid locally
			// In offline mode or with no peers, transaction can be broadcast later
			log.Printf("Warning: failed to broadcast NAME_UPDATE transaction: %v", err)
		}
	}

	result := &TxResult{
		TxHash:        updateTxHash.String(),
		Name:          name,
		Status:        TxStatusPending,
		Confirmations: 0,
		BlockHeight:   0,
		BlockHash:     "",
	}

	// If WaitForConfirmation is false, return immediately with NAME_UPDATE tx hash
	if !opts.WaitForConfirmation {
		return result, nil
	}

	// Wait for NAME_UPDATE confirmation
	// Use default confirmations if not specified
	confirmations := opts.Confirmations
	if confirmations == 0 {
		confirmations = 1
	}

	if err := c.WaitForConfirmation(ctx, updateTxHash.String(), confirmations); err != nil {
		return nil, fmt.Errorf("failed to wait for NAME_UPDATE confirmation: %w", err)
	}

	// Update result with confirmation status
	result.Status = TxStatusConfirmed
	result.Confirmations = confirmations

	return result, nil
}

// ListNames returns all registered names, optionally filtered.
// Supports filtering by namespace, name pattern, address, and expiration status.
// Also supports pagination with limit and offset.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - filter: Optional filter configuration. If nil, returns all names with default limit.
//
// Returns:
//   - []*NameRecord: Slice of name records matching the filter criteria
//   - error: Error retrieving names, or nil on success
//
// Example:
//
//	// List all domain names (d/ namespace) with pagination
//	records, err := client.ListNames(ctx, &ListFilter{
//	    Namespace: "d/",
//	    Limit:     100,
//	    Offset:    0,
//	})
//
//	// List names owned by a specific address
//	records, err := client.ListNames(ctx, &ListFilter{
//	    Address: "N1A2B3C4...",
//	    IncludeExpired: false,
//	})
func (c *EmbeddedClient) ListNames(ctx context.Context, filter *ListFilter) ([]*NameRecord, error) {
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

	// Set default filter if nil
	if filter == nil {
		filter = &ListFilter{
			Limit: 100,
		}
	}

	// Set default limit if not specified
	if filter.Limit == 0 {
		filter.Limit = 100
	}

	// Cap limit at maximum
	if filter.Limit > 10000 {
		filter.Limit = 10000
	}

	// Get all names from database
	// nameDB.ListNames already uses RLock internally for thread safety
	dbRecords, err := c.nameDB.ListNames()
	if err != nil {
		return nil, fmt.Errorf("failed to list names: %w", err)
	}

	// Get current blockchain height for expiration calculation
	// Use the blockchain's best snapshot to get the current tip
	bestHeight := c.chain.BestSnapshot().Height

	// Apply filtering and convert to client format
	var filtered []*NameRecord
	for _, record := range dbRecords {
		// Check expiration
		// A name expires AFTER the block at ExpiresAt height, not during it
		// So ExpiresAt == bestHeight means ExpiresIn = 0, which is still valid
		if !filter.IncludeExpired && record.ExpiresAt < bestHeight {
			continue
		}

		// Filter by namespace (e.g., "d/", "id/", "p/")
		if filter.Namespace != "" {
			if len(record.Name) < len(filter.Namespace) {
				continue
			}
			if record.Name[:len(filter.Namespace)] != filter.Namespace {
				continue
			}
		}

		// Filter by name pattern (string prefix matching)
		// Matches if record.Name starts with filter.NamePattern (character-by-character)
		// More advanced pattern matching (glob, regex) can be added later if needed
		if filter.NamePattern != "" {
			if len(record.Name) < len(filter.NamePattern) {
				continue // Name is shorter than pattern, cannot match
			}
			// String prefix matching: check if name starts with pattern
			if record.Name[:len(filter.NamePattern)] != filter.NamePattern {
				continue
			}
		}

		// Filter by address
		if filter.Address != "" && record.Address != filter.Address {
			continue
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

		filtered = append(filtered, clientRecord)
	}

	// Apply offset
	if filter.Offset > 0 {
		if filter.Offset >= len(filtered) {
			return []*NameRecord{}, nil
		}
		filtered = filtered[filter.Offset:]
	}

	// Apply limit
	if len(filtered) > filter.Limit {
		filtered = filtered[:filter.Limit]
	}

	return filtered, nil
}

// GetNameHistory returns the full history of operations for a name.
// Includes all NAME_FIRSTUPDATE and NAME_UPDATE operations in chronological order.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - name: Name to retrieve history for (e.g., "d/example", "id/alice")
//
// Returns:
//   - []*NameRecord: Slice of name records in chronological order (oldest first)
//   - error: Error retrieving history, or nil on success. Returns empty slice if no history exists.
//
// Example:
//
//	history, err := client.GetNameHistory(ctx, "d/example")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for i, record := range history {
//	    fmt.Printf("Operation %d: Height=%d, Value=%s\n",
//	        i+1, record.Height, record.Value)
//	}
func (c *EmbeddedClient) GetNameHistory(ctx context.Context, name string) ([]*NameRecord, error) {
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

	// Get history from database
	// nameDB.GetHistory already uses RLock internally for thread safety
	dbRecords, err := c.nameDB.GetHistory(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get name history: %w", err)
	}

	// Get current blockchain height for expiration calculation
	// Use the blockchain's best snapshot to get the current tip
	bestHeight := c.chain.BestSnapshot().Height

	// Convert to client NameRecord format
	var history []*NameRecord
	for _, record := range dbRecords {
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

		history = append(history, clientRecord)
	}

	return history, nil
}

// WaitForConfirmation waits for a transaction to be confirmed in a block.
// Blocks until the transaction appears in the blockchain with the specified
// number of confirmations or the context is canceled.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - txHash: Transaction hash to wait for (hex string)
//   - confirmations: Number of block confirmations to wait for (min 1)
//
// Returns:
//   - error: ErrContextCanceled if context is canceled, or other errors
//
// Implementation notes:
// - Uses polling approach to check for transaction in blockchain
// - Poll interval: 10 seconds (average block time is 600 seconds)
// - Does not handle reorganizations (assumes forward-only blockchain)
func (c *EmbeddedClient) WaitForConfirmation(ctx context.Context, txHash string, confirmations int) error {
	// Check context
	select {
	case <-ctx.Done():
		return ErrContextCanceled
	default:
	}

	// Check if client is closed
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return fmt.Errorf("client is closed")
	}
	c.mu.RUnlock()

	// Validate confirmations
	if confirmations < 1 {
		return fmt.Errorf("confirmations must be at least 1, got %d", confirmations)
	}

	// Parse transaction hash
	txHashBytes, err := chainhash.NewHashFromStr(txHash)
	if err != nil {
		return fmt.Errorf("invalid transaction hash: %w", err)
	}

	// Check transaction status immediately before entering polling loop
	// This provides faster response for already-confirmed transactions
	txHeight, currentHeight, err := c.getTransactionConfirmationStatus(txHashBytes)
	if err == nil {
		// Transaction found, check if it has enough confirmations
		txConfirmations := currentHeight - txHeight + 1
		if txConfirmations >= int32(confirmations) {
			return nil
		}
	}

	// Poll for transaction confirmation
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ErrContextCanceled
		case <-ticker.C:
			// Check if transaction is confirmed
			txHeight, currentHeight, err := c.getTransactionConfirmationStatus(txHashBytes)
			if err != nil {
				// Transaction not found yet, continue waiting
				continue
			}

			// Calculate confirmations (current height - tx height + 1)
			txConfirmations := currentHeight - txHeight + 1
			if txConfirmations >= int32(confirmations) {
				return nil
			}
		}
	}
}

// getTransactionConfirmationStatus checks if a transaction is confirmed and returns its height.
// Returns (txHeight, currentHeight, error).
// Returns error if transaction is not found in blockchain.
func (c *EmbeddedClient) getTransactionConfirmationStatus(txHash *chainhash.Hash) (int32, int32, error) {
	// Get current best height
	bestSnapshot := c.chain.BestSnapshot()
	currentHeight := bestSnapshot.Height

	// Search through recent blocks for the transaction
	// We search backwards from current height for efficiency
	// In production, this would use a transaction index
	// For now, we check the last 100 blocks (should cover most cases)
	// Performance note: This creates O(blocks * transactions_per_block) complexity.
	// For blocks with many transactions, this linear search may be slow.
	// Consider reducing maxBlocksToSearch if performance becomes an issue.
	maxBlocksToSearch := int32(100)
	startHeight := currentHeight - maxBlocksToSearch
	if startHeight < 0 {
		startHeight = 0
	}

	for height := currentHeight; height >= startHeight; height-- {
		block, err := c.chain.BlockByHeight(height)
		if err != nil {
			continue
		}

		// Check if transaction is in this block
		for _, tx := range block.Transactions() {
			if tx.Hash().IsEqual(txHash) {
				return height, currentHeight, nil
			}
		}
	}

	return 0, 0, fmt.Errorf("transaction not found in recent blocks")
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

	// Get current blockchain state
	bestSnapshot := c.chain.BestSnapshot()

	// Get actual connection count from peer manager
	connections := 0
	if c.peerMgr != nil {
		connections = c.peerMgr.GetConnectedPeers()
	}

	info := &NodeInfo{
		Version:         "0.1.0",
		ProtocolVersion: 70015,
		BlockHeight:     bestSnapshot.Height,
		BestBlockHash:   bestSnapshot.Hash.String(),
		Connections:     connections,
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

	// Stop peer manager
	if c.peerMgr != nil {
		c.peerMgr.Stop()
	}

	// Close blockchain (which also closes the name database)
	var errs []error
	if c.chain != nil {
		if err := c.chain.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close blockchain: %w", err))
		}
	}

	// Wallet doesn't have a Close method in current implementation

	c.closed = true

	// Return first error if any
	if len(errs) > 0 {
		return errs[0]
	}

	return nil
}

// defaultConfig returns the default client configuration
func defaultConfig() *Config {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		// Fallback to current working directory if the home directory cannot be determined.
		homeDir = "."
	}
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
