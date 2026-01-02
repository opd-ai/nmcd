package chain

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// TestMerkleBranchSerializeDeserialize tests serialization and deserialization
// of merkle branch structures.
func TestMerkleBranchSerializeDeserialize(t *testing.T) {
	tests := []struct {
		name    string
		branch  MerkleBranch
		wantErr bool
	}{
		{
			name: "empty branch",
			branch: MerkleBranch{
				Branch:   []chainhash.Hash{},
				SideMask: 0,
			},
			wantErr: false,
		},
		{
			name: "single hash branch",
			branch: MerkleBranch{
				Branch: []chainhash.Hash{
					mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000001"),
				},
				SideMask: 0,
			},
			wantErr: false,
		},
		{
			name: "single hash branch with right side",
			branch: MerkleBranch{
				Branch: []chainhash.Hash{
					mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000001"),
				},
				SideMask: 1, // sibling on left, we are on right
			},
			wantErr: false,
		},
		{
			name: "three level branch",
			branch: MerkleBranch{
				Branch: []chainhash.Hash{
					mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000001"),
					mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000002"),
					mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000003"),
				},
				SideMask: 0b101, // alternating left/right positions
			},
			wantErr: false,
		},
		{
			name: "max depth branch (32 levels)",
			branch: MerkleBranch{
				Branch:   make([]chainhash.Hash, 32),
				SideMask: 0xAAAAAAAA, // alternating pattern
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Serialize
			var buf bytes.Buffer
			err := tt.branch.SerializeMerkleBranch(&buf)
			if tt.wantErr {
				if err == nil {
					t.Errorf("SerializeMerkleBranch() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("SerializeMerkleBranch() unexpected error: %v", err)
			}

			// Deserialize
			deserialized, err := DeserializeMerkleBranch(&buf)
			if err != nil {
				t.Fatalf("DeserializeMerkleBranch() unexpected error: %v", err)
			}

			// Verify
			if len(deserialized.Branch) != len(tt.branch.Branch) {
				t.Errorf("Branch length mismatch: got %d, want %d",
					len(deserialized.Branch), len(tt.branch.Branch))
			}

			for i := range tt.branch.Branch {
				if !deserialized.Branch[i].IsEqual(&tt.branch.Branch[i]) {
					t.Errorf("Branch hash %d mismatch: got %s, want %s",
						i, deserialized.Branch[i], tt.branch.Branch[i])
				}
			}

			if deserialized.SideMask != tt.branch.SideMask {
				t.Errorf("SideMask mismatch: got 0x%x, want 0x%x",
					deserialized.SideMask, tt.branch.SideMask)
			}
		})
	}
}

// TestMerkleBranchDeserializeTooDeep tests that excessively deep merkle branches are rejected.
func TestMerkleBranchDeserializeTooDeep(t *testing.T) {
	// Create a merkle branch with 33 levels (exceeds maximum of 32)
	var buf bytes.Buffer

	// Write branch size (33 levels)
	if err := wire.WriteVarInt(&buf, 0, 33); err != nil {
		t.Fatalf("Failed to write branch size: %v", err)
	}

	// Attempt to deserialize (should fail)
	_, err := DeserializeMerkleBranch(&buf)
	if err == nil {
		t.Error("DeserializeMerkleBranch() expected error for 33 levels, got nil")
	}
}

// TestMerkleBranchSerializeTooDeep tests that serialization rejects branches with >32 levels.
func TestMerkleBranchSerializeTooDeep(t *testing.T) {
	// Create a merkle branch with 33 levels (exceeds maximum of 32)
	branch := MerkleBranch{
		Branch:   make([]chainhash.Hash, 33),
		SideMask: 0,
	}

	var buf bytes.Buffer
	err := branch.SerializeMerkleBranch(&buf)
	if err == nil {
		t.Error("SerializeMerkleBranch() expected error for 33 levels, got nil")
	}
	if !strings.Contains(err.Error(), "merkle branch too deep") {
		t.Errorf("SerializeMerkleBranch() error = %v, want error containing 'merkle branch too deep'", err)
	}
}

// TestAuxPowSerializeDeserialize tests serialization and deserialization
// of complete AuxPow structures.
func TestAuxPowSerializeDeserialize(t *testing.T) {
	// Create a minimal valid coinbase transaction
	coinbaseTx := wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{}, // Null hash for coinbase
					Index: 0xffffffff,       // Max index for coinbase
				},
				SignatureScript: []byte{0x03, 0x00, 0x4b, 0x00}, // Block height 19200 in coinbase
				Sequence:        0xffffffff,
			},
		},
		TxOut: []*wire.TxOut{
			{
				Value:    5000000000,               // 50 NMC
				PkScript: []byte{0x76, 0xa9, 0x14}, // P2PKH prefix
			},
		},
		LockTime: 0,
	}

	blockHash := mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000abc")

	coinbaseBranch := MerkleBranch{
		Branch: []chainhash.Hash{
			mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000001"),
			mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000002"),
		},
		SideMask: 0b10, // bit 0 = 0 (left at level 0), bit 1 = 1 (right at level 1)
	}

	chainBranch := MerkleBranch{
		Branch: []chainhash.Hash{
			mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000003"),
		},
		SideMask: 0,
	}

	parentHeader := wire.BlockHeader{
		Version:    1,
		PrevBlock:  mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000def"),
		MerkleRoot: mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000456"),
		Timestamp:  time.Unix(1234567890, 0),
		Bits:       0x1d00ffff,
		Nonce:      987654321,
	}

	auxpow := &AuxPow{
		CoinbaseTx:        coinbaseTx,
		BlockHash:         blockHash,
		CoinbaseBranch:    coinbaseBranch,
		ChainMerkleBranch: chainBranch,
		ParentBlock:       parentHeader,
	}

	// Serialize
	var buf bytes.Buffer
	err := auxpow.SerializeAuxPow(&buf)
	if err != nil {
		t.Fatalf("SerializeAuxPow() unexpected error: %v", err)
	}

	// Deserialize
	deserialized, err := DeserializeAuxPow(&buf)
	if err != nil {
		t.Fatalf("DeserializeAuxPow() unexpected error: %v", err)
	}

	// Verify coinbase transaction
	if deserialized.CoinbaseTx.TxHash() != auxpow.CoinbaseTx.TxHash() {
		t.Errorf("Coinbase tx hash mismatch: got %s, want %s",
			deserialized.CoinbaseTx.TxHash(), auxpow.CoinbaseTx.TxHash())
	}

	// Verify block hash
	if !deserialized.BlockHash.IsEqual(&auxpow.BlockHash) {
		t.Errorf("Block hash mismatch: got %s, want %s",
			deserialized.BlockHash, auxpow.BlockHash)
	}

	// Verify coinbase branch
	if len(deserialized.CoinbaseBranch.Branch) != len(auxpow.CoinbaseBranch.Branch) {
		t.Errorf("Coinbase branch length mismatch: got %d, want %d",
			len(deserialized.CoinbaseBranch.Branch), len(auxpow.CoinbaseBranch.Branch))
	}
	if deserialized.CoinbaseBranch.SideMask != auxpow.CoinbaseBranch.SideMask {
		t.Errorf("Coinbase branch side mask mismatch: got 0x%x, want 0x%x",
			deserialized.CoinbaseBranch.SideMask, auxpow.CoinbaseBranch.SideMask)
	}

	// Verify chain branch
	if len(deserialized.ChainMerkleBranch.Branch) != len(auxpow.ChainMerkleBranch.Branch) {
		t.Errorf("Chain branch length mismatch: got %d, want %d",
			len(deserialized.ChainMerkleBranch.Branch), len(auxpow.ChainMerkleBranch.Branch))
	}

	// Verify parent block header
	parentHash1 := deserialized.ParentBlock.BlockHash()
	parentHash2 := auxpow.ParentBlock.BlockHash()
	if !parentHash1.IsEqual(&parentHash2) {
		t.Errorf("Parent block hash mismatch: got %s, want %s",
			parentHash1, parentHash2)
	}
}

