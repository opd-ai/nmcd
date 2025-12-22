# nmcd Functional Audit Report

**Audit Date:** 2025-12-22  
**Auditor:** Automated Code Audit  
**Codebase Version:** Current HEAD  
**Last Updated:** 2025-12-22  

---

## AUDIT SUMMARY

| Category | Count | Severity Distribution |
|----------|-------|----------------------|
| CRITICAL BUG | 0 | - |
| FUNCTIONAL MISMATCH | 3 (2 fixed) | High: 1, Medium: 2 |
| MISSING FEATURE | 5 | High: 1, Medium: 3, Low: 1 |
| EDGE CASE BUG | 1 | Medium: 1 |
| PERFORMANCE ISSUE | 0 | - |
| DOCUMENTATION DISCREPANCY | 1 | Low: 1 |

**Total Findings: 12 (2 fixed, 10 remaining)**

The codebase is well-structured with good test coverage. The primary concerns are around incomplete integration between components and missing address tracking for name records. No critical bugs that would cause crashes or data corruption were identified.

---

## DETAILED FINDINGS

### [FIXED] FUNCTIONAL MISMATCH: Address Field Not Populated in Name Records

**File:** chain/blockchain.go:207-241  
**Severity:** High  
**Status:** ✅ FIXED  
**Fix Date:** 2025-12-22  

**Description:** When creating NameRecord entries during NAME_FIRSTUPDATE and NAME_UPDATE operations in `updateNameDatabase()`, the `Address` field is never populated. The NameRecord struct has an Address field (namedb/namedb.go:42), and the RPC responses include this field, but it will always be empty.

**Fix Applied:**
- Added `extractAddressFromNameScript()` function to extract the owner address from the P2PKH portion of name scripts
- Updated `updateNameDatabase()` to populate the `Address` field when creating records for NAME_FIRSTUPDATE and NAME_UPDATE operations
- Added comprehensive unit tests for address extraction

---

### [FIXED] FUNCTIONAL MISMATCH: UpdatedAt Field Not Populated

**File:** chain/blockchain.go:207-241  
**Severity:** Low  
**Status:** ✅ FIXED  
**Fix Date:** 2025-12-22  

**Description:** The `UpdatedAt` field in NameRecord is never set when processing blocks. This field is defined in namedb/namedb.go:43 and is properly encoded/decoded, but the chain package doesn't populate it.

**Fix Applied:**
- Updated `updateNameDatabase()` to set `UpdatedAt` using the block's timestamp (`block.MsgBlock().Header.Timestamp`) when creating records for NAME_FIRSTUPDATE and NAME_UPDATE operations
- This ensures deterministic replay and historical accuracy when reprocessing the blockchain

---

### FUNCTIONAL MISMATCH: onBlock Handler Does Not Process Blocks

**File:** network/peermgr.go:225-228  
**Severity:** High  
**Description:** The `onBlock` message handler receives blocks from peers but does not actually process them through the blockchain. There is a comment indicating this is where ProcessBlock should be called, but the implementation is empty.

**Expected Behavior:** Per README.md "Handles block/tx propagation", received blocks should be validated and added to the blockchain.

**Actual Behavior:** Blocks are received but silently discarded. The blockchain state never advances from peer-received blocks.

**Impact:** The node cannot synchronize with the network. It will only have the genesis block unless blocks are submitted through some other mechanism.

**Reproduction:** Start the node and connect to peers. The block count will remain at 0 (or genesis) even after receiving blocks.

**Code Reference:**
```go
func (pm *PeerManager) onBlock(p *peer.Peer, msg *wire.MsgBlock, buf []byte) {
    // Handle block message - would process with blockchain
    // This is where we'd call blockchain.ProcessBlock
}
```

---

### FUNCTIONAL MISMATCH: Blockchain Notifications Not Connected

**File:** chain/blockchain.go:490-508, cmd/nmcd/main.go  
**Severity:** Medium  
**Description:** The `HandleBlockchainNotification` method exists to handle blockchain reorganization events, but it is never registered with btcd's blockchain notification system. The method handles NTBlockConnected and NTBlockDisconnected events, but no subscription is created.

**Expected Behavior:** The blockchain should subscribe to notifications to properly handle chain reorganizations and update the name database accordingly.

**Actual Behavior:** HandleBlockchainNotification is never called. Chain reorganizations would leave the name database in an inconsistent state with the blockchain.

