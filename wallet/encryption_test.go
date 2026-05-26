package wallet

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	tests := []struct {
		name      string
		plaintext string
		password  string
	}{
		{
			name:      "simple text",
			plaintext: "Hello, World!",
			password:  "TestPassword123",
		},
		{
			name:      "empty plaintext",
			plaintext: "",
			password:  "TestPassword123",
		},
		{
			name:      "long plaintext",
			plaintext: strings.Repeat("Lorem ipsum dolor sit amet. ", 100),
			password:  "TestPassword123",
		},
		{
			name:      "binary data",
			plaintext: string([]byte{0, 1, 2, 3, 255, 254, 253}),
			password:  "TestPassword123",
		},
		{
			name:      "special characters in password",
			plaintext: "secret data",
			password:  "P@ssw0rd!#$%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt
			encrypted, err := encrypt([]byte(tt.plaintext), []byte(tt.password))
			if err != nil {
				t.Fatalf("encrypt() error = %v", err)
			}

			// Verify encrypted data has required fields
			if len(encrypted.Salt) != saltLen {
				t.Errorf("salt length = %d, want %d", len(encrypted.Salt), saltLen)
			}
			if len(encrypted.Nonce) != nonceLen {
				t.Errorf("nonce length = %d, want %d", len(encrypted.Nonce), nonceLen)
			}
			if len(encrypted.Ciphertext) == 0 {
				t.Error("ciphertext is empty")
			}

			// Decrypt
			decrypted, err := decrypt(encrypted, []byte(tt.password))
			if err != nil {
				t.Fatalf("decrypt() error = %v", err)
			}

			// Verify decrypted matches original
			if string(decrypted) != tt.plaintext {
				t.Errorf("decrypted = %q, want %q", string(decrypted), tt.plaintext)
			}
		})
	}
}

func TestDecryptWrongPassword(t *testing.T) {
	plaintext := []byte("sensitive data")
	correctPassword := "CorrectPassword123"
	wrongPassword := "WrongPassword456"

	encrypted, err := encrypt(plaintext, []byte(correctPassword))
	if err != nil {
		t.Fatalf("encrypt() error = %v", err)
	}

	// Try to decrypt with wrong password
	_, err = decrypt(encrypted, []byte(wrongPassword))
	if err == nil {
		t.Error("decrypt() with wrong password should fail")
	}
}

func TestEncryptEmptyPassword(t *testing.T) {
	plaintext := []byte("data")
	_, err := encrypt(plaintext, []byte(""))
	if err == nil {
		t.Error("encrypt() with empty password should fail")
	}
}

func TestDecryptNilData(t *testing.T) {
	_, err := decrypt(nil, []byte("password"))
	if err == nil {
		t.Error("decrypt() with nil data should fail")
	}
}

