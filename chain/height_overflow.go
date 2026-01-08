package chain

import (
	"fmt"
	"math"

	"github.com/opd-ai/nmcd/config"
)

// safeCalcExpiresAt calculates the expiration height with overflow protection.
// If the addition would overflow int32, it returns math.MaxInt32 (maximum safe value).
// This prevents negative expiration heights that would cause incorrect behavior.
//
// The function checks if height + config.NameExpirationBlocks would exceed MaxInt32.
// In practice, this is a theoretical concern as Namecoin would need ~400 years
// to reach heights where this matters (at 10 min/block).
func safeCalcExpiresAt(height int32) int32 {
	// Check if addition would overflow int32
	// math.MaxInt32 = 2147483647
	// If height > (MaxInt32 - NameExpirationBlocks), addition would overflow
	if height > int32(math.MaxInt32)-config.NameExpirationBlocks {
		// Return maximum safe value instead of overflowing to negative
		return int32(math.MaxInt32)
	}
	return height + config.NameExpirationBlocks
}

// validateHeightForNameOp validates that a block height is safe for name operations.
// Returns an error if the height would cause overflow in expiration calculations.
// This is a defensive check for theoretical edge cases.
func validateHeightForNameOp(height int32) error {
	// Heights above this threshold would cause overflow in expiration calculation
	maxSafeHeight := int32(math.MaxInt32) - config.NameExpirationBlocks
	
	if height > maxSafeHeight {
		return fmt.Errorf("block height %d too large for name operations (max safe height: %d)", 
			height, maxSafeHeight)
	}
	
	return nil
}
