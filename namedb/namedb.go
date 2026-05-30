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

// requireBucket returns the named bucket from tx, or an error if it is nil.
// A nil bucket indicates database corruption or a missing migration.
func requireBucket(tx *bbolt.Tx, name []byte) (*bbolt.Bucket, error) {
	b := tx.Bucket(name)
	if b == nil {
		return nil, fmt.Errorf("required bucket %q not found: database may be corrupted or requires migration", name)
	}
	return b, nil
}

// requireBuckets fetches all named buckets from tx in a single call.
// Returns a slice of buckets in the same order as names, or the first error
// encountered when a bucket is nil (indicating database corruption).
func requireBuckets(tx *bbolt.Tx, names ...[]byte) ([]*bbolt.Bucket, error) {
	buckets := make([]*bbolt.Bucket, len(names))
	for i, name := range names {
		b := tx.Bucket(name)
		if b == nil {
			return nil, fmt.Errorf("required bucket %q not found: database may be corrupted or requires migration", name)
		}
		buckets[i] = b
	}
	return buckets, nil
}

// withBucket calls fn with the named bucket, or returns an error if the bucket is nil.
// The nil check is encapsulated here to avoid adding an extra branch to callers.
func withBucket(tx *bbolt.Tx, name []byte, fn func(*bbolt.Bucket) error) error {
	b := tx.Bucket(name)
	if b == nil {
		return fmt.Errorf("required bucket %q not found: database may be corrupted or requires migration", name)
	}
	return fn(b)
}

// withBuckets fetches all named buckets from tx and calls fn with them.
// The nil checks are encapsulated here to avoid adding extra branches to callers.
func withBuckets(tx *bbolt.Tx, names [][]byte, fn func([]*bbolt.Bucket) error) error {
	buckets := make([]*bbolt.Bucket, len(names))
	for i, name := range names {
		b := tx.Bucket(name)
		if b == nil {
			return fmt.Errorf("required bucket %q not found: database may be corrupted or requires migration", name)
		}
		buckets[i] = b
	}
	return fn(buckets)
}

// txHashSize is the size of a transaction hash in bytes.
// Bitcoin/Namecoin use double SHA256 (SHA256(SHA256(data))) which produces
// a 32-byte result, same as a single SHA256.
const txHashSize = 32

// maxRecordStringLen is the upper bound for any on-disk length-prefixed string field.
// Namecoin consensus limits names to 255 bytes and values to 1 023 bytes, so any
// strLen field exceeding this constant on a 32-bit platform would overflow int.
const maxRecordStringLen = 1024

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

// PutName stores a name record in the database.
// If record.ExpiresAt is 0, the encoder derives it from record.Height + ExpirationDepth;
// callers that reuse the struct after PutName should read back ExpiresAt to see the
// computed value — or set ExpiresAt explicitly before calling.
func (ndb *NameDatabase) PutName(name string, record *NameRecord) error {
	if record == nil {
		return fmt.Errorf("record cannot be nil")
	}

	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	err := ndb.db.Update(func(tx *bbolt.Tx) error {
		bkts, err := requireBuckets(tx, namesBucket, expirationBucket)
		if err != nil {
			return err
		}
		namesBkt, expirationBkt := bkts[0], bkts[1]
		if err := deleteOldExpirationIndex(namesBkt, expirationBkt, name); err != nil {
			return err
		}
		if err := namesBkt.Put([]byte(name), encodeNameRecord(record)); err != nil {
			return err
		}
		return expirationBkt.Put(makeExpirationKey(record.ExpiresAt, name), []byte{1})
	})
	if err == nil {
		cacheNameRecord(ndb.cache, name, record)
	}
	return err
}

// deleteOldExpirationIndex removes an existing expiration index entry before overwriting a name.
func deleteOldExpirationIndex(namesBkt, expirationBkt *bbolt.Bucket, name string) error {
	existingData := namesBkt.Get([]byte(name))
	if existingData == nil {
		return nil
	}
	existingRecord, err := decodeNameRecord(existingData)
	if err != nil {
		return fmt.Errorf("failed to decode existing record for %q: %w", name, err)
	}
	return expirationBkt.Delete(makeExpirationKey(existingRecord.ExpiresAt, name))
}

