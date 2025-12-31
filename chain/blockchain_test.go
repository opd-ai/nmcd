package chain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/config"
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

// makeP2PKHScript creates a standard P2PKH script for testing.
// Returns a 25-byte P2PKH script: OP_DUP OP_HASH160 <20 bytes> OP_EQUALVERIFY OP_CHECKSIG
func makeP2PKHScript() []byte {
	return []byte{
		0x76, 0xa9, 0x14, // OP_DUP OP_HASH160 OP_PUSHDATA(20)
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a,
		0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14, // 20 bytes of hash
		0x88, 0xac, // OP_EQUALVERIFY OP_CHECKSIG
	}
}

// buildNameNewScript creates a complete, valid NAME_NEW script.
// Format: OP_NAME_NEW <hash> OP_2DROP <P2PKH>
func buildNameNewScript(hash []byte) []byte {
	return buildScript(
		[]byte{opNameNew},
		pushData(hash),
		[]byte{op2Drop},    // Required OP_2DROP
		makeP2PKHScript(),  // Required P2PKH suffix
	)
}

// buildNameFirstUpdateScript creates a complete, valid NAME_FIRSTUPDATE script.
// Format: OP_NAME_FIRSTUPDATE <name> <rand> <value> OP_2DROP OP_2DROP <P2PKH>
func buildNameFirstUpdateScript(name, rand, value []byte) []byte {
	return buildScript(
		[]byte{opNameFirstUpdate},
		pushData(name),
		pushData(rand),
		pushData(value),
		[]byte{op2Drop},    // Required first OP_2DROP
		[]byte{op2Drop},    // Required second OP_2DROP
		makeP2PKHScript(),  // Required P2PKH suffix
	)
}

// buildNameUpdateScript creates a complete, valid NAME_UPDATE script.
// Format: OP_NAME_UPDATE <name> <value> OP_2DROP OP_DROP <P2PKH>
func buildNameUpdateScript(name, value []byte) []byte {
	return buildScript(
		[]byte{opNameUpdate},
		pushData(name),
		pushData(value),
		[]byte{op2Drop},    // Required OP_2DROP
		[]byte{opDrop},     // Required OP_DROP
		makeP2PKHScript(),  // Required P2PKH suffix
	)
}

func TestParseNameScript_NameNew(t *testing.T) {
	// NAME_NEW: OP_NAME_NEW <hash> OP_2DROP <P2PKH>
	// The hash is typically 20 bytes
	hash := make([]byte, 20)
	for i := range hash {
		hash[i] = byte(i)
	}

	script := buildScript(
		[]byte{opNameNew},
		pushData(hash),
		[]byte{op2Drop},    // Required OP_2DROP
		makeP2PKHScript(),  // Required P2PKH suffix
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
	// NAME_FIRSTUPDATE: OP_NAME_FIRSTUPDATE <name> <rand> <value> OP_2DROP OP_2DROP <P2PKH>
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
				[]byte{op2Drop},    // Required first OP_2DROP
				[]byte{op2Drop},    // Required second OP_2DROP
				makeP2PKHScript(),  // Required P2PKH suffix
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
	// NAME_UPDATE: OP_NAME_UPDATE <name> <value> OP_2DROP OP_DROP <P2PKH>
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
				[]byte{op2Drop},    // Required OP_2DROP
				[]byte{opDrop},     // Required OP_DROP
				makeP2PKHScript(),  // Required P2PKH suffix
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
			[]byte{op2Drop},    // Required OP_2DROP
			[]byte{opDrop},     // Required OP_DROP
			makeP2PKHScript(),  // Required P2PKH suffix
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
			[]byte{op2Drop},    // Required OP_2DROP
			[]byte{opDrop},     // Required OP_DROP
			makeP2PKHScript(),  // Required P2PKH suffix
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
			[]byte{op2Drop},    // Required OP_2DROP
			[]byte{opDrop},     // Required OP_DROP
			makeP2PKHScript(),  // Required P2PKH suffix
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
			name:      "max valid name length with namespace",
			inputName: "d/" + string(make([]byte, 253)), // d/ (2 bytes) + 253 bytes = 255 total (MaxNameLength)
			value:     `{"ip":"1.2.3.4"}`,               // Valid JSON for d/ namespace
			wantErr:   false,
		},
		{
			name:      "max valid value length",
			inputName: "d/test",
			value:     `{"data":"` + strings.Repeat("x", 1000) + `"}`, // Valid JSON close to max length
			wantErr:   false,
		},
		{
			name:      "invalid namespace - no prefix",
			inputName: "example",
			value:     "test",
			wantErr:   true,
		},
		{
			name:      "invalid namespace - wrong prefix",
			inputName: "x/example",
			value:     "test",
			wantErr:   true,
		},
		{
			name:      "valid id namespace",
			inputName: "id/johndoe",
			value:     `{"email":"john@example.com"}`,
			wantErr:   false,
		},
		{
			name:      "valid p namespace",
			inputName: "p/alice",
			value:     "personal data",
			wantErr:   false,
		},
		{
			name:      "namespace only - d/ with no content",
			inputName: "d/",
			value:     "test",
			wantErr:   true,
		},
		{
			name:      "namespace only - id/ with no content",
			inputName: "id/",
			value:     "test",
			wantErr:   true,
		},
		{
			name:      "namespace only - p/ with no content",
			inputName: "p/",
			value:     "test",
			wantErr:   true,
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
	chainParams := &config.NamecoinRegTestParams

	hash1 := computeCommitHash(rand, name, chainParams)
	hash2 := computeCommitHash(rand, name, chainParams)

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
	chainParams := &config.NamecoinRegTestParams

	// Different names should produce different hashes
	hash1 := computeCommitHash(rand, "d/name1", chainParams)
	hash2 := computeCommitHash(rand, "d/name2", chainParams)

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
	hash3 := computeCommitHash(rand2, "d/name1", chainParams)

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

// TestComputeCommitHashCrossChainReplay tests that commitment hashes
// are different across different Namecoin networks (mainnet/testnet/regtest)
// to prevent cross-chain replay attacks
func TestComputeCommitHashCrossChainReplay(t *testing.T) {
	rand := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}
	name := "d/example"

	// Compute commitment hash for the same name and rand on different networks
	mainnetHash := computeCommitHash(rand, name, &config.NamecoinMainNetParams)
	testnetHash := computeCommitHash(rand, name, &config.NamecoinTestNetParams)
	regtestHash := computeCommitHash(rand, name, &config.NamecoinRegTestParams)

	// Hashes should be different for different networks
	// This prevents a NAME_NEW commitment from being replayed across networks
	if len(mainnetHash) != 20 || len(testnetHash) != 20 || len(regtestHash) != 20 {
		t.Errorf("All hashes should be 20 bytes: mainnet=%d, testnet=%d, regtest=%d",
			len(mainnetHash), len(testnetHash), len(regtestHash))
	}

	// Check mainnet vs testnet
	if equalBytes(mainnetHash, testnetHash) {
		t.Error("Mainnet and testnet should produce different commitment hashes (replay attack vulnerability)")
	}

	// Check mainnet vs regtest
	if equalBytes(mainnetHash, regtestHash) {
		t.Error("Mainnet and regtest should produce different commitment hashes (replay attack vulnerability)")
	}

	// Check testnet vs regtest
	if equalBytes(testnetHash, regtestHash) {
		t.Error("Testnet and regtest should produce different commitment hashes (replay attack vulnerability)")
	}
}

// equalBytes compares two byte slices for equality
func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestNameFirstUpdateCrossNetworkValidation tests that a NAME_FIRSTUPDATE
// created for one network (e.g., mainnet) will fail validation on another
// network (e.g., testnet) due to mismatched commitment hashes
func TestNameFirstUpdateCrossNetworkValidation(t *testing.T) {
	// Setup: Create NAME_NEW on mainnet
	mainnetDBPath := filepath.Join(t.TempDir(), "mainnet-db.db")
	mainnetDB, err := namedb.NewNameDatabase(mainnetDBPath)
	if err != nil {
		t.Fatalf("Failed to create mainnet database: %v", err)
	}
	defer mainnetDB.Close()

	mainnetBC := &BlockChain{
		nameDB:      mainnetDB,
		chainParams: &config.NamecoinMainNetParams,
	}

	// Create NAME_NEW commitment on mainnet
	nameStr := "d/cross-network-test"
	rand := []byte{0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6, 0xa7, 0xa8,
		0xa9, 0xaa, 0xab, 0xac, 0xad, 0xae, 0xaf, 0xb0,
		0xb1, 0xb2, 0xb3, 0xb4}
	mainnetCommitHash := computeCommitHash(rand, nameStr, mainnetBC.chainParams)

	if err := mainnetDB.PutNameNew(mainnetCommitHash, 100); err != nil {
		t.Fatalf("Failed to store NAME_NEW on mainnet: %v", err)
	}

	// Setup: Create BlockChain for testnet
	testnetDBPath := filepath.Join(t.TempDir(), "testnet-db.db")
	testnetDB, err := namedb.NewNameDatabase(testnetDBPath)
	if err != nil {
		t.Fatalf("Failed to create testnet database: %v", err)
	}
	defer testnetDB.Close()

	testnetBC := &BlockChain{
		nameDB:      testnetDB,
		chainParams: &config.NamecoinTestNetParams,
	}

	// Try to use the same NAME_NEW commitment on testnet
	// This simulates a replay attack attempt
	testnetCommitHash := computeCommitHash(rand, nameStr, testnetBC.chainParams)

	// The commitment hashes should be different due to chain ID
	if equalBytes(mainnetCommitHash, testnetCommitHash) {
		t.Fatal("Commitment hashes should differ between mainnet and testnet")
	}

	// Store the mainnet commitment hash on testnet DB (simulating the replay)
	if err := testnetDB.PutNameNew(mainnetCommitHash, 100); err != nil {
		t.Fatalf("Failed to store NAME_NEW on testnet: %v", err)
	}

	// Create a NAME_FIRSTUPDATE transaction using the mainnet commitment
	value := `{"ip":"1.2.3.4"}`
	script := buildNameFirstUpdateScript([]byte(nameStr), rand, []byte(value))

	msgBlock := wire.NewMsgBlock(&wire.BlockHeader{})
	tx := wire.NewMsgTx(1)
	tx.AddTxOut(wire.NewTxOut(config.DustLimit, script))
	msgBlock.AddTransaction(tx)

	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(113) // After MinBlocksBeforeFirstUpdate

	// Validate on testnet - should FAIL because commitment hash doesn't match
	// The blockchain computes testnetCommitHash but database has mainnetCommitHash
	err = testnetBC.validateNameOperations(block)

	if err == nil {
		t.Error("Expected validation to fail for cross-network replay attempt, but it succeeded")
	}

	if err != nil && !strings.Contains(err.Error(), "no matching name_new found") {
		t.Logf("Got expected error (may vary): %v", err)
	}
}

func TestParseNameScriptFull_NameNew(t *testing.T) {
	// NAME_NEW should return the commitment hash as extra data
	hash := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}

	script := buildNameNewScript(hash)

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

	script := buildNameFirstUpdateScript([]byte(name), rand, []byte(value))

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

	script := buildNameUpdateScript([]byte(name), []byte(value))

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

// TestRollbackNameNew tests that NAME_NEW operations are properly rolled back
// when a block is disconnected during a blockchain reorg.
func TestRollbackNameNew(t *testing.T) {
	// Create temporary database
	dbPath := filepath.Join(os.TempDir(), "test-rollback-namenew.db")
	defer os.Remove(dbPath)

	ndb, err := namedb.NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	// Create a BlockChain wrapper with just the nameDB (no btcd blockchain)
	bc := &BlockChain{
		nameDB: ndb,
	}

	// Create a NAME_NEW commitment
	commitHash := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}
	height := int32(100)

	// Add the NAME_NEW to the database
	if err := ndb.PutNameNew(commitHash, height); err != nil {
		t.Fatalf("Failed to put name_new: %v", err)
	}

	// Verify it exists
	if _, err := ndb.GetNameNew(commitHash); err != nil {
		t.Fatalf("Expected name_new to exist: %v", err)
	}

	// Create a block with the NAME_NEW
	msgBlock := wire.NewMsgBlock(&wire.BlockHeader{})
	tx := wire.NewMsgTx(1)
	script := buildNameNewScript(commitHash)
	tx.AddTxOut(wire.NewTxOut(config.DustLimit, script))
	msgBlock.AddTransaction(tx)

	// Create btcutil.Block wrapper
	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(height)

	// Rollback the block
	bc.rollbackNameOperations(block)

	// Verify the NAME_NEW was removed
	if _, err := ndb.GetNameNew(commitHash); err == nil {
		t.Error("Expected name_new to be deleted after rollback, but it still exists")
	}
}

