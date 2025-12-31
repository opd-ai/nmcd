package namedb

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"go.etcd.io/bbolt"
)

var (
	namesBucket        = []byte("names")
	historyBucket      = []byte("history")
	historyIndexBucket = []byte("history_index")
	expirationBucket   = []byte("expiration")
	nameNewBucket      = []byte("name_new")  // Tracks NAME_NEW commitments
	utxoBucket         = []byte("utxo")      // Tracks unspent transaction outputs
	utxoAddrBucket     = []byte("utxo_addr") // Index: address -> UTXOs
)

// txHashSize is the size of a transaction hash in bytes.
// Bitcoin/Namecoin use double SHA256 (SHA256(SHA256(data))) which produces
// a 32-byte result, same as a single SHA256.
const txHashSize = 32

// NameOperation represents a name operation type
type NameOperation uint8

const (
	NameNew NameOperation = iota
	NameFirstUpdate
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
	Name      string
	Value     string
	TxHash    chainhash.Hash
	Height    int32
	ExpiresAt int32
	Address   string
	UpdatedAt time.Time
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

// NameDatabase manages name operations with bbolt storage
type NameDatabase struct {
	db *bbolt.DB
	mu sync.RWMutex
}

// NewNameDatabase creates a new name database
func NewNameDatabase(dbPath string) (*NameDatabase, error) {
	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize buckets
	err = db.Update(func(tx *bbolt.Tx) error {
		for _, bucket := range [][]byte{namesBucket, historyBucket, historyIndexBucket, expirationBucket, nameNewBucket, utxoBucket, utxoAddrBucket} {
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

	return &NameDatabase{db: db}, nil
}

// Close closes the database
func (ndb *NameDatabase) Close() error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()
	return ndb.db.Close()
}

// PutName stores a name record
func (ndb *NameDatabase) PutName(name string, record *NameRecord) error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(namesBucket)
		data := encodeNameRecord(record)
		return bucket.Put([]byte(name), data)
	})
}

// GetName retrieves a name record
func (ndb *NameDatabase) GetName(name string) (*NameRecord, error) {
	ndb.mu.RLock()
	defer ndb.mu.RUnlock()

	var record *NameRecord
	err := ndb.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(namesBucket)
		data := bucket.Get([]byte(name))
		if data == nil {
			return fmt.Errorf("name not found")
		}
		var decodeErr error
		record, decodeErr = decodeNameRecord(data)
		if decodeErr != nil {
			return fmt.Errorf("failed to decode name %s: %w", name, decodeErr)
		}
		record.Name = name
		return nil
	})
	return record, err
}

// DeleteName removes a name record
func (ndb *NameDatabase) DeleteName(name string) error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(namesBucket)
		return bucket.Delete([]byte(name))
	})
}

// GetExpiredNames returns names that have expired before the given height.
// A name is considered valid through its ExpiresAt block and only expired after.
// For example, a name with ExpiresAt=100 is valid at height 100 but expired at height 101.
func (ndb *NameDatabase) GetExpiredNames(height int32) ([]string, error) {
	ndb.mu.RLock()
	defer ndb.mu.RUnlock()

	var expired []string
	err := ndb.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(namesBucket)
		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			record, decodeErr := decodeNameRecord(v)
			if decodeErr != nil {
				return fmt.Errorf("failed to decode name %s: %w", string(k), decodeErr)
			}
			if record.ExpiresAt < height {
				expired = append(expired, string(k))
			}
		}
		return nil
	})
	return expired, err
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

