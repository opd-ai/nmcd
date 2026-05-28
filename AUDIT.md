# UNIVERSAL BUG AUDIT (END-TO-END) — 2026-05-28

## Project Profile
- **Purpose**: `nmcd` is a pure-Go Namecoin library-and-daemon built on `btcd` libraries (not a fork). It can be embedded directly into Go applications (`client.NewClient` with `ModeAuto`/`ModeEmbedded`) or run as a standalone JSON-RPC daemon.
- **Target users**: Go developers needing in-process Namecoin name resolution/registration, and operators running a standalone daemon for RPC.
- **Deployment model**: Library import OR daemon process bound to a local TCP port (defaults to loopback). `cmd/permamail` provides an additional SMTP relay daemon that uses Namecoin records for routing.
- **Critical paths** (deserve deeper scrutiny):
  1. `chain/` — block validation, AuxPow validation, name-script parsing
  2. `namedb/` — bbolt persistence of name and UTXO state; reorg restore paths
  3. `network/` — P2P peer manager, headers-first sync, mempool
  4. `rpc/` — exposed JSON-RPC handlers, auth, request parsing
  5. `wallet/` — key encryption, transaction construction, signing
  6. `client/` — public API surface used by library consumers
- **Trust boundaries**:
  - Untrusted bytes from P2P peers (`network/`) → block/tx validation in `chain/` → persistence in `namedb/`
  - Untrusted JSON-RPC requests (`rpc/`) → wallet/chain operations; protected by optional HTTP Basic auth (constant-time HMAC compare) and a token-bucket rate limiter
  - Untrusted SMTP traffic (`mail/`, `cmd/permamail/`)
  - File-system inputs: `DataDir`, config TOML, wallet files

## Audit Scope
- **Packages audited** (16): `bridge`, `chain`, `client`, `cmd/nmcd`, `cmd/permamail`, `config`, `internal/logging`, `internal/server`, `internal/version`, `loadtest`, `mail`, `metrics`, `namedb`, `network`, `rpc`, `wallet`. The `examples/*` packages were scope-checked but treated as documentation samples (not production code).
- **Functions inspected**: 706 declared functions across 21,245 LOC (excluding `_test.go`).
- **go-stats-generator metrics summary**:
  - Total functions: 706
  - Functions above cyclomatic complexity 15: **1** (`namedb.RestoreExpiredNamesForBlock`, complexity 17)
  - Average cyclomatic complexity: **3.49**
  - Functions above 50 lines: 60+ (top: `metrics.NewPrometheusCollector` 243 lines; `chain.ValidateAuxPow` 110; `namedb.RestoreExpiredNamesForBlock` 106; `chain.ProcessBlock` 105)
  - Duplication: 7 clones detected (all in `examples/*` or low-impact helpers; suggestion ratio low)
- **Baseline test/vet results**:
  - `go test -race ./...` — **all suites pass** (`ok` for every non-example package, including `client` 47s, `wallet` 51s, `rpc` 18s)
  - `go vet ./...` — **0 warnings**

## Coverage Log
Every package below was inspected for each Phase 3b–3j category. ✅ = category reviewed and either clean or findings recorded below; ➖ = category not applicable (no code of that kind in the package).

| Package | 3b Logic | 3c Nil | 3d Errors | 3e Resources | 3f Concurrency | 3g Security | 3h Aliasing | 3i Init | 3j API |
|---|---|---|---|---|---|---|---|---|---|
| bridge | ✅ | ✅ | ✅ | ✅ | ✅ | ➖ | ✅ | ✅ | ✅ |
| chain | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| client | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| cmd/nmcd | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| cmd/permamail | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| config | ✅ | ✅ | ✅ | ✅ | ➖ | ✅ | ✅ | ✅ | ✅ |
| internal/logging | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| internal/server | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| internal/version | ➖ | ➖ | ➖ | ➖ | ➖ | ➖ | ➖ | ➖ | ✅ |
| loadtest | ✅ | ✅ | ✅ | ✅ | ✅ | ➖ | ✅ | ✅ | ✅ |
| mail | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| metrics | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| namedb | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| network | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| rpc | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| wallet | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

