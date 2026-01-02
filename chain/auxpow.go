package chain

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// AuxPow represents an auxiliary proof of work used in merged mining.
// This structure contains the proof that a block was included in a parent
// blockchain's (typically Bitcoin's) merkle tree, allowing Namecoin to
// leverage Bitcoin's proof of work.
//
// Per Namecoin Core src/primitives/pureheader.h and src/auxpow.cpp:
// An AuxPow consists of:
// 1. Coinbase transaction from parent chain containing the merge-mined block hash
// 2. Block hash of the merge-mined block
// 3. Merkle branch proving coinbase is in parent block
// 4. Merkle branch proving aux block hash is in coinbase
// 5. Parent block header
//
// References:
// - Namecoin Core: https://github.com/namecoin/namecoin-core/blob/master/src/primitives/pureheader.h
// - BIP: https://en.bitcoin.it/wiki/Merged_mining_specification
type AuxPow struct {
	// CoinbaseTx is the coinbase transaction from the parent blockchain
	// that commits to this block's hash in its scriptSig or output.
	CoinbaseTx wire.MsgTx

	// BlockHash is the hash of the auxiliary blockchain block (this Namecoin block).
	// This hash must appear in the parent chain's coinbase transaction.
	BlockHash chainhash.Hash

	// CoinbaseBranch is the merkle branch proving the coinbase transaction
	// is included in the parent block's merkle tree.
	CoinbaseBranch MerkleBranch

	// ChainMerkleBranch is the merkle branch proving the auxiliary block hash
	// is committed to in the coinbase transaction.
	ChainMerkleBranch MerkleBranch

	// ParentBlock is the header of the parent blockchain block.
	// This header's merkle root must match the merkle proof from CoinbaseBranch,
	// and its hash must meet the difficulty target.
	ParentBlock wire.BlockHeader

	// ChainID is the unique identifier for this blockchain in the merge-mining tree.
	// Namecoin uses chain ID 1. This prevents the same AuxPow from being used
	// across different merge-mined chains.
	// NOTE: This is derived from the coinbase transaction, not stored separately
	// in the wire format. It's extracted during validation.
}

// MerkleBranch represents a merkle branch proof in the AuxPow structure.
// A merkle branch is a path from a leaf (transaction) to the merkle root,
// consisting of the sibling hashes at each level of the tree.
//
// Per Namecoin Core src/auxpow.cpp (CAuxMerkleBranch):
// - Branch: List of sibling hashes in the merkle path
// - SideMask: Bit mask indicating whether the sibling is on the left (0) or right (1)
//
// Example: To prove a transaction is in a block with 8 transactions:
// - 3 levels of tree (2^3 = 8 leaves)
// - Branch has 3 hashes (one sibling at each level)
// - SideMask has 3 bits (indicating left/right position at each level)
type MerkleBranch struct {
	// Branch is the list of sibling hashes in the merkle path.
	// The length of this slice is the depth of the merkle tree from leaf to root.
	Branch []chainhash.Hash

	// SideMask is a bit mask indicating the position of the hash at each level.
	// Bit i corresponds to level i of the tree:
	// - 0 = sibling is on the right, we are on the left
	// - 1 = sibling is on the left, we are on the right
	// This determines whether to compute Hash(us || sibling) or Hash(sibling || us)
	SideMask uint32
}

// Namecoin chain ID for merge mining
// Per Namecoin Core src/auxpow.cpp: Namecoin uses chain ID = 1
const NamecoinChainID = 1

// AuxPow version bit - must be set in block version for AuxPow blocks
// This is already defined in config/config.go but repeated here for reference
// const AuxPowVersionBit = 0x100

