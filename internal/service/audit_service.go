package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrAuditNotFound            = errors.New("audit not found")
	ErrAuditInvalid             = errors.New("invalid audit")
	ErrAuditInvalidID           = errors.New("invalid audit identifier")
	ErrAuditInvalidReference    = errors.New("audit references a resource outside this organization")
	ErrAuditInvalidTransition   = errors.New("invalid audit status transition")
	ErrAuditConflict            = errors.New("audit request conflicts with current state")
	ErrFindingNotFound          = errors.New("audit finding not found")
	ErrFindingInvalid           = errors.New("invalid audit finding")
	ErrFindingInvalidTransition = errors.New("invalid audit finding status transition")
)

type AuditManagementRepository interface {
	Create(context.Context, *models.Audit) (*models.Audit, error)
	GetByID(context.Context, string, string) (*models.Audit, error)
	Update(context.Context, string, *models.Audit) (*models.Audit, error)
	Delete(context.Context, string, string) error
	List(context.Context, string, models.AuditListFilter) ([]models.Audit, int, error)
	ListUpcoming(context.Context, string, int) ([]models.Audit, error)
	CreateFinding(context.Context, *models.AuditFinding) (*models.AuditFinding, error)
	GetFindingByID(context.Context, string, string, string) (*models.AuditFinding, error)
	UpdateFinding(context.Context, string, string, *models.AuditFinding) (*models.AuditFinding, error)
	DeleteFinding(context.Context, string, string, string) error
	ListFindings(context.Context, string, string, models.PaginationRequest) ([]models.AuditFinding, int, error)
	FindingStats(context.Context, string, string) (*models.AuditFindingStats, error)
}

// AuditRepository is kept as the read/reporting-facing name while all callers
// migrate to the explicit tenant-aware contract above.
type AuditRepository = AuditManagementRepository

type AuditService struct {
	repository AuditManagementRepository
	logger     zerolog.Logger
}

func NewAuditService(repository AuditManagementRepository, logger zerolog.Logger) *AuditService {
	return &AuditService{repository: repository, logger: logger.With().Str("service", "audit").Logger()}
}

func (s *AuditService) Create(ctx context.Context, orgID, userID string, input models.AuditCreateInput) (*models.Audit, error) {
	if !validUUID(orgID) || !validUUID(userID) {
		return nil, ErrAuditInvalidID
	}
	normalizeAuditCreate(&input)
	start, end, err := validateAuditCreate(input)
	if err != nil {
		return nil, err
	}
	audit := &models.Audit{
		TenantModel: models.TenantModel{OrganizationID: orgID},
		Title:       input.Title, Description: input.Description, Type: input.AuditType,
		Status: models.AuditStatusPlanned, LeadAuditorID: input.LeadAuditorID,
		Scope: input.Scope, ScheduledStartDate: &start, ScheduledEndDate: &end,
		FrameworkID: input.FrameworkID, CreatedBy: userID, Metadata: input.Metadata,
	}
	item, err := s.repository.Create(ctx, audit)
	if err != nil {
		return nil, mapAuditWriteError(err)
	}
	s.logger.Info().Str("organization_id", orgID).Str("audit_id", item.ID).Str("audit_ref", item.AuditRef).Msg("audit created")
	return item, nil
}

func (s *AuditService) GetByID(ctx context.Context, orgID, id string) (*models.Audit, error) {
	if !validUUID(orgID) || !validUUID(id) {
		return nil, ErrAuditInvalidID
	}
	item, err := s.repository.GetByID(ctx, orgID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAuditNotFound
	}
	return item, err
}

