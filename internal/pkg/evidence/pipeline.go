// Package evidence implements the security boundary between untrusted evidence
// uploads and the application's durable object store.
package evidence

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

const (
	DefaultMaximumBytes    int64 = 25 << 20
	MaximumEvidenceBytes   int64 = 2 << 30
	detectionBytes               = 512
	maximumArchiveFiles          = 2_000
	maximumArchiveRatio    int64 = 100
	maximumArchiveExpanded       = 250 << 20
)

var (
	ErrEmptyObject        = errors.New("evidence object is empty")
	ErrObjectTooLarge     = errors.New("evidence object exceeds the upload limit")
	ErrInvalidFilename    = errors.New("invalid evidence filename")
	ErrUnsupportedType    = errors.New("unsupported evidence content type")
	ErrContentMismatch    = errors.New("evidence content does not match its declared type")
	ErrUnsafeArchive      = errors.New("unsafe evidence archive")
	ErrMalwareDetected    = errors.New("malware detected in evidence object")
	ErrScannerUnavailable = errors.New("malware scanner unavailable")
)

// ObjectStore is deliberately smaller than any cloud-provider SDK. Uploads are
// written to a quarantine key first and copied to a non-public clean key only
// after a successful scan.
type ObjectStore interface {
	Upload(ctx context.Context, key string, data io.Reader) (string, error)
	Delete(ctx context.Context, key string) error
}

type ScanVerdict string

const (
	ScanClean    ScanVerdict = "clean"
	ScanInfected ScanVerdict = "infected"
)

type ScanReport struct {
	Verdict   ScanVerdict
	Engine    string
	Signature string
}

type MalwareScanner interface {
	Scan(ctx context.Context, data io.Reader) (ScanReport, error)
}

type UploadRequest struct {
	OrganizationID      string
	Filename            string
	DeclaredContentType string
	Body                io.Reader
}

type UploadResult struct {
	ObjectKey        string
	QuarantineKey    string
	OriginalFilename string
	ContentType      string
	SizeBytes        int64
	SHA256           string
	ScanVerdict      ScanVerdict
	ScanEngine       string
	MalwareSignature string
	ScannedAt        time.Time
}

// Pipeline receives a stream once, derives all security-sensitive metadata on
// the server, quarantines the object, and fails closed when scanning cannot
// conclusively mark it clean.
type Pipeline struct {
	store       ObjectStore
	scanner     MalwareScanner
	maximumSize int64
	tempDir     string
	now         func() time.Time
	newID       func() string
}

type Option func(*Pipeline)

func WithMaximumSize(bytes int64) Option {
	return func(p *Pipeline) {
		if bytes > 0 {
			p.maximumSize = bytes
		}
	}
}

func WithTemporaryDirectory(path string) Option {
	return func(p *Pipeline) { p.tempDir = path }
}

func NewPipeline(store ObjectStore, scanner MalwareScanner, options ...Option) (*Pipeline, error) {
	if store == nil {
		return nil, errors.New("evidence object store is required")
	}
	if scanner == nil {
		return nil, errors.New("evidence malware scanner is required")
	}
	pipeline := &Pipeline{
		store:       store,
		scanner:     scanner,
		maximumSize: DefaultMaximumBytes,
		now:         func() time.Time { return time.Now().UTC() },
		newID:       func() string { return uuid.NewString() },
	}
	for _, option := range options {
		option(pipeline)
	}
	if pipeline.maximumSize <= 0 || pipeline.maximumSize > MaximumEvidenceBytes {
		return nil, errors.New("evidence upload limit must be between 1 byte and 2 GiB")
	}
	return pipeline, nil
}

