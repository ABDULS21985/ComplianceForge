package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

const gdprBreachNotificationHours = 72

var (
	ErrIncidentNotFound          = errors.New("incident not found")
	ErrIncidentInvalid           = errors.New("invalid incident request")
	ErrIncidentInvalidID         = errors.New("invalid incident identifier")
	ErrIncidentInvalidTransition = errors.New("invalid incident status transition")
	ErrIncidentConflict          = errors.New("incident conflicts with current state")
	ErrIncidentVersionConflict   = errors.New("incident was modified by another request")
	ErrIncidentIdempotency       = errors.New("incident idempotency key conflicts with an existing action")
)

type IncidentManagementRepository interface {
	Create(context.Context, string, string, models.IncidentCreateInput) (*models.Incident, error)
	GetByID(context.Context, string, string) (*models.Incident, error)
	Update(context.Context, string, string, string, models.IncidentPatch) (*models.Incident, error)
	Delete(context.Context, string, string, string, int64) error
	List(context.Context, string, models.IncidentListFilter) ([]models.Incident, int, error)
	Transition(context.Context, string, string, string, models.IncidentTransitionInput) (*models.Incident, error)
	Escalate(context.Context, string, string, string, models.IncidentEscalationInput) (*models.Incident, error)
	AssessBreach(context.Context, string, string, string, models.IncidentBreachAssessmentInput) (*models.Incident, error)
	NotifyDPA(context.Context, string, string, string, models.IncidentDPANotificationInput) (*models.Incident, error)
	ListBreachDue(context.Context, string, time.Time, int) ([]models.Incident, error)
	CreateAssignment(context.Context, string, string, string, models.IncidentAssignmentInput) (*models.IncidentAssignment, *models.Incident, error)
	Unassign(context.Context, string, string, string, string, models.IncidentUnassignmentInput) (*models.IncidentAssignment, *models.Incident, error)
	ListAssignments(context.Context, string, string, bool) ([]models.IncidentAssignment, error)
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.IncidentEvent, int, error)
	Statistics(context.Context, string) (*models.IncidentStatistics, error)
}

// IncidentRepository is kept as a source-compatible name for report services
// and test doubles while the management contract is now complete.
type IncidentRepository = IncidentManagementRepository

type IncidentService struct {
	repository IncidentManagementRepository
	logger     zerolog.Logger
	now        func() time.Time
}

func NewIncidentService(repository IncidentManagementRepository, logger zerolog.Logger) *IncidentService {
	return &IncidentService{repository: repository, logger: logger.With().Str("service", "incident").Logger(), now: func() time.Time { return time.Now().UTC() }}
}

var _ IncidentManagementRepository = repository.IncidentRepository(nil)

func (s *IncidentService) Create(ctx context.Context, orgID, actorID string, input models.IncidentCreateInput) (*models.Incident, error) {
	if err := validateIncidentIdentity(orgID, actorID); err != nil {
		return nil, err
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Category = strings.TrimSpace(input.Category)
	input.Severity = models.IncidentSeverity(strings.ToLower(strings.TrimSpace(string(input.Severity))))
	if input.DetectedAt == nil {
		now := s.now()
		input.DetectedAt = &now
	} else {
		detected := input.DetectedAt.UTC().Truncate(time.Microsecond)
		input.DetectedAt = &detected
	}
	if input.OccurredAt != nil {
		occurred := input.OccurredAt.UTC().Truncate(time.Microsecond)
		input.OccurredAt = &occurred
	}
	if input.RelatedAssetID != nil && !isUUID(*input.RelatedAssetID) {
		return nil, fmt.Errorf("%w: related_asset_id must be a UUID", ErrIncidentInvalidID)
	}
	if len(input.Metadata) == 0 {
		input.Metadata = json.RawMessage(`{}`)
	}
	if err := validateIncidentCreate(input, s.now()); err != nil {
		return nil, err
	}
	item, err := s.repository.Create(ctx, orgID, actorID, input)
	if err != nil {
		return nil, s.translateError(err)
	}
	s.logger.Info().Str("incident_id", item.ID).Str("organization_id", orgID).Str("severity", string(item.Severity)).Msg("incident created")
	return item, nil
}

func (s *IncidentService) GetByID(ctx context.Context, orgID, id string) (*models.Incident, error) {
	if err := validateIncidentIDs(orgID, id); err != nil {
		return nil, err
	}
	item, err := s.repository.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, s.translateError(err)
	}
	decorateDeadline(item, s.now())
	return item, nil
}

