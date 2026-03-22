package network

import (
	"fmt"
	"testing"
	"time"

	"github.com/btcsuite/btcd/peer"
)

func TestPeerScoreManager_GetOrCreateScore(t *testing.T) {
	psm := NewPeerScoreManager()

	// Get score for new peer
	score1 := psm.GetOrCreateScore("peer1")
	if score1 == nil {
		t.Fatal("GetOrCreateScore returned nil")
	}

	// Verify initial state
	if score1.blocksProvided != 0 {
		t.Errorf("New peer blocksProvided = %d, want 0", score1.blocksProvided)
	}
	if score1.score != 50.0 {
		t.Errorf("New peer score = %f, want 50.0", score1.score)
	}

	// Get again - should return same instance
	score2 := psm.GetOrCreateScore("peer1")
	if score2 != score1 {
		t.Error("GetOrCreateScore returned different instance for same peer")
	}
}

func TestPeerScoreManager_RecordBlockReceived(t *testing.T) {
	psm := NewPeerScoreManager()
	addr := "peer1"

	// Record block received
	psm.RecordBlockReceived(addr, 500*time.Millisecond)

	score := psm.GetOrCreateScore(addr)
	score.mu.RLock()
	blocksProvided := score.blocksProvided
	consecutiveFailures := score.consecutiveFailures
	score.mu.RUnlock()

	if blocksProvided != 1 {
		t.Errorf("blocksProvided = %d, want 1", blocksProvided)
	}
	if consecutiveFailures != 0 {
		t.Errorf("consecutiveFailures = %d, want 0", consecutiveFailures)
	}
}

func TestPeerScoreManager_RecordTxReceived(t *testing.T) {
	psm := NewPeerScoreManager()
	addr := "peer1"

	// Record transaction received
	psm.RecordTxReceived(addr, 200*time.Millisecond)

	score := psm.GetOrCreateScore(addr)
	score.mu.RLock()
	txsProvided := score.txsProvided
	score.mu.RUnlock()

	if txsProvided != 1 {
		t.Errorf("txsProvided = %d, want 1", txsProvided)
	}
}

func TestPeerScoreManager_RecordFailure(t *testing.T) {
	psm := NewPeerScoreManager()
	addr := "peer1"

	// Record failures
	psm.RecordFailure(addr)
	psm.RecordFailure(addr)

	score := psm.GetOrCreateScore(addr)
	score.mu.RLock()
	failedRequests := score.failedRequests
	consecutiveFailures := score.consecutiveFailures
	score.mu.RUnlock()

	if failedRequests != 2 {
		t.Errorf("failedRequests = %d, want 2", failedRequests)
	}
	if consecutiveFailures != 2 {
		t.Errorf("consecutiveFailures = %d, want 2", consecutiveFailures)
	}
}

func TestPeerScoreManager_FailureResetsOnSuccess(t *testing.T) {
	psm := NewPeerScoreManager()
	addr := "peer1"

	// Record failures
	psm.RecordFailure(addr)
	psm.RecordFailure(addr)

	// Record success - should reset consecutive failures
	psm.RecordBlockReceived(addr, 100*time.Millisecond)

	score := psm.GetOrCreateScore(addr)
	score.mu.RLock()
	consecutiveFailures := score.consecutiveFailures
	score.mu.RUnlock()

	if consecutiveFailures != 0 {
		t.Errorf("consecutiveFailures after success = %d, want 0", consecutiveFailures)
	}
}

func TestPeerScoreManager_GetScore(t *testing.T) {
	psm := NewPeerScoreManager()
	addr := "peer1"

	// New peer should have neutral score
	score := psm.GetScore(addr)
	if score != 50.0 {
		t.Errorf("New peer score = %f, want 50.0", score)
	}

	// Record some successes - score should increase
	for i := 0; i < 5; i++ {
		psm.RecordBlockReceived(addr, 100*time.Millisecond)
	}

	scoreAfter := psm.GetScore(addr)
	if scoreAfter <= score {
		t.Errorf("Score after successes (%f) not greater than initial score (%f)", scoreAfter, score)
	}
}

func TestPeerScoreManager_ScoreCalculation(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(*PeerScoreManager, string)
		wantScoreRange [2]float64 // min, max
	}{
		{
			name: "many_blocks_provided",
			setup: func(psm *PeerScoreManager, addr string) {
				for i := 0; i < 10; i++ {
					psm.RecordBlockReceived(addr, 100*time.Millisecond)
				}
			},
			wantScoreRange: [2]float64{50.0, 75.0}, // Should be at or above neutral
		},
		{
			name: "many_failures",
			setup: func(psm *PeerScoreManager, addr string) {
				for i := 0; i < 10; i++ {
					psm.RecordFailure(addr)
				}
			},
			wantScoreRange: [2]float64{0.0, 30.0}, // Should be well below neutral
		},
		{
			name: "slow_responses",
			setup: func(psm *PeerScoreManager, addr string) {
				for i := 0; i < 5; i++ {
					psm.RecordBlockReceived(addr, 10*time.Second) // Very slow
				}
			},
			wantScoreRange: [2]float64{30.0, 60.0}, // Penalty for slowness offsets data bonus
		},
		{
			name: "fast_responses",
			setup: func(psm *PeerScoreManager, addr string) {
				for i := 0; i < 5; i++ {
					psm.RecordBlockReceived(addr, 100*time.Millisecond) // Fast
				}
			},
			wantScoreRange: [2]float64{50.0, 70.0}, // Bonus for providing data, no latency penalty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			psm := NewPeerScoreManager()
			addr := "test-peer"

			tt.setup(psm, addr)

			score := psm.GetScore(addr)
			if score < tt.wantScoreRange[0] || score > tt.wantScoreRange[1] {
				t.Errorf("score = %f, want range [%f, %f]",
					score, tt.wantScoreRange[0], tt.wantScoreRange[1])
			}
		})
	}
}

