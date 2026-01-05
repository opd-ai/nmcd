# Production Readiness Plan

**Project:** nmcd - Pure Go Namecoin Library and Daemon  
**Version:** Pre-v1.0  
**Last Updated:** 2026-01-05  
**Target:** Production-ready v1.0 Release

---

## Current State Assessment

### Purpose
nmcd is a **library-first** Namecoin implementation built on btcd libraries, enabling in-process name resolution and registration for Go applications. It can also run as a standalone daemon for traditional RPC access. The project provides a lightweight alternative to Namecoin Core with focus on composition over reimplementation.

### Completed Capabilities

**Core Protocol Implementation (93% Namecoin Compliant):**
- ✅ Name operations: NAME_NEW, NAME_FIRSTUPDATE, NAME_UPDATE with full validation
- ✅ Name database with bbolt storage and expiration tracking (36,000 blocks)
- ✅ Blockchain integration using btcd libraries with name validation hooks
- ✅ Transaction mempool with validation, relay, and expiration (24-hour TTL)
- ✅ Block subsidy calculation verified to match Namecoin Core exactly
- ✅ UTXO tracking and restoration during chain reorganizations
- ✅ AuxPow (merged mining) data structures and validation logic
- ✅ Protocol constants matching Namecoin Core (100% accuracy)

**Network & RPC:**
- ✅ P2P networking using btcd/peer with DNS seed discovery
- ✅ JSON-RPC server with standard methods (getinfo, getblock, getrawtransaction)
- ✅ Name-specific RPC methods (name_show, name_list, name_history, name_new, name_firstupdate, name_update)
- ✅ HTTP Basic Authentication for RPC security
- ✅ Prometheus metrics exporter (32 metrics) with HTTP endpoint

**Library Features:**
- ✅ EmbeddedClient for in-process blockchain and name resolution
- ✅ DaemonClient for connecting to external RPC servers
- ✅ Auto-detection mode (daemon-first with embedded fallback)
- ✅ Context support for cancellation and timeouts
- ✅ Thread-safe operations with proper mutex protection

**Additional Components:**
- ✅ Basic wallet with ECDSA key management (unencrypted JSON storage)
- ✅ Permamail: Email forwarding via Namecoin with SMTP relay
- ✅ Example applications demonstrating library usage patterns
- ✅ Comprehensive test coverage (38 test files, all passing)

**Codebase Quality:**
- ~34,000 lines of Go code across 80 files
- Clean architecture with separation of concerns
- Interface-based network types for testability
- Well-documented with README, API docs, and guides

### Critical Gaps for Production

**Security & Stability:**
1. ❌ **Wallet encryption missing** - Private keys stored unencrypted in wallet.json (file: `wallet/wallet.go`)
2. ❌ **AuxPow mainnet validation untested** - Logic exists but not validated against real blocks past height 19,200 (file: `chain/auxpow.go`)
3. ❌ **No rate limiting on RPC endpoints** - Vulnerable to DoS attacks (file: `rpc/server.go`)
4. ❌ **Insufficient input validation** - Some RPC methods lack comprehensive validation (file: `rpc/server.go`)
5. ❌ **Credentials exposed in process listings** - Command-line flags visible via `ps` (file: `cmd/nmcd/main.go`)

**Observability & Operations:**
1. ❌ **Production logging incomplete** - Uses basic log package, no structured logging or levels (files: `cmd/nmcd/main.go`, `rpc/server.go`, `network/peermgr.go`)
2. ❌ **No health check endpoint** - Cannot verify daemon health programmatically (file: `rpc/server.go`)
3. ❌ **Metrics not comprehensive** - Missing database metrics, cache hit rates, error breakdowns (file: `metrics/prometheus.go`)
4. ❌ **No distributed tracing** - Cannot track requests across components

**API & Compatibility:**
1. ❌ **No semantic versioning** - Library API lacks version guarantees (file: `go.mod`)
2. ❌ **Unstable public API** - No commitment to backward compatibility (files: `client/*.go`)
3. ❌ **Missing deprecation strategy** - No path for evolving APIs safely

