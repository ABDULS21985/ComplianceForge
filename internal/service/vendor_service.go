package service

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrVendorNotFound = errors.New("vendor not found")
)

// VendorRiskAssessment holds the result of a vendor risk evaluation.
type VendorRiskAssessment struct {
	VendorID               string          `json:"vendor_id"`
	VendorName             string          `json:"vendor_name"`
	RiskLevel              models.RiskLevel `json:"risk_level"`
	DataProcessingAgreement bool           `json:"data_processing_agreement"`
	HasSubProcessors       bool            `json:"has_sub_processors"`
	ContractActive         bool            `json:"contract_active"`
	DaysSinceLastAssessment int            `json:"days_since_last_assessment"`
	Recommendations        []string        `json:"recommendations"`
}

// VendorRepository defines the data access interface for vendors.
type VendorRepository interface {
	Create(ctx context.Context, vendor *models.Vendor) error
	GetByID(ctx context.Context, id string) (*models.Vendor, error)
	Update(ctx context.Context, vendor *models.Vendor) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, orgID string, page, pageSize int) ([]models.Vendor, int, error)
	ListDueForAssessment(ctx context.Context, orgID string, before time.Time) ([]models.Vendor, error)
}

// VendorService handles business logic for third-party vendor management.
type VendorService struct {
	vendorRepo VendorRepository
	logger     zerolog.Logger
}

// NewVendorService constructs a new VendorService.
func NewVendorService(vendorRepo VendorRepository, logger zerolog.Logger) *VendorService {
	return &VendorService{
		vendorRepo: vendorRepo,
		logger:     logger.With().Str("service", "vendor").Logger(),
	}
}

// Create persists a new vendor record.
func (s *VendorService) Create(ctx context.Context, vendor *models.Vendor) error {
	if vendor.Status == "" {
		vendor.Status = models.VendorStatusActive
	}

	if err := s.vendorRepo.Create(ctx, vendor); err != nil {
		s.logger.Error().Err(err).Str("name", vendor.Name).Msg("failed to create vendor")
		return err
	}

	s.logger.Info().Str("vendor_id", vendor.ID).Str("name", vendor.Name).Msg("vendor created")
	return nil
}

// GetByID retrieves a vendor by its unique identifier.
func (s *VendorService) GetByID(ctx context.Context, id string) (*models.Vendor, error) {
	vendor, err := s.vendorRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrVendorNotFound
	}
	return vendor, nil
}

// Update modifies an existing vendor record.
func (s *VendorService) Update(ctx context.Context, vendor *models.Vendor) error {
	if _, err := s.vendorRepo.GetByID(ctx, vendor.ID); err != nil {
		return ErrVendorNotFound
	}

	if err := s.vendorRepo.Update(ctx, vendor); err != nil {
		s.logger.Error().Err(err).Str("vendor_id", vendor.ID).Msg("failed to update vendor")
		return err
	}

	s.logger.Info().Str("vendor_id", vendor.ID).Msg("vendor updated")
	return nil
}

// Delete soft-deletes a vendor by ID.
func (s *VendorService) Delete(ctx context.Context, id string) error {
	if _, err := s.vendorRepo.GetByID(ctx, id); err != nil {
		return ErrVendorNotFound
	}

	if err := s.vendorRepo.Delete(ctx, id); err != nil {
		s.logger.Error().Err(err).Str("vendor_id", id).Msg("failed to delete vendor")
		return err
	}

	s.logger.Info().Str("vendor_id", id).Msg("vendor deleted")
	return nil
}

// List returns a paginated list of vendors for an organization.
func (s *VendorService) List(ctx context.Context, orgID string, page, pageSize int) ([]models.Vendor, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	vendors, total, err := s.vendorRepo.List(ctx, orgID, page, pageSize)
	if err != nil {
		s.logger.Error().Err(err).Str("org_id", orgID).Msg("failed to list vendors")
		return nil, 0, err
	}
	return vendors, total, nil
}

