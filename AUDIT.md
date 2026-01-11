# Functional Audit: nmcd

**Audit Date:** January 11, 2026  
**nmcd Version:** v0.1.0 (development)  
**Auditor:** Comprehensive code analysis  
**Scope:** Verification of README.md documentation vs actual implementation

---

## AUDIT SUMMARY

| Category | Count |
|----------|-------|
| **CRITICAL BUG** | 0 |
| **FUNCTIONAL MISMATCH** | 2 (1 resolved) |
| **MISSING FEATURE** | 2 |
| **EDGE CASE BUG** | 1 |
| **PERFORMANCE ISSUE** | 0 |

**Overall Assessment:** The codebase is well-implemented and production-ready. Minor discrepancies exist between documentation and implementation, primarily around RPC methods and client features that are documented but not fully implemented.

---

## DETAILED FINDINGS

---

### ~~FUNCTIONAL MISMATCH: sendrawtransaction RPC Not Implemented~~ ✅ RESOLVED

**Status:** Fixed - Implemented `sendrawtransaction` RPC method in `rpc/server.go`

**Original Issue:**  
**File:** rpc/server.go:309-365  
**Severity:** Medium  
**Description:** The `sendrawtransaction` RPC method was listed in `docs/development/PROTOCOL_COMPLIANCE_AUDIT.md` as a "Standard Method" but was not implemented in the RPC server.  
**Resolution:** Implemented the `sendRawTransaction` method which:
- Accepts hex-encoded raw transaction as parameter
- Decodes and deserializes the transaction
- Broadcasts to the network via peer manager (validates and adds to mempool)
- Returns the transaction hash on success
- Added comprehensive unit tests in `rpc/sendrawtransaction_test.go`

---

### FUNCTIONAL MISMATCH: name_pending Returns Empty List Always

**File:** rpc/server.go:1371-1387  
**Severity:** Low  
**Description:** The `name_pending` RPC method is implemented but always returns an empty array. It does not actually parse mempool transactions for pending name operations.  
**Expected Behavior:** Per Namecoin Core compatibility, `name_pending` should return pending name operations from the mempool.  
**Actual Behavior:** Always returns `[]` regardless of mempool state. Code comments explicitly state: "Currently nmcd does not have a dedicated pending name operations tracker."  
**Impact:** Applications relying on `name_pending` to monitor unconfirmed name operations will not see pending operations. This is a known limitation documented in the code comments.  
**Reproduction:** Create a name operation transaction, check `name_pending` before it's confirmed.  
**Code Reference:**
```go
// rpc/server.go:1371-1387
func (s *Server) namePending(req *Request) *Response {
    // Currently nmcd does not have a dedicated pending name operations tracker.
    // The mempool stores transactions but doesn't parse name operations from them.
    // Return an empty list for now - this is valid behavior when no names are pending.
    result := []map[string]interface{}{}
    return &Response{
        Jsonrpc: "2.0",
        Result:  result,
        ID:      req.ID,
    }
}
```

---

### FUNCTIONAL MISMATCH: EmbeddedClient TransferTo Not Implemented

**File:** client/embedded.go:687-699  
**Severity:** Medium  
**Description:** The `UpdateName` method in `EmbeddedClient` advertises a `TransferTo` option in the `UpdateOpts` struct, but it does not actually support name transfers to different addresses.  
**Expected Behavior:** According to client/types.go:180-182, the `TransferTo` field should transfer the name to a new address.  
**Actual Behavior:** When `TransferTo` is set to a different address than the current owner, the method returns an error: "name transfers (TransferTo) require network integration (coming in future phase)".  
**Impact:** Applications using the embedded client cannot transfer name ownership to other addresses. Only daemon mode (RPC) supports transfers through `name_update ["name", "value", "address"]`.  
**Reproduction:** Call `UpdateName` with `opts.TransferTo` set to a different address than the name owner.  
**Code Reference:**
```go
// client/embedded.go:687-699
if opts.TransferTo != "" {
    // For now, only same-address "transfers" are allowed and treated as no-transfer
    if opts.TransferTo != nameRecord.Address {
        return nil, fmt.Errorf("name transfers (TransferTo) require network integration (coming in future phase)")
    }
    // Transferring to same address is redundant but allowed
    c.logger.Warn("TransferTo address matches current owner - transfer is redundant and will be ignored",
        "address", opts.TransferTo,
        "name", name)
    destAddr = nil
}
```

---

### MISSING FEATURE: No getbalance or listunspent RPC Methods

**File:** rpc/server.go  
**Severity:** Low  
**Description:** Common wallet RPC methods `getbalance` and `listunspent` are not implemented. These are standard Bitcoin/Namecoin wallet RPC methods that users might expect.  
**Expected Behavior:** Wallet-enabled nodes typically provide balance querying capabilities.  
**Actual Behavior:** Methods not available. Users cannot query wallet balance or list unspent outputs via RPC.  
**Impact:** Applications or users wanting to check wallet balance must track UTXOs externally or use the embedded client API.  
**Reproduction:** Call `getbalance` or `listunspent` RPC methods.  
**Code Reference:**
```go
// Not implemented in rpc/server.go processRequest switch statement
```

---

### MISSING FEATURE: No Transaction Index for Historical TX Lookups

