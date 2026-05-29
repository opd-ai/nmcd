package chain

import (
	"errors"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"sync"
	"time"

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

	// auxPowCache stores AuxPow data temporarily keyed by block hash.
	// This is needed because btcd's blockchain package works with btcutil.Block
	// which doesn't have AuxPow fields, but we need to validate AuxPow.
	// The cache is populated when blocks arrive from the network and cleared after validation.
	//
	// The LRU cache ensures bounded memory usage (max DefaultAuxPowCacheSize entries)
	// to prevent unbounded growth under adversarial conditions (e.g., many blocks
	// with AuxPow arriving simultaneously with delayed validation).
	auxPowCache *auxPowLRUCache
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
		auxPowCache: newAuxPowLRUCache(DefaultAuxPowCacheSize),
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
	version := uint32(serializedBlock[0]) | uint32(serializedBlock[1])<<8 |
		uint32(serializedBlock[2])<<16 | uint32(serializedBlock[3])<<24

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
		bc.auxPowCache.Put(blockHash, block.AuxPow())
	}

	return nil
}

// getBlockAuxPow retrieves cached AuxPow data for a block.
// Returns nil if no AuxPow is cached for this block.
func (bc *BlockChain) getBlockAuxPow(blockHash *chainhash.Hash) *AuxPow {
	auxPow, _ := bc.auxPowCache.Get(blockHash)
	return auxPow
}

// clearBlockAuxPow removes AuxPow data from the cache after validation.
// This prevents unbounded memory growth.
func (bc *BlockChain) clearBlockAuxPow(blockHash *chainhash.Hash) {
	bc.auxPowCache.Delete(blockHash)
}

// ProcessBlock processes a block, validates name operations, and returns its chain status.
//
// The supplied block is treated as read-only; the caller retains ownership and
// may safely reuse the block after ProcessBlock returns.
//
// Return values (isMainChain, isOrphan, err):
//
//	(true,  false, nil) — block accepted onto the main chain; name database updated.
//	(false, false, nil) — block accepted as a side-chain block; name database NOT updated.
//	(false, true,  nil) — block is an isolated orphan (parent unknown); name database NOT updated.
//	                      Note: (true, true, nil) is theoretically possible from the embedded
//	                      btcd BlockChain; callers should handle it as an orphan.
//	(_,     _,     err) — block rejected; no state was changed.
//
// Callers in network/peermgr.go use `isOrphan || isMainChain` to decide whether to
// request the orphan's parent; a pure side-chain block (false, false, nil) does not
// trigger that path.
func (bc *BlockChain) ProcessBlock(block *btcutil.Block, flags blockchain.BehaviorFlags) (bool, bool, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	startTime := time.Now()
	height := bc.resolveBlockHeight(block)
	if err := bc.validateBlockForProcessing(block, height); err != nil {
		return false, false, err
	}

	isMainChain, isOrphan, err := bc.processBtcdBlock(block, flags, height)
	if err != nil {
		metrics.Get().RecordBlockRejected()
		return isMainChain, isOrphan, err
	}
	if err := bc.updateMainChainNames(block, isMainChain); err != nil {
		return isMainChain, isOrphan, err
	}

	processingTime := time.Since(startTime)
	metrics.Get().RecordBlockProcessed(block.Hash().String(), block.Height(), isMainChain, isOrphan, processingTime)
	return isMainChain, isOrphan, nil
}

func (bc *BlockChain) validateBlockForProcessing(block *btcutil.Block, height int32) error {
	checks := []struct {
		metric string
		format string
		run    func() error
	}{
		{
			metric: "proof_of_work",
			format: "invalid proof of work for block %s at height %d: %w",
			run: func() error {
				if !bc.requiresChildPoWCheck(height) {
					return nil
				}
				return bc.validateProofOfWork(block)
			},
		},
		{
			metric: "version",
			format: "invalid block version for block %s at height %d: %w",
			run:    func() error { return bc.validateBlockVersion(block) },
		},
		{
			metric: "auxpow",
			format: "invalid AuxPow for block %s at height %d: %w",
			run:    func() error { return bc.validateAuxPow(block) },
		},
		{
			metric: "subsidy",
			format: "invalid block subsidy for block %s at height %d: %w",
			run:    func() error { return bc.validateBlockSubsidy(block) },
		},
		{
			metric: "name",
			format: "invalid name operations in block %s at height %d: %w",
			run:    func() error { return bc.validateNameOperations(block) },
		},
	}

	for _, check := range checks {
		if err := check.run(); err != nil {
			bc.recordRejectedValidation(check.metric)
			return fmt.Errorf(check.format, block.Hash(), block.Height(), err)
		}
	}
	return nil
}

func (bc *BlockChain) requiresChildPoWCheck(height int32) bool {
	return height >= 0 && height < config.GetAuxPowActivationHeight(bc.chainParams)
}

func (bc *BlockChain) recordRejectedValidation(metric string) {
	metrics.Get().RecordBlockRejected()
	if metric == "name" {
		metrics.Get().RecordNameError()
		return
	}
	metrics.Get().RecordValidationError(metric)
}

func (bc *BlockChain) processBtcdBlock(block *btcutil.Block, flags blockchain.BehaviorFlags, height int32) (bool, bool, error) {
	btcdFlags := flags
	if height >= config.GetAuxPowActivationHeight(bc.chainParams) {
		btcdFlags |= blockchain.BFNoPoWCheck
	}
	return bc.BlockChain.ProcessBlock(block, btcdFlags)
}

func (bc *BlockChain) updateMainChainNames(block *btcutil.Block, isMainChain bool) error {
	if !isMainChain {
		return nil
	}
	if err := bc.updateNameDatabase(block); err != nil {
		return fmt.Errorf("failed to update name database for block %s at height %d: %w",
			block.Hash(), block.Height(), err)
	}
	return nil
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
// The maximum allowed coinbase output is: blockReward + totalTransactionFees.
// When UTXO data is unavailable for historical blocks, fee validation is skipped
// to avoid false rejections of legitimate blocks.
func (bc *BlockChain) validateBlockSubsidy(block *btcutil.Block) error {
	txs := block.Transactions()
	if len(txs) == 0 {
		return fmt.Errorf("block has no transactions")
	}

	totalOutput, err := sumOutputValues(txs[0].MsgTx())
	if err != nil {
		return err
	}
	height := bc.resolveSubsidyHeight(block)
	if height < 0 {
		return nil
	}

	maxSubsidy := config.CalcBlockSubsidy(height, bc.chainParams)
	totalFees, skipFeeCheck, err := bc.computeBlockFees(txs[1:], height)
	if err != nil {
		return err
	}
	if skipFeeCheck {
		return nil
	}
	return validateCoinbaseSubsidy(totalOutput, maxSubsidy, totalFees, height)
}

func (bc *BlockChain) resolveSubsidyHeight(block *btcutil.Block) int32 {
	height, err := bc.determineBlockHeight(block)
	if err != nil {
		return -1
	}
	return height
}

func validateCoinbaseSubsidy(totalOutput, maxSubsidy, totalFees int64, height int32) error {
	if totalOutput > maxSubsidy+totalFees {
		return fmt.Errorf("coinbase output %d exceeds maximum block subsidy %d plus fees %d at height %d",
			totalOutput, maxSubsidy, totalFees, height)
	}
	return nil
}

// computeBlockFees sums the transaction fees for a set of non-coinbase transactions.
// Returns (totalFees, skipCheck, err). skipCheck is true when UTXO data is unavailable
// for historical blocks, which means the caller should skip fee-based validation to
// avoid false rejections.
func (bc *BlockChain) computeBlockFees(txs []*btcutil.Tx, height int32) (int64, bool, error) {
	if bc.nameDB == nil || len(txs) == 0 {
		// No non-coinbase transactions means no fees; strict check is valid.
		return 0, false, nil
	}
	var totalFees int64
	for _, tx := range txs {
		inputSum, skip, err := bc.sumInputValues(tx.MsgTx(), height)
		if skip {
			return 0, true, nil
		}
		if err != nil {
			return 0, false, err
		}
		outputSum, err := sumOutputValues(tx.MsgTx())
		if err != nil {
			return 0, false, err
		}
		fee := inputSum - outputSum
		if fee > 0 {
			totalFees += fee
		}
	}
	return totalFees, false, nil
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

// nameValidationContext holds state for validating name operations in a block.
type nameValidationContext struct {
	seenNameNewCommits map[string]bool // Track NAME_NEW commits in this block
	seenNames          map[string]bool // Track names in this block
}

// newNameValidationContext creates a new validation context.
func newNameValidationContext() *nameValidationContext {
	return &nameValidationContext{
		seenNameNewCommits: make(map[string]bool),
		seenNames:          make(map[string]bool),
	}
}

// validateNameNewOp validates a NAME_NEW operation.
func (bc *BlockChain) validateNameNewOp(txOut *wire.TxOut, extra []byte, txHash chainhash.Hash, ctx *nameValidationContext) error {
	// Validate NAME_NEW output value meets dust limit
	if txOut.Value < config.DustLimit {
		return fmt.Errorf("name_new output value %d below dust limit %d in tx %s",
			txOut.Value, config.DustLimit, txHash)
	}

	// Check for duplicate commitment hash in this block
	commitHashStr := string(extra)
	if ctx.seenNameNewCommits[commitHashStr] {
		return fmt.Errorf("duplicate name_new commitment in block (tx: %s)", txHash)
	}
	ctx.seenNameNewCommits[commitHashStr] = true

	// Check if commitment already exists in database
	if _, err := bc.nameDB.GetNameNew(extra); err == nil {
		return fmt.Errorf("name_new commitment already exists (tx: %s)", txHash)
	}

	return nil
}

// validateNamedTxOut checks dust rules and duplicate in-block operations for a name output.
func validateNamedTxOut(txOut *wire.TxOut, name string, txHash chainhash.Hash, op string, ctx *nameValidationContext) error {
	if txOut.Value < config.DustLimit {
		return fmt.Errorf("%s output value %d below dust limit %d in tx %s", op, txOut.Value, config.DustLimit, txHash)
	}
	if ctx.seenNames[name] {
		return fmt.Errorf("duplicate name operation in block for name: %s (tx: %s)", name, txHash)
	}
	ctx.seenNames[name] = true
	return nil
}

// validateNameFirstUpdateOp validates a NAME_FIRSTUPDATE operation.
func (bc *BlockChain) validateNameFirstUpdateOp(txOut *wire.TxOut, name string, extra []byte, txHash chainhash.Hash, height int32, ctx *nameValidationContext) error {
	if err := validateNamedTxOut(txOut, name, txHash, "name_firstupdate", ctx); err != nil {
		return err
	}
	if err := bc.ensureNameAvailableForFirstUpdate(name, height, txHash); err != nil {
		return err
	}
	return bc.validateFirstUpdateWindow(name, computeCommitHash(extra, name), height, txHash)
}

func (bc *BlockChain) ensureNameAvailableForFirstUpdate(name string, height int32, txHash chainhash.Hash) error {
	existingRecord, err := bc.nameDB.GetName(name)
	if errors.Is(err, namedb.ErrNameNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to check existing name %s: %w", name, err)
	}
	if existingRecord.ExpiresAt >= height {
		return fmt.Errorf("name already exists and is not expired: %s (expires at %d, current height %d, tx: %s)",
			name, existingRecord.ExpiresAt, height, txHash)
	}
	return nil
}

func (bc *BlockChain) validateFirstUpdateWindow(name string, commitHash []byte, height int32, txHash chainhash.Hash) error {
	nameNewRecord, err := bc.nameDB.GetNameNew(commitHash)
	if err != nil {
		return fmt.Errorf("no matching name_new found for name: %s (tx: %s)", name, txHash)
	}
	if height < nameNewRecord.Height {
		return fmt.Errorf("name_firstupdate before name_new: block %d < name_new block %d (name: '%s', tx: %s)",
			height, nameNewRecord.Height, name, txHash)
	}

	blocksSinceNew := height - nameNewRecord.Height
	if blocksSinceNew < config.MinBlocksBeforeFirstUpdate {
		return fmt.Errorf("name_firstupdate too early: %d blocks since name_new, minimum %d required (name: '%s', tx: %s)",
			blocksSinceNew, config.MinBlocksBeforeFirstUpdate, name, txHash)
	}
	if blocksSinceNew > config.MaxBlocksBeforeFirstUpdate {
		return fmt.Errorf("name_firstupdate too late: %d blocks since name_new, maximum %d allowed (commitment expired) (name: '%s', tx: %s)",
			blocksSinceNew, config.MaxBlocksBeforeFirstUpdate, name, txHash)
	}
	return nil
}

// validateNameUpdateOp validates a NAME_UPDATE operation.
func (bc *BlockChain) validateNameUpdateOp(msgTx *wire.MsgTx, txOut *wire.TxOut, name string, txHash chainhash.Hash, height int32, ctx *nameValidationContext) error {
	if err := validateNamedTxOut(txOut, name, txHash, "name_update", ctx); err != nil {
		return err
	}

	// Verify name exists and not expired
	record, err := bc.nameDB.GetName(name)
	if err != nil {
		if errors.Is(err, namedb.ErrNameNotFound) {
			return fmt.Errorf("name not found for update: %s (tx: %s)", name, txHash)
		}
		return fmt.Errorf("failed to get name %s for update: %w", name, err)
	}
	if record.ExpiresAt < height {
		return fmt.Errorf("name expired: %s (expires at block %d, current %d, tx: %s)",
			name, record.ExpiresAt, height, txHash)
	}

	// UTXO chain validation: Verify the transaction spends the current name UTXO
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

	return nil
}

// validateNameOperations validates name operations in a block
func (bc *BlockChain) validateNameOperations(block *btcutil.Block) error {
	height, err := bc.determineBlockHeight(block)
	if err != nil {
		return fmt.Errorf("cannot determine block height for name validation: %w", err)
	}

	ctx := newNameValidationContext()

	for txIdx, tx := range block.Transactions() {
		if err := bc.validateTransactionNameOps(tx, txIdx, height, ctx); err != nil {
			return err
		}
	}

	return nil
}

// validateTransactionNameOps validates all name operations in a transaction.
func (bc *BlockChain) validateTransactionNameOps(tx *btcutil.Tx, txIdx int, height int32, ctx *nameValidationContext) error {
	msgTx := tx.MsgTx()
	nameOpTypes := bc.collectNameOperationTypes(msgTx)

	if err := bc.validateNameOpFees(msgTx, nameOpTypes, txIdx, height); err != nil {
		return err
	}

	return bc.validateNameOutputs(msgTx, height, ctx)
}

// collectNameOperationTypes identifies unique name operation types in transaction outputs.
func (bc *BlockChain) collectNameOperationTypes(msgTx *wire.MsgTx) map[namedb.NameOperation]struct{} {
	nameOpTypes := make(map[namedb.NameOperation]struct{})
	for _, txOut := range msgTx.TxOut {
		if op, _, _, _, err := parseNameScriptFull(txOut.PkScript); err == nil {
			nameOpTypes[op] = struct{}{}
		}
	}
	return nameOpTypes
}

// validateNameOpFees validates transaction fees for name operations.
func (bc *BlockChain) validateNameOpFees(msgTx *wire.MsgTx, nameOpTypes map[namedb.NameOperation]struct{}, txIdx int, height int32) error {
	if len(nameOpTypes) == 0 || txIdx == 0 {
		return nil
	}

	txHash := msgTx.TxHash()
	for opType := range nameOpTypes {
		if err := bc.validateTransactionFee(msgTx, opType, height); err != nil {
			return fmt.Errorf("invalid transaction fee for %s in tx %s: %w", opType, txHash, err)
		}
	}
	return nil
}

// validateNameOutputs validates each name operation output in a transaction.
func (bc *BlockChain) validateNameOutputs(msgTx *wire.MsgTx, height int32, ctx *nameValidationContext) error {
	txHash := msgTx.TxHash()

	for _, txOut := range msgTx.TxOut {
		op, name, value, extra, err := parseNameScriptFull(txOut.PkScript)
		if err != nil {
			continue
		}

		if err := bc.dispatchNameOpValidation(msgTx, txOut, op, name, value, extra, txHash, height, ctx); err != nil {
			return err
		}

		// Consensus validation: only enforce consensus constraints, not local policies
		// This allows nmcd to accept valid mainnet blocks with non-JSON, non-UTF8, or non-namespace values
		if op != namedb.NameNew {
			if err := validateConsensusNameFormat(name, value); err != nil {
				return fmt.Errorf("%w (name: '%s', tx: %s)", err, name, txHash)
			}
		}
	}

	return nil
}

// dispatchNameOpValidation dispatches validation to operation-specific handlers.
func (bc *BlockChain) dispatchNameOpValidation(msgTx *wire.MsgTx, txOut *wire.TxOut, op namedb.NameOperation, name, value string, extra []byte, txHash chainhash.Hash, height int32, ctx *nameValidationContext) error {
	switch op {
	case namedb.NameNew:
		return bc.validateNameNewOp(txOut, extra, txHash, ctx)
	case namedb.NameFirstUpdate:
		return bc.validateNameFirstUpdateOp(txOut, name, extra, txHash, height, ctx)
	case namedb.NameUpdate:
		return bc.validateNameUpdateOp(msgTx, txOut, name, txHash, height, ctx)
	}
	return nil
}

// determineBlockHeight determines the height for a block being validated.
func (bc *BlockChain) determineBlockHeight(block *btcutil.Block) (int32, error) {
	var height int32 = -1
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
			return -1, fmt.Errorf("cannot determine block height")
		}
	}

	return height, nil
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
	if len(tx.TxIn) == 0 {
		return fmt.Errorf("transaction has no inputs (cannot validate fee)")
	}

	totalInputValue, skip, err := bc.sumInputValues(tx, height)
	if err != nil {
		return err
	}
	if skip {
		log.Printf("Info: Skipping fee validation for historical block %d due to missing UTXO data", height)
		return nil
	}

	totalOutputValue, err := sumOutputValues(tx)
	if err != nil {
		return err
	}

	return validateMinFee(totalInputValue-totalOutputValue, opType)
}