func (p *Pipeline) Upload(ctx context.Context, request UploadRequest) (*UploadResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(request.OrganizationID); err != nil {
		return nil, fmt.Errorf("invalid organization identifier: %w", err)
	}
	filename, err := normalizeFilename(request.Filename)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, ErrEmptyObject
	}

	temporary, err := os.CreateTemp(p.tempDir, "complianceforge-evidence-*")
	if err != nil {
		return nil, fmt.Errorf("create evidence spool: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return nil, fmt.Errorf("secure evidence spool: %w", err)
	}

	hasher := sha256.New()
	limited := &io.LimitedReader{R: &contextReader{ctx: ctx, reader: request.Body}, N: p.maximumSize + 1}
	written, err := io.Copy(io.MultiWriter(temporary, hasher), limited)
	if err != nil {
		return nil, fmt.Errorf("receive evidence object: %w", err)
	}
	if written == 0 {
		return nil, ErrEmptyObject
	}
	if written > p.maximumSize {
		return nil, ErrObjectTooLarge
	}
	if err := temporary.Sync(); err != nil {
		return nil, fmt.Errorf("sync evidence spool: %w", err)
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind evidence spool: %w", err)
	}

	contentType, err := validateContent(temporary, filename, request.DeclaredContentType, written)
	if err != nil {
		return nil, err
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind evidence spool: %w", err)
	}

	objectID := p.newID()
	quarantineKey := filepath.ToSlash(filepath.Join("quarantine", request.OrganizationID, objectID))
	storedQuarantineKey, err := p.store.Upload(ctx, quarantineKey, temporary)
	if err != nil {
		return nil, fmt.Errorf("quarantine evidence object: %w", err)
	}
	if strings.TrimSpace(storedQuarantineKey) == "" {
		storedQuarantineKey = quarantineKey
	}

	result := &UploadResult{
		QuarantineKey:    storedQuarantineKey,
		OriginalFilename: filename,
		ContentType:      contentType,
		SizeBytes:        written,
		SHA256:           hex.EncodeToString(hasher.Sum(nil)),
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return result, fmt.Errorf("rewind quarantined evidence: %w", err)
	}
	report, err := p.scanner.Scan(ctx, temporary)
	result.ScanVerdict = report.Verdict
	result.ScanEngine = strings.TrimSpace(report.Engine)
	result.MalwareSignature = sanitizeSignature(report.Signature)
	result.ScannedAt = p.now()
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrScannerUnavailable, err)
	}
	if report.Verdict != ScanClean {
		if report.Verdict == ScanInfected {
			return result, ErrMalwareDetected
		}
		return result, ErrScannerUnavailable
	}

	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return result, fmt.Errorf("rewind clean evidence: %w", err)
	}
	cleanKey := filepath.ToSlash(filepath.Join("evidence", request.OrganizationID, objectID))
	storedCleanKey, err := p.store.Upload(ctx, cleanKey, temporary)
	if err != nil {
		return result, fmt.Errorf("promote clean evidence object: %w", err)
	}
	if strings.TrimSpace(storedCleanKey) == "" {
		storedCleanKey = cleanKey
	}
	result.ObjectKey = storedCleanKey
	if err := p.store.Delete(ctx, storedQuarantineKey); err != nil {
		// A duplicate quarantine object is safer than deleting the clean copy and
		// losing an otherwise valid upload. Returning both keys lets the caller
		// persist a reconciliation job without orphaning the clean object.
		return result, nil
	}
	result.QuarantineKey = ""
	return result, nil
}

func normalizeFilename(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 255 || value != filepath.Base(value) || value == "." || value == ".." {
		return "", ErrInvalidFilename
	}
	for _, character := range value {
		if unicode.IsControl(character) || character == '/' || character == '\\' {
			return "", ErrInvalidFilename
		}
	}
	return value, nil
}

type allowedType struct {
	extensions      []string
	detected        []string
	requiresJSON    bool
	officeDirectory string
}

var allowedTypes = map[string]allowedType{
	"application/pdf":  {extensions: []string{".pdf"}, detected: []string{"application/pdf"}},
	"image/png":        {extensions: []string{".png"}, detected: []string{"image/png"}},
	"image/jpeg":       {extensions: []string{".jpg", ".jpeg"}, detected: []string{"image/jpeg"}},
	"text/plain":       {extensions: []string{".txt", ".log"}, detected: []string{"text/plain; charset=utf-8"}},
	"text/csv":         {extensions: []string{".csv"}, detected: []string{"text/plain; charset=utf-8"}},
	"application/json": {extensions: []string{".json"}, detected: []string{"text/plain; charset=utf-8", "application/json"}, requiresJSON: true},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": {
		extensions: []string{".docx"}, detected: []string{"application/zip"}, officeDirectory: "word/",
	},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": {
		extensions: []string{".xlsx"}, detected: []string{"application/zip"}, officeDirectory: "xl/",
	},
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": {
		extensions: []string{".pptx"}, detected: []string{"application/zip"}, officeDirectory: "ppt/",
	},
}

