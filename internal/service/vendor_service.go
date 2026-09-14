package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

var (
	ErrVendorNotFound          = errors.New("vendor not found")
	ErrVendorInvalid           = errors.New("invalid vendor request")
	ErrVendorInvalidID         = errors.New("invalid vendor identifier")
	ErrVendorInvalidTransition = errors.New("invalid vendor status transition")
	ErrVendorConflict          = errors.New("vendor conflicts with current state")
	ErrVendorVersionConflict   = errors.New("vendor was modified by another request")
	ErrVendorUserNotFound      = errors.New("vendor owner is not an active tenant user")
)

type VendorManagementRepository interface {
	Create(context.Context, string, string, models.VendorCreateInput) (*models.Vendor, error)
	GetByID(context.Context, string, string) (*models.Vendor, error)
	Update(context.Context, string, string, string, models.VendorPatch) (*models.Vendor, error)
	Delete(context.Context, string, string, string, int64) error
	List(context.Context, string, models.VendorListFilter) ([]models.Vendor, int, error)
	Transition(context.Context, string, string, string, models.VendorTransitionInput) (*models.Vendor, error)
	RecordAssessment(context.Context, string, string, string, models.VendorAssessmentInput) (*models.Vendor, error)
	Statistics(context.Context, string) (*models.VendorStatistics, error)
	ListDueForAssessment(context.Context, string, time.Time, int) ([]models.Vendor, error)
	ListDueContracts(context.Context, string, time.Time, int) ([]models.VendorDueContract, error)
	ListExpiringCertifications(context.Context, string, time.Time, int) ([]models.VendorExpiringCertification, error)
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.VendorEvent, int, error)
	SaveContact(context.Context, string, string, string, string, models.VendorContactInput) (*models.VendorContact, *models.Vendor, error)
	DeleteContact(context.Context, string, string, string, string, int64) (*models.Vendor, error)
	SaveContract(context.Context, string, string, string, string, models.VendorContractInput) (*models.VendorContract, *models.Vendor, error)
	DeleteContract(context.Context, string, string, string, string, int64) (*models.Vendor, error)
	SaveCertification(context.Context, string, string, string, string, models.VendorCertificationInput) (*models.VendorCertification, *models.Vendor, error)
	DeleteCertification(context.Context, string, string, string, string, int64) (*models.Vendor, error)
	SaveSubprocessor(context.Context, string, string, string, string, models.VendorSubprocessorInput) (*models.VendorSubprocessor, *models.Vendor, error)
	DeleteSubprocessor(context.Context, string, string, string, string, int64) (*models.Vendor, error)
}

type VendorRepository = VendorManagementRepository

type VendorService struct {
	repository VendorManagementRepository
	logger     zerolog.Logger
	now        func() time.Time
}

func NewVendorService(repository VendorManagementRepository, logger zerolog.Logger) *VendorService {
	return &VendorService{
		repository: repository,
		logger:     logger.With().Str("service", "vendor").Logger(),
		now:        func() time.Time { return time.Now().UTC() },
	}
}

var _ VendorManagementRepository = repository.VendorRepository(nil)

func (s *VendorService) Create(ctx context.Context, orgID, actorID string, input models.VendorCreateInput) (*models.Vendor, error) {
	if err := validateVendorIdentity(orgID, actorID); err != nil {
		return nil, err
	}
	normalizeVendorCreate(&input)
	if input.Criticality == "" {
		input.Criticality = models.VendorCriticalityMedium
	}
	if input.VendorTier == "" {
		input.VendorTier = models.VendorTierThree
	}
	if input.RiskTier == "" {
		input.RiskTier = models.VendorRiskMedium
	}
	if input.AssessmentFrequency == "" {
		input.AssessmentFrequency = "annual"
	}
	if input.AssessmentCadenceDays == 0 {
		input.AssessmentCadenceDays = cadenceForFrequency(input.AssessmentFrequency)
	}
	if input.DataProcessing {
		input.DPARequired = true
		if input.DPAStatus == "" || input.DPAStatus == models.VendorDPANotRequired {
			input.DPAStatus = models.VendorDPAPending
		}
	} else if input.DPAStatus == "" {
		input.DPAStatus = models.VendorDPANotRequired
	}
	if input.NextAssessmentDate == nil {
		next := s.now().AddDate(0, 0, input.AssessmentCadenceDays).Truncate(24 * time.Hour)
		input.NextAssessmentDate = &next
	}
	if len(input.Metadata) == 0 {
		input.Metadata = json.RawMessage(`{}`)
	}
	if err := validateVendorCreate(input, s.now()); err != nil {
		return nil, err
	}
	item, err := s.repository.Create(ctx, orgID, actorID, input)
	if err != nil {
		return nil, s.translateError(err)
	}
	s.logger.Info().Str("vendor_id", item.ID).Str("organization_id", orgID).Str("vendor_ref", item.VendorRef).Msg("vendor created")
	return item, nil
}

func (s *VendorService) GetByID(ctx context.Context, orgID, id string) (*models.Vendor, error) {
	if err := validateVendorIDs(orgID, id); err != nil {
		return nil, err
	}
	item, err := s.repository.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, s.translateError(err)
	}
	decorateVendorAssessment(item, s.now())
	return item, nil
}

