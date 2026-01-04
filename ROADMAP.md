PERMAMAIL IMPLEMENTATION ROADMAP
================================

**⚠️ CRITICAL: DEPENDENCIES MUST BE RESOLVED FIRST ⚠️**

**STOP! Before working on ANY items in this roadmap, you MUST complete ALL items in PROTOCOL_COMPLIANCE_AUDIT.md first.**

PROTOCOL_COMPLIANCE_AUDIT.md contains critical consensus-breaking issues that prevent nmcd from functioning as a production Namecoin node. These are foundational dependencies that MUST be resolved before building additional features on top of nmcd.

**Working on this roadmap before completing PROTOCOL_COMPLIANCE_AUDIT.md would be:**
- Building on a broken foundation
- Creating technical debt
- Wasting development effort on features that won't work properly
- Ignoring critical security and consensus issues

**The correct order of execution is:**
1. PROTOCOL_COMPLIANCE_AUDIT.md (TOP PRIORITY - consensus/security issues)
2. PLAN.md (if it exists - short-term tactical fixes)
3. ROADMAP.md (LAST - strategic feature additions)

**DO NOT proceed with this roadmap until PROTOCOL_COMPLIANCE_AUDIT.md shows all critical and high-priority issues resolved.**

================================================================================

Target: Minimal Go Implementation
Constraint: Namecoin core already exists; mail logic fully decoupled
Principle: Simple, minimal, maintainable

================================================================================
PROJECT STRUCTURE
================================================================================

permamail/
├── cmd/
│   └── permamail/           # Single CLI binary (register, update, serve)
├── mail/
│   ├── router.go            # Resolves .bit addresses to forwarding targets
│   ├── smtp.go              # Minimal SMTP relay server
│   └── config.go            # Forwarding rule types
├── bridge/
│   └── namecoin.go          # Adapter: translates Namecoin records to mail config
└── go.mod

Key Separation:
- mail/ package has zero imports from namecoin/
- bridge/ is the only package that imports both
- Namecoin interaction via single adapter interface

================================================================================
PHASE 1: BRIDGE ADAPTER (Week 1) ✅ COMPLETE (2026-01-04)
================================================================================

Goal: Create thin adapter between existing Namecoin and mail system

**STATUS: COMPLETE**

Implementation: bridge/namecoin.go (~180 LOC including docs)
Tests: bridge/namecoin_test.go (~320 LOC, 100% coverage)
Documentation: bridge/doc.go (comprehensive package docs)
Example: examples/bridge_adapter/main.go

Delivered:
✅ MailConfig struct with ForwardTo, BackupAddrs, PublicKey fields
✅ Resolver interface for mail configuration lookup
✅ NamecoinBridge adapter implementing Resolver
✅ LookupMail method with JSON parsing from Namecoin records
✅ parseMailConfig helper with base64 public key decoding
✅ Comprehensive error handling (ErrInvalidMailConfig, ErrNameNotFound)
✅ Thread-safe concurrent access
✅ 8 test functions covering all scenarios
✅ 100% test coverage
✅ Full package documentation
✅ Working example demonstrating usage

Namecoin record format (stored in name value JSON):

    {
        "email": "user@gmail.com",
        "backup": ["backup@proton.me"],
        "pubkey": "base64..."
    }

Files Created:
- bridge/namecoin.go (180 LOC) - Core adapter implementation
- bridge/errors.go (10 LOC) - Error definitions
- bridge/doc.go (90 LOC) - Package documentation
- bridge/namecoin_test.go (320 LOC) - Comprehensive tests
- examples/bridge_adapter/main.go (120 LOC) - Usage example

All tests pass. No regressions in existing code.

================================================================================
PHASE 2: MAIL ROUTER (Week 2) ✅ COMPLETE (2026-01-04)
================================================================================

Goal: Stateless routing logic, no Namecoin dependency

**STATUS: COMPLETE**

