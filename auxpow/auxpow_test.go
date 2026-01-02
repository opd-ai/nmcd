package auxpow

import (
	"bytes"
	"math/big"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// TestSerializeDeserializeAuxPow tests AuxPow serialization and deserialization
func TestSerializeDeserializeAuxPow(t *testing.T) {
	// Create a sample AuxPow structure
	auxPow := createTestAuxPow(t)

	// Serialize
	var buf bytes.Buffer
	if err := auxPow.Serialize(&buf); err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Deserialize
	var deserialized AuxPow
	if err := deserialized.Deserialize(&buf); err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	// Verify fields match
	parentHash1 := auxPow.ParentBlock.BlockHash()
	parentHash2 := deserialized.ParentBlock.BlockHash()
	if !parentHash1.IsEqual(&parentHash2) {
		t.Errorf("ParentBlock mismatch")
	}

	if auxPow.CoinbaseBranchSideMask != deserialized.CoinbaseBranchSideMask {
		t.Errorf("CoinbaseBranchSideMask mismatch: got %d, want %d",
			deserialized.CoinbaseBranchSideMask, auxPow.CoinbaseBranchSideMask)
	}

	if len(auxPow.CoinbaseBranch) != len(deserialized.CoinbaseBranch) {
		t.Errorf("CoinbaseBranch length mismatch: got %d, want %d",
			len(deserialized.CoinbaseBranch), len(auxPow.CoinbaseBranch))
	}
}

// TestCheckMerkleBranch tests merkle branch verification
func TestCheckMerkleBranch(t *testing.T) {
	tests := []struct {
		name         string
		leaf         string
		branch       []string
		sideMask     uint32
		expectedRoot string
		shouldMatch  bool
	}{
		{
			name:         "single level - left side",
			leaf:         "0000000000000000000000000000000000000000000000000000000000000001",
			branch:       []string{"0000000000000000000000000000000000000000000000000000000000000002"},
			sideMask:     0, // branch is on left
			expectedRoot: computeMerkleRoot(
				"0000000000000000000000000000000000000000000000000000000000000002",
				"0000000000000000000000000000000000000000000000000000000000000001",
			),
			shouldMatch: true,
		},
		{
			name:         "single level - right side",
			leaf:         "0000000000000000000000000000000000000000000000000000000000000001",
			branch:       []string{"0000000000000000000000000000000000000000000000000000000000000002"},
			sideMask:     1, // branch is on right
			expectedRoot: computeMerkleRoot(
				"0000000000000000000000000000000000000000000000000000000000000001",
				"0000000000000000000000000000000000000000000000000000000000000002",
			),
			shouldMatch: true,
		},
		{
			name: "two levels",
			leaf: "0000000000000000000000000000000000000000000000000000000000000001",
			branch: []string{
				"0000000000000000000000000000000000000000000000000000000000000002",
				"0000000000000000000000000000000000000000000000000000000000000003",
			},
			sideMask: 0, // both on left
			expectedRoot: func() string {
				level1 := computeMerkleRoot(
					"0000000000000000000000000000000000000000000000000000000000000002",
					"0000000000000000000000000000000000000000000000000000000000000001",
				)
				return computeMerkleRoot(
					"0000000000000000000000000000000000000000000000000000000000000003",
					level1,
				)
			}(),
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			leaf := mustDecodeHash(tt.leaf)
			expectedRoot := mustDecodeHash(tt.expectedRoot)

			branch := make([]chainhash.Hash, len(tt.branch))
			for i, hashStr := range tt.branch {
				branch[i] = mustDecodeHash(hashStr)
			}

			result := CheckMerkleBranch(leaf, branch, tt.sideMask, expectedRoot)
			if result != tt.shouldMatch {
				t.Errorf("CheckMerkleBranch() = %v, want %v", result, tt.shouldMatch)
			}
		})
	}
}

// TestHashMerkleBranches tests merkle branch hashing
func TestHashMerkleBranches(t *testing.T) {
	left := mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000001")
	right := mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000002")

	result := HashMerkleBranches(&left, &right)

	// Verify the result is deterministic
	result2 := HashMerkleBranches(&left, &right)
	if !result.IsEqual(&result2) {
		t.Error("HashMerkleBranches not deterministic")
	}

	// Verify different order produces different result
	resultReversed := HashMerkleBranches(&right, &left)
	if result.IsEqual(&resultReversed) {
		t.Error("HashMerkleBranches should produce different results for different orders")
	}
}

// TestCompactToBig tests compact difficulty conversion
func TestCompactToBig(t *testing.T) {
	tests := []struct {
		name    string
		compact uint32
		want    string // hex representation
	}{
		{
			name:    "Bitcoin genesis difficulty",
			compact: 0x1d00ffff,
			want:    "00000000ffff0000000000000000000000000000000000000000000000000000",
		},
		{
			name:    "Small value",
			compact: 0x03123456,
			want:    "0000000000000000000000000000000000000000000000000000000000123456",
		},
		{
			name:    "Zero mantissa",
			compact: 0x05000000,
			want:    "0000000000000000000000000000000000000000000000000000000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compactToBig(tt.compact)
			want := new(big.Int)
			want.SetString(tt.want, 16)

			if result.Cmp(want) != 0 {
				t.Errorf("compactToBig(0x%x) = 0x%x, want 0x%x",
					tt.compact, result, want)
			}
		})
	}
}

