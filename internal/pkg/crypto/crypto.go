package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultBcryptCost is the default cost factor for bcrypt hashing.
	DefaultBcryptCost = 12
)

// HashPassword hashes the given password using bcrypt with the default cost.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), DefaultBcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(bytes), nil
}

// CheckPassword compares a plaintext password against a bcrypt hash.
// Returns nil on success, or an error if they do not match.
func CheckPassword(password, hash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return fmt.Errorf("password mismatch: %w", err)
	}
	return nil
}

// GenerateRandomToken produces a cryptographically secure random hex string
// of the specified byte length (the returned string will be twice as long).
func GenerateRandomToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// Encrypt encrypts plaintext using AES-256-GCM with the provided hex-encoded key.
// The key must be 64 hex characters (32 bytes).
func Encrypt(plaintext, key string) (string, error) {
	keyBytes, err := hex.DecodeString(key)
	if err != nil {
		return "", fmt.Errorf("invalid key encoding: %w", err)
	}
	if len(keyBytes) != 32 {
		return "", fmt.Errorf("key must be 32 bytes (64 hex characters), got %d bytes", len(keyBytes))
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// Decrypt decrypts AES-256-GCM ciphertext (hex-encoded) using the provided hex-encoded key.
// The key must be 64 hex characters (32 bytes).
func Decrypt(ciphertext, key string) (string, error) {
	keyBytes, err := hex.DecodeString(key)
	if err != nil {
		return "", fmt.Errorf("invalid key encoding: %w", err)
	}
	if len(keyBytes) != 32 {
		return "", fmt.Errorf("key must be 32 bytes (64 hex characters), got %d bytes", len(keyBytes))
	}

	ciphertextBytes, err := hex.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("invalid ciphertext encoding: %w", err)
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertextBytes) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextRaw := ciphertextBytes[:nonceSize], ciphertextBytes[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertextRaw, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}
