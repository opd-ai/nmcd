package rpc

import (
	"net"
	"sync"
	"time"
)

const (
	// DefaultMaxIPsInRateLimiter is the maximum number of IP addresses to track
	// in the rate limiter. When this limit is reached, the oldest entries are evicted.
	// This prevents unbounded memory growth with rapid IP rotation.
	DefaultMaxIPsInRateLimiter = 10000
)

// rateLimiter implements per-IP rate limiting using token bucket algorithm
type rateLimiter struct {
	mu      sync.RWMutex
	buckets map[string]*bucket
	rate    int           // requests per minute
	maxSize int           // maximum number of IPs to track (prevents unbounded growth)
	cleanup *time.Ticker  // cleanup ticker
	done    chan struct{} // cleanup stop signal
}

// bucket represents a token bucket for a single IP
type bucket struct {
	tokens     float64
	lastRefill time.Time
	lastUsed   time.Time // Track last usage for cleanup
}

// newRateLimiter creates a new rate limiter with the specified rate (requests per minute)
// and a default maximum size to prevent unbounded memory growth.
func newRateLimiter(rate int) *rateLimiter {
	return newBoundedRateLimiter(rate, DefaultMaxIPsInRateLimiter)
}

// newBoundedRateLimiter creates a new rate limiter with the specified rate and maximum size.
// When maxSize IPs are tracked and a new IP arrives, the least recently used IP is evicted.
// This prevents unbounded memory growth in scenarios with rapid IP rotation.
func newBoundedRateLimiter(rate int, maxSize int) *rateLimiter {
	rl := &rateLimiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		maxSize: maxSize,
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
		// Before creating a new bucket, check if we're at capacity
		if len(rl.buckets) >= rl.maxSize {
			// Evict the least recently used IP to make room
			rl.evictOldestBucket()
		}
		
		b = &bucket{
			tokens:     float64(rl.rate),
			lastRefill: now,
			lastUsed:   now,
		}
		rl.buckets[ip] = b
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastRefill)
	tokensToAdd := elapsed.Minutes() * float64(rl.rate)
	if tokensToAdd > 0 {
		b.tokens += tokensToAdd
		if b.tokens > float64(rl.rate) {
			b.tokens = float64(rl.rate)
		}
	}

	// Always update lastRefill when tokens are added or consumed
	b.lastRefill = now
	b.lastUsed = now

	// Check if request can be allowed
	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}

	return false
}

// evictOldestBucket removes the least recently used bucket to make room for new entries.
// This method must be called while holding rl.mu lock.
func (rl *rateLimiter) evictOldestBucket() {
	var oldestIP string
	var oldestTime time.Time
	
	// Find the bucket with the oldest lastUsed time
	first := true
	for ip, b := range rl.buckets {
		if first || b.lastUsed.Before(oldestTime) {
			oldestIP = ip
			oldestTime = b.lastUsed
			first = false
		}
	}
	
	// Remove the oldest bucket if found
	if oldestIP != "" {
		delete(rl.buckets, oldestIP)
	}
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
				if now.Sub(b.lastUsed) > 10*time.Minute {
					delete(rl.buckets, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.done:
			return
		}
	}
}

// triggerCleanup manually triggers the cleanup process (for testing)
func (rl *rateLimiter) triggerCleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, b := range rl.buckets {
		// Remove buckets that haven't been used in 10 minutes
		if now.Sub(b.lastUsed) > 10*time.Minute {
			delete(rl.buckets, ip)
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