// cacheNameRecord sets the record name and updates the cache after a successful write.
func cacheNameRecord(cache *lruCache, name string, record *NameRecord) {
	record.Name = name
	cache.Put(name, record)
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
		bucket, err := requireBucket(tx, namesBucket)
		if err != nil {
			return err
		}
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
		bkts, err := requireBuckets(tx, namesBucket, expirationBucket)
		if err != nil {
			return err
		}
		namesBkt, expirationBkt := bkts[0], bkts[1]

		// Get existing record to remove from expiration index
		existingData := namesBkt.Get([]byte(name))
		if existingData != nil {
			existingRecord, decodeErr := decodeNameRecord(existingData)
			if decodeErr == nil {
				expirationKey := makeExpirationKey(existingRecord.ExpiresAt, name)
				if err := expirationBkt.Delete(expirationKey); err != nil {
					return err
				}
			}
		}

		// Delete name record
		return namesBkt.Delete([]byte(name))
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
		bkts, err := requireBuckets(tx, historyIndexBucket, historyBucket)
		if err != nil {
			return err
		}
		idxBkt, histBkt := bkts[0], bkts[1]

		// Get the list of txHashes from the index
		indexData := idxBkt.Get([]byte(name))
		if indexData == nil {
			return nil // No history for this name
		}

		if len(indexData)%txHashSize != 0 {
			return fmt.Errorf("corrupt history index for name: %s", name)
		}

		// Delete all history records from the history bucket
		for i := 0; i < len(indexData); i += txHashSize {
			txHashBytes := indexData[i : i+txHashSize]
			if err := histBkt.Delete(txHashBytes); err != nil {
				return err
			}
		}

		// Delete the index entry
		return idxBkt.Delete([]byte(name))
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
		expirationBucket, err := requireBucket(tx, expirationBucket)
		if err != nil {
			return err
		}

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
		bkts, err := requireBuckets(
			tx,
			expiredNamesBucket,
			expiredNamesIdxBucket,
			expiredHistBucket,
			expiredHistIdxBucket,
			historyIndexBucket,
			historyBucket,
		)
		if err != nil {
			return err
		}
		expiredBkt, idxBkt := bkts[0], bkts[1]
		expiredHistBkt, expiredHistIdxBkt := bkts[2], bkts[3]
		histIdxBkt, histBkt := bkts[4], bkts[5]

		heightKey := makeHeightNameKey(expiredAtHeight, record.Name)
		if err := expiredBkt.Put(heightKey, encodeNameRecord(record)); err != nil {
			return err
		}
		if err := idxBkt.Put(heightKey, []byte{1}); err != nil {
			return err
		}
		return backupExpiredHistory(heightKey, []byte(record.Name), histIdxBkt, histBkt, expiredHistIdxBkt, expiredHistBkt)
	})
}

// copyBytes clones a byte slice so bbolt-backed memory is not retained or mutated.
func copyBytes(data []byte) []byte {
	return append([]byte(nil), data...)
}

// makeHeightNameKey builds the composite height+name key used by reorg backup buckets.
func makeHeightNameKey(height int32, name string) []byte {
	key := make([]byte, 4+len(name))
	binary.BigEndian.PutUint32(key[:4], uint32(height))
	copy(key[4:], name)
	return key
}

// backupExpiredHistory stores the history index and records needed to restore an expired name.
func backupExpiredHistory(heightKey, nameKey []byte, histIdxBkt, histBkt, expiredHistIdxBkt, expiredHistBkt *bbolt.Bucket) error {
	txHashList := histIdxBkt.Get(nameKey)
	if len(txHashList) == 0 {
		return nil
	}
	if err := expiredHistIdxBkt.Put(heightKey, copyBytes(txHashList)); err != nil {
		return err
	}
	return copyHistoryRecords(txHashList, histBkt, expiredHistBkt)
}

// copyHistoryRecords copies historical name records referenced by a tx-hash list.
func copyHistoryRecords(txHashList []byte, src, dst *bbolt.Bucket) error {
	for i := 0; i+txHashSize <= len(txHashList); i += txHashSize {
		txHashBytes := txHashList[i : i+txHashSize]
		histData := src.Get(txHashBytes)
		if histData == nil {
			continue
		}
		if err := dst.Put(txHashBytes, copyBytes(histData)); err != nil {
			return err
		}
	}
	return nil
}

