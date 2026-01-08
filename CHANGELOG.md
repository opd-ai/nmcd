# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial v1.0.0 API contract for client package
- CHANGELOG.md for tracking version history
- API stability guarantees documentation

## [0.1.0] - 2026-01-08

### Added

#### Core Protocol Implementation
- Name operations: NAME_NEW, NAME_FIRSTUPDATE, NAME_UPDATE with full validation
- Name database with bbolt storage and expiration tracking (36,000 blocks)
- Blockchain integration using btcd libraries with name validation hooks
- Transaction mempool with validation, relay, and expiration (24-hour TTL)
- Block subsidy calculation verified to match Namecoin Core exactly
- UTXO tracking and restoration during chain reorganizations
- AuxPow (merged mining) data structures and validation logic
- Protocol constants matching Namecoin Core (100% accuracy)

#### Network & RPC
- P2P networking using btcd/peer with DNS seed discovery
- JSON-RPC server with standard methods (getinfo, getblock, getrawtransaction)
- Name-specific RPC methods (name_show, name_list, name_history, name_new, name_firstupdate, name_update)
- HTTP Basic Authentication for RPC security
- RPC rate limiting (100 req/min default, configurable)
- Request size limits (1MB max body size, configurable)
- Security headers (X-Content-Type-Options, X-Frame-Options, CSP)
- Prometheus metrics exporter (43 metrics) with HTTP endpoint
- Health check endpoints (/health, /ready) for Kubernetes/container orchestration

#### Client Library
- NameClient interface for interacting with Namecoin network
- EmbeddedClient for in-process blockchain and name resolution
- DaemonClient for connecting to external RPC servers
- Auto-detection mode (daemon-first with embedded fallback)
- Context support for cancellation and timeouts
- Thread-safe operations with proper mutex protection

#### Security & Operations
- Wallet encryption (AES-256-GCM) with password-based key derivation (scrypt)
- Migration path from unencrypted to encrypted wallets
- RPC methods: walletpassphrase, walletlock, encryptwallet
- Memory clearing on lock, auto-lock timer, password hash verification
- TOML config file support for RPC credentials
- Environment variable support for configuration
- systemd service file with security best practices
- Structured logging using Go standard library slog
- Configurable log levels (DEBUG, INFO, WARN, ERROR)
- JSON and text output formats for logs
- Panic recovery middleware for all RPC handlers
- Graceful degradation for missing components

#### Performance Optimizations
- LRU cache for name lookups (10,000 entries, 6.4x faster)
- Batch database writes (BatchWriter API, ~100x faster for bulk operations)
- Optimized GetExpiredNames() with expiration index (3.9x faster)
- Connection pooling for RPC client
- Peer scoring system for reliability tracking
- Buffer pooling for message serialization (3.6x faster, zero allocations on reuse)
- Parallelized RPC request handling (17,697 req/s concurrent throughput)

#### Testing & Quality
- Comprehensive test coverage (38 test files, 25 packages)
- Benchmark suite for namedb, chain, rpc packages
- Integration test suite (regtest workflow, multi-node sync, network partition recovery)
- Load testing infrastructure with configurable duration and concurrency
- Fuzzing for security (13 fuzz tests across 3 critical packages)
- Race detector clean (zero race conditions detected)

#### Additional Components
- Basic wallet with ECDSA key management
- Permamail: Email forwarding via Namecoin with SMTP relay
- Example applications demonstrating library usage patterns
- Documentation: README, API docs, performance guides, operations guides

### Known Limitations
- AuxPow validation logic present but not fully tested against mainnet blocks past height 19,200
- No mempool transaction prioritization by fee
- Wallet stores keys in JSON format (encrypted with user-provided password)
- Test coverage for chain (68.1%), rpc (45.8%), network (43.5%) packages below 80% target

### Breaking Changes
- Initial release, no breaking changes from previous versions

### Deprecated
- None

### Security
- Wallet encryption protects private keys at rest with AES-256-GCM
- RPC rate limiting prevents DoS attacks
- HTTP Basic Auth transmits base64-encoded credentials (use over localhost or with proper network security)
- Security headers prevent common web vulnerabilities
- All user inputs validated with explicit bounds checking

## Version History

- **0.1.0** (2026-01-08): Initial development release with core functionality
- **Unreleased**: Working towards v1.0.0 production release

## Semantic Versioning Policy

### Version Format: MAJOR.MINOR.PATCH

- **MAJOR**: Breaking changes to public API
- **MINOR**: New features, backward-compatible
- **PATCH**: Bug fixes, backward-compatible

### API Stability Guarantees (v1.0.0+)

