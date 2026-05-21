# GAPS.md — Implementation Gap Analysis

**Date:** 2025-07-23  
**Reference:** Namecoin Core v22.x, Bitcoin Core wire protocol  
**Scope:** All Go source files in `/home/runner/work/nmcd/nmcd`

---

## Overview

nmcd achieves approximately **40–45% Namecoin protocol compatibility** for name operations against regtest / early mainnet (pre-block 19,200). Against live mainnet or testnet it is **not production-ready** due to the gaps below.

---

## GAP-001 — AuxPow validation is structural only, not cryptographically verified

**Severity:** CRITICAL for mainnet  
**Files:** `chain/auxpow.go`, `chain/blockchain.go:validateAuxPow`

The `AuxPow` struct is deserialized correctly and the version bit check is performed. However, the actual cryptographic validation steps required by the Namecoin/merged-mining specification are not implemented:

- **Parent chain coinbase contains commitment**: The coinbase scriptSig must contain a magic marker (`0xfa, 0xbe, 0x6d, 0x6d`) followed by the aux block hash. This is not verified.
- **Merkle proof verification**: `CoinbaseBranch` proves the coinbase is in the parent block's merkle tree. `ChainMerkleBranch` proves the aux hash commitment is in the coinbase. Neither proof is cryptographically verified.
- **Parent block header hash vs difficulty target**: The parent block's hash must meet Namecoin's difficulty target. This is not checked.
- **Chain ID check**: The commitment must use chain ID = 1 (Namecoin). Not verified.

**Impact:** Any block claiming to be AuxPow will pass validation, allowing fake blocks to be accepted on mainnet after block 19,200.

---

## GAP-002 — Subsidy validation rejects blocks with transaction fees (BUG-001)

**Severity:** CRITICAL  
**Files:** `chain/blockchain.go:330–383`

See BUG-001 in AUDIT.md. This is also an implementation gap against Namecoin Core's `CheckBlock()` which correctly computes `coinbaseValue ≤ GetBlockSubsidy(height) + nFees`.

---

## GAP-003 — No mempool: transactions cannot be stored, validated, or relayed

**Severity:** HIGH  
**Files:** `network/mempool.go` (exists but is a stub)

`network/mempool.go` provides a basic in-memory map of transactions with an LRU-style eviction loop but:

- No UTXO-based double-spend checking
- No transaction script validation
- No fee-rate ordering or replacement logic
- No relay to connected peers
- `name_pending` RPC returns mempool entries but they're not validated

Namecoin Core's mempool enforces name uniqueness across pending transactions and validates script signatures. nmcd accepts any transaction into the mempool without validation.

---

## GAP-004 — No block announcement or relay to peers

**Severity:** HIGH  
**Files:** `network/peermgr.go`, `network/sync.go`

nmcd can receive blocks from peers (`onBlock` handler) and process them. It cannot:

- Announce newly processed blocks to peers via `inv` messages
- Relay transactions to peers
- Respond to `getblocks` requests with inventory (partial implementation only)

A node that cannot relay is a passive consumer. It can sync the chain but cannot contribute to the network.

---

## GAP-005 — `name_update` RPC creates a transaction but does not broadcast it

**Severity:** HIGH  
**Files:** `rpc/server.go:nameUpdate` method

The `name_update` RPC method calls `wallet.CreateNameUpdateTx()` and returns the raw transaction hex, but does not:

- Submit the transaction to the local mempool
- Broadcast it to peers via `inv`/`tx` messages

The user must manually extract and broadcast the raw hex, which is not a documented workflow and differs from Namecoin Core's behavior where `name_update` broadcasts the transaction directly.

---

## GAP-006 — NAME_FIRSTUPDATE commitment validation uses wrong chain ID

**Severity:** HIGH  
**Files:** `chain/blockchain.go:computeCommitHash`

See BUG-013 in AUDIT.md. The commitment hash `Hash160(rand || name)` is computed with incorrect chain-ID encoding, causing mismatch with commitments created by Namecoin Core. A NAME_NEW created by Namecoin Core cannot be completed by nmcd's NAME_FIRSTUPDATE (and vice versa) on real networks.