// TestRollbackNameFirstUpdate tests that NAME_FIRSTUPDATE operations are properly
// rolled back when a block is disconnected during a blockchain reorg.
func TestRollbackNameFirstUpdate(t *testing.T) {
	// Create temporary database
	dbPath := filepath.Join(os.TempDir(), "test-rollback-namefirstupdate.db")
	defer os.Remove(dbPath)

	ndb, err := namedb.NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	// Create a BlockChain wrapper with just the nameDB
	bc := &BlockChain{
		nameDB:      ndb,
		chainParams: &config.NamecoinRegTestParams,
	}

	// Create a name record (as if NAME_FIRSTUPDATE was processed)
	nameStr := "d/example"
	value := `{"ip":"1.2.3.4"}`
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	rand := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}
	height := int32(112) // After MinBlocksBeforeFirstUpdate

	record := &namedb.NameRecord{
		Name:      nameStr,
		Value:     value,
		TxHash:    *txHash,
		OutIndex:  0,
		Height:    height,
		ExpiresAt: height + 36000,
		Address:   "N1234567890",
		UpdatedAt: time.Now(),
	}

	// Put the name record
	if err := ndb.PutName(nameStr, record); err != nil {
		t.Fatalf("Failed to put name: %v", err)
	}

	// Add history entry
	if err := ndb.AddHistory(*txHash, record); err != nil {
		t.Fatalf("Failed to add history: %v", err)
	}

	// Verify the name exists
	if _, err := ndb.GetName(nameStr); err != nil {
		t.Fatalf("Expected name to exist: %v", err)
	}

	// Create a block with the NAME_FIRSTUPDATE
	msgBlock := wire.NewMsgBlock(&wire.BlockHeader{})
	tx := wire.NewMsgTx(1)
	script := buildNameFirstUpdateScript([]byte(nameStr), rand, []byte(value))
	tx.AddTxOut(wire.NewTxOut(config.DustLimit, script))
	msgBlock.AddTransaction(tx)

	// Create btcutil.Block wrapper
	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(height)

	// Rollback the block
	bc.rollbackNameOperations(block)

	// Verify the name was removed
	if _, err := ndb.GetName(nameStr); err == nil {
		t.Error("Expected name to be deleted after rollback, but it still exists")
	}

	// Verify history was removed
	history, err := ndb.GetHistory(nameStr)
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("Expected 0 history entries after rollback, got %d", len(history))
	}

	// Verify that the NAME_NEW commitment was restored
	// The commit hash is computed from rand, name, and chain ID
	commitHash := computeCommitHash(rand, nameStr, bc.chainParams)
	restoredNameNew, err := ndb.GetNameNew(commitHash)
	if err != nil {
		t.Fatalf("Expected NAME_NEW to be restored after rollback: %v", err)
	}
	// The restored height should be estimated as block height - MinBlocksBeforeFirstUpdate
	expectedHeight := height - 12 // MinBlocksBeforeFirstUpdate = 12
	if restoredNameNew.Height != expectedHeight {
		t.Errorf("Expected restored NAME_NEW height %d, got %d", expectedHeight, restoredNameNew.Height)
	}
}

// TestRollbackNameUpdate tests that NAME_UPDATE operations are properly rolled back
// when a block is disconnected, restoring the previous value from history.
func TestRollbackNameUpdate(t *testing.T) {
	// Create temporary database
	dbPath := filepath.Join(os.TempDir(), "test-rollback-nameupdate.db")
	defer os.Remove(dbPath)

	ndb, err := namedb.NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	// Create a BlockChain wrapper with just the nameDB
	bc := &BlockChain{
		nameDB: ndb,
	}

	nameStr := "d/example"
	originalValue := `{"ip":"1.2.3.4"}`
	updatedValue := `{"ip":"5.6.7.8"}`
	txHash1, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	txHash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000002")
	originalHeight := int32(100)
	updateHeight := int32(200)

	// Create the original record (from NAME_FIRSTUPDATE)
	originalRecord := &namedb.NameRecord{
		Name:      nameStr,
		Value:     originalValue,
		TxHash:    *txHash1,
		OutIndex:  0,
		Height:    originalHeight,
		ExpiresAt: originalHeight + 36000,
		Address:   "N1234567890",
		UpdatedAt: time.Now(),
	}

	// Add original history entry
	if err := ndb.AddHistory(*txHash1, originalRecord); err != nil {
		t.Fatalf("Failed to add original history: %v", err)
	}

	// Create the updated record (from NAME_UPDATE)
	updatedRecord := &namedb.NameRecord{
		Name:      nameStr,
		Value:     updatedValue,
		TxHash:    *txHash2,
		OutIndex:  0,
		Height:    updateHeight,
		ExpiresAt: updateHeight + 36000,
		Address:   "N1234567890",
		UpdatedAt: time.Now(),
	}

	// Put the updated name record (this is the current state)
	if err := ndb.PutName(nameStr, updatedRecord); err != nil {
		t.Fatalf("Failed to put updated name: %v", err)
	}

	// Add updated history entry
	if err := ndb.AddHistory(*txHash2, updatedRecord); err != nil {
		t.Fatalf("Failed to add updated history: %v", err)
	}

	// Verify the current value is the updated one
	current, err := ndb.GetName(nameStr)
	if err != nil {
		t.Fatalf("Expected name to exist: %v", err)
	}
	if current.Value != updatedValue {
		t.Errorf("Expected current value %q, got %q", updatedValue, current.Value)
	}

	// Create a block with the NAME_UPDATE
	msgBlock := wire.NewMsgBlock(&wire.BlockHeader{})
	tx := wire.NewMsgTx(1)
	script := buildNameUpdateScript([]byte(nameStr), []byte(updatedValue))
	tx.AddTxOut(wire.NewTxOut(config.DustLimit, script))
	msgBlock.AddTransaction(tx)

	// Create btcutil.Block wrapper
	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(updateHeight)

	// Rollback the block
	bc.rollbackNameOperations(block)

	// Verify the value was restored to the original
	restored, err := ndb.GetName(nameStr)
	if err != nil {
		t.Fatalf("Expected name to still exist after rollback: %v", err)
	}
	if restored.Value != originalValue {
		t.Errorf("Expected restored value %q, got %q", originalValue, restored.Value)
	}
	if restored.Height != originalHeight {
		t.Errorf("Expected restored height %d, got %d", originalHeight, restored.Height)
	}
	// Verify history has only the original entry
	history, err := ndb.GetHistory(nameStr)
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	if len(history) != 1 {
		t.Errorf("Expected 1 history entry after rollback, got %d", len(history))
	}
}

