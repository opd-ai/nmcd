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
| FUNCTIONAL MISMATCH | 3 | Medium |
| MISSING FEATURE | 3 | Medium-Low |
| EDGE CASE BUG | 2 | Low |
| PERFORMANCE ISSUE | 0 | - |

---

## DETAILED FINDINGS

### FUNCTIONAL MISMATCH: name_history RPC Method Returns Error Instead of History Data

**File:** rpc/server.go:310-323  
**Severity:** Medium  
**Description:** The README.md documents `name_history` as a functional RPC method that returns name history. However, the implementation is a stub that always returns an error stating the method is not yet implemented.

**Expected Behavior:** According to README.md:
```bash
curl -X POST http://127.0.0.1:8336 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"name_history","params":["d/example"],"id":1}'
```
Should return historical name operations.

**Actual Behavior:** The method returns:
```json
{"jsonrpc":"2.0","error":{"code":-32601,"message":"name_history method is not yet implemented"},"id":1}
```

**Impact:** Users following the README documentation will be unable to retrieve name history as documented.

**Reproduction:** Call the `name_history` RPC method with any valid name parameter.

**Code Reference:**
```go
// nameHistory returns the history of a name
func (s *Server) nameHistory(req *Request) *Response {
	// Method stub: name_history is not yet implemented.
	// Returning an explicit error avoids misleading clients into thinking
	// they are receiving full historical data.
	return &Response{
		Jsonrpc: "2.0",
		Error: &Error{
			Code:    -32601,
			Message: "name_history method is not yet implemented",
		},
		ID: req.ID,
	}
}
```

---

### FUNCTIONAL MISMATCH: parseNameScript Uses Placeholder Opcodes Instead of Real Namecoin Script Parsing

**File:** chain/blockchain.go:203-235  
**Severity:** Medium  
**Description:** The `parseNameScript` function uses placeholder opcode values (0x51, 0x52, 0x53) instead of the actual Namecoin name operation opcodes. The real Namecoin protocol uses OP_NAME_NEW, OP_NAME_FIRSTUPDATE, and OP_NAME_UPDATE with specific script structures.

**Expected Behavior:** According to README.md, the implementation should support:
1. NAME_NEW: Pre-register a name
2. NAME_FIRSTUPDATE: First registration of a name
3. NAME_UPDATE: Update existing name value

These should be parsed from actual Namecoin transaction scripts.

**Actual Behavior:** The function uses arbitrary placeholder opcodes that do not correspond to real Namecoin protocol scripts. Additionally, it uses fixed-size extraction (bytes 1-11 for name, 11+ for value) which does not match real Namecoin script encoding that uses length-prefixed push data.

**Impact:** The implementation cannot process real Namecoin transactions from the network. It will fail to recognize valid name operations and may incorrectly identify non-name transactions.

**Reproduction:** Attempt to process a real Namecoin block containing name transactions.

**Code Reference:**
```go
// parseNameScript extracts name operation from script
func parseNameScript(script []byte) (namedb.NameOperation, string, string, error) {
	// Simple parsing - in real implementation would use proper script parsing
	// This is a placeholder that looks for OP_NAME patterns
	if len(script) < 10 {
		return 0, "", "", fmt.Errorf("script too short")
	}

	// Check for name operation opcodes (simplified)
	// Real implementation would properly parse script opcodes
	if script[0] == 0x51 { // OP_NAME_NEW placeholder
		return namedb.NameNew, "", "", nil
	}
	if script[0] == 0x52 { // OP_NAME_FIRSTUPDATE placeholder
		// Extract name and value from script
		if len(script) < 20 {
			return 0, "", "", fmt.Errorf("invalid firstupdate script")
		}
		name := string(script[1:11])
		value := string(script[11:])
		return namedb.NameFirstUpdate, name, value, nil
	}
	// ...
}
```

---

### FUNCTIONAL MISMATCH: Command-Line Flag Parsing Does Not Handle Comma-Separated Values as Documented

**File:** cmd/nmcd/main.go:103-121  
**Severity:** Low  
**Description:** The README.md suggests that multiple peers can be specified, but the command-line flag parsing does not properly handle comma-separated values. The `listenAddrs` and `addPeers` flags expect comma-separated values according to comments, but the actual implementation just wraps the entire string in a single-element slice.

**Expected Behavior:** According to flag comments and typical CLI conventions:
```bash
./nmcd -addpeer=peer1.example.com:8334,peer2.example.com:8334
```
Should connect to multiple peers.

**Actual Behavior:** The entire comma-separated string is treated as a single peer address, causing connection failures.

**Impact:** Users cannot specify multiple initial peers or listen addresses via command line as the documentation comments suggest.

**Reproduction:** Run with `-addpeer=peer1:8334,peer2:8334` and observe connection attempt to the literal string including the comma.

**Code Reference:**
```go
var addPeers string
flag.StringVar(&addPeers, "addpeer", "", "Peers to connect to (comma-separated)")

flag.Parse()

// Parse listen addresses
if listenAddrs != "" {
	cfg.ListenAddrs = []string{listenAddrs}  // Does not split on comma
}

// Parse add peers
if addPeers != "" {
	cfg.AddPeers = []string{addPeers}  // Does not split on comma
}
```

---

### MISSING FEATURE: GetHistory Function Not Exposed in NameDatabase

**File:** namedb/namedb.go  
**Severity:** Medium  
**Description:** While the `AddHistory` function exists to record historical name operations (lines 157-167), there is no corresponding `GetHistory` function to retrieve the history for a specific name. The history bucket stores records keyed by transaction hash, but there's no index or method to query history by name.

**Expected Behavior:** According to README.md:
- "Historical operation tracking" should be available
- The `name_history` RPC method should return history (which requires history retrieval)

**Actual Behavior:** History can be added but cannot be retrieved. The history bucket uses transaction hash as the key, making it impossible to query by name without a full bucket scan.

**Impact:** The documented "Historical operation tracking" feature is incomplete. History is stored but not retrievable.

**Reproduction:** Call `AddHistory` multiple times for the same name, then attempt to retrieve all history entries for that name - no such function exists.

**Code Reference:**
```go
// AddHistory adds a historical name operation
func (ndb *NameDatabase) AddHistory(txHash chainhash.Hash, record *NameRecord) error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(historyBucket)
		data := encodeNameRecord(record)
		return bucket.Put(txHash[:], data)  // Keyed by txHash, not by name
	})
}
// Note: No corresponding GetHistory function exists
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

1. **High Priority:** Implement real Namecoin script parsing in `parseNameScript` to support actual network transactions
2. **High Priority:** Implement NAME_NEW commitment tracking and validation for front-running protection
3. **Medium Priority:** Add `GetHistory` function and fix `name_history` RPC endpoint
4. **Medium Priority:** Implement MinBlocksBeforeFirstUpdate validation
5. **Low Priority:** Fix comma-separated flag parsing for multiple peers/addresses
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
