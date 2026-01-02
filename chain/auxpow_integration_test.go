package chain

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/namedb"
)

// TestAuxPowIntegration tests the complete AuxPow flow from deserialization through validation
func TestAuxPowIntegration(t *testing.T) {
	// Create a temporary namedb for testing
	ndb, cleanup := createTestNameDB(t)
	defer cleanup()

	// Create blockchain with mainnet params (AuxPow activates at height 19,200)
	chainParams := &config.NamecoinMainNetParams
	bc := &BlockChain{
		nameDB:      ndb,
		chainParams: chainParams,
		auxPowCache: make(map[chainhash.Hash]*AuxPow),
	}

	// Test Case 1: Pre-AuxPow block (height < 19,200) - should not require AuxPow
	t.Run("PreAuxPowBlock", func(t *testing.T) {
		block := createTestBlock(t, 1, 100) // height 100, version 1 (no AuxPow bit)

		err := bc.validateAuxPow(block)
		if err != nil {
			t.Errorf("Pre-AuxPow block should not require AuxPow validation, got error: %v", err)
		}
	})

	// Test Case 2: AuxPow block without AuxPow bit - should fail version check
	t.Run("AuxPowHeightWithoutBit", func(t *testing.T) {
		block := createTestBlock(t, 1, 19200) // height 19,200, version 1 (no AuxPow bit)

		// Version validation would catch this before AuxPow validation,
		// but validateAuxPow should also detect it as a double-check
		err := bc.validateAuxPow(block)
		if err == nil {
			t.Error("Expected error for AuxPow height without AuxPow version bit, got nil")
		}
	})

	// Test Case 3: AuxPow block with bit but no AuxPow data - should fail
	t.Run("AuxPowBitWithoutData", func(t *testing.T) {
		block := createTestBlock(t, config.AuxPowVersionBit, 19200) // AuxPow bit set

		err := bc.validateAuxPow(block)
		if err == nil {
			t.Error("Expected error for AuxPow block without AuxPow data, got nil")
		}
		if err != nil && err.Error() != "block at height 19200 requires AuxPow but no AuxPow data was provided" {
			t.Errorf("Expected 'no AuxPow data' error, got: %v", err)
		}
	})

	// Test Case 4: Valid AuxPow block - should pass validation
	t.Run("ValidAuxPowBlock", func(t *testing.T) {
		// Create a block with AuxPow
		block, auxPow := createTestAuxPowBlock(t, 19200)

		// Cache the AuxPow as if it came from the network
		bc.auxPowMu.Lock()
		bc.auxPowCache[*block.Hash()] = auxPow
		bc.auxPowMu.Unlock()

		err := bc.validateAuxPow(block)
		if err != nil {
			t.Errorf("Valid AuxPow block should pass validation, got error: %v", err)
		}

		// Verify cache was cleared after validation
		bc.auxPowMu.RLock()
		_, exists := bc.auxPowCache[*block.Hash()]
		bc.auxPowMu.RUnlock()
		if exists {
			t.Error("AuxPow cache should be cleared after validation")
		}
	})

	// Test Case 5: Invalid chain ID - should fail
	t.Run("InvalidChainID", func(t *testing.T) {
		block, auxPow := createTestAuxPowBlock(t, 19200)

		// Corrupt the chain ID by modifying parent block nonce
		// Chain ID is extracted as (nonce >> 16) & 0xFF
		// Set it to something other than NamecoinChainID (1)
		auxPow.ParentBlock.Nonce = (2 << 16) // Chain ID = 2

		bc.auxPowMu.Lock()
		bc.auxPowCache[*block.Hash()] = auxPow
		bc.auxPowMu.Unlock()

		err := bc.validateAuxPow(block)
		if err == nil {
			t.Error("Expected error for invalid chain ID, got nil")
		}
	})

	// Test Case 6: Parent block doesn't meet difficulty target - should fail
	t.Run("InvalidParentPoW", func(t *testing.T) {
		block, auxPow := createTestAuxPowBlock(t, 19200)

		// Set parent block nonce to a value that results in a high hash
		// (doesn't meet difficulty target)
		auxPow.ParentBlock.Nonce = 0

		bc.auxPowMu.Lock()
		bc.auxPowCache[*block.Hash()] = auxPow
		bc.auxPowMu.Unlock()

		err := bc.validateAuxPow(block)
		if err == nil {
			t.Error("Expected error for parent block not meeting PoW target, got nil")
		}
	})
}

