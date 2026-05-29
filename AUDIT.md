# UNIVERSAL BUG AUDIT (END-TO-END) — 2026-05-29

## Project Profile
- **Purpose**: `nmcd` is a pure-Go, library-first Namecoin implementation built on btcd
  libraries. It can be embedded in-process (full blockchain + bbolt name DB + P2P stack)
  or run as a standalone daemon exposing a JSON-RPC interface. It also ships an SMTP relay
  (`permamail`) that routes mail using Namecoin name records.
- **Target users**: Go developers embedding name resolution/registration, and operators
  running a Namecoin daemon / mail relay.
- **Deployment model**: 64-bit hosts (server/desktop). RPC bound to `127.0.0.1:8336` by
  default; P2P listener on `0.0.0.0:8334`. The wallet stores keys encrypted at rest.
- **Critical paths** (deepest scrutiny):
  1. `chain` — block processing and Namecoin consensus validation (name ops, AuxPow,
     subsidy, PoW). A bug here forks the node from the network.
  2. `namedb` — bbolt-backed name/UTXO storage with an LRU cache and reorg restore logic.
  3. `network` — P2P peer management, mempool, headers-first sync (untrusted peer input).
  4. `wallet` / `rpc` — key encryption and the untrusted JSON-RPC input boundary.
- **Trust boundaries**: untrusted input enters via (a) P2P messages/blocks/txs → `network`
  → `chain` (script parsing in `chain/name_script.go`), (b) JSON-RPC params → `rpc`, and
  (c) SMTP commands and name-record-derived upstream hosts → `mail`.

## Audit Scope
- **Packages audited**: all non-example packages — `chain`, `namedb`, `network`, `wallet`,
  `rpc`, `mail`, `metrics`, `bridge`, `config`, `client`, `internal/logging`,
  `internal/server`, `internal/version`, `loadtest`, plus root (`main`, `integration_test`),
  `cmd/nmcd`, `cmd/permamail`. Example programs under `examples/` were not deeply audited
  (non-production, no test files).
- **Functions inspected**: 241 free functions + 467 methods (go-stats-generator). All 17
  functions exceeding complexity 15 **or** 50 code lines were manually reviewed (see Metrics).
- **Tooling baseline**: `go test -race ./...` → all 16 test packages pass; `go vet ./...` →
  0 warnings; `go-stats-generator` JSON/structural baseline collected (then `tmp/` deleted).

## Coverage Log
| Package | 3b Logic | 3c Nil | 3d Errors | 3e Resources | 3f Concurrency | 3g Security | 3h Aliasing | 3i Init | 3j API |
|---------|----------|--------|-----------|--------------|----------------|-------------|-------------|---------|--------|
| chain | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| namedb | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| network | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| wallet | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| rpc | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| mail | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| config | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| client | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| bridge | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| metrics | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| internal/logging | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| internal/server | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| internal/version | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| loadtest | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

## Goal-Achievement Summary
| Stated Goal | Status | Blocking Findings |
|-------------|--------|-------------------|
| "Automatic Initial Block Download (IBD) and ongoing sync with the network" | ❌ | CRIT-1 |
| Faithful Namecoin consensus validation (name ops) | ❌ | CRIT-1, MED-5 |
| Pure-Go embedded library / daemon modes | ✅ | — |
| Thread-safe: "Mutex protection for all shared state" | ✅ | (no race detected; `-race` clean) — see LOW-8 |
| Wallet keys encrypted at rest (AES-256-GCM + scrypt) | ✅ | — |
| Name resolution with expiration checking | ✅ | — |
| JSON-RPC server | ⚠️ | MED-2 |
| SMTP relay via name records (`permamail`) | ⚠️ | MED-4, LOW-2, LOW-3 |

## Findings

