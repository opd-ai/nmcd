package mail

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/opd-ai/nmcd/bridge"
)

// mockResolver implements the Resolver interface for testing
type mockResolver struct {
	configs map[string]bridge.MailConfig
	err     error
}

func (m *mockResolver) LookupMail(ctx context.Context, name string) (bridge.MailConfig, error) {
	if m.err != nil {
		return bridge.MailConfig{}, m.err
	}
	if config, ok := m.configs[name]; ok {
		return config, nil
	}
	return bridge.MailConfig{}, errors.New("name not found")
}

// TestNewRouter tests router construction
func TestNewRouter(t *testing.T) {
	resolver := &mockResolver{configs: make(map[string]bridge.MailConfig)}
	ttl := time.Hour

	router := NewRouter(resolver, ttl)

	if router == nil {
		t.Fatal("NewRouter returned nil")
	}
	if router.resolver != resolver {
		t.Error("Router resolver not set correctly")
	}
	if router.ttl != ttl {
		t.Errorf("Router TTL = %v, want %v", router.ttl, ttl)
	}
	if router.cache == nil {
		t.Error("Router cache map not initialized")
	}
}

// TestRoute_Success tests successful routing
func TestRoute_Success(t *testing.T) {
	tests := []struct {
		name       string
		bitAddr    string
		lookupName string
		config     bridge.MailConfig
		wantTarget string
	}{
		{
			name:       "simple_route",
			bitAddr:    "alice@mail.bit",
			lookupName: "alice",
			config: bridge.MailConfig{
				ForwardTo: "alice@gmail.com",
			},
			wantTarget: "alice@gmail.com",
		},
		{
			name:       "route_with_backups",
			bitAddr:    "bob@inbox.bit",
			lookupName: "bob",
			config: bridge.MailConfig{
				ForwardTo:   "bob@proton.me",
				BackupAddrs: []string{"bob-backup@gmail.com"},
			},
			wantTarget: "bob@proton.me",
		},
		{
			name:       "subdomain",
			bitAddr:    "user@mail.example.bit",
			lookupName: "user",
			config: bridge.MailConfig{
				ForwardTo: "user@example.com",
			},
			wantTarget: "user@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockResolver{
				configs: map[string]bridge.MailConfig{
					tt.lookupName: tt.config,
				},
			}
			router := NewRouter(resolver, 0) // No caching for test clarity

			ctx := context.Background()
			got, err := router.Route(ctx, tt.bitAddr)
			if err != nil {
				t.Errorf("Route(%q) unexpected error: %v", tt.bitAddr, err)
				return
			}
			if got != tt.wantTarget {
				t.Errorf("Route(%q) = %q, want %q", tt.bitAddr, got, tt.wantTarget)
			}
		})
	}
}

// TestRoute_Errors tests error cases
func TestRoute_Errors(t *testing.T) {
	tests := []struct {
		name        string
		bitAddr     string
		resolverErr error
		wantErrMsg  string
	}{
		{
			name:       "invalid_address_format",
			bitAddr:    "alice.gmail.com",
			wantErrMsg: "invalid address",
		},
		{
			name:       "not_bit_domain",
			bitAddr:    "alice@gmail.com",
			wantErrMsg: "invalid address",
		},
		{
			name:        "resolver_error",
			bitAddr:     "alice@mail.bit",
			resolverErr: errors.New("database error"),
			wantErrMsg:  "lookup failed",
		},
		{
			name:       "name_not_found",
			bitAddr:    "nonexistent@mail.bit",
			wantErrMsg: "lookup failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockResolver{
				configs: make(map[string]bridge.MailConfig),
				err:     tt.resolverErr,
			}
			router := NewRouter(resolver, 0)

			ctx := context.Background()
			got, err := router.Route(ctx, tt.bitAddr)
			if err == nil {
				t.Errorf("Route(%q) expected error containing %q, got nil (result: %q)", tt.bitAddr, tt.wantErrMsg, got)
				return
			}
			if got != "" {
				t.Errorf("Route(%q) error case should return empty string, got %q", tt.bitAddr, got)
			}
			errMsg := err.Error()
			if !contains(errMsg, tt.wantErrMsg) {
				t.Errorf("Route(%q) error = %q, want error containing %q", tt.bitAddr, errMsg, tt.wantErrMsg)
			}
		})
	}
}