## Goal-Achievement Summary

| Stated Goal | Status | Blocking Findings |
|---|---|---|
| Library-first design (in-process embedding via `client`) | ✅ | none |
| Embedded or daemon mode with auto-detection | ✅ | none |
| Thread-safe public client API | ✅ | none |
| Pure Go / no C dependencies | ✅ | none (verified via `go.sum`) |
| bbolt-backed name database with expiration | ⚠️ | H-2, H-3 (silent index-write failures can leave stale expiration index) |
| Headers-first block synchronization | ⚠️ | H-4, H-5 (sync-peer identity confusion on reconnect with same address) |
| RPC server resists abuse | ⚠️ | H-6, H-7, H-8 (nil-deref panics in `name_update`/`name_new`/`name_firstupdate` when wallet is configured without blockchain; recovered by panic middleware but returns 500) |
| Wallet encryption protects user passwords | ⚠️ | H-1 (scrypt N=16384 below modern OWASP guidance) |
| AuxPow validation rejects invalid proofs | ⚠️ | H-9 (target-bytes copy lacks length guard; edge case but defensive) |
| `~18,000 LOC of production code` claim | ❌ | G-1, G-2 (actual ≈21,245 LOC; ROADMAP cites contradictory 9,729) |
| RPC API documentation reflects implemented surface | ❌ | G-3 (≈11 RPC methods implemented but not listed in README) |
| Mempool relays validated transactions | ✅ | none |

(Status legend: ✅ delivered, ⚠️ delivered with defects, ❌ not delivered as documented.)

## Findings

All findings include a specific file and line, observed consequence, and remediation with a validation command. Severity follows the prompt's classification table.

### CRITICAL

- [x] **C-1 — HTTP server has no timeouts (Slowloris) on `prometheus_exporter` internal server** — `internal/server/server.go:154-157` — resource / security — `http.Server` is constructed with `Handler` and `Addr` only; `ReadTimeout`, `WriteTimeout`, `IdleTimeout`, and `ReadHeaderTimeout` all default to zero (no limit). A single slow client can hold a connection open indefinitely, exhausting file descriptors and goroutines on the metrics endpoint. The same `internal/server.Server` is wired into the Prometheus exporter (`metrics/prometheus.go`, `examples/prometheus_exporter`).
  **Remediation:** Set `ReadHeaderTimeout: 10 * time.Second`, `ReadTimeout: 30 * time.Second`, `WriteTimeout: 30 * time.Second`, `IdleTimeout: 60 * time.Second` on the `http.Server` literal in `internal/server/server.go`. Validate with `go test -race ./internal/server/...` and `go vet ./...`.

### HIGH

- [x] **H-1 — Wallet scrypt key-derivation work factor is below modern guidance** — `wallet/encryption.go:202` — security (weak crypto) — `hashPassword` uses `scrypt(N=16384, r=8, p=1)` (the values from RFC 7914 ca. 2012). Current OWASP/NIST guidance is `N≥2^17` for new deployments. The accompanying comment claims `N=16384 ... is strong enough to resist brute-force`, which contradicts the project's own `deriveKey` constant of `N=32768` for actual encryption-key derivation. An attacker who obtains the wallet file can mount faster offline brute-force attacks against weak passwords.
  **Remediation:** Raise `scryptN` in `hashPassword` to at least `1<<17` (preferably `1<<19`) and remove the misleading comment; add a wallet-file version bump path so existing files re-hash on next unlock. Validate: `go test ./wallet/...`.

- [x] **H-2 — Silent `bbolt.Bucket.Delete` failure leaves stale expiration index (update path)** — `namedb/batch.go:197` — error handling — `BatchWriter.updateExpirationIndex` calls `expirationBucket.Delete(oldExpirationKey)` and discards the returned `error`, then returns `nil`. A later read at the same key during expiration sweep can resurrect or double-expire a name. Reachable on every `Put` of an existing name.
  **Remediation:** Capture the error: `if err := expirationBucket.Delete(oldExpirationKey); err != nil { return fmt.Errorf("delete old expiration index: %w", err) }`. Also do not silently swallow `decodeErr` — log or wrap it. Validate: `go test -race ./namedb/...`.

