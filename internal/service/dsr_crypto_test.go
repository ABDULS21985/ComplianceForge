package service

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDSRPIIEncryptionIsTenantAndFieldBound(t *testing.T) {
	service := &DSRService{encKey: bytes.Repeat([]byte{0x5a}, 32)}

	ciphertext, err := service.encryptPII("org-one", "email", "person@example.com")
	if err != nil {
		t.Fatalf("encryptPII() error = %v", err)
	}
	if !strings.HasPrefix(ciphertext, "v1:") || strings.Contains(ciphertext, "person@example.com") {
		t.Fatalf("encryptPII() returned unexpected ciphertext %q", ciphertext)
	}

	plaintext, err := service.decryptPII("org-one", "email", ciphertext)
	if err != nil {
		t.Fatalf("decryptPII() error = %v", err)
	}
	if plaintext != "person@example.com" {
		t.Fatalf("decryptPII() = %q, want person@example.com", plaintext)
	}

	if _, err := service.decryptPII("org-two", "email", ciphertext); err == nil {
		t.Fatal("decryptPII() with another tenant succeeded, want authentication failure")
	}
	if _, err := service.decryptPII("org-one", "name", ciphertext); err == nil {
		t.Fatal("decryptPII() with another field succeeded, want authentication failure")
	}
}

func TestDSRPIIDecryptionRejectsTampering(t *testing.T) {
	service := &DSRService{encKey: bytes.Repeat([]byte{0x7c}, 32)}
	ciphertext, err := service.encryptPII("org-one", "name", "A Person")
	if err != nil {
		t.Fatalf("encryptPII() error = %v", err)
	}

	encoded := strings.TrimPrefix(ciphertext, "v1:")
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	raw[len(raw)-1] ^= 0xff
	tampered := "v1:" + base64.StdEncoding.EncodeToString(raw)

	if _, err := service.decryptPII("org-one", "name", tampered); err == nil {
		t.Fatal("decryptPII() accepted tampered ciphertext")
	}
}

func TestNewDSRServiceFailsClosedWithoutValidKey(t *testing.T) {
	pool := new(pgxpool.Pool)
	bus := NewEventBus()

	t.Setenv("DSR_ENCRYPTION_KEY", "")
	if _, err := NewDSRService(pool, bus); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("NewDSRService() error = %v, want required-key error", err)
	}

	t.Setenv("DSR_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("too short")))
	if _, err := NewDSRService(pool, bus); err == nil || !strings.Contains(err.Error(), "32 bytes") {
		t.Fatalf("NewDSRService() error = %v, want key-length error", err)
	}
}

func TestNewDSRServiceRejectsMissingDependencies(t *testing.T) {
	t.Setenv("DSR_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)))

	if _, err := NewDSRService(nil, NewEventBus()); err == nil {
		t.Fatal("NewDSRService() accepted a nil database")
	}
	if _, err := NewDSRService(new(pgxpool.Pool), nil); err == nil {
		t.Fatal("NewDSRService() accepted a nil event bus")
	}
}

func TestDSRPIIDecryptionRejectsMalformedCiphertext(t *testing.T) {
	service := &DSRService{encKey: bytes.Repeat([]byte{0x2a}, 32)}
	for _, ciphertext := range []string{"v1:not-base64", "v1:" + base64.StdEncoding.EncodeToString([]byte("short"))} {
		if _, err := service.decryptPII("org-one", "email", ciphertext); err == nil {
			t.Fatalf("decryptPII(%q) succeeded", ciphertext)
		}
	}

	if _, err := service.decryptPII("org-one", "email", ""); err == nil {
		t.Fatal("decryptPII() accepted empty ciphertext")
	}
}