### CRITICAL
- [ ] **CRIT-1: Name-operation validation is far stricter than Namecoin consensus, which rejects valid mainnet blocks and halts IBD** — `chain/name_script.go:381-433` (`validateNameFormat` / `validateValueEncoding`), enforced on every block at `chain/blockchain.go:759` (called from `ProcessBlock` → `validateNameOperations` `chain/blockchain.go:307,689,748-762`); namespace allowlist at `config/config.go:96-100` — **bug class: API/behavioral contract + logic (consensus divergence)** — **Consequence:** `validateNameOutputs` runs `validateNameFormat` for every `NAME_FIRSTUPDATE`/`NAME_UPDATE` output in every block during sync. It rejects the block (returns error from `ProcessBlock`) when: (a) the name does not start with `d/`, `id/`, or `p/` (`config.IsValidNamespace`); (b) the value of a `d/`/`id/` name is not valid JSON; or (c) any value is not valid UTF-8. Namecoin consensus enforces **none** of these — namespace prefixes are conventions only, values are arbitrary bytes up to 520, and JSON is not required (verified against Namecoin consensus rules). The real mainnet chain contains name transactions that violate all three constraints, so the first such block stalls IBD permanently, and the node can never follow the network — directly defeating the README's "Automatic IBD and ongoing sync with the network" goal. Data path: P2P block → `network` → `chain.ProcessBlock` (`blockchain.go:242`) → `validateNameOperations` (`:307`) → `validateTransactionNameOps` (`:698`) → `validateNameOutputs` (`:748`) → `validateNameFormat` (`:759`) → error → block rejected (`:307-312`). *Empirical note:* the sandbox cannot reach mainnet, so this was confirmed by source analysis + the documented Namecoin consensus rules, not by a live sync; the existing test suite uses only conforming JSON `d/` values, which is why `go test -race` passes. — **Remediation:** In `chain/name_script.go`, scope the JSON/UTF-8/namespace checks to *local creation* (wallet/RPC registration) only; for consensus block validation, validate only what Namecoin enforces (name length ≤ 255, value length ≤ 520, correct script structure). Concretely, stop calling `validateNameFormat` from `validateNameOutputs` (`blockchain.go:759`) and instead apply a consensus-only length/structure check there, leaving `validateValueEncoding`/`IsValidNamespace` for `wallet`/`rpc` registration paths. Validate with a regtest/testnet sync and a new unit test feeding a non-JSON `d/` value and a non-`d//id//p/` name through `ProcessBlock` and asserting acceptance (`go test -race ./chain/`).

### HIGH
- [ ] *(none confirmed above MEDIUM after false-positive filtering; the most serious issue, CRIT-1, is consensus-blocking.)*

