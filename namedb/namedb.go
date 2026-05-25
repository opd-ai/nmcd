package namedb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"go.etcd.io/bbolt"
)

var (
	namesBucket           = []byte("names")
	historyBucket         = []byte("history")
	historyIndexBucket    = []byte("history_index")
	expirationBucket      = []byte("expiration")
	nameNewBucket         = []byte("name_new")          // Tracks NAME_NEW commitments
	utxoBucket            = []byte("utxo")              // Tracks unspent transaction outputs
	utxoAddrBucket        = []byte("utxo_addr")         // Index: address -> UTXOs
	spentUtxoBucket       = []byte("spent_utxo")        // Tracks spent UTXOs for reorg restoration (indexed by block height)
	spentUtxoIdxBucket    = []byte("spent_utxo_idx")    // Index: height -> list of spent UTXO keys
	expiredNamesBucket    = []byte("expired_names")     // Tracks expired names for reorg restoration (indexed by block height)
	expiredNamesIdxBucket = []byte("expired_names_idx") // Index: height+name -> presence marker
	expiredHistBucket     = []byte("expired_hist")      // History records for expired names (keyed by txHash)
	expiredHistIdxBucket  = []byte("expired_hist_idx")  // Index: height+name -> list of txHashes for history restoration
)

// Sentinel errors for namedb operations
var (
	// ErrNameNotFound is returned when a requested name does not exist in the database
	ErrNameNotFound = errors.New("name not found")
)

// txHashSize is the size of a transaction hash in bytes.
// Bitcoin/Namecoin use double SHA256 (SHA256(SHA256(data))) which produces
// a 32-byte result, same as a single SHA256.
const txHashSize = 32

// NameRecord encoding version. This implementation uses versioned format:
// - Version 2: Includes OutIndex for UTXO chain validation
// - Version 3: Adds NameNewHeight for accurate reorg handling
// This is a clean Namecoin implementation - no legacy versions exist.
const NameRecordVersion = 3

// NameOperation represents a name operation type
type NameOperation uint8

const (
	// NameNew represents a NAME_NEW operation that creates a commitment hash
	// to reserve a name without revealing it. This prevents front-running attacks.
	NameNew NameOperation = iota

	// NameFirstUpdate represents a NAME_FIRSTUPDATE operation that reveals the
	// name from a previous NAME_NEW commitment and sets its initial value.
	NameFirstUpdate

	// NameUpdate represents a NAME_UPDATE operation that updates an existing
	// name's value and extends its expiration by 36,000 blocks.
	NameUpdate
)

// String returns the string representation of the NameOperation
func (op NameOperation) String() string {
	switch op {
	case NameNew:
		return "NAME_NEW"
	case NameFirstUpdate:
		return "NAME_FIRSTUPDATE"
	case NameUpdate:
		return "NAME_UPDATE"
	default:
		return fmt.Sprintf("UnknownOperation(%d)", op)
	}
}

// NameRecord represents a name in the database
type NameRecord struct {
	Name          string
	Value         string
	TxHash        chainhash.Hash
	OutIndex      uint32 // Output index of the UTXO that owns this name
	Height        int32
	ExpiresAt     int32
	Address       string
	UpdatedAt     time.Time
	NameNewHeight int32 // Original NAME_NEW height (for NAME_FIRSTUPDATE only, used during reorg rollback)
}

// Copy creates a deep copy of the NameRecord.
// This prevents aliasing issues when caching or returning records.
func (nr *NameRecord) Copy() *NameRecord {
	if nr == nil {
		return nil
	}
	return &NameRecord{
		Name:          nr.Name,
		Value:         nr.Value,
		TxHash:        nr.TxHash, // chainhash.Hash is a value type ([32]byte)
		OutIndex:      nr.OutIndex,
		Height:        nr.Height,
		ExpiresAt:     nr.ExpiresAt,
		Address:       nr.Address,
		UpdatedAt:     nr.UpdatedAt,
		NameNewHeight: nr.NameNewHeight,
	}
}

// UTXO represents an unspent transaction output
type UTXO struct {
	TxHash   chainhash.Hash // Transaction hash
	OutIndex uint32         // Output index
	Value    int64          // Output value in satoshis
	Address  string         // Output address
	PkScript []byte         // Output script
	Height   int32          // Block height where UTXO was created
}