// sumInputValues calculates the total input value for a transaction.
// Returns the total value, whether to skip fee validation (historical blocks), and any error.
func (bc *BlockChain) sumInputValues(tx *wire.MsgTx, height int32) (int64, bool, error) {
	var totalInputValue int64
	for _, txIn := range tx.TxIn {
		utxo, err := bc.nameDB.GetUTXO(&txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index)
		if err != nil {
			if height < config.UTXOTrackingStartHeight {
				return 0, true, nil
			}
			return 0, false, fmt.Errorf("cannot validate transaction fee: UTXO %s:%d not found at height %d: %w",
				txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index, height, err)
		}
		if utxo.Value < 0 {
			return 0, false, fmt.Errorf("negative UTXO value %d at %s:%d",
				utxo.Value, txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index)
		}
		if totalInputValue > 0 && utxo.Value > 0 && totalInputValue > (1<<63-1)-utxo.Value {
			return 0, false, fmt.Errorf("transaction input value overflow: %d + %d", totalInputValue, utxo.Value)
		}
		totalInputValue += utxo.Value
	}
	return totalInputValue, false, nil
}

// sumOutputValues calculates the total output value for a transaction.
func sumOutputValues(tx *wire.MsgTx) (int64, error) {
	var totalOutputValue int64
	for _, txOut := range tx.TxOut {
		if totalOutputValue > 0 && txOut.Value > 0 && totalOutputValue > (1<<63-1)-txOut.Value {
			return 0, fmt.Errorf("transaction output value overflow: %d + %d", totalOutputValue, txOut.Value)
		}
		totalOutputValue += txOut.Value
	}
	return totalOutputValue, nil
}