func (s *VendorService) Update(ctx context.Context, orgID, actorID, id string, patch models.VendorPatch) (*models.Vendor, error) {
	if err := validateVendorMutation(orgID, actorID, id, patch.Version); err != nil {
		return nil, err
	}
	current, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if current.Status == models.VendorStatusOffboarded || current.Status == models.VendorStatusRejected {
		return nil, fmt.Errorf("%w: terminal vendors must re-enter onboarding before profile changes", ErrVendorConflict)
	}
	normalizeVendorPatch(&patch)
	if patch.ClearOwner && patch.OwnerUserID != nil || patch.ClearRiskScore && patch.RiskScore != nil ||
		patch.ClearNextAssessment && patch.NextAssessmentDate != nil || patch.ClearNextReview && patch.NextReviewDate != nil ||
		patch.ClearDPA && (patch.DPAStatus != nil || patch.DPAReference != nil || patch.DPASignedDate != nil || patch.DPAExpiryDate != nil) {
		return nil, fmt.Errorf("%w: a field cannot be set and cleared together", ErrVendorInvalid)
	}
	if patch.OwnerUserID != nil && !isVendorUUID(*patch.OwnerUserID) {
		return nil, ErrVendorInvalidID
	}
	merged := *current
	applyVendorPatch(&merged, patch)
	if err := validateVendorRecord(&merged, s.now()); err != nil {
		return nil, err
	}
	item, err := s.repository.Update(ctx, orgID, actorID, id, patch)
	if err != nil {
		return nil, s.translateError(err)
	}
	return item, nil
}

func (s *VendorService) Delete(ctx context.Context, orgID, actorID, id string, version int64) error {
	if err := validateVendorMutation(orgID, actorID, id, version); err != nil {
		return err
	}
	item, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return err
	}
	if item.Status != models.VendorStatusOffboarded && item.Status != models.VendorStatusRejected {
		return fmt.Errorf("%w: only offboarded or rejected vendors may be deleted", ErrVendorConflict)
	}
	if item.LegalHold || item.RetentionUntil != nil && item.RetentionUntil.After(s.now()) {
		return fmt.Errorf("%w: legal hold or retention prevents deletion", ErrVendorConflict)
	}
	if err := s.repository.Delete(ctx, orgID, actorID, id, version); err != nil {
		return s.translateError(err)
	}
	return nil
}

func (s *VendorService) List(ctx context.Context, orgID string, filter models.VendorListFilter) ([]models.Vendor, int, error) {
	if !isVendorUUID(orgID) {
		return nil, 0, ErrVendorInvalidID
	}
	normalizeVendorFilter(&filter)
	if filter.Status != "" && !validVendorStatus(models.VendorStatus(filter.Status)) ||
		filter.Criticality != "" && !validVendorCriticality(models.VendorCriticality(filter.Criticality)) ||
		filter.VendorTier != "" && !validVendorTier(models.VendorTier(filter.VendorTier)) ||
		filter.RiskTier != "" && !validVendorRiskTier(models.VendorRiskTier(filter.RiskTier)) {
		return nil, 0, fmt.Errorf("%w: unsupported vendor filter", ErrVendorInvalid)
	}
	if filter.OwnerUserID != "" && !isVendorUUID(filter.OwnerUserID) {
		return nil, 0, ErrVendorInvalidID
	}
	if len(filter.Search) > 500 || len(filter.CountryCode) > 2 {
		return nil, 0, fmt.Errorf("%w: filter is too long", ErrVendorInvalid)
	}
	validSort := map[string]bool{"": true, "updated_at": true, "created_at": true, "name": true, "vendor_ref": true, "criticality": true, "risk_tier": true, "next_assessment_date": true}
	if !validSort[filter.SortBy] || filter.SortDirection != "asc" && filter.SortDirection != "desc" {
		return nil, 0, fmt.Errorf("%w: unsupported sort", ErrVendorInvalid)
	}
	items, total, err := s.repository.List(ctx, orgID, filter)
	if err != nil {
		return nil, 0, s.translateError(err)
	}
	for index := range items {
		decorateVendorAssessment(&items[index], s.now())
	}
	return items, total, nil
}

func (s *VendorService) Transition(ctx context.Context, orgID, actorID, id string, input models.VendorTransitionInput) (*models.Vendor, error) {
	if err := validateVendorMutation(orgID, actorID, id, input.Version); err != nil {
		return nil, err
	}
	input.Status = models.VendorStatus(strings.ToLower(strings.TrimSpace(string(input.Status))))
	input.Reason = strings.TrimSpace(input.Reason)
	current, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if !isValidVendorTransition(current.Status, input.Status) {
		return nil, fmt.Errorf("%w: %s to %s", ErrVendorInvalidTransition, current.Status, input.Status)
	}
	if requiresVendorTransitionReason(current.Status, input.Status) && !validVendorText(input.Reason, 3, 4000) {
		return nil, fmt.Errorf("%w: transition reason must contain 3 to 4000 characters", ErrVendorInvalid)
	}
	if input.Status == models.VendorStatusActive {
		if current.OwnerUserID == nil || strings.TrimSpace(current.ServiceDescription) == "" && len(current.Services) == 0 || len(current.Contacts) == 0 {
			return nil, fmt.Errorf("%w: activation requires an owner, service scope, and vendor contact", ErrVendorConflict)
		}
		if current.DPARequired && !current.DPAInPlace {
			return nil, fmt.Errorf("%w: required DPA must be executed before activation", ErrVendorConflict)
		}
	}
	if input.Status == models.VendorStatusOffboarded {
		for _, contract := range current.Contracts {
			if contract.Status == "active" || contract.Status == "renewal_due" {
				return nil, fmt.Errorf("%w: active contracts must be terminated before offboarding completes", ErrVendorConflict)
			}
		}
	}
	item, err := s.repository.Transition(ctx, orgID, actorID, id, input)
	if err != nil {
		return nil, s.translateError(err)
	}
	return item, nil
}

