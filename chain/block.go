package chain

import (
	"bytes"
	"fmt"
	"io"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/config"
)

// Block extends btcutil.Block with Namecoin-specific AuxPow data.
// This wrapper allows us to work with both standard Bitcoin blocks (pre-AuxPow)
// and Namecoin merged-mined blocks (post-AuxPow activation at height 19,200).
//
// Design: Instead of forking btcd's Block type, we compose it and add AuxPow as an
// additional field. This maintains compatibility with btcd's blockchain package while
// adding Namecoin-specific functionality.
type Block struct {
	*btcutil.Block
	auxPow *AuxPow // nil for pre-AuxPow blocks, populated for blocks >= 19,200
}

// NewBlock creates a new Block from a wire.MsgBlock.
// The AuxPow field is initially nil and must be set separately via SetAuxPow()
// or by using NewBlockFromReader() which deserializes the full block including AuxPow.
func NewBlock(msgBlock *wire.MsgBlock) *Block {
	return &Block{
		Block:  btcutil.NewBlock(msgBlock),
		auxPow: nil,
	}
}

// NewBlockFromBytes deserializes a block from a byte slice, including AuxPow data if present.
//
// The wire format for Namecoin blocks differs based on the block version:
//
// Pre-AuxPoW blocks (version bit 0x100 NOT set):
//  1. Block header (80 bytes)
//  2. Transaction count (varint)
//  3. Transactions (variable length)
//
// AuxPoW blocks (version bit 0x100 set):
//  1. Block header (80 bytes)
//  2. AuxPoW data (variable length) - BEFORE transactions!
//  3. Transaction count (varint)
//  4. Transactions (variable length)
//
// This function automatically detects whether AuxPow is present based on the block version
// and deserializes it in the correct order.
//
// Arguments:
//   - serializedBlock: The complete serialized block including AuxPow (if present)
//
// Returns:
//   - Block with AuxPow populated (if block version indicates AuxPow)
//   - Error if deserialization fails
func NewBlockFromBytes(serializedBlock []byte) (*Block, error) {
	return NewBlockFromReader(bytes.NewReader(serializedBlock))
}

// NewBlockFromReader deserializes a block from an io.Reader, including AuxPow data if present.
//
// This is the primary deserialization function for Namecoin blocks. It handles both:
// 1. Pre-AuxPow blocks (< height 19,200): Standard Bitcoin block format
// 2. AuxPow blocks (>= height 19,200): Header + AuxPow + Transactions
//
// IMPORTANT: The wire format for AuxPoW blocks places the AuxPoW data BEFORE
// the transaction count and transactions, NOT after. This differs from what
// one might expect and requires custom deserialization logic.
//
// Reference: This ordering (AuxPoW serialized immediately after the header and before
// the transaction vector) matches Namecoin Core's wire format as implemented in
// src/primitives/block.h (CBlockHeader serialization with AuxPow) and related AuxPow
// handling code. Maintain this ordering to stay consensus-compatible.
//
// Wire format for AuxPoW blocks:
//  1. Block header (80 bytes)
//  2. AuxPoW structure:
//     - Coinbase TX from parent chain
//     - Block hash (32 bytes)
//     - Coinbase merkle branch
//     - Chain merkle branch
//     - Parent block header (80 bytes)
//  3. Transaction count (varint)
//  4. Block transactions
//
// Arguments:
//   - r: Reader containing the serialized block data
//
// Returns:
//   - Block with AuxPow populated (if block version indicates AuxPow)
//   - Error if deserialization fails (malformed block, incomplete AuxPow, etc.)
func NewBlockFromReader(r io.Reader) (*Block, error) {
	// Step 1: Read block header (80 bytes)
	var header wire.BlockHeader
	if err := header.Deserialize(r); err != nil {
		return nil, fmt.Errorf("failed to deserialize block header: %w", err)
	}

	// Step 2: Check if this block has AuxPow data
	// AuxPow is present if the block version has the AuxPow bit (0x100) set
	hasAuxPow := (header.Version & config.AuxPowVersionBit) != 0

	var auxPow *AuxPow
	if hasAuxPow {
		// Step 3a: For AuxPoW blocks, read AuxPoW data BEFORE transactions
		var err error
		auxPow, err = DeserializeAuxPow(r)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize AuxPow: %w", err)
		}
	}

	// Step 4: Read transaction count (varint)
	txCount, err := wire.ReadVarInt(r, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to read transaction count: %w", err)
	}

	// Sanity check: prevent excessive memory allocation
	// MaxBlockPayload (4MB) / minimum tx size (~60 bytes) ≈ 66K transactions
	// Using 100K as a safe upper bound
	const maxTxPerBlock = 100000
	if txCount > maxTxPerBlock {
		return nil, fmt.Errorf("transaction count %d exceeds maximum %d", txCount, maxTxPerBlock)
	}

	// Step 5: Read transactions
	transactions := make([]*wire.MsgTx, 0, txCount)
	for i := uint64(0); i < txCount; i++ {
		tx := &wire.MsgTx{}
		if err := tx.Deserialize(r); err != nil {
			return nil, fmt.Errorf("failed to deserialize transaction %d: %w", i, err)
		}
		transactions = append(transactions, tx)
	}

	// Step 6: Construct the block
	msgBlock := &wire.MsgBlock{
		Header:       header,
		Transactions: transactions,
	}

	block := &Block{
		Block:  btcutil.NewBlock(msgBlock),
		auxPow: auxPow,
	}

	return block, nil
}