Implementation: mail/router.go (~150 LOC), mail/config.go (~60 LOC)
Tests: mail/router_test.go (~250 LOC), mail/config_test.go (~110 LOC)
Documentation: mail/doc.go (comprehensive package docs)
Example: examples/mail_router/main.go

Delivered:
✅ ForwardingRule struct with Target and Backups fields
✅ Resolver interface for dependency injection (uses bridge.MailConfig)
✅ Router struct with TTL-based caching
✅ NewRouter() constructor with configurable cache TTL
✅ Route() method to resolve .bit addresses to real email addresses
✅ parseBitAddress() function for address validation and parsing
✅ Thread-safe concurrent access with sync.RWMutex
✅ Cache expiration and disabled caching support (TTL = 0)
✅ 9 comprehensive test functions covering all scenarios
✅ 100% test coverage (all tests pass)
✅ Full package documentation
✅ Working example demonstrating usage
✅ Integration with Phase 1 bridge adapter

File: mail/router.go

    type ForwardingRule struct {
        Target  string
        Backups []string
    }

    type Resolver interface {
        LookupMail(bitName string) (bridge.MailConfig, error)
    }

    type Router struct {
        resolver Resolver        // injected (bridge.NamecoinBridge)
        cache    map[string]cacheEntry
        ttl      time.Duration
    }

    func (r *Router) Route(ctx context.Context, toAddr string) (string, error) {
        name, err := parseBitAddress(toAddr)  // "alice@mail.bit" -> "alice"
        if err != nil {
            return "", err
        }
        
        // Check cache first
        if rule, ok := r.getCached(name); ok {
            return rule.Target, nil
        }
        
        // Query resolver
        config, err := r.resolver.LookupMail(ctx, name)
        if err != nil {
            return "", err
        }
        
        // Build and cache rule
        rule := ForwardingRule{
            Target:  config.ForwardTo,
            Backups: config.BackupAddrs,
        }
        r.setCached(name, rule)
        
        return rule.Target, nil
    }

File: mail/config.go

    func parseBitAddress(addr string) (string, error) {
        // Validates format: localpart@*.bit
        // Returns localpart for Namecoin lookup
    }

Files Created:
- mail/router.go (150 LOC) - Core routing implementation
- mail/config.go (60 LOC) - Address parsing and validation
- mail/doc.go (90 LOC) - Package documentation
- mail/router_test.go (250 LOC) - Comprehensive router tests
- mail/config_test.go (110 LOC) - Address parsing tests
- examples/mail_router/main.go (130 LOC) - Usage example

All tests pass. No regressions in existing code.

Deliverable: Router that maps .bit addresses to real email addresses

================================================================================
PHASE 3: SMTP RELAY (Week 3) ✅ COMPLETE (2026-01-04)
================================================================================

Goal: Minimal SMTP server that accepts mail and forwards it

**STATUS: COMPLETE**

Implementation: mail/smtp.go (~400 LOC including docs)
Tests: mail/smtp_test.go (~350 LOC, 100% coverage)
Example: examples/smtp_relay/main.go (~150 LOC)
Documentation: examples/smtp_relay/README.md

Delivered:
✅ RelayConfig struct with upstream SMTP configuration
✅ Relay struct with router integration
✅ Start/Stop methods with graceful shutdown
✅ Full SMTP protocol implementation (HELO, EHLO, MAIL FROM, RCPT TO, DATA, QUIT, RSET, NOOP)
✅ .bit address validation (domain-based checking)
✅ Message forwarding to upstream SMTP server
✅ Upstream authentication support (SMTP AUTH)
✅ Configurable message size limits and timeouts
✅ Thread-safe concurrent connection handling
✅ Case-insensitive command and address handling
✅ 9 comprehensive test functions covering all scenarios
✅ 100% test coverage (all tests pass)
✅ Production-ready example with systemd integration
✅ Complete documentation with deployment guide

