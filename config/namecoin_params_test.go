package config

import (
	"encoding/binary"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

// TestNamecoinMainNetMagic verifies the Namecoin mainnet magic bytes
func TestNamecoinMainNetMagic(t *testing.T) {
	// Namecoin mainnet magic bytes are 0xf9, 0xbe, 0xb4, 0xfe on the wire
	// When read as a little-endian uint32: 0xfeb4bef9
	// btcd's wire.BitcoinNet stores this as the little-endian value
	expectedMagic := wire.BitcoinNet(0xf9beb4fe)
	if MainNetMagic != expectedMagic {
		t.Errorf("Expected mainnet magic 0x%x, got 0x%x", expectedMagic, MainNetMagic)
	}

	// Verify it's used in params
	if NamecoinMainNetParams.Net != expectedMagic {
		t.Errorf("MainNet params has wrong magic: expected 0x%x, got 0x%x",
			expectedMagic, NamecoinMainNetParams.Net)
	}
}

// TestNamecoinMainNetPort verifies the default port
func TestNamecoinMainNetPort(t *testing.T) {
	expectedPort := "8334"
	if MainNetDefaultPort != expectedPort {
		t.Errorf("Expected mainnet port %s, got %s", expectedPort, MainNetDefaultPort)
	}

	if NamecoinMainNetParams.DefaultPort != expectedPort {
		t.Errorf("MainNet params has wrong port: expected %s, got %s",
			expectedPort, NamecoinMainNetParams.DefaultPort)
	}
}

// TestNamecoinAddressPrefix verifies the address prefix for mainnet
func TestNamecoinAddressPrefix(t *testing.T) {
	// Namecoin mainnet P2PKH addresses start with 'N' (version byte 0x34 = 52)
	expectedPubKeyHashAddrID := byte(0x34)
	if NamecoinMainNetParams.PubKeyHashAddrID != expectedPubKeyHashAddrID {
		t.Errorf("Expected PubKeyHashAddrID 0x%02x, got 0x%02x",
			expectedPubKeyHashAddrID, NamecoinMainNetParams.PubKeyHashAddrID)
	}
}

// TestNamecoinProtocolConstants verifies name operation constants
func TestNamecoinProtocolConstants(t *testing.T) {
	// Name expiration: 36000 blocks (~250 days)
	if NameExpirationBlocks != 36000 {
		t.Errorf("Expected NameExpirationBlocks 36000, got %d", NameExpirationBlocks)
	}

	// Minimum blocks before first update: 12
	if MinBlocksBeforeFirstUpdate != 12 {
		t.Errorf("Expected MinBlocksBeforeFirstUpdate 12, got %d", MinBlocksBeforeFirstUpdate)
	}

	// Maximum name length: 255 bytes
	if MaxNameLength != 255 {
		t.Errorf("Expected MaxNameLength 255, got %d", MaxNameLength)
	}

	// Maximum value length: 1023 bytes
	if MaxValueLength != 1023 {
		t.Errorf("Expected MaxValueLength 1023, got %d", MaxValueLength)
	}
}

// TestNamecoinGenesisBlock verifies the genesis block parameters
func TestNamecoinGenesisBlock(t *testing.T) {
	if NamecoinMainNetParams.GenesisHash == nil {
		t.Error("GenesisHash is nil")
		return
	}

	// Verify genesis hash is not zero
	var zeroHash [32]byte
	if *NamecoinMainNetParams.GenesisHash == [32]byte(zeroHash) {
		t.Error("GenesisHash should not be zero")
	}

	if NamecoinMainNetParams.GenesisBlock == nil {
		t.Error("GenesisBlock is nil")
		return
	}

	// Verify genesis block has version 1
	if NamecoinMainNetParams.GenesisBlock.Header.Version != 1 {
		t.Errorf("Expected genesis block version 1, got %d",
			NamecoinMainNetParams.GenesisBlock.Header.Version)
	}

	// Verify genesis block contains at least one transaction (coinbase)
	if len(NamecoinMainNetParams.GenesisBlock.Transactions) == 0 {
		t.Error("Genesis block should contain at least one transaction (coinbase)")
	}

	// Verify the coinbase transaction has proper structure
	if len(NamecoinMainNetParams.GenesisBlock.Transactions) > 0 {
		coinbase := NamecoinMainNetParams.GenesisBlock.Transactions[0]
		if len(coinbase.TxIn) == 0 {
			t.Error("Coinbase transaction should have at least one input")
		}
		if len(coinbase.TxOut) == 0 {
			t.Error("Coinbase transaction should have at least one output")
		}
	}
}

// TestNamecoinTestNetParams verifies testnet parameters
func TestNamecoinTestNetParams(t *testing.T) {
	if NamecoinTestNetParams.DefaultPort != "18334" {
		t.Errorf("Expected testnet port 18334, got %s", NamecoinTestNetParams.DefaultPort)
	}

	// Testnet uses different magic bytes
	if NamecoinTestNetParams.Net == NamecoinMainNetParams.Net {
		t.Error("Testnet and mainnet should have different magic bytes")
	}

	// Testnet uses m/n address prefix (0x6f)
	if NamecoinTestNetParams.PubKeyHashAddrID != 0x6f {
		t.Errorf("Expected testnet PubKeyHashAddrID 0x6f, got 0x%02x",
			NamecoinTestNetParams.PubKeyHashAddrID)
	}

	// Verify testnet has genesis block
	if NamecoinTestNetParams.GenesisBlock == nil {
		t.Error("Testnet GenesisBlock is nil")
	}
	if NamecoinTestNetParams.GenesisHash == nil {
		t.Error("Testnet GenesisHash is nil")
	}
	if NamecoinTestNetParams.GenesisBlock != nil && len(NamecoinTestNetParams.GenesisBlock.Transactions) == 0 {
		t.Error("Testnet genesis block should contain at least one transaction")
	}
}

// TestNamecoinRegTestParams verifies regtest parameters
func TestNamecoinRegTestParams(t *testing.T) {
	if NamecoinRegTestParams.DefaultPort != "18445" {
		t.Errorf("Expected regtest port 18445, got %s", NamecoinRegTestParams.DefaultPort)
	}

	// Regtest uses different magic bytes from mainnet and testnet
	if NamecoinRegTestParams.Net == NamecoinMainNetParams.Net {
		t.Error("Regtest and mainnet should have different magic bytes")
	}
	if NamecoinRegTestParams.Net == NamecoinTestNetParams.Net {
		t.Error("Regtest and testnet should have different magic bytes")
	}

	// Regtest should allow generation
	if !NamecoinRegTestParams.GenerateSupported {
		t.Error("Regtest should support block generation")
	}

	// Verify regtest has genesis block
	if NamecoinRegTestParams.GenesisBlock == nil {
		t.Error("Regtest GenesisBlock is nil")
	}
	if NamecoinRegTestParams.GenesisHash == nil {
		t.Error("Regtest GenesisHash is nil")
	}
	if NamecoinRegTestParams.GenesisBlock != nil && len(NamecoinRegTestParams.GenesisBlock.Transactions) == 0 {
		t.Error("Regtest genesis block should contain at least one transaction")
	}
}

// TestNamecoinCoinType verifies BIP44 coin type
func TestNamecoinCoinType(t *testing.T) {
	// Namecoin mainnet uses coin type 7
	if NamecoinMainNetParams.HDCoinType != 7 {
		t.Errorf("Expected mainnet HDCoinType 7, got %d", NamecoinMainNetParams.HDCoinType)
	}

	// Testnet uses coin type 1 (standard for all testnets)
	if NamecoinTestNetParams.HDCoinType != 1 {
		t.Errorf("Expected testnet HDCoinType 1, got %d", NamecoinTestNetParams.HDCoinType)
	}
}

// TestNamecoinDifficultyParams verifies consensus parameters
func TestNamecoinDifficultyParams(t *testing.T) {
	// Mainnet should not reduce min difficulty
	if NamecoinMainNetParams.ReduceMinDifficulty {
		t.Error("Mainnet should not reduce minimum difficulty")
	}

	// Testnet should allow min difficulty reduction
	if !NamecoinTestNetParams.ReduceMinDifficulty {
		t.Error("Testnet should allow minimum difficulty reduction")
	}

	// Coinbase maturity should be 100 blocks
	if NamecoinMainNetParams.CoinbaseMaturity != 100 {
		t.Errorf("Expected CoinbaseMaturity 100, got %d", NamecoinMainNetParams.CoinbaseMaturity)
	}

	// Subsidy halving every 210000 blocks (same as Bitcoin)
	if NamecoinMainNetParams.SubsidyReductionInterval != 210000 {
		t.Errorf("Expected SubsidyReductionInterval 210000, got %d",
			NamecoinMainNetParams.SubsidyReductionInterval)
	}
}

// TestNamecoinBech32HRP verifies Bech32 human-readable parts
func TestNamecoinBech32HRP(t *testing.T) {
	if NamecoinMainNetParams.Bech32HRPSegwit != "nc" {
		t.Errorf("Expected mainnet Bech32HRPSegwit 'nc', got '%s'",
			NamecoinMainNetParams.Bech32HRPSegwit)
	}

	if NamecoinTestNetParams.Bech32HRPSegwit != "tn" {
		t.Errorf("Expected testnet Bech32HRPSegwit 'tn', got '%s'",
			NamecoinTestNetParams.Bech32HRPSegwit)
	}

	if NamecoinRegTestParams.Bech32HRPSegwit != "ncrt" {
		t.Errorf("Expected regtest Bech32HRPSegwit 'ncrt', got '%s'",
			NamecoinRegTestParams.Bech32HRPSegwit)
	}
}

// TestNetworkMagicBytesMatchNamecoinCore verifies that network magic bytes match Namecoin Core's pchMessageStart values.
// Source: https://github.com/namecoin/namecoin-core/blob/master/src/kernel/chainparams.cpp
//
// Namecoin Core uses pchMessageStart as a 4-byte array in wire order:
//   Mainnet:  {0xf9, 0xbe, 0xb4, 0xfe}
//   Testnet:  {0xfa, 0xbf, 0xb5, 0xfe}
//   Regtest:  {0xfa, 0xbf, 0xb5, 0xda}
//
// These bytes are sent in network byte order (big-endian) during P2P protocol handshakes.
// CRITICAL: Wrong magic bytes prevent network communication entirely.
func TestNetworkMagicBytesMatchNamecoinCore(t *testing.T) {
	tests := []struct {
		name              string
		magic             wire.BitcoinNet
		expectedWireBytes [4]byte // Network byte order (big-endian) - must match Namecoin Core
		network           string
	}{
		{
			name:              "Mainnet magic bytes",
			magic:             MainNetMagic,
			expectedWireBytes: [4]byte{0xf9, 0xbe, 0xb4, 0xfe},
			network:           "mainnet",
		},
		{
			name:              "Testnet magic bytes",
			magic:             TestNetMagic,
			expectedWireBytes: [4]byte{0xfa, 0xbf, 0xb5, 0xfe},
			network:           "testnet",
		},
		{
			name:              "Regtest magic bytes",
			magic:             RegTestMagic,
			expectedWireBytes: [4]byte{0xfa, 0xbf, 0xb5, 0xda},
			network:           "regtest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert wire.BitcoinNet (uint32) to byte array in big-endian order
			// This is how the magic bytes appear on the wire during protocol handshake
			var actualBytes [4]byte
			binary.BigEndian.PutUint32(actualBytes[:], uint32(tt.magic))

			// Compare each byte - must match Namecoin Core exactly
			for i := 0; i < 4; i++ {
				if actualBytes[i] != tt.expectedWireBytes[i] {
					t.Errorf("%s byte[%d] = 0x%02x, want 0x%02x (full: %#v, want: %#v) - CRITICAL: Network communication will fail!",
						tt.network, i, actualBytes[i], tt.expectedWireBytes[i],
						actualBytes, tt.expectedWireBytes)
				}
			}

			// Also verify the uint32 representation for documentation
			expectedUint32 := binary.BigEndian.Uint32(tt.expectedWireBytes[:])
			if uint32(tt.magic) != expectedUint32 {
				t.Errorf("%s magic = 0x%08x, want 0x%08x - CRITICAL: Network communication will fail!",
					tt.network, uint32(tt.magic), expectedUint32)
			}

			t.Logf("%s: magic=0x%08x, wire bytes={0x%02x, 0x%02x, 0x%02x, 0x%02x} ✓",
				tt.network, uint32(tt.magic),
				actualBytes[0], actualBytes[1], actualBytes[2], actualBytes[3])
		})
	}
}