---

## GAP-007 — No UTXO set for historical blocks

**Severity:** MEDIUM  
**Files:** `config/config.go:UTXOTrackingStartHeight`, `chain/blockchain.go`

`UTXOTrackingStartHeight = 0` suggests UTXO tracking from genesis, but in practice UTXO tracking only covers blocks processed after the daemon starts. Historical blocks loaded from the chain database do not have their UTXOs recorded. This means:

- Fee validation is disabled for historical blocks (acknowledged in code comments)
- `name_update` cannot find UTXOs for names registered before the node started
- Wallet balance for pre-existing addresses is zero until UTXOs are re-indexed

**Required:** A UTXO re-indexing pass on startup, or a clear documented limitation.

---

## GAP-008 — Namespace validation is more restrictive than Namecoin Core

**Severity:** MEDIUM  
**Files:** `config/config.go:ValidNamespaces`, `chain/blockchain.go:validateNameFormat`

nmcd enforces a strict namespace whitelist (`d/`, `id/`, `p/`) and rejects names in other namespaces. Namecoin Core's consensus rules do **not** restrict namespaces in this way — any name up to 255 bytes is valid on-chain. The `p/` namespace is also not an officially documented Namecoin namespace.

This means nmcd would reject blocks containing names in non-whitelisted namespaces (e.g., `u/`, `nf/`, or custom namespaces used by some projects), causing a chain fork.

**Fix:** Remove namespace validation from consensus-level block processing. Namespace filtering can remain in wallet/RPC as a user-facing policy.

---

## GAP-009 — JSON-only value enforcement for `d/` and `id/` at consensus level

**Severity:** MEDIUM  
**Files:** `chain/blockchain.go:validateValueEncoding`

nmcd rejects blocks where `d/` or `id/` names have non-JSON values. Namecoin Core does not enforce JSON encoding at the consensus level — any valid UTF-8 (or even arbitrary bytes) are acceptable name values. Enforcing JSON at consensus level would cause nmcd to reject valid Namecoin Core blocks.

**Fix:** Move JSON validation to the RPC/wallet layer as a policy check, not to block validation.

---

## GAP-010 — No checkpoint data for testnet or recent mainnet

**Severity:** MEDIUM  
**Files:** `config/namecoin_params.go`

Mainnet has only 2 checkpoints (genesis + block 19,200). Testnet has only genesis. Without dense checkpoints:

- Initial sync is vulnerable to long-range history-rewriting attacks
- Sync is slower (no trusted skip-ahead)
- btcd's checkpoint-based optimizations are not available

Namecoin Core includes checkpoints up to recent blocks.

---

## GAP-011 — No `getblocktemplate` / `submitblock` (mining RPC)

**Severity:** LOW  
**Files:** `rpc/server.go`

Mining-related RPC methods are absent. This prevents:

- Solo mining
- Pool mining integration
- regtest block generation for testing

The `GenerateSupported: true` flag in regtest params implies intent, but no implementation exists.

---

## GAP-012 — No `sendrawtransaction` RPC

**Severity:** LOW  
**Files:** `rpc/server.go`

There is no way to broadcast a raw transaction via RPC. Combined with GAP-005 (name_update not broadcasting), users have no path to submit transactions to the network through nmcd.

---

## GAP-013 — `getinfo` returns raw `Bits` field as difficulty

**Severity:** LOW  
**Files:** `rpc/server.go:getInfo` (~line 532)

The `getinfo` RPC response returns the raw compact-format `Bits` value as the difficulty field. Namecoin Core returns the actual difficulty as a floating-point ratio (`powLimit / currentTarget`). The function `getDifficultyRatio` exists in the codebase but is not used in `getInfo`.

---

## GAP-014 — Testnet genesis block uses Bitcoin's genesis merkle root

**Severity:** LOW  
**Files:** `config/namecoin_params.go:133–167`

