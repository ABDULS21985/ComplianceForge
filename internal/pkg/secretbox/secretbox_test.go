package secretbox

import (
	"errors"
	"strings"
	"testing"
)

func TestBoxRoundTripAndAADIsolation(t *testing.T) {
	box, err := NewHex(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := box.Seal("tenant-a", "notification/channel-a", []byte(`{"secret":"value"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ciphertext, "value") || !strings.HasPrefix(ciphertext, "v1:") {
		t.Fatalf("unexpected ciphertext %q", ciphertext)
	}
	plaintext, err := box.Open("tenant-a", "notification/channel-a", ciphertext)
	if err != nil || string(plaintext) != `{"secret":"value"}` {
		t.Fatalf("Open() plaintext=%q error=%v", plaintext, err)
	}
	for _, test := range []struct{ tenant, purpose string }{
		{tenant: "tenant-b", purpose: "notification/channel-a"},
		{tenant: "tenant-a", purpose: "notification/channel-b"},
	} {
		if _, err := box.Open(test.tenant, test.purpose, ciphertext); !errors.Is(err, ErrInvalidCiphertext) {
			t.Fatalf("Open(%q,%q) error=%v, want ErrInvalidCiphertext", test.tenant, test.purpose, err)
		}
	}
}

func TestBoxRejectsBadKeysAndCiphertexts(t *testing.T) {
	for _, key := range []string{"", "not-hex", strings.Repeat("aa", 31)} {
		if _, err := NewHex(key); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("NewHex(%q) error=%v, want ErrInvalidKey", key, err)
		}
	}
	box, err := NewHex(strings.Repeat("cd", 32))
	if err != nil {
		t.Fatal(err)
	}
	for _, ciphertext := range []string{"plaintext", "v1:not-base64", "v1:YWJj"} {
		if _, err := box.Open("tenant", "purpose", ciphertext); !errors.Is(err, ErrInvalidCiphertext) {
			t.Fatalf("Open(%q) error=%v, want ErrInvalidCiphertext", ciphertext, err)
		}
	}
}

func TestBoxSealsJSONWithoutPlaintextMetadata(t *testing.T) {
	box, err := NewHex(strings.Repeat("ef", 32))
	if err != nil {
		t.Fatal(err)
	}
	document, err := box.SealJSON("tenant-a", "webhook", map[string]string{"secret": "never-store-me"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(document), "never-store-me") {
		t.Fatalf("sealed document leaked plaintext: %s", document)
	}
	var opened map[string]string
	if err := box.OpenJSON("tenant-a", "webhook", document, &opened); err != nil {
		t.Fatal(err)
	}
	if opened["secret"] != "never-store-me" {
		t.Fatalf("opened document = %#v", opened)
	}
	malformed := append(document[:len(document)-1], []byte(`,"plaintext":true}`)...)
	if err := box.OpenJSON("tenant-a", "webhook", malformed, &opened); !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("OpenJSON() error=%v, want ErrInvalidCiphertext", err)
	}
}