func (s *AuditService) Update(ctx context.Context, orgID, id string, patch models.AuditPatch) (*models.Audit, error) {
	if !validUUID(orgID) || !validUUID(id) {
		return nil, ErrAuditInvalidID
	}
	if auditPatchEmpty(patch) {
		return nil, fmt.Errorf("%w: no fields supplied", ErrAuditInvalid)
	}
	audit, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if audit.Status == models.AuditStatusClosed || audit.Status == models.AuditStatusCancelled {
		return nil, fmt.Errorf("%w: closed and cancelled audits are immutable", ErrAuditConflict)
	}
	if err := applyAuditPatch(audit, patch); err != nil {
		return nil, err
	}
	if err := validateAudit(audit); err != nil {
		return nil, err
	}
	updated, err := s.repository.Update(ctx, orgID, audit)
	if err != nil {
		return nil, mapAuditWriteError(err)
	}
	s.logger.Info().Str("organization_id", orgID).Str("audit_id", id).Msg("audit updated")
	return updated, nil
}

func (s *AuditService) Delete(ctx context.Context, orgID, id string) error {
	audit, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return err
	}
	if audit.Status != models.AuditStatusPlanned && audit.Status != models.AuditStatusCancelled {
		return fmt.Errorf("%w: executed audits must be retained", ErrAuditConflict)
	}
	if err := s.repository.Delete(ctx, orgID, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAuditNotFound
		}
		return err
	}
	s.logger.Info().Str("organization_id", orgID).Str("audit_id", id).Msg("audit soft-deleted")
	return nil
}

func (s *AuditService) List(ctx context.Context, orgID string, filter models.AuditListFilter) ([]models.Audit, int, error) {
	if !validUUID(orgID) {
		return nil, 0, ErrAuditInvalidID
	}
	filter.PaginationRequest = normalizeAuditPagination(filter.PaginationRequest)
	filter.Status = strings.TrimSpace(strings.ToLower(filter.Status))
	filter.AuditType = strings.TrimSpace(strings.ToLower(filter.AuditType))
	filter.LeadAuditorID = strings.TrimSpace(filter.LeadAuditorID)
	filter.FrameworkID = strings.TrimSpace(filter.FrameworkID)
	filter.Search = strings.TrimSpace(filter.Search)
	if filter.Status != "" && !allowed(filter.Status, auditStatuses()...) {
		return nil, 0, fmt.Errorf("%w: unsupported status filter", ErrAuditInvalid)
	}
	if filter.AuditType != "" && !allowed(filter.AuditType, auditTypes()...) {
		return nil, 0, fmt.Errorf("%w: unsupported audit type filter", ErrAuditInvalid)
	}
	for _, id := range []string{filter.LeadAuditorID, filter.FrameworkID} {
		if id != "" && !validUUID(id) {
			return nil, 0, ErrAuditInvalidID
		}
	}
	if len(filter.Search) > 200 {
		return nil, 0, fmt.Errorf("%w: search is too long", ErrAuditInvalid)
	}
	return s.repository.List(ctx, orgID, filter)
}

