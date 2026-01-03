package config

import (
	"testing"
)

func TestCalcBlockSubsidy(t *testing.T) {
	tests := []struct {
		name        string
		height      int32
		wantSubsidy int64
		description string
	}{
		{
			name:        "genesis block",
			height:      0,
			wantSubsidy: 50 * CoinValue,
			description: "First block should have 50 NMC reward",
		},
		{
			name:        "block 1",
			height:      1,
			wantSubsidy: 50 * CoinValue,
			description: "Second block should have 50 NMC reward",
		},
		{
			name:        "block 100",
			height:      100,
			wantSubsidy: 50 * CoinValue,
			description: "Block 100 should have 50 NMC reward",
		},
		{
			name:        "last block before first halving",
			height:      209999,
			wantSubsidy: 50 * CoinValue,
			description: "Last block of first era should have 50 NMC reward",
		},
		{
			name:        "first halving - block 210000",
			height:      210000,
			wantSubsidy: 25 * CoinValue,
			description: "First block of second era should have 25 NMC reward",
		},
		{
			name:        "block in second era",
			height:      300000,
			wantSubsidy: 25 * CoinValue,
			description: "Block in second era should have 25 NMC reward",
		},
		{
			name:        "last block before second halving",
			height:      419999,
			wantSubsidy: 25 * CoinValue,
			description: "Last block of second era should have 25 NMC reward",
		},
		{
			name:        "second halving - block 420000",
			height:      420000,
			wantSubsidy: 12.5 * CoinValue,
			description: "First block of third era should have 12.5 NMC reward",
		},
		{
			name:        "third halving - block 630000",
			height:      630000,
			wantSubsidy: 6.25 * CoinValue,
			description: "First block of fourth era should have 6.25 NMC reward",
		},
		{
			name:        "fourth halving - block 840000",
			height:      840000,
			wantSubsidy: 3.125 * CoinValue,
			description: "First block of fifth era should have 3.125 NMC reward",
		},
		{
			name:        "tenth halving - block 2100000",
			height:      2100000,
			wantSubsidy: InitialBlockSubsidy >> 10, // 50 / 1024
			description: "After 10 halvings, subsidy should be 50/1024 NMC",
		},
		{
			name:        "twentieth halving - block 4200000",
			height:      4200000,
			wantSubsidy: InitialBlockSubsidy >> 20, // 50 / 1048576
			description: "After 20 halvings, subsidy should be 50/1048576 NMC",
		},
		{
			name:        "64th halving - subsidy becomes 0",
			height:      64 * 210000,
			wantSubsidy: 0,
			description: "After 64 halvings, subsidy should be 0",
		},
		{
			name:        "beyond 64 halvings",
			height:      65 * 210000,
			wantSubsidy: 0,
			description: "After 64 halvings, subsidy remains 0",
		},
		{
			name:        "far future block",
			height:      100000000,
			wantSubsidy: 0,
			description: "Far future blocks should have 0 subsidy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with mainnet params
			got := CalcBlockSubsidy(tt.height, &NamecoinMainNetParams)
			if got != tt.wantSubsidy {
				t.Errorf("CalcBlockSubsidy(height=%d, mainnet) = %d, want %d\nDescription: %s",
					tt.height, got, tt.wantSubsidy, tt.description)
			}

			// Test with testnet params (same subsidy schedule)
			got = CalcBlockSubsidy(tt.height, &NamecoinTestNetParams)
			if got != tt.wantSubsidy {
				t.Errorf("CalcBlockSubsidy(height=%d, testnet) = %d, want %d\nDescription: %s",
					tt.height, got, tt.wantSubsidy, tt.description)
			}
		})
	}
}

func TestCalcBlockSubsidyRegtest(t *testing.T) {
	// Regtest uses a faster halving interval (150 blocks instead of 210000)
	tests := []struct {
		name        string
		height      int32
		wantSubsidy int64
	}{
		{
			name:        "regtest genesis",
			height:      0,
			wantSubsidy: 50 * CoinValue,
		},
		{
			name:        "regtest block 149 (before first halving)",
			height:      149,
			wantSubsidy: 50 * CoinValue,
		},
		{
			name:        "regtest first halving - block 150",
			height:      150,
			wantSubsidy: 25 * CoinValue,
		},
		{
			name:        "regtest block 299 (before second halving)",
			height:      299,
			wantSubsidy: 25 * CoinValue,
		},
		{
			name:        "regtest second halving - block 300",
			height:      300,
			wantSubsidy: 12.5 * CoinValue,
		},
		{
			name:        "regtest third halving - block 450",
			height:      450,
			wantSubsidy: 6.25 * CoinValue,
		},
		{
			name:        "regtest after 64 halvings",
			height:      64 * 150,
			wantSubsidy: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalcBlockSubsidy(tt.height, &NamecoinRegTestParams)
			if got != tt.wantSubsidy {
				t.Errorf("CalcBlockSubsidy(height=%d, regtest) = %d, want %d",
					tt.height, got, tt.wantSubsidy)
			}
		})
	}
}

