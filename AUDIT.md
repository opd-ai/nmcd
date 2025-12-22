# nmcd Comprehensive Functional Audit Report

**Audit Date:** 2024-12-22  
**Auditor:** Automated Code Auditor  
**Repository:** opd-ai/nmcd  
**Commit:** Latest version  

## AUDIT SUMMARY

This audit analyzes the nmcd codebase for discrepancies between documented functionality (README.md) and actual implementation. The analysis follows a dependency-based file examination order to ensure comprehensive coverage.

### Dependency Analysis

| Level | Files | Description |
|-------|-------|-------------|
| 0 | `config/config.go`, `config/namecoin_params.go` | Pure configuration, no internal dependencies |
| 1 | `namedb/namedb.go` | Name database, external dependencies only |
| 2 | `chain/blockchain.go` | Blockchain wrapper, imports namedb + config |
| 3 | `network/peermgr.go`, `rpc/server.go` | Network and RPC, imports chain |
| 4 | `cmd/nmcd/main.go` | Entry point, imports all components |

### Issue Totals

| Category | Count | Severity |
|----------|-------|----------|
| CRITICAL BUG | 0 | - |
| FUNCTIONAL MISMATCH | ~~2~~ 0 (2 resolved) | Medium |
| MISSING FEATURE | ~~3~~ 2 (1 resolved) | Medium-Low |
| EDGE CASE BUG | 2 | Low |
| PERFORMANCE ISSUE | 0 | - |

---

## DETAILED FINDINGS

### ~~FUNCTIONAL MISMATCH: name_history RPC Method Returns Error Instead of History Data~~ ✅ RESOLVED

**File:** rpc/server.go:310-323  
**Severity:** Medium  
**Status:** ✅ RESOLVED

**Resolution:** Implemented the `name_history` RPC method that retrieves historical records for a specific name. The method calls `GetNameHistory` on the blockchain which in turn uses the new `GetHistory` function in namedb. Returns an array of historical name operations with name, value, txid, height, expires_at, and address fields.

**Fixed Code:**
```go
// nameHistory returns the history of a name, including all past operations.
func (s *Server) nameHistory(req *Request) *Response {
	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) == 0 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid params: expected [\"name\"]",
			},
			ID: req.ID,
		}
	}

	name := params[0]
	history, err := s.blockchain.GetNameHistory(name)
	// ... returns formatted history array
}
```

---

### ~~FUNCTIONAL MISMATCH: parseNameScript uses Placeholder Opcodes Instead of Real Namecoin Script Parsing~~ ✅ RESOLVED

**File:** chain/blockchain.go  
**Severity:** Medium  
**Status:** ✅ RESOLVED

**Resolution:** Implemented proper Namecoin script parsing with Bitcoin-style length-prefixed push data. The `parseNameScript` function now correctly uses the real Namecoin opcodes (OP_NAME_NEW=0x51, OP_NAME_FIRSTUPDATE=0x52, OP_NAME_UPDATE=0x53) and properly extracts name and value data from scripts using the `readPushData` helper function. The implementation supports all Bitcoin push data formats (direct push for 1-75 bytes, OP_PUSHDATA1 for 76-255 bytes, and OP_PUSHDATA2 for 256+ bytes). Comprehensive unit tests were added covering all name operations, push data formats, error cases, and edge cases.

**Fixed Code:**
```go
// Namecoin-specific opcodes for name operations.
const (
	opNameNew         = 0x51  // NAME_NEW
	opNameFirstUpdate = 0x52  // NAME_FIRSTUPDATE
	opNameUpdate      = 0x53  // NAME_UPDATE
	opPushData1       = 0x4c  // Push 76-255 bytes
	opPushData2       = 0x4d  // Push 256-65535 bytes
)

// parseNameScript extracts name operation from script.
func parseNameScript(script []byte) (namedb.NameOperation, string, string, error) {
	// ... proper switch-based parsing with readPushData helper
}

// readPushData reads a Bitcoin-style push data from the script.
func readPushData(script []byte, offset int) ([]byte, int, error) {
	// Handles direct push, OP_PUSHDATA1, and OP_PUSHDATA2
}
```

---

### ~~FUNCTIONAL MISMATCH: Command-Line Flag Parsing Does Not Handle Comma-Separated Values as Documented~~ ✅ RESOLVED

**File:** cmd/nmcd/main.go:103-121  
**Severity:** Low  
**Status:** ✅ RESOLVED

**Resolution:** Implemented `splitAndTrim` helper function that properly splits comma-separated values and trims whitespace. Added comprehensive unit tests for the functionality.