func validateContent(file *os.File, filename, declared string, size int64) (string, error) {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(declared))
	if err != nil {
		return "", ErrUnsupportedType
	}
	mediaType = strings.ToLower(mediaType)
	rule, ok := allowedTypes[mediaType]
	if !ok {
		return "", ErrUnsupportedType
	}
	extension := strings.ToLower(filepath.Ext(filename))
	if !contains(rule.extensions, extension) {
		return "", ErrContentMismatch
	}

	header := make([]byte, detectionBytes)
	count, err := io.ReadFull(file, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", fmt.Errorf("inspect evidence content: %w", err)
	}
	header = header[:count]
	detected := http.DetectContentType(header)
	if !contains(rule.detected, detected) {
		return "", ErrContentMismatch
	}
	if rule.requiresJSON && !json.Valid(readSmallFile(file, size)) {
		return "", ErrContentMismatch
	}
	if rule.officeDirectory != "" {
		if err := validateOfficeArchive(file, size, rule.officeDirectory); err != nil {
			return "", err
		}
	}
	return mediaType, nil
}

func readSmallFile(file *os.File, size int64) []byte {
	if file == nil || size <= 0 || size > DefaultMaximumBytes {
		return nil
	}
	// Inspect the already-open private spool, not a reopened pathname. The
	// section is independently bounded and does not change the stream offset.
	contents, err := io.ReadAll(io.NewSectionReader(file, 0, size+1))
	if err != nil || int64(len(contents)) != size {
		return nil
	}
	return contents
}

func validateOfficeArchive(file *os.File, compressedSize int64, requiredDirectory string) error {
	if file == nil || compressedSize <= 0 || compressedSize > MaximumEvidenceBytes {
		return ErrUnsafeArchive
	}
	archive, err := zip.NewReader(file, compressedSize)
	if err != nil {
		return ErrContentMismatch
	}
	if len(archive.File) == 0 || len(archive.File) > maximumArchiveFiles {
		return ErrUnsafeArchive
	}
	var hasContentTypes, hasOfficeDirectory bool
	var expanded uint64
	// The positive 2-GiB bound above makes conversion safe. Multiply after
	// conversion; never multiply a caller-provided signed size before checking.
	maximumExpanded := uint64(compressedSize) * uint64(maximumArchiveRatio)
	if maximumExpanded > uint64(maximumArchiveExpanded) {
		maximumExpanded = uint64(maximumArchiveExpanded)
	}
	for _, entry := range archive.File {
		name := filepath.ToSlash(entry.Name)
		clean := filepath.ToSlash(filepath.Clean(name))
		lowerName := strings.ToLower(name)
		if strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || clean == ".." || strings.HasPrefix(clean, "../") ||
			strings.ContainsRune(name, '\x00') || entry.Mode()&os.ModeSymlink != 0 || entry.Flags&1 != 0 ||
			strings.Contains(lowerName, "vbaproject.bin") || strings.Contains(lowerName, "/activex/") ||
			strings.Contains(lowerName, "/embeddings/") || strings.Contains(lowerName, "/externallinks/") {
			return ErrUnsafeArchive
		}
		if entry.UncompressedSize64 > maximumExpanded-expanded {
			return ErrUnsafeArchive
		}
		expanded += entry.UncompressedSize64
		if name == "[Content_Types].xml" {
			hasContentTypes = true
		}
		if strings.HasPrefix(name, requiredDirectory) {
			hasOfficeDirectory = true
		}
	}
	if !hasContentTypes || !hasOfficeDirectory {
		return ErrContentMismatch
	}
	return nil
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func sanitizeSignature(value string) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, strings.TrimSpace(value))
	if len(value) > 200 {
		value = value[:200]
	}
	return value
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
