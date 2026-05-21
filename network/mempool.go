package network

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// TxValidator defines the interface for transaction validation
// This allows the mempool to validate transactions before accepting them
type TxValidator interface {
	// ValidateMempoolTransaction validates a transaction for inclusion in the mempool
	// Returns an error if the transaction is invalid
	ValidateMempoolTransaction(tx *wire.MsgTx) error
}

// mempoolTx represents a transaction in the mempool with metadata
type mempoolTx struct {
	tx       *wire.MsgTx
	addedAt  time.Time
	lastSeen time.Time
}

// Mempool stores unconfirmed transactions with validation
type Mempool struct {
	txs       map[chainhash.Hash]*mempoolTx
	validator TxValidator
	mu        sync.RWMutex

	// Configuration
	maxTxs      int           // Maximum number of transactions to store
	txExpiry    time.Duration // How long to keep unconfirmed transactions
	cleanupTick time.Duration // How often to cleanup expired transactions

	// Lifecycle management
	quit     chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once
}

// MempoolConfig configures mempool behavior
type MempoolConfig struct {
	Validator   TxValidator   // Transaction validator (required)
	MaxTxs      int           // Maximum transactions (default: 5000)
	TxExpiry    time.Duration // Transaction expiry (default: 24 hours)
	CleanupTick time.Duration // Cleanup interval (default: 10 minutes)
}

// NewMempool creates a new transaction mempool.
// Callers must call Stop() when done to avoid goroutine leaks.
func NewMempool() *Mempool {
	return NewMempoolWithConfig(nil)
}

// NewMempoolWithConfig creates a new transaction mempool with custom configuration.
// Callers must call Stop() when done to avoid goroutine leaks.
func NewMempoolWithConfig(cfg *MempoolConfig) *Mempool {
	if cfg == nil {
		cfg = &MempoolConfig{}
	}

	// Set defaults
	if cfg.MaxTxs <= 0 {
		cfg.MaxTxs = 5000
	}
	if cfg.TxExpiry <= 0 {
		cfg.TxExpiry = 24 * time.Hour
	}
	if cfg.CleanupTick <= 0 {
		cfg.CleanupTick = 10 * time.Minute
	}

	mp := &Mempool{
		txs:         make(map[chainhash.Hash]*mempoolTx),
		validator:   cfg.Validator,
		maxTxs:      cfg.MaxTxs,
		txExpiry:    cfg.TxExpiry,
		cleanupTick: cfg.CleanupTick,
		quit:        make(chan struct{}),
	}

	// Start cleanup goroutine
	mp.wg.Add(1)
	go mp.cleanupLoop()

	return mp
}

// cleanupLoop periodically removes expired transactions
func (mp *Mempool) cleanupLoop() {
	defer mp.wg.Done()

	ticker := time.NewTicker(mp.cleanupTick)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mp.cleanupExpired()
		case <-mp.quit:
			return
		}
	}
}

// cleanupExpired removes transactions that have been in the mempool too long
func (mp *Mempool) cleanupExpired() {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	now := time.Now()
	expiredCount := 0

	for hash, mtx := range mp.txs {
		if now.Sub(mtx.addedAt) > mp.txExpiry {
			delete(mp.txs, hash)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		log.Printf("Mempool: cleaned up %d expired transactions (remaining: %d)",
			expiredCount, len(mp.txs))
	}
}

// AddTx adds a transaction to the mempool with validation
func (mp *Mempool) AddTx(tx *wire.MsgTx) error {
	if tx == nil {
		return fmt.Errorf("cannot add nil transaction")
	}

	txHash := tx.TxHash()

	mp.mu.Lock()
	defer mp.mu.Unlock()

	// Check if transaction is already in mempool
	if mtx, exists := mp.txs[txHash]; exists {
		// Update last seen time for existing transaction
		mtx.lastSeen = time.Now()
		return nil
	}

	// Check mempool capacity
	if len(mp.txs) >= mp.maxTxs {
		return fmt.Errorf("mempool is full (%d transactions)", mp.maxTxs)
	}

	// Validate transaction if validator is available
	// LOCK ORDERING: This holds the mempool write lock while calling the validator,
	// which acquires a blockchain read lock. To prevent deadlocks, ensure that no
	// code path acquires the blockchain write lock and then calls mempool operations.
	// Current lock order: Mempool write lock → Blockchain read lock
	if mp.validator != nil {
		if err := mp.validator.ValidateMempoolTransaction(tx); err != nil {
			return fmt.Errorf("transaction validation failed: %w", err)
		}
	}

	// Add transaction to mempool
	now := time.Now()
	mp.txs[txHash] = &mempoolTx{
		tx:       tx,
		addedAt:  now,
		lastSeen: now,
	}

	log.Printf("Mempool: accepted transaction %s (total: %d)", txHash, len(mp.txs))
	return nil
}

// RemoveTx removes a transaction from the mempool
func (mp *Mempool) RemoveTx(txHash *chainhash.Hash) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	delete(mp.txs, *txHash)
}

// RemoveTxs removes multiple transactions from the mempool
// This is used when transactions are confirmed in a block
func (mp *Mempool) RemoveTxs(txHashes []chainhash.Hash) {
	if len(txHashes) == 0 {
		return
	}

	mp.mu.Lock()
	defer mp.mu.Unlock()

	for i := range txHashes {
		delete(mp.txs, txHashes[i])
	}

	log.Printf("Mempool: removed %d confirmed transactions (remaining: %d)",
		len(txHashes), len(mp.txs))
}

// GetTx retrieves a transaction from the mempool
func (mp *Mempool) GetTx(txHash *chainhash.Hash) (*wire.MsgTx, bool) {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	if mtx, exists := mp.txs[*txHash]; exists {
		return mtx.tx, true
	}
	return nil, false
}

// HasTx checks if a transaction exists in the mempool
func (mp *Mempool) HasTx(txHash *chainhash.Hash) bool {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	_, exists := mp.txs[*txHash]
	return exists
}

// Count returns the number of transactions in the mempool
func (mp *Mempool) Count() int {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	return len(mp.txs)
}

// GetAll returns all transactions in the mempool
func (mp *Mempool) GetAll() []*wire.MsgTx {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	txs := make([]*wire.MsgTx, 0, len(mp.txs))
	for _, mtx := range mp.txs {
		txs = append(txs, mtx.tx)
	}
	return txs
}

// Clear removes all transactions from the mempool
func (mp *Mempool) Clear() {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.txs = make(map[chainhash.Hash]*mempoolTx)
	log.Printf("Mempool: cleared all transactions")
}

// Stop stops the mempool cleanup goroutine.
// Safe to call multiple times; only the first call has effect.
// Callers must invoke Stop to avoid goroutine leaks.
func (mp *Mempool) Stop() {
	mp.stopOnce.Do(func() { close(mp.quit) })
	mp.wg.Wait()
}