// AuxPow returns the AuxPow data for this block, or nil if the block is pre-AuxPow.
//
// Returns:
//   - AuxPow structure for merged-mined blocks (version bit 0x100 set)
//   - nil for pre-AuxPow blocks or if AuxPow was not deserialized
func (b *Block) AuxPow() *AuxPow {
	return b.auxPow
}

// SetAuxPow sets the AuxPow data for this block.
// This is used when constructing blocks programmatically (e.g., in tests).
//
// Arguments:
//   - auxPow: The AuxPow structure to attach to this block (can be nil)
func (b *Block) SetAuxPow(auxPow *AuxPow) {
	b.auxPow = auxPow
}

// HasAuxPow returns true if this block has AuxPow data.
// This checks both:
// 1. The block version has the AuxPow bit set
// 2. The auxPow field is non-nil
//
// Returns:
//   - true if block should have and does have AuxPow
//   - false for pre-AuxPow blocks or if AuxPow is missing
func (b *Block) HasAuxPow() bool {
	version := b.MsgBlock().Header.Version
	hasAuxPowBit := (version & config.AuxPowVersionBit) != 0
	return hasAuxPowBit && b.auxPow != nil
}

// Serialize writes the complete block to a writer, including AuxPow if present.
//
// Wire format for AuxPoW blocks:
//  1. Block header (80 bytes)
//  2. AuxPoW data (if HasAuxPow() returns true) - BEFORE transactions!
//  3. Transaction count (varint)
//  4. Transactions (variable length)
//
// Wire format for pre-AuxPoW blocks:
//  1. Block header (80 bytes)
//  2. Transaction count (varint)
//  3. Transactions (variable length)
//
// This produces a serialized block that can be sent over the Namecoin P2P network
// or stored in block files.
//
// Arguments:
//   - w: Writer to serialize the block to
//
// Returns:
//   - nil on success
//   - error if serialization fails
func (b *Block) Serialize(w io.Writer) error {
	msgBlock := b.MsgBlock()

	// Step 1: Serialize block header (80 bytes)
	if err := msgBlock.Header.Serialize(w); err != nil {
		return fmt.Errorf("failed to serialize block header: %w", err)
	}

	// Step 2: Serialize AuxPow if present (BEFORE transactions)
	if b.auxPow != nil {
		if err := b.auxPow.SerializeAuxPow(w); err != nil {
			return fmt.Errorf("failed to serialize AuxPow: %w", err)
		}
	}

	// Step 3: Serialize transaction count
	if err := wire.WriteVarInt(w, 0, uint64(len(msgBlock.Transactions))); err != nil {
		return fmt.Errorf("failed to serialize transaction count: %w", err)
	}

	// Step 4: Serialize transactions
	for i, tx := range msgBlock.Transactions {
		if err := tx.Serialize(w); err != nil {
			return fmt.Errorf("failed to serialize transaction %d: %w", i, err)
		}
	}

	return nil
}

// Bytes returns the serialized block as a byte slice, including AuxPow if present.
//
// Returns:
//   - Byte slice containing the complete serialized block
//   - Error if serialization fails
func (b *Block) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := b.Serialize(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
