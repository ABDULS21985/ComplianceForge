package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewTranslatorLoadsRegularLocaleFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{"common":{"save":"Save"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	translator, err := NewTranslator(dir, "en")
	if err != nil {
		t.Fatalf("NewTranslator() error = %v", err)
	}
	if got := translator.T("en", "common.save"); got != "Save" {
		t.Fatalf("translation = %q, want Save", got)
	}
}

func TestNewTranslatorRejectsSymlinkedLocaleOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"secret":"outside"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "en.json")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	if _, err := NewTranslator(dir, "en"); err == nil {
		t.Fatal("expected symlinked locale file to be rejected")
	}
}
