package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrFrameworkNotFound   = errors.New("framework not found")
	ErrControlNotFound     = errors.New("control not found or framework not adopted")
	ErrInvalidComplianceID = errors.New("invalid compliance identifier")
	ErrInvalidControlPatch = errors.New("invalid control implementation update")
	ErrInvalidEvidence     = errors.New("invalid evidence metadata")
)

type FrameworkCatalogRepository interface {
	List(context.Context, string, models.PaginationRequest) ([]models.ComplianceFramework, int, error)
	GetByID(context.Context, string, string) (*models.ComplianceFramework, error)
	Adopt(context.Context, string, string, string) (*models.OrganizationFramework, error)
}

type ControlImplementationRepository interface {
	ListByFramework(context.Context, string, string, models.PaginationRequest) ([]models.Control, int, error)
	ListAdopted(context.Context, string, string, models.PaginationRequest) ([]models.Control, int, error)
	GetAdoptedByID(context.Context, string, string) (*models.Control, error)
	UpdateImplementation(context.Context, string, string, models.ControlImplementationPatch) (*models.ControlImplementation, error)
	AttachEvidence(context.Context, string, string, string, models.AttachControlEvidenceInput) (*models.ControlEvidence, error)
	ListEvidence(context.Context, string, string, models.PaginationRequest) ([]models.ControlEvidence, int, error)
}

// FrameworkService coordinates the framework catalog and tenant-owned
// implementations without allowing a tenant to mutate catalog definitions.
type FrameworkService struct {
	frameworks FrameworkCatalogRepository
	controls   ControlImplementationRepository
	logger     zerolog.Logger
}

func NewFrameworkService(frameworks FrameworkCatalogRepository, controls ControlImplementationRepository, logger zerolog.Logger) *FrameworkService {
	return &FrameworkService{frameworks: frameworks, controls: controls, logger: logger.With().Str("service", "compliance").Logger()}
}

func (s *FrameworkService) ListFrameworks(ctx context.Context, orgID string, p models.PaginationRequest) ([]models.ComplianceFramework, int, error) {
	if !validUUID(orgID) {
		return nil, 0, ErrInvalidComplianceID
	}
	return s.frameworks.List(ctx, orgID, normalizeCompliancePagination(p))
}

func (s *FrameworkService) GetFramework(ctx context.Context, orgID, frameworkID string) (*models.ComplianceFramework, error) {
	if !validUUID(orgID) || !validUUID(frameworkID) {
		return nil, ErrInvalidComplianceID
	}
	framework, err := s.frameworks.GetByID(ctx, orgID, frameworkID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFrameworkNotFound
	}
	return framework, err
}

func (s *FrameworkService) AdoptFramework(ctx context.Context, orgID, userID, frameworkID string) (*models.OrganizationFramework, error) {
	if !validUUID(orgID) || !validUUID(userID) || !validUUID(frameworkID) {
		return nil, ErrInvalidComplianceID
	}
	adoption, err := s.frameworks.Adopt(ctx, orgID, userID, frameworkID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFrameworkNotFound
	}
	if err == nil {
		s.logger.Info().Str("organization_id", orgID).Str("framework_id", frameworkID).Msg("framework adopted")
	}
	return adoption, err
}

func (s *FrameworkService) ListFrameworkControls(ctx context.Context, orgID, frameworkID string, p models.PaginationRequest) ([]models.Control, int, error) {
	if !validUUID(orgID) || !validUUID(frameworkID) {
		return nil, 0, ErrInvalidComplianceID
	}
	if _, err := s.GetFramework(ctx, orgID, frameworkID); err != nil {
		return nil, 0, err
	}
	return s.controls.ListByFramework(ctx, orgID, frameworkID, normalizeCompliancePagination(p))
}

func (s *FrameworkService) ListControls(ctx context.Context, orgID, frameworkID string, p models.PaginationRequest) ([]models.Control, int, error) {
	if !validUUID(orgID) || (frameworkID != "" && !validUUID(frameworkID)) {
		return nil, 0, ErrInvalidComplianceID
	}
	return s.controls.ListAdopted(ctx, orgID, frameworkID, normalizeCompliancePagination(p))
}

func (s *FrameworkService) GetControl(ctx context.Context, orgID, controlID string) (*models.Control, error) {
	if !validUUID(orgID) || !validUUID(controlID) {
		return nil, ErrInvalidComplianceID
	}
	control, err := s.controls.GetAdoptedByID(ctx, orgID, controlID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrControlNotFound
	}
	return control, err
}

