package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrFrameworkNotFound     = errors.New("framework not found")
	ErrUnsupportedFramework  = errors.New("unsupported framework type for import")
)

// FrameworkRepository defines the data access interface for compliance frameworks.
type FrameworkRepository interface {
	Create(ctx context.Context, framework *models.ComplianceFramework) error
	GetByID(ctx context.Context, id string) (*models.ComplianceFramework, error)
	GetWithControls(ctx context.Context, id string) (*models.ComplianceFramework, error)
	Update(ctx context.Context, framework *models.ComplianceFramework) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, orgID string, page, pageSize int) ([]models.ComplianceFramework, int, error)
}

// ControlRepository defines the data access interface for controls.
type ControlRepository interface {
	Create(ctx context.Context, control *models.Control) error
	GetByID(ctx context.Context, id string) (*models.Control, error)
	Update(ctx context.Context, control *models.Control) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, orgID string, page, pageSize int) ([]models.Control, int, error)
	ListByFrameworkID(ctx context.Context, frameworkID string) ([]models.Control, error)
	CountByStatus(ctx context.Context, frameworkID string) (map[models.ComplianceStatus]int, error)
}

// FrameworkService handles business logic for compliance framework management.
type FrameworkService struct {
	frameworkRepo FrameworkRepository
	controlRepo   ControlRepository
	logger        zerolog.Logger
}

// NewFrameworkService constructs a new FrameworkService.
func NewFrameworkService(frameworkRepo FrameworkRepository, controlRepo ControlRepository, logger zerolog.Logger) *FrameworkService {
	return &FrameworkService{
		frameworkRepo: frameworkRepo,
		controlRepo:   controlRepo,
		logger:        logger.With().Str("service", "framework").Logger(),
	}
}

// Create persists a new compliance framework.
func (s *FrameworkService) Create(ctx context.Context, framework *models.ComplianceFramework) error {
	framework.IsActive = true

	if err := s.frameworkRepo.Create(ctx, framework); err != nil {
		s.logger.Error().Err(err).Str("name", framework.Name).Msg("failed to create framework")
		return err
	}

	s.logger.Info().Str("framework_id", framework.ID).Str("name", framework.Name).Msg("framework created")
	return nil
}

// GetByID retrieves a framework by its unique identifier.
func (s *FrameworkService) GetByID(ctx context.Context, id string) (*models.ComplianceFramework, error) {
	framework, err := s.frameworkRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrFrameworkNotFound
	}
	return framework, nil
}

// GetWithControls retrieves a framework along with all its associated controls.
func (s *FrameworkService) GetWithControls(ctx context.Context, id string) (*models.ComplianceFramework, error) {
	framework, err := s.frameworkRepo.GetWithControls(ctx, id)
	if err != nil {
		return nil, ErrFrameworkNotFound
	}
	return framework, nil
}

// Update modifies an existing compliance framework.
func (s *FrameworkService) Update(ctx context.Context, framework *models.ComplianceFramework) error {
	if _, err := s.frameworkRepo.GetByID(ctx, framework.ID); err != nil {
		return ErrFrameworkNotFound
	}

	if err := s.frameworkRepo.Update(ctx, framework); err != nil {
		s.logger.Error().Err(err).Str("framework_id", framework.ID).Msg("failed to update framework")
		return err
	}

	s.logger.Info().Str("framework_id", framework.ID).Msg("framework updated")
	return nil
}

// Delete soft-deletes a framework by ID.
func (s *FrameworkService) Delete(ctx context.Context, id string) error {
	if _, err := s.frameworkRepo.GetByID(ctx, id); err != nil {
		return ErrFrameworkNotFound
	}

	if err := s.frameworkRepo.Delete(ctx, id); err != nil {
		s.logger.Error().Err(err).Str("framework_id", id).Msg("failed to delete framework")
		return err
	}

	s.logger.Info().Str("framework_id", id).Msg("framework deleted")
	return nil
}

// List returns a paginated list of frameworks for an organization.
func (s *FrameworkService) List(ctx context.Context, orgID string, page, pageSize int) ([]models.ComplianceFramework, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	frameworks, total, err := s.frameworkRepo.List(ctx, orgID, page, pageSize)
	if err != nil {
		s.logger.Error().Err(err).Str("org_id", orgID).Msg("failed to list frameworks")
		return nil, 0, err
	}
	return frameworks, total, nil
}

// ImportFramework imports a standard framework template (e.g., SOC2, ISO27001, NIST)
// and creates the framework with its controls for the given organization.
func (s *FrameworkService) ImportFramework(ctx context.Context, orgID, frameworkType string) error {
	switch frameworkType {
	case "SOC2", "ISO27001", "NIST-CSF", "GDPR", "HIPAA", "PCI-DSS":
		// TODO: Load framework definitions from embedded templates or external catalog.
		// Each template should include framework metadata and a list of controls.
		s.logger.Info().
			Str("org_id", orgID).
			Str("framework_type", frameworkType).
			Msg("importing standard framework")

		framework := &models.ComplianceFramework{
			TenantModel: models.TenantModel{
				OrganizationID: orgID,
			},
			Name:        frameworkType,
			Version:     "1.0",
			Description: fmt.Sprintf("Imported %s framework template", frameworkType),
			Authority:   frameworkType,
			Category:    "Regulatory",
			IsActive:    true,
		}

		if err := s.frameworkRepo.Create(ctx, framework); err != nil {
			s.logger.Error().Err(err).Str("framework_type", frameworkType).Msg("failed to import framework")
			return err
		}

		// TODO: Create controls from template definitions.
		// Example: iterate over template controls and call s.controlRepo.Create(ctx, &control)

		s.logger.Info().
			Str("framework_id", framework.ID).
			Str("framework_type", frameworkType).
			Msg("framework imported successfully")
		return nil

	default:
		return ErrUnsupportedFramework
	}
}
