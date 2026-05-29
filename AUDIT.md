# UNIVERSAL BUG AUDIT (END-TO-END) — 2026-05-29

## Project Profile
- **Purpose:** Library-first Namecoin implementation that can run embedded or as a daemon.
- **Target users:** Go application developers embedding Namecoin capabilities, and operators running `nmcd`/RPC daemon.
- **Deployment model:** In-process embedded mode and standalone daemon mode with P2P sync, bbolt persistence, JSON-RPC.
- **Critical paths:** `chain` block/name validation, `namedb` persistence and reorg restore, `rpc` public API surface, `client` embedded/daemon behavior parity, `network` sync/mempool.

## Audit Scope
- **Packages audited (go list):**
  `github.com/opd-ai/nmcd`, `bridge`, `chain`, `client`, `cmd/nmcd`, `cmd/permamail`, `config`, `examples/bridge_adapter`, `examples/embedded_client`, `examples/list_names`, `examples/mail_router`, `examples/namedb`, `examples/prometheus_exporter`, `examples/register_name`, `examples/shared`, `examples/simple_resolve`, `examples/smtp_relay`, `examples/update_name`, `internal/logging`, `internal/server`, `internal/version`, `loadtest`, `loadtest/cmd`, `mail`, `metrics`, `namedb`, `network`, `rpc`, `wallet`.
- **Baseline executed:**
  - `go-stats-generator analyze . --skip-tests --format json --sections functions,packages,documentation,duplication,patterns,interfaces,structs`
  - `go-stats-generator analyze . --skip-tests`
  - `go test -race ./...`
  - `go vet ./...`
- **go-stats summary:** 70 files, 243 functions, 467 methods, 16 package names, doc coverage 83.5%, duplication ratio 0.53%, 51 functions >50 lines, 4 functions >100 lines.

## Coverage Log
| Package | 3b Logic | 3c Nil | 3d Errors | 3e Resources | 3f Concurrency | 3g Security | 3h Aliasing | 3i Init | 3j API |
|---------|----------|--------|-----------|--------------|----------------|-------------|-------------|---------|--------|
| github.com/opd-ai/nmcd | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/bridge | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/chain | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/client | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/cmd/nmcd | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/cmd/permamail | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/config | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/examples/bridge_adapter | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/examples/embedded_client | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/examples/list_names | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/examples/mail_router | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/examples/namedb | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/examples/prometheus_exporter | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/examples/register_name | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/examples/shared | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/examples/simple_resolve | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/examples/smtp_relay | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/examples/update_name | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/internal/logging | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/internal/server | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/internal/version | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/loadtest | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/loadtest/cmd | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/mail | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/metrics | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/namedb | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/network | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/rpc | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/nmcd/wallet | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

## Goal-Achievement Summary
| Stated Goal | Status | Blocking Findings |
|-------------|--------|-------------------|
| Thread-safe operations for concurrent use | ⚠️ | L-2 |
| Name listing with filters and pagination | ⚠️ | H-1 |
| Consistent RPC behavior for name metadata | ⚠️ | M-1 |
| Embedded and daemon usable as primary library interfaces | ⚠️ | H-1 |

## Findings

### CRITICAL
- [ ] None confirmed.

### HIGH
- [ ] **H-1: Daemon `ListNames` fails to exclude expired names when `IncludeExpired=false`** — `rpc/name_handlers.go:488-492`, `client/daemon.go:617-619` — **API/logic contract bug** — **Code path:** `DaemonClient.ListNames` → RPC `name_list` → server clamps negative `expires_in` to `0` for expired names → client filter excludes only `<0`, so expired names are treated as active. **Concrete consequence:** applications using daemon mode can process expired names as valid in default filtering flow, diverging from embedded mode behavior and README listing expectations. — **Remediation:** In `client/daemon.go` (`ListNames`/`matchesListFilter`), parse and use RPC `expired` flag (or compare against current height directly) instead of `expires_in < 0`; optionally stop clamping `expires_in` in `rpc/name_handlers.go` if compatibility allows. **Validation:** `go test ./client ./rpc -run 'ListNames|nameList|nameScan'` and `go test -race ./...`.

### MEDIUM
- [ ] **M-1: `name_show` expiration semantics are inconsistent with `name_list`/`name_scan`** — `rpc/name_handlers.go:50-53`, `rpc/name_handlers.go:488-492`, `rpc/name_handlers.go:622-626` — **API behavioral inconsistency** — **Code path:** `name_show` returns negative `expires_in` for expired records, while `name_list` and `name_scan` clamp expired values to `0`. **Concrete consequence:** clients that share normalization logic across RPC methods receive conflicting representations for the same state, causing integration bugs and inconsistent UI/business rules. — **Remediation:** Standardize `expires_in` semantics across all name RPCs (prefer one rule and document it); update all method implementations in `rpc/name_handlers.go` and corresponding tests to enforce uniform behavior. **Validation:** `go test ./rpc -run 'NameShow|NameList|NameScan'`.

### LOW
- [ ] **L-1: `name_scan` accepts fractional `count` and silently truncates** — `rpc/name_handlers.go:605-610` — **input validation bug** — **Code path:** JSON number parsed as `float64`, directly cast to `int` without integral check. **Concrete consequence:** request like `{"params":["d/",1.9]}` is accepted as count `1` instead of returning invalid params, masking caller errors and creating surprising API behavior. — **Remediation:** In `parseNameScanParams`, reject non-integer numeric values (e.g., compare `countFloat` vs `math.Trunc(countFloat)`) before conversion. **Validation:** `go test ./rpc -run 'NameScan|parseNameScanParams'`.
- [ ] **L-2: Client list-filter normalization mutates caller-provided filter objects** — `client/daemon.go:606-610`, `client/embedded.go:873-877` — **aliasing/side-effect bug** — **Code path:** both clients set defaults/caps by writing into the incoming `*ListFilter`. **Concrete consequence:** unexpected caller-visible mutation (`Limit` changes), and potential caller-side data races if one filter object is reused across goroutines despite thread-safe API expectations. — **Remediation:** copy filter values into a local struct before normalization; avoid mutating caller-owned memory in `normalizeListFilter` helpers. **Validation:** `go test ./client -run 'ListNames'` and `go test -race ./client`.

## Metrics Snapshot
| Metric | Value |
|--------|-------|
| Total functions | 243 |
| Functions above complexity 15 | 3 |
| Avg cyclomatic complexity | 5.1 |
| Doc coverage | 83.5% |
| Duplication ratio | 0.53% |
| Test pass rate | 20/20 tested packages (plus no-test packages) |
| go vet warnings | 0 |

## False Positives Considered and Rejected
| Candidate | Reason Rejected |
|-----------|----------------|
| Consensus rejection of non-JSON/non-UTF8/non-namespace names in block validation | Rejected: current code uses `validateConsensusNameFormat` on consensus path (`chain/blockchain.go:758-763`), strict namespace/encoding checks remain local-policy-only (`chain/name_script.go:409-450`). |
| Panic recovery in RPC middleware swallowing errors silently | Rejected: panic is logged with stack and request metadata, metric is recorded, and HTTP 500 returned (`rpc/server.go:426-441`). |
| SMTP upstream auth sent over cleartext | Rejected: code explicitly refuses auth when TLS mode is disabled (`mail/smtp.go:565-569`). |

## Remaining Scope (if session ended before completion)
| Package | Status | Notes |
|---------|--------|-------|
| None | Complete | Full package set reviewed for this pass. |
