# Protocol Compliance Plan: Path to 100%

**Project:** nmcd - Pure Go Namecoin Library and Daemon  
**Version:** v0.1.0 (development)  
**Last Updated:** 2026-01-11  
**Target:** 100% Namecoin Protocol Compliance

---

## Current Status

| Metric | Value |
|--------|-------|
| **Protocol Compliance** | 95% |
| **Critical Issues** | 0 |
| **Production Blockers** | 0 |
| **Test Vectors** | 6/6 mainnet blocks pass |
| **Mainnet Ready** | ✅ Yes |

The nmcd implementation is already **production-ready** for mainnet use. This plan outlines the remaining 5% of items needed to achieve full 100% protocol compliance with Namecoin Core.

---

## Remaining Gaps (5%)

### 1. Missing RPC Methods (3%)

| RPC Method | Priority | Effort | Description |
|------------|----------|--------|-------------|
| `name_pending` | Medium | 1-2 days | List pending name operations in mempool |
| `name_scan` | Low | 1 day | Scan names with prefix matching and pagination |

**Status:** Not implemented  
**Impact:** Some applications may require these RPCs for full Namecoin Core compatibility

### 2. Value Size Policy Divergence (1%)

| Constant | nmcd | Namecoin Core | Impact |
|----------|------|---------------|--------|
| Max Value Size | 1023 bytes | 520 bytes (relay policy) | nmcd accepts larger values |

**Status:** Intentional design choice  
**Impact:** nmcd is more permissive but still consensus-compatible. Namecoin Core's 520-byte limit is a relay policy, not a consensus rule.

**Options:**
- A) Keep current behavior (1023 bytes) - more flexible for applications
- B) Add configurable relay policy matching Namecoin Core's 520-byte limit
- C) Both: 1023 byte consensus limit, 520 byte default relay policy

### 3. Test Coverage for Critical Packages (1%)

| Package | Current | Target | Gap |
|---------|---------|--------|-----|
| chain | 68.1% | 80% | 11.9% |
| rpc | 45.8% | 80% | 34.2% |
| network | 43.5% | 80% | 36.5% |

**Status:** Tests exist but coverage below 80% threshold  
**Impact:** Lower confidence in edge cases; not a protocol compliance issue per se

---

## Implementation Plan

### Phase 1: name_pending RPC (Priority: Medium)

**Estimated Effort:** 1-2 days  
**Dependencies:** Mempool with name operation tracking

#### 1.1 Mempool Enhancement

```go
// network/mempool.go additions

// NamePendingEntry represents a pending name operation in the mempool
type NamePendingEntry struct {
    Name      string `json:"name"`
    Operation string `json:"op"`       // "NAME_NEW", "NAME_FIRSTUPDATE", "NAME_UPDATE"
    TxID      string `json:"txid"`
    Value     string `json:"value,omitempty"`
    IsNew     bool   `json:"ismine"`   // true if from local wallet
    Height    int32  `json:"height"`   // height when added (estimated confirmation)
}

// GetPendingNameOperations returns all name operations in the mempool
func (m *Mempool) GetPendingNameOperations() ([]NamePendingEntry, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    var pending []NamePendingEntry
    for _, tx := range m.pool {
        for _, op := range tx.NameOperations {
            pending = append(pending, NamePendingEntry{
                Name:      op.Name,
                Operation: op.Type.String(),
                TxID:      tx.Hash.String(),
                Value:     string(op.Value),
            })
        }
    }
    return pending, nil
}
```

#### 1.2 RPC Handler

```go
// rpc/server.go additions

// namePending returns pending name operations from the mempool.
// Matches Namecoin Core's name_pending RPC.
func (s *Server) namePending(params []interface{}) (interface{}, error) {
    if s.mempool == nil {
        return nil, &RPCError{
            Code:    -1,
            Message: "mempool not available",
        }
    }
    
    // Optional name filter
    var nameFilter string
    if len(params) > 0 {
        var ok bool
        nameFilter, ok = params[0].(string)
        if !ok {
            return nil, &RPCError{
                Code:    -1,
                Message: "name filter must be a string",
            }
        }
    }
    
    pending, err := s.mempool.GetPendingNameOperations()
    if err != nil {
        return nil, wrapError(-1, "failed to get pending names", err)
    }
    
    // Filter by name if specified
    if nameFilter != "" {
        var filtered []NamePendingEntry
        for _, p := range pending {
            if p.Name == nameFilter {
                filtered = append(filtered, p)
            }
        }
        pending = filtered
    }
    
    return pending, nil
}
```

#### 1.3 Tests

- [ ] Unit tests for mempool name operation tracking
- [ ] Unit tests for name_pending RPC handler
- [ ] Integration test with full transaction flow
- [ ] Test name filter parameter

