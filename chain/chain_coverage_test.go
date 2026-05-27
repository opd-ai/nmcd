package chain

import (
	"bytes"
	"testing"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/config"
)

// TestParseNameOperationsFromTxEmpty verifies an empty tx returns no operations.
func TestParseNameOperationsFromTxEmpty(t *testing.T) {
	tx := wire.NewMsgTx(1)
	ops := ParseNameOperationsFromTx(tx)
	if len(ops) != 0 {
		t.Errorf("expected 0 ops from empty tx, got %d", len(ops))
	}
}

// TestParseNameOperationsFromTxNoNameOp verifies a plain P2PKH output returns no operations.
func TestParseNameOperationsFromTxNoNameOp(t *testing.T) {
	tx := wire.NewMsgTx(1)
	tx.AddTxOut(&wire.TxOut{
		Value:    1000,
		PkScript: []byte{0x76, 0xa9, 0x14, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x88, 0xac},
	})
	ops := ParseNameOperationsFromTx(tx)
	if len(ops) != 0 {
		t.Errorf("expected 0 ops from P2PKH tx, got %d", len(ops))
	}
}

// TestParseNameOperationsFromTxNameUpdate verifies a NAME_UPDATE output is parsed.
func TestParseNameOperationsFromTxNameUpdate(t *testing.T) {
	tx := wire.NewMsgTx(1)
	script := buildNameUpdateScript([]byte("d/test"), []byte(`{"ip":"1.2.3.4"}`))
	tx.AddTxOut(&wire.TxOut{Value: 1000, PkScript: script})
	ops := ParseNameOperationsFromTx(tx)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Name != "d/test" {
		t.Errorf("expected name 'd/test', got %q", ops[0].Name)
	}
}

// TestHandleBlockchainNotificationConnected confirms NTBlockConnected is a no-op.
func TestHandleBlockchainNotificationConnected(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	notif := &blockchain.Notification{
		Type: blockchain.NTBlockConnected,
	}
	// Should not panic
	bc.HandleBlockchainNotification(notif)
}

// TestHandleBlockchainNotificationDisconnectedWrongType confirms non-block data is
// handled safely.
func TestHandleBlockchainNotificationDisconnectedWrongType(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	notif := &blockchain.Notification{
		Type: blockchain.NTBlockDisconnected,
		Data: "not a block",
	}
	bc.HandleBlockchainNotification(notif)
}

// TestHandleBlockchainNotificationDisconnectedBlock exercises the rollback path.
func TestHandleBlockchainNotificationDisconnectedBlock(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	header := wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
		Bits:      0x1d00ffff,
	}
	msgBlock := wire.NewMsgBlock(&header)
	coinbase := wire.NewMsgTx(1)
	coinbase.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 0xffffffff},
		SignatureScript:  []byte("coinbase"),
	})
	coinbase.AddTxOut(&wire.TxOut{Value: 50 * 1e8, PkScript: []byte{0x51}})
	msgBlock.AddTransaction(coinbase)

	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(1)

	notif := &blockchain.Notification{
		Type: blockchain.NTBlockDisconnected,
		Data: block,
	}
	// Should not panic even if rollback finds no data
	bc.HandleBlockchainNotification(notif)
}

// TestValidateBlockSubsidyEmptyBlock verifies an error is returned for a block
// with no transactions.
func TestValidateBlockSubsidyEmptyBlock(t *testing.T) {
	bc := &BlockChain{
		chainParams: &config.NamecoinRegTestParams,
	}

	header := wire.BlockHeader{Version: 1, Timestamp: time.Now(), Bits: 0x1d00ffff}
	msgBlock := wire.NewMsgBlock(&header)
	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(0)

	err := bc.validateBlockSubsidy(block)
	if err == nil {
		t.Fatal("expected error for block with no transactions, got nil")
	}
}