### MEDIUM
- [ ] **MED-1: `OP_PUSHDATA4` length parsing sign-extends on 32-bit platforms, bypassing the bounds check and panicking on adversarial scripts** — `chain/name_script.go:358-376` — **bug class: arithmetic overflow / nil-boundary on untrusted input** — **Consequence:** `dataLen` is built as a signed `int` from 4 attacker-controlled bytes via `<<24`. On a 32-bit build, a length byte `≥ 0x80` produces a negative `dataLen`; the guard `offset+dataLen > len(script)` (`:372`) then passes, and `script[offset:offset+dataLen]` (`:376`) panics with a negative high index while parsing a peer-supplied transaction/script. On 64-bit (the documented deployment) `int` is 64 bits, so the value stays positive and the bounds check holds — hence MEDIUM, not HIGH. The `fuzz_test.go` length oracle explicitly guards `expectedLen >= 0`, indicating awareness of the boundary. — **Remediation:** Parse the `OP_PUSHDATA4` length as `uint32` (or `uint64`) and reject `dataLen > maxScriptElementSize` before slicing, in `readPushData` (`name_script.go:355-364`). Validate with `go test -race ./chain/` and the existing fuzz target `FuzzParseNameScript`.
- [ ] **MED-2: `walletpassphrase` timeout has no upper bound, causing `time.Duration` overflow** — `rpc/wallet_handlers.go:144-152` and `:119` — **bug class: logic / integer overflow** — **Consequence:** `timeout := int(timeoutFloat)` accepts any positive magnitude (e.g. `1e18`); `time.Duration(timeout)*time.Second` (`:119`) then overflows the int64 nanosecond counter and wraps, so the auto-lock `time.AfterFunc` fires at an unintended time (potentially immediately, re-locking the wallet, or after a wrapped/huge delay). The call is authenticated (self-inflicted), limiting impact, but the wallet's auto-lock guarantee is silently violated. NaN/Inf inputs are already caught by the `timeout <= 0` check, so only the missing upper bound is the bug. — **Remediation:** In `parsePassphraseTimeout`, reject timeouts above a sane cap (e.g. `> 86400*365`) before converting, mirroring the lower-bound check. Validate with a unit test passing `timeout = 1e18` and asserting an error (`go test -race ./rpc/`).
- [ ] **MED-3: Batch expiration-index maintenance swallows decode errors, leaving the expiration index inconsistent** — `namedb/batch.go:196-198` (`updateExpirationIndex`) and `namedb/batch.go:238-240` (`removeExpirationIndex`) — **bug class: error handling (not propagated) → data integrity** — **Consequence:** When an existing on-disk record fails to decode, the function logs and returns `nil`, so the **old** expiration key is never deleted while the **new** key is still written (in `writeNames`) — producing duplicate/stale entries. `GetExpiredNames` then reports a name at the wrong height, and reorg restore (`RestoreExpiredNamesForBlock`) may process it twice. Reachability requires already-corrupted/version-mismatched on-disk data (the same package encodes records), so it is a latent integrity bug rather than externally triggerable — hence MEDIUM. — **Remediation:** Return the decode error to abort the batch (surface the corruption) instead of logging-and-continuing, in both functions. Validate with a unit test that injects a corrupted record and asserts the batch fails (`go test -race ./namedb/`).
- [ ] **MED-4: Batch UTXO address-index removal swallows decode errors, leaving dangling address-index entries** — `namedb/batch.go:371-374` (`removeUTXOFromAddressIndex`) — **bug class: error handling (not propagated) → data integrity** — **Consequence:** On decode failure the main UTXO is deleted (`deleteUTXOs`) but its address-index entry is kept, so `GetUTXOsForAddress` repeatedly resolves a now-missing UTXO (skipped at read time) and the index grows with dangling references. Same reachability caveat as MED-3 (requires corrupted on-disk data). — **Remediation:** Propagate the decode error to abort the batch. Validate with `go test -race ./namedb/`.
- [ ] **MED-5: SMTP forwarding uses `context.Background()` with no deadline and net/smtp dials without timeout, so a dead upstream blocks the session indefinitely** — `mail/smtp.go:443-450` (`handleData`) → `forwardMessage`/`connectUpstream` `mail/smtp.go:497-548` (`smtp.Dial`/`tls.Dial`) — **bug class: resource lifecycle / availability** — **Consequence:** The `ctx` passed to `forwardMessage` is never wired into a dial/IO deadline (net/smtp ignores context), and `connectUpstream` uses `smtp.Dial`/`tls.Dial` with no timeout. An unresponsive upstream SMTP server hangs the per-connection goroutine, holding the slot until the OS TCP timeout (or forever for a half-open peer). A few stuck connections exhaust the relay. — **Remediation:** Replace bare `smtp.Dial`/`tls.Dial` with a `net.DialTimeout` (or `tls.DialWithDialer` with a timeout) and set `client`/`conn` deadlines derived from a bounded context in `connectUpstream`. Validate by adding a test that points the relay at an unaccepting TCP listener and asserts the forward fails within the timeout (`go test -race ./mail/`).
- [ ] **MED-6: Consensus value-length limit (1023) is more permissive than Namecoin's actual 520-byte cap** — `config/config.go:23-27,74-80`; enforced at `chain/name_script.go:415` via `config.MaxValueLength` — **bug class: API/behavioral contract (consensus divergence)** — **Consequence:** Namecoin consensus rejects name values over 520 bytes, but nmcd treats 1023 as the consensus limit (the in-repo comment even asserts "consensus limit is 1023 bytes"). nmcd will therefore *accept* a block/transaction carrying a 521–1023-byte value that the real network rejects, diverging from consensus (a fork risk on a crafted block, and over-permissive mempool acceptance). The honest mainnet chain never exceeds 520, so this does not block IBD — hence MEDIUM, secondary to CRIT-1. — **Remediation:** Set the consensus value-length limit used by block validation to 520 (Namecoin `MAX_VALUE_LENGTH`); keep any UI limit separate. Validate with `go test -race ./chain/ ./config/` and a test asserting a 600-byte value is rejected by `ProcessBlock`.

