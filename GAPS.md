# Implementation Gaps — 2026-05-22

## AuxPow (Merged Mining) Support is Non-Functional

- **Stated Goal**: README claims "Pure Go Namecoin daemon" with blockchain integration and block synchronization
- **Current State**: `chain/auxpow.go` has AuxPow parsing/validation code, but `ProcessBlock` still enforces standard Bitcoin PoW on all blocks. The AuxPow chain-merkle validation accepts empty proofs. The node cannot sync past block 19,200 (AuxPow activation height) on mainnet.
- **Impact**: The daemon is unusable on mainnet for any real blockchain operations. Only regtest/testnet (below AuxPow activation) can function.
- **Closing the Gap**: Complete the AuxPow integration in `ProcessBlock` — skip child-header PoW for AuxPow blocks and delegate to the AuxPow validation path. Fix the chain-merkle branch verification to require actual proof verification. Add integration tests with real mainnet AuxPow blocks.

## NAME_FIRSTUPDATE Transaction Generation is Broken

- **Stated Goal**: README claims "Name Registration: Two-step NAME_NEW → NAME_FIRSTUPDATE process" as a library feature
- **Current State**: `wallet.CreateNameFirstUpdateTx` builds the NAME_FIRSTUPDATE script with the hex-encoded random string (40 ASCII chars) instead of the decoded 20-byte value. The resulting transaction will never match the NAME_NEW commitment hash and will be rejected by any validating node.
- **Impact**: No user can successfully register a name through this software. The two-step registration flow is fundamentally broken.
- **Closing the Gap**: Pass the decoded raw bytes (not the hex string) into the NAME_FIRSTUPDATE script construction. Add an integration test that verifies the commitment hash matches between NAME_NEW and NAME_FIRSTUPDATE.

## Name Re-Registration After Expiration is Rejected

- **Stated Goal**: README states names expire after 36,000 blocks and implies they can be re-registered
- **Current State**: `chain/blockchain.go:658-661` rejects NAME_FIRSTUPDATE whenever the name exists in the database, regardless of whether it has expired. Expired names cannot be re-registered.
- **Impact**: The name system effectively makes all names permanent once registered (until a reorg removes them), contradicting the 36,000-block expiration model.
- **Closing the Gap**: Modify the NAME_FIRSTUPDATE validation to allow registration when the existing record's `ExpiresAt < currentHeight`.

## Thread-Safety Claims vs. Mutable Shared References

- **Stated Goal**: README highlights "Thread-Safe: All operations safe for concurrent use" and "Mutex protection for all shared state"
- **Current State**: The name cache (`namedb/cache.go`) stores and returns mutable `*NameRecord` pointers. The wallet's `GetKey` returns internal `*KeyPair` pointers. The mempool stores caller-owned `*wire.MsgTx` pointers. All of these allow mutation of shared state without holding any lock.
- **Impact**: Concurrent usage (the advertised primary use case) can corrupt name records, wallet keys, and mempool state through aliased pointer mutation.
- **Closing the Gap**: Deep-copy all values on cache/mempool insert and return. Return copies (or read-only interfaces) from `GetKey`. Alternatively, document that returned pointers must not be mutated.

## Block Sync Stalls on Peer Disconnect

- **Stated Goal**: README claims "Automatic Initial Block Download (IBD) and ongoing sync" with "Peer Selection: Tracks peer reliability and latency to choose the best sync sources"
- **Current State**: `network/sync.go` sets a `syncPeer` but never clears it on disconnect. If the sync peer goes away, `syncPeer` remains non-nil and sync never reselects another peer. There is no reliability/latency tracking — just last-seen height comparison.
- **Impact**: IBD can stall permanently after a single peer disconnect, requiring a restart. The "reliability and latency tracking" feature does not exist.
- **Closing the Gap**: Implement peer disconnect notification to `SyncManager`. Clear and reselect `syncPeer` on disconnect. Implement actual reliability/latency metrics if claiming them in documentation.

## Reorg Safety for Name Expirations

- **Stated Goal**: The architecture claims nameDB stays consistent during chain reorganizations via NTBlockConnected/NTBlockDisconnected notifications
- **Current State**: Expiration processing permanently deletes names and history. The disconnect/rollback path has no mechanism to restore names that were expired by a now-disconnected block. Additionally, rollback errors are silently discarded.
- **Impact**: Any reorg that crosses a block where names expired will permanently lose those names from the database, corrupting state irreversibly.
- **Closing the Gap**: Either persist expired records in a separate bucket for rollback restoration, or reconstruct them from transaction history on disconnect. Propagate rollback errors.

## Wallet Change Address Handling Risks Fund Loss

- **Stated Goal**: The wallet provides name update and registration transaction creation
- **Current State**: `CreateNameUpdateTx` and `CreateNameFirstUpdateTx` use the name destination/owner address as the change address. In a name transfer scenario, all excess coins go to the new owner.
- **Impact**: Users performing name transfers lose all excess UTXO value to the recipient. This is a potential fund-loss scenario.
- **Closing the Gap**: Accept a separate change address parameter (defaulting to the wallet's own address) for change outputs. Never use the name-owner address for change.

## Transaction Mempool Missing Core Validation

- **Stated Goal**: README claims "Transaction Mempool: Validates and relays unconfirmed transactions with automatic expiration"
- **Current State**: When `cfg.Blockchain` is nil, `onTx` accepts and relays transactions without any validation. Even with a blockchain, NAME_FIRSTUPDATE mempool validation (`chain/blockchain.go:1987-1991`) does not enforce reveal timing windows.
- **Impact**: Invalid or malformed transactions can propagate through the network. The validation claim is incomplete.
- **Closing the Gap**: Require a non-nil blockchain for mempool operation. Implement reveal-window timing checks in mempool name validation.

## Header Validation Missing in Sync

- **Stated Goal**: "Downloads block headers from peers, validates the chain, then fetches full blocks"
- **Current State**: `network/sync.go HandleHeaders` blindly converts received headers into block download requests without validating header linkage, PoW, or timestamps. Headers are accepted from any peer, not just the sync peer.
- **Impact**: Any peer can inject bogus headers and trigger large numbers of useless block requests, stalling sync and wasting bandwidth.
- **Closing the Gap**: Validate header chain (prev-hash linkage, PoW, timestamps) before requesting blocks. Only accept headers from the designated sync peer during IBD.