// Copy creates a deep copy of the UTXO.
// This prevents aliasing issues when caching or batching UTXOs.
func (u *UTXO) Copy() *UTXO {
	if u == nil {
		return nil
	}
	// Create a copy with a new PkScript slice
	pkScript := make([]byte, len(u.PkScript))
	copy(pkScript, u.PkScript)
	return &UTXO{
		TxHash:   u.TxHash, // chainhash.Hash is a value type ([32]byte)
		OutIndex: u.OutIndex,
		Value:    u.Value,
		Address:  u.Address,
		PkScript: pkScript,
		Height:   u.Height,
	}
}

// NameDatabase manages name operations with bbolt storage
type NameDatabase struct {
	db     *bbolt.DB
	mu     sync.RWMutex
	cache  *lruCache // LRU cache for name lookups (10,000 entries)
	closed bool      // Tracks whether database has been closed
}

// NewNameDatabase creates a new name database
func NewNameDatabase(dbPath string) (*NameDatabase, error) {
	db, err := bbolt.Open(dbPath, 0o600, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize buckets
	err = db.Update(func(tx *bbolt.Tx) error {
		for _, bucket := range [][]byte{namesBucket, historyBucket, historyIndexBucket, expirationBucket, nameNewBucket, utxoBucket, utxoAddrBucket, spentUtxoBucket, spentUtxoIdxBucket, expiredNamesBucket, expiredNamesIdxBucket, expiredHistBucket, expiredHistIdxBucket} {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	return &NameDatabase{
		db:    db,
		cache: newLRUCache(10000), // 10,000 entry LRU cache as per PLAN.md
	}, nil
}

// Close closes the database
func (ndb *NameDatabase) Close() error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	if ndb.closed {
		return nil // Already closed
	}

	if err := ndb.db.Close(); err != nil {
		return err
	}

	// Clear cache only after successful close.
	ndb.cache.Clear()
	ndb.closed = true
	return nil
}

// PutName stores a name record
func (ndb *NameDatabase) PutName(name string, record *NameRecord) error {
	if record == nil {
		return fmt.Errorf("record cannot be nil")
	}
	
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	err := ndb.db.Update(func(tx *bbolt.Tx) error {
		namesBucket := tx.Bucket(namesBucket)
		expirationBucket := tx.Bucket(expirationBucket)

		// Check if name already exists to update expiration index
		existingData := namesBucket.Get([]byte(name))
		if existingData != nil {
			// Remove old expiration index entry
			existingRecord, decodeErr := decodeNameRecord(existingData)
			if decodeErr == nil {
				oldExpirationKey := makeExpirationKey(existingRecord.ExpiresAt, name)
				expirationBucket.Delete(oldExpirationKey)
			}
		}

		// Store name record
		data := encodeNameRecord(record)
		if err := namesBucket.Put([]byte(name), data); err != nil {
			return err
		}

		// Add new expiration index entry
		// Key format: height (4 bytes) + name
		expirationKey := makeExpirationKey(record.ExpiresAt, name)
		return expirationBucket.Put(expirationKey, []byte{1}) // Value doesn't matter
	})

	if err == nil {
		// Ensure Name field is set before caching
		record.Name = name
		// Update cache with new value
		ndb.cache.Put(name, record)
	}

	return err
}

// makeExpirationKey creates an expiration index key from height and name.
// Format: height (4 bytes, big-endian) + name
// Big-endian ensures proper sorting by height.
func makeExpirationKey(height int32, name string) []byte {
	key := make([]byte, 4+len(name))
	binary.BigEndian.PutUint32(key[:4], uint32(height))
	copy(key[4:], []byte(name))
	return key
}

// GetName retrieves a name record
func (ndb *NameDatabase) GetName(name string) (*NameRecord, error) {
	// Check cache first (with RLock for cache read)
	if cached, ok := ndb.cache.Get(name); ok {
		return cached, nil
	}

	ndb.mu.RLock()
	defer ndb.mu.RUnlock()

	var record *NameRecord
	err := ndb.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(namesBucket)
		data := bucket.Get([]byte(name))
		if data == nil {
			return ErrNameNotFound
		}
		var decodeErr error
		record, decodeErr = decodeNameRecord(data)
		if decodeErr != nil {
			return fmt.Errorf("failed to decode name %s: %w", name, decodeErr)
		}
		record.Name = name
		return nil
	})

	// Cache the result if found
	if err == nil && record != nil {
		ndb.cache.Put(name, record)
	}

	return record, err
}

// DeleteName removes a name record
func (ndb *NameDatabase) DeleteName(name string) error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	err := ndb.db.Update(func(tx *bbolt.Tx) error {
		namesBucket := tx.Bucket(namesBucket)
		expirationBucket := tx.Bucket(expirationBucket)

		// Get existing record to remove from expiration index
		existingData := namesBucket.Get([]byte(name))
		if existingData != nil {
			existingRecord, decodeErr := decodeNameRecord(existingData)
			if decodeErr == nil {
				expirationKey := makeExpirationKey(existingRecord.ExpiresAt, name)
				expirationBucket.Delete(expirationKey)
			}
		}

		// Delete name record
		return namesBucket.Delete([]byte(name))
	})

	if err == nil {
		// Invalidate cache entry
		ndb.cache.Delete(name)
	}

	return err
}

