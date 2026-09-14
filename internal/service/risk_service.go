package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrRiskNotFound          = errors.New("risk not found")
	ErrRiskTreatmentNotFound = errors.New("risk treatment not found")
	ErrRiskIndicatorNotFound = errors.New("risk indicator not found")
	ErrRiskConflict          = errors.New("risk reference already exists")
	ErrInvalidRisk           = errors.New("invalid risk")
	ErrInvalidRiskID         = errors.New("invalid risk identifier")
	ErrInvalidRiskReference  = errors.New("risk references a resource outside this organization")
	ErrInvalidRiskTransition = errors.New("invalid risk status transition")
	ErrInvalidAssessment     = errors.New("invalid risk assessment")
	ErrInvalidTreatment      = errors.New("invalid risk treatment")
	ErrInvalidTreatmentState = errors.New("invalid treatment status transition")
	ErrInvalidRiskAppetite   = errors.New("invalid risk appetite statement")
	ErrInvalidRiskIndicator  = errors.New("invalid risk indicator")
)

type RiskManagementRepository interface {
	Create(context.Context, string, models.RiskCreateInput) (*models.Risk, error)
	GetByID(context.Context, string, string) (*models.Risk, error)
	Update(context.Context, string, *models.Risk) (*models.Risk, error)
	Delete(context.Context, string, string) error
	List(context.Context, string, models.RiskListFilter) ([]models.Risk, int, error)
	ListCategories(context.Context, string) ([]models.RiskCategory, error)
	GetRiskMatrix(context.Context, string, string) (*models.RiskMatrixView, error)
	GetRiskHeatmap(context.Context, string) ([]models.RiskHeatmapEntry, error)
	CreateAssessment(context.Context, string, string, string, models.RiskAssessmentInput, *float64, *string, *float64, *string) (*models.RiskAssessment, error)
	ListAssessments(context.Context, string, string, models.PaginationRequest) ([]models.RiskAssessment, int, error)
	CreateTreatment(context.Context, string, string, models.RiskTreatmentInput) (*models.RiskTreatment, error)
	GetTreatment(context.Context, string, string, string) (*models.RiskTreatment, error)
	UpdateTreatment(context.Context, string, string, *models.RiskTreatment) (*models.RiskTreatment, error)
	ListTreatments(context.Context, string, string, models.PaginationRequest) ([]models.RiskTreatment, int, error)
	ListAppetite(context.Context, string) ([]models.RiskAppetiteStatement, error)
	UpsertAppetite(context.Context, string, string, string, models.RiskAppetiteInput) (*models.RiskAppetiteStatement, error)
	CreateIndicator(context.Context, string, string, models.RiskIndicatorInput) (*models.RiskIndicator, error)
	ListIndicators(context.Context, string, string) ([]models.RiskIndicator, error)
	RecordIndicatorValue(context.Context, string, string, string, string, models.RiskIndicatorValueInput) (*models.RiskIndicatorValue, error)
	ListIndicatorValues(context.Context, string, string, string, models.PaginationRequest) ([]models.RiskIndicatorValue, int, error)
}

type RiskService struct {
	repository RiskManagementRepository
	logger     zerolog.Logger
}

func NewRiskService(repository RiskManagementRepository, logger zerolog.Logger) *RiskService {
	return &RiskService{repository: repository, logger: logger.With().Str("service", "risk").Logger()}
}

func (s *RiskService) Create(ctx context.Context, orgID string, input models.RiskCreateInput) (*models.Risk, error) {
	if !validUUID(orgID) {
		return nil, ErrInvalidRiskID
	}
	normalizeRiskCreate(&input)
	if err := validateRiskCreate(input); err != nil {
		return nil, err
	}
	risk, err := s.repository.Create(ctx, orgID, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidRiskReference
	}
	if err != nil {
		return nil, mapRiskWriteError(err)
	}
	s.logger.Info().Str("organization_id", orgID).Str("risk_id", risk.ID).Str("risk_ref", risk.RiskRef).Msg("risk created")
	return risk, nil
}

func (s *RiskService) GetByID(ctx context.Context, orgID, id string) (*models.Risk, error) {
	if !validUUID(orgID) || !validUUID(id) {
		return nil, ErrInvalidRiskID
	}
	risk, err := s.repository.GetByID(ctx, orgID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRiskNotFound
	}
	return risk, err
}