// validateMinFee checks that the transaction fee meets the minimum for the operation type.
func validateMinFee(fee int64, opType namedb.NameOperation) error {
	if fee < 0 {
		return fmt.Errorf("transaction fee cannot be negative: %d satoshis", fee)
	}

	var minFee int64
	switch opType {
	case namedb.NameNew:
		minFee = config.MinRelayTxFee
	case namedb.NameFirstUpdate, namedb.NameUpdate:
		minFee = config.MinNameOperationFee
	default:
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
	blockTime := block.MsgBlock().Header.Timestamp

	if err := bc.handleExpiredNames(height); err != nil {
		return err
	}

	if err := bc.processBlockTransactions(block, height, blockTime); err != nil {
		return err
	}

	bc.cleanupSpentUTXOsIfNeeded(height)
	bc.cleanupOldExpiredNamesIfNeeded(height)

	return nil
}

// handleExpiredNames deletes expired names and their history at given height.
// Before deletion, names are stored in the expired names bucket for potential
// restoration during blockchain reorganizations.
func (bc *BlockChain) handleExpiredNames(height int32) error {
	expired, err := bc.nameDB.GetExpiredNames(height)
	if err != nil {
		return err
	}

	for _, name := range expired {
		// Get the name record before deletion for rollback support.
		// Only skip if the name is genuinely not found (already deleted).
		// For decode corruption or other errors, log and still proceed with
		// deletion to prevent an expired name from remaining active.
		record, err := bc.nameDB.GetName(name)
		if err == nil {
			// Store the expired name for potential restoration during reorg
			if storeErr := bc.nameDB.StoreExpiredName(record, height); storeErr != nil {
				return fmt.Errorf("failed to store expired name %s for rollback: %w", name, storeErr)
			}
		} else if !errors.Is(err, namedb.ErrNameNotFound) {
			log.Printf("Warning: failed to get name %s before expiration (possible corruption): %v; proceeding with deletion", name, err)
		}

		// Delete the name and its history regardless of whether we got the record.
		if err := bc.nameDB.DeleteName(name); err != nil {
			return err
		}
		if err := bc.nameDB.DeleteHistory(name); err != nil {
			return err
		}
		metrics.Get().RecordNameExpired()
	}
	return nil
}

// processBlockTransactions processes all transactions in a block for UTXO and name operations.
func (bc *BlockChain) processBlockTransactions(block *btcutil.Block, height int32, blockTime time.Time) error {
	for txIdx, tx := range block.Transactions() {
		if txIdx > 0 {
			if err := bc.processTransactionInputs(tx.MsgTx(), height); err != nil {
				return err
			}
		}

		if err := bc.processTransactionOutputs(tx, height, blockTime); err != nil {
			return err
		}
	}
	return nil
}

// processTransactionInputs processes spent UTXOs from transaction inputs.
func (bc *BlockChain) processTransactionInputs(msgTx *wire.MsgTx, height int32) error {
	for _, txIn := range msgTx.TxIn {
		utxo, err := bc.nameDB.GetUTXO(&txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index)
		if err == nil && utxo != nil {
			if err := bc.nameDB.StoreSpentUTXO(utxo, height); err != nil {
				return fmt.Errorf("store spent UTXO %s:%d at height %d: %w",
					txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index, height, err)
			}
		}

		if err := bc.nameDB.RemoveUTXO(&txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index); err != nil {
			log.Printf("Info: Could not remove UTXO %s:%d (may not exist): %v",
				txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index, err)
		}
	}
	return nil
}

// processTransactionOutputs processes new UTXOs and name operations from transaction outputs.
func (bc *BlockChain) processTransactionOutputs(tx *btcutil.Tx, height int32, blockTime time.Time) error {
	msgTx := tx.MsgTx()
	txHash := tx.Hash()

	for outIdx, txOut := range msgTx.TxOut {
		if err := bc.addUTXO(txHash, outIdx, txOut, height); err != nil {
			return err
		}

		if err := bc.processNameOperation(txHash, outIdx, txOut, height, blockTime); err != nil {
			return err
		}
	}
	return nil
}

// addUTXO creates and stores a UTXO entry for a transaction output.
func (bc *BlockChain) addUTXO(txHash *chainhash.Hash, outIdx int, txOut *wire.TxOut, height int32) error {
	_, addresses, _, err := txscript.ExtractPkScriptAddrs(txOut.PkScript, bc.chainParams)
	var address string
	if err == nil && len(addresses) > 0 {
		address = addresses[0].EncodeAddress()
	}

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
	return nil
}

// processNameOperation parses and handles name operations from transaction outputs.
func (bc *BlockChain) processNameOperation(txHash *chainhash.Hash, outIdx int, txOut *wire.TxOut, height int32, blockTime time.Time) error {
	op, name, value, extra, err := parseNameScriptFull(txOut.PkScript)
	if err != nil {
		return nil // Not a name operation
	}

	switch op {
	case namedb.NameNew:
		return bc.processNameNew(extra, height)
	case namedb.NameFirstUpdate:
		return bc.processNameFirstUpdate(txHash, outIdx, txOut, name, value, extra, height, blockTime)
	case namedb.NameUpdate:
		return bc.processNameUpdate(txHash, outIdx, txOut, name, value, height, blockTime)
	}
	return nil
}

// processNameNew handles NAME_NEW operations.
func (bc *BlockChain) processNameNew(commitHash []byte, height int32) error {
	if err := bc.nameDB.PutNameNew(commitHash, height); err != nil {
		return err
	}
	metrics.Get().RecordNameOperation("NAME_NEW")
	return nil
}

// safeCalcExpiresAt calculates the expiration height with overflow protection.
// If the addition would overflow int32, it returns math.MaxInt32 (maximum safe value).
// This prevents negative expiration heights that would cause incorrect behavior.
//
// The function checks if height + config.NameExpirationBlocks would exceed MaxInt32.
// In practice, this is a theoretical concern as Namecoin would need ~400 years
// to reach heights where this matters (at 10 min/block).
func safeCalcExpiresAt(height int32) int32 {
	if height > int32(math.MaxInt32)-config.NameExpirationBlocks {
		return int32(math.MaxInt32)
	}
	return height + config.NameExpirationBlocks
}

// processNameFirstUpdate handles NAME_FIRSTUPDATE operations.
func (bc *BlockChain) processNameFirstUpdate(txHash *chainhash.Hash, outIdx int, txOut *wire.TxOut, name, value string, extra []byte, height int32, blockTime time.Time) error {
	address := extractAddressFromNameScript(txOut.PkScript, bc.chainParams)
	nameNewHeight := bc.getNameNewHeight(extra, name, height)

	record := &namedb.NameRecord{
		Name:          name,
		Value:         value,
		TxHash:        *txHash,
		OutIndex:      uint32(outIdx),
		Height:        height,
		ExpiresAt:     safeCalcExpiresAt(height),
		Address:       address,
		UpdatedAt:     blockTime,
		NameNewHeight: nameNewHeight,
	}

	if err := bc.nameDB.PutName(name, record); err != nil {
		return err
	}
	if err := bc.nameDB.AddHistory(*txHash, record); err != nil {
		return err
	}

	commitHash := computeCommitHash(extra, name)
	if err := bc.nameDB.DeleteNameNew(commitHash); err != nil {
		return err
	}

	metrics.Get().RecordNameOperation("NAME_FIRSTUPDATE")
	return nil
}

// getNameNewHeight retrieves the NAME_NEW height or estimates it.
func (bc *BlockChain) getNameNewHeight(extra []byte, name string, currentHeight int32) int32 {
	commitHash := computeCommitHash(extra, name)
	nameNewRecord, err := bc.nameDB.GetNameNew(commitHash)
	if err == nil && nameNewRecord != nil {
		return nameNewRecord.Height
	}

	// Fallback estimation for missing NAME_NEW records
	nameNewHeight := currentHeight - config.MinBlocksBeforeFirstUpdate
	if nameNewHeight < 0 {
		nameNewHeight = 0
	}
	return nameNewHeight
}

// processNameUpdate handles NAME_UPDATE operations.
func (bc *BlockChain) processNameUpdate(txHash *chainhash.Hash, outIdx int, txOut *wire.TxOut, name, value string, height int32, blockTime time.Time) error {
	address := extractAddressFromNameScript(txOut.PkScript, bc.chainParams)

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
		ExpiresAt:     safeCalcExpiresAt(height),
		Address:       address,
		UpdatedAt:     blockTime,
		NameNewHeight: nameNewHeight,
	}

	if err := bc.nameDB.PutName(name, record); err != nil {
		return err
	}
	if err := bc.nameDB.AddHistory(*txHash, record); err != nil {
		return err
	}

	metrics.Get().RecordNameOperation("NAME_UPDATE")
	return nil
}

