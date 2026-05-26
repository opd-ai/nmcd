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

			// Add to address index
			// Key: address + txhash + outindex
			addrKey := make([]byte, len(utxo.Address)+txHashSize+4)
			copy(addrKey, []byte(utxo.Address))
			copy(addrKey[len(utxo.Address):], utxo.TxHash[:])
			binary.BigEndian.PutUint32(addrKey[len(utxo.Address)+txHashSize:], utxo.OutIndex)

			return addrBkt.Put(addrKey, []byte{1}) // Value doesn't matter, just presence
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
			addrKey := make([]byte, len(utxo.Address)+txHashSize+4)
			copy(addrKey, []byte(utxo.Address))
			copy(addrKey[len(utxo.Address):], txHash[:])
			binary.BigEndian.PutUint32(addrKey[len(utxo.Address)+txHashSize:], outIndex)
			if err := addrBkt.Delete(addrKey); err != nil {
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

// GetUTXOsForAddress retrieves all UTXOs for a specific address
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

		// Seek to the address prefix
		prefix := []byte(address)
		c := addrBkt.Cursor()

		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			// Verify key is exactly address + txhash + outindex (no partial matches)
			expectedKeyLen := len(address) + txHashSize + 4
			if len(k) != expectedKeyLen {
				continue
			}

			// Extract txhash and outindex from the key
			var txHash chainhash.Hash
			copy(txHash[:], k[len(address):len(address)+txHashSize])
			outIndex := binary.BigEndian.Uint32(k[len(address)+txHashSize:])

			// Get the full UTXO data
			utxoKey := makeUTXOKey(&txHash, outIndex)
			data := utxoBkt.Get(utxoKey)
			if data == nil {
				continue // Inconsistency - skip
			}

			utxo, err := decodeUTXO(&txHash, outIndex, data)
			if err != nil {
				continue // Skip corrupted entries
			}

			utxos = append(utxos, utxo)
		}

		return nil
	})

	return utxos, err
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

		// Add to height index for efficient lookup and cleanup
		// Key: height(4) + txhash(32) + outindex(4)
		heightKey := make([]byte, 4+32+4)
		binary.BigEndian.PutUint32(heightKey[0:4], uint32(spentAtHeight))
		copy(heightKey[4:36], utxo.TxHash[:])
		binary.BigEndian.PutUint32(heightKey[36:40], utxo.OutIndex)

		return idxBkt.Put(heightKey, []byte{1}) // Value doesn't matter, just presence
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

		// Seek to height prefix in index
		heightPrefix := make([]byte, 4)
		binary.BigEndian.PutUint32(heightPrefix, uint32(height))

		c := idxBkt.Cursor()
		var keysToDelete [][]byte

		for k, _ := c.Seek(heightPrefix); k != nil && bytes.HasPrefix(k, heightPrefix); k, _ = c.Next() {
			// Extract txhash and outindex from the index key
			if len(k) < 4+32+4 {
				continue
			}

			var txHash chainhash.Hash
			copy(txHash[:], k[4:36])
			outIndex := binary.BigEndian.Uint32(k[36:40])

			// Get the spent UTXO data
			utxoKey := makeUTXOKey(&txHash, outIndex)
			data := spentBkt.Get(utxoKey)
			if data == nil {
				// Inconsistency - index exists but data doesn't
				keysToDelete = append(keysToDelete, append([]byte(nil), k...))
				continue
			}

			// Decode UTXO
			utxo, err := decodeUTXO(&txHash, outIndex, data)
			if err != nil {
				// Skip corrupted entries
				keysToDelete = append(keysToDelete, append([]byte(nil), k...))
				continue
			}

			// Restore to active UTXO set
			utxoData, err := encodeUTXO(utxo)
			if err != nil {
				// Skip entries that cannot be re-encoded and mark them for cleanup
				keysToDelete = append(keysToDelete, append([]byte(nil), k...))
				continue
			}

			// Add back to main UTXO bucket
			if err := utxoBkt.Put(utxoKey, utxoData); err != nil {
				return fmt.Errorf("failed to restore UTXO %s:%d: %w", txHash, outIndex, err)
			}

			// Add back to address index
			addrKey := make([]byte, len(utxo.Address)+txHashSize+4)
			copy(addrKey, []byte(utxo.Address))
			copy(addrKey[len(utxo.Address):], utxo.TxHash[:])
			binary.BigEndian.PutUint32(addrKey[len(utxo.Address)+txHashSize:], utxo.OutIndex)
			if err := addrBkt.Put(addrKey, []byte{1}); err != nil {
				return fmt.Errorf("failed to restore UTXO address index: %w", err)
			}

			// Mark for deletion from spent bucket
			keysToDelete = append(keysToDelete, append([]byte(nil), k...))
		}

		// Clean up spent UTXO records
		for _, k := range keysToDelete {
			// Extract txhash and outindex to delete from spent bucket
			if len(k) >= 4+32+4 {
				var txHash chainhash.Hash
				copy(txHash[:], k[4:36])
				outIndex := binary.BigEndian.Uint32(k[36:40])
				utxoKey := makeUTXOKey(&txHash, outIndex)
				if err := spentBkt.Delete(utxoKey); err != nil {
					return fmt.Errorf("failed to delete spent UTXO %s:%d: %w", txHash, outIndex, err)
				}
			}
			if err := idxBkt.Delete(k); err != nil {
				return fmt.Errorf("failed to delete spent UTXO index entry: %w", err)
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

		c := idxBkt.Cursor()
		var keysToDelete [][]byte

		// Iterate through all entries
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			if len(k) < 4 {
				continue
			}

			// Extract height from key
			height := int32(binary.BigEndian.Uint32(k[0:4]))

			// If height is older than keepFromHeight, mark for deletion
			if height < keepFromHeight {
				keysToDelete = append(keysToDelete, append([]byte(nil), k...))
			}
		}

		// Delete old entries
		for _, k := range keysToDelete {
			// Extract txhash and outindex to delete from spent bucket
			if len(k) >= 4+32+4 {
				var txHash chainhash.Hash
				copy(txHash[:], k[4:36])
				outIndex := binary.BigEndian.Uint32(k[36:40])
				utxoKey := makeUTXOKey(&txHash, outIndex)
				if err := spentBkt.Delete(utxoKey); err != nil {
					return fmt.Errorf("failed to delete old spent UTXO %s:%d: %w", txHash, outIndex, err)
				}
			}
			if err := idxBkt.Delete(k); err != nil {
				return fmt.Errorf("failed to delete old spent UTXO index entry: %w", err)
			}
		}

		return nil
	})
}