// DeleteHistory removes all history entries for a name.
// This should be called when a name is deleted (e.g., due to expiration)
// to clean up storage and prevent history entries from accumulating.
func (ndb *NameDatabase) DeleteHistory(name string) error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		indexBucket := tx.Bucket(historyIndexBucket)
		histBucket := tx.Bucket(historyBucket)

		// Get the list of txHashes from the index
		indexData := indexBucket.Get([]byte(name))
		if indexData == nil {
			return nil // No history for this name
		}

		if len(indexData)%txHashSize != 0 {
			return fmt.Errorf("corrupt history index for name: %s", name)
		}

		// Delete all history records from the history bucket
		for i := 0; i < len(indexData); i += txHashSize {
			txHashBytes := indexData[i : i+txHashSize]
			if err := histBucket.Delete(txHashBytes); err != nil {
				return err
			}
		}

		// Delete the index entry
		return indexBucket.Delete([]byte(name))
	})
}

// GetExpiredNames returns names that have expired before the given height.
// A name is considered valid through its ExpiresAt block and only expired after.
// For example, a name with ExpiresAt=100 is valid at height 100 but expired at height 101.
// This implementation uses an expiration index for O(k) performance where k is the number
// of expired names, rather than O(n) where n is total names.
func (ndb *NameDatabase) GetExpiredNames(height int32) ([]string, error) {
	ndb.mu.RLock()
	defer ndb.mu.RUnlock()

	var expired []string
	err := ndb.db.View(func(tx *bbolt.Tx) error {
		expirationBucket := tx.Bucket(expirationBucket)

		// Use cursor to scan expiration index up to the given height
		c := expirationBucket.Cursor()

		// Seek to the beginning and iterate until we reach entries beyond height
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			if len(k) < 4 {
				continue // Invalid key, skip
			}

			// Extract height from key (first 4 bytes, big-endian)
			expiresAt := int32(binary.BigEndian.Uint32(k[:4]))

			// Names expire AFTER their ExpiresAt height
			// So ExpiresAt < height means expired at the given height
			if expiresAt < height {
				// Extract name from key (remaining bytes)
				name := string(k[4:])
				expired = append(expired, name)
			} else {
				// Since keys are sorted by height, we can stop here
				break
			}
		}

		return nil
	})
	return expired, err
}