// TestRoute_Caching tests cache behavior
func TestRoute_Caching(t *testing.T) {
	resolver := &mockResolver{
		configs: map[string]bridge.MailConfig{
			"alice": {ForwardTo: "alice@gmail.com"},
		},
	}

	// Create router with 1 hour cache
	router := NewRouter(resolver, time.Hour)
	ctx := context.Background()

	// First lookup - should hit resolver
	got1, err := router.Route(ctx, "alice@mail.bit")
	if err != nil {
		t.Fatalf("First Route() error: %v", err)
	}
	if got1 != "alice@gmail.com" {
		t.Errorf("First Route() = %q, want %q", got1, "alice@gmail.com")
	}

	// Second lookup - should hit cache (change resolver to verify)
	resolver.configs["alice"] = bridge.MailConfig{ForwardTo: "different@example.com"}
	got2, err := router.Route(ctx, "alice@mail.bit")
	if err != nil {
		t.Fatalf("Second Route() error: %v", err)
	}
	// Should still return cached value
	if got2 != "alice@gmail.com" {
		t.Errorf("Second Route() = %q, want cached value %q", got2, "alice@gmail.com")
	}
}

// TestRoute_CacheDisabled tests that caching can be disabled
func TestRoute_CacheDisabled(t *testing.T) {
	resolver := &mockResolver{
		configs: map[string]bridge.MailConfig{
			"alice": {ForwardTo: "alice@gmail.com"},
		},
	}

	// Create router with TTL = 0 (caching disabled)
	router := NewRouter(resolver, 0)
	ctx := context.Background()

	// First lookup
	got1, err := router.Route(ctx, "alice@mail.bit")
	if err != nil {
		t.Fatalf("First Route() error: %v", err)
	}
	if got1 != "alice@gmail.com" {
		t.Errorf("First Route() = %q, want %q", got1, "alice@gmail.com")
	}

	// Change resolver config
	resolver.configs["alice"] = bridge.MailConfig{ForwardTo: "updated@example.com"}

	// Second lookup should see updated value (no cache)
	got2, err := router.Route(ctx, "alice@mail.bit")
	if err != nil {
		t.Fatalf("Second Route() error: %v", err)
	}
	if got2 != "updated@example.com" {
		t.Errorf("Second Route() = %q, want updated value %q", got2, "updated@example.com")
	}
}

// TestRoute_CacheExpiration tests that cache entries expire
func TestRoute_CacheExpiration(t *testing.T) {
	resolver := &mockResolver{
		configs: map[string]bridge.MailConfig{
			"alice": {ForwardTo: "alice@gmail.com"},
		},
	}

	// Create router with very short TTL
	router := NewRouter(resolver, 10*time.Millisecond)
	ctx := context.Background()

	// First lookup - caches result
	got1, err := router.Route(ctx, "alice@mail.bit")
	if err != nil {
		t.Fatalf("First Route() error: %v", err)
	}
	if got1 != "alice@gmail.com" {
		t.Errorf("First Route() = %q, want %q", got1, "alice@gmail.com")
	}

	// Wait for cache to expire
	time.Sleep(15 * time.Millisecond)

	// Update resolver config
	resolver.configs["alice"] = bridge.MailConfig{ForwardTo: "updated@example.com"}

	// Lookup after expiration should see new value
	got2, err := router.Route(ctx, "alice@mail.bit")
	if err != nil {
		t.Fatalf("Second Route() error: %v", err)
	}
	if got2 != "updated@example.com" {
		t.Errorf("Second Route() after expiration = %q, want %q", got2, "updated@example.com")
	}
}

// TestRoute_ConcurrentAccess tests thread-safe concurrent routing
func TestRoute_ConcurrentAccess(t *testing.T) {
	resolver := &mockResolver{
		configs: map[string]bridge.MailConfig{
			"alice": {ForwardTo: "alice@gmail.com"},
			"bob":   {ForwardTo: "bob@proton.me"},
		},
	}
	router := NewRouter(resolver, time.Hour)

	ctx := context.Background()
	done := make(chan bool)

	// Launch multiple concurrent goroutines
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_, _ = router.Route(ctx, "alice@mail.bit")
				_, _ = router.Route(ctx, "bob@mail.bit")
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
	// Test passes if no race conditions occur
}