func (s *IncidentService) Update(ctx context.Context, orgID, actorID, id string, patch models.IncidentPatch) (*models.Incident, error) {
	if err := validateMutationIdentity(orgID, actorID, id, patch.Version); err != nil {
		return nil, err
	}
	current, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if current.Status == models.IncidentStatusClosed || current.Status == models.IncidentStatusCancelled {
		return nil, fmt.Errorf("%w: closed or cancelled incidents must be reopened before editing", ErrIncidentConflict)
	}
	if patch.Title != nil {
		value := strings.TrimSpace(*patch.Title)
		patch.Title = &value
	}
	if patch.Description != nil {
		value := strings.TrimSpace(*patch.Description)
		patch.Description = &value
	}
	if patch.Category != nil {
		value := strings.TrimSpace(*patch.Category)
		patch.Category = &value
	}
	if patch.Severity != nil {
		value := models.IncidentSeverity(strings.ToLower(strings.TrimSpace(string(*patch.Severity))))
		patch.Severity = &value
	}
	for _, value := range []*string{patch.RootCause, patch.Impact, patch.LessonsLearned} {
		if value != nil {
			*value = strings.TrimSpace(*value)
		}
	}
	if (patch.ClearOccurred && patch.OccurredAt != nil) || (patch.ClearAsset && patch.RelatedAssetID != nil) || (patch.ClearFollowup && patch.FollowupDate != nil) {
		return nil, fmt.Errorf("%w: a field cannot be set and cleared in the same request", ErrIncidentInvalid)
	}
	if patch.RelatedAssetID != nil && !isUUID(*patch.RelatedAssetID) {
		return nil, fmt.Errorf("%w: related_asset_id must be a UUID", ErrIncidentInvalidID)
	}
	merged := *current
	applyIncidentPatch(&merged, patch)
	if patch.Severity != nil && *patch.Severity != current.Severity {
		return nil, fmt.Errorf("%w: use the escalation action to change severity", ErrIncidentConflict)
	}
	if err := validateIncidentRecord(&merged, s.now()); err != nil {
		return nil, err
	}
	item, err := s.repository.Update(ctx, orgID, actorID, id, patch)
	if err != nil {
		return nil, s.translateError(err)
	}
	s.logger.Info().Str("incident_id", id).Str("organization_id", orgID).Int64("version", item.Version).Msg("incident updated")
	return item, nil
}

func (s *IncidentService) Delete(ctx context.Context, orgID, actorID, id string, version int64) error {
	if err := validateMutationIdentity(orgID, actorID, id, version); err != nil {
		return err
	}
	item, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return err
	}
	if item.Status != models.IncidentStatusClosed && item.Status != models.IncidentStatusCancelled {
		return fmt.Errorf("%w: only closed or cancelled incidents may be deleted", ErrIncidentConflict)
	}
	if item.LegalHold {
		return fmt.Errorf("%w: incident is under legal hold", ErrIncidentConflict)
	}
	if item.RetentionUntil != nil && item.RetentionUntil.After(s.now()) {
		return fmt.Errorf("%w: incident retention period has not elapsed", ErrIncidentConflict)
	}
	if err := s.repository.Delete(ctx, orgID, actorID, id, version); err != nil {
		return s.translateError(err)
	}
	s.logger.Info().Str("incident_id", id).Str("organization_id", orgID).Msg("incident soft-deleted")
	return nil
}

