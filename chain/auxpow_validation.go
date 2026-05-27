package chain

import (
	"fmt"
	"log"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/opd-ai/nmcd/config"
)

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
	height := bc.resolveBlockHeight(block)
	if height < 0 {
		return nil
	}

	if height < config.GetAuxPowActivationHeight(bc.chainParams) {
		return nil
	}

	if err := bc.verifyAuxPowVersionBit(block, height); err != nil {
		return err
	}

	blockHash := block.Hash()
	auxPow := bc.getBlockAuxPow(blockHash)
	defer bc.clearBlockAuxPow(blockHash)

	if auxPow == nil {
		return fmt.Errorf("block at height %d requires AuxPow but no AuxPow data was provided", height)
	}

	if err := bc.verifyChainID(block, height); err != nil {
		return err
	}

	return bc.verifyAuxPowProof(block, auxPow, blockHash, height)
}

// resolveBlockHeight resolves the height of a block from its parent or metadata for AuxPow validation.
func (bc *BlockChain) resolveBlockHeight(block *btcutil.Block) int32 {
	prevHash := block.MsgBlock().Header.PrevBlock
	if bc.BlockChain != nil && !prevHash.IsEqual(&chainhash.Hash{}) {
		if parentHeight, err := bc.BlockChain.BlockHeightByHash(&prevHash); err == nil {
			return parentHeight + 1
		}
	}
	return block.Height()
}

// verifyAuxPowVersionBit checks that the AuxPow version bit is set on the block.
func (bc *BlockChain) verifyAuxPowVersionBit(block *btcutil.Block, height int32) error {
	version := block.MsgBlock().Header.Version
	if (version & config.AuxPowVersionBit) == 0 {
		return fmt.Errorf("block at height %d requires AuxPow version bit but it's not set", height)
	}
	return nil
}

// verifyChainID checks that the block's chain ID matches Namecoin's chain ID.
func (bc *BlockChain) verifyChainID(block *btcutil.Block, height int32) error {
	blockChainID := ExtractChainIDFromVersion(block.MsgBlock().Header.Version)
	if blockChainID != NamecoinChainID {
		return fmt.Errorf("block at height %d has invalid chain ID %d (expected %d)",
			height, blockChainID, NamecoinChainID)
	}
	return nil
}

// verifyAuxPowProof validates the AuxPow proof-of-work against the target difficulty.
func (bc *BlockChain) verifyAuxPowProof(block *btcutil.Block, auxPow *AuxPow, blockHash *chainhash.Hash, height int32) error {
	targetDifficulty := blockchain.CompactToBig(block.MsgBlock().Header.Bits)

	var targetHash chainhash.Hash
	targetBytes := targetDifficulty.Bytes()
	for i := 0; i < len(targetBytes); i++ {
		targetHash[len(targetBytes)-1-i] = targetBytes[i]
	}

	if err := auxPow.ValidateAuxPow(blockHash, &targetHash); err != nil {
		return fmt.Errorf("AuxPow validation failed for block %s at height %d: %w",
			blockHash.String(), height, err)
	}

	log.Printf("Successfully validated AuxPow for block %s at height %d", blockHash.String(), height)
	return nil
}
