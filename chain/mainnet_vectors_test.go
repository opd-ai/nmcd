package chain

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/opd-ai/nmcd/config"
)

// TestMainnetBlockVectors tests AuxPoW validation against real Namecoin mainnet blocks
// This is the critical test for PROTOCOL_COMPLIANCE_AUDIT.md Issue #1
func TestMainnetBlockVectors(t *testing.T) {
	// Skip if test vectors haven't been extracted yet (contains PLACEHOLDER)
	// To populate real test vectors, run: scripts/extract_test_vectors.sh
	// See testdata/EXTRACTION_GUIDE.md for instructions

	vectorsDir := filepath.Join("..", "testdata", "blocks")
	vectors, err := LoadMainnetTestVectors(vectorsDir, "block_*.json")
	if err != nil {
		t.Skipf("Test vectors not available (this is expected): %v", err)
		return
	}

	if len(vectors) == 0 {
		t.Skip("No test vectors found - extract real blocks using scripts/extract_test_vectors.sh")
		return
	}

	t.Logf("Loaded %d mainnet block test vectors", len(vectors))

	// Track which blocks we've tested
	testedHeights := make(map[int32]bool)

	for _, vector := range vectors {
		// Skip placeholder vectors (not yet extracted from blockchain)
		if strings.Contains(vector.Hex, "PLACEHOLDER") {
			t.Logf("Skipping placeholder vector: %s (height %d)", vector.Description, vector.Height)
			continue
		}

		t.Run(vector.Description, func(t *testing.T) {
			testMainnetBlock(t, vector)
			testedHeights[vector.Height] = true
		})
	}

	// Report testing coverage
	criticalHeights := []int32{0, 19199, 19200, 19201}
	missingCritical := []int32{}
	for _, height := range criticalHeights {
		if !testedHeights[height] {
			missingCritical = append(missingCritical, height)
		}
	}

	if len(missingCritical) > 0 {
		t.Logf("⚠️  Missing critical block heights: %v", missingCritical)
		t.Logf("Extract these blocks using: scripts/extract_test_vectors.sh")
		t.Logf("See testdata/EXTRACTION_GUIDE.md for instructions")
	}

	if len(testedHeights) > 0 {
		t.Logf("✓ Successfully tested %d real mainnet blocks", len(testedHeights))
	} else {
		t.Skip("No real mainnet blocks available for testing - all vectors are placeholders")
	}
}

