package chain

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/namedb"
)

// Namecoin-specific opcodes for name operations.
// These opcodes extend Bitcoin's script language for name management.
// See: https://github.com/namecoin/namecoin-core for reference.
const (
	// opNameNew is the opcode for NAME_NEW (pre-registration with hash commitment)
	// Script format: OP_NAME_NEW <hash> OP_2DROP <standard script>
	opNameNew = 0xd0

	// opNameFirstUpdate is the opcode for NAME_FIRSTUPDATE (first registration)
	// Script format: OP_NAME_FIRSTUPDATE <name> <rand> <value> OP_2DROP OP_2DROP <standard script>
	opNameFirstUpdate = 0xd1

	// opNameUpdate is the opcode for NAME_UPDATE (update existing name)
	// Script format: OP_NAME_UPDATE <name> <value> OP_2DROP OP_DROP <standard script>
	opNameUpdate = 0xd2

	// opPushData1 is the opcode for pushing 76-255 bytes
	opPushData1 = 0x4c

	// opPushData2 is the opcode for pushing 256-65535 bytes
	opPushData2 = 0x4d

	// opPushData4 is the opcode for pushing up to 4GB of data (rarely used)
	opPushData4 = 0x4e

	// opDrop removes the top stack item
	opDrop = 0x75

	// op2Drop removes the top two stack items
	op2Drop = 0x6d
)

// computeCommitHash computes the NAME_NEW commitment hash.
// The commitment is RIPEMD160(SHA256(rand || name)).
func computeCommitHash(rand []byte, name string) []byte {
	data := append(append([]byte(nil), rand...), name...)
	return btcutil.Hash160(data)
}

// parseNameScript extracts name operation from script.
// Namecoin scripts use Bitcoin's push data format with length-prefixed data.
// Returns the operation type, name, value, and any parsing error.
func parseNameScript(script []byte) (namedb.NameOperation, string, string, error) {
	op, name, value, _, err := parseNameScriptFull(script)
	return op, name, value, err
}

// validateScriptFormat validates the strict format of a Namecoin name operation script.
// This enforces consensus rules by ensuring drop opcodes and P2PKH suffix are correctly placed.
// Returns the offset after the drop opcodes where the P2PKH script begins, or an error if invalid.
//
// Expected formats per Namecoin Core:
//   - NAME_NEW: OP_NAME_NEW <hash> OP_2DROP <P2PKH>
//   - NAME_FIRSTUPDATE: OP_NAME_FIRSTUPDATE <name> <rand> <value> OP_2DROP OP_2DROP <P2PKH>
//   - NAME_UPDATE: OP_NAME_UPDATE <name> <value> OP_2DROP OP_DROP <P2PKH>
//
// The P2PKH suffix must be at least 25 bytes (standard P2PKH script size).
func validateScriptFormat(script []byte, opType namedb.NameOperation, dataEndOffset int) (int, error) {
	if dataEndOffset >= len(script) {
		return 0, fmt.Errorf("script ends after name operation data, missing drop opcodes")
	}

	offset := dataEndOffset

	switch opType {
	case namedb.NameNew:
		// NAME_NEW requires OP_2DROP after the hash
		if offset >= len(script) || script[offset] != op2Drop {
			return 0, fmt.Errorf("NAME_NEW script missing required OP_2DROP (0x6d) at offset %d", offset)
		}
		offset++

	case namedb.NameFirstUpdate:
		// NAME_FIRSTUPDATE requires OP_2DROP OP_2DROP after name, rand, value
		if offset >= len(script) || script[offset] != op2Drop {
			return 0, fmt.Errorf("NAME_FIRSTUPDATE script missing first OP_2DROP (0x6d) at offset %d", offset)
		}
		offset++
		if offset >= len(script) || script[offset] != op2Drop {
			return 0, fmt.Errorf("NAME_FIRSTUPDATE script missing second OP_2DROP (0x6d) at offset %d", offset)
		}
		offset++

	case namedb.NameUpdate:
		// NAME_UPDATE requires OP_2DROP OP_DROP after name and value
		if offset >= len(script) || script[offset] != op2Drop {
			return 0, fmt.Errorf("NAME_UPDATE script missing required OP_2DROP (0x6d) at offset %d", offset)
		}
		offset++
		if offset >= len(script) || script[offset] != opDrop {
			return 0, fmt.Errorf("NAME_UPDATE script missing required OP_DROP (0x75) at offset %d", offset)
		}
		offset++

	default:
		return 0, fmt.Errorf("unknown name operation type: %d", opType)
	}

	// Validate P2PKH suffix exists and has minimum valid length
	// Standard P2PKH script is 25 bytes: OP_DUP OP_HASH160 <20 bytes> OP_EQUALVERIFY OP_CHECKSIG
	const minP2PKHSize = 25
	remainingBytes := len(script) - offset
	if remainingBytes < minP2PKHSize {
		return 0, fmt.Errorf("P2PKH suffix too short: %d bytes (minimum %d bytes required)", remainingBytes, minP2PKHSize)
	}

	return offset, nil
}

