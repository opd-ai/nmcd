package chain

import (
	"testing"

	"github.com/opd-ai/nmcd/namedb"
)

// buildScript is a helper to create scripts with proper push data encoding.
func buildScript(parts ...[]byte) []byte {
	var result []byte
	for _, part := range parts {
		result = append(result, part...)
	}
	return result
}

// pushData creates a Bitcoin-style push data encoding for the given data.
// For data <= 75 bytes, uses direct push (opcode = length).
// For data 76-255 bytes, uses OP_PUSHDATA1.
// For data 256+ bytes, uses OP_PUSHDATA2.
func pushData(data []byte) []byte {
	length := len(data)
	switch {
	case length <= 75:
		return append([]byte{byte(length)}, data...)
	case length <= 255:
		return append([]byte{opPushData1, byte(length)}, data...)
	default:
		return append([]byte{opPushData2, byte(length & 0xff), byte(length >> 8)}, data...)
	}
}

func TestParseNameScript_NameNew(t *testing.T) {
	// NAME_NEW: OP_NAME_NEW <hash> ...
	// The hash is typically 20 bytes
	hash := make([]byte, 20)
	for i := range hash {
		hash[i] = byte(i)
	}

	script := buildScript(
		[]byte{opNameNew},
		pushData(hash),
	)

	op, name, value, err := parseNameScript(script)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op != namedb.NameNew {
		t.Errorf("expected NameNew, got %v", op)
	}
	if name != "" {
		t.Errorf("expected empty name for NameNew, got %q", name)
	}
	if value != "" {
		t.Errorf("expected empty value for NameNew, got %q", value)
	}
}

func TestParseNameScript_NameFirstUpdate(t *testing.T) {
	// NAME_FIRSTUPDATE: OP_NAME_FIRSTUPDATE <name> <rand> <value> ...
	testCases := []struct {
		name        string
		scriptName  string
		rand        []byte
		scriptValue string
	}{
		{
			name:        "simple name and value",
			scriptName:  "d/example",
			rand:        make([]byte, 20),
			scriptValue: `{"ip":"1.2.3.4"}`,
		},
		{
			name:        "short name",
			scriptName:  "d/x",
			rand:        []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
			scriptValue: "test",
		},
		{
			name:        "empty value",
			scriptName:  "d/test",
			rand:        make([]byte, 20),
			scriptValue: "",
		},
		{
			name:        "long value",
			scriptName:  "d/longtest",
			rand:        make([]byte, 20),
			scriptValue: string(make([]byte, 100)),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			script := buildScript(
				[]byte{opNameFirstUpdate},
				pushData([]byte(tc.scriptName)),
				pushData(tc.rand),
				pushData([]byte(tc.scriptValue)),
			)

			op, name, value, err := parseNameScript(script)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if op != namedb.NameFirstUpdate {
				t.Errorf("expected NameFirstUpdate, got %v", op)
			}
			if name != tc.scriptName {
				t.Errorf("expected name %q, got %q", tc.scriptName, name)
			}
			if value != tc.scriptValue {
				t.Errorf("expected value %q, got %q", tc.scriptValue, value)
			}
		})
	}
}

func TestParseNameScript_NameUpdate(t *testing.T) {
	// NAME_UPDATE: OP_NAME_UPDATE <name> <value> ...
	testCases := []struct {
		name        string
		scriptName  string
		scriptValue string
	}{
		{
			name:        "simple update",
			scriptName:  "d/example",
			scriptValue: `{"ip":"5.6.7.8"}`,
		},
		{
			name:        "empty value",
			scriptName:  "d/test",
			scriptValue: "",
		},
		{
			name:        "unicode value",
			scriptName:  "d/unicode",
			scriptValue: "hello 世界",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			script := buildScript(
				[]byte{opNameUpdate},
				pushData([]byte(tc.scriptName)),
				pushData([]byte(tc.scriptValue)),
			)

			op, name, value, err := parseNameScript(script)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if op != namedb.NameUpdate {
				t.Errorf("expected NameUpdate, got %v", op)
			}
			if name != tc.scriptName {
				t.Errorf("expected name %q, got %q", tc.scriptName, name)
			}
			if value != tc.scriptValue {
				t.Errorf("expected value %q, got %q", tc.scriptValue, value)
			}
		})
	}
}