func (s *RiskService) Update(ctx context.Context, orgID, id string, patch models.RiskPatch) (*models.Risk, error) {
	if !validUUID(orgID) || !validUUID(id) {
		return nil, ErrInvalidRiskID
	}
	if riskPatchEmpty(patch) {
		return nil, fmt.Errorf("%w: no fields supplied", ErrInvalidRisk)
	}
	risk, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	previousStatus := risk.Status
	applyRiskPatch(risk, patch)
	if !validRiskStatusTransition(previousStatus, risk.Status) {
		return nil, fmt.Errorf("%w: %s to %s", ErrInvalidRiskTransition, previousStatus, risk.Status)
	}
	if err := validateRisk(risk); err != nil {
		return nil, err
	}
	updated, err := s.repository.Update(ctx, orgID, risk)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidRiskReference
	}
	if err == nil {
		s.logger.Info().Str("organization_id", orgID).Str("risk_id", id).Msg("risk updated")
	}
	return updated, mapRiskWriteError(err)
}

func (s *RiskService) Assign(ctx context.Context, orgID, id string, ownerID, delegateID *string) (*models.Risk, error) {
	if ownerID == nil && delegateID == nil {
		return nil, fmt.Errorf("%w: owner_user_id or delegate_user_id is required", ErrInvalidRisk)
	}
	return s.Update(ctx, orgID, id, models.RiskPatch{OwnerUserID: ownerID, DelegateUserID: delegateID})
}

func (s *RiskService) Delete(ctx context.Context, orgID, id string) error {
	if !validUUID(orgID) || !validUUID(id) {
		return ErrInvalidRiskID
	}
	err := s.repository.Delete(ctx, orgID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRiskNotFound
	}
	if err == nil {
		s.logger.Info().Str("organization_id", orgID).Str("risk_id", id).Msg("risk soft-deleted")
	}
	return err
}

func (s *RiskService) List(ctx context.Context, orgID string, filter models.RiskListFilter) ([]models.Risk, int, error) {
	if !validUUID(orgID) {
		return nil, 0, ErrInvalidRiskID
	}
	filter.PaginationRequest = normalizeRiskPagination(filter.PaginationRequest)
	filter.Status, filter.Level, filter.Search = strings.TrimSpace(filter.Status), strings.TrimSpace(filter.Level), strings.TrimSpace(filter.Search)
	if filter.Status != "" && !allowed(filter.Status, riskStatuses()...) {
		return nil, 0, fmt.Errorf("%w: unsupported status filter", ErrInvalidRisk)
	}
	if filter.Level != "" && !allowed(filter.Level, "critical", "high", "medium", "low", "very_low") {
		return nil, 0, fmt.Errorf("%w: unsupported level filter", ErrInvalidRisk)
	}
	for _, id := range []string{filter.CategoryID, filter.OwnerUserID} {
		if id != "" && !validUUID(id) {
			return nil, 0, ErrInvalidRiskID
		}
	}
	return s.repository.List(ctx, orgID, filter)
}

func (s *RiskService) GetRiskMatrix(ctx context.Context, orgID, dimension string) (*models.RiskMatrixView, error) {
	if !validUUID(orgID) {
		return nil, ErrInvalidRiskID
	}
	if dimension == "" {
		dimension = "residual"
	}
	if !allowed(dimension, "inherent", "residual", "target") {
		return nil, fmt.Errorf("%w: dimension must be inherent, residual, or target", ErrInvalidRisk)
	}
	return s.repository.GetRiskMatrix(ctx, orgID, dimension)
}

func (s *RiskService) ListCategories(ctx context.Context, orgID string) ([]models.RiskCategory, error) {
	if !validUUID(orgID) {
		return nil, ErrInvalidRiskID
	}
	return s.repository.ListCategories(ctx, orgID)
}

func (s *RiskService) GetRiskHeatmap(ctx context.Context, orgID string) ([]models.RiskHeatmapEntry, error) {
	if !validUUID(orgID) {
		return nil, ErrInvalidRiskID
	}
	return s.repository.GetRiskHeatmap(ctx, orgID)
}

func (s *RiskService) CreateAssessment(ctx context.Context, orgID, riskID, userID string, input models.RiskAssessmentInput) (*models.RiskAssessment, error) {
	if !validUUID(orgID) || !validUUID(riskID) || !validUUID(userID) {
		return nil, ErrInvalidRiskID
	}
	if err := validateAssessment(input); err != nil {
		return nil, err
	}
	beforeScore, beforeLevel := scorePair(input.LikelihoodBefore, input.ImpactBefore)
	afterScore, afterLevel := scorePair(input.LikelihoodAfter, input.ImpactAfter)
	item, err := s.repository.CreateAssessment(ctx, orgID, riskID, userID, input, beforeScore, beforeLevel, afterScore, afterLevel)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRiskNotFound
	}
	return item, err
}