// parseNameScriptFull extracts name operation from script with additional data.
// Returns the operation type, name, value, extra data (hash for NAME_NEW, rand for
// NAME_FIRSTUPDATE), and any parsing error.
//
// This function enforces strict script format validation per Namecoin Core consensus rules.
// Scripts must include proper drop opcodes (OP_2DROP, OP_DROP) and P2PKH suffix.
func parseNameScriptFull(script []byte) (namedb.NameOperation, string, string, []byte, error) {
	if len(script) < 2 {
		return 0, "", "", nil, fmt.Errorf("script too short")
	}

	var opType namedb.NameOperation
	var name, value string
	var extra []byte
	var dataEndOffset int

	switch script[0] {
	case opNameNew:
		// NAME_NEW: OP_NAME_NEW <hash> OP_2DROP <P2PKH>
		// Extract the commitment hash (typically 20 bytes)
		hash, newOffset, err := readPushData(script, 1)
		if err != nil {
			return 0, "", "", nil, fmt.Errorf("failed to read hash: %w", err)
		}
		opType = namedb.NameNew
		extra = append([]byte(nil), hash...)
		dataEndOffset = newOffset

	case opNameFirstUpdate:
		// NAME_FIRSTUPDATE: OP_NAME_FIRSTUPDATE <name> <rand> <value> OP_2DROP OP_2DROP <P2PKH>
		nameBytes, offset, err := readPushDataWithError(script, 1, "name")
		if err != nil {
			return 0, "", "", nil, err
		}

		rand, offset, err := readPushDataWithError(script, offset, "rand")
		if err != nil {
			return 0, "", "", nil, err
		}

		valueBytes, newOffset, err := readPushDataWithError(script, offset, "value")
		if err != nil {
			return 0, "", "", nil, err
		}

		opType = namedb.NameFirstUpdate
		name = string(nameBytes)
		value = string(valueBytes)
		extra = append([]byte(nil), rand...)
		dataEndOffset = newOffset

	case opNameUpdate:
		// NAME_UPDATE: OP_NAME_UPDATE <name> <value> OP_2DROP OP_DROP <P2PKH>
		var err error
		name, value, dataEndOffset, err = parseNameValue(script, 1)
		if err != nil {
			return 0, "", "", nil, err
		}
		opType = namedb.NameUpdate

	default:
		return 0, "", "", nil, fmt.Errorf("not a name operation")
	}

	// Validate strict script format (drop opcodes and P2PKH suffix)
	_, err := validateScriptFormat(script, opType, dataEndOffset)
	if err != nil {
		return 0, "", "", nil, fmt.Errorf("invalid script format: %w", err)
	}

	return opType, name, value, extra, nil
}

// parseNameValue reads the name and value push-data fields from a name-operation script
// starting at startOffset. Returns the field strings and the offset after both fields.
func parseNameValue(script []byte, startOffset int) (name, value string, offset int, err error) {
	nameBytes, newOffset, readErr := readPushDataWithError(script, startOffset, "name")
	if readErr != nil {
		return "", "", 0, readErr
	}
	valueBytes, finalOffset, readErr := readPushDataWithError(script, newOffset, "value")
	if readErr != nil {
		return "", "", 0, readErr
	}
	return string(nameBytes), string(valueBytes), finalOffset, nil
}

