package namedb

import (
	"encoding/binary"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"go.etcd.io/bbolt"
)

// BatchWriter provides batched database write operations to reduce fsync overhead.
// It accumulates write operations and commits them in a single transaction.
type BatchWriter struct {
	ndb          *NameDatabase
	names        map[string]*NameRecord         // Names to put
	deletedNames map[string]struct{}            // Names to delete
	history      map[chainhash.Hash]*NameRecord // History entries to add
	nameNews     map[string]nameNewEntry        // NAME_NEW commitments to add
	utxos        []*UTXO                        // UTXOs to add
	deletedUTXOs []utxoKey                      // UTXOs to delete
	batchSize    int                            // Current batch size
	maxBatchSize int                            // Maximum batch size before auto-commit
}

// nameNewEntry represents a NAME_NEW commitment to be written
type nameNewEntry struct {
	commitHash []byte
	height     int32
}

// utxoKey represents a UTXO identifier for deletion
type utxoKey struct {
	txHash   chainhash.Hash
	outIndex uint32
}

// NewBatchWriter creates a new batch writer.
// maxBatchSize: maximum number of operations before auto-commit (0 = no auto-commit)
func (ndb *NameDatabase) NewBatchWriter(maxBatchSize int) *BatchWriter {
	if maxBatchSize < 0 {
		maxBatchSize = 100 // Default as per PLAN.md (commit every 100 operations)
	}
	return &BatchWriter{
		ndb:          ndb,
		names:        make(map[string]*NameRecord),
		deletedNames: make(map[string]struct{}),
		history:      make(map[chainhash.Hash]*NameRecord),
		nameNews:     make(map[string]nameNewEntry),
		maxBatchSize: maxBatchSize,
	}
}

// PutName adds a name write operation to the batch.
// Stores a copy of the record to prevent external mutation.
// Returns error if auto-commit fails.
func (bw *BatchWriter) PutName(name string, record *NameRecord) error {
	// Copy the record to prevent external mutation
	bw.names[name] = record.Copy()
	delete(bw.deletedNames, name) // Remove from delete set if exists
	bw.batchSize++
	return bw.autoCommitIfNeeded()
}

// DeleteName adds a name delete operation to the batch.
// Returns error if auto-commit fails.
func (bw *BatchWriter) DeleteName(name string) error {
	bw.deletedNames[name] = struct{}{}
	delete(bw.names, name) // Remove from put set if exists
	bw.batchSize++
	return bw.autoCommitIfNeeded()
}

// AddHistory adds a history entry write operation to the batch.
// Stores a copy of the record to prevent external mutation.
// Returns error if auto-commit fails.
func (bw *BatchWriter) AddHistory(txHash chainhash.Hash, record *NameRecord) error {
	// Copy the record to prevent external mutation
	bw.history[txHash] = record.Copy()
	bw.batchSize++
	return bw.autoCommitIfNeeded()
}

// PutNameNew adds a NAME_NEW commitment write operation to the batch.
// Returns error if auto-commit fails.
func (bw *BatchWriter) PutNameNew(commitHash []byte, height int32) error {
	commitHashCopy := append([]byte(nil), commitHash...)
	key := string(commitHashCopy)
	bw.nameNews[key] = nameNewEntry{commitHash: commitHashCopy, height: height}
	bw.batchSize++
	return bw.autoCommitIfNeeded()
}

// AddUTXO adds a UTXO write operation to the batch.
// Stores a copy of the UTXO to prevent external mutation.
// Returns error if auto-commit fails.
func (bw *BatchWriter) AddUTXO(utxo *UTXO) error {
	// Copy the UTXO to prevent external mutation
	bw.utxos = append(bw.utxos, utxo.Copy())
	bw.batchSize++
	return bw.autoCommitIfNeeded()
}

// RemoveUTXO adds a UTXO delete operation to the batch.
// Returns error if auto-commit fails.
func (bw *BatchWriter) RemoveUTXO(txHash *chainhash.Hash, outIndex uint32) error {
	bw.deletedUTXOs = append(bw.deletedUTXOs, utxoKey{txHash: *txHash, outIndex: outIndex})
	bw.batchSize++
	return bw.autoCommitIfNeeded()
}

// autoCommitIfNeeded commits the batch if it reaches maxBatchSize.
func (bw *BatchWriter) autoCommitIfNeeded() error {
	if bw.maxBatchSize > 0 && bw.batchSize >= bw.maxBatchSize {
		return bw.Commit()
	}
	return nil
}