func (s *IncidentService) List(ctx context.Context, orgID string, filter models.IncidentListFilter) ([]models.Incident, int, error) {
	if !isUUID(orgID) {
		return nil, 0, ErrIncidentInvalidID
	}
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	filter.Severity = strings.ToLower(strings.TrimSpace(filter.Severity))
	filter.Category = strings.TrimSpace(filter.Category)
	filter.AssigneeID = strings.TrimSpace(filter.AssigneeID)
	filter.Search = strings.TrimSpace(filter.Search)
	filter.Sort = strings.ToLower(strings.TrimSpace(filter.Sort))
	filter.Direction = strings.ToLower(strings.TrimSpace(filter.Direction))
	filter.PaginationRequest = normalizeIncidentPagination(filter.PaginationRequest)
	if filter.Status != "" && !validIncidentStatus(models.IncidentStatus(filter.Status)) {
		return nil, 0, fmt.Errorf("%w: unsupported status filter", ErrIncidentInvalid)
	}
	if filter.Severity != "" && !validIncidentSeverity(models.IncidentSeverity(filter.Severity)) {
		return nil, 0, fmt.Errorf("%w: unsupported severity filter", ErrIncidentInvalid)
	}
	if filter.AssigneeID != "" && !isUUID(filter.AssigneeID) {
		return nil, 0, ErrIncidentInvalidID
	}
	if len(filter.Category) > 100 {
		return nil, 0, fmt.Errorf("%w: category filter cannot exceed 100 characters", ErrIncidentInvalid)
	}
	if len(filter.Search) > 500 {
		return nil, 0, fmt.Errorf("%w: search filter cannot exceed 500 characters", ErrIncidentInvalid)
	}
	validSorts := map[string]bool{"": true, "reported_at": true, "updated_at": true, "severity": true, "status": true, "title": true, "notification_deadline": true}
	if !validSorts[filter.Sort] || (filter.Direction != "" && filter.Direction != "asc" && filter.Direction != "desc") {
		return nil, 0, fmt.Errorf("%w: unsupported sort", ErrIncidentInvalid)
	}
	items, total, err := s.repository.List(ctx, orgID, filter)
	if err != nil {
		return nil, 0, s.translateError(err)
	}
	for index := range items {
		decorateDeadline(&items[index], s.now())
	}
	return items, total, nil
}

func (s *IncidentService) Transition(ctx context.Context, orgID, actorID, id string, input models.IncidentTransitionInput) (*models.Incident, error) {
	if err := validateMutationIdentity(orgID, actorID, id, input.Version); err != nil {
		return nil, err
	}
	input.Status = models.IncidentStatus(strings.ToLower(strings.TrimSpace(string(input.Status))))
	input.Reason = strings.TrimSpace(input.Reason)
	current, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if !isValidIncidentTransition(current.Status, input.Status) {
		return nil, fmt.Errorf("%w: %s to %s", ErrIncidentInvalidTransition, current.Status, input.Status)
	}
	if requiresTransitionReason(current.Status, input.Status) && !validText(input.Reason, 3, 4000) {
		return nil, fmt.Errorf("%w: a reason between 3 and 4000 characters is required", ErrIncidentInvalid)
	}
	if input.Status == models.IncidentStatusClosed && strings.TrimSpace(current.LessonsLearned) == "" {
		return nil, fmt.Errorf("%w: lessons_learned is required before closing", ErrIncidentConflict)
	}
	if input.Status == models.IncidentStatusClosed && current.IsBreachNotifiable && current.DPANotifiedAt == nil {
		return nil, fmt.Errorf("%w: a notifiable breach cannot be closed before the DPA notification is recorded", ErrIncidentConflict)
	}
	item, err := s.repository.Transition(ctx, orgID, actorID, id, input)
	if err != nil {
		return nil, s.translateError(err)
	}
	s.logger.Info().Str("incident_id", id).Str("organization_id", orgID).Str("from_status", string(current.Status)).Str("to_status", string(input.Status)).Msg("incident lifecycle changed")
	return item, nil
}

