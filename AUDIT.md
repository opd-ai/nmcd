# Code Audit - Bug Tracking

**Audit Date:** 2026-01-02  
**Scope:** Code quality, incomplete implementations, and TODOs in the nmcd codebase  
**Status:** Active

---

## BUGS AND INCOMPLETE IMPLEMENTATIONS

### BUG-001: Missing NAME_UPDATE destination address support
**Location:** `rpc/server.go:403`  
**Severity:** MEDIUM  
**Status:** **UNRESOLVED**  
**Type:** Missing Feature

**Description:**
The `name_update` RPC endpoint does not support changing the destination address (third parameter). Names currently remain at the same address after update, preventing ownership transfer.

**Current Code:**
```go
// TODO: Support changing the destination address (third parameter)
// For now, the name remains at the same address
```

**Expected Behavior:**
The `name_update` RPC should accept a third parameter for the destination address, allowing users to transfer name ownership to a different address.

**Impact:**
- Users cannot transfer names to different addresses via RPC
- Limits functionality compared to Namecoin Core
- Reduces usability for name trading/transfer scenarios

**Fix Required:**
1. Update `name_update` RPC signature to accept optional third parameter (destination address)
2. Validate destination address format
3. Build NAME_UPDATE transaction with new destination address in script
4. Update transaction signing to work with address changes
5. Add tests for address transfer functionality

---

### BUG-002: Incomplete transaction deserialization in test vectors
**Location:** `chain/testvectors_test.go:131`  
**Severity:** LOW  
**Status:** **UNRESOLVED**  
**Type:** Missing Test Implementation

**Description:**
The test vector framework for transactions is incomplete. Transaction test vectors cannot be validated because deserialization and validation logic is not implemented.

**Current Code:**
```go
// TODO: Implement transaction deserialization and validation
// - Deserialize wire.MsgTx from bytes
// - Parse name operation scripts
// - Validate against consensus rules
// - Check expected validity matches actual
```

**Expected Behavior:**
Transaction test vectors should be:
1. Deserialized from hex data
2. Parsed for name operation scripts
3. Validated against consensus rules
4. Compared against expected validity

**Impact:**
- Cannot validate transaction test vectors from Namecoin Core
- Missing test coverage for transaction validation
- Potential consensus divergence not caught by tests

**Fix Required:**
1. Implement `wire.MsgTx` deserialization in test framework
2. Add name operation script parsing
3. Integrate transaction validation logic
4. Add assertions for expected vs actual validity
5. Create sample transaction test vectors

---

### BUG-003: Missing testnet checkpoints
**Location:** `config/namecoin_params.go:293`  
**Severity:** LOW  
**Status:** **UNRESOLVED**  
**Type:** Missing Configuration

**Description:**
Testnet configuration only has genesis block checkpoint. Additional checkpoints from Namecoin Core should be added for reorg protection and faster sync verification.

**Current Code:**
```go
Checkpoints: []chaincfg.Checkpoint{
    {Height: 0, Hash: &testNetGenesisHash},
    // TODO: Add additional testnet checkpoints from Namecoin Core
},
```

**Expected Behavior:**
Testnet should have multiple checkpoints at significant heights (e.g., every 50,000-100,000 blocks) matching Namecoin Core checkpoints.

**Impact:**
- Reduced reorg protection on testnet
- Slower sync verification
- Missing safety checks for testnet development

**Fix Required:**
1. Extract testnet checkpoints from Namecoin Core source
2. Add checkpoint hashes and heights to NamecoinTestNetParams
3. Verify checkpoint hashes against Namecoin testnet blockchain
4. Add tests to validate checkpoint integrity
5. Document checkpoint sources in comments

---

## RESOLUTION TRACKING

| Bug ID | Description | Status | Resolved Date | Commit |
|--------|-------------|--------|---------------|--------|
| BUG-001 | NAME_UPDATE destination address | UNRESOLVED | - | - |
| BUG-002 | Transaction test vectors | UNRESOLVED | - | - |
| BUG-003 | Testnet checkpoints | UNRESOLVED | - | - |

---

## PRIORITY ORDER

1. **BUG-001** (MEDIUM) - NAME_UPDATE destination address support
2. **BUG-002** (LOW) - Transaction test vectors
3. **BUG-003** (LOW) - Testnet checkpoints

---

*Last Updated: 2026-01-02*