func TestEncryptionUniqueness(t *testing.T) {
	// Same plaintext and password should produce different ciphertext due to random salt/nonce
	plaintext := []byte("test data")
	password := "TestPassword123"

	encrypted1, err := encrypt(plaintext, []byte(password))
	if err != nil {
		t.Fatalf("encrypt() error = %v", err)
	}

	encrypted2, err := encrypt(plaintext, []byte(password))
	if err != nil {
		t.Fatalf("encrypt() error = %v", err)
	}

	// Salts should be different
	if bytes.Equal(encrypted1.Salt, encrypted2.Salt) {
		t.Error("salts should be unique")
	}

	// Nonces should be different
	if bytes.Equal(encrypted1.Nonce, encrypted2.Nonce) {
		t.Error("nonces should be unique")
	}

	// Ciphertexts should be different
	if bytes.Equal(encrypted1.Ciphertext, encrypted2.Ciphertext) {
		t.Error("ciphertexts should be unique")
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{
			name:     "valid password with mixed case and digits",
			password: "Password123",
			wantErr:  false,
		},
		{
			name:     "valid password with special chars",
			password: "Pass@word!",
			wantErr:  false,
		},
		{
			name:     "valid password lowercase and digits",
			password: "password123",
			wantErr:  false,
		},
		{
			name:     "valid password uppercase and special",
			password: "PASSWORD!@#",
			wantErr:  false,
		},
		{
			name:     "too short",
			password: "Pass1",
			wantErr:  true,
		},
		{
			name:     "minimum length",
			password: "Pass123!",
			wantErr:  false,
		},
		{
			name:     "only lowercase",
			password: "password",
			wantErr:  true,
		},
		{
			name:     "only uppercase",
			password: "PASSWORD",
			wantErr:  true,
		},
		{
			name:     "only digits",
			password: "12345678",
			wantErr:  true,
		},
		{
			name:     "only special chars",
			password: "!@#$%^&*",
			wantErr:  true,
		},
		{
			name:     "too long",
			password: strings.Repeat("a", 257),
			wantErr:  true,
		},
		{
			name:     "maximum length",
			password: strings.Repeat("A1", 128), // 256 chars
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePassword() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeriveKeyConsistency(t *testing.T) {
	// Same password and salt should produce the same key
	password := "TestPassword123"
	salt := make([]byte, saltLen)
	rand.Read(salt)

	key1, err := deriveKey([]byte(password), salt)
	if err != nil {
		t.Fatalf("deriveKey() error = %v", err)
	}

	key2, err := deriveKey([]byte(password), salt)
	if err != nil {
		t.Fatalf("deriveKey() error = %v", err)
	}

	if !bytes.Equal(key1, key2) {
		t.Error("derived keys should be identical for same password and salt")
	}

	// Different salt should produce different key
	salt2 := make([]byte, saltLen)
	rand.Read(salt2)

	key3, err := deriveKey([]byte(password), salt2)
	if err != nil {
		t.Fatalf("deriveKey() error = %v", err)
	}

	if bytes.Equal(key1, key3) {
		t.Error("derived keys should be different for different salts")
	}
}

func TestHashPassword(t *testing.T) {
	password := "TestPassword123"
	salt := []byte("test-salt-for-password-hashing")

	hash1 := hashPassword([]byte(password), salt)
	hash2 := hashPassword([]byte(password), salt)

	// Same password and salt should produce same hash
	if !bytes.Equal(hash1, hash2) {
		t.Error("hash should be consistent for same password and salt")
	}

	// Different password should produce different hash
	hash3 := hashPassword([]byte("DifferentPassword456"), salt)
	if bytes.Equal(hash1, hash3) {
		t.Error("hash should be different for different passwords")
	}

	// Different salt should produce different hash
	salt2 := []byte("different-salt-for-password-hashing")
	hash4 := hashPassword([]byte(password), salt2)
	if bytes.Equal(hash1, hash4) {
		t.Error("hash should be different for different salts")
	}

	// Hash should be 32 bytes (scrypt output)
	if len(hash1) != 32 {
		t.Errorf("hash length = %d, want 32", len(hash1))
	}
}

func TestEncodeDecodeEncryptedData(t *testing.T) {
	// Create sample encrypted data
	original := &encryptedData{
		Salt:       make([]byte, saltLen),
		Nonce:      make([]byte, nonceLen),
		Ciphertext: []byte("test ciphertext data"),
	}
	rand.Read(original.Salt)
	rand.Read(original.Nonce)

	// Encode
	encoded := encodeEncryptedData(original)
	if encoded == "" {
		t.Fatal("encoded string is empty")
	}

	// Verify format (should have 2 colons)
	colonCount := strings.Count(encoded, ":")
	if colonCount != 2 {
		t.Errorf("encoded format should have 2 colons, got %d", colonCount)
	}

	// Decode
	decoded, err := decodeEncryptedData(encoded)
	if err != nil {
		t.Fatalf("decodeEncryptedData() error = %v", err)
	}

	// Verify decoded matches original
	if !bytes.Equal(decoded.Salt, original.Salt) {
		t.Error("decoded salt doesn't match original")
	}
	if !bytes.Equal(decoded.Nonce, original.Nonce) {
		t.Error("decoded nonce doesn't match original")
	}
	if !bytes.Equal(decoded.Ciphertext, original.Ciphertext) {
		t.Error("decoded ciphertext doesn't match original")
	}
}

func TestDecodeInvalidEncryptedData(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
	}{
		{
			name:    "missing parts",
			encoded: "salt:nonce",
		},
		{
			name:    "too many parts",
			encoded: "salt:nonce:ciphertext:extra",
		},
		{
			name:    "invalid base64",
			encoded: "!!!:valid:valid",
		},
		{
			name:    "empty string",
			encoded: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeEncryptedData(tt.encoded)
			if err == nil {
				t.Error("decodeEncryptedData() should fail for invalid input")
			}
		})
	}
}

func TestEncryptDecryptLargeData(t *testing.T) {
	// Test with 1MB of random data
	plaintext := make([]byte, 1024*1024)
	rand.Read(plaintext)

	password := "TestPassword123"

	encrypted, err := encrypt(plaintext, []byte(password))
	if err != nil {
		t.Fatalf("encrypt() error = %v", err)
	}

	decrypted, err := decrypt(encrypted, []byte(password))
	if err != nil {
		t.Fatalf("decrypt() error = %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Error("decrypted data doesn't match original large data")
	}
}
