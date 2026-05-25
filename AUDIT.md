# UNIVERSAL BUG AUDIT (END-TO-END) — 2026-05-25

## Project Profile

**Project:** `nmcd` — pure-Go Namecoin daemon and embeddable library, composed on top of `btcd` libraries.

**Stated goals (from `README.md`):**
- Library-first design (embed in Go applications) and daemon mode (JSON-RPC server).
- Three operating modes for the `client` package: `ModeAuto`, `ModeEmbedded`, `ModeDaemon`.
- Thread-safe operations across all packages.
- Composition over reimplementation: extend `btcd` blockchain with Namecoin name-operation hooks.
- bbolt-backed `NameDatabase` with name records, history, expiration tracking and UTXO indexes.
- P2P networking via `btcd/peer` with **interface-based** `net.Conn` connections.
- "Transaction Mempool" that "validates and relays unconfirmed transactions with automatic expiration".
- Initial Block Download (headers-first), ongoing sync, peer scoring.
- JSON-RPC 2.0 server with HTTP Basic Auth.
- Pure Go, no C dependencies.

**Critical paths (deserve deeper scrutiny):** `chain` (name op validation, block processing), `namedb` (persistent state), `network` (peer + mempool + sync), `rpc` (auth + name update), `wallet` (private key handling), `client/embedded` (in-process integration).

**Trust boundaries:**
1. Peer-to-peer messages → `network/peermgr.go` / `chain/blockchain.go` validators.
2. HTTP JSON-RPC requests → `rpc/server.go` (with Basic Auth + rate limiter).
3. SMTP traffic (mail relay) → `mail/smtp.go`.
4. Filesystem (wallet, names DB, log files, config file).

## Audit Scope

- 14 packages, 63 production `.go` files, 10 087 LOC.
- All packages and every production file in the table below were inspected for **all** checklist categories (3b–3k) listed in the task prompt.
- Tooling baseline: `go-stats-generator`, `go vet`, `go test -race ./...`.

### Metrics summary (go-stats-generator)

| Metric | Value |
|--------|-------|
| Total functions (production) | 226 |
| Total methods | 459 |
| Average function length | 18.4 lines |
| Functions > 50 lines | 46 (6.7 %) |
| Functions > 100 lines | 3 |
| Avg cyclomatic complexity | 5.0 |
| Functions with complexity > 10 | 10 |
| Doc coverage (package / func / type / method) | 85.7 % / 93.7 % / 79.4 % / 79.5 % |
| Duplication ratio | 0.86 % |
| Circular dependencies | 0 |
| Test pass rate (`go test -race ./...`) | 23 / 24 packages (1 failing test in `rpc`) |
| `go vet ./...` warnings | 0 |

Top complexity hot-spots inspected manually: `RestoreExpiredNamesForBlock` (namedb 22.8), `HandleHeaders` (network 16.6), `decodeNameRecord` (namedb 16.6), `ValidateAuxPow` (chain 16.3), `CleanupOldExpiredNames` (namedb 16.3), `applyFlagOverrides` (cmd/nmcd 16.1), `ProcessBlock` (chain 15.3), `CreateNameUpdateTx` (wallet 15.3), `walletPassphrase` (rpc 15.3), `runLoadWorker` (loadtest 15.0).

## Coverage Log

| Package | 3b Logic | 3c Nil | 3d Errors | 3e Resources | 3f Concurrency | 3g Security | 3h Aliasing | 3i Init | 3j API |
|---------|----------|--------|-----------|--------------|----------------|-------------|-------------|---------|--------|
| `namedb` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `chain` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `rpc` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `network` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `client` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `wallet` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `config` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `internal/server` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `internal/logging` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `cmd/nmcd` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `cmd/permamail` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `mail` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `metrics` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `loadtest` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `bridge` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `examples/*` | ✅ (skim) | — | — | — | — | — | — | — | — |

## Goal-Achievement Summary

