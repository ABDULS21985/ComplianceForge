package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrInvalidPath is returned when a caller attempts to address data outside
	// the configured storage root or traverse the root through a symbolic link.
	ErrInvalidPath = errors.New("invalid storage path")
	// ErrNotRegularFile is returned when a download target is not a regular file.
	ErrNotRegularFile = errors.New("storage object is not a regular file")
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

// NewLocalStorageService creates a LocalStorageService rooted at basePath.
// The canonical base directory is created with owner/group-only permissions.
func NewLocalStorageService(basePath string) (*LocalStorageService, error) {
	if strings.TrimSpace(basePath) == "" {
		return nil, fmt.Errorf("%w: base path is empty", ErrInvalidPath)
	}

	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("resolve storage root: %w", err)
	}
	if err := os.MkdirAll(absPath, 0o750); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}

	canonicalPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return nil, fmt.Errorf("resolve canonical storage root: %w", err)
	}
	info, err := os.Stat(canonicalPath)
	if err != nil {
		return nil, fmt.Errorf("inspect storage root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: storage root is not a directory", ErrInvalidPath)
	}

	return &LocalStorageService{basePath: canonicalPath}, nil
}

// Upload streams data to an owner-readable temporary file and atomically moves
// it into place. Traversal and existing symbolic-link components are rejected.
func (s *LocalStorageService) Upload(ctx context.Context, filename string, data io.Reader) (string, error) {
	if data == nil {
		return "", errors.New("upload data is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	fullPath, relativePath, err := s.resolve(filename)
	if err != nil {
		return "", err
	}
	if err := s.rejectSymlinks(relativePath, true); err != nil {
		return "", err
	}

	root, err := os.OpenRoot(s.basePath)
	if err != nil {
		return "", fmt.Errorf("open storage root: %w", err)
	}
	defer root.Close()

	if err := root.MkdirAll(filepath.Dir(relativePath), 0o750); err != nil {
		return "", fmt.Errorf("create storage directory: %w", err)
	}
	if err := s.rejectSymlinks(relativePath, true); err != nil {
		return "", err
	}

	// The temporary object is created directly under the canonical root. Root's
	// rename operation then proves both names remain contained even if a parent
	// path is changed concurrently.
	temporary, err := os.CreateTemp(s.basePath, ".upload-*")
	if err != nil {
		return "", fmt.Errorf("create temporary storage object: %w", err)
	}
	temporaryPath := temporary.Name()
	keepTemporary := false
	defer func() {
		_ = temporary.Close()
		if !keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := temporary.Chmod(0o600); err != nil {
		return "", fmt.Errorf("secure temporary storage object: %w", err)
	}
	if _, err := io.Copy(temporary, &contextReader{ctx: ctx, reader: data}); err != nil {
		return "", fmt.Errorf("write storage object: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return "", fmt.Errorf("sync storage object: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close storage object: %w", err)
	}

	// Recheck immediately before the rename so a pre-existing symlink is never
	// followed as the destination. Rename replaces a leaf symlink atomically.
	if err := s.rejectSymlinks(relativePath, false); err != nil {
		return "", err
	}
	if err := root.Rename(filepath.Base(temporaryPath), relativePath); err != nil {
		return "", fmt.Errorf("commit storage object: %w", err)
	}
	keepTemporary = true

	return fullPath, nil
}

// Download opens a regular file beneath the storage root for reading.
func (s *LocalStorageService) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	_, relativePath, err := s.resolve(path)
	if err != nil {
		return nil, err
	}
	if err := s.rejectSymlinks(relativePath, true); err != nil {
		return nil, err
	}

	// os.Root makes the containment check part of the open operation, closing
	// the TOCTOU escape possible with a string-only path validation.
	root, err := os.OpenRoot(s.basePath)
	if err != nil {
		return nil, fmt.Errorf("open storage root: %w", err)
	}
	file, err := root.Open(relativePath)
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("open storage object: %w", err)
	}
	if err := root.Close(); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("close storage root: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect storage object: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, ErrNotRegularFile
	}

	return file, nil
}

// Delete removes a file beneath the storage root without following symlinks in
// any parent component.
func (s *LocalStorageService) Delete(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	_, relativePath, err := s.resolve(path)
	if err != nil {
		return err
	}
	if err := s.rejectSymlinks(relativePath, false); err != nil {
		return err
	}

	root, err := os.OpenRoot(s.basePath)
	if err != nil {
		return fmt.Errorf("open storage root: %w", err)
	}
	defer root.Close()

	info, err := root.Lstat(relativePath)
	if err != nil {
		return fmt.Errorf("inspect storage object: %w", err)
	}
	if info.IsDir() {
		return ErrNotRegularFile
	}
	if err := root.Remove(relativePath); err != nil {
		return fmt.Errorf("delete storage object: %w", err)
	}
	return nil
}

// resolve accepts either a relative object key or a previously returned
// absolute local path, then proves that it remains under the canonical root.
func (s *LocalStorageService) resolve(path string) (fullPath, relativePath string, err error) {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, '\x00') {
		return "", "", ErrInvalidPath
	}

	if filepath.IsAbs(path) {
		fullPath = filepath.Clean(path)
	} else {
		fullPath = filepath.Join(s.basePath, filepath.Clean(filepath.FromSlash(path)))
	}

	relativePath, err = filepath.Rel(s.basePath, fullPath)
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || filepath.IsAbs(relativePath) {
		return "", "", ErrInvalidPath
	}
	return fullPath, relativePath, nil
}

// rejectSymlinks walks existing path components. Missing components are valid
// for uploads; any symbolic link is rejected even when it points back inside the
// storage root, keeping object addressing deterministic and auditable.
func (s *LocalStorageService) rejectSymlinks(relativePath string, includeLeaf bool) error {
	parts := strings.Split(relativePath, string(filepath.Separator))
	limit := len(parts)
	if !includeLeaf {
		limit--
	}

	current := s.basePath
	for i := 0; i < limit; i++ {
		current = filepath.Join(current, parts[i])
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect storage path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrInvalidPath
		}
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