func (s *IncidentService) Cancel(ctx context.Context, orgID, actorID, id string, version int64, reason string) (*models.Incident, error) {
	return s.Transition(ctx, orgID, actorID, id, models.IncidentTransitionInput{Status: models.IncidentStatusCancelled, Version: version, Reason: reason})
}

func (s *IncidentService) Close(ctx context.Context, orgID, actorID, id string, version int64, reason string) (*models.Incident, error) {
	return s.Transition(ctx, orgID, actorID, id, models.IncidentTransitionInput{Status: models.IncidentStatusClosed, Version: version, Reason: reason})
}

func (s *IncidentService) Reopen(ctx context.Context, orgID, actorID, id string, version int64, reason string) (*models.Incident, error) {
	current, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	target := models.IncidentStatusInvestigating
	if current.Status == models.IncidentStatusCancelled {
		target = models.IncidentStatusReported
	}
	return s.Transition(ctx, orgID, actorID, id, models.IncidentTransitionInput{Status: target, Version: version, Reason: reason})
}

func (s *IncidentService) Escalate(ctx context.Context, orgID, actorID, id string, input models.IncidentEscalationInput) (*models.Incident, error) {
	if err := validateMutationIdentity(orgID, actorID, id, input.Version); err != nil {
		return nil, err
	}
	input.Severity = models.IncidentSeverity(strings.ToLower(strings.TrimSpace(string(input.Severity))))
	input.Reason = strings.TrimSpace(input.Reason)
	if !validIncidentSeverity(input.Severity) || !validText(input.Reason, 3, 2000) {
		return nil, fmt.Errorf("%w: valid higher severity and reason are required", ErrIncidentInvalid)
	}
	current, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if severityRank(input.Severity) <= severityRank(current.Severity) {
		return nil, fmt.Errorf("%w: escalation must increase severity", ErrIncidentConflict)
	}
	if current.Status == models.IncidentStatusClosed || current.Status == models.IncidentStatusCancelled {
		return nil, fmt.Errorf("%w: terminal incident must be reopened before escalation", ErrIncidentConflict)
	}
	item, err := s.repository.Escalate(ctx, orgID, actorID, id, input)
	if err != nil {
		return nil, s.translateError(err)
	}
	return item, nil
}

func (s *IncidentService) AssessBreach(ctx context.Context, orgID, actorID, id string, input models.IncidentBreachAssessmentInput) (*models.Incident, error) {
	if err := validateMutationIdentity(orgID, actorID, id, input.Version); err != nil {
		return nil, err
	}
	input.Status = models.BreachAssessmentStatus(strings.ToLower(strings.TrimSpace(string(input.Status))))
	input.Reason = strings.TrimSpace(input.Reason)
	input.BreachNature = strings.TrimSpace(input.BreachNature)
	input.LikelyConsequences = strings.TrimSpace(input.LikelyConsequences)
	input.MitigationMeasures = strings.TrimSpace(input.MitigationMeasures)
	if len(input.DataCategories) > 20 {
		return nil, fmt.Errorf("%w: no more than 20 data categories are allowed", ErrIncidentInvalid)
	}
	for _, category := range input.DataCategories {
		if !validText(category, 1, 100) {
			return nil, fmt.Errorf("%w: data categories must contain 1 to 100 characters", ErrIncidentInvalid)
		}
	}
	input.DataCategories = normalizeStrings(input.DataCategories, 20, 100)
	if input.Status != models.BreachAssessmentNotifiable && input.Status != models.BreachAssessmentNotNotifiable {
		return nil, fmt.Errorf("%w: status must be notifiable or not_notifiable", ErrIncidentInvalid)
	}
	if !validText(input.Reason, 3, 4000) {
		return nil, fmt.Errorf("%w: assessment reason is required", ErrIncidentInvalid)
	}
	if input.Status == models.BreachAssessmentNotifiable {
		if !input.IsDataBreach || input.AwarenessAt == nil {
			return nil, fmt.Errorf("%w: a notifiable assessment requires a personal-data breach and awareness_at", ErrIncidentInvalid)
		}
		awareness := input.AwarenessAt.UTC()
		input.AwarenessAt = &awareness
		if awareness.After(s.now().Add(5 * time.Minute)) {
			return nil, fmt.Errorf("%w: awareness_at cannot be in the future", ErrIncidentInvalid)
		}
	}
	if input.DataSubjectsAffected != nil && *input.DataSubjectsAffected < 0 || input.RecordsAffected != nil && *input.RecordsAffected < 0 {
		return nil, fmt.Errorf("%w: affected counts cannot be negative", ErrIncidentInvalid)
	}
	current, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if current.DPANotifiedAt != nil {
		return nil, fmt.Errorf("%w: a notified assessment is immutable", ErrIncidentConflict)
	}
	item, err := s.repository.AssessBreach(ctx, orgID, actorID, id, input)
	if err != nil {
		return nil, s.translateError(err)
	}
	decorateDeadline(item, s.now())
	return item, nil
}

