# IMPLEMENTATION GAP AUDIT — 2026-04-20

## Project Architecture Overview

**nmcd** is a library-first pure Go Namecoin implementation built on btcd v0.25.0 libraries. It provides both an embeddable library (client package) and a standalone daemon for Namecoin name resolution, registration, and updates.

### Stated Goals (from README + ROADMAP.md)
1. Library-first design with embedded and daemon modes
2. Composition over reimplementation using btcd components
3. Thread-safe operations with mutex protection
4. Pure Go (no C dependencies)
5. Name operations: NAME_NEW, NAME_FIRSTUPDATE, NAME_UPDATE
6. Block synchronization via headers-first protocol
7. Transaction mempool with validation and relay
8. JSON-RPC server with standard + name methods
9. Wallet encryption (AES-256-GCM)
10. Health/readiness endpoints for container orchestration
11. Prometheus metrics (43 metrics)
12. Test coverage ≥70% for critical packages
13. Protocol compliance ≥95%
14. Performance: <1ms name lookup, <100ms RPC latency, 1000+ req/s

### Package Responsibilities
| Package | Responsibility | Files |
|---------|---------------|-------|
| `client/` | Public API (EmbeddedClient, DaemonClient, NameClient interface) | 5 |
| `chain/` | Blockchain wrapper with name validation hooks, AuxPow support | 7 |
| `namedb/` | bbolt-backed name database with caching, batching, UTXO tracking | 6 |
| `network/` | P2P peer management, mempool, sync manager, peer scoring | 7 |
| `rpc/` | JSON-RPC server with 26 methods | 3 |
| `wallet/` | ECDSA key management, AES-256-GCM encryption | 3 |
| `config/` | Network parameters, chain params, config file support | 6 |
| `mail/` | SMTP routing and relay (Permamail) | 4 |
| `bridge/` | Email forwarding adapter between Namecoin and mail | 3 |
| `metrics/` | Prometheus metrics collection | 3 |
| `internal/logging/` | Structured logging (slog) | 1 |
| `internal/server/` | Daemon server orchestration | 1 |
| `cmd/nmcd/` | Main daemon entry point | 1+ |
| `cmd/permamail/` | Permamail SMTP service entry point | 1 |

### Key Interfaces
| Interface | Package | Methods | Implementations |
|-----------|---------|---------|----------------|
| `NameClient` | client | 8 | EmbeddedClient, DaemonClient |
| `Resolver` | bridge | 1 (LookupMail) | NamecoinBridge |
| `Resolver` | mail | 1 (LookupMail) | (uses bridge.Resolver) |
| `TxValidator` | network | 1 (ValidateMempoolTransaction) | chain.BlockChain |

---

## Gap Summary

| Category | Count | Critical | High | Medium | Low |
|----------|-------|----------|------|--------|-----|
| Stubs/TODOs | 2 | 0 | 1 | 1 | 0 |
| Dead Code | 1 | 0 | 0 | 1 | 0 |
| Partially Wired | 3 | 0 | 1 | 1 | 1 |
| Interface Gaps | 2 | 0 | 0 | 1 | 1 |
| Dependency Gaps | 1 | 0 | 0 | 0 | 1 |

**Total: 9 findings (0 Critical, 2 High, 4 Medium, 3 Low)**

---

## Implementation Completeness by Package

| Package | Exported Functions | Implemented | Stubs | Dead | Coverage Notes |
|---------|-------------------|-------------|-------|------|----------------|
| client | 76 | 75 | 1 | 0 | DaemonClient.RegisterName returns error |
| chain | 115 | 115 | 0 | 0 | Fully implemented |
| namedb | 67 | 67 | 0 | 0 | Fully implemented |
| network | 78 | 77 | 1 | 0 | onGetData block handling incomplete |
| rpc | 85 | 85 | 0 | 0 | searchTransaction limited to 1000 blocks |
| wallet | 46 | 46 | 0 | 0 | Fully implemented |
| config | 14 | 14 | 0 | 0 | Fully implemented |
| mail | 32 | 32 | 0 | 0 | Fully implemented |
| bridge | 3 | 3 | 0 | 0 | Fully implemented |
| metrics | 27 | 27 | 0 | 0 | Fully implemented |
| internal/server | 12 | 12 | 0 | 0 | Fully implemented |
| internal/logging | 20 | 20 | 0 | 0 | Fully implemented |

---

## Findings

### HIGH

