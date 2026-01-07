package chain

import (
"path/filepath"
"testing"

"github.com/btcsuite/btcd/chaincfg/chainhash"
"github.com/opd-ai/nmcd/config"
"github.com/opd-ai/nmcd/namedb"
)

// setupTestBlockChain creates a test blockchain instance
func setupTestBlockChain(t *testing.T) (*BlockChain, *namedb.NameDatabase, func()) {
tmpDir := t.TempDir()

cfg := &Config{
ChainParams:  &config.NamecoinRegTestParams,
DataDir:      tmpDir,
BlockDBPath:  filepath.Join(tmpDir, "blocks.db"),
NameDBPath:   filepath.Join(tmpDir, "names.db"),
}

bc, err := NewBlockChain(cfg, nil)
if err != nil {
t.Fatalf("Failed to create blockchain: %v", err)
}

cleanup := func() {
bc.Close()
}

return bc, bc.nameDB, cleanup
}

// TestNewBlockChain tests the NewBlockChain constructor
func TestNewBlockChain(t *testing.T) {
bc, _, cleanup := setupTestBlockChain(t)
defer cleanup()

// Verify blockchain is initialized
if bc.BlockChain == nil {
t.Error("Expected non-nil embedded blockchain")
}
if bc.nameDB == nil {
t.Error("Expected non-nil namedb")
}
if bc.chainParams == nil {
t.Error("Expected non-nil chain params")
}

// Verify genesis block is loaded
snapshot := bc.BestSnapshot()
if snapshot.Height != 0 {
t.Errorf("Expected genesis block at height 0, got %d", snapshot.Height)
}
}

// TestBlockChainClose tests the Close method
func TestBlockChainClose(t *testing.T) {
tmpDir := t.TempDir()

cfg := &Config{
ChainParams:  &config.NamecoinRegTestParams,
DataDir:      tmpDir,
BlockDBPath:  filepath.Join(tmpDir, "blocks.db"),
NameDBPath:   filepath.Join(tmpDir, "names.db"),
}

bc, err := NewBlockChain(cfg, nil)
if err != nil {
t.Fatalf("Failed to create blockchain: %v", err)
}

// Test closing blockchain
err = bc.Close()
if err != nil {
t.Fatalf("Failed to close blockchain: %v", err)
}
}

// TestBlockChainBestSnapshot tests the BestSnapshot wrapper method
func TestBlockChainBestSnapshot(t *testing.T) {
bc, _, cleanup := setupTestBlockChain(t)
defer cleanup()

// Test getting best snapshot
snapshot := bc.BestSnapshot()
if snapshot == nil {
t.Error("Expected non-nil snapshot")
}
// Genesis block should be at height 0
if snapshot.Height != 0 {
t.Errorf("Expected genesis block at height 0, got %d", snapshot.Height)
}
}

// TestBlockChainChainParams tests the ChainParams wrapper method
func TestBlockChainChainParams(t *testing.T) {
bc, _, cleanup := setupTestBlockChain(t)
defer cleanup()

// Test getting chain params
params := bc.ChainParams()
if params == nil {
t.Error("Expected non-nil chain params")
}
if params.Name != "regtest" {
t.Errorf("Expected network 'regtest', got '%s'", params.Name)
}
}

// TestBlockChainGetBlockByHash tests the GetBlockByHash wrapper method
func TestBlockChainGetBlockByHash(t *testing.T) {
bc, _, cleanup := setupTestBlockChain(t)
defer cleanup()

// Test getting genesis block
genesisHash := bc.BestSnapshot().Hash
block, err := bc.GetBlockByHash(&genesisHash)
if err != nil {
t.Fatalf("Failed to get genesis block: %v", err)
}
if block == nil {
t.Error("Expected non-nil block")
}

// Test getting non-existent block
nonExistentHash := chainhash.HashH([]byte("nonexistent"))
_, err = bc.GetBlockByHash(&nonExistentHash)
if err == nil {
t.Error("Expected error for non-existent block")
}
}

