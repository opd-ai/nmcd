// Package auxpow implements Namecoin's merged mining (AuxPow) support.
//
// AuxPow (Auxiliary Proof of Work) allows Namecoin to be merge-mined with Bitcoin.
// Miners can simultaneously mine both Bitcoin and Namecoin blocks with the same
// computational work by embedding a commitment to the Namecoin block in the Bitcoin
// coinbase transaction.
//
// This implementation follows the Namecoin Core specification (src/auxpow.cpp) and
// the merged mining BIP: https://en.bitcoin.it/wiki/Merged_mining_specification
package auxpow

import (
	"bytes"
	"fmt"
	"io"
	"math/big"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// AuxPow represents an auxiliary proof of work for merged mining.
//
// When a block has the AuxPow version bit set (0x100), it must include AuxPow data
// that proves the block was merge-mined with sufficient proof of work from the parent
// blockchain (typically Bitcoin).
//
// Structure (per Namecoin Core):
//   - ParentCoinbaseTx: The parent blockchain's coinbase transaction containing the
//     commitment to this auxiliary block
//   - CoinbaseBranch: Merkle branch linking the coinbase tx to parent block's merkle root
//   - ChainMerkleBranch: Merkle branch for the auxiliary blockchain (for multi-merge-mining)
//   - ParentBlock: The parent block header that provides the actual proof of work
//   - ChainID: Identifies which auxiliary chain this is (Namecoin = 1)
type AuxPow struct {
	// ParentCoinbaseTx is the coinbase transaction of the parent block.
	// This transaction must include a commitment to the auxiliary block hash
	// in its scriptSig or outputs.
	ParentCoinbaseTx *wire.MsgTx

	// CoinbaseBranch is the merkle branch linking ParentCoinbaseTx to
	// the merkle root in ParentBlock. This proves the coinbase transaction
	// is part of the parent block.
	CoinbaseBranch []chainhash.Hash

	// CoinbaseBranchSideMask indicates which side of the merkle tree each
	// hash in CoinbaseBranch is on (0 = left, 1 = right).
	// Encoded as a bitmask where bit N corresponds to CoinbaseBranch[N].
	CoinbaseBranchSideMask uint32

	// ChainMerkleBranch is the merkle branch for multi-merge-mining support.
	// For Namecoin (single merge-mined chain), this is typically empty.
	ChainMerkleBranch []chainhash.Hash

	// ChainMerkleBranchSideMask indicates which side each hash in
	// ChainMerkleBranch is on. Encoded as bitmask.
	ChainMerkleBranchSideMask uint32

	// ParentBlock is the block header of the parent blockchain (Bitcoin).
	// This header's proof of work is used to secure the auxiliary block.
	// The merkle root in this header must match the root computed from
	// CoinbaseBranch and ParentCoinbaseTx.
	ParentBlock wire.BlockHeader
}

// Serialize encodes the AuxPow to a writer using Bitcoin wire protocol format.
//
// Format (per Namecoin Core src/auxpow.cpp):
//   - Parent coinbase transaction (wire.MsgTx format)
//   - Parent block hash (32 bytes, chainhash.Hash)
//   - Coinbase merkle branch (varint count + hashes)
//   - Coinbase branch side mask (uint32)
//   - Chain merkle branch (varint count + hashes)
//   - Chain branch side mask (uint32)
//   - Parent block header (80 bytes, wire.BlockHeader)
func (ap *AuxPow) Serialize(w io.Writer) error {
	// Serialize parent coinbase transaction
	if err := ap.ParentCoinbaseTx.Serialize(w); err != nil {
		return fmt.Errorf("failed to serialize parent coinbase: %w", err)
	}

	// Serialize parent block hash (the hash of ParentBlock)
	parentHash := ap.ParentBlock.BlockHash()
	if _, err := w.Write(parentHash[:]); err != nil {
		return fmt.Errorf("failed to write parent block hash: %w", err)
	}

	// Serialize coinbase merkle branch
	if err := serializeMerkleBranch(w, ap.CoinbaseBranch); err != nil {
		return fmt.Errorf("failed to serialize coinbase branch: %w", err)
	}

	// Serialize coinbase branch side mask (uint32, little-endian)
	var sideMaskBytes [4]byte
	sideMaskBytes[0] = byte(ap.CoinbaseBranchSideMask)
	sideMaskBytes[1] = byte(ap.CoinbaseBranchSideMask >> 8)
	sideMaskBytes[2] = byte(ap.CoinbaseBranchSideMask >> 16)
	sideMaskBytes[3] = byte(ap.CoinbaseBranchSideMask >> 24)
	if _, err := w.Write(sideMaskBytes[:]); err != nil {
		return fmt.Errorf("failed to write coinbase side mask: %w", err)
	}

	// Serialize chain merkle branch
	if err := serializeMerkleBranch(w, ap.ChainMerkleBranch); err != nil {
		return fmt.Errorf("failed to serialize chain branch: %w", err)
	}

	// Serialize chain branch side mask (uint32, little-endian)
	var chainMaskBytes [4]byte
	chainMaskBytes[0] = byte(ap.ChainMerkleBranchSideMask)
	chainMaskBytes[1] = byte(ap.ChainMerkleBranchSideMask >> 8)
	chainMaskBytes[2] = byte(ap.ChainMerkleBranchSideMask >> 16)
	chainMaskBytes[3] = byte(ap.ChainMerkleBranchSideMask >> 24)
	if _, err := w.Write(chainMaskBytes[:]); err != nil {
		return fmt.Errorf("failed to write chain side mask: %w", err)
	}

	// Serialize parent block header
	if err := ap.ParentBlock.Serialize(w); err != nil {
		return fmt.Errorf("failed to serialize parent block: %w", err)
	}

	return nil
}

// Deserialize decodes an AuxPow from a reader.
func (ap *AuxPow) Deserialize(r io.Reader) error {
	// Deserialize parent coinbase transaction
	ap.ParentCoinbaseTx = &wire.MsgTx{}
	if err := ap.ParentCoinbaseTx.Deserialize(r); err != nil {
		return fmt.Errorf("failed to deserialize parent coinbase: %w", err)
	}

	// Deserialize parent block hash (we read it but verify against ParentBlock later)
	var parentHash chainhash.Hash
	if _, err := io.ReadFull(r, parentHash[:]); err != nil {
		return fmt.Errorf("failed to read parent block hash: %w", err)
	}

	// Deserialize coinbase merkle branch
	var err error
	ap.CoinbaseBranch, err = deserializeMerkleBranch(r)
	if err != nil {
		return fmt.Errorf("failed to deserialize coinbase branch: %w", err)
	}

	// Deserialize coinbase branch side mask (uint32, little-endian)
	var sideMaskBytes [4]byte
	if _, err := io.ReadFull(r, sideMaskBytes[:]); err != nil {
		return fmt.Errorf("failed to read coinbase side mask: %w", err)
	}
	ap.CoinbaseBranchSideMask = uint32(sideMaskBytes[0]) |
		uint32(sideMaskBytes[1])<<8 |
		uint32(sideMaskBytes[2])<<16 |
		uint32(sideMaskBytes[3])<<24

	// Deserialize chain merkle branch
	ap.ChainMerkleBranch, err = deserializeMerkleBranch(r)
	if err != nil {
		return fmt.Errorf("failed to deserialize chain branch: %w", err)
	}

	// Deserialize chain branch side mask (uint32, little-endian)
	var chainMaskBytes [4]byte
	if _, err := io.ReadFull(r, chainMaskBytes[:]); err != nil {
		return fmt.Errorf("failed to read chain side mask: %w", err)
	}
	ap.ChainMerkleBranchSideMask = uint32(chainMaskBytes[0]) |
		uint32(chainMaskBytes[1])<<8 |
		uint32(chainMaskBytes[2])<<16 |
		uint32(chainMaskBytes[3])<<24

	// Deserialize parent block header
	if err := ap.ParentBlock.Deserialize(r); err != nil {
		return fmt.Errorf("failed to deserialize parent block: %w", err)
	}

	// Verify the parent block hash matches
	computedHash := ap.ParentBlock.BlockHash()
	if !parentHash.IsEqual(&computedHash) {
		return fmt.Errorf("parent block hash mismatch: expected %s, got %s",
			parentHash, computedHash)
	}

	return nil
}

// serializeMerkleBranch serializes a merkle branch (array of hashes) to a writer.
// Format: varint count + 32-byte hashes
func serializeMerkleBranch(w io.Writer, branch []chainhash.Hash) error {
	// Write branch length as varint
	count := uint64(len(branch))
	if err := wire.WriteVarInt(w, 0, count); err != nil {
		return err
	}

	// Write each hash
	for i := range branch {
		if _, err := w.Write(branch[i][:]); err != nil {
			return err
		}
	}

	return nil
}

// deserializeMerkleBranch deserializes a merkle branch from a reader.
func deserializeMerkleBranch(r io.Reader) ([]chainhash.Hash, error) {
	// Read branch length
	count, err := wire.ReadVarInt(r, 0)
	if err != nil {
		return nil, err
	}

	// Sanity check: merkle branches shouldn't be longer than block height log2
	// Bitcoin block height is ~800k, log2(800000) ≈ 20, use 30 as safe upper bound
	if count > 30 {
		return nil, fmt.Errorf("merkle branch too long: %d (max 30)", count)
	}

	// Read hashes
	branch := make([]chainhash.Hash, count)
	for i := uint64(0); i < count; i++ {
		if _, err := io.ReadFull(r, branch[i][:]); err != nil {
			return nil, err
		}
	}

	return branch, nil
}

// CheckMerkleBranch verifies a merkle branch proof.
//
// Given a leaf hash, merkle branch, and side mask, computes the merkle root
// and compares it to the expected root. Returns true if the proof is valid.
//
// Parameters:
//   - leaf: The hash to verify (e.g., transaction hash)
//   - branch: Array of merkle branch hashes
//   - sideMask: Bitmask indicating which side each branch hash is on (0=left, 1=right)
//   - expectedRoot: The merkle root to verify against
//
// Algorithm (per Bitcoin merkle tree construction):
//  1. Start with leaf hash
//  2. For each branch hash (index i):
//     - If bit i of sideMask is 0: hash = Hash(branch[i] || current)
//     - If bit i of sideMask is 1: hash = Hash(current || branch[i])
//  3. Compare final hash to expectedRoot
func CheckMerkleBranch(leaf chainhash.Hash, branch []chainhash.Hash, sideMask uint32, expectedRoot chainhash.Hash) bool {
	current := leaf

	for i, branchHash := range branch {
		// Determine which side the branch hash is on
		if sideMask&(1<<uint(i)) == 0 {
			// Branch hash is on the left
			current = HashMerkleBranches(&branchHash, &current)
		} else {
			// Branch hash is on the right
			current = HashMerkleBranches(&current, &branchHash)
		}
	}

	return current.IsEqual(&expectedRoot)
}

// HashMerkleBranches combines two merkle tree nodes using double SHA256.
// This matches Bitcoin's merkle tree hashing: Hash(left || right)
func HashMerkleBranches(left, right *chainhash.Hash) chainhash.Hash {
	// Concatenate left and right hashes
	var data [64]byte
	copy(data[:32], left[:])
	copy(data[32:], right[:])

	// Double SHA256 (Bitcoin standard)
	return chainhash.DoubleHashH(data[:])
}

// Validate performs consensus validation on the AuxPow.
//
// This checks:
// 1. Parent block meets difficulty target
// 2. Coinbase merkle branch links to parent block's merkle root
// 3. Chain merkle branch is valid (for multi-merge-mining)
// 4. Coinbase contains commitment to auxiliary block hash
// 5. Chain ID is correct (Namecoin = 1)
//
// Parameters:
//   - auxBlockHash: Hash of the auxiliary block (Namecoin block being validated)
//   - chainID: Expected chain ID (1 for Namecoin)
//   - powLimit: Maximum allowed difficulty target for parent block
//
// Returns error if validation fails.
func (ap *AuxPow) Validate(auxBlockHash chainhash.Hash, chainID int32, powLimit *big.Int) error {
	// 1. Validate parent block proof of work
	parentBlockHash := ap.ParentBlock.BlockHash()
	if err := checkProofOfWork(&parentBlockHash, ap.ParentBlock.Bits, powLimit); err != nil {
		return fmt.Errorf("parent block proof of work invalid: %w", err)
	}

	// 2. Validate coinbase merkle branch
	// The coinbase transaction must be part of the parent block's merkle tree
	coinbaseTxHash := ap.ParentCoinbaseTx.TxHash()
	if !CheckMerkleBranch(coinbaseTxHash, ap.CoinbaseBranch, ap.CoinbaseBranchSideMask, ap.ParentBlock.MerkleRoot) {
		return fmt.Errorf("coinbase merkle branch verification failed")
	}

	// 3. Validate chain merkle branch (for multi-merge-mining)
	// For Namecoin (single auxiliary chain), this is typically empty or has special handling
	// We compute the chain merkle root from auxBlockHash and verify it's in the coinbase
	chainMerkleRoot := auxBlockHash
	if len(ap.ChainMerkleBranch) > 0 {
		// Compute chain merkle root from aux block hash and branch
		for i, branchHash := range ap.ChainMerkleBranch {
			if ap.ChainMerkleBranchSideMask&(1<<uint(i)) == 0 {
				chainMerkleRoot = HashMerkleBranches(&branchHash, &chainMerkleRoot)
			} else {
				chainMerkleRoot = HashMerkleBranches(&chainMerkleRoot, &branchHash)
			}
		}
	}

	// 4. Verify coinbase contains commitment to auxiliary block
	// The commitment format per Namecoin: the chain merkle root must appear in the
	// coinbase scriptSig after the expected position
	if err := checkCoinbaseCommitment(ap.ParentCoinbaseTx, chainMerkleRoot, chainID); err != nil {
		return fmt.Errorf("coinbase commitment check failed: %w", err)
	}

	// All validations passed
	return nil
}

// checkProofOfWork validates that a block hash meets the difficulty target.
// This is equivalent to btcd's blockchain.CheckProofOfWork but works with raw hash.
func checkProofOfWork(blockHash *chainhash.Hash, bits uint32, powLimit *big.Int) error {
	// Decode the compact difficulty bits to a target big.Int
	target := compactToBig(bits)
	if target.Sign() <= 0 || target.Cmp(powLimit) > 0 {
		return fmt.Errorf("difficulty target 0x%064x exceeds pow limit", target)
	}

	// Convert block hash to big.Int for comparison
	// Block hash bytes are in little-endian, need to reverse for big.Int
	hashNum := hashToBig(blockHash)

	// Block hash must be <= target
	if hashNum.Cmp(target) > 0 {
		return fmt.Errorf("block hash 0x%064x exceeds target 0x%064x", hashNum, target)
	}

	return nil
}

// compactToBig converts compact difficulty representation to big.Int.
// This matches Bitcoin's CompactToBig function (nBits encoding).
//
// Format: 0xAABBCCDD where:
//   - AA is the exponent (size in bytes)
//   - BBCCDD is the mantissa
//   - Result = mantissa * 256^(exponent-3)
func compactToBig(compact uint32) *big.Int {
	// Extract exponent and mantissa
	size := compact >> 24
	mantissa := compact & 0x00ffffff

	var result *big.Int
	if size <= 3 {
		// If size is 3 or less, mantissa represents the full value
		mantissa >>= 8 * (3 - size)
		result = big.NewInt(int64(mantissa))
	} else {
		// Mantissa * 256^(size-3)
		result = big.NewInt(int64(mantissa))
		result.Lsh(result, 8*uint(size-3))
	}

	return result
}

// hashToBig converts a chainhash.Hash to big.Int.
// Hash bytes are in little-endian, so we need to reverse them.
func hashToBig(hash *chainhash.Hash) *big.Int {
	// Reverse the bytes (little-endian to big-endian)
	var reversed [32]byte
	for i := 0; i < 32; i++ {
		reversed[i] = hash[31-i]
	}
	return new(big.Int).SetBytes(reversed[:])
}

// checkCoinbaseCommitment verifies the coinbase contains the auxiliary block commitment.
//
// Per merged mining specification, the commitment appears in the coinbase scriptSig
// in a specific format to prevent false positives. The format is:
//   - Chain merkle root hash (32 bytes)
//   - Size marker (4 bytes, 0x01000000 in little-endian = 1)
//   - Chain ID (4 bytes, little-endian)
//
// The commitment must appear in the coinbase scriptSig at the correct position.
// For Namecoin, chainID = 1.
//
// Additional rules from Namecoin Core (src/auxpow.cpp):
// 1. The coinbase scriptSig must be at least 2 bytes (for BIP34 height)
// 2. The commitment must not appear too early in the scriptSig (prevents false positives)
// 3. Chain ID must match the expected value
func checkCoinbaseCommitment(coinbase *wire.MsgTx, merkleRoot chainhash.Hash, chainID int32) error {
	// Coinbase transaction must have at least one input (the coinbase input)
	if len(coinbase.TxIn) == 0 {
		return fmt.Errorf("coinbase has no inputs")
	}

	scriptSig := coinbase.TxIn[0].SignatureScript

	// Minimum scriptSig size: 2 bytes for BIP34 height + 32 bytes merkle root + 4 bytes size + 4 bytes chain ID = 42 bytes
	if len(scriptSig) < 42 {
		return fmt.Errorf("coinbase scriptSig too short: %d bytes (minimum 42)", len(scriptSig))
	}

	// Build the expected commitment suffix: merkleRoot (32 bytes) + size (4 bytes, 0x01000000) + chainID (4 bytes)
	var commitment [40]byte
	copy(commitment[0:32], merkleRoot[:])

	// Size marker: 0x01000000 (little-endian uint32 = 1)
	commitment[32] = 0x01
	commitment[33] = 0x00
	commitment[34] = 0x00
	commitment[35] = 0x00

	// Chain ID (little-endian)
	commitment[36] = byte(chainID)
	commitment[37] = byte(chainID >> 8)
	commitment[38] = byte(chainID >> 16)
	commitment[39] = byte(chainID >> 24)

	// Search for the commitment in scriptSig
	// Per Namecoin Core, the commitment must not appear too early (at least 2 bytes for BIP34 height)
	// We search starting from position 2
	found := bytes.Index(scriptSig[2:], commitment[:])
	if found == -1 {
		return fmt.Errorf("auxiliary block commitment not found in coinbase")
	}

	// Commitment found at position found+2 (accounting for the skip of first 2 bytes)
	// Additional validation could check position limits if needed

	return nil
}
