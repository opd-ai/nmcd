package mail

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/opd-ai/nmcd/bridge"
)

// ForwardingRule contains the resolved email forwarding configuration.
// This is the result of routing a .bit address to real email addresses.
type ForwardingRule struct {
	// Target is the primary email address where mail should be forwarded
	Target string

	// Backups contains fallback email addresses to use if the primary fails
	Backups []string
}

// Resolver defines the interface for looking up mail forwarding rules.
// This abstraction decouples the router from the specific bridge implementation,
// enabling testing with mock resolvers.
//
// Note: This interface matches bridge.Resolver for seamless integration.
type Resolver interface {
	// LookupMail retrieves email configuration for a given name.
	// The name should be a localpart (e.g., "alice") without the .bit domain.
	LookupMail(ctx context.Context, name string) (bridge.MailConfig, error)
}

// cacheEntry stores a cached forwarding rule with expiration time
type cacheEntry struct {
	rule      ForwardingRule
	expiresAt time.Time
}

// Router resolves .bit email addresses to real email addresses using a pluggable resolver.
// It provides caching to reduce lookup overhead and improve performance.
//
// Thread-safety: Router is safe for concurrent use by multiple goroutines.
type Router struct {
	resolver Resolver              // Resolver for mail config lookups
	cache    map[string]cacheEntry // Cache mapping names to forwarding rules
	ttl      time.Duration         // Time-to-live for cache entries
	mu       sync.RWMutex          // Protects cache map
}

// NewRouter creates a new mail router with the given resolver and cache TTL.
//
// Parameters:
//   - resolver: Implementation of Resolver interface (typically bridge.NamecoinBridge)
//   - ttl: Cache time-to-live duration (0 disables caching)
//
// Returns:
//   - *Router: Initialized router ready for address resolution
//
// Example:
//
//	// Create with 1 hour cache TTL
//	router := NewRouter(bridgeResolver, time.Hour)
//
//	// Create with no caching
//	router := NewRouter(bridgeResolver, 0)
func NewRouter(resolver Resolver, ttl time.Duration) *Router {
	return &Router{
		resolver: resolver,
		cache:    make(map[string]cacheEntry),
		ttl:      ttl,
	}
}

// Route resolves a .bit email address to a real email address.
//
// The routing process:
//  1. Parse and validate the .bit address format
//  2. Extract the localpart for name lookup
//  3. Check cache for recent lookup (if TTL > 0)
//  4. Query resolver for mail configuration
//  5. Cache the result (if TTL > 0)
//  6. Return the primary target address
//
// Parameters:
//   - ctx: Context for cancellation and timeout support
//   - toAddr: Email address in .bit format (e.g., "alice@mail.bit")
//
// Returns:
//   - string: The resolved real email address
//   - error: Parsing or lookup error
//
// Example:
//
//	ctx := context.Background()
//	realAddr, err := router.Route(ctx, "alice@mail.bit")
//	if err != nil {
//	    log.Printf("Failed to route: %v", err)
//	    return
//	}
//	fmt.Printf("Forward mail to: %s\n", realAddr)
func (r *Router) Route(ctx context.Context, toAddr string) (string, error) {
	// Parse .bit address to extract name
	name, err := parseBitAddress(toAddr)
	if err != nil {
		return "", fmt.Errorf("invalid address: %w", err)
	}

	// Check cache first (read lock)
	if rule, ok := r.getCached(name); ok {
		return rule.Target, nil
	}

	// Cache miss - query resolver
	config, err := r.resolver.LookupMail(ctx, name)
	if err != nil {
		return "", fmt.Errorf("lookup failed for %s: %w", name, err)
	}

	// Build forwarding rule from config
	rule := ForwardingRule{
		Target:  config.ForwardTo,
		Backups: config.BackupAddrs,
	}

	// Cache the result (write lock)
	r.setCached(name, rule)

	return rule.Target, nil
}

// getCached retrieves a cached forwarding rule if it exists and hasn't expired.
// Returns the cached rule and true if found and valid, or zero value and false otherwise.
func (r *Router) getCached(name string) (ForwardingRule, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Caching disabled
	if r.ttl == 0 {
		return ForwardingRule{}, false
	}

	entry, ok := r.cache[name]
	if !ok {
		return ForwardingRule{}, false
	}

	// Check expiration
	if time.Now().After(entry.expiresAt) {
		return ForwardingRule{}, false
	}

	return entry.rule, true
}

// setCached stores a forwarding rule in the cache with expiration time.
// Does nothing if caching is disabled (TTL = 0).
func (r *Router) setCached(name string, rule ForwardingRule) {
	// Caching disabled
	if r.ttl == 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.cache[name] = cacheEntry{
		rule:      rule,
		expiresAt: time.Now().Add(r.ttl),
	}
}