// TestRollbackSameBlockNameNewAndFirstUpdate tests the edge case where both
// NAME_NEW and NAME_FIRSTUPDATE are in the same block. When rolling back,
// the NAME_FIRSTUPDATE restores the NAME_NEW commitment, and then the NAME_NEW
// rollback should NOT delete it (since it was just restored).
func TestRollbackSameBlockNameNewAndFirstUpdate(t *testing.T) {
	// Create temporary database
	dbPath := filepath.Join(os.TempDir(), "test-rollback-sameblock.db")
	defer os.Remove(dbPath)

	ndb, err := namedb.NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	// Create a BlockChain wrapper with just the nameDB
	bc := &BlockChain{
		nameDB:      ndb,
		chainParams: &config.NamecoinRegTestParams,
	}

	// Set up scenario: Both NAME_NEW and NAME_FIRSTUPDATE in same block
	nameStr := "d/example"
	value := `{"ip":"1.2.3.4"}`
	txHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
	rand := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}
	height := int32(100)

	// Compute the commitment hash that links NAME_NEW to NAME_FIRSTUPDATE
	commitHash := computeCommitHash(rand, nameStr, bc.chainParams)

	// Simulate the state after the block was processed:
	// - NAME_NEW commitment was consumed (deleted)
	// - Name was registered via NAME_FIRSTUPDATE
	record := &namedb.NameRecord{
		Name:      nameStr,
		Value:     value,
		TxHash:    *txHash,
		OutIndex:  0,
		Height:    height,
		ExpiresAt: height + 36000,
		Address:   "N1234567890",
		UpdatedAt: time.Now(),
	}
	if err := ndb.PutName(nameStr, record); err != nil {
		t.Fatalf("Failed to put name: %v", err)
	}
	if err := ndb.AddHistory(*txHash, record); err != nil {
		t.Fatalf("Failed to add history: %v", err)
	}

	// Create a block with both NAME_NEW and NAME_FIRSTUPDATE
	// NAME_NEW comes first (earlier in block), NAME_FIRSTUPDATE comes later
	msgBlock := wire.NewMsgBlock(&wire.BlockHeader{})

	// First transaction: NAME_NEW
	tx1 := wire.NewMsgTx(1)
	nameNewScript := buildNameNewScript(commitHash) // The commitment hash
	tx1.AddTxOut(wire.NewTxOut(config.DustLimit, nameNewScript))
	msgBlock.AddTransaction(tx1)

	// Second transaction: NAME_FIRSTUPDATE that consumes the NAME_NEW
	tx2 := wire.NewMsgTx(1)
	firstUpdateScript := buildNameFirstUpdateScript([]byte(nameStr), rand, []byte(value))
	tx2.AddTxOut(wire.NewTxOut(config.DustLimit, firstUpdateScript))
	msgBlock.AddTransaction(tx2)

	// Create btcutil.Block wrapper
	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(height)

	// Rollback the block
	bc.rollbackNameOperations(block)

	// Verify the name was removed
	if _, err := ndb.GetName(nameStr); err == nil {
		t.Error("Expected name to be deleted after rollback, but it still exists")
	}

	// Verify history was removed
	history, err := ndb.GetHistory(nameStr)
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("Expected 0 history entries after rollback, got %d", len(history))
	}

	// KEY TEST: The NAME_NEW commitment should still exist!
	// Even though we rolled back the NAME_NEW (which would normally delete it),
	// it should be preserved because the NAME_FIRSTUPDATE rollback restored it.
	restoredNameNew, err := ndb.GetNameNew(commitHash)
	if err != nil {
		t.Fatalf("Expected NAME_NEW commitment to exist after rollback of same-block NAME_NEW + NAME_FIRSTUPDATE: %v", err)
	}

	// The height should be the estimated value
	expectedHeight := height - 12 // MinBlocksBeforeFirstUpdate = 12
	if restoredNameNew.Height != expectedHeight {
		t.Errorf("Expected restored NAME_NEW height %d, got %d", expectedHeight, restoredNameNew.Height)
	}
}

// TestExtractAddressFromNameScript tests address extraction from name scripts.
func TestExtractAddressFromNameScript(t *testing.T) {
	// Use Namecoin mainnet params for testing
	chainParams := &config.NamecoinMainNetParams

	// Create a test pubkey hash (20 bytes)
	pubKeyHash := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a,
		0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14,
	}

	// Build a P2PKH script: OP_DUP OP_HASH160 <pubkeyhash> OP_EQUALVERIFY OP_CHECKSIG
	p2pkhScript := buildScript(
		[]byte{0x76}, // OP_DUP
		[]byte{0xa9}, // OP_HASH160
		[]byte{0x14}, // Push 20 bytes
		pubKeyHash,   // pubkeyhash
		[]byte{0x88}, // OP_EQUALVERIFY
		[]byte{0xac}, // OP_CHECKSIG
	)

	t.Run("NAME_FIRSTUPDATE with P2PKH", func(t *testing.T) {
		nameBytes := []byte("d/example")
		rand := make([]byte, 20)
		valueBytes := []byte(`{"ip":"1.2.3.4"}`)

		// Build NAME_FIRSTUPDATE script:
		// OP_NAME_FIRSTUPDATE <name> <rand> <value> OP_2DROP OP_2DROP <P2PKH>
		script := buildScript(
			[]byte{opNameFirstUpdate},
			pushData(nameBytes),
			pushData(rand),
			pushData(valueBytes),
			[]byte{0x6d}, // OP_2DROP
			[]byte{0x6d}, // OP_2DROP
			p2pkhScript,
		)

		address := extractAddressFromNameScript(script, chainParams)
		if address == "" {
			t.Error("Expected non-empty address from NAME_FIRSTUPDATE script")
		}
	})

	t.Run("NAME_UPDATE with P2PKH", func(t *testing.T) {
		nameBytes := []byte("d/example")
		valueBytes := []byte(`{"ip":"5.6.7.8"}`)

		// Build NAME_UPDATE script:
		// OP_NAME_UPDATE <name> <value> OP_2DROP OP_DROP <P2PKH>
		script := buildScript(
			[]byte{opNameUpdate},
			pushData(nameBytes),
			pushData(valueBytes),
			[]byte{0x6d}, // OP_2DROP
			[]byte{0x75}, // OP_DROP
			p2pkhScript,
		)

		address := extractAddressFromNameScript(script, chainParams)
		if address == "" {
			t.Error("Expected non-empty address from NAME_UPDATE script")
		}
	})

	t.Run("NAME_NEW with P2PKH", func(t *testing.T) {
		commitHash := make([]byte, 20)

		// Build NAME_NEW script:
		// OP_NAME_NEW <hash> OP_2DROP <P2PKH>
		script := buildScript(
			[]byte{opNameNew},
			pushData(commitHash),
			[]byte{0x6d}, // OP_2DROP
			p2pkhScript,
		)

		address := extractAddressFromNameScript(script, chainParams)
		if address == "" {
			t.Error("Expected non-empty address from NAME_NEW script")
		}
	})

	t.Run("empty script returns empty address", func(t *testing.T) {
		address := extractAddressFromNameScript([]byte{}, chainParams)
		if address != "" {
			t.Errorf("Expected empty address for empty script, got %q", address)
		}
	})

	t.Run("nil chainParams returns empty address", func(t *testing.T) {
		script := buildScript(
			[]byte{opNameUpdate},
			pushData([]byte("d/test")),
			pushData([]byte("value")),
			[]byte{0x6d, 0x75},
			p2pkhScript,
		)
		address := extractAddressFromNameScript(script, nil)
		if address != "" {
			t.Errorf("Expected empty address for nil chainParams, got %q", address)
		}
	})

	t.Run("script without P2PKH returns empty address", func(t *testing.T) {
		// Script with name operation but no valid P2PKH portion
		script := buildScript(
			[]byte{opNameUpdate},
			pushData([]byte("d/test")),
			pushData([]byte("value")),
			[]byte{0x6d, 0x75},
			// Invalid/incomplete P2PKH
			[]byte{0x76, 0xa9},
		)
		address := extractAddressFromNameScript(script, chainParams)
		if address != "" {
			t.Errorf("Expected empty address for invalid P2PKH, got %q", address)
		}
	})

	t.Run("non-name operation returns empty address", func(t *testing.T) {
		// Just a standard P2PKH script (not a name operation)
		address := extractAddressFromNameScript(p2pkhScript, chainParams)
		if address != "" {
			t.Errorf("Expected empty address for non-name script, got %q", address)
		}
	})
}

// TestNameFirstUpdateTimingWindow tests the validation of NAME_FIRSTUPDATE timing window.
// Per Namecoin protocol, NAME_FIRSTUPDATE must occur between 12 and 36,000 blocks
// after NAME_NEW, otherwise the commitment is either too early or expired.
func TestNameFirstUpdateTimingWindow(t *testing.T) {
	// Create temporary database
	dbPath := filepath.Join(t.TempDir(), "test-timing-window.db")
	ndb, err := namedb.NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	// Create a BlockChain wrapper with just the nameDB for testing
	bc := &BlockChain{
		nameDB:      ndb,
		chainParams: &config.NamecoinRegTestParams,
	}

	// Setup: Create a NAME_NEW commitment in the database
	nameStr := "d/example"
	rand := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}
	commitHash := computeCommitHash(rand, nameStr, bc.chainParams)
	nameNewHeight := int32(100)

	// Store NAME_NEW commitment
	if err := ndb.PutNameNew(commitHash, nameNewHeight); err != nil {
		t.Fatalf("Failed to store NAME_NEW: %v", err)
	}

	// Test cases for timing window validation
	testCases := []struct {
		name              string
		firstUpdateHeight int32
		wantErr           bool
		errContains       string
	}{
		{
			name:              "too early - 0 blocks after NAME_NEW",
			firstUpdateHeight: nameNewHeight, // Same block
			wantErr:           true,
			errContains:       "name_firstupdate too early",
		},
		{
			name:              "too early - 1 block after NAME_NEW",
			firstUpdateHeight: nameNewHeight + 1,
			wantErr:           true,
			errContains:       "name_firstupdate too early",
		},
		{
			name:              "too early - 11 blocks after NAME_NEW (just below minimum)",
			firstUpdateHeight: nameNewHeight + 11,
			wantErr:           true,
			errContains:       "name_firstupdate too early",
		},
		{
			name:              "valid - exactly at minimum (12 blocks)",
			firstUpdateHeight: nameNewHeight + config.MinBlocksBeforeFirstUpdate,
			wantErr:           false,
		},
		{
			name:              "valid - 100 blocks after NAME_NEW",
			firstUpdateHeight: nameNewHeight + 100,
			wantErr:           false,
		},
		{
			name:              "valid - 1000 blocks after NAME_NEW",
			firstUpdateHeight: nameNewHeight + 1000,
			wantErr:           false,
		},
		{
			name:              "valid - exactly at maximum (36,000 blocks)",
			firstUpdateHeight: nameNewHeight + config.MaxBlocksBeforeFirstUpdate,
			wantErr:           false,
		},
		{
			name:              "too late - 36,001 blocks (just above maximum)",
			firstUpdateHeight: nameNewHeight + config.MaxBlocksBeforeFirstUpdate + 1,
			wantErr:           true,
			errContains:       "name_firstupdate too late",
		},
		{
			name:              "too late - 50,000 blocks after NAME_NEW",
			firstUpdateHeight: nameNewHeight + 50000,
			wantErr:           true,
			errContains:       "name_firstupdate too late",
		},
		{
			name:              "too late - 100,000 blocks after NAME_NEW",
			firstUpdateHeight: nameNewHeight + 100000,
			wantErr:           true,
			errContains:       "name_firstupdate too late",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a block with NAME_FIRSTUPDATE at the specified height
			block := createBlockWithNameFirstUpdate(t, nameStr, rand, tc.firstUpdateHeight)

			// Validate name operations
			err := bc.validateNameOperations(block)

			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tc.errContains)
				} else if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("expected error containing %q, got %q", tc.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// createBlockWithNameFirstUpdate creates a test block containing a NAME_FIRSTUPDATE operation
func createBlockWithNameFirstUpdate(t *testing.T, name string, rand []byte, height int32) *btcutil.Block {
	t.Helper()

	value := `{"ip":"1.2.3.4"}`

	// Build NAME_FIRSTUPDATE script
	script := buildScript(
		[]byte{opNameFirstUpdate},
		pushData([]byte(name)),
		pushData(rand),
		pushData([]byte(value)),
		// NAME_FIRSTUPDATE pushes 4 items onto the stack:
		// 1. The opcode result/status (from OP_NAME_FIRSTUPDATE itself)
		// 2. name
		// 3. rand
		// 4. value
		// OP_2DROP OP_2DROP removes all 4 items, leaving a clean stack for P2PKH
		[]byte{0x6d, 0x6d}, // OP_2DROP OP_2DROP
		// Add minimal P2PKH suffix for valid script
		[]byte{0x76, 0xa9, 0x14}, // OP_DUP OP_HASH160 OP_PUSHDATA(20)
		make([]byte, 20),         // 20-byte pubkey hash
		[]byte{0x88, 0xac},       // OP_EQUALVERIFY OP_CHECKSIG
	)

	// Create transaction with NAME_FIRSTUPDATE
	tx := wire.NewMsgTx(1)
	tx.AddTxOut(&wire.TxOut{
		Value:    config.DustLimit, // Use dust limit (546 satoshis) for valid transaction
		PkScript: script,
	})

	// Create block
	block := wire.NewMsgBlock(&wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
		Bits:      0x207fffff, // Regtest difficulty
	})
	block.AddTransaction(tx)

	utilBlock := btcutil.NewBlock(block)
	utilBlock.SetHeight(height)

	return utilBlock
}