func (s *VendorService) RecordAssessment(ctx context.Context, orgID, actorID, id string, input models.VendorAssessmentInput) (*models.Vendor, error) {
	if err := validateVendorMutation(orgID, actorID, id, input.Version); err != nil {
		return nil, err
	}
	input.Status = models.VendorAssessmentStatus(strings.ToLower(strings.TrimSpace(string(input.Status))))
	input.RiskTier = models.VendorRiskTier(strings.ToLower(strings.TrimSpace(string(input.RiskTier))))
	input.Notes = strings.TrimSpace(input.Notes)
	input.AssessedAt = input.AssessedAt.UTC().Truncate(time.Microsecond)
	current, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if current.Status != models.VendorStatusActive && current.Status != models.VendorStatusSuspended {
		return nil, fmt.Errorf("%w: only active or suspended vendors can be assessed", ErrVendorConflict)
	}
	if input.Status != models.VendorAssessmentCompleted && input.Status != models.VendorAssessmentWaived ||
		!validVendorRiskTier(input.RiskTier) || input.AssessedAt.IsZero() || input.AssessedAt.After(s.now().Add(5*time.Minute)) ||
		input.RiskScore != nil && (*input.RiskScore < 0 || *input.RiskScore > 100) || !validVendorText(input.Notes, 3, 10000) {
		return nil, fmt.Errorf("%w: completed/waived status, risk result, assessment time, and notes are required", ErrVendorInvalid)
	}
	if input.NextAssessmentDate == nil {
		next := input.AssessedAt.AddDate(0, 0, current.AssessmentCadenceDays)
		input.NextAssessmentDate = &next
	} else {
		next := input.NextAssessmentDate.UTC()
		input.NextAssessmentDate = &next
	}
	if input.NextAssessmentDate.Before(input.AssessedAt) {
		return nil, fmt.Errorf("%w: next assessment cannot precede this assessment", ErrVendorInvalid)
	}
	item, err := s.repository.RecordAssessment(ctx, orgID, actorID, id, input)
	if err != nil {
		return nil, s.translateError(err)
	}
	return item, nil
}

func (s *VendorService) Statistics(ctx context.Context, orgID string) (*models.VendorStatistics, error) {
	if !isVendorUUID(orgID) {
		return nil, ErrVendorInvalidID
	}
	item, err := s.repository.Statistics(ctx, orgID)
	if err != nil {
		return nil, s.translateError(err)
	}
	return item, nil
}

func (s *VendorService) ListDueForAssessment(ctx context.Context, orgID string, horizonDays, limit int) ([]models.Vendor, error) {
	if !isVendorUUID(orgID) {
		return nil, ErrVendorInvalidID
	}
	horizonDays, limit, err := normalizeVendorHorizon(horizonDays, limit)
	if err != nil {
		return nil, err
	}
	items, err := s.repository.ListDueForAssessment(ctx, orgID, s.now().AddDate(0, 0, horizonDays), limit)
	if err != nil {
		return nil, s.translateError(err)
	}
	for index := range items {
		decorateVendorAssessment(&items[index], s.now())
	}
	return items, nil
}

func (s *VendorService) ListDueContracts(ctx context.Context, orgID string, horizonDays, limit int) ([]models.VendorDueContract, error) {
	if !isVendorUUID(orgID) {
		return nil, ErrVendorInvalidID
	}
	horizonDays, limit, err := normalizeVendorHorizon(horizonDays, limit)
	if err != nil {
		return nil, err
	}
	items, err := s.repository.ListDueContracts(ctx, orgID, s.now().AddDate(0, 0, horizonDays), limit)
	return items, s.translateError(err)
}

func (s *VendorService) ListExpiringCertifications(ctx context.Context, orgID string, horizonDays, limit int) ([]models.VendorExpiringCertification, error) {
	if !isVendorUUID(orgID) {
		return nil, ErrVendorInvalidID
	}
	horizonDays, limit, err := normalizeVendorHorizon(horizonDays, limit)
	if err != nil {
		return nil, err
	}
	items, err := s.repository.ListExpiringCertifications(ctx, orgID, s.now().AddDate(0, 0, horizonDays), limit)
	return items, s.translateError(err)
}

func (s *VendorService) ListEvents(ctx context.Context, orgID, id string, pagination models.PaginationRequest) ([]models.VendorEvent, int, error) {
	if err := validateVendorIDs(orgID, id); err != nil {
		return nil, 0, err
	}
	pagination = normalizeVendorPagination(pagination)
	items, total, err := s.repository.ListEvents(ctx, orgID, id, pagination)
	if err != nil {
		return nil, 0, s.translateError(err)
	}
	return items, total, nil
}

func (s *VendorService) SaveContact(ctx context.Context, orgID, actorID, vendorID, contactID string, input models.VendorContactInput) (*models.VendorContact, *models.Vendor, error) {
	if err := s.validateRelatedMutation(ctx, orgID, actorID, vendorID, contactID, input.Version); err != nil {
		return nil, nil, err
	}
	input.Name, input.Email = strings.TrimSpace(input.Name), strings.ToLower(strings.TrimSpace(input.Email))
	input.Phone, input.Title, input.ContactType = strings.TrimSpace(input.Phone), strings.TrimSpace(input.Title), strings.ToLower(strings.TrimSpace(input.ContactType))
	if !validVendorText(input.Name, 1, 200) || !validVendorEmail(input.Email) || !validVendorContactType(input.ContactType) ||
		!validVendorOptionalText(input.Phone, 50) || !validVendorOptionalText(input.Title, 150) {
		return nil, nil, fmt.Errorf("%w: invalid vendor contact", ErrVendorInvalid)
	}
	item, vendor, err := s.repository.SaveContact(ctx, orgID, actorID, vendorID, contactID, input)
	if err != nil {
		return nil, nil, s.translateError(err)
	}
	return item, vendor, nil
}