// TestAuxPowDeserializeInvalidData tests deserialization error handling.
func TestAuxPowDeserializeInvalidData(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{
			name:    "empty buffer",
			data:    []byte{},
			wantErr: "failed to deserialize coinbase tx",
		},
		{
			name:    "truncated data",
			data:    []byte{0x01, 0x02, 0x03},
			wantErr: "failed to deserialize coinbase tx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.NewBuffer(tt.data)
			_, err := DeserializeAuxPow(buf)
			if err == nil {
				t.Error("DeserializeAuxPow() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("DeserializeAuxPow() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

// TestMerkleBranchRoundTrip tests multiple round trips to ensure stability.
func TestMerkleBranchRoundTrip(t *testing.T) {
	original := MerkleBranch{
		Branch: []chainhash.Hash{
			mustDecodeHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			mustDecodeHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
			mustDecodeHash("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
		},
		SideMask: 0b110, // bit0=0 (level 0: left), bit1=1 (level 1: right), bit2=1 (level 2: right)
	}

	// Perform 3 round trips
	current := original
	for i := 0; i < 3; i++ {
		var buf bytes.Buffer
		if err := current.SerializeMerkleBranch(&buf); err != nil {
			t.Fatalf("Round trip %d: SerializeMerkleBranch() failed: %v", i+1, err)
		}

		deserialized, err := DeserializeMerkleBranch(&buf)
		if err != nil {
			t.Fatalf("Round trip %d: DeserializeMerkleBranch() failed: %v", i+1, err)
		}

		// Verify matches original
		if len(deserialized.Branch) != len(original.Branch) {
			t.Errorf("Round trip %d: Branch length changed", i+1)
		}
		if deserialized.SideMask != original.SideMask {
			t.Errorf("Round trip %d: SideMask changed", i+1)
		}

		current = *deserialized
	}
}

// Helper function to decode hex hash or panic
func mustDecodeHash(hexStr string) chainhash.Hash {
	hash, err := chainhash.NewHashFromStr(hexStr)
	if err != nil {
		panic(err)
	}
	return *hash
}

// TestAuxPowStructSize tests that AuxPow structures can handle realistic sizes.
func TestAuxPowStructSize(t *testing.T) {
	// Create a realistic AuxPow with moderate merkle tree depth
	auxpow := &AuxPow{
		CoinbaseTx: wire.MsgTx{
			Version: 1,
			TxIn: []*wire.TxIn{
				{
					PreviousOutPoint: wire.OutPoint{
						Hash:  chainhash.Hash{},
						Index: 0xffffffff,
					},
					SignatureScript: make([]byte, 100), // Typical coinbase script size
					Sequence:        0xffffffff,
				},
			},
			TxOut: []*wire.TxOut{
				{
					Value:    5000000000,
					PkScript: make([]byte, 25), // P2PKH size
				},
			},
			LockTime: 0,
		},
		BlockHash: mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000abc"),
		CoinbaseBranch: MerkleBranch{
			Branch:   make([]chainhash.Hash, 10), // Realistic tree depth for ~1000 tx block
			SideMask: 0x1ff,
		},
		ChainMerkleBranch: MerkleBranch{
			Branch:   make([]chainhash.Hash, 3), // Small tree for merge-mined chains
			SideMask: 0x5,
		},
		ParentBlock: wire.BlockHeader{
			Version:    1,
			PrevBlock:  mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000def"),
			MerkleRoot: mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000456"),
			Timestamp:  time.Unix(1234567890, 0),
			Bits:       0x1d00ffff,
			Nonce:      987654321,
		},
	}

	// Serialize and measure
	var buf bytes.Buffer
	if err := auxpow.SerializeAuxPow(&buf); err != nil {
		t.Fatalf("SerializeAuxPow() failed: %v", err)
	}

	serializedSize := buf.Len()
	t.Logf("Serialized AuxPow size: %d bytes", serializedSize)

	// Deserialize to verify
	deserialized, err := DeserializeAuxPow(&buf)
	if err != nil {
		t.Fatalf("DeserializeAuxPow() failed: %v", err)
	}

	// Spot check a few fields
	if len(deserialized.CoinbaseBranch.Branch) != 10 {
		t.Errorf("CoinbaseBranch depth mismatch: got %d, want 10",
			len(deserialized.CoinbaseBranch.Branch))
	}

	if len(deserialized.ChainMerkleBranch.Branch) != 3 {
		t.Errorf("ChainMerkleBranch depth mismatch: got %d, want 3",
			len(deserialized.ChainMerkleBranch.Branch))
	}
}

// TestCheckMerkleBranch tests merkle branch verification with known good and bad proofs.
func TestCheckMerkleBranch(t *testing.T) {
tests := []struct {
name     string
leaf     string
branch   MerkleBranch
root     string
expected bool
}{
{
name: "empty branch - leaf equals root",
leaf: "0000000000000000000000000000000000000000000000000000000000000001",
branch: MerkleBranch{
Branch:   []chainhash.Hash{},
SideMask: 0,
},
root:     "0000000000000000000000000000000000000000000000000000000000000001",
expected: true,
},
{
name: "empty branch - leaf not equal to root",
leaf: "0000000000000000000000000000000000000000000000000000000000000001",
branch: MerkleBranch{
Branch:   []chainhash.Hash{},
SideMask: 0,
},
root:     "0000000000000000000000000000000000000000000000000000000000000002",
expected: false,
},
{
name: "single level - sibling on right",
leaf: "0000000000000000000000000000000000000000000000000000000000000001",
branch: MerkleBranch{
Branch: []chainhash.Hash{
mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000002"),
},
SideMask: 0, // sibling on right, leaf on left
},
// root = Hash(leaf || sibling)
root:     computeMerkleRoot("0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000002", false),
expected: true,
},
{
name: "single level - sibling on left",
leaf: "0000000000000000000000000000000000000000000000000000000000000001",
branch: MerkleBranch{
Branch: []chainhash.Hash{
mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000002"),
},
SideMask: 1, // sibling on left, leaf on right
},
// root = Hash(sibling || leaf)
root:     computeMerkleRoot("0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000002", true),
expected: true,
},
{
name: "single level - wrong root",
leaf: "0000000000000000000000000000000000000000000000000000000000000001",
branch: MerkleBranch{
Branch: []chainhash.Hash{
mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000002"),
},
SideMask: 0,
},
root:     "0000000000000000000000000000000000000000000000000000000000000999",
expected: false,
},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
leaf := mustDecodeHash(tt.leaf)
root := mustDecodeHash(tt.root)

result := CheckMerkleBranch(&leaf, &tt.branch, &root)
if result != tt.expected {
t.Errorf("CheckMerkleBranch() = %v, want %v", result, tt.expected)
}
})
}
}

// TestExtractChainID tests chain ID extraction from parent block nonce.
func TestExtractChainID(t *testing.T) {
tests := []struct {
name            string
parentNonce     uint32
expectedChainID uint32
}{
{
name:            "Namecoin chain ID (1)",
parentNonce:     0x00010000, // chain ID in bits 16-23
expectedChainID: 1,
},
{
name:            "chain ID 0",
parentNonce:     0x00000000,
expectedChainID: 0,
},
{
name:            "chain ID 255 (max)",
parentNonce:     0x00FF0000,
expectedChainID: 255,
},
{
name:            "chain ID with other bits set",
parentNonce:     0x12345678, // chain ID = 0x34
expectedChainID: 0x34,
},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
auxpow := &AuxPow{
ParentBlock: wire.BlockHeader{
Nonce: tt.parentNonce,
},
}

chainID, err := auxpow.ExtractChainID()
if err != nil {
t.Fatalf("ExtractChainID() unexpected error: %v", err)
}
if chainID != tt.expectedChainID {
t.Errorf("ExtractChainID() = %d, want %d", chainID, tt.expectedChainID)
}
})
}
}

// TestValidateAuxPow tests full AuxPow validation.
func TestValidateAuxPow(t *testing.T) {
t.Run("chain ID mismatch", func(t *testing.T) {
auxpow := &AuxPow{
ParentBlock: wire.BlockHeader{
Nonce: 0x00020000, // chain ID = 2
},
}

blockHash := mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000abc")
targetDiff := mustDecodeHash("00000000ffff0000000000000000000000000000000000000000000000000000")

err := auxpow.ValidateAuxPow(&blockHash, NamecoinChainID, &targetDiff)
if err == nil {
t.Fatal("ValidateAuxPow() expected error for chain ID mismatch, got nil")
}
if !strings.Contains(err.Error(), "chain ID mismatch") {
t.Errorf("ValidateAuxPow() error = %v, want error containing 'chain ID mismatch'", err)
}
})

t.Run("parent block hash exceeds difficulty", func(t *testing.T) {
auxpow := &AuxPow{
ParentBlock: wire.BlockHeader{
Nonce:   0x00010000, // chain ID = 1
Version: 1,
Bits:    0x1d00ffff,
// BlockHash will be very high (many leading zeros when viewed as big endian)
},
}

blockHash := mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000abc")
// Very restrictive target - parent hash will exceed it
targetDiff := mustDecodeHash("0000000000000001000000000000000000000000000000000000000000000000")

err := auxpow.ValidateAuxPow(&blockHash, NamecoinChainID, &targetDiff)
if err == nil {
t.Fatal("ValidateAuxPow() expected error for difficulty not met, got nil")
}
if !strings.Contains(err.Error(), "does not meet difficulty target") {
t.Errorf("ValidateAuxPow() error = %v, want error containing 'does not meet difficulty target'", err)
}
})

t.Run("coinbase merkle branch verification failed", func(t *testing.T) {
// Create a coinbase transaction
coinbaseTx := wire.MsgTx{
Version: 1,
TxIn: []*wire.TxIn{
{
PreviousOutPoint: wire.OutPoint{
Hash:  chainhash.Hash{},
Index: 0xffffffff,
},
SignatureScript: []byte("test coinbase"),
Sequence:        0xffffffff,
},
},
TxOut: []*wire.TxOut{
{
Value:    5000000000,
PkScript: []byte{0x76, 0xa9, 0x14}, // partial P2PKH
},
},
}

auxpow := &AuxPow{
CoinbaseTx: coinbaseTx,
ParentBlock: wire.BlockHeader{
Nonce:      0x00010000, // chain ID = 1
Version:    1,
Bits:       0x1d00ffff,
MerkleRoot: mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000999"), // Wrong merkle root
},
CoinbaseBranch: MerkleBranch{
Branch:   []chainhash.Hash{},
SideMask: 0,
},
}

blockHash := mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000abc")
targetDiff := mustDecodeHash("00000000ffff0000000000000000000000000000000000000000000000000000")

err := auxpow.ValidateAuxPow(&blockHash, NamecoinChainID, &targetDiff)
if err == nil {
t.Fatal("ValidateAuxPow() expected error for invalid coinbase merkle branch, got nil")
}
if !strings.Contains(err.Error(), "coinbase merkle branch verification failed") {
t.Errorf("ValidateAuxPow() error = %v, want error containing 'coinbase merkle branch verification failed'", err)
}
})
}

// Helper function to compute merkle root for testing
// siblingOnLeft determines if sibling hash goes on left (true) or right (false)
func computeMerkleRoot(hash1Str, hash2Str string, siblingOnLeft bool) string {
hash1 := mustDecodeHash(hash1Str)
hash2 := mustDecodeHash(hash2Str)

var combined [64]byte
if siblingOnLeft {
copy(combined[:32], hash2[:])   // sibling on left
copy(combined[32:], hash1[:])   // leaf on right
} else {
copy(combined[:32], hash1[:])   // leaf on left
copy(combined[32:], hash2[:])   // sibling on right
}

result := chainhash.DoubleHashH(combined[:])
return result.String()
}