// StoreExpiredName stores an expired name record and its history for potential
// restoration during reorganization. The backup is keyed by (height+name) so
// multiple expirations of the same name at different heights never collide.
// This should be called before DeleteName/DeleteHistory during expiration processing.
func (ndb *NameDatabase) StoreExpiredName(record *NameRecord, expiredAtHeight int32) error {
	if record == nil {
		return fmt.Errorf("record cannot be nil")
	}
	
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		expiredBkt := tx.Bucket(expiredNamesBucket)
		idxBkt := tx.Bucket(expiredNamesIdxBucket)
		expiredHistBkt := tx.Bucket(expiredHistBucket)
		expiredHistIdxBkt := tx.Bucket(expiredHistIdxBucket)

		// Build the height+name composite key used by both the name bucket and the index.
		nameBytes := []byte(record.Name)
		heightKey := make([]byte, 4+len(nameBytes))
		binary.BigEndian.PutUint32(heightKey[0:4], uint32(expiredAtHeight))
		copy(heightKey[4:], nameBytes)

		// Store the name record in the expired names bucket keyed by height+name.
		data := encodeNameRecord(record)
		if err := expiredBkt.Put(heightKey, data); err != nil {
			return err
		}

		// Add to the height index (value is a presence marker).
		if err := idxBkt.Put(heightKey, []byte{1}); err != nil {
			return err
		}

		// Backup the history for this name so it can be restored on reorg.
		histIdxBkt := tx.Bucket(historyIndexBucket)
		histBkt := tx.Bucket(historyBucket)

		txHashList := histIdxBkt.Get(nameBytes)
		if len(txHashList) > 0 {
			// Store the tx-hash list keyed by height+name.
			txHashListCopy := make([]byte, len(txHashList))
			copy(txHashListCopy, txHashList)
			if err := expiredHistIdxBkt.Put(heightKey, txHashListCopy); err != nil {
				return err
			}

			// Store each history record keyed by txHash.
			for i := 0; i+txHashSize <= len(txHashList); i += txHashSize {
				txHashBytes := txHashList[i : i+txHashSize]
				histData := histBkt.Get(txHashBytes)
				if histData == nil {
					continue // Missing record – skip, best-effort backup.
				}
				histDataCopy := make([]byte, len(histData))
				copy(histDataCopy, histData)
				if err := expiredHistBkt.Put(txHashBytes, histDataCopy); err != nil {
					return err
				}
			}
		}

		return nil
	})
}

// RestoreExpiredNamesForBlock restores all names that were expired at the given height.
// This is called during blockchain reorganization to undo expiration processing.
// Names are restored to both the names bucket and expiration index, their history is
// restored to the history buckets, and all backup data is removed.
func (ndb *NameDatabase) RestoreExpiredNamesForBlock(height int32) error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		namesBkt := tx.Bucket(namesBucket)
		expirationBkt := tx.Bucket(expirationBucket)
		expiredBkt := tx.Bucket(expiredNamesBucket)
		idxBkt := tx.Bucket(expiredNamesIdxBucket)
		expiredHistBkt := tx.Bucket(expiredHistBucket)
		expiredHistIdxBkt := tx.Bucket(expiredHistIdxBucket)
		histBkt := tx.Bucket(historyBucket)
		histIdxBkt := tx.Bucket(historyIndexBucket)

		// Find all names expired at this height using the index.
		heightPrefix := make([]byte, 4)
		binary.BigEndian.PutUint32(heightPrefix, uint32(height))

		c := idxBkt.Cursor()
		var heightKeys [][]byte

		// Collect all height+name keys for this height.
		for k, _ := c.Seek(heightPrefix); k != nil && bytes.HasPrefix(k, heightPrefix); k, _ = c.Next() {
			heightKeys = append(heightKeys, append([]byte(nil), k...))
		}

		// Restore each name.
		for _, heightKey := range heightKeys {
			name := string(heightKey[4:])

			// Get the stored expired name record (keyed by height+name).
			data := expiredBkt.Get(heightKey)
			if data == nil {
				continue // Already cleaned up or never stored.
			}

			// Decode the record.
			record, err := decodeNameRecord(data)
			if err != nil {
				return fmt.Errorf("failed to decode expired name %s: %w", name, err)
			}
			record.Name = name

			// Restore to names bucket.
			if err := namesBkt.Put([]byte(name), data); err != nil {
				return fmt.Errorf("failed to restore name %s: %w", name, err)
			}

			// Restore to expiration index.
			expirationKey := makeExpirationKey(record.ExpiresAt, name)
			if err := expirationBkt.Put(expirationKey, []byte{1}); err != nil {
				return fmt.Errorf("failed to restore expiration index for %s: %w", name, err)
			}

			// Restore history: put tx-hash list back into history index and
			// each history record back into the history bucket.
			txHashList := expiredHistIdxBkt.Get(heightKey)
			if len(txHashList) > 0 {
				txHashListCopy := make([]byte, len(txHashList))
				copy(txHashListCopy, txHashList)
				if err := histIdxBkt.Put([]byte(name), txHashListCopy); err != nil {
					return fmt.Errorf("failed to restore history index for %s: %w", name, err)
				}
				for i := 0; i+txHashSize <= len(txHashList); i += txHashSize {
					txHashBytes := txHashList[i : i+txHashSize]
					histData := expiredHistBkt.Get(txHashBytes)
					if histData == nil {
						continue // Missing backup record – skip.
					}
					histDataCopy := make([]byte, len(histData))
					copy(histDataCopy, histData)
					if err := histBkt.Put(txHashBytes, histDataCopy); err != nil {
						return fmt.Errorf("failed to restore history record for %s: %w", name, err)
					}
					if err := expiredHistBkt.Delete(txHashBytes); err != nil {
						return fmt.Errorf("failed to delete restored history backup for %s: %w", name, err)
					}
				}
				if err := expiredHistIdxBkt.Delete(heightKey); err != nil {
					return fmt.Errorf("failed to delete expired history index for %s: %w", name, err)
				}
			}

			// Remove backup entries.
			if err := expiredBkt.Delete(heightKey); err != nil {
				return fmt.Errorf("failed to delete from expired names bucket: %w", err)
			}
			if err := idxBkt.Delete(heightKey); err != nil {
				return fmt.Errorf("failed to delete from expired names index: %w", err)
			}

			// Invalidate cache entry since we're restoring the name.
			ndb.cache.Delete(name)
		}

		return nil
	})
}