// testMainnetBlock validates a single mainnet block from a test vector
func testMainnetBlock(t *testing.T, vector *MainnetTestVector) {
	t.Logf("Testing block %d: %s", vector.Height, vector.Description)
	t.Logf("  Hash: %s", vector.Hash)
	t.Logf("  Network: %s", vector.Network)
	t.Logf("  Expected valid: %v", vector.Valid)
	if vector.Notes != "" {
		t.Logf("  Notes: %s", vector.Notes)
	}

	// Decode hex data
	blockBytes, err := vector.DecodeHex()
	if err != nil {
		t.Fatalf("Failed to decode block hex: %v", err)
	}

	t.Logf("  Block size: %d bytes", len(blockBytes))

	// Deserialize block
	block, err := NewBlockFromBytes(blockBytes)
	if err != nil {
		if vector.Valid {
			t.Fatalf("Failed to deserialize valid block: %v", err)
		} else {
			t.Logf("✓ Block correctly rejected during deserialization: %v", err)
			return
		}
	}

	if !vector.Valid {
		t.Fatalf("Invalid block should have failed deserialization")
	}

	// Verify block hash matches
	blockHash := block.Hash()
	expectedHash, err := chainhash.NewHashFromStr(vector.Hash)
	if err != nil {
		t.Fatalf("Invalid hash in test vector: %v", err)
	}

	if !blockHash.IsEqual(expectedHash) {
		t.Errorf("Block hash mismatch:\n  Expected: %s\n  Got:      %s",
			expectedHash.String(), blockHash.String())
	} else {
		t.Logf("  ✓ Block hash matches: %s", blockHash.String())
	}

	// Note: Block height is not stored in the serialized block data.
	// It's determined by the blockchain context (position in the chain).
	// The btcutil.Block.Height() method returns -1 for blocks not in a chain.
	// This is expected behavior, not a bug.
	if block.Height() == -1 {
		t.Logf("  Block height: not set (expected for deserialized blocks)")
	} else if block.Height() != vector.Height {
		t.Errorf("Block height mismatch: expected %d, got %d",
			vector.Height, block.Height())
	} else {
		t.Logf("  ✓ Block height matches: %d", vector.Height)
	}

	// Check AuxPoW based on expected height from test vector
	requiresAuxPow := vector.Height >= config.MainNetAuxPowActivationHeight
	hasAuxPow := block.HasAuxPow()

	t.Logf("  Requires AuxPoW: %v (height >= %d)", requiresAuxPow, config.MainNetAuxPowActivationHeight)
	t.Logf("  Has AuxPoW: %v", hasAuxPow)

	if requiresAuxPow && !hasAuxPow {
		t.Errorf("Block at height %d should have AuxPoW but doesn't", vector.Height)
		return
	}

	if !requiresAuxPow && hasAuxPow {
		t.Errorf("Block at height %d should not have AuxPoW but does", vector.Height)
		return
	}

	// Validate AuxPoW if present
	if hasAuxPow {
		t.Log("  Validating AuxPoW...")
		auxPow := block.AuxPow()
		if auxPow == nil {
			t.Fatal("HasAuxPow() returned true but AuxPow() returned nil")
		}

		// Extract and verify chain ID from the Namecoin block's version
		// The chain ID is in bits 16+ of the block version, not in the AuxPow structure
		blockVersion := block.MsgBlock().Header.Version
		chainID := ExtractChainIDFromVersion(blockVersion)
		t.Logf("    ✓ Chain ID: %d (expected: %d for Namecoin)", chainID, NamecoinChainID)
		if chainID != NamecoinChainID {
			t.Errorf("Invalid chain ID: expected %d (Namecoin), got %d",
				NamecoinChainID, chainID)
		}

		// Validate parent block PoW
		// Note: We use the block's difficulty target from the header
		targetDifficulty := blockchain.CompactToBig(block.MsgBlock().Header.Bits)

		// Convert to chainhash.Hash (big.Int is big-endian, Hash expects little-endian for HashToBig)
		var targetHash chainhash.Hash
		targetBytes := targetDifficulty.Bytes()
		for i := 0; i < len(targetBytes); i++ {
			targetHash[len(targetBytes)-1-i] = targetBytes[i]
		}

		err = auxPow.ValidateAuxPow(blockHash, NamecoinChainID, &targetHash)
		if err != nil {
			t.Errorf("AuxPoW validation failed: %v", err)
		} else {
			t.Log("    ✓ AuxPoW validation passed")
		}

		// Verify AuxPow structure details
		t.Logf("    Coinbase merkle branch size: %d", len(auxPow.CoinbaseBranch.Branch))
		t.Logf("    Chain merkle branch size: %d", len(auxPow.ChainMerkleBranch.Branch))
		t.Logf("    Parent block version: 0x%08x", auxPow.ParentBlock.Version)
		t.Logf("    Parent block difficulty: 0x%08x", auxPow.ParentBlock.Bits)
	} else {
		t.Log("  No AuxPoW (pre-activation block)")
	}

	// Verify block version for post-AuxPoW blocks
	version := block.MsgBlock().Header.Version
	hasAuxPowBit := (version & config.AuxPowVersionBit) != 0

	if requiresAuxPow && !hasAuxPowBit {
		t.Errorf("Block at height %d should have AuxPoW version bit (0x%x) set, version: 0x%08x",
			vector.Height, config.AuxPowVersionBit, version)
	} else if requiresAuxPow {
		t.Logf("  ✓ AuxPoW version bit set: 0x%08x", version)
	} else {
		t.Logf("  ✓ Pre-AuxPoW block version: 0x%08x", version)
	}

	t.Logf("✓ Block %d validation complete", vector.Height)
}

// TestMainnetBlockVector_Genesis specifically tests the genesis block if available
func TestMainnetBlockVector_Genesis(t *testing.T) {
	vector, err := LoadMainnetTestVector(filepath.Join("..", "testdata", "blocks", "block_0_genesis.json"))
	if err != nil {
		t.Skipf("Genesis block test vector not available: %v", err)
		return
	}

	if strings.Contains(vector.Hex, "PLACEHOLDER") {
		t.Skip("Genesis block not yet extracted - see testdata/EXTRACTION_GUIDE.md")
		return
	}

	testMainnetBlock(t, vector)
}

// TestMainnetBlockVector_PreAuxPoW tests the last pre-AuxPoW block
func TestMainnetBlockVector_PreAuxPoW(t *testing.T) {
	vector, err := LoadMainnetTestVector(filepath.Join("..", "testdata", "blocks", "block_19199_pre_auxpow.json"))
	if err != nil {
		t.Skipf("Pre-AuxPoW block test vector not available: %v", err)
		return
	}

	if strings.Contains(vector.Hex, "PLACEHOLDER") {
		t.Skip("Block 19,199 not yet extracted - see testdata/EXTRACTION_GUIDE.md")
		return
	}

	testMainnetBlock(t, vector)

	// Additional checks specific to pre-AuxPoW block
	blockBytes, _ := vector.DecodeHex()
	block, _ := NewBlockFromBytes(blockBytes)

	if block.HasAuxPow() {
		t.Error("Block 19,199 should NOT have AuxPoW")
	}

	version := block.MsgBlock().Header.Version
	if (version & config.AuxPowVersionBit) != 0 {
		t.Errorf("Block 19,199 should NOT have AuxPoW version bit set, got: 0x%08x", version)
	}
}

