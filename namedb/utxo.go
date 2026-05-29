package namedb

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"go.etcd.io/bbolt"
)

// makeUTXOKey creates a key for the UTXO bucket from txhash and output index
func makeUTXOKey(txHash *chainhash.Hash, outIndex uint32) []byte {
	key := make([]byte, txHashSize+4)
	copy(key[:txHashSize], txHash[:])
	binary.BigEndian.PutUint32(key[txHashSize:], outIndex)
	return key
}

// makeUTXOAddressKey creates the address index key for a UTXO.
func makeUTXOAddressKey(address string, txHash *chainhash.Hash, outIndex uint32) []byte {
	key := make([]byte, len(address)+txHashSize+4)
	copy(key, address)
	copy(key[len(address):], txHash[:])
	binary.BigEndian.PutUint32(key[len(address)+txHashSize:], outIndex)
	return key
}

// makeSpentUTXOHeightKey creates the height index key for a spent UTXO.
func makeSpentUTXOHeightKey(height int32, txHash *chainhash.Hash, outIndex uint32) []byte {
	key := make([]byte, 4+txHashSize+4)
	binary.BigEndian.PutUint32(key[:4], uint32(height))
	copy(key[4:4+txHashSize], txHash[:])
	binary.BigEndian.PutUint32(key[4+txHashSize:], outIndex)
	return key
}

// parseSpentUTXOHeightKey decodes a spent-UTXO height index key.
func parseSpentUTXOHeightKey(key []byte) (chainhash.Hash, uint32, bool) {
	if len(key) < 4+txHashSize+4 {
		return chainhash.Hash{}, 0, false
	}
	var txHash chainhash.Hash
	copy(txHash[:], key[4:4+txHashSize])
	return txHash, binary.BigEndian.Uint32(key[4+txHashSize:]), true
}

// collectSpentUTXOKeysForHeight gathers spent-UTXO index keys for a single block height.
func collectSpentUTXOKeysForHeight(idxBkt *bbolt.Bucket, height int32) [][]byte {
	heightPrefix := make([]byte, 4)
	binary.BigEndian.PutUint32(heightPrefix, uint32(height))

	var keys [][]byte
	c := idxBkt.Cursor()
	for k, _ := c.Seek(heightPrefix); k != nil && bytes.HasPrefix(k, heightPrefix); k, _ = c.Next() {
		keys = append(keys, copyBytes(k))
	}
	return keys
}

// collectSpentUTXOKeysBeforeHeight gathers spent-UTXO index keys older than keepFromHeight.
func collectSpentUTXOKeysBeforeHeight(idxBkt *bbolt.Bucket, keepFromHeight int32) [][]byte {
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

// restoreSpentUTXOEntry restores one spent UTXO back into the active UTXO buckets.
func restoreSpentUTXOEntry(indexKey []byte, spentBkt, utxoBkt, addrBkt *bbolt.Bucket) error {
	txHash, outIndex, ok := parseSpentUTXOHeightKey(indexKey)
	if !ok {
		return nil
	}
	utxoKey := makeUTXOKey(&txHash, outIndex)
	data := spentBkt.Get(utxoKey)
	if data == nil {
		return nil
	}
	utxo, err := decodeUTXO(&txHash, outIndex, data)
	if err != nil {
		return nil
	}
	return putRestoredUTXO(utxoKey, utxo, utxoBkt, addrBkt)
}

// putRestoredUTXO writes a restored UTXO back to the active set and address index.
func putRestoredUTXO(utxoKey []byte, utxo *UTXO, utxoBkt, addrBkt *bbolt.Bucket) error {
	utxoData, err := encodeUTXO(utxo)
	if err != nil {
		return nil
	}
	if err := utxoBkt.Put(utxoKey, utxoData); err != nil {
		return fmt.Errorf("failed to restore UTXO %s:%d: %w", utxo.TxHash, utxo.OutIndex, err)
	}
	if err := addrBkt.Put(makeUTXOAddressKey(utxo.Address, &utxo.TxHash, utxo.OutIndex), []byte{1}); err != nil {
		return fmt.Errorf("failed to restore UTXO address index: %w", err)
	}
	return nil
}

// deleteSpentUTXOBackup removes spent-UTXO backup data and index state for one entry.
func deleteSpentUTXOBackup(indexKey []byte, spentBkt, idxBkt *bbolt.Bucket, message string) error {
	txHash, outIndex, ok := parseSpentUTXOHeightKey(indexKey)
	if ok {
		if err := spentBkt.Delete(makeUTXOKey(&txHash, outIndex)); err != nil {
			return fmt.Errorf(message, txHash, outIndex, err)
		}
	}
	if err := idxBkt.Delete(indexKey); err != nil {
		return fmt.Errorf("failed to delete spent UTXO index entry: %w", err)
	}
	return nil
}

// encodeUTXO encodes a UTXO for storage
func encodeUTXO(utxo *UTXO) ([]byte, error) {
	// Format: value(8) + height(4) + address_len(1) + address + script_len(2) + script
	addrBytes := []byte(utxo.Address)
	if len(addrBytes) > 255 {
		return nil, fmt.Errorf("address too long: %d bytes", len(addrBytes))
	}
	if len(utxo.PkScript) > 65535 {
		return nil, fmt.Errorf("script too long: %d bytes", len(utxo.PkScript))
	}

	buf := make([]byte, 8+4+1+len(addrBytes)+2+len(utxo.PkScript))
	binary.BigEndian.PutUint64(buf[0:8], uint64(utxo.Value))
	binary.BigEndian.PutUint32(buf[8:12], uint32(utxo.Height))
	buf[12] = byte(len(addrBytes))
	copy(buf[13:], addrBytes)
	offset := 13 + len(addrBytes)
	binary.BigEndian.PutUint16(buf[offset:offset+2], uint16(len(utxo.PkScript)))
	copy(buf[offset+2:], utxo.PkScript)
	return buf, nil
}

// decodeUTXO decodes a UTXO from storage
func decodeUTXO(txHash *chainhash.Hash, outIndex uint32, data []byte) (*UTXO, error) {
	if len(data) < 8+4+1 {
		return nil, fmt.Errorf("invalid UTXO data: too short")
	}

	utxo := &UTXO{
		TxHash:   *txHash,
		OutIndex: outIndex,
		Value:    int64(binary.BigEndian.Uint64(data[0:8])),
		Height:   int32(binary.BigEndian.Uint32(data[8:12])),
	}

	addrLen := int(data[12])
	if len(data) < 13+addrLen+2 {
		return nil, fmt.Errorf("invalid UTXO data: address truncated")
	}
	utxo.Address = string(data[13 : 13+addrLen])

	offset := 13 + addrLen
	scriptLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	if len(data) < offset+2+scriptLen {
		return nil, fmt.Errorf("invalid UTXO data: script truncated")
	}
	utxo.PkScript = make([]byte, scriptLen)
	copy(utxo.PkScript, data[offset+2:offset+2+scriptLen])

	return utxo, nil
}

// AddUTXO adds an unspent transaction output to the database
func (ndb *NameDatabase) AddUTXO(utxo *UTXO) error {
	if utxo == nil {
		return fmt.Errorf("utxo cannot be nil")
	}

	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		return withBuckets(tx, [][]byte{utxoBucket, utxoAddrBucket}, func(bkts []*bbolt.Bucket) error {
			utxoBkt, addrBkt := bkts[0], bkts[1]

			// Encode UTXO
			data, err := encodeUTXO(utxo)
			if err != nil {
				return err
			}

			// Store in main UTXO bucket
			key := makeUTXOKey(&utxo.TxHash, utxo.OutIndex)
			if err := utxoBkt.Put(key, data); err != nil {
				return err
			}

			// Add to address index.
			return addrBkt.Put(makeUTXOAddressKey(utxo.Address, &utxo.TxHash, utxo.OutIndex), []byte{1})
		})
	})
}

// RemoveUTXO removes a spent transaction output from the database
func (ndb *NameDatabase) RemoveUTXO(txHash *chainhash.Hash, outIndex uint32) error {
	if txHash == nil {
		return fmt.Errorf("txHash cannot be nil")
	}

	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		return withBuckets(tx, [][]byte{utxoBucket, utxoAddrBucket}, func(bkts []*bbolt.Bucket) error {
			utxoBkt, addrBkt := bkts[0], bkts[1]

			key := makeUTXOKey(txHash, outIndex)

			// Get the UTXO to extract the address
			data := utxoBkt.Get(key)
			if data == nil {
				// UTXO not found - may have been already spent
				return nil
			}

			utxo, err := decodeUTXO(txHash, outIndex, data)
			if err != nil {
				return err
			}

			// Remove from address index
			if err := addrBkt.Delete(makeUTXOAddressKey(utxo.Address, txHash, outIndex)); err != nil {
				return err
			}

			// Remove from main UTXO bucket
			return utxoBkt.Delete(key)
		})
	})
}

// GetUTXO retrieves a specific UTXO
func (ndb *NameDatabase) GetUTXO(txHash *chainhash.Hash, outIndex uint32) (*UTXO, error) {
	if txHash == nil {
		return nil, fmt.Errorf("txHash cannot be nil")
	}

	ndb.mu.RLock()
	defer ndb.mu.RUnlock()

	var utxo *UTXO
	err := ndb.db.View(func(tx *bbolt.Tx) error {
		return withBucket(tx, utxoBucket, func(utxoBkt *bbolt.Bucket) error {
			key := makeUTXOKey(txHash, outIndex)
			data := utxoBkt.Get(key)
			if data == nil {
				return fmt.Errorf("UTXO not found: %s:%d", txHash, outIndex)
			}
			var err error
			utxo, err = decodeUTXO(txHash, outIndex, data)
			return err
		})
	})

	return utxo, err
}

// GetUTXOsForAddress retrieves all UTXOs for a specific address.
//
// The index key is address || txhash || outindex; the scan uses a bbolt
// prefix cursor on the address bytes.  An exact-length check on every key
// prevents false matches when one address is a byte-prefix of another.
// This works correctly for fixed-length address encodings (e.g. P2PKH) where
// all keys for a given address type have the same length.  If non-P2PKH
// outputs with different-length address strings are ever stored, they will
// be silently skipped — the index key schema would need a separator byte or
// length-prefix to handle variable-length addresses safely.
//
// Extension note: to support SegWit bech32 or other variable-length address
// formats, prepend a 1-byte address-length prefix to the key and update both
// AddUTXO and the Cursor scan in this function accordingly.
func (ndb *NameDatabase) GetUTXOsForAddress(address string) ([]*UTXO, error) {
	if address == "" {
		return nil, fmt.Errorf("address cannot be empty")
	}

	ndb.mu.RLock()
	defer ndb.mu.RUnlock()

	var utxos []*UTXO
	err := ndb.db.View(func(tx *bbolt.Tx) error {
		bkts, err := requireBuckets(tx, utxoBucket, utxoAddrBucket)
		if err != nil {
			return err
		}
		utxoBkt, addrBkt := bkts[0], bkts[1]
		prefix := []byte(address)
		c := addrBkt.Cursor()
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			utxo, ok := loadAddressIndexedUTXO(address, k, utxoBkt)
			if ok {
				utxos = append(utxos, utxo)
			}
		}
		return nil
	})
	return utxos, err
}

// loadAddressIndexedUTXO loads a UTXO referenced by an address index key.
func loadAddressIndexedUTXO(address string, indexKey []byte, utxoBkt *bbolt.Bucket) (*UTXO, bool) {
	txHash, outIndex, ok := parseAddressIndexKey(address, indexKey)
	if !ok {
		return nil, false
	}
	data := utxoBkt.Get(makeUTXOKey(&txHash, outIndex))
	if data == nil {
		return nil, false
	}
	utxo, err := decodeUTXO(&txHash, outIndex, data)
	return utxo, err == nil
}

// parseAddressIndexKey extracts the tx hash and output index from an address index key.
func parseAddressIndexKey(address string, key []byte) (chainhash.Hash, uint32, bool) {
	if len(key) != len(address)+txHashSize+4 {
		return chainhash.Hash{}, 0, false
	}
	var txHash chainhash.Hash
	copy(txHash[:], key[len(address):len(address)+txHashSize])
	return txHash, binary.BigEndian.Uint32(key[len(address)+txHashSize:]), true
}

// GetNameUTXO retrieves the UTXO that holds a specific name
// This is the output from the last NAME_FIRSTUPDATE or NAME_UPDATE for this name
func (ndb *NameDatabase) GetNameUTXO(name string) (*UTXO, error) {
	// First get the name record to find the transaction
	record, err := ndb.GetName(name)
	if err != nil {
		return nil, fmt.Errorf("name not found: %w", err)
	}

	// Get the name UTXO using the correct output index from the record
	// NAME_FIRSTUPDATE and NAME_UPDATE put the name in output specified by OutIndex
	utxo, err := ndb.GetUTXO(&record.TxHash, record.OutIndex)
	if err != nil {
		return nil, fmt.Errorf("name UTXO not found for %s: %w", name, err)
	}

	return utxo, nil
}

// StoreSpentUTXO stores a spent UTXO for potential restoration during reorganization.
// The UTXO is indexed by the block height where it was spent, allowing efficient
// cleanup and restoration. This should be called before RemoveUTXO during block connection.
func (ndb *NameDatabase) StoreSpentUTXO(utxo *UTXO, spentAtHeight int32) error {
	if utxo == nil {
		return fmt.Errorf("utxo cannot be nil")
	}

	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		bkts, err := requireBuckets(tx, spentUtxoBucket, spentUtxoIdxBucket)
		if err != nil {
			return err
		}
		spentBkt, idxBkt := bkts[0], bkts[1]

		// Encode UTXO
		data, err := encodeUTXO(utxo)
		if err != nil {
			return err
		}

		// Store in spent UTXO bucket
		// Key: txhash(32) + outindex(4)
		key := makeUTXOKey(&utxo.TxHash, utxo.OutIndex)
		if err := spentBkt.Put(key, data); err != nil {
			return err
		}

		// Add to height index for efficient lookup and cleanup.
		return idxBkt.Put(makeSpentUTXOHeightKey(spentAtHeight, &utxo.TxHash, utxo.OutIndex), []byte{1})
	})
}

// RestoreSpentUTXOsForBlock restores all UTXOs that were spent in the given block.
// This is called during block disconnection (reorg) to restore the UTXO set to its
// previous state. After restoration, the spent UTXO records are cleaned up.
func (ndb *NameDatabase) RestoreSpentUTXOsForBlock(height int32) error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		bkts, err := requireBuckets(tx, spentUtxoBucket, spentUtxoIdxBucket, utxoBucket, utxoAddrBucket)
		if err != nil {
			return err
		}
		spentBkt, idxBkt := bkts[0], bkts[1]
		utxoBkt, addrBkt := bkts[2], bkts[3]

		for _, indexKey := range collectSpentUTXOKeysForHeight(idxBkt, height) {
			if err := restoreSpentUTXOEntry(indexKey, spentBkt, utxoBkt, addrBkt); err != nil {
				return err
			}
			if err := deleteSpentUTXOBackup(indexKey, spentBkt, idxBkt, "failed to delete spent UTXO %s:%d: %w"); err != nil {
				return err
			}
		}
		return nil
	})
}

// CleanupOldSpentUTXOs removes spent UTXO records older than the given height.
// This is used to prevent unbounded growth of the spent UTXO bucket. Typically,
// only recent spent UTXOs need to be kept (e.g., last 1000 blocks worth of reorgs).
func (ndb *NameDatabase) CleanupOldSpentUTXOs(keepFromHeight int32) error {
	ndb.mu.Lock()
	defer ndb.mu.Unlock()

	return ndb.db.Update(func(tx *bbolt.Tx) error {
		bkts, err := requireBuckets(tx, spentUtxoBucket, spentUtxoIdxBucket)
		if err != nil {
			return err
		}
		spentBkt, idxBkt := bkts[0], bkts[1]

		for _, indexKey := range collectSpentUTXOKeysBeforeHeight(idxBkt, keepFromHeight) {
			if err := deleteSpentUTXOBackup(indexKey, spentBkt, idxBkt, "failed to delete old spent UTXO %s:%d: %w"); err != nil {
				return err
			}
		}
		return nil
	})
}