func TestParseNameScript_NotNameOperation(t *testing.T) {
	testCases := []struct {
		name   string
		script []byte
	}{
		{
			name:   "empty script",
			script: []byte{},
		},
		{
			name:   "too short",
			script: []byte{0x00},
		},
		{
			name:   "standard p2pkh",
			script: []byte{0x76, 0xa9, 0x14}, // OP_DUP OP_HASH160 ...
		},
		{
			name:   "unknown opcode",
			script: []byte{0x99, 0x01, 0x00},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := parseNameScript(tc.script)
			if err == nil {
				t.Error("expected error for non-name script")
			}
		})
	}
}

func TestParseNameScript_TruncatedScripts(t *testing.T) {
	testCases := []struct {
		name   string
		script []byte
	}{
		{
			name:   "NameFirstUpdate missing name",
			script: []byte{opNameFirstUpdate},
		},
		{
			name:   "NameFirstUpdate truncated name",
			script: []byte{opNameFirstUpdate, 0x05, 0x01, 0x02}, // says 5 bytes but only 2
		},
		{
			name: "NameFirstUpdate missing rand",
			script: buildScript(
				[]byte{opNameFirstUpdate},
				pushData([]byte("d/test")),
			),
		},
		{
			name: "NameFirstUpdate missing value",
			script: buildScript(
				[]byte{opNameFirstUpdate},
				pushData([]byte("d/test")),
				pushData(make([]byte, 20)),
			),
		},
		{
			name:   "NameUpdate missing name",
			script: []byte{opNameUpdate},
		},
		{
			name: "NameUpdate missing value",
			script: buildScript(
				[]byte{opNameUpdate},
				pushData([]byte("d/test")),
			),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := parseNameScript(tc.script)
			if err == nil {
				t.Error("expected error for truncated script")
			}
		})
	}
}

func TestParseNameScript_PushDataFormats(t *testing.T) {
	// Test different push data encoding formats

	t.Run("direct push (1-75 bytes)", func(t *testing.T) {
		name := "d/example"
		value := "test value"
		script := buildScript(
			[]byte{opNameUpdate},
			pushData([]byte(name)),
			pushData([]byte(value)),
		)

		op, parsedName, parsedValue, err := parseNameScript(script)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if op != namedb.NameUpdate {
			t.Errorf("expected NameUpdate, got %v", op)
		}
		if parsedName != name {
			t.Errorf("expected name %q, got %q", name, parsedName)
		}
		if parsedValue != value {
			t.Errorf("expected value %q, got %q", value, parsedValue)
		}
	})

	t.Run("OP_PUSHDATA1 (76-255 bytes)", func(t *testing.T) {
		name := "d/example"
		// Create a value that's exactly 100 bytes with identifiable content
		valueBytes := make([]byte, 100)
		for i := range valueBytes {
			valueBytes[i] = byte(i)
		}
		value := string(valueBytes)
		script := buildScript(
			[]byte{opNameUpdate},
			pushData([]byte(name)),
			pushData([]byte(value)),
		)

		op, parsedName, parsedValue, err := parseNameScript(script)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if op != namedb.NameUpdate {
			t.Errorf("expected NameUpdate, got %v", op)
		}
		if parsedName != name {
			t.Errorf("expected name %q, got %q", name, parsedName)
		}
		if parsedValue != value {
			t.Errorf("expected value to match, got length %d vs %d", len(parsedValue), len(value))
		}
	})

	t.Run("OP_PUSHDATA2 (256+ bytes)", func(t *testing.T) {
		name := "d/example"
		// Create a value that's 300 bytes with identifiable content
		valueBytes := make([]byte, 300)
		for i := range valueBytes {
			valueBytes[i] = byte(i)
		}
		value := string(valueBytes)
		script := buildScript(
			[]byte{opNameUpdate},
			pushData([]byte(name)),
			pushData([]byte(value)),
		)

		op, parsedName, parsedValue, err := parseNameScript(script)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if op != namedb.NameUpdate {
			t.Errorf("expected NameUpdate, got %v", op)
		}
		if parsedName != name {
			t.Errorf("expected name %q, got %q", name, parsedName)
		}
		if parsedValue != value {
			t.Errorf("expected value to match, got length %d vs %d", len(parsedValue), len(value))
		}
	})
}

