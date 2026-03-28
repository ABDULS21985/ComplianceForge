package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// StorageService defines the interface for file storage operations.
type StorageService interface {
	// Upload stores data from the reader under the given filename and returns
	// the path or URL where the file can be retrieved.
	Upload(ctx context.Context, filename string, data io.Reader) (string, error)

	// Download returns a ReadCloser for the file at the given path.
	// The caller is responsible for closing the returned reader.
	Download(ctx context.Context, path string) (io.ReadCloser, error)

	// Delete removes the file at the given path.
	Delete(ctx context.Context, path string) error
}

// LocalStorageService implements StorageService using the local filesystem.
type LocalStorageService struct {
	basePath string
}

// NewLocalStorageService creates a new LocalStorageService rooted at basePath.
// The base directory is created if it does not exist.
func NewLocalStorageService(basePath string) (*LocalStorageService, error) {
	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base path: %w", err)
	}

	if err := os.MkdirAll(absPath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	return &LocalStorageService{basePath: absPath}, nil
}

// Upload writes the contents of data to a file under the base directory.
func (s *LocalStorageService) Upload(_ context.Context, filename string, data io.Reader) (string, error) {
	fullPath := filepath.Join(s.basePath, filepath.Clean(filename))

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file %s: %w", fullPath, err)
	}
	defer file.Close()

	if _, err := io.Copy(file, data); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", fullPath, err)
	}

	return fullPath, nil
}

// Download opens the file at the given path for reading.
func (s *LocalStorageService) Download(_ context.Context, path string) (io.ReadCloser, error) {
	fullPath := filepath.Join(s.basePath, filepath.Clean(path))

	file, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", fullPath, err)
	}

	return file, nil
}

// Delete removes the file at the given path.
func (s *LocalStorageService) Delete(_ context.Context, path string) error {
	fullPath := filepath.Join(s.basePath, filepath.Clean(path))

	if err := os.Remove(fullPath); err != nil {
		return fmt.Errorf("failed to delete file %s: %w", fullPath, err)
	}

	return nil
}
