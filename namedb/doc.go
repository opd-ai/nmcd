// Package namedb provides persistent storage for Namecoin name records.
//
// The namedb package implements a bbolt-backed database for storing Namecoin
// names, their history, expiration tracking, and UTXO management. It supports
// all name operations (NAME_NEW, NAME_FIRSTUPDATE, NAME_UPDATE) and handles
// blockchain reorganizations by maintaining sufficient history for rollback.
//
// # Storage Architecture
//
// The database uses multiple bbolt buckets for different data types:
//
//   - names: Current state of each registered name (NameRecord)
//   - history: Complete history of name operations for auditing
//   - history_index: Index for efficient history lookups
//   - expiration: Names indexed by expiration height for cleanup
//   - name_new: Pending NAME_NEW commitments awaiting NAME_FIRSTUPDATE
//   - utxo: Unspent transaction outputs for wallet functionality
//   - utxo_addr: Index mapping addresses to their UTXOs
//   - spent_utxo: Spent UTXOs indexed by block height for reorg handling
//
// # Name Record Versioning
//
// Name records use a versioned binary format for forward compatibility:
//
//   - Version 2: Added OutIndex for UTXO chain validation
//   - Version 3: Added NameNewHeight for accurate reorg handling
//
// # Expiration Tracking
//
// Names expire 36,000 blocks after their last update. The package maintains
// an expiration index to efficiently identify expired names during block
// processing. Expired names are removed from the active names bucket but
// retained in history.
//
// # UTXO Management
//
// The package includes UTXO tracking for:
//
//   - Wallet balance calculation
//   - Name ownership verification
//   - Transaction input validation
//   - Blockchain reorganization restoration
//
// UTXOs are indexed by both outpoint (txid:vout) and address for efficient
// lookups in both directions.
//
// # Thread Safety
//
// NameDatabase is safe for concurrent use. All operations acquire appropriate
// locks (RWMutex) and database transactions are atomic. Batch operations
// provide better performance for bulk writes.
//
// # Batch Operations
//
// For bulk operations, use batching to improve performance:
//
//	batch := ndb.NewBatch()
//	for _, name := range names {
//	    batch.AddNameUpdate(name, value, height, txHash, address)
//	}
//	err := ndb.CommitBatch(batch)
//
// # Example Usage
//
// Creating and using a name database:
//
//	ndb, err := namedb.NewNameDatabase("/path/to/names.db")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer ndb.Close()
//
//	// Store a name record
//	record := &namedb.NameRecord{
//	    Name:      "d/example",
//	    Value:     `{"ip":"1.2.3.4"}`,
//	    TxHash:    txHash,
//	    OutIndex:  0,
//	    Height:    12345,
//	    ExpiresAt: 12345 + 36000,
//	    Address:   "N1...",
//	}
//	err = ndb.UpdateName(record)
//
//	// Lookup a name
//	record, err := ndb.GetName("d/example")
//	if err == namedb.ErrNameNotFound {
//	    fmt.Println("Name not registered")
//	}
//
//	// List names expiring at a height
//	names, err := ndb.GetExpiringNames(height)
//
// # Blockchain Reorganization
//
// The package supports reorganization by:
//
//  1. Keeping history of all name operations
//  2. Storing spent UTXOs indexed by block height
//  3. Providing RollbackToHeight to revert state
//
// During a reorg, call RollbackToHeight with the common ancestor height,
// then replay blocks from the new chain.
//
// # Performance Considerations
//
// For optimal performance:
//
//   - Use batch operations for bulk writes
//   - The internal cache reduces repeated lookups
//   - Consider periodic compaction for long-running nodes
//   - Monitor database size via the Stats method
package namedb