func (s *AuditService) ListUpcoming(ctx context.Context, orgID string, limit int) ([]models.Audit, error) {
	if !validUUID(orgID) {
		return nil, ErrAuditInvalidID
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	return s.repository.ListUpcoming(ctx, orgID, limit)
}

func (s *AuditService) Start(ctx context.Context, orgID, id string) (*models.Audit, error) {
	audit, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if audit.Status != models.AuditStatusPlanned {
		return nil, fmt.Errorf("%w: only planned audits can start", ErrAuditInvalidTransition)
	}
	now := dateOnly(time.Now().UTC())
	audit.Status = models.AuditStatusInProgress
	audit.ActualStartDate = &now
	audit.ActualEndDate = nil
	updated, err := s.repository.Update(ctx, orgID, audit)
	if err != nil {
		return nil, mapAuditWriteError(err)
	}
	s.logger.Info().Str("organization_id", orgID).Str("audit_id", id).Msg("audit started")
	return updated, nil
}

func (s *AuditService) Complete(ctx context.Context, orgID, id string) (*models.Audit, error) {
	audit, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if audit.Status != models.AuditStatusInProgress {
		return nil, fmt.Errorf("%w: only in-progress audits can complete", ErrAuditInvalidTransition)
	}
	now := dateOnly(time.Now().UTC())
	if audit.ActualStartDate != nil && now.Before(*audit.ActualStartDate) {
		now = *audit.ActualStartDate
	}
	audit.Status = models.AuditStatusCompleted
	audit.ActualEndDate = &now
	return s.repository.Update(ctx, orgID, audit)
}

func (s *AuditService) Close(ctx context.Context, orgID, id string) (*models.Audit, error) {
	audit, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if audit.Status != models.AuditStatusCompleted {
		return nil, fmt.Errorf("%w: only completed audits can close", ErrAuditInvalidTransition)
	}
	stats, err := s.repository.FindingStats(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if stats.Open+stats.InProgress > 0 {
		return nil, fmt.Errorf("%w: open findings must be resolved or accepted before closure", ErrAuditConflict)
	}
	audit.Status = models.AuditStatusClosed
	return s.repository.Update(ctx, orgID, audit)
}

func (s *AuditService) Cancel(ctx context.Context, orgID, id string) (*models.Audit, error) {
	audit, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if audit.Status != models.AuditStatusPlanned {
		return nil, fmt.Errorf("%w: only planned audits can be cancelled", ErrAuditInvalidTransition)
	}
	audit.Status = models.AuditStatusCancelled
	return s.repository.Update(ctx, orgID, audit)
}

func (s *AuditService) CreateFinding(ctx context.Context, orgID, auditID, userID string, input models.AuditFindingInput) (*models.AuditFinding, error) {
	if !validUUID(orgID) || !validUUID(auditID) || !validUUID(userID) {
		return nil, ErrAuditInvalidID
	}
	audit, err := s.GetByID(ctx, orgID, auditID)
	if err != nil {
		return nil, err
	}
	if audit.Status != models.AuditStatusPlanned && audit.Status != models.AuditStatusInProgress {
		return nil, fmt.Errorf("%w: findings cannot be added after execution completes", ErrAuditConflict)
	}
	normalizeFindingInput(&input)
	dueDate, err := validateFindingInput(input)
	if err != nil {
		return nil, err
	}
	item := &models.AuditFinding{
		TenantModel: models.TenantModel{OrganizationID: orgID}, AuditID: auditID,
		ControlID: input.ControlID, Title: input.Title, Description: input.Description,
		Severity: input.Severity, Status: models.FindingStatusOpen,
		FindingType: input.FindingType, RootCause: input.RootCause,
		Recommendation: input.Recommendation, RemediationPlan: input.RemediationPlan,
		ResponsibleUserID: input.ResponsibleUserID, DueDate: &dueDate,
		CreatedBy: userID, Metadata: input.Metadata,
	}
	created, err := s.repository.CreateFinding(ctx, item)
	if err != nil {
		return nil, mapAuditWriteError(err)
	}
	s.logger.Info().Str("organization_id", orgID).Str("audit_id", auditID).Str("finding_id", created.ID).Msg("audit finding created")
	return created, nil
}

func (s *AuditService) GetFinding(ctx context.Context, orgID, auditID, id string) (*models.AuditFinding, error) {
	if !validUUID(orgID) || !validUUID(auditID) || !validUUID(id) {
		return nil, ErrAuditInvalidID
	}
	item, err := s.repository.GetFindingByID(ctx, orgID, auditID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFindingNotFound
	}
	return item, err
}

func (s *AuditService) UpdateFinding(ctx context.Context, orgID, auditID, id string, patch models.AuditFindingPatch) (*models.AuditFinding, error) {
	if !validUUID(orgID) || !validUUID(auditID) || !validUUID(id) {
		return nil, ErrAuditInvalidID
	}
	if findingPatchEmpty(patch) {
		return nil, fmt.Errorf("%w: no fields supplied", ErrFindingInvalid)
	}
	audit, err := s.GetByID(ctx, orgID, auditID)
	if err != nil {
		return nil, err
	}
	if audit.Status == models.AuditStatusClosed || audit.Status == models.AuditStatusCancelled {
		return nil, fmt.Errorf("%w: findings on closed or cancelled audits are immutable", ErrAuditConflict)
	}
	finding, err := s.GetFinding(ctx, orgID, auditID, id)
	if err != nil {
		return nil, err
	}
	previousStatus := finding.Status
	if err := applyFindingPatch(finding, patch); err != nil {
		return nil, err
	}
	if !validFindingTransition(previousStatus, finding.Status) {
		return nil, fmt.Errorf("%w: %s to %s", ErrFindingInvalidTransition, previousStatus, finding.Status)
	}
	if finding.Status == models.FindingStatusResolved || finding.Status == models.FindingStatusClosed {
		if finding.ResolvedAt == nil {
			now := time.Now().UTC()
			finding.ResolvedAt = &now
		}
	} else {
		finding.ResolvedAt = nil
	}
	if finding.Status != models.FindingStatusAccepted {
		finding.AcceptedRiskReason = nil
	}
	if err := validateFinding(finding); err != nil {
		return nil, err
	}
	updated, err := s.repository.UpdateFinding(ctx, orgID, auditID, finding)
	if err != nil {
		return nil, mapAuditWriteError(err)
	}
	s.logger.Info().Str("organization_id", orgID).Str("audit_id", auditID).Str("finding_id", id).Msg("audit finding updated")
	return updated, nil
}

func (s *AuditService) DeleteFinding(ctx context.Context, orgID, auditID, id string) error {
	audit, err := s.GetByID(ctx, orgID, auditID)
	if err != nil {
		return err
	}
	if audit.Status != models.AuditStatusPlanned && audit.Status != models.AuditStatusInProgress {
		return fmt.Errorf("%w: findings must be retained after audit completion", ErrAuditConflict)
	}
	if _, err := s.GetFinding(ctx, orgID, auditID, id); err != nil {
		return err
	}
	if err := s.repository.DeleteFinding(ctx, orgID, auditID, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFindingNotFound
		}
		return err
	}
	return nil
}

func (s *AuditService) ListFindings(ctx context.Context, orgID, auditID string, pagination models.PaginationRequest) ([]models.AuditFinding, int, error) {
	if _, err := s.GetByID(ctx, orgID, auditID); err != nil {
		return nil, 0, err
	}
	return s.repository.ListFindings(ctx, orgID, auditID, normalizeAuditPagination(pagination))
}

func (s *AuditService) FindingStats(ctx context.Context, orgID, auditID string) (*models.AuditFindingStats, error) {
	if _, err := s.GetByID(ctx, orgID, auditID); err != nil {
		return nil, err
	}
	return s.repository.FindingStats(ctx, orgID, auditID)
}

func normalizeAuditCreate(input *models.AuditCreateInput) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.AuditType = models.AuditType(strings.ToLower(strings.TrimSpace(string(input.AuditType))))
	input.LeadAuditorID = strings.TrimSpace(input.LeadAuditorID)
	input.Scope = strings.TrimSpace(input.Scope)
	input.ScheduledStartDate = strings.TrimSpace(input.ScheduledStartDate)
	input.ScheduledEndDate = strings.TrimSpace(input.ScheduledEndDate)
	normalizeOptionalID(&input.FrameworkID)
	if len(input.Metadata) == 0 {
		input.Metadata = json.RawMessage(`{}`)
	}
}

