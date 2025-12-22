package chain

import (
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/namedb"
)

// BlockChain wraps btcd blockchain with name operation validation
type BlockChain struct {
	*blockchain.BlockChain
	nameDB *namedb.NameDatabase
	mu     sync.RWMutex
}

// Config holds blockchain configuration
type Config struct {
	ChainParams *chaincfg.Params
	NameDBPath  string
	DataDir     string
}

// NewBlockChain creates a new blockchain with name support
func NewBlockChain(cfg *Config, indexManager blockchain.IndexManager) (*BlockChain, error) {
	// Create name database
	nameDB, err := namedb.NewNameDatabase(cfg.NameDBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create name database: %w", err)
	}

	bc := &BlockChain{
		nameDB: nameDB,
	}

	// Create blockchain config
	bcConfig := blockchain.Config{
		ChainParams:  cfg.ChainParams,
		TimeSource:   blockchain.NewMedianTime(),
		IndexManager: indexManager,
	}

	// Create the blockchain instance
	chain, err := blockchain.New(&bcConfig)
	if err != nil {
		nameDB.Close()
		return nil, fmt.Errorf("failed to create blockchain: %w", err)
	}

	bc.BlockChain = chain
	return bc, nil
}

// Close closes the blockchain and name database
func (bc *BlockChain) Close() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return bc.nameDB.Close()
}

// ProcessBlock processes a block and validates name operations
func (bc *BlockChain) ProcessBlock(block *btcutil.Block, flags blockchain.BehaviorFlags) (bool, bool, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Validate name operations before processing
	if err := bc.validateNameOperations(block); err != nil {
		return false, false, fmt.Errorf("invalid name operations: %w", err)
	}

	// Process the block using btcd blockchain
	isMainChain, isOrphan, err := bc.BlockChain.ProcessBlock(block, flags)
	if err != nil {
		return isMainChain, isOrphan, err
	}

	// Update name database if block is on main chain
	if isMainChain {
		if err := bc.updateNameDatabase(block); err != nil {
			return isMainChain, isOrphan, fmt.Errorf("failed to update name database: %w", err)
		}
	}

	return isMainChain, isOrphan, nil
}

// validateNameOperations validates name operations in a block
func (bc *BlockChain) validateNameOperations(block *btcutil.Block) error {
	height := block.Height()

	for _, tx := range block.Transactions() {
		msgTx := tx.MsgTx()

		// Check for name operations in transaction outputs
		for _, txOut := range msgTx.TxOut {
			op, name, value, err := parseNameScript(txOut.PkScript)
			if err != nil {
				continue // Not a name operation
			}

			switch op {
			case namedb.NameNew:
				// NAME_NEW is always valid (pre-registration)
				continue

			case namedb.NameFirstUpdate:
				// Verify name doesn't exist
				if _, err := bc.nameDB.GetName(name); err == nil {
					return fmt.Errorf("name already exists: %s", name)
				}

			case namedb.NameUpdate:
				// Verify name exists and not expired
				record, err := bc.nameDB.GetName(name)
				if err != nil {
					return fmt.Errorf("name not found for update: %s", name)
				}
				if record.ExpiresAt <= height {
					return fmt.Errorf("name expired: %s", name)
				}
			}

			// Validate name format and value size
			if err := validateNameFormat(name, value); err != nil {
				return err
			}
		}
	}

	return nil
}