// CleanupOldExpiredNames removes expired name backups (name records and history) older
// than the given height. This should be called periodically to prevent unbounded growth
// of the expired names storage. Typical usage: keep the last 100-200 blocks worth of
// expired names for reorg safety.
func (ndb *NameDatabase) CleanupOldExpiredNames(keepFromHeight int32) error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		expiredBkt := tx.Bucket(expiredNamesBucket)
		idxBkt := tx.Bucket(expiredNamesIdxBucket)
		expiredHistBkt := tx.Bucket(expiredHistBucket)
		expiredHistIdxBkt := tx.Bucket(expiredHistIdxBucket)

		c := idxBkt.Cursor()
		var keysToDelete [][]byte

		// Scan index for entries older than keepFromHeight.
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			if len(k) < 4 {
				continue // Invalid key
			}

			entryHeight := int32(binary.BigEndian.Uint32(k[:4]))
			if entryHeight < keepFromHeight {
				keysToDelete = append(keysToDelete, append([]byte(nil), k...))
			}
		}

		// Delete expired names, their history backups, and all index entries.
		for _, heightKey := range keysToDelete {
			// Remove history backup for this height+name entry.
			txHashList := expiredHistIdxBkt.Get(heightKey)
			if len(txHashList) > 0 {
				for i := 0; i+txHashSize <= len(txHashList); i += txHashSize {
					if err := expiredHistBkt.Delete(txHashList[i : i+txHashSize]); err != nil {
						return err
					}
				}
				if err := expiredHistIdxBkt.Delete(heightKey); err != nil {
					return err
				}
			}

			// Remove name backup and index entry (both keyed by height+name).
			if err := expiredBkt.Delete(heightKey); err != nil {
				return err
			}
			if err := idxBkt.Delete(heightKey); err != nil {
				return err
			}
		}

		return nil
	})
}

// ListNames returns all names in the database
func (ndb *NameDatabase) ListNames() ([]*NameRecord, error) {
	ndb.mu.RLock()
	defer ndb.mu.RUnlock()

	var names []*NameRecord
	err := ndb.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(namesBucket)
		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			record, decodeErr := decodeNameRecord(v)
			if decodeErr != nil {
				return fmt.Errorf("failed to decode name %s: %w", string(k), decodeErr)
			}
			record.Name = string(k)
			names = append(names, record)
		}
		return nil
	})
	return names, err
}

// ScanNames scans names matching a prefix with pagination.
// Returns up to count names starting from the given prefix.
// This is used by the name_scan RPC to provide Namecoin Core compatibility.
func (ndb *NameDatabase) ScanNames(prefix string, count int) ([]*NameRecord, error) {
	ndb.mu.RLock()
	defer ndb.mu.RUnlock()

	// Short-circuit for count <= 0 to avoid returning any results
	if count <= 0 {
		return nil, nil
	}

	var results []*NameRecord

	err := ndb.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(namesBucket)
		if bucket == nil {
			return nil
		}

		cursor := bucket.Cursor()
		prefixBytes := []byte(prefix)

		for k, v := cursor.Seek(prefixBytes); k != nil; k, v = cursor.Next() {
			// Check if key still has prefix
			if !bytes.HasPrefix(k, prefixBytes) {
				break
			}

			record, decodeErr := decodeNameRecord(v)
			if decodeErr != nil {
				log.Printf("Warning: skipping corrupted name entry in ScanNames for key %q: %v", string(k), decodeErr)
				continue
			}
			record.Name = string(k)

			results = append(results, record)

			if len(results) >= count {
				break
			}
		}

		return nil
	})

	return results, err
}