- [x] **H-3 — Silent `bbolt.Bucket.Delete` failure leaves stale expiration index (delete path)** — `namedb/batch.go:234` — error handling — Same defect as H-2 in `removeExpirationIndex`, executed during batched name deletions. The function always returns `nil`, so callers cannot detect partial-commit corruption.
  **Remediation:** Capture and return the `Delete` error; surface `decodeErr` instead of silently skipping. Validate: `go test -race ./namedb/...`.

- [x] **H-4 — Sync-peer identity tracked by address string allows confusion on reconnect** — `network/sync.go:284` — concurrency / logic — `OnPeerDisconnected` compares `sm.syncPeer.Addr() == p.Addr()`. If peer P1 disconnects and a new connection P2 with the same address (but different `*peer.Peer`) becomes the sync peer beforehand, the disconnect callback for P1 will nil out `sm.syncPeer` and stall sync until the next selection cycle.
  **Remediation:** Compare by pointer identity (`sm.syncPeer == p`) for the active comparison. The address can remain as a logging field. Validate: `go test -race ./network/...`.

- [x] **H-5 — Best-peer identity tracked by address string — same race as H-4** — `network/sync.go:290` — concurrency / logic — `OnPeerDisconnected` also nils `sm.bestPeer` if any peer with the same address as the previous `bestPeer` disconnects. Same fix as H-4.
  **Remediation:** Change to `sm.bestPeer == p`. Validate: `go test -race ./network/...`.

- [x] **H-6 — `name_update` RPC handler can nil-deref blockchain when wallet is configured without blockchain** — `rpc/name_handlers.go:80,143` — nil safety — `nameUpdate` calls `requireWallet` but not `requireBlockchain`; `parseOptionalDestAddress` then dereferences `s.blockchain.ChainParams()` at line 143. The `rpc.Server` constructor (`rpc/server.go:222`) accepts `cfg.Blockchain == nil` without rejecting it, and the daemon mode of `client.NewDaemonClient` is intended for blockchain-less wallet usage. The panic is caught by `withPanicRecovery` (`rpc/server.go:280`), so the process survives, but every such request returns HTTP 500 and the panic is reported in logs.
  **Remediation:** Add `if errResp := s.requireBlockchain(req.ID); errResp != nil { return errResp }` immediately after the `requireWallet` check in `nameUpdate`. Validate: `go test -race ./rpc/...` plus a targeted test that constructs `Server{wallet: x, blockchain: nil}` and calls `name_update`.

- [x] **H-7 — `name_new` RPC handler can nil-deref blockchain** — `rpc/name_handlers.go:304,349` — nil safety — Same defect as H-6: `nameNew` only checks `requireWallet`; `checkNameNotActive` invokes `s.blockchain.GetName(...)`.
  **Remediation:** Add a `requireBlockchain` guard at function entry. Validate: same as H-6.

- [x] **H-8 — `name_firstupdate` RPC handler can nil-deref blockchain** — `rpc/name_handlers.go:375,430` — nil safety — Same defect as H-6/H-7: `nameFirstUpdate` only checks `requireWallet`; `validateNameNewCommitment` invokes `s.blockchain.GetNameDB()`.
  **Remediation:** Add a `requireBlockchain` guard at function entry. Validate: same as H-6.

- [x] **H-9 — `StoreSpentUTXO` failures are logged but not propagated** — `chain/blockchain.go:981-984` — error handling — `processTransactionInputs` is declared `error` but logs and continues whenever `bc.nameDB.StoreSpentUTXO(utxo, height)` fails, and ultimately `return nil`s. The on-disk spent-UTXO journal is the input to `RestoreSpentUTXOsForBlock` during chain reorgs; if writes silently fail (disk full, bbolt error), reorg recovery will be permanently incomplete and the index will diverge from the canonical chain.
  **Remediation:** Return the wrapped error: `if err := bc.nameDB.StoreSpentUTXO(utxo, height); err != nil { return fmt.Errorf("store spent UTXO %s:%d: %w", txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index, err) }`. If best-effort behaviour is genuinely intended, document it in the function GoDoc and add a metric. Validate: `go test -race ./chain/... ./namedb/...`.