// TestBlockChainGetBlockHeader tests the GetBlockHeader wrapper method
func TestBlockChainGetBlockHeader(t *testing.T) {
bc, _, cleanup := setupTestBlockChain(t)
defer cleanup()

// Test getting genesis block header
genesisHash := bc.BestSnapshot().Hash
header, err := bc.GetBlockHeader(&genesisHash)
if err != nil {
t.Fatalf("Failed to get genesis header: %v", err)
}
if header.Version == 0 {
t.Error("Expected non-nil header")
}

// Test getting non-existent block header
nonExistentHash := chainhash.HashH([]byte("nonexistent"))
_, err = bc.GetBlockHeader(&nonExistentHash)
if err == nil {
t.Error("Expected error for non-existent block header")
}
}

// TestBlockChainGetNameDB tests the GetNameDB wrapper method
func TestBlockChainGetNameDB(t *testing.T) {
bc, ndb, cleanup := setupTestBlockChain(t)
defer cleanup()

// Test getting namedb
retrievedDB := bc.GetNameDB()
if retrievedDB == nil {
t.Error("Expected non-nil namedb")
}
if retrievedDB != ndb {
t.Error("Expected same namedb instance")
}
}

// TestBlockChainGetName tests the GetName wrapper method
func TestBlockChainGetName(t *testing.T) {
bc, ndb, cleanup := setupTestBlockChain(t)
defer cleanup()

// Test getting non-existent name
_, err := bc.GetName("d/nonexistent")
if err == nil {
t.Error("Expected error for non-existent name")
}

// Add a test name
testName := &namedb.NameRecord{
Name:      "d/test",
Value:     `{"ip":"127.0.0.1"}`,
Height:    100,
ExpiresAt: 36100,
Address:   "N1234567890",
}
if err := ndb.PutName(testName.Name, testName); err != nil {
t.Fatalf("Failed to add test name: %v", err)
}

// Test getting existing name
name, err := bc.GetName("d/test")
if err != nil {
t.Fatalf("Failed to get name: %v", err)
}
if name.Name != "d/test" {
t.Errorf("Expected name 'd/test', got '%s'", name.Name)
}
}

// TestBlockChainListNames tests the ListNames wrapper method
func TestBlockChainListNames(t *testing.T) {
bc, ndb, cleanup := setupTestBlockChain(t)
defer cleanup()

// List names when empty
names, err := bc.ListNames()
if err != nil {
t.Fatalf("Failed to list names: %v", err)
}
if len(names) != 0 {
t.Errorf("Expected 0 names, got %d", len(names))
}

// Add test names
testNames := []*namedb.NameRecord{
{Name: "d/test1", Value: `{"ip":"127.0.0.1"}`, Height: 100, ExpiresAt: 36100, Address: "N1"},
{Name: "d/test2", Value: `{"ip":"127.0.0.2"}`, Height: 101, ExpiresAt: 36101, Address: "N2"},
}
for _, name := range testNames {
if err := ndb.PutName(name.Name, name); err != nil {
t.Fatalf("Failed to add test name: %v", err)
}
}

// List names
names, err = bc.ListNames()
if err != nil {
t.Fatalf("Failed to list names: %v", err)
}
if len(names) != 2 {
t.Errorf("Expected 2 names, got %d", len(names))
}
}

// TestBlockChainGetNameHistory tests the GetNameHistory wrapper method
func TestBlockChainGetNameHistory(t *testing.T) {
bc, ndb, cleanup := setupTestBlockChain(t)
defer cleanup()

// Test getting history for non-existent name
history, err := bc.GetNameHistory("d/nonexistent")
if err != nil {
t.Fatalf("Failed to get history: %v", err)
}
if len(history) != 0 {
t.Errorf("Expected 0 history records, got %d", len(history))
}

// Add a test name with history
testName := &namedb.NameRecord{
Name:      "d/test",
Value:     `{"ip":"127.0.0.1"}`,
Height:    100,
ExpiresAt: 36100,
Address:   "N1",
}
txHash := chainhash.HashH([]byte("test-tx"))
testName.TxHash = txHash

if err := ndb.PutName(testName.Name, testName); err != nil {
t.Fatalf("Failed to add test name: %v", err)
}
if err := ndb.AddHistory(txHash, testName); err != nil {
t.Fatalf("Failed to add history: %v", err)
}

// Test getting history
history, err = bc.GetNameHistory("d/test")
if err != nil {
t.Fatalf("Failed to get history: %v", err)
}
if len(history) != 1 {
t.Errorf("Expected 1 history record, got %d", len(history))
}
}
