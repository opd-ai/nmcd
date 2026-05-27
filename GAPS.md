# Implementation Gaps — 2026-05-27

This document records gaps between what `nmcd`'s README and module documentation claim and what the code actually delivers. Each gap lists the stated goal, the current state, the impact, and a specific, proportionate path to closing it. Gaps that are *bugs* are also referenced in `AUDIT.md`.

---

## G-01 — SemVer / v1.0.0 API stability promise

- **Stated Goal**: README and module documentation imply a stable, library-first Go API and reference SemVer for downstream consumers. The roadmap (`ROADMAP.md`) targets `v1.0.0` milestones.
- **Current State**: Module is on a pre-`v1` tag (no `/v2` import path; `go.mod` declares `module github.com/opd-ai/nmcd` without a version suffix). The RPC server hardcodes `"version": "0.1.0"` (`rpc/server.go:569`). `CHANGELOG.md` shows ongoing breaking changes in the embedded API surface.
- **Impact**: Downstream library consumers cannot rely on SemVer guarantees; importing nmcd into production code is risky because any future minor release can break compilation. Operators using the JSON-RPC `getinfo`/`version` field for compatibility checks will get a stale value.
- **Closing the Gap**:
  1. Centralize the version string in a single `internal/version` package and source it from both `cmd/nmcd` (build-time `ldflags -X`) and `rpc/server.go`.
  2. Add a `CompatibilityGuarantees.md` documenting which packages are stable (`client`, `namedb`) vs experimental (`bridge`, `loadtest`).
  3. Tag the first SemVer-stable release (`v1.0.0`) once the items in `ROADMAP.md` are closed.

---

## G-02 — Expiration boundary semantics inconsistent across the public API

- **Stated Goal**: Namecoin's documented consensus rule is "a name expires `12000` blocks after registration" — a single, unambiguous predicate.
- **Current State**: Three different boundary conventions coexist in the codebase:
  - **Consensus path** (`chain/blockchain.go:597, 1555`) treats a name as **active** while `ExpiresAt >= height`.
  - **Most callers** (`rpc/name_handlers.go:163`, `chain/blockchain.go:656, 1601`, `client/embedded.go:295, 710, 896`, `namedb/namedb.go:316-318`) treat a name as **expired** with strict `ExpiresAt < currentHeight`.
  - **Two pre-broadcast guards** (`rpc/name_handlers.go:354` in `checkNameNotActive`, `client/embedded.go:481` in `checkNameAvailability`) use `ExpiresAt > bestHeight` — *less strict* than every other call site.
- **Impact**: At the precise block where `ExpiresAt == bestHeight`, the two pre-broadcast guards will accept a name as "still active" while users who query via `name_show` or scan via `name_scan` may see different status. This is a user-visible inconsistency at exactly the moment a name becomes available for re-registration.
- **Closing the Gap**: Pick a single canonical helper (`func (n *NameRecord) IsExpired(height int32) bool { return n.ExpiresAt < height }`) in `namedb` and use it everywhere. Replace the two `>` guards with `>=`. Add a regression test that asserts the same answer from every call site at the boundary height. Cross-referenced in `AUDIT.md` finding M-01.

---

## G-03 — "Parallel block download" claim during IBD

- **Stated Goal**: README describes headers-first IBD with parallel block download.
- **Current State**: `SyncManager.HandleHeaders` (`network/sync.go:171-225`) iterates the received header batch and issues `getdata` requests against the *single* current sync peer for each header. There is no fan-out, no per-peer outstanding-request budget, and no concurrent retrieval from multiple peers.
- **Impact**: IBD throughput is capped at the bandwidth of one peer. Users with multiple connected peers gain no speedup, contradicting the README claim.
- **Closing the Gap**: Distribute `requestBlock` calls across the entries of `sm.peerHeights` using a round-robin or weighted-by-height strategy, with a per-peer outstanding-request budget (e.g. 16 blocks/peer). Add a multi-peer integration test that verifies block requests are spread across peers. Cross-referenced in `AUDIT.md` finding L-08.

---

## G-04 — "Thread-safe / safe for concurrent use" docstring claims vs documented locking discipline

- **Stated Goal**: Multiple types declare in their GoDoc that they are "safe for concurrent use by multiple goroutines" (`mail.Relay`, `wallet.Wallet`, `namedb.NameDB`, `chain.BlockChain`, `network.PeerManager`, `network.SyncManager`).
- **Current State**: Empirically the test suite passes `go test -race ./...` with no detected races. However, the codebase does not document its **lock-order graph**, and at least one cross-package lock acquisition exists (`SyncManager.findReplacementPeer` at `network/sync.go:298-309` acquires `sm.pm.mu` while `sm.mu` is held by the caller). Lock-order inversions are not detected by the race detector and could be introduced by future contributors.
- **Impact**: Operationally low today, but the absence of a documented lock order is a maintainability gap that contradicts the strength of the "thread-safe" claim. A future change could deadlock the daemon under high concurrency.
- **Closing the Gap**: Add a `concurrency.md` (or top-of-file comment block in `network/sync.go` and `network/peermgr.go`) documenting the lock acquisition order. Refactor `findReplacementPeer` to snapshot the peer list under `sm.pm.mu` *without* holding `sm.mu`. Cross-referenced in `AUDIT.md` finding L-11.

