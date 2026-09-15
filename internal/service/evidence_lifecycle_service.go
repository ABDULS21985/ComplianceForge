package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	storagepkg "github.com/complianceforge/platform/internal/pkg/storage"
)

var (
	ErrEvidenceLifecycleNotFound    = errors.New("evidence lifecycle record not found")
	ErrEvidenceIntegrityFailed      = errors.New("evidence integrity verification failed")
	ErrEvidenceIntegrityUnavailable = errors.New("evidence integrity verification unavailable")
	ErrEvidenceCustodyChainInvalid  = errors.New("evidence custody chain verification failed")
)

type EvidenceLifecycleStore interface {
	GetEvidenceLifecycle(context.Context, string, string, string) (*models.EvidenceLifecycleRecord, error)
	GetEvidenceIntegrity(context.Context, string, string, string) (*models.EvidenceIntegrityTarget, error)
	AppendEvidenceCustodyEvent(context.Context, string, string, string, models.EvidenceCustodyEventInput) (*models.EvidenceCustodyEvent, error)
}

type EvidenceIntegrityStore interface {
	Verify(context.Context, string, string, int64) error
}

// EvidenceLifecycleService owns history and custody operations that are
// separate from byte ingestion. Every access remains scoped by tenant,
// framework control, and evidence identifier in the repository.
type EvidenceLifecycleService struct {
	repository EvidenceLifecycleStore
	storage    EvidenceIntegrityStore
	now        func() time.Time
	logger     zerolog.Logger
}

func NewEvidenceLifecycleService(repository EvidenceLifecycleStore, storage EvidenceIntegrityStore, logger zerolog.Logger) (*EvidenceLifecycleService, error) {
	if evidenceLifecycleDependencyNil(repository) || evidenceLifecycleDependencyNil(storage) {
		return nil, errors.New("evidence lifecycle repository and integrity store are required")
	}
	return &EvidenceLifecycleService{
		repository: repository,
		storage:    storage,
		now:        func() time.Time { return time.Now().UTC() },
		logger:     logger.With().Str("service", "evidence_lifecycle").Logger(),
	}, nil
}

func evidenceLifecycleDependencyNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (s *EvidenceLifecycleService) Ready() bool {
	return s != nil && s.repository != nil && s.storage != nil
}

func (s *EvidenceLifecycleService) History(ctx context.Context, organizationID, controlID, evidenceID string) (*models.EvidenceLifecycleRecord, error) {
	if !validUUID(organizationID) || !validUUID(controlID) || !validUUID(evidenceID) {
		return nil, ErrInvalidComplianceID
	}
	record, err := s.repository.GetEvidenceLifecycle(ctx, organizationID, controlID, evidenceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEvidenceLifecycleNotFound
	}
	if err != nil {
		return nil, err
	}
	if record == nil || record.Evidence.ID == "" || record.Evidence.OrganizationID != organizationID {
		return nil, ErrEvidenceLifecycleNotFound
	}
	if record.Versions == nil {
		record.Versions = []models.ControlEvidence{}
	}
	if record.Reviews == nil {
		record.Reviews = []models.EvidenceReview{}
	}
	if record.CustodyEvents == nil {
		record.CustodyEvents = []models.EvidenceCustodyEvent{}
	}
	return record, nil
}