- [x] **H-10 — `CreateNameUpdateTxRaw` can call `txscript.PayToAddrScript(nil)` when changeAddress is nil and change is dust-or-greater** — `wallet/wallet.go:835-862` (with `addChangeOutput` at `wallet/wallet.go:682`) — nil safety — `CreateNameUpdateTxRaw` accepts a `changeAddress btcutil.Address` parameter without validating it; if `nil` is passed and the residual `changeValue ≥ DustLimit`, `addChangeOutput` calls `txscript.PayToAddrScript(nil)`, which panics inside `btcutil`. No upstream caller currently passes `nil`, but the function is exported and the contract is undocumented.
  **Remediation:** Reject `nil` at function entry with `return nil, errors.New("changeAddress must not be nil")`, or fall back to a wallet-managed address. Validate: `go test ./wallet/...` plus a regression test with a `nil` address.

### MEDIUM

- [x] **M-1 — RPC HTTP server lacks `IdleTimeout` and `ReadHeaderTimeout`** — `rpc/server.go:283-284` — security / resource — `ReadTimeout` and `WriteTimeout` are set to 30s, but `IdleTimeout` is unset and `ReadHeaderTimeout` defaults to `ReadTimeout`. Keep-alive abuse and slowloris-style header trickling are partially but not fully mitigated.
  **Remediation:** Add `IdleTimeout: 60 * time.Second, ReadHeaderTimeout: 10 * time.Second` to the `http.Server` literal. Validate: `go test -race ./rpc/...`.

- [x] **M-2 — AuxPow target-byte unrolling lacks an upper-bound guard** — `chain/auxpow_validation.go:93-96` — logic / defensive — `targetBytes := targetDifficulty.Bytes()` then `for i := 0; i < len(targetBytes); i++ { targetHash[len(targetBytes)-1-i] = targetBytes[i] }`. `chainhash.Hash` is `[32]byte`; if `len(targetBytes) > 32` (only reachable for malformed/extreme `Bits`), the index overflows and panics. `blockchain.CompactToBig` clamps to 256 bits for valid headers, so this is largely unreachable in practice, but a malicious peer feeding a crafted header would trigger a panic that is *not* wrapped by recover in this code path.
  **Remediation:** Replace with `if len(targetBytes) > chainhash.HashSize { return fmt.Errorf("compact target exceeds 256 bits") }` before the loop, or use `copy(targetHash[chainhash.HashSize-len(targetBytes):], reverse(targetBytes))`. Validate: `go test -race ./chain/...`.

- [x] **M-3 — Sync peer-height map keyed by `*peer.Peer` pointer** — `network/sync.go:255,289,321,334` — concurrency / logic — `peerHeights[p] = height` uses the peer pointer as the map key while other branches compare by address string (see H-4/H-5). The two access patterns can disagree if the peer pool reuses pointers after disconnect, leaving stale height data.
  **Remediation:** Pick a single canonical identity (preferably the `*peer.Peer` pointer for the lifetime of a connection) and use it consistently for all sync-state lookups, including `OnPeerDisconnected`. Validate: `go test -race ./network/...`.

- [x] **M-4 — `name_update`/`name_new`/`name_firstupdate` panics are recovered but counted as 500s without distinguishing programmer error** — `rpc/server.go:280-...` (and the H-6/H-7/H-8 sites) — observability — The `withPanicRecovery` middleware converts panics into generic HTTP 500 responses. This conceals nil-deref bugs and makes operators see only "Internal Server Error". Combined with H-6/H-7/H-8, real bugs are silently shipped to clients.
  **Remediation:** When the recovered value is a runtime error (`runtime.Error`), include the panic site in structured logs at `ERROR` level (already partially done) and surface a distinguishable error code (e.g., `-32099 "panic recovered"`). Validate: `go test -race ./rpc/...`.

- [x] **M-5 — `silent decodeNameRecord` skip in batch index maintenance** — `namedb/batch.go:194,231` — error handling — `if decodeErr == nil { ... }`-style branches silently skip index maintenance for records that fail to decode (corruption or schema drift). This hides on-disk corruption from operators.
  **Remediation:** Surface the error: at minimum, `log.Printf("namedb: skipping expiration index update for %q: %v", name, decodeErr)`; ideally return it as a wrapped error and abort the batch. Validate: `go test -race ./namedb/...`.

