package network

import (
	"sync"
	"time"

	"github.com/btcsuite/btcd/peer"
)

// PeerScore tracks performance metrics for a peer to enable smart peer selection.
// Higher scores indicate more reliable, faster peers that should be preferred for requests.
type PeerScore struct {
	mu sync.RWMutex

	// Connection time tracking
	connectedAt time.Time
	lastSeen    time.Time

	// Performance metrics
	blocksProvided    int       // Count of blocks successfully received from peer
	txsProvided       int       // Count of transactions successfully received from peer
	failedRequests    int       // Count of failed or timed-out requests
	avgResponseTime   float64   // Exponential moving average of response times (seconds)
	lastResponseTimes []float64 // Recent response times for calculating average

	// Error tracking
	consecutiveFailures int       // Count of consecutive failures (reset on success)
	lastFailure         time.Time // Time of last failure for backoff calculations

	// Calculated score (cached)
	score     float64   // Current overall score (0-100, higher is better)
	scoreTime time.Time // When score was last calculated
}

// PeerScoreManager manages scores for all peers
type PeerScoreManager struct {
	mu     sync.RWMutex
	scores map[string]*PeerScore // keyed by peer address

	// Configuration
	maxResponseSamples int           // Maximum number of response time samples to keep (default: 10)
	scoreDecayInterval time.Duration // How often to decay scores for inactive peers (default: 1 hour)
}

// NewPeerScoreManager creates a new peer score manager
func NewPeerScoreManager() *PeerScoreManager {
	return &PeerScoreManager{
		scores:             make(map[string]*PeerScore),
		maxResponseSamples: 10,
		scoreDecayInterval: time.Hour,
	}
}

// GetOrCreateScore gets or creates a score entry for a peer
func (psm *PeerScoreManager) GetOrCreateScore(addr string) *PeerScore {
	psm.mu.Lock()
	defer psm.mu.Unlock()

	score, exists := psm.scores[addr]
	if !exists {
		score = &PeerScore{
			connectedAt:       time.Now(),
			lastSeen:          time.Now(),
			score:             50.0, // Start with neutral score
			scoreTime:         time.Now(),
			lastResponseTimes: make([]float64, 0, psm.maxResponseSamples),
		}
		psm.scores[addr] = score
	}

	return score
}

// RecordBlockReceived records that a block was successfully received from a peer
func (psm *PeerScoreManager) RecordBlockReceived(addr string, responseTime time.Duration) {
	score := psm.GetOrCreateScore(addr)
	score.mu.Lock()
	defer score.mu.Unlock()

	score.blocksProvided++
	score.consecutiveFailures = 0
	score.lastSeen = time.Now()
	score.recordResponseTime(responseTime.Seconds())
	score.invalidateScore()
}

// RecordTxReceived records that a transaction was successfully received from a peer
func (psm *PeerScoreManager) RecordTxReceived(addr string, responseTime time.Duration) {
	score := psm.GetOrCreateScore(addr)
	score.mu.Lock()
	defer score.mu.Unlock()

	score.txsProvided++
	score.consecutiveFailures = 0
	score.lastSeen = time.Now()
	score.recordResponseTime(responseTime.Seconds())
	score.invalidateScore()
}

// RecordFailure records a failed request to a peer
func (psm *PeerScoreManager) RecordFailure(addr string) {
	score := psm.GetOrCreateScore(addr)
	score.mu.Lock()
	defer score.mu.Unlock()

	score.failedRequests++
	score.consecutiveFailures++
	score.lastFailure = time.Now()
	score.invalidateScore()
}

// RecordPeerSeen updates the last seen time for a peer (for keepalive/ping)
func (psm *PeerScoreManager) RecordPeerSeen(addr string) {
	score := psm.GetOrCreateScore(addr)
	score.mu.Lock()
	defer score.mu.Unlock()

	score.lastSeen = time.Now()
}

// GetScore calculates and returns the current score for a peer
func (psm *PeerScoreManager) GetScore(addr string) float64 {
	psm.mu.RLock()
	score, exists := psm.scores[addr]
	psm.mu.RUnlock()

	if !exists {
		return 50.0 // Neutral score for unknown peers
	}

	score.mu.RLock()
	defer score.mu.RUnlock()

	// Recalculate if stale (>1 minute old)
	if time.Since(score.scoreTime) > time.Minute {
		score.mu.RUnlock()
		score.mu.Lock()
		score.calculateScore()
		currentScore := score.score
		score.mu.Unlock()
		score.mu.RLock()
		return currentScore
	}

	return score.score
}

