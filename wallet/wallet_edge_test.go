package wallet

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
)

// TestLoadEmptyWalletFile tests loading a wallet from an empty/nonexistent file
func TestLoadEmptyWalletFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a new wallet in the temp directory
	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Verify wallet starts with no keys
	addrs := w.GetAddresses()
	if len(addrs) != 0 {
		t.Errorf("Expected 0 addresses in new wallet, got %d", len(addrs))
	}
}

// TestGetKeyNotFound tests getting a key for an address that doesn't exist
func TestGetKeyNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Try to get a key for a non-existent address
	_, err = w.GetKey("mNonExistentAddress12345")
	if err == nil {
		t.Error("Expected error when getting non-existent key")
	}
}

// TestHasKeyNonExistent tests checking for a non-existent key
func TestHasKeyNonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	if w.HasKey("mNonExistentAddress12345") {
		t.Error("Expected HasKey to return false for non-existent address")
	}
}

// TestEncryptWalletAlreadyEncrypted tests encrypting an already encrypted wallet
func TestEncryptWalletAlreadyEncrypted(t *testing.T) {
	tmpDir := t.TempDir()

	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Generate a key first
	_, err = w.GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Encrypt once
	password := "TestPassword123!@#"
	err = w.EncryptWallet(password)
	if err != nil {
		t.Fatalf("Failed to encrypt wallet: %v", err)
	}

	// Try to encrypt again
	err = w.EncryptWallet(password)
	if err == nil {
		t.Error("Expected error when encrypting already encrypted wallet")
	}
}

// TestLockUnlockSequence tests the lock/unlock cycle edge cases
func TestLockUnlockSequence(t *testing.T) {
	tmpDir := t.TempDir()

	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	_, err = w.GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	password := "TestPassword123!@#"
	err = w.EncryptWallet(password)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// Lock
	err = w.Lock()
	if err != nil {
		t.Fatalf("Failed to lock: %v", err)
	}

	// Try locking again (should fail)
	err = w.Lock()
	if err == nil {
		t.Error("Expected error when locking already locked wallet")
	}

	// Unlock with correct password
	err = w.Unlock(password)
	if err != nil {
		t.Fatalf("Failed to unlock: %v", err)
	}

	// Try unlocking again (should fail)
	err = w.Unlock(password)
	if err == nil {
		t.Error("Expected error when unlocking already unlocked wallet")
	}
}

// TestUnlockNotEncrypted tests unlocking an unencrypted wallet
func TestUnlockNotEncrypted(t *testing.T) {
	tmpDir := t.TempDir()

	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Try to unlock an unencrypted wallet
	err = w.Unlock("anyPassword")
	if err == nil {
		t.Error("Expected error when unlocking unencrypted wallet")
	}
}

// TestIsEncryptedAndIsLocked tests the state query functions
func TestIsEncryptedAndIsLocked(t *testing.T) {
	tmpDir := t.TempDir()

	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Initially not encrypted and not locked
	if w.IsEncrypted() {
		t.Error("New wallet should not be encrypted")
	}
	if w.IsLocked() {
		t.Error("New wallet should not be locked")
	}

	// Generate key and encrypt
	_, err = w.GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	password := "TestPassword123!@#"
	err = w.EncryptWallet(password)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// After encryption, should be encrypted but not locked
	if !w.IsEncrypted() {
		t.Error("Wallet should be encrypted after EncryptWallet")
	}
	if w.IsLocked() {
		t.Error("Wallet should not be locked immediately after encryption")
	}

	// After locking
	err = w.Lock()
	if err != nil {
		t.Fatalf("Failed to lock: %v", err)
	}

	if !w.IsLocked() {
		t.Error("Wallet should be locked after Lock()")
	}
}

