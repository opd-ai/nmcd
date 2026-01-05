package rpc

import (
	"net"
	"sync"
	"time"
)

// rateLimiter implements per-IP rate limiting using token bucket algorithm
type rateLimiter struct {
	mu      sync.RWMutex
	buckets map[string]*bucket
	rate    int           // requests per minute
	cleanup *time.Ticker  // cleanup ticker
	done    chan struct{} // cleanup stop signal
}

// bucket represents a token bucket for a single IP
type bucket struct {
	tokens    int
	lastRefill time.Time
}

// newRateLimiter creates a new rate limiter with the specified rate (requests per minute)
func newRateLimiter(rate int) *rateLimiter {
	rl := &rateLimiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		cleanup: time.NewTicker(5 * time.Minute),
		done:    make(chan struct{}),
	}

	// Start cleanup goroutine to remove stale entries
	go rl.cleanupLoop()

	return rl
}

// allow checks if a request from the given IP should be allowed
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Get or create bucket for this IP
	b, exists := rl.buckets[ip]
	if !exists {
		b = &bucket{
			tokens:     rl.rate,
			lastRefill: now,
		}
		rl.buckets[ip] = b
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastRefill)
	tokensToAdd := int(elapsed.Minutes() * float64(rl.rate))
	if tokensToAdd > 0 {
		b.tokens += tokensToAdd
		if b.tokens > rl.rate {
			b.tokens = rl.rate
		}
		b.lastRefill = now
	}

	// Check if request can be allowed
	if b.tokens > 0 {
		b.tokens--
		return true
	}

	return false
}

// cleanupLoop removes stale bucket entries
func (rl *rateLimiter) cleanupLoop() {
	for {
		select {
		case <-rl.cleanup.C:
			rl.mu.Lock()
			now := time.Now()
			for ip, b := range rl.buckets {
				// Remove buckets that haven't been used in 10 minutes
				if now.Sub(b.lastRefill) > 10*time.Minute {
					delete(rl.buckets, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.done:
			return
		}
	}
}

// stop stops the cleanup goroutine
func (rl *rateLimiter) stop() {
	close(rl.done)
	rl.cleanup.Stop()
}

// extractIP extracts the IP address from a request's RemoteAddr
func extractIP(remoteAddr string) string {
	// RemoteAddr is in format "ip:port"
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// If parsing fails, use the entire remoteAddr
		return remoteAddr
	}
	return host
}