// TestValidateBlockSubsidyHeightUnresolvable verifies that when height cannot be
// determined the function returns nil (cannot validate).
func TestValidateBlockSubsidyHeightUnresolvable(t *testing.T) {
	bc := &BlockChain{
		chainParams: &config.NamecoinRegTestParams,
	}

	header := wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
		Bits:      0x1d00ffff,
		PrevBlock: chainhash.Hash{1}, // non-zero so BlockChain path would be taken (bc.BlockChain is nil)
	}
	msgBlock := wire.NewMsgBlock(&header)
	coinbase := wire.NewMsgTx(1)
	coinbase.AddTxIn(&wire.TxIn{PreviousOutPoint: wire.OutPoint{Index: 0xffffffff}})
	coinbase.AddTxOut(&wire.TxOut{Value: 50 * 1e8, PkScript: []byte{0x51}})
	msgBlock.AddTransaction(coinbase)

	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(-1) // explicitly unknown height

	err := bc.validateBlockSubsidy(block)
	if err != nil {
		t.Errorf("expected nil for unknown height block, got %v", err)
	}
}

// TestValidateBlockVersionHeightUnresolvable verifies that when height cannot be
// determined the function returns nil.
func TestValidateBlockVersionHeightUnresolvable(t *testing.T) {
	bc := &BlockChain{
		chainParams: &config.NamecoinRegTestParams,
	}

	header := wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
		Bits:      0x1d00ffff,
		PrevBlock: chainhash.Hash{1},
	}
	msgBlock := wire.NewMsgBlock(&header)
	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(-1)

	err := bc.validateBlockVersion(block)
	if err != nil {
		t.Errorf("expected nil for unknown height block, got %v", err)
	}
}

// TestResolveBlockHeightZeroPrevHash confirms block.Height() is returned for genesis.
func TestResolveBlockHeightZeroPrevHash(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	header := wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
		Bits:      0x1d00ffff,
		PrevBlock: chainhash.Hash{}, // zero prev hash → genesis path
	}
	msgBlock := wire.NewMsgBlock(&header)
	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(42)

	h := bc.resolveBlockHeight(block)
	if h != 42 {
		t.Errorf("expected height 42, got %d", h)
	}
}

// TestBlockSerializeNoAuxPow covers Block.Serialize / Bytes for a plain block.
func TestBlockSerializeNoAuxPow(t *testing.T) {
	header := wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
		Bits:      0x1d00ffff,
	}
	msgBlock := wire.NewMsgBlock(&header)
	coinbase := wire.NewMsgTx(1)
	coinbase.AddTxIn(&wire.TxIn{PreviousOutPoint: wire.OutPoint{Index: 0xffffffff}})
	coinbase.AddTxOut(&wire.TxOut{Value: 50 * 1e8, PkScript: []byte{0x51}})
	msgBlock.AddTransaction(coinbase)

	block := NewBlock(msgBlock)
	data, err := block.Bytes()
	if err != nil {
		t.Fatalf("Bytes() error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Bytes() returned empty slice")
	}

	// Re-serialize via Serialize into a writer
	var buf bytes.Buffer
	if err := block.Serialize(&buf); err != nil {
		t.Fatalf("Serialize() error: %v", err)
	}
	if !bytes.Equal(data, buf.Bytes()) {
		t.Error("Serialize and Bytes results differ")
	}
}

// TestScanNames verifies ScanNames returns no error on an empty name database.
func TestScanNames(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	records, err := bc.ScanNames("d/", 10)
	if err != nil {
		t.Fatalf("ScanNames unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records on empty DB, got %d", len(records))
	}
}

// TestGetNameUTXO verifies GetNameUTXO returns an error for an unknown name.
func TestGetNameUTXO(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	_, err := bc.GetNameUTXO("d/nonexistent")
	if err == nil {
		t.Error("expected error for unknown name, got nil")
	}
}

// TestGetUTXOsForAddress verifies GetUTXOsForAddress works on an empty database.
func TestGetUTXOsForAddress(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	utxos, err := bc.GetUTXOsForAddress("someaddress")
	if err != nil {
		t.Fatalf("GetUTXOsForAddress unexpected error: %v", err)
	}
	_ = utxos
}

// TestVerifyBlockNoNameOps verifies VerifyBlock succeeds on a coinbase-only block.
func TestVerifyBlockNoNameOps(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	header := wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
		Bits:      0x1d00ffff,
	}
	msgBlock := wire.NewMsgBlock(&header)
	coinbase := wire.NewMsgTx(1)
	coinbase.AddTxIn(&wire.TxIn{PreviousOutPoint: wire.OutPoint{Index: 0xffffffff}})
	coinbase.AddTxOut(&wire.TxOut{Value: 50 * 1e8, PkScript: []byte{0x51}})
	msgBlock.AddTransaction(coinbase)

	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(0)

	if err := bc.VerifyBlock(block); err != nil {
		t.Fatalf("VerifyBlock unexpected error: %v", err)
	}
}

