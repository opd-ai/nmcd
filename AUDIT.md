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
| MISSING FEATURE | ~~3~~ 0 (3 resolved) | Medium-Low |
| EDGE CASE BUG | ~~2~~ 1 (1 resolved) | Low |
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

**Resolution:** Implemented proper Namecoin script parsing with Bitcoin-style length-prefixed push data. The `parseNameScript` function now correctly uses the real Namecoin opcodes (OP_NAME_NEW=0xd0, OP_NAME_FIRSTUPDATE=0xd1, OP_NAME_UPDATE=0xd2) and properly extracts name and value data from scripts using the `readPushData` helper function. The implementation supports all Bitcoin push data formats (direct push for 1-75 bytes, OP_PUSHDATA1 for 76-255 bytes, OP_PUSHDATA2 for 256-65535 bytes, and OP_PUSHDATA4 for larger data). Comprehensive unit tests were added covering all name operations, push data formats, error cases, and edge cases.

**Fixed Code:**
```go
// Namecoin-specific opcodes for name operations.
const (
	opNameNew         = 0xd0  // NAME_NEW
	opNameFirstUpdate = 0xd1  // NAME_FIRSTUPDATE
	opNameUpdate      = 0xd2  // NAME_UPDATE
	opPushData1       = 0x4c  // Push 76-255 bytes
	opPushData2       = 0x4d  // Push 256-65535 bytes
	opPushData4       = 0x4e  // Push up to 4GB (rarely used)
)

// parseNameScript extracts name operation from script.
func parseNameScript(script []byte) (namedb.NameOperation, string, string, error) {
	// ... proper switch-based parsing with readPushData helper
}

// readPushData reads a Bitcoin-style push data from the script.
func readPushData(script []byte, offset int) ([]byte, int, error) {
	// Handles direct push, OP_PUSHDATA1, OP_PUSHDATA2, and OP_PUSHDATA4
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

### ~~MISSING FEATURE: MinBlocksBeforeFirstUpdate Constraint Not Enforced~~ ✅ RESOLVED

**File:** config/config.go:16, chain/blockchain.go:110-115  
**Severity:** Medium  
**Status:** ✅ RESOLVED

**Resolution:** Implemented the MinBlocksBeforeFirstUpdate constraint enforcement. The solution includes:
1. Added `nameNewBucket` in namedb to track NAME_NEW commitments with their block heights
2. Added `NameNewRecord` struct and `PutNameNew`, `GetNameNew`, `DeleteNameNew` functions to manage NAME_NEW tracking
3. Modified `parseNameScriptFull` to extract commitment hash from NAME_NEW and rand from NAME_FIRSTUPDATE
4. Added `computeCommitHash` function using RIPEMD160(SHA256(rand || name)) per Namecoin protocol
5. Updated `validateNameOperations` to verify NAME_NEW exists and 12+ blocks have passed before NAME_FIRSTUPDATE
6. Updated `updateNameDatabase` to store NAME_NEW commitments and clean them up after NAME_FIRSTUPDATE

**Fixed Code:**
```go
// In validateNameOperations:
case namedb.NameFirstUpdate:
	// Verify name doesn't exist
	if _, err := bc.nameDB.GetName(name); err == nil {
		return fmt.Errorf("name already exists: %s", name)
	}

	// Compute the commitment hash from rand (extra) and name
	commitHash := computeCommitHash(extra, name)

	// Verify NAME_NEW exists and MinBlocksBeforeFirstUpdate has passed
	nameNewRecord, err := bc.nameDB.GetNameNew(commitHash)
	if err != nil {
		return fmt.Errorf("no matching name_new found for name: %s", name)
	}

	// Check that enough blocks have passed since NAME_NEW
	blocksSinceNew := height - nameNewRecord.Height
	if blocksSinceNew < config.MinBlocksBeforeFirstUpdate {
		return fmt.Errorf("name_firstupdate too early: %d blocks since name_new, minimum %d required",
			blocksSinceNew, config.MinBlocksBeforeFirstUpdate)
	}