| Stated Goal | Status | Blocking Findings |
|-------------|--------|-------------------|
| Name operation validation (NAME_NEW / NAME_FIRSTUPDATE / NAME_UPDATE) | ❌ | CRIT-1, HIGH-1, HIGH-2 |
| Strict-`<` expiration convention used consistently | ❌ | HIGH-1, HIGH-2 |
| `go test -race ./...` passes (basic correctness baseline) | ❌ | HIGH-3 (failing test in `rpc`) |
| Thread-safe operations across all packages | ⚠️ | MED-1 (uncancellable goroutine in metrics) |
| Library-first design (embed safely in 3rd-party apps) | ⚠️ | MED-1 (goroutine leak on import); MED-9 (file perms) |
| Pure-Go, no C deps | ✅ | None |
| Composition over reimplementation | ✅ | None |
| Wallet protects private keys | ⚠️ | MED-3 (passphrase retained as Go string); MED-4 (no constant-time compare in wallet path) |
| Interface-based `net.Conn` networking | ✅ | None |
| Mempool validates + relays unconfirmed txs | ✅ | Verified at `network/peermgr.go:436-452` |
| RPC HTTP Basic Auth | ✅ (constant-time compare used) | None |
| README claim: "auto-detect daemon at localhost:8336" | ✅ | Implemented in `client/client.go:59` |
| README claim: "embedded mode auto-syncs (MaxPeers default 8)" | ⚠️ | LOW-7 (dead branch obscures intent) |

## Findings

### CRITICAL

- [ ] **CRIT-1: `validateNameOperations` silently swallows height-determination error** — `chain/blockchain.go:765-769` — *Error handling / Logic* — When `bc.determineBlockHeight(block)` fails, the function returns `nil` (success) instead of propagating the error. Consequence: every name-operation validation rule (NAME_NEW / NAME_FIRSTUPDATE / NAME_UPDATE — fees, duplicates, commitment matching, expiration, UTXO chain) is *completely skipped* for that block. This is a direct breach of the project's primary stated goal ("name validation hooks"). An attacker who can construct a block whose height cannot be derived (e.g., malformed coinbase, edge-case at genesis) bypasses all name validation. Triggered from `chain.BlockChain.ProcessBlock` and from the validateName-driven test paths.
  ```go
  func (bc *BlockChain) validateNameOperations(block *btcutil.Block) error {
  	height, err := bc.determineBlockHeight(block)
  	if err != nil {
  		return nil          // <-- swallows error, skips ALL name validation
  	}
  	…
  ```
  **Remediation:** change `return nil` on line 768 to `return fmt.Errorf("cannot determine block height for name validation: %w", err)`. Validate with `go test -race ./chain/...` and a regression test that injects a height-undeterminable block and asserts an error is returned.

### HIGH

- [ ] **HIGH-1: Inconsistent expiration comparison in `validateNameUpdateOp`** — `chain/blockchain.go:735` — *Logic (operator)* — Uses `if record.ExpiresAt <= height` while the rest of the codebase (`rpc/server.go:779`, `client/embedded.go:293,708,894`, `namedb/namedb.go:376` comment, repository memory) uses strict `<`. Consequence: a legitimate NAME_UPDATE submitted *exactly* at the expiration block (`height == ExpiresAt`) is rejected at block-validation time, even though the same record is considered "active" by every other reader. This causes the daemon to reject blocks that other Namecoin nodes accept (consensus divergence) and prevents owners from renewing their name on the last valid block.
  ```go
  if record.ExpiresAt <= height {       // should be `<`
  	return fmt.Errorf("name expired: %s …", …)
  }
  ```
  **Remediation:** change `<=` to `<` at line 735. Validate with `go test -race ./chain/...` and add a table-driven test case where `record.ExpiresAt == height` and the NAME_UPDATE is expected to succeed.

- [ ] **HIGH-2: Same inconsistency in `validateNameUpdate`** — `chain/blockchain.go:2129` — *Logic (operator)* — Identical bug to HIGH-1 in a second NAME_UPDATE validation entry point (`validateNameUpdate(name, value, tx, currentHeight)`), which is called from a different code path. Both copies have drifted from the convention; the duplicate-clone analyzer also flagged the surrounding region.
  ```go
  if record.ExpiresAt <= currentHeight { // should be `<`
  ```
  **Remediation:** change `<=` to `<` at line 2129; both HIGH-1 and HIGH-2 should be fixed in the same commit. Validate with `go test -race ./chain/...`.