func (s *EvidenceLifecycleService) VerifyIntegrity(
	ctx context.Context,
	organizationID, actorUserID, controlID, evidenceID, requestID string,
) (*models.EvidenceIntegrityResult, error) {
	if !validUUID(organizationID) || !validUUID(actorUserID) || !validUUID(controlID) || !validUUID(evidenceID) {
		return nil, ErrInvalidComplianceID
	}
	requestID = strings.TrimSpace(requestID)
	if len(requestID) > 160 || strings.ContainsAny(requestID, "\r\n") {
		return nil, ErrInvalidComplianceID
	}
	target, err := s.repository.GetEvidenceIntegrity(ctx, organizationID, controlID, evidenceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEvidenceLifecycleNotFound
	}
	if err != nil {
		return nil, err
	}
	if target == nil || target.EvidenceID != evidenceID || target.OrganizationID != organizationID {
		return nil, ErrEvidenceLifecycleNotFound
	}
	// A byte-level verification cannot establish evidence integrity when the
	// immutable history describing that object is already inconsistent. Do not
	// append a reassuring verdict to a chain that failed its own verification.
	if !target.Chain.Valid {
		s.logger.Error().
			Str("evidence_id", evidenceID).
			Int64("event_count", target.Chain.EventCount).
			Int64("head_sequence", target.Chain.HeadSequence).
			Msg("evidence custody chain verification failed")
		return nil, ErrEvidenceCustodyChainInvalid
	}
	if target.ObjectKey == nil || target.SHA256 == nil || target.SizeBytes == nil ||
		strings.TrimSpace(*target.ObjectKey) == "" || strings.TrimSpace(*target.SHA256) == "" || *target.SizeBytes < 0 {
		return nil, ErrEvidenceObjectUnavailable
	}

	verifiedAt := s.now().UTC()
	actor := actorUserID
	requestPointer := optionalLifecycleString(requestID)
	verifyErr := s.storage.Verify(ctx, *target.ObjectKey, *target.SHA256, *target.SizeBytes)
	eventType := models.EvidenceCustodyIntegrityVerified
	reason := "Evidence object integrity verified"
	if errors.Is(verifyErr, storagepkg.ErrIntegrityMismatch) {
		eventType = models.EvidenceCustodyIntegrityFailed
		reason = "Evidence object integrity verification failed"
	} else if verifyErr != nil {
		// Network, authorization, cancellation, and backend I/O failures prove
		// neither integrity nor tampering. Surface the operational failure and
		// leave the immutable custody history unchanged so operators can retry.
		s.logger.Error().Err(verifyErr).Str("evidence_id", evidenceID).
			Msg("evidence object integrity check was unavailable")
		return nil, fmt.Errorf("%w: %w", ErrEvidenceIntegrityUnavailable, verifyErr)
	}
	_, eventErr := s.repository.AppendEvidenceCustodyEvent(ctx, organizationID, controlID, evidenceID, models.EvidenceCustodyEventInput{
		EventType: eventType, ActorUserID: &actor, ActorType: "user", Reason: reason,
		ObjectSHA256: target.SHA256, RequestID: requestPointer,
		Details: map[string]any{"verification_method": "sha256_and_size"},
	})
	if eventErr != nil {
		return nil, fmt.Errorf("record evidence integrity custody event: %w", eventErr)
	}
	result := &models.EvidenceIntegrityResult{
		EvidenceID: evidenceID, Valid: verifyErr == nil, SHA256: *target.SHA256,
		SizeBytes: *target.SizeBytes, VerifiedAt: verifiedAt,
	}
	if errors.Is(verifyErr, storagepkg.ErrIntegrityMismatch) {
		s.logger.Error().Err(verifyErr).Str("evidence_id", evidenceID).Msg("evidence integrity verification failed")
		return result, ErrEvidenceIntegrityFailed
	}
	return result, nil
}

func (s *EvidenceLifecycleService) RecordDownloadAuthorization(
	ctx context.Context,
	organizationID, actorUserID, controlID, evidenceID, requestID, deliveryMode string,
) error {
	if !validUUID(organizationID) || !validUUID(actorUserID) || !validUUID(controlID) || !validUUID(evidenceID) {
		return ErrInvalidComplianceID
	}
	deliveryMode = strings.TrimSpace(deliveryMode)
	if deliveryMode != "private_stream" && deliveryMode != "signed_url" {
		return ErrInvalidComplianceID
	}
	requestID = strings.TrimSpace(requestID)
	if len(requestID) > 160 || strings.ContainsAny(requestID, "\r\n") {
		return ErrInvalidComplianceID
	}
	actor := actorUserID
	_, err := s.repository.AppendEvidenceCustodyEvent(ctx, organizationID, controlID, evidenceID, models.EvidenceCustodyEventInput{
		EventType: models.EvidenceCustodyDownloadAuthorized, ActorUserID: &actor,
		ActorType: "user", Reason: "Authorized evidence download",
		RequestID: optionalLifecycleString(requestID), Details: map[string]any{"delivery_mode": deliveryMode},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEvidenceLifecycleNotFound
	}
	return err
}

func optionalLifecycleString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
