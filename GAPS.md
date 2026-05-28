# Implementation Gaps — 2026-05-28

Gaps between what `nmcd` claims (README.md, ROADMAP.md, CHANGELOG.md, `docs/`) and what the code actually does. Each entry was verified against the source on the audited revision.

## G-1 — Production LOC count is overstated by ≈18%
- **Stated Goal**: README.md:104 — *"Focused Implementation: ~18,000 lines of production code (excluding tests)"*.
- **Current State**: `find . -name "*.go" -not -name "*_test.go" -not -path "./examples/*" | xargs wc -l` yields **21,245 lines** across non-test, non-example Go files (`tmp/baseline.json` `lines.total` per package totals to the same number).
- **Impact**: Sets an inaccurate expectation of audit surface area for downstream consumers and reviewers; the project is materially larger than advertised.
- **Closing the Gap**: Update README.md:104 to *"~21,000 lines of production code (excluding tests)"* and add a `make loc` target that prints the current count so it stays in sync with reality.

## G-2 — ROADMAP cites contradictory LOC metrics
- **Stated Goal**: ROADMAP.md:55 — *"go-stats-generator reports ~9,729 LOC (63 files, 14 packages)"*; ROADMAP.md:85 marks this as *"✅ Achieved"* against the README's ~18,000 target.
- **Current State**: Actual code base is 21,245 LOC across 16 packages and ~140 production Go files. The 9,729 figure cannot be reproduced with the tool as invoked in this audit. The comparison ("9,729 is well under 18,000") is also logically backwards if the goal is "≤18,000".
- **Impact**: Two different ground-truth metrics in the same repository undermine confidence in other roadmap claims.
- **Closing the Gap**: Re-run `go-stats-generator analyze . --skip-tests` and replace the stale numbers in ROADMAP.md:55 and ROADMAP.md:85 with the current values. If the original 9,729 figure excluded a subset (e.g., `cmd/` and `examples/`), state the exclusion explicitly.

## G-3 — RPC method coverage in README is incomplete
- **Stated Goal**: README.md "RPC Methods" section enumerates ~16 methods (5 standard, 6 name, 5 wallet). The README is the only user-facing reference.
- **Current State**: `rpc/server.go` registers at least 27 methods. README does not document: `getblock`, `getblockhash`, `getrawtransaction`, `sendrawtransaction`, `getmetrics`, `getbalance`, `listunspent`, `name_pending`, `name_scan`, and the `/health` endpoint sibling of `/ready` (`rpc/server.go:280`).
- **Impact**: Library and daemon users cannot discover supported endpoints from the documentation and must read source to find them. Some of the missing methods (`listunspent`, `name_scan`) are core to the project's stated use cases.
- **Closing the Gap**: Add a complete "RPC Methods" table to README.md (or extract to `docs/RPC.md`) covering every method registered in `rpc/server.go`, with one-line descriptions and `curl` examples. Add automated drift detection: a small test in `rpc/` that lists registered methods and fails if the doc file does not mention each.

## G-4 — `/health` endpoint exists but is undocumented
- **Stated Goal**: README.md:571-589 documents the `/ready` endpoint as the operator's health check.
- **Current State**: `rpc/server.go:281` also registers `/health` via `mux.HandleFunc("/health", s.withPanicRecovery(s.handleHealth))`, but `/health` is not mentioned in README, `docs/OPERATIONS.md`, or `docs/API.md`.
- **Impact**: Operators may rely solely on `/ready` (which can fail during initial sync) when `/health` (always-live liveness probe) is more appropriate for orchestration systems like Kubernetes.
- **Closing the Gap**: Add a `/health` subsection adjacent to `/ready` in README.md with the response shape and the recommended use (liveness vs. readiness).

## G-5 — Wallet docs imply strong password protection but use a low scrypt work factor
- **Stated Goal**: README.md highlights wallet features and the `walletpassphrase` RPC method; `docs/OPERATIONS.md` implies wallet-file confidentiality at rest is protected by the user's password.
- **Current State**: `wallet/encryption.go:202` uses `scrypt(N=16384, r=8, p=1)` for password verification (see AUDIT.md finding H-1) while the data-encryption path uses `N=32768`. Neither is at current OWASP guidance, and the gap is not disclosed in user-facing docs.
- **Impact**: Users with low-entropy passwords have a weaker offline-attack resistance than the documentation implies.
- **Closing the Gap**: Increase scrypt N in `hashPassword` and `deriveKey` to at least `1<<17` (preferably `1<<19`), document the chosen factor and the upgrade path in `docs/OPERATIONS.md`, and add a wallet-file version bump that re-hashes the password on next unlock.

## G-6 — README "Thread-Safe" claim is true for the public client but does not extend uniformly to RPC handler call paths
- **Stated Goal**: README.md (top of page) — *"Thread-Safe: All operations safe for concurrent use"*.
- **Current State**: The `client` package is thread-safe (verified). However, the `rpc` server's `name_update`/`name_new`/`name_firstupdate` handlers (`rpc/name_handlers.go:80,304,375`) nil-deref the blockchain field under valid-but-undocumented configurations (see AUDIT.md H-6/H-7/H-8). These panics are recovered, but the public surface is not "safe" for the documented `wallet-without-blockchain` topology.
- **Impact**: An operator who legitimately constructs `rpc.Server` with only a wallet (e.g., to expose wallet RPCs while delegating chain reads to a separate daemon) will see HTTP 500 panic-recovered responses for name operations.
- **Closing the Gap**: Either (a) add `requireBlockchain` guards (per AUDIT H-6/H-7/H-8) so the handlers degrade gracefully, or (b) require a non-nil blockchain in `rpc.NewServer` and document that mixed mode is unsupported.

## G-7 — `examples/mail_router` ships placeholder mocks that may be mistaken for real integrations
- **Stated Goal**: `examples/mail_router/main.go` is referenced from `docs/EXAMPLES.md` as a working example of router integration.
- **Current State**: Lines 124, 128, 132, 136, 140, 144 return `errors.New("not implemented in mock")`. This is correct for a mock but the example is positioned as a "router integration" reference and a casual reader may believe the integration is functional.
- **Impact**: Users following the example may copy stub returns into production code.
- **Closing the Gap**: Add a prominent comment at the top of `examples/mail_router/main.go` clarifying that the resolver methods are mock-only and link to a real implementation in `docs/EXAMPLES.md`.

## G-8 — `name_scan` and `listaddresses` are implemented but absent from the "Wallet Methods" section
- **Stated Goal**: README.md "Wallet Methods" enumerates `getnewaddress`, `listunspent`, `walletpassphrase`, etc.
- **Current State**: `listaddresses` is registered in `rpc/server.go` (case statement near the standard RPC switch) and is mentioned in passing at README.md:483 but not formatted as a first-class wallet method. `name_scan` is similarly registered but not documented at all.
- **Impact**: Operators cannot enumerate names by namespace or list wallet addresses without reading the source.
- **Closing the Gap**: Promote both to first-class entries in the README RPC tables and add `curl` examples mirroring the existing wallet-method entries.