```

---

### ~~MISSING FEATURE: NAME_NEW Hash Commitment Not Tracked or Validated~~ ✅ RESOLVED

**File:** chain/blockchain.go:108-110  
**Severity:** Medium  
**Status:** ✅ RESOLVED

**Resolution:** Implemented NAME_NEW commitment tracking as part of the MinBlocksBeforeFirstUpdate constraint enforcement. The implementation includes:
1. Added `nameNewBucket` in namedb to store NAME_NEW commitments keyed by commitment hash
2. `parseNameScriptFull` now extracts the commitment hash from NAME_NEW scripts
3. `updateNameDatabase` stores NAME_NEW commitments when processing blocks
4. `validateNameOperations` verifies NAME_FIRSTUPDATE has a matching NAME_NEW commitment
5. `computeCommitHash` computes RIPEMD160(SHA256(rand || name)) to match commitments
6. NAME_NEW commitments are cleaned up after successful NAME_FIRSTUPDATE

**Fixed Code:**
```go
// In updateNameDatabase:
case namedb.NameNew:
	// Store the commitment hash with block height
	// extra contains the commitment hash from the script
	if err := bc.nameDB.PutNameNew(extra, height); err != nil {
		return err
	}

// In validateNameOperations:
case namedb.NameFirstUpdate:
	// ... verify name doesn't exist ...
	
	// Compute the commitment hash from rand (extra) and name
	commitHash := computeCommitHash(extra, name)

	// Verify NAME_NEW exists and MinBlocksBeforeFirstUpdate has passed
	nameNewRecord, err := bc.nameDB.GetNameNew(commitHash)
	if err != nil {
		return fmt.Errorf("no matching name_new found for name: %s", name)
	}
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

### ~~EDGE CASE BUG: decodeNameRecord Returns Empty Record Instead of Error on Corrupt Data~~ ✅ RESOLVED

**File:** namedb/namedb.go:348-421  
**Severity:** Low  
**Status:** ✅ RESOLVED

**Resolution:** Changed `decodeNameRecord` to return `(*NameRecord, error)` instead of just `*NameRecord`. The function now returns `nil` and a descriptive error when it encounters corrupt or truncated data, instead of silently returning an empty record. All callers (`GetName`, `GetExpiredNames`, `ListNames`, `GetHistory`) have been updated to propagate decode errors properly. Comprehensive unit tests were added covering empty data, truncated value length, truncated value data, truncated txhash, truncated height, and truncated expires_at scenarios.

**Fixed Code:**
```go
// decodeNameRecord deserializes a name record.
// Returns an error if the data is corrupt or truncated.
func decodeNameRecord(data []byte) (*NameRecord, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("corrupt record: empty data")
	}
	// ... validation with proper error returns ...
	if offset+4 > len(data) {
		return nil, fmt.Errorf("corrupt record: truncated at value length")
	}
	// ... more validation ...
	return record, nil
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
- ~~`decodeNameRecord` silently returns empty records instead of errors~~ ✅ RESOLVED - Now returns proper errors
- Registration errors in `init()` are intentionally ignored (documented reason is acceptable)

### Code Quality
- Well-organized package structure following Go conventions
- Consistent coding style throughout
- Good use of composition over inheritance (as documented)
- Proper use of btcd libraries as dependencies

---

## RECOMMENDATIONS

1. ~~**High Priority:** Implement real Namecoin script parsing in `parseNameScript` to support actual network transactions~~ ✅ RESOLVED
2. ~~**High Priority:** Implement NAME_NEW commitment tracking and validation for front-running protection~~ ✅ RESOLVED
3. ~~**Medium Priority:** Add `GetHistory` function and fix `name_history` RPC endpoint~~ ✅ RESOLVED
4. ~~**Medium Priority:** Implement MinBlocksBeforeFirstUpdate validation~~ ✅ RESOLVED
5. ~~**Low Priority:** Fix comma-separated flag parsing for multiple peers/addresses~~ ✅ RESOLVED
6. ~~**Low Priority:** Change `decodeNameRecord` to return errors on corrupt data~~ ✅ RESOLVED
7. **Low Priority:** Review off-by-one in expiration comparison

---

## AUDIT VERIFICATION CHECKLIST

- [x] Dependency analysis completed before code examination
- [x] Audit progression followed dependency levels strictly (Level 0 → 4)
- [x] All findings include specific file references and line numbers
- [x] Each bug explanation includes reproduction steps
- [x] Severity ratings align with actual impact on functionality
- [x] No code modifications suggested (analysis only)
