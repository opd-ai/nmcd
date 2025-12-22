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

// Namecoin-specific opcodes for name operations.
// These opcodes extend Bitcoin's script language for name management.
// See: https://github.com/namecoin/namecoin-core for reference.
const (
	// opNameNew is the opcode for NAME_NEW (pre-registration with hash commitment)
	// Script format: OP_NAME_NEW <hash> OP_2DROP <standard script>
	opNameNew = 0xd0

	// opNameFirstUpdate is the opcode for NAME_FIRSTUPDATE (first registration)
	// Script format: OP_NAME_FIRSTUPDATE <name> <rand> <value> OP_2DROP OP_2DROP <standard script>
	opNameFirstUpdate = 0xd1

	// opNameUpdate is the opcode for NAME_UPDATE (update existing name)
	// Script format: OP_NAME_UPDATE <name> <value> OP_2DROP OP_DROP <standard script>
	opNameUpdate = 0xd2

	// opPushData1 is the opcode for pushing 76-255 bytes
	opPushData1 = 0x4c

	// opPushData2 is the opcode for pushing 256-65535 bytes
	opPushData2 = 0x4d

	// opPushData4 is the opcode for pushing up to 4GB of data (rarely used)
	opPushData4 = 0x4e
)

// parseNameScript extracts name operation from script.
// Namecoin scripts use Bitcoin's push data format with length-prefixed data.
// Returns the operation type, name, value, and any parsing error.
func parseNameScript(script []byte) (namedb.NameOperation, string, string, error) {
	if len(script) < 2 {
		return 0, "", "", fmt.Errorf("script too short")
	}

	switch script[0] {
	case opNameNew:
		// NAME_NEW: OP_NAME_NEW <hash> ...
		// The hash is a 20-byte commitment, but we don't need to extract it
		// for validation purposes in this implementation.
		return namedb.NameNew, "", "", nil

	case opNameFirstUpdate:
		// NAME_FIRSTUPDATE: OP_NAME_FIRSTUPDATE <name> <rand> <value> ...
		// Parse: name, skip rand, then value
		offset := 1

		// Extract name
		name, newOffset, err := readPushData(script, offset)
		if err != nil {
			return 0, "", "", fmt.Errorf("failed to read name: %w", err)
		}
		offset = newOffset

		// Skip rand (20 bytes typically, but use push data format)
		_, newOffset, err = readPushData(script, offset)
		if err != nil {
			return 0, "", "", fmt.Errorf("failed to read rand: %w", err)
		}
		offset = newOffset

		// Extract value
		value, _, err := readPushData(script, offset)
		if err != nil {
			return 0, "", "", fmt.Errorf("failed to read value: %w", err)
		}

		return namedb.NameFirstUpdate, string(name), string(value), nil

	case opNameUpdate:
		// NAME_UPDATE: OP_NAME_UPDATE <name> <value> ...
		offset := 1

		// Extract name
		name, newOffset, err := readPushData(script, offset)
		if err != nil {
			return 0, "", "", fmt.Errorf("failed to read name: %w", err)
		}
		offset = newOffset

		// Extract value
		value, _, err := readPushData(script, offset)
		if err != nil {
			return 0, "", "", fmt.Errorf("failed to read value: %w", err)
		}

		return namedb.NameUpdate, string(name), string(value), nil
	}

	return 0, "", "", fmt.Errorf("not a name operation")
}

// readPushData reads a Bitcoin-style push data from the script at the given offset.
// Returns the data, the new offset after reading, and any error.
// Bitcoin script push data format:
//   - 0x00: push empty byte array (OP_0)
//   - 0x01-0x4b: next N bytes are data (N is the opcode value)
//   - 0x4c (OP_PUSHDATA1): next byte is length, then data
//   - 0x4d (OP_PUSHDATA2): next 2 bytes are length (little-endian), then data
//   - 0x4e (OP_PUSHDATA4): next 4 bytes are length (little-endian), then data
func readPushData(script []byte, offset int) ([]byte, int, error) {
	if offset >= len(script) {
		return nil, offset, fmt.Errorf("offset beyond script length")
	}

	startOffset := offset
	opcode := script[offset]
	offset++

	var dataLen int

	switch {
	case opcode == 0x00:
		// OP_0: push empty byte array
		dataLen = 0

	case opcode >= 0x01 && opcode <= 0x4b:
		// Direct push: opcode is the length (1-75 bytes)
		dataLen = int(opcode)

	case opcode == opPushData1:
		// OP_PUSHDATA1: next byte is length
		if offset >= len(script) {
			return nil, offset, fmt.Errorf("missing length byte for OP_PUSHDATA1")
		}
		dataLen = int(script[offset])
		offset++

	case opcode == opPushData2:
		// OP_PUSHDATA2: next 2 bytes are length (little-endian)
		if offset+1 >= len(script) {
			return nil, offset, fmt.Errorf("missing length bytes for OP_PUSHDATA2")
		}
		dataLen = int(script[offset]) | (int(script[offset+1]) << 8)
		offset += 2

	case opcode == opPushData4:
		// OP_PUSHDATA4: next 4 bytes are length (little-endian)
		if offset+3 >= len(script) {
			return nil, offset, fmt.Errorf("missing length bytes for OP_PUSHDATA4")
		}
		dataLen = int(script[offset]) |
			(int(script[offset+1]) << 8) |
			(int(script[offset+2]) << 16) |
			(int(script[offset+3]) << 24)
		offset += 4

	default:
		return nil, offset, fmt.Errorf("unexpected opcode 0x%02x at offset %d", opcode, startOffset)
	}

	// Check if we have enough data
	if offset+dataLen > len(script) {
		return nil, offset, fmt.Errorf("data length %d exceeds remaining script", dataLen)
	}

	data := script[offset : offset+dataLen]
	return data, offset + dataLen, nil
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
