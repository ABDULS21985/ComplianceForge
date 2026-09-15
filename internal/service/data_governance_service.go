package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

var (
	ErrDataGovernanceInvalid  = errors.New("data governance request is invalid")
	ErrDataGovernanceNotFound = errors.New("data governance record was not found")
	ErrDataGovernanceConflict = errors.New("data governance record changed or conflicts")
	ErrDataGovernanceState    = errors.New("data governance record state prevents the operation")
)

type DataGovernanceStore interface {
	GetPolicy(context.Context, string) (*models.DataGovernancePolicy, error)
	UpsertPolicy(context.Context, string, string, string, models.DataGovernancePolicyInput) (*models.DataGovernancePolicy, error)
	CreateSchedule(context.Context, string, string, string, models.RetentionScheduleInput) (*models.RetentionSchedule, error)
	GetSchedule(context.Context, string, string) (*models.RetentionSchedule, error)
	ListSchedules(context.Context, string, models.RetentionScheduleFilter) ([]models.RetentionSchedule, int, error)
	UpdateSchedule(context.Context, string, string, string, string, models.RetentionSchedulePatch) (*models.RetentionSchedule, error)
	RetireSchedule(context.Context, string, string, string, string, int64, string) error
	CreateAssignment(context.Context, string, string, string, models.RetentionAssignmentInput) (*models.RecordRetentionAssignment, error)
	GetAssignment(context.Context, string, string) (*models.RecordRetentionAssignment, error)
	GetRecordDisposition(context.Context, string, string, string) (*models.RecordDispositionDecision, error)
	ReviewDisposition(context.Context, string, string, string, string, models.RetentionReviewInput) (*models.RecordRetentionAssignment, error)
	RequestException(context.Context, string, string, string, string, models.RetentionExceptionInput) (*models.RetentionException, error)
	DecideException(context.Context, string, string, string, string, models.RetentionExceptionDecisionInput) (*models.RetentionException, error)
	ListExceptions(context.Context, string, string) ([]models.RetentionException, error)
	CreateLegalHold(context.Context, string, string, string, models.LegalHoldInput) (*models.LegalHold, error)
	GetLegalHold(context.Context, string, string) (*models.LegalHold, error)
	ListLegalHolds(context.Context, string, string, models.PaginationRequest) ([]models.LegalHold, int, error)
	UpdateLegalHold(context.Context, string, string, string, string, models.LegalHoldPatch) (*models.LegalHold, error)
	ReleaseLegalHold(context.Context, string, string, string, string, models.LegalHoldReleaseInput) (*models.LegalHold, error)
	AddLegalHoldRecord(context.Context, string, string, string, string, models.LegalHoldRecordInput) (*models.LegalHoldRecord, error)
	ReleaseLegalHoldRecord(context.Context, string, string, string, string, string, models.LegalHoldRecordReleaseInput) error
	ListLegalHoldRecords(context.Context, string, string, bool) ([]models.LegalHoldRecord, error)
	ListEvents(context.Context, string, models.DataGovernanceEventFilter) ([]models.DataGovernanceEvent, int, error)
	VerifyEventChain(context.Context, string) (*models.GovernanceChainVerification, error)
}

type DataGovernanceService struct {
	store  DataGovernanceStore
	logger zerolog.Logger
	now    func() time.Time
}

func NewDataGovernanceService(store DataGovernanceStore, logger zerolog.Logger) *DataGovernanceService {
	return NewDataGovernanceServiceWithClock(store, logger, time.Now)
}

func NewDataGovernanceServiceWithClock(
	store DataGovernanceStore, logger zerolog.Logger, now func() time.Time,
) *DataGovernanceService {
	if now == nil {
		now = time.Now
	}
	return &DataGovernanceService{
		store: store, logger: logger.With().Str("service", "data_governance").Logger(), now: now,
	}
}

