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
