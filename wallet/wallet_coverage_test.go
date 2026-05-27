package wallet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/config"
)

// TestNormalizeWalletVersionZero tests that a version-0 wallet is upgraded to version 1.
func TestNormalizeWalletVersionZero(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a wallet data file with version 0 (legacy format)
	wd := walletData{
		Version:   0,
		Encrypted: true, // This should be reset to false
		Keys:      []keyData{},
	}
	data, err := json.Marshal(wd)
	if err != nil {
		t.Fatalf("failed to marshal wallet data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "wallet.json"), data, 0o600); err != nil {
		t.Fatalf("failed to write wallet file: %v", err)
	}

	// Load the wallet — normalizeWalletVersion should upgrade version to 1.
	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("NewWallet failed: %v", err)
	}

	// The wallet should be loaded successfully
	if w == nil {
		t.Fatal("expected non-nil wallet")
	}
}

// TestSignTransactionInputMismatch tests SignTransaction with mismatched UTXO count.
func TestSignTransactionInputMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewWallet(tmpDir, &config.NamecoinMainNetParams)
	if err != nil {
		t.Fatalf("NewWallet failed: %v", err)
	}

	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(&wire.TxIn{})
	tx.AddTxIn(&wire.TxIn{})

	// Only one UTXO for two inputs — mismatch
	utxos := []UTXO{{Value: 1000}}

	err = w.SignTransaction(tx, utxos)
	if err == nil {
		t.Error("expected error for UTXO count mismatch")
	}
}

// TestSignTransactionEmptyMatch tests SignTransaction with matching empty inputs.
func TestSignTransactionEmptyMatch(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewWallet(tmpDir, &config.NamecoinMainNetParams)
	if err != nil {
		t.Fatalf("NewWallet failed: %v", err)
	}

	tx := wire.NewMsgTx(wire.TxVersion)
	utxos := []UTXO{}

	// 0 inputs, 0 UTXOs — no mismatch, no signing needed
	err = w.SignTransaction(tx, utxos)
	if err != nil {
		t.Errorf("unexpected error for empty tx: %v", err)
	}
}

// TestCreateNameUpdateTxNegativeFeeRate tests CreateNameUpdateTx with negative fee rate.
func TestCreateNameUpdateTxNegativeFeeRate(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewWallet(tmpDir, &config.NamecoinMainNetParams)
	if err != nil {
		t.Fatalf("NewWallet failed: %v", err)
	}

	_, err = w.CreateNameUpdateTx("d/test", "value", []UTXO{{Value: 10000}}, 0, -1, nil)
	if err == nil {
		t.Error("expected error for negative feeRate")
	}
}

// TestCreateNameUpdateTxInvalidIndex tests CreateNameUpdateTx with invalid UTXO index.
func TestCreateNameUpdateTxInvalidIndex(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewWallet(tmpDir, &config.NamecoinMainNetParams)
	if err != nil {
		t.Fatalf("NewWallet failed: %v", err)
	}

	_, err = w.CreateNameUpdateTx("d/test", "value", []UTXO{{Value: 10000}}, 5, 100, nil)
	if err == nil {
		t.Error("expected error for invalid UTXO index")
	}
}

// TestCreateNameUpdateTxNoKey tests CreateNameUpdateTx when wallet has no key for the address.
func TestCreateNameUpdateTxNoKey(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewWallet(tmpDir, &config.NamecoinMainNetParams)
	if err != nil {
		t.Fatalf("NewWallet failed: %v", err)
	}

	utxos := []UTXO{{
		Address: "NNNnone",
		Value:   50000,
	}}

	_, err = w.CreateNameUpdateTx("d/test", "value", utxos, 0, 100, nil)
	if err == nil {
		t.Error("expected error when wallet has no key for address")
	}
}

// TestCreateNameUpdateTxWithDestAddress tests CreateNameUpdateTx with a dest address.
func TestCreateNameUpdateTxWithDestAddress(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewWallet(tmpDir, &config.NamecoinMainNetParams)
	if err != nil {
		t.Fatalf("NewWallet failed: %v", err)
	}

	// Generate a key so we have an address to use
	addrStr, err := w.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	w.mu.RLock()
	kp := w.keys[addrStr]
	w.mu.RUnlock()

	utxos := []UTXO{{
		Address: addrStr,
		Value:   50000,
	}}

	// Use an unsupported address type (btcutil.AddressPubKey) to hit the unsupported type branch.
	pkAddr, err := btcutil.NewAddressPubKey(kp.PublicKey.SerializeCompressed(), &config.NamecoinMainNetParams)
	if err != nil {
		t.Fatalf("failed to create pubkey address: %v", err)
	}

	_, err = w.CreateNameUpdateTx("d/test", "value", utxos, 0, 100, pkAddr)
	if err == nil {
		t.Error("expected error for unsupported destination address type")
	}
}