// DeserializeAuxPow reads an AuxPow structure from a reader.
// This implements the wire protocol deserialization for AuxPow blocks.
//
// Per Namecoin Core wire format (src/auxpow.cpp):
// 1. Coinbase transaction (standard Bitcoin tx format)
// 2. Block hash (32 bytes)
// 3. Coinbase merkle branch (variable length)
// 4. Chain merkle branch (variable length)
// 5. Parent block header (80 bytes)
//
// Returns:
//   - AuxPow structure if successful
//   - Error if deserialization fails (malformed data, EOF, etc.)
func DeserializeAuxPow(r io.Reader) (*AuxPow, error) {
	auxpow := &AuxPow{}

	// 1. Read coinbase transaction
	if err := auxpow.CoinbaseTx.Deserialize(r); err != nil {
		return nil, fmt.Errorf("failed to deserialize coinbase tx: %w", err)
	}

	// 2. Read block hash (32 bytes)
	if _, err := io.ReadFull(r, auxpow.BlockHash[:]); err != nil {
		return nil, fmt.Errorf("failed to read block hash: %w", err)
	}

	// 3. Read coinbase merkle branch
	coinbaseBranch, err := DeserializeMerkleBranch(r)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize coinbase branch: %w", err)
	}
	auxpow.CoinbaseBranch = *coinbaseBranch

	// 4. Read chain merkle branch
	chainBranch, err := DeserializeMerkleBranch(r)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize chain branch: %w", err)
	}
	auxpow.ChainMerkleBranch = *chainBranch

	// 5. Read parent block header (80 bytes standard Bitcoin header)
	if err := auxpow.ParentBlock.Deserialize(r); err != nil {
		return nil, fmt.Errorf("failed to deserialize parent block header: %w", err)
	}

	return auxpow, nil
}

// SerializeAuxPow writes an AuxPow structure to a writer.
// This implements the wire protocol serialization for AuxPow blocks.
//
// Format matches DeserializeAuxPow:
// 1. Coinbase transaction
// 2. Block hash
// 3. Coinbase merkle branch
// 4. Chain merkle branch
// 5. Parent block header
func (ap *AuxPow) SerializeAuxPow(w io.Writer) error {
	// 1. Write coinbase transaction
	if err := ap.CoinbaseTx.Serialize(w); err != nil {
		return fmt.Errorf("failed to serialize coinbase tx: %w", err)
	}

	// 2. Write block hash
	if _, err := w.Write(ap.BlockHash[:]); err != nil {
		return fmt.Errorf("failed to write block hash: %w", err)
	}

	// 3. Write coinbase merkle branch
	if err := ap.CoinbaseBranch.SerializeMerkleBranch(w); err != nil {
		return fmt.Errorf("failed to serialize coinbase branch: %w", err)
	}

	// 4. Write chain merkle branch
	if err := ap.ChainMerkleBranch.SerializeMerkleBranch(w); err != nil {
		return fmt.Errorf("failed to serialize chain branch: %w", err)
	}

	// 5. Write parent block header
	if err := ap.ParentBlock.Serialize(w); err != nil {
		return fmt.Errorf("failed to serialize parent block header: %w", err)
	}

	return nil
}

// DeserializeMerkleBranch reads a MerkleBranch from a reader.
//
// Wire format:
// 1. Branch size (varint) - number of hashes in the branch
// 2. Branch hashes (32 bytes each)
// 3. Side mask (4 bytes, little-endian uint32)
func DeserializeMerkleBranch(r io.Reader) (*MerkleBranch, error) {
	branch := &MerkleBranch{}

	// Read branch size (number of hashes)
	branchSize, err := wire.ReadVarInt(r, 0) // 0 = protocol version (unused for varint)
	if err != nil {
		return nil, fmt.Errorf("failed to read branch size: %w", err)
	}

	// Sanity check: merkle tree depth should not exceed 32 levels
	// (2^32 transactions would be an impossibly large block)
	if branchSize > 32 {
		return nil, fmt.Errorf("merkle branch too deep: %d levels (max 32)", branchSize)
	}

	// Read branch hashes
	branch.Branch = make([]chainhash.Hash, branchSize)
	for i := uint64(0); i < branchSize; i++ {
		if _, err := io.ReadFull(r, branch.Branch[i][:]); err != nil {
			return nil, fmt.Errorf("failed to read branch hash %d: %w", i, err)
		}
	}

	// Read side mask (4 bytes, little-endian)
	var sideMaskBytes [4]byte
	if _, err := io.ReadFull(r, sideMaskBytes[:]); err != nil {
		return nil, fmt.Errorf("failed to read side mask: %w", err)
	}
	branch.SideMask = binary.LittleEndian.Uint32(sideMaskBytes[:])

	return branch, nil
}

