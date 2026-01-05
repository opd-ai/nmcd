// Package wallet provides basic wallet functionality for nmcd.
package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/scrypt"
)

// Scrypt parameters for key derivation.
// These values balance security and performance based on OWASP recommendations.
const (
	// scryptN is the CPU/memory cost parameter (must be power of 2)
	// 32768 (2^15) provides strong security while remaining usable on modern hardware
	scryptN = 32768

	// scryptR is the block size parameter
	scryptR = 8

	// scryptP is the parallelization parameter
	scryptP = 1

	// keyLen is the desired key length in bytes (256 bits for AES-256)
	keyLen = 32

	// saltLen is the length of the random salt in bytes
	saltLen = 32

	// nonceLen is the length of the GCM nonce in bytes
	nonceLen = 12
)

// encryptedData represents an encrypted blob with its salt and nonce.
type encryptedData struct {
	Salt       []byte // Random salt for key derivation
	Nonce      []byte // Random nonce for GCM
	Ciphertext []byte // Encrypted data
}

// deriveKey derives an encryption key from a password using scrypt.
// The salt ensures that the same password produces different keys.
func deriveKey(password string, salt []byte) ([]byte, error) {
	if len(password) == 0 {
		return nil, fmt.Errorf("password cannot be empty")
	}
	if len(salt) != saltLen {
		return nil, fmt.Errorf("invalid salt length: %d (expected %d)", len(salt), saltLen)
	}

	key, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}
	return key, nil
}

// encrypt encrypts plaintext using AES-256-GCM with a password-derived key.
// Returns the encrypted data with salt and nonce, or an error.
func encrypt(plaintext []byte, password string) (*encryptedData, error) {
	// Generate random salt for key derivation
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive encryption key from password
	key, err := deriveKey(password, salt)
	if err != nil {
		return nil, err
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt plaintext
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	return &encryptedData{
		Salt:       salt,
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}, nil
}

// decrypt decrypts ciphertext using AES-256-GCM with a password-derived key.
// Returns the plaintext or an error if decryption fails.
func decrypt(data *encryptedData, password string) ([]byte, error) {
	if data == nil {
		return nil, fmt.Errorf("encrypted data cannot be nil")
	}

	// Derive decryption key from password
	key, err := deriveKey(password, data.Salt)
	if err != nil {
		return nil, err
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Verify nonce size
	if len(data.Nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid nonce size: %d (expected %d)", len(data.Nonce), gcm.NonceSize())
	}

	// Decrypt ciphertext
	plaintext, err := gcm.Open(nil, data.Nonce, data.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// validatePassword checks if a password meets minimum security requirements.
// Returns an error describing any issues, or nil if the password is acceptable.
func validatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}
	if len(password) > 256 {
		return fmt.Errorf("password must be at most 256 characters long")
	}

	// Check for password strength: require at least 2 character types
	var hasLower, hasUpper, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case ch >= 'a' && ch <= 'z':
			hasLower = true
		case ch >= 'A' && ch <= 'Z':
			hasUpper = true
		case ch >= '0' && ch <= '9':
			hasDigit = true
		case ch < ' ' || ch > '~':
			// Control or non-ASCII characters not allowed for better compatibility
			return fmt.Errorf("password contains invalid characters")
		default:
			hasSpecial = true
		}
	}

	// Count character types present
	typeCount := 0
	if hasLower {
		typeCount++
	}
	if hasUpper {
		typeCount++
	}
	if hasDigit {
		typeCount++
	}
	if hasSpecial {
		typeCount++
	}

	if typeCount < 2 {
		return fmt.Errorf("password must contain at least 2 of: lowercase, uppercase, digits, or special characters")
	}

	return nil
}

// hashPassword creates a non-reversible hash of a password for verification.
// Uses scrypt with a fixed salt to prevent rainbow table attacks while allowing
// verification without storing the salt separately. The hash is used only for
// password verification, not for encryption (encryption uses its own random salt).
func hashPassword(password string) []byte {
	// Use a fixed salt for password verification
	// This is safe because:
	// 1. We're not encrypting with this hash (encryption uses random salt)
	// 2. We're only verifying the password matches
	// 3. Scrypt with any salt is better than SHA-256 alone
	salt := []byte("nmcd-wallet-password-verification")
	
	// Use lighter scrypt parameters for verification (faster unlock)
	// N=16384 is still strong enough to resist brute-force
	hash, err := scrypt.Key([]byte(password), salt, 16384, 8, 1, 32)
	if err != nil {
		// Fallback to SHA-256 if scrypt fails (should never happen)
		h := sha256.Sum256([]byte(password))
		return h[:]
	}
	return hash
}

// encodeEncryptedData encodes encrypted data to base64 for storage.
func encodeEncryptedData(data *encryptedData) string {
	// Format: base64(salt):base64(nonce):base64(ciphertext)
	saltB64 := base64.StdEncoding.EncodeToString(data.Salt)
	nonceB64 := base64.StdEncoding.EncodeToString(data.Nonce)
	ciphertextB64 := base64.StdEncoding.EncodeToString(data.Ciphertext)
	return saltB64 + ":" + nonceB64 + ":" + ciphertextB64
}

// decodeEncryptedData decodes base64-encoded encrypted data.
func decodeEncryptedData(encoded string) (*encryptedData, error) {
	// Parse format: base64(salt):base64(nonce):base64(ciphertext)
	parts := strings.Split(encoded, ":")

	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid encrypted data format")
	}

	salt, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("failed to decode salt: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	return &encryptedData{
		Salt:       salt,
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}, nil
}