See BUG-014 in AUDIT.md. testnet and regtest genesis blocks use Bitcoin's genesis merkle root. Namecoin's actual testnet genesis differs.

---

## GAP-015 — `ismine` field always false in `name_pending`

**Severity:** LOW  
**Files:** `rpc/server.go:namePending` (~line 1257)

```go
"ismine": false, // Would require wallet lookup
```

Namecoin Core's `name_pending` sets `ismine: true` for names controlled by the local wallet. nmcd always returns `false`, making it impossible for wallet software to identify which pending operations belong to the user.

---

## GAP-016 — No BIP37 bloom filter support

**Severity:** LOW  
**Files:** `network/peermgr.go`

Bloom filter-based transaction filtering (`filterload`, `filteradd`, `merkleblock`) is not implemented. Required for SPV client support.

---

## GAP-017 — `name_new` rand bytes must be provided by caller, not generated internally

**Severity:** LOW  
**Files:** `wallet/wallet.go:CreateNameNewTx`

`CreateNameNewTx` requires the caller to supply `randBytes`. If empty, the function returns an error. Namecoin Core's `name_new` generates random bytes internally and stores them for later use in `name_firstupdate`. nmcd places the burden on the caller to generate, store, and supply random bytes — creating a UX gap and risk of rand bytes being lost.

---

## Feature Coverage Matrix

| Feature | nmcd | Namecoin Core | Notes |
|---------|------|---------------|-------|
| NAME_NEW script build | ✅ | ✅ | |
| NAME_FIRSTUPDATE script build | ✅ | ✅ | |
| NAME_UPDATE script build | ✅ | ✅ | |
| NAME_NEW commitment validation | ⚠️ | ✅ | Wrong chain ID (BUG-013) |
| NAME expiration tracking | ✅ | ✅ | |
| NAME history storage | ✅ | ✅ | |
| Block reorg / rollback | ⚠️ | ✅ | Potential deadlock (BUG-002) |
| AuxPow deserialization | ✅ | ✅ | |
| AuxPow cryptographic validation | ❌ | ✅ | GAP-001 |
| Subsidy validation | ❌ | ✅ | BUG-001 rejects valid blocks |
| Fee validation | ❌ | ✅ | GAP-007 |
| Mempool (basic) | ⚠️ | ✅ | No validation / relay |
| Transaction relay | ❌ | ✅ | GAP-004 |
| Block relay / announcement | ❌ | ✅ | GAP-004 |
| P2P peer discovery | ✅ | ✅ | DNS seeds implemented |
| Headers-first sync | ✅ | ✅ | |
| RPC: getinfo | ⚠️ | ✅ | Wrong difficulty (GAP-013) |
| RPC: name_show | ⚠️ | ✅ | Negative expires_in (BUG-007) |
| RPC: name_list | ⚠️ | ✅ | Negative expires_in (BUG-007) |
| RPC: name_history | ✅ | ✅ | |
| RPC: name_scan | ✅ | ✅ | |
| RPC: name_pending | ⚠️ | ✅ | ismine always false (GAP-015) |
| RPC: name_update | ⚠️ | ✅ | No broadcast (GAP-005) |
| RPC: sendrawtransaction | ❌ | ✅ | GAP-012 |
| RPC: getblocktemplate | ❌ | ✅ | GAP-011 |
| RPC: submitblock | ❌ | ✅ | GAP-011 |
| Wallet encryption | ✅ | ✅ | AES-256-GCM + scrypt |
| Wallet key zeroing on lock | ❌ | ✅ | BUG-009 ineffective |
| Namespace enforcement | ⚠️ | N/A | Too strict at consensus level (GAP-008) |
| JSON value enforcement | ⚠️ | N/A | Too strict at consensus level (GAP-009) |
| Checkpoints (mainnet) | ⚠️ | ✅ | Only 2 checkpoints (GAP-010) |
| Prometheus metrics | ✅ | N/A | nmcd extension |
| Structured logging | ✅ | N/A | nmcd extension |

**Legend:** ✅ Implemented correctly · ⚠️ Partial/incorrect · ❌ Not implemented
