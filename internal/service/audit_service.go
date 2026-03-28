package service

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrAuditNotFound          = errors.New("audit not found")
	ErrAuditInvalidTransition = errors.New("invalid audit status transition")
	ErrFindingNotFound        = errors.New("audit finding not found")
)

// AuditRepository defines the data access interface for audits and findings.
type AuditRepository interface {
	Create(ctx context.Context, audit *models.Audit) error
	GetByID(ctx context.Context, id string) (*models.Audit, error)
	Update(ctx context.Context, audit *models.Audit) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, orgID string, page, pageSize int) ([]models.Audit, int, error)
	ListUpcoming(ctx context.Context, orgID string, before time.Time) ([]models.Audit, error)

	CreateFinding(ctx context.Context, finding *models.AuditFinding) error
	GetFindingByID(ctx context.Context, id string) (*models.AuditFinding, error)
	UpdateFinding(ctx context.Context, finding *models.AuditFinding) error
	ListFindings(ctx context.Context, auditID string, page, pageSize int) ([]models.AuditFinding, int, error)
}

// AuditService handles business logic for audit management and findings.
type AuditService struct {
	auditRepo AuditRepository
	logger    zerolog.Logger
}

// NewAuditService constructs a new AuditService.
func NewAuditService(auditRepo AuditRepository, logger zerolog.Logger) *AuditService {
	return &AuditService{
		auditRepo: auditRepo,
		logger:    logger.With().Str("service", "audit").Logger(),
	}
}

// Create persists a new audit engagement in Planned status.
func (s *AuditService) Create(ctx context.Context, audit *models.Audit) error {
	audit.Status = models.AuditStatusPlanned

	if err := s.auditRepo.Create(ctx, audit); err != nil {
		s.logger.Error().Err(err).Str("title", audit.Title).Msg("failed to create audit")
		return err
	}

	s.logger.Info().Str("audit_id", audit.ID).Str("title", audit.Title).Msg("audit created")
	return nil
}

// GetByID retrieves an audit by its unique identifier.
func (s *AuditService) GetByID(ctx context.Context, id string) (*models.Audit, error) {
	audit, err := s.auditRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrAuditNotFound
	}
	return audit, nil
}

// Update modifies an existing audit.
func (s *AuditService) Update(ctx context.Context, audit *models.Audit) error {
	if _, err := s.auditRepo.GetByID(ctx, audit.ID); err != nil {
		return ErrAuditNotFound
	}

	if err := s.auditRepo.Update(ctx, audit); err != nil {
		s.logger.Error().Err(err).Str("audit_id", audit.ID).Msg("failed to update audit")
		return err
	}

	s.logger.Info().Str("audit_id", audit.ID).Msg("audit updated")
	return nil
}

// Delete soft-deletes an audit by ID.
func (s *AuditService) Delete(ctx context.Context, id string) error {
	if _, err := s.auditRepo.GetByID(ctx, id); err != nil {
		return ErrAuditNotFound
	}

	if err := s.auditRepo.Delete(ctx, id); err != nil {
		s.logger.Error().Err(err).Str("audit_id", id).Msg("failed to delete audit")
		return err
	}

	s.logger.Info().Str("audit_id", id).Msg("audit deleted")
	return nil
}

// List returns a paginated list of audits for an organization.
func (s *AuditService) List(ctx context.Context, orgID string, page, pageSize int) ([]models.Audit, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	audits, total, err := s.auditRepo.List(ctx, orgID, page, pageSize)
	if err != nil {
		s.logger.Error().Err(err).Str("org_id", orgID).Msg("failed to list audits")
		return nil, 0, err
	}
	return audits, total, nil
}

// StartAudit transitions an audit from Planned to InProgress and records the actual start date.
func (s *AuditService) StartAudit(ctx context.Context, id string) error {
	audit, err := s.auditRepo.GetByID(ctx, id)
	if err != nil {
		return ErrAuditNotFound
	}

	if audit.Status != models.AuditStatusPlanned {
		return ErrAuditInvalidTransition
	}

	now := time.Now()
	audit.Status = models.AuditStatusInProgress
	audit.ActualStartDate = &now

	if err := s.auditRepo.Update(ctx, audit); err != nil {
		s.logger.Error().Err(err).Str("audit_id", id).Msg("failed to start audit")
		return err
	}

	s.logger.Info().Str("audit_id", id).Msg("audit started")
	return nil
}

// CompleteAudit transitions an audit from InProgress to Completed and records the actual end date.
func (s *AuditService) CompleteAudit(ctx context.Context, id string) error {
	audit, err := s.auditRepo.GetByID(ctx, id)
	if err != nil {
		return ErrAuditNotFound
	}

	if audit.Status != models.AuditStatusInProgress {
		return ErrAuditInvalidTransition
	}

	now := time.Now()
	audit.Status = models.AuditStatusCompleted
	audit.ActualEndDate = &now

	if err := s.auditRepo.Update(ctx, audit); err != nil {
		s.logger.Error().Err(err).Str("audit_id", id).Msg("failed to complete audit")
		return err
	}

	s.logger.Info().Str("audit_id", id).Msg("audit completed")
	return nil
}

// CreateFinding adds a new finding to an audit.
func (s *AuditService) CreateFinding(ctx context.Context, finding *models.AuditFinding) error {
	// Verify the parent audit exists.
	if _, err := s.auditRepo.GetByID(ctx, finding.AuditID); err != nil {
		return ErrAuditNotFound
	}

	finding.Status = models.FindingStatusOpen

	if err := s.auditRepo.CreateFinding(ctx, finding); err != nil {
		s.logger.Error().Err(err).Str("audit_id", finding.AuditID).Msg("failed to create finding")
		return err
	}

	s.logger.Info().
		Str("finding_id", finding.ID).
		Str("audit_id", finding.AuditID).
		Str("title", finding.Title).
		Msg("audit finding created")
	return nil
}

// UpdateFinding modifies an existing audit finding.
func (s *AuditService) UpdateFinding(ctx context.Context, finding *models.AuditFinding) error {
	existing, err := s.auditRepo.GetFindingByID(ctx, finding.ID)
	if err != nil {
		return ErrFindingNotFound
	}

	// Auto-set resolved timestamp when status transitions to Resolved.
	if finding.Status == models.FindingStatusResolved && existing.Status != models.FindingStatusResolved {
		now := time.Now()
		finding.ResolvedAt = &now
	}

	if err := s.auditRepo.UpdateFinding(ctx, finding); err != nil {
		s.logger.Error().Err(err).Str("finding_id", finding.ID).Msg("failed to update finding")
		return err
	}

	s.logger.Info().Str("finding_id", finding.ID).Msg("audit finding updated")
	return nil
}

// ListFindings returns a paginated list of findings for a given audit.
func (s *AuditService) ListFindings(ctx context.Context, auditID string, page, pageSize int) ([]models.AuditFinding, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	findings, total, err := s.auditRepo.ListFindings(ctx, auditID, page, pageSize)
	if err != nil {
		s.logger.Error().Err(err).Str("audit_id", auditID).Msg("failed to list findings")
		return nil, 0, err
	}
	return findings, total, nil
}