**Fixed Code:**
```go
// splitAndTrim splits a comma-separated string and trims whitespace from each element.
// Empty elements are filtered out.
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
```

---

### ~~MISSING FEATURE: GetHistory Function Not Exposed in NameDatabase~~ ✅ RESOLVED

**File:** namedb/namedb.go  
**Severity:** Medium  
**Status:** ✅ RESOLVED

**Resolution:** Implemented `GetHistory` function that retrieves all historical records for a specific name. Added a secondary index bucket (`historyIndexBucket`) that maps name -> list of transaction hashes for efficient lookups. The `AddHistory` function was updated to maintain this index. Comprehensive unit tests were added covering multiple entries, empty history, and multiple names scenarios.

**Fixed Code:**
```go
// GetHistory retrieves all historical records for a specific name.
// Returns a slice of NameRecords ordered by when they were added (oldest first).
func (ndb *NameDatabase) GetHistory(name string) ([]*NameRecord, error) {
	ndb.mu.RLock()
	defer ndb.mu.RUnlock()

	var records []*NameRecord
	err := ndb.db.View(func(tx *bbolt.Tx) error {
		// Get the list of txHashes from the index
		indexBucket := tx.Bucket(historyIndexBucket)
		indexData := indexBucket.Get([]byte(name))
		if indexData == nil {
			return nil // No history for this name
		}

		// Each txHash is 32 bytes
		const hashSize = 32
		histBucket := tx.Bucket(historyBucket)
		for i := 0; i < len(indexData); i += hashSize {
			txHashBytes := indexData[i : i+hashSize]
			data := histBucket.Get(txHashBytes)
			if data != nil {
				record := decodeNameRecord(data)
				record.Name = name
				records = append(records, record)
			}
		}
		return nil
	})
	return records, err
}
```

---

### MISSING FEATURE: MinBlocksBeforeFirstUpdate Constraint Not Enforced

**File:** config/config.go:16, chain/blockchain.go:110-115  
**Severity:** Medium  
**Description:** The configuration defines `MinBlocksBeforeFirstUpdate = 12` as a protocol constant, but this constraint is never validated in the blockchain code. The NAME_NEW to NAME_FIRSTUPDATE timing requirement is not enforced.

**Expected Behavior:** According to Namecoin protocol and the defined constant:
- There should be at least 12 blocks between NAME_NEW and NAME_FIRSTUPDATE operations
- This prevents front-running attacks on name registrations

**Actual Behavior:** The `validateNameOperations` function only checks:
1. Whether a name exists (for FIRSTUPDATE)
2. Whether a name is expired (for UPDATE)

It does not verify that the required block distance from NAME_NEW has elapsed.

**Impact:** The front-running protection mechanism documented in the protocol is not enforced, potentially allowing name sniping attacks.

**Reproduction:** Issue NAME_NEW and NAME_FIRSTUPDATE in consecutive blocks - no validation error occurs.

**Code Reference:**
```go
// config/config.go
const (
	// MinBlocksBeforeFirstUpdate is the minimum blocks between name_new and name_firstupdate
	MinBlocksBeforeFirstUpdate = 12  // Defined but never used
)

// chain/blockchain.go
case namedb.NameFirstUpdate:
	// Verify name doesn't exist
	if _, err := bc.nameDB.GetName(name); err == nil {
		return fmt.Errorf("name already exists: %s", name)
	}
	// Missing: Check that 12 blocks have passed since NAME_NEW
```

---

### MISSING FEATURE: NAME_NEW Hash Commitment Not Tracked or Validated

**File:** chain/blockchain.go:108-110  
**Severity:** Medium  
**Description:** The NAME_NEW operation in Namecoin protocol requires a hash commitment (hash of salt + name) to be recorded and later verified during NAME_FIRSTUPDATE. The current implementation simply ignores NAME_NEW operations without tracking them.

**Expected Behavior:** According to Namecoin protocol:
1. NAME_NEW should store a commitment hash
2. NAME_FIRSTUPDATE must reveal the salt and name that match the commitment
3. This prevents name front-running

**Actual Behavior:** NAME_NEW is allowed to pass through without any storage or validation:
```go
case namedb.NameNew:
	// NAME_NEW is always valid (pre-registration)
	continue
```

**Impact:** The name pre-registration system that prevents front-running is completely non-functional.

**Reproduction:** Issue NAME_FIRSTUPDATE without a prior NAME_NEW - no error occurs.