**Performance:**
1. ❌ **Incomplete benchmarks** - Client benchmarks exist (see `client/benchmark_test.go`), but core packages (`namedb`, `chain`, `rpc`) lack coverage so overall performance characteristics remain largely unknown
2. ❌ **Database query optimization needed** - No indexes on frequently-accessed fields (file: `namedb/namedb.go`)
3. ❌ **Memory usage not profiled** - Potential leaks or inefficiencies undetected
4. ❌ **No connection pooling** - RPC client creates new connections (file: `client/daemon.go`)

**Distribution & Documentation:**
1. ❌ **No binary releases** - Users must build from source (missing release automation)
2. ❌ **Incomplete installation guide** - Dependencies, system requirements not documented
3. ❌ **No upgrade/migration guide** - Breaking changes not communicated (missing CHANGELOG.md)
4. ❌ **Docker images not published** - Containerized deployment unavailable (missing Dockerfile)
5. ❌ **No performance tuning guide** - Users don't know how to optimize

---

## Phase 1: Critical Security & Stability (Estimated: 5-7 days)

**Goal:** Eliminate security vulnerabilities and production-blocking stability issues to enable safe deployment.

### Deliverables

- [x] **Wallet Encryption Implementation** (2 days) **COMPLETED 2026-01-05**
  - File: `wallet/wallet.go`, `wallet/encryption.go`
  - ✅ Added AES-256-GCM encryption for private keys at rest
  - ✅ Implemented password-based key derivation (scrypt with salt: N=32768, r=8, p=1)
  - ✅ Created migration path from unencrypted to encrypted wallets (version 1 → version 2)
  - ✅ Added `walletpassphrase`, `walletlock`, and `encryptwallet` RPC methods
  - ✅ Test: Encryption/decryption correctness, password strength validation (37 tests passing)
  - ✅ Security features: Memory clearing on lock, auto-lock timer, password hash verification
  - ✅ Documentation: Updated README.md with security warnings and usage examples
  
- [ ] **AuxPow Mainnet Validation Testing** (2 days)
  - Files: `chain/auxpow.go`, `chain/auxpow_test.go`, `testdata/blocks/*.json`
  - Extract real mainnet blocks using provided extraction script
  - Validate blocks 19,200 (activation), 19,201, 50,000, 210,000 (halving)
  - Verify chain ID, merkle branches, parent PoW against Namecoin Core
  - Test: All AuxPow test vectors pass with real block data
  
- [x] **RPC Security Hardening** (1.5 days) **COMPLETED 2026-01-05**
  - File: `rpc/server.go`, `rpc/ratelimit.go`, `rpc/security_test.go`
  - ✅ Implemented per-IP rate limiting (100 req/min default, configurable via Config.RateLimit)
  - ✅ Added request size limits (1MB max body size, configurable via Config.MaxRequestSize)
  - ✅ Validated all input parameters with explicit bounds checking
  - ✅ Added security headers (X-Content-Type-Options: nosniff, X-Frame-Options: DENY, Content-Security-Policy: default-src 'none')
  - ✅ Test: Rate limit enforcement, oversized request rejection (17 new tests, all passing)
  - ✅ Token bucket algorithm with automatic cleanup of stale IP entries
  - ✅ Thread-safe concurrent request handling
  - ✅ MaxBytesReader protection against memory exhaustion attacks
  
- [ ] **Credential Security Improvements** (1 day)
  - Files: `cmd/nmcd/main.go`, `config/config.go`
  - Add config file support for RPC credentials (TOML format)
  - Read credentials from environment variables as fallback
  - Document secure deployment patterns (systemd with EnvironmentFile)
  - Warn on startup if credentials provided via command-line flags
  - Test: Config file parsing, environment variable precedence
  