// TestSubsidyMoneySupply verifies that the total money supply approaches 21 million NMC
// as expected from the halving schedule
func TestSubsidyMoneySupply(t *testing.T) {
	// Calculate total supply for mainnet
	var totalSupply int64
	for height := int32(0); height < 64*210000; height += 210000 {
		subsidy := CalcBlockSubsidy(height, &NamecoinMainNetParams)
		// Each era has 210,000 blocks with the same subsidy
		totalSupply += subsidy * 210000
	}

	// Convert to NMC (divide by CoinValue)
	totalSupplyNMC := float64(totalSupply) / float64(CoinValue)

	// Should be very close to 21 million NMC
	// Exact value: 20,999,999.9769 NMC
	expectedSupply := 21000000.0
	tolerance := 0.1 // Allow 0.1 NMC difference due to rounding

	if totalSupplyNMC < expectedSupply-tolerance || totalSupplyNMC > expectedSupply+tolerance {
		t.Errorf("Total money supply = %.4f NMC, want approximately %.0f NMC",
			totalSupplyNMC, expectedSupply)
	}
}

// TestSubsidyConsistency ensures subsidy calculation is deterministic
func TestSubsidyConsistency(t *testing.T) {
	// Test that calling the function multiple times with the same input
	// always returns the same result
	testHeights := []int32{0, 1, 210000, 420000, 1000000}

	for _, height := range testHeights {
		result1 := CalcBlockSubsidy(height, &NamecoinMainNetParams)
		result2 := CalcBlockSubsidy(height, &NamecoinMainNetParams)
		result3 := CalcBlockSubsidy(height, &NamecoinMainNetParams)

		if result1 != result2 || result2 != result3 {
			t.Errorf("Inconsistent results for height %d: %d, %d, %d",
				height, result1, result2, result3)
		}
	}
}

// TestSubsidyNeverNegative ensures the subsidy is never negative
func TestSubsidyNeverNegative(t *testing.T) {
	// Test a wide range of heights including edge cases
	testHeights := []int32{
		0,
		1,
		209999,
		210000,
		419999,
		420000,
		1000000,
		10000000,
		100000000,
		2147483647, // Max int32
	}

	for _, height := range testHeights {
		subsidy := CalcBlockSubsidy(height, &NamecoinMainNetParams)
		if subsidy < 0 {
			t.Errorf("Negative subsidy at height %d: %d", height, subsidy)
		}
	}
}

// TestSubsidyDecreases verifies that subsidy never increases
func TestSubsidyDecreases(t *testing.T) {
	prevSubsidy := CalcBlockSubsidy(0, &NamecoinMainNetParams)

	// Check every 10,000 blocks up to 10 million
	for height := int32(10000); height < 10000000; height += 10000 {
		subsidy := CalcBlockSubsidy(height, &NamecoinMainNetParams)

		if subsidy > prevSubsidy {
			t.Errorf("Subsidy increased from %d to %d at height %d",
				prevSubsidy, subsidy, height)
		}

		prevSubsidy = subsidy
	}
}

// TestSubsidyMatchesNamecoinCore verifies that our subsidy calculation
// matches Namecoin Core's GetBlockSubsidy implementation exactly.
// These test cases are based on the actual Namecoin Core implementation:
// https://github.com/namecoin/namecoin-core/blob/master/src/validation.cpp
//
// The GetBlockSubsidy function in Namecoin Core uses the exact same logic:
//
//	CAmount GetBlockSubsidy(int nHeight, const Consensus::Params& consensusParams)
//	{
//	    int halvings = nHeight / consensusParams.nSubsidyHalvingInterval;
//	    if (halvings >= 64)
//	        return 0;
//	    CAmount nSubsidy = 50 * COIN;
//	    nSubsidy >>= halvings;
//	    return nSubsidy;
//	}
//
// This test verifies bit-for-bit compatibility with Namecoin Core's subsidy calculation.
func TestSubsidyMatchesNamecoinCore(t *testing.T) {
	tests := []struct {
		name        string
		height      int32
		wantSubsidy int64
		description string
	}{
		{
			name:        "genesis_block_mainnet",
			height:      0,
			wantSubsidy: 5000000000, // 50 NMC in satoshis
			description: "Genesis block reward (Namecoin Core: 50 * COIN)",
		},
		{
			name:        "early_block_mainnet",
			height:      19200, // AuxPoW activation height
			wantSubsidy: 5000000000,
			description: "Block at AuxPoW activation (still 50 NMC)",
		},
		{
			name:        "halving_boundary_minus_1",
			height:      209999,
			wantSubsidy: 5000000000,
			description: "Last block before first halving (still 50 NMC)",
		},
		{
			name:        "first_halving_exact",
			height:      210000,
			wantSubsidy: 2500000000, // 25 NMC in satoshis
			description: "First halving at block 210000 (Namecoin Core: 50 >> 1)",
		},
		{
			name:        "second_halving_exact",
			height:      420000,
			wantSubsidy: 1250000000, // 12.5 NMC in satoshis
			description: "Second halving at block 420000 (Namecoin Core: 50 >> 2)",
		},
		{
			name:        "third_halving_exact",
			height:      630000,
			wantSubsidy: 625000000, // 6.25 NMC in satoshis
			description: "Third halving at block 630000 (Namecoin Core: 50 >> 3)",
		},
		{
			name:        "fourth_halving_exact",
			height:      840000,
			wantSubsidy: 312500000, // 3.125 NMC in satoshis
			description: "Fourth halving at block 840000 (Namecoin Core: 50 >> 4)",
		},
		{
			name:        "fifth_halving",
			height:      1050000,
			wantSubsidy: 156250000, // 1.5625 NMC in satoshis
			description: "Fifth halving (Namecoin Core: 50 >> 5)",
		},
		{
			name:        "tenth_halving",
			height:      2100000,
			wantSubsidy: 4882812, // 50 / 1024 NMC = 0.04882812 NMC in satoshis
			description: "Tenth halving (Namecoin Core: 50 >> 10)",
		},
		{
			name:        "twentieth_halving",
			height:      4200000,
			wantSubsidy: 4768, // 50 / 1048576 NMC = 0.00004768 NMC in satoshis
			description: "Twentieth halving (Namecoin Core: 50 >> 20)",
		},
		{
			name:        "thirtieth_halving",
			height:      6300000,
			wantSubsidy: 4, // 50 / 1073741824 NMC = 0.00000004 NMC in satoshis (4 satoshis)
			description: "Thirtieth halving (Namecoin Core: 50 >> 30)",
		},
		{
			name:        "halving_33_very_small_reward",
			height:      6930000,
			wantSubsidy: 0, // 50 >> 33 = 0 (less than 1 satoshi)
			description: "33rd halving - subsidy rounds to 0 satoshis (Namecoin Core: 50 >> 33)",
		},
		{
			name:        "halving_63_last_before_zero",
			height:      13230000,
			wantSubsidy: 0,
			description: "63rd halving - still 0 (Namecoin Core: 50 >> 63)",
		},
		{
			name:        "halving_64_zero_enforced",
			height:      13440000,
			wantSubsidy: 0,
			description: "64th halving - zero enforced by halvings >= 64 check (Namecoin Core)",
		},
		{
			name:        "beyond_64_halvings",
			height:      13440001,
			wantSubsidy: 0,
			description: "Beyond 64 halvings - always zero (Namecoin Core)",
		},
		{
			name:        "max_int32_height",
			height:      2147483647,
			wantSubsidy: 0,
			description: "Maximum int32 height - subsidy is zero (Namecoin Core)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalcBlockSubsidy(tt.height, &NamecoinMainNetParams)
			if got != tt.wantSubsidy {
				t.Errorf("CalcBlockSubsidy(height=%d) = %d satoshis, want %d satoshis\nDescription: %s\nDifference: %d satoshis",
					tt.height, got, tt.wantSubsidy, tt.description, got-tt.wantSubsidy)
			}
		})
	}
}

