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

	// First, check staleness under a read lock.
	score.mu.RLock()
	isStale := time.Since(score.scoreTime) > time.Minute
	currentScore := score.score
	score.mu.RUnlock()

	if !isStale {
		return currentScore
	}

	// Score is stale; acquire write lock to recalculate safely.
	score.mu.Lock()
	// Another goroutine might have already updated the score, so re-check.
	if time.Since(score.scoreTime) > time.Minute {
		score.calculateScore()
	}
	currentScore = score.score
	score.mu.Unlock()

	return currentScore
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
	bonuses := score.calcDataBonus() + score.calcLongevityBonus()
	penalties := score.calcFailurePenalty() + score.calcConsecutiveFailurePenalty() +
		score.calcLatencyPenalty() + score.calcIdlePenalty()

	finalScore := 50.0 + bonuses - penalties
	score.score = clampScore(finalScore)
	score.scoreTime = time.Now()
}

// calcDataBonus returns a bonus (max 20) for providing blocks and transactions.
func (score *PeerScore) calcDataBonus() float64 {
	return clampScore(float64(score.blocksProvided+score.txsProvided)*0.5, 20.0)
}

// calcFailurePenalty returns a penalty (max 30) for failed requests.
func (score *PeerScore) calcFailurePenalty() float64 {
	return clampScore(float64(score.failedRequests)*2.0, 30.0)
}

// calcConsecutiveFailurePenalty returns a penalty (max 20) for consecutive failures.
func (score *PeerScore) calcConsecutiveFailurePenalty() float64 {
	return clampScore(float64(score.consecutiveFailures)*5.0, 20.0)
}

// calcLatencyPenalty returns a penalty (max 15) based on average response time.
func (score *PeerScore) calcLatencyPenalty() float64 {
	if score.avgResponseTime > 5.0 {
		return 15.0
	}
	if score.avgResponseTime > 1.0 {
		return (score.avgResponseTime - 1.0) * 3.0
	}
	return 0
}

// calcLongevityBonus returns a bonus (max 15) for connection stability.
func (score *PeerScore) calcLongevityBonus() float64 {
	return clampScore(time.Since(score.connectedAt).Minutes()*0.1, 15.0)
}

// calcIdlePenalty returns a penalty (max 20) for idle peers.
func (score *PeerScore) calcIdlePenalty() float64 {
	idleMinutes := time.Since(score.lastSeen).Minutes()
	if idleMinutes > 60 {
		return 20.0
	}
	if idleMinutes > 10 {
		return (idleMinutes - 10.0) * 0.4
	}
	return 0
}

// clampScore clamps a value to [0, max]. If no max is given, clamps to [0, 100].
func clampScore(value float64, maxValues ...float64) float64 {
	maxVal := 100.0
	if len(maxValues) > 0 {
		maxVal = maxValues[0]
	}
	if value < 0 {
		return 0
	}
	if value > maxVal {
		return maxVal
	}
	return value
}
