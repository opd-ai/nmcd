package network

import (
	"sync"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// Mempool stores unconfirmed transactions
type Mempool struct {
	txs map[chainhash.Hash]*wire.MsgTx
	mu  sync.RWMutex
}

// NewMempool creates a new transaction mempool
func NewMempool() *Mempool {
	return &Mempool{
		txs: make(map[chainhash.Hash]*wire.MsgTx),
	}
}

// AddTx adds a transaction to the mempool
func (mp *Mempool) AddTx(tx *wire.MsgTx) error {
	if tx == nil {
		return nil
	}

	mp.mu.Lock()
	defer mp.mu.Unlock()

	txHash := tx.TxHash()
	mp.txs[txHash] = tx
	return nil
}

// RemoveTx removes a transaction from the mempool
func (mp *Mempool) RemoveTx(txHash *chainhash.Hash) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	delete(mp.txs, *txHash)
}

// GetTx retrieves a transaction from the mempool
func (mp *Mempool) GetTx(txHash *chainhash.Hash) (*wire.MsgTx, bool) {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	tx, exists := mp.txs[*txHash]
	return tx, exists
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
	for _, tx := range mp.txs {
		txs = append(txs, tx)
	}
	return txs
}

// Clear removes all transactions from the mempool
func (mp *Mempool) Clear() {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.txs = make(map[chainhash.Hash]*wire.MsgTx)
}