func validateAuditCreate(input models.AuditCreateInput) (time.Time, time.Time, error) {
	if input.Title == "" || len(input.Title) > 200 || input.Description == "" || len(input.Description) > 10000 || input.Scope == "" || len(input.Scope) > 20000 {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: title, description, and scope are required and must fit their limits", ErrAuditInvalid)
	}
	if !allowed(string(input.AuditType), auditTypes()...) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: unsupported audit_type", ErrAuditInvalid)
	}
	if !validUUID(input.LeadAuditorID) || (input.FrameworkID != nil && !validUUID(*input.FrameworkID)) {
		return time.Time{}, time.Time{}, ErrAuditInvalidID
	}
	start, err := parseAuditDate(input.ScheduledStartDate)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: invalid scheduled_start_date", ErrAuditInvalid)
	}
	end, err := parseAuditDate(input.ScheduledEndDate)
	if err != nil || end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: scheduled_end_date must be on or after the start date", ErrAuditInvalid)
	}
	if err := validateJSONObject(input.Metadata); err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: metadata must be a JSON object", ErrAuditInvalid)
	}
	return start, end, nil
}

func validateAudit(audit *models.Audit) error {
	input := models.AuditCreateInput{
		Title: audit.Title, Description: audit.Description, AuditType: audit.Type,
		LeadAuditorID: audit.LeadAuditorID, Scope: audit.Scope,
		FrameworkID: audit.FrameworkID, Metadata: audit.Metadata,
	}
	if audit.ScheduledStartDate == nil || audit.ScheduledEndDate == nil {
		return fmt.Errorf("%w: scheduled dates are required", ErrAuditInvalid)
	}
	input.ScheduledStartDate = audit.ScheduledStartDate.Format("2006-01-02")
	input.ScheduledEndDate = audit.ScheduledEndDate.Format("2006-01-02")
	_, _, err := validateAuditCreate(input)
	if err != nil {
		return err
	}
	if !allowed(string(audit.Status), auditStatuses()...) {
		return fmt.Errorf("%w: unsupported status", ErrAuditInvalid)
	}
	if audit.ActualEndDate != nil && (audit.ActualStartDate == nil || audit.ActualEndDate.Before(*audit.ActualStartDate)) {
		return fmt.Errorf("%w: actual end date cannot precede start", ErrAuditInvalid)
	}
	return nil
}