func (s *VendorService) DeleteContact(ctx context.Context, orgID, actorID, vendorID, id string, version int64) (*models.Vendor, error) {
	return s.deleteRelated(ctx, orgID, actorID, vendorID, id, version, s.repository.DeleteContact)
}

func (s *VendorService) SaveContract(ctx context.Context, orgID, actorID, vendorID, contractID string, input models.VendorContractInput) (*models.VendorContract, *models.Vendor, error) {
	if err := s.validateRelatedMutation(ctx, orgID, actorID, vendorID, contractID, input.Version); err != nil {
		return nil, nil, err
	}
	normalizeVendorContract(&input)
	if err := validateVendorContract(input); err != nil {
		return nil, nil, err
	}
	item, vendor, err := s.repository.SaveContract(ctx, orgID, actorID, vendorID, contractID, input)
	if err != nil {
		return nil, nil, s.translateError(err)
	}
	return item, vendor, nil
}

func (s *VendorService) DeleteContract(ctx context.Context, orgID, actorID, vendorID, id string, version int64) (*models.Vendor, error) {
	return s.deleteRelated(ctx, orgID, actorID, vendorID, id, version, s.repository.DeleteContract)
}

func (s *VendorService) SaveCertification(ctx context.Context, orgID, actorID, vendorID, certificationID string, input models.VendorCertificationInput) (*models.VendorCertification, *models.Vendor, error) {
	if err := s.validateRelatedMutation(ctx, orgID, actorID, vendorID, certificationID, input.Version); err != nil {
		return nil, nil, err
	}
	normalizeVendorCertification(&input)
	if err := validateVendorCertification(input); err != nil {
		return nil, nil, err
	}
	item, vendor, err := s.repository.SaveCertification(ctx, orgID, actorID, vendorID, certificationID, input)
	if err != nil {
		return nil, nil, s.translateError(err)
	}
	return item, vendor, nil
}

func (s *VendorService) DeleteCertification(ctx context.Context, orgID, actorID, vendorID, id string, version int64) (*models.Vendor, error) {
	return s.deleteRelated(ctx, orgID, actorID, vendorID, id, version, s.repository.DeleteCertification)
}

func (s *VendorService) SaveSubprocessor(ctx context.Context, orgID, actorID, vendorID, subprocessorID string, input models.VendorSubprocessorInput) (*models.VendorSubprocessor, *models.Vendor, error) {
	if err := s.validateRelatedMutation(ctx, orgID, actorID, vendorID, subprocessorID, input.Version); err != nil {
		return nil, nil, err
	}
	normalizeVendorSubprocessor(&input)
	if err := validateVendorSubprocessor(input); err != nil {
		return nil, nil, err
	}
	item, vendor, err := s.repository.SaveSubprocessor(ctx, orgID, actorID, vendorID, subprocessorID, input)
	if err != nil {
		return nil, nil, s.translateError(err)
	}
	return item, vendor, nil
}

func (s *VendorService) DeleteSubprocessor(ctx context.Context, orgID, actorID, vendorID, id string, version int64) (*models.Vendor, error) {
	return s.deleteRelated(ctx, orgID, actorID, vendorID, id, version, s.repository.DeleteSubprocessor)
}

type vendorRelatedDeleter func(context.Context, string, string, string, string, int64) (*models.Vendor, error)

func (s *VendorService) deleteRelated(ctx context.Context, orgID, actorID, vendorID, id string, version int64, deleteFn vendorRelatedDeleter) (*models.Vendor, error) {
	if err := s.validateRelatedMutation(ctx, orgID, actorID, vendorID, id, version); err != nil {
		return nil, err
	}
	item, err := deleteFn(ctx, orgID, actorID, vendorID, id, version)
	if err != nil {
		return nil, s.translateError(err)
	}
	return item, nil
}

func (s *VendorService) validateRelatedMutation(ctx context.Context, orgID, actorID, vendorID, resourceID string, version int64) error {
	if err := validateVendorMutation(orgID, actorID, vendorID, version); err != nil {
		return err
	}
	if resourceID != "" && !isVendorUUID(resourceID) {
		return ErrVendorInvalidID
	}
	item, err := s.GetByID(ctx, orgID, vendorID)
	if err != nil {
		return err
	}
	if item.Status == models.VendorStatusOffboarded || item.Status == models.VendorStatusRejected {
		return fmt.Errorf("%w: terminal vendor related records are immutable", ErrVendorConflict)
	}
	return nil
}

func (s *VendorService) translateError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrVendorNotFound
	case errors.Is(err, repository.ErrVendorVersionConflict):
		return ErrVendorVersionConflict
	case errors.Is(err, repository.ErrVendorUserInvalid):
		return ErrVendorUserNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: duplicate vendor or related record", ErrVendorConflict)
		case "23503":
			return fmt.Errorf("%w: referenced tenant record does not exist", ErrVendorInvalid)
		case "23502", "23514", "22001", "22P02":
			return ErrVendorInvalid
		}
	}
	return err
}