#### Stable APIs (Guaranteed Compatibility)
The following packages and interfaces are considered stable and will maintain backward compatibility:

- **client package**: All exported types, interfaces, functions
  - `NameClient` interface and all its methods
  - `Config`, `NameRecord`, `RegisterOpts`, `UpdateOpts`, `TxResult`, `ListFilter`, `NodeInfo` types
  - `NewClient`, `NewEmbeddedClient`, `NewDaemonClient` functions
  - All exported error variables (e.g., `ErrNameNotFound`, `ErrNameExpired`)

#### What Constitutes a Breaking Change

Breaking changes that trigger a MAJOR version bump:

1. **Removing or renaming** exported types, functions, methods, or fields
2. **Changing function signatures** (parameters, return types)
3. **Changing interface definitions** (adding/removing methods)
4. **Changing behavior** in ways that break existing usage patterns
5. **Removing support** for previously supported network types or data formats
6. **Changing error types** that clients might depend on

#### What Does NOT Constitute a Breaking Change

Non-breaking changes that can be included in MINOR or PATCH releases:

1. **Adding new standalone functions or types** (without changing existing interfaces)
2. **Adding new optional fields** to struct types
3. **Adding new error types** (without removing existing ones)
4. **Performance improvements** that don't change behavior
5. **Bug fixes** that restore documented behavior
6. **Internal implementation changes** that don't affect public API

Note: Adding methods to existing interfaces is a breaking change in Go and requires a MAJOR version bump.

### Deprecation Policy

1. **Deprecation Notice**: Features to be removed will be marked as deprecated at least **one MINOR version** before removal
2. **Documentation**: Deprecated features will include:
   - Clear deprecation notice in GoDoc comments
   - Suggested alternative/replacement
   - Earliest version in which feature will be removed
3. **Deprecation Format**:
   ```go
   // Deprecated: Use NewFeature instead. Will be removed in v2.0.0.
   func OldFeature() {}
   ```
4. **Removal**: Deprecated features are removed only in MAJOR version releases

### Pre-1.0 Stability

Before v1.0.0:

- API may change without prior notice
- No backward compatibility guarantees
- MINOR version changes may include breaking changes
- Stability is being established but not guaranteed

Starting with v1.0.0:

- Full semantic versioning guarantees apply
- Breaking changes only in MAJOR releases
- Backward compatibility within MAJOR version

### Release Cadence

- **PATCH releases**: As needed for critical bug fixes (within 1-2 weeks)
- **MINOR releases**: Quarterly (every 3 months) for new features
- **MAJOR releases**: Annually or when significant breaking changes accumulate

### Long-Term Support (LTS)

- Latest MAJOR version receives active development
- Previous MAJOR version receives security fixes for 6 months after new MAJOR release
- Older MAJOR versions are unsupported (community patches accepted)

### Examples

#### PATCH Release (1.0.0 → 1.0.1)
```
✅ Fixed memory leak in name cache
✅ Corrected expiration calculation for edge case
✅ Updated dependencies for security patches
❌ Cannot add new methods to NameClient interface
❌ Cannot change Config field types
```

#### MINOR Release (1.0.0 → 1.1.0)
```
✅ Added ListNamesWithPagination helper function
✅ Added optional MaxRetries field to Config
✅ Added new convenience methods (non-interface)
✅ Performance improvements to database queries
❌ Cannot remove existing Config fields
❌ Cannot change NameClient interface methods
```

#### MAJOR Release (1.5.0 → 2.0.0)
```
✅ Refactored Config structure for clarity
✅ Removed deprecated methods marked in v1.x
✅ Changed NameClient interface for better error handling
✅ Updated minimum Go version requirement
✅ Any other breaking changes
```

## Migration Guides

### Upgrading to v1.0.0 from v0.1.0

No migration required for client package users. The v0.1.0 API is forward-compatible with v1.0.0.

**Action Items:**
1. Review your code for any use of undocumented internal packages
2. Update import paths if you were using internal packages (not recommended)
3. Test your application with v1.0.0 before deploying to production

**Notable Improvements:**
- Enhanced performance (6.4x faster name lookups)
- Better error handling and reporting
- Comprehensive logging and metrics
- Production-ready security features

## Contributing

Future CONTRIBUTING.md will document:
- How to report bugs
- How to propose features
- Pull request process
- Testing requirements
- Code review standards

## Support

- **Documentation**: https://pkg.go.dev/github.com/opd-ai/nmcd
- **Issues**: https://github.com/opd-ai/nmcd/issues
- **Discussions**: https://github.com/opd-ai/nmcd/discussions

[Unreleased]: https://github.com/opd-ai/nmcd/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/opd-ai/nmcd/releases/tag/v0.1.0
