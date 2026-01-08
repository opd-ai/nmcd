package rpc

import (
	"fmt"
	"testing"
)

// Test_bug_9_rate_limiter_rapid_ip_changes demonstrates the memory accumulation issue
// documented in AUDIT.md Issue #9. When rapid IP address changes occur (e.g., 10,000+
// unique IPs per minute), the cleanup mechanism may not keep pace, allowing unbounded
// memory growth.
//
// The first subtest simulates effectively unbounded growth by using a very large
// maxSize so that all 10,000 buckets remain in memory, while the second subtest
// uses a smaller maxSize to show that the number of buckets is capped.
func Test_bug_9_rate_limiter_rapid_ip_changes(t *testing.T) {
	t.Run("large_capacity_scenario", func(t *testing.T) {
		// Create rate limiter with very high maxSize to allow all buckets
		rl := newBoundedRateLimiter(100, 100000)
		defer rl.stop()

		// Simulate rapid IP rotation: create 10,000 unique IP entries
		const numIPs = 10000
		for i := 0; i < numIPs; i++ {
			ip := fmt.Sprintf("192.168.%d.%d", i/256, i%256)
			rl.allow(ip)
		}

		// Check how many buckets exist
		rl.mu.RLock()
		bucketCount := len(rl.buckets)
		rl.mu.RUnlock()

		// With large capacity, all 10,000 buckets remain in memory
		if bucketCount != numIPs {
			t.Errorf("Expected %d buckets, got %d", numIPs, bucketCount)
		}

		t.Logf("Large capacity scenario: %d buckets in memory", bucketCount)
	})

	t.Run("bounded_growth_with_fix", func(t *testing.T) {
		// Create rate limiter with reasonable maxSize
		maxSize := 1000
		rl := newBoundedRateLimiter(100, maxSize)
		defer rl.stop()

		// Simulate rapid IP rotation: try to create 10,000 unique IP entries
		const numIPs = 10000
		for i := 0; i < numIPs; i++ {
			ip := fmt.Sprintf("10.%d.%d.%d", i/65536, (i/256)%256, i%256)
			rl.allow(ip)
		}

		// Check that bucket count is bounded
		rl.mu.RLock()
		bucketCount := len(rl.buckets)
		rl.mu.RUnlock()

		// With the fix, should be exactly at maxSize (LRU eviction maintains this)
		if bucketCount > maxSize {
			t.Errorf("Bucket count should be bounded to %d, got %d", maxSize, bucketCount)
		}

		t.Logf("Bounded scenario: %d buckets in memory (max: %d)", bucketCount, maxSize)
	})
}

// Test_bug_9_lru_eviction verifies that the least recently used entries are evicted
func Test_bug_9_lru_eviction(t *testing.T) {
	// Create rate limiter with small maxSize for easy testing
	maxSize := 5
	rl := newBoundedRateLimiter(100, maxSize)
	defer rl.stop()

	// Create 5 IP entries
	ips := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3", "192.168.1.4", "192.168.1.5"}
	for _, ip := range ips {
		rl.allow(ip)
	}

	// Verify we have 5 buckets
	rl.mu.RLock()
	if len(rl.buckets) != 5 {
		t.Errorf("Expected 5 buckets, got %d", len(rl.buckets))
	}
	rl.mu.RUnlock()

	// Add a 6th IP - should evict the oldest (192.168.1.1)
	rl.allow("192.168.1.6")

	rl.mu.RLock()
	bucketCount := len(rl.buckets)
	_, stillHasFirst := rl.buckets["192.168.1.1"]
	_, hasNew := rl.buckets["192.168.1.6"]
	rl.mu.RUnlock()

	// Should still have exactly 5 buckets
	if bucketCount != 5 {
		t.Errorf("Expected 5 buckets after eviction, got %d", bucketCount)
	}

	// First IP should have been evicted (it was the oldest/LRU)
	if stillHasFirst {
		t.Error("Oldest IP should have been evicted")
	}

	// New IP should exist
	if !hasNew {
		t.Error("New IP should have been added")
	}
}

// Test_bug_9_default_limit verifies that the default newRateLimiter uses a bounded size
func Test_bug_9_default_limit(t *testing.T) {
	rl := newRateLimiter(100)
	defer rl.stop()

	// Verify maxSize is set to the default
	if rl.maxSize != DefaultMaxIPsInRateLimiter {
		t.Errorf("Expected default maxSize %d, got %d", DefaultMaxIPsInRateLimiter, rl.maxSize)
	}

	// Try to create more than the default limit
	const numIPs = DefaultMaxIPsInRateLimiter + 1000
	for i := 0; i < numIPs; i++ {
		ip := fmt.Sprintf("172.16.%d.%d", (i/256)%256, i%256)
		rl.allow(ip)
	}

	rl.mu.RLock()
	bucketCount := len(rl.buckets)
	rl.mu.RUnlock()

	// Should be capped at default limit
	if bucketCount > DefaultMaxIPsInRateLimiter {
		t.Errorf("Bucket count should be bounded to %d, got %d", DefaultMaxIPsInRateLimiter, bucketCount)
	}

	t.Logf("Default limiter: %d buckets (max: %d)", bucketCount, DefaultMaxIPsInRateLimiter)
}

// Benchmark to measure memory impact of rapid IP changes
func Benchmark_bug_9_rapid_ip_rotation(b *testing.B) {
	rl := newRateLimiter(1000)
	defer rl.stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Generate unique IP for each request
		ip := fmt.Sprintf("172.16.%d.%d", (i/256)%256, i%256)
		rl.allow(ip)
	}

	b.StopTimer()

	rl.mu.RLock()
	bucketCount := len(rl.buckets)
	rl.mu.RUnlock()

	b.Logf("Created %d buckets for %d requests (capped at %d)", bucketCount, b.N, DefaultMaxIPsInRateLimiter)
}
