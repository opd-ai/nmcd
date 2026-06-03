# Product Completeness Audit — nmcd

- **Date:** 2026-06-03
- **Scope:** Verify that every documented feature, capability, and user-facing promise in the README (and the docs/examples it references) is fully implemented, functional, and accessible to the target audience.
- **Mode:** Report only — no source code was modified.
- **Method:** README-driven product-surface extraction, cross-referenced against the source tree, the public API, the RPC dispatch table, the metrics registry, CI/release configuration, and a full build + test run.

## Verification Environment

| Step | Result |
|------|--------|
| `go build ./...` | ✅ Succeeds |
| `go test ./...` | ✅ All packages pass (no failures) |
| `go-stats-generator analyze . --skip-tests` | ✅ 70 files, 16 packages, 10,498 logical LOC, 334 functions, 523 methods, 95 structs, 3 interfaces |
| Tooling | `go-stats-generator` installed via `go install github.com/opd-ai/go-stats-generator@latest` |

## Summary Verdict

nmcd is a **substantially complete** product. Nearly every feature claim in the README maps to working, tested code that is reachable by the stated target audience (Go library consumers, daemon operators, and permamail users). The build is green and the entire test suite passes.

The gaps that remain are **low-severity and documentation-centric** rather than missing functionality:

1. The permamail `register`/`update` CLI commands are intentionally *prepare-only* (they print manual RPC instructions instead of completing on-chain registration). This is disclosed in prose but contradicted by the imperative tone of the CLI usage examples.
2. The README's "Test Coverage Status" table is **stale** — most packages now have materially *higher* coverage than documented (e.g., rpc 45.8% → 81.0%, network 43.5% → 75.9%).
3. The "~18,000 lines of production code" figure is approximate/stale relative to the current tree.
4. The previous `GAPS.md` (dated 2026-05-29) is **obsolete**: all three gaps it listed have since been resolved in the code.
5. Several implemented RPC methods are not listed in the README (a documentation-completeness omission, not a missing feature).

Details and remediation are in `GAPS.md`.

---

## Product Surface → Implementation Matrix

### 1. Library Public API (`client` package)

| Documented promise | Evidence | Status |
|--------------------|----------|--------|
| `ResolveName(ctx, name) → NameRecord` | `client/types.go:103`, `client/embedded.go` | ✅ Implemented |
| `RegisterName(ctx, name, value, opts) → TxResult` (two-step NAME_NEW → NAME_FIRSTUPDATE) | `client/types.go:112`; embedded path creates & broadcasts NAME_NEW then completes registration (`client/embedded.go` `RegisterName`/`completeRegistration`) | ✅ Implemented & functional |
| `UpdateName(ctx, name, value, opts) → TxResult` | `client/types.go:117`, `client/embedded.go:640+` | ✅ Implemented |
| `ListNames(ctx, filter) → []NameRecord` with namespace/address/pattern filters + pagination | `client/types.go:121`, `client/embedded.go` `normalizeListFilter`/filtering, `client/daemon.go` `matchesListFilter` | ✅ Implemented |
| `GetNameHistory(ctx, name) → []NameRecord` | `client/types.go:125` | ✅ Implemented |
| `Close()` | `client/types.go:137` | ✅ Implemented |
| Context support (timeouts/cancellation) | `checkClientState(ctx)` guards in embedded client | ✅ Implemented |
| Name deletion via `UpdateName(..., "", nil)` | README §"Name Deletion"; UpdateName accepts empty value | ✅ Supported |
| Thread-safe / concurrent use | RWMutex guards; `ListFilter` is copied before normalization in both clients (`client/embedded.go:880+`, `client/daemon.go:614+`) | ✅ Implemented |

### 2. Operational Modes

| Mode | Documented behavior | Evidence | Status |
|------|--------------------|----------|--------|
| Auto | Detect daemon on localhost:8336, else embedded | `client/client.go:47` `NewClient`, `case ModeAuto` | ✅ |
| Embedded | In-process chain/db/network; DNS-seed sync when `MaxPeers>0` (default 8); offline when `MaxPeers=0`; custom `BootstrapPeers` | `client/types.go` Config fields, `client/embedded.go` | ✅ |
| Daemon | RPC client to nmcd/Namecoin Core | `client/daemon.go` `NewDaemonClient` | ✅ |
| Config knobs: `DisableWallet`, `RateLimit`, `MaxRequestSize`, `MaxPeers`, `BootstrapPeers` | `client/types.go` | ✅ Present |

### 3. Daemon — JSON-RPC Methods

Dispatch verified in `rpc/server.go:558–628`.

| Category | Documented methods | Status |
|----------|-------------------|--------|
| Standard | `getinfo`, `getblockcount`, `getbestblockhash`, `getconnectioncount`, `getpeerinfo` | ✅ All present |
| Name | `name_new`, `name_firstupdate`, `name_show`, `name_list`, `name_history`, `name_update` | ✅ All present |
| Wallet | `getnewaddress`, `listaddresses`, `encryptwallet`, `walletpassphrase`, `walletlock` | ✅ All present |