func (s *RiskService) ListAssessments(ctx context.Context, orgID, riskID string, p models.PaginationRequest) ([]models.RiskAssessment, int, error) {
	if _, err := s.GetByID(ctx, orgID, riskID); err != nil {
		return nil, 0, err
	}
	return s.repository.ListAssessments(ctx, orgID, riskID, normalizeRiskPagination(p))
}

func (s *RiskService) CreateTreatment(ctx context.Context, orgID, riskID, userID string, input models.RiskTreatmentInput) (*models.RiskTreatment, error) {
	if !validUUID(orgID) || !validUUID(riskID) || !validUUID(userID) {
		return nil, ErrInvalidRiskID
	}
	if input.OwnerUserID == nil {
		input.OwnerUserID = &userID
	}
	normalizeOptionalID(&input.OwnerUserID)
	if err := validateTreatmentInput(input); err != nil {
		return nil, err
	}
	item, err := s.repository.CreateTreatment(ctx, orgID, riskID, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidRiskReference
	}
	return item, err
}

func (s *RiskService) ListTreatments(ctx context.Context, orgID, riskID string, p models.PaginationRequest) ([]models.RiskTreatment, int, error) {
	if _, err := s.GetByID(ctx, orgID, riskID); err != nil {
		return nil, 0, err
	}
	return s.repository.ListTreatments(ctx, orgID, riskID, normalizeRiskPagination(p))
}

func (s *RiskService) GetTreatment(ctx context.Context, orgID, riskID, treatmentID string) (*models.RiskTreatment, error) {
	if !validUUID(orgID) || !validUUID(riskID) || !validUUID(treatmentID) {
		return nil, ErrInvalidRiskID
	}
	item, err := s.repository.GetTreatment(ctx, orgID, riskID, treatmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRiskTreatmentNotFound
	}
	return item, err
}

func (s *RiskService) UpdateTreatment(ctx context.Context, orgID, riskID, treatmentID string, patch models.RiskTreatmentPatch) (*models.RiskTreatment, error) {
	if !validUUID(orgID) || !validUUID(riskID) || !validUUID(treatmentID) {
		return nil, ErrInvalidRiskID
	}
	if treatmentPatchEmpty(patch) {
		return nil, fmt.Errorf("%w: no fields supplied", ErrInvalidTreatment)
	}
	treatment, err := s.GetTreatment(ctx, orgID, riskID, treatmentID)
	if err != nil {
		return nil, err
	}
	previous := treatment.Status
	applyTreatmentPatch(treatment, patch)
	if !validTreatmentTransition(previous, treatment.Status) {
		return nil, fmt.Errorf("%w: %s to %s", ErrInvalidTreatmentState, previous, treatment.Status)
	}
	if treatment.Status == "completed" {
		if treatment.ProgressPercentage != 100 {
			return nil, fmt.Errorf("%w: completed treatment requires 100 percent progress", ErrInvalidTreatment)
		}
		now := time.Now().UTC()
		treatment.CompletedDate = &now
	} else if treatment.Status != "completed" {
		treatment.CompletedDate = nil
	}
	if err := validateTreatment(treatment); err != nil {
		return nil, err
	}
	updated, err := s.repository.UpdateTreatment(ctx, orgID, riskID, treatment)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidRiskReference
	}
	return updated, err
}

func (s *RiskService) ListAppetite(ctx context.Context, orgID string) ([]models.RiskAppetiteStatement, error) {
	if !validUUID(orgID) {
		return nil, ErrInvalidRiskID
	}
	return s.repository.ListAppetite(ctx, orgID)
}

func (s *RiskService) UpsertAppetite(ctx context.Context, orgID, categoryID, userID string, input models.RiskAppetiteInput) (*models.RiskAppetiteStatement, error) {
	if !validUUID(orgID) || !validUUID(categoryID) || !validUUID(userID) {
		return nil, ErrInvalidRiskID
	}
	if err := validateAppetite(input); err != nil {
		return nil, err
	}
	if input.Status == "approved" {
		return nil, fmt.Errorf("%w: use the approval endpoint to approve appetite", ErrInvalidRiskAppetite)
	}
	item, err := s.repository.UpsertAppetite(ctx, orgID, categoryID, userID, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidRiskReference
	}
	return item, err
}

func (s *RiskService) ApproveAppetite(ctx context.Context, orgID, categoryID, userID string, input models.RiskAppetiteInput) (*models.RiskAppetiteStatement, error) {
	if !validUUID(orgID) || !validUUID(categoryID) || !validUUID(userID) {
		return nil, ErrInvalidRiskID
	}
	input.Status = "approved"
	if err := validateAppetite(input); err != nil {
		return nil, err
	}
	item, err := s.repository.UpsertAppetite(ctx, orgID, categoryID, userID, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidRiskReference
	}
	return item, err
}