func (s *IncidentService) NotifyDPA(ctx context.Context, orgID, actorID, id string, input models.IncidentDPANotificationInput) (*models.Incident, error) {
	if err := validateMutationIdentity(orgID, actorID, id, input.Version); err != nil {
		return nil, err
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Reference = strings.TrimSpace(input.Reference)
	input.Reason = strings.TrimSpace(input.Reason)
	input.NotifiedAt = input.NotifiedAt.UTC().Truncate(time.Microsecond)
	if !isUUID(input.IdempotencyKey) || input.NotifiedAt.IsZero() || input.NotifiedAt.After(s.now().Add(5*time.Minute)) ||
		!validText(input.Reference, 1, 200) || !validText(input.Reason, 3, 4000) {
		return nil, fmt.Errorf("%w: idempotency_key, notified_at, reference, and reason are required", ErrIncidentInvalid)
	}
	current, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if !current.IsBreachNotifiable && current.DPANotifiedAt == nil {
		return nil, fmt.Errorf("%w: only assessed notifiable breaches can be notified", ErrIncidentConflict)
	}
	if current.BreachAwarenessAt != nil && input.NotifiedAt.Before(*current.BreachAwarenessAt) {
		return nil, fmt.Errorf("%w: notified_at cannot precede breach awareness", ErrIncidentInvalid)
	}
	item, err := s.repository.NotifyDPA(ctx, orgID, actorID, id, input)
	if err != nil {
		return nil, s.translateError(err)
	}
	decorateDeadline(item, s.now())
	return item, nil
}

func (s *IncidentService) ListBreachDue(ctx context.Context, orgID string, horizonHours, limit int) ([]models.Incident, error) {
	if !isUUID(orgID) {
		return nil, ErrIncidentInvalidID
	}
	if horizonHours < 1 {
		horizonHours = 24
	}
	if horizonHours > 24*30 {
		return nil, fmt.Errorf("%w: horizon_hours cannot exceed 720", ErrIncidentInvalid)
	}
	if limit < 1 || limit > 200 {
		limit = 100
	}
	now := s.now()
	items, err := s.repository.ListBreachDue(ctx, orgID, now.Add(time.Duration(horizonHours)*time.Hour), limit)
	if err != nil {
		return nil, s.translateError(err)
	}
	for index := range items {
		decorateDeadline(&items[index], now)
	}
	return items, nil
}

func (s *IncidentService) CreateAssignment(ctx context.Context, orgID, actorID, incidentID string, input models.IncidentAssignmentInput) (*models.IncidentAssignment, *models.Incident, error) {
	if err := validateMutationIdentity(orgID, actorID, incidentID, input.Version); err != nil {
		return nil, nil, err
	}
	input.AssigneeID = strings.TrimSpace(input.AssigneeID)
	input.Role = models.IncidentAssignmentRole(strings.ToLower(strings.TrimSpace(string(input.Role))))
	input.Reason = strings.TrimSpace(input.Reason)
	if !isUUID(input.AssigneeID) || !validAssignmentRole(input.Role) || !validText(input.Reason, 3, 2000) {
		return nil, nil, fmt.Errorf("%w: valid assignee, role, and reason are required", ErrIncidentInvalid)
	}
	item, err := s.GetByID(ctx, orgID, incidentID)
	if err != nil {
		return nil, nil, err
	}
	if item.Status == models.IncidentStatusClosed || item.Status == models.IncidentStatusCancelled {
		return nil, nil, fmt.Errorf("%w: terminal incident cannot be assigned", ErrIncidentConflict)
	}
	assignment, updated, err := s.repository.CreateAssignment(ctx, orgID, actorID, incidentID, input)
	if err != nil {
		return nil, nil, s.translateError(err)
	}
	return assignment, updated, nil
}

func (s *IncidentService) Unassign(ctx context.Context, orgID, actorID, incidentID, assignmentID string, input models.IncidentUnassignmentInput) (*models.IncidentAssignment, *models.Incident, error) {
	if err := validateMutationIdentity(orgID, actorID, incidentID, input.Version); err != nil || !isUUID(assignmentID) {
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, ErrIncidentInvalidID
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if !validText(input.Reason, 3, 2000) {
		return nil, nil, fmt.Errorf("%w: unassignment reason is required", ErrIncidentInvalid)
	}
	assignment, updated, err := s.repository.Unassign(ctx, orgID, actorID, incidentID, assignmentID, input)
	if err != nil {
		return nil, nil, s.translateError(err)
	}
	return assignment, updated, nil
}

func (s *IncidentService) ListAssignments(ctx context.Context, orgID, incidentID string, activeOnly bool) ([]models.IncidentAssignment, error) {
	if err := validateIncidentIDs(orgID, incidentID); err != nil {
		return nil, err
	}
	items, err := s.repository.ListAssignments(ctx, orgID, incidentID, activeOnly)
	if err != nil {
		return nil, s.translateError(err)
	}
	return items, nil
}

func (s *IncidentService) ListEvents(ctx context.Context, orgID, incidentID string, pagination models.PaginationRequest) ([]models.IncidentEvent, int, error) {
	if err := validateIncidentIDs(orgID, incidentID); err != nil {
		return nil, 0, err
	}
	pagination = normalizeIncidentPagination(pagination)
	items, total, err := s.repository.ListEvents(ctx, orgID, incidentID, pagination)
	if err != nil {
		return nil, 0, s.translateError(err)
	}
	return items, total, nil
}

func (s *IncidentService) Statistics(ctx context.Context, orgID string) (*models.IncidentStatistics, error) {
	if !isUUID(orgID) {
		return nil, ErrIncidentInvalidID
	}
	stats, err := s.repository.Statistics(ctx, orgID)
	if err != nil {
		return nil, s.translateError(err)
	}
	return stats, nil
}

func (s *IncidentService) translateError(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrIncidentNotFound
	case errors.Is(err, repository.ErrIncidentVersionConflict):
		return ErrIncidentVersionConflict
	case errors.Is(err, repository.ErrIncidentIdempotencyConflict):
		return ErrIncidentIdempotency
	default:
		return err
	}
}

func isValidIncidentTransition(current, next models.IncidentStatus) bool {
	transitions := map[models.IncidentStatus]map[models.IncidentStatus]bool{
		models.IncidentStatusReported:      {models.IncidentStatusTriaged: true, models.IncidentStatusCancelled: true},
		models.IncidentStatusTriaged:       {models.IncidentStatusInvestigating: true, models.IncidentStatusCancelled: true},
		models.IncidentStatusInvestigating: {models.IncidentStatusContained: true, models.IncidentStatusResolved: true, models.IncidentStatusCancelled: true},
		models.IncidentStatusContained:     {models.IncidentStatusInvestigating: true, models.IncidentStatusResolved: true, models.IncidentStatusCancelled: true},
		models.IncidentStatusResolved:      {models.IncidentStatusClosed: true, models.IncidentStatusInvestigating: true},
		models.IncidentStatusClosed:        {models.IncidentStatusInvestigating: true},
		models.IncidentStatusCancelled:     {models.IncidentStatusReported: true},
	}
	return transitions[current][next]
}

func requiresTransitionReason(current, next models.IncidentStatus) bool {
	return next == models.IncidentStatusCancelled ||
		(current == models.IncidentStatusResolved || current == models.IncidentStatusClosed || current == models.IncidentStatusCancelled)
}

func validateIncidentCreate(input models.IncidentCreateInput, now time.Time) error {
	if !validText(input.Title, 1, 200) || !validText(input.Description, 1, 20000) || !validText(input.Category, 1, 100) {
		return fmt.Errorf("%w: title, description, and category are required and bounded", ErrIncidentInvalid)
	}
	if !validIncidentSeverity(input.Severity) {
		return fmt.Errorf("%w: unsupported severity", ErrIncidentInvalid)
	}
	if input.DetectedAt == nil || input.DetectedAt.After(now.Add(5*time.Minute)) {
		return fmt.Errorf("%w: detected_at cannot be in the future", ErrIncidentInvalid)
	}
	if input.OccurredAt != nil && input.OccurredAt.After(now.Add(5*time.Minute)) {
		return fmt.Errorf("%w: occurred_at cannot be in the future", ErrIncidentInvalid)
	}
	if input.OccurredAt != nil && input.DetectedAt != nil && input.OccurredAt.After(*input.DetectedAt) {
		return fmt.Errorf("%w: occurred_at cannot be after detected_at", ErrIncidentInvalid)
	}
	if input.RetentionUntil != nil && input.RetentionUntil.Before(now) {
		return fmt.Errorf("%w: retention_until cannot be in the past", ErrIncidentInvalid)
	}
	return validateIncidentJSONObject(input.Metadata, 64<<10)
}

func validateIncidentRecord(item *models.Incident, now time.Time) error {
	if !validTextOrEmpty(item.RootCause, 20000) || !validTextOrEmpty(item.Impact, 20000) || !validTextOrEmpty(item.LessonsLearned, 20000) {
		return fmt.Errorf("%w: investigation narrative fields cannot exceed 20000 characters", ErrIncidentInvalid)
	}
	return validateIncidentCreate(models.IncidentCreateInput{Title: item.Title, Description: item.Description,
		Category: item.Category, Severity: item.Severity, DetectedAt: item.DetectedAt,
		OccurredAt: item.OccurredAt, RelatedAssetID: item.RelatedAssetID,
		FollowupDate: item.FollowupDate, RetentionUntil: item.RetentionUntil, Metadata: item.Metadata}, now)
}

func applyIncidentPatch(item *models.Incident, patch models.IncidentPatch) {
	if patch.Title != nil {
		item.Title = *patch.Title
	}
	if patch.Description != nil {
		item.Description = *patch.Description
	}
	if patch.Category != nil {
		item.Category = *patch.Category
	}
	if patch.Severity != nil {
		item.Severity = *patch.Severity
	}
	if patch.DetectedAt != nil {
		item.DetectedAt = patch.DetectedAt
	}
	if patch.ClearOccurred {
		item.OccurredAt = nil
	} else if patch.OccurredAt != nil {
		item.OccurredAt = patch.OccurredAt
	}
	if patch.RootCause != nil {
		item.RootCause = strings.TrimSpace(*patch.RootCause)
	}
	if patch.Impact != nil {
		item.Impact = strings.TrimSpace(*patch.Impact)
	}
	if patch.LessonsLearned != nil {
		item.LessonsLearned = strings.TrimSpace(*patch.LessonsLearned)
	}
	if patch.ClearAsset {
		item.RelatedAssetID = nil
	} else if patch.RelatedAssetID != nil {
		item.RelatedAssetID = patch.RelatedAssetID
	}
	if patch.ClearFollowup {
		item.FollowupDate = nil
	} else if patch.FollowupDate != nil {
		item.FollowupDate = patch.FollowupDate
	}
	if patch.RetentionUntil != nil {
		item.RetentionUntil = patch.RetentionUntil
	}
	if patch.LegalHold != nil {
		item.LegalHold = *patch.LegalHold
	}
	if len(patch.Metadata) > 0 {
		item.Metadata = patch.Metadata
	}
}

func decorateDeadline(item *models.Incident, now time.Time) {
	if item == nil || item.NotificationDeadline == nil || item.DPANotifiedAt != nil {
		return
	}
	hours := item.NotificationDeadline.Sub(now).Hours()
	item.HoursRemaining = &hours
	switch {
	case hours < 0:
		item.DeadlineState = "overdue"
	case hours <= 24:
		item.DeadlineState = "urgent"
	default:
		item.DeadlineState = "upcoming"
	}
}

func normalizeIncidentPagination(value models.PaginationRequest) models.PaginationRequest {
	if value.Page < 1 {
		value.Page = 1
	}
	if value.PageSize < 1 || value.PageSize > 100 {
		value.PageSize = 20
	}
	return value
}

func validateIncidentIdentity(orgID, actorID string) error {
	if !isUUID(orgID) || !isUUID(actorID) {
		return ErrIncidentInvalidID
	}
	return nil
}

func validateIncidentIDs(orgID, id string) error {
	if !isUUID(orgID) || !isUUID(id) {
		return ErrIncidentInvalidID
	}
	return nil
}

func validateMutationIdentity(orgID, actorID, id string, version int64) error {
	if err := validateIncidentIdentity(orgID, actorID); err != nil {
		return err
	}
	if !isUUID(id) {
		return ErrIncidentInvalidID
	}
	if version < 1 {
		return fmt.Errorf("%w: version must be positive", ErrIncidentInvalid)
	}
	return nil
}

func validIncidentSeverity(value models.IncidentSeverity) bool {
	return value == models.IncidentSeverityCritical || value == models.IncidentSeverityHigh || value == models.IncidentSeverityMedium || value == models.IncidentSeverityLow
}

func validIncidentStatus(value models.IncidentStatus) bool {
	return value == models.IncidentStatusReported || value == models.IncidentStatusTriaged || value == models.IncidentStatusInvestigating || value == models.IncidentStatusContained || value == models.IncidentStatusResolved || value == models.IncidentStatusClosed || value == models.IncidentStatusCancelled
}

func validAssignmentRole(value models.IncidentAssignmentRole) bool {
	return value == models.IncidentAssignmentPrimary || value == models.IncidentAssignmentInvestigator || value == models.IncidentAssignmentObserver
}

func severityRank(value models.IncidentSeverity) int {
	switch value {
	case models.IncidentSeverityLow:
		return 1
	case models.IncidentSeverityMedium:
		return 2
	case models.IncidentSeverityHigh:
		return 3
	case models.IncidentSeverityCritical:
		return 4
	default:
		return 0
	}
}

func validText(value string, minimum, maximum int) bool {
	length := len([]rune(strings.TrimSpace(value)))
	return length >= minimum && length <= maximum
}

func validTextOrEmpty(value string, maximum int) bool {
	return len([]rune(strings.TrimSpace(value))) <= maximum
}

func validateIncidentJSONObject(value json.RawMessage, maximum int) error {
	if len(value) > maximum || !json.Valid(value) {
		return fmt.Errorf("%w: metadata must be valid JSON no larger than %d bytes", ErrIncidentInvalid, maximum)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(value, &object); err != nil || object == nil {
		return fmt.Errorf("%w: metadata must be a JSON object", ErrIncidentInvalid)
	}
	return nil
}

func normalizeStrings(values []string, maximumItems, maximumLength int) []string {
	if len(values) > maximumItems {
		values = values[:maximumItems]
	}
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len([]rune(value)) > maximumLength || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func isUUID(value string) bool { _, err := uuid.Parse(strings.TrimSpace(value)); return err == nil }
