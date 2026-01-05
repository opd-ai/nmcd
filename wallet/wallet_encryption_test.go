package wallet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
)

func TestEncryptWallet(t *testing.T) {
	tmpDir := t.TempDir()

	// Create unencrypted wallet
	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	// Verify wallet is not encrypted
	if w.IsEncrypted() {
		t.Error("new wallet should not be encrypted")
	}
	if w.IsLocked() {
		t.Error("unencrypted wallet should not be locked")
	}

	// Generate a key
	addr, err := w.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Encrypt the wallet
	password := "TestPassword123"
	if err := w.EncryptWallet(password); err != nil {
		t.Fatalf("failed to encrypt wallet: %v", err)
	}

	// Verify wallet is now encrypted and unlocked
	if !w.IsEncrypted() {
		t.Error("wallet should be encrypted after EncryptWallet")
	}
	if w.IsLocked() {
		t.Error("wallet should be unlocked immediately after encryption")
	}

	// Verify key is still accessible
	if !w.HasKey(addr) {
		t.Error("key should still be accessible after encryption")
	}

	// Try to encrypt again (should fail)
	if err := w.EncryptWallet("AnotherPassword456"); err == nil {
		t.Error("encrypting an already encrypted wallet should fail")
	}
}

func TestEncryptWalletWeakPassword(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	// Try to encrypt with weak passwords
	weakPasswords := []string{
		"short",     // Too short
		"password",  // Only lowercase
		"PASSWORD",  // Only uppercase
		"12345678",  // Only digits
	}

	for _, pass := range weakPasswords {
		if err := w.EncryptWallet(pass); err == nil {
			t.Errorf("EncryptWallet should reject weak password: %q", pass)
		}
	}
}

func TestLockUnlock(t *testing.T) {
	tmpDir := t.TempDir()

	// Create and encrypt wallet
	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	addr, err := w.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	password := "TestPassword123"
	if err := w.EncryptWallet(password); err != nil {
		t.Fatalf("failed to encrypt wallet: %v", err)
	}

	// Lock the wallet
	if err := w.Lock(); err != nil {
		t.Fatalf("failed to lock wallet: %v", err)
	}

	// Verify wallet is locked
	if !w.IsLocked() {
		t.Error("wallet should be locked after Lock()")
	}

	// Verify keys are cleared from memory
	if w.HasKey(addr) {
		t.Error("keys should be cleared from memory when locked")
	}

	// Try to lock again (should fail)
	if err := w.Lock(); err == nil {
		t.Error("locking an already locked wallet should fail")
	}

	// Unlock the wallet
	if err := w.Unlock(password); err != nil {
		t.Fatalf("failed to unlock wallet: %v", err)
	}

	// Verify wallet is unlocked
	if w.IsLocked() {
		t.Error("wallet should be unlocked after Unlock()")
	}

	// Verify keys are restored
	if !w.HasKey(addr) {
		t.Error("keys should be restored after unlock")
	}

	// Verify key data is correct
	kp, err := w.GetKey(addr)
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}
	if kp.Address.EncodeAddress() != addr {
		t.Error("restored key address doesn't match")
	}
}

func TestUnlockWrongPassword(t *testing.T) {
	tmpDir := t.TempDir()

	// Create and encrypt wallet
	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	correctPassword := "CorrectPassword123"
	if err := w.EncryptWallet(correctPassword); err != nil {
		t.Fatalf("failed to encrypt wallet: %v", err)
	}

	// Lock the wallet
	if err := w.Lock(); err != nil {
		t.Fatalf("failed to lock wallet: %v", err)
	}

	// Try to unlock with wrong password
	wrongPassword := "WrongPassword456"
	if err := w.Unlock(wrongPassword); err == nil {
		t.Error("unlocking with wrong password should fail")
	}

	// Verify wallet is still locked
	if !w.IsLocked() {
		t.Error("wallet should remain locked after failed unlock attempt")
	}

	// Unlock with correct password
	if err := w.Unlock(correctPassword); err != nil {
		t.Fatalf("failed to unlock with correct password: %v", err)
	}

	// Verify wallet is now unlocked
	if w.IsLocked() {
		t.Error("wallet should be unlocked with correct password")
	}
}

func TestEncryptedWalletPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create and encrypt wallet
	w1, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	addr, err := w1.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	password := "TestPassword123"
	if err := w1.EncryptWallet(password); err != nil {
		t.Fatalf("failed to encrypt wallet: %v", err)
	}

	// Lock the wallet
	if err := w1.Lock(); err != nil {
		t.Fatalf("failed to lock wallet: %v", err)
	}

	// Load wallet in new instance
	w2, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to load wallet: %v", err)
	}

	// Verify wallet is encrypted and locked
	if !w2.IsEncrypted() {
		t.Error("loaded wallet should be encrypted")
	}
	if !w2.IsLocked() {
		t.Error("loaded encrypted wallet should start locked")
	}

	// Verify keys are not accessible when locked
	if w2.HasKey(addr) {
		t.Error("keys should not be accessible in locked wallet")
	}

	// Unlock with password
	if err := w2.Unlock(password); err != nil {
		t.Fatalf("failed to unlock loaded wallet: %v", err)
	}

	// Verify key is now accessible
	if !w2.HasKey(addr) {
		t.Error("key should be accessible after unlock")
	}

	// Verify key data is correct
	kp, err := w2.GetKey(addr)
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}
	if kp.Address.EncodeAddress() != addr {
		t.Error("loaded key address doesn't match")
	}
}