// TestMainnetBlockVector_AuxPowActivation tests the critical AuxPoW activation block
func TestMainnetBlockVector_AuxPowActivation(t *testing.T) {
	vector, err := LoadMainnetTestVector(filepath.Join("..", "testdata", "blocks", "block_19200_auxpow_activation.json"))
	if err != nil {
		t.Skipf("AuxPoW activation block test vector not available: %v", err)
		return
	}

	if strings.Contains(vector.Hex, "PLACEHOLDER") {
		t.Skip("Block 19,200 (AuxPoW activation) not yet extracted")
		t.Log("⚠️  CRITICAL: This is the most important test vector for AuxPoW validation!")
		t.Log("Extract using: namecoin-cli getblock $(namecoin-cli getblockhash 19200) 0")
		t.Log("See testdata/EXTRACTION_GUIDE.md for full instructions")
		return
	}

	testMainnetBlock(t, vector)

	// Additional checks specific to AuxPoW activation block
	blockBytes, _ := vector.DecodeHex()
	block, _ := NewBlockFromBytes(blockBytes)

	if !block.HasAuxPow() {
		t.Fatal("Block 19,200 MUST have AuxPoW - this is the activation block!")
	}

	version := block.MsgBlock().Header.Version
	if (version & config.AuxPowVersionBit) == 0 {
		t.Fatalf("Block 19,200 MUST have AuxPoW version bit set, got: 0x%08x", version)
	}

	auxPow := block.AuxPow()
	if auxPow == nil {
		t.Fatal("AuxPoW should not be nil for activation block")
	}

	// Verify this is truly merged mining with Namecoin chain ID
	// Chain ID is extracted from the Namecoin block's version, not from AuxPow
	blockVersion := block.MsgBlock().Header.Version
	chainID := ExtractChainIDFromVersion(blockVersion)
	if chainID != NamecoinChainID {
		t.Errorf("AuxPoW activation block should have Namecoin chain ID (%d), got %d",
			NamecoinChainID, chainID)
	}

	t.Log("✓ AuxPoW activation block validation complete")
	t.Log("This confirms nmcd can validate the critical consensus-changing block!")
}

// TestMainnetBlockVector_Coverage reports on test vector coverage
func TestMainnetBlockVector_Coverage(t *testing.T) {
	vectorsDir := filepath.Join("..", "testdata", "blocks")
	vectors, err := LoadMainnetTestVectors(vectorsDir, "block_*.json")
	if err != nil {
		t.Skipf("Test vectors not available: %v", err)
		return
	}

	// Define critical blocks for consensus validation
	criticalBlocks := map[int32]string{
		0:      "Genesis block",
		19199:  "Last pre-AuxPoW block",
		19200:  "AuxPoW activation (MOST CRITICAL)",
		19201:  "Second AuxPoW block",
		210000: "First subsidy halving",
	}

	// Check which critical blocks we have
	available := make(map[int32]bool)
	extracted := make(map[int32]bool)

	for _, vector := range vectors {
		available[vector.Height] = true
		if !strings.Contains(vector.Hex, "PLACEHOLDER") {
			extracted[vector.Height] = true
		}
	}

	t.Log("Test Vector Coverage Report")
	t.Log("============================")
	t.Logf("Total vectors: %d", len(vectors))

	var extractedCount int
	for _, isExtracted := range extracted {
		if isExtracted {
			extractedCount++
		}
	}
	t.Logf("Extracted vectors: %d", extractedCount)
	t.Logf("Placeholder vectors: %d", len(vectors)-extractedCount)
	t.Log("")

	t.Log("Critical Block Status:")
	for height, description := range criticalBlocks {
		status := "❌ MISSING"
		if available[height] {
			if extracted[height] {
				status = "✓ EXTRACTED"
			} else {
				status = "⚠️  PLACEHOLDER"
			}
		}
		t.Logf("  Block %6d: %s - %s", height, status, description)
	}
	t.Log("")

	if extractedCount == 0 {
		t.Log("⚠️  No real mainnet blocks have been extracted yet!")
		t.Log("To complete PROTOCOL_COMPLIANCE_AUDIT.md Issue #1:")
		t.Log("1. Run scripts/extract_test_vectors.sh (requires Namecoin Core)")
		t.Log("2. See testdata/EXTRACTION_GUIDE.md for detailed instructions")
		t.Log("3. Re-run tests to validate against real blockchain data")
	} else if extractedCount < len(criticalBlocks) {
		t.Logf("⚠️  %d of %d critical blocks still need extraction",
			len(criticalBlocks)-extractedCount, len(criticalBlocks))
	} else {
		t.Log("✓ All critical test vectors extracted!")
		t.Log("nmcd AuxPoW validation has been tested against real mainnet blocks")
	}
}