// cleanupSpentUTXOsIfNeeded periodically removes old spent UTXOs to prevent unbounded growth.
func (bc *BlockChain) cleanupSpentUTXOsIfNeeded(height int32) {
	const spentUtxoRetentionDepth = 1000
	if height > spentUtxoRetentionDepth && height%100 == 0 {
		cleanupHeight := height - spentUtxoRetentionDepth
		if err := bc.nameDB.CleanupOldSpentUTXOs(cleanupHeight); err != nil {
			log.Printf("Warning: Failed to cleanup old spent UTXOs at height %d: %v", height, err)
		}
	}
}

// cleanupOldExpiredNamesIfNeeded periodically removes old expired-name backups to prevent unbounded growth.
func (bc *BlockChain) cleanupOldExpiredNamesIfNeeded(height int32) {
	const expiredNameRetentionDepth = 1000
	if height > expiredNameRetentionDepth && height%100 == 0 {
		cleanupHeight := height - expiredNameRetentionDepth
		if err := bc.nameDB.CleanupOldExpiredNames(cleanupHeight); err != nil {
			log.Printf("Warning: Failed to cleanup old expired name backups at height %d: %v", height, err)
		}
	}
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

// ScanNames scans names matching a prefix with pagination.
// Returns up to count names starting from the given prefix.
// This is used by the name_scan RPC to provide Namecoin Core compatibility.
func (bc *BlockChain) ScanNames(prefix string, count int) ([]*namedb.NameRecord, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.nameDB.ScanNames(prefix, count)
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

// HandleBlockchainNotification processes blockchain notifications.
// This method must NOT acquire bc.mu because it may be dispatched synchronously
// from within bc.BlockChain.ProcessBlock() (which holds bc.mu), causing a deadlock.
// All operations here delegate to bc.nameDB which has its own independent mutex.
func (bc *BlockChain) HandleBlockchainNotification(notification *blockchain.Notification) {
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
	restoredCommitments := make(map[string]bool)
	var rollbackErrors []error

	// First, restore any names that were expired at this block height
	if err := bc.nameDB.RestoreExpiredNamesForBlock(block.Height()); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("failed to restore expired names for block %d: %w", block.Height(), err))
	}

	txs := block.Transactions()
	for i := len(txs) - 1; i >= 0; i-- {
		tx := txs[i]
		if err := bc.rollbackTransactionUTXOs(tx); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
		if err := bc.rollbackTransactionNameOps(tx, block.Height(), restoredCommitments); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
	}

	if err := bc.nameDB.RestoreSpentUTXOsForBlock(block.Height()); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("failed to restore spent UTXOs for block %d: %w", block.Height(), err))
	}

	// Log all accumulated errors
	if len(rollbackErrors) > 0 {
		log.Printf("ERROR: Rollback for block %d encountered %d error(s):", block.Height(), len(rollbackErrors))
		for i, err := range rollbackErrors {
			log.Printf("  [%d] %v", i+1, err)
		}
		log.Printf("WARNING: Name database may be in an inconsistent state after failed rollback of block %d", block.Height())
	}
}

