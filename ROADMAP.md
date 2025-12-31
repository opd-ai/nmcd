PERMAMAIL IMPLEMENTATION ROADMAP
================================
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
PHASE 1: BRIDGE ADAPTER (Week 1)
================================================================================

Goal: Create thin adapter between existing Namecoin and mail system

File: bridge/namecoin.go

    type MailConfig struct {
        ForwardTo   string   // e.g., "user@gmail.com"
        BackupAddrs []string // fallback addresses
        PublicKey   []byte   // for sender verification (optional)
    }

    type Resolver interface {
        LookupMail(bitName string) (MailConfig, error)
    }

    type NamecoinBridge struct {
        nc *namecoin.Client  // existing Namecoin client
    }

    func (b *NamecoinBridge) LookupMail(name string) (MailConfig, error) {
        record, err := b.nc.NameShow(name)
        if err != nil {
            return MailConfig{}, err
        }
        return parseMailConfig(record.Value)
    }

Namecoin record format (stored in name value JSON):

    {
        "email": "user@gmail.com",
        "backup": ["backup@proton.me"],
        "pubkey": "base64..."
    }

Deliverable: Adapter that reads existing Namecoin names, extracts mail config

================================================================================
PHASE 2: MAIL ROUTER (Week 2)
================================================================================

Goal: Stateless routing logic, no Namecoin dependency

File: mail/router.go

    type ForwardingRule struct {
        Target  string
        Backups []string
    }

    type Resolver interface {
        LookupMail(bitName string) (ForwardingRule, error)
    }

    type Router struct {
        resolver Resolver        // injected (bridge.NamecoinBridge)
        cache    map[string]cacheEntry
        ttl      time.Duration
    }

    func (r *Router) Route(toAddr string) (string, error) {
        name, err := parseBitAddress(toAddr)  // "alice@mail.bit" -> "alice"
        if err != nil {
            return "", err
        }
        rule, err := r.resolver.LookupMail(name)
        if err != nil {
            return "", err
        }
        return rule.Target, nil
    }

File: mail/config.go

    func parseBitAddress(addr string) (string, error) {
        // Validates format: localpart@*.bit
        // Returns localpart for Namecoin lookup
    }

Deliverable: Router that maps .bit addresses to real email addresses

================================================================================
PHASE 3: SMTP RELAY (Week 3)
================================================================================

Goal: Minimal SMTP server that accepts mail and forwards it

File: mail/smtp.go

    type Relay struct {
        router    *Router
        upstream  string        // outbound SMTP server
        listenAddr string
    }

    func (s *Relay) Start() error {
        // Uses go-smtp or net/smtp
        // Listen on port 25/587
    }

    func (s *Relay) handleMessage(from string, to string, body []byte) error {
        realAddr, err := s.router.Route(to)
        if err != nil {
            return err
        }
        return s.forward(from, realAddr, body)
    }

    func (s *Relay) forward(from, to string, body []byte) error {
        // Relay via upstream SMTP (sendgrid, mailgun, or direct)
    }

External dependency: github.com/emersion/go-smtp (lightweight, maintained)

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