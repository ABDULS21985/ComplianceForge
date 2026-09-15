package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	evidencepipeline "github.com/complianceforge/platform/internal/pkg/evidence"
	"github.com/complianceforge/platform/internal/repository"
)

var (
	ErrEvidenceObjectRejected    = errors.New("evidence object was rejected")
	ErrEvidenceScannerOffline    = errors.New("evidence malware scanning is temporarily unavailable")
	ErrEvidenceObjectNotFound    = errors.New("evidence object not found")
	ErrEvidenceObjectUnavailable = errors.New("evidence object is unavailable")
	ErrInvalidEvidenceReview     = errors.New("invalid evidence review")
	ErrInvalidEvidenceVersion    = errors.New("invalid evidence version")
	ErrEvidenceReviewConflict    = errors.New("evidence review request conflicts with a prior review")
)

type EvidenceObjectRepository interface {
	AttachEvidence(context.Context, string, string, string, models.AttachControlEvidenceInput) (*models.ControlEvidence, error)
	GetEvidence(context.Context, string, string, string) (*models.ControlEvidence, error)
	ReviewEvidence(context.Context, string, string, string, string, models.ReviewControlEvidenceInput) (*models.ControlEvidence, error)
	SupersedeEvidence(context.Context, string, string, string, string, models.AttachControlEvidenceInput) (*models.ControlEvidence, error)
}

type EvidenceObjectPipeline interface {
	Upload(context.Context, evidencepipeline.UploadRequest) (*evidencepipeline.UploadResult, error)
}

type EvidenceObjectStorage interface {
	Download(context.Context, string) (io.ReadCloser, error)
	Verify(context.Context, string, string, int64) error
	Delete(context.Context, string) error
}

type signedEvidenceObjectStorage interface {
	PresignDownload(context.Context, string, string, time.Duration) (string, error)
}

type EvidenceUploadMetadata struct {
	Title         string
	Description   *string
	EvidenceType  string
	ValidFrom     *time.Time
	ValidUntil    *time.Time
	Metadata      json.RawMessage
	VersionReason string
}

type EvidenceDownload struct {
	SignedURL   string
	Body        io.ReadCloser
	Filename    string
	SizeBytes   int64
	SHA256      string
	ContentType string
}

type EvidenceObjectService struct {
	repository     EvidenceObjectRepository
	pipeline       EvidenceObjectPipeline
	storage        EvidenceObjectStorage
	signedLifetime time.Duration
	logger         zerolog.Logger
}

func NewEvidenceObjectService(
	repository EvidenceObjectRepository,
	pipeline EvidenceObjectPipeline,
	storage EvidenceObjectStorage,
	signedLifetime time.Duration,
	logger zerolog.Logger,
) (*EvidenceObjectService, error) {
	if evidenceLifecycleDependencyNil(repository) || evidenceLifecycleDependencyNil(pipeline) || evidenceLifecycleDependencyNil(storage) {
		return nil, errors.New("evidence repository, pipeline, and storage are required")
	}
	if signedLifetime < 30*time.Second || signedLifetime > 15*time.Minute {
		return nil, errors.New("evidence signed-download lifetime must be between 30 seconds and 15 minutes")
	}
	return &EvidenceObjectService{
		repository: repository, pipeline: pipeline, storage: storage,
		signedLifetime: signedLifetime,
		logger:         logger.With().Str("service", "evidence_objects").Logger(),
	}, nil
}

func (s *EvidenceObjectService) Ready() bool {
	return s != nil && s.repository != nil && s.pipeline != nil && s.storage != nil
}

func (s *EvidenceObjectService) Upload(
	ctx context.Context,
	organizationID, userID, controlID, filename, declaredContentType string,
	body io.Reader,
	metadata EvidenceUploadMetadata,
) (*models.ControlEvidence, error) {
	return s.uploadVersion(ctx, organizationID, userID, controlID, "", filename, declaredContentType, body, metadata)
}

func (s *EvidenceObjectService) Supersede(
	ctx context.Context,
	organizationID, userID, controlID, evidenceID, filename, declaredContentType string,
	body io.Reader,
	metadata EvidenceUploadMetadata,
) (*models.ControlEvidence, error) {
	if !validUUID(evidenceID) {
		return nil, ErrInvalidComplianceID
	}
	metadata.VersionReason = strings.TrimSpace(metadata.VersionReason)
	if len(metadata.VersionReason) < 3 || len(metadata.VersionReason) > 1_000 || strings.ContainsAny(metadata.VersionReason, "\r\n") {
		return nil, fmt.Errorf("%w: version_reason must contain 3-1000 safe characters", ErrInvalidEvidenceVersion)
	}
	return s.uploadVersion(ctx, organizationID, userID, controlID, evidenceID, filename, declaredContentType, body, metadata)
}