Implemented **beyond** what the README documents (see GAPS.md §5): `name_scan`, `name_pending`, `getmetrics`, `getbalance`, `listunspent`, `getblock`, `getblockhash`, `getrawtransaction`, `sendrawtransaction`.

### 4. Daemon — CLI Flags

Verified in `cmd/nmcd/main.go:129–141`.

| Documented flag | Status |
|-----------------|--------|
| `-datadir`, `-network`, `-rpcaddr`, `-addpeer`, `-rpcuser`, `-rpcpassword`, `-prometheusaddr` | ✅ All present |
| Also present: `-config`, `-listen`, `-maxpeers`, `-loglevel`, `-logformat`, `-logoutput` | ✅ Bonus |

### 5. Health, Readiness & Metrics

| Promise | Evidence | Status |
|---------|----------|--------|
| `/health` liveness endpoint (200/503) | `rpc/server.go:297,815` | ✅ |
| `/ready` readiness endpoint (200/503) | `rpc/server.go:298,858` | ✅ |
| `/metrics` Prometheus endpoint (opt-in via `-prometheusaddr`) | `internal/server/server.go:153,162` | ✅ |
| All documented metric names (incl. items marked "New": namedb size/latency, rpc requests/duration, errors_total by type, go runtime) | Spot-checked 15 representative metrics against `metrics/*.go` — all found | ✅ |

### 6. Wallet & Encryption

| Promise | Status |
|---------|--------|
| `wallet.json` storage, 0600 permissions, unencrypted by default | ✅ (`wallet/wallet.go`) |
| AES-256-GCM + scrypt (N=32768) encryption, lock/unlock/timeout | ✅ (`wallet/encryption.go`, RPC handlers) |

### 7. Permamail (Decentralized Email Forwarding)

| Promise | Evidence | Status |
|---------|----------|--------|
| `permamail lookup` | `cmd/permamail/main.go` | ✅ Functional |
| `permamail serve` (SMTP relay) | `mail/smtp.go`, `mail/router.go` | ✅ Functional |
| Bridge adapter / mail router / SMTP relay components | `bridge/`, `mail/router.go`, `mail/smtp.go` | ✅ Present |
| `permamail register` / `update` | `cmd/permamail/main.go:216,240` | ⚠️ **Prepare-only** — prints manual RPC instructions, does not complete registration (disclosed as "planned for future releases"). See GAPS.md §1 |

### 8. Documentation & Examples

| Referenced asset | Status |
|------------------|--------|
| docs: INSTALLATION, OPERATIONS, API, EMBEDDING, EXAMPLES, MODES, PERFORMANCE, development/COVERAGE | ✅ All exist |
| examples: simple_resolve, embedded_client, register_name, update_name, list_names, namedb, bridge_adapter, mail_router, smtp_relay | ✅ All exist (plus prometheus_exporter, shared, systemd) |

### 9. Build, Release & Distribution

| Promise | Evidence | Status |
|---------|----------|--------|
| `make build` produces `nmcd` + `permamail` | `Makefile` | ✅ |
| Multi-platform release binaries (linux amd64/arm64, darwin amd64/arm64, windows amd64) | `.github/workflows/release.yml:74+` | ✅ |
| Multi-arch Docker image (`ghcr.io/opd-ai/nmcd`), multi-stage build | `Dockerfile`, release workflow | ✅ |
| Mainnet/testnet DNS seeds | `config/seeds.go:10–20` — all six mainnet + one testnet seed match README | ✅ |

### 10. Security Features

| Promise | Status |
|---------|--------|
| HTTP Basic Auth with constant-time comparison | ✅ (`rpc/server.go`) |
| Per-IP token-bucket rate limiting (100/min) | ✅ (`rpc/ratelimit.go`) |
| Request size limit (default 1MB) | ✅ |
| Security headers (nosniff / DENY / CSP none) | ✅ |
| Name-operation validation (uniqueness, value ≤1023 bytes, name 1–255 chars) | ✅ (`chain/`, `rpc/name_handlers.go`) |

---

## Coverage Claims vs. Measured (this run)

The README "Test Coverage Status" section is **out of date**. Measured `go test -cover` results:

| Package | README claim | Measured | Drift |
|---------|--------------|----------|-------|
| chain | 68.1% | **79.8%** | +11.7 (understated) |
| rpc | 45.8% | **81.0%** | +35.2 (understated) |
| network | 43.5% | **75.9%** | +32.4 (understated) |
| namedb | 87.3% | 81.9% | −5.4 |
| bridge | 100.0% | 100.0% | 0 |
| client | 82.9% | 82.8% | ≈0 |
| config | 98.6% | 94.9% | −3.7 |
| metrics | 83.9% | 83.9% | 0 |
| wallet | 69.7% | **83.7%** | +14.0 (understated) |

The "comprehensive test coverage" claim **holds** (all measured packages are well-covered); only the specific printed percentages are stale.

---

## Conclusion

The documented product surface is **almost entirely delivered**. No documented *capability* is missing or non-functional except the permamail `register`/`update` end-to-end flow, which is explicitly framed as forthcoming. The remaining findings are documentation drift (coverage numbers, LOC estimate, an obsolete `GAPS.md`) and under-documentation of already-implemented RPC methods. Itemized gaps and remediation steps are in `GAPS.md`.
