package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	evidencepipeline "github.com/complianceforge/platform/internal/pkg/evidence"
	repositorypkg "github.com/complianceforge/platform/internal/repository"
)

const (
	evidenceServiceOrgID     = "10000000-0000-0000-0000-000000000001"
	evidenceServiceUserID    = "20000000-0000-0000-0000-000000000002"
	evidenceServiceControlID = "30000000-0000-0000-0000-000000000003"
	evidenceServiceObjectID  = "40000000-0000-0000-0000-000000000004"
)

type evidenceRepositoryStub struct {
	attachInput    models.AttachControlEvidenceInput
	attachErr      error
	getItem        *models.ControlEvidence
	getErr         error
	reviewInput    models.ReviewControlEvidenceInput
	reviewErr      error
	supersedeInput models.AttachControlEvidenceInput
	supersedeErr   error
}

func (stub *evidenceRepositoryStub) AttachEvidence(_ context.Context, _, _, _ string, input models.AttachControlEvidenceInput) (*models.ControlEvidence, error) {
	stub.attachInput = input
	if stub.attachErr != nil {
		return nil, stub.attachErr
	}
	return &models.ControlEvidence{BaseModel: models.BaseModel{ID: evidenceServiceObjectID}, ObjectKey: input.ObjectKey, FileName: input.FileName, FileSizeBytes: input.FileSizeBytes, FileHash: input.FileHash, Metadata: input.Metadata}, nil
}

func (stub *evidenceRepositoryStub) GetEvidence(context.Context, string, string, string) (*models.ControlEvidence, error) {
	return stub.getItem, stub.getErr
}

func (stub *evidenceRepositoryStub) ReviewEvidence(_ context.Context, _, _, _, _ string, input models.ReviewControlEvidenceInput) (*models.ControlEvidence, error) {
	stub.reviewInput = input
	if stub.reviewErr != nil {
		return nil, stub.reviewErr
	}
	return &models.ControlEvidence{BaseModel: models.BaseModel{ID: evidenceServiceObjectID}, ReviewStatus: input.Status, ReviewNotes: input.Comment}, nil
}

func (stub *evidenceRepositoryStub) SupersedeEvidence(_ context.Context, _, _, _, _ string, input models.AttachControlEvidenceInput) (*models.ControlEvidence, error) {
	stub.supersedeInput = input
	if stub.supersedeErr != nil {
		return nil, stub.supersedeErr
	}
	return &models.ControlEvidence{BaseModel: models.BaseModel{ID: evidenceServiceObjectID}, ObjectKey: input.ObjectKey, FileName: input.FileName, FileSizeBytes: input.FileSizeBytes, FileHash: input.FileHash, Metadata: input.Metadata, VersionReason: input.VersionReason}, nil
}

type evidencePipelineStub struct {
	request evidencepipeline.UploadRequest
	result  *evidencepipeline.UploadResult
	err     error
}

func (stub *evidencePipelineStub) Upload(_ context.Context, request evidencepipeline.UploadRequest) (*evidencepipeline.UploadResult, error) {
	stub.request = request
	return stub.result, stub.err
}

type evidenceStorageStub struct {
	deleted      []string
	body         string
	err          error
	verifiedKey  string
	verifiedHash string
	verifiedSize int64
	verifyErr    error
}

func (stub *evidenceStorageStub) Download(context.Context, string) (io.ReadCloser, error) {
	if stub.err != nil {
		return nil, stub.err
	}
	return io.NopCloser(strings.NewReader(stub.body)), nil
}

func (stub *evidenceStorageStub) Delete(_ context.Context, key string) error {
	stub.deleted = append(stub.deleted, key)
	return stub.err
}

func (stub *evidenceStorageStub) Verify(_ context.Context, key, expectedSHA256 string, expectedSize int64) error {
	stub.verifiedKey, stub.verifiedHash, stub.verifiedSize = key, expectedSHA256, expectedSize
	return stub.verifyErr
}

type signedEvidenceStorageStub struct {
	evidenceStorageStub
	key      string
	filename string
	lifetime time.Duration
	url      string
}

func (stub *signedEvidenceStorageStub) PresignDownload(_ context.Context, key, filename string, lifetime time.Duration) (string, error) {
	stub.key, stub.filename, stub.lifetime = key, filename, lifetime
	return stub.url, stub.err
}

func newEvidenceServiceForTest(t *testing.T, repository *evidenceRepositoryStub, pipeline *evidencePipelineStub, storage EvidenceObjectStorage) *EvidenceObjectService {
	t.Helper()
	service, err := NewEvidenceObjectService(repository, pipeline, storage, 5*time.Minute, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewEvidenceObjectService() error = %v", err)
	}
	return service
}

func cleanEvidenceUploadResult() *evidencepipeline.UploadResult {
	return &evidencepipeline.UploadResult{
		ObjectKey: "evidence/tenant/object", OriginalFilename: "control.pdf", ContentType: "application/pdf",
		SizeBytes: 42, SHA256: strings.Repeat("a", 64), ScanVerdict: evidencepipeline.ScanClean,
		ScanEngine: "clamav", ScannedAt: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
	}
}

func TestEvidenceObjectServicePersistsOnlyServerDerivedFileMetadata(t *testing.T) {
	repository := &evidenceRepositoryStub{}
	pipeline := &evidencePipelineStub{result: cleanEvidenceUploadResult()}
	storage := &evidenceStorageStub{}
	service := newEvidenceServiceForTest(t, repository, pipeline, storage)
	description := "Quarterly access review"
	item, err := service.Upload(context.Background(), evidenceServiceOrgID, evidenceServiceUserID, evidenceServiceControlID,
		"control.pdf", "application/pdf", bytes.NewReader([]byte("%PDF-content")), EvidenceUploadMetadata{
			Title: "Access review", Description: &description, EvidenceType: "document", Metadata: json.RawMessage(`{"source":"manual"}`),
		})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	input := repository.attachInput
	if item == nil || input.ObjectKey == nil || *input.ObjectKey != pipeline.result.ObjectKey || input.FileName == nil || *input.FileName != "control.pdf" ||
		input.FileSizeBytes == nil || *input.FileSizeBytes != 42 || input.MIMEType == nil || *input.MIMEType != "application/pdf" ||
		input.FileHash == nil || *input.FileHash != strings.Repeat("a", 64) {
		t.Fatalf("trusted attachment = %#v", input)
	}
	var metadata map[string]any
	if err := json.Unmarshal(input.Metadata, &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	security, ok := metadata["object_security"].(map[string]any)
	if !ok || security["checksum_source"] != "server_sha256" || security["scan_engine"] != "clamav" || metadata["source"] != "manual" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if pipeline.request.OrganizationID != evidenceServiceOrgID || pipeline.request.Filename != "control.pdf" {
		t.Fatalf("pipeline request = %#v", pipeline.request)
	}
}

func TestEvidenceObjectServiceDeletesPromotedObjectWhenDatabaseCommitFails(t *testing.T) {
	for _, test := range []struct {
		name      string
		repoError error
		wantError error
	}{
		{name: "quota", repoError: &repositorypkg.EntitlementLimitError{Metric: "storage_bytes", Limit: 1, Usage: 1, Additional: 1}, wantError: ErrSubscriptionLimitExceeded},
		{name: "control missing", repoError: pgx.ErrNoRows, wantError: ErrControlNotFound},
		{name: "store failure", repoError: errors.New("database unavailable"), wantError: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &evidenceRepositoryStub{attachErr: test.repoError}
			pipeline := &evidencePipelineStub{result: cleanEvidenceUploadResult()}
			storage := &evidenceStorageStub{}
			service := newEvidenceServiceForTest(t, repository, pipeline, storage)
			_, err := service.Upload(context.Background(), evidenceServiceOrgID, evidenceServiceUserID, evidenceServiceControlID,
				"control.pdf", "application/pdf", strings.NewReader("%PDF-content"), EvidenceUploadMetadata{Title: "Evidence", EvidenceType: "document"})
			if test.wantError != nil && !errors.Is(err, test.wantError) {
				t.Fatalf("Upload() error = %v, want %v", err, test.wantError)
			}
			if test.wantError == nil && !errors.Is(err, test.repoError) {
				t.Fatalf("Upload() error = %v, want repository error", err)
			}
			if len(storage.deleted) != 1 || storage.deleted[0] != pipeline.result.ObjectKey {
				t.Fatalf("deleted = %#v", storage.deleted)
			}
		})
	}
}

func TestEvidenceObjectServiceMapsQuarantineOutcomes(t *testing.T) {
	tests := []struct {
		name      string
		pipeline  error
		wantError error
	}{
		{name: "infected", pipeline: evidencepipeline.ErrMalwareDetected, wantError: ErrEvidenceObjectRejected},
		{name: "scanner unavailable", pipeline: evidencepipeline.ErrScannerUnavailable, wantError: ErrEvidenceScannerOffline},
		{name: "content mismatch", pipeline: evidencepipeline.ErrContentMismatch, wantError: ErrEvidenceObjectRejected},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &evidenceRepositoryStub{}
			pipeline := &evidencePipelineStub{err: test.pipeline}
			service := newEvidenceServiceForTest(t, repository, pipeline, &evidenceStorageStub{})
			_, err := service.Upload(context.Background(), evidenceServiceOrgID, evidenceServiceUserID, evidenceServiceControlID,
				"control.pdf", "application/pdf", strings.NewReader("content"), EvidenceUploadMetadata{Title: "Evidence", EvidenceType: "document"})
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Upload() error = %v, want %v", err, test.wantError)
			}
			if repository.attachInput.Title != "" {
				t.Fatal("quarantined upload reached repository")
			}
		})
	}
}

func TestEvidenceObjectServiceRejectsReservedClientMetadata(t *testing.T) {
	repository := &evidenceRepositoryStub{}
	pipeline := &evidencePipelineStub{result: cleanEvidenceUploadResult()}
	service := newEvidenceServiceForTest(t, repository, pipeline, &evidenceStorageStub{})
	_, err := service.Upload(context.Background(), evidenceServiceOrgID, evidenceServiceUserID, evidenceServiceControlID,
		"control.pdf", "application/pdf", strings.NewReader("content"), EvidenceUploadMetadata{
			Title: "Evidence", EvidenceType: "document", Metadata: json.RawMessage(`{"object_security":{"scan_verdict":"clean"}}`),
		})
	if !errors.Is(err, ErrInvalidEvidence) || pipeline.request.Body != nil {
		t.Fatalf("Upload() error = %v pipeline=%#v", err, pipeline.request)
	}
}

func TestEvidenceObjectServiceDownloadUsesShortLivedSignerOrPrivateStream(t *testing.T) {
	filename, key, hash, size := "evidence.pdf", "evidence/tenant/object", strings.Repeat("a", 64), int64(42)
	item := &models.ControlEvidence{ObjectKey: &key, FileName: &filename, FileHash: &hash, FileSizeBytes: &size}
	repository := &evidenceRepositoryStub{getItem: item}
	pipeline := &evidencePipelineStub{}

	signedStorage := &signedEvidenceStorageStub{url: "https://signed.example.test/object"}
	service := newEvidenceServiceForTest(t, repository, pipeline, signedStorage)
	download, err := service.Download(context.Background(), evidenceServiceOrgID, evidenceServiceControlID, evidenceServiceObjectID)
	if err != nil || download.SignedURL != signedStorage.url || download.Body != nil || signedStorage.lifetime != 5*time.Minute || signedStorage.key != key {
		t.Fatalf("signed Download() = %#v error=%v storage=%#v", download, err, signedStorage)
	}
	if signedStorage.verifiedKey != key || signedStorage.verifiedHash != hash || signedStorage.verifiedSize != size {
		t.Fatalf("signed integrity verification = %#v", signedStorage)
	}

	streamStorage := &evidenceStorageStub{body: "evidence bytes"}
	service = newEvidenceServiceForTest(t, repository, pipeline, streamStorage)
	download, err = service.Download(context.Background(), evidenceServiceOrgID, evidenceServiceControlID, evidenceServiceObjectID)
	if err != nil || download.Body == nil || download.SignedURL != "" {
		t.Fatalf("stream Download() = %#v error=%v", download, err)
	}
	contents, _ := io.ReadAll(download.Body)
	_ = download.Body.Close()
	if string(contents) != "evidence bytes" {
		t.Fatalf("download contents = %q", contents)
	}
}

func TestEvidenceObjectServiceFailsClosedBeforeDownloadOnIntegrityMismatch(t *testing.T) {
	filename, key, hash, size := "evidence.pdf", "evidence/tenant/object", strings.Repeat("a", 64), int64(42)
	repository := &evidenceRepositoryStub{getItem: &models.ControlEvidence{
		ObjectKey: &key, FileName: &filename, FileHash: &hash, FileSizeBytes: &size,
	}}
	storage := &signedEvidenceStorageStub{url: "https://must-not-be-issued.example.test", evidenceStorageStub: evidenceStorageStub{verifyErr: errors.New("checksum mismatch")}}
	service := newEvidenceServiceForTest(t, repository, &evidencePipelineStub{}, storage)
	download, err := service.Download(context.Background(), evidenceServiceOrgID, evidenceServiceControlID, evidenceServiceObjectID)
	if download != nil || !errors.Is(err, ErrEvidenceObjectUnavailable) || storage.key != "" {
		t.Fatalf("Download()=%#v error=%v storage=%#v", download, err, storage)
	}
}