- [ ] **Critical Bug Fixes** (0.5 days)
  - Files: Various as issues are identified
  - Fix any data corruption risks in namedb transaction handling
  - Address race conditions found via `go test -race`
  - Resolve memory leaks detected in long-running daemon tests
  - Test: Race detector clean, memory profiling stable

### Success Criteria

- ✅ Wallet encryption enabled by default with migration tool
- ⏳ All AuxPow test vectors pass with real mainnet blocks (infrastructure complete, awaiting extraction)
- ✅ RPC rate limiting prevents DoS with < 1% legitimate request impact
- ⏳ No credentials visible in process listings when using config file (pending credential security improvements)
- ⏳ Zero race conditions reported by `go test -race ./...` (pending critical bug fixes)
- ⏳ 24-hour daemon stability test (no crashes, memory leaks < 1MB/hour) (pending critical bug fixes)

### Dependencies

**Blocks:** Phase 2 (requires stable base)

---

## Phase 2: Production Logging & Observability (Estimated: 4-5 days)

**Goal:** Enable production operations with comprehensive logging, metrics, and health checks.

### Deliverables

- [ ] **Structured Logging Implementation** (2 days)
  - Files: `cmd/nmcd/main.go`, `rpc/server.go`, `network/peermgr.go`, `chain/blockchain.go`, `namedb/namedb.go`
  - Replace `log` package with structured logger (e.g., Go standard library `slog`; project uses Go 1.24.11)
  - Add configurable log levels (DEBUG, INFO, WARN, ERROR)
  - Include context fields: component, operation, block_height, peer_id, tx_hash
  - Support JSON output format for log aggregation systems
  - Add log rotation with size and age limits (10 files × 100MB)
  - Test: Log output format validation, level filtering
  
- [ ] **Enhanced Prometheus Metrics** (1 day)
  - File: `metrics/prometheus.go`
  - Add database metrics: `namedb_size_bytes`, `namedb_read_latency_seconds`, `namedb_write_latency_seconds`
  - Add cache metrics: `cache_hit_rate`, `cache_evictions_total`
  - Add error breakdown: `errors_total{type="validation|network|database"}`
  - Add RPC metrics: `rpc_requests_total{method}`, `rpc_duration_seconds{method}`
  - Expose Go runtime metrics: `go_goroutines`, `go_memstats_alloc_bytes`
  - Test: Metric registration, value accuracy
  
- [ ] **Health Check & Readiness Endpoints** (1 day)
  - File: `rpc/server.go`
  - Add `/health` endpoint (HTTP 200 if daemon running, 503 if initializing)
  - Add `/ready` endpoint (HTTP 200 if sync complete, 503 if syncing)
  - Include JSON response: `{"status": "healthy", "block_height": 500000, "peers": 8}`
  - Support Kubernetes liveness and readiness probe patterns
  - Test: Endpoint response codes, JSON structure
  
- [ ] **Error Handling & Recovery** (1 day)
  - Files: `chain/blockchain.go`, `network/peermgr.go`, `rpc/server.go`
  - Add panic recovery middleware for RPC handlers
  - Implement graceful degradation: serve cached data when DB unavailable
  - Add circuit breaker for unreliable peers (3 failures → 5 minute cooldown)
  - Log all errors with full context before returning generic messages to clients
  - Test: Panic recovery, circuit breaker state transitions

### Success Criteria

- ✅ All log messages include structured context fields
- ✅ Log level configurable via flag/config without restart
- ✅ Prometheus metrics cover 95% of operational scenarios
- ✅ Health endpoints return < 10ms (p99 latency)
- ✅ RPC handlers never panic (all panics logged and recovered)
- ✅ Daemon auto-recovers from transient peer disconnections

### Dependencies

**Requires:** Phase 1 (stable foundation)  
**Blocks:** Phase 3 (metrics needed for performance analysis)

---

## Phase 3: Testing & Quality Assurance (Estimated: 6-7 days)

**Goal:** Achieve production-grade test coverage and validate system behavior under realistic load.