func applyAuditPatch(audit *models.Audit, patch models.AuditPatch) error {
	if patch.Title != nil {
		audit.Title = strings.TrimSpace(*patch.Title)
	}
	if patch.Description != nil {
		audit.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.AuditType != nil {
		audit.Type = models.AuditType(strings.ToLower(strings.TrimSpace(string(*patch.AuditType))))
	}
	if patch.LeadAuditorID != nil {
		audit.LeadAuditorID = strings.TrimSpace(*patch.LeadAuditorID)
	}
	if patch.Scope != nil {
		audit.Scope = strings.TrimSpace(*patch.Scope)
	}
	if patch.ScheduledStartDate != nil {
		value, err := parseAuditDate(*patch.ScheduledStartDate)
		if err != nil {
			return fmt.Errorf("%w: invalid scheduled_start_date", ErrAuditInvalid)
		}
		audit.ScheduledStartDate = &value
	}
	if patch.ScheduledEndDate != nil {
		value, err := parseAuditDate(*patch.ScheduledEndDate)
		if err != nil {
			return fmt.Errorf("%w: invalid scheduled_end_date", ErrAuditInvalid)
		}
		audit.ScheduledEndDate = &value
	}
	if patch.ClearFramework {
		audit.FrameworkID = nil
	} else if patch.FrameworkID != nil {
		value := strings.TrimSpace(*patch.FrameworkID)
		audit.FrameworkID = &value
	}
	if len(patch.Metadata) > 0 {
		audit.Metadata = patch.Metadata
	}
	return nil
}

func normalizeFindingInput(input *models.AuditFindingInput) {
	normalizeOptionalID(&input.ControlID)
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Severity = strings.ToLower(strings.TrimSpace(input.Severity))
	input.FindingType = strings.TrimSpace(input.FindingType)
	input.RootCause = strings.TrimSpace(input.RootCause)
	input.Recommendation = strings.TrimSpace(input.Recommendation)
	input.RemediationPlan = strings.TrimSpace(input.RemediationPlan)
	input.ResponsibleUserID = strings.TrimSpace(input.ResponsibleUserID)
	input.DueDate = strings.TrimSpace(input.DueDate)
	if len(input.Metadata) == 0 {
		input.Metadata = json.RawMessage(`{}`)
	}
}

func validateFindingInput(input models.AuditFindingInput) (time.Time, error) {
	if input.Title == "" || len(input.Title) > 200 || input.Description == "" || len(input.Description) > 20000 || input.FindingType == "" || len(input.FindingType) > 100 || input.Recommendation == "" || len(input.Recommendation) > 20000 {
		return time.Time{}, fmt.Errorf("%w: title, description, finding_type, and recommendation are required and must fit their limits", ErrFindingInvalid)
	}
	if len(input.RootCause) > 20000 || len(input.RemediationPlan) > 20000 || !allowed(input.Severity, findingSeverities()...) {
		return time.Time{}, fmt.Errorf("%w: unsupported severity or text exceeds its limit", ErrFindingInvalid)
	}
	if !validUUID(input.ResponsibleUserID) || (input.ControlID != nil && !validUUID(*input.ControlID)) {
		return time.Time{}, ErrAuditInvalidID
	}
	dueDate, err := parseAuditDate(input.DueDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: invalid due_date", ErrFindingInvalid)
	}
	if err := validateJSONObject(input.Metadata); err != nil {
		return time.Time{}, fmt.Errorf("%w: metadata must be a JSON object", ErrFindingInvalid)
	}
	return dueDate, nil
}

func validateFinding(finding *models.AuditFinding) error {
	if finding.DueDate == nil {
		return fmt.Errorf("%w: due_date is required", ErrFindingInvalid)
	}
	input := models.AuditFindingInput{
		ControlID: finding.ControlID, Title: finding.Title, Description: finding.Description,
		Severity: finding.Severity, FindingType: finding.FindingType,
		RootCause: finding.RootCause, Recommendation: finding.Recommendation,
		RemediationPlan: finding.RemediationPlan, ResponsibleUserID: finding.ResponsibleUserID,
		DueDate: finding.DueDate.Format("2006-01-02"), Metadata: finding.Metadata,
	}
	if _, err := validateFindingInput(input); err != nil {
		return err
	}
	if !allowed(string(finding.Status), findingStatuses()...) {
		return fmt.Errorf("%w: unsupported status", ErrFindingInvalid)
	}
	if finding.Status == models.FindingStatusAccepted && (finding.AcceptedRiskReason == nil || strings.TrimSpace(*finding.AcceptedRiskReason) == "") {
		return fmt.Errorf("%w: accepted findings require accepted_risk_reason", ErrFindingInvalid)
	}
	return nil
}

func applyFindingPatch(finding *models.AuditFinding, patch models.AuditFindingPatch) error {
	if patch.ClearControl {
		finding.ControlID = nil
	} else if patch.ControlID != nil {
		value := strings.TrimSpace(*patch.ControlID)
		finding.ControlID = &value
	}
	if patch.Title != nil {
		finding.Title = strings.TrimSpace(*patch.Title)
	}
	if patch.Description != nil {
		finding.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.Severity != nil {
		finding.Severity = strings.ToLower(strings.TrimSpace(*patch.Severity))
	}
	if patch.Status != nil {
		finding.Status = models.FindingStatus(strings.ToLower(strings.TrimSpace(string(*patch.Status))))
	}
	if patch.FindingType != nil {
		finding.FindingType = strings.TrimSpace(*patch.FindingType)
	}
	if patch.RootCause != nil {
		finding.RootCause = strings.TrimSpace(*patch.RootCause)
	}
	if patch.Recommendation != nil {
		finding.Recommendation = strings.TrimSpace(*patch.Recommendation)
	}
	if patch.RemediationPlan != nil {
		finding.RemediationPlan = strings.TrimSpace(*patch.RemediationPlan)
	}
	if patch.ResponsibleUserID != nil {
		finding.ResponsibleUserID = strings.TrimSpace(*patch.ResponsibleUserID)
	}
	if patch.DueDate != nil {
		value, err := parseAuditDate(*patch.DueDate)
		if err != nil {
			return fmt.Errorf("%w: invalid due_date", ErrFindingInvalid)
		}
		finding.DueDate = &value
	}
	if patch.AcceptedRiskReason != nil {
		value := strings.TrimSpace(*patch.AcceptedRiskReason)
		finding.AcceptedRiskReason = &value
	}
	if len(patch.Metadata) > 0 {
		finding.Metadata = patch.Metadata
	}
	return nil
}

func validFindingTransition(from, to models.FindingStatus) bool {
	if !allowed(string(from), findingStatuses()...) || !allowed(string(to), findingStatuses()...) {
		return false
	}
	if from == to {
		return true
	}
	transitions := map[models.FindingStatus][]models.FindingStatus{
		models.FindingStatusOpen:       {models.FindingStatusInProgress, models.FindingStatusResolved, models.FindingStatusAccepted},
		models.FindingStatusInProgress: {models.FindingStatusOpen, models.FindingStatusResolved, models.FindingStatusAccepted},
		models.FindingStatusResolved:   {models.FindingStatusInProgress, models.FindingStatusClosed},
		models.FindingStatusAccepted:   {models.FindingStatusInProgress, models.FindingStatusClosed},
	}
	for _, candidate := range transitions[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

func normalizeAuditPagination(p models.PaginationRequest) models.PaginationRequest {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 || p.PageSize > 100 {
		p.PageSize = 20
	}
	return p
}

func parseAuditDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return dateOnly(parsed.UTC()), nil
		}
	}
	return time.Time{}, errors.New("date must be YYYY-MM-DD or RFC3339")
}

func dateOnly(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func validateJSONObject(value json.RawMessage) error {
	if !json.Valid(value) {
		return errors.New("invalid JSON")
	}
	var object map[string]any
	if err := json.Unmarshal(value, &object); err != nil || object == nil {
		return errors.New("expected JSON object")
	}
	return nil
}

func auditTypes() []string { return []string{"internal", "external", "certification"} }
func auditStatuses() []string {
	return []string{"planned", "in_progress", "completed", "closed", "cancelled"}
}
func findingSeverities() []string {
	return []string{"critical", "high", "medium", "low", "informational"}
}
func findingStatuses() []string {
	return []string{"open", "in_progress", "resolved", "closed", "accepted"}
}

func auditPatchEmpty(p models.AuditPatch) bool {
	return p.Title == nil && p.Description == nil && p.AuditType == nil && p.LeadAuditorID == nil && p.Scope == nil && p.ScheduledStartDate == nil && p.ScheduledEndDate == nil && p.FrameworkID == nil && !p.ClearFramework && len(p.Metadata) == 0
}

func findingPatchEmpty(p models.AuditFindingPatch) bool {
	return p.ControlID == nil && !p.ClearControl && p.Title == nil && p.Description == nil && p.Severity == nil && p.Status == nil && p.FindingType == nil && p.RootCause == nil && p.Recommendation == nil && p.RemediationPlan == nil && p.ResponsibleUserID == nil && p.DueDate == nil && p.AcceptedRiskReason == nil && len(p.Metadata) == 0
}

func mapAuditWriteError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAuditInvalidReference
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		switch pgError.Code {
		case "23505":
			return fmt.Errorf("%w: duplicate reference", ErrAuditConflict)
		case "23503":
			return ErrAuditInvalidReference
		case "23514", "22001", "22P02", "22007":
			return fmt.Errorf("%w: database constraint rejected the request", ErrAuditInvalid)
		}
	}
	return err
}