// collectKeysForHeight gathers height-prefixed backup keys for a single block height.
func collectKeysForHeight(idxBkt *bbolt.Bucket, height int32) [][]byte {
	heightPrefix := make([]byte, 4)
	binary.BigEndian.PutUint32(heightPrefix, uint32(height))

	var heightKeys [][]byte
	c := idxBkt.Cursor()
	for k, _ := c.Seek(heightPrefix); k != nil && bytes.HasPrefix(k, heightPrefix); k, _ = c.Next() {
		heightKeys = append(heightKeys, copyBytes(k))
	}
	return heightKeys
}

// restoreExpiredNameRecord restores an expired name record to the live buckets.
func restoreExpiredNameRecord(heightKey []byte, expiredBkt, namesBkt, expirationBkt *bbolt.Bucket) (string, error) {
	data := expiredBkt.Get(heightKey)
	if data == nil {
		return "", nil
	}
	name := string(heightKey[4:])
	record, err := decodeNameRecord(data)
	if err != nil {
		return "", fmt.Errorf("failed to decode expired name %s: %w", name, err)
	}
	if err := namesBkt.Put([]byte(name), data); err != nil {
		return "", fmt.Errorf("failed to restore name %s: %w", name, err)
	}
	if err := expirationBkt.Put(makeExpirationKey(record.ExpiresAt, name), []byte{1}); err != nil {
		return "", fmt.Errorf("failed to restore expiration index for %s: %w", name, err)
	}
	return name, nil
}

// restoreExpiredHistory restores the history index and records for a previously expired name.
func restoreExpiredHistory(heightKey []byte, name string, expiredHistIdxBkt, expiredHistBkt, histIdxBkt, histBkt *bbolt.Bucket) error {
	txHashList := expiredHistIdxBkt.Get(heightKey)
	if len(txHashList) == 0 {
		return nil
	}
	if err := histIdxBkt.Put([]byte(name), copyBytes(txHashList)); err != nil {
		return fmt.Errorf("failed to restore history index for %s: %w", name, err)
	}
	if err := moveHistoryRecords(name, txHashList, expiredHistBkt, histBkt); err != nil {
		return err
	}
	if err := expiredHistIdxBkt.Delete(heightKey); err != nil {
		return fmt.Errorf("failed to delete expired history index for %s: %w", name, err)
	}
	return nil
}

// moveHistoryRecords copies restored history entries and removes the backup copies.
func moveHistoryRecords(name string, txHashList []byte, src, dst *bbolt.Bucket) error {
	for i := 0; i+txHashSize <= len(txHashList); i += txHashSize {
		txHashBytes := txHashList[i : i+txHashSize]
		histData := src.Get(txHashBytes)
		if histData == nil {
			continue
		}
		if err := dst.Put(txHashBytes, copyBytes(histData)); err != nil {
			return fmt.Errorf("failed to restore history record for %s: %w", name, err)
		}
		if err := src.Delete(txHashBytes); err != nil {
			return fmt.Errorf("failed to delete restored history backup for %s: %w", name, err)
		}
	}
	return nil
}

// deleteExpiredBackup removes expired-name backup entries after successful restoration.
func deleteExpiredBackup(heightKey []byte, expiredBkt, idxBkt *bbolt.Bucket) error {
	if err := expiredBkt.Delete(heightKey); err != nil {
		return fmt.Errorf("failed to delete from expired names bucket: %w", err)
	}
	if err := idxBkt.Delete(heightKey); err != nil {
		return fmt.Errorf("failed to delete from expired names index: %w", err)
	}
	return nil
}

// collectKeysBeforeHeight gathers backup keys older than the reorg retention threshold.
func collectKeysBeforeHeight(idxBkt *bbolt.Bucket, keepFromHeight int32) [][]byte {
	var keys [][]byte
	c := idxBkt.Cursor()
	for k, _ := c.First(); k != nil; k, _ = c.Next() {
		if len(k) < 4 || int32(binary.BigEndian.Uint32(k[:4])) >= keepFromHeight {
			continue
		}
		keys = append(keys, copyBytes(k))
	}
	return keys
}