// GetVendorsDueForAssessment returns vendors whose next assessment date is
// within the next 30 days.
func (s *VendorService) GetVendorsDueForAssessment(ctx context.Context, orgID string) ([]models.Vendor, error) {
	cutoff := time.Now().AddDate(0, 1, 0)
	vendors, err := s.vendorRepo.ListDueForAssessment(ctx, orgID, cutoff)
	if err != nil {
		s.logger.Error().Err(err).Str("org_id", orgID).Msg("failed to get vendors due for assessment")
		return nil, err
	}

	s.logger.Info().Str("org_id", orgID).Int("count", len(vendors)).Msg("vendors due for assessment retrieved")
	return vendors, nil
}

// AssessVendorRisk evaluates a vendor's risk based on their profile and returns
// an assessment with recommendations.
func (s *VendorService) AssessVendorRisk(ctx context.Context, vendorID string) (*VendorRiskAssessment, error) {
	vendor, err := s.vendorRepo.GetByID(ctx, vendorID)
	if err != nil {
		return nil, ErrVendorNotFound
	}

	assessment := &VendorRiskAssessment{
		VendorID:                vendor.ID,
		VendorName:              vendor.Name,
		RiskLevel:               vendor.RiskLevel,
		DataProcessingAgreement: vendor.DataProcessingAgreement,
		HasSubProcessors:        len(vendor.SubProcessors) > 0,
	}

	// Check contract status.
	now := time.Now()
	if vendor.ContractStartDate != nil && vendor.ContractEndDate != nil {
		assessment.ContractActive = now.After(*vendor.ContractStartDate) && now.Before(*vendor.ContractEndDate)
	}

	// Calculate days since last assessment.
	if vendor.LastAssessmentDate != nil {
		assessment.DaysSinceLastAssessment = int(now.Sub(*vendor.LastAssessmentDate).Hours() / 24)
	} else {
		assessment.DaysSinceLastAssessment = -1 // Never assessed.
	}

	// Generate recommendations.
	var recommendations []string

	if !vendor.DataProcessingAgreement {
		recommendations = append(recommendations, "Missing Data Processing Agreement - required for GDPR compliance")
	}

	if len(vendor.SubProcessors) > 0 {
		recommendations = append(recommendations, "Vendor uses sub-processors - ensure Article 28 GDPR compliance for sub-processing chain")
	}

	if assessment.DaysSinceLastAssessment > 365 || assessment.DaysSinceLastAssessment == -1 {
		recommendations = append(recommendations, "Vendor has not been assessed in over 12 months - schedule reassessment")
	}

	if !assessment.ContractActive {
		recommendations = append(recommendations, "Vendor contract is expired or not yet active - review contract status")
	}

	if vendor.RiskLevel == models.RiskLevelCritical || vendor.RiskLevel == models.RiskLevelHigh {
		recommendations = append(recommendations, "High/Critical risk vendor - consider quarterly review cadence")
	}

	assessment.Recommendations = recommendations

	// Update the vendor's last assessment date.
	vendor.LastAssessmentDate = &now
	nextAssessment := now.AddDate(0, 6, 0) // Default 6-month reassessment cycle.
	if vendor.RiskLevel == models.RiskLevelCritical || vendor.RiskLevel == models.RiskLevelHigh {
		nextAssessment = now.AddDate(0, 3, 0) // Quarterly for high-risk vendors.
	}
	vendor.NextAssessmentDate = &nextAssessment

	if err := s.vendorRepo.Update(ctx, vendor); err != nil {
		s.logger.Error().Err(err).Str("vendor_id", vendorID).Msg("failed to update vendor after assessment")
		return nil, err
	}

	s.logger.Info().
		Str("vendor_id", vendorID).
		Str("risk_level", string(vendor.RiskLevel)).
		Int("recommendations", len(recommendations)).
		Msg("vendor risk assessment completed")

	return assessment, nil
}