func normalizeVendorCreate(input *models.VendorCreateInput) {
	input.Name, input.LegalName = strings.TrimSpace(input.Name), strings.TrimSpace(input.LegalName)
	input.Description, input.Website = strings.TrimSpace(input.Description), strings.TrimSpace(input.Website)
	input.Industry, input.Category = strings.TrimSpace(input.Industry), strings.TrimSpace(input.Category)
	input.CountryCode = strings.ToUpper(strings.TrimSpace(input.CountryCode))
	input.ServiceDescription = strings.TrimSpace(input.ServiceDescription)
	input.Criticality = models.VendorCriticality(strings.ToLower(strings.TrimSpace(string(input.Criticality))))
	input.VendorTier = models.VendorTier(strings.ToLower(strings.TrimSpace(string(input.VendorTier))))
	input.RiskTier = models.VendorRiskTier(strings.ToLower(strings.TrimSpace(string(input.RiskTier))))
	input.DPAStatus = models.VendorDPAStatus(strings.ToLower(strings.TrimSpace(string(input.DPAStatus))))
	input.DPAReference = strings.TrimSpace(input.DPAReference)
	input.AssessmentFrequency = strings.ToLower(strings.TrimSpace(input.AssessmentFrequency))
	input.ContactName, input.ContactEmail = strings.TrimSpace(input.ContactName), strings.ToLower(strings.TrimSpace(input.ContactEmail))
	input.ContactPhone = strings.TrimSpace(input.ContactPhone)
	input.Services = normalizeVendorStrings(input.Services, 50, 200)
	input.DataCategories = normalizeVendorStrings(input.DataCategories, 50, 100)
	input.ProcessingLocations = normalizeVendorStrings(input.ProcessingLocations, 50, 100)
	input.Certifications = normalizeVendorStrings(input.Certifications, 50, 150)
	for index := range input.InitialContracts {
		normalizeVendorContract(&input.InitialContracts[index])
	}
	for index := range input.InitialSubProcessors {
		normalizeVendorSubprocessor(&input.InitialSubProcessors[index])
	}
}

func normalizeVendorPatch(patch *models.VendorPatch) {
	trim := func(value **string) {
		if *value != nil {
			next := strings.TrimSpace(**value)
			*value = &next
		}
	}
	for _, value := range []**string{&patch.Name, &patch.LegalName, &patch.Description, &patch.Website, &patch.Industry, &patch.Category, &patch.ServiceDescription, &patch.DPAReference} {
		trim(value)
	}
	if patch.CountryCode != nil {
		value := strings.ToUpper(strings.TrimSpace(*patch.CountryCode))
		patch.CountryCode = &value
	}
	if patch.Criticality != nil {
		value := models.VendorCriticality(strings.ToLower(strings.TrimSpace(string(*patch.Criticality))))
		patch.Criticality = &value
	}
	if patch.VendorTier != nil {
		value := models.VendorTier(strings.ToLower(strings.TrimSpace(string(*patch.VendorTier))))
		patch.VendorTier = &value
	}
	if patch.RiskTier != nil {
		value := models.VendorRiskTier(strings.ToLower(strings.TrimSpace(string(*patch.RiskTier))))
		patch.RiskTier = &value
	}
	if patch.DPAStatus != nil {
		value := models.VendorDPAStatus(strings.ToLower(strings.TrimSpace(string(*patch.DPAStatus))))
		patch.DPAStatus = &value
	}
	if patch.AssessmentFrequency != nil {
		value := strings.ToLower(strings.TrimSpace(*patch.AssessmentFrequency))
		patch.AssessmentFrequency = &value
	}
	if patch.Services != nil {
		values := normalizeVendorStrings(*patch.Services, 50, 200)
		patch.Services = &values
	}
	if patch.DataCategories != nil {
		values := normalizeVendorStrings(*patch.DataCategories, 50, 100)
		patch.DataCategories = &values
	}
	if patch.ProcessingLocations != nil {
		values := normalizeVendorStrings(*patch.ProcessingLocations, 50, 100)
		patch.ProcessingLocations = &values
	}
}

func validateVendorCreate(input models.VendorCreateInput, now time.Time) error {
	if !validVendorText(input.Name, 1, 200) || !validVendorOptionalText(input.LegalName, 250) || !validVendorOptionalText(input.Description, 20000) ||
		!validVendorOptionalText(input.Industry, 100) || !validVendorOptionalText(input.Category, 100) || !validVendorOptionalText(input.ServiceDescription, 10000) {
		return fmt.Errorf("%w: vendor text fields are missing or exceed their limits", ErrVendorInvalid)
	}
	if input.Website != "" && !validVendorURL(input.Website) || input.CountryCode != "" && len(input.CountryCode) != 2 ||
		!validVendorCriticality(input.Criticality) || !validVendorTier(input.VendorTier) || !validVendorRiskTier(input.RiskTier) ||
		input.RiskScore != nil && (*input.RiskScore < 0 || *input.RiskScore > 100) ||
		!validAssessmentFrequency(input.AssessmentFrequency) || input.AssessmentCadenceDays < 30 || input.AssessmentCadenceDays > 1095 {
		return fmt.Errorf("%w: unsupported vendor classification, URL, risk, or assessment cadence", ErrVendorInvalid)
	}
	if input.OwnerUserID != nil && !isVendorUUID(*input.OwnerUserID) {
		return ErrVendorInvalidID
	}
	if len(input.Services) > 50 || len(input.DataCategories) > 50 || len(input.ProcessingLocations) > 50 || len(input.Certifications) > 50 {
		return fmt.Errorf("%w: vendor collections exceed their limits", ErrVendorInvalid)
	}
	if (input.ContactName == "") != (input.ContactEmail == "") || input.ContactEmail != "" && !validVendorEmail(input.ContactEmail) {
		return fmt.Errorf("%w: primary contact requires a valid name and email", ErrVendorInvalid)
	}
	if input.DataProcessing && len(input.DataCategories) == 0 {
		return fmt.Errorf("%w: data categories are required when personal data is processed", ErrVendorInvalid)
	}
	if !validVendorDPA(input.DataProcessing, input.DPARequired, input.DPAStatus, input.DPAReference, input.DPASignedDate, input.DPAExpiryDate) {
		return fmt.Errorf("%w: inconsistent DPA state", ErrVendorInvalid)
	}
	if input.RetentionUntil != nil && input.RetentionUntil.Before(now) || input.NextAssessmentDate != nil && input.NextAssessmentDate.Before(now.Add(-24*time.Hour)) {
		return fmt.Errorf("%w: retention and next assessment dates cannot be in the past", ErrVendorInvalid)
	}
	for _, contract := range input.InitialContracts {
		if err := validateVendorContract(contract); err != nil {
			return err
		}
	}
	for _, subprocessor := range input.InitialSubProcessors {
		if err := validateVendorSubprocessor(subprocessor); err != nil {
			return err
		}
	}
	return validateVendorJSON(input.Metadata, 64<<10)
}

func validateVendorRecord(item *models.Vendor, now time.Time) error {
	return validateVendorCreate(models.VendorCreateInput{Name: item.Name, LegalName: item.LegalName, Description: item.Description, Website: item.Website, Industry: item.Industry, Category: item.Category, CountryCode: item.CountryCode, OwnerUserID: item.OwnerUserID, Criticality: item.Criticality, VendorTier: item.VendorTier, RiskTier: item.RiskTier, RiskScore: item.RiskScore, ServiceDescription: item.ServiceDescription, Services: item.Services, DataProcessing: item.DataProcessing, DataCategories: item.DataCategories, ProcessingLocations: item.ProcessingLocations, DPARequired: item.DPARequired, DPAStatus: item.DPAStatus, DPAReference: item.DPAReference, DPASignedDate: item.DPASignedDate, DPAExpiryDate: item.DPAExpiryDate, AssessmentFrequency: item.AssessmentFrequency, AssessmentCadenceDays: item.AssessmentCadenceDays, NextAssessmentDate: item.NextAssessmentDate, NextReviewDate: item.NextReviewDate, RetentionUntil: item.RetentionUntil, Metadata: item.Metadata}, now)
}

func applyVendorPatch(item *models.Vendor, patch models.VendorPatch) {
	if patch.Name != nil {
		item.Name = *patch.Name
	}
	if patch.LegalName != nil {
		item.LegalName = *patch.LegalName
	}
	if patch.Description != nil {
		item.Description = *patch.Description
	}
	if patch.Website != nil {
		item.Website = *patch.Website
	}
	if patch.Industry != nil {
		item.Industry = *patch.Industry
	}
	if patch.Category != nil {
		item.Category = *patch.Category
	}
	if patch.CountryCode != nil {
		item.CountryCode = *patch.CountryCode
	}
	if patch.ClearOwner {
		item.OwnerUserID = nil
	} else if patch.OwnerUserID != nil {
		item.OwnerUserID = patch.OwnerUserID
	}
	if patch.Criticality != nil {
		item.Criticality = *patch.Criticality
	}
	if patch.VendorTier != nil {
		item.VendorTier = *patch.VendorTier
	}
	if patch.RiskTier != nil {
		item.RiskTier = *patch.RiskTier
	}
	if patch.ClearRiskScore {
		item.RiskScore = nil
	} else if patch.RiskScore != nil {
		item.RiskScore = patch.RiskScore
	}
	if patch.ServiceDescription != nil {
		item.ServiceDescription = *patch.ServiceDescription
	}
	if patch.Services != nil {
		item.Services = *patch.Services
	}
	if patch.DataProcessing != nil {
		item.DataProcessing = *patch.DataProcessing
	}
	if patch.DataCategories != nil {
		item.DataCategories = *patch.DataCategories
	}
	if patch.ProcessingLocations != nil {
		item.ProcessingLocations = *patch.ProcessingLocations
	}
	if patch.DPARequired != nil {
		item.DPARequired = *patch.DPARequired
	}
	if patch.ClearDPA {
		item.DPAStatus = models.VendorDPANotRequired
		item.DPAInPlace = false
		item.DPAReference = ""
		item.DPASignedDate = nil
		item.DPAExpiryDate = nil
	} else {
		if patch.DPAStatus != nil {
			item.DPAStatus = *patch.DPAStatus
			item.DPAInPlace = *patch.DPAStatus == models.VendorDPAExecuted
		}
		if patch.DPAReference != nil {
			item.DPAReference = *patch.DPAReference
		}
		if patch.DPASignedDate != nil {
			item.DPASignedDate = patch.DPASignedDate
		}
		if patch.DPAExpiryDate != nil {
			item.DPAExpiryDate = patch.DPAExpiryDate
		}
	}
	if patch.AssessmentFrequency != nil {
		item.AssessmentFrequency = *patch.AssessmentFrequency
	}
	if patch.AssessmentCadenceDays != nil {
		item.AssessmentCadenceDays = *patch.AssessmentCadenceDays
	}
	if patch.ClearNextAssessment {
		item.NextAssessmentDate = nil
	} else if patch.NextAssessmentDate != nil {
		item.NextAssessmentDate = patch.NextAssessmentDate
	}
	if patch.ClearNextReview {
		item.NextReviewDate = nil
	} else if patch.NextReviewDate != nil {
		item.NextReviewDate = patch.NextReviewDate
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

func normalizeVendorContract(input *models.VendorContractInput) {
	input.ContractRef = strings.TrimSpace(input.ContractRef)
	input.Name = strings.TrimSpace(input.Name)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = "draft"
	}
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if input.Currency == "" {
		input.Currency = "EUR"
	}
	if input.NoticeDays == 0 {
		input.NoticeDays = 30
	}
}
func validateVendorContract(input models.VendorContractInput) error {
	statuses := map[string]bool{"draft": true, "active": true, "renewal_due": true, "expired": true, "terminated": true}
	if !validVendorText(input.ContractRef, 1, 100) || !validVendorText(input.Name, 1, 200) || !statuses[input.Status] || len(input.Currency) != 3 || input.NoticeDays < 0 || input.NoticeDays > 1095 || input.ValueAmount != nil && *input.ValueAmount < 0 || input.StartDate != nil && input.EndDate != nil && input.EndDate.Before(*input.StartDate) || input.OwnerUserID != nil && !isVendorUUID(*input.OwnerUserID) {
		return fmt.Errorf("%w: invalid vendor contract", ErrVendorInvalid)
	}
	return validateVendorJSON(input.Metadata, 64<<10)
}
func normalizeVendorCertification(input *models.VendorCertificationInput) {
	input.Name = strings.TrimSpace(input.Name)
	input.Issuer = strings.TrimSpace(input.Issuer)
	input.CertificateNumber = strings.TrimSpace(input.CertificateNumber)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = "active"
	}
	input.EvidenceReference = strings.TrimSpace(input.EvidenceReference)
}
func validateVendorCertification(input models.VendorCertificationInput) error {
	statuses := map[string]bool{"pending": true, "active": true, "expired": true, "revoked": true}
	if !validVendorText(input.Name, 1, 150) || !validVendorOptionalText(input.Issuer, 200) || !validVendorOptionalText(input.CertificateNumber, 150) || !validVendorOptionalText(input.EvidenceReference, 500) || !statuses[input.Status] || input.IssuedOn != nil && input.ExpiresOn != nil && input.ExpiresOn.Before(*input.IssuedOn) {
		return fmt.Errorf("%w: invalid vendor certification", ErrVendorInvalid)
	}
	return validateVendorJSON(input.Metadata, 64<<10)
}
func normalizeVendorSubprocessor(input *models.VendorSubprocessorInput) {
	input.Name = strings.TrimSpace(input.Name)
	input.Purpose = strings.TrimSpace(input.Purpose)
	input.CountryCode = strings.ToUpper(strings.TrimSpace(input.CountryCode))
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = "proposed"
	}
	input.DataCategories = normalizeVendorStrings(input.DataCategories, 50, 100)
}
func validateVendorSubprocessor(input models.VendorSubprocessorInput) error {
	statuses := map[string]bool{"proposed": true, "approved": true, "rejected": true, "removed": true}
	if !validVendorText(input.Name, 1, 200) || !validVendorText(input.Purpose, 3, 1000) || input.CountryCode != "" && len(input.CountryCode) != 2 || !statuses[input.Status] || len(input.DataCategories) > 50 {
		return fmt.Errorf("%w: invalid vendor subprocessor", ErrVendorInvalid)
	}
	return validateVendorJSON(input.Metadata, 64<<10)
}

