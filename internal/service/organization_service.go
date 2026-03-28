package service

import (
	"context"
	"errors"

	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrOrganizationExists   = errors.New("organization with this domain already exists")
)

// OrganizationRepository defines the data access interface for organizations.
type OrganizationRepository interface {
	Create(ctx context.Context, org *models.Organization) error
	GetByID(ctx context.Context, id string) (*models.Organization, error)
	GetByDomain(ctx context.Context, domain string) (*models.Organization, error)
	Update(ctx context.Context, org *models.Organization) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, page, pageSize int) ([]models.Organization, int, error)
}

// OrganizationService handles business logic for organization management.
type OrganizationService struct {
	orgRepo OrganizationRepository
	logger  zerolog.Logger
}

// NewOrganizationService constructs a new OrganizationService.
func NewOrganizationService(orgRepo OrganizationRepository, logger zerolog.Logger) *OrganizationService {
	return &OrganizationService{
		orgRepo: orgRepo,
		logger:  logger.With().Str("service", "organization").Logger(),
	}
}

// Create validates and persists a new organization.
func (s *OrganizationService) Create(ctx context.Context, org *models.Organization) error {
	existing, err := s.orgRepo.GetByDomain(ctx, org.Domain)
	if err == nil && existing != nil {
		return ErrOrganizationExists
	}

	if org.SubscriptionTier == "" {
		org.SubscriptionTier = "free"
	}
	org.IsActive = true

	if err := s.orgRepo.Create(ctx, org); err != nil {
		s.logger.Error().Err(err).Str("domain", org.Domain).Msg("failed to create organization")
		return err
	}

	s.logger.Info().Str("org_id", org.ID).Str("domain", org.Domain).Msg("organization created")
	return nil
}

// GetByID retrieves an organization by its unique identifier.
func (s *OrganizationService) GetByID(ctx context.Context, id string) (*models.Organization, error) {
	org, err := s.orgRepo.GetByID(ctx, id)
	if err != nil {
		s.logger.Warn().Err(err).Str("org_id", id).Msg("organization not found")
		return nil, ErrOrganizationNotFound
	}
	return org, nil
}

// Update modifies an existing organization's details.
func (s *OrganizationService) Update(ctx context.Context, org *models.Organization) error {
	existing, err := s.orgRepo.GetByID(ctx, org.ID)
	if err != nil {
		return ErrOrganizationNotFound
	}

	// Prevent domain collision with another org.
	if org.Domain != existing.Domain {
		dup, err := s.orgRepo.GetByDomain(ctx, org.Domain)
		if err == nil && dup != nil && dup.ID != org.ID {
			return ErrOrganizationExists
		}
	}

	if err := s.orgRepo.Update(ctx, org); err != nil {
		s.logger.Error().Err(err).Str("org_id", org.ID).Msg("failed to update organization")
		return err
	}

	s.logger.Info().Str("org_id", org.ID).Msg("organization updated")
	return nil
}

// Delete soft-deletes an organization by ID.
func (s *OrganizationService) Delete(ctx context.Context, id string) error {
	if _, err := s.orgRepo.GetByID(ctx, id); err != nil {
		return ErrOrganizationNotFound
	}

	if err := s.orgRepo.Delete(ctx, id); err != nil {
		s.logger.Error().Err(err).Str("org_id", id).Msg("failed to delete organization")
		return err
	}

	s.logger.Info().Str("org_id", id).Msg("organization deleted")
	return nil
}

// List returns a paginated list of organizations.
func (s *OrganizationService) List(ctx context.Context, page, pageSize int) ([]models.Organization, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	orgs, total, err := s.orgRepo.List(ctx, page, pageSize)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to list organizations")
		return nil, 0, err
	}
	return orgs, total, nil
}