// updateNameDatabase updates the name database with operations from a block
func (bc *BlockChain) updateNameDatabase(block *btcutil.Block) error {
	height := block.Height()

	// Handle expired names
	expired, err := bc.nameDB.GetExpiredNames(height)
	if err != nil {
		return err
	}
	for _, name := range expired {
		if err := bc.nameDB.DeleteName(name); err != nil {
			return err
		}
	}

	// Process name operations
	for _, tx := range block.Transactions() {
		msgTx := tx.MsgTx()
		txHash := tx.Hash()

		for _, txOut := range msgTx.TxOut {
			op, name, value, err := parseNameScript(txOut.PkScript)
			if err != nil {
				continue
			}

			record := &namedb.NameRecord{
				Name:      name,
				Value:     value,
				TxHash:    *txHash,
				Height:    height,
				ExpiresAt: height + config.NameExpirationBlocks,
			}

			switch op {
			case namedb.NameFirstUpdate, namedb.NameUpdate:
				if err := bc.nameDB.PutName(name, record); err != nil {
					return err
				}
				if err := bc.nameDB.AddHistory(*txHash, record); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// GetName retrieves a name from the database
func (bc *BlockChain) GetName(name string) (*namedb.NameRecord, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.nameDB.GetName(name)
}

// ListNames retrieves all names from the database
func (bc *BlockChain) ListNames() ([]*namedb.NameRecord, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.nameDB.ListNames()
}

// GetNameHistory retrieves the historical records for a specific name.
// Returns all past operations on the name in chronological order.
func (bc *BlockChain) GetNameHistory(name string) ([]*namedb.NameRecord, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.nameDB.GetHistory(name)
}

// parseNameScript extracts name operation from script
func parseNameScript(script []byte) (namedb.NameOperation, string, string, error) {
	// Simple parsing - in real implementation would use proper script parsing
	// This is a placeholder that looks for OP_NAME patterns
	if len(script) < 10 {
		return 0, "", "", fmt.Errorf("script too short")
	}

	// Check for name operation opcodes (simplified)
	// Real implementation would properly parse script opcodes
	if script[0] == 0x51 { // OP_NAME_NEW placeholder
		return namedb.NameNew, "", "", nil
	}
	if script[0] == 0x52 { // OP_NAME_FIRSTUPDATE placeholder
		// Extract name and value from script
		if len(script) < 20 {
			return 0, "", "", fmt.Errorf("invalid firstupdate script")
		}
		name := string(script[1:11])
		value := string(script[11:])
		return namedb.NameFirstUpdate, name, value, nil
	}
	if script[0] == 0x53 { // OP_NAME_UPDATE placeholder
		// Extract name and value from script
		if len(script) < 20 {
			return 0, "", "", fmt.Errorf("invalid update script")
		}
		name := string(script[1:11])
		value := string(script[11:])
		return namedb.NameUpdate, name, value, nil
	}

	return 0, "", "", fmt.Errorf("not a name operation")
}

// validateNameFormat validates name and value format
func validateNameFormat(name, value string) error {
	if len(name) == 0 || len(name) > config.MaxNameLength {
		return fmt.Errorf("invalid name length: %d (max: %d)", len(name), config.MaxNameLength)
	}
	if len(value) > config.MaxValueLength {
		return fmt.Errorf("value too large: %d bytes (max: %d)", len(value), config.MaxValueLength)
	}
	return nil
}

// BestSnapshot returns the current best chain snapshot
func (bc *BlockChain) BestSnapshot() *blockchain.BestState {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.BlockChain.BestSnapshot()
}

// GetBlockByHash returns a block by hash
func (bc *BlockChain) GetBlockByHash(hash *chainhash.Hash) (*btcutil.Block, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.BlockChain.BlockByHash(hash)
}

// VerifyBlock verifies a block
func (bc *BlockChain) VerifyBlock(block *btcutil.Block) error {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	// Validate name operations
	if err := bc.validateNameOperations(block); err != nil {
		return err
	}

	// Additional verification could be added here
	return nil
}

// HandleBlockchainNotification processes blockchain notifications
func (bc *BlockChain) HandleBlockchainNotification(notification *blockchain.Notification) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	switch notification.Type {
	case blockchain.NTBlockConnected:
		// Block connected to main chain
	case blockchain.NTBlockDisconnected:
		// Block disconnected from main chain (reorg)
	}
}

// GetBlockHeader returns a block header by hash
func (bc *BlockChain) GetBlockHeader(hash *chainhash.Hash) (wire.BlockHeader, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.BlockChain.HeaderByHash(hash)
}