// TestResolveBlockHeightFromChain verifies resolveBlockHeight uses the blockchain
// index when the previous block hash refers to a known block.
func TestResolveBlockHeightFromChain(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// The genesis block is the best block; use its hash as PrevBlock.
	genesisHash := bc.BestSnapshot().Hash

	header := wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
		Bits:      0x1d00ffff,
		PrevBlock: genesisHash,
	}
	msgBlock := wire.NewMsgBlock(&header)
	block := btcutil.NewBlock(msgBlock)
	block.SetHeight(0)

	// genesis is at height 0; chain lookup should return 0+1 = 1.
	h := bc.resolveBlockHeight(block)
	if h != 1 {
		t.Errorf("expected height 1 via chain lookup, got %d", h)
	}
}

// TestValidateNameNewNotFound verifies validateNameNew returns nil when the commitment
// is not already registered (the normal case before NAME_FIRSTUPDATE).
func TestValidateNameNewNotFound(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	err := bc.validateNameNew([]byte("randomcommithash1234"))
	if err != nil {
		t.Errorf("expected nil for unknown commitment, got %v", err)
	}
}

// TestValidateNameNewAlreadyExists verifies validateNameNew returns an error when
// the commitment hash is already registered.
func TestValidateNameNewAlreadyExists(t *testing.T) {
	bc, ndb, cleanup := setupTestBlockChain(t)
	defer cleanup()

	commitHash := []byte("existingcommithash12")
	if err := ndb.PutNameNew(commitHash, 10); err != nil {
		t.Fatalf("PutNameNew: %v", err)
	}

	err := bc.validateNameNew(commitHash)
	if err == nil {
		t.Error("expected error for existing commitment, got nil")
	}
}

// TestGetNameNewHeightFound verifies getNameNewHeight returns the recorded height
// when a NAME_NEW entry is present in the database.
func TestGetNameNewHeightFound(t *testing.T) {
	bc, ndb, cleanup := setupTestBlockChain(t)
	defer cleanup()

	rand := []byte("randombytes12345")
	name := "d/test"
	commitHash := computeCommitHash(rand, name, bc.chainParams)
	if err := ndb.PutNameNew(commitHash, 42); err != nil {
		t.Fatalf("PutNameNew: %v", err)
	}

	h := bc.getNameNewHeight(rand, name, 100)
	if h != 42 {
		t.Errorf("expected height 42, got %d", h)
	}
}

// TestGetNameNewHeightFallback verifies getNameNewHeight uses the estimation fallback
// when no NAME_NEW entry is present in the database.
func TestGetNameNewHeightFallback(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// Call with a rand/name pair that has no corresponding NAME_NEW in the DB.
	h := bc.getNameNewHeight([]byte("nosuchrand"), "d/notfound", 100)
	expected := int32(100 - config.MinBlocksBeforeFirstUpdate)
	if h != expected {
		t.Errorf("expected fallback height %d, got %d", expected, h)
	}
}

// TestGetNameNewHeightFallbackNegative verifies getNameNewHeight clamps the fallback
// to 0 when currentHeight < MinBlocksBeforeFirstUpdate.
func TestGetNameNewHeightFallbackNegative(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	// With currentHeight = 0, fallback would be negative → clamped to 0.
	h := bc.getNameNewHeight([]byte("nosuchrand"), "d/notfound2", 0)
	if h != 0 {
		t.Errorf("expected 0 for clamped negative height, got %d", h)
	}
}

// TestValidateNameOperationUnknown exercises the default (no-op) branch of the switch.
func TestValidateNameOperationUnknown(t *testing.T) {
	bc, _, cleanup := setupTestBlockChain(t)
	defer cleanup()

	tx := wire.NewMsgTx(1)
	// Pass an op value that matches none of the known cases → should return nil.
	err := bc.validateNameOperation(0, "", "", nil, tx, 0)
	if err != nil {
		t.Errorf("expected nil for unknown op, got %v", err)
	}
}