// TestNetworkMagicUniqueness ensures all network magic values are unique
// to prevent cross-network message confusion.
func TestNetworkMagicUniqueness(t *testing.T) {
	magics := map[wire.BitcoinNet]string{
		MainNetMagic: "mainnet",
		TestNetMagic: "testnet",
		RegTestMagic: "regtest",
	}

	// Verify we have exactly 3 unique values
	if len(magics) != 3 {
		t.Errorf("Expected 3 unique network magic values, got %d", len(magics))
		t.Logf("MainNet: 0x%08x", MainNetMagic)
		t.Logf("TestNet: 0x%08x", TestNetMagic)
		t.Logf("RegTest: 0x%08x", RegTestMagic)
	}
}

// TestChainParamsNetworkMagic verifies that chain params use the correct magic bytes.
func TestChainParamsNetworkMagic(t *testing.T) {
	tests := []struct {
		name          string
		params        *chaincfg.Params
		expectedMagic wire.BitcoinNet
	}{
		{
			name:          "Mainnet params",
			params:        &NamecoinMainNetParams,
			expectedMagic: MainNetMagic,
		},
		{
			name:          "Testnet params",
			params:        &NamecoinTestNetParams,
			expectedMagic: TestNetMagic,
		},
		{
			name:          "Regtest params",
			params:        &NamecoinRegTestParams,
			expectedMagic: RegTestMagic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.params.Net != tt.expectedMagic {
				t.Errorf("%s params.Net = 0x%08x, want 0x%08x - CRITICAL: Network communication will fail!",
					tt.name, tt.params.Net, tt.expectedMagic)
			}
		})
	}
}