// TestSubsidyEdgeCasesMatchNamecoinCore tests edge cases and boundary conditions
// to ensure our implementation handles all scenarios the same way as Namecoin Core.
func TestSubsidyEdgeCasesMatchNamecoinCore(t *testing.T) {
	tests := []struct {
		name        string
		height      int32
		description string
	}{
		{
			name:        "exactly_at_halving_boundary",
			height:      210000,
			description: "Exactly at halving boundary - should use new reward",
		},
		{
			name:        "one_block_before_halving",
			height:      209999,
			description: "One block before halving - should use old reward",
		},
		{
			name:        "one_block_after_halving",
			height:      210001,
			description: "One block after halving - should use new reward",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subsidy := CalcBlockSubsidy(tt.height, &NamecoinMainNetParams)

			// Verify subsidy is non-negative
			if subsidy < 0 {
				t.Errorf("Negative subsidy at height %d: %d (Namecoin Core never returns negative)",
					tt.height, subsidy)
			}

			// Verify subsidy is correct based on halving interval
			halvings := tt.height / NamecoinMainNetParams.SubsidyReductionInterval
			expectedSubsidy := int64(InitialBlockSubsidy)
			if halvings >= MaxHalvings {
				expectedSubsidy = 0
			} else {
				expectedSubsidy >>= uint(halvings)
			}

			if subsidy != expectedSubsidy {
				t.Errorf("Height %d: got %d, want %d (based on %d halvings)\nDescription: %s",
					tt.height, subsidy, expectedSubsidy, halvings, tt.description)
			}
		})
	}
}

// TestSubsidyTotalSupplyMatchesNamecoin verifies that the total coin supply
// calculated from our subsidy function matches Namecoin's expected supply.
// This is a critical consensus check - if our total supply doesn't match,
// we would fork from the Namecoin network.
func TestSubsidyTotalSupplyMatchesNamecoin(t *testing.T) {
	var totalSupply int64

	// Calculate total supply by summing all block rewards
	// We iterate through each halving era
	for era := int32(0); era < 64; era++ {
		startHeight := era * NamecoinMainNetParams.SubsidyReductionInterval
		subsidy := CalcBlockSubsidy(startHeight, &NamecoinMainNetParams)

		// Add this era's total supply (subsidy * blocks in era)
		totalSupply += subsidy * int64(NamecoinMainNetParams.SubsidyReductionInterval)

		// If subsidy is 0, no point continuing
		if subsidy == 0 {
			break
		}
	}

	// Convert to NMC (from satoshis)
	totalSupplyNMC := float64(totalSupply) / float64(CoinValue)

	// Namecoin's total supply should be approximately 21,000,000 NMC
	// The exact value is 20,999,999.9769 NMC due to the halving schedule
	// This matches Bitcoin and Namecoin Core exactly
	expectedSupply := 20999999.9769
	tolerance := 0.0001 // Very tight tolerance for exact match

	if totalSupplyNMC < expectedSupply-tolerance || totalSupplyNMC > expectedSupply+tolerance {
		t.Errorf("Total money supply = %.10f NMC, want %.10f NMC (Namecoin Core)\nDifference: %.10f NMC",
			totalSupplyNMC, expectedSupply, totalSupplyNMC-expectedSupply)
	}

	t.Logf("Total money supply: %.10f NMC (matches Namecoin Core)", totalSupplyNMC)
}

// TestSubsidyBitShiftMatchesNamecoinCore verifies that our bit-shift based
// halving calculation produces identical results to Namecoin Core's implementation.
// This is critical because any difference would cause a consensus fork.
func TestSubsidyBitShiftMatchesNamecoinCore(t *testing.T) {
	// Test that our bit-shift calculation matches Namecoin Core's exactly
	// Namecoin Core uses: nSubsidy >>= halvings
	// We use the same approach in CalcBlockSubsidy

	testCases := []struct {
		halvings      int32
		expectedShift int64
	}{
		{0, 5000000000}, // 50 NMC (no shift)
		{1, 2500000000}, // 50 >> 1 = 25 NMC
		{2, 1250000000}, // 50 >> 2 = 12.5 NMC
		{3, 625000000},  // 50 >> 3 = 6.25 NMC
		{4, 312500000},  // 50 >> 4 = 3.125 NMC
		{10, 4882812},   // 50 >> 10 = 0.04882812... NMC
		{20, 4768},      // 50 >> 20 = 0.00004768... NMC
		{30, 4},         // 50 >> 30 = 0.00000004... NMC (4 satoshis)
		{32, 1},         // 50 >> 32 = 0.00000001... NMC (1 satoshi)
		{33, 0},         // 50 >> 33 = 0 (rounds to 0 satoshis)
		{63, 0},         // 50 >> 63 = 0
		{64, 0},         // Explicitly set to 0 by halvings >= 64 check
	}

	for _, tc := range testCases {
		t.Run(t.Name(), func(t *testing.T) {
			height := tc.halvings * NamecoinMainNetParams.SubsidyReductionInterval
			got := CalcBlockSubsidy(height, &NamecoinMainNetParams)

			if got != tc.expectedShift {
				t.Errorf("After %d halvings (height %d): got %d, want %d\nBit shift: 50 NMC >> %d",
					tc.halvings, height, got, tc.expectedShift, tc.halvings)
			}
		})
	}
}