// TestNameOperationDustLimitValidation tests that name operations enforce
// the dust limit (546 satoshis) to prevent spam and uneconomical UTXOs.
// This implements Issue #4 from PROTOCOL_COMPLIANCE_AUDIT.md.
func TestNameOperationDustLimitValidation(t *testing.T) {
	// Create temporary database
	dbPath := filepath.Join(t.TempDir(), "test-dust-limit.db")
	ndb, err := namedb.NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	// Create a BlockChain wrapper with nameDB for testing
	bc := &BlockChain{
		nameDB:      ndb,
		chainParams: &config.NamecoinRegTestParams,
	}

	// Helper function to create a block with a name operation at specified output value
	createBlockWithValue := func(opType namedb.NameOperation, outputValue int64, height int32) *btcutil.Block {
		var script []byte
		nameStr := "d/example"
		value := `{"ip":"1.2.3.4"}`
		rand := make([]byte, 20)

		switch opType {
		case namedb.NameNew:
			commitHash := make([]byte, 20)
			script = buildNameNewScript(commitHash)
		case namedb.NameFirstUpdate:
			script = buildNameFirstUpdateScript([]byte(nameStr), rand, []byte(value))
		case namedb.NameUpdate:
			script = buildNameUpdateScript([]byte(nameStr), []byte(value))
		}

		tx := wire.NewMsgTx(1)
		tx.AddTxOut(&wire.TxOut{
			Value:    outputValue,
			PkScript: script,
		})

		block := wire.NewMsgBlock(&wire.BlockHeader{
			Version:   1,
			Timestamp: time.Now(),
			Bits:      0x207fffff,
		})
		block.AddTransaction(tx)

		utilBlock := btcutil.NewBlock(block)
		utilBlock.SetHeight(height)
		return utilBlock
	}

	t.Run("NAME_NEW with dust limit validation", func(t *testing.T) {
		testCases := []struct {
			name        string
			outputValue int64
			wantErr     bool
			errContains string
		}{
			{
				name:        "below dust limit - 0 satoshis",
				outputValue: 0,
				wantErr:     true,
				errContains: "below dust limit",
			},
			{
				name:        "below dust limit - 545 satoshis",
				outputValue: 545,
				wantErr:     true,
				errContains: "below dust limit",
			},
			{
				name:        "exactly at dust limit - 546 satoshis",
				outputValue: config.DustLimit,
				wantErr:     false,
			},
			{
				name:        "above dust limit - 547 satoshis",
				outputValue: 547,
				wantErr:     false,
			},
			{
				name:        "above dust limit - 1000 satoshis",
				outputValue: 1000,
				wantErr:     false,
			},
			{
				name:        "above dust limit - 100000 satoshis (0.001 NMC)",
				outputValue: 100000,
				wantErr:     false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				block := createBlockWithValue(namedb.NameNew, tc.outputValue, 100)
				err := bc.validateNameOperations(block)

				if tc.wantErr {
					if err == nil {
						t.Errorf("expected error containing %q, got nil", tc.errContains)
					} else if !strings.Contains(err.Error(), tc.errContains) {
						t.Errorf("expected error containing %q, got %q", tc.errContains, err.Error())
					}
				} else {
					if err != nil {
						t.Errorf("unexpected error: %v", err)
					}
				}
			})
		}
	})

	t.Run("NAME_FIRSTUPDATE with dust limit validation", func(t *testing.T) {
		// Setup: Create NAME_NEW commitment for NAME_FIRSTUPDATE to reference
		nameStr := "d/example"
		rand := make([]byte, 20)
		commitHash := computeCommitHash(rand, nameStr, bc.chainParams)
		nameNewHeight := int32(100)
		if err := ndb.PutNameNew(commitHash, nameNewHeight); err != nil {
			t.Fatalf("Failed to store NAME_NEW: %v", err)
		}

		testCases := []struct {
			name        string
			outputValue int64
			wantErr     bool
			errContains string
		}{
			{
				name:        "below dust limit - 0 satoshis",
				outputValue: 0,
				wantErr:     true,
				errContains: "below dust limit",
			},
			{
				name:        "below dust limit - 100 satoshis",
				outputValue: 100,
				wantErr:     true,
				errContains: "below dust limit",
			},
			{
				name:        "below dust limit - 545 satoshis",
				outputValue: 545,
				wantErr:     true,
				errContains: "below dust limit",
			},
			{
				name:        "exactly at dust limit - 546 satoshis",
				outputValue: config.DustLimit,
				wantErr:     false,
			},
			{
				name:        "above dust limit - 1000 satoshis",
				outputValue: 1000,
				wantErr:     false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// NAME_FIRSTUPDATE must be at least MinBlocksBeforeFirstUpdate after NAME_NEW
				firstUpdateHeight := nameNewHeight + config.MinBlocksBeforeFirstUpdate
				block := createBlockWithValue(namedb.NameFirstUpdate, tc.outputValue, firstUpdateHeight)
				err := bc.validateNameOperations(block)

				if tc.wantErr {
					if err == nil {
						t.Errorf("expected error containing %q, got nil", tc.errContains)
					} else if !strings.Contains(err.Error(), tc.errContains) {
						t.Errorf("expected error containing %q, got %q", tc.errContains, err.Error())
					}
				} else {
					if err != nil {
						t.Errorf("unexpected error: %v", err)
					}
				}
			})
		}
	})

	t.Run("NAME_UPDATE with dust limit validation", func(t *testing.T) {
		// Setup: Create existing name for NAME_UPDATE to reference
		nameStr := "d/updatetest"
		txHash, err := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
		if err != nil {
			t.Fatalf("Failed to create test hash: %v", err)
		}
		nameHeight := int32(100)
		outIdx := uint32(0)

		record := &namedb.NameRecord{
			Name:      nameStr,
			Value:     `{"ip":"1.2.3.4"}`,
			TxHash:    *txHash,
			OutIndex:  outIdx,
			Height:    nameHeight,
			ExpiresAt: nameHeight + config.NameExpirationBlocks,
			Address:   "N1234567890",
			UpdatedAt: time.Now(),
		}
		if err := ndb.PutName(nameStr, record); err != nil {
			t.Fatalf("Failed to create name for update test: %v", err)
		}

		testCases := []struct {
			name        string
			outputValue int64
			wantErr     bool
			errContains string
		}{
			{
				name:        "below dust limit - 0 satoshis",
				outputValue: 0,
				wantErr:     true,
				errContains: "below dust limit",
			},
			{
				name:        "below dust limit - 500 satoshis",
				outputValue: 500,
				wantErr:     true,
				errContains: "below dust limit",
			},
			{
				name:        "below dust limit - 545 satoshis",
				outputValue: 545,
				wantErr:     true,
				errContains: "below dust limit",
			},
			{
				name:        "exactly at dust limit - 546 satoshis",
				outputValue: config.DustLimit,
				wantErr:     false,
			},
			{
				name:        "above dust limit - 600 satoshis",
				outputValue: 600,
				wantErr:     false,
			},
			{
				name:        "above dust limit - 10000 satoshis",
				outputValue: 10000,
				wantErr:     false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Create NAME_UPDATE block with custom name for this test
				updateHeight := nameHeight + 50

				script := buildNameUpdateScript([]byte(nameStr), []byte(`{"ip":"5.6.7.8"}`))

				tx := wire.NewMsgTx(1)
				// Add input that spends the current name UTXO
				tx.AddTxIn(&wire.TxIn{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *txHash,
						Index: outIdx,
					},
					SignatureScript: []byte{}, // Empty for test
					Sequence:        0xffffffff,
				})
				tx.AddTxOut(&wire.TxOut{
					Value:    tc.outputValue,
					PkScript: script,
				})

				block := wire.NewMsgBlock(&wire.BlockHeader{
					Version:   1,
					Timestamp: time.Now(),
					Bits:      0x207fffff,
				})
				block.AddTransaction(tx)

				utilBlock := btcutil.NewBlock(block)
				utilBlock.SetHeight(updateHeight)

				err := bc.validateNameOperations(utilBlock)

				if tc.wantErr {
					if err == nil {
						t.Errorf("expected error containing %q, got nil", tc.errContains)
					} else if !strings.Contains(err.Error(), tc.errContains) {
						t.Errorf("expected error containing %q, got %q", tc.errContains, err.Error())
					}
				} else {
					if err != nil {
						t.Errorf("unexpected error: %v", err)
					}
				}
			})
		}
	})
}