**File:** rpc/server.go:1908-2016  
**Severity:** Low  
**Description:** The `getrawtransaction` RPC only searches the last 1000 blocks for transactions. There is no full transaction index for efficient historical transaction lookups.  
**Expected Behavior:** Full nodes typically maintain a transaction index for O(1) lookups of any historical transaction.  
**Actual Behavior:** Linear search through recent blocks only. Transactions older than ~1000 blocks from the tip are not found.  
**Impact:** Applications requiring historical transaction data may fail to find older transactions. Comment in code explicitly documents this: "Limit search to last 1000 blocks to prevent excessive lookups."  
**Reproduction:** Query for a transaction that was confirmed more than 1000 blocks ago.  
**Code Reference:**
```go
// rpc/server.go:1969-1977
// Limit search to last 1000 blocks to prevent excessive lookups
// For a full transaction index, use btcd's txindex
startHeight := bestHeight - 1000
if startHeight < 0 {
    startHeight = 0
}
```

---

### EDGE CASE BUG: WaitForConfirmation Only Searches 100 Blocks

**File:** client/embedded.go:1063-1086  
**Severity:** Low  
**Description:** The `WaitForConfirmation` method in `EmbeddedClient` only searches the last 100 blocks to find a transaction. If synchronization is slow or the polling interval is long, transactions confirmed further back may not be found.  
**Expected Behavior:** Method should reliably find any confirmed transaction.  
**Actual Behavior:** Only searches last 100 blocks. Comment notes: "For now, we check the last 100 blocks (should cover most cases)."  
**Impact:** In rare edge cases with very slow syncing or very long poll intervals, a confirmed transaction might not be detected, causing `WaitForConfirmation` to time out despite the transaction being confirmed.  
**Reproduction:** Have a transaction confirmed, then experience a sync delay of more than 100 blocks before the next poll.  
**Code Reference:**
```go
// client/embedded.go:1065-1070
maxBlocksToSearch := int32(100)
startHeight := currentHeight - maxBlocksToSearch
if startHeight < 0 {
    startHeight = 0
}
```

---

## VERIFIED FEATURES

The following documented features are correctly implemented:

### Library Features (All Working)
- ✅ Name Resolution with expiration checking
- ✅ Name Registration (NAME_NEW → NAME_FIRSTUPDATE two-phase)
- ✅ Name Updates (NAME_UPDATE with value changes)
- ✅ Name Listing with namespace/address/pattern filters
- ✅ Embedded Mode (in-process blockchain and database)
- ✅ Daemon Mode (JSON-RPC interface)
- ✅ Auto-Detection mode selection
- ✅ Thread-Safe operations with mutex protection
- ✅ Context Support for timeouts and cancellation

### Daemon Features (All Working)
- ✅ Pure Go implementation with btcd libraries
- ✅ bbolt-backed NameDatabase
- ✅ Blockchain integration with name validation hooks
- ✅ Block Synchronization via headers-first protocol
- ✅ P2P networking using btcd/peer
- ✅ Transaction Mempool with validation and relay
- ✅ JSON-RPC server using net/http
- ✅ Health (`/health`) and Readiness (`/ready`) endpoints
- ✅ Prometheus metrics endpoint

### RPC Methods (All Documented Methods Working)
- ✅ getinfo, getblockcount, getbestblockhash
- ✅ getconnectioncount, getpeerinfo, getmetrics
- ✅ name_show, name_list, name_history
- ✅ name_new, name_firstupdate, name_update
- ✅ name_scan (prefix matching with pagination)
- ✅ getnewaddress, listaddresses
- ✅ walletpassphrase, walletlock, encryptwallet
- ✅ getblock, getblockhash, getrawtransaction

### Protocol Compliance (All Verified)
- ✅ Namecoin network magic bytes (mainnet: 0xf9beb4fe)
- ✅ Protocol version 70015
- ✅ AuxPoW support for blocks ≥ 19,200
- ✅ Name expiration at 36,000 blocks
- ✅ Name operation fees (0.01 NMC for FIRSTUPDATE/UPDATE)
- ✅ All test vectors pass (genesis through first halving)

---

## RECOMMENDATIONS

1. ~~**Fix sendrawtransaction Documentation Mismatch**: Update `docs/development/PROTOCOL_COMPLIANCE_AUDIT.md` to remove `sendrawtransaction` from the "Standard Methods" list, or implement the method.~~ ✅ DONE - Implemented sendrawtransaction RPC

2. ~~**Add sendrawtransaction RPC**: Implement `sendrawtransaction` for compatibility with existing tooling expecting standard Bitcoin/Namecoin RPC.~~ ✅ DONE

3. **Document EmbeddedClient Limitations**: Add clear documentation that `TransferTo` is only supported in daemon mode, not embedded mode.

4. **Consider Transaction Index** (Future): For production use with full blockchain history, consider implementing a transaction index using btcd's txindex.

5. **Expand name_pending** (Future): When resources permit, implement proper mempool parsing for `name_pending` to support pending name operation tracking.

---

## AUDIT METHODOLOGY

1. **Documentation Review**: Thorough analysis of README.md and `docs/development/PROTOCOL_COMPLIANCE_AUDIT.md`
2. **Dependency Analysis**: Mapped import relationships across all packages
3. **Code Verification**: Traced documented features to implementation
4. **Build Verification**: Confirmed clean build with no errors
5. **Test Verification**: Confirmed all existing tests pass

---

## CONCLUSION

nmcd is a well-implemented Namecoin library and daemon. The core functionality matches documentation, with only minor discrepancies in auxiliary RPC methods and embedded client features. The codebase follows Go best practices with proper mutex protection, error handling, and interface-based design. The identified issues are non-critical and well-documented in code comments where applicable.

**Production Readiness:** ✅ Ready for mainnet use  
**API Stability:** Stable for documented features  
**Code Quality:** High - follows Go idioms and best practices

---

*Last Updated: January 11, 2026*