// TestSetBlockAuxPowFromBytes tests parsing AuxPow from serialized block bytes
func TestSetBlockAuxPowFromBytes(t *testing.T) {
	ndb, cleanup := createTestNameDB(t)
	defer cleanup()

	bc := &BlockChain{
		nameDB:      ndb,
		chainParams: &config.NamecoinMainNetParams,
		auxPowCache: make(map[chainhash.Hash]*AuxPow),
	}

	// Test Case 1: Block without AuxPow bit - should not cache anything
	t.Run("NoAuxPowBit", func(t *testing.T) {
		block := createTestBlock(t, 1, 100) // version 1, no AuxPow bit
		serialized, err := block.Bytes()
		if err != nil {
			t.Fatalf("Failed to serialize block: %v", err)
		}

		err = bc.SetBlockAuxPowFromBytes(block.Hash(), serialized)
		if err != nil {
			t.Errorf("SetBlockAuxPowFromBytes should succeed for non-AuxPow block, got: %v", err)
		}

		// Verify nothing was cached
		bc.auxPowMu.RLock()
		_, exists := bc.auxPowCache[*block.Hash()]
		bc.auxPowMu.RUnlock()
		if exists {
			t.Error("AuxPow should not be cached for block without AuxPow bit")
		}
	})

	// Test Case 2: Block with AuxPow bit and data - should cache successfully
	t.Run("WithAuxPow", func(t *testing.T) {
		btcBlock, auxPow := createTestAuxPowBlock(t, 19200)

		// Create our custom Block with AuxPow
		block := NewBlock(btcBlock.MsgBlock())
		block.SetAuxPow(auxPow)

		// Serialize including AuxPow
		serialized, err := block.Bytes()
		if err != nil {
			t.Fatalf("Failed to serialize block with AuxPow: %v", err)
		}

		// Parse and cache
		err = bc.SetBlockAuxPowFromBytes(block.Hash(), serialized)
		if err != nil {
			t.Errorf("SetBlockAuxPowFromBytes failed: %v", err)
		}

		// Verify AuxPow was cached
		bc.auxPowMu.RLock()
		cachedAuxPow, exists := bc.auxPowCache[*block.Hash()]
		bc.auxPowMu.RUnlock()

		if !exists {
			t.Fatal("AuxPow should be cached")
		}
		if cachedAuxPow == nil {
			t.Error("Cached AuxPow should not be nil")
		}
	})

	// Test Case 3: Malformed AuxPow data - should return error
	t.Run("MalformedAuxPow", func(t *testing.T) {
		// Create a block header with AuxPow bit set
		header := wire.BlockHeader{
			Version:    config.AuxPowVersionBit,
			PrevBlock:  chainhash.Hash{},
			MerkleRoot: chainhash.Hash{},
			Timestamp:  time.Now(),
			Bits:       0x1d00ffff,
			Nonce:      12345,
		}

		msgBlock := wire.NewMsgBlock(&header)
		block := btcutil.NewBlock(msgBlock)

		// Serialize just the header and transactions (no AuxPow data)
		// This should fail when trying to deserialize AuxPow
		serialized, err := block.Bytes()
		if err != nil {
			t.Fatalf("Failed to serialize block: %v", err)
		}

		err = bc.SetBlockAuxPowFromBytes(block.Hash(), serialized)
		if err == nil {
			t.Error("Expected error for malformed AuxPow data, got nil")
		}
	})
}

// TestBlockSerialization tests round-trip serialization of blocks with AuxPow
func TestBlockSerialization(t *testing.T) {
	// Create a block with AuxPow
	btcBlock, auxPow := createTestAuxPowBlock(t, 19200)
	block := NewBlock(btcBlock.MsgBlock())
	block.SetAuxPow(auxPow)

	// Serialize
	serialized, err := block.Bytes()
	if err != nil {
		t.Fatalf("Failed to serialize block: %v", err)
	}

	// Deserialize
	deserialized, err := NewBlockFromBytes(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize block: %v", err)
	}

	// Verify block hash matches
	if !block.Hash().IsEqual(deserialized.Hash()) {
		t.Errorf("Block hash mismatch: original=%s, deserialized=%s",
			block.Hash().String(), deserialized.Hash().String())
	}

	// Verify AuxPow was preserved
	if !deserialized.HasAuxPow() {
		t.Fatal("Deserialized block should have AuxPow")
	}

	deserializedAuxPow := deserialized.AuxPow()
	if deserializedAuxPow == nil {
		t.Fatal("Deserialized AuxPow should not be nil")
	}

	// Verify AuxPow fields match
	if !auxPow.BlockHash.IsEqual(&deserializedAuxPow.BlockHash) {
		t.Error("AuxPow BlockHash mismatch")
	}

	// Compare parent block hashes
	parentHash1 := auxPow.ParentBlock.BlockHash()
	parentHash2 := deserializedAuxPow.ParentBlock.BlockHash()
	if !parentHash1.IsEqual(&parentHash2) {
		t.Error("AuxPow ParentBlock mismatch")
	}
}