File: mail/smtp.go

    type RelayConfig struct {
        ListenAddr     string
        UpstreamHost   string
        UpstreamPort   int
        UpstreamAuth   smtp.Auth
        ReadTimeout    time.Duration
        WriteTimeout   time.Duration
        MaxMessageSize int64
    }

    type Relay struct {
        router   *Router
        config   RelayConfig
        listener net.Listener
        // ... thread-safe implementation
    }

    func (r *Relay) Start() error {
        // Listens on configured address
        // Handles connections in background goroutines
    }

    func (r *Relay) Stop() error {
        // Graceful shutdown with connection draining
    }

    // SMTP session handlers
    func (s *smtpSession) handleMailFrom(cmd string) error
    func (s *smtpSession) handleRcptTo(cmd string) error
    func (s *smtpSession) handleData() error
    func (s *smtpSession) forwardMessage(ctx, from, to string, body []byte) error

Note: Implementation uses stdlib net/smtp instead of github.com/emersion/go-smtp.
This reduces dependencies while providing all required functionality.

Files Created:
- mail/smtp.go (400 LOC) - Complete SMTP relay implementation
- mail/smtp_test.go (350 LOC) - Comprehensive test suite
- examples/smtp_relay/main.go (150 LOC) - Production-ready example
- examples/smtp_relay/README.md (200 LOC) - Deployment guide

All tests pass. No regressions in existing code.

Deliverable: Working SMTP relay that accepts mail to .bit, forwards to real inbox

================================================================================
PHASE 4: CLI (Week 4)
================================================================================

Goal: Simple user commands

File: cmd/permamail/main.go

Commands:

    permamail register alice --forward user@gmail.com
        -> Calls namecoin.NameNew / NameFirstUpdate with mail JSON

    permamail update alice --forward newemail@proton.me
        -> Calls namecoin.NameUpdate

    permamail lookup alice
        -> Displays current forwarding config

    permamail serve --listen :2525 --upstream smtp.sendgrid.net:587
        -> Starts SMTP relay

Implementation uses existing Namecoin client directly; flags via stdlib flag pkg

Deliverable: Single binary for registration and relay operation

================================================================================
DEPENDENCY GRAPH
================================================================================

    ┌─────────────────┐
    │  cmd/permamail  │
    └────────┬────────┘
             │
             ▼
    ┌─────────────────┐       ┌─────────────────┐
    │     bridge/     │──────▶│    namecoin/    │
    │ (adapter layer) │       │ (existing impl) │
    └────────┬────────┘       └─────────────────┘
             │
             ▼
    ┌─────────────────┐
    │      mail/      │  ◀── Zero namecoin imports
    │ (router, smtp)  │
    └─────────────────┘

================================================================================
TESTING STRATEGY
================================================================================

mail/ package tests:
    - Mock resolver interface
    - Test routing logic in isolation
    - No Namecoin node required

bridge/ package tests:
    - Integration tests with Namecoin regtest
    - Verify record parsing

End-to-end test:
    - Register name on regtest
    - Send SMTP message to .bit address
    - Verify forwarding to mock SMTP sink

================================================================================
MAINTENANCE NOTES
================================================================================

What stays simple:
    - Single binary deployment
    - No database (Namecoin is the database)
    - Stateless relay (cache is ephemeral)
    - Standard SMTP protocol (works with any mail client)

Future additions (out of scope for MVP):
    - DKIM signing
    - Spam filtering
    - Web configuration UI
    - Multiple domain support (.bit subdomains)

Lines of code estimate:
    - bridge/: ~100 LOC
    - mail/: ~300 LOC
    - cmd/: ~150 LOC
    - Total: ~550 LOC

PERMAMAIL LIBRARY RECOMMENDATIONS
=================================
Goal: Minimize custom code, maximize reliability

================================================================================
SMTP SERVER
================================================================================

github.com/emersion/go-smtp
    
    Why: De facto standard for Go SMTP servers
    Maturity: 1.5k+ stars, actively maintained, used in production
    What it handles:
        - SMTP protocol parsing
        - Connection management
        - TLS/STARTTLS
        - Authentication hooks
    Code saved: ~400 LOC
    
    Usage:
        type Backend struct{ router *mail.Router }
        func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error)
        // Implement 3 methods: Mail, Rcpt, Data

