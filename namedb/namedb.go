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
	namesBucket      = []byte("names")
	historyBucket    = []byte("history")
	expirationBucket = []byte("expiration")
)

// NameOperation represents a name operation type
type NameOperation uint8

const (
	NameNew NameOperation = iota
	NameFirstUpdate
	NameUpdate
)

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
		for _, bucket := range [][]byte{namesBucket, historyBucket, expirationBucket} {
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
		record = decodeNameRecord(data)
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

// GetExpiredNames returns names that have expired at the given height
func (ndb *NameDatabase) GetExpiredNames(height int32) ([]string, error) {
	ndb.mu.RLock()
	defer ndb.mu.RUnlock()

	var expired []string
	err := ndb.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(namesBucket)
		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			record := decodeNameRecord(v)
			if record.ExpiresAt <= height {
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
			record := decodeNameRecord(v)
			record.Name = string(k)
			names = append(names, record)
		}
		return nil
	})
	return names, err
}

// AddHistory adds a historical name operation
func (ndb *NameDatabase) AddHistory(txHash chainhash.Hash, record *NameRecord) error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(historyBucket)
		data := encodeNameRecord(record)
		return bucket.Put(txHash[:], data)
	})
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

// decodeNameRecord deserializes a name record
func decodeNameRecord(data []byte) *NameRecord {
	if len(data) < 1 {
		return &NameRecord{}
	}

	offset := 0

	// Check version byte
	version := data[offset]
	offset++

	// Handle legacy format (no version byte) - check if first byte looks like a length
	if version > 1 {
		// Likely legacy format without version byte, rewind
		offset = 0
		version = 0
	}

	record := &NameRecord{}

	// Value
	if offset+4 > len(data) {
		return &NameRecord{}
	}
	valLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4
	if offset+int(valLen) > len(data) {
		return &NameRecord{}
	}
	record.Value = string(data[offset : offset+int(valLen)])
	offset += int(valLen)

	// TxHash
	if offset+32 > len(data) {
		return &NameRecord{}
	}
	copy(record.TxHash[:], data[offset:offset+32])
	offset += 32

	// Height
	if offset+4 > len(data) {
		return &NameRecord{}
	}
	record.Height = int32(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4

	// ExpiresAt
	if offset+4 > len(data) {
		return &NameRecord{}
	}
	record.ExpiresAt = int32(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4

	// Address
	if offset+4 > len(data) {
		return &NameRecord{}
	}
	addrLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4
	if offset+int(addrLen) > len(data) {
		return &NameRecord{}
	}
	record.Address = string(data[offset : offset+int(addrLen)])
	offset += int(addrLen)

	// Timestamp
	if offset+8 <= len(data) {
		ts := binary.LittleEndian.Uint64(data[offset : offset+8])
		record.UpdatedAt = time.Unix(int64(ts), 0)
	}

	return record
}
