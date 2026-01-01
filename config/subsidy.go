package config

import (
	"github.com/btcsuite/btcd/chaincfg"
)

const (
	// CoinValue is the number of satoshis in one coin (10^8)
	CoinValue = 100000000

	// InitialBlockSubsidy is the initial block reward in satoshis (50 NMC)
	InitialBlockSubsidy = 50 * CoinValue

	// MaxHalvings is the maximum number of subsidy halvings before reward becomes zero
	// After 64 halvings, the subsidy would be so small it rounds to zero anyway
	MaxHalvings = 64
)

// CalcBlockSubsidy calculates the block subsidy (mining reward) for a given block height
// following Namecoin's consensus rules.
//
// The subsidy schedule is:
// - Initial subsidy: 50 NMC (5,000,000,000 satoshis)
// - Halving interval: Every 210,000 blocks (same as Bitcoin)
// - Maximum supply: ~21,000,000 NMC
//
// The subsidy halves every 210,000 blocks:
// - Blocks 0-209,999: 50 NMC
// - Blocks 210,000-419,999: 25 NMC
// - Blocks 420,000-629,999: 12.5 NMC
// - And so on...
//
// After 64 halvings (block 13,440,000), the subsidy becomes 0.
//
// Note: This matches Bitcoin's subsidy schedule, which Namecoin inherited.
// There is no special "smooth start" in Namecoin - the first block reward is 50 NMC.
func CalcBlockSubsidy(height int32, chainParams *chaincfg.Params) int64 {
	// Calculate the number of halvings that have occurred
	halvings := int32(height) / chainParams.SubsidyReductionInterval

	// Once we've had MaxHalvings, the subsidy is 0 (no more coins are created)
	if halvings >= MaxHalvings {
		return 0
	}

	// Start with the initial subsidy
	subsidy := int64(InitialBlockSubsidy)

	// Right shift divides by 2 for each halving
	// This is equivalent to: subsidy = subsidy / (2^halvings)
	// But bit shifting is faster and matches Bitcoin/Namecoin Core's implementation
	subsidy >>= uint(halvings)

	return subsidy
}
