# Code Audit - Bug Tracking

**Audit Date:** 2026-01-02  
**Scope:** Code quality, incomplete implementations, and TODOs in the nmcd codebase  
**Status:** Active

---

## BUGS AND INCOMPLETE IMPLEMENTATIONS

### BUG-001: Missing NAME_UPDATE destination address support
**Location:** `rpc/server.go:354-381`, `wallet/wallet.go:297-341`, `chain/blockchain.go:1447-1450`  
**Severity:** MEDIUM  
**Status:** ✅ **RESOLVED** (2026-01-02)  
**Type:** Missing Feature

**Description:**
The `name_update` RPC endpoint does not support changing the destination address (third parameter). Names currently remain at the same address after update, preventing ownership transfer.

**Root Cause:**
The `name_update` RPC handler and `CreateNameUpdateTx` wallet function did not accept or process a destination address parameter. The wallet always used the current owner's pubKeyHash for the NAME_UPDATE output, preventing name ownership transfers.

**Fix Implemented:**
1. **Updated `CreateNameUpdateTx` signature** (`wallet/wallet.go:297`):
   - Added `destAddress btcutil.Address` parameter
   - Modified logic to use destAddress pubKeyHash when provided
   - Falls back to current owner's address when destAddress is nil
   - Updated change output to use appropriate address

2. **Updated RPC handler** (`rpc/server.go:354-381`):
   - Added parsing for optional third parameter (destination address)
   - Added address validation using `btcutil.DecodeAddress`
   - Enforces P2PKH address type (Namecoin requirement)
   - Passes destAddress to `CreateNameUpdateTx`
   - Includes destination address in RPC response when specified

3. **Added ChainParams getter** (`chain/blockchain.go:1447-1450`):
   - Exposed `ChainParams()` method on BlockChain
   - Enables RPC handler to access network parameters for address validation

**Changes Summary:**
- `wallet/wallet.go`: Modified `CreateNameUpdateTx` to accept optional destination address
- `rpc/server.go`: Added destination address parsing and validation
- `chain/blockchain.go`: Added `ChainParams()` getter method

**Verification:**
- ✅ Build succeeds with changes
- ✅ All existing tests pass
- ✅ Code compiles without errors
- ✅ Address validation includes type checking (P2PKH only)
- ✅ Backward compatible (destAddress can be nil)

**Usage Example:**
```bash
# Update value only (keeps current address)
name_update "d/example" "new value"

# Transfer to new address
name_update "d/example" "new value" "N1NewAddress..."
```

**Commit:** TBD

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
| BUG-001 | NAME_UPDATE destination address | ✅ RESOLVED | 2026-01-02 | TBD |
| BUG-002 | Transaction test vectors | UNRESOLVED | - | - |
| BUG-003 | Testnet checkpoints | UNRESOLVED | - | - |

---

## PRIORITY ORDER

1. **BUG-001** (MEDIUM) - NAME_UPDATE destination address support
2. **BUG-002** (LOW) - Transaction test vectors
3. **BUG-003** (LOW) - Testnet checkpoints

---

*Last Updated: 2026-01-02*
