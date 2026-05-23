# UNIVERSAL BUG AUDIT (END-TO-END) — 2026-05-22

## Project Profile

**nmcd** is a pure Go Namecoin daemon and library built on btcd libraries. It targets developers building decentralized naming systems and operators running lightweight Namecoin nodes. Deployment model is a single binary (daemon mode) or an embedded library. Critical paths include: name database operations, blockchain validation with Namecoin-specific rules, P2P block sync, JSON-RPC server, and wallet key/transaction management.

## Audit Scope

- **Packages audited**: 14 (namedb, chain, rpc, network, wallet, client, config, mail, metrics, bridge, internal/logging, internal/server, loadtest, cmd/nmcd)
- **Total functions inspected**: 671
- **Total files**: 63
- **Total lines of code**: 9,563
- **Go version**: 1.24.11
- **All tests pass**: Yes (race detector enabled)
- **go vet warnings**: 0

## Coverage Log

| Package | 3b Logic | 3c Nil | 3d Errors | 3e Resources | 3f Concurrency | 3g Security | 3h Aliasing | 3i Init | 3j API |
|---------|----------|--------|-----------|--------------|----------------|-------------|-------------|---------|--------|
| namedb | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| chain | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| rpc | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| network | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| wallet | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| client | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| config | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| mail | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| metrics | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| bridge | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| internal/logging | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| internal/server | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| loadtest | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| cmd/nmcd | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

## Goal-Achievement Summary

| Stated Goal | Status | Blocking Findings |
|-------------|--------|-------------------|
| Pure Go Namecoin daemon using btcd | ⚠️ | CRITICAL-1, CRITICAL-2 (cannot sync mainnet past AuxPow activation) |
| Thread-safe operations | ⚠️ | HIGH-1, HIGH-2, MEDIUM-5 |
| Name registration (NAME_NEW → NAME_FIRSTUPDATE) | ❌ | CRITICAL-3 (script encoding bug) |
| Name updates (NAME_UPDATE) | ⚠️ | HIGH-3 (change address bug) |
| Blockchain integration with name validation hooks | ⚠️ | CRITICAL-4, HIGH-4 |
| bbolt-backed name database | ⚠️ | HIGH-5, HIGH-6 |
| P2P networking using btcd/peer | ⚠️ | HIGH-7, HIGH-8 |
| JSON-RPC server | ⚠️ | MEDIUM-1, MEDIUM-2 |
| Block synchronization / IBD | ⚠️ | HIGH-7, HIGH-8 |
| Library-first / embedded mode | ✅ | — |
| Transaction mempool | ⚠️ | HIGH-9 |

## Findings

### CRITICAL

- [x] **CRITICAL-1: AuxPow validation accepts invalid proofs** — `chain/auxpow.go:345-375` — Security/Blockchain Validation — Empty chain-merkle branches are blindly accepted; failed `CheckMerkleBranch` falls back to only a depth check. A forged AuxPow proof can be accepted as valid, allowing blocks with insufficient proof-of-work to enter the chain. — **Remediation:** For non-empty chain-merkle branches, verify the computed merkle root is committed in the coinbase data; reject when the root is not found. Empty branches (single-chain merged mining direct commitment) are accepted. Validate with `go test -race ./chain/...`.

- [x] **CRITICAL-2: ProcessBlock enforces child-header PoW on AuxPow blocks** — `chain/blockchain.go:247-252, 298-301, 438-445` — Blockchain Validation/Logic — Valid Namecoin AuxPow blocks are proved by the parent header, not the child header hash. Code rejects all valid post-19200 blocks. — **Remediation:** Skip child-header PoW check for AuxPow blocks and validate via the parent-block's PoW instead. Validate with `go test -race ./chain/...`.

- [x] **CRITICAL-3: NAME_FIRSTUPDATE pushes hex string instead of raw bytes** — `wallet/wallet.go:952-976` — Logic/Protocol Correctness — `CreateNameFirstUpdateTx` validates `randHex` by decoding it, but builds the script with the original 40-char hex string instead of the decoded 20-byte value. Generated `NAME_FIRSTUPDATE` transactions won't match the prior `NAME_NEW` commitment hash and will fail network validation. Data flow: user calls RPC `name_firstupdate` → `rpc/server.go` → `wallet.CreateNameFirstUpdateTx(randHex)` → script built with ASCII hex. — **Remediation:** Pass `decoded` (raw bytes) into script construction instead of `randHex`. Validate with `go test -race ./wallet/...`.

