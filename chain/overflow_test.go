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
// This test first demonstrates the raw overflow bug, then verifies the fix using safeCalcExpiresAt.
func Test_bug_8_height_overflow_in_expiration_calculation(t *testing.T) {
	// PART 1: Demonstrate the raw overflow bug (without fix)
	t.Run("raw_overflow_bug", func(t *testing.T) {
		// Test case 1: Height at max int32
		height := int32(math.MaxInt32)
		rawExpiresAt := height + config.NameExpirationBlocks
		
		// This demonstrates the bug: overflow causes negative value
		if rawExpiresAt >= 0 {
			t.Errorf("Expected overflow to cause negative value, but got positive: %d", rawExpiresAt)
		}
		
		// Test case 2: Height that would cause overflow
		height2 := int32(math.MaxInt32) - config.NameExpirationBlocks + 1
		rawExpiresAt2 := height2 + config.NameExpirationBlocks
		
		// Verify overflow occurs (result wraps around to negative or less than input)
		if rawExpiresAt2 >= height2 {
			t.Errorf("Expected overflow/wrap-around, but got valid result: %d >= %d", rawExpiresAt2, height2)
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

// Test_bug_8_validateHeightForNameOp tests the validation function
func Test_bug_8_validateHeightForNameOp(t *testing.T) {
	tests := []struct {
		name      string
		height    int32
		wantError bool
	}{
		{
			name:      "normal_height",
			height:    100000,
			wantError: false,
		},
		{
			name:      "max_safe_height",
			height:    math.MaxInt32 - config.NameExpirationBlocks,
			wantError: false,
		},
		{
			name:      "one_above_max_safe",
			height:    math.MaxInt32 - config.NameExpirationBlocks + 1,
			wantError: true,
		},
		{
			name:      "max_int32",
			height:    math.MaxInt32,
			wantError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHeightForNameOp(tt.height)
			if (err != nil) != tt.wantError {
				t.Errorf("validateHeightForNameOp(height=%d) error = %v, wantError %v",
					tt.height, err, tt.wantError)
			}
		})
	}
}

