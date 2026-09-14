package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNewIntegrationServiceFailsClosed(t *testing.T) {
	validKey := hex.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

	if _, err := NewIntegrationService(nil, validKey); err == nil {
		t.Fatal("NewIntegrationService() accepted a nil database")
	}
	if _, err := NewIntegrationService(new(pgxpool.Pool), ""); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("NewIntegrationService() error = %v, want required-key error", err)
	}
	if _, err := NewIntegrationService(new(pgxpool.Pool), "not-hex"); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("NewIntegrationService() error = %v, want decode error", err)
	}
	if _, err := NewIntegrationService(new(pgxpool.Pool), hex.EncodeToString([]byte("too short"))); err == nil || !strings.Contains(err.Error(), "32 bytes") {
		t.Fatalf("NewIntegrationService() error = %v, want key-length error", err)
	}
}

func TestIntegrationConfigEncryptionIsVersionedAndTenantBound(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	svc, err := NewIntegrationService(new(pgxpool.Pool), hex.EncodeToString(key))
	if err != nil {
		t.Fatalf("NewIntegrationService() error = %v", err)
	}

	ciphertext, err := svc.encryptConfig("org-a", `{"token":"secret"}`)
	if err != nil {
		t.Fatalf("encryptConfig() error = %v", err)
	}
	if !strings.HasPrefix(ciphertext, "v1:") {
		t.Fatalf("ciphertext = %q, want v1 prefix", ciphertext)
	}

	plaintext, err := svc.decryptConfig("org-a", ciphertext)
	if err != nil {
		t.Fatalf("decryptConfig() error = %v", err)
	}
	if plaintext != `{"token":"secret"}` {
		t.Fatalf("plaintext = %q", plaintext)
	}

	if _, err := svc.decryptConfig("org-b", ciphertext); err == nil {
		t.Fatal("decryptConfig() accepted ciphertext copied to another tenant")
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, "v1:"))
	if err != nil {
		t.Fatalf("decode ciphertext: %v", err)
	}
	decoded[len(decoded)-1] ^= 0xff
	tampered := "v1:" + base64.StdEncoding.EncodeToString(decoded)
	if _, err := svc.decryptConfig("org-a", tampered); err == nil {
		t.Fatal("decryptConfig() accepted tampered ciphertext")
	}
}

func TestIntegrationConfigDecryptsLegacyCiphertext(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	svc, err := NewIntegrationService(new(pgxpool.Pool), hex.EncodeToString(key))
	if err != nil {
		t.Fatalf("NewIntegrationService() error = %v", err)
	}

	legacy, err := encryptLegacyIntegrationConfig(key, `{"legacy":true}`)
	if err != nil {
		t.Fatalf("encrypt legacy config: %v", err)
	}
	plaintext, err := svc.decryptConfig("org-a", legacy)
	if err != nil {
		t.Fatalf("decrypt legacy config: %v", err)
	}
	if plaintext != `{"legacy":true}` {
		t.Fatalf("plaintext = %q", plaintext)
	}
}

func TestIntegrationSecretEncryptionBindsTenantAndPurpose(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	svc, err := NewIntegrationService(new(pgxpool.Pool), hex.EncodeToString(key))
	if err != nil {
		t.Fatalf("NewIntegrationService() error = %v", err)
	}

	ciphertext, err := svc.encryptSecret("org-a", "oidc-client-secret", "secret")
	if err != nil {
		t.Fatalf("encryptSecret() error = %v", err)
	}
	plaintext, err := svc.decryptSecret("org-a", "oidc-client-secret", ciphertext)
	if err != nil || plaintext != "secret" {
		t.Fatalf("decryptSecret() = %q, %v", plaintext, err)
	}
	if _, err := svc.decryptSecret("org-b", "oidc-client-secret", ciphertext); err == nil {
		t.Fatal("decryptSecret() accepted another tenant")
	}
	if _, err := svc.decryptSecret("org-a", "different-purpose", ciphertext); err == nil {
		t.Fatal("decryptSecret() accepted another secret purpose")
	}
	if _, err := svc.decryptSecret("org-a", "oidc-client-secret", strings.TrimPrefix(ciphertext, "v1:")); err == nil {
		t.Fatal("decryptSecret() accepted an unversioned secret")
	}
}

func TestValidateSecureEndpoint(t *testing.T) {
	valid := []string{
		"https://idp.example.com/.well-known/openid-configuration",
		"http://localhost:8080/issuer",
		"http://127.0.0.1:8080/saml",
	}
	for _, endpoint := range valid {
		if err := validateSecureEndpoint(endpoint); err != nil {
			t.Errorf("validateSecureEndpoint(%q) error = %v", endpoint, err)
		}
	}

	invalid := []string{
		"idp.example.com",
		"http://idp.example.com",
		"https://user:password@idp.example.com",
		"https://idp.example.com/#fragment",
	}
	for _, endpoint := range invalid {
		if err := validateSecureEndpoint(endpoint); err == nil {
			t.Errorf("validateSecureEndpoint(%q) unexpectedly succeeded", endpoint)
		}
	}
}

func TestNormalizeSSODomains(t *testing.T) {
	input := UpdateSSOConfigurationInput{AllowedDomains: []string{" Example.COM ", "example.com", "staff.example.org"}}
	if err := normalizeSSODomains(&input); err != nil {
		t.Fatalf("normalizeSSODomains() error = %v", err)
	}
	if len(input.AllowedDomains) != 2 || input.AllowedDomains[0] != "example.com" || input.AllowedDomains[1] != "staff.example.org" {
		t.Fatalf("domains = %#v", input.AllowedDomains)
	}

	input.AllowedDomains = []string{"user@example.com"}
	if err := normalizeSSODomains(&input); err == nil {
		t.Fatal("normalizeSSODomains() accepted an email address")
	}
}

func encryptLegacyIntegrationConfig(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}
