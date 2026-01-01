package chain

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/config"
)

// TestValidateProofOfWork tests that proof of work validation correctly accepts
// and rejects blocks based on their difficulty target and hash.
//
// This test verifies that our validateProofOfWork function correctly uses
// btcd's CheckProofOfWork with Namecoin's PoW parameters.
func TestValidateProofOfWork(t *testing.T) {
	t.Run("block with difficulty target exceeding PoW limit is rejected", func(t *testing.T) {
		bc := &BlockChain{
			chainParams: &config.NamecoinRegTestParams,
		}

		// Create a block with difficulty bits that exceed the PoW limit.
		// Regtest's PoW limit is 0x207fffff (2^255 - 1, very easy).
		// To exceed this limit (be even easier/invalid), we use 0x21000000
		// which would decode to a target larger than the PoW limit.
		invalidBlock := btcutil.NewBlock(&wire.MsgBlock{
			Header: wire.BlockHeader{
				Version:    1,
				PrevBlock:  chainhash.Hash{},
				MerkleRoot: chainhash.Hash{},
				Timestamp:  time.Now(),
				Bits:       0x21000000, // Invalid: decodes to target > PowLimit
				Nonce:      0,
			},
			Transactions: []*wire.MsgTx{&wire.MsgTx{}},
		})
		invalidBlock.SetHeight(100)

		err := bc.validateProofOfWork(invalidBlock)
		if err == nil {
			t.Error("Expected error for block with difficulty exceeding PoW limit, got nil")
		}
	})

	t.Run("block hash not meeting difficulty target is rejected", func(t *testing.T) {
		bc := &BlockChain{
			chainParams: &config.NamecoinMainNetParams,
		}

		// Create a block where the hash is unlikely to meet a difficult target
		difficultBlock := btcutil.NewBlock(&wire.MsgBlock{
			Header: wire.BlockHeader{
				Version:    1,
				PrevBlock:  chainhash.Hash{},
				MerkleRoot: chainhash.Hash{},
				Timestamp:  time.Now(),
				Bits:       0x1d00ffff, // Difficult target
				Nonce:      0,          // Wrong nonce
			},
			Transactions: []*wire.MsgTx{&wire.MsgTx{}},
		})
		difficultBlock.SetHeight(100)

		err := bc.validateProofOfWork(difficultBlock)
		if err == nil {
			t.Error("Expected error for block hash not meeting difficulty target, got nil")
		}
	})

	t.Run("validateProofOfWork uses correct PoW limit from chain params", func(t *testing.T) {
		// Verify that different networks use their correct PoW limits
		mainnetBC := &BlockChain{chainParams: &config.NamecoinMainNetParams}
		testnetBC := &BlockChain{chainParams: &config.NamecoinTestNetParams}
		regtestBC := &BlockChain{chainParams: &config.NamecoinRegTestParams}

		// All should have non-nil PoW limits
		if mainnetBC.chainParams.PowLimit == nil {
			t.Error("Mainnet PoW limit is nil")
		}
		if testnetBC.chainParams.PowLimit == nil {
			t.Error("Testnet PoW limit is nil")
		}
		if regtestBC.chainParams.PowLimit == nil {
			t.Error("Regtest PoW limit is nil")
		}

		// Regtest should have higher (easier) limit than mainnet/testnet
		if regtestBC.chainParams.PowLimit.Cmp(mainnetBC.chainParams.PowLimit) <= 0 {
			t.Error("Expected regtest PoW limit to be higher (easier) than mainnet")
		}

		// Mainnet and testnet should have same limit
		if mainnetBC.chainParams.PowLimit.Cmp(testnetBC.chainParams.PowLimit) != 0 {
			t.Error("Expected mainnet and testnet to have same PoW limit")
		}
	})
}

// TestDifficultyRetargeting verifies that Namecoin difficulty parameters
// are correctly configured for btcd's retargeting algorithm.
func TestDifficultyRetargeting(t *testing.T) {
	t.Run("Namecoin uses same retarget interval as Bitcoin", func(t *testing.T) {
		// Mainnet and testnet both use 2016 block intervals
		if config.NamecoinMainNetParams.MinerConfirmationWindow != 2016 {
			t.Errorf("Expected mainnet retarget interval of 2016, got %d",
				config.NamecoinMainNetParams.MinerConfirmationWindow)
		}

		if config.NamecoinTestNetParams.MinerConfirmationWindow != 2016 {
			t.Errorf("Expected testnet retarget interval of 2016, got %d",
				config.NamecoinTestNetParams.MinerConfirmationWindow)
		}
	})

	t.Run("Regtest has faster retargeting for testing", func(t *testing.T) {
		// Regtest uses 144 blocks for faster testing
		if config.NamecoinRegTestParams.MinerConfirmationWindow != 144 {
			t.Errorf("Expected regtest retarget interval of 144, got %d",
				config.NamecoinRegTestParams.MinerConfirmationWindow)
		}
	})

	t.Run("Namecoin PoW limits are correctly configured", func(t *testing.T) {
		// Mainnet/testnet: 2^224 - 1 (same as Bitcoin)
		// Format: 0x00000000FFFF0000000000000000000000000000000000000000000000000000
		mainnetLimit := config.NamecoinMainNetParams.PowLimit.Text(16)
		testnetLimit := config.NamecoinTestNetParams.PowLimit.Text(16)

		// Both should be the same
		if mainnetLimit != testnetLimit {
			t.Errorf("Mainnet and testnet PoW limits should match:\nMainnet: %s\nTestnet: %s",
				mainnetLimit, testnetLimit)
		}

		// Regtest: 2^255 - 1 (much easier)
		regtestLimit := config.NamecoinRegTestParams.PowLimit.Text(16)

		// Regtest should be much larger (easier)
		if len(regtestLimit) <= len(mainnetLimit) {
			t.Errorf("Regtest PoW limit should be larger than mainnet:\nRegtest: %s\nMainnet: %s",
				regtestLimit, mainnetLimit)
		}
	})
}