- [x] **M-6 — `BroadcastBlock` queues `inv` to disconnected peers** — `network/peermgr.go:640` — resource — Unlike `relayTransaction` (`network/peermgr.go:494`), `BroadcastTx` (`network/peermgr.go:671`), and `SyncBlocks` (`network/peermgr.go:743`), the loop in `BroadcastBlock` does not call `p.Connected()` before appending to the broadcast target list. Queued sends on disconnected peers waste work and may briefly retain peer state past disconnect.
  **Remediation:** Add the `if !p.Connected() { continue }` guard before appending. Validate: `go test -race ./network/...`.

- [x] **M-7 — `cmd/permamail` signal handler goroutine is not stopped on shutdown** — `cmd/permamail/main.go:379` — resource — `signal.Notify(sigChan, ...)` is called without a paired `signal.Stop(sigChan)` before `serve()` returns. In a long-running process this is once-only, but the signal-channel goroutine is retained for the life of the process and accumulates if `serve` is ever called more than once (tests, restarts).
  **Remediation:** `defer signal.Stop(sigChan)` immediately after the `Notify` call. Validate: `go test ./cmd/permamail/...`.

- [x] **M-8 — SMTP `extractAddress` does not reject CRLF in user input** — `mail/smtp.go:685-695` — security (input validation, defense-in-depth) — `extractAddress` parses the argument of `MAIL FROM`/`RCPT TO` without rejecting embedded `\r` or `\n`. The `net/smtp` client used downstream sanitizes for its own command framing, but defense-in-depth is appropriate for a server that may pass these values into logs, headers, and routing decisions.
  **Remediation:** After parsing, return early on `strings.ContainsAny(addr, "\r\n")`. Validate: `go test ./mail/...`.

### LOW

- [x] **L-1 — `hashPassword` comment misrepresents the chosen work factor** — `wallet/encryption.go:202` — documentation / API — Comment asserts `N=16384` is "strong enough to resist brute-force", which contradicts both the project's own `deriveKey` (`N=32768`) and current public guidance. Even after H-1 is fixed, the comment should be honest about the trade-off.
  **Remediation:** Rewrite the comment to cite the chosen N, the documented attacker model, and a link to the relevant guidance. Validate: `go vet ./wallet/...`.

- [x] **L-2 — `removeUTXOFromAddressIndex` silently ignores `Delete` errors** — `namedb/batch.go:366` — error handling — Same class as H-2/H-3 but on a lower-impact index (address → UTXO mapping). Leaves orphan address-index entries that will eventually be pruned on the next full UTXO rebuild but can confuse `listunspent` queries until then.
  **Remediation:** Capture and propagate the `Delete` error. Validate: `go test -race ./namedb/...`.

- [x] **L-3 — `signal.Notify` in `cmd/nmcd` should be paired with `signal.Stop`** — `cmd/nmcd/main.go` (signal-handling block in `main`) — resource — Same class as M-7, but `cmd/nmcd` is a `main` that only runs once per process, so the leak is theoretical.
  **Remediation:** `defer signal.Stop(sigChan)` for symmetry. Validate: `go build ./cmd/nmcd/...`.

- [x] **L-4 — Largest function (`metrics.NewPrometheusCollector`, 243 lines) is composed of mechanical metric declarations and is correctly straight-line code, but its size makes future drift risky** — `metrics/prometheus.go:99-...` — code organization — No bug today. Flagged because the prompt requires inspection of every function above 50 lines.
  **Remediation:** Optional — group metric definitions into helper slices indexed by name to keep the constructor short. Validate: `go test ./metrics/...`.

- [x] **L-5 — `loadtest.MemoryLeakTest` allocates without bound by design** — `loadtest/runner.go:306` — false-positive considered — The function intentionally retains references to drive memory pressure for load testing. Comment near the function acknowledges this. No action.
  **Remediation:** None. Add a `//nolint:` style annotation if/when the project adopts a linter that flags this. Validate: n/a.