// AddHistory adds a historical name operation and updates the name-to-history index.
// The history is stored keyed by transaction hash, and an index entry is added to map
// the name to its list of transaction hashes for efficient retrieval.
func (ndb *NameDatabase) AddHistory(txHash chainhash.Hash, record *NameRecord) error {
	if record == nil {
		return fmt.Errorf("record cannot be nil")
	}
	
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		// Store the history record keyed by txHash
		histBucket := tx.Bucket(historyBucket)
		data := encodeNameRecord(record)
		if err := histBucket.Put(txHash[:], data); err != nil {
			return err
		}

		// Update the name-to-history index
		// The index stores a list of txHashes for each name
		indexBucket := tx.Bucket(historyIndexBucket)
		nameKey := []byte(record.Name)
		existing := indexBucket.Get(nameKey)

		// Append the new txHash to the existing list.
		// Copy existing to a new slice first to avoid mutating bbolt's mmap-backed memory.
		newIndex := append(append([]byte(nil), existing...), txHash[:]...)
		return indexBucket.Put(nameKey, newIndex)
	})
}

// GetHistory retrieves all historical records for a specific name.
// Returns a slice of NameRecords ordered by when they were added (oldest first).
func (ndb *NameDatabase) GetHistory(name string) ([]*NameRecord, error) {
	ndb.mu.RLock()
	defer ndb.mu.RUnlock()

	var records []*NameRecord
	err := ndb.db.View(func(tx *bbolt.Tx) error {
		// Get the list of txHashes from the index
		indexBucket := tx.Bucket(historyIndexBucket)
		indexData := indexBucket.Get([]byte(name))
		if indexData == nil {
			return nil // No history for this name
		}

		if len(indexData)%txHashSize != 0 {
			return fmt.Errorf("corrupt history index for name: %s", name)
		}

		histBucket := tx.Bucket(historyBucket)
		for i := 0; i < len(indexData); i += txHashSize {
			txHashBytes := indexData[i : i+txHashSize]
			data := histBucket.Get(txHashBytes)
			if data == nil {
				continue // Skip missing records
			}
			record, decodeErr := decodeNameRecord(data)
			if decodeErr != nil {
				return fmt.Errorf("failed to decode history for name %s: %w", name, decodeErr)
			}
			record.Name = name
			records = append(records, record)
		}
		return nil
	})
	return records, err
}

// NameNewRecord represents a NAME_NEW commitment stored in the database.
// It tracks the block height where the NAME_NEW was issued to enforce
// the minimum block distance before NAME_FIRSTUPDATE.
type NameNewRecord struct {
	Hash   []byte // The commitment hash from NAME_NEW
	Height int32  // Block height where NAME_NEW was included
}

// PutNameNew stores a NAME_NEW commitment with its block height.
// The commitment hash is used as the key, and the height is stored as the value.
// This allows validation that MinBlocksBeforeFirstUpdate has passed.
// Returns an error if the commitment already exists to prevent commitment replacement attacks.
func (ndb *NameDatabase) PutNameNew(commitHash []byte, height int32) error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(nameNewBucket)
		// Check if commitment already exists to prevent replacement attacks
		if bucket.Get(commitHash) != nil {
			return fmt.Errorf("name_new commitment already exists")
		}
		// Store height as 4-byte little-endian
		data := make([]byte, 4)
		binary.LittleEndian.PutUint32(data, uint32(height))
		return bucket.Put(commitHash, data)
	})
}

// RestoreNameNew restores a NAME_NEW commitment during block reorg rollback.
// Unlike PutNameNew (which rejects duplicates to prevent commitment replacement
// attacks during normal operation), RestoreNameNew allows overwriting because:
// 1. During rollback, we need to restore commitments consumed by NAME_FIRSTUPDATEs
// 2. The restored height may be an estimate since we don't store the original
// 3. This is safe because it only happens during reorg, not normal block processing
func (ndb *NameDatabase) RestoreNameNew(commitHash []byte, height int32) error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(nameNewBucket)
		// Store height as 4-byte little-endian (overwriting if exists)
		data := make([]byte, 4)
		binary.LittleEndian.PutUint32(data, uint32(height))
		return bucket.Put(commitHash, data)
	})
}