// skipPushDataFields skips the specified number of push data fields in the script.
func skipPushDataFields(script []byte, offset, count int) (int, error) {
	for i := 0; i < count; i++ {
		_, newOffset, err := readPushData(script, offset)
		if err != nil {
			return 0, err
		}
		offset = newOffset
	}
	return offset, nil
}

// readPushDataWithError reads push data and wraps errors with field name.
func readPushDataWithError(script []byte, offset int, fieldName string) ([]byte, int, error) {
	data, newOffset, err := readPushData(script, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read %s: %w", fieldName, err)
	}
	return data, newOffset, nil
}

// extractAddressFromNameScript extracts the owner address from a name operation script.
// Namecoin name scripts have the format: <name_op> <data...> <drop_ops> <P2PKH script>
// This function parses past the name operation data and drop opcodes to extract
// the address from the embedded P2PKH script.
// Returns an empty string if the address cannot be extracted.
//
// Note: This function is called after script validation, so it can safely skip
// past the drop opcodes that have already been validated by parseNameScriptFull.
func extractAddressFromNameScript(script []byte, chainParams *chaincfg.Params) string {
	if len(script) < 2 || chainParams == nil {
		return ""
	}

	offset, err := skipNameScriptPrefix(script)
	if err != nil || offset >= len(script) {
		return ""
	}

	_, addrs, _, err := txscript.ExtractPkScriptAddrs(script[offset:], chainParams)
	if err != nil || len(addrs) == 0 {
		return ""
	}
	return addrs[0].EncodeAddress()
}

// skipNameScriptPrefix advances past the name operation prefix to the P2PKH portion.
func skipNameScriptPrefix(script []byte) (int, error) {
	switch script[0] {
	case opNameNew:
		return skipNameNewPrefix(script)
	case opNameFirstUpdate:
		return skipNameFirstUpdatePrefix(script)
	case opNameUpdate:
		return skipNameUpdatePrefix(script)
	default:
		return 0, fmt.Errorf("not a name script")
	}
}

// skipNameNewPrefix skips the NAME_NEW prefix: OP_NAME_NEW <hash> OP_2DROP.
func skipNameNewPrefix(script []byte) (int, error) {
	_, offset, err := readPushData(script, 1)
	if err != nil {
		return 0, err
	}
	if offset < len(script) && script[offset] == 0x6d {
		offset++
	}
	return offset, nil
}

// skipNameFirstUpdatePrefix skips the NAME_FIRSTUPDATE prefix: OP_NAME_FIRSTUPDATE <name> <rand> <value> OP_2DROP OP_2DROP.
func skipNameFirstUpdatePrefix(script []byte) (int, error) {
	offset, err := skipPushDataFields(script, 1, 3)
	if err != nil {
		return 0, err
	}
	for i := 0; i < 2 && offset < len(script) && script[offset] == 0x6d; i++ {
		offset++
	}
	return offset, nil
}

// skipNameUpdatePrefix skips the NAME_UPDATE prefix: OP_NAME_UPDATE <name> <value> OP_2DROP OP_DROP.
func skipNameUpdatePrefix(script []byte) (int, error) {
	offset, err := skipPushDataFields(script, 1, 2)
	if err != nil {
		return 0, err
	}
	if offset < len(script) && script[offset] == 0x6d {
		offset++
	}
	if offset < len(script) && script[offset] == 0x75 {
		offset++
	}
	return offset, nil
}