// SerializeMerkleBranch writes a MerkleBranch to a writer.
//
// Wire format matches DeserializeMerkleBranch:
// 1. Branch size (varint)
// 2. Branch hashes (32 bytes each)
// 3. Side mask (4 bytes, little-endian uint32)
func (mb *MerkleBranch) SerializeMerkleBranch(w io.Writer) error {
	// Validate branch depth doesn't exceed maximum
	if len(mb.Branch) > 32 {
		return fmt.Errorf("merkle branch too deep: %d levels (max 32)", len(mb.Branch))
	}

	// Write branch size
	if err := wire.WriteVarInt(w, 0, uint64(len(mb.Branch))); err != nil {
		return fmt.Errorf("failed to write branch size: %w", err)
	}

	// Write branch hashes
	for i, hash := range mb.Branch {
		if _, err := w.Write(hash[:]); err != nil {
			return fmt.Errorf("failed to write branch hash %d: %w", i, err)
		}
	}

	// Write side mask (4 bytes, little-endian)
	var sideMaskBytes [4]byte
	binary.LittleEndian.PutUint32(sideMaskBytes[:], mb.SideMask)
	if _, err := w.Write(sideMaskBytes[:]); err != nil {
		return fmt.Errorf("failed to write side mask: %w", err)
	}

	return nil
}

// ValidateAuxPow validates an AuxPow proof.
// This is a placeholder for Phase 2 implementation.
//
// Full validation includes:
// 1. Verify coinbase merkle branch connects coinbase to parent block merkle root
// 2. Verify chain merkle branch connects aux block hash to coinbase
// 3. Verify parent block hash meets difficulty target
// 4. Extract and validate chain ID from coinbase
// 5. Verify aux block hash appears in correct position in coinbase
//
// Returns:
//   - nil if validation succeeds
//   - error describing validation failure
func (ap *AuxPow) ValidateAuxPow(blockHash *chainhash.Hash, chainID uint32, targetDifficulty *chainhash.Hash) error {
	// TODO: Phase 2 - Implement full validation
	// For now, return an error indicating AuxPow validation is not yet implemented
	return fmt.Errorf("AuxPow validation not yet implemented (Phase 2)")
}

// ExtractChainID extracts the chain ID from the coinbase transaction.
// This is a placeholder for Phase 2 implementation.
//
// Per Namecoin merged mining spec:
// The chain ID is encoded in the coinbase scriptSig or outputs.
// The exact location and format depends on the merge-mining protocol version.
//
// Returns:
//   - Chain ID if successfully extracted
//   - Error if chain ID cannot be found or is invalid
func (ap *AuxPow) ExtractChainID() (uint32, error) {
	// TODO: Phase 2 - Implement chain ID extraction
	// For now, return an error indicating extraction is not yet implemented
	return 0, fmt.Errorf("chain ID extraction not yet implemented (Phase 2)")
}

// CheckMerkleBranch verifies a merkle branch proof.
// This is a placeholder for Phase 2 implementation.
//
// Verifies that 'leaf' is included in a merkle tree with 'root' as the merkle root,
// using the provided merkle branch path.
//
// Arguments:
//   - leaf: The hash of the leaf node (e.g., transaction hash)
//   - branch: The merkle branch proof
//   - root: The expected merkle root
//
// Returns:
//   - true if the proof is valid
//   - false if the proof is invalid
func CheckMerkleBranch(leaf *chainhash.Hash, branch *MerkleBranch, root *chainhash.Hash) bool {
	// TODO: Phase 2 - Implement merkle branch verification
	// Algorithm:
	// 1. Start with leaf hash
	// 2. For each level in the branch:
	//    a. Get sibling hash from branch
	//    b. Check side mask bit to determine left/right position
	//    c. Compute parent = Hash(left || right)
	// 3. Compare final computed hash with root
	return false
}