// AddHistory adds a historical name operation and updates the name-to-history index.
// The history is stored keyed by transaction hash, and an index entry is added to map
// the name to its list of transaction hashes for efficient retrieval.
func (ndb *NameDatabase) AddHistory(txHash chainhash.Hash, record *NameRecord) error {
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
		record = &NameNewRecord{
			Hash:   commitHash,
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
		if indexData == nil || len(indexData) == 0 {
			return nil // No history to remove
		}

		if len(indexData)%txHashSize != 0 {
			return fmt.Errorf("corrupt history index for name: %s", name)
		}

		numEntries := len(indexData) / txHashSize
		if numEntries == 0 {
			return nil
		}

		// Get the last txHash to remove from history bucket
		lastTxHash := indexData[len(indexData)-txHashSize:]
		if err := histBucket.Delete(lastTxHash); err != nil {
			return err
		}

		// Remove the last txHash from the index
		newIndexData := indexData[:len(indexData)-txHashSize]
		if len(newIndexData) == 0 {
			// No more entries, delete the index
			if err := indexBucket.Delete([]byte(name)); err != nil {
				return err
			}
		} else {
			if err := indexBucket.Put([]byte(name), newIndexData); err != nil {
				return err
			}

			// Get the previous record (now the last one)
			prevTxHash := newIndexData[len(newIndexData)-txHashSize:]
			data := histBucket.Get(prevTxHash)
			if data != nil {
				var decodeErr error
				prevRecord, decodeErr = decodeNameRecord(data)
				if decodeErr != nil {
					return fmt.Errorf("failed to decode previous record: %w", decodeErr)
				}
				prevRecord.Name = name
			}
		}
		return nil
	})
	return prevRecord, err
}

// encodeNameRecord serializes a name record
func encodeNameRecord(record *NameRecord) []byte {
	// Version 1 encoding: version byte + value + txhash + height + expiresAt + address + timestamp
	data := make([]byte, 0, 1+len(record.Value)+32+4+4+len(record.Address)+8)

	// Version byte (v1)
	data = append(data, byte(1))

	// Value length + value
	valLen := make([]byte, 4)
	binary.LittleEndian.PutUint32(valLen, uint32(len(record.Value)))
	data = append(data, valLen...)
	data = append(data, []byte(record.Value)...)

	// TxHash
	data = append(data, record.TxHash[:]...)

	// Height
	height := make([]byte, 4)
	binary.LittleEndian.PutUint32(height, uint32(record.Height))
	data = append(data, height...)

	// ExpiresAt
	expires := make([]byte, 4)
	binary.LittleEndian.PutUint32(expires, uint32(record.ExpiresAt))
	data = append(data, expires...)

	// Address length + address
	addrLen := make([]byte, 4)
	binary.LittleEndian.PutUint32(addrLen, uint32(len(record.Address)))
	data = append(data, addrLen...)
	data = append(data, []byte(record.Address)...)

	// Timestamp
	ts := make([]byte, 8)
	binary.LittleEndian.PutUint64(ts, uint64(record.UpdatedAt.Unix()))
	data = append(data, ts...)

	return data
}

// decodeNameRecord deserializes a name record.
// Returns an error if the data is corrupt or truncated.
func decodeNameRecord(data []byte) (*NameRecord, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("corrupt record: empty data")
	}

	offset := 0

	// Check version byte
	version := data[offset]
	offset++

	// Handle legacy format (no version byte) - values > 1 indicate legacy format
	// where the first byte is actually part of the value length, not a version byte
	if version > 1 {
		// Likely legacy format without version byte, rewind
		offset = 0
	}

	record := &NameRecord{}

	// Value
	if offset+4 > len(data) {
		return nil, fmt.Errorf("corrupt record: truncated at value length")
	}
	valLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4
	if offset+int(valLen) > len(data) {
		return nil, fmt.Errorf("corrupt record: truncated at value data")
	}
	record.Value = string(data[offset : offset+int(valLen)])
	offset += int(valLen)

	// TxHash
	if offset+32 > len(data) {
		return nil, fmt.Errorf("corrupt record: truncated at txhash")
	}
	copy(record.TxHash[:], data[offset:offset+32])
	offset += 32

	// Height
	if offset+4 > len(data) {
		return nil, fmt.Errorf("corrupt record: truncated at height")
	}
	record.Height = int32(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4

	// ExpiresAt
	if offset+4 > len(data) {
		return nil, fmt.Errorf("corrupt record: truncated at expires_at")
	}
	record.ExpiresAt = int32(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4

	// Address
	if offset+4 > len(data) {
		return nil, fmt.Errorf("corrupt record: truncated at address length")
	}
	addrLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4
	if offset+int(addrLen) > len(data) {
		return nil, fmt.Errorf("corrupt record: truncated at address data")
	}
	record.Address = string(data[offset : offset+int(addrLen)])
	offset += int(addrLen)

	// Timestamp
	if offset+8 <= len(data) {
		ts := binary.LittleEndian.Uint64(data[offset : offset+8])
		record.UpdatedAt = time.Unix(int64(ts), 0)
	}

	return record, nil
}
