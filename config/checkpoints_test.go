package config

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
)

// TestMainnetCheckpoints verifies that mainnet has checkpoints configured
func TestMainnetCheckpoints(t *testing.T) {
	checkpoints := NamecoinMainNetParams.Checkpoints

	if checkpoints == nil {
		t.Fatal("mainnet checkpoints should not be nil")
	}

	if len(checkpoints) == 0 {
		t.Fatal("mainnet should have at least genesis checkpoint")
	}

	// Verify genesis checkpoint exists
	if checkpoints[0].Height != 0 {
		t.Errorf("first checkpoint should be genesis (height 0), got height %d", checkpoints[0].Height)
	}

	if checkpoints[0].Hash == nil {
		t.Fatal("genesis checkpoint hash should not be nil")
	}

	// Verify it matches the MainNetGenesisHash
	if !checkpoints[0].Hash.IsEqual(&MainNetGenesisHash) {
		t.Errorf("genesis checkpoint hash mismatch: got %s, want %s",
			checkpoints[0].Hash, MainNetGenesisHash)
	}
}

// TestTestnetCheckpoints verifies that testnet has checkpoints configured
func TestTestnetCheckpoints(t *testing.T) {
	checkpoints := NamecoinTestNetParams.Checkpoints

	if checkpoints == nil {
		t.Fatal("testnet checkpoints should not be nil")
	}

	if len(checkpoints) == 0 {
		t.Fatal("testnet should have at least genesis checkpoint")
	}

	// Verify genesis checkpoint exists
	if checkpoints[0].Height != 0 {
		t.Errorf("first checkpoint should be genesis (height 0), got height %d", checkpoints[0].Height)
	}

	if checkpoints[0].Hash == nil {
		t.Fatal("genesis checkpoint hash should not be nil")
	}

	// Verify it matches the testNetGenesisHash
	if !checkpoints[0].Hash.IsEqual(&testNetGenesisHash) {
		t.Errorf("genesis checkpoint hash mismatch: got %s, want %s",
			checkpoints[0].Hash, testNetGenesisHash)
	}
}

// TestRegtestCheckpoints verifies that regtest has checkpoints configured
func TestRegtestCheckpoints(t *testing.T) {
	checkpoints := NamecoinRegTestParams.Checkpoints

	if checkpoints == nil {
		t.Fatal("regtest checkpoints should not be nil")
	}

	if len(checkpoints) == 0 {
		t.Fatal("regtest should have at least genesis checkpoint")
	}

	// Verify genesis checkpoint exists
	if checkpoints[0].Height != 0 {
		t.Errorf("first checkpoint should be genesis (height 0), got height %d", checkpoints[0].Height)
	}

	if checkpoints[0].Hash == nil {
		t.Fatal("genesis checkpoint hash should not be nil")
	}

	// Verify it matches the regTestGenesisHash
	if !checkpoints[0].Hash.IsEqual(&regTestGenesisHash) {
		t.Errorf("genesis checkpoint hash mismatch: got %s, want %s",
			checkpoints[0].Hash, regTestGenesisHash)
	}
}

// TestCheckpointsSorted verifies that checkpoints are sorted by height
func TestCheckpointsSorted(t *testing.T) {
	testCases := []struct {
		name        string
		checkpoints []chaincfg.Checkpoint
	}{
		{"mainnet", NamecoinMainNetParams.Checkpoints},
		{"testnet", NamecoinTestNetParams.Checkpoints},
		{"regtest", NamecoinRegTestParams.Checkpoints},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.checkpoints) < 2 {
				// Skip sorting test if only genesis checkpoint
				return
			}

			for i := 1; i < len(tc.checkpoints); i++ {
				if tc.checkpoints[i].Height <= tc.checkpoints[i-1].Height {
					t.Errorf("checkpoints not sorted: checkpoint %d (height %d) should be after checkpoint %d (height %d)",
						i, tc.checkpoints[i].Height, i-1, tc.checkpoints[i-1].Height)
				}
			}
		})
	}
}

// TestCheckpointHashesValid verifies that all checkpoint hashes are non-nil
func TestCheckpointHashesValid(t *testing.T) {
	testCases := []struct {
		name        string
		checkpoints []chaincfg.Checkpoint
	}{
		{"mainnet", NamecoinMainNetParams.Checkpoints},
		{"testnet", NamecoinTestNetParams.Checkpoints},
		{"regtest", NamecoinRegTestParams.Checkpoints},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for i, cp := range tc.checkpoints {
				if cp.Hash == nil {
					t.Errorf("checkpoint %d (height %d) has nil hash", i, cp.Height)
				}
			}
		})
	}
}

// TestCheckpointHeightsValid verifies that all checkpoint heights are non-negative
func TestCheckpointHeightsValid(t *testing.T) {
	testCases := []struct {
		name        string
		checkpoints []chaincfg.Checkpoint
	}{
		{"mainnet", NamecoinMainNetParams.Checkpoints},
		{"testnet", NamecoinTestNetParams.Checkpoints},
		{"regtest", NamecoinRegTestParams.Checkpoints},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for i, cp := range tc.checkpoints {
				if cp.Height < 0 {
					t.Errorf("checkpoint %d has negative height: %d", i, cp.Height)
				}
			}
		})
	}
}

// TestCheckpointUniqueness verifies that checkpoint heights are unique
func TestCheckpointUniqueness(t *testing.T) {
	testCases := []struct {
		name        string
		checkpoints []chaincfg.Checkpoint
	}{
		{"mainnet", NamecoinMainNetParams.Checkpoints},
		{"testnet", NamecoinTestNetParams.Checkpoints},
		{"regtest", NamecoinRegTestParams.Checkpoints},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			heightSeen := make(map[int32]bool)
			for i, cp := range tc.checkpoints {
				if heightSeen[cp.Height] {
					t.Errorf("duplicate checkpoint height %d at index %d", cp.Height, i)
				}
				heightSeen[cp.Height] = true
			}
		})
	}
}
