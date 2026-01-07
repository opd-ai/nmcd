package chain

import (
	"testing"

	"github.com/opd-ai/nmcd/namedb"
)

// FuzzParseNameScript fuzzes the name operation script parser to catch:
// - Buffer overflows with malformed scripts
// - Invalid opcodes
// - Truncated push data
// - Missing drop opcodes
// - Oversized name/value data
// - Malformed UTF-8 in names/values
// - Edge cases in script length validation
//
// Run with: go test -fuzz=FuzzParseNameScript -fuzztime=1m
func FuzzParseNameScript(f *testing.F) {
	// Seed corpus with valid name operation scripts

	// Valid NAME_NEW: OP_NAME_NEW <20 bytes> OP_2DROP <25 byte P2PKH>
	nameNewScript := make([]byte, 0, 47)
	nameNewScript = append(nameNewScript, opNameNew)     // OP_NAME_NEW (0xd0)
	nameNewScript = append(nameNewScript, 0x14)          // Push 20 bytes
	nameNewScript = append(nameNewScript, make([]byte, 20)...) // 20 bytes hash
	nameNewScript = append(nameNewScript, op2Drop)       // OP_2DROP (0x6d)
	nameNewScript = append(nameNewScript, make([]byte, 25)...) // P2PKH (25 bytes)
	f.Add(nameNewScript)

	// Valid NAME_FIRSTUPDATE: OP_NAME_FIRSTUPDATE <name> <rand> <value> OP_2DROP OP_2DROP <P2PKH>
	nameFirstUpdateScript := make([]byte, 0, 100)
	nameFirstUpdateScript = append(nameFirstUpdateScript, opNameFirstUpdate) // 0xd1
	nameFirstUpdateScript = append(nameFirstUpdateScript, 0x07)              // Push 7 bytes (name)
	nameFirstUpdateScript = append(nameFirstUpdateScript, []byte("d/test")...)
	nameFirstUpdateScript = append(nameFirstUpdateScript, 0x10)              // Push 16 bytes (rand)
	nameFirstUpdateScript = append(nameFirstUpdateScript, make([]byte, 16)...)
	nameFirstUpdateScript = append(nameFirstUpdateScript, 0x0a)              // Push 10 bytes (value)
	nameFirstUpdateScript = append(nameFirstUpdateScript, []byte(`{"ip":"1.2.3.4"}`)...)
	nameFirstUpdateScript = append(nameFirstUpdateScript, op2Drop) // OP_2DROP
	nameFirstUpdateScript = append(nameFirstUpdateScript, op2Drop) // OP_2DROP
	nameFirstUpdateScript = append(nameFirstUpdateScript, make([]byte, 25)...) // P2PKH
	f.Add(nameFirstUpdateScript)

	// Valid NAME_UPDATE: OP_NAME_UPDATE <name> <value> OP_2DROP OP_DROP <P2PKH>
	nameUpdateScript := make([]byte, 0, 80)
	nameUpdateScript = append(nameUpdateScript, opNameUpdate) // 0xd2
	nameUpdateScript = append(nameUpdateScript, 0x07)         // Push 7 bytes (name)
	nameUpdateScript = append(nameUpdateScript, []byte("d/test")...)
	nameUpdateScript = append(nameUpdateScript, 0x0a)         // Push 10 bytes (value)
	nameUpdateScript = append(nameUpdateScript, []byte(`{"ip":"5.6.7.8"}`)...)
	nameUpdateScript = append(nameUpdateScript, op2Drop) // OP_2DROP
	nameUpdateScript = append(nameUpdateScript, opDrop)  // OP_DROP
	nameUpdateScript = append(nameUpdateScript, make([]byte, 25)...) // P2PKH
	f.Add(nameUpdateScript)

	// Edge cases
	f.Add([]byte{})                         // Empty script
	f.Add([]byte{opNameNew})                // Just opcode
	f.Add([]byte{opNameNew, 0x01, 0x00})    // Truncated
	f.Add(make([]byte, 10000))              // Very long script
	f.Add([]byte{0xff, 0xff, 0xff})         // Invalid opcodes
	f.Add([]byte{opNameNew, 0xff})          // Invalid push data length
	f.Add([]byte{opNameUpdate, 0x00, 0x00}) // Empty push data

	f.Fuzz(func(t *testing.T, script []byte) {
		// Attempt to parse the script
		// The function should never panic, even with malformed input
		op, name, value, err := parseNameScript(script)

		if err != nil {
			// Error is acceptable for malformed scripts
			return
		}

		// If parsing succeeded, verify the results are valid
		if op != namedb.NameNew && op != namedb.NameFirstUpdate && op != namedb.NameUpdate {
			t.Errorf("invalid operation type: %d", op)
			return
		}

		// Verify name length is within limits for operations that have names
		if op == namedb.NameFirstUpdate || op == namedb.NameUpdate {
			if len(name) > 255 {
				t.Errorf("name too long: %d bytes (max 255)", len(name))
			}
		}

		// Verify value length is within limits for operations that have values
		if op == namedb.NameFirstUpdate || op == namedb.NameUpdate {
			if len(value) > 1023 {
				t.Errorf("value too long: %d bytes (max 1023)", len(value))
			}
		}

		// Try to parse again with the full parser to ensure consistency
		op2, name2, value2, extra, err := parseNameScriptFull(script)
		if err != nil {
			t.Errorf("parseNameScriptFull failed but parseNameScript succeeded: %v", err)
			return
		}

		// Verify both parsers agree
		if op2 != op {
			t.Errorf("operation mismatch: parseNameScript=%d, parseNameScriptFull=%d", op, op2)
		}
		if name2 != name {
			t.Errorf("name mismatch: parseNameScript=%q, parseNameScriptFull=%q", name, name2)
		}
		if value2 != value {
			t.Errorf("value mismatch: parseNameScript=%q, parseNameScriptFull=%q", value, value2)
		}

		// For NAME_NEW, extra should contain the hash
		if op == namedb.NameNew && len(extra) == 0 {
			t.Errorf("NAME_NEW missing hash in extra data")
		}

		// For NAME_FIRSTUPDATE, extra should contain rand
		if op == namedb.NameFirstUpdate && len(extra) == 0 {
			t.Errorf("NAME_FIRSTUPDATE missing rand in extra data")
		}
	})
}