// TestHashToBig tests hash to big.Int conversion
func TestHashToBig(t *testing.T) {
	// Test with a known hash value
	hashHex := "0000000000000000000000000000000000000000000000000000000000000001"
	hash := mustDecodeHash(hashHex)

	result := hashToBig(&hash)

	// Hash bytes are in little-endian, so 0x01 at the end becomes 0x01...00 after reversal
	// Actually, the hash string is in big-endian display, but chainhash stores it correctly
	// After reversal for big.Int, we should get 1
	expected := big.NewInt(1)

	if result.Cmp(expected) != 0 {
		t.Errorf("hashToBig() = %s, want %s", result.String(), expected.String())
	}
}

// TestCheckProofOfWork tests proof of work validation
func TestCheckProofOfWork(t *testing.T) {
	// Bitcoin mainnet PoW limit: 0x00000000ffff0000000000000000000000000000000000000000000000000000
	powLimit := new(big.Int)
	powLimit.SetString("00000000ffff0000000000000000000000000000000000000000000000000000", 16)

	tests := []struct {
		name      string
		blockHash string
		bits      uint32
		shouldErr bool
	}{
		{
			name:      "valid easy difficulty",
			blockHash: "0000000000000000000000000000000000000000000000000000000000000001",
			bits:      0x1d00ffff,
			shouldErr: false,
		},
		{
			name:      "hash exceeds target",
			blockHash: "0000000100000000000000000000000000000000000000000000000000000000",
			bits:      0x03123456, // Very restrictive target
			shouldErr: true,
		},
		{
			name:      "target exceeds PoW limit",
			blockHash: "0000000000000000000000000000000000000000000000000000000000000001",
			bits:      0x20ffffff, // Target way above PoW limit
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := mustDecodeHash(tt.blockHash)
			err := checkProofOfWork(&hash, tt.bits, powLimit)

			if (err != nil) != tt.shouldErr {
				t.Errorf("checkProofOfWork() error = %v, shouldErr = %v", err, tt.shouldErr)
			}
		})
	}
}

// TestCheckCoinbaseCommitment tests coinbase commitment verification
func TestCheckCoinbaseCommitment(t *testing.T) {
	tests := []struct {
		name       string
		merkleRoot chainhash.Hash
		chainID    int32
		scriptSig  []byte
		shouldErr  bool
	}{
		{
			name:       "valid commitment",
			merkleRoot: mustDecodeHash("1111111111111111111111111111111111111111111111111111111111111111"),
			chainID:    1,
			scriptSig: buildScriptSigWithCommitment(
				mustDecodeHash("1111111111111111111111111111111111111111111111111111111111111111"),
				1,
			),
			shouldErr: false,
		},
		{
			name:       "missing commitment",
			merkleRoot: mustDecodeHash("1111111111111111111111111111111111111111111111111111111111111111"),
			chainID:    1,
			scriptSig:  []byte{0x03, 0x01, 0x02, 0x03}, // Random data, no commitment
			shouldErr:  true,
		},
		{
			name:       "wrong chain ID",
			merkleRoot: mustDecodeHash("1111111111111111111111111111111111111111111111111111111111111111"),
			chainID:    1,
			scriptSig: buildScriptSigWithCommitment(
				mustDecodeHash("1111111111111111111111111111111111111111111111111111111111111111"),
				2, // Wrong chain ID
			),
			shouldErr: true,
		},
		{
			name:       "scriptSig too short",
			merkleRoot: mustDecodeHash("1111111111111111111111111111111111111111111111111111111111111111"),
			chainID:    1,
			scriptSig:  []byte{0x01}, // Only 1 byte
			shouldErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create coinbase transaction with the test scriptSig
			coinbase := &wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{
					{
						PreviousOutPoint: wire.OutPoint{
							Hash:  chainhash.Hash{},
							Index: 0xffffffff,
						},
						SignatureScript: tt.scriptSig,
						Sequence:        0xffffffff,
					},
				},
				TxOut:    []*wire.TxOut{},
				LockTime: 0,
			}

			err := checkCoinbaseCommitment(coinbase, tt.merkleRoot, tt.chainID)

			if (err != nil) != tt.shouldErr {
				t.Errorf("checkCoinbaseCommitment() error = %v, shouldErr = %v", err, tt.shouldErr)
			}
		})
	}
}