// TestGenerateMultipleKeys tests generating multiple keys
func TestGenerateMultipleKeys(t *testing.T) {
	tmpDir := t.TempDir()

	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	addresses := make(map[string]bool)

	// Generate 10 keys
	for i := 0; i < 10; i++ {
		addr, err := w.GenerateKey()
		if err != nil {
			t.Fatalf("Failed to generate key %d: %v", i, err)
		}

		// Verify uniqueness
		if addresses[addr] {
			t.Errorf("Duplicate address generated: %s", addr)
		}
		addresses[addr] = true
	}

	// Verify all keys are accessible
	walletAddrs := w.GetAddresses()
	if len(walletAddrs) != 10 {
		t.Errorf("Expected 10 addresses, got %d", len(walletAddrs))
	}
}

// TestDecodeInvalidBase64 tests decoding invalid encrypted data
func TestDecodeInvalidBase64(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{"missing parts", "invalid"},
		{"two parts", "part1:part2"},
		{"invalid base64 in salt", "!!!:YWJj:YWJj"},
		{"invalid base64 in nonce", "YWJj:!!!:YWJj"},
		{"invalid base64 in ciphertext", "YWJj:YWJj:!!!"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeEncryptedData(tc.input)
			if err == nil {
				t.Error("Expected error for invalid encrypted data")
			}
		})
	}
}

// TestHashPasswordDeterminism tests that hashPassword is deterministic
func TestHashPasswordDeterminism(t *testing.T) {
	password := "testPassword123!"
	salt := []byte("testsalt12345678901234567890")

	hash1 := hashPassword(password, salt)
	hash2 := hashPassword(password, salt)

	if len(hash1) != len(hash2) {
		t.Error("Hash lengths don't match")
	}

	for i := range hash1 {
		if hash1[i] != hash2[i] {
			t.Error("Hashes don't match for same input")
			break
		}
	}
}

// TestValidatePasswordEdgeCases tests password validation edge cases
func TestValidatePasswordEdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"empty", "", true},
		{"too short", "Ab1!", true},
		{"only lowercase", "abcdefgh", true},             // only 1 type
		{"only uppercase", "ABCDEFGH", true},             // only 1 type
		{"only digits", "12345678", true},                // only 1 type
		{"only special", "!@#$%^&*", true},               // only 1 type
		{"two types lowercase_digit", "abcd1234", false}, // 2 types OK
		{"two types upper_lower", "ABCDabcd", false},     // 2 types OK
		{"valid three types", "Abcdef1!", false},
		{"valid long", "ThisIsAVeryLongPassword123!@#", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePassword(tc.password)
			if tc.wantErr && err == nil {
				t.Error("Expected error for invalid password")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestEncryptDecryptRoundTrip tests the full encrypt/decrypt cycle
func TestEncryptDecryptRoundTrip(t *testing.T) {
	password := "TestPassword123!"
	plaintext := []byte("This is secret data")

	// Encrypt
	encrypted, err := encrypt(plaintext, password)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	// Decrypt
	decrypted, err := decrypt(encrypted, password)
	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	// Verify
	if string(decrypted) != string(plaintext) {
		t.Error("Decrypted data doesn't match original")
	}
}

// TestDeriveKeyEmptySalt tests deriveKey with empty salt
func TestDeriveKeyEmptySalt(t *testing.T) {
	password := "testpassword"
	salt := []byte{}

	// Should return error for invalid salt length
	_, err := deriveKey(password, salt)
	if err == nil {
		t.Error("Expected error for empty salt")
	}
}

// TestDeriveKeyEmptyPassword tests deriveKey with empty password
func TestDeriveKeyEmptyPassword(t *testing.T) {
	salt := make([]byte, 32) // proper salt length

	// Should return error for empty password
	_, err := deriveKey("", salt)
	if err == nil {
		t.Error("Expected error for empty password")
	}
}

// TestDeriveKeyValid tests deriveKey with valid inputs
func TestDeriveKeyValid(t *testing.T) {
	password := "testpassword"
	salt := make([]byte, 32)
	for i := range salt {
		salt[i] = byte(i)
	}

	key, err := deriveKey(password, salt)
	if err != nil {
		t.Fatalf("deriveKey failed: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("Expected 32-byte key, got %d bytes", len(key))
	}
}