- [x] **L-6 — Duplicate clones in `examples/*`** — `examples/embedded_client/main.go:55-60` and `:81-86`; `examples/simple_resolve/main.go:85-90`; `loadtest/runner.go:504-509` (six-line repeated wallet/setup snippet, similarity > 0.9) — duplication — Examples intentionally duplicate setup boilerplate for clarity. Not a defect.
  **Remediation:** None for examples. If a shared `examples/internal` helper is introduced, dedupe there. Validate: n/a.

- [x] **L-7 — RPC has no `IdleTimeout`-style accounting for per-connection request count** — `rpc/server.go:283` — performance/security minor — Beyond M-1, there is no per-IP connection cap. Token-bucket `rateLimiter` already throttles request rate.
  **Remediation:** Optional — add `MaxHeaderBytes` and consider a connection limiter for non-loopback deployments. Validate: `go test -race ./rpc/...`.

## Metrics Snapshot

| Metric | Value |
|---|---|
| Total functions | 706 |
| Functions above cyclomatic complexity 15 | 1 (`namedb.RestoreExpiredNamesForBlock`, complexity 17) |
| Avg cyclomatic complexity | 3.49 |
| Doc coverage (per-package, from go-stats-generator) | not emitted by tool in this run (all reported `null`); spot-checks show public APIs are documented in `client/`, `namedb/`, `wallet/`, partial in `rpc/` |
| Duplication ratio | low; 7 clone groups, all in `examples/*` or trivial helpers |
| Test pass rate | 100% (`ok` for every non-example package under `go test -race ./...`) |
| `go vet` warnings | 0 |
| Production LOC | 21,245 (vs. README claim of "~18,000") |

## False Positives Considered and Rejected

| Candidate | Reason Rejected |
|---|---|
| `chain.ProcessBlock` is 105 lines / complexity 11 | Reviewed; cleanly factored into validation → indexing → notification steps; no buggy paths found. |
| `client.NewClient` may race with daemon coming up between probe and use | Probe failure → embedded mode is the documented fallback; the auto-detect contract does not promise daemon use is monotonic. |
| `mail/smtp.go` returns `502 Command not implemented` | This is the SMTP-protocol response for an unsupported verb, not a stub TODO. |
| `examples/mail_router/main.go` "not implemented in mock" returns | These are documented mock implementations for the example, not production stubs. |
| Loop-variable capture in goroutines (pre-Go 1.22) | `go.mod` requires Go 1.24.11; the language-level fix is in effect for the whole codebase. |
| `math/rand` misuse | No production usage found; all secret-bearing paths use `crypto/rand` (verified in `rpc/server.go:257`, wallet, etc.). |
| `InsecureSkipVerify` in TLS | Not present anywhere outside test fixtures. |
| `panic` in non-init paths | All production `panic` sites are either documented "should never happen" invariants or are caught by `withPanicRecovery` in the RPC server. |
| Mempool tx broadcast race | `PeerManager.BroadcastTx` (`network/peermgr.go:629-637`) intentionally adds the tx to the local mempool first so validation runs before relay; verified consistent with stored repository fact and tests. |
| Expiration off-by-one (`ExpiresAt < currentHeight` vs `≤`) | Verified strict `<` is the project's definition (`namedb/namedb.go:316-318`); not a bug. |
| `metrics.NewPrometheusCollector` 243 lines | Pure metric registration; reviewed line-by-line, no double-registration or shared-state hazard. |
| `cmd/permamail` `serve` 70 lines | Standard server setup with proper `defer ln.Close()` and signal handling; only M-7 noted. |

## Remaining Scope (session-end status)

| Package | Status | Notes |
|---|---|---|
| (none) | All 16 production packages were audited to completion. | Findings are stable across two full passes; no new findings above LOW emerged in the second pass. |

If a follow-up session is opened, the recommended next pass is **fuzz-style targeting of the `chain/` AuxPow and `chain/name_script.go` parsers** with `go test -fuzz=Fuzz...` (no fuzz tests exist today), and **property-based testing of `namedb/batch.go` commit semantics** under simulated bbolt I/O failures — these are areas the bug-class checklist cannot fully cover by static reading.
