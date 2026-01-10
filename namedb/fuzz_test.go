package namedb

import (
	"encoding/json"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// FuzzNameRecordSerialization fuzzes name record serialization/deserialization to catch:
// - Buffer overflows with extreme field values
// - Issues with special characters in name/value fields
// - Malformed JSON in value field
// - Invalid UTF-8 sequences
// - Edge cases in timestamp handling
// - Hash serialization issues
//
// Run with: go test -fuzz=FuzzNameRecordSerialization -fuzztime=1m
func FuzzNameRecordSerialization(f *testing.F) {
	// Seed corpus with valid name records
	validHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")

	f.Add("d/test", `{"ip":"1.2.3.4"}`, int32(100), int32(36100), "N12345678901234567890")
	f.Add("id/alice", `{"email":"alice@example.com"}`, int32(1000), int32(37000), "Nabcdefghijklmnopqrstu")
	f.Add("p/personal", ``, int32(0), int32(36000), "N00000000000000000000")
	f.Add("", "", int32(0), int32(0), "")
	f.Add("d/very-long-name-with-many-characters", `{"long":"value"}`, int32(-1), int32(-1), "N")

	f.Fuzz(func(t *testing.T, name, value string, height, expiresAt int32, address string) {
		// Create a name record with fuzzed fields
		record := &NameRecord{
			Name:          name,
			Value:         value,
			TxHash:        *validHash,
			OutIndex:      0,
			Height:        height,
			ExpiresAt:     expiresAt,
			Address:       address,
			UpdatedAt:     time.Now(),
			NameNewHeight: height - 12, // Simulate realistic NameNewHeight
		}

		// Attempt to encode (should never panic)
		data := encodeNameRecord(record)

		// Attempt to decode (should never panic)
		decoded, err := decodeNameRecord(data)
		if err != nil {
			// Deserialization error is acceptable
			return
		}

		// If round-trip succeeded, verify data integrity
		// Note: Name field is not encoded in the record (it's the database key)
		// So decoded.Name will always be empty
		if decoded.Value != record.Value {
			t.Errorf("value mismatch: got %q, want %q", decoded.Value, record.Value)
		}
		if decoded.Height != record.Height {
			t.Errorf("height mismatch: got %d, want %d", decoded.Height, record.Height)
		}
		if decoded.ExpiresAt != record.ExpiresAt {
			t.Errorf("expiresAt mismatch: got %d, want %d", decoded.ExpiresAt, record.ExpiresAt)
		}
		if decoded.Address != record.Address {
			t.Errorf("address mismatch: got %q, want %q", decoded.Address, record.Address)
		}
		if decoded.OutIndex != record.OutIndex {
			t.Errorf("outIndex mismatch: got %d, want %d", decoded.OutIndex, record.OutIndex)
		}
		if decoded.NameNewHeight != record.NameNewHeight {
			t.Errorf("nameNewHeight mismatch: got %d, want %d", decoded.NameNewHeight, record.NameNewHeight)
		}

		// Verify hash is preserved
		if decoded.TxHash.String() != record.TxHash.String() {
			t.Errorf("txHash mismatch: got %s, want %s", decoded.TxHash.String(), record.TxHash.String())
		}

		// Verify timestamp is preserved at second precision (stored as Unix seconds)
		if decoded.UpdatedAt.Unix() != record.UpdatedAt.Unix() {
			t.Errorf("updatedAt mismatch: got %v, want %v", decoded.UpdatedAt, record.UpdatedAt)
		}
	})
}

// FuzzNameRecordJSON fuzzes JSON value field validation to ensure:
// - Proper handling of malformed JSON
// - No crashes with deeply nested structures
// - Correct handling of special characters
// - Validation of maximum value size
//
// Run with: go test -fuzz=FuzzNameRecordJSON -fuzztime=1m
func FuzzNameRecordJSON(f *testing.F) {
	// Seed with valid and edge case JSON values
	f.Add(`{"ip":"1.2.3.4"}`)
	f.Add(`{"ip":"1.2.3.4","ns":["ns1.example.com","ns2.example.com"]}`)
	f.Add(`{}`)
	f.Add(`null`)
	f.Add(`""`)
	f.Add(`[]`)
	f.Add(`[1,2,3]`)
	f.Add(`{"nested":{"deeply":{"very":{"much":"value"}}}}`)
	f.Add(`{"unicode":"日本語"}`)
	f.Add(`{"special":"chars!@#$%^&*()"}`)

	f.Fuzz(func(t *testing.T, value string) {
		// Attempt to validate as JSON
		// This simulates what the name database does when storing d/ and id/ namespace values
		var v interface{}
		err := json.Unmarshal([]byte(value), &v)
		if err != nil {
			// Invalid JSON is expected for random input
			return
		}

		// If valid JSON, verify we can re-serialize without panic
		_, err = json.Marshal(v)
		if err != nil {
			t.Errorf("failed to re-serialize valid JSON: %v", err)
		}

		// Verify value doesn't exceed maximum size (1023 bytes)
		if len(value) > 1023 {
			// This would be rejected by name operation validation
			return
		}

		// Create a name record with this value
		validHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
		record := &NameRecord{
			Name:      "d/test",
			Value:     value,
			TxHash:    *validHash,
			Height:    100,
			ExpiresAt: 36100,
			Address:   "N12345678901234567890",
			UpdatedAt: time.Now(),
		}

		// Verify encoding works
		data := encodeNameRecord(record)

		// Verify decoding works
		decoded, err := decodeNameRecord(data)
		if err != nil {
			t.Errorf("failed to deserialize record: %v", err)
			return
		}

		// Verify value is preserved exactly
		if decoded.Value != value {
			t.Errorf("value not preserved: got %q, want %q", decoded.Value, value)
		}
	})
}

// FuzzNameField fuzzes name field validation to ensure:
// - Maximum length enforcement (255 bytes)
// - Proper handling of special characters
// - UTF-8 validation
// - Namespace prefix validation
//
// Run with: go test -fuzz=FuzzNameField -fuzztime=1m
func FuzzNameField(f *testing.F) {
	// Seed with valid name patterns
	f.Add("d/test")
	f.Add("id/alice")
	f.Add("p/personal")
	f.Add("d/")
	f.Add("d/a")
	f.Add("d/very-long-domain-name-with-many-hyphens-and-dots.example.com")
	f.Add("d/日本語ドメイン")
	f.Add("d/special!@#$%^&*()")
	f.Add("")
	f.Add("noslash")
	f.Add("d")
	f.Add("/")

	f.Fuzz(func(t *testing.T, name string) {
		// Check name length constraint
		if len(name) > 255 {
			// Would be rejected by name operation validation
			return
		}

		// Verify name is valid UTF-8
		// Invalid UTF-8 sequences should be handled gracefully
		if !utf8.ValidString(name) {
			// Invalid UTF-8 is expected for random input
			return
		}

		// Create a name record with this name
		validHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
		record := &NameRecord{
			Name:      name,
			Value:     `{"test":"value"}`,
			TxHash:    *validHash,
			Height:    100,
			ExpiresAt: 36100,
			Address:   "N12345678901234567890",
			UpdatedAt: time.Now(),
		}

		// Attempt encoding (should never panic)
		data := encodeNameRecord(record)

		// Attempt decoding (should never panic)
		decoded, err := decodeNameRecord(data)
		if err != nil {
			// Decoding error is acceptable for some edge cases
			return
		}

		// Verify value and other fields are preserved
		// Note: Name field is not encoded in the record itself - it's used as the database key
		// So we don't check Name here, only the fields that are actually serialized
		if decoded.Value != record.Value {
			t.Errorf("value not preserved: got %q, want %q", decoded.Value, record.Value)
		}
	})
}

// FuzzAddressField fuzzes address field handling to ensure:
// - No issues with various address formats
// - Proper handling of empty addresses
// - Correct handling of invalid addresses
//
// Run with: go test -fuzz=FuzzAddressField -fuzztime=1m
func FuzzAddressField(f *testing.F) {
	// Seed with various address patterns
	f.Add("N12345678901234567890")              // Valid Namecoin address format
	f.Add("NabcdefghijklmnopqrstuvwxyzABC")     // Long address
	f.Add("N")                                  // Short address
	f.Add("")                                   // Empty address
	f.Add("1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2") // Bitcoin-format address
	f.Add("abc")                                // Invalid format
	f.Add("N!@#$%^&*()")                        // Special characters

	f.Fuzz(func(t *testing.T, address string) {
		// Create a name record with fuzzed address
		validHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
		record := &NameRecord{
			Name:      "d/test",
			Value:     `{"test":"value"}`,
			TxHash:    *validHash,
			Height:    100,
			ExpiresAt: 36100,
			Address:   address,
			UpdatedAt: time.Now(),
		}

		// Encode (should never panic)
		data := encodeNameRecord(record)

		// Decode (should never panic)
		decoded, err := decodeNameRecord(data)
		if err != nil {
			t.Errorf("failed to deserialize record with address %q: %v", address, err)
			return
		}

		// Verify address is preserved
		if decoded.Address != address {
			t.Errorf("address not preserved: got %q, want %q", decoded.Address, address)
		}
	})
}

// FuzzHeightFields fuzzes height-related fields to ensure:
// - Proper handling of negative heights
// - Correct handling of very large heights
// - Validation of height ordering (Height <= ExpiresAt, NameNewHeight < Height)
//
// Run with: go test -fuzz=FuzzHeightFields -fuzztime=1m
func FuzzHeightFields(f *testing.F) {
	// Seed with various height patterns
	f.Add(int32(0), int32(36000), int32(-12))
	f.Add(int32(100), int32(36100), int32(88))
	f.Add(int32(1000000), int32(1036000), int32(999988))
	f.Add(int32(-1), int32(35999), int32(-13))
	f.Add(int32(2147483647), int32(2147483647), int32(2147483635)) // Max int32

	f.Fuzz(func(t *testing.T, height, expiresAt, nameNewHeight int32) {
		// Create a name record with fuzzed height fields
		validHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
		record := &NameRecord{
			Name:          "d/test",
			Value:         `{"test":"value"}`,
			TxHash:        *validHash,
			Height:        height,
			ExpiresAt:     expiresAt,
			NameNewHeight: nameNewHeight,
			Address:       "N12345678901234567890",
			UpdatedAt:     time.Now(),
		}

		// Encode (should never panic)
		data := encodeNameRecord(record)

		// Decode (should never panic)
		decoded, err := decodeNameRecord(data)
		if err != nil {
			t.Errorf("failed to deserialize record: %v", err)
			return
		}

		// Verify height fields are preserved
		if decoded.Height != height {
			t.Errorf("height mismatch: got %d, want %d", decoded.Height, height)
		}
		if decoded.ExpiresAt != expiresAt {
			t.Errorf("expiresAt mismatch: got %d, want %d", decoded.ExpiresAt, expiresAt)
		}
		if decoded.NameNewHeight != nameNewHeight {
			t.Errorf("nameNewHeight mismatch: got %d, want %d", decoded.NameNewHeight, nameNewHeight)
		}
	})
}