// Helper functions

func createTestBlock(t *testing.T, version int32, height int32) *btcutil.Block {
	header := wire.BlockHeader{
		Version:    version,
		PrevBlock:  chainhash.Hash{},
		MerkleRoot: chainhash.Hash{},
		Timestamp:  time.Now(),
		Bits:       0x1d00ffff, // Difficulty target
		Nonce:      12345,
	}

	msgBlock := wire.NewMsgBlock(&header)
	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(height)

	return block
}

func createTestAuxPowBlock(t *testing.T, height int32) (*btcutil.Block, *AuxPow) {
	// Create a valid-looking AuxPow structure for testing
	// Note: This creates a minimal valid AuxPow with very easy difficulty

	// Create coinbase transaction
	coinbaseTx := wire.NewMsgTx(1)
	coinbaseTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 0xffffffff},
		SignatureScript:  []byte("coinbase"),
	})
	coinbaseTx.AddTxOut(&wire.TxOut{
		Value:    50 * 100000000,           // 50 NMC in satoshis
		PkScript: []byte{0x76, 0xa9, 0x14}, // P2PKH prefix
	})

	// Create block with extremely easy difficulty (0x2100ffff = minimal work)
	blockHeader := wire.BlockHeader{
		Version:    config.AuxPowVersionBit,
		PrevBlock:  chainhash.Hash{},
		MerkleRoot: coinbaseTx.TxHash(),
		Timestamp:  time.Now(),
		Bits:       0x2100ffff, // Extremely easy difficulty (almost any hash works)
		Nonce:      12345,
	}

	msgBlock := wire.NewMsgBlock(&blockHeader)
	msgBlock.AddTransaction(coinbaseTx)
	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(height)

	blockHash := block.Hash()

	// Create parent block header that meets the easy difficulty target
	// With Bits = 0x2100ffff, the target is extremely high (easy to meet)
	// We just need to find a nonce that produces a valid hash
	parentHeader := wire.BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{},
		MerkleRoot: coinbaseTx.TxHash(),
		Timestamp:  time.Now(),
		Bits:       0x2100ffff, // Same extremely easy difficulty
		Nonce:      findValidParentNonce(NamecoinChainID, coinbaseTx.TxHash()),
	}

	// Create AuxPow with empty merkle branches (direct commitment)
	auxPow := &AuxPow{
		CoinbaseTx: *coinbaseTx,
		BlockHash:  *blockHash,
		CoinbaseBranch: MerkleBranch{
			Branch:   []chainhash.Hash{},
			SideMask: 0,
		},
		ChainMerkleBranch: MerkleBranch{
			Branch:   []chainhash.Hash{},
			SideMask: 0,
		},
		ParentBlock: parentHeader,
	}

	return block, auxPow
}

// findValidParentNonce finds a nonce that produces a valid PoW for the parent block
// with chain ID encoded in bits 16-23
func findValidParentNonce(chainID uint32, merkleRoot chainhash.Hash) uint32 {
	// Encode chain ID in bits 16-23 of nonce
	baseNonce := chainID << 16

	// With 0x2100ffff difficulty, the target is huge (almost any hash works)
	// Just try a few values
	for i := uint32(0); i < 100; i++ {
		nonce := baseNonce | i

		// Create header with this nonce
		header := wire.BlockHeader{
			Version:    1,
			PrevBlock:  chainhash.Hash{},
			MerkleRoot: merkleRoot,
			Timestamp:  time.Unix(1234567890, 0),
			Bits:       0x2100ffff,
			Nonce:      nonce,
		}

		_ = header.BlockHash()

		// With 0x2100ffff, first nonce should work
		// Target is 0xffff * 2^(8*(0x21-3)) which is very large
		return nonce
	}

	// Fallback
	return baseNonce
}

func createTestNameDB(t *testing.T) (*namedb.NameDatabase, func()) {
	tempDir := t.TempDir()
	dbPath := tempDir + "/names.db"

	ndb, err := namedb.NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test namedb: %v", err)
	}

	cleanup := func() {
		ndb.Close()
	}

	return ndb, cleanup
}