func TestEvidenceObjectServiceLogsOnlySafeFailureTaxonomy(t *testing.T) {
	const providerError = "https://internal-object-store.test?X-Amz-Signature=PROVIDER_SECRET credential AKIA_SECRET"
	filename, key, hash, size := "evidence.pdf", "evidence/tenant/object", strings.Repeat("a", 64), int64(42)
	for _, name := range []string{"integrity", "cleanup"} {
		t.Run(name, func(t *testing.T) {
			repository := &evidenceRepositoryStub{getItem: &models.ControlEvidence{ObjectKey: &key, FileName: &filename, FileHash: &hash, FileSizeBytes: &size}}
			storage := &evidenceStorageStub{}
			svc := newEvidenceServiceForTest(t, repository, &evidencePipelineStub{}, storage)
			var output bytes.Buffer
			svc.logger = zerolog.New(&output)
			if name == "integrity" {
				storage.verifyErr = errors.New(providerError)
				if _, err := svc.Download(context.Background(), evidenceServiceOrgID, evidenceServiceControlID, evidenceServiceObjectID); !errors.Is(err, ErrEvidenceObjectUnavailable) {
					t.Fatalf("unsafe/incorrect download failure: %v", err)
				}
			} else {
				storage.err = errors.New(providerError)
				svc.cleanupObject(context.Background(), key)
			}
			if output.Len() == 0 || !strings.Contains(output.String(), `"error_code":"evidence_`) ||
				strings.Contains(output.String(), "PROVIDER_SECRET") || strings.Contains(output.String(), "AKIA_SECRET") || strings.Contains(output.String(), "internal-object-store") {
				t.Fatalf("unsafe failure log: %s", output.String())
			}
		})
	}
}

func TestEvidenceObjectServiceDownloadAndReviewValidation(t *testing.T) {
	repository := &evidenceRepositoryStub{getErr: pgx.ErrNoRows}
	service := newEvidenceServiceForTest(t, repository, &evidencePipelineStub{}, &evidenceStorageStub{})
	if _, err := service.Download(context.Background(), evidenceServiceOrgID, evidenceServiceControlID, evidenceServiceObjectID); !errors.Is(err, ErrEvidenceObjectNotFound) {
		t.Fatalf("Download() error = %v", err)
	}

	for _, input := range []models.ReviewControlEvidenceInput{{Status: "pending"}, {Status: "rejected"}, {Status: "accepted", Comment: stringPointer(strings.Repeat("x", 4_001))}} {
		if _, err := service.Review(context.Background(), evidenceServiceOrgID, evidenceServiceUserID, evidenceServiceControlID, evidenceServiceObjectID, input); !errors.Is(err, ErrInvalidEvidenceReview) {
			t.Fatalf("Review(%#v) error = %v", input, err)
		}
	}
	comment := "  insufficient scope  "
	repository.getErr = nil
	item, err := service.Review(context.Background(), evidenceServiceOrgID, evidenceServiceUserID, evidenceServiceControlID, evidenceServiceObjectID,
		models.ReviewControlEvidenceInput{Status: "REJECTED", Comment: &comment})
	if err != nil || item.ReviewStatus != "rejected" || repository.reviewInput.Comment == nil || *repository.reviewInput.Comment != "insufficient scope" {
		t.Fatalf("Review() item=%#v input=%#v error=%v", item, repository.reviewInput, err)
	}
}

func TestNewEvidenceObjectServiceRequiresDependenciesAndBoundedTTL(t *testing.T) {
	repository := &evidenceRepositoryStub{}
	pipeline := &evidencePipelineStub{}
	storage := &evidenceStorageStub{}
	for _, test := range []struct {
		repository EvidenceObjectRepository
		pipeline   EvidenceObjectPipeline
		storage    EvidenceObjectStorage
		ttl        time.Duration
	}{{nil, pipeline, storage, time.Minute}, {repository, nil, storage, time.Minute}, {repository, pipeline, nil, time.Minute}, {repository, pipeline, storage, time.Second}, {repository, pipeline, storage, time.Hour}} {
		if _, err := NewEvidenceObjectService(test.repository, test.pipeline, test.storage, test.ttl, zerolog.Nop()); err == nil {
			t.Fatal("NewEvidenceObjectService() error = nil")
		}
	}
	var typedRepository *evidenceRepositoryStub
	if _, err := NewEvidenceObjectService(typedRepository, pipeline, storage, time.Minute, zerolog.Nop()); err == nil {
		t.Fatal("typed nil evidence repository accepted")
	}
}
