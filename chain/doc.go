// Package chain provides Namecoin blockchain functionality built on btcd.
//
// The chain package wraps btcd's blockchain.BlockChain with Namecoin-specific
// name operation validation, AuxPow (Auxiliary Proof of Work) support, and
// name database integration. It implements composition over reimplementation,
// leveraging btcd's battle-tested blockchain components while adding the
// necessary extensions for Namecoin's naming system.
//
// # Architecture
//
// The package follows a layered design:
//
//   - BlockChain: Embeds btcd's blockchain.BlockChain with name validation hooks
//   - AuxPow: Merged mining support for blocks after height 19,200
//   - Block: Namecoin block wrapper with AuxPow header support
//   - Validation: Name operation and protocol compliance checking
//
// # Name Operations
//
// The chain validates three types of name operations:
//
//   - NAME_NEW: Pre-registration commitment to prevent front-running
//   - NAME_FIRSTUPDATE: First registration, reveals the name from NAME_NEW
//   - NAME_UPDATE: Updates name value and extends expiration by 36,000 blocks
//
// Each operation must comply with Namecoin protocol rules including:
//
//   - Name length: 1-255 bytes
//   - Value size: ≤1023 bytes (consensus), ≤520 bytes (UI limit for user-facing APIs)
//   - Timing: NAME_FIRSTUPDATE within 12-36,000 blocks of NAME_NEW
//   - Uniqueness: No duplicate unexpired names
//
// # AuxPow (Merged Mining)
//
// After block 19,200, Namecoin requires Auxiliary Proof of Work (AuxPow) which
// allows miners to mine Namecoin alongside Bitcoin or other Scrypt-based chains.
// AuxPow validation includes:
//
//   - Version bit verification (0x100 must be set)
//   - Parent block header validation
//   - Coinbase transaction merkle proof verification
//   - Chain ID validation (Namecoin = 1)
//
// The package includes an LRU cache for AuxPow data to handle the mismatch
// between btcd's block representation and Namecoin's extended block format.
//
// # Thread Safety
//
// BlockChain is safe for concurrent use. All shared state is protected with
// sync.RWMutex. Block validation and name database updates are atomic.
//
// # Example Usage
//
// Creating a new blockchain:
//
//	cfg := &chain.Config{
//	    ChainParams: &config.NamecoinMainNetParams,
//	    NameDBPath:  "/path/to/names.db",
//	    DataDir:     "/path/to/data",
//	}
//	bc, err := chain.NewBlockChain(cfg, nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer bc.Close()
//
//	// Process a block
//	err = bc.ProcessBlock(block, blockchain.BFNone)
//	if err != nil {
//	    log.Printf("Block rejected: %v", err)
//	}
//
// # Known Limitations
//
// The current implementation has approximately 35% protocol compatibility with
// Namecoin Core due to these missing features:
//
//   - Full AuxPow validation (parent chain PoW not verified)
//   - Block version validation for AuxPow (version bits not fully validated)
//   - Namecoin-specific subsidy calculation edge cases
//
// The package is suitable for development and testing but not production mainnet
// use beyond block 19,200 (AuxPow activation).
package chain
