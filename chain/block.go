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
// The wire format for Namecoin blocks is:
// 1. Block header (80 bytes)
// 2. Transaction count (varint)
// 3. Transactions (variable length)
// 4. AuxPow data (if block version has AuxPow bit set)
//
// This function automatically detects whether AuxPow is present based on the block version
// and deserializes it if needed.
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
// 2. AuxPow blocks (>= height 19,200): Bitcoin block + AuxPow data appended
//
// The function reads:
// 1. Standard Bitcoin block (header + transactions)
// 2. If block version has AuxPow bit (0x100), reads AuxPow data
//
// Arguments:
//   - r: Reader containing the serialized block data
//
// Returns:
//   - Block with AuxPow populated (if block version indicates AuxPow)
//   - Error if deserialization fails (malformed block, incomplete AuxPow, etc.)
func NewBlockFromReader(r io.Reader) (*Block, error) {
	// Deserialize standard Bitcoin block (header + transactions)
	// This uses btcd's standard deserialization which handles:
	// - Block header (80 bytes)
	// - Transaction count (varint)
	// - Transactions (variable length)
	btcBlock, err := btcutil.NewBlockFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize block: %w", err)
	}

	block := &Block{
		Block:  btcBlock,
		auxPow: nil,
	}

	// Check if this block has AuxPow data
	// AuxPow is present if the block version has the AuxPow bit (0x100) set
	version := btcBlock.MsgBlock().Header.Version
	hasAuxPow := (version & config.AuxPowVersionBit) != 0

	if hasAuxPow {
		// Deserialize AuxPow data
		// Per Namecoin wire protocol, AuxPow data follows immediately after the transactions
		auxPow, err := DeserializeAuxPow(r)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize AuxPow: %w", err)
		}
		block.auxPow = auxPow
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
// Wire format:
// 1. Block header (80 bytes)
// 2. Transaction count (varint)
// 3. Transactions (variable length)
// 4. AuxPow data (if HasAuxPow() returns true)
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
	// Serialize standard block (header + transactions)
	msgBlock := b.MsgBlock()
	if err := msgBlock.Serialize(w); err != nil {
		return fmt.Errorf("failed to serialize block: %w", err)
	}

	// Serialize AuxPow if present
	if b.auxPow != nil {
		if err := b.auxPow.SerializeAuxPow(w); err != nil {
			return fmt.Errorf("failed to serialize AuxPow: %w", err)
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
