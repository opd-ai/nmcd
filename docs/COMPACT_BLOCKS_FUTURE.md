# Compact Block Relay (BIP152) - Future Enhancement

**Status:** Deferred to future release (v1.1+)  
**Priority:** P3 (Low) - Performance optimization, not critical for production  
**Estimated Effort:** 3-4 days

## Overview

Compact Block Relay (BIP152) is a bandwidth optimization technique that reduces block propagation latency by avoiding redundant transmission of transactions. When a new block is mined, most nodes already have the majority of its transactions in their mempools. BIP152 allows nodes to reconstruct blocks from their mempool rather than downloading all transaction data.

## Benefits

1. **Reduced Bandwidth**: ~90% reduction in block data transmission (only send transaction IDs, not full transactions)
2. **Faster Propagation**: 30-50% reduction in block relay latency (critical for miners to prevent orphaned blocks)
3. **Network Efficiency**: Less congestion during block announcements
4. **Improved User Experience**: Faster block confirmations perceived by users

## Current Implementation Status

nmcd currently uses **full block relay**:
- Entire blocks transmitted via `MsgBlock` wire messages
- Simple, reliable, but bandwidth-intensive
- Adequate for current network conditions and testnet/regtest use

## Implementation Plan (When Prioritized)

### Phase 1: Data Structures (1 day)

1. Define compact block messages:
   - `MsgCmpctBlock`: Header + short transaction IDs (6-byte hashes)
   - `MsgGetBlockTxn`: Request missing transactions by index
   - `MsgBlockTxn`: Provide requested transactions

2. Add short transaction ID calculation:
   - SipHash-2-4 with block-specific key (header hash derived)
   - 6-byte output for collision resistance

### Phase 2: High Bandwidth Mode (1 day)

1. Implement `MsgSendCmpct` negotiation:
   - Request high-bandwidth mode from select peers
   - Track which peers support compact blocks

2. Add compact block sending:
   - On block announce, send `MsgCmpctBlock` to high-bandwidth peers
   - Include header + short IDs + prefilled transactions (coinbase, name ops)

### Phase 3: Block Reconstruction (1 day)

1. Implement mempool lookup by short ID:
   - Build index of mempool transactions by short ID
   - Handle short ID collisions gracefully

2. Add block reconstruction logic:
   - Receive `MsgCmpctBlock`
   - Look up transactions in mempool
   - Request missing transactions via `MsgGetBlockTxn`
   - Reconstruct and validate full block

### Phase 4: Testing & Optimization (0.5 days)

1. Test with real Namecoin network:
   - Verify correct block reconstruction
   - Measure bandwidth savings (expect ~90%)
   - Measure latency improvement (expect -30-50%)

2. Performance tuning:
   - Optimize short ID lookup (hash table or bloom filter)
   - Adjust prefilled transaction selection

## Technical Considerations

### Mempool Synchronization

Compact blocks rely on sender and receiver having similar mempools. Namecoin-specific considerations:

- **Name operations** may not be in mempool if:
  - NAME_NEW commitment window (12 blocks) hasn't expired
  - NAME_FIRSTUPDATE revealing name after commitment
  - Solution: Always prefill name operation transactions in compact blocks

### Short ID Collision Handling

- Probability of collision with 6-byte IDs: ~1 in 281 trillion per transaction pair
- Mempool typically < 10,000 transactions
- Expected collisions: negligible, but handle gracefully:
  - If reconstruction fails due to collision, fall back to requesting full block

### Namecoin Protocol Compatibility

- BIP152 is Bitcoin-focused but protocol-agnostic
- Should work with Namecoin's transaction format
- Test with Namecoin Core for compatibility (if it supports BIP152)

## Dependencies

- Requires functional mempool (✅ implemented)
- Requires peer protocol version negotiation (✅ implemented via btcd/peer)
- Requires transaction validation (✅ implemented)

## Alternatives Considered

1. **FIBRE (Fast Internet Bitcoin Relay Engine)**: More complex, requires additional infrastructure
2. **Graphene**: Newer, more efficient, but not widely adopted
3. **Status quo**: Acceptable for current deployment scale

## References

- [BIP152 Specification](https://github.com/bitcoin/bips/blob/master/bip-0152.mediawiki)
- [Bitcoin Core Implementation](https://github.com/bitcoin/bitcoin/pull/8068)
- [Compact Blocks FAQ](https://bitcoincore.org/en/2016/06/07/compact-blocks-faq/)

## When to Implement

Compact block relay should be implemented when:

1. **Network load increases**: High transaction volume causes bandwidth constraints
2. **Block propagation latency matters**: Mining nodes need faster block relay
3. **Production deployment**: After v1.0 release, when optimization becomes priority
4. **Namecoin Core compatibility**: If Namecoin Core implements BIP152, compatibility may be needed

## Current Workaround

For now, network optimizations focus on:
- ✅ Connection pooling (reduce connection overhead)
- ✅ Buffer pooling (reduce allocations)
- ✅ Peer scoring (prioritize fast, reliable peers)

These provide significant performance improvements without the complexity of BIP152.