// FuzzReadPushData fuzzes the push data reader used in script parsing.
// This tests the low-level primitive that extracts variable-length data
// from Bitcoin/Namecoin scripts.
//
// Run with: go test -fuzz=FuzzReadPushData -fuzztime=1m
func FuzzReadPushData(f *testing.F) {
	// Seed with valid push data patterns
	f.Add([]byte{0x01, 0xff}, 0)                    // Push 1 byte
	f.Add([]byte{0x4b, 0xff}, 0)                    // Push 75 bytes (max OP_PUSHDATA)
	f.Add([]byte{0x4c, 0x01, 0xff}, 0)              // OP_PUSHDATA1
	f.Add([]byte{0x4d, 0x01, 0x00, 0xff}, 0)        // OP_PUSHDATA2
	f.Add([]byte{0x4e, 0x01, 0x00, 0x00, 0x00, 0xff}, 0) // OP_PUSHDATA4
	f.Add([]byte{0x00}, 0)                          // Push 0 bytes
	f.Add([]byte{}, 0)                              // Empty script
	f.Add([]byte{0x01}, 0)                          // Truncated
	f.Add([]byte{0x4c}, 0)                          // OP_PUSHDATA1 without length
	f.Add([]byte{0x4c, 0xff}, 0)                    // OP_PUSHDATA1 with truncated data

	f.Fuzz(func(t *testing.T, script []byte, offset int) {
		// Clamp offset to valid range
		if offset < 0 {
			offset = 0
		}
		if offset > len(script) {
			offset = len(script)
		}

		// Attempt to read push data
		// Should never panic regardless of input
		data, newOffset, err := readPushData(script, offset)

		if err != nil {
			// Error is acceptable for malformed scripts
			return
		}

		// If successful, verify invariants
		if newOffset < offset {
			t.Errorf("newOffset (%d) < offset (%d)", newOffset, offset)
		}
		if newOffset > len(script) {
			t.Errorf("newOffset (%d) > script length (%d)", newOffset, len(script))
		}
		if data == nil {
			t.Errorf("data is nil but no error returned")
		}

		// Verify we can read the data length from the script
		if offset < len(script) {
			opcode := script[offset]
			
			// Direct push (1-75 bytes)
			if opcode >= 0x01 && opcode <= 0x4b {
				expectedLen := int(opcode)
				if len(data) != expectedLen {
					t.Errorf("direct push: data length %d != expected %d", len(data), expectedLen)
				}
			}
			
			// OP_PUSHDATA1 (0x4c)
			if opcode == 0x4c && offset+1 < len(script) {
				expectedLen := int(script[offset+1])
				if len(data) != expectedLen {
					t.Errorf("OP_PUSHDATA1: data length %d != expected %d", len(data), expectedLen)
				}
			}
			
			// OP_PUSHDATA2 (0x4d)
			if opcode == 0x4d && offset+2 < len(script) {
				expectedLen := int(script[offset+1]) | (int(script[offset+2]) << 8)
				if len(data) != expectedLen {
					t.Errorf("OP_PUSHDATA2: data length %d != expected %d", len(data), expectedLen)
				}
			}
			
			// OP_PUSHDATA4 (0x4e)
			if opcode == 0x4e && offset+4 < len(script) {
				expectedLen := int(script[offset+1]) | (int(script[offset+2]) << 8) |
					(int(script[offset+3]) << 16) | (int(script[offset+4]) << 24)
				// Only check if reasonable length (prevent integer overflow)
				if expectedLen >= 0 && expectedLen < 1000000 {
					if len(data) != expectedLen {
						t.Errorf("OP_PUSHDATA4: data length %d != expected %d", len(data), expectedLen)
					}
				}
			}
		}
	})
}