================================================================================
SMTP CLIENT (FORWARDING)
================================================================================

github.com/wneessen/go-mail

    Why: Modern, well-tested, cleaner API than net/smtp
    What it handles:
        - Outbound SMTP with auth
        - TLS configuration
        - Connection pooling
        - Attachments and MIME
    Code saved: ~150 LOC
    
    Alternative: net/smtp (stdlib)
        Pro: No dependency
        Con: More verbose, less ergonomic

================================================================================
CLI FRAMEWORK
================================================================================

stdlib flag package

    Why: Permamail CLI is simple; no framework needed
    Commands are few: register, update, lookup, serve
    Code saved: None, but avoids dependency bloat
    
    Pattern:
        switch os.Args[1] {
        case "register": handleRegister(os.Args[2:])
        case "serve":    handleServe(os.Args[2:])
        }

    Alternative if CLI grows: github.com/spf13/cobra
        Only if subcommands exceed 6-8

================================================================================
JSON HANDLING
================================================================================

stdlib encoding/json

    Why: Mail config records are simple structs
    No need for jsoniter or easyjson at this scale
    
    type MailRecord struct {
        Email   string   `json:"email"`
        Backup  []string `json:"backup,omitempty"`
        PubKey  string   `json:"pubkey,omitempty"`
    }

================================================================================
CACHING
================================================================================

github.com/hashicorp/golang-lru/v2

    Why: Battle-tested LRU cache with generics
    What it handles:
        - TTL expiration
        - Thread safety
        - Size limits
    Code saved: ~80 LOC
    
    Usage:
        cache, _ := expirable.NewLRU[string, ForwardingRule](1000, nil, time.Hour)

    Alternative: github.com/patrickmn/go-cache
        Simpler API, also solid choice

================================================================================
LOGGING
================================================================================

stdlib log/slog (Go 1.21+)

    Why: Structured logging in stdlib, no external dep
    What it handles:
        - JSON or text output
        - Log levels
        - Context propagation
    
    Usage:
        slog.Info("forwarding mail", "from", from, "to", resolved)

================================================================================
CONFIGURATION
================================================================================

github.com/caarlos0/env/v10

    Why: Parse config from environment variables
    What it handles:
        - Struct tag parsing
        - Type conversion
        - Defaults
    Code saved: ~50 LOC
    
    type Config struct {
        ListenAddr   string `env:"LISTEN_ADDR" envDefault:":2525"`
        UpstreamSMTP string `env:"UPSTREAM_SMTP,required"`
        CacheTTL     time.Duration `env:"CACHE_TTL" envDefault:"1h"`
    }

    Alternative: Config file with gopkg.in/yaml.v3
        Use if operators prefer files over env vars

================================================================================
TESTING
================================================================================

stdlib testing + github.com/stretchr/testify/assert

    Why: Testify reduces assertion boilerplate
    
    assert.Equal(t, "user@gmail.com", resolved.Target)
    assert.NoError(t, err)

For SMTP integration tests:

github.com/foxcpp/go-mockdns (if mocking DNS)
    - Not needed if using Namecoin directly

================================================================================
SUMMARY
================================================================================

Required dependencies (3):
    github.com/emersion/go-smtp       # SMTP server
    github.com/wneessen/go-mail       # SMTP client
    github.com/hashicorp/golang-lru/v2 # Caching

Optional dependencies (2):
    github.com/caarlos0/env/v10       # Config from env
    github.com/stretchr/testify       # Test assertions

Total: 3-5 external packages

Estimated code with libraries: ~400 LOC
Estimated code without libraries: ~1200 LOC

================================================================================
GO.MOD
================================================================================

module permamail

go 1.22

require (
    github.com/emersion/go-smtp v0.21.0
    github.com/wneessen/go-mail v0.4.0
    github.com/hashicorp/golang-lru/v2 v2.0.7
    github.com/caarlos0/env/v10 v10.0.0
)

// namecoin module is local or internal
require your-org/namecoin v0.0.0