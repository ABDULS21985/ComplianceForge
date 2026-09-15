package evidence

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

const evidenceTestOrgID = "10000000-0000-0000-0000-000000000001"

type memoryStore struct {
	objects    map[string][]byte
	deleteErr  error
	uploadKeys []string
}

func newMemoryStore() *memoryStore { return &memoryStore{objects: make(map[string][]byte)} }

func (s *memoryStore) Upload(_ context.Context, key string, data io.Reader) (string, error) {
	contents, err := io.ReadAll(data)
	if err != nil {
		return "", err
	}
	s.objects[key] = contents
	s.uploadKeys = append(s.uploadKeys, key)
	return key, nil
}

func (s *memoryStore) Delete(_ context.Context, key string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.objects, key)
	return nil
}

type scannerFunc func(context.Context, io.Reader) (ScanReport, error)

func (function scannerFunc) Scan(ctx context.Context, data io.Reader) (ScanReport, error) {
	return function(ctx, data)
}

func cleanScanner(t *testing.T, expected []byte) MalwareScanner {
	t.Helper()
	return scannerFunc(func(_ context.Context, data io.Reader) (ScanReport, error) {
		actual, err := io.ReadAll(data)
		if err != nil {
			t.Fatalf("read scan stream: %v", err)
		}
		if expected != nil && !bytes.Equal(actual, expected) {
			t.Fatalf("scan content = %q, want %q", actual, expected)
		}
		return ScanReport{Verdict: ScanClean, Engine: "test-scanner"}, nil
	})
}

func newTestPipeline(t *testing.T, store *memoryStore, scanner MalwareScanner, maximum int64) *Pipeline {
	t.Helper()
	pipeline, err := NewPipeline(store, scanner, WithMaximumSize(maximum), WithTemporaryDirectory(t.TempDir()))
	if err != nil {
		t.Fatalf("NewPipeline() error = %v", err)
	}
	pipeline.newID = func() string { return "20000000-0000-0000-0000-000000000002" }
	pipeline.now = func() time.Time { return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC) }
	return pipeline
}

func TestPipelinePromotesOnlyCleanServerValidatedContent(t *testing.T) {
	contents := []byte("%PDF-1.7\nenterprise evidence\n%%EOF")
	store := newMemoryStore()
	pipeline := newTestPipeline(t, store, cleanScanner(t, contents), 1<<20)

	result, err := pipeline.Upload(context.Background(), UploadRequest{
		OrganizationID:      evidenceTestOrgID,
		Filename:            "annual-control.pdf",
		DeclaredContentType: "application/pdf; charset=binary",
		Body:                bytes.NewReader(contents),
	})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	wantKey := "evidence/" + evidenceTestOrgID + "/20000000-0000-0000-0000-000000000002"
	if result.ObjectKey != wantKey || result.QuarantineKey != "" {
		t.Fatalf("keys = clean %q quarantine %q, want clean %q", result.ObjectKey, result.QuarantineKey, wantKey)
	}
	if result.SHA256 != "58f2d645283144a03de37a6ca5d71ece00fac9d834061546cb5f210a304f8cc2" {
		t.Fatalf("SHA256 = %q", result.SHA256)
	}
	if result.ContentType != "application/pdf" || result.SizeBytes != int64(len(contents)) || result.ScanVerdict != ScanClean || result.ScanEngine != "test-scanner" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !bytes.Equal(store.objects[wantKey], contents) || len(store.objects) != 1 {
		t.Fatalf("stored objects = %#v", store.objects)
	}
	if len(store.uploadKeys) != 2 || !strings.HasPrefix(store.uploadKeys[0], "quarantine/") {
		t.Fatalf("upload order = %#v", store.uploadKeys)
	}
}

func TestPipelineKeepsInfectedAndIndeterminateObjectsQuarantined(t *testing.T) {
	contents := []byte("plain evidence")
	tests := []struct {
		name      string
		scanner   MalwareScanner
		wantError error
	}{
		{
			name: "infected",
			scanner: scannerFunc(func(context.Context, io.Reader) (ScanReport, error) {
				return ScanReport{Verdict: ScanInfected, Engine: "clamav", Signature: " EICAR\r\n"}, nil
			}),
			wantError: ErrMalwareDetected,
		},
		{
			name: "scanner unavailable",
			scanner: scannerFunc(func(context.Context, io.Reader) (ScanReport, error) {
				return ScanReport{}, errors.New("socket path and internal detail")
			}),
			wantError: ErrScannerUnavailable,
		},
		{
			name: "unknown verdict",
			scanner: scannerFunc(func(context.Context, io.Reader) (ScanReport, error) {
				return ScanReport{Engine: "clamav"}, nil
			}),
			wantError: ErrScannerUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newMemoryStore()
			pipeline := newTestPipeline(t, store, test.scanner, 1<<20)
			result, err := pipeline.Upload(context.Background(), UploadRequest{
				OrganizationID: evidenceTestOrgID, Filename: "control.txt", DeclaredContentType: "text/plain", Body: bytes.NewReader(contents),
			})
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Upload() error = %v, want %v", err, test.wantError)
			}
			if result == nil || result.ObjectKey != "" || result.QuarantineKey == "" || len(store.objects) != 1 {
				t.Fatalf("quarantine result = %#v objects=%#v", result, store.objects)
			}
			if test.name == "infected" && result.MalwareSignature != "EICAR" {
				t.Fatalf("signature = %q", result.MalwareSignature)
			}
		})
	}
}

func TestPipelineReportsQuarantineCleanupForReconciliation(t *testing.T) {
	contents := []byte("safe evidence")
	store := newMemoryStore()
	store.deleteErr = errors.New("temporary object-store failure")
	pipeline := newTestPipeline(t, store, cleanScanner(t, contents), 1<<20)
	result, err := pipeline.Upload(context.Background(), UploadRequest{
		OrganizationID: evidenceTestOrgID, Filename: "safe.txt", DeclaredContentType: "text/plain", Body: bytes.NewReader(contents),
	})
	if err != nil || result.ObjectKey == "" || result.QuarantineKey == "" || len(store.objects) != 2 {
		t.Fatalf("Upload() result=%#v error=%v objects=%d", result, err, len(store.objects))
	}
}

func TestPipelineRejectsUntrustedMetadataBeforeStorage(t *testing.T) {
	executable := append([]byte("MZ"), bytes.Repeat([]byte{0}, 600)...)
	tests := []struct {
		name      string
		request   UploadRequest
		maximum   int64
		wantError error
	}{
		{name: "empty", request: UploadRequest{OrganizationID: evidenceTestOrgID, Filename: "a.txt", DeclaredContentType: "text/plain", Body: bytes.NewReader(nil)}, maximum: 100, wantError: ErrEmptyObject},
		{name: "too large", request: UploadRequest{OrganizationID: evidenceTestOrgID, Filename: "a.txt", DeclaredContentType: "text/plain", Body: strings.NewReader("123456")}, maximum: 5, wantError: ErrObjectTooLarge},
		{name: "path filename", request: UploadRequest{OrganizationID: evidenceTestOrgID, Filename: "../a.pdf", DeclaredContentType: "application/pdf", Body: strings.NewReader("%PDF-x")}, maximum: 100, wantError: ErrInvalidFilename},
		{name: "control filename", request: UploadRequest{OrganizationID: evidenceTestOrgID, Filename: "a\r\n.txt", DeclaredContentType: "text/plain", Body: strings.NewReader("hello")}, maximum: 100, wantError: ErrInvalidFilename},
		{name: "unsupported active type", request: UploadRequest{OrganizationID: evidenceTestOrgID, Filename: "a.svg", DeclaredContentType: "image/svg+xml", Body: strings.NewReader("<svg></svg>")}, maximum: 100, wantError: ErrUnsupportedType},
		{name: "extension mismatch", request: UploadRequest{OrganizationID: evidenceTestOrgID, Filename: "a.txt", DeclaredContentType: "application/pdf", Body: strings.NewReader("%PDF-x")}, maximum: 100, wantError: ErrContentMismatch},
		{name: "magic mismatch", request: UploadRequest{OrganizationID: evidenceTestOrgID, Filename: "a.pdf", DeclaredContentType: "application/pdf", Body: strings.NewReader("not a pdf")}, maximum: 100, wantError: ErrContentMismatch},
		{name: "executable renamed text", request: UploadRequest{OrganizationID: evidenceTestOrgID, Filename: "a.txt", DeclaredContentType: "text/plain", Body: bytes.NewReader(executable)}, maximum: 1 << 20, wantError: ErrContentMismatch},
		{name: "invalid json", request: UploadRequest{OrganizationID: evidenceTestOrgID, Filename: "a.json", DeclaredContentType: "application/json", Body: strings.NewReader("{no")}, maximum: 100, wantError: ErrContentMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newMemoryStore()
			pipeline := newTestPipeline(t, store, cleanScanner(t, nil), test.maximum)
			_, err := pipeline.Upload(context.Background(), test.request)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Upload() error = %v, want %v", err, test.wantError)
			}
			if len(store.objects) != 0 {
				t.Fatalf("rejected upload stored %d objects", len(store.objects))
			}
		})
	}
}