// FuzzValidateScriptFormat fuzzes script format validation to ensure:
// - Proper handling of missing drop opcodes
// - Correct P2PKH suffix validation
// - No panics with malformed input
//
// Run with: go test -fuzz=FuzzValidateScriptFormat -fuzztime=1m
func FuzzValidateScriptFormat(f *testing.F) {
	// Seed with valid patterns
	f.Add([]byte{op2Drop}, int(0), int(0))  // NAME_NEW format
	f.Add([]byte{op2Drop, op2Drop}, int(1), int(0)) // NAME_FIRSTUPDATE format
	f.Add([]byte{op2Drop, opDrop}, int(2), int(0))  // NAME_UPDATE format
	f.Add([]byte{}, int(0), int(0))         // Empty
	f.Add(make([]byte, 100), int(0), int(50)) // Long script

	f.Fuzz(func(t *testing.T, script []byte, opTypeInt int, dataEndOffset int) {
		// Clamp opType to valid range
		var opType namedb.NameOperation
		switch opTypeInt % 3 {
		case 0:
			opType = namedb.NameNew
		case 1:
			opType = namedb.NameFirstUpdate
		case 2:
			opType = namedb.NameUpdate
		}

		// Clamp dataEndOffset to valid range
		if dataEndOffset < 0 {
			dataEndOffset = 0
		}
		if dataEndOffset > len(script) {
			dataEndOffset = len(script)
		}

		// Validate script format - should never panic
		offset, err := validateScriptFormat(script, opType, dataEndOffset)

		if err != nil {
			// Error is acceptable for malformed scripts
			return
		}

		// If validation succeeded, verify offset is valid
		if offset < 0 {
			t.Errorf("negative offset: %d", offset)
		}
		if offset > len(script) {
			t.Errorf("offset %d > script length %d", offset, len(script))
		}
		if offset < dataEndOffset {
			t.Errorf("offset %d < dataEndOffset %d", offset, dataEndOffset)
		}
	})
}