// rollbackTransactionUTXOs removes UTXOs created by a transaction's outputs.
func (bc *BlockChain) rollbackTransactionUTXOs(tx *btcutil.Tx) error {
	txHash := tx.Hash()
	var rollbackErrors []error

	for outIdx := range tx.MsgTx().TxOut {
		if err := bc.nameDB.RemoveUTXO(txHash, uint32(outIdx)); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("failed to remove created UTXO %s:%d: %w", txHash, outIdx, err))
		}
	}

	if len(rollbackErrors) == 0 {
		return nil
	}

	return fmt.Errorf("rollback UTXOs for tx %s: %w", txHash, errors.Join(rollbackErrors...))
}

// rollbackTransactionNameOps reverses name operations within a transaction during block rollback.
// Returns error if any database operation fails during rollback.
func (bc *BlockChain) rollbackTransactionNameOps(tx *btcutil.Tx, blockHeight int32, restoredCommitments map[string]bool) error {
	msgTx := tx.MsgTx()
	var rollbackErrors []error

	for j := len(msgTx.TxOut) - 1; j >= 0; j-- {
		txOut := msgTx.TxOut[j]
		op, name, _, extra, err := parseNameScriptFull(txOut.PkScript)
		if err != nil {
			continue
		}

		switch op {
		case namedb.NameNew:
			if err := bc.rollbackNameNew(extra, restoredCommitments); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("NAME_NEW rollback failed: %w", err))
			}
		case namedb.NameFirstUpdate:
			if err := bc.rollbackNameFirstUpdate(name, extra, blockHeight, restoredCommitments); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("NAME_FIRSTUPDATE rollback for %s failed: %w", name, err))
			}
		case namedb.NameUpdate:
			if err := bc.rollbackNameUpdate(name); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("NAME_UPDATE rollback for %s failed: %w", name, err))
			}
		}
	}

	if len(rollbackErrors) > 0 {
		return fmt.Errorf("rollback name operations for tx %s: %w", tx.Hash(), errors.Join(rollbackErrors...))
	}
	return nil
}