func (s *RiskService) CreateIndicator(ctx context.Context, orgID, riskID, userID string, input models.RiskIndicatorInput) (*models.RiskIndicator, error) {
	if !validUUID(orgID) || !validUUID(riskID) || !validUUID(userID) {
		return nil, ErrInvalidRiskID
	}
	if input.OwnerUserID == nil {
		input.OwnerUserID = &userID
	}
	if input.CollectionFrequency == "" {
		input.CollectionFrequency = "monthly"
	}
	if len(input.AutomationConfig) == 0 {
		input.AutomationConfig = json.RawMessage(`{}`)
	}
	if err := validateIndicator(input); err != nil {
		return nil, err
	}
	item, err := s.repository.CreateIndicator(ctx, orgID, riskID, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidRiskReference
	}
	return item, err
}

func (s *RiskService) ListIndicators(ctx context.Context, orgID, riskID string) ([]models.RiskIndicator, error) {
	if _, err := s.GetByID(ctx, orgID, riskID); err != nil {
		return nil, err
	}
	return s.repository.ListIndicators(ctx, orgID, riskID)
}

func (s *RiskService) RecordIndicatorValue(ctx context.Context, orgID, riskID, indicatorID, userID string, input models.RiskIndicatorValueInput) (*models.RiskIndicatorValue, error) {
	if !validUUID(orgID) || !validUUID(riskID) || !validUUID(indicatorID) || !validUUID(userID) {
		return nil, ErrInvalidRiskID
	}
	if math.IsNaN(input.Value) || math.IsInf(input.Value, 0) {
		return nil, fmt.Errorf("%w: value must be finite", ErrInvalidRiskIndicator)
	}
	if input.MeasuredAt != nil && input.MeasuredAt.After(time.Now().Add(5*time.Minute)) {
		return nil, fmt.Errorf("%w: measured_at cannot be in the future", ErrInvalidRiskIndicator)
	}
	item, err := s.repository.RecordIndicatorValue(ctx, orgID, riskID, indicatorID, userID, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRiskIndicatorNotFound
	}
	return item, err
}

func (s *RiskService) ListIndicatorValues(ctx context.Context, orgID, riskID, indicatorID string, p models.PaginationRequest) ([]models.RiskIndicatorValue, int, error) {
	if !validUUID(orgID) || !validUUID(riskID) || !validUUID(indicatorID) {
		return nil, 0, ErrInvalidRiskID
	}
	indicators, err := s.ListIndicators(ctx, orgID, riskID)
	if err != nil {
		return nil, 0, err
	}
	found := false
	for _, indicator := range indicators {
		if indicator.ID == indicatorID {
			found = true
			break
		}
	}
	if !found {
		return nil, 0, ErrRiskIndicatorNotFound
	}
	return s.repository.ListIndicatorValues(ctx, orgID, riskID, indicatorID, normalizeRiskPagination(p))
}

func mapRiskWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return fmt.Errorf("%w: %s", ErrRiskConflict, pgError.ConstraintName)
	}
	return err
}

func (s *RiskService) CalculateRiskScore(likelihood, impact int) (float64, string) {
	score := float64(likelihood * impact)
	return score, riskLevel(score)
}

func riskLevel(score float64) string {
	switch {
	case score >= 20:
		return "critical"
	case score >= 12:
		return "high"
	case score >= 6:
		return "medium"
	case score >= 3:
		return "low"
	default:
		return "very_low"
	}
}

func scorePair(likelihood, impact *int) (*float64, *string) {
	if likelihood == nil || impact == nil {
		return nil, nil
	}
	score := float64(*likelihood * *impact)
	level := riskLevel(score)
	return &score, &level
}

func validateRiskCreate(input models.RiskCreateInput) error {
	if strings.TrimSpace(input.Title) == "" || len(input.Title) > 500 {
		return fmt.Errorf("%w: title is required and must not exceed 500 characters", ErrInvalidRisk)
	}
	if len(input.RiskRef) > 20 {
		return fmt.Errorf("%w: risk_ref exceeds 20 characters", ErrInvalidRisk)
	}
	if err := validateRiskValues(input.RiskSource, input.RiskType, input.RiskVelocity,
		input.RiskProximity, input.ReviewFrequency, input.InherentLikelihood,
		input.InherentImpact, input.ResidualLikelihood, input.ResidualImpact,
		input.TargetLikelihood, input.TargetImpact, input.FinancialImpactEUR,
		input.ImpactCategories, input.Attachments, input.Metadata); err != nil {
		return err
	}
	if input.IdentifiedDate != nil && input.NextReviewDate != nil && input.NextReviewDate.Before(*input.IdentifiedDate) {
		return fmt.Errorf("%w: next_review_date precedes identified_date", ErrInvalidRisk)
	}
	return validateRiskIDs(input.RiskCategoryID, input.OwnerUserID, input.DelegateUserID,
		input.BusinessUnitID, input.RiskMatrixID, input.LinkedControlIDs)
}