// TestTransactionFeeValidation tests that name operations pay required minimum fees
func TestTransactionFeeValidation(t *testing.T) {
	// Create temporary database
	dbPath := filepath.Join(os.TempDir(), "test-fee-validation.db")
	defer os.Remove(dbPath)

	ndb, err := namedb.NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	// Create a BlockChain wrapper with just the nameDB
	bc := &BlockChain{
		nameDB:      ndb,
		chainParams: &config.NamecoinRegTestParams,
	}

	// Helper function to create a UTXO for spending
	createUTXO := func(value int64, height int32) (chainhash.Hash, uint32) {
		tx := wire.NewMsgTx(1)
		// Create a simple output script (P2PKH-like)
		script := []byte{0x76, 0xa9, 0x14}           // OP_DUP OP_HASH160 OP_PUSH(20)
		script = append(script, make([]byte, 20)...) // 20-byte pubkey hash
		script = append(script, 0x88, 0xac)          // OP_EQUALVERIFY OP_CHECKSIG
		tx.AddTxOut(&wire.TxOut{
			Value:    value,
			PkScript: script,
		})

		txHash := tx.TxHash()
		utxo := &namedb.UTXO{
			TxHash:   txHash,
			OutIndex: 0,
			Value:    value,
			Address:  "N1234567890",
			PkScript: script,
			Height:   height,
		}
		if err := bc.nameDB.AddUTXO(utxo); err != nil {
			t.Fatalf("Failed to add UTXO: %v", err)
		}
		return txHash, 0
	}

	t.Run("NAME_NEW fee validation", func(t *testing.T) {
		testCases := []struct {
			name        string
			inputValue  int64
			outputValue int64
			wantErr     bool
			errContains string
		}{
			{
				name:        "fee below minimum relay fee",
				inputValue:  1000,
				outputValue: 100, // Fee = 1000 - 100 = 900 (below MinRelayTxFee of 1000)
				wantErr:     true,
				errContains: "below minimum",
			},
			{
				name:        "fee exactly at minimum relay fee",
				inputValue:  2000,
				outputValue: 1000, // Fee = 2000 - 1000 = 1000 (exactly MinRelayTxFee)
				wantErr:     false,
			},
			{
				name:        "fee above minimum relay fee",
				inputValue:  5000,
				outputValue: 1000, // Fee = 5000 - 1000 = 4000 (above MinRelayTxFee)
				wantErr:     false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Create input UTXO
				txHashIn, outIdx := createUTXO(tc.inputValue, 100)

				// Create NAME_NEW transaction
				commitHash := make([]byte, 20)
				script := buildScript(
					[]byte{opNameNew},
					pushData(commitHash),
				)

				tx := wire.NewMsgTx(1)
				// Add input
				tx.AddTxIn(wire.NewTxIn(
					wire.NewOutPoint(&txHashIn, outIdx),
					nil,
					nil,
				))
				// Add NAME_NEW output
				tx.AddTxOut(&wire.TxOut{
					Value:    tc.outputValue,
					PkScript: script,
				})

				// Validate the transaction fee
				err := bc.validateTransactionFee(tx, namedb.NameNew, 150)

				if tc.wantErr {
					if err == nil {
						t.Errorf("expected error containing %q, got nil", tc.errContains)
					} else if !strings.Contains(err.Error(), tc.errContains) {
						t.Errorf("expected error containing %q, got %q", tc.errContains, err.Error())
					}
				} else {
					if err != nil {
						t.Errorf("unexpected error: %v", err)
					}
				}
			})
		}
	})

	t.Run("NAME_FIRSTUPDATE fee validation", func(t *testing.T) {
		// First, create a NAME_NEW commitment using proper hash calculation
		// Use unique rand to avoid collision with other tests
		nameStr := "d/fee-test-firstupdate"
		rand := []byte{0xf1, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf7, 0xf8,
			0xf9, 0xfa, 0xfb, 0xfc, 0xfd, 0xfe, 0xff, 0x00,
			0x01, 0x02, 0x03, 0x04}
		commitHash := computeCommitHash(rand, nameStr, bc.chainParams)
		if err := bc.nameDB.PutNameNew(commitHash, 100); err != nil {
			t.Fatalf("Failed to create NAME_NEW: %v", err)
		}

		testCases := []struct {
			name        string
			inputValue  int64
			outputValue int64
			wantErr     bool
			errContains string
		}{
			{
				name:        "fee well below minimum name operation fee",
				inputValue:  10000,
				outputValue: 9500, // Fee = 10000 - 9500 = 500 (well below 1,000,000)
				wantErr:     true,
				errContains: "below minimum",
			},
			{
				name:        "fee below minimum name operation fee",
				inputValue:  1500000,
				outputValue: 600000, // Fee = 1500000 - 600000 = 900,000 (below 1,000,000)
				wantErr:     true,
				errContains: "below minimum",
			},
			{
				name:        "fee exactly at minimum name operation fee",
				inputValue:  2000000,
				outputValue: 1000000, // Fee = 2000000 - 1000000 = 1,000,000 (exactly MinNameOperationFee)
				wantErr:     false,
			},
			{
				name:        "fee above minimum name operation fee",
				inputValue:  3000000,
				outputValue: 1000000, // Fee = 3000000 - 1000000 = 2,000,000 (above MinNameOperationFee)
				wantErr:     false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Create input UTXO
				txHashIn, outIdx := createUTXO(tc.inputValue, 100)

				// Create NAME_FIRSTUPDATE transaction using the name and rand from the commitment
				value := []byte(`{"ip":"1.2.3.4"}`)

				script := buildScript(
					[]byte{opNameFirstUpdate},
					pushData([]byte(nameStr)),
					pushData(rand),
					pushData(value),
				)

				tx := wire.NewMsgTx(1)
				// Add input
				tx.AddTxIn(wire.NewTxIn(
					wire.NewOutPoint(&txHashIn, outIdx),
					nil,
					nil,
				))
				// Add NAME_FIRSTUPDATE output
				tx.AddTxOut(&wire.TxOut{
					Value:    tc.outputValue,
					PkScript: script,
				})

				// Validate the transaction fee
				err := bc.validateTransactionFee(tx, namedb.NameFirstUpdate, 150)

				if tc.wantErr {
					if err == nil {
						t.Errorf("expected error containing %q, got nil", tc.errContains)
					} else if !strings.Contains(err.Error(), tc.errContains) {
						t.Errorf("expected error containing %q, got %q", tc.errContains, err.Error())
					}
				} else {
					if err != nil {
						t.Errorf("unexpected error: %v", err)
					}
				}
			})
		}
	})

	t.Run("NAME_UPDATE fee validation", func(t *testing.T) {
		// First, create a name in the database with a proper UTXO
		nameStr := "d/testupdate"
		// Create a UTXO that the name will reference
		nameUTXOTxHash, nameUTXOOutIdx := createUTXO(config.DustLimit, 100)
		nameHeight := int32(100)

		record := &namedb.NameRecord{
			Name:      nameStr,
			Value:     `{"ip":"1.2.3.4"}`,
			TxHash:    nameUTXOTxHash,
			OutIndex:  nameUTXOOutIdx,
			Height:    nameHeight,
			ExpiresAt: nameHeight + config.NameExpirationBlocks,
			Address:   "N1234567890",
			UpdatedAt: time.Now(),
		}
		if err := bc.nameDB.PutName(nameStr, record); err != nil {
			t.Fatalf("Failed to create name for update test: %v", err)
		}

		testCases := []struct {
			name        string
			inputValue  int64
			outputValue int64
			wantErr     bool
			errContains string
		}{
			{
				name:        "fee well below minimum name operation fee",
				inputValue:  10000,
				outputValue: 9500, // Fee = 10000 - 9500 = 500 (well below 1,000,000)
				wantErr:     true,
				errContains: "below minimum",
			},
			{
				name:        "fee below minimum name operation fee",
				inputValue:  1500000,
				outputValue: 600000, // Fee = 1500000 - 600000 = 900,000 (below 1,000,000)
				wantErr:     true,
				errContains: "below minimum",
			},
			{
				name:        "fee exactly at minimum name operation fee",
				inputValue:  2000000,
				outputValue: 1000000, // Fee = 2000000 - 1000000 = 1,000,000 (exactly MinNameOperationFee)
				wantErr:     false,
			},
			{
				name:        "fee above minimum name operation fee",
				inputValue:  3000000,
				outputValue: 1000000, // Fee = 3000000 - 1000000 = 2,000,000 (above MinNameOperationFee)
				wantErr:     false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Create input UTXO
				txHashIn, outIdx := createUTXO(tc.inputValue, 100)

				// Create NAME_UPDATE transaction
				script := buildScript(
					[]byte{opNameUpdate},
					pushData([]byte(nameStr)),
					pushData([]byte(`{"ip":"5.6.7.8"}`)),
				)

				tx := wire.NewMsgTx(1)
				// Add input
				tx.AddTxIn(wire.NewTxIn(
					wire.NewOutPoint(&txHashIn, outIdx),
					nil,
					nil,
				))
				// Add NAME_UPDATE output
				tx.AddTxOut(&wire.TxOut{
					Value:    tc.outputValue,
					PkScript: script,
				})

				// Validate the transaction fee
				err := bc.validateTransactionFee(tx, namedb.NameUpdate, 150)

				if tc.wantErr {
					if err == nil {
						t.Errorf("expected error containing %q, got nil", tc.errContains)
					} else if !strings.Contains(err.Error(), tc.errContains) {
						t.Errorf("expected error containing %q, got %q", tc.errContains, err.Error())
					}
				} else {
					if err != nil {
						t.Errorf("unexpected error: %v", err)
					}
				}
			})
		}
	})

	t.Run("negative fee validation", func(t *testing.T) {
		// Create input UTXO with low value
		txHashIn, outIdx := createUTXO(1000, 100)

		// Create transaction with output value greater than input (invalid)
		script := buildScript(
			[]byte{opNameNew},
			pushData(make([]byte, 20)),
		)

		tx := wire.NewMsgTx(1)
		tx.AddTxIn(wire.NewTxIn(
			wire.NewOutPoint(&txHashIn, outIdx),
			nil,
			nil,
		))
		tx.AddTxOut(&wire.TxOut{
			Value:    2000, // Greater than input
			PkScript: script,
		})

		err := bc.validateTransactionFee(tx, namedb.NameNew, 150)
		if err == nil {
			t.Error("expected error for negative fee, got nil")
		} else if !strings.Contains(err.Error(), "cannot be negative") {
			t.Errorf("expected error about negative fee, got: %v", err)
		}
	})

	t.Run("multiple inputs fee calculation", func(t *testing.T) {
		// Create multiple input UTXOs
		txHashIn1, outIdx1 := createUTXO(500000, 100)
		txHashIn2, outIdx2 := createUTXO(700000, 101)
		txHashIn3, outIdx3 := createUTXO(300000, 102)
		// Total inputs: 1,500,000 satoshis

		// Create NAME_FIRSTUPDATE transaction with multiple inputs
		nameBytes := []byte("d/multiinput")
		rand := make([]byte, 20)
		value := []byte(`{"ip":"1.2.3.4"}`)

		script := buildScript(
			[]byte{opNameFirstUpdate},
			pushData(nameBytes),
			pushData(rand),
			pushData(value),
		)

		tx := wire.NewMsgTx(1)
		// Add three inputs
		tx.AddTxIn(wire.NewTxIn(
			wire.NewOutPoint(&txHashIn1, outIdx1),
			nil,
			nil,
		))
		tx.AddTxIn(wire.NewTxIn(
			wire.NewOutPoint(&txHashIn2, outIdx2),
			nil,
			nil,
		))
		tx.AddTxIn(wire.NewTxIn(
			wire.NewOutPoint(&txHashIn3, outIdx3),
			nil,
			nil,
		))
		// Add NAME_FIRSTUPDATE output
		// Total inputs: 1,500,000
		// Output: 400,000
		// Fee: 1,100,000 (above minimum of 1,000,000)
		tx.AddTxOut(&wire.TxOut{
			Value:    400000,
			PkScript: script,
		})

		// First create NAME_NEW commitment
		commitHash := computeCommitHash(rand, string(nameBytes), bc.chainParams)
		if err := bc.nameDB.PutNameNew(commitHash, 100); err != nil {
			t.Fatalf("Failed to create NAME_NEW: %v", err)
		}

		// Validate the transaction fee - should pass with fee of 1,100,000
		err := bc.validateTransactionFee(tx, namedb.NameFirstUpdate, 150)
		if err != nil {
			t.Errorf("unexpected error with multiple inputs: %v", err)
		}

		// Now test with insufficient fee
		tx2 := wire.NewMsgTx(1)
		tx2.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&txHashIn1, outIdx1), nil, nil))
		tx2.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&txHashIn2, outIdx2), nil, nil))
		tx2.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&txHashIn3, outIdx3), nil, nil))
		// Output: 1,400,000 -> Fee: 100,000 (below minimum of 1,000,000)
		tx2.AddTxOut(&wire.TxOut{
			Value:    1400000,
			PkScript: script,
		})

		err = bc.validateTransactionFee(tx2, namedb.NameFirstUpdate, 150)
		if err == nil {
			t.Error("expected error for insufficient fee with multiple inputs, got nil")
		} else if !strings.Contains(err.Error(), "below minimum") {
			t.Errorf("expected error about insufficient fee, got: %v", err)
		}
	})
}