// rollbackNameNew removes a NAME_NEW commitment unless it was restored during this rollback.
func (bc *BlockChain) rollbackNameNew(extra []byte, restoredCommitments map[string]bool) error {
	if restoredCommitments[string(extra)] {
		return nil
	}
	return bc.nameDB.DeleteNameNew(extra)
}

// rollbackNameFirstUpdate removes a name registration and restores its NAME_NEW commitment.
func (bc *BlockChain) rollbackNameFirstUpdate(name string, extra []byte, blockHeight int32, restoredCommitments map[string]bool) error {
	nameNewHeight := bc.getNameNewHeightForRollback(name, blockHeight)

	if _, err := bc.nameDB.RemoveLastHistoryEntry(name); err != nil {
		return fmt.Errorf("failed to remove history for %s: %w", name, err)
	}
	if err := bc.nameDB.DeleteName(name); err != nil {
		return fmt.Errorf("failed to delete name %s: %w", name, err)
	}

	commitHash := computeCommitHash(extra, name)
	if err := bc.nameDB.RestoreNameNew(commitHash, nameNewHeight); err != nil {
		return fmt.Errorf("failed to restore NAME_NEW for %s: %w", name, err)
	}
	restoredCommitments[string(commitHash)] = true
	return nil
}

// getNameNewHeightForRollback retrieves the NAME_NEW height for rollback, with fallback estimation.
func (bc *BlockChain) getNameNewHeightForRollback(name string, blockHeight int32) int32 {
	nameRecord, err := bc.nameDB.GetName(name)
	if err == nil && nameRecord != nil && nameRecord.NameNewHeight != 0 {
		return nameRecord.NameNewHeight
	}
	nameNewHeight := blockHeight - config.MinBlocksBeforeFirstUpdate
	if nameNewHeight < 0 {
		return 0
	}
	return nameNewHeight
}

// rollbackNameUpdate removes a NAME_UPDATE and restores the previous record.
func (bc *BlockChain) rollbackNameUpdate(name string) error {
	prevRecord, err := bc.nameDB.RemoveLastHistoryEntry(name)
	if err != nil {
		return fmt.Errorf("failed to remove history for %s: %w", name, err)
	}
	if prevRecord != nil {
		if err := bc.nameDB.PutName(name, prevRecord); err != nil {
			return fmt.Errorf("failed to restore previous record for %s: %w", name, err)
		}
	}
	return nil
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

	bestSnapshot := bc.BlockChain.BestSnapshot()
	currentHeight := bestSnapshot.Height

	hasNameOp, err := bc.validateNameOperationsInTx(tx, currentHeight)
	if err != nil {
		return err
	}

	if !hasNameOp {
		if err := bc.validateRegularTransaction(tx); err != nil {
			return err
		}
	}

	return nil
}

// validateNameOperationsInTx validates all name operations in a transaction.
func (bc *BlockChain) validateNameOperationsInTx(tx *wire.MsgTx, currentHeight int32) (bool, error) {
	var hasNameOp bool

	for i, txOut := range tx.TxOut {
		if len(txOut.PkScript) == 0 {
			continue
		}

		op, name, value, extra, err := parseNameScriptFull(txOut.PkScript)
		if err != nil {
			continue
		}

		if hasNameOp {
			return false, fmt.Errorf("transaction has multiple name operations (not allowed)")
		}
		hasNameOp = true

		if txOut.Value < config.DustLimit {
			return false, fmt.Errorf("name operation output index %d has value %d below dust limit %d",
				i, txOut.Value, config.DustLimit)
		}

		if err := bc.validateNameOperation(op, name, value, extra, tx, currentHeight); err != nil {
			return false, err
		}

		if err := bc.validateTransactionFee(tx, op, currentHeight); err != nil {
			return false, fmt.Errorf("fee validation failed: %w", err)
		}
	}

	return hasNameOp, nil
}

