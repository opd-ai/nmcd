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

	// Track NAME_NEW commitment hashes seen in this block to detect duplicates.
	// Using string conversion of byte slice as map key is idiomatic in Go.
	seenNameNewCommits := make(map[string]bool)

	for _, tx := range block.Transactions() {
		msgTx := tx.MsgTx()

		// Check for name operations in transaction outputs
		for _, txOut := range msgTx.TxOut {
			op, name, value, extra, err := parseNameScriptFull(txOut.PkScript)
			if err != nil {
				continue // Not a name operation
			}

			switch op {
			case namedb.NameNew:
				// Check for duplicate commitment hash in this block
				commitHashStr := string(extra)
				if seenNameNewCommits[commitHashStr] {
					return fmt.Errorf("duplicate name_new commitment in block")
				}
				seenNameNewCommits[commitHashStr] = true

				// Check if commitment already exists in database
				if _, err := bc.nameDB.GetNameNew(extra); err == nil {
					return fmt.Errorf("name_new commitment already exists")
				}

			case namedb.NameFirstUpdate:
				// Verify name doesn't exist
				if _, err := bc.nameDB.GetName(name); err == nil {
					return fmt.Errorf("name already exists: %s", name)
				}

				// Compute the commitment hash from rand (extra) and name
				commitHash := computeCommitHash(extra, name)

				// Verify NAME_NEW exists and MinBlocksBeforeFirstUpdate has passed
				nameNewRecord, err := bc.nameDB.GetNameNew(commitHash)
				if err != nil {
					return fmt.Errorf("no matching name_new found for name: %s", name)
				}

				// Check that enough blocks have passed since NAME_NEW
				// Handle edge case where height < nameNewRecord.Height (e.g., during reorg)
				if height < nameNewRecord.Height {
					return fmt.Errorf("name_firstupdate before name_new: block %d < name_new block %d",
						height, nameNewRecord.Height)
				}
				blocksSinceNew := height - nameNewRecord.Height
				if blocksSinceNew < config.MinBlocksBeforeFirstUpdate {
					return fmt.Errorf("name_firstupdate too early: %d blocks since name_new, minimum %d required",
						blocksSinceNew, config.MinBlocksBeforeFirstUpdate)
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
			op, name, value, extra, err := parseNameScriptFull(txOut.PkScript)
			if err != nil {
				continue
			}

			switch op {
			case namedb.NameNew:
				// Store the commitment hash with block height
				// extra contains the commitment hash from the script
				if err := bc.nameDB.PutNameNew(extra, height); err != nil {
					return err
				}

			case namedb.NameFirstUpdate:
				record := &namedb.NameRecord{
					Name:      name,
					Value:     value,
					TxHash:    *txHash,
					Height:    height,
					ExpiresAt: height + config.NameExpirationBlocks,
				}
				if err := bc.nameDB.PutName(name, record); err != nil {
					return err
				}
				if err := bc.nameDB.AddHistory(*txHash, record); err != nil {
					return err
				}
				// Clean up the NAME_NEW commitment after successful registration
				commitHash := computeCommitHash(extra, name)
				if err := bc.nameDB.DeleteNameNew(commitHash); err != nil {
					return err
				}

			case namedb.NameUpdate:
				record := &namedb.NameRecord{
					Name:      name,
					Value:     value,
					TxHash:    *txHash,
					Height:    height,
					ExpiresAt: height + config.NameExpirationBlocks,
				}
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

// computeCommitHash computes the NAME_NEW commitment hash.
// The commitment is RIPEMD160(SHA256(rand || name)) as per Namecoin protocol.
// This hash is stored in NAME_NEW and verified during NAME_FIRSTUPDATE.
func computeCommitHash(rand []byte, name string) []byte {
	nameBytes := []byte(name)
	data := make([]byte, len(rand)+len(nameBytes))
	copy(data, rand)
	copy(data[len(rand):], nameBytes)
	return btcutil.Hash160(data)
}

// parseNameScript extracts name operation from script.
// Namecoin scripts use Bitcoin's push data format with length-prefixed data.
// Returns the operation type, name, value, and any parsing error.
func parseNameScript(script []byte) (namedb.NameOperation, string, string, error) {
	op, name, value, _, err := parseNameScriptFull(script)
	return op, name, value, err
}

// parseNameScriptFull extracts name operation from script with additional data.
// Returns the operation type, name, value, extra data (hash for NAME_NEW, rand for
// NAME_FIRSTUPDATE), and any parsing error.
func parseNameScriptFull(script []byte) (namedb.NameOperation, string, string, []byte, error) {
	if len(script) < 2 {
		return 0, "", "", nil, fmt.Errorf("script too short")
	}

	switch script[0] {
	case opNameNew:
		// NAME_NEW: OP_NAME_NEW <hash> ...
		// Extract the commitment hash (typically 20 bytes)
		hash, _, err := readPushData(script, 1)
		if err != nil {
			return 0, "", "", nil, fmt.Errorf("failed to read hash: %w", err)
		}
		return namedb.NameNew, "", "", hash, nil

	case opNameFirstUpdate:
		// NAME_FIRSTUPDATE: OP_NAME_FIRSTUPDATE <name> <rand> <value> ...
		// Parse: name, rand, value
		offset := 1

		// Extract name
		name, newOffset, err := readPushData(script, offset)
		if err != nil {
			return 0, "", "", nil, fmt.Errorf("failed to read name: %w", err)
		}
		offset = newOffset

		// Extract rand (needed to compute commitment hash)
		rand, newOffset, err := readPushData(script, offset)
		if err != nil {
			return 0, "", "", nil, fmt.Errorf("failed to read rand: %w", err)
		}
		offset = newOffset

		// Extract value
		value, _, err := readPushData(script, offset)
		if err != nil {
			return 0, "", "", nil, fmt.Errorf("failed to read value: %w", err)
		}

		return namedb.NameFirstUpdate, string(name), string(value), rand, nil

	case opNameUpdate:
		// NAME_UPDATE: OP_NAME_UPDATE <name> <value> ...
		offset := 1

		// Extract name
		name, newOffset, err := readPushData(script, offset)
		if err != nil {
			return 0, "", "", nil, fmt.Errorf("failed to read name: %w", err)
		}
		offset = newOffset

		// Extract value
		value, _, err := readPushData(script, offset)
		if err != nil {
			return 0, "", "", nil, fmt.Errorf("failed to read value: %w", err)
		}

		return namedb.NameUpdate, string(name), string(value), nil, nil
	}

	return 0, "", "", nil, fmt.Errorf("not a name operation")
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
		// Block connected to main chain - name operations are already
		// processed in ProcessBlock when isMainChain is true
	case blockchain.NTBlockDisconnected:
		// Block disconnected from main chain (reorg)
		// We need to undo any name operations from this block
		block, ok := notification.Data.(*btcutil.Block)
		if !ok {
			return
		}
		bc.rollbackNameOperations(block)
	}
}

// rollbackNameOperations reverses all name operations from a disconnected block.
// This is called during a blockchain reorganization to maintain consistency
// between the name database and the main chain.
func (bc *BlockChain) rollbackNameOperations(block *btcutil.Block) {
	// Process transactions in reverse order to properly undo operations
	txs := block.Transactions()
	for i := len(txs) - 1; i >= 0; i-- {
		tx := txs[i]
		msgTx := tx.MsgTx()

		// Process outputs in reverse order within the transaction
		for j := len(msgTx.TxOut) - 1; j >= 0; j-- {
			txOut := msgTx.TxOut[j]
			op, name, _, extra, err := parseNameScriptFull(txOut.PkScript)
			if err != nil {
				continue // Not a name operation
			}

			switch op {
			case namedb.NameNew:
				// Rollback NAME_NEW: remove the commitment from the database.
				// extra contains the commitment hash.
				// Note: Deletion may fail if commitment was already consumed by
				// a NAME_FIRSTUPDATE - this is expected and safe to ignore.
				_ = bc.nameDB.DeleteNameNew(extra)

			case namedb.NameFirstUpdate:
				// Rollback NAME_FIRSTUPDATE:
				// 1. Remove the history entry for this operation
				// 2. Delete the name from the database
				// 3. Restore the NAME_NEW commitment that was consumed
				_, _ = bc.nameDB.RemoveLastHistoryEntry(name)
				_ = bc.nameDB.DeleteName(name)

				// Restore the NAME_NEW commitment. The commitment hash is
				// computed from rand (extra) and name.
				//
				// Height estimation: We use block.Height() - MinBlocksBeforeFirstUpdate
				// as a conservative estimate. The actual NAME_NEW could have been
				// created earlier, but this is the earliest possible height. This
				// is safe because:
				// - If a new NAME_FIRSTUPDATE is attempted, it will pass the
				//   MinBlocksBeforeFirstUpdate check since actual elapsed blocks >= min
				// - The exact original height isn't stored, so estimation is necessary
				commitHash := computeCommitHash(extra, name)
				estimatedNameNewHeight := block.Height() - config.MinBlocksBeforeFirstUpdate
				// Ensure height is non-negative (shouldn't happen in practice
				// since NAME_FIRSTUPDATE requires MinBlocksBeforeFirstUpdate to pass)
				if estimatedNameNewHeight < 0 {
					estimatedNameNewHeight = 0
				}
				_ = bc.nameDB.RestoreNameNew(commitHash, estimatedNameNewHeight)

			case namedb.NameUpdate:
				// Rollback NAME_UPDATE:
				// 1. Remove the history entry for this operation
				// 2. Restore the previous value from history
				prevRecord, err := bc.nameDB.RemoveLastHistoryEntry(name)
				if err != nil {
					continue
				}
				if prevRecord != nil {
					// Restore the previous record
					_ = bc.nameDB.PutName(name, prevRecord)
				}
				// If prevRecord is nil, it means there was no previous state,
				// which shouldn't happen for NAME_UPDATE (name should have been
				// registered first), but we handle it gracefully
			}
		}
	}
}

// GetBlockHeader returns a block header by hash
func (bc *BlockChain) GetBlockHeader(hash *chainhash.Hash) (wire.BlockHeader, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.BlockChain.HeaderByHash(hash)
}