### Deliverables

- [ ] **Benchmark Suite** (2 days)
  - Files: `namedb/namedb_bench_test.go`, `chain/blockchain_bench_test.go`, `rpc/rpc_bench_test.go`, `client/client_bench_test.go`
  - Benchmark name resolution: target < 1ms for cached, < 10ms for uncached
  - Benchmark name registration: target < 50ms for validation
  - Benchmark RPC methods: target < 100ms p99 latency for all methods
  - Benchmark concurrent client operations: 1000 req/s throughput
  - Document baseline performance in `docs/PERFORMANCE.md`
  - Test: Benchmarks run via `go test -bench=. -benchmem`
  
- [ ] **Integration Test Suite** (2 days)
  - Files: `integration_test.go` (new), `testdata/scenarios/` (new)
  - End-to-end regtest scenario: genesis → name registration → update → expiration
  - Multi-node synchronization test: 3 nodes sync and agree on chain state
  - Network partition recovery test: nodes reconnect and resolve fork
  - RPC client compatibility test: daemon + embedded clients produce same results
  - Transaction relay test: broadcast → mempool → block inclusion → confirmation
  - Test: All scenarios pass with < 1% flakiness
  
- [ ] **Load & Stress Testing** (1.5 days)
  - Files: `loadtest/` (new directory)
  - Continuous operation test: 72 hours runtime, process 100,000 names
  - RPC load test: 500 concurrent clients, 1000 req/s sustained
  - Memory leak detection: RSS growth < 10MB over 24 hours
  - Connection exhaustion test: handle 1000 peer connections gracefully
  - Database corruption test: kill -9 during writes, verify DB integrity on restart
  - Test: All stress tests pass without daemon failure
  
- [ ] **Fuzzing for Security** (1 day)
  - Files: `chain/fuzz_test.go`, `namedb/fuzz_test.go`, `rpc/fuzz_test.go`
  - Fuzz RPC inputs: malformed JSON, extreme values, injection attempts
  - Fuzz name operation scripts: invalid opcodes, oversized data, malformed UTF-8
  - Fuzz wire protocol: truncated messages, invalid lengths, wrong magic bytes
  - Run fuzzing for 1 million iterations per target
  - Test: No crashes, no data corruption, all inputs handled gracefully
  
- [ ] **Test Coverage Analysis** (0.5 days)
  - All packages
  - Generate coverage report: `go test -coverprofile=coverage.out ./...`
  - Identify untested critical paths (goal: 80% coverage for core packages)
  - Add tests for uncovered branches in validation logic
  - Document coverage metrics in README
  - Test: Coverage >= 80% for `chain/`, `namedb/`, `rpc/`

### Success Criteria

- ✅ Benchmark suite documents baseline performance (added to docs/PERFORMANCE.md)
- ✅ Integration tests cover 10+ real-world scenarios
- ✅ 72-hour stability test completes with zero crashes
- ✅ Fuzzing finds and fixes 0 critical issues (or all found issues resolved)
- ✅ Test coverage >= 80% for critical packages
- ✅ All tests pass with `go test -race -count=10 ./...` (flakiness check)

### Dependencies

**Requires:** Phase 2 (logging needed to diagnose test failures)  
**Blocks:** Phase 4 (performance validation needed before optimization)

---

## Phase 4: Performance Optimization & Scalability (Estimated: 5-6 days)

**Goal:** Optimize hot paths and validate performance under expected production load.

### Deliverables

- [ ] **Database Performance Tuning** (2 days)
  - File: `namedb/namedb.go`
  - Add secondary indexes: namespace prefix, expiration time, owner address
  - Implement read-through cache for name lookups (LRU, 10,000 entries)
  - Batch database writes during block processing (commit every 100 operations)
  - Optimize GetExpiredNames() query with index scan
  - Profile database with `pprof` and eliminate N+1 queries
  - Test: Name resolution < 1ms (p95), block processing throughput +50%
  