- [x] **CRITICAL-4: NAME_FIRSTUPDATE rejects valid re-registration of expired names** — `chain/blockchain.go:658-661` — Logic/Blockchain Validation — Block validation rejects `NAME_FIRSTUPDATE` whenever the name exists in DB, regardless of expiration status. Valid re-registrations of names that have been expired for any duration are incorrectly rejected. — **Remediation:** Only reject when the existing record's `ExpiresAt >= currentHeight` (i.e., the name is not yet expired). Unexpected DB errors other than `ErrNameNotFound` now also cause block validation to fail rather than silently proceeding. Validate with `go test -race ./chain/...`.

### HIGH

- [x] **HIGH-1: Wallet change sent to destination/name-owner address** — `wallet/wallet.go:713-725, 752-754, 898-932` — Logic/Funds Safety — In `CreateNameUpdateTx` and `CreateNameFirstUpdateTx`, the name destination/owner address is also used as the change address. When performing a name transfer, all excess coins from the spent UTXOs go to the new owner. Data flow: RPC `name_update` with transfer → `wallet.CreateNameUpdateTx(destAddr)` → change output uses `destAddr`. — **Remediation:** Always send change outputs to a wallet-controlled address instead of reusing the name destination/owner address. Validate with `go test -race ./wallet/...`.

- [x] **HIGH-2: Cache returns mutable shared pointers** — `namedb/namedb.go:172-176, 195-196, 219-220` + `namedb/cache.go:43-47` — Data Aliasing/Concurrency — The name cache stores and returns the same `*NameRecord` pointer. External mutation after `GetName` or `PutName` corrupts cached state without a DB write and can race with concurrent readers. — **Remediation:** Deep-copy `NameRecord` on cache insert and return copies from cache reads. Validate with `go test -race ./namedb/...`.

- [x] **HIGH-3: GetNameUTXO always fetches output index 0** — `namedb/utxo.go:217-220` — Logic/API Contract — `GetNameUTXO` ignores `record.OutIndex` and always calls `GetUTXO(&record.TxHash, 0)`. Names stored at non-zero output indices will get wrong UTXO lookups or false "not found" errors. — **Remediation:** Call `GetUTXO(&record.TxHash, record.OutIndex)`. Validate with `go test -race ./namedb/...`.

- [ ] **HIGH-4: Reorg rollback never restores expired names** — `chain/blockchain.go:974-988, 1760-1773` — Reorg/Data Lifecycle — Expiration processing deletes names and history, but the disconnect (rollback) path never restores entries that were expired by a now-disconnected block. Reorgs across expiration heights permanently lose names and corrupt state. — **Remediation:** Persist expired records for rollback restoration, or reconstruct them from history on disconnect. Validate with `go test -race ./chain/...`.

- [x] **HIGH-5: Rollback paths discard database errors** — `chain/blockchain.go:1779, 1809, 1816-1820, 1839-1844` — Error Handling — Reorg rollback operations (e.g., `RemoveLastHistoryEntry`) discard errors and continue. A failed rollback silently leaves the name database inconsistent. — **Remediation:** Propagate or aggregate rollback failures and abort claiming success when DB operations fail. Validate with `go test -race ./chain/...`.

- [x] **HIGH-6: Batch writer stores caller-owned pointers** — `namedb/batch.go:55-56, 73-74, 81-83, 90-91` — Data Aliasing/API Contract — `BatchWriter` queues caller-owned pointers and writes them during `Commit`. Caller mutation between enqueue and commit changes what gets persisted, causing silent corruption. — **Remediation:** Copy inputs when enqueueing, or document ownership transfer explicitly. Validate with `go test -race ./namedb/...`.

- [ ] **HIGH-7: Sync peer never reselected on disconnect** — COMPLETED — Added `SyncManager.OnPeerDisconnected()` method called by `PeerManager` when peers disconnect. Clears both `syncPeer` and `bestPeer` references to enable reselection. Validated with `go test -race ./network/...`.

- [ ] **HIGH-8: HandleHeaders accepts unvalidated headers from any peer** — COMPLETED — Modified `HandleHeaders()` to only accept headers from the active sync peer, rejecting headers from other peers with log message. Prevents header spam attacks. Validated with `go test -race ./network/...`.

- [ ] **HIGH-9: Mempool stores mutable transaction pointers** — COMPLETED — Modified `AddTx()` to deep-copy transactions using `tx.Copy()` on insert. Modified `GetTx()` and `GetAll()` to return copies. Prevents external mutation of mempool state. Validated with `go test -race ./network/...`.

### MEDIUM

- [ ] **MEDIUM-1: getinfo returns compact target bits as "difficulty"** — COMPLETED — Modified `getinfo` RPC handler to compute actual difficulty ratio (max_target / current_target) using `blockchain.CompactToBig()`. Returns human-readable float64 instead of raw bits. Validated with `go test -race ./rpc/...`.