// Commit writes all batched operations to the database in a single transaction.
// This significantly reduces fsync overhead compared to individual writes.
func (bw *BatchWriter) Commit() error {
	if bw.batchSize == 0 {
		return nil // Nothing to commit
	}

	bw.ndb.mu.Lock()
	defer bw.ndb.mu.Unlock()

	err := bw.ndb.db.Update(func(tx *bbolt.Tx) error {
		if err := bw.writeNames(tx); err != nil {
			return err
		}
		if err := bw.deleteNames(tx); err != nil {
			return err
		}
		if err := bw.writeHistory(tx); err != nil {
			return err
		}
		if err := bw.writeNameNews(tx); err != nil {
			return err
		}
		if err := bw.writeUTXOs(tx); err != nil {
			return err
		}
		if err := bw.deleteUTXOs(tx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("batch commit failed: %w", err)
	}

	bw.updateCache()
	bw.clear()

	return nil
}

// writeNames writes pending name records to the database.
func (bw *BatchWriter) writeNames(tx *bbolt.Tx) error {
	if len(bw.names) == 0 {
		return nil
	}

	bkts, err := requireBuckets(tx, namesBucket, expirationBucket)
	if err != nil {
		return err
	}
	namesBkt, expirationBkt := bkts[0], bkts[1]

	for name, record := range bw.names {
		if err := bw.updateExpirationIndex(namesBkt, expirationBkt, name); err != nil {
			return err
		}

		data := encodeNameRecord(record)
		if err := namesBkt.Put([]byte(name), data); err != nil {
			return fmt.Errorf("failed to put name %s: %w", name, err)
		}

		expirationKey := makeExpirationKey(record.ExpiresAt, name)
		if err := expirationBkt.Put(expirationKey, []byte{1}); err != nil {
			return fmt.Errorf("failed to update expiration index for %s: %w", name, err)
		}
	}
	return nil
}

// updateExpirationIndex removes old expiration index entry if name exists.
func (bw *BatchWriter) updateExpirationIndex(namesBucket, expirationBucket *bbolt.Bucket, name string) error {
	existingData := namesBucket.Get([]byte(name))
	if existingData != nil {
		existingRecord, decodeErr := decodeNameRecord(existingData)
		if decodeErr == nil {
			oldExpirationKey := makeExpirationKey(existingRecord.ExpiresAt, name)
			expirationBucket.Delete(oldExpirationKey)
		}
	}
	return nil
}

// deleteNames removes pending name deletions from the database.
func (bw *BatchWriter) deleteNames(tx *bbolt.Tx) error {
	if len(bw.deletedNames) == 0 {
		return nil
	}

	bkts, err := requireBuckets(tx, namesBucket, expirationBucket)
	if err != nil {
		return err
	}
	namesBkt, expirationBkt := bkts[0], bkts[1]

	for name := range bw.deletedNames {
		if err := bw.removeExpirationIndex(namesBkt, expirationBkt, name); err != nil {
			return err
		}

		if err := namesBkt.Delete([]byte(name)); err != nil {
			return fmt.Errorf("failed to delete name %s: %w", name, err)
		}
	}
	return nil
}

// removeExpirationIndex removes expiration index entry for a name.
func (bw *BatchWriter) removeExpirationIndex(namesBucket, expirationBucket *bbolt.Bucket, name string) error {
	existingData := namesBucket.Get([]byte(name))
	if existingData != nil {
		existingRecord, decodeErr := decodeNameRecord(existingData)
		if decodeErr == nil {
			expirationKey := makeExpirationKey(existingRecord.ExpiresAt, name)
			expirationBucket.Delete(expirationKey)
		}
	}
	return nil
}

// writeHistory writes pending history entries to the database.
func (bw *BatchWriter) writeHistory(tx *bbolt.Tx) error {
	if len(bw.history) == 0 {
		return nil
	}

	bkts, err := requireBuckets(tx, historyBucket, historyIndexBucket)
	if err != nil {
		return err
	}
	histBkt, idxBkt := bkts[0], bkts[1]

	for txHash, record := range bw.history {
		data := encodeNameRecord(record)
		if err := histBkt.Put(txHash[:], data); err != nil {
			return fmt.Errorf("failed to put history: %w", err)
		}

		nameKey := []byte(record.Name)
		existing := idxBkt.Get(nameKey)
		newIndex := append(append([]byte(nil), existing...), txHash[:]...)
		if err := idxBkt.Put(nameKey, newIndex); err != nil {
			return fmt.Errorf("failed to update history index: %w", err)
		}
	}
	return nil
}

// writeNameNews writes pending NAME_NEW commitments to the database.
func (bw *BatchWriter) writeNameNews(tx *bbolt.Tx) error {
	if len(bw.nameNews) == 0 {
		return nil
	}

	bucket, err := requireBucket(tx, nameNewBucket)
	if err != nil {
		return err
	}
	for _, entry := range bw.nameNews {
		data := make([]byte, 4)
		binary.LittleEndian.PutUint32(data, uint32(entry.height))
		if err := bucket.Put(entry.commitHash, data); err != nil {
			return fmt.Errorf("failed to put name_new: %w", err)
		}
	}
	return nil
}

// writeUTXOs writes pending UTXOs to the database.
func (bw *BatchWriter) writeUTXOs(tx *bbolt.Tx) error {
	if len(bw.utxos) == 0 {
		return nil
	}

	bkts, err := requireBuckets(tx, utxoBucket, utxoAddrBucket)
	if err != nil {
		return err
	}
	utxoBkt, addrBkt := bkts[0], bkts[1]

	for _, utxo := range bw.utxos {
		data, err := encodeUTXO(utxo)
		if err != nil {
			return fmt.Errorf("failed to encode utxo: %w", err)
		}

		key := makeUTXOKey(&utxo.TxHash, utxo.OutIndex)
		if err := utxoBkt.Put(key, data); err != nil {
			return fmt.Errorf("failed to put utxo: %w", err)
		}

		if err := bw.addUTXOToAddressIndex(addrBkt, utxo); err != nil {
			return err
		}
	}
	return nil
}

// addUTXOToAddressIndex adds a UTXO to the address index.
func (bw *BatchWriter) addUTXOToAddressIndex(utxoAddrBucket *bbolt.Bucket, utxo *UTXO) error {
	addrKey := make([]byte, len(utxo.Address)+32+4)
	copy(addrKey, []byte(utxo.Address))
	copy(addrKey[len(utxo.Address):], utxo.TxHash[:])
	binary.BigEndian.PutUint32(addrKey[len(utxo.Address)+32:], utxo.OutIndex)
	if err := utxoAddrBucket.Put(addrKey, []byte{1}); err != nil {
		return fmt.Errorf("failed to update utxo address index: %w", err)
	}
	return nil
}

// deleteUTXOs removes pending UTXO deletions from the database.
func (bw *BatchWriter) deleteUTXOs(tx *bbolt.Tx) error {
	if len(bw.deletedUTXOs) == 0 {
		return nil
	}

	bkts, err := requireBuckets(tx, utxoBucket, utxoAddrBucket)
	if err != nil {
		return err
	}
	utxoBkt, addrBkt := bkts[0], bkts[1]

	for _, uk := range bw.deletedUTXOs {
		if err := bw.removeUTXOFromAddressIndex(utxoBkt, addrBkt, uk); err != nil {
			return err
		}

		key := makeUTXOKey(&uk.txHash, uk.outIndex)
		if err := utxoBkt.Delete(key); err != nil {
			return fmt.Errorf("failed to delete utxo: %w", err)
		}
	}
	return nil
}

// removeUTXOFromAddressIndex removes a UTXO from the address index.
func (bw *BatchWriter) removeUTXOFromAddressIndex(utxoBucket, utxoAddrBucket *bbolt.Bucket, uk utxoKey) error {
	key := makeUTXOKey(&uk.txHash, uk.outIndex)
	data := utxoBucket.Get(key)
	if data != nil {
		utxo, err := decodeUTXO(&uk.txHash, uk.outIndex, data)
		if err == nil {
			addrKey := make([]byte, len(utxo.Address)+32+4)
			copy(addrKey, []byte(utxo.Address))
			copy(addrKey[len(utxo.Address):], uk.txHash[:])
			binary.BigEndian.PutUint32(addrKey[len(utxo.Address)+32:], uk.outIndex)
			utxoAddrBucket.Delete(addrKey)
		}
	}
	return nil
}

// updateCache updates the cache with committed changes.
func (bw *BatchWriter) updateCache() {
	for name, record := range bw.names {
		record.Name = name
		bw.ndb.cache.Put(name, record)
	}
	for name := range bw.deletedNames {
		bw.ndb.cache.Delete(name)
	}
}

// clear resets the batch writer state.
func (bw *BatchWriter) clear() {
	bw.names = make(map[string]*NameRecord)
	bw.deletedNames = make(map[string]struct{})
	bw.history = make(map[chainhash.Hash]*NameRecord)
	bw.nameNews = make(map[string]nameNewEntry)
	bw.utxos = nil
	bw.deletedUTXOs = nil
	bw.batchSize = 0
}

// Size returns the current number of operations in the batch.
func (bw *BatchWriter) Size() int {
	return bw.batchSize
}