func TestPeerScoreManager_RemovePeer(t *testing.T) {
	psm := NewPeerScoreManager()
	addr := "peer1"

	// Create score
	psm.RecordBlockReceived(addr, 100*time.Millisecond)

	// Verify exists
	psm.mu.RLock()
	_, exists := psm.scores[addr]
	psm.mu.RUnlock()
	if !exists {
		t.Fatal("Peer score not created")
	}

	// Remove peer
	psm.RemovePeer(addr)

	// Verify removed
	psm.mu.RLock()
	_, exists = psm.scores[addr]
	psm.mu.RUnlock()
	if exists {
		t.Error("Peer score not removed")
	}
}

func TestPeerScoreManager_RecordPeerSeen(t *testing.T) {
	psm := NewPeerScoreManager()
	addr := "peer1"

	// Create score
	score := psm.GetOrCreateScore(addr)

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Record peer seen
	psm.RecordPeerSeen(addr)

	score.mu.RLock()
	timeSinceLastSeen := time.Since(score.lastSeen)
	score.mu.RUnlock()

	// Should be very recent
	if timeSinceLastSeen > 50*time.Millisecond {
		t.Errorf("lastSeen not updated: %v ago", timeSinceLastSeen)
	}
}

func TestPeerScore_ResponseTimeTracking(t *testing.T) {
	score := &PeerScore{
		connectedAt:       time.Now(),
		lastSeen:          time.Now(),
		lastResponseTimes: make([]float64, 0, 10),
	}

	// Record response times
	times := []float64{0.1, 0.2, 0.15, 0.3, 0.25}
	for _, rt := range times {
		score.recordResponseTime(rt)
	}

	// Check that average is reasonable
	if score.avgResponseTime <= 0 || score.avgResponseTime > 1.0 {
		t.Errorf("avgResponseTime = %f, expected in range (0, 1.0]", score.avgResponseTime)
	}

	// Check that we're keeping samples
	if len(score.lastResponseTimes) != len(times) {
		t.Errorf("lastResponseTimes length = %d, want %d", len(score.lastResponseTimes), len(times))
	}
}

func TestPeerScore_ResponseTimeSampleLimit(t *testing.T) {
	score := &PeerScore{
		connectedAt:       time.Now(),
		lastSeen:          time.Now(),
		lastResponseTimes: make([]float64, 0, 10),
	}

	// Record more than max samples
	for i := 0; i < 20; i++ {
		score.recordResponseTime(0.1)
	}

	// Should not exceed max
	if len(score.lastResponseTimes) > 10 {
		t.Errorf("lastResponseTimes length = %d, expected <= 10", len(score.lastResponseTimes))
	}
}

func TestPeerScoreManager_Concurrent(t *testing.T) {
	// Test concurrent access to PeerScoreManager to ensure thread-safety
	psm := NewPeerScoreManager()
	const numGoroutines = 100
	const numOpsPerGoroutine = 100
	done := make(chan bool, numGoroutines)

	// Concurrent operations: GetScore, RecordBlockReceived, RecordTxReceived, RecordFailure
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			addr := fmt.Sprintf("peer-%d", id%10) // Use 10 different peers

			for j := 0; j < numOpsPerGoroutine; j++ {
				switch j % 5 {
				case 0:
					psm.RecordBlockReceived(addr, 100*time.Millisecond)
				case 1:
					psm.RecordTxReceived(addr, 50*time.Millisecond)
				case 2:
					psm.RecordFailure(addr)
				case 3:
					_ = psm.GetScore(addr)
				case 4:
					psm.RecordPeerSeen(addr)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify that peer scores exist and are reasonable
	for i := 0; i < 10; i++ {
		addr := fmt.Sprintf("peer-%d", i)
		score := psm.GetScore(addr)
		if score < 0 || score > 100 {
			t.Errorf("Invalid score for %s: %f (expected 0-100)", addr, score)
		}
	}
}

// BenchmarkPeerScoreManager_GetScore measures score calculation performance
func BenchmarkPeerScoreManager_GetScore(b *testing.B) {
	psm := NewPeerScoreManager()
	addr := "benchmark-peer"

	// Setup peer with some history
	for i := 0; i < 10; i++ {
		psm.RecordBlockReceived(addr, 100*time.Millisecond)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = psm.GetScore(addr)
	}
}

// BenchmarkPeerScoreManager_RecordBlock measures record performance
func BenchmarkPeerScoreManager_RecordBlock(b *testing.B) {
	psm := NewPeerScoreManager()
	addr := "benchmark-peer"

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		psm.RecordBlockReceived(addr, 100*time.Millisecond)
	}
}

// TestPeerScoreManager_GetBestPeers_EdgeCases tests GetBestPeers with edge cases
func TestPeerScoreManager_GetBestPeers_EdgeCases(t *testing.T) {
	psm := NewPeerScoreManager()

	// Test with n=0
	result := psm.GetBestPeers(nil, 0)
	if result != nil {
		t.Errorf("GetBestPeers(nil, 0) should return nil, got %v", result)
	}

	// Test with empty slice
	result = psm.GetBestPeers([]*peer.Peer{}, 5)
	if result != nil {
		t.Errorf("GetBestPeers([], 5) should return nil, got %v", result)
	}

	// Test with negative n
	result = psm.GetBestPeers(nil, -1)
	if result != nil {
		t.Errorf("GetBestPeers(nil, -1) should return nil, got %v", result)
	}
}