- [ ] **Network Optimization** (1.5 days)
  - Files: `network/peermgr.go`, `client/daemon.go`
  - Implement connection pooling for RPC client (reuse HTTP connections)
  - Add peer scoring: prioritize reliable peers, deprioritize slow/unreliable
  - Optimize message serialization: use buffer pools to reduce allocations
  - Implement compact block relay (BIP152) for faster block propagation
  - Test: Block relay latency -30%, RPC client allocations -50%
  
- [ ] **Memory Optimization** (1.5 days)
  - Files: `chain/blockchain.go`, `network/mempool.go`, `namedb/namedb.go`
  - Profile memory usage with `go test -memprofile` and identify hot allocations
  - Reduce UTXO cache size with eviction policy (keep hot UTXOs, evict cold)
  - Optimize name record serialization (avoid intermediate string copies)
  - Implement sync.Pool for frequently allocated objects (buffers, slices)
  - Test: Memory usage -20% during sync, allocation rate -30%
  
- [ ] **Concurrency Improvements** (1 day)
  - Files: `rpc/server.go`, `chain/blockchain.go`
  - Parallelize RPC request handling (currently sequential per connection)
  - Add worker pool for block validation (validate signatures in parallel)
  - Optimize lock granularity in namedb (per-bucket locks instead of global)
  - Use read-write locks appropriately (favor RLock for read-heavy operations)
  - Test: RPC throughput +100%, CPU utilization improved

### Success Criteria

- ✅ Name resolution latency: p95 < 1ms, p99 < 5ms
- ✅ Block processing throughput: > 100 blocks/second on modern hardware
- ✅ RPC throughput: > 1000 requests/second (concurrent clients)
- ✅ Memory usage: < 500MB during normal operation, < 1GB during sync
- ✅ CPU efficiency: > 90% time in useful work (< 10% lock contention)
- ✅ Benchmark suite shows 50%+ improvement over Phase 3 baseline

### Dependencies

**Requires:** Phase 3 (benchmarks needed to measure improvements)  
**Blocks:** Phase 5 (optimization complete before release)

---

## Phase 5: Distribution & Documentation (Estimated: 5-6 days)

**Goal:** Package nmcd for distribution and provide comprehensive documentation for users and contributors.

### Deliverables

- [ ] **Semantic Versioning & API Stability** (1 day)
  - Files: `go.mod`, `client/types.go`, `CHANGELOG.md` (new)
  - Establish v1.0.0 API contract for client package
  - Document breaking vs non-breaking change policy
  - Create CHANGELOG.md with Keep a Changelog format
  - Add deprecation warnings for methods to be removed in v2
  - Tag release: `git tag v1.0.0`
  - Test: `go list -m github.com/opd-ai/nmcd@v1.0.0` resolves
  
- [ ] **Binary Releases & Packaging** (2 days)
  - Files: `.github/workflows/release.yml` (new), `Dockerfile` (new), `scripts/build-release.sh` (new)
  - Set up GitHub Actions for automated releases (Linux, macOS, Windows binaries)
  - Create multi-arch Docker images (amd64, arm64)
  - Publish Docker images to GitHub Container Registry
  - Generate checksums (SHA256) and GPG signatures for binaries
  - Create DEB/RPM packages for Linux distributions
  - Test: Install from package, run daemon, verify functionality
  
- [ ] **Installation & Operations Guide** (1.5 days)
  - Files: `docs/INSTALLATION.md` (new), `docs/OPERATIONS.md` (new), `examples/systemd/nmcd.service` (new)
  - Document system requirements: Go version, disk space, memory
  - Create step-by-step installation guide for each platform
  - Add systemd service file with best practices (restart policy, resource limits)
  - Document backup and restore procedures for wallet and database
  - Add monitoring guide: Prometheus scraping, Grafana dashboards
  - Create troubleshooting guide for common issues
  - Test: New user can install and run daemon following docs
  
