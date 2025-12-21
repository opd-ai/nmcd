package config

import (
	"testing"

	"github.com/btcsuite/btcd/wire"
)

// TestNamecoinMainNetMagic verifies the Namecoin mainnet magic bytes
func TestNamecoinMainNetMagic(t *testing.T) {
	// Namecoin mainnet magic bytes: 0xfeb4bef9
	// In little-endian wire format: 0xf9beb4fe
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