// TestValidateValueEncoding tests value encoding validation for different namespaces
func TestValidateValueEncoding(t *testing.T) {
	tests := []struct {
		name        string
		nameInput   string
		value       string
		expectError bool
		errorText   string
	}{
		// Empty value tests - should be allowed for all namespaces
		{
			name:        "empty value for d/ namespace",
			nameInput:   "d/example",
			value:       "",
			expectError: false,
		},
		{
			name:        "empty value for id/ namespace",
			nameInput:   "id/alice",
			value:       "",
			expectError: false,
		},
		{
			name:        "empty value for p/ namespace",
			nameInput:   "p/bob",
			value:       "",
			expectError: false,
		},

		// Valid JSON for d/ namespace (domain names)
		{
			name:        "valid JSON for d/ namespace - simple object",
			nameInput:   "d/example",
			value:       `{"ip":"1.2.3.4"}`,
			expectError: false,
		},
		{
			name:        "valid JSON for d/ namespace - complex DNS record",
			nameInput:   "d/bitcoin",
			value:       `{"ip":"10.0.0.1","ns":["ns1.example.com","ns2.example.com"],"email":"admin@example.com"}`,
			expectError: false,
		},
		{
			name:        "valid JSON for d/ namespace - array",
			nameInput:   "d/test",
			value:       `["value1","value2"]`,
			expectError: false,
		},
		{
			name:        "valid JSON for d/ namespace - string",
			nameInput:   "d/simple",
			value:       `"simple string value"`,
			expectError: false,
		},
		{
			name:        "valid JSON for d/ namespace - number",
			nameInput:   "d/number",
			value:       `42`,
			expectError: false,
		},
		{
			name:        "valid JSON for d/ namespace - boolean",
			nameInput:   "d/flag",
			value:       `true`,
			expectError: false,
		},
		{
			name:        "valid JSON for d/ namespace - null",
			nameInput:   "d/null",
			value:       `null`,
			expectError: false,
		},

		// Invalid JSON for d/ namespace
		{
			name:        "invalid JSON for d/ namespace - malformed object",
			nameInput:   "d/example",
			value:       `{"ip":"1.2.3.4"`,
			expectError: true,
			errorText:   "value must be valid JSON",
		},
		{
			name:        "invalid JSON for d/ namespace - plain text",
			nameInput:   "d/example",
			value:       `just plain text`,
			expectError: true,
			errorText:   "value must be valid JSON",
		},
		{
			name:        "invalid JSON for d/ namespace - incomplete array",
			nameInput:   "d/test",
			value:       `["incomplete"`,
			expectError: true,
			errorText:   "value must be valid JSON",
		},
		{
			name:        "invalid JSON for d/ namespace - trailing comma",
			nameInput:   "d/example",
			value:       `{"ip":"1.2.3.4",}`,
			expectError: true,
			errorText:   "value must be valid JSON",
		},

		// Valid JSON for id/ namespace (identity records)
		{
			name:        "valid JSON for id/ namespace - identity record",
			nameInput:   "id/alice",
			value:       `{"name":"Alice","email":"alice@example.com","pubkey":"abc123"}`,
			expectError: false,
		},
		{
			name:        "valid JSON for id/ namespace - simple object",
			nameInput:   "id/bob",
			value:       `{"profile":"https://example.com/bob"}`,
			expectError: false,
		},

		// Invalid JSON for id/ namespace
		{
			name:        "invalid JSON for id/ namespace - plain text",
			nameInput:   "id/charlie",
			value:       `not json content`,
			expectError: true,
			errorText:   "value must be valid JSON",
		},
		{
			name:        "invalid JSON for id/ namespace - malformed",
			nameInput:   "id/dave",
			value:       `{invalid}`,
			expectError: true,
			errorText:   "value must be valid JSON",
		},

		// p/ namespace - flexible format (UTF-8 text, JSON optional)
		{
			name:        "valid plain text for p/ namespace",
			nameInput:   "p/alice",
			value:       `This is plain text content`,
			expectError: false,
		},
		{
			name:        "valid JSON for p/ namespace",
			nameInput:   "p/bob",
			value:       `{"note":"personal note"}`,
			expectError: false,
		},
		{
			name:        "valid UTF-8 text with special chars for p/ namespace",
			nameInput:   "p/unicode",
			value:       `Hello 世界 🌍`,
			expectError: false,
		},
		{
			name:        "valid multiline text for p/ namespace",
			nameInput:   "p/diary",
			value:       "Line 1\nLine 2\nLine 3",
			expectError: false,
		},

		// Invalid UTF-8 tests for all namespaces
		{
			name:        "invalid UTF-8 for d/ namespace",
			nameInput:   "d/example",
			value:       string([]byte{0xff, 0xfe, 0xfd}),
			expectError: true,
			errorText:   "value must be valid UTF-8",
		},
		{
			name:        "invalid UTF-8 for id/ namespace",
			nameInput:   "id/test",
			value:       string([]byte{0x80, 0x81, 0x82}),
			expectError: true,
			errorText:   "value must be valid UTF-8",
		},
		{
			name:        "invalid UTF-8 for p/ namespace",
			nameInput:   "p/test",
			value:       string([]byte{0xc0, 0xc1}),
			expectError: true,
			errorText:   "value must be valid UTF-8",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateValueEncoding(tc.nameInput, tc.value)
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error containing '%s', got nil", tc.errorText)
				} else if !strings.Contains(err.Error(), tc.errorText) {
					t.Errorf("expected error containing '%s', got: %v", tc.errorText, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			}
		})
	}
}

// TestValidateNameFormat_WithValueEncoding tests the integrated validateNameFormat function
// including the new value encoding validation
func TestValidateNameFormat_WithValueEncoding(t *testing.T) {
	tests := []struct {
		name        string
		nameInput   string
		value       string
		expectError bool
		errorText   string
	}{
		// Valid cases
		{
			name:        "valid d/ namespace with JSON value",
			nameInput:   "d/example",
			value:       `{"ip":"1.2.3.4"}`,
			expectError: false,
		},
		{
			name:        "valid id/ namespace with JSON value",
			nameInput:   "id/alice",
			value:       `{"email":"alice@example.com"}`,
			expectError: false,
		},
		{
			name:        "valid p/ namespace with plain text",
			nameInput:   "p/bob",
			value:       "plain text is ok here",
			expectError: false,
		},
		{
			name:        "valid p/ namespace with JSON",
			nameInput:   "p/charlie",
			value:       `{"note":"JSON also works"}`,
			expectError: false,
		},

		// Invalid namespace
		{
			name:        "invalid namespace prefix",
			nameInput:   "x/test",
			value:       `{"data":"value"}`,
			expectError: true,
			errorText:   "invalid namespace",
		},

		// Invalid name length
		{
			name:        "name too long",
			nameInput:   "d/" + strings.Repeat("x", 254), // d/ (2) + 254 = 256 (exceeds MaxNameLength)
			value:       `{"ip":"1.2.3.4"}`,
			expectError: true,
			errorText:   "invalid name length",
		},
		{
			name:        "empty name",
			nameInput:   "",
			value:       `{"ip":"1.2.3.4"}`,
			expectError: true,
			errorText:   "invalid name length",
		},

		// Invalid value length
		{
			name:        "value too large",
			nameInput:   "d/example",
			value:       strings.Repeat("x", 1024), // exceeds MaxValueLength (1023)
			expectError: true,
			errorText:   "value too large",
		},

		// Invalid JSON for d/ namespace
		{
			name:        "d/ namespace with invalid JSON",
			nameInput:   "d/example",
			value:       "not valid json",
			expectError: true,
			errorText:   "value must be valid JSON",
		},

		// Invalid JSON for id/ namespace
		{
			name:        "id/ namespace with invalid JSON",
			nameInput:   "id/test",
			value:       "{broken json}",
			expectError: true,
			errorText:   "value must be valid JSON",
		},

		// Invalid UTF-8
		{
			name:        "invalid UTF-8 encoding",
			nameInput:   "p/test",
			value:       string([]byte{0xff, 0xfe}),
			expectError: true,
			errorText:   "value must be valid UTF-8",
		},

		// Edge cases
		{
			name:        "namespace prefix only - no content",
			nameInput:   "d/",
			value:       `{"ip":"1.2.3.4"}`,
			expectError: true,
			errorText:   "must have content after namespace prefix",
		},
		{
			name:        "max length name with valid value",
			nameInput:   "d/" + strings.Repeat("x", 253), // d/ (2) + 253 = 255 (MaxNameLength)
			value:       `{"ip":"1.2.3.4"}`,
			expectError: false,
		},
		{
			name:        "max length value",
			nameInput:   "d/example",
			value:       `{"data":"` + strings.Repeat("x", 1000) + `"}`, // construct JSON that's exactly at limit
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNameFormat(tc.nameInput, tc.value)
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error containing '%s', got nil", tc.errorText)
				} else if !strings.Contains(err.Error(), tc.errorText) {
					t.Errorf("expected error containing '%s', got: %v", tc.errorText, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			}
		})
	}
}