- [ ] **API Documentation & Examples** (1 day)
  - Files: `docs/API.md`, `examples/` (update), `README.md` (update)
  - Complete API documentation with all methods and parameters
  - Add code examples for common use cases (15+ examples)
  - Document error codes and handling strategies
  - Create quickstart guide: "Hello Namecoin" in 5 minutes
  - Generate godoc documentation and publish to pkg.go.dev
  - Test: All code examples compile and run successfully
  
- [ ] **Contributor Guide & Governance** (0.5 days)
  - Files: `CONTRIBUTING.md` (new), `CODE_OF_CONDUCT.md` (new), `.github/PULL_REQUEST_TEMPLATE.md` (new)
  - Document contribution process: fork, branch, PR, review
  - Establish code review guidelines and quality standards
  - Define release process and cadence (quarterly minor releases)
  - Add issue templates for bugs and feature requests
  - Document testing requirements for PRs (coverage, race detector)
  - Test: External contributor can submit PR following guide

### Success Criteria

- ✅ v1.0.0 tagged and API stability committed in CHANGELOG.md
- ✅ Binary releases available for Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64)
- ✅ Docker images published to ghcr.io with < 100MB compressed size
- ✅ Installation guide tested by 3 external users (success without support)
- ✅ API documentation complete with 90%+ method coverage
- ✅ All examples in docs/ run successfully on fresh install
- ✅ pkg.go.dev shows complete documentation with examples

### Dependencies

**Requires:** Phase 4 (performance optimized before release)  
**Blocks:** None (final phase)

---

## Release Checklist

**Pre-Release Validation (v1.0.0-rc1):**
- [ ] All Phase 1-5 deliverables complete
- [ ] Test suite passes: `go test -race -count=3 ./...`
- [ ] Benchmark suite shows acceptable performance (meets Phase 4 criteria)
- [ ] 72-hour stability test completes successfully
- [ ] Security audit complete (no critical vulnerabilities)
- [ ] Documentation reviewed by 2 maintainers
- [ ] AuxPow validation tested against mainnet blocks 19,200-250,000
- [ ] Zero known data corruption bugs
- [ ] Memory leak test passes (< 10MB growth over 24 hours)
- [ ] All critical TODOs resolved in codebase

**Release Candidate Testing (2 weeks):**
- [ ] Deploy RC1 to testnet, observe for 1 week
- [ ] Community testing: 10+ users test on their systems
- [ ] Mainnet dry-run: sync past block 19,200 (AuxPow activation)
- [ ] Performance testing on production-like hardware
- [ ] Security review by external auditor (optional but recommended)
- [ ] Fix all critical/high issues found during RC testing
- [ ] Create RC2 if major issues found, repeat testing

**v1.0.0 Release:**
- [ ] Final regression testing: all tests pass
- [ ] Update version in go.mod, cmd/nmcd/main.go
- [ ] Update CHANGELOG.md with release notes
- [ ] Create annotated git tag: `git tag -a v1.0.0 -m "Release v1.0.0"`
- [ ] Push tag: `git push origin v1.0.0`
- [ ] GitHub Actions builds and publishes binaries
- [ ] Publish Docker images to ghcr.io
- [ ] Publish release notes on GitHub
- [ ] Update documentation on website/wiki
- [ ] Announce on Namecoin forums, mailing lists, social media

**Post-Release:**
- [ ] Monitor GitHub issues for bug reports (24/7 for first week)
- [ ] Deploy to production infrastructure if applicable
- [ ] Set up monitoring dashboards and alerts
- [ ] Document any issues and create v1.0.1 plan if needed
- [ ] Begin planning v1.1.0 features (non-breaking enhancements)

---

## Effort Summary