func validateRisk(risk *models.Risk) error {
	if strings.TrimSpace(risk.Title) == "" || len(risk.Title) > 500 {
		return fmt.Errorf("%w: title is required and must not exceed 500 characters", ErrInvalidRisk)
	}
	if !allowed(risk.Status, riskStatuses()...) {
		return fmt.Errorf("%w: unsupported status", ErrInvalidRisk)
	}
	if err := validateRiskValues(risk.RiskSource, risk.RiskType, risk.RiskVelocity,
		risk.RiskProximity, risk.ReviewFrequency, risk.InherentLikelihood,
		risk.InherentImpact, risk.ResidualLikelihood, risk.ResidualImpact,
		risk.TargetLikelihood, risk.TargetImpact, risk.FinancialImpactEUR,
		risk.ImpactCategories, risk.Attachments, risk.Metadata); err != nil {
		return err
	}
	return validateRiskIDs(risk.RiskCategoryID, risk.OwnerUserID, risk.DelegateUserID,
		risk.BusinessUnitID, risk.RiskMatrixID, risk.LinkedControlIDs)
}

func validateRiskValues(source, riskType, velocity, proximity, frequency *string,
	il, ii, rl, ri, tl, ti *int, financial *float64, jsonValues ...json.RawMessage) error {
	if source != nil && !allowed(*source, "internal", "external", "third_party", "regulatory", "environmental", "emerging") {
		return fmt.Errorf("%w: unsupported risk_source", ErrInvalidRisk)
	}
	if riskType != nil && !allowed(*riskType, "threat", "vulnerability", "event", "consequence", "opportunity") {
		return fmt.Errorf("%w: unsupported risk_type", ErrInvalidRisk)
	}
	if velocity != nil && !allowed(*velocity, "immediate", "fast", "moderate", "slow") {
		return fmt.Errorf("%w: unsupported risk_velocity", ErrInvalidRisk)
	}
	if proximity != nil && !allowed(*proximity, "imminent", "short_term", "medium_term", "long_term") {
		return fmt.Errorf("%w: unsupported risk_proximity", ErrInvalidRisk)
	}
	if frequency != nil && !allowed(*frequency, "monthly", "quarterly", "semi_annually", "annually") {
		return fmt.Errorf("%w: unsupported review_frequency", ErrInvalidRisk)
	}
	for name, pair := range map[string][2]*int{"inherent": {il, ii}, "residual": {rl, ri}, "target": {tl, ti}} {
		if (pair[0] == nil) != (pair[1] == nil) {
			return fmt.Errorf("%w: %s likelihood and impact must be supplied together", ErrInvalidRisk, name)
		}
		for _, value := range pair {
			if value != nil && (*value < 1 || *value > 10) {
				return fmt.Errorf("%w: %s rating must be between 1 and 10", ErrInvalidRisk, name)
			}
		}
	}
	if financial != nil && *financial < 0 {
		return fmt.Errorf("%w: financial impact cannot be negative", ErrInvalidRisk)
	}
	for _, value := range jsonValues {
		if len(value) > 0 && !json.Valid(value) {
			return fmt.Errorf("%w: malformed JSON metadata", ErrInvalidRisk)
		}
	}
	return nil
}

func validateRiskIDs(category, owner, delegate, businessUnit, matrix *string, controls []string) error {
	for _, id := range []*string{category, owner, delegate, businessUnit, matrix} {
		if id != nil && *id != "" && !validUUID(*id) {
			return ErrInvalidRiskID
		}
	}
	for _, id := range controls {
		if !validUUID(id) {
			return ErrInvalidRiskID
		}
	}
	return nil
}

func normalizeRiskCreate(input *models.RiskCreateInput) {
	input.Title, input.RiskRef = strings.TrimSpace(input.Title), strings.TrimSpace(input.RiskRef)
	for _, id := range []**string{&input.RiskCategoryID, &input.OwnerUserID, &input.DelegateUserID, &input.BusinessUnitID, &input.RiskMatrixID} {
		normalizeOptionalID(id)
	}
	if input.ReviewFrequency == nil {
		value := "quarterly"
		input.ReviewFrequency = &value
	}
	if len(input.ImpactCategories) == 0 {
		input.ImpactCategories = json.RawMessage(`{}`)
	}
	if len(input.Attachments) == 0 {
		input.Attachments = json.RawMessage(`[]`)
	}
	if len(input.Metadata) == 0 {
		input.Metadata = json.RawMessage(`{}`)
	}
	if input.LinkedRegulations == nil {
		input.LinkedRegulations = []string{}
	}
	if input.LinkedControlIDs == nil {
		input.LinkedControlIDs = []string{}
	}
	if input.Tags == nil {
		input.Tags = []string{}
	}
}