// TestNameUpdateUTXOChainValidation tests that NAME_UPDATE operations properly validate
// the UTXO chain to prevent name theft. This implements Issue #10 from PROTOCOL_COMPLIANCE_AUDIT.md.
func TestNameUpdateUTXOChainValidation(t *testing.T) {
	// Create temporary database
	dbPath := filepath.Join(t.TempDir(), "test-utxo-chain.db")
	ndb, err := namedb.NewNameDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer ndb.Close()

	// Create a BlockChain wrapper with nameDB for testing
	bc := &BlockChain{
		nameDB:      ndb,
		chainParams: &config.NamecoinRegTestParams,
	}

	nameStr := "d/securetest"
	nameValue := `{"ip":"1.2.3.4"}`
	updatedValue := `{"ip":"5.6.7.8"}`

	// Create the UTXO that currently owns the name
	currentTxHash, err := chainhash.NewHashFromStr("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("Failed to create test hash: %v", err)
	}
	currentOutIdx := uint32(1)
	nameHeight := int32(100)

	// Store the name record with its current UTXO
	record := &namedb.NameRecord{
		Name:      nameStr,
		Value:     nameValue,
		TxHash:    *currentTxHash,
		OutIndex:  currentOutIdx,
		Height:    nameHeight,
		ExpiresAt: nameHeight + config.NameExpirationBlocks,
		Address:   "N1234567890",
		UpdatedAt: time.Now(),
	}
	if err := ndb.PutName(nameStr, record); err != nil {
		t.Fatalf("Failed to create name: %v", err)
	}

	t.Run("valid NAME_UPDATE spending correct UTXO", func(t *testing.T) {
		// Create a transaction that properly spends the current name UTXO
		script := buildScript(
			[]byte{opNameUpdate},
			pushData([]byte(nameStr)),
			pushData([]byte(updatedValue)),
		)

		tx := wire.NewMsgTx(1)
		// Add input that spends the current name UTXO (correct hash and index)
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  *currentTxHash,
				Index: currentOutIdx,
			},
			SignatureScript: []byte{},
			Sequence:        0xffffffff,
		})
		tx.AddTxOut(&wire.TxOut{
			Value:    config.DustLimit,
			PkScript: script,
		})

		block := wire.NewMsgBlock(&wire.BlockHeader{
			Version:   1,
			Timestamp: time.Now(),
			Bits:      0x207fffff,
		})
		block.AddTransaction(tx)

		utilBlock := btcutil.NewBlock(block)
		utilBlock.SetHeight(nameHeight + 50)

		// This should pass validation
		err := bc.validateNameOperations(utilBlock)
		if err != nil {
			t.Errorf("expected no error for valid UTXO chain, got: %v", err)
		}
	})

	t.Run("invalid NAME_UPDATE - wrong transaction hash", func(t *testing.T) {
		// Create a transaction that spends a different transaction (name theft attempt)
		wrongTxHash, _ := chainhash.NewHashFromStr("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

		script := buildScript(
			[]byte{opNameUpdate},
			pushData([]byte(nameStr)),
			pushData([]byte(updatedValue)),
			[]byte{op2Drop},    // Required OP_2DROP
			[]byte{opDrop},     // Required OP_DROP
			makeP2PKHScript(),  // Required P2PKH suffix
		)

		tx := wire.NewMsgTx(1)
		// Add input with wrong transaction hash
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  *wrongTxHash,
				Index: currentOutIdx,
			},
			SignatureScript: []byte{},
			Sequence:        0xffffffff,
		})
		tx.AddTxOut(&wire.TxOut{
			Value:    config.DustLimit,
			PkScript: script,
		})

		block := wire.NewMsgBlock(&wire.BlockHeader{
			Version:   1,
			Timestamp: time.Now(),
			Bits:      0x207fffff,
		})
		block.AddTransaction(tx)

		utilBlock := btcutil.NewBlock(block)
		utilBlock.SetHeight(nameHeight + 50)

		// This should fail validation
		err := bc.validateNameOperations(utilBlock)
		if err == nil {
			t.Error("expected error for wrong transaction hash, got nil")
		} else if !strings.Contains(err.Error(), "does not spend current name UTXO") {
			t.Errorf("expected error about UTXO validation, got: %v", err)
		}
	})

	t.Run("invalid NAME_UPDATE - wrong output index", func(t *testing.T) {
		// Create a transaction that spends the same transaction but wrong output index
		wrongOutIdx := uint32(0) // Different from currentOutIdx (1)

		script := buildScript(
			[]byte{opNameUpdate},
			pushData([]byte(nameStr)),
			pushData([]byte(updatedValue)),
			[]byte{op2Drop},    // Required OP_2DROP
			[]byte{opDrop},     // Required OP_DROP
			makeP2PKHScript(),  // Required P2PKH suffix
		)

		tx := wire.NewMsgTx(1)
		// Add input with wrong output index
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  *currentTxHash,
				Index: wrongOutIdx,
			},
			SignatureScript: []byte{},
			Sequence:        0xffffffff,
		})
		tx.AddTxOut(&wire.TxOut{
			Value:    config.DustLimit,
			PkScript: script,
		})

		block := wire.NewMsgBlock(&wire.BlockHeader{
			Version:   1,
			Timestamp: time.Now(),
			Bits:      0x207fffff,
		})
		block.AddTransaction(tx)

		utilBlock := btcutil.NewBlock(block)
		utilBlock.SetHeight(nameHeight + 50)

		// This should fail validation
		err := bc.validateNameOperations(utilBlock)
		if err == nil {
			t.Error("expected error for wrong output index, got nil")
		} else if !strings.Contains(err.Error(), "does not spend current name UTXO") {
			t.Errorf("expected error about UTXO validation, got: %v", err)
		}
	})

	t.Run("invalid NAME_UPDATE - no inputs (theft attempt)", func(t *testing.T) {
		// Create a transaction with no inputs (trying to update without spending the UTXO)
		script := buildScript(
			[]byte{opNameUpdate},
			pushData([]byte(nameStr)),
			pushData([]byte(updatedValue)),
			[]byte{op2Drop},    // Required OP_2DROP
			[]byte{opDrop},     // Required OP_DROP
			makeP2PKHScript(),  // Required P2PKH suffix
		)

		tx := wire.NewMsgTx(1)
		// No inputs added - theft attempt
		tx.AddTxOut(&wire.TxOut{
			Value:    config.DustLimit,
			PkScript: script,
		})

		block := wire.NewMsgBlock(&wire.BlockHeader{
			Version:   1,
			Timestamp: time.Now(),
			Bits:      0x207fffff,
		})
		block.AddTransaction(tx)

		utilBlock := btcutil.NewBlock(block)
		utilBlock.SetHeight(nameHeight + 50)

		// This should fail validation
		err := bc.validateNameOperations(utilBlock)
		if err == nil {
			t.Error("expected error for missing inputs, got nil")
		} else if !strings.Contains(err.Error(), "does not spend current name UTXO") {
			t.Errorf("expected error about UTXO validation, got: %v", err)
		}
	})

	t.Run("valid NAME_UPDATE with multiple inputs - one spends name UTXO", func(t *testing.T) {
		// Create a transaction with multiple inputs, where one of them spends the name UTXO
		otherTxHash, _ := chainhash.NewHashFromStr("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")

		script := buildScript(
			[]byte{opNameUpdate},
			pushData([]byte(nameStr)),
			pushData([]byte(updatedValue)),
			[]byte{op2Drop},    // Required OP_2DROP
			[]byte{opDrop},     // Required OP_DROP
			makeP2PKHScript(),  // Required P2PKH suffix
		)

		tx := wire.NewMsgTx(1)
		// Add a different input first
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  *otherTxHash,
				Index: 0,
			},
			SignatureScript: []byte{},
			Sequence:        0xffffffff,
		})
		// Add the correct name UTXO input
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  *currentTxHash,
				Index: currentOutIdx,
			},
			SignatureScript: []byte{},
			Sequence:        0xffffffff,
		})
		tx.AddTxOut(&wire.TxOut{
			Value:    config.DustLimit,
			PkScript: script,
		})

		block := wire.NewMsgBlock(&wire.BlockHeader{
			Version:   1,
			Timestamp: time.Now(),
			Bits:      0x207fffff,
		})
		block.AddTransaction(tx)

		utilBlock := btcutil.NewBlock(block)
		utilBlock.SetHeight(nameHeight + 50)

		// This should pass validation - one of the inputs spends the correct UTXO
		err := bc.validateNameOperations(utilBlock)
		if err != nil {
			t.Errorf("expected no error for multiple inputs with correct UTXO, got: %v", err)
		}
	})
}