// validateNameOperation validates a specific name operation type.
// Uses consensus-critical validation only (name/value length checks).
// Strict format validation (namespace prefixes, JSON encoding, UTF-8) is applied
// only during local name creation (wallet/RPC), not during consensus validation,
// allowing nmcd to accept valid mainnet blocks that contain arbitrary namespace prefixes
// and non-JSON/non-UTF8 values.
func (bc *BlockChain) validateNameOperation(op namedb.NameOperation, name, value string, extra []byte, tx *wire.MsgTx, currentHeight int32) error {
	switch op {
	case namedb.NameNew:
		return bc.validateNameNew(extra)
	case namedb.NameFirstUpdate:
		return bc.validateNameFirstUpdate(name, value, extra, currentHeight)
	case namedb.NameUpdate:
		return bc.validateNameUpdate(name, value, tx, currentHeight)
	}
	return nil
}

// validateNameNew validates NAME_NEW operations.
func (bc *BlockChain) validateNameNew(commitHash []byte) error {
	if _, err := bc.nameDB.GetNameNew(commitHash); err == nil {
		return fmt.Errorf("name_new commitment already exists")
	}
	return nil
}

// validateNameFirstUpdate validates NAME_FIRSTUPDATE operations in the mempool.
// Uses consensus validation constraints only, allowing non-JSON and non-UTF8 values
// to pass through the mempool, consistent with mainnet Namecoin behavior.
func (bc *BlockChain) validateNameFirstUpdate(name, value string, extra []byte, currentHeight int32) error {
	if existingRecord, err := bc.nameDB.GetName(name); err == nil {
		if existingRecord.ExpiresAt >= currentHeight {
			return fmt.Errorf("name already exists and not expired: %s (expires at block %d)",
				name, existingRecord.ExpiresAt)
		}
	} else if !errors.Is(err, namedb.ErrNameNotFound) {
		return fmt.Errorf("failed to check existing name %s: %w", name, err)
	}

	commitHash := computeCommitHash(extra, name)
	nameNewRecord, err := bc.nameDB.GetNameNew(commitHash)
	if err != nil {
		return fmt.Errorf("no matching name_new found for name: %s", name)
	}

	// Enforce timing window: NAME_FIRSTUPDATE must be between MinBlocksBeforeFirstUpdate
	// and MaxBlocksBeforeFirstUpdate blocks after the NAME_NEW
	if currentHeight < nameNewRecord.Height {
		return fmt.Errorf("name_firstupdate before name_new: block %d < name_new block %d (name: '%s')",
			currentHeight, nameNewRecord.Height, name)
	}
	blocksSinceNameNew := currentHeight - nameNewRecord.Height
	if blocksSinceNameNew < config.MinBlocksBeforeFirstUpdate {
		return fmt.Errorf("name_firstupdate too early: must wait %d blocks after name_new (current: %d blocks)",
			config.MinBlocksBeforeFirstUpdate, blocksSinceNameNew)
	}
	if blocksSinceNameNew > config.MaxBlocksBeforeFirstUpdate {
		return fmt.Errorf("name_firstupdate expired: name_new commitment is too old (must reveal within %d blocks, current: %d blocks)",
			config.MaxBlocksBeforeFirstUpdate, blocksSinceNameNew)
	}

	// Use consensus validation: accept blocks with non-JSON/non-UTF8 values
	if err := validateConsensusNameFormat(name, value); err != nil {
		return fmt.Errorf("invalid name format: %w", err)
	}

	return nil
}

// validateNameUpdate validates NAME_UPDATE operations in the mempool.
// Uses consensus validation constraints only, allowing non-JSON and non-UTF8 values
// to pass through the mempool, consistent with mainnet Namecoin behavior.
func (bc *BlockChain) validateNameUpdate(name, value string, tx *wire.MsgTx, currentHeight int32) error {
	record, err := bc.nameDB.GetName(name)
	if err != nil {
		if errors.Is(err, namedb.ErrNameNotFound) {
			return fmt.Errorf("name not found for update: %s", name)
		}
		return fmt.Errorf("failed to get name %s for update: %w", name, err)
	}
	if record.ExpiresAt < currentHeight {
		return fmt.Errorf("name expired: %s (expired at block %d, current %d)",
			name, record.ExpiresAt, currentHeight)
	}

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

	// Use consensus validation: accept blocks with non-JSON/non-UTF8 values
	if err := validateConsensusNameFormat(name, value); err != nil {
		return fmt.Errorf("invalid name format: %w", err)
	}

	return nil
}

// validateRegularTransaction validates non-name transactions.
func (bc *BlockChain) validateRegularTransaction(tx *wire.MsgTx) error {
	if len(tx.TxOut) == 0 {
		return fmt.Errorf("transaction has no outputs")
	}

	if len(tx.TxIn) == 0 {
		return fmt.Errorf("transaction has no inputs")
	}

	if tx.TxIn[0].PreviousOutPoint.Hash.IsEqual(&chainhash.Hash{}) {
		return fmt.Errorf("coinbase transactions not allowed in mempool")
	}

	return nil
}