func normalizeOptionalID(id **string) {
	if *id != nil && strings.TrimSpace(**id) == "" {
		*id = nil
	}
}

func applyRiskPatch(r *models.Risk, p models.RiskPatch) {
	if p.Title != nil {
		r.Title = strings.TrimSpace(*p.Title)
	}
	if p.Description != nil {
		r.Description = p.Description
	}
	if p.RiskCategoryID != nil {
		r.RiskCategoryID = nullableID(*p.RiskCategoryID)
	}
	if p.RiskSource != nil {
		r.RiskSource = nullableString(*p.RiskSource)
	}
	if p.RiskType != nil {
		r.RiskType = nullableString(*p.RiskType)
	}
	if p.Status != nil {
		r.Status = *p.Status
	}
	if p.OwnerUserID != nil {
		r.OwnerUserID = nullableID(*p.OwnerUserID)
	}
	if p.DelegateUserID != nil {
		r.DelegateUserID = nullableID(*p.DelegateUserID)
	}
	if p.BusinessUnitID != nil {
		r.BusinessUnitID = nullableID(*p.BusinessUnitID)
	}
	if p.RiskMatrixID != nil {
		r.RiskMatrixID = nullableID(*p.RiskMatrixID)
	}
	if p.InherentLikelihood != nil {
		r.InherentLikelihood = p.InherentLikelihood
	}
	if p.InherentImpact != nil {
		r.InherentImpact = p.InherentImpact
	}
	if p.ResidualLikelihood != nil {
		r.ResidualLikelihood = p.ResidualLikelihood
	}
	if p.ResidualImpact != nil {
		r.ResidualImpact = p.ResidualImpact
	}
	if p.TargetLikelihood != nil {
		r.TargetLikelihood = p.TargetLikelihood
	}
	if p.TargetImpact != nil {
		r.TargetImpact = p.TargetImpact
	}
	if p.FinancialImpactEUR != nil {
		r.FinancialImpactEUR = p.FinancialImpactEUR
	}
	if p.ImpactDescription != nil {
		r.ImpactDescription = p.ImpactDescription
	}
	if p.ImpactCategories != nil {
		r.ImpactCategories = p.ImpactCategories
	}
	if p.RiskVelocity != nil {
		r.RiskVelocity = nullableString(*p.RiskVelocity)
	}
	if p.RiskProximity != nil {
		r.RiskProximity = nullableString(*p.RiskProximity)
	}
	if p.NextReviewDate != nil {
		r.NextReviewDate = p.NextReviewDate
	}
	if p.ReviewFrequency != nil {
		r.ReviewFrequency = nullableString(*p.ReviewFrequency)
	}
	if p.LinkedRegulations != nil {
		r.LinkedRegulations = p.LinkedRegulations
	}
	if p.LinkedControlIDs != nil {
		r.LinkedControlIDs = p.LinkedControlIDs
	}
	if p.Tags != nil {
		r.Tags = p.Tags
	}
	if p.Attachments != nil {
		r.Attachments = p.Attachments
	}
	if p.IsEmerging != nil {
		r.IsEmerging = *p.IsEmerging
	}
	if p.Metadata != nil {
		r.Metadata = p.Metadata
	}
}