| Phase | Estimated Effort | Team Size | Calendar Time |
|-------|------------------|-----------|---------------|
| Phase 1: Security & Stability | 5-7 days | 1-2 engineers | 1 week |
| Phase 2: Logging & Observability | 4-5 days | 1 engineer | 1 week |
| Phase 3: Testing & QA | 6-7 days | 1-2 engineers | 1-2 weeks |
| Phase 4: Performance | 5-6 days | 1 engineer | 1 week |
| Phase 5: Distribution | 5-6 days | 1-2 engineers | 1 week |
| **Total** | **25-31 days** | **1-2 engineers** | **5-7 weeks** |

**Notes:**
- Estimates assume experienced Go developers familiar with blockchain systems
- Calendar time includes testing, code review, and iteration
- Parallel work possible: Phase 2 can start when Phase 1 is 80% complete
- Add 20-30% buffer for unforeseen issues and scope adjustments

---

## Risk Mitigation

**Critical Risks:**

1. **AuxPow Validation Failures on Mainnet**
   - **Impact:** Cannot sync past block 19,200, breaking production use
   - **Mitigation:** Extensive testing with real blocks in Phase 1, testnet deployment before mainnet
   - **Contingency:** Checkpoint approach as fallback (trust recent blocks, validate going forward)

2. **Performance Below Acceptable Thresholds**
   - **Impact:** Daemon unusable for production workloads
   - **Mitigation:** Early benchmarking in Phase 3, optimization in Phase 4
   - **Contingency:** Reduce scope for v1.0 (regtest/testnet only), defer mainnet to v1.1

3. **Security Vulnerabilities Discovered Late**
   - **Impact:** Delay release, emergency patches needed
   - **Mitigation:** Security-first in Phase 1, fuzzing in Phase 3, optional external audit
   - **Contingency:** Responsible disclosure process, rapid patch release workflow

4. **Breaking API Changes Required**
   - **Impact:** User disruption, ecosystem fragmentation
   - **Mitigation:** API review in Phase 5, deprecation warnings for v2 changes
   - **Contingency:** Dual API support (v1 + v2) for transition period

---

## Success Metrics (v1.0 Goals)

**Functional:**
- ✅ 95%+ Namecoin protocol compliance
- ✅ Mainnet sync to current height without errors
- ✅ All name operations (NEW, FIRSTUPDATE, UPDATE) functional
- ✅ Embedded and daemon modes both production-ready

**Quality:**
- ✅ Test coverage >= 80% for critical packages
- ✅ Zero known critical or high severity bugs
- ✅ 72-hour stability test passes (no crashes, memory leaks)
- ✅ Fuzzing finds no new crashes after 1M iterations

**Performance:**
- ✅ Name resolution: < 1ms (p95), < 5ms (p99)
- ✅ RPC throughput: > 1000 req/s
- ✅ Memory usage: < 500MB normal, < 1GB sync
- ✅ Block processing: > 100 blocks/s

**Usability:**
- ✅ Binary releases for 3+ platforms (Linux, macOS, Windows)
- ✅ Docker images with multi-arch support
- ✅ Documentation complete (API, installation, operations)
- ✅ New user can install and run in < 15 minutes

**Community:**
- ✅ 10+ successful external user installations
- ✅ 5+ GitHub stars and watchers
- ✅ Active issue tracking and response (< 48 hour initial response)

---

## Beyond v1.0 (Future Roadmap)

**v1.1 (Q2 2026) - Performance & Scaling:**
- Compact block relay (BIP152) implementation
- Improved UTXO cache eviction algorithms
- Parallel signature validation
- Database sharding for high-throughput deployments

**v1.2 (Q3 2026) - Advanced Features:**
- SegWit support (if Namecoin adopts)
- BIP9 soft fork signaling (if needed)
- Wallet HD key derivation (BIP32/44)
- Hardware wallet integration

**v2.0 (Q4 2026) - Breaking Changes:**
- Refactored API based on v1 feedback
- Plugin architecture for custom name validators
- gRPC API alongside JSON-RPC
- Multi-sig and time-locked name operations

---

**Roadmap Maintained By:** nmcd Core Team  
**Feedback:** GitHub Issues or community@nmcd.dev  
**Last Review:** 2026-01-05