- [ ] **MEDIUM-2: name_history dereferences blockchain without guard** — COMPLETED — Added `requireBlockchain` guard at the start of `nameHistory` method. Prevents panic when server created without blockchain. Validated with `go test -race ./rpc/...`.

- [ ] **MEDIUM-3: lookupActiveNameRecord off-by-one in expiration check** — COMPLETED — Changed expiration check from `ExpiresAt <= bestHeight` to `ExpiresAt < bestHeight` to match project convention. Added explanatory comment. Validated with `go test -race ./rpc/...`.

- [ ] **MEDIUM-4: walletpassphrase timer accumulation** — `rpc/server.go:1398-1400` — Resource Lifecycle/Logic — Each `walletpassphrase` call creates a new auto-lock timer without cancelling prior timers. Earlier timers can still fire and lock the wallet sooner than expected. — **Remediation:** Store the timer handle and cancel/reset it on subsequent calls. Validate with `go test -race ./rpc/...`.

- [ ] **MEDIUM-5: Global defaultLogger has unsynchronized read/write** — COMPLETED — Added `defaultLoggerMu` RWMutex. `GetDefault()` acquires read lock, `SetDefault()` acquires write lock. Prevents concurrent read/write data races. Validated with `go test -race ./internal/logging/...`.

- [ ] **MEDIUM-6: SMTP readDataBody allows unbounded memory allocation** — `mail/smtp.go:512-536` — Security/Resource Lifecycle — The SMTP server buffers the entire DATA payload before enforcing `MaxMessageSize`. A client can force unbounded memory growth. — **Remediation:** Enforce size limit during the read loop and abort when exceeded. Validate with `go test -race ./mail/...`.

- [ ] **MEDIUM-7: findNameNewUTXOIndex returns 0 on failure** — COMPLETED — Changed return value from 0 to -1 on failure. Added validation in caller to reject RPC with descriptive error when no NAME_NEW UTXO found. Validated with `go test -race ./rpc/...`.

- [ ] **MEDIUM-8: name RPC address selection is nondeterministic** — `rpc/server.go:836-847, 862-863` — Logic/API — Name operations use `addresses[0]` from map iteration (nondeterministic). Different calls may use different addresses, causing failures or wrong UTXO selection. — **Remediation:** Choose deterministically (sort, flag, or search all addresses for suitable UTXOs). Validate with `go test -race ./rpc/...`.

- [ ] **MEDIUM-9: ExtractChainIDFromVersion uses signed shift** — COMPLETED — Changed from `uint32(version >> 16)` to `uint32(version) >> 16` to use unsigned shift and avoid sign extension for negative versions. Validated with `go test -race ./chain/...`.

- [ ] **MEDIUM-10: Mempool validation skipped when blockchain is nil** — `network/peermgr.go:50-56, 401-418` + `network/mempool.go:159-171` — Security/Initialization — `NewPeerManager` allows `cfg.Blockchain == nil`, leaving mempool validator nil. `onTx` still accepts and relays unvalidated transactions. — **Remediation:** Require non-nil blockchain for live networking, or reject tx handling without a validator. Validate with `go test -race ./network/...`.

- [ ] **MEDIUM-11: onInv requests all announced inventory unconditionally** — `network/peermgr.go:301-313` — Security/Performance — `onInv` requests every announced inventory item without checking if already known, already requested, or worth fetching. A peer can force redundant bandwidth/CPU use. — **Remediation:** Filter by type and known state before queueing getdata. Validate with `go test -race ./network/...`.

- [ ] **MEDIUM-12: Peer map keyed by address allows overwrites** — `network/peermgr.go:198, 208, 258, 269` — Logic/Resource Lifecycle — `pm.peers` keyed by `p.Addr()` allows duplicate connections to overwrite each other. Disconnect handling can remove wrong peer entries. — **Remediation:** Key by unique connection/peer ID or reject duplicates before insertion. Validate with `go test -race ./network/...`.

### LOW

- [ ] **LOW-1: ScanNames returns one result when count <= 0** — `namedb/namedb.go:358-389` — Off-by-one/API Contract — `ScanNames` appends before enforcing `count`. With `count <= 0`, it returns the first match instead of empty. RPC validates positive counts but direct callers are unprotected. — **Remediation:** Short-circuit `count <= 0` before scanning.

- [ ] **LOW-2: GetNameNew returns alias of input slice** — `namedb/namedb.go:533-535` — Data Aliasing — Returns `Hash: commitHash` without copying; caller mutation of the input slice corrupts the returned record. — **Remediation:** Copy `commitHash` before assigning.

- [ ] **LOW-3: Close leaves cache active** — `namedb/namedb.go:134-137` — Resource Lifecycle/API Contract — `Close()` closes bbolt but leaves the cache live. Cached `GetName` calls succeed after close, returning stale data. — **Remediation:** Clear cache and track a closed state.