// GetBestPeers returns up to n peers with the highest scores
func (psm *PeerScoreManager) GetBestPeers(peers []*peer.Peer, n int) []*peer.Peer {
	if n <= 0 || len(peers) == 0 {
		return nil
	}

	type peerWithScore struct {
		peer  *peer.Peer
		score float64
	}

	// Calculate scores for all peers
	scored := make([]peerWithScore, 0, len(peers))
	for _, p := range peers {
		score := psm.GetScore(p.Addr())
		scored = append(scored, peerWithScore{peer: p, score: score})
	}

	// Simple selection sort to find top n (sufficient for small peer counts)
	if n > len(scored) {
		n = len(scored)
	}

	for i := 0; i < n; i++ {
		maxIdx := i
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[maxIdx].score {
				maxIdx = j
			}
		}
		if maxIdx != i {
			scored[i], scored[maxIdx] = scored[maxIdx], scored[i]
		}
	}

	// Extract top n peers
	result := make([]*peer.Peer, n)
	for i := 0; i < n; i++ {
		result[i] = scored[i].peer
	}

	return result
}

// RemovePeer removes a peer's score entry (called when peer disconnects)
func (psm *PeerScoreManager) RemovePeer(addr string) {
	psm.mu.Lock()
	defer psm.mu.Unlock()
	delete(psm.scores, addr)
}

// recordResponseTime adds a response time sample and updates moving average
// Must be called with score.mu locked
func (score *PeerScore) recordResponseTime(seconds float64) {
	// Add to recent samples
	score.lastResponseTimes = append(score.lastResponseTimes, seconds)

	// Keep only recent samples
	if len(score.lastResponseTimes) > 10 {
		score.lastResponseTimes = score.lastResponseTimes[1:]
	}

	// Calculate exponential moving average (EMA) with alpha=0.3
	// EMA gives more weight to recent samples while smoothing noise
	if score.avgResponseTime == 0 {
		score.avgResponseTime = seconds
	} else {
		alpha := 0.3
		score.avgResponseTime = alpha*seconds + (1-alpha)*score.avgResponseTime
	}
}

// invalidateScore marks the cached score as stale
// Must be called with score.mu locked
func (score *PeerScore) invalidateScore() {
	score.scoreTime = time.Time{} // Zero time indicates stale
}

// calculateScore computes the overall peer score based on metrics.
// Score ranges from 0 (worst) to 100 (best).
// Must be called with score.mu locked (write lock)
func (score *PeerScore) calculateScore() {
	baseScore := 50.0

	// Bonus for providing blocks/transactions (+20 max)
	dataBonus := float64(score.blocksProvided+score.txsProvided) * 0.5
	if dataBonus > 20.0 {
		dataBonus = 20.0
	}

	// Penalty for failed requests (-30 max)
	failurePenalty := float64(score.failedRequests) * 2.0
	if failurePenalty > 30.0 {
		failurePenalty = 30.0
	}

	// Penalty for consecutive failures (-20 max)
	consecutivePenalty := float64(score.consecutiveFailures) * 5.0
	if consecutivePenalty > 20.0 {
		consecutivePenalty = 20.0
	}

	// Penalty for slow response times (-15 max)
	// Fast: < 1s (no penalty), Medium: 1-5s (small penalty), Slow: >5s (large penalty)
	var latencyPenalty float64
	if score.avgResponseTime > 5.0 {
		latencyPenalty = 15.0
	} else if score.avgResponseTime > 1.0 {
		latencyPenalty = (score.avgResponseTime - 1.0) * 3.0 // Linear scale 0-12
	}

	// Bonus for longevity (stable connection) (+15 max)
	connectionMinutes := time.Since(score.connectedAt).Minutes()
	longevityBonus := connectionMinutes * 0.1
	if longevityBonus > 15.0 {
		longevityBonus = 15.0
	}

	// Penalty if peer hasn't been seen recently (-20 max)
	idleMinutes := time.Since(score.lastSeen).Minutes()
	var idlePenalty float64
	if idleMinutes > 60 {
		idlePenalty = 20.0
	} else if idleMinutes > 10 {
		idlePenalty = (idleMinutes - 10.0) * 0.4
	}

	// Calculate final score
	finalScore := baseScore + dataBonus + longevityBonus - failurePenalty - consecutivePenalty - latencyPenalty - idlePenalty

	// Clamp to 0-100 range
	if finalScore < 0 {
		finalScore = 0
	}
	if finalScore > 100 {
		finalScore = 100
	}

	score.score = finalScore
	score.scoreTime = time.Now()
}