// deleteExpiredHistoryBackup removes backed-up history records and index state for a name.
func deleteExpiredHistoryBackup(heightKey []byte, expiredHistIdxBkt, expiredHistBkt *bbolt.Bucket) error {
	txHashList := expiredHistIdxBkt.Get(heightKey)
	for i := 0; i+txHashSize <= len(txHashList); i += txHashSize {
		if err := expiredHistBkt.Delete(txHashList[i : i+txHashSize]); err != nil {
			return err
		}
	}
	if len(txHashList) == 0 {
		return nil
	}
	return expiredHistIdxBkt.Delete(heightKey)
}

// RestoreExpiredNamesForBlock restores all names that were expired at the given height.
// This is called during blockchain reorganization to undo expiration processing.
// Names are restored to both the names bucket and expiration index, their history is
// restored to the history buckets, and all backup data is removed.
func (ndb *NameDatabase) RestoreExpiredNamesForBlock(height int32) error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		bkts, err := requireBuckets(tx, namesBucket, expirationBucket, expiredNamesBucket, expiredNamesIdxBucket, expiredHistBucket, expiredHistIdxBucket, historyBucket, historyIndexBucket)
		if err != nil {
			return err
		}
		namesBkt, expirationBkt := bkts[0], bkts[1]
		expiredBkt, idxBkt := bkts[2], bkts[3]
		expiredHistBkt, expiredHistIdxBkt := bkts[4], bkts[5]
		histBkt, histIdxBkt := bkts[6], bkts[7]

		for _, heightKey := range collectKeysForHeight(idxBkt, height) {
			name, err := restoreExpiredNameRecord(heightKey, expiredBkt, namesBkt, expirationBkt)
			if err != nil {
				return err
			}
			if name == "" {
				continue
			}
			if err := restoreExpiredHistory(heightKey, name, expiredHistIdxBkt, expiredHistBkt, histIdxBkt, histBkt); err != nil {
				return err
			}
			if err := deleteExpiredBackup(heightKey, expiredBkt, idxBkt); err != nil {
				return err
			}
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
		bkts, err := requireBuckets(tx, expiredNamesBucket, expiredNamesIdxBucket, expiredHistBucket, expiredHistIdxBucket)
		if err != nil {
			return err
		}
		expiredBkt, idxBkt := bkts[0], bkts[1]
		expiredHistBkt, expiredHistIdxBkt := bkts[2], bkts[3]

		for _, heightKey := range collectKeysBeforeHeight(idxBkt, keepFromHeight) {
			if err := deleteExpiredHistoryBackup(heightKey, expiredHistIdxBkt, expiredHistBkt); err != nil {
				return err
			}
			if err := deleteExpiredBackup(heightKey, expiredBkt, idxBkt); err != nil {
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
		return withBucket(tx, namesBucket, func(bucket *bbolt.Bucket) error {
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
	})
	return names, err
}

// ScanNames scans names matching a prefix with pagination.
// Returns up to count names starting from the given prefix.
// This is used by the name_scan RPC to provide Namecoin Core compatibility.
func (ndb *NameDatabase) ScanNames(prefix string, count int) ([]*NameRecord, error) {
	ndb.mu.RLock()
	defer ndb.mu.RUnlock()
	if count <= 0 {
		return nil, nil
	}

	var results []*NameRecord
	err := ndb.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(namesBucket)
		if bucket == nil {
			return nil
		}
		prefixBytes := []byte(prefix)
		cursor := bucket.Cursor()
		for k, v := cursor.Seek(prefixBytes); k != nil; k, v = cursor.Next() {
			if !bytes.HasPrefix(k, prefixBytes) || len(results) >= count {
				break
			}
			if record := decodeScannedName(k, v); record != nil {
				results = append(results, record)
			}
		}
		return nil
	})
	return results, err
}

// decodeScannedName decodes a name entry for scanning and logs corrupt records.
func decodeScannedName(key, value []byte) *NameRecord {
	record, err := decodeNameRecord(value)
	if err != nil {
		log.Printf("Warning: skipping corrupted name entry in ScanNames for key %q: %v", string(key), err)
		return nil
	}
	record.Name = string(key)
	return record
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
		return withBuckets(tx, [][]byte{historyBucket, historyIndexBucket}, func(bkts []*bbolt.Bucket) error {
			// Store the history record keyed by txHash
			histBkt, idxBkt := bkts[0], bkts[1]
			data := encodeNameRecord(record)
			if err := histBkt.Put(txHash[:], data); err != nil {
				return err
			}

			// Update the name-to-history index
			// The index stores a list of txHashes for each name
			nameKey := []byte(record.Name)
			existing := idxBkt.Get(nameKey)

			// Append the new txHash to the existing list.
			// Copy existing to a new slice first to avoid mutating bbolt's mmap-backed memory.
			newIndex := append(append([]byte(nil), existing...), txHash[:]...)
			return idxBkt.Put(nameKey, newIndex)
		})
	})
}