// TestStrictScriptValidation tests strict script format validation for name operations.
// This ensures compliance with Namecoin Core consensus rules by rejecting scripts
// with missing, extra, or malformed drop opcodes.
func TestStrictScriptValidation(t *testing.T) {
	// Standard P2PKH script for testing (25 bytes)
	// OP_DUP OP_HASH160 <20 bytes> OP_EQUALVERIFY OP_CHECKSIG
	p2pkhScript := []byte{
		0x76, 0xa9, 0x14, // OP_DUP OP_HASH160 OP_PUSHDATA(20)
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a,
		0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14, // 20 bytes of hash
		0x88, 0xac, // OP_EQUALVERIFY OP_CHECKSIG
	}

	t.Run("NAME_NEW valid format", func(t *testing.T) {
		hash := make([]byte, 20)
		script := buildScript(
			[]byte{opNameNew},
			pushData(hash),
			[]byte{op2Drop}, // Required OP_2DROP
			p2pkhScript,
		)

		op, _, _, _, err := parseNameScriptFull(script)
		if err != nil {
			t.Errorf("valid NAME_NEW script failed: %v", err)
		}
		if op != namedb.NameNew {
			t.Errorf("expected NameNew, got %v", op)
		}
	})

	t.Run("NAME_NEW missing OP_2DROP", func(t *testing.T) {
		hash := make([]byte, 20)
		script := buildScript(
			[]byte{opNameNew},
			pushData(hash),
			// Missing OP_2DROP
			p2pkhScript,
		)

		_, _, _, _, err := parseNameScriptFull(script)
		if err == nil {
			t.Error("expected error for NAME_NEW missing OP_2DROP, got nil")
		}
		if !strings.Contains(err.Error(), "missing required OP_2DROP") {
			t.Errorf("expected missing OP_2DROP error, got: %v", err)
		}
	})

	t.Run("NAME_NEW wrong drop opcode", func(t *testing.T) {
		hash := make([]byte, 20)
		script := buildScript(
			[]byte{opNameNew},
			pushData(hash),
			[]byte{opDrop}, // Wrong - should be OP_2DROP
			p2pkhScript,
		)

		_, _, _, _, err := parseNameScriptFull(script)
		if err == nil {
			t.Error("expected error for NAME_NEW with wrong drop opcode, got nil")
		}
		if !strings.Contains(err.Error(), "missing required OP_2DROP") {
			t.Errorf("expected missing OP_2DROP error, got: %v", err)
		}
	})

	t.Run("NAME_NEW missing P2PKH suffix", func(t *testing.T) {
		hash := make([]byte, 20)
		script := buildScript(
			[]byte{opNameNew},
			pushData(hash),
			[]byte{op2Drop},
			// Missing or too short P2PKH
		)

		_, _, _, _, err := parseNameScriptFull(script)
		if err == nil {
			t.Error("expected error for NAME_NEW missing P2PKH suffix, got nil")
		}
		if !strings.Contains(err.Error(), "P2PKH suffix too short") {
			t.Errorf("expected P2PKH suffix error, got: %v", err)
		}
	})

	t.Run("NAME_FIRSTUPDATE valid format", func(t *testing.T) {
		nameBytes := []byte("d/example")
		rand := make([]byte, 20)
		valueBytes := []byte(`{"ip":"1.2.3.4"}`)

		script := buildScript(
			[]byte{opNameFirstUpdate},
			pushData(nameBytes),
			pushData(rand),
			pushData(valueBytes),
			[]byte{op2Drop}, // Required first OP_2DROP
			[]byte{op2Drop}, // Required second OP_2DROP
			p2pkhScript,
		)

		op, name, value, _, err := parseNameScriptFull(script)
		if err != nil {
			t.Errorf("valid NAME_FIRSTUPDATE script failed: %v", err)
		}
		if op != namedb.NameFirstUpdate {
			t.Errorf("expected NameFirstUpdate, got %v", op)
		}
		if name != "d/example" {
			t.Errorf("expected name 'd/example', got %q", name)
		}
		if value != `{"ip":"1.2.3.4"}` {
			t.Errorf("expected value %q, got %q", `{"ip":"1.2.3.4"}`, value)
		}
	})

	t.Run("NAME_FIRSTUPDATE missing first OP_2DROP", func(t *testing.T) {
		nameBytes := []byte("d/example")
		rand := make([]byte, 20)
		valueBytes := []byte(`{"ip":"1.2.3.4"}`)

		script := buildScript(
			[]byte{opNameFirstUpdate},
			pushData(nameBytes),
			pushData(rand),
			pushData(valueBytes),
			// Missing first OP_2DROP
			[]byte{op2Drop}, // Only second OP_2DROP present (but in wrong position)
			p2pkhScript,
		)

		_, _, _, _, err := parseNameScriptFull(script)
		if err == nil {
			t.Error("expected error for NAME_FIRSTUPDATE missing first OP_2DROP, got nil")
		}
		// When first OP_2DROP is missing, the second one appears where the first should be,
		// so we detect the missing second OP_2DROP
		if !strings.Contains(err.Error(), "OP_2DROP") {
			t.Errorf("expected OP_2DROP error, got: %v", err)
		}
	})

	t.Run("NAME_FIRSTUPDATE missing second OP_2DROP", func(t *testing.T) {
		nameBytes := []byte("d/example")
		rand := make([]byte, 20)
		valueBytes := []byte(`{"ip":"1.2.3.4"}`)

		script := buildScript(
			[]byte{opNameFirstUpdate},
			pushData(nameBytes),
			pushData(rand),
			pushData(valueBytes),
			[]byte{op2Drop}, // First OP_2DROP present
			// Missing second OP_2DROP
			p2pkhScript,
		)

		_, _, _, _, err := parseNameScriptFull(script)
		if err == nil {
			t.Error("expected error for NAME_FIRSTUPDATE missing second OP_2DROP, got nil")
		}
		if !strings.Contains(err.Error(), "missing second OP_2DROP") {
			t.Errorf("expected missing second OP_2DROP error, got: %v", err)
		}
	})

	t.Run("NAME_FIRSTUPDATE missing both OP_2DROP opcodes", func(t *testing.T) {
		nameBytes := []byte("d/example")
		rand := make([]byte, 20)
		valueBytes := []byte(`{"ip":"1.2.3.4"}`)

		script := buildScript(
			[]byte{opNameFirstUpdate},
			pushData(nameBytes),
			pushData(rand),
			pushData(valueBytes),
			// Missing both OP_2DROP opcodes
			p2pkhScript,
		)

		_, _, _, _, err := parseNameScriptFull(script)
		if err == nil {
			t.Error("expected error for NAME_FIRSTUPDATE missing both OP_2DROP opcodes, got nil")
		}
		if !strings.Contains(err.Error(), "missing first OP_2DROP") {
			t.Errorf("expected missing first OP_2DROP error, got: %v", err)
		}
	})

	t.Run("NAME_UPDATE valid format", func(t *testing.T) {
		nameBytes := []byte("d/example")
		valueBytes := []byte(`{"ip":"5.6.7.8"}`)

		script := buildScript(
			[]byte{opNameUpdate},
			pushData(nameBytes),
			pushData(valueBytes),
			[]byte{op2Drop}, // Required OP_2DROP
			[]byte{opDrop},  // Required OP_DROP
			p2pkhScript,
		)

		op, name, value, _, err := parseNameScriptFull(script)
		if err != nil {
			t.Errorf("valid NAME_UPDATE script failed: %v", err)
		}
		if op != namedb.NameUpdate {
			t.Errorf("expected NameUpdate, got %v", op)
		}
		if name != "d/example" {
			t.Errorf("expected name 'd/example', got %q", name)
		}
		if value != `{"ip":"5.6.7.8"}` {
			t.Errorf("expected value %q, got %q", `{"ip":"5.6.7.8"}`, value)
		}
	})

	t.Run("NAME_UPDATE missing OP_2DROP", func(t *testing.T) {
		nameBytes := []byte("d/example")
		valueBytes := []byte(`{"ip":"5.6.7.8"}`)

		script := buildScript(
			[]byte{opNameUpdate},
			pushData(nameBytes),
			pushData(valueBytes),
			// Missing OP_2DROP
			[]byte{opDrop}, // Only OP_DROP present
			p2pkhScript,
		)

		_, _, _, _, err := parseNameScriptFull(script)
		if err == nil {
			t.Error("expected error for NAME_UPDATE missing OP_2DROP, got nil")
		}
		if !strings.Contains(err.Error(), "missing required OP_2DROP") {
			t.Errorf("expected missing OP_2DROP error, got: %v", err)
		}
	})

	t.Run("NAME_UPDATE missing OP_DROP", func(t *testing.T) {
		nameBytes := []byte("d/example")
		valueBytes := []byte(`{"ip":"5.6.7.8"}`)

		script := buildScript(
			[]byte{opNameUpdate},
			pushData(nameBytes),
			pushData(valueBytes),
			[]byte{op2Drop}, // OP_2DROP present
			// Missing OP_DROP
			p2pkhScript,
		)

		_, _, _, _, err := parseNameScriptFull(script)
		if err == nil {
			t.Error("expected error for NAME_UPDATE missing OP_DROP, got nil")
		}
		if !strings.Contains(err.Error(), "missing required OP_DROP") {
			t.Errorf("expected missing required OP_DROP error, got: %v", err)
		}
	})

	t.Run("NAME_UPDATE missing both drop opcodes", func(t *testing.T) {
		nameBytes := []byte("d/example")
		valueBytes := []byte(`{"ip":"5.6.7.8"}`)

		script := buildScript(
			[]byte{opNameUpdate},
			pushData(nameBytes),
			pushData(valueBytes),
			// Missing both OP_2DROP and OP_DROP
			p2pkhScript,
		)

		_, _, _, _, err := parseNameScriptFull(script)
		if err == nil {
			t.Error("expected error for NAME_UPDATE missing both drop opcodes, got nil")
		}
		if !strings.Contains(err.Error(), "missing required OP_2DROP") {
			t.Errorf("expected missing OP_2DROP error, got: %v", err)
		}
	})

	t.Run("NAME_UPDATE wrong order of drop opcodes", func(t *testing.T) {
		nameBytes := []byte("d/example")
		valueBytes := []byte(`{"ip":"5.6.7.8"}`)

		script := buildScript(
			[]byte{opNameUpdate},
			pushData(nameBytes),
			pushData(valueBytes),
			[]byte{opDrop},  // Wrong - should be OP_2DROP first
			[]byte{op2Drop}, // Wrong - should be OP_DROP second
			p2pkhScript,
		)

		_, _, _, _, err := parseNameScriptFull(script)
		if err == nil {
			t.Error("expected error for NAME_UPDATE with wrong drop opcode order, got nil")
		}
		if !strings.Contains(err.Error(), "missing required OP_2DROP") {
			t.Errorf("expected missing OP_2DROP error, got: %v", err)
		}
	})

	t.Run("script ending at drop opcodes without P2PKH", func(t *testing.T) {
		hash := make([]byte, 20)
		script := buildScript(
			[]byte{opNameNew},
			pushData(hash),
			[]byte{op2Drop},
			// P2PKH too short (less than 25 bytes)
			[]byte{0x76, 0xa9}, // Just 2 bytes
		)

		_, _, _, _, err := parseNameScriptFull(script)
		if err == nil {
			t.Error("expected error for script with insufficient P2PKH suffix, got nil")
		}
		if !strings.Contains(err.Error(), "P2PKH suffix too short") {
			t.Errorf("expected P2PKH suffix error, got: %v", err)
		}
	})

	t.Run("extra opcodes after drop opcodes should still work if P2PKH valid", func(t *testing.T) {
		// Extra opcodes in the P2PKH portion are allowed - we only validate minimum size
		hash := make([]byte, 20)
		script := buildScript(
			[]byte{opNameNew},
			pushData(hash),
			[]byte{op2Drop},
			p2pkhScript,
			[]byte{0x00, 0x00}, // Extra bytes after P2PKH - should be allowed
		)

		op, _, _, _, err := parseNameScriptFull(script)
		if err != nil {
			t.Errorf("script with extra bytes after valid P2PKH should parse: %v", err)
		}
		if op != namedb.NameNew {
			t.Errorf("expected NameNew, got %v", op)
		}
	})
}