- [ ] **HIGH-3: Inverted comparison in `validateNameFirstUpdate`** — `chain/blockchain.go:2088` — *Logic (operator)* — Uses `if existingRecord.ExpiresAt > currentHeight` (not expired only when *strictly greater*) while the sister function `validateNameFirstUpdateOp` at line 679 correctly uses `>= height` (not expired when `>=`). Consequence: at exactly `currentHeight == ExpiresAt` this function treats the prior owner's record as *expired* and allows re-registration, while `validateNameFirstUpdateOp` rejects the same operation as a duplicate. The two validators disagree on the boundary block, producing fork-relevant behavior depending on which entry point is used.
  ```go
  // Line 2088 (wrong):
  if existingRecord.ExpiresAt > currentHeight { … }
  // Line 679 (correct):
  if existingRecord.ExpiresAt >= height { … }
  ```
  **Remediation:** change `>` to `>=` at line 2088 to match the documented convention and the sister function. Validate with `go test -race ./chain/...` and add a regression test where `record.ExpiresAt == currentHeight` and re-registration is expected to fail.

- [ ] **HIGH-4: Failing test in `rpc` package blocks CI baseline** — `rpc/coverage_boost_test.go:549-575` + `rpc/server.go:778-781` — *Testing / API contract* — `TestLookupActiveNameRecordExpired` fails with "Expected error response for expired name". The test sets `ExpiresAt: 0` and queries at `bestHeight == 0`. Comment on line 561 claims "ExpiresAt=0 is <= bestHeight=0 (genesis), so name is expired", but the implementation at `rpc/server.go:779` correctly uses strict `<`: `if record.ExpiresAt < bestHeight` (matching the project's `<` convention). The bug is in the **test**, not the production code, but the result is the same: `go test -race ./...` exits non-zero, breaking the CI baseline.
  ```go
  // coverage_boost_test.go:561 (test's incorrect expectation)
  ExpiresAt: 0, // ExpiresAt=0 is <= bestHeight=0 (genesis), so name is expired
  ```
  **Remediation:** edit `rpc/coverage_boost_test.go` so the record's `ExpiresAt` is strictly less than `bestHeight` (e.g. set up a chain with `bestHeight >= 1` and `ExpiresAt = 0`, or set `ExpiresAt = -1`). Validate with `go test -race ./rpc/...`.

- [ ] **HIGH-5: SMTP STARTTLS triggered only when upstream port == 587** — `mail/smtp.go:453-461` — *Security (transport)* — The upstream SMTP connection only upgrades to TLS if `s.relay.config.UpstreamPort == 587`. For any other port (25, 465, custom corporate ports, or a deliberately mis-configured port) `client.Auth(...)` at line 464 then transmits SASL credentials (PLAIN/LOGIN) in cleartext. An attacker observing the network or controlling the upstream host can capture credentials and message bodies. Port 465 (SMTPS, implicit TLS) is also silently downgraded to plaintext because the code uses `smtp.Dial` (cleartext) regardless.
  ```go
  if s.relay.config.UpstreamPort == 587 {
  	tlsConfig := &tls.Config{ServerName: s.relay.config.UpstreamHost}
  	if err := client.StartTLS(tlsConfig); err != nil { … }
  }
  // No TLS for port 25 / 465 / any other port → Auth is plaintext below.
  ```
  **Remediation:** introduce an explicit `UpstreamTLS` configuration enum (`disabled` / `starttls` / `implicit`) independent of port number. For `implicit`, use `tls.Dial` + `smtp.NewClient` instead of `smtp.Dial`. Refuse to call `client.Auth` over a cleartext connection unless the user explicitly opts in. Validate with `go test -race ./mail/...` and an integration test against a self-signed TLS SMTP server.

- [ ] **HIGH-6: `CreateNameUpdateTxRaw` sends change to the new owner instead of the sender** — `wallet/wallet.go:803-857` — *Logic / Funds* — The standalone library function `CreateNameUpdateTxRaw` (exported, intended for callers who don't have all keys) passes `destAddress` as both the NAME_UPDATE script destination *and* the change-output destination. When a NAME_UPDATE transfers a name from address A to address B, the change is delivered to B (the new owner), not A (the funder). The receiver-method variant at line 712 (`CreateNameUpdateTx`) correctly directs change to `kp.Address` (the current owner) — line 762-764 explicitly comments on this. Library users calling the exported `CreateNameUpdateTxRaw` will lose the change to the recipient of a name transfer.
  ```go
  // Line 853 — destAddress is the name destination, not the change recipient
  if err := addChangeOutput(tx, changeValue, destAddress); err != nil {
  	return nil, err
  }
  ```
  **Remediation:** add a `changeAddress btcutil.Address` parameter to `CreateNameUpdateTxRaw` and use it for `addChangeOutput`. Document that change should be the sender's address. Bump the package's minor version (exported-API change) and validate with `go test -race ./wallet/...`.

### MEDIUM

- [ ] **MED-1: `init()` in `metrics/prometheus.go` spawns an unstoppable goroutine** — `metrics/prometheus.go:24-37` — *Concurrency / Resources* — `init()` starts a 30-second ticker goroutine to refresh cached Go runtime stats. There is no shutdown channel; the goroutine survives for the entire process lifetime. Because the project promotes library-first usage, every program that imports `nmcd/metrics` (or transitively imports it via `internal/server`) inherits this background goroutine even when it never serves Prometheus. This violates the project's "no surprising side-effects of import" expectation and complicates clean shutdown in test harnesses.
  ```go
  func init() {
  	updateGoRuntimeStats()
  	go func() {
  		ticker := time.NewTicker(30 * time.Second)
  		defer ticker.Stop()
  		for range ticker.C { updateGoRuntimeStats() } // never exits
  	}()
  }
  ```
  **Remediation:** Replace `init` with an explicit `Start(ctx context.Context)` (or `StartBackgroundRefresh()` + `Stop()`) called once from `internal/server/server.go`. If you must keep `init`, refresh on demand inside `Collect()` instead of in a background goroutine. Validate with `go test -race ./metrics/...`.

- [ ] **MED-2: Log file written with world-readable permissions** — `internal/logging/logger.go:124` — *Security (info disclosure)* — `os.OpenFile(cfg.Output, …, 0o644)` creates the log file world-readable. Logs include peer IPs, remote_addr of RPC clients, request paths, panic stack traces (rpc/server.go:316-345), and may include filenames containing the wallet path. On multi-tenant hosts any local user can read this.
  **Remediation:** change mode to `0o600` (matches the wallet file convention at `namedb/namedb.go:151` and `wallet/wallet.go`). Validate with `go test -race ./internal/logging/...`.

- [ ] **MED-3: Log directory created world-traversable** — `internal/logging/logger.go:119` — *Security (info disclosure)* — `os.MkdirAll(logDir, 0o755)` allows any local user to list the log directory. Combined with MED-2 this exposes operational logs to all local users.
  **Remediation:** change mode to `0o700`. Validate with `go test -race ./internal/logging/...`.

- [ ] **MED-4: Wallet caches passphrase as a Go `string`, preventing memory wiping** — `wallet/wallet.go:412-416, 456` — *Security (forensic)* — `w.unlockPassword string` retains the user's passphrase for the duration of the unlock interval. The defensive byte-zero loop at lines 412-416 zeros only the temporary `[]byte` copy; Go `string` is immutable and its backing array cannot be zeroed in place, so the original password remains in heap until garbage collection. A memory-dump attack (core file, debugger, swap) can recover it.
  **Remediation:** store the passphrase as `[]byte` from the moment it is received, never convert to `string`. Zero the slice when locking. Validate with `go test -race ./wallet/...` and ensure no callers convert back to `string`.

- [ ] **MED-5: `examples/smtp_relay/main.go` (and other examples) use `~/.nmcd` literally** — `examples/smtp_relay/main.go:~26` (default flag value) — *API contract / Usability* — Go does not expand `~`; the default value is treated as a literal directory called `~` under the current working directory. New users following the example will silently create a `~/` directory in the wrong place. The same pattern appears in several example programs.
  **Remediation:** resolve the default via `os.UserHomeDir()` and `filepath.Join`. Validate manually by running each example with no flags. (Examples are not covered by `go test`; document the change in `docs/EXAMPLES.md`.)

- [ ] **MED-6: `applyFlagOverrides` has cyclomatic complexity 16 with manual flag-by-flag mutation** — `cmd/nmcd/main.go:~33` — *Maintainability / Logic risk* — The function is short (33 lines) but has 16 branches. Manual review confirms no current correctness bug, but the high complexity combined with the high coupling (`main: 12 dependencies, coupling: 6.0` in stats) makes this a regression hot-spot. Any new CLI flag is likely to introduce subtle precedence mistakes.
  **Remediation:** extract the flag-merging logic into a table-driven helper (`applyFlagOverride(name string, set func(string))`). Validate with `go test -race ./cmd/nmcd/...`.

- [ ] **MED-7: `parseBlockHeightParam` has unreachable `int` / `int32` switch arms** — `rpc/server.go:1925-1939` — *Logic (dead code / latent overflow)* — `encoding/json` unmarshals JSON numbers into `float64` only, so the `case int:` (line 1933) and `case int32:` (line 1935) arms are unreachable from any real JSON-RPC call. The `case int:` arm also lacks the bounds check that `case float64:` (line 1929) has, so if this helper is ever reused with a non-JSON caller the `int32(v)` cast at line 1934 will silently truncate values > `math.MaxInt32`.
  **Remediation:** either delete the unreachable arms (preferred) or replicate the bounds check from the `float64` arm into the `int` arm. Validate with `go test -race ./rpc/...`.

- [ ] **MED-8: `name_update` and `walletpassphrase` raw-error pass-through can leak internal state** — `rpc/server.go:1408` (and similar `fmt.Sprintf("…: %v", err)` in name_update / sendrawtransaction) — *Security (info disclosure)* — Detailed wallet/crypto errors from `wallet.Unlock`, key derivation, or AEAD verification are returned verbatim to the RPC client. While the project model assumes localhost-only access, this still aids an attacker who has *any* RPC access in probing the wallet (e.g., distinguishing "wrong password" from "ciphertext truncated").
  **Remediation:** convert sensitive errors to a generic message ("authentication failed") at the RPC boundary while logging the detail server-side. Validate with `go test -race ./rpc/...`.

- [ ] **MED-9: `cmd/nmcd/main.go` never calls `signal.Stop` on the registered channel** — `cmd/nmcd/main.go:73-74` — *Resources (signal handler)* — `signal.Notify(sigChan, …)` without a matching `defer signal.Stop(sigChan)` keeps the runtime's signal-relay table populated past intended teardown. Not impactful for the production binary (process exits right after) but it complicates use of `main` as an embedded library entry point and races with tests that spawn the binary.
  **Remediation:** add `defer signal.Stop(sigChan)` immediately after `signal.Notify`. Validate with `go vet ./cmd/nmcd/...` and `go test -race ./cmd/nmcd/...`.

- [ ] **MED-10: bbolt operations omit `tx.Bucket(...) == nil` defensive checks throughout `namedb`** — `namedb/namedb.go:205-206, 263, 291-292, 324-325, 361, 405-408, 468-475, 571-574, 626-627, 703-705, 731-741, 777-779, 800-803, 816-817, 847, 862-863`, `namedb/batch.go:166-167, 206-207, 240-241, 265, 282-283, 321-322`, `namedb/utxo.go:82-83, 118-119, 160-161, 186-187, 257-258, 292-295, 382-383` — *Nil/boundary* — Every public `NameDatabase` method calls `tx.Bucket(...)` and immediately dereferences the result. All buckets are created in `NewNameDatabase` (line 158-162), so this is normally safe — but if the bbolt file is corrupted, manually edited, or a future migration forgets a bucket name, every code path here panics inside the bbolt transaction. The function `ScanNames` at line 656-661 already shows the correct pattern (`if bucket == nil { return nil }`). Reported as one consolidated finding rather than 30+ individual entries because root cause is identical.
  **Remediation:** introduce a small helper `requireBucket(tx, name) (*bbolt.Bucket, error)` and use it everywhere. Validate with `go test -race ./namedb/...`.

- [ ] **MED-11: `utxo.go` cleanup ignores delete errors** — `namedb/utxo.go:365, 367, 411, 413` — *Error handling* — `_ = spentBkt.Delete(utxoKey)` and `_ = idxBkt.Delete(k)` in the spent-UTXO cleanup paths silently discard errors. A failure here leaks rows into `spentUtxoBucket` / `spentUtxoIdxBucket` indefinitely (DB grows without bound), and there is no metric or log to detect it.
  **Remediation:** propagate the error (`return fmt.Errorf("cleanup spent utxo: %w", err)`); the outer `Update` will then surface it. Validate with `go test -race ./namedb/...`.

### LOW

- [ ] **LOW-1: Defensive `nil` checks missing for `s.logger` in `withPanicRecovery`** — `rpc/server.go:316-345` (and similar `s.logger.Warn` calls at `1624-1626, 1635-1636, 1750-1752, 1764-1765`) — *Nil/boundary* — `s.logger` is initialized in the `Server` constructor and not mutated thereafter, so in practice it is never nil. The deferred `recover()` handler nonetheless logs through `s.logger.Error(...)`, which would itself panic if the logger ever became nil (e.g., via a future zero-value `Server{}` construction in tests). Documented for future hardening; not exploitable today.
  **Remediation:** guard with `if s.logger != nil` (one-liner) or document that `Server{}` zero-value is not a supported construction. Validate with `go test -race ./rpc/...`.

- [ ] **LOW-2: Dead `cfg.MaxPeers == 0` branch in `resolveBootstrapPeers`** — `client/embedded.go:196` — *Logic (dead code)* — `if len(cfg.BootstrapPeers) > 0 || cfg.MaxPeers == 0` — `applyConfigDefaults` (line 145-146) has already set `MaxPeers` to 8 by the time this is reached, so `cfg.MaxPeers == 0` can never be true here. The branch obscures intent: a README reader who sets `MaxPeers: 0` expects "no peer connections" (README line 186) but the only place that intent is honored is at the peer-manager start, not here.
  **Remediation:** either remove the dead clause and document the actual behavior, or move the `MaxPeers == 0` short-circuit earlier (before `applyConfigDefaults`). Validate with `go test -race ./client/...`.

- [ ] **LOW-3: P95 / P99 latency computation off-by-one (statistical, never out-of-bounds)** — `loadtest/runner.go:267-268` — *Logic (statistical)* — `latenciesCopy[int(float64(N)*0.95)]` returns the 96-th element for N=100 instead of the 95-th. For N=20 (the minimum threshold) it returns the last (20-th) element for both P95 and P99. Not a panic (always `< N`) but inaccurate.
  **Remediation:** use `latenciesCopy[int(math.Ceil(float64(N)*0.95))-1]` clamped to `[0, N-1]`. Validate with `go test -race ./loadtest/...` and a unit test verifying P95 over a known distribution.

- [ ] **LOW-4: `GetKey` doc claims "copy to prevent external mutation" but returns shallow copy** — `wallet/wallet.go:314-330` — *Documentation / API contract* — The returned `KeyPair` shares the same `*btcec.PrivateKey` and `*btcec.PublicKey` pointers as the wallet's internal map entry. The struct fields are copied but the key objects are not. Callers who follow the doc literally and modify the returned key affect the wallet's internal state.
  **Remediation:** either rewrite the comment to say "shares key pointers; do not mutate" or perform a deep copy via `btcec.PrivKeyFromBytes(kp.PrivateKey.Serialize())`. Validate with `go test -race ./wallet/...`.

- [ ] **LOW-5: Misleading comment on inversion in `validateNameFirstUpdate`** — `chain/blockchain.go:2086-2092` — *Documentation* — The comment block above the buggy comparison cites the convention correctly ("ExpiresAt < currentHeight means expired … not expired means ExpiresAt >= currentHeight") but the code below uses `>` (HIGH-3). The comment will need to stay accurate after HIGH-3 is fixed.
  **Remediation:** keep the comment as-is once HIGH-3 is fixed.

- [ ] **LOW-6: `startSync(p *peer.Peer)` will panic if `p` is nil** — `network/sync.go:112` — *Nil/boundary* — Only ever called when `sm.bestPeer != nil`, so unreachable today. Documented for defensive hardening.
  **Remediation:** add `if p == nil { return }` at the top of `startSync`. Validate with `go test -race ./network/...`.

- [ ] **LOW-7: Unused exported function `CreateNameUpdateTxRaw`** — `wallet/wallet.go:803` — *API surface* — In addition to HIGH-6's correctness issue, the function has zero callers in the codebase (verified by `grep -r`). It is exported and therefore must be maintained.
  **Remediation:** either fix HIGH-6 and add an example/test demonstrating it, or unexport / remove the function. Validate with `go test -race ./wallet/...` and `go build ./...`.

- [ ] **LOW-8: Duplicated 12- and 15-line blocks in `chain/blockchain.go:1452-1495`** — *Duplication* — go-stats-generator flagged two adjacent renamed-clone pairs (`chain/blockchain.go:1452-1463 ↔ 1479-1490` and `1460-1474 ↔ 1481-1495`). The two copies validated identically on inspection; risk is *future* drift (as already happened between `validateNameUpdateOp` and `validateNameUpdate` — see HIGH-1/HIGH-2).
  **Remediation:** extract the common UTXO-chain-validation block into a helper. Validate with `go test -race ./chain/...`.

- [ ] **LOW-9: Duplicated 14-line block in `examples/register_name/main.go:152-165` ↔ `examples/update_name/main.go:158-171`** — *Duplication* — Example-only; low risk.
  **Remediation:** factor into a small helper in an `examples/internal/` package, or leave as-is for example clarity.

- [ ] **LOW-10: `// BUG:` annotations present in two files** — go-stats-generator reports 2 `BUG:` annotations (and 1 `XXX:`); these are author-known issues that should either be filed as tickets or fixed.
  **Remediation:** run `grep -rn "BUG:" --include="*.go"` and either fix or open issues.

- [ ] **LOW-11: 100 misplaced functions / 19 misplaced methods reported by structural analysis** — *Organization* — Examples: `Config.ApplyEnvironmentVariables` lives in `config/configfile.go` but is most-cohesive with `rpc/server.go`; `safeCalcExpiresAt` lives in `chain/height_overflow.go` but is most-cohesive with `chain/blockchain.go`. Pure organizational debt; no functional consequence.
  **Remediation:** address opportunistically during related work.

- [ ] **LOW-12: 9 file-name and 7 identifier-naming violations** — `chain/testvector.go` (improper test-name pattern), `bridge/errors.go` (generic), `client/client.go` (stuttering), `client/types.go` (generic), `config/config.go` (stuttering), `internal/server/server.go` (stuttering), `metrics/metrics.go` (stuttering), `namedb/namedb.go` (stuttering), `wallet/wallet.go` (stuttering); identifier `ChainParams` on `chain.BlockChain`, `ClientMode`, `ConfigPath`, `LoadTestConfig`, `MetricsSnapshot`, single-letter `b` in `rpc/ratelimit.go:77`, stuttering `walletPath` method — *Style* — Conventional Go style violations flagged by stats analyzer.
  **Remediation:** address as part of v1.0 API freeze planning; these are breaking changes if applied to exported identifiers.

## False Positives Considered and Rejected

| Candidate | Reason Rejected |
|-----------|----------------|
| Loop-variable-capture in goroutine closures (3f checklist item) | Project declares `go 1.24` in `go.mod`; the 1.22+ semantics already make each iteration's loop variable scoped — no real risk. |
| Race on `result.ErrorDetails` append in `loadtest/runner.go:226` (raised by sub-audit) | The append is inside `counters.errorMu.Lock()/.Unlock()` (lines 224-228). Not a race. |
| Nil bbolt-bucket panics raised individually for ~30 sites in `namedb` | Buckets are created at DB open (`namedb/namedb.go:158-162`). Consolidated into MED-10 rather than 30 separate findings. |
| `defer client.Close()` in `mail/smtp.go:436` ignoring return error | Standard Go idiom for `Closer` cleanup; the connection is being torn down anyway and any error would not change control flow. Not a bug. |
| `s.logger.*` nil dereferences in `rpc/server.go` (HIGH from a sub-audit) | `s.logger` is set in `NewServer` from `logging.GetDefault()` and never mutated; cannot be nil in practice. Recorded as LOW-1 only. |
| `parseHashParam` not bounds-checking `params[index]` | All call sites validate `len(params)` via `parseInterfaceParams(minCount, …)` before calling this helper. No reachable panic. |
| JSON-RPC response missing `id` field in `listunspent` error path (HIGH from a sub-audit) | Verified that the calling site embeds the `id` into the outer `Response` before returning to the client; the inner helper returns a value that's overlaid. Not actually missing on the wire. |
| `lookupActiveNameRecord` "expired" check uses `<` (raised as HIGH-3 in chain audit) | This is correct per project convention (memory note: `ExpiresAt < currentHeight` ⇒ expired). The associated test (HIGH-4) is the actual bug. |
| README "AuxPow" / "merged mining" advertised but missing | Documented in pre-existing `docs/development/PROTOCOL_COMPLIANCE_AUDIT.md`; surfaced into `GAPS.md` rather than re-flagged as a finding. |

## Remaining Scope

None — full coverage pass complete; all 14 production packages and all production `.go` files were inspected across every checklist category. No follow-up session required.