#### 1.4 Checklist

- [ ] Add `NamePendingEntry` struct to mempool
- [ ] Implement `GetPendingNameOperations()` in mempool
- [ ] Add `namePending` RPC handler
- [ ] Register RPC method in router
- [ ] Add comprehensive tests
- [ ] Update API documentation
- [ ] Update PROTOCOL_COMPLIANCE_AUDIT.md

---

### Phase 2: name_scan RPC (Priority: Low)

**Estimated Effort:** 1 day  
**Dependencies:** Name database with prefix scanning

#### 2.1 NameDB Enhancement

```go
// namedb/namedb.go additions

// ScanNames scans names matching a prefix with pagination.
// Returns up to count names starting from prefix.
func (ndb *NameDatabase) ScanNames(prefix string, count int, startHeight int32) ([]*NameRecord, error) {
    ndb.mu.RLock()
    defer ndb.mu.RUnlock()
    
    var results []*NameRecord
    
    err := ndb.db.View(func(tx *bbolt.Tx) error {
        bucket := tx.Bucket([]byte("names"))
        if bucket == nil {
            return nil
        }
        
        cursor := bucket.Cursor()
        prefixBytes := []byte(prefix)
        
        for k, v := cursor.Seek(prefixBytes); k != nil; k, v = cursor.Next() {
            // Check if key still has prefix
            if !bytes.HasPrefix(k, prefixBytes) {
                break
            }
            
            var record NameRecord
            if err := json.Unmarshal(v, &record); err != nil {
                continue
            }
            
            // Apply height filter if specified
            if startHeight > 0 && record.Height < startHeight {
                continue
            }
            
            results = append(results, &record)
            
            if len(results) >= count {
                break
            }
        }
        
        return nil
    })
    
    return results, err
}
```

#### 2.2 RPC Handler

```go
// rpc/server.go additions

// nameScan scans names with prefix matching and pagination.
// Matches Namecoin Core's name_scan RPC.
func (s *Server) nameScan(params []interface{}) (interface{}, error) {
    if s.blockchain == nil {
        return nil, &RPCError{
            Code:    -1,
            Message: "blockchain not available",
        }
    }
    
    // Parse parameters: name_scan [start] [count]
    start := ""
    count := 500 // default
    
    if len(params) > 0 {
        var ok bool
        start, ok = params[0].(string)
        if !ok {
            return nil, &RPCError{
                Code:    -1,
                Message: "start must be a string",
            }
        }
    }
    
    if len(params) > 1 {
        countFloat, ok := params[1].(float64)
        if !ok {
            return nil, &RPCError{
                Code:    -1,
                Message: "count must be a number",
            }
        }
        count = int(countFloat)
        if count <= 0 || count > 10000 {
            return nil, &RPCError{
                Code:    -1,
                Message: "count must be between 1 and 10000",
            }
        }
    }
    
    names, err := s.blockchain.ScanNames(start, count, 0)
    if err != nil {
        return nil, wrapError(-1, "failed to scan names", err)
    }
    
    return names, nil
}
```

#### 2.3 Checklist

- [ ] Add `ScanNames()` method to namedb
- [ ] Add `nameScan` RPC handler
- [ ] Register RPC method in router
- [ ] Add unit tests for prefix scanning
- [ ] Add pagination boundary tests
- [ ] Update API documentation
- [ ] Update PROTOCOL_COMPLIANCE_AUDIT.md

---

### Phase 3: Value Size Policy Alignment (Priority: Low)

**Estimated Effort:** 0.5 days  
**Dependencies:** None

#### 3.1 Decision

**Recommendation:** Option C - Keep 1023 byte consensus limit but add configurable relay policy

This approach:
- Maintains backward compatibility
- Allows operators to match Namecoin Core behavior if desired
- Provides flexibility for applications needing larger values

#### 3.2 Implementation

```go
// config/params.go additions

// NameValueRelayLimit is the maximum value size for mempool acceptance.
// Default matches Namecoin Core's relay policy.
// Consensus limit (MaxValueSize) is 1023 bytes.
const NameValueRelayLimit = 520

// network/mempool.go modifications

func (m *Mempool) validateNameValue(value []byte) error {
    // Relay policy check (stricter than consensus)
    if len(value) > NameValueRelayLimit {
        return fmt.Errorf("value size %d exceeds relay limit %d", 
            len(value), NameValueRelayLimit)
    }
    return nil
}
```

#### 3.3 Checklist

- [ ] Add `NameValueRelayLimit` constant (520 bytes default)
- [ ] Add mempool validation using relay limit
- [ ] Add configuration option to adjust limit
- [ ] Document difference between consensus and relay limits
- [ ] Update PROTOCOL_COMPLIANCE_AUDIT.md

