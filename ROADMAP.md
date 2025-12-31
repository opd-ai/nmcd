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