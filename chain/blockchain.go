package chain

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb" // Import ffldb driver
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/metrics"
	"github.com/opd-ai/nmcd/namedb"
)

// BlockChain wraps btcd blockchain with name operation validation
type BlockChain struct {
	*blockchain.BlockChain
	nameDB      *namedb.NameDatabase
	blockDB     database.DB // Block database for blockchain storage
	chainParams *chaincfg.Params
	mu          sync.RWMutex

	// auxPowCache stores AuxPow data temporarily keyed by block hash
	// This is needed because btcd's blockchain package works with btcutil.Block
	// which doesn't have AuxPow fields, but we need to validate AuxPow.
	// The cache is populated when blocks arrive from the network and cleared after validation.
	auxPowCache map[chainhash.Hash]*AuxPow
	auxPowMu    sync.RWMutex
}

// Config holds blockchain configuration
type Config struct {
	ChainParams *chaincfg.Params
	NameDBPath  string
	DataDir     string
	// BlockDBPath is the path to the block database directory.
	// If empty, blocks.db will be created in DataDir.
	BlockDBPath string
}

// NewBlockChain creates a new blockchain with name support
func NewBlockChain(cfg *Config, indexManager blockchain.IndexManager) (*BlockChain, error) {
	// Create name database
	nameDB, err := namedb.NewNameDatabase(cfg.NameDBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create name database: %w", err)
	}

	// Determine block database path
	blockDBPath := cfg.BlockDBPath
	if blockDBPath == "" {
		blockDBPath = filepath.Join(cfg.DataDir, "blocks.db")
	}

	// Get the appropriate wire network based on chain params
	var dbNet wire.BitcoinNet
	switch cfg.ChainParams.Net {
	case config.NamecoinMainNetParams.Net:
		// Use MainNet wire protocol for Namecoin mainnet
		dbNet = wire.MainNet
	case config.NamecoinTestNetParams.Net:
		// Use TestNet3 wire protocol for Namecoin testnet
		dbNet = wire.TestNet3
	case config.NamecoinRegTestParams.Net:
		// Use TestNet wire protocol for Namecoin regtest (regtest reuses TestNet wire format)
		dbNet = wire.TestNet
	default:
		// Default to MainNet
		dbNet = wire.MainNet
	}

	// Create or open block database using ffldb backend
	// ffldb is the recommended database backend for btcd
	blockDB, err := database.Create("ffldb", blockDBPath, dbNet)
	if err != nil {
		// If database already exists, try to open it
		blockDB, err = database.Open("ffldb", blockDBPath, dbNet)
		if err != nil {
			nameDB.Close()
			return nil, fmt.Errorf("failed to create/open block database: %w", err)
		}
	}

	bc := &BlockChain{
		nameDB:      nameDB,
		blockDB:     blockDB,
		chainParams: cfg.ChainParams,
		auxPowCache: make(map[chainhash.Hash]*AuxPow),
	}

	// Create blockchain config
	bcConfig := blockchain.Config{
		DB:               blockDB,
		ChainParams:      cfg.ChainParams,
		TimeSource:       blockchain.NewMedianTime(),
		IndexManager:     indexManager,
		UtxoCacheMaxSize: 250 * 1024 * 1024, // 250 MB UTXO cache (recommended default)
	}

	// Create the blockchain instance
	chain, err := blockchain.New(&bcConfig)
	if err != nil {
		blockDB.Close()
		nameDB.Close()
		return nil, fmt.Errorf("failed to create blockchain: %w", err)
	}

	bc.BlockChain = chain

	// Subscribe to blockchain notifications to handle chain reorganizations.
	// This ensures the name database stays consistent during reorgs by
	// rolling back name operations when blocks are disconnected.
	chain.Subscribe(bc.HandleBlockchainNotification)

	return bc, nil
}

// Close closes the blockchain and name database
func (bc *BlockChain) Close() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	var errs []error

	// Close name database
	if bc.nameDB != nil {
		if err := bc.nameDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close name database: %w", err))
		}
	}

	// Close block database
	if bc.blockDB != nil {
		if err := bc.blockDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close block database: %w", err))
		}
	}

	// Return all errors if any occurred
	if len(errs) > 0 {
		// Join all errors into a single error message
		errMsg := "failed to close blockchain resources:"
		for _, err := range errs {
			errMsg += fmt.Sprintf(" %v;", err)
		}
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

// SetBlockAuxPowFromBytes extracts and caches AuxPow data from a serialized block.
// This is called by the network layer when blocks arrive from peers.
//
// The function:
// 1. Checks if the block version has the AuxPow bit set
// 2. If so, deserializes the entire block including AuxPow
// 3. Caches the AuxPow for later validation
//
// Arguments:
//   - blockHash: Hash of the block (used as cache key)
//   - serializedBlock: Complete serialized block bytes including AuxPow
//
// Returns:
//   - nil if successful or if block doesn't have AuxPow
//   - error if deserialization fails for a block that should have AuxPow
func (bc *BlockChain) SetBlockAuxPowFromBytes(blockHash *chainhash.Hash, serializedBlock []byte) error {
	// Quick check: does the block version indicate AuxPow?
	// Block version is in bytes 0-3 (little-endian int32)
	if len(serializedBlock) < 80 {
		// Block header is incomplete, skip AuxPow parsing
		return nil
	}

	// Extract version from header (bytes 0-3, little-endian)
	version := int32(serializedBlock[0]) | int32(serializedBlock[1])<<8 |
		int32(serializedBlock[2])<<16 | int32(serializedBlock[3])<<24

	hasAuxPowBit := (version & config.AuxPowVersionBit) != 0
	if !hasAuxPowBit {
		// No AuxPow for this block
		return nil
	}

	// Deserialize the full block including AuxPow
	block, err := NewBlockFromBytes(serializedBlock)
	if err != nil {
		return fmt.Errorf("failed to deserialize block with AuxPow: %w", err)
	}

	// Cache the AuxPow for validation
	if block.AuxPow() != nil {
		bc.auxPowMu.Lock()
		bc.auxPowCache[*blockHash] = block.AuxPow()
		bc.auxPowMu.Unlock()
	}

	return nil
}

// getBlockAuxPow retrieves cached AuxPow data for a block.
// Returns nil if no AuxPow is cached for this block.
func (bc *BlockChain) getBlockAuxPow(blockHash *chainhash.Hash) *AuxPow {
	bc.auxPowMu.RLock()
	defer bc.auxPowMu.RUnlock()
	return bc.auxPowCache[*blockHash]
}

// clearBlockAuxPow removes AuxPow data from the cache after validation.
// This prevents unbounded memory growth.
func (bc *BlockChain) clearBlockAuxPow(blockHash *chainhash.Hash) {
	bc.auxPowMu.Lock()
	defer bc.auxPowMu.Unlock()
	delete(bc.auxPowCache, *blockHash)
}

// ProcessBlock processes a block and validates name operations
func (bc *BlockChain) ProcessBlock(block *btcutil.Block, flags blockchain.BehaviorFlags) (bool, bool, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Track processing time for metrics
	startTime := time.Now()

	// Validate proof of work (difficulty) before processing
	// This ensures the block hash meets the difficulty target specified in the block header.
	// While btcd's ProcessBlock also validates this, we perform an explicit check here for:
	// 1. Clear visibility that difficulty validation is happening
	// 2. Early rejection of invalid blocks before expensive processing
	// 3. Explicit verification that Namecoin's PoW parameters are correctly validated
	//
	// Namecoin uses the same difficulty adjustment algorithm as Bitcoin:
	// - Retargets every 2016 blocks
	// - Targets 10 minute block time
	// - Max 4x adjustment per retarget period
	//
	// Note: This validates pre-AuxPoW blocks (< 19,200). AuxPoW blocks (>= 19,200) require
	// additional validation of the parent Bitcoin block, which is not yet implemented.
	if err := bc.validateProofOfWork(block); err != nil {
		metrics.Get().RecordBlockRejected()
		metrics.Get().RecordValidationError("proof_of_work")
		return false, false, fmt.Errorf("invalid proof of work for block %s at height %d: %w",
			block.Hash(), block.Height(), err)
	}

	// Validate block version (AuxPow version bit) before processing
	// This ensures blocks at or after AuxPow activation height have the required version bit set.
	// This is a consensus-critical check that must match Namecoin Core's validation.
	// Blocks that fail this check will be rejected to prevent chain forks.
	if err := bc.validateBlockVersion(block); err != nil {
		metrics.Get().RecordBlockRejected()
		metrics.Get().RecordValidationError("version")
		return false, false, fmt.Errorf("invalid block version for block %s at height %d: %w",
			block.Hash(), block.Height(), err)
	}

	// Validate AuxPow for blocks at or after activation height
	// This checks if the block requires AuxPow validation based on height and version bits.
	// AuxPow blocks (>= 19,200 on mainnet) include merged mining proof that must be validated.
	if err := bc.validateAuxPow(block); err != nil {
		metrics.Get().RecordBlockRejected()
		metrics.Get().RecordValidationError("auxpow")
		return false, false, fmt.Errorf("invalid AuxPow for block %s at height %d: %w",
			block.Hash(), block.Height(), err)
	}

	// Validate block subsidy before processing
	if err := bc.validateBlockSubsidy(block); err != nil {
		metrics.Get().RecordBlockRejected()
		metrics.Get().RecordValidationError("subsidy")
		return false, false, fmt.Errorf("invalid block subsidy for block %s at height %d: %w",
			block.Hash(), block.Height(), err)
	}

	// Validate name operations before processing
	if err := bc.validateNameOperations(block); err != nil {
		metrics.Get().RecordBlockRejected()
		metrics.Get().RecordNameError()
		return false, false, fmt.Errorf("invalid name operations in block %s at height %d: %w",
			block.Hash(), block.Height(), err)
	}

	// Process the block using btcd blockchain
	// btcd will perform additional validations including:
	// - Difficulty retargeting logic (every 2016 blocks)
	// - Timestamp validation
	// - Merkle root verification
	// - Transaction validation
	// - etc.
	isMainChain, isOrphan, err := bc.BlockChain.ProcessBlock(block, flags)
	if err != nil {
		metrics.Get().RecordBlockRejected()
		return isMainChain, isOrphan, err
	}

	// Update name database if block is on main chain
	if isMainChain {
		if err := bc.updateNameDatabase(block); err != nil {
			return isMainChain, isOrphan, fmt.Errorf("failed to update name database for block %s at height %d: %w",
				block.Hash(), block.Height(), err)
		}
	}

	// Record successful block processing
	processingTime := time.Since(startTime)
	metrics.Get().RecordBlockProcessed(block.Hash().String(), block.Height(), isMainChain, isOrphan, processingTime)

	return isMainChain, isOrphan, nil
}

// validateBlockSubsidy validates that the coinbase transaction reward does not exceed
// the maximum allowed block subsidy for the given height.
//
// This is a consensus-critical validation that ensures miners cannot create more coins
// than the protocol allows. The subsidy follows Namecoin's halving schedule:
// - Initial: 50 NMC per block
// - Halves every 210,000 blocks
// - Maximum supply: ~21,000,000 NMC
//
// Note: This function only validates that the reward doesn't EXCEED the maximum.
// It's acceptable for miners to claim less than the maximum (though unusual).
func (bc *BlockChain) validateBlockSubsidy(block *btcutil.Block) error {
	// Get the coinbase transaction (always the first transaction)
	if len(block.Transactions()) == 0 {
		return fmt.Errorf("block has no transactions")
	}

	coinbaseTx := block.Transactions()[0].MsgTx()

	// Calculate total coinbase outputs
	var totalOutput int64
	for _, txOut := range coinbaseTx.TxOut {
		totalOutput += txOut.Value
	}

	// Determine the block height. For network blocks, we derive it from the parent
	// block's height. For test blocks or when the blockchain index isn't available,
	// we fall back to block.Height() if it was explicitly set.
	var height int32 = -1 // -1 indicates height is not yet determined
	prevHash := block.MsgBlock().Header.PrevBlock

	// Try to determine height from the blockchain index
	if bc.BlockChain != nil && !prevHash.IsEqual(&chainhash.Hash{}) {
		parentHeight, err := bc.BlockChain.BlockHeightByHash(&prevHash)
		if err == nil {
			height = parentHeight + 1
		}
	}

	// Fallback: use block.Height() if it was explicitly set
	if height < 0 {
		height = block.Height()
		if height < 0 {
			// Cannot determine height - skip validation
			return nil
		}
	}

	// Calculate maximum allowed subsidy for this block height
	maxSubsidy := config.CalcBlockSubsidy(height, bc.chainParams)

	// In a proper implementation, we should also add transaction fees to maxSubsidy.
	// However, since we don't have full UTXO tracking for all historical blocks,
	// we'll skip fee validation for now. This is documented as a known limitation.
	//
	// For now, we only validate that the coinbase output doesn't exceed the base subsidy.
	// This catches the most egregious cases where miners try to create too many coins.

	if totalOutput > maxSubsidy {
		return fmt.Errorf("coinbase output %d exceeds maximum block subsidy %d at height %d",
			totalOutput, maxSubsidy, height)
	}

	return nil
}

// validateProofOfWork validates that the block hash meets the difficulty target
// specified in the block header's Bits field.
//
// This function ensures:
// 1. The block header Bits field is within the proof of work limit (not too easy)
// 2. The block hash is less than the target difficulty (proof of work completed)
//
// Namecoin uses the same proof of work validation as Bitcoin:
// - SHA-256 double hash of the block header
// - Target difficulty encoded in compact form in the Bits field
// - Block hash must be numerically less than the target
//
// The difficulty adjustment (retargeting) is handled by btcd's blockchain package
// and occurs every 2016 blocks following Bitcoin's algorithm.
//
// Note: This validates pre-AuxPoW blocks. AuxPoW blocks (>= 19,200 on mainnet)
// require additional validation of the parent Bitcoin block's proof of work,
// which is not yet implemented. See PROTOCOL_COMPLIANCE_AUDIT.md Issue #1.
func (bc *BlockChain) validateProofOfWork(block *btcutil.Block) error {
	// Use btcd's CheckProofOfWork function which validates:
	// 1. The target difficulty from Bits is <= PowLimit (from chain params)
	// 2. The block hash is <= target difficulty
	//
	// This uses the PowLimit from our Namecoin chain parameters, ensuring
	// Namecoin-specific limits are enforced.
	return blockchain.CheckProofOfWork(block, bc.chainParams.PowLimit)
}

// validateBlockVersion validates that the block version conforms to Namecoin
// consensus rules, specifically for AuxPow (merged mining) support.
//
// Per Namecoin protocol (from Namecoin Core src/validation.cpp):
// - Blocks before AuxPow activation can have any version (typically version 1)
// - Blocks at or after AuxPow activation MUST have the AuxPow version bit (0x100) set
// - The version bit indicates the block uses merged mining with Bitcoin
//
// Activation heights:
//   - Mainnet: block 19,200 (circa 2011)
//   - Testnet: block 19,200 (same as mainnet)
//   - Regtest: block 999,999,999 (effectively disabled for local testing)
//
// This is a consensus-critical validation. Blocks that don't follow these rules
// will be rejected by Namecoin Core nodes, causing a chain fork.
//
// Note: This function only validates the VERSION BIT. It does NOT validate the
// full AuxPow structure (parent block, merkle proof, etc.) which is not yet
// implemented. See PROTOCOL_COMPLIANCE_AUDIT.md Issue #1.
func (bc *BlockChain) validateBlockVersion(block *btcutil.Block) error {
	// Determine the block height. For network blocks, we derive it from the parent
	// block's height to avoid relying on block.Height() which is not set until after
	// btcd processes the block. For test blocks or when the blockchain index isn't
	// available, we fall back to block.Height() if it was explicitly set.
	var height int32 = -1 // -1 indicates height is not yet determined
	prevHash := block.MsgBlock().Header.PrevBlock

	// Try to determine height from the blockchain index
	if bc.BlockChain != nil && !prevHash.IsEqual(&chainhash.Hash{}) {
		// Look up parent block height from the blockchain index
		parentHeight, err := bc.BlockChain.BlockHeightByHash(&prevHash)
		if err == nil {
			height = parentHeight + 1
		}
	}

	// If we couldn't determine height from the chain, use block.Height() as fallback
	// This handles: (1) test blocks with explicitly set height, (2) genesis blocks,
	// (3) cases where blockchain index is not available
	if height < 0 {
		height = block.Height()
		// If height is still unset (-1), we cannot validate - skip for now
		if height < 0 {
			// Cannot determine height - block will be validated when parent is available
			return nil
		}
	}

	version := block.MsgBlock().Header.Version
	auxPowActivationHeight := config.GetAuxPowActivationHeight(bc.chainParams)

	// Check if this block is at or after AuxPow activation
	if height >= auxPowActivationHeight {
		// Block must have AuxPow version bit set
		// Per Namecoin Core: if (!(nVersion & BLOCK_VERSION_AUXPOW)) return error
		if (version & config.AuxPowVersionBit) == 0 {
			return fmt.Errorf("block version 0x%x at height %d missing required AuxPow version bit 0x%x (activation height: %d)",
				version, height, config.AuxPowVersionBit, auxPowActivationHeight)
		}
	}
	// Pre-AuxPow blocks can have any version (no validation needed)

	return nil
}

// validateAuxPow validates AuxPow (merged mining proof) for blocks that require it.
//
// Per Namecoin consensus rules:
// - Blocks at or after AuxPow activation height (19,200 on mainnet) must have AuxPow
// - AuxPow includes proof that the block was merge-mined with a parent chain (Bitcoin)
// - Validation includes checking merkle branches, parent block PoW, and chain ID
//
// This function:
// 1. Checks if the block should have AuxPow (based on height and version)
// 2. Retrieves cached AuxPow data (set by SetBlockAuxPowFromBytes)
// 3. Calls ValidateAuxPow() to verify the proof
// 4. Clears the cache entry after validation
//
// Returns:
//   - nil if validation succeeds or block doesn't require AuxPow
//   - error if AuxPow is missing or validation fails
func (bc *BlockChain) validateAuxPow(block *btcutil.Block) error {
	// Determine block height
	var height int32 = -1
	prevHash := block.MsgBlock().Header.PrevBlock

	if bc.BlockChain != nil && !prevHash.IsEqual(&chainhash.Hash{}) {
		parentHeight, err := bc.BlockChain.BlockHeightByHash(&prevHash)
		if err == nil {
			height = parentHeight + 1
		}
	}

	if height < 0 {
		height = block.Height()
		if height < 0 {
			// Cannot determine height - skip AuxPow validation
			return nil
		}
	}

	// Check if this block should have AuxPow
	auxPowActivationHeight := config.GetAuxPowActivationHeight(bc.chainParams)
	if height < auxPowActivationHeight {
		// Pre-AuxPow block - no validation needed
		return nil
	}

	// Block requires AuxPow validation
	version := block.MsgBlock().Header.Version
	hasAuxPowBit := (version & config.AuxPowVersionBit) != 0

	if !hasAuxPowBit {
		// Block version validation should have caught this, but double-check
		return fmt.Errorf("block at height %d requires AuxPow version bit but it's not set", height)
	}

	// Retrieve cached AuxPow data
	blockHash := block.Hash()
	auxPow := bc.getBlockAuxPow(blockHash)

	// Ensure we clean up the cache entry when done (success or failure)
	defer bc.clearBlockAuxPow(blockHash)

	if auxPow == nil {
		return fmt.Errorf("block at height %d requires AuxPow but no AuxPow data was provided", height)
	}

	// Get the target difficulty for the parent block
	// For merged mining, the parent block (Bitcoin) must meet a difficulty target.
	// We use the current block's difficulty target from its Bits field.
	targetDifficulty := blockchain.CompactToBig(block.MsgBlock().Header.Bits)

	// Validate the AuxPow proof
	// This checks:
	// 1. Chain ID matches Namecoin (NamecoinChainID = 1)
	// 2. Parent block hash meets difficulty target
	// 3. Coinbase merkle branch proves coinbase is in parent block
	// 4. Chain merkle branch proves aux block hash is committed in coinbase
	//
	// Note: We need to convert targetDifficulty (big.Int) to a Hash for ValidateAuxPow.
	// blockchain.HashToBig treats hash bytes as little-endian, so we must reverse
	// the big-endian bytes from big.Int.
	var targetHash chainhash.Hash
	targetBytes := targetDifficulty.Bytes()

	// Reverse bytes: big.Int is big-endian, Hash (for HashToBig) is little-endian
	for i := 0; i < len(targetBytes); i++ {
		targetHash[len(targetBytes)-1-i] = targetBytes[i]
	}

	if err := auxPow.ValidateAuxPow(blockHash, NamecoinChainID, &targetHash); err != nil {
		return fmt.Errorf("AuxPow validation failed for block %s at height %d: %w",
			blockHash.String(), height, err)
	}

	// All AuxPow validations passed
	log.Printf("Successfully validated AuxPow for block %s at height %d", blockHash.String(), height)
	return nil
}

// validateNameOperations validates name operations in a block
func (bc *BlockChain) validateNameOperations(block *btcutil.Block) error {
	// Determine the block height. For network blocks, we derive it from the parent
	// block's height. For test blocks or when the blockchain index isn't available,
	// we fall back to block.Height() if it was explicitly set.
	var height int32 = -1 // -1 indicates height is not yet determined
	prevHash := block.MsgBlock().Header.PrevBlock

	// Try to determine height from the blockchain index
	if bc.BlockChain != nil && !prevHash.IsEqual(&chainhash.Hash{}) {
		parentHeight, err := bc.BlockChain.BlockHeightByHash(&prevHash)
		if err == nil {
			height = parentHeight + 1
		}
	}

	// Fallback: use block.Height() if it was explicitly set
	if height < 0 {
		height = block.Height()
		if height < 0 {
			// Cannot determine height - skip validation
			return nil
		}
	}

	// Track NAME_NEW commitment hashes seen in this block to detect duplicates.
	// Using string conversion of byte slice as map key is idiomatic in Go.
	seenNameNewCommits := make(map[string]bool)

	// Track names seen in this block to prevent double-spending
	// (multiple NAME_FIRSTUPDATE or NAME_UPDATE operations for the same name)
	seenNames := make(map[string]bool)

	for txIdx, tx := range block.Transactions() {
		msgTx := tx.MsgTx()

		// Detect and validate name operations in transaction outputs
		nameOpTypes := make(map[namedb.NameOperation]struct{})
		for _, txOut := range msgTx.TxOut {
			op, _, _, _, err := parseNameScriptFull(txOut.PkScript)
			if err != nil {
				continue // Not a name operation
			}
			nameOpTypes[op] = struct{}{}
		}

		// Validate transaction fee for name operations.
		// Skip coinbase transaction (no inputs to validate).
		if len(nameOpTypes) > 0 && txIdx > 0 {
			txHash := msgTx.TxHash()
			for opType := range nameOpTypes {
				if err := bc.validateTransactionFee(msgTx, opType, height); err != nil {
					return fmt.Errorf("invalid transaction fee for %s in tx %s: %w", opType, txHash, err)
				}
			}
		}

		// Check for name operations in transaction outputs
		for _, txOut := range msgTx.TxOut {
			op, name, value, extra, err := parseNameScriptFull(txOut.PkScript)
			if err != nil {
				continue // Not a name operation
			}

			// Get transaction hash for error messages
			txHash := msgTx.TxHash()

			switch op {
			case namedb.NameNew:
				// Validate NAME_NEW output value meets dust limit
				// This prevents spam and uneconomical UTXO creation
				if txOut.Value < config.DustLimit {
					return fmt.Errorf("name_new output value %d below dust limit %d in tx %s",
						txOut.Value, config.DustLimit, txHash)
				}

				// Check for duplicate commitment hash in this block
				commitHashStr := string(extra)
				if seenNameNewCommits[commitHashStr] {
					return fmt.Errorf("duplicate name_new commitment in block (tx: %s)", txHash)
				}
				seenNameNewCommits[commitHashStr] = true

				// Check if commitment already exists in database
				if _, err := bc.nameDB.GetNameNew(extra); err == nil {
					return fmt.Errorf("name_new commitment already exists (tx: %s)", txHash)
				}

			case namedb.NameFirstUpdate:
				// Validate NAME_FIRSTUPDATE output value meets dust limit
				if txOut.Value < config.DustLimit {
					return fmt.Errorf("name_firstupdate output value %d below dust limit %d in tx %s",
						txOut.Value, config.DustLimit, txHash)
				}

				// Check for duplicate name operation in this block
				if seenNames[name] {
					return fmt.Errorf("duplicate name operation in block for name: %s (tx: %s)", name, txHash)
				}
				seenNames[name] = true

				// Verify name doesn't exist
				if _, err := bc.nameDB.GetName(name); err == nil {
					return fmt.Errorf("name already exists: %s (tx: %s)", name, txHash)
				}

				// Compute the commitment hash from rand (extra), name, and chain ID
				// This prevents cross-chain replay attacks
				commitHash := computeCommitHash(extra, name, bc.chainParams)

				// Verify NAME_NEW exists and MinBlocksBeforeFirstUpdate has passed
				nameNewRecord, err := bc.nameDB.GetNameNew(commitHash)
				if err != nil {
					return fmt.Errorf("no matching name_new found for name: %s (tx: %s)", name, txHash)
				}

				// Check that enough blocks have passed since NAME_NEW
				// Handle edge case where height < nameNewRecord.Height (e.g., during reorg)
				if height < nameNewRecord.Height {
					return fmt.Errorf("name_firstupdate before name_new: block %d < name_new block %d (name: '%s', tx: %s)",
						height, nameNewRecord.Height, name, txHash)
				}
				blocksSinceNew := height - nameNewRecord.Height
				if blocksSinceNew < config.MinBlocksBeforeFirstUpdate {
					return fmt.Errorf("name_firstupdate too early: %d blocks since name_new, minimum %d required (name: '%s', tx: %s)",
						blocksSinceNew, config.MinBlocksBeforeFirstUpdate, name, txHash)
				}
				// Validate maximum timing window - NAME_NEW commitment expires after MaxBlocksBeforeFirstUpdate
				if blocksSinceNew > config.MaxBlocksBeforeFirstUpdate {
					return fmt.Errorf("name_firstupdate too late: %d blocks since name_new, maximum %d allowed (commitment expired) (name: '%s', tx: %s)",
						blocksSinceNew, config.MaxBlocksBeforeFirstUpdate, name, txHash)
				}

			case namedb.NameUpdate:
				// Validate NAME_UPDATE output value meets dust limit
				if txOut.Value < config.DustLimit {
					return fmt.Errorf("name_update output value %d below dust limit %d in tx %s",
						txOut.Value, config.DustLimit, txHash)
				}

				// Check for duplicate name operation in this block
				if seenNames[name] {
					return fmt.Errorf("duplicate name operation in block for name: %s (tx: %s)", name, txHash)
				}
				seenNames[name] = true

				// Verify name exists and not expired
				record, err := bc.nameDB.GetName(name)
				if err != nil {
					return fmt.Errorf("name not found for update: %s (tx: %s)", name, txHash)
				}
				if record.ExpiresAt <= height {
					return fmt.Errorf("name expired: %s (expires at block %d, current %d, tx: %s)",
						name, record.ExpiresAt, height, txHash)
				}

				// UTXO chain validation: Verify the transaction spends the current name UTXO
				// This prevents name theft by ensuring only the current owner can update
				currentUTXO := wire.OutPoint{
					Hash:  record.TxHash,
					Index: record.OutIndex,
				}

				// Check if any transaction input spends the current name UTXO
				found := false
				for _, txIn := range msgTx.TxIn {
					if txIn.PreviousOutPoint.Hash.IsEqual(&currentUTXO.Hash) &&
						txIn.PreviousOutPoint.Index == currentUTXO.Index {
						found = true
						break
					}
				}

				if !found {
					return fmt.Errorf("name_update does not spend current name UTXO (tx=%s, out=%d): name theft attempt for %s (tx: %s)",
						currentUTXO.Hash.String(), currentUTXO.Index, name, txHash)
				}
			}

			// Validate name format and value size (not applicable to NAME_NEW which has no name field)
			if op != namedb.NameNew {
				if err := validateNameFormat(name, value); err != nil {
					return fmt.Errorf("%w (name: '%s', tx: %s)", err, name, txHash)
				}
			}
		}
	}

	return nil
}

// validateTransactionFee validates that a transaction with name operations
// pays the required minimum fee. Transaction fee is calculated as:
// fee = total_input_value - total_output_value
//
// Fee requirements per Namecoin protocol:
// - NAME_NEW: Standard minimum relay fee (1000 satoshis)
// - NAME_FIRSTUPDATE: 0.01 NMC (1,000,000 satoshis) network fee (destroyed/burned)
// - NAME_UPDATE: 0.01 NMC (1,000,000 satoshis) network fee (destroyed/burned)
//
// The network fee for NAME_FIRSTUPDATE and NAME_UPDATE is "destroyed" by making
// it part of the transaction fee, which reduces the total coin supply.
//
// Historical Block Handling:
// For blocks before config.UTXOTrackingStartHeight, fee validation is lenient.
// If UTXO data is missing for such blocks, validation is skipped with a warning
// rather than failing. This allows syncing of historical blocks that predate
// UTXO tracking in this implementation. Strict validation applies for blocks
// at or above UTXOTrackingStartHeight.
func (bc *BlockChain) validateTransactionFee(tx *wire.MsgTx, opType namedb.NameOperation, height int32) error {
	// Calculate total input value by looking up previous outputs
	var totalInputValue int64
	var missingUTXOs bool

	for _, txIn := range tx.TxIn {
		// Look up the UTXO being spent
		utxo, err := bc.nameDB.GetUTXO(&txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index)
		if err != nil {
			// UTXO not found in our database. This could happen for:
			// 1. Historical blocks before UTXO tracking started
			// 2. Blocks being validated before they're added to our UTXO set
			// 3. Database inconsistencies

			// For historical blocks (before UTXO tracking), allow missing UTXOs
			if height < config.UTXOTrackingStartHeight {
				log.Printf("Info: Skipping fee validation for historical block %d - UTXO not found: %s:%d",
					height, txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index)
				missingUTXOs = true
				break // Cannot validate fee without all input values
			}

			// For recent blocks, missing UTXOs indicate a problem
			log.Printf("Warning: Cannot validate transaction fee at height %d - UTXO not found: %s:%d",
				height, txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index)
			return fmt.Errorf("cannot validate transaction fee: UTXO %s:%d not found at height %d: %w",
				txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index, height, err)
		}

		// Check for overflow when adding input values
		// This prevents integer overflow attacks where sum of inputs wraps around
		if totalInputValue > 0 && utxo.Value > 0 && totalInputValue > (1<<63-1)-utxo.Value {
			return fmt.Errorf("transaction input value overflow: %d + %d", totalInputValue, utxo.Value)
		}
		totalInputValue += utxo.Value
	}

	// If we're dealing with a historical block and missing UTXOs, skip validation
	// This allows syncing of old blocks without complete UTXO data
	if missingUTXOs {
		log.Printf("Info: Skipping fee validation for historical block %d due to missing UTXO data", height)
		return nil
	}

	// Calculate total output value
	var totalOutputValue int64
	for _, txOut := range tx.TxOut {
		// Check for overflow when adding output values
		if totalOutputValue > 0 && txOut.Value > 0 && totalOutputValue > (1<<63-1)-txOut.Value {
			return fmt.Errorf("transaction output value overflow: %d + %d", totalOutputValue, txOut.Value)
		}
		totalOutputValue += txOut.Value
	}

	// Calculate fee (inputs - outputs)
	fee := totalInputValue - totalOutputValue
	if fee < 0 {
		return fmt.Errorf("transaction fee cannot be negative: %d satoshis", fee)
	}

	// Validate minimum fee based on operation type
	var minFee int64
	switch opType {
	case namedb.NameNew:
		// NAME_NEW requires standard minimum relay fee
		minFee = config.MinRelayTxFee
	case namedb.NameFirstUpdate, namedb.NameUpdate:
		// NAME_FIRSTUPDATE and NAME_UPDATE require 0.01 NMC network fee
		minFee = config.MinNameOperationFee
	default:
		// Unknown operation type - should not happen
		return fmt.Errorf("unknown name operation type: %d", opType)
	}

	if fee < minFee {
		return fmt.Errorf("transaction fee %d satoshis below minimum %d satoshis for %s",
			fee, minFee, opType)
	}

	return nil
}

// updateNameDatabase updates the name database with operations from a block
func (bc *BlockChain) updateNameDatabase(block *btcutil.Block) error {
	height := block.Height()
	// Use block timestamp for deterministic replay and historical accuracy
	blockTime := block.MsgBlock().Header.Timestamp

	// Handle expired names
	expired, err := bc.nameDB.GetExpiredNames(height)
	if err != nil {
		return err
	}
	for _, name := range expired {
		// Delete the name record
		if err := bc.nameDB.DeleteName(name); err != nil {
			return err
		}
		// Clean up history entries for the expired name to prevent storage waste
		if err := bc.nameDB.DeleteHistory(name); err != nil {
			return err
		}
		// Record name expiration metric
		metrics.Get().RecordNameExpired()
	}

	// Process name operations and track UTXOs
	for txIdx, tx := range block.Transactions() {
		msgTx := tx.MsgTx()
		txHash := tx.Hash()

		// Skip coinbase transaction for input processing
		if txIdx > 0 {
			// Process spent UTXOs (inputs)
			// Store spent UTXOs before removing them for potential restoration during reorg
			for _, txIn := range msgTx.TxIn {
				// Try to get the UTXO before removing it
				utxo, err := bc.nameDB.GetUTXO(&txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index)
				if err == nil && utxo != nil {
					// Store the spent UTXO for potential restoration during reorgs.
					// This is best-effort storage: failures are logged but do not block
					// block processing to avoid stalling the chain on bookkeeping issues.
					if err := bc.nameDB.StoreSpentUTXO(utxo, height); err != nil {
						log.Printf("Warning: Failed to store spent UTXO %s:%d at height %d: %v",
							txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index, height, err)
					}
				}

				// Remove the UTXO from active set
				if err := bc.nameDB.RemoveUTXO(&txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index); err != nil {
					// UTXO might not exist (e.g., old block before UTXO tracking was implemented)
					// This is normal and not an error condition
					log.Printf("Info: Could not remove UTXO %s:%d (may not exist): %v",
						txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index, err)
				}
			}
		}

		// Add new UTXOs and process name operations (process outputs)
		for outIdx, txOut := range msgTx.TxOut {
			// Try to extract address from the script for UTXO tracking
			_, addresses, _, err := txscript.ExtractPkScriptAddrs(txOut.PkScript, bc.chainParams)
			var address string
			if err == nil && len(addresses) > 0 {
				address = addresses[0].EncodeAddress()
			}

			// Create UTXO entry
			utxo := &namedb.UTXO{
				TxHash:   *txHash,
				OutIndex: uint32(outIdx),
				Value:    txOut.Value,
				Address:  address,
				PkScript: txOut.PkScript,
				Height:   height,
			}
			if err := bc.nameDB.AddUTXO(utxo); err != nil {
				return fmt.Errorf("failed to add UTXO %s:%d: %w", txHash, outIdx, err)
			}

			// Parse and process name operations
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
				// Record name operation metric
				metrics.Get().RecordNameOperation("NAME_NEW")

			case namedb.NameFirstUpdate:
				// Extract the owner address from the script
				address := extractAddressFromNameScript(txOut.PkScript, bc.chainParams)

				// Retrieve the NAME_NEW record before deleting it so we can store
				// the original height for accurate reorg handling
				commitHash := computeCommitHash(extra, name, bc.chainParams)
				nameNewRecord, err := bc.nameDB.GetNameNew(commitHash)
				var nameNewHeight int32
				if err == nil && nameNewRecord != nil {
					// Use the exact NAME_NEW height from the database
					nameNewHeight = nameNewRecord.Height
				} else {
					// Fallback estimation for cases where NAME_NEW record is not found.
					// This can occur during database upgrades or if processing old blocks
					// where NAME_NEW was not properly tracked. Uses conservative estimate
					// based on minimum timing requirement.
					nameNewHeight = height - config.MinBlocksBeforeFirstUpdate
					if nameNewHeight < 0 {
						nameNewHeight = 0
					}
				}

				record := &namedb.NameRecord{
					Name:          name,
					Value:         value,
					TxHash:        *txHash,
					OutIndex:      uint32(outIdx),
					Height:        height,
					ExpiresAt:     height + config.NameExpirationBlocks,
					Address:       address,
					UpdatedAt:     blockTime,
					NameNewHeight: nameNewHeight, // Store for accurate rollback
				}
				if err := bc.nameDB.PutName(name, record); err != nil {
					return err
				}
				if err := bc.nameDB.AddHistory(*txHash, record); err != nil {
					return err
				}
				// Clean up the NAME_NEW commitment after successful registration
				if err := bc.nameDB.DeleteNameNew(commitHash); err != nil {
					return err
				}
				// Record name operation metric
				metrics.Get().RecordNameOperation("NAME_FIRSTUPDATE")

			case namedb.NameUpdate:
				// Extract the owner address from the script
				address := extractAddressFromNameScript(txOut.PkScript, bc.chainParams)

				// Preserve the NameNewHeight from the previous record (if available)
				// This is needed for accurate rollback if this update is later rolled back
				var nameNewHeight int32
				prevRecord, err := bc.nameDB.GetName(name)
				if err == nil && prevRecord != nil {
					nameNewHeight = prevRecord.NameNewHeight
				}

				record := &namedb.NameRecord{
					Name:          name,
					Value:         value,
					TxHash:        *txHash,
					OutIndex:      uint32(outIdx),
					Height:        height,
					ExpiresAt:     height + config.NameExpirationBlocks,
					Address:       address,
					UpdatedAt:     blockTime,
					NameNewHeight: nameNewHeight, // Preserve from previous record
				}
				if err := bc.nameDB.PutName(name, record); err != nil {
					return err
				}
				if err := bc.nameDB.AddHistory(*txHash, record); err != nil {
					return err
				}
				// Record name operation metric
				metrics.Get().RecordNameOperation("NAME_UPDATE")
			}
		}
	}

	// Cleanup old spent UTXOs periodically
	// Keep spent UTXOs for the last 1000 blocks to handle potential reorgs
	// This prevents unbounded growth of the spent UTXO bucket
	const spentUtxoRetentionDepth = 1000
	if height > spentUtxoRetentionDepth && height%100 == 0 { // Cleanup every 100 blocks
		cleanupHeight := height - spentUtxoRetentionDepth
		if err := bc.nameDB.CleanupOldSpentUTXOs(cleanupHeight); err != nil {
			// Log but don't fail the block - cleanup is best-effort
			log.Printf("Warning: Failed to cleanup old spent UTXOs at height %d: %v", height, err)
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

// GetNameUTXO retrieves the UTXO that holds a specific name
func (bc *BlockChain) GetNameUTXO(name string) (*namedb.UTXO, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.nameDB.GetNameUTXO(name)
}

// GetUTXOsForAddress retrieves all UTXOs for a specific address
func (bc *BlockChain) GetUTXOsForAddress(address string) ([]*namedb.UTXO, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.nameDB.GetUTXOsForAddress(address)
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

	// opDrop removes the top stack item
	opDrop = 0x75

	// op2Drop removes the top two stack items
	op2Drop = 0x6d
)

// computeCommitHash computes the NAME_NEW commitment hash with chain ID.
// The commitment is RIPEMD160(SHA256(rand || name || chainID)) to prevent
// cross-chain replay attacks. The chain ID is derived from the network magic bytes.
// This ensures that NAME_NEW commitments are network-specific and cannot be
// replayed across mainnet, testnet, or regtest networks.
//
// Parameters:
//   - rand: Random salt value from NAME_NEW
//   - name: Name to be registered
//   - chainParams: Network parameters containing the unique network magic bytes
//
// Returns: 20-byte commitment hash (RIPEMD160(SHA256(data)))
func computeCommitHash(rand []byte, name string, chainParams *chaincfg.Params) []byte {
	nameBytes := []byte(name)
	// Extract network magic bytes as chain ID (4 bytes)
	chainID := make([]byte, 4)
	chainID[0] = byte(chainParams.Net)
	chainID[1] = byte(chainParams.Net >> 8)
	chainID[2] = byte(chainParams.Net >> 16)
	chainID[3] = byte(chainParams.Net >> 24)

	// Concatenate: rand || name || chainID
	data := make([]byte, len(rand)+len(nameBytes)+len(chainID))
	copy(data, rand)
	copy(data[len(rand):], nameBytes)
	copy(data[len(rand)+len(nameBytes):], chainID)

	return btcutil.Hash160(data)
}

// parseNameScript extracts name operation from script.
// Namecoin scripts use Bitcoin's push data format with length-prefixed data.
// Returns the operation type, name, value, and any parsing error.
func parseNameScript(script []byte) (namedb.NameOperation, string, string, error) {
	op, name, value, _, err := parseNameScriptFull(script)
	return op, name, value, err
}

// validateScriptFormat validates the strict format of a Namecoin name operation script.
// This enforces consensus rules by ensuring drop opcodes and P2PKH suffix are correctly placed.
// Returns the offset after the drop opcodes where the P2PKH script begins, or an error if invalid.
//
// Expected formats per Namecoin Core:
//   - NAME_NEW: OP_NAME_NEW <hash> OP_2DROP <P2PKH>
//   - NAME_FIRSTUPDATE: OP_NAME_FIRSTUPDATE <name> <rand> <value> OP_2DROP OP_2DROP <P2PKH>
//   - NAME_UPDATE: OP_NAME_UPDATE <name> <value> OP_2DROP OP_DROP <P2PKH>
//
// The P2PKH suffix must be at least 25 bytes (standard P2PKH script size).
func validateScriptFormat(script []byte, opType namedb.NameOperation, dataEndOffset int) (int, error) {
	if dataEndOffset >= len(script) {
		return 0, fmt.Errorf("script ends after name operation data, missing drop opcodes")
	}

	offset := dataEndOffset

	switch opType {
	case namedb.NameNew:
		// NAME_NEW requires OP_2DROP after the hash
		if offset >= len(script) || script[offset] != op2Drop {
			return 0, fmt.Errorf("NAME_NEW script missing required OP_2DROP (0x6d) at offset %d", offset)
		}
		offset++

	case namedb.NameFirstUpdate:
		// NAME_FIRSTUPDATE requires OP_2DROP OP_2DROP after name, rand, value
		if offset >= len(script) || script[offset] != op2Drop {
			return 0, fmt.Errorf("NAME_FIRSTUPDATE script missing first OP_2DROP (0x6d) at offset %d", offset)
		}
		offset++
		if offset >= len(script) || script[offset] != op2Drop {
			return 0, fmt.Errorf("NAME_FIRSTUPDATE script missing second OP_2DROP (0x6d) at offset %d", offset)
		}
		offset++

	case namedb.NameUpdate:
		// NAME_UPDATE requires OP_2DROP OP_DROP after name and value
		if offset >= len(script) || script[offset] != op2Drop {
			return 0, fmt.Errorf("NAME_UPDATE script missing required OP_2DROP (0x6d) at offset %d", offset)
		}
		offset++
		if offset >= len(script) || script[offset] != opDrop {
			return 0, fmt.Errorf("NAME_UPDATE script missing required OP_DROP (0x75) at offset %d", offset)
		}
		offset++

	default:
		return 0, fmt.Errorf("unknown name operation type: %d", opType)
	}

	// Validate P2PKH suffix exists and has minimum valid length
	// Standard P2PKH script is 25 bytes: OP_DUP OP_HASH160 <20 bytes> OP_EQUALVERIFY OP_CHECKSIG
	const minP2PKHSize = 25
	remainingBytes := len(script) - offset
	if remainingBytes < minP2PKHSize {
		return 0, fmt.Errorf("P2PKH suffix too short: %d bytes (minimum %d bytes required)", remainingBytes, minP2PKHSize)
	}

	return offset, nil
}

// parseNameScriptFull extracts name operation from script with additional data.
// Returns the operation type, name, value, extra data (hash for NAME_NEW, rand for
// NAME_FIRSTUPDATE), and any parsing error.
//
// This function enforces strict script format validation per Namecoin Core consensus rules.
// Scripts must include proper drop opcodes (OP_2DROP, OP_DROP) and P2PKH suffix.
func parseNameScriptFull(script []byte) (namedb.NameOperation, string, string, []byte, error) {
	if len(script) < 2 {
		return 0, "", "", nil, fmt.Errorf("script too short")
	}

	var opType namedb.NameOperation
	var name, value string
	var extra []byte
	var dataEndOffset int

	switch script[0] {
	case opNameNew:
		// NAME_NEW: OP_NAME_NEW <hash> OP_2DROP <P2PKH>
		// Extract the commitment hash (typically 20 bytes)
		hash, newOffset, err := readPushData(script, 1)
		if err != nil {
			return 0, "", "", nil, fmt.Errorf("failed to read hash: %w", err)
		}
		opType = namedb.NameNew
		extra = hash
		dataEndOffset = newOffset

	case opNameFirstUpdate:
		// NAME_FIRSTUPDATE: OP_NAME_FIRSTUPDATE <name> <rand> <value> OP_2DROP OP_2DROP <P2PKH>
		offset := 1

		// Extract name
		nameBytes, newOffset, err := readPushData(script, offset)
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
		valueBytes, newOffset, err := readPushData(script, offset)
		if err != nil {
			return 0, "", "", nil, fmt.Errorf("failed to read value: %w", err)
		}

		opType = namedb.NameFirstUpdate
		name = string(nameBytes)
		value = string(valueBytes)
		extra = rand
		dataEndOffset = newOffset

	case opNameUpdate:
		// NAME_UPDATE: OP_NAME_UPDATE <name> <value> OP_2DROP OP_DROP <P2PKH>
		offset := 1

		// Extract name
		nameBytes, newOffset, err := readPushData(script, offset)
		if err != nil {
			return 0, "", "", nil, fmt.Errorf("failed to read name: %w", err)
		}
		offset = newOffset

		// Extract value
		valueBytes, newOffset, err := readPushData(script, offset)
		if err != nil {
			return 0, "", "", nil, fmt.Errorf("failed to read value: %w", err)
		}

		opType = namedb.NameUpdate
		name = string(nameBytes)
		value = string(valueBytes)
		dataEndOffset = newOffset

	default:
		return 0, "", "", nil, fmt.Errorf("not a name operation")
	}

	// Validate strict script format (drop opcodes and P2PKH suffix)
	_, err := validateScriptFormat(script, opType, dataEndOffset)
	if err != nil {
		return 0, "", "", nil, fmt.Errorf("invalid script format: %w", err)
	}

	return opType, name, value, extra, nil
}

// extractAddressFromNameScript extracts the owner address from a name operation script.
// Namecoin name scripts have the format: <name_op> <data...> <drop_ops> <P2PKH script>
// This function parses past the name operation data and drop opcodes to extract
// the address from the embedded P2PKH script.
// Returns an empty string if the address cannot be extracted.
//
// Note: This function is called after script validation, so it can safely skip
// past the drop opcodes that have already been validated by parseNameScriptFull.
func extractAddressFromNameScript(script []byte, chainParams *chaincfg.Params) string {
	if len(script) < 2 || chainParams == nil {
		return ""
	}

	var offset int
	switch script[0] {
	case opNameNew:
		// NAME_NEW: OP_NAME_NEW <hash> OP_2DROP <P2PKH>
		_, newOffset, err := readPushData(script, 1)
		if err != nil {
			return ""
		}
		offset = newOffset
		// Skip OP_2DROP (0x6d)
		if offset < len(script) && script[offset] == 0x6d {
			offset++
		}

	case opNameFirstUpdate:
		// NAME_FIRSTUPDATE: OP_NAME_FIRSTUPDATE <name> <rand> <value> OP_2DROP OP_2DROP <P2PKH>
		offset = 1
		// Skip name
		_, newOffset, err := readPushData(script, offset)
		if err != nil {
			return ""
		}
		offset = newOffset
		// Skip rand
		_, newOffset, err = readPushData(script, offset)
		if err != nil {
			return ""
		}
		offset = newOffset
		// Skip value
		_, newOffset, err = readPushData(script, offset)
		if err != nil {
			return ""
		}
		offset = newOffset
		// Skip OP_2DROP OP_2DROP (0x6d 0x6d)
		for i := 0; i < 2 && offset < len(script) && script[offset] == 0x6d; i++ {
			offset++
		}

	case opNameUpdate:
		// NAME_UPDATE: OP_NAME_UPDATE <name> <value> OP_2DROP OP_DROP <P2PKH>
		offset = 1
		// Skip name
		_, newOffset, err := readPushData(script, offset)
		if err != nil {
			return ""
		}
		offset = newOffset
		// Skip value
		_, newOffset, err = readPushData(script, offset)
		if err != nil {
			return ""
		}
		offset = newOffset
		// Skip OP_2DROP (0x6d)
		if offset < len(script) && script[offset] == 0x6d {
			offset++
		}
		// Skip OP_DROP (0x75)
		if offset < len(script) && script[offset] == 0x75 {
			offset++
		}

	default:
		return ""
	}

	// Extract the P2PKH script portion
	if offset >= len(script) {
		return ""
	}
	p2pkhScript := script[offset:]

	// Use txscript to extract the address from the P2PKH portion
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(p2pkhScript, chainParams)
	if err != nil || len(addrs) == 0 {
		return ""
	}

	return addrs[0].EncodeAddress()
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

	// Validate namespace prefix
	if !config.IsValidNamespace(name) {
		return fmt.Errorf("invalid namespace: name must start with a valid namespace prefix (d/, id/, p/)")
	}

	// Ensure there is content after the namespace prefix
	// Check each valid namespace to find which one matches and verify content exists after it
	hasContent := false
	for _, ns := range config.ValidNamespaces {
		if len(name) >= len(ns) && name[:len(ns)] == ns {
			if len(name) > len(ns) {
				hasContent = true
				break
			}
		}
	}
	if !hasContent {
		return fmt.Errorf("invalid name: must have content after namespace prefix")
	}

	if len(value) > config.MaxValueLength {
		return fmt.Errorf("value too large: %d bytes (max: %d)", len(value), config.MaxValueLength)
	}

	// Validate value encoding based on namespace
	// Per Namecoin protocol, different namespaces have different encoding requirements
	if err := validateValueEncoding(name, value); err != nil {
		return err
	}

	return nil
}

// validateValueEncoding validates the encoding of a name value based on its namespace.
// Per this implementation:
// - d/ (domain) namespace: values must be valid UTF-8 and must be valid JSON for DNS records
// - id/ (identity) namespace: values must be valid UTF-8 and must be valid JSON
// - p/ (personal) namespace: values must be valid UTF-8; JSON is optional and not enforced
func validateValueEncoding(name, value string) error {
	// Empty values are allowed (deletion/reservation pattern)
	if len(value) == 0 {
		return nil
	}

	// All namespaces require valid UTF-8 encoding
	if !utf8.ValidString(value) {
		return fmt.Errorf("value must be valid UTF-8")
	}

	// For d/ (domain) and id/ (identity) namespaces, validate JSON encoding
	// These namespaces store structured data (DNS records, identity records)
	if (len(name) >= 2 && name[:2] == "d/") || (len(name) >= 3 && name[:3] == "id/") {
		// Attempt to parse as JSON
		var jsonData interface{}
		if err := json.Unmarshal([]byte(value), &jsonData); err != nil {
			ns := "specified"
			if len(name) >= 2 && name[:2] == "d/" {
				ns = "d/"
			} else if len(name) >= 3 && name[:3] == "id/" {
				ns = "id/"
			}
			return fmt.Errorf("value must be valid JSON for %s namespace: %w", ns, err)
		}
	}

	// For p/ (personal) namespace, only UTF-8 validation is required
	// Personal namespace is more flexible and can contain arbitrary text

	return nil
}

// BestSnapshot returns the current best chain snapshot
func (bc *BlockChain) BestSnapshot() *blockchain.BestState {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.BlockChain.BestSnapshot()
}

// ChainParams returns the chain parameters
func (bc *BlockChain) ChainParams() *chaincfg.Params {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.chainParams
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
		// Record blockchain reorganization metric
		// Count this as 1 block rolled back (in a multi-block reorg, this will be called multiple times)
		metrics.Get().RecordReorg(1)
	}
}

// rollbackNameOperations reverses all name operations from a disconnected block.
// This is called during a blockchain reorganization to maintain consistency
// between the name database and the main chain.
func (bc *BlockChain) rollbackNameOperations(block *btcutil.Block) {
	// Track NAME_NEW commitments that are restored during this rollback.
	// When a NAME_FIRSTUPDATE is rolled back, it restores the NAME_NEW commitment.
	// If the same block also contains that NAME_NEW, we must NOT delete it
	// during the NAME_NEW rollback, as it was restored for a reason (the
	// NAME_FIRSTUPDATE that consumed it is also being rolled back).
	restoredCommitments := make(map[string]bool)

	// Process transactions in reverse order to properly undo operations
	txs := block.Transactions()
	for i := len(txs) - 1; i >= 0; i-- {
		tx := txs[i]
		msgTx := tx.MsgTx()
		txHash := tx.Hash()

		// Rollback UTXOs: remove outputs created by this block
		for outIdx := range msgTx.TxOut {
			_ = bc.nameDB.RemoveUTXO(txHash, uint32(outIdx))
		}

		// Restore UTXOs: restore inputs that were spent by this block
		// Skip coinbase (has no real inputs)
		// Note: The actual restoration is done in batch after this loop
		// via RestoreSpentUTXOsForBlock for efficiency

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
				//
				// Skip deletion if this commitment was restored during rollback
				// of a NAME_FIRSTUPDATE in the same block. This handles the case
				// where both NAME_NEW and NAME_FIRSTUPDATE are in the same block.
				commitHashKey := string(extra)
				if restoredCommitments[commitHashKey] {
					continue
				}
				_ = bc.nameDB.DeleteNameNew(extra)

			case namedb.NameFirstUpdate:
				// Rollback NAME_FIRSTUPDATE:
				// 1. Retrieve the name record to get the original NAME_NEW height
				// 2. Remove the history entry for this operation
				// 3. Delete the name from the database
				// 4. Restore the NAME_NEW commitment that was consumed with exact height

				// Retrieve the name record before deleting to get NameNewHeight
				nameRecord, err := bc.nameDB.GetName(name)
				var nameNewHeight int32
				if err == nil && nameRecord != nil && nameRecord.NameNewHeight != 0 {
					// Use the exact NAME_NEW height stored during NAME_FIRSTUPDATE
					nameNewHeight = nameRecord.NameNewHeight
				} else {
					// Fallback: estimate if NameNewHeight not set (old records from v2 or earlier)
					// This maintains backward compatibility with existing databases
					nameNewHeight = block.Height() - config.MinBlocksBeforeFirstUpdate
					if nameNewHeight < 0 {
						nameNewHeight = 0
					}
				}

				_, _ = bc.nameDB.RemoveLastHistoryEntry(name)
				_ = bc.nameDB.DeleteName(name)

				// Restore the NAME_NEW commitment with the exact original height.
				// The commitment hash is computed from rand (extra), name, and chain ID.
				commitHash := computeCommitHash(extra, name, bc.chainParams)
				_ = bc.nameDB.RestoreNameNew(commitHash, nameNewHeight)

				// Track this commitment as restored so we don't delete it if
				// the NAME_NEW for this commitment is also in this block
				restoredCommitments[string(commitHash)] = true

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

	// Restore all UTXOs that were spent in this block
	// This is done after processing name operations to ensure consistency
	if err := bc.nameDB.RestoreSpentUTXOsForBlock(block.Height()); err != nil {
		log.Printf("Warning: Failed to restore spent UTXOs for block %d: %v", block.Height(), err)
	}
}

// GetBlockHeader returns a block header by hash
func (bc *BlockChain) GetBlockHeader(hash *chainhash.Hash) (wire.BlockHeader, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.BlockChain.HeaderByHash(hash)
}

// GetNameDB returns the name database instance.
// This allows external packages to access the name database for read operations.
func (bc *BlockChain) GetNameDB() *namedb.NameDatabase {
	return bc.nameDB
}

// ValidateMempoolTransaction validates a transaction for inclusion in the mempool
// This performs consensus validation on name operations without requiring the transaction
// to be part of a block. It checks:
// - Basic transaction structure and format
// - Name operation syntax and semantics
// - Minimum fees for name operations
// - Name existence and expiration state
// - UTXO availability for name updates
//
// IMPORTANT LIMITATIONS:
//
//  1. This method does NOT validate script signatures or verify that transactions can
//     actually spend the UTXOs they reference. Signature validation is expensive and
//     deferred to block validation. Invalid transactions with incorrect signatures may
//     be accepted into the mempool and relayed to peers, but will be rejected during
//     block validation. This is an intentional trade-off between DoS resistance and
//     validation cost.
//
//  2. Fee validation requires that all input UTXOs are present in this node's UTXO
//     database. If any input UTXO cannot be found, the transaction is rejected, even
//     if it might be valid on the network. This means a node with incomplete UTXO data
//     (e.g., still syncing or missing historical data) cannot accept transactions that
//     depend on:
//     - UTXOs created in blocks before the node started tracking UTXOs
//     - Recent unconfirmed transactions in other nodes' mempools
//     - Transactions this node has not yet seen
//     This behavior differs from Bitcoin Core's mempool, which can validate and store
//     chains of unconfirmed transactions.
//
// This method is thread-safe and can be called concurrently.
func (bc *BlockChain) ValidateMempoolTransaction(tx *wire.MsgTx) error {
	if tx == nil {
		return fmt.Errorf("transaction is nil")
	}

	bc.mu.RLock()
	defer bc.mu.RUnlock()

	// Get current blockchain height for validation
	bestSnapshot := bc.BlockChain.BestSnapshot()
	currentHeight := bestSnapshot.Height

	// Scan transaction outputs for name operations
	var hasNameOp bool

	for i, txOut := range tx.TxOut {
		if len(txOut.PkScript) == 0 {
			continue
		}

		// Try to parse as name operation
		op, name, value, extra, err := parseNameScriptFull(txOut.PkScript)
		if err != nil {
			// Not a name operation, skip
			continue
		}

		// Found a name operation
		if hasNameOp {
			return fmt.Errorf("transaction has multiple name operations (not allowed)")
		}
		hasNameOp = true

		// Validate output value meets dust limit
		if txOut.Value < config.DustLimit {
			return fmt.Errorf("name operation output index %d has value %d below dust limit %d",
				i, txOut.Value, config.DustLimit)
		}

		// Validate name operation based on type
		switch op {
		case namedb.NameNew:
			// NAME_NEW validation
			// Check if commitment already exists in database
			if _, err := bc.nameDB.GetNameNew(extra); err == nil {
				return fmt.Errorf("name_new commitment already exists")
			}
			// NAME_NEW is valid - commitment will be stored when transaction is mined

		case namedb.NameFirstUpdate:
			// NAME_FIRSTUPDATE validation
			// Verify name doesn't already exist
			if existingRecord, err := bc.nameDB.GetName(name); err == nil {
				// Name exists - check if it's expired
				if existingRecord.ExpiresAt > currentHeight {
					return fmt.Errorf("name already exists and not expired: %s (expires at block %d)",
						name, existingRecord.ExpiresAt)
				}
				// Name exists but is expired - can be re-registered
			}

			// Verify NAME_NEW commitment exists
			commitHash := computeCommitHash(extra, name, bc.chainParams)
			nameNewRecord, err := bc.nameDB.GetNameNew(commitHash)
			if err != nil {
				return fmt.Errorf("no matching name_new found for name: %s", name)
			}

			// Note: We can't validate the block delay here because we don't know
			// when this transaction will be mined. The miner will validate this
			// when including in a block. We only check that the NAME_NEW exists.
			_ = nameNewRecord // Mark as used

			// Validate name format and value
			if err := validateNameFormat(name, value); err != nil {
				return fmt.Errorf("invalid name format: %w", err)
			}

		case namedb.NameUpdate:
			// NAME_UPDATE validation
			// Verify name exists and not expired
			record, err := bc.nameDB.GetName(name)
			if err != nil {
				return fmt.Errorf("name not found for update: %s", name)
			}
			if record.ExpiresAt <= currentHeight {
				return fmt.Errorf("name expired: %s (expired at block %d, current %d)",
					name, record.ExpiresAt, currentHeight)
			}

			// UTXO chain validation: Verify transaction spends the current name UTXO
			currentUTXO := wire.OutPoint{
				Hash:  record.TxHash,
				Index: record.OutIndex,
			}

			found := false
			for _, txIn := range tx.TxIn {
				if txIn.PreviousOutPoint.Hash.IsEqual(&currentUTXO.Hash) &&
					txIn.PreviousOutPoint.Index == currentUTXO.Index {
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf("name_update does not spend current name UTXO: name theft attempt for %s", name)
			}

			// Validate name format and value
			if err := validateNameFormat(name, value); err != nil {
				return fmt.Errorf("invalid name format: %w", err)
			}
		}

		// Validate transaction fee for name operations
		if err := bc.validateTransactionFee(tx, op, currentHeight); err != nil {
			return fmt.Errorf("fee validation failed: %w", err)
		}
	}

	// If this is a regular transaction (no name operations), apply basic validation
	if !hasNameOp {
		// Basic transaction validation
		// Check that transaction has at least one output
		if len(tx.TxOut) == 0 {
			return fmt.Errorf("transaction has no outputs")
		}

		// Check that transaction has at least one input (not a coinbase)
		if len(tx.TxIn) == 0 {
			return fmt.Errorf("transaction has no inputs")
		}

		// Check for coinbase transaction (not allowed in mempool)
		if tx.TxIn[0].PreviousOutPoint.Hash.IsEqual(&chainhash.Hash{}) {
			return fmt.Errorf("coinbase transactions not allowed in mempool")
		}
	}

	return nil
}