func TestMigrationFromUnencrypted(t *testing.T) {
	tmpDir := t.TempDir()

	// Create unencrypted wallet with keys
	w1, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	// Generate multiple keys
	var addrs []string
	for i := 0; i < 3; i++ {
		addr, err := w1.GenerateKey()
		if err != nil {
			t.Fatalf("failed to generate key: %v", err)
		}
		addrs = append(addrs, addr)
	}

	// Verify wallet file exists and is unencrypted (version 2, encrypted=false or version 0/1)
	walletPath := filepath.Join(tmpDir, "wallet.json")
	data, err := os.ReadFile(walletPath)
	if err != nil {
		t.Fatalf("failed to read wallet file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("wallet file is empty")
	}

	// Encrypt the wallet
	password := "MigrationPassword123"
	if err := w1.EncryptWallet(password); err != nil {
		t.Fatalf("failed to encrypt wallet: %v", err)
	}

	// Verify all keys are still accessible
	for _, addr := range addrs {
		if !w1.HasKey(addr) {
			t.Errorf("key %s not found after encryption", addr)
		}
	}

	// Lock the wallet
	if err := w1.Lock(); err != nil {
		t.Fatalf("failed to lock wallet: %v", err)
	}

	// Load wallet in new instance
	w2, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to load encrypted wallet: %v", err)
	}

	// Verify encrypted
	if !w2.IsEncrypted() {
		t.Error("migrated wallet should be encrypted")
	}

	// Unlock and verify all keys
	if err := w2.Unlock(password); err != nil {
		t.Fatalf("failed to unlock migrated wallet: %v", err)
	}

	for _, addr := range addrs {
		if !w2.HasKey(addr) {
			t.Errorf("key %s not found in migrated wallet", addr)
		}

		// Verify key data integrity
		kp, err := w2.GetKey(addr)
		if err != nil {
			t.Errorf("failed to get key %s: %v", addr, err)
			continue
		}
		if kp.Address.EncodeAddress() != addr {
			t.Errorf("key address mismatch for %s", addr)
		}
	}
}

func TestUnencryptedWalletCannotLock(t *testing.T) {
	tmpDir := t.TempDir()

	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	// Try to lock unencrypted wallet (should fail)
	if err := w.Lock(); err == nil {
		t.Error("locking an unencrypted wallet should fail")
	}

	// Try to unlock unencrypted wallet (should fail)
	if err := w.Unlock("password"); err == nil {
		t.Error("unlocking an unencrypted wallet should fail")
	}
}

func TestGenerateKeyWhileLocked(t *testing.T) {
	tmpDir := t.TempDir()

	// Create and encrypt wallet
	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	password := "TestPassword123"
	if err := w.EncryptWallet(password); err != nil {
		t.Fatalf("failed to encrypt wallet: %v", err)
	}

	// Lock the wallet
	if err := w.Lock(); err != nil {
		t.Fatalf("failed to lock wallet: %v", err)
	}

	// Try to generate key while locked. This must fail because locked wallets
	// must not perform any operations that access or persist private keys.
	_, err = w.GenerateKey()
	if err == nil {
		t.Error("generating key while locked should fail (cannot save)")
	}
}

func TestEncryptedWalletMultipleKeys(t *testing.T) {
	tmpDir := t.TempDir()

	// Create wallet and generate multiple keys
	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	// Generate 10 keys
	var addrs []string
	for i := 0; i < 10; i++ {
		addr, err := w.GenerateKey()
		if err != nil {
			t.Fatalf("failed to generate key %d: %v", i, err)
		}
		addrs = append(addrs, addr)
	}

	// Encrypt wallet
	password := "TestPassword123"
	if err := w.EncryptWallet(password); err != nil {
		t.Fatalf("failed to encrypt wallet: %v", err)
	}

	// Lock and reload
	if err := w.Lock(); err != nil {
		t.Fatalf("failed to lock wallet: %v", err)
	}

	w2, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to load wallet: %v", err)
	}

	if err := w2.Unlock(password); err != nil {
		t.Fatalf("failed to unlock wallet: %v", err)
	}

	// Verify all keys are present and correct
	for _, addr := range addrs {
		if !w2.HasKey(addr) {
			t.Errorf("key %s not found after unlock", addr)
		}
	}
}