// GetHistory retrieves all historical records for a specific name.
// Returns a slice of NameRecords ordered by when they were added (oldest first).
func (ndb *NameDatabase) GetHistory(name string) ([]*NameRecord, error) {
	ndb.mu.RLock()
	defer ndb.mu.RUnlock()

	var records []*NameRecord
	err := ndb.db.View(func(tx *bbolt.Tx) error {
		bkts, err := requireBuckets(tx, historyIndexBucket, historyBucket)
		if err != nil {
			return err
		}
		// Get the list of txHashes from the index
		idxBkt, histBkt := bkts[0], bkts[1]
		indexData := idxBkt.Get([]byte(name))
		if indexData == nil {
			return nil // No history for this name
		}

		if len(indexData)%txHashSize != 0 {
			return fmt.Errorf("corrupt history index for name: %s", name)
		}

		for i := 0; i < len(indexData); i += txHashSize {
			txHashBytes := indexData[i : i+txHashSize]
			data := histBkt.Get(txHashBytes)
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
		return withBucket(tx, nameNewBucket, func(bucket *bbolt.Bucket) error {
			// Check if commitment already exists to prevent replacement attacks
			if bucket.Get(commitHash) != nil {
				return fmt.Errorf("name_new commitment already exists")
			}
			// Store height as 4-byte little-endian
			data := make([]byte, 4)
			binary.LittleEndian.PutUint32(data, uint32(height))
			return bucket.Put(commitHash, data)
		})
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
		return withBucket(tx, nameNewBucket, func(bucket *bbolt.Bucket) error {
			// Store height as 4-byte little-endian (overwriting if exists)
			data := make([]byte, 4)
			binary.LittleEndian.PutUint32(data, uint32(height))
			return bucket.Put(commitHash, data)
		})
	})
}

// GetNameNew retrieves a NAME_NEW commitment record by its hash.
// Returns the record if found, or an error if not found.
func (ndb *NameDatabase) GetNameNew(commitHash []byte) (*NameNewRecord, error) {
	ndb.mu.RLock()
	defer ndb.mu.RUnlock()

	var record *NameNewRecord
	err := ndb.db.View(func(tx *bbolt.Tx) error {
		return withBucket(tx, nameNewBucket, func(bucket *bbolt.Bucket) error {
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
	})
	return record, err
}

// DeleteNameNew removes a NAME_NEW commitment after it has been used.
// Called after a successful NAME_FIRSTUPDATE to clean up.
func (ndb *NameDatabase) DeleteNameNew(commitHash []byte) error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		return withBucket(tx, nameNewBucket, func(bucket *bbolt.Bucket) error {
			return bucket.Delete(commitHash)
		})
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
		return withBuckets(tx, [][]byte{historyIndexBucket, historyBucket}, func(bkts []*bbolt.Bucket) error {
			idxBkt, histBkt := bkts[0], bkts[1]

			indexData := idxBkt.Get([]byte(name))
			if len(indexData) == 0 {
				return nil
			}

			if len(indexData)%txHashSize != 0 {
				return fmt.Errorf("corrupt history index for name: %s", name)
			}

			if err := histBkt.Delete(indexData[len(indexData)-txHashSize:]); err != nil {
				return err
			}

			var err error
			prevRecord, err = truncateHistoryIndex(idxBkt, histBkt, name, indexData)
			return err
		})
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
	buf := getBuffer()
	defer putBuffer(buf)
	tmp := make([]byte, 8)

	buf.WriteByte(byte(NameRecordVersion))
	writeRecordString(buf, tmp, record.Value)
	buf.Write(record.TxHash[:])
	writeRecordUint32(buf, tmp, record.OutIndex)
	writeRecordUint32(buf, tmp, uint32(record.Height))
	writeRecordUint32(buf, tmp, uint32(record.ExpiresAt))
	writeRecordString(buf, tmp, record.Address)
	writeRecordUint64(buf, tmp, uint64(record.UpdatedAt.Unix()))
	writeRecordUint32(buf, tmp, uint32(record.NameNewHeight))
	return copyBufferBytes(buf)
}

// writeRecordString appends a length-prefixed string to an encoded name record.
func writeRecordString(buf *bytes.Buffer, tmp []byte, value string) {
	writeRecordUint32(buf, tmp, uint32(len(value)))
	buf.WriteString(value)
}

// writeRecordUint32 appends a uint32 field to an encoded name record.
func writeRecordUint32(buf *bytes.Buffer, tmp []byte, value uint32) {
	binary.LittleEndian.PutUint32(tmp[:4], value)
	buf.Write(tmp[:4])
}

// writeRecordUint64 appends a uint64 field to an encoded name record.
func writeRecordUint64(buf *bytes.Buffer, tmp []byte, value uint64) {
	binary.LittleEndian.PutUint64(tmp[:8], value)
	buf.Write(tmp[:8])
}

// copyBufferBytes copies pooled buffer contents before the buffer is returned.
func copyBufferBytes(buf *bytes.Buffer) []byte {
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	return result
}

// decodeNameRecord deserializes a name record.
// Returns an error if the data is corrupt or truncated.
// Supports both version 2 (without NameNewHeight) and version 3 (with NameNewHeight)
// for backward compatibility during upgrades.
func decodeNameRecord(data []byte) (*NameRecord, error) {
	version, err := validateRecordVersion(data)
	if err != nil {
		return nil, err
	}
	record := &NameRecord{}
	r := &recordReader{data: data, offset: 1}
	if err := decodeNameRecordCore(r, record); err != nil {
		return nil, err
	}
	record.UpdatedAt = r.readOptionalTimestamp()
	if record.NameNewHeight, err = r.readNameNewHeight(version); err != nil {
		return nil, err
	}
	return record, nil
}

// validateRecordVersion validates the serialized name record header and returns its version.
func validateRecordVersion(data []byte) (byte, error) {
	if len(data) < 1 {
		return 0, fmt.Errorf("corrupt record: empty data")
	}
	version := data[0]
	if version != 2 && version != 3 {
		return 0, fmt.Errorf("unsupported record version: %d (expected 2 or 3)", version)
	}
	return version, nil
}

// decodeNameRecordCore decodes the mandatory fields shared by all name record versions.
func decodeNameRecordCore(r *recordReader, record *NameRecord) error {
	var err error
	if record.Value, err = r.readString("value"); err != nil {
		return err
	}
	if err := r.readFixedBytes(record.TxHash[:], 32, "txhash"); err != nil {
		return err
	}
	if record.OutIndex, err = r.readUint32("outindex"); err != nil {
		return err
	}
	if record.Height, err = r.readInt32("height"); err != nil {
		return err
	}
	if record.ExpiresAt, err = r.readInt32("expires_at"); err != nil {
		return err
	}
	record.Address, err = r.readString("address")
	return err
}

// recordReader provides sequential reading from a byte slice with bounds checking.
type recordReader struct {
	data   []byte
	offset int
}

// readInt32 reads an int32 field encoded as little-endian uint32.
func (r *recordReader) readInt32(field string) (int32, error) {
	value, err := r.readUint32(field)
	if err != nil {
		return 0, err
	}
	return int32(value), nil
}

// readOptionalTimestamp reads an optional unix timestamp if present.
func (r *recordReader) readOptionalTimestamp() time.Time {
	if r.offset+8 > len(r.data) {
		return time.Time{}
	}
	ts := binary.LittleEndian.Uint64(r.data[r.offset : r.offset+8])
	r.offset += 8
	return time.Unix(int64(ts), 0)
}

// readNameNewHeight reads the version-3 NameNewHeight field when present.
func (r *recordReader) readNameNewHeight(version byte) (int32, error) {
	if version < 3 {
		return 0, nil
	}
	if r.offset+4 > len(r.data) {
		return 0, fmt.Errorf("corrupt record: version 3 requires NameNewHeight but data is truncated")
	}
	value := int32(binary.LittleEndian.Uint32(r.data[r.offset : r.offset+4]))
	r.offset += 4
	return value, nil
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
	if strLen > maxRecordStringLen {
		return "", fmt.Errorf("corrupt record: %s length %d exceeds maximum %d", field, strLen, maxRecordStringLen)
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