**Impact:** If a blockchain reorganization occurs (which shouldn't happen much on an isolated node), the name database would become inconsistent with the actual chain state.

**Reproduction:** Induce a chain reorganization - the name database will not be updated.

**Code Reference:**
```go
func (bc *BlockChain) HandleBlockchainNotification(notification *blockchain.Notification) {
    bc.mu.Lock()
    defer bc.mu.Unlock()
    // This method exists but is never registered/called
    switch notification.Type {
    case blockchain.NTBlockConnected:
        // ...
    case blockchain.NTBlockDisconnected:
        // ...
    }
}
```

---

### FUNCTIONAL MISMATCH: name_update RPC Does Not Broadcast Transaction

**File:** rpc/server.go:277-423  
**Severity:** Medium  
**Description:** The `name_update` RPC method creates a transaction script and returns it, but explicitly states in the response that "Broadcasting requires UTXO management." The transaction is never actually broadcast to the network.

**Expected Behavior:** Per README.md documentation, `name_update` should "Update an existing name's value" and the wallet should handle transaction creation and broadcasting.

**Actual Behavior:** The RPC returns a prepared script with status "prepared" but the name is never actually updated on the blockchain.

**Impact:** Users cannot update names through the RPC API. The method gives the appearance of functionality but requires external tooling to actually perform updates.

**Reproduction:** Call `name_update` RPC - it returns success but the name's value doesn't change in the blockchain.

**Code Reference:**
```go
result := map[string]interface{}{
    "name":             name,
    "value":            newValue,
    // ...
    "status":           "prepared",
    "message":          "NAME_UPDATE transaction prepared. Broadcasting requires UTXO management.",
}
```

---

### MISSING FEATURE: getnewaddress and listaddresses RPC Methods

**File:** rpc/server.go, README.md:117-119  
**Severity:** Medium  
**Description:** The README.md documents `getnewaddress` and `listaddresses` RPC methods as "(planned)" but the wallet package has `GenerateKey()` and `GetAddresses()` methods that could implement these. The RPC server does not expose these wallet methods.

**Expected Behavior:** README indicates these are planned features. The underlying wallet implementation exists.

**Actual Behavior:** The RPC methods are not implemented despite the wallet having the necessary functionality.

**Impact:** Users cannot generate new addresses or list existing addresses through the RPC API, despite the wallet supporting these operations internally.

**Reproduction:** Call `getnewaddress` or `listaddresses` RPC - returns "Method not found" error.

**Code Reference:**
```go
// In rpc/server.go processRequest - these cases are missing:
// case "getnewaddress":
// case "listaddresses":
```

---

### MISSING FEATURE: No Seed Node Support

**File:** network/peermgr.go, config/config.go  
**Severity:** Medium  
**Description:** There is no implementation of DNS seed resolution or hardcoded seed nodes for initial peer discovery. The only way to connect to peers is via the `-addpeer` command line flag.

**Expected Behavior:** A blockchain node typically has hardcoded seed nodes or DNS seeds for initial peer discovery.

**Actual Behavior:** The node starts with zero peer connections unless `-addpeer` is explicitly specified.

**Impact:** New nodes have no automatic way to discover the network and synchronize. Manual peer configuration is required.

**Reproduction:** Start the node without `-addpeer` flag - it will have 0 connections.

**Code Reference:**
```go
// config/config.go - no seed nodes defined
return &Config{
    // ...
    AddPeers:    []string{},  // Empty by default
}
```

---

### MISSING FEATURE: No RPC Authentication

**File:** rpc/server.go  
**Severity:** High  
**Description:** The RPC server accepts all incoming requests without any authentication. There is no username/password, API key, or other authentication mechanism.

**Expected Behavior:** Blockchain RPC servers typically require authentication (rpcuser/rpcpassword) to prevent unauthorized access.

**Actual Behavior:** Any client that can reach the RPC port (default 127.0.0.1:8336) can execute all RPC commands including wallet operations.

**Impact:** Security vulnerability - if the RPC port is exposed (e.g., via misconfiguration or tunneling), anyone can access wallet functions and name operations.

**Reproduction:** Send any RPC request to the server without credentials - it will be processed.

**Code Reference:**
```go
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    // No authentication check here
    var req Request
    // ...
}
```

---

### MISSING FEATURE: No Mempool Implementation

**File:** (none - feature absent)  
**Severity:** Medium  
**Description:** There is no transaction mempool implementation. The node cannot receive, validate, or store unconfirmed transactions.

**Expected Behavior:** A full node typically maintains a mempool of unconfirmed transactions for mining and relay.

**Actual Behavior:** Unconfirmed transactions cannot be stored or processed. The `onTx` handler in peermgr.go is empty.

**Impact:** The node cannot participate in transaction relay or provide mempool-related RPC functionality.

**Reproduction:** Attempt to submit an unconfirmed transaction - it cannot be stored or relayed.

**Code Reference:**
```go
func (pm *PeerManager) onTx(p *peer.Peer, msg *wire.MsgTx) {
    // Handle transaction message
    // Empty implementation
}
```

---

### MISSING FEATURE: No Block Request/Sync Mechanism

**File:** network/peermgr.go  
**Severity:** Low  
**Description:** While the node responds to inventory messages by requesting data, there is no implementation of initial block download (IBD) or "getheaders"/"getblocks" message handling for synchronizing with peers.

**Expected Behavior:** A node should actively request blocks it's missing during initial sync.

**Actual Behavior:** The node is passive - it only requests blocks when peers announce them via inventory messages.

**Impact:** Initial synchronization would be extremely slow or non-functional.

**Reproduction:** Start a fresh node - it will not actively sync blocks from peers.

**Code Reference:**
```go
// Missing: getheaders/getblocks handlers for active sync
func (pm *PeerManager) onInv(p *peer.Peer, msg *wire.MsgInv) {
    gdmsg := wire.NewMsgGetData()
    for _, inv := range msg.InvList {
        gdmsg.AddInvVect(inv)  // Only reacts to announcements
    }
    // ...
}
```

---

### EDGE CASE BUG: Potential Race in PeerManager Accept Loop

**File:** network/peermgr.go:62-94  
**Severity:** Medium  
**Description:** The `listenLoop` function spawns a goroutine for Accept() that sends on channels, but if the listener is closed while the goroutine is blocked on Accept(), the goroutine will send an error and exit, but subsequent accepts on the closed listener may cause issues.

**Expected Behavior:** Clean shutdown without goroutine leaks or race conditions.

**Actual Behavior:** The accept goroutine may leak if the listener is closed while no connection is pending, as the goroutine is blocked on Accept() which will return an error, then exit normally. However, the error channel send may race with quit channel select.

**Impact:** Minor - potential goroutine leak or race during shutdown, but unlikely to cause actual problems in practice.

**Reproduction:** Start and stop the node rapidly multiple times.

**Code Reference:**
```go
go func() {
    for {
        conn, err := listener.Accept()
        if err != nil {
            errCh <- err
            return  // Goroutine exits on error
        }
        acceptCh <- conn
    }
}()
```

---

### DOCUMENTATION DISCREPANCY: Code Size Claim

**File:** README.md:17  
**Severity:** Low  
**Description:** README.md states "~1200 lines of focused custom code" but the actual line count (excluding tests and examples) is 3044 lines across the main packages:
- config: 398 lines
- namedb: 508 lines
- chain: 605 lines
- network: 321 lines
- rpc: 525 lines
- wallet: 530 lines
- cmd/nmcd: 157 lines

**Expected Behavior:** Documentation should accurately reflect codebase size.

**Actual Behavior:** The codebase is approximately 2.5x larger than documented. This may be an outdated claim from when the codebase was smaller, or the original author may have counted only the core logic excluding boilerplate.

**Impact:** Minor - sets incorrect expectations for code review or maintenance effort.

**Reproduction:** Run `find . -name '*.go' -not -path './.git/*' -not -name '*_test.go' -not -path './examples/*' | xargs wc -l`

---

## POSITIVE OBSERVATIONS

The audit also identified several well-implemented aspects:

1. **Thread Safety:** Proper mutex usage throughout the codebase (RWMutex for read-heavy operations)
2. **Error Handling:** Consistent error wrapping with `fmt.Errorf` and `%w` for error chains
3. **Test Coverage:** Comprehensive tests for namedb, chain parsing, wallet operations
4. **Clean Architecture:** Clear separation of concerns between packages
5. **Resource Management:** Proper cleanup with deferred Close() calls
6. **Reorg Handling:** Good rollback logic for blockchain reorganizations in the name database

---

## RECOMMENDATIONS

1. **High Priority:**
   - Implement block processing in `onBlock` handler
   - Add RPC authentication
   - Populate Address field when creating name records

2. **Medium Priority:**
   - Implement `getnewaddress` and `listaddresses` RPC methods
   - Add seed node support for peer discovery
   - Complete `name_update` transaction broadcasting

3. **Low Priority:**
   - Set UpdatedAt timestamps on name records
   - Update README to reflect actual code size
   - Add mempool for transaction relay

---

## AUDIT METHODOLOGY

This audit followed a systematic approach:

1. **Documentation Review:** Extracted 15+ functional requirements from README.md
2. **Dependency Analysis:** Mapped package dependencies (config → namedb → chain → network/rpc → cmd)
3. **Code Analysis:** Reviewed all 3044 lines of non-test Go code
4. **Test Verification:** All 67 existing tests pass
5. **Build Verification:** Project builds successfully with `go build -v ./cmd/nmcd`

---

*End of Audit Report*