func (s *FrameworkService) UpdateControlImplementation(ctx context.Context, orgID, controlID string, patch models.ControlImplementationPatch) (*models.ControlImplementation, error) {
	if !validUUID(orgID) || !validUUID(controlID) {
		return nil, ErrInvalidComplianceID
	}
	if err := validateControlPatch(patch); err != nil {
		return nil, err
	}
	implementation, err := s.controls.UpdateImplementation(ctx, orgID, controlID, patch)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrControlNotFound
	}
	return implementation, err
}

func (s *FrameworkService) AttachControlEvidence(ctx context.Context, orgID, userID, controlID string, input models.AttachControlEvidenceInput) (*models.ControlEvidence, error) {
	if !validUUID(orgID) || !validUUID(userID) || !validUUID(controlID) {
		return nil, ErrInvalidComplianceID
	}
	if err := validateEvidence(input); err != nil {
		return nil, err
	}
	evidence, err := s.controls.AttachEvidence(ctx, orgID, userID, controlID, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrControlNotFound
	}
	return evidence, err
}

func (s *FrameworkService) ListControlEvidence(ctx context.Context, orgID, controlID string, p models.PaginationRequest) ([]models.ControlEvidence, int, error) {
	if !validUUID(orgID) || !validUUID(controlID) {
		return nil, 0, ErrInvalidComplianceID
	}
	if _, err := s.GetControl(ctx, orgID, controlID); err != nil {
		return nil, 0, err
	}
	return s.controls.ListEvidence(ctx, orgID, controlID, normalizeCompliancePagination(p))
}

func normalizeCompliancePagination(p models.PaginationRequest) models.PaginationRequest {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 || p.PageSize > 100 {
		p.PageSize = 20
	}
	return p
}

func validUUID(value string) bool { _, err := uuid.Parse(value); return err == nil }

func validateControlPatch(p models.ControlImplementationPatch) error {
	if p.Status == nil && p.ImplementationStatus == nil && p.MaturityLevel == nil &&
		p.OwnerUserID == nil && p.ReviewerUserID == nil && p.ImplementationDescription == nil &&
		p.ImplementationNotes == nil && p.GapDescription == nil && p.RemediationPlan == nil &&
		p.RemediationDueDate == nil && p.AutomationLevel == nil && p.Tags == nil {
		return fmt.Errorf("%w: no fields supplied", ErrInvalidControlPatch)
	}
	if p.Status != nil && !allowed(string(*p.Status), "not_applicable", "not_implemented", "planned", "partial", "implemented", "effective") {
		return fmt.Errorf("%w: unsupported status", ErrInvalidControlPatch)
	}
	if p.ImplementationStatus != nil && !allowed(string(*p.ImplementationStatus), "not_started", "in_progress", "completed", "failed") {
		return fmt.Errorf("%w: unsupported implementation_status", ErrInvalidControlPatch)
	}
	if p.MaturityLevel != nil && (*p.MaturityLevel < 0 || *p.MaturityLevel > 5) {
		return fmt.Errorf("%w: maturity_level must be 0-5", ErrInvalidControlPatch)
	}
	for _, candidate := range []*string{p.OwnerUserID, p.ReviewerUserID} {
		if candidate != nil && *candidate != "" && !validUUID(*candidate) {
			return fmt.Errorf("%w: invalid user id", ErrInvalidControlPatch)
		}
	}
	if p.AutomationLevel != nil && !allowed(*p.AutomationLevel, "fully_automated", "semi_automated", "manual") {
		return fmt.Errorf("%w: unsupported automation_level", ErrInvalidControlPatch)
	}
	return nil
}

func validateEvidence(input models.AttachControlEvidenceInput) error {
	if strings.TrimSpace(input.Title) == "" {
		return fmt.Errorf("%w: title is required", ErrInvalidEvidence)
	}
	if !allowed(input.EvidenceType, "document", "screenshot", "log", "configuration", "report", "certificate", "interview_notes", "test_result", "policy", "procedure", "training_record") {
		return fmt.Errorf("%w: unsupported evidence_type", ErrInvalidEvidence)
	}
	if input.CollectionMethod != "" && !allowed(input.CollectionMethod, "manual_upload", "automated", "api_pull", "scan_result", "integration") {
		return fmt.Errorf("%w: unsupported collection_method", ErrInvalidEvidence)
	}
	if input.FileSizeBytes != nil && *input.FileSizeBytes < 0 {
		return fmt.Errorf("%w: file_size_bytes cannot be negative", ErrInvalidEvidence)
	}
	if input.FileHash != nil && len(*input.FileHash) > 128 {
		return fmt.Errorf("%w: file_hash is too long", ErrInvalidEvidence)
	}
	if input.ValidFrom != nil && input.ValidUntil != nil && input.ValidUntil.Before(*input.ValidFrom) {
		return fmt.Errorf("%w: valid_until precedes valid_from", ErrInvalidEvidence)
	}
	return nil
}

func allowed(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
