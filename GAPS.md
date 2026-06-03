# Implementation Gaps — 2026-06-03

Gaps between the documented product surface (README + referenced docs) and the actual
implementation of nmcd. Severity reflects user-facing impact, not effort. The codebase
builds cleanly and the full test suite passes; every gap below is **low severity** and
documentation- or UX-centric. No missing core capability was found.

---

## 1. permamail `register` / `update` are prepare-only, but documented imperatively

- **Stated Goal:** The README's Permamail section shows imperative usage —
  `permamail register alice --forward user@gmail.com` and
  `permamail update alice --forward newemail@proton.me --backup backup@proton.me` —
  implying the CLI performs the registration/update.
- **Current State:** `cmd/permamail/main.go:216` (`register`) and `:240` (`update`) only
  validate inputs, build the JSON mail config, and **print manual instructions** ("To
  complete registration, use the nmcd RPC interface… Full wallet integration is planned
  for future releases"). No NAME_NEW/NAME_FIRSTUPDATE/NAME_UPDATE transaction is created.
- **Impact:** A user following the Usage examples will believe the name was registered
  when it was not. The limitation *is* disclosed later in prose ("Note: Currently, name
  registration and updates require a running nmcd node…"), so this is a tone/UX mismatch
  rather than an undocumented gap.
- **Closing the Gap (docs-only, recommended):** Mark the `register`/`update` examples as
  "(prepares config; completion is manual today)" inline, next to the commands, so the
  caveat is co-located with the usage. **Or (feature):** wire the commands to the embedded
  client's `RegisterName`/`UpdateName` (which are fully functional) to deliver true
  end-to-end registration.

## 2. README "Test Coverage Status" numbers are stale (mostly understated)

- **Stated Goal:** README §Testing publishes specific per-package coverage figures
  (chain 68.1%, rpc 45.8%, network 43.5%, namedb 87.3%, config 98.6%, wallet 69.7%, …).
- **Current State:** Measured `go test -cover` this run: chain **79.8%**, rpc **81.0%**,
  network **75.9%**, namedb **81.9%**, config **94.9%**, wallet **83.7%** (full table in
  `AUDIT.md`). Several packages improved by 12–35 points; a few drifted down slightly.
- **Impact:** Low. The headline claim ("comprehensive test coverage") remains true and is
  in fact understated for the critical rpc/network/chain packages. Only the printed numbers
  are inaccurate, which can mislead contributors prioritizing coverage work.
- **Closing the Gap:** Regenerate the coverage table from a fresh `go test -cover ./...`
  run (or automate it in CI so it cannot drift), and update
  `docs/development/COVERAGE.md` to match.

## 3. "~18,000 lines of production code (excluding tests)" is approximate/stale

- **Stated Goal:** README §Daemon Features claims "~18,000 lines of production code
  (excluding tests)."
- **Current State:** Non-test Go is ~21,388 raw lines; `go-stats-generator` reports 10,498
  *logical* LOC across 70 non-test files. Neither matches 18,000 closely.
- **Impact:** Negligible — it is a descriptive figure, not a functional promise.
- **Closing the Gap:** Either soften to "~20K+ lines" or remove the precise count; if kept,
  define the counting method (raw vs. logical) to avoid ambiguity.

## 4. Previous `GAPS.md` was obsolete (now regenerated)

- **Stated Goal:** A `GAPS.md` accurately reflecting current gaps.
- **Current State:** The prior `GAPS.md` (dated 2026-05-29) listed three gaps that are all
  now **resolved** in the code:
  1. *Daemon-vs-embedded expired-name filtering drift* — both paths now agree: embedded
     filters on `record.ExpiresAt < bestHeight` (`client/embedded.go:907`) and the daemon
     filters on the RPC `expired` boolean (`client/daemon.go:586,598,631`), which is
     computed identically (`expiresIn < 0`).
  2. *Inconsistent `expires_in` across name methods* — `name_show`, `name_list`, and
     `name_scan` now all compute `expiresIn := record.ExpiresAt - bestHeight` (unclamped)
     and emit an explicit `expired` boolean (`rpc/name_handlers.go:48–55, 478–486,
     593–601`).
  3. *Caller-visible `ListFilter` mutation* — both clients now copy the filter before
     applying defaults (`client/embedded.go:880+`, `client/daemon.go:614+`).
- **Impact:** A stale gaps file misdirects remediation effort.
- **Closing the Gap:** Done — this file supersedes it. Consider regenerating `GAPS.md`
  as part of release tooling so it does not drift again.

## 5. Several implemented RPC methods are undocumented in the README

- **Stated Goal:** README §RPC API enumerates the supported Standard/Name/Wallet methods,
  presenting itself as the daemon's interface reference.
- **Current State:** The dispatch table (`rpc/server.go:558–628`) registers methods absent
  from the README: `name_scan`, `name_pending`, `getmetrics`, `getbalance`, `listunspent`,
  `getblock`, `getblockhash`, `getrawtransaction`, `sendrawtransaction`.
- **Impact:** Low and *positive direction* (more capability than advertised), but
  consumers may not discover useful endpoints, and the README implies the list is complete.
- **Closing the Gap:** Add the missing methods to the README RPC reference (or link to
  `docs/API.md`), or explicitly state the list is non-exhaustive.

---

## Out of Scope / Confirmed Non-Gaps

The following README promises were verified as **fully implemented and reachable**, so they
are *not* gaps: the library API (`ResolveName`/`RegisterName`/`UpdateName`/`ListNames`/
`GetNameHistory`/`Close`), all three modes (auto/embedded/daemon), embedded NAME_NEW →
NAME_FIRSTUPDATE registration, name deletion via empty `UpdateName`, all documented daemon
CLI flags, `/health` + `/ready` + `/metrics` endpoints, every documented Prometheus metric
(including those marked "New"), wallet encryption (AES-256-GCM + scrypt), all six mainnet +
one testnet DNS seeds, multi-platform release workflow and multi-arch Docker image, and the
RPC security stack (basic auth, rate limiting, request-size limit, security headers).