func TestReadPushData(t *testing.T) {
	t.Run("OP_0 (empty push)", func(t *testing.T) {
		// OP_0 is opcode 0x00, which pushes an empty byte array
		script := []byte{0x00}

		result, offset, err := readPushData(script, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty result, got length %d", len(result))
		}
		if offset != 1 {
			t.Errorf("expected offset 1, got %d", offset)
		}
	})

	t.Run("direct push", func(t *testing.T) {
		data := []byte("hello")
		script := pushData(data)

		result, offset, err := readPushData(script, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(result) != string(data) {
			t.Errorf("expected %q, got %q", data, result)
		}
		if offset != len(script) {
			t.Errorf("expected offset %d, got %d", len(script), offset)
		}
	})

	t.Run("OP_PUSHDATA1", func(t *testing.T) {
		data := make([]byte, 100)
		for i := range data {
			data[i] = byte(i)
		}
		script := pushData(data)

		result, offset, err := readPushData(script, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != len(data) {
			t.Errorf("expected length %d, got %d", len(data), len(result))
		}
		// Verify actual content matches
		for i := range data {
			if result[i] != data[i] {
				t.Errorf("data mismatch at index %d: expected 0x%02x, got 0x%02x", i, data[i], result[i])
				break
			}
		}
		if offset != len(script) {
			t.Errorf("expected offset %d, got %d", len(script), offset)
		}
	})

	t.Run("OP_PUSHDATA2", func(t *testing.T) {
		data := make([]byte, 300)
		for i := range data {
			data[i] = byte(i)
		}
		script := pushData(data)

		result, offset, err := readPushData(script, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != len(data) {
			t.Errorf("expected length %d, got %d", len(data), len(result))
		}
		// Verify actual content matches
		for i := range data {
			if result[i] != data[i] {
				t.Errorf("data mismatch at index %d: expected 0x%02x, got 0x%02x", i, data[i], result[i])
				break
			}
		}
		if offset != len(script) {
			t.Errorf("expected offset %d, got %d", len(script), offset)
		}
	})

	t.Run("offset beyond script", func(t *testing.T) {
		script := []byte{0x01, 0x00}
		_, _, err := readPushData(script, 10)
		if err == nil {
			t.Error("expected error for offset beyond script")
		}
	})

	t.Run("truncated data", func(t *testing.T) {
		// Says 5 bytes but only provides 2
		script := []byte{0x05, 0x01, 0x02}
		_, _, err := readPushData(script, 0)
		if err == nil {
			t.Error("expected error for truncated data")
		}
	})
}

func TestValidateNameFormat(t *testing.T) {
	testCases := []struct {
		name      string
		inputName string
		value     string
		wantErr   bool
	}{
		{
			name:      "valid name and value",
			inputName: "d/example",
			value:     `{"ip":"1.2.3.4"}`,
			wantErr:   false,
		},
		{
			name:      "empty name",
			inputName: "",
			value:     "test",
			wantErr:   true,
		},
		{
			name:      "name too long",
			inputName: string(make([]byte, 256)),
			value:     "test",
			wantErr:   true,
		},
		{
			name:      "value too long",
			inputName: "d/test",
			value:     string(make([]byte, 1024)),
			wantErr:   true,
		},
		{
			name:      "max valid name length",
			inputName: string(make([]byte, 255)),
			value:     "test",
			wantErr:   false,
		},
		{
			name:      "max valid value length",
			inputName: "d/test",
			value:     string(make([]byte, 1023)),
			wantErr:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNameFormat(tc.inputName, tc.value)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestComputeCommitHash(t *testing.T) {
	// Test that commitment hash is computed consistently
	rand := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}
	name := "d/example"

	hash1 := computeCommitHash(rand, name)
	hash2 := computeCommitHash(rand, name)

	// Same inputs should produce same output
	if len(hash1) != len(hash2) {
		t.Errorf("Hash lengths differ: %d vs %d", len(hash1), len(hash2))
	}
	for i := range hash1 {
		if hash1[i] != hash2[i] {
			t.Errorf("Hash mismatch at byte %d", i)
			break
		}
	}

	// Hash should be 20 bytes (RIPEMD160 output)
	if len(hash1) != 20 {
		t.Errorf("Expected 20-byte hash, got %d bytes", len(hash1))
	}
}

func TestComputeCommitHashDifferentInputs(t *testing.T) {
	rand := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}

	// Different names should produce different hashes
	hash1 := computeCommitHash(rand, "d/name1")
	hash2 := computeCommitHash(rand, "d/name2")

	same := true
	for i := range hash1 {
		if hash1[i] != hash2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("Different names should produce different hashes")
	}

	// Different rands should produce different hashes
	rand2 := []byte{0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
		0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0,
		0xef, 0xee, 0xed, 0xec}
	hash3 := computeCommitHash(rand2, "d/name1")

	same = true
	for i := range hash1 {
		if hash1[i] != hash3[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("Different rands should produce different hashes")
	}
}

func TestParseNameScriptFull_NameNew(t *testing.T) {
	// NAME_NEW should return the commitment hash as extra data
	hash := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}

	script := buildScript(
		[]byte{opNameNew},
		pushData(hash),
	)

	op, name, value, extra, err := parseNameScriptFull(script)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op != namedb.NameNew {
		t.Errorf("expected NameNew, got %v", op)
	}
	if name != "" {
		t.Errorf("expected empty name, got %q", name)
	}
	if value != "" {
		t.Errorf("expected empty value, got %q", value)
	}
	if len(extra) != len(hash) {
		t.Errorf("expected %d bytes in extra, got %d", len(hash), len(extra))
	}
	for i := range hash {
		if extra[i] != hash[i] {
			t.Errorf("extra data mismatch at byte %d", i)
			break
		}
	}
}

func TestParseNameScriptFull_NameFirstUpdate(t *testing.T) {
	// NAME_FIRSTUPDATE should return the rand as extra data
	name := "d/example"
	rand := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}
	value := `{"ip":"1.2.3.4"}`

	script := buildScript(
		[]byte{opNameFirstUpdate},
		pushData([]byte(name)),
		pushData(rand),
		pushData([]byte(value)),
	)

	op, parsedName, parsedValue, extra, err := parseNameScriptFull(script)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op != namedb.NameFirstUpdate {
		t.Errorf("expected NameFirstUpdate, got %v", op)
	}
	if parsedName != name {
		t.Errorf("expected name %q, got %q", name, parsedName)
	}
	if parsedValue != value {
		t.Errorf("expected value %q, got %q", value, parsedValue)
	}
	if len(extra) != len(rand) {
		t.Errorf("expected %d bytes in extra, got %d", len(rand), len(extra))
	}
	for i := range rand {
		if extra[i] != rand[i] {
			t.Errorf("extra data mismatch at byte %d", i)
			break
		}
	}
}

func TestParseNameScriptFull_NameUpdate(t *testing.T) {
	// NAME_UPDATE should have nil extra data
	name := "d/example"
	value := `{"ip":"5.6.7.8"}`

	script := buildScript(
		[]byte{opNameUpdate},
		pushData([]byte(name)),
		pushData([]byte(value)),
	)

	op, parsedName, parsedValue, extra, err := parseNameScriptFull(script)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op != namedb.NameUpdate {
		t.Errorf("expected NameUpdate, got %v", op)
	}
	if parsedName != name {
		t.Errorf("expected name %q, got %q", name, parsedName)
	}
	if parsedValue != value {
		t.Errorf("expected value %q, got %q", value, parsedValue)
	}
	if extra != nil {
		t.Errorf("expected nil extra data for NameUpdate, got %v", extra)
	}
}