func isValidVendorTransition(current, next models.VendorStatus) bool {
	allowed := map[models.VendorStatus]map[models.VendorStatus]bool{models.VendorStatusProspective: {models.VendorStatusOnboarding: true, models.VendorStatusRejected: true}, models.VendorStatusOnboarding: {models.VendorStatusActive: true, models.VendorStatusRejected: true}, models.VendorStatusActive: {models.VendorStatusSuspended: true, models.VendorStatusOffboarding: true}, models.VendorStatusSuspended: {models.VendorStatusActive: true, models.VendorStatusOffboarding: true}, models.VendorStatusOffboarding: {models.VendorStatusOffboarded: true, models.VendorStatusActive: true}, models.VendorStatusOffboarded: {models.VendorStatusOnboarding: true}, models.VendorStatusRejected: {models.VendorStatusOnboarding: true}}
	return allowed[current][next]
}
func requiresVendorTransitionReason(current, next models.VendorStatus) bool {
	return next == models.VendorStatusRejected || next == models.VendorStatusSuspended || next == models.VendorStatusOffboarding || next == models.VendorStatusOffboarded || current == models.VendorStatusOffboarding || current == models.VendorStatusOffboarded || current == models.VendorStatusRejected
}
func validVendorStatus(v models.VendorStatus) bool {
	switch v {
	case models.VendorStatusProspective, models.VendorStatusOnboarding, models.VendorStatusActive, models.VendorStatusSuspended, models.VendorStatusOffboarding, models.VendorStatusOffboarded, models.VendorStatusRejected:
		return true
	}
	return false
}
func validVendorCriticality(v models.VendorCriticality) bool {
	switch v {
	case models.VendorCriticalityCritical, models.VendorCriticalityHigh, models.VendorCriticalityMedium, models.VendorCriticalityLow:
		return true
	}
	return false
}
func validVendorRiskTier(v models.VendorRiskTier) bool {
	switch v {
	case models.VendorRiskCritical, models.VendorRiskHigh, models.VendorRiskMedium, models.VendorRiskLow:
		return true
	}
	return false
}
func validVendorTier(v models.VendorTier) bool {
	switch v {
	case models.VendorTierOne, models.VendorTierTwo, models.VendorTierThree, models.VendorTierFour:
		return true
	}
	return false
}
func validAssessmentFrequency(v string) bool {
	switch v {
	case "monthly", "quarterly", "semi_annual", "annual", "biennial", "custom":
		return true
	}
	return false
}
func cadenceForFrequency(v string) int {
	switch v {
	case "monthly":
		return 30
	case "quarterly":
		return 90
	case "semi_annual":
		return 182
	case "biennial":
		return 730
	default:
		return 365
	}
}
func validVendorContactType(v string) bool {
	switch v {
	case "business", "security", "privacy", "legal", "billing", "technical":
		return true
	}
	return false
}
func validVendorDPA(processing, required bool, status models.VendorDPAStatus, reference string, signed, expiry *time.Time) bool {
	if required && !processing {
		return false
	}
	switch status {
	case models.VendorDPANotRequired:
		return !required && reference == "" && signed == nil && expiry == nil
	case models.VendorDPAPending:
		return processing && !(!required && reference != "")
	case models.VendorDPAExecuted:
		if !processing || signed == nil || strings.TrimSpace(reference) == "" {
			return false
		}
	case models.VendorDPAExpired, models.VendorDPATerminated:
		if !processing {
			return false
		}
	default:
		return false
	}
	return expiry == nil || signed == nil || !expiry.Before(*signed)
}
func validVendorText(v string, min, max int) bool {
	n := len([]rune(strings.TrimSpace(v)))
	return n >= min && n <= max
}
func validVendorOptionalText(v string, max int) bool { return v == "" || validVendorText(v, 1, max) }
func validVendorEmail(v string) bool {
	address, err := mail.ParseAddress(v)
	return err == nil && strings.EqualFold(address.Address, v) && len(v) <= 320
}
func validVendorURL(v string) bool {
	parsed, err := url.ParseRequestURI(v)
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" && len(v) <= 500
}
func isVendorUUID(v string) bool { _, err := uuid.Parse(strings.TrimSpace(v)); return err == nil }
func validateVendorIdentity(orgID, actorID string) error {
	if !isVendorUUID(orgID) || !isVendorUUID(actorID) {
		return ErrVendorInvalidID
	}
	return nil
}
func validateVendorIDs(orgID, id string) error {
	if !isVendorUUID(orgID) || !isVendorUUID(id) {
		return ErrVendorInvalidID
	}
	return nil
}
func validateVendorMutation(orgID, actorID, id string, version int64) error {
	if err := validateVendorIdentity(orgID, actorID); err != nil {
		return err
	}
	if !isVendorUUID(id) {
		return ErrVendorInvalidID
	}
	if version < 1 {
		return fmt.Errorf("%w: version must be positive", ErrVendorInvalid)
	}
	return nil
}
func validateVendorJSON(value json.RawMessage, max int) error {
	if len(value) == 0 {
		return nil
	}
	if len(value) > max {
		return fmt.Errorf("%w: metadata exceeds %d bytes", ErrVendorInvalid, max)
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return fmt.Errorf("%w: metadata must be valid JSON", ErrVendorInvalid)
	}
	if _, ok := decoded.(map[string]any); !ok {
		return fmt.Errorf("%w: metadata must be an object", ErrVendorInvalid)
	}
	return nil
}
func normalizeVendorStrings(values []string, limit, maxLength int) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || len([]rune(value)) > maxLength || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result
}
func normalizeVendorFilter(filter *models.VendorListFilter) {
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	filter.Criticality = strings.ToLower(strings.TrimSpace(filter.Criticality))
	filter.VendorTier = strings.ToLower(strings.TrimSpace(filter.VendorTier))
	filter.RiskTier = strings.ToLower(strings.TrimSpace(filter.RiskTier))
	filter.OwnerUserID = strings.TrimSpace(filter.OwnerUserID)
	filter.CountryCode = strings.ToUpper(strings.TrimSpace(filter.CountryCode))
	filter.Search = strings.TrimSpace(filter.Search)
	filter.SortBy = strings.ToLower(strings.TrimSpace(filter.SortBy))
	filter.SortDirection = strings.ToLower(strings.TrimSpace(filter.SortDirection))
	if filter.SortDirection == "" {
		filter.SortDirection = "desc"
	}
	filter.PaginationRequest = normalizeVendorPagination(filter.PaginationRequest)
}
func normalizeVendorPagination(v models.PaginationRequest) models.PaginationRequest {
	if v.Page < 1 {
		v.Page = 1
	}
	if v.PageSize < 1 || v.PageSize > 100 {
		v.PageSize = 20
	}
	return v
}
func normalizeVendorHorizon(days, limit int) (int, int, error) {
	if days < 1 {
		days = 30
	}
	if days > 365 {
		return 0, 0, fmt.Errorf("%w: horizon_days cannot exceed 365", ErrVendorInvalid)
	}
	if limit < 1 {
		limit = 100
	}
	if limit > 200 {
		return 0, 0, fmt.Errorf("%w: limit cannot exceed 200", ErrVendorInvalid)
	}
	return days, limit, nil
}
func decorateVendorAssessment(item *models.Vendor, now time.Time) {
	if item == nil || item.NextAssessmentDate == nil {
		return
	}
	today := now.UTC().Truncate(24 * time.Hour)
	switch {
	case item.NextAssessmentDate.Before(today):
		item.AssessmentStatus = models.VendorAssessmentOverdue
	case !item.NextAssessmentDate.After(today):
		item.AssessmentStatus = models.VendorAssessmentDue
	}
}