func (s *EvidenceObjectService) uploadVersion(
	ctx context.Context,
	organizationID, userID, controlID, evidenceID, filename, declaredContentType string,
	body io.Reader,
	metadata EvidenceUploadMetadata,
) (*models.ControlEvidence, error) {
	if !validUUID(organizationID) || !validUUID(userID) || !validUUID(controlID) {
		return nil, ErrInvalidComplianceID
	}
	trustedMetadata, err := evidenceMetadata(metadata.Metadata, nil)
	if err != nil {
		return nil, err
	}
	validationInput := models.AttachControlEvidenceInput{
		Title: metadata.Title, Description: metadata.Description, EvidenceType: metadata.EvidenceType,
		CollectionMethod: "manual_upload", ValidFrom: metadata.ValidFrom, ValidUntil: metadata.ValidUntil,
		Metadata: trustedMetadata, VersionReason: metadata.VersionReason,
	}
	if evidenceID != "" {
		validationInput.SupersedesEvidenceID = &evidenceID
	}
	if err := validateEvidence(validationInput); err != nil {
		return nil, err
	}

	result, err := s.pipeline.Upload(ctx, evidencepipeline.UploadRequest{
		OrganizationID: organizationID, Filename: filename,
		DeclaredContentType: declaredContentType, Body: body,
	})
	if err != nil {
		switch {
		case errors.Is(err, evidencepipeline.ErrScannerUnavailable):
			s.logger.Warn().Str("organization_id", organizationID).Msg("evidence upload retained in quarantine because scanning was unavailable")
			return nil, ErrEvidenceScannerOffline
		case errors.Is(err, evidencepipeline.ErrMalwareDetected):
			s.logger.Warn().Str("organization_id", organizationID).Msg("malware-infected evidence upload retained in quarantine")
			return nil, ErrEvidenceObjectRejected
		case errors.Is(err, evidencepipeline.ErrEmptyObject), errors.Is(err, evidencepipeline.ErrObjectTooLarge),
			errors.Is(err, evidencepipeline.ErrInvalidFilename), errors.Is(err, evidencepipeline.ErrUnsupportedType),
			errors.Is(err, evidencepipeline.ErrContentMismatch), errors.Is(err, evidencepipeline.ErrUnsafeArchive):
			return nil, fmt.Errorf("%w: %v", ErrEvidenceObjectRejected, err)
		default:
			return nil, err
		}
	}

	trustedMetadata, err = evidenceMetadata(metadata.Metadata, result)
	if err != nil {
		s.cleanupObject(ctx, result.ObjectKey)
		return nil, err
	}
	size, hash, contentType, objectKey, originalFilename := result.SizeBytes, result.SHA256, result.ContentType, result.ObjectKey, result.OriginalFilename
	validationInput.ObjectKey = &objectKey
	validationInput.FileName = &originalFilename
	validationInput.FileSizeBytes = &size
	validationInput.MIMEType = &contentType
	validationInput.FileHash = &hash
	validationInput.Metadata = trustedMetadata

	var item *models.ControlEvidence
	if evidenceID == "" {
		item, err = s.repository.AttachEvidence(ctx, organizationID, userID, controlID, validationInput)
	} else {
		item, err = s.repository.SupersedeEvidence(ctx, organizationID, userID, controlID, evidenceID, validationInput)
	}
	if err != nil {
		s.cleanupObject(ctx, result.ObjectKey)
		if errors.Is(err, pgx.ErrNoRows) {
			if evidenceID != "" {
				return nil, ErrEvidenceObjectNotFound
			}
			return nil, ErrControlNotFound
		}
		if errors.Is(err, repository.ErrEntitlementLimitExceeded) {
			return nil, fmt.Errorf("%w: evidence storage capacity is exhausted", ErrSubscriptionLimitExceeded)
		}
		return nil, err
	}
	return item, nil
}