---

## G-05 — SMTP relay DoS resistance

- **Stated Goal**: The `permamail` README presents the SMTP relay as a production-ready way to receive mail addressed to `.bit` domains and forward it via an upstream SMTP server.
- **Current State**:
  - No cap on concurrent connections (`mail/smtp.go:200-218`).
  - Read deadline is set once at session start (default 5 minutes), allowing a slow client to hold a goroutine for the entire window with no per-read renewal (`mail/smtp.go:226-230`, `:603-635`).
  - `MaxMessageSize` overflow check uses `int(len(body)+len(line))` which can wrap on 32-bit platforms (`mail/smtp.go:628`).
- **Impact**: A modest attacker can exhaust goroutines and file descriptors on a public-facing relay deployment. This is at odds with positioning `permamail` as "production-ready."
- **Closing the Gap**:
  1. Add a connection-count semaphore in `Relay` (`mail/smtp.go:102-110`).
  2. Refresh `conn.SetReadDeadline` on every read in `readLine` and `readDataBody`.
  3. Use `int64` arithmetic in the size cap.
  Cross-referenced in `AUDIT.md` findings M-04, L-14, L-30.

---

## G-06 — Wallet `Lock()` "clears keys from memory" claim

- **Stated Goal**: `Wallet.Lock()` GoDoc says it "locks an encrypted wallet, clearing keys from memory" (`wallet/wallet.go:388-389`).
- **Current State**: `Lock()` clears the `w.keys` map and zeroes the password slice, but the private-key bytes inside each `btcec.PrivateKey` are *not* zeroed — `kp.PrivateKey.Serialize()` returns a copy and only that copy is zeroed (`wallet/wallet.go:408-414`). The inline comment at `wallet/wallet.go:402-407` explicitly acknowledges this limitation, but the exported GoDoc does not.
- **Impact**: A forensic memory dump of the daemon process *after* `Lock()` may still contain the private key bytes until garbage collection runs. Users who relied on the GoDoc to plan an operational lock-on-idle workflow may overestimate the protection.
- **Closing the Gap**: Surface the limitation in the exported GoDoc of `Lock()`. Consider holding raw key bytes in a wallet-owned `[]byte` slice (or `memguard`-style buffer) instead of `btcec.PrivateKey` so true zeroing is possible. Cross-referenced in `AUDIT.md` finding L-12.

---

## G-07 — Hardened RPC defaults vs documented behavior

- **Stated Goal**: README describes JSON-RPC as Namecoin-Core-compatible with optional basic-auth and rate limiting.
- **Current State**:
  - When `RPCUser == "" && RPCPassword == ""` and the listen address is not loopback, the server logs a warning *but starts anyway* (`rpc/server.go:228-236`).
  - The HMAC concatenation `user + ":" + pass` (`rpc/server.go:343-349`) is ambiguous if either field contains `:`.
  - There is no operator-rotation API for credentials.
- **Impact**: Operators deploying behind misconfigured firewalls may unintentionally expose an unauthenticated wallet+RPC interface to the internet. The README emphasizes safety-by-default but the code merely warns.
- **Closing the Gap**:
  1. Change the default in `NewServer` to *refuse to start* when listening on a non-loopback address without credentials, with an explicit opt-out flag `--allow-unauth-public`.
  2. Use distinct `mac.Write([]byte(user))` and `mac.Write([]byte(pass))` calls.
  3. Add a credential-rotation method on `Server`.
  Cross-referenced in `AUDIT.md` findings L-05, L-10.

---

## G-08 — Documentation gaps in `config` and `bridge` packages

- **Stated Goal**: The README presents `config.NamecoinMainNetParams` and bridge-adapter examples as primary public surfaces.
- **Current State**: `go-stats-generator` reports the `config` package at doc-coverage ~0.6 and cohesion ~0.6; `bridge` at cohesion ~0.4. Many exported constants in `config/namecoin_params.go` lack per-constant GoDoc.
- **Impact**: Library users hand-tuning consensus parameters lack inline guidance; `go doc` output is sparse.
- **Closing the Gap**: Add per-constant GoDoc comments in `config/namecoin_params.go`. Split `bridge` into role-focused subpackages if cohesion does not improve after documentation. Cross-referenced in `AUDIT.md` findings L-24, L-25.

---

## G-09 — `loadtest.RPCLoadTest` input validation

- **Stated Goal**: README and `loadtest` package doc present `RPCLoadTest` as a public Go API for load and stress testing.
- **Current State**: `RPCLoadTest` divides by `config.Concurrency` without validation (`loadtest/runner.go:183`), panicking on `Concurrency == 0`. Workers may also remain blocked on in-flight HTTP calls for up to 30s after shutdown (`loadtest/runner.go:206-238`).
- **Impact**: A library user passing an unconfigured `LoadTestConfig{RateLimit: 10}` crashes their program. Shutdown latency can confuse callers who expect a clean return at `Duration + small ε`.
- **Closing the Gap**: Validate `Concurrency > 0` and `Duration > 0` at the top of `RPCLoadTest`. Pass a `context.WithCancel`-derived context to workers and cancel it on shutdown. Cross-referenced in `AUDIT.md` findings H-01 and M-03.