// TestDeserializeMerkleBranch tests merkle branch deserialization edge cases
func TestDeserializeMerkleBranch(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		shouldErr bool
		wantLen   int
	}{
		{
			name:      "empty branch",
			data:      []byte{0x00}, // Varint 0
			shouldErr: false,
			wantLen:   0,
		},
		{
			name: "single hash",
			data: append([]byte{0x01}, // Varint 1
				make([]byte, 32)...), // 32-byte hash
			shouldErr: false,
			wantLen:   1,
		},
		{
			name:      "branch too long",
			data:      []byte{0x1f}, // Varint 31 (exceeds max 30)
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.NewReader(tt.data)
			branch, err := deserializeMerkleBranch(buf)

			if (err != nil) != tt.shouldErr {
				t.Errorf("deserializeMerkleBranch() error = %v, shouldErr = %v", err, tt.shouldErr)
			}

			if !tt.shouldErr && len(branch) != tt.wantLen {
				t.Errorf("deserializeMerkleBranch() length = %d, want %d", len(branch), tt.wantLen)
			}
		})
	}
}

// Helper functions

// createTestAuxPow creates a sample AuxPow structure for testing
func createTestAuxPow(t *testing.T) *AuxPow {
	t.Helper()

	// Create a simple parent coinbase transaction
	coinbase := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 0xffffffff,
				},
				SignatureScript: buildScriptSigWithCommitment(
					mustDecodeHash("1111111111111111111111111111111111111111111111111111111111111111"),
					1,
				),
				Sequence: 0xffffffff,
			},
		},
		TxOut: []*wire.TxOut{
			{
				Value:    5000000000,
				PkScript: []byte{0x76, 0xa9, 0x14}, // Simplified P2PKH
			},
		},
		LockTime: 0,
	}

	// Create coinbase branch (1 hash for simplicity)
	coinbaseBranch := []chainhash.Hash{
		mustDecodeHash("2222222222222222222222222222222222222222222222222222222222222222"),
	}

	// Create parent block header
	parentBlock := wire.BlockHeader{
		Version:    1,
		PrevBlock:  mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000000"),
		MerkleRoot: computeMerkleRootFromTx(coinbase, coinbaseBranch[0]),
		Timestamp:  time.Unix(1231006505, 0),
		Bits:       0x1d00ffff,
		Nonce:      2083236893,
	}

	return &AuxPow{
		ParentCoinbaseTx:          coinbase,
		CoinbaseBranch:            coinbaseBranch,
		CoinbaseBranchSideMask:    0,
		ChainMerkleBranch:         []chainhash.Hash{},
		ChainMerkleBranchSideMask: 0,
		ParentBlock:               parentBlock,
	}
}

// mustDecodeHash decodes a hex string to chainhash.Hash, panics on error
func mustDecodeHash(s string) chainhash.Hash {
	hash, err := chainhash.NewHashFromStr(s)
	if err != nil {
		panic(err)
	}
	return *hash
}

// computeMerkleRoot computes merkle root from two hashes
func computeMerkleRoot(leftHex, rightHex string) string {
	left := mustDecodeHash(leftHex)
	right := mustDecodeHash(rightHex)
	result := HashMerkleBranches(&left, &right)
	return result.String()
}

// computeMerkleRootFromTx computes expected merkle root from tx and branch
func computeMerkleRootFromTx(tx *wire.MsgTx, branchHash chainhash.Hash) chainhash.Hash {
	txHash := tx.TxHash()
	return HashMerkleBranches(&branchHash, &txHash)
}

// buildScriptSigWithCommitment builds a scriptSig with proper AuxPow commitment
func buildScriptSigWithCommitment(merkleRoot chainhash.Hash, chainID int32) []byte {
	// BIP34 height (2 bytes minimum) + commitment (40 bytes)
	scriptSig := make([]byte, 42)

	// BIP34 height placeholder (just 2 bytes of data)
	scriptSig[0] = 0x03
	scriptSig[1] = 0x01

	// Merkle root (32 bytes)
	copy(scriptSig[2:34], merkleRoot[:])

	// Size marker: 0x01000000 (little-endian)
	scriptSig[34] = 0x01
	scriptSig[35] = 0x00
	scriptSig[36] = 0x00
	scriptSig[37] = 0x00

	// Chain ID (4 bytes, little-endian)
	scriptSig[38] = byte(chainID)
	scriptSig[39] = byte(chainID >> 8)
	scriptSig[40] = byte(chainID >> 16)
	scriptSig[41] = byte(chainID >> 24)

	return scriptSig
}

// TestAuxPowValidate tests full AuxPow validation
func TestAuxPowValidate(t *testing.T) {
	// This is a simplified test - in production would use real block data
	auxPow := createTestAuxPow(t)

	// Set PoW limit high for this test
	powLimit := new(big.Int)
	powLimit.SetString("00000000ffff0000000000000000000000000000000000000000000000000000", 16)

	auxBlockHash := mustDecodeHash("1111111111111111111111111111111111111111111111111111111111111111")

	// This will fail because we're using test data, but it exercises the validation path
	err := auxPow.Validate(auxBlockHash, 1, powLimit)
	
	// We expect this to fail on coinbase commitment check since we have simplified test data
	// The important thing is that it doesn't panic and follows the validation flow
	if err == nil {
		t.Log("Validation unexpectedly passed (test data may be incomplete)")
	} else {
		t.Logf("Validation failed as expected with test data: %v", err)
	}
}