func (s *EvidenceObjectService) Download(ctx context.Context, organizationID, controlID, evidenceID string) (*EvidenceDownload, error) {
	if !validUUID(organizationID) || !validUUID(controlID) || !validUUID(evidenceID) {
		return nil, ErrInvalidComplianceID
	}
	item, err := s.repository.GetEvidence(ctx, organizationID, controlID, evidenceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEvidenceObjectNotFound
	}
	if err != nil {
		return nil, err
	}
	if item.ObjectKey == nil || strings.TrimSpace(*item.ObjectKey) == "" || item.FileName == nil || item.FileHash == nil || item.FileSizeBytes == nil {
		return nil, ErrEvidenceObjectUnavailable
	}
	download := &EvidenceDownload{
		Filename: *item.FileName, SizeBytes: *item.FileSizeBytes, SHA256: *item.FileHash,
		ContentType: "application/octet-stream",
	}
	if err := s.storage.Verify(ctx, *item.ObjectKey, *item.FileHash, *item.FileSizeBytes); err != nil {
		s.logger.Error().Str("error_code", "evidence_integrity_verification_failed").Str("evidence_id", evidenceID).Msg("evidence object failed pre-download integrity verification")
		return nil, ErrEvidenceObjectUnavailable
	}
	if signer, ok := s.storage.(signedEvidenceObjectStorage); ok {
		download.SignedURL, err = signer.PresignDownload(ctx, *item.ObjectKey, *item.FileName, s.signedLifetime)
		if err != nil {
			return nil, fmt.Errorf("create evidence download: %w", err)
		}
		return download, nil
	}
	download.Body, err = s.storage.Download(ctx, *item.ObjectKey)
	if err != nil {
		return nil, fmt.Errorf("open evidence download: %w", err)
	}
	return download, nil
}

func (s *EvidenceObjectService) Review(ctx context.Context, organizationID, userID, controlID, evidenceID string, input models.ReviewControlEvidenceInput) (*models.ControlEvidence, error) {
	if !validUUID(organizationID) || !validUUID(userID) || !validUUID(controlID) || !validUUID(evidenceID) {
		return nil, ErrInvalidComplianceID
	}
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Status != "accepted" && input.Status != "rejected" {
		return nil, ErrInvalidEvidenceReview
	}
	if input.Comment != nil {
		trimmed := strings.TrimSpace(*input.Comment)
		if len(trimmed) > 4_000 {
			return nil, ErrInvalidEvidenceReview
		}
		input.Comment = &trimmed
	}
	if input.Status == "rejected" && (input.Comment == nil || *input.Comment == "") {
		return nil, fmt.Errorf("%w: a rejection comment is required", ErrInvalidEvidenceReview)
	}
	item, err := s.repository.ReviewEvidence(ctx, organizationID, userID, controlID, evidenceID, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEvidenceObjectNotFound
	}
	if errors.Is(err, repository.ErrEvidenceReviewConflict) {
		return nil, ErrEvidenceReviewConflict
	}
	return item, err
}

func evidenceMetadata(raw json.RawMessage, upload *evidencepipeline.UploadResult) (json.RawMessage, error) {
	metadata := make(map[string]any)
	if len(raw) > 0 {
		if len(raw) > 16<<10 {
			return nil, fmt.Errorf("%w: metadata must not exceed 16 KiB", ErrInvalidEvidence)
		}
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return nil, fmt.Errorf("%w: metadata must be a JSON object", ErrInvalidEvidence)
		}
		if metadata == nil {
			metadata = make(map[string]any)
		}
	}
	if _, reserved := metadata["object_security"]; reserved {
		return nil, fmt.Errorf("%w: object_security metadata is server managed", ErrInvalidEvidence)
	}
	if upload != nil {
		metadata["object_security"] = map[string]any{
			"checksum_source":            "server_sha256",
			"scan_engine":                upload.ScanEngine,
			"scan_verdict":               upload.ScanVerdict,
			"scanned_at":                 upload.ScannedAt,
			"quarantine_cleanup_pending": upload.QuarantineKey != "",
		}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("encode evidence metadata: %w", err)
	}
	return encoded, nil
}

func (s *EvidenceObjectService) cleanupObject(ctx context.Context, key string) {
	if strings.TrimSpace(key) == "" {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.storage.Delete(cleanupCtx, key); err != nil {
		s.logger.Error().Str("error_code", "evidence_cleanup_failed").Msg("failed to remove uncommitted evidence object")
	}
}