// GetNameNew retrieves a NAME_NEW commitment record by its hash.
// Returns the record if found, or an error if not found.
func (ndb *NameDatabase) GetNameNew(commitHash []byte) (*NameNewRecord, error) {
	ndb.mu.RLock()
	defer ndb.mu.RUnlock()

	var record *NameNewRecord
	err := ndb.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(nameNewBucket)
		data := bucket.Get(commitHash)
		if data == nil {
			return fmt.Errorf("name_new commitment not found")
		}
		if len(data) < 4 {
			return fmt.Errorf("corrupt name_new record")
		}
		// Block heights are always non-negative and stored as int32 throughout
		// the codebase. The cast is safe as blockchain heights won't exceed
		// MaxInt32 (would take ~4000 years at current block rates).
		height := int32(binary.LittleEndian.Uint32(data))
		// Copy commitHash to avoid aliasing with caller's slice
		hashCopy := make([]byte, len(commitHash))
		copy(hashCopy, commitHash)
		record = &NameNewRecord{
			Hash:   hashCopy,
			Height: height,
		}
		return nil
	})
	return record, err
}

// DeleteNameNew removes a NAME_NEW commitment after it has been used.
// Called after a successful NAME_FIRSTUPDATE to clean up.
func (ndb *NameDatabase) DeleteNameNew(commitHash []byte) error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(nameNewBucket)
		return bucket.Delete(commitHash)
	})
}

// RemoveLastHistoryEntry removes the most recent history entry for a name
// and returns the previous record if one exists. Used during block reorg
// to restore the previous state of a name.
// Returns nil, nil if there was only one entry (no previous state to restore).
func (ndb *NameDatabase) RemoveLastHistoryEntry(name string) (*NameRecord, error) {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	var prevRecord *NameRecord
	err := ndb.db.Update(func(tx *bbolt.Tx) error {
		indexBucket := tx.Bucket(historyIndexBucket)
		histBucket := tx.Bucket(historyBucket)

		indexData := indexBucket.Get([]byte(name))
		if len(indexData) == 0 {
			return nil
		}

		if len(indexData)%txHashSize != 0 {
			return fmt.Errorf("corrupt history index for name: %s", name)
		}

		if err := histBucket.Delete(indexData[len(indexData)-txHashSize:]); err != nil {
			return err
		}

		var err error
		prevRecord, err = truncateHistoryIndex(indexBucket, histBucket, name, indexData)
		return err
	})
	return prevRecord, err
}

// truncateHistoryIndex removes the last entry from the history index and returns the new last record.
func truncateHistoryIndex(indexBucket, histBucket *bbolt.Bucket, name string, indexData []byte) (*NameRecord, error) {
	newIndexData := indexData[:len(indexData)-txHashSize]
	if len(newIndexData) == 0 {
		return nil, indexBucket.Delete([]byte(name))
	}

	if err := indexBucket.Put([]byte(name), newIndexData); err != nil {
		return nil, err
	}

	prevTxHash := newIndexData[len(newIndexData)-txHashSize:]
	data := histBucket.Get(prevTxHash)
	if data == nil {
		return nil, nil
	}

	record, err := decodeNameRecord(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode previous record: %w", err)
	}
	record.Name = name
	return record, nil
}