// readPushData reads a Bitcoin-style push data from the script at the given offset.
// Returns the data, the new offset after reading, and any error.
// Bitcoin script push data format:
//   - 0x00: push empty byte array (OP_0)
//   - 0x01-0x4b: next N bytes are data (N is the opcode value)
//   - 0x4c (OP_PUSHDATA1): next byte is length, then data
//   - 0x4d (OP_PUSHDATA2): next 2 bytes are length (little-endian), then data
//   - 0x4e (OP_PUSHDATA4): next 4 bytes are length (little-endian), then data
func readPushData(script []byte, offset int) ([]byte, int, error) {
	if offset >= len(script) {
		return nil, offset, fmt.Errorf("offset beyond script length")
	}

	startOffset := offset
	opcode := script[offset]
	offset++

	var dataLen int

	switch {
	case opcode == 0x00:
		// OP_0: push empty byte array
		dataLen = 0

	case opcode >= 0x01 && opcode <= 0x4b:
		// Direct push: opcode is the length (1-75 bytes)
		dataLen = int(opcode)

	case opcode == opPushData1:
		// OP_PUSHDATA1: next byte is length
		if offset >= len(script) {
			return nil, offset, fmt.Errorf("missing length byte for OP_PUSHDATA1")
		}
		dataLen = int(script[offset])
		offset++

	case opcode == opPushData2:
		// OP_PUSHDATA2: next 2 bytes are length (little-endian)
		if offset+1 >= len(script) {
			return nil, offset, fmt.Errorf("missing length bytes for OP_PUSHDATA2")
		}
		dataLen = int(script[offset]) | (int(script[offset+1]) << 8)
		offset += 2

	case opcode == opPushData4:
		// OP_PUSHDATA4: next 4 bytes are length (little-endian)
		// Parse as uint32 to avoid sign-extension issues on 32-bit platforms
		if offset+3 >= len(script) {
			return nil, offset, fmt.Errorf("missing length bytes for OP_PUSHDATA4")
		}
		lenU32 := uint32(script[offset]) |
			(uint32(script[offset+1]) << 8) |
			(uint32(script[offset+2]) << 16) |
			(uint32(script[offset+3]) << 24)
		// Convert to int with overflow check (int is at least 32 bits, max value 2^31-1)
		// Scripts are never this large in practice, so reject absurdly large values
		if lenU32 > 0x7FFFFFFF { // int32 max
			return nil, offset, fmt.Errorf("OP_PUSHDATA4 length %d exceeds maximum", lenU32)
		}
		dataLen = int(lenU32)
		offset += 4

	default:
		return nil, offset, fmt.Errorf("unexpected opcode 0x%02x at offset %d", opcode, startOffset)
	}

	// Check if we have enough data
	if offset+dataLen > len(script) {
		return nil, offset, fmt.Errorf("data length %d exceeds remaining script", dataLen)
	}

	data := script[offset : offset+dataLen]
	return data, offset + dataLen, nil
}

// validateConsensusNameFormat validates only the consensus-critical constraints for a name operation.
// This function enforces only what Namecoin Core consensus enforces:
// - Name length must be ≤ 255 bytes
// - Value length must be ≤ MaxValueLength (520 bytes, matching Namecoin Core)
// It does NOT enforce namespace prefixes, JSON encoding, or UTF-8 validation,
// as these are local policies, not part of the consensus protocol.
// Mainnet blocks may contain names without d/, id/, p/ prefixes and values
// that are not UTF-8 or valid JSON; accepting these is required for IBD.
func validateConsensusNameFormat(name, value string) error {
	// Validate name length (consensus-critical)
	if len(name) == 0 || len(name) > config.MaxNameLength {
		return fmt.Errorf("invalid name length: %d (max: %d)", len(name), config.MaxNameLength)
	}

	// Validate value length (consensus-critical)
	if len(value) > config.MaxValueLength {
		return fmt.Errorf("value too large: %d bytes (max: %d)", len(value), config.MaxValueLength)
	}

	return nil
}