### LOW
- [ ] **LOW-1: `onInv` ignores the error from `gdmsg.AddInvVect`** — `network/peermgr.go:340` — error handling — **Consequence:** the error is dropped; benign because `msg.InvList` is bounded by the wire protocol (`MaxInvPerMsg`), so `gdmsg` can never exceed the limit in practice. — **Remediation:** check and `break`/log on error for robustness; `go vet ./network/`.
- [ ] **LOW-2: SMTP `554` failure response leaks the internal error string to the client** — `mail/smtp.go:467` — security (information disclosure) — **Consequence:** internal details (e.g. "name not found for …") are returned to the SMTP client, contrary to RFC 5321 guidance. — **Remediation:** return a generic `"554 Transaction failed"` and log details server-side.
- [ ] **LOW-3: `MAIL FROM` accepts addresses without an `@`, inconsistent with `RCPT TO` validation** — `mail/smtp.go:383-392` — API/logic — **Consequence:** a malformed sender is accepted where the recipient path enforces a `.bit` domain; cosmetic/inconsistent, no exploit. — **Remediation:** validate the `MAIL FROM` address shape consistently with `RCPT TO`.
- [ ] **LOW-4: `pushData` truncates the length prefix for `OP_PUSHDATA2` payloads > 65535 bytes** — `wallet/wallet.go:666` — logic — **Consequence:** would emit an invalid script, but is unreachable: all callers push name (≤255), rand (small), or value (≤`MaxValueLength`=1023). Defensive-code smell only. — **Remediation:** add an `OP_PUSHDATA4` branch or return an error for oversized data.
- [ ] **LOW-5: Block version is parsed with sign-extension on 32-bit builds** — `chain/blockchain.go:189-190` — arithmetic — **Consequence:** harmless; the value is only used with bitwise `&` against `AuxPowVersionBit`, which is correct for negative values. — **Remediation:** parse as `uint32` for clarity.
- [ ] **LOW-6: Auto mode silently swallows `DetectNetwork` errors when falling back to embedded** — `client/client.go:69-89` — error handling — **Consequence:** a misconfigured/unreachable daemon is masked by a silent embedded fallback; behavior is intentional per the "auto" contract but loses diagnostics. — **Remediation:** log the suppressed error at WARN level.
- [ ] **LOW-7: `CalcBlockSubsidy` does not validate negative heights** — `config/subsidy.go:34-52` — logic — **Consequence:** a negative `height` yields negative `halvings`; `subsidy >>= uint(halvings)` shifts by a huge count and returns 0 (no panic). Heights are never negative on real paths. — **Remediation:** guard `height < 0` returning 0 explicitly.
- [ ] **LOW-8: Lock ordering `sm.mu` → `pm.mu` in `findReplacementPeer`** — `network/sync.go:308` (called with `sm.mu` held from `sync.go:95-99`) — concurrency — **Consequence:** establishes a `sm.mu`→`pm.mu` ordering; no reverse ordering was found and `go test -race` is clean, but the inversion risk should be documented to prevent future regressions. — **Remediation:** add a comment documenting the canonical lock order; keep `-race` in CI.

## Metrics Snapshot
| Metric | Value |
|--------|-------|
| Total functions (free) | 241 |
| Total methods | 467 |
| Functions above complexity 15 | 1 (`namedb.RestoreExpiredNamesForBlock`, cc=17) |
| Functions > 50 code lines | 17 (all manually reviewed) |
| Avg cyclomatic complexity | 3.50 (max 17) |
| Doc coverage (overall) | 83.5% (pkgs 87.5%, funcs 93.9%, types 79.7%, methods 81.5%) |
| Duplication ratio | 0.47% (7 clone pairs, all in `examples/`) |
| Test pass rate (`go test -race ./...`) | 16/16 packages OK |
| go vet warnings | 0 |

## False Positives Considered and Rejected
| Candidate | Reason Rejected |
|-----------|----------------|
| TOCTOU: `QueueMessage` on a peer that disconnects after the snapshot (`peermgr.go` relay/broadcast) | btcd `peer.QueueMessage` is safe on a disconnected peer (selects on the quit channel; message is dropped, no panic). |
| Data race on `SyncManager.peerHeights` read in `findReplacementPeer` (`sync.go:321`) | The sole caller (`sync.go:99`) holds `sm.mu`; `peerHeights` is consistently guarded by `sm.mu`. `-race` clean. |
| Off-by-one keeping 11 response-time samples (`peerscore.go:211`) | After `append` to 11, `len > 10` is true and `[1:]` trims back to 10; the cap is correct. |
| NaN/Inf `float64`→`int` for `timeout`/`count` (`wallet_handlers.go:148`, `name_handlers.go:609`) | Resulting `MinInt` is caught by the subsequent `<= 0` / range checks, returning an error. |
| Race in `metrics.Collect` iterating snapshot maps | `Snapshot()` deep-copies via `copyUint64Map`/`copyDurationMap`; the iterated maps are private copies. |
| Log-file FD leak in `logging.Init` | The file is `Close()`d on the `Chmod` error path and otherwise owned by the logger's `Close()`. |
| `daemon.go` reads `resp.Body` after the drain/close `defer` | The `defer` runs at function return, after the decode; ordering is correct. |
| `bridge.LookupMail` "double-wraps" error (`%w: %v`) | Wraps the sentinel plus the detail; messages are not actually duplicated — idiomatic, not a bug. |
| Nil-peer deref in `GetPeerInfo`/broadcast loops | btcd peers stored in the map are never nil; the existing `p != nil` guards are defensive only. |
| `nmcd` claims "Thread-Safe … Mutex protection for all shared state" | Verified by `-race` clean run across all packages and manual review of `namedb` cache/`mempool`/`syncmanager` locking. |

## Remaining Scope (session completed)
| Package | Status | Notes |
|---------|--------|-------|
| All non-example packages | ✅ Audited | Full pass completed; no new findings above LOW expected on re-pass. |
| `examples/*` | ⚠️ Light pass | Demonstration code, no test files; not production. Only the duplicated boilerplate (0.47% duplication) noted. |
