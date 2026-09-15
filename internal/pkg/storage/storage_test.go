package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStorageRoundTrip(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalStorageService(root)
	if err != nil {
		t.Fatalf("NewLocalStorageService() error = %v", err)
	}

	storedPath, err := store.Upload(context.Background(), "tenant/evidence.txt", strings.NewReader("evidence"))
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if !filepath.IsAbs(storedPath) {
		t.Fatalf("Upload() path = %q, want absolute path", storedPath)
	}

	reader, err := store.Download(context.Background(), storedPath)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	contents, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read error = %v, close error = %v", readErr, closeErr)
	}
	if string(contents) != "evidence" {
		t.Fatalf("contents = %q, want evidence", contents)
	}

	if err := store.Delete(context.Background(), storedPath); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := os.Stat(storedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted object Stat() error = %v, want not exist", err)
	}
}

func TestLocalStorageVerifiesImmutableObjectDigestAndSize(t *testing.T) {
	store, err := NewLocalStorageService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("verified evidence")
	path, err := store.Upload(context.Background(), "tenant/evidence.bin", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	hash := hex.EncodeToString(digest[:])
	if err := store.Verify(context.Background(), path, hash, int64(len(payload))); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	for _, test := range []struct {
		name string
		hash string
		size int64
	}{
		{name: "digest mismatch", hash: strings.Repeat("0", 64), size: int64(len(payload))},
		{name: "size mismatch", hash: hash, size: int64(len(payload) + 1)},
		{name: "invalid digest", hash: "not-sha256", size: int64(len(payload))},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := store.Verify(context.Background(), path, test.hash, test.size); !errors.Is(err, ErrIntegrityMismatch) {
				t.Fatalf("Verify() error = %v", err)
			}
		})
	}
}

func TestLocalStorageRejectsPathsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalStorageService(root)
	if err != nil {
		t.Fatalf("NewLocalStorageService() error = %v", err)
	}

	paths := []string{
		"../outside.txt",
		filepath.Join(root, "..", "outside.txt"),
		".",
		"",
		"bad\x00path",
	}
	for _, path := range paths {
		t.Run(strings.ReplaceAll(path, string(filepath.Separator), "_"), func(t *testing.T) {
			if _, err := store.Upload(context.Background(), path, strings.NewReader("no")); !errors.Is(err, ErrInvalidPath) {
				t.Fatalf("Upload(%q) error = %v, want ErrInvalidPath", path, err)
			}
			if _, err := store.Download(context.Background(), path); !errors.Is(err, ErrInvalidPath) {
				t.Fatalf("Download(%q) error = %v, want ErrInvalidPath", path, err)
			}
			if err := store.Delete(context.Background(), path); !errors.Is(err, ErrInvalidPath) {
				t.Fatalf("Delete(%q) error = %v, want ErrInvalidPath", path, err)
			}
		})
	}
}

func TestLocalStorageNeverOverwritesAnExistingObjectKey(t *testing.T) {
	store, err := NewLocalStorageService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.Upload(context.Background(), "tenant/immutable.txt", strings.NewReader("original"))
	if err != nil {
		t.Fatalf("first Upload() error = %v", err)
	}
	if _, err := store.Upload(context.Background(), "tenant/immutable.txt", strings.NewReader("replacement")); err == nil {
		t.Fatal("second Upload() error = nil")
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "original" {
		t.Fatalf("stored contents=%q error=%v", contents, err)
	}
}

func TestLocalStorageRejectsSymlinkTraversal(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	store, err := NewLocalStorageService(root)
	if err != nil {
		t.Fatalf("NewLocalStorageService() error = %v", err)
	}

	link := filepath.Join(root, "tenant")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	if _, err := store.Upload(context.Background(), "tenant/evidence.txt", strings.NewReader("no")); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("Upload() error = %v, want ErrInvalidPath", err)
	}
	if _, err := store.Download(context.Background(), "tenant/evidence.txt"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("Download() error = %v, want ErrInvalidPath", err)
	}
	if err := store.Delete(context.Background(), "tenant/evidence.txt"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("Delete() error = %v, want ErrInvalidPath", err)
	}
}

func TestLocalStorageRejectsSymlinkDownload(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store, err := NewLocalStorageService(root)
	if err != nil {
		t.Fatalf("NewLocalStorageService() error = %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "evidence.txt")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	if _, err := store.Download(context.Background(), "evidence.txt"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("Download() error = %v, want ErrInvalidPath", err)
	}
}

func TestLocalStorageHonorsCanceledContext(t *testing.T) {
	store, err := NewLocalStorageService(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStorageService() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.Upload(ctx, "evidence.txt", strings.NewReader("evidence")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Upload() error = %v, want context.Canceled", err)
	}
	if _, err := store.Download(ctx, "evidence.txt"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Download() error = %v, want context.Canceled", err)
	}
	if err := store.Delete(ctx, "evidence.txt"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete() error = %v, want context.Canceled", err)
	}
}