// encodeNameRecord serializes a name record.
// Encoding format (version 3): version + value + txhash + outindex + height + expiresAt + address + timestamp + namenewheight
// Uses buffer pool to reduce allocations.
func encodeNameRecord(record *NameRecord) []byte {
	// Get a buffer from the pool
	buf := getBuffer()
	defer putBuffer(buf)

	// Pre-allocate a temporary 8-byte buffer for encoding integers
	// This avoids multiple small allocations
	tmp := make([]byte, 8)

	// Version byte
	buf.WriteByte(byte(NameRecordVersion))

	// Value length + value
	binary.LittleEndian.PutUint32(tmp[:4], uint32(len(record.Value)))
	buf.Write(tmp[:4])
	buf.WriteString(record.Value)

	// TxHash
	buf.Write(record.TxHash[:])

	// OutIndex (new in v2)
	binary.LittleEndian.PutUint32(tmp[:4], record.OutIndex)
	buf.Write(tmp[:4])

	// Height
	binary.LittleEndian.PutUint32(tmp[:4], uint32(record.Height))
	buf.Write(tmp[:4])

	// ExpiresAt
	binary.LittleEndian.PutUint32(tmp[:4], uint32(record.ExpiresAt))
	buf.Write(tmp[:4])

	// Address length + address
	binary.LittleEndian.PutUint32(tmp[:4], uint32(len(record.Address)))
	buf.Write(tmp[:4])
	buf.WriteString(record.Address)

	// Timestamp
	binary.LittleEndian.PutUint64(tmp[:8], uint64(record.UpdatedAt.Unix()))
	buf.Write(tmp[:8])

	// NameNewHeight (new in v3)
	binary.LittleEndian.PutUint32(tmp[:4], uint32(record.NameNewHeight))
	buf.Write(tmp[:4])

	// Return a copy of the buffer's bytes
	// We need to copy because the buffer will be returned to the pool
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	return result
}

// decodeNameRecord deserializes a name record.
// Returns an error if the data is corrupt or truncated.
// Supports both version 2 (without NameNewHeight) and version 3 (with NameNewHeight)
// for backward compatibility during upgrades.
func decodeNameRecord(data []byte) (*NameRecord, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("corrupt record: empty data")
	}

	version := data[0]
	if version != 2 && version != 3 {
		return nil, fmt.Errorf("unsupported record version: %d (expected 2 or 3)", version)
	}

	r := &recordReader{data: data, offset: 1}
	record := &NameRecord{}

	var err error
	record.Value, err = r.readString("value")
	if err != nil {
		return nil, err
	}

	if err := r.readFixedBytes(record.TxHash[:], 32, "txhash"); err != nil {
		return nil, err
	}

	record.OutIndex, err = r.readUint32("outindex")
	if err != nil {
		return nil, err
	}

	heightU32, err := r.readUint32("height")
	if err != nil {
		return nil, err
	}
	record.Height = int32(heightU32)

	expiresU32, err := r.readUint32("expires_at")
	if err != nil {
		return nil, err
	}
	record.ExpiresAt = int32(expiresU32)

	record.Address, err = r.readString("address")
	if err != nil {
		return nil, err
	}

	if r.offset+8 <= len(r.data) {
		ts := binary.LittleEndian.Uint64(r.data[r.offset : r.offset+8])
		record.UpdatedAt = time.Unix(int64(ts), 0)
		r.offset += 8
	}

	if version >= 3 {
		if r.offset+4 > len(r.data) {
			return nil, fmt.Errorf("corrupt record: version 3 requires NameNewHeight but data is truncated")
		}
		record.NameNewHeight = int32(binary.LittleEndian.Uint32(r.data[r.offset : r.offset+4]))
	}

	return record, nil
}

// recordReader provides sequential reading from a byte slice with bounds checking.
type recordReader struct {
	data   []byte
	offset int
}

// readUint32 reads a uint32 at the current offset.
func (r *recordReader) readUint32(field string) (uint32, error) {
	if r.offset+4 > len(r.data) {
		return 0, fmt.Errorf("corrupt record: truncated at %s", field)
	}
	v := binary.LittleEndian.Uint32(r.data[r.offset : r.offset+4])
	r.offset += 4
	return v, nil
}

// readString reads a length-prefixed string at the current offset.
func (r *recordReader) readString(field string) (string, error) {
	strLen, err := r.readUint32(field + " length")
	if err != nil {
		return "", err
	}
	if r.offset+int(strLen) > len(r.data) {
		return "", fmt.Errorf("corrupt record: truncated at %s data", field)
	}
	s := string(r.data[r.offset : r.offset+int(strLen)])
	r.offset += int(strLen)
	return s, nil
}

// readFixedBytes reads exactly n bytes into dst.
func (r *recordReader) readFixedBytes(dst []byte, n int, field string) error {
	if r.offset+n > len(r.data) {
		return fmt.Errorf("corrupt record: truncated at %s", field)
	}
	copy(dst, r.data[r.offset:r.offset+n])
	r.offset += n
	return nil
}