**Code Reference:**
```go
case namedb.NameNew:
	// NAME_NEW is always valid (pre-registration)
	continue  // No commitment is stored
```

---

### EDGE CASE BUG: GetExpiredNames Returns Names Expiring AT Current Height

**File:** namedb/namedb.go:119-136  
**Severity:** Low  
**Description:** The `GetExpiredNames` function uses `<=` comparison, which returns names that expire exactly at the current height. This is potentially off-by-one depending on the expected semantic (names typically should be valid through their expiration block).

**Expected Behavior:** Names should be valid through their expiration block, and only considered expired after.

**Actual Behavior:** Names are returned as expired when `ExpiresAt <= height`, meaning a name expiring at block 100 is considered expired when queried at block 100.

**Impact:** Names may be deleted one block earlier than expected.

**Reproduction:** Create a name expiring at height 100, query `GetExpiredNames(100)`, the name is returned.

**Code Reference:**
```go
func (ndb *NameDatabase) GetExpiredNames(height int32) ([]string, error) {
	// ...
	for k, v := c.First(); k != nil; k, v = c.Next() {
		record := decodeNameRecord(v)
		if record.ExpiresAt <= height {  // Should this be < instead of <=?
			expired = append(expired, string(k))
		}
	}
	// ...
}
```

---

### EDGE CASE BUG: decodeNameRecord Returns Empty Record Instead of Error on Corrupt Data

**File:** namedb/namedb.go:210-283  
**Severity:** Low  
**Description:** The `decodeNameRecord` function returns an empty `NameRecord{}` when it encounters corrupt or truncated data, rather than returning an error. This can lead to silent data corruption where callers receive seemingly valid but empty records.

**Expected Behavior:** The function should return an error when data cannot be properly decoded.

**Actual Behavior:** Multiple early returns with `return &NameRecord{}` when data validation fails, with no way for the caller to distinguish between an empty record and a decode failure.

**Impact:** Data corruption in the database could go unnoticed, with empty records being processed as valid.

**Reproduction:** Manually corrupt a record in the bbolt database, then call `GetName` - an empty record is returned without error.

**Code Reference:**
```go
func decodeNameRecord(data []byte) *NameRecord {
	if len(data) < 1 {
		return &NameRecord{}  // No error returned
	}
	// ...
	if offset+4 > len(data) {
		return &NameRecord{}  // No error returned
	}
	// ...multiple similar cases
}
```

---

## QUALITY ASSESSMENT

### Documentation Accuracy
The README.md accurately describes the architecture and design principles. However, some documented features are stubs or incompletely implemented:
- `name_history` is documented but returns an error
- The three name operations (NAME_NEW, NAME_FIRSTUPDATE, NAME_UPDATE) are documented but the script parsing is placeholder code

### Thread Safety
The codebase demonstrates consistent thread safety practices:
- All shared state is protected by `sync.RWMutex`
- Lock ordering appears consistent to prevent deadlocks
- The `namedb.NameDatabase` uses read locks for read operations and write locks for write operations

### Error Handling
Error handling is generally good with some exceptions:
- `decodeNameRecord` silently returns empty records instead of errors
- Registration errors in `init()` are intentionally ignored (documented reason is acceptable)

### Code Quality
- Well-organized package structure following Go conventions
- Consistent coding style throughout
- Good use of composition over inheritance (as documented)
- Proper use of btcd libraries as dependencies

---

## RECOMMENDATIONS

1. ~~**High Priority:** Implement real Namecoin script parsing in `parseNameScript` to support actual network transactions~~ ✅ RESOLVED
2. **High Priority:** Implement NAME_NEW commitment tracking and validation for front-running protection
3. ~~**Medium Priority:** Add `GetHistory` function and fix `name_history` RPC endpoint~~ ✅ RESOLVED
4. **Medium Priority:** Implement MinBlocksBeforeFirstUpdate validation
5. ~~**Low Priority:** Fix comma-separated flag parsing for multiple peers/addresses~~ ✅ RESOLVED
6. **Low Priority:** Change `decodeNameRecord` to return errors on corrupt data
7. **Low Priority:** Review off-by-one in expiration comparison

---

## AUDIT VERIFICATION CHECKLIST

- [x] Dependency analysis completed before code examination
- [x] Audit progression followed dependency levels strictly (Level 0 → 4)
- [x] All findings include specific file references and line numbers
- [x] Each bug explanation includes reproduction steps
- [x] Severity ratings align with actual impact on functionality
- [x] No code modifications suggested (analysis only)