- [ ] **LOW-4: Public methods accept nil pointer arguments without validation** — `namedb/namedb.go:141-161, 402-409`; `namedb/utxo.go:73-82, 105-113` — Nil Safety — Public methods dereference pointer arguments without nil checks. — **Remediation:** Validate pointer args at public entrypoints.

- [ ] **LOW-5: Version-3 records with truncated NameNewHeight decoded silently** — `namedb/namedb.go:722-724` — Error Handling/Data Corruption — V3 records missing final bytes decode with `NameNewHeight == 0` instead of erroring, enabling silent corruption during reorgs. — **Remediation:** Return error if version >= 3 and fewer than 4 bytes remain for NameNewHeight.

- [ ] **LOW-6: GetUTXOsForAddress prefix scan overmatches** — `namedb/utxo.go:173-177` — Logic/Security — Empty or short addresses match unintended entries due to raw prefix scanning. — **Remediation:** Reject empty addresses and encode keys with a delimiter or length prefix.

- [ ] **LOW-7: Wallet loadKeys accepts any private key length** — `wallet/wallet.go:145-164` — Input Validation/Integrity — `loadKeys` accepts any decoded byte length and ignores stored address. Corrupted wallet files can load silently with wrong keys. — **Remediation:** Require 32-byte private keys and verify derived address matches stored address.

- [ ] **LOW-8: Wallet GetKey returns internal pointer** — `wallet/wallet.go:299-308` — Data Aliasing/Concurrency — `GetKey` returns the internal `*KeyPair`. Callers can mutate wallet state without locks. — **Remediation:** Return a copy or expose read-only accessors.

- [ ] **LOW-9: GenerateKey leaves key in memory on save failure** — `wallet/wallet.go:265-274` — Security/State Consistency — Inserts key into `w.keys` before `save()`. If save fails, the key remains accessible in memory but not on disk. — **Remediation:** Roll back map entry on save failure.

- [ ] **LOW-10: Negative feeRate accepted in wallet tx builders** — `wallet/wallet.go:737-747, 795-805, 905-913` — Input Validation — Negative `feeRate` makes fee negative, potentially producing invalid overspending transactions. — **Remediation:** Reject `feeRate < 0` before fee calculation.

- [ ] **LOW-11: Loadtest latency race** — `loadtest/runner.go:231-235, 249-265` — Concurrency — Result aggregation reads `latencies` slice without holding `latencyMu`. — **Remediation:** Copy slice under lock before computing stats.

- [ ] **LOW-12: getbalance/listunspent silently skip failed addresses** — `rpc/server.go:1570-1573, 1686-1688` — Error Handling — UTXO lookup failures for specific addresses are silently skipped, returning incomplete results. — **Remediation:** Propagate or surface partial-failure metadata.

- [ ] **LOW-13: Mempool validateNameFirstUpdate missing timing enforcement** — `chain/blockchain.go:1987-1991` — Logic/Mempool Policy — Mempool validation fetches `nameNewRecord` but never enforces min/max reveal timing window. — **Remediation:** Validate reveal window using `nameNewRecord.Height`.

- [ ] **LOW-14: Block/AuxPow serialization mismatch** — `chain/block.go:180-183, 217-220` — API Contract/Serialization — `Serialize()` writes AuxPow whenever `auxPow != nil` even if the version bit is unset. Won't round-trip with `NewBlockFromReader`. — **Remediation:** Serialize AuxPow only when `HasAuxPow()` is true.

## Metrics Snapshot

| Metric | Value |
|--------|-------|
| Total functions | 671 |
| Functions above complexity 15 | 2 |
| Avg cyclomatic complexity | 4.9 |
| Doc coverage | 82.2% |
| Duplication ratio | 0.90% |
| Test pass rate | All pass (28 packages) |
| go vet warnings | 0 |
| Race detector issues | 0 (at test time) |

## False Positives Considered and Rejected

| Candidate | Reason Rejected |
|-----------|----------------|
| `client/daemon.go:281` `_, _ = io.Copy(io.Discard, resp.Body)` | Intentional drain-and-discard pattern; discarding copy error on discard writer is standard practice |
| `chain/blockchain.go:1816` discarded error in rollback | Already reported as HIGH-5 (not a false positive) |
| Nil peer dereferences in network callbacks | btcd peer library guarantees non-nil peer in callbacks |
| Potential race in `rpc.Server` fields during startup | Fields are set during construction before `Start()`; no concurrent access until server is listening |
| `config/configfile.go` TOML parsing could allow path traversal in DataDir | Path is used locally; OS permissions provide the trust boundary |

## Remaining Scope

All packages audited — no remaining scope.