func TestPipelineRejectsInvalidTenantBeforeStorage(t *testing.T) {
	store := newMemoryStore()
	pipeline := newTestPipeline(t, store, cleanScanner(t, nil), 100)
	_, err := pipeline.Upload(context.Background(), UploadRequest{
		OrganizationID: "not-a-tenant", Filename: "a.txt", DeclaredContentType: "text/plain", Body: strings.NewReader("hello"),
	})
	if err == nil || len(store.objects) != 0 {
		t.Fatalf("Upload() error = %v objects=%d", err, len(store.objects))
	}
}

func TestPipelineValidatesOpenXMLContainer(t *testing.T) {
	valid := officeArchive(t, map[string]string{
		"[Content_Types].xml": "<Types/>",
		"word/document.xml":   "<document>evidence</document>",
	})
	store := newMemoryStore()
	pipeline := newTestPipeline(t, store, cleanScanner(t, valid), 1<<20)
	if _, err := pipeline.Upload(context.Background(), UploadRequest{
		OrganizationID:      evidenceTestOrgID,
		Filename:            "evidence.docx",
		DeclaredContentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		Body:                bytes.NewReader(valid),
	}); err != nil {
		t.Fatalf("valid docx Upload() error = %v", err)
	}

	unsafe := officeArchive(t, map[string]string{
		"[Content_Types].xml": "<Types/>",
		"word/document.xml":   "<document/>",
		"../escape":           "unsafe",
	})
	store = newMemoryStore()
	pipeline = newTestPipeline(t, store, cleanScanner(t, nil), 1<<20)
	_, err := pipeline.Upload(context.Background(), UploadRequest{
		OrganizationID:      evidenceTestOrgID,
		Filename:            "unsafe.docx",
		DeclaredContentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		Body:                bytes.NewReader(unsafe),
	})
	if !errors.Is(err, ErrUnsafeArchive) || len(store.objects) != 0 {
		t.Fatalf("unsafe docx error = %v objects=%d", err, len(store.objects))
	}
}

func officeArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for name, contents := range files {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatalf("create archive entry: %v", err)
		}
		if _, err := fmt.Fprint(entry, contents); err != nil {
			t.Fatalf("write archive entry: %v", err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return output.Bytes()
}

func TestNewPipelineRequiresSecurityDependencies(t *testing.T) {
	store := newMemoryStore()
	scanner := scannerFunc(func(context.Context, io.Reader) (ScanReport, error) { return ScanReport{Verdict: ScanClean}, nil })
	if _, err := NewPipeline(nil, scanner); err == nil {
		t.Fatal("NewPipeline(nil store) error = nil")
	}
	if _, err := NewPipeline(store, nil); err == nil {
		t.Fatal("NewPipeline(nil scanner) error = nil")
	}
}

func TestPipelineRejectsOversizedConfiguredBudgetsBeforeReceivingBytes(t *testing.T) {
	store := newMemoryStore()
	scanner := scannerFunc(func(context.Context, io.Reader) (ScanReport, error) {
		t.Fatal("oversized configuration reached scanner")
		return ScanReport{}, nil
	})
	for _, maximum := range []int64{MaximumEvidenceBytes + 1, math.MaxInt64} {
		if _, err := NewPipeline(store, scanner, WithMaximumSize(maximum)); err == nil {
			t.Fatalf("unbounded maximum %d accepted", maximum)
		}
	}
	if _, err := NewPipeline(store, scanner, WithMaximumSize(MaximumEvidenceBytes)); err != nil {
		t.Fatalf("reviewed 2-GiB maximum rejected: %v", err)
	}
}

func TestEvidenceJSONInspectionUsesBoundedOpenSpoolAndPreservesOffset(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "private-json-*")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	contents := []byte(`{"safe":true}`)
	if _, err := file.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(file.Name(), file.Name()+".private"); err != nil {
		t.Fatal(err)
	}
	if got := readSmallFile(file, int64(len(contents))); !bytes.Equal(got, contents) {
		t.Fatalf("JSON inspection reopened a pathname or changed bytes: %q", got)
	}
	position, err := file.Seek(0, io.SeekCurrent)
	if err != nil || position != int64(len(contents)) {
		t.Fatalf("inspection changed stream position=%d error=%v", position, err)
	}
	for _, size := range []int64{-1, 0, int64(len(contents) - 1), int64(len(contents) + 1), DefaultMaximumBytes + 1, math.MaxInt64} {
		if got := readSmallFile(file, size); got != nil {
			t.Fatalf("invalid/mismatched JSON size %d accepted", size)
		}
	}
}

func TestEvidenceArchiveSizeConversionAndExpandedAdditionFailClosed(t *testing.T) {
	contents := officeArchive(t, map[string]string{"[Content_Types].xml": "<Types/>", "word/document.xml": "<document/>"})
	file, err := os.CreateTemp(t.TempDir(), "private-office-*")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.Write(contents); err != nil {
		t.Fatal(err)
	}
	for _, size := range []int64{-1, 0, MaximumEvidenceBytes + 1, math.MaxInt64} {
		if err := validateOfficeArchive(file, size, "word/"); !errors.Is(err, ErrUnsafeArchive) {
			t.Fatalf("invalid compressed size %d error=%v", size, err)
		}
	}
	if err := os.Rename(file.Name(), file.Name()+".private"); err != nil {
		t.Fatal(err)
	}
	if err := validateOfficeArchive(file, int64(len(contents)), "word/"); err != nil {
		t.Fatalf("valid open spool archive failed: %v", err)
	}
	// Forge an extreme ZIP64 central-directory expanded size. No bytes are
	// expanded: the reviewed budget must reject it before addition can wrap.
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, header := range []zip.FileHeader{
		{Name: "[Content_Types].xml", Method: zip.Store, UncompressedSize64: 1, CompressedSize64: 1},
		{Name: "word/document.xml", Method: zip.Store, UncompressedSize64: math.MaxUint64, CompressedSize64: 1},
	} {
		entry, err := writer.CreateRaw(&header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte{'x'}); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	forged, err := os.CreateTemp(t.TempDir(), "forged-office-*")
	if err != nil {
		t.Fatal(err)
	}
	defer forged.Close()
	if _, err := forged.Write(output.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := validateOfficeArchive(forged, int64(output.Len()), "word/"); !errors.Is(err, ErrUnsafeArchive) {
		t.Fatalf("expanded ZIP64 size overflow accepted: %v", err)
	}
}
