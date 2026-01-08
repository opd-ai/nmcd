package chain

import (
	"math"
	"testing"

	"github.com/opd-ai/nmcd/config"
)

// Test_bug_8_height_overflow_in_expiration_calculation demonstrates the integer overflow
// bug documented in AUDIT.md Issue #8. When a very large block height (close to max int32)
// is used to calculate ExpiresAt, adding NameExpirationBlocks causes int32 overflow.
//
// This test first demonstrates the overflow condition using int64 to avoid undefined
// behavior, then verifies the fix using safeCalcExpiresAt.
func Test_bug_8_height_overflow_in_expiration_calculation(t *testing.T) {
	// PART 1: Demonstrate the overflow *condition* (without actually overflowing int32)
	t.Run("overflow_condition", func(t *testing.T) {
		// Test case 1: Height at max int32
		height := int32(math.MaxInt32)

		// Perform the calculation in int64 to detect that it would overflow int32.
		overflowSum := int64(height) + int64(config.NameExpirationBlocks)
		if overflowSum <= int64(math.MaxInt32) {
			t.Errorf("Expected height + NameExpirationBlocks to exceed MaxInt32 (overflow condition), got %d", overflowSum)
		}

		// Test case 2: Minimal height that would cause overflow when adding NameExpirationBlocks
		height2 := int32(math.MaxInt32) - config.NameExpirationBlocks + 1
		overflowSum2 := int64(height2) + int64(config.NameExpirationBlocks)

		// Verify that this boundary height also exceeds MaxInt32 when adding NameExpirationBlocks.
		if overflowSum2 <= int64(math.MaxInt32) {
			t.Errorf("Expected boundary height to overflow when adding NameExpirationBlocks, got %d", overflowSum2)
		}
	})
	
	// PART 2: Verify the fix using safeCalcExpiresAt
	t.Run("safe_calculation_fix", func(t *testing.T) {
		// Test case 1: Height at max int32 should be clamped to max int32
		height := int32(math.MaxInt32)
		safeExpiresAt := safeCalcExpiresAt(height)
		
		if safeExpiresAt != math.MaxInt32 {
			t.Errorf("Expected safeCalcExpiresAt to clamp to MaxInt32, got %d", safeExpiresAt)
		}
		
		if safeExpiresAt < 0 {
			t.Errorf("safeCalcExpiresAt returned negative value: %d", safeExpiresAt)
		}
		
		// Test case 2: Height that would overflow should be clamped to max int32
		height2 := int32(math.MaxInt32) - config.NameExpirationBlocks + 1
		safeExpiresAt2 := safeCalcExpiresAt(height2)
		
		if safeExpiresAt2 != math.MaxInt32 {
			t.Errorf("Expected safeCalcExpiresAt to clamp to MaxInt32, got %d", safeExpiresAt2)
		}
		
		if safeExpiresAt2 < height2 {
			t.Errorf("safeCalcExpiresAt should not wrap around: %d < %d", safeExpiresAt2, height2)
		}
		
		// Test case 3: Safe height should calculate normally
		safeHeight := int32(math.MaxInt32) - config.NameExpirationBlocks // Exactly at boundary
		safeExpiresAt3 := safeCalcExpiresAt(safeHeight)
		
		expectedSafeExpiresAt := safeHeight + config.NameExpirationBlocks
		if safeExpiresAt3 != expectedSafeExpiresAt {
			t.Errorf("Safe calculation incorrect: got %d, want %d", safeExpiresAt3, expectedSafeExpiresAt)
		}
		
		// Test case 4: Normal heights should work normally
		normalHeight := int32(100000)
		normalExpiresAt := safeCalcExpiresAt(normalHeight)
		expectedNormalExpiresAt := normalHeight + config.NameExpirationBlocks
		
		if normalExpiresAt != expectedNormalExpiresAt {
			t.Errorf("Normal height calculation incorrect: got %d, want %d", normalExpiresAt, expectedNormalExpiresAt)
		}
	})
}