- [ ] **H-1: Block serving not implemented in onGetData** — `network/peermgr.go:465-470` — When remote peers request blocks via `getdata`, the handler logs a debug message but never fetches or sends the requested block. Transaction serving (line 472-489) works correctly, but block serving is a no-op. — **Blocked Goal:** Block propagation to peers (goal #6: block synchronization). Peers syncing from this node will never receive requested blocks, breaking the P2P protocol contract for block serving. — **Remediation:** Implement block fetching from `pm.blockchain.GetBlockByHash()` in the `case wire.InvTypeBlock` branch. Serialize the block via `wire.MsgBlock` and queue it to the requesting peer with `p.QueueMessage()`. Add a test verifying block data is sent. Validate with `go test ./network -run TestOnGetData`.

- [ ] **H-2: DaemonClient.RegisterName returns stub error** — `client/daemon.go:402-413` — `RegisterName()` always returns an error message: `"RegisterName via daemon mode is not yet supported"`. The function performs input validation (lines 404-409) but then unconditionally returns an error. The documentation (lines 390-401) explicitly acknowledges this as incomplete. — **Blocked Goal:** Library-first design (goal #1) — the `NameClient` interface contract promises `RegisterName` but the DaemonClient implementation cannot fulfill it. — **Remediation:** Implement the two-phase registration workflow: (1) call `name_new` RPC, (2) track pending registrations in a local store, (3) after 12 blocks, call `name_firstupdate` RPC. This requires adding a pending registration tracker to `DaemonClient` and a polling mechanism for block height. Validate with `go test ./client -run TestDaemonRegisterName`.

### MEDIUM

- [ ] **M-1: onGetBlocks sends empty inventory response** — `network/peermgr.go:524-545` — The `onGetBlocks` handler receives block locator hashes from a peer but sends back an empty `MsgInv` (line 543-544). The comment on line 534-536 explicitly states: "In a full implementation, this would: 1. Find the common ancestor... 2. Send inventory message with block hashes from that point. For now, just log that we received the request." — **Blocked Goal:** Block synchronization (goal #6). Peers using the legacy `getblocks` protocol (vs. `getheaders`) will receive no block inventory and cannot sync from this node. — **Remediation:** Use `pm.blockchain.LocateBlocks()` (similar to the working `onGetHeaders` at line 508) to find block hashes after the common ancestor, then populate the inventory message with `wire.InvTypeBlock` entries. Validate with `go test ./network -run TestOnGetBlocks`.

- [ ] **M-2: getRawTransaction and getTransactionConfirmationStatus limited to last 1000 blocks** — `rpc/server.go:1891-1917` and `client/embedded.go:1082-1096` — Transaction search scans only the last 1000 blocks (hardcoded). The documentation at line 1855-1856 acknowledges: "It does not currently support mempool transactions or a full transaction index." The same pattern appears in `EmbeddedClient.getTransactionConfirmationStatus()` at line 1092. — **Blocked Goal:** JSON-RPC server completeness (goal #8). Users querying transactions older than ~7 days (1000 blocks at 10 min/block) will get "Transaction not found" errors. — **Remediation:** Implement a transaction index in `namedb` that maps `txHash → (blockHeight, blockHash)`. Populate during block processing in `chain.ProcessBlock()`. Use the index in `searchTransaction()` for O(1) lookups instead of linear block scanning. Validate with `go test ./rpc -run TestGetRawTransaction`.

- [ ] **M-3: Duplicate Resolver interface definition** — `bridge/namecoin.go:29` and `mail/router.go:27` — The `Resolver` interface is defined identically in both `bridge` and `mail` packages. While `mail.Resolver` documents "Note: This interface matches bridge.Resolver for seamless integration" (line 26), having two identical interfaces creates a maintenance risk. — **Blocked Goal:** None directly, but violates the project's composition-over-reimplementation principle and adds confusion. — **Remediation:** Remove `mail.Resolver` and use `bridge.Resolver` directly in `mail.Router`. Update `NewRouter` to accept `bridge.Resolver`. This is a minor refactor. Validate with `go build ./... && go test ./mail ./bridge`.

- [ ] **M-4: onVerAck handler is an empty no-op** — `network/peermgr.go:293-295` — The `onVerAck` callback is registered in peer listeners (lines 183, 243) but has an empty body. While the verack message in Bitcoin protocol is a simple acknowledgment, the handler could be used to mark the peer as fully connected and trigger initial sync. The `onVersion` handler correctly updates sync state (line 288), but `onVerAck` does nothing. — **Blocked Goal:** None critical — the version exchange via `onVersion` handles the essential sync triggering. However, per Bitcoin protocol, sync should be initiated after verack, not after version. — **Remediation:** If the project intends to follow strict Bitcoin protocol ordering, move the sync initiation logic to trigger after `onVerAck` rather than `onVersion`. Alternatively, document this as intentional. Validate: `go test ./network -run TestOnVerAck`.

### LOW

- [ ] **L-1: chain/doc.go states 35% protocol compatibility but PROTOCOL_COMPLIANCE_AUDIT.md claims 100%** — `chain/doc.go:76-83` vs `docs/development/PROTOCOL_COMPLIANCE_AUDIT.md:12` — The doc.go Known Limitations section states "approximately 35% protocol compatibility with Namecoin Core" citing missing AuxPow parent chain PoW verification, block version validation, and subsidy edge cases. However, the protocol compliance audit claims "100% Protocol Compliance" with 22/22 checks passed. — **Blocked Goal:** Documentation accuracy (part of goal #13). These contradictory claims create confusion about production readiness. — **Remediation:** Update `chain/doc.go` Known Limitations to accurately reflect the current compliance state. The 35% figure appears to be outdated. Validate by reviewing `docs/development/PROTOCOL_COMPLIANCE_AUDIT.md` and aligning the doc.go text. Validate with `go doc ./chain`.

- [ ] **L-2: AuxPow validation uses pragmatic shortcut for chain merkle branch** — `chain/auxpow.go:345-375` — The chain merkle branch validation falls back to accepting proofs if the branch is "structurally valid" (length ≤32) even when the computed root doesn't match the coinbase transaction hash. Comments (lines 362-368) explain this as "a pragmatic approach that works with various merged mining formats." — **Blocked Goal:** Protocol compliance (goal #13) — this is a known relaxation of AuxPow validation. The code documents it as intentional. — **Remediation:** No code change needed unless full AuxPow validation is a priority. The current approach is documented in `chain/doc.go` Known Limitations. If tightening is desired, implement strict chain merkle root verification against the coinbase commitment per Namecoin Core's `CheckMerkleBranch` logic.

- [ ] **L-3: go.mod includes `golang.org/x/crypto` but usage may be limited** — `go.mod:14` — The `golang.org/x/crypto` package is a direct dependency. It is used in `wallet/encryption.go` for scrypt key derivation. This is a legitimate and necessary dependency. However, `go.yaml.in/yaml/v2` appears only as an indirect dependency of btcd, and `github.com/kylelemons/godebug` is only used in tests. — **Blocked Goal:** None. All dependencies appear to be in use. — **Remediation:** Run `go mod tidy` periodically to ensure no unused dependencies accumulate. Current state is clean.

---

## False Positives Considered and Rejected

| Candidate Finding | Reason Rejected |
|-------------------|----------------|
| `network/seeds.go:32` returns `nil, nil` when seeds list is empty | Intentional — no seeds means no addresses to resolve. Documented behavior. |
| `namedb/namedb.go:599` returns `nil, nil` | Part of `GetExpiredNames()` — returns nil when no expired names found. Correct behavior. |
| `client/embedded.go:185` returns `nil, nil` | Part of `parseTransferAddress()` — returns nil when no transfer address specified. Correct behavior. |
| `client/embedded.go:773,780` returns `nil, nil` | Part of filtering logic — returns nil to skip records. Correct behavior. |
| `rpc/server.go:743` returns `nil, nil` | Part of `parseOptionalDestAddress()` — returns nil when no destination specified. Correct. |
| `onVersion` handler returning nil (no reject) | Correct per btcd/peer API — nil means "accept the peer." |
| `examples/smtp_relay/main.go` mock returns `nil, nil` | Example code mock — intentionally minimal, not production code. |
| `examples/mail_router/main.go` mock returns "not implemented" | Example code mock — intentionally minimal, not production code. |
| `chain/testvector.go` exported functions (LoadMainnetTestVector, etc.) | Test helper functions used by test files — part of testing infrastructure, not dead production code. |
| Short exported functions (BestSnapshot, ChainParams, GetNameDB, etc.) | These are intentional accessor/wrapper methods providing thread-safe access to embedded btcd types. They are complete as documented. |
| `wallet/wallet.go` functions returning `nil` after processing | These are successful return paths after completing operations (Save, Lock, etc.). Not stubs. |
| `rpc/server.go` `validateValueSize` returning `nil` on success | Correct — nil means validation passed. Called at lines 703 and 988. |
| `mail/smtp.go:295` "502 Command not implemented" | SMTP protocol response for commands the server intentionally doesn't support (e.g., VRFY, EXPN). Standard SMTP behavior. |
| `chain/blockchain.go` many `return nil` instances | These are successful return paths after error checking and processing. Each function performs validation and operations before returning nil. |
| `bridge/namecoin_test.go` mock methods returning "not implemented" | Test mock — only `ResolveName` is needed for the test. Other methods are interface satisfaction stubs. |
| `rpc/graceful_degradation_test.go:166` "placeholder test" comment | Test code pattern documentation, not production code gap. |
| `client/embedded_test.go:512` "Phase 2 placeholder" | Test checking that `BlockHeight` returns 0 — this is the expected behavior for the embedded client's `GetInfo()`, not an incomplete implementation. |
