// Package secretbox provides versioned application-layer encryption for
// tenant-scoped secrets stored in the database.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const ciphertextVersion = "v1:"
const maxJSONDocumentBytes = 2 << 20

var (
	ErrInvalidKey        = errors.New("invalid secretbox key")
	ErrInvalidCiphertext = errors.New("invalid secretbox ciphertext")
)

// Box encrypts and authenticates values with AES-256-GCM. Tenant and purpose
// are authenticated as associated data, preventing ciphertext substitution
// between organizations or fields.
type Box struct {
	aead cipher.AEAD
}

// NewHex creates a Box from exactly 32 bytes encoded as hexadecimal.
func NewHex(encodedKey string) (*Box, error) {
	key, err := hex.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("%w: key must be hex-encoded 32-byte material", ErrInvalidKey)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: initialize AES: %v", ErrInvalidKey, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: initialize GCM: %v", ErrInvalidKey, err)
	}
	return &Box{aead: aead}, nil
}

// Seal returns a versioned, URL-safe ciphertext.
func (b *Box) Seal(tenantID, purpose string, plaintext []byte) (string, error) {
	if b == nil || b.aead == nil {
		return "", fmt.Errorf("%w: box is not initialized", ErrInvalidKey)
	}
	associatedData, err := buildAssociatedData(tenantID, purpose)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate encryption nonce: %w", err)
	}
	sealed := b.aead.Seal(nonce, nonce, plaintext, associatedData)
	return ciphertextVersion + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Open authenticates and decrypts a versioned ciphertext.
func (b *Box) Open(tenantID, purpose, ciphertext string) ([]byte, error) {
	if b == nil || b.aead == nil {
		return nil, fmt.Errorf("%w: box is not initialized", ErrInvalidKey)
	}
	associatedData, err := buildAssociatedData(tenantID, purpose)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(ciphertext, ciphertextVersion) {
		return nil, ErrInvalidCiphertext
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ciphertext, ciphertextVersion))
	if err != nil || len(decoded) < b.aead.NonceSize()+b.aead.Overhead() {
		return nil, ErrInvalidCiphertext
	}
	nonce, encrypted := decoded[:b.aead.NonceSize()], decoded[b.aead.NonceSize():]
	plaintext, err := b.aead.Open(nil, nonce, encrypted, associatedData)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plaintext, nil
}

// SealJSON marshals a value and wraps its ciphertext in a small JSON document
// suitable for a JSONB column. No plaintext metadata is retained.
func (b *Box) SealJSON(tenantID, purpose string, value any) ([]byte, error) {
	plaintext, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal secret JSON: %w", err)
	}
	if len(plaintext) > maxJSONDocumentBytes {
		return nil, fmt.Errorf("secret JSON exceeds maximum size")
	}
	ciphertext, err := b.Seal(tenantID, purpose, plaintext)
	if err != nil {
		return nil, err
	}
	document, err := json.Marshal(struct {
		Sealed string `json:"sealed"`
	}{Sealed: ciphertext})
	if err != nil {
		return nil, fmt.Errorf("marshal sealed JSON document: %w", err)
	}
	return document, nil
}

// OpenJSON authenticates a sealed JSON document and unmarshals its plaintext.
func (b *Box) OpenJSON(tenantID, purpose string, document []byte, destination any) error {
	if len(document) == 0 || len(document) > maxJSONDocumentBytes*2 {
		return ErrInvalidCiphertext
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(document, &fields) != nil || len(fields) != 1 {
		return ErrInvalidCiphertext
	}
	var ciphertext string
	if json.Unmarshal(fields["sealed"], &ciphertext) != nil || ciphertext == "" {
		return ErrInvalidCiphertext
	}
	plaintext, err := b.Open(tenantID, purpose, ciphertext)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(plaintext, destination); err != nil {
		return ErrInvalidCiphertext
	}
	return nil
}

func buildAssociatedData(tenantID, purpose string) ([]byte, error) {
	tenantID = strings.TrimSpace(tenantID)
	purpose = strings.TrimSpace(purpose)
	if tenantID == "" || purpose == "" || len(tenantID) > 128 || len(purpose) > 256 ||
		strings.ContainsRune(tenantID, '\x00') || strings.ContainsRune(purpose, '\x00') {
		return nil, fmt.Errorf("%w: tenant and purpose are required", ErrInvalidCiphertext)
	}
	return []byte("complianceforge-secretbox\x00" + tenantID + "\x00" + purpose), nil
}