func (s *DataGovernanceService) GetPolicy(
	ctx context.Context, organizationID string,
) (*models.DataGovernancePolicy, error) {
	if err := validateGovernanceUUID("organization_id", organizationID); err != nil {
		return nil, err
	}
	item, err := s.store.GetPolicy(ctx, organizationID)
	return item, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) UpsertPolicy(
	ctx context.Context, organizationID, actorID, requestID string, input models.DataGovernancePolicyInput,
) (*models.DataGovernancePolicy, error) {
	if err := validateGovernanceActor(organizationID, actorID, requestID); err != nil {
		return nil, err
	}
	normalizeGovernancePolicy(&input)
	if err := validateGovernancePolicy(input); err != nil {
		return nil, err
	}
	item, err := s.store.UpsertPolicy(ctx, organizationID, actorID, requestID, input)
	if err != nil {
		return nil, mapDataGovernanceServiceError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Int64("version", item.Version).
		Msg("data governance policy saved")
	return item, nil
}

func (s *DataGovernanceService) CreateSchedule(
	ctx context.Context, organizationID, actorID, requestID string, input models.RetentionScheduleInput,
) (*models.RetentionSchedule, error) {
	if err := validateGovernanceActor(organizationID, actorID, requestID); err != nil {
		return nil, err
	}
	normalizeRetentionSchedule(&input, s.now().UTC())
	if err := validateRetentionSchedule(input); err != nil {
		return nil, err
	}
	item, err := s.store.CreateSchedule(ctx, organizationID, actorID, requestID, input)
	if err != nil {
		return nil, mapDataGovernanceServiceError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("schedule_id", item.ID).
		Msg("retention schedule created")
	return item, nil
}

func (s *DataGovernanceService) GetSchedule(
	ctx context.Context, organizationID, id string,
) (*models.RetentionSchedule, error) {
	if err := validateGovernanceObject(organizationID, id); err != nil {
		return nil, err
	}
	item, err := s.store.GetSchedule(ctx, organizationID, id)
	return item, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) ListSchedules(
	ctx context.Context, organizationID string, filter models.RetentionScheduleFilter,
) ([]models.RetentionSchedule, int, error) {
	if err := validateGovernanceUUID("organization_id", organizationID); err != nil {
		return nil, 0, err
	}
	filter = normalizeRetentionScheduleFilter(filter)
	if err := validateRetentionScheduleFilter(filter); err != nil {
		return nil, 0, err
	}
	items, total, err := s.store.ListSchedules(ctx, organizationID, filter)
	return items, total, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) UpdateSchedule(
	ctx context.Context, organizationID, id, actorID, requestID string, patch models.RetentionSchedulePatch,
) (*models.RetentionSchedule, error) {
	if err := validateGovernanceActorObject(organizationID, id, actorID, requestID); err != nil {
		return nil, err
	}
	normalizeRetentionSchedulePatch(&patch)
	if patch.ExpectedVersion < 1 || !validGovernanceReason(patch.Reason) || !retentionPatchHasChange(patch) {
		return nil, fmt.Errorf("%w: expected_version, a change, and a 3-2000 character reason are required", ErrDataGovernanceInvalid)
	}
	current, err := s.GetSchedule(ctx, organizationID, id)
	if err != nil {
		return nil, err
	}
	merged := mergeRetentionSchedule(*current, patch)
	if err := validateRetentionSchedule(merged); err != nil {
		return nil, err
	}
	item, err := s.store.UpdateSchedule(ctx, organizationID, id, actorID, requestID, patch)
	return item, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) RetireSchedule(
	ctx context.Context, organizationID, id, actorID, requestID string, expectedVersion int64, reason string,
) error {
	if err := validateGovernanceActorObject(organizationID, id, actorID, requestID); err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if expectedVersion < 1 || !validGovernanceReason(reason) {
		return fmt.Errorf("%w: expected_version and a 3-2000 character reason are required", ErrDataGovernanceInvalid)
	}
	return mapDataGovernanceServiceError(s.store.RetireSchedule(
		ctx, organizationID, id, actorID, requestID, expectedVersion, reason,
	))
}

func (s *DataGovernanceService) CreateAssignment(
	ctx context.Context, organizationID, actorID, requestID string, input models.RetentionAssignmentInput,
) (*models.RecordRetentionAssignment, error) {
	if err := validateGovernanceActor(organizationID, actorID, requestID); err != nil {
		return nil, err
	}
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	input.RecordType = normalizeRecordType(input.RecordType)
	input.RecordID = strings.TrimSpace(input.RecordID)
	input.DataClassification = strings.ToLower(strings.TrimSpace(input.DataClassification))
	input.Jurisdiction = strings.ToUpper(strings.TrimSpace(input.Jurisdiction))
	input.Source = strings.ToLower(strings.TrimSpace(input.Source))
	if input.Source == "" {
		input.Source = "manual"
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if err := validateGovernanceUUID("schedule_id", input.ScheduleID); err != nil {
		return nil, err
	}
	if err := validateGovernanceUUID("record_id", input.RecordID); err != nil {
		return nil, err
	}
	if !validRecordType(input.RecordType) || !validClassification(input.DataClassification) ||
		!validJurisdiction(input.Jurisdiction) || !stringIn(input.Source, "automatic", "manual", "import") ||
		input.RetentionStartedAt.IsZero() || input.RetentionStartedAt.After(s.now().UTC().Add(5*time.Minute)) ||
		!validGovernanceReason(input.Reason) {
		return nil, fmt.Errorf("%w: invalid record scope, retention start, source, or reason", ErrDataGovernanceInvalid)
	}
	item, err := s.store.CreateAssignment(ctx, organizationID, actorID, requestID, input)
	return item, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) GetAssignment(
	ctx context.Context, organizationID, id string,
) (*models.RecordRetentionAssignment, error) {
	if err := validateGovernanceObject(organizationID, id); err != nil {
		return nil, err
	}
	item, err := s.store.GetAssignment(ctx, organizationID, id)
	return item, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) GetRecordDisposition(
	ctx context.Context, organizationID, recordType, recordID string,
) (*models.RecordDispositionDecision, error) {
	recordType = normalizeRecordType(recordType)
	if err := validateGovernanceObject(organizationID, recordID); err != nil {
		return nil, err
	}
	if !validRecordType(recordType) {
		return nil, fmt.Errorf("%w: unsupported record_type", ErrDataGovernanceInvalid)
	}
	item, err := s.store.GetRecordDisposition(ctx, organizationID, recordType, recordID)
	return item, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) ReviewDisposition(
	ctx context.Context, organizationID, id, actorID, requestID string, input models.RetentionReviewInput,
) (*models.RecordRetentionAssignment, error) {
	if err := validateGovernanceActorObject(organizationID, id, actorID, requestID); err != nil {
		return nil, err
	}
	input.Decision = strings.ToLower(strings.TrimSpace(input.Decision))
	input.Reason = strings.TrimSpace(input.Reason)
	if input.ExpectedVersion < 1 || !stringIn(input.Decision, "approve", "reject") || !validGovernanceReason(input.Reason) {
		return nil, fmt.Errorf("%w: expected_version, approve/reject decision, and reason are required", ErrDataGovernanceInvalid)
	}
	item, err := s.store.ReviewDisposition(ctx, organizationID, id, actorID, requestID, input)
	return item, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) RequestException(
	ctx context.Context, organizationID, assignmentID, actorID, requestID string,
	input models.RetentionExceptionInput,
) (*models.RetentionException, error) {
	if err := validateGovernanceActorObject(organizationID, assignmentID, actorID, requestID); err != nil {
		return nil, err
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.RequestedUntil.IsZero() || !input.RequestedUntil.After(s.now().UTC()) || !validGovernanceReason(input.Reason) {
		return nil, fmt.Errorf("%w: a future requested_until and reason are required", ErrDataGovernanceInvalid)
	}
	item, err := s.store.RequestException(ctx, organizationID, assignmentID, actorID, requestID, input)
	return item, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) DecideException(
	ctx context.Context, organizationID, id, actorID, requestID string,
	input models.RetentionExceptionDecisionInput,
) (*models.RetentionException, error) {
	if err := validateGovernanceActorObject(organizationID, id, actorID, requestID); err != nil {
		return nil, err
	}
	input.Decision = strings.ToLower(strings.TrimSpace(input.Decision))
	input.Reason = strings.TrimSpace(input.Reason)
	if input.ExpectedVersion < 1 || !stringIn(input.Decision, "approve", "reject") || !validGovernanceReason(input.Reason) {
		return nil, fmt.Errorf("%w: expected_version, approve/reject decision, and reason are required", ErrDataGovernanceInvalid)
	}
	item, err := s.store.DecideException(ctx, organizationID, id, actorID, requestID, input)
	return item, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) ListExceptions(
	ctx context.Context, organizationID, assignmentID string,
) ([]models.RetentionException, error) {
	if err := validateGovernanceObject(organizationID, assignmentID); err != nil {
		return nil, err
	}
	items, err := s.store.ListExceptions(ctx, organizationID, assignmentID)
	return items, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) CreateLegalHold(
	ctx context.Context, organizationID, actorID, requestID string, input models.LegalHoldInput,
) (*models.LegalHold, error) {
	if err := validateGovernanceActor(organizationID, actorID, requestID); err != nil {
		return nil, err
	}
	normalizeLegalHoldInput(&input)
	if err := validateLegalHoldInput(input, s.now().UTC()); err != nil {
		return nil, err
	}
	item, err := s.store.CreateLegalHold(ctx, organizationID, actorID, requestID, input)
	if err != nil {
		return nil, mapDataGovernanceServiceError(err)
	}
	s.logger.Warn().Str("organization_id", organizationID).Str("hold_id", item.ID).
		Str("hold_ref", item.HoldRef).Msg("legal hold placed")
	return item, nil
}

func (s *DataGovernanceService) GetLegalHold(
	ctx context.Context, organizationID, id string,
) (*models.LegalHold, error) {
	if err := validateGovernanceObject(organizationID, id); err != nil {
		return nil, err
	}
	item, err := s.store.GetLegalHold(ctx, organizationID, id)
	return item, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) ListLegalHolds(
	ctx context.Context, organizationID, status string, pagination models.PaginationRequest,
) ([]models.LegalHold, int, error) {
	if err := validateGovernanceUUID("organization_id", organizationID); err != nil {
		return nil, 0, err
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && !stringIn(status, "active", "released", "cancelled") {
		return nil, 0, fmt.Errorf("%w: invalid legal hold status", ErrDataGovernanceInvalid)
	}
	pagination = normalizeGovernancePagination(pagination)
	items, total, err := s.store.ListLegalHolds(ctx, organizationID, status, pagination)
	return items, total, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) UpdateLegalHold(
	ctx context.Context, organizationID, id, actorID, requestID string, patch models.LegalHoldPatch,
) (*models.LegalHold, error) {
	if err := validateGovernanceActorObject(organizationID, id, actorID, requestID); err != nil {
		return nil, err
	}
	normalizeLegalHoldPatch(&patch)
	if patch.ExpectedVersion < 1 || !validGovernanceReason(patch.Reason) || !legalHoldPatchHasChange(patch) {
		return nil, fmt.Errorf("%w: expected_version, a change, and reason are required", ErrDataGovernanceInvalid)
	}
	current, err := s.GetLegalHold(ctx, organizationID, id)
	if err != nil {
		return nil, err
	}
	if err := validateMergedLegalHold(*current, patch, s.now().UTC()); err != nil {
		return nil, err
	}
	item, err := s.store.UpdateLegalHold(ctx, organizationID, id, actorID, requestID, patch)
	return item, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) ReleaseLegalHold(
	ctx context.Context, organizationID, id, actorID, requestID string, input models.LegalHoldReleaseInput,
) (*models.LegalHold, error) {
	if err := validateGovernanceActorObject(organizationID, id, actorID, requestID); err != nil {
		return nil, err
	}
	input.Outcome = strings.ToLower(strings.TrimSpace(input.Outcome))
	input.Reason = strings.TrimSpace(input.Reason)
	if input.ExpectedVersion < 1 || !stringIn(input.Outcome, "release", "cancel") || !validLongReason(input.Reason, 4000) {
		return nil, fmt.Errorf("%w: expected_version, release/cancel outcome, and reason are required", ErrDataGovernanceInvalid)
	}
	item, err := s.store.ReleaseLegalHold(ctx, organizationID, id, actorID, requestID, input)
	if err == nil {
		s.logger.Warn().Str("organization_id", organizationID).Str("hold_id", id).
			Str("outcome", input.Outcome).Msg("legal hold closed")
	}
	return item, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) AddLegalHoldRecord(
	ctx context.Context, organizationID, holdID, actorID, requestID string,
	input models.LegalHoldRecordInput,
) (*models.LegalHoldRecord, error) {
	if err := validateGovernanceActorObject(organizationID, holdID, actorID, requestID); err != nil {
		return nil, err
	}
	input.RecordType = normalizeRecordType(input.RecordType)
	input.RecordID = strings.TrimSpace(input.RecordID)
	input.Reason = strings.TrimSpace(input.Reason)
	if !validRecordType(input.RecordType) || !validGovernanceReason(input.Reason) {
		return nil, fmt.Errorf("%w: supported record_type and reason are required", ErrDataGovernanceInvalid)
	}
	if err := validateGovernanceUUID("record_id", input.RecordID); err != nil {
		return nil, err
	}
	item, err := s.store.AddLegalHoldRecord(ctx, organizationID, holdID, actorID, requestID, input)
	return item, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) ReleaseLegalHoldRecord(
	ctx context.Context, organizationID, holdID, recordID, actorID, requestID string,
	input models.LegalHoldRecordReleaseInput,
) error {
	if err := validateGovernanceActorObject(organizationID, holdID, actorID, requestID); err != nil {
		return err
	}
	if err := validateGovernanceUUID("hold_record_id", recordID); err != nil {
		return err
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if !validGovernanceReason(input.Reason) {
		return fmt.Errorf("%w: a 3-2000 character release reason is required", ErrDataGovernanceInvalid)
	}
	return mapDataGovernanceServiceError(s.store.ReleaseLegalHoldRecord(
		ctx, organizationID, holdID, recordID, actorID, requestID, input,
	))
}

func (s *DataGovernanceService) ListLegalHoldRecords(
	ctx context.Context, organizationID, holdID string, activeOnly bool,
) ([]models.LegalHoldRecord, error) {
	if err := validateGovernanceObject(organizationID, holdID); err != nil {
		return nil, err
	}
	items, err := s.store.ListLegalHoldRecords(ctx, organizationID, holdID, activeOnly)
	return items, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) ListEvents(
	ctx context.Context, organizationID string, filter models.DataGovernanceEventFilter,
) ([]models.DataGovernanceEvent, int, error) {
	if err := validateGovernanceUUID("organization_id", organizationID); err != nil {
		return nil, 0, err
	}
	filter.EntityType = strings.ToLower(strings.TrimSpace(filter.EntityType))
	filter.EntityID = strings.TrimSpace(filter.EntityID)
	filter.EventType = strings.ToLower(strings.TrimSpace(filter.EventType))
	filter.PaginationRequest = normalizeGovernancePagination(filter.PaginationRequest)
	if filter.EntityType != "" && !stringIn(filter.EntityType,
		"policy", "retention_schedule", "retention_assignment", "retention_exception", "legal_hold") {
		return nil, 0, fmt.Errorf("%w: invalid event entity_type", ErrDataGovernanceInvalid)
	}
	if filter.EntityID != "" {
		if err := validateGovernanceUUID("entity_id", filter.EntityID); err != nil {
			return nil, 0, err
		}
	}
	if utf8.RuneCountInString(filter.EventType) > 40 {
		return nil, 0, fmt.Errorf("%w: event_type is too long", ErrDataGovernanceInvalid)
	}
	items, total, err := s.store.ListEvents(ctx, organizationID, filter)
	return items, total, mapDataGovernanceServiceError(err)
}

func (s *DataGovernanceService) VerifyEventChain(
	ctx context.Context, organizationID string,
) (*models.GovernanceChainVerification, error) {
	if err := validateGovernanceUUID("organization_id", organizationID); err != nil {
		return nil, err
	}
	item, err := s.store.VerifyEventChain(ctx, organizationID)
	return item, mapDataGovernanceServiceError(err)
}

func normalizeGovernancePolicy(input *models.DataGovernancePolicyInput) {
	input.PrimaryRegion = strings.ToLower(strings.TrimSpace(input.PrimaryRegion))
	seen := make(map[string]struct{}, len(input.AllowedRegions))
	regions := make([]string, 0, len(input.AllowedRegions))
	for _, region := range input.AllowedRegions {
		region = strings.ToLower(strings.TrimSpace(region))
		if region == "" {
			continue
		}
		if _, exists := seen[region]; !exists {
			seen[region] = struct{}{}
			regions = append(regions, region)
		}
	}
	sort.Strings(regions)
	input.AllowedRegions = regions
	input.CrossBorderTransferMode = strings.ToLower(strings.TrimSpace(input.CrossBorderTransferMode))
	input.DispositionApprovalMode = strings.ToLower(strings.TrimSpace(input.DispositionApprovalMode))
	input.PolicyStatement = strings.TrimSpace(input.PolicyStatement)
	input.Reason = strings.TrimSpace(input.Reason)
	if len(input.Metadata) == 0 {
		input.Metadata = json.RawMessage(`{}`)
	}
}

func validateGovernancePolicy(input models.DataGovernancePolicyInput) error {
	regionValid := func(value string) bool {
		if len(value) < 2 || len(value) > 32 || value[0] < 'a' || value[0] > 'z' {
			return false
		}
		for _, char := range value[1:] {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
		return true
	}
	if !regionValid(input.PrimaryRegion) || len(input.AllowedRegions) < 1 || len(input.AllowedRegions) > 50 {
		return fmt.Errorf("%w: primary_region and 1-50 valid allowed_regions are required", ErrDataGovernanceInvalid)
	}
	foundPrimary := false
	for _, region := range input.AllowedRegions {
		if !regionValid(region) {
			return fmt.Errorf("%w: allowed_regions contains an invalid region", ErrDataGovernanceInvalid)
		}
		foundPrimary = foundPrimary || region == input.PrimaryRegion
	}
	if !foundPrimary || !stringIn(input.CrossBorderTransferMode, "prohibited", "approved_regions", "contractual_safeguards") ||
		input.DefaultRetentionDays < 1 || input.DefaultRetentionDays > 36500 ||
		(input.DefaultArchiveAfterDays != nil && (*input.DefaultArchiveAfterDays < 1 || *input.DefaultArchiveAfterDays >= input.DefaultRetentionDays)) ||
		input.DeletionGraceDays < 0 || input.DeletionGraceDays > 365 ||
		!stringIn(input.DispositionApprovalMode, "none", "single", "dual") ||
		utf8.RuneCountInString(input.PolicyStatement) > 20000 || !validGovernanceReason(input.Reason) {
		return fmt.Errorf("%w: invalid transfer, retention, approval, statement, or reason settings", ErrDataGovernanceInvalid)
	}
	if input.ExpectedVersion != nil && *input.ExpectedVersion < 1 {
		return fmt.Errorf("%w: expected_version must be positive", ErrDataGovernanceInvalid)
	}
	return validateGovernanceJSONObject(input.Metadata, 32768, "metadata")
}

func normalizeRetentionSchedule(input *models.RetentionScheduleInput, now time.Time) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.RecordType = normalizeRecordType(input.RecordType)
	input.DataClassification = strings.ToLower(strings.TrimSpace(input.DataClassification))
	input.Jurisdiction = strings.ToUpper(strings.TrimSpace(input.Jurisdiction))
	input.TriggerEvent = strings.ToLower(strings.TrimSpace(input.TriggerEvent))
	if input.TriggerEvent == "" {
		input.TriggerEvent = "record_created"
	}
	input.LegalBasis = strings.TrimSpace(input.LegalBasis)
	input.DispositionAction = strings.ToLower(strings.TrimSpace(input.DispositionAction))
	if input.DispositionAction == "" {
		input.DispositionAction = "review"
	}
	if input.Priority == 0 {
		input.Priority = 100
	}
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = "draft"
	}
	if input.EffectiveFrom.IsZero() {
		year, month, day := now.Date()
		input.EffectiveFrom = time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}
	input.Reason = strings.TrimSpace(input.Reason)
}

func validateRetentionSchedule(input models.RetentionScheduleInput) error {
	if utf8.RuneCountInString(input.Name) < 3 || utf8.RuneCountInString(input.Name) > 200 ||
		utf8.RuneCountInString(input.Description) > 20000 || !validRecordType(input.RecordType) ||
		!validClassification(input.DataClassification) || !validJurisdiction(input.Jurisdiction) ||
		!stringIn(input.TriggerEvent, "record_created", "record_closed", "contract_ended",
			"employment_ended", "consent_withdrawn", "superseded", "case_closed") ||
		utf8.RuneCountInString(input.LegalBasis) < 3 || utf8.RuneCountInString(input.LegalBasis) > 4000 ||
		input.RetentionDays < 1 || input.RetentionDays > 36500 ||
		(input.ArchiveAfterDays != nil && (*input.ArchiveAfterDays < 1 || *input.ArchiveAfterDays >= input.RetentionDays)) ||
		!stringIn(input.DispositionAction, "review", "delete", "anonymize", "archive") ||
		input.Priority < 1 || input.Priority > 10000 ||
		!stringIn(input.Status, "draft", "active", "retired") || input.EffectiveFrom.IsZero() ||
		(input.EffectiveUntil != nil && input.EffectiveUntil.Before(input.EffectiveFrom)) ||
		!validGovernanceReason(input.Reason) {
		return fmt.Errorf("%w: invalid schedule scope, dates, duration, action, status, or reason", ErrDataGovernanceInvalid)
	}
	return nil
}

func normalizeRetentionSchedulePatch(patch *models.RetentionSchedulePatch) {
	trimStringPointer(patch.Name, false)
	trimStringPointer(patch.Description, false)
	trimStringPointer(patch.DataClassification, true)
	trimStringPointer(patch.Jurisdiction, false)
	if patch.Jurisdiction != nil {
		value := strings.ToUpper(*patch.Jurisdiction)
		patch.Jurisdiction = &value
	}
	trimStringPointer(patch.TriggerEvent, true)
	trimStringPointer(patch.LegalBasis, false)
	trimStringPointer(patch.DispositionAction, true)
	trimStringPointer(patch.Status, true)
	patch.Reason = strings.TrimSpace(patch.Reason)
}

func mergeRetentionSchedule(current models.RetentionSchedule, patch models.RetentionSchedulePatch) models.RetentionScheduleInput {
	result := models.RetentionScheduleInput{
		Name: current.Name, Description: current.Description, RecordType: current.RecordType,
		DataClassification: current.DataClassification, Jurisdiction: current.Jurisdiction,
		TriggerEvent: current.TriggerEvent, LegalBasis: current.LegalBasis,
		RetentionDays: current.RetentionDays, ArchiveAfterDays: current.ArchiveAfterDays,
		DispositionAction: current.DispositionAction, ReviewRequired: current.ReviewRequired,
		Priority: current.Priority, Status: current.Status, EffectiveFrom: current.EffectiveFrom,
		EffectiveUntil: current.EffectiveUntil, Reason: patch.Reason,
	}
	if patch.Name != nil {
		result.Name = *patch.Name
	}
	if patch.Description != nil {
		result.Description = *patch.Description
	}
	if patch.ClearClassification {
		result.DataClassification = ""
	} else if patch.DataClassification != nil {
		result.DataClassification = *patch.DataClassification
	}
	if patch.ClearJurisdiction {
		result.Jurisdiction = ""
	} else if patch.Jurisdiction != nil {
		result.Jurisdiction = *patch.Jurisdiction
	}
	if patch.TriggerEvent != nil {
		result.TriggerEvent = *patch.TriggerEvent
	}
	if patch.LegalBasis != nil {
		result.LegalBasis = *patch.LegalBasis
	}
	if patch.RetentionDays != nil {
		result.RetentionDays = *patch.RetentionDays
	}
	if patch.ClearArchiveAfter {
		result.ArchiveAfterDays = nil
	} else if patch.ArchiveAfterDays != nil {
		result.ArchiveAfterDays = patch.ArchiveAfterDays
	}
	if patch.DispositionAction != nil {
		result.DispositionAction = *patch.DispositionAction
	}
	if patch.ReviewRequired != nil {
		result.ReviewRequired = *patch.ReviewRequired
	}
	if patch.Priority != nil {
		result.Priority = *patch.Priority
	}
	if patch.Status != nil {
		result.Status = *patch.Status
	}
	if patch.EffectiveFrom != nil {
		result.EffectiveFrom = *patch.EffectiveFrom
	}
	if patch.ClearEffectiveUntil {
		result.EffectiveUntil = nil
	} else if patch.EffectiveUntil != nil {
		result.EffectiveUntil = patch.EffectiveUntil
	}
	return result
}

func retentionPatchHasChange(patch models.RetentionSchedulePatch) bool {
	return patch.Name != nil || patch.Description != nil || patch.DataClassification != nil ||
		patch.ClearClassification || patch.Jurisdiction != nil || patch.ClearJurisdiction ||
		patch.TriggerEvent != nil || patch.LegalBasis != nil || patch.RetentionDays != nil ||
		patch.ArchiveAfterDays != nil || patch.ClearArchiveAfter || patch.DispositionAction != nil ||
		patch.ReviewRequired != nil || patch.Priority != nil || patch.Status != nil ||
		patch.EffectiveFrom != nil || patch.EffectiveUntil != nil || patch.ClearEffectiveUntil
}

func normalizeRetentionScheduleFilter(filter models.RetentionScheduleFilter) models.RetentionScheduleFilter {
	filter.RecordType = normalizeRecordType(filter.RecordType)
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	filter.DataClassification = strings.ToLower(strings.TrimSpace(filter.DataClassification))
	filter.Jurisdiction = strings.ToUpper(strings.TrimSpace(filter.Jurisdiction))
	filter.Search = strings.TrimSpace(filter.Search)
	filter.SortBy = strings.ToLower(strings.TrimSpace(filter.SortBy))
	filter.SortDirection = strings.ToLower(strings.TrimSpace(filter.SortDirection))
	filter.PaginationRequest = normalizeGovernancePagination(filter.PaginationRequest)
	return filter
}

func validateRetentionScheduleFilter(filter models.RetentionScheduleFilter) error {
	if filter.RecordType != "" && !validRecordType(filter.RecordType) {
		return fmt.Errorf("%w: invalid record_type filter", ErrDataGovernanceInvalid)
	}
	if filter.Status != "" && !stringIn(filter.Status, "draft", "active", "retired") {
		return fmt.Errorf("%w: invalid status filter", ErrDataGovernanceInvalid)
	}
	if !validClassification(filter.DataClassification) || !validJurisdiction(filter.Jurisdiction) ||
		utf8.RuneCountInString(filter.Search) > 200 ||
		(filter.SortBy != "" && !stringIn(filter.SortBy, "name", "record_type", "retention_days", "priority", "effective_from", "updated_at")) ||
		(filter.SortDirection != "" && !stringIn(filter.SortDirection, "asc", "desc")) {
		return fmt.Errorf("%w: invalid schedule filter or sort", ErrDataGovernanceInvalid)
	}
	return nil
}

func normalizeLegalHoldInput(input *models.LegalHoldInput) {
	input.Name = strings.TrimSpace(input.Name)
	input.MatterReference = strings.TrimSpace(input.MatterReference)
	input.Description = strings.TrimSpace(input.Description)
	input.LegalAuthority = strings.TrimSpace(input.LegalAuthority)
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.Reason = strings.TrimSpace(input.Reason)
	if len(input.Scope) == 0 {
		input.Scope = json.RawMessage(`{}`)
	}
	seen := make(map[string]struct{}, len(input.CustodianIDs))
	ids := make([]string, 0, len(input.CustodianIDs))
	for _, id := range input.CustodianIDs {
		id = strings.TrimSpace(id)
		if _, exists := seen[id]; id != "" && !exists {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	input.CustodianIDs = ids
}

func validateLegalHoldInput(input models.LegalHoldInput, now time.Time) error {
	if utf8.RuneCountInString(input.Name) < 3 || utf8.RuneCountInString(input.Name) > 200 ||
		utf8.RuneCountInString(input.MatterReference) > 200 ||
		utf8.RuneCountInString(input.Description) < 3 || utf8.RuneCountInString(input.Description) > 20000 ||
		utf8.RuneCountInString(input.LegalAuthority) < 3 || utf8.RuneCountInString(input.LegalAuthority) > 4000 ||
		len(input.CustodianIDs) > 500 || !validGovernanceReason(input.Reason) ||
		(input.ReviewDueAt != nil && !input.ReviewDueAt.After(now)) {
		return fmt.Errorf("%w: invalid hold details, review date, custodians, or reason", ErrDataGovernanceInvalid)
	}
	if err := validateGovernanceUUID("owner_user_id", input.OwnerUserID); err != nil {
		return err
	}
	for _, id := range input.CustodianIDs {
		if err := validateGovernanceUUID("custodian_id", id); err != nil {
			return err
		}
	}
	return validateGovernanceJSONObject(input.Scope, 32768, "scope")
}

func normalizeLegalHoldPatch(patch *models.LegalHoldPatch) {
	trimStringPointer(patch.Name, false)
	trimStringPointer(patch.MatterReference, false)
	trimStringPointer(patch.Description, false)
	trimStringPointer(patch.LegalAuthority, false)
	trimStringPointer(patch.OwnerUserID, false)
	patch.Reason = strings.TrimSpace(patch.Reason)
}

func validateMergedLegalHold(current models.LegalHold, patch models.LegalHoldPatch, now time.Time) error {
	input := models.LegalHoldInput{
		Name: current.Name, MatterReference: current.MatterReference, Description: current.Description,
		LegalAuthority: current.LegalAuthority, Scope: current.Scope, OwnerUserID: current.OwnerUserID,
		ReviewDueAt: current.ReviewDueAt, Reason: patch.Reason,
	}
	if patch.Name != nil {
		input.Name = *patch.Name
	}
	if patch.ClearMatterReference {
		input.MatterReference = ""
	} else if patch.MatterReference != nil {
		input.MatterReference = *patch.MatterReference
	}
	if patch.Description != nil {
		input.Description = *patch.Description
	}
	if patch.LegalAuthority != nil {
		input.LegalAuthority = *patch.LegalAuthority
	}
	if patch.Scope != nil {
		input.Scope = *patch.Scope
	}
	if patch.OwnerUserID != nil {
		input.OwnerUserID = *patch.OwnerUserID
	}
	if patch.ClearReviewDue {
		input.ReviewDueAt = nil
	} else if patch.ReviewDueAt != nil {
		input.ReviewDueAt = patch.ReviewDueAt
	}
	return validateLegalHoldInput(input, now)
}

func legalHoldPatchHasChange(patch models.LegalHoldPatch) bool {
	return patch.Name != nil || patch.MatterReference != nil || patch.ClearMatterReference ||
		patch.Description != nil || patch.LegalAuthority != nil || patch.Scope != nil ||
		patch.OwnerUserID != nil || patch.ReviewDueAt != nil || patch.ClearReviewDue
}

func validateGovernanceJSONObject(value json.RawMessage, maximum int, field string) error {
	if len(value) == 0 || len(value) > maximum || !json.Valid(value) {
		return fmt.Errorf("%w: %s must be valid JSON no larger than %d bytes", ErrDataGovernanceInvalid, field, maximum)
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	var object map[string]any
	if err := decoder.Decode(&object); err != nil || object == nil {
		return fmt.Errorf("%w: %s must be a JSON object", ErrDataGovernanceInvalid, field)
	}
	return nil
}

func validateGovernanceActor(organizationID, actorID, requestID string) error {
	if err := validateGovernanceUUID("organization_id", organizationID); err != nil {
		return err
	}
	if err := validateGovernanceUUID("actor_user_id", actorID); err != nil {
		return err
	}
	if utf8.RuneCountInString(strings.TrimSpace(requestID)) > 160 {
		return fmt.Errorf("%w: request_id is too long", ErrDataGovernanceInvalid)
	}
	return nil
}

func validateGovernanceActorObject(organizationID, id, actorID, requestID string) error {
	if err := validateGovernanceActor(organizationID, actorID, requestID); err != nil {
		return err
	}
	return validateGovernanceUUID("id", id)
}

func validateGovernanceObject(organizationID, id string) error {
	if err := validateGovernanceUUID("organization_id", organizationID); err != nil {
		return err
	}
	return validateGovernanceUUID("id", id)
}

func validateGovernanceUUID(field, value string) error {
	if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
		return fmt.Errorf("%w: %s must be a UUID", ErrDataGovernanceInvalid, field)
	}
	return nil
}

func normalizeRecordType(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_"), " ", "_")
}

func validRecordType(value string) bool {
	return stringIn(value, "asset", "audit", "audit_finding", "comment", "control", "evidence",
		"incident", "policy", "report", "risk", "vendor")
}

func validClassification(value string) bool {
	return value == "" || stringIn(value, "public", "internal", "confidential", "restricted", "personal", "special_category")
}

func validJurisdiction(value string) bool {
	if value == "" {
		return true
	}
	if len(value) < 2 || len(value) > 16 {
		return false
	}
	for _, char := range value {
		if (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' {
			return false
		}
	}
	return true
}

func validGovernanceReason(value string) bool { return validLongReason(value, 2000) }

func validLongReason(value string, maximum int) bool {
	length := utf8.RuneCountInString(strings.TrimSpace(value))
	return length >= 3 && length <= maximum
}

func stringIn(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func trimStringPointer(value *string, lower bool) {
	if value == nil {
		return
	}
	normalized := strings.TrimSpace(*value)
	if lower {
		normalized = strings.ToLower(normalized)
	}
	*value = normalized
}

func normalizeGovernancePagination(value models.PaginationRequest) models.PaginationRequest {
	if value.Page < 1 {
		value.Page = 1
	}
	if value.PageSize < 1 {
		value.PageSize = 25
	}
	if value.PageSize > 100 {
		value.PageSize = 100
	}
	return value
}

func mapDataGovernanceServiceError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repository.ErrDataGovernanceNotFound),
		errors.Is(err, repository.ErrGovernedRecordNotFound):
		return fmt.Errorf("%w: %v", ErrDataGovernanceNotFound, err)
	case errors.Is(err, repository.ErrDataGovernanceConflict):
		return fmt.Errorf("%w: %v", ErrDataGovernanceConflict, err)
	case errors.Is(err, repository.ErrDataGovernanceState):
		return fmt.Errorf("%w: %v", ErrDataGovernanceState, err)
	default:
		return err
	}
}