func nullableID(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func riskPatchEmpty(p models.RiskPatch) bool {
	return p.Title == nil && p.Description == nil && p.RiskCategoryID == nil && p.RiskSource == nil &&
		p.RiskType == nil && p.Status == nil && p.OwnerUserID == nil && p.DelegateUserID == nil &&
		p.BusinessUnitID == nil && p.RiskMatrixID == nil && p.InherentLikelihood == nil &&
		p.InherentImpact == nil && p.ResidualLikelihood == nil && p.ResidualImpact == nil &&
		p.TargetLikelihood == nil && p.TargetImpact == nil && p.FinancialImpactEUR == nil &&
		p.ImpactDescription == nil && p.ImpactCategories == nil && p.RiskVelocity == nil &&
		p.RiskProximity == nil && p.NextReviewDate == nil && p.ReviewFrequency == nil &&
		p.LinkedRegulations == nil && p.LinkedControlIDs == nil && p.Tags == nil &&
		p.Attachments == nil && p.IsEmerging == nil && p.Metadata == nil
}

func riskStatuses() []string {
	return []string{models.RiskStatusIdentified, models.RiskStatusAssessed, models.RiskStatusTreated,
		models.RiskStatusAccepted, models.RiskStatusClosed, models.RiskStatusMonitoring}
}

func validRiskStatusTransition(from, to string) bool {
	if from == to {
		return true
	}
	transitions := map[string]map[string]bool{
		"identified": {"assessed": true, "accepted": true, "closed": true},
		"assessed":   {"treated": true, "accepted": true, "monitoring": true, "closed": true},
		"treated":    {"monitoring": true, "accepted": true, "closed": true},
		"accepted":   {"monitoring": true, "closed": true},
		"monitoring": {"assessed": true, "treated": true, "accepted": true, "closed": true},
		"closed":     {},
	}
	return transitions[from][to]
}

func validateAssessment(input models.RiskAssessmentInput) error {
	if !allowed(input.AssessmentType, "initial", "periodic", "triggered", "post_incident", "ad_hoc") {
		return fmt.Errorf("%w: unsupported assessment_type", ErrInvalidAssessment)
	}
	if input.Methodology != nil && !allowed(*input.Methodology, "qualitative", "semi_quantitative", "quantitative", "monte_carlo", "bayesian", "bow_tie") {
		return fmt.Errorf("%w: unsupported methodology", ErrInvalidAssessment)
	}
	if input.ConfidenceLevel != nil && !allowed(*input.ConfidenceLevel, "low", "medium", "high", "very_high") {
		return fmt.Errorf("%w: unsupported confidence_level", ErrInvalidAssessment)
	}
	if input.LikelihoodAfter == nil || input.ImpactAfter == nil {
		return fmt.Errorf("%w: after likelihood and impact are required", ErrInvalidAssessment)
	}
	for name, pair := range map[string][2]*int{"before": {input.LikelihoodBefore, input.ImpactBefore}, "after": {input.LikelihoodAfter, input.ImpactAfter}} {
		if (pair[0] == nil) != (pair[1] == nil) {
			return fmt.Errorf("%w: %s likelihood and impact must be supplied together", ErrInvalidAssessment, name)
		}
		for _, value := range pair {
			if value != nil && (*value < 1 || *value > 10) {
				return fmt.Errorf("%w: ratings must be between 1 and 10", ErrInvalidAssessment)
			}
		}
	}
	return nil
}

func validateTreatmentInput(input models.RiskTreatmentInput) error {
	if !allowed(input.TreatmentType, "mitigate", "transfer", "avoid", "accept") {
		return fmt.Errorf("%w: unsupported treatment_type", ErrInvalidTreatment)
	}
	if strings.TrimSpace(input.Title) == "" || len(input.Title) > 500 {
		return fmt.Errorf("%w: title is required and must not exceed 500 characters", ErrInvalidTreatment)
	}
	if input.Priority != nil && !allowed(*input.Priority, "critical", "high", "medium", "low") {
		return fmt.Errorf("%w: unsupported priority", ErrInvalidTreatment)
	}
	if input.StartDate != nil && input.TargetDate != nil && input.TargetDate.Before(*input.StartDate) {
		return fmt.Errorf("%w: target_date precedes start_date", ErrInvalidTreatment)
	}
	if input.EstimatedCostEUR != nil && *input.EstimatedCostEUR < 0 {
		return fmt.Errorf("%w: estimated cost cannot be negative", ErrInvalidTreatment)
	}
	if input.ExpectedRiskReduction != nil && (*input.ExpectedRiskReduction < 0 || *input.ExpectedRiskReduction > 100) {
		return fmt.Errorf("%w: expected risk reduction must be 0-100", ErrInvalidTreatment)
	}
	if input.OwnerUserID != nil && !validUUID(*input.OwnerUserID) {
		return ErrInvalidRiskID
	}
	for _, id := range input.LinkedControlIDs {
		if !validUUID(id) {
			return ErrInvalidRiskID
		}
	}
	return nil
}

func validateTreatment(t *models.RiskTreatment) error {
	if t.Priority != nil && !allowed(*t.Priority, "critical", "high", "medium", "low") {
		return fmt.Errorf("%w: unsupported priority", ErrInvalidTreatment)
	}
	if t.ProgressPercentage < 0 || t.ProgressPercentage > 100 {
		return fmt.Errorf("%w: progress_percentage must be 0-100", ErrInvalidTreatment)
	}
	if t.StartDate != nil && t.TargetDate != nil && t.TargetDate.Before(*t.StartDate) {
		return fmt.Errorf("%w: target_date precedes start_date", ErrInvalidTreatment)
	}
	if t.ActualCostEUR != nil && *t.ActualCostEUR < 0 {
		return fmt.Errorf("%w: actual cost cannot be negative", ErrInvalidTreatment)
	}
	if t.OwnerUserID != nil && !validUUID(*t.OwnerUserID) {
		return ErrInvalidRiskID
	}
	return nil
}

func applyTreatmentPatch(t *models.RiskTreatment, p models.RiskTreatmentPatch) {
	if p.Status != nil {
		t.Status = *p.Status
	}
	if p.Priority != nil {
		t.Priority = nullableString(*p.Priority)
	}
	if p.OwnerUserID != nil {
		t.OwnerUserID = nullableID(*p.OwnerUserID)
	}
	if p.StartDate != nil {
		t.StartDate = p.StartDate
	}
	if p.TargetDate != nil {
		t.TargetDate = p.TargetDate
	}
	if p.ActualCostEUR != nil {
		t.ActualCostEUR = p.ActualCostEUR
	}
	if p.ProgressPercentage != nil {
		t.ProgressPercentage = *p.ProgressPercentage
	}
	if p.Notes != nil {
		t.Notes = p.Notes
	}
}

func treatmentPatchEmpty(p models.RiskTreatmentPatch) bool {
	return p.Status == nil && p.Priority == nil && p.OwnerUserID == nil && p.StartDate == nil &&
		p.TargetDate == nil && p.ActualCostEUR == nil && p.ProgressPercentage == nil && p.Notes == nil
}

func validTreatmentTransition(from, to string) bool {
	if from == to {
		return true
	}
	transitions := map[string]map[string]bool{
		"planned":     {"in_progress": true, "overdue": true, "cancelled": true},
		"in_progress": {"completed": true, "overdue": true, "cancelled": true},
		"overdue":     {"in_progress": true, "completed": true, "cancelled": true},
		"completed":   {}, "cancelled": {},
	}
	return transitions[from][to]
}

func validateAppetite(input models.RiskAppetiteInput) error {
	if !allowed(input.AppetiteLevel, "averse", "minimal", "cautious", "open", "hungry") {
		return fmt.Errorf("%w: unsupported appetite_level", ErrInvalidRiskAppetite)
	}
	if !allowed(input.ToleranceLevel, "zero", "low", "moderate", "high") {
		return fmt.Errorf("%w: unsupported tolerance_level", ErrInvalidRiskAppetite)
	}
	if !allowed(input.Status, "draft", "approved", "under_review") {
		return fmt.Errorf("%w: unsupported status", ErrInvalidRiskAppetite)
	}
	if input.QuantitativeThresholdLow != nil && input.QuantitativeThresholdHigh != nil && *input.QuantitativeThresholdHigh < *input.QuantitativeThresholdLow {
		return fmt.Errorf("%w: high threshold is below low threshold", ErrInvalidRiskAppetite)
	}
	return nil
}

func validateIndicator(input models.RiskIndicatorInput) error {
	if strings.TrimSpace(input.Name) == "" || len(input.Name) > 255 {
		return fmt.Errorf("%w: name is required and must not exceed 255 characters", ErrInvalidRiskIndicator)
	}
	if !allowed(input.MetricType, "count", "percentage", "currency", "duration", "ratio", "score", "boolean") {
		return fmt.Errorf("%w: unsupported metric_type", ErrInvalidRiskIndicator)
	}
	if !allowed(input.CollectionFrequency, "real_time", "daily", "weekly", "monthly", "quarterly") {
		return fmt.Errorf("%w: unsupported collection_frequency", ErrInvalidRiskIndicator)
	}
	if input.OwnerUserID != nil && !validUUID(*input.OwnerUserID) {
		return ErrInvalidRiskID
	}
	thresholds := []*float64{input.ThresholdGreen, input.ThresholdAmber, input.ThresholdRed}
	for _, value := range thresholds {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return fmt.Errorf("%w: thresholds must be finite", ErrInvalidRiskIndicator)
		}
	}
	if input.ThresholdGreen != nil && input.ThresholdAmber != nil && *input.ThresholdGreen > *input.ThresholdAmber ||
		input.ThresholdAmber != nil && input.ThresholdRed != nil && *input.ThresholdAmber > *input.ThresholdRed {
		return fmt.Errorf("%w: thresholds must be ordered green, amber, red", ErrInvalidRiskIndicator)
	}
	if !json.Valid(input.AutomationConfig) {
		return fmt.Errorf("%w: malformed automation_config", ErrInvalidRiskIndicator)
	}
	if input.IsAutomated && input.DataSource == nil {
		return fmt.Errorf("%w: automated indicator requires data_source", ErrInvalidRiskIndicator)
	}
	return nil
}

func normalizeRiskPagination(p models.PaginationRequest) models.PaginationRequest {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 || p.PageSize > 100 {
		p.PageSize = 20
	}
	return p
}