// validateNameFormat validates name and value format for local operations (wallet/RPC).
// This enforces stricter policies than consensus:
// - Namespace prefix validation (d/, id/, p/)
// - JSON encoding requirement for d/ and id/ namespaces
// - UTF-8 encoding requirement for all values
// These constraints are applied to locally-created transactions but NOT enforced
// during consensus validation, allowing nmcd to accept valid mainnet blocks.
func validateNameFormat(name, value string) error {
	if len(name) == 0 || len(name) > config.MaxNameLength {
		return fmt.Errorf("invalid name length: %d (max: %d)", len(name), config.MaxNameLength)
	}

	// Validate namespace prefix (local policy, not consensus)
	if !config.IsValidNamespace(name) {
		return fmt.Errorf("invalid namespace: name must start with a valid namespace prefix (d/, id/, p/)")
	}

	// Ensure there is content after the namespace prefix
	// Check each valid namespace to find which one matches and verify content exists after it
	hasContent := false
	for _, ns := range config.ValidNamespaces {
		if len(name) >= len(ns) && name[:len(ns)] == ns {
			if len(name) > len(ns) {
				hasContent = true
				break
			}
		}
	}
	if !hasContent {
		return fmt.Errorf("invalid name: must have content after namespace prefix")
	}

	if len(value) > config.MaxValueLength {
		return fmt.Errorf("value too large: %d bytes (max: %d)", len(value), config.MaxValueLength)
	}

	// Validate value encoding based on namespace (local policy, not consensus)
	if err := validateValueEncoding(name, value); err != nil {
		return err
	}

	return nil
}

// validateValueEncoding validates the encoding of a name value based on its namespace.
// Per this implementation:
// - d/ (domain) namespace: values must be valid UTF-8 and must be valid JSON for DNS records
// - id/ (identity) namespace: values must be valid UTF-8 and must be valid JSON
// - p/ (personal) namespace: values must be valid UTF-8; JSON is optional and not enforced
func validateValueEncoding(name, value string) error {
	// Empty values are allowed (deletion/reservation pattern)
	if len(value) == 0 {
		return nil
	}

	// All namespaces require valid UTF-8 encoding
	if !utf8.ValidString(value) {
		return fmt.Errorf("value must be valid UTF-8")
	}

	// For d/ (domain) and id/ (identity) namespaces, validate JSON encoding
	// These namespaces store structured data (DNS records, identity records)
	if (len(name) >= 2 && name[:2] == "d/") || (len(name) >= 3 && name[:3] == "id/") {
		// Attempt to parse as JSON
		var jsonData interface{}
		if err := json.Unmarshal([]byte(value), &jsonData); err != nil {
			ns := "specified"
			if len(name) >= 2 && name[:2] == "d/" {
				ns = "d/"
			} else if len(name) >= 3 && name[:3] == "id/" {
				ns = "id/"
			}
			return fmt.Errorf("value must be valid JSON for %s namespace: %w", ns, err)
		}
	}

	// For p/ (personal) namespace, only UTF-8 validation is required
	// Personal namespace is more flexible and can contain arbitrary text

	return nil
}

// NameOperationInfo contains parsed name operation data extracted from a transaction output.
// Used by RPC methods like name_pending to provide information about pending name operations
// in the mempool before they are confirmed in a block.
type NameOperationInfo struct {
	OpType      namedb.NameOperation // The type of name operation (NameNew, NameFirstUpdate, NameUpdate)
	Name        string               // The name being operated on (empty for NAME_NEW)
	Value       string               // The value associated with the name (empty for NAME_NEW)
	TxHash      chainhash.Hash       // The transaction hash containing this operation
	OutputIndex int                  // The output index within the transaction
}

// ParseNameOperationsFromTx extracts name operations from a transaction's outputs.
// Returns a slice of NameOperationInfo for each name operation found in the transaction.
// This is useful for finding pending name operations in the mempool.
func ParseNameOperationsFromTx(tx *wire.MsgTx) []NameOperationInfo {
	var operations []NameOperationInfo

	txHash := tx.TxHash()

	for i, txOut := range tx.TxOut {
		opType, name, value, err := parseNameScript(txOut.PkScript)
		if err == nil && opType != namedb.NameOperation(0) {
			operations = append(operations, NameOperationInfo{
				OpType:      opType,
				Name:        name,
				Value:       value,
				TxHash:      txHash,
				OutputIndex: i,
			})
		}
	}

	return operations
}