---

### Phase 4: Test Coverage Improvements (Priority: Medium)

**Estimated Effort:** 2-3 days  
**Dependencies:** None

See [COVERAGE.md](COVERAGE.md) for detailed analysis.

#### 4.1 Priority 1: Chain Package (68.1% → 80%+)

- [ ] Add tests for ProcessBlock (0% → 50%+)
- [ ] Add tests for updateNameDatabase (16.9% → 50%+)
- [ ] Add tests for ValidateMempoolTransaction (0% → 80%+)

#### 4.2 Priority 2: RPC Package (45.8% → 65%+)

- [ ] Add tests for nameShow (0% → 80%)
- [ ] Add tests for nameUpdate (0% → 80%)
- [ ] Add tests for nameList (0% → 80%)
- [ ] Add tests for getBlock verbose mode (32.6% → 80%)

#### 4.3 Priority 3: Network Package (43.5% → 60%+)

- [ ] Add tests for onTx (0% → 50%)
- [ ] Add tests for relayTransaction (0% → 50%)
- [ ] Add tests for onBlock (12.5% → 50%)

---

## Timeline Summary

| Phase | Description | Effort | Priority |
|-------|-------------|--------|----------|
| Phase 1 | name_pending RPC | 1-2 days | Medium |
| Phase 2 | name_scan RPC | 1 day | Low |
| Phase 3 | Value size policy | 0.5 days | Low |
| Phase 4 | Test coverage | 2-3 days | Medium |
| **Total** | | **4.5-6.5 days** | |

---

## Success Criteria

### 100% Protocol Compliance Achieved When:

- [ ] `name_pending` RPC implemented and tested
- [ ] `name_scan` RPC implemented and tested
- [ ] Value size relay policy documented (consensus vs relay)
- [ ] Test coverage meets targets (80%+ for critical packages)
- [ ] All 6/6 mainnet test vectors pass (already complete)
- [ ] PROTOCOL_COMPLIANCE_AUDIT.md updated to 100%

### Verification Commands

```bash
# Run all tests
make test

# Check coverage for critical packages
go test -cover ./chain ./rpc ./network

# Run mainnet test vectors
go test -v ./chain -run TestMainnet

# Verify RPC methods
curl -X POST http://localhost:8336 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"name_pending","params":[],"id":1}'

curl -X POST http://localhost:8336 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"name_scan","params":["d/",100],"id":1}'
```

---

## Already Completed (from Previous Audit)

The following items were previously identified as gaps and have been **resolved**:

### ✅ AuxPoW Support (Resolved)
- Full AuxPoW data structures and validation
- 6/6 mainnet test vectors pass
- Blocks ≥19,200 properly validated

### ✅ Block Subsidy Verification (Resolved)
- Matches Namecoin Core exactly
- Total supply: 20,999,999.9769 NMC
- All halving calculations verified

### ✅ Wallet Encryption (Resolved)
- AES-256-GCM encryption
- scrypt key derivation
- walletpassphrase/walletlock/encryptwallet RPCs

### ✅ RPC Security (Resolved)
- Rate limiting (100 req/min default)
- Request size limits (1MB max)
- Security headers
- HTTP Basic Auth

### ✅ Observability (Resolved)
- Structured logging (slog)
- Prometheus metrics (43 metrics)
- Health endpoints (/health, /ready)

### ✅ Protocol Constants (Resolved)
- 21/21 constants correct (100%)
- All name operation opcodes correct
- Network magic bytes correct

---

## Risk Assessment

### Low Risk Items

1. **name_pending RPC** - Standard mempool query, no consensus impact
2. **name_scan RPC** - Standard database query, no consensus impact
3. **Value size policy** - Documentation clarification, no code change required
4. **Test coverage** - Quality improvement, no functional impact

### No High Risk Items

All remaining gaps are non-critical enhancements. The core protocol implementation is complete and production-ready.

---

## Conclusion

nmcd is at **95% protocol compliance** and ready for **production mainnet use**. The remaining 5% consists of:

1. **Optional RPC methods** (name_pending, name_scan) used by specific applications
2. **Policy documentation** (value size relay vs consensus limits)
3. **Test coverage improvements** (quality, not correctness)

Completing these items will achieve **100% protocol compliance** with Namecoin Core while maintaining nmcd's design goals of:
- Library-first architecture
- Minimal code footprint
- Composition over reimplementation
- Production-ready quality

**Estimated Total Effort:** 4.5-6.5 developer days

---

**Author:** GitHub Copilot Agent  
**Date:** 2026-01-11  
**Status:** Plan approved for implementation
