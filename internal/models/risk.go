package models

import (
	"encoding/json"
	"time"
)

const (
	RiskStatusIdentified = "identified"
	RiskStatusAssessed   = "assessed"
	RiskStatusTreated    = "treated"
	RiskStatusAccepted   = "accepted"
	RiskStatusClosed     = "closed"
	RiskStatusMonitoring = "monitoring"
)

// Risk is the canonical risk-register record introduced by migration 000010.
// Scores and levels are database-calculated from their likelihood/impact pair.
type Risk struct {
	TenantModel
	RiskRef            string          `json:"risk_ref"`
	Title              string          `json:"title"`
	Description        *string         `json:"description,omitempty"`
	RiskCategoryID     *string         `json:"risk_category_id,omitempty"`
	RiskSource         *string         `json:"risk_source,omitempty"`
	RiskType           *string         `json:"risk_type,omitempty"`
	Status             string          `json:"status"`
	OwnerUserID        *string         `json:"owner_user_id,omitempty"`
	DelegateUserID     *string         `json:"delegate_user_id,omitempty"`
	BusinessUnitID     *string         `json:"business_unit_id,omitempty"`
	RiskMatrixID       *string         `json:"risk_matrix_id,omitempty"`
	InherentLikelihood *int            `json:"inherent_likelihood,omitempty"`
	InherentImpact     *int            `json:"inherent_impact,omitempty"`
	InherentRiskScore  *float64        `json:"inherent_risk_score,omitempty"`
	InherentRiskLevel  *string         `json:"inherent_risk_level,omitempty"`
	ResidualLikelihood *int            `json:"residual_likelihood,omitempty"`
	ResidualImpact     *int            `json:"residual_impact,omitempty"`
	ResidualRiskScore  *float64        `json:"residual_risk_score,omitempty"`
	ResidualRiskLevel  *string         `json:"residual_risk_level,omitempty"`
	TargetLikelihood   *int            `json:"target_likelihood,omitempty"`
	TargetImpact       *int            `json:"target_impact,omitempty"`
	TargetRiskScore    *float64        `json:"target_risk_score,omitempty"`
	TargetRiskLevel    *string         `json:"target_risk_level,omitempty"`
	FinancialImpactEUR *float64        `json:"financial_impact_eur,omitempty"`
	ImpactDescription  *string         `json:"impact_description,omitempty"`
	ImpactCategories   json.RawMessage `json:"impact_categories"`
	RiskVelocity       *string         `json:"risk_velocity,omitempty"`
	RiskProximity      *string         `json:"risk_proximity,omitempty"`
	IdentifiedDate     time.Time       `json:"identified_date"`
	LastAssessedDate   *time.Time      `json:"last_assessed_date,omitempty"`
	NextReviewDate     *time.Time      `json:"next_review_date,omitempty"`
	ReviewFrequency    *string         `json:"review_frequency,omitempty"`
	LinkedRegulations  []string        `json:"linked_regulations"`
	LinkedControlIDs   []string        `json:"linked_control_ids"`
	Tags               []string        `json:"tags"`
	Attachments        json.RawMessage `json:"attachments"`
	IsEmerging         bool            `json:"is_emerging"`
	Metadata           json.RawMessage `json:"metadata"`
}

type RiskCreateInput struct {
	RiskRef            string          `json:"risk_ref,omitempty"`
	Title              string          `json:"title"`
	Description        *string         `json:"description,omitempty"`
	RiskCategoryID     *string         `json:"risk_category_id,omitempty"`
	RiskSource         *string         `json:"risk_source,omitempty"`
	RiskType           *string         `json:"risk_type,omitempty"`
	OwnerUserID        *string         `json:"owner_user_id,omitempty"`
	DelegateUserID     *string         `json:"delegate_user_id,omitempty"`
	BusinessUnitID     *string         `json:"business_unit_id,omitempty"`
	RiskMatrixID       *string         `json:"risk_matrix_id,omitempty"`
	InherentLikelihood *int            `json:"inherent_likelihood,omitempty"`
	InherentImpact     *int            `json:"inherent_impact,omitempty"`
	ResidualLikelihood *int            `json:"residual_likelihood,omitempty"`
	ResidualImpact     *int            `json:"residual_impact,omitempty"`
	TargetLikelihood   *int            `json:"target_likelihood,omitempty"`
	TargetImpact       *int            `json:"target_impact,omitempty"`
	FinancialImpactEUR *float64        `json:"financial_impact_eur,omitempty"`
	ImpactDescription  *string         `json:"impact_description,omitempty"`
	ImpactCategories   json.RawMessage `json:"impact_categories,omitempty"`
	RiskVelocity       *string         `json:"risk_velocity,omitempty"`
	RiskProximity      *string         `json:"risk_proximity,omitempty"`
	IdentifiedDate     *time.Time      `json:"identified_date,omitempty"`
	NextReviewDate     *time.Time      `json:"next_review_date,omitempty"`
	ReviewFrequency    *string         `json:"review_frequency,omitempty"`
	LinkedRegulations  []string        `json:"linked_regulations,omitempty"`
	LinkedControlIDs   []string        `json:"linked_control_ids,omitempty"`
	Tags               []string        `json:"tags,omitempty"`
	Attachments        json.RawMessage `json:"attachments,omitempty"`
	IsEmerging         bool            `json:"is_emerging,omitempty"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
}

// RiskPatch has PATCH semantics. Optional relationship IDs accept an empty
// string to clear the relationship.
type RiskPatch struct {
	Title              *string         `json:"title,omitempty"`
	Description        *string         `json:"description,omitempty"`
	RiskCategoryID     *string         `json:"risk_category_id,omitempty"`
	RiskSource         *string         `json:"risk_source,omitempty"`
	RiskType           *string         `json:"risk_type,omitempty"`
	Status             *string         `json:"status,omitempty"`
	OwnerUserID        *string         `json:"owner_user_id,omitempty"`
	DelegateUserID     *string         `json:"delegate_user_id,omitempty"`
	BusinessUnitID     *string         `json:"business_unit_id,omitempty"`
	RiskMatrixID       *string         `json:"risk_matrix_id,omitempty"`
	InherentLikelihood *int            `json:"inherent_likelihood,omitempty"`
	InherentImpact     *int            `json:"inherent_impact,omitempty"`
	ResidualLikelihood *int            `json:"residual_likelihood,omitempty"`
	ResidualImpact     *int            `json:"residual_impact,omitempty"`
	TargetLikelihood   *int            `json:"target_likelihood,omitempty"`
	TargetImpact       *int            `json:"target_impact,omitempty"`
	FinancialImpactEUR *float64        `json:"financial_impact_eur,omitempty"`
	ImpactDescription  *string         `json:"impact_description,omitempty"`
	ImpactCategories   json.RawMessage `json:"impact_categories,omitempty"`
	RiskVelocity       *string         `json:"risk_velocity,omitempty"`
	RiskProximity      *string         `json:"risk_proximity,omitempty"`
	NextReviewDate     *time.Time      `json:"next_review_date,omitempty"`
	ReviewFrequency    *string         `json:"review_frequency,omitempty"`
	LinkedRegulations  []string        `json:"linked_regulations,omitempty"`
	LinkedControlIDs   []string        `json:"linked_control_ids,omitempty"`
	Tags               []string        `json:"tags,omitempty"`
	Attachments        json.RawMessage `json:"attachments,omitempty"`
	IsEmerging         *bool           `json:"is_emerging,omitempty"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
}

type RiskListFilter struct {
	PaginationRequest
	Status, Level, CategoryID, OwnerUserID, Search string
	Emerging                                       *bool
}

type RiskCategory struct {
	BaseModel
	OrganizationID   *string `json:"organization_id,omitempty"`
	Name             string  `json:"name"`
	Code             string  `json:"code"`
	Description      *string `json:"description,omitempty"`
	ParentCategoryID *string `json:"parent_category_id,omitempty"`
	ColorHex         *string `json:"color_hex,omitempty"`
	Icon             *string `json:"icon,omitempty"`
	SortOrder        int     `json:"sort_order"`
	IsSystemDefault  bool    `json:"is_system_default"`
}

type RiskAssessment struct {
	ID               string    `json:"id"`
	CreatedAt        time.Time `json:"created_at"`
	OrganizationID   string    `json:"organization_id"`
	RiskID           string    `json:"risk_id"`
	AssessmentType   string    `json:"assessment_type"`
	AssessorUserID   *string   `json:"assessor_user_id,omitempty"`
	AssessmentDate   time.Time `json:"assessment_date"`
	LikelihoodBefore *int      `json:"likelihood_before,omitempty"`
	ImpactBefore     *int      `json:"impact_before,omitempty"`
	ScoreBefore      *float64  `json:"score_before,omitempty"`
	LevelBefore      *string   `json:"level_before,omitempty"`
	LikelihoodAfter  *int      `json:"likelihood_after,omitempty"`
	ImpactAfter      *int      `json:"impact_after,omitempty"`
	ScoreAfter       *float64  `json:"score_after,omitempty"`
	LevelAfter       *string   `json:"level_after,omitempty"`
	AssessmentNotes  *string   `json:"assessment_notes,omitempty"`
	Methodology      *string   `json:"methodology,omitempty"`
	ConfidenceLevel  *string   `json:"confidence_level,omitempty"`
	DataSources      []string  `json:"data_sources"`
}

type RiskAssessmentInput struct {
	AssessmentType   string     `json:"assessment_type"`
	AssessmentDate   *time.Time `json:"assessment_date,omitempty"`
	LikelihoodBefore *int       `json:"likelihood_before,omitempty"`
	ImpactBefore     *int       `json:"impact_before,omitempty"`
	LikelihoodAfter  *int       `json:"likelihood_after,omitempty"`
	ImpactAfter      *int       `json:"impact_after,omitempty"`
	AssessmentNotes  *string    `json:"assessment_notes,omitempty"`
	Methodology      *string    `json:"methodology,omitempty"`
	ConfidenceLevel  *string    `json:"confidence_level,omitempty"`
	DataSources      []string   `json:"data_sources,omitempty"`
}

type RiskTreatment struct {
	TenantModel
	RiskID                string     `json:"risk_id"`
	TreatmentType         string     `json:"treatment_type"`
	Title                 string     `json:"title"`
	Description           *string    `json:"description,omitempty"`
	Status                string     `json:"status"`
	Priority              *string    `json:"priority,omitempty"`
	OwnerUserID           *string    `json:"owner_user_id,omitempty"`
	StartDate             *time.Time `json:"start_date,omitempty"`
	TargetDate            *time.Time `json:"target_date,omitempty"`
	CompletedDate         *time.Time `json:"completed_date,omitempty"`
	EstimatedCostEUR      *float64   `json:"estimated_cost_eur,omitempty"`
	ActualCostEUR         *float64   `json:"actual_cost_eur,omitempty"`
	ExpectedRiskReduction *float64   `json:"expected_risk_reduction,omitempty"`
	ProgressPercentage    int        `json:"progress_percentage"`
	LinkedControlIDs      []string   `json:"linked_control_ids"`
	Notes                 *string    `json:"notes,omitempty"`
}

type RiskTreatmentInput struct {
	TreatmentType         string     `json:"treatment_type"`
	Title                 string     `json:"title"`
	Description           *string    `json:"description,omitempty"`
	Priority              *string    `json:"priority,omitempty"`
	OwnerUserID           *string    `json:"owner_user_id,omitempty"`
	StartDate             *time.Time `json:"start_date,omitempty"`
	TargetDate            *time.Time `json:"target_date,omitempty"`
	EstimatedCostEUR      *float64   `json:"estimated_cost_eur,omitempty"`
	ExpectedRiskReduction *float64   `json:"expected_risk_reduction,omitempty"`
	LinkedControlIDs      []string   `json:"linked_control_ids,omitempty"`
	Notes                 *string    `json:"notes,omitempty"`
}

type RiskTreatmentPatch struct {
	Status             *string    `json:"status,omitempty"`
	Priority           *string    `json:"priority,omitempty"`
	OwnerUserID        *string    `json:"owner_user_id,omitempty"`
	StartDate          *time.Time `json:"start_date,omitempty"`
	TargetDate         *time.Time `json:"target_date,omitempty"`
	ActualCostEUR      *float64   `json:"actual_cost_eur,omitempty"`
	ProgressPercentage *int       `json:"progress_percentage,omitempty"`
	Notes              *string    `json:"notes,omitempty"`
}

type RiskAppetiteStatement struct {
	TenantModel
	RiskCategoryID            string     `json:"risk_category_id"`
	AppetiteLevel             string     `json:"appetite_level"`
	AppetiteDescription       *string    `json:"appetite_description,omitempty"`
	QuantitativeThresholdLow  *float64   `json:"quantitative_threshold_low,omitempty"`
	QuantitativeThresholdHigh *float64   `json:"quantitative_threshold_high,omitempty"`
	ThresholdMetric           *string    `json:"threshold_metric,omitempty"`
	ToleranceLevel            string     `json:"tolerance_level"`
	ApprovedBy                *string    `json:"approved_by,omitempty"`
	ApprovedAt                *time.Time `json:"approved_at,omitempty"`
	ReviewDate                *time.Time `json:"review_date,omitempty"`
	Status                    string     `json:"status"`
}

type RiskAppetiteInput struct {
	AppetiteLevel             string     `json:"appetite_level"`
	AppetiteDescription       *string    `json:"appetite_description,omitempty"`
	QuantitativeThresholdLow  *float64   `json:"quantitative_threshold_low,omitempty"`
	QuantitativeThresholdHigh *float64   `json:"quantitative_threshold_high,omitempty"`
	ThresholdMetric           *string    `json:"threshold_metric,omitempty"`
	ToleranceLevel            string     `json:"tolerance_level"`
	ReviewDate                *time.Time `json:"review_date,omitempty"`
	Status                    string     `json:"status"`
}

type RiskIndicator struct {
	TenantModel
	RiskID              *string         `json:"risk_id,omitempty"`
	Name                string          `json:"name"`
	Description         *string         `json:"description,omitempty"`
	MetricType          string          `json:"metric_type"`
	MeasurementUnit     *string         `json:"measurement_unit,omitempty"`
	CollectionFrequency string          `json:"collection_frequency"`
	DataSource          *string         `json:"data_source,omitempty"`
	ThresholdGreen      *float64        `json:"threshold_green,omitempty"`
	ThresholdAmber      *float64        `json:"threshold_amber,omitempty"`
	ThresholdRed        *float64        `json:"threshold_red,omitempty"`
	CurrentValue        *float64        `json:"current_value,omitempty"`
	Trend               *string         `json:"trend,omitempty"`
	OwnerUserID         *string         `json:"owner_user_id,omitempty"`
	LastUpdatedAt       *time.Time      `json:"last_updated_at,omitempty"`
	IsAutomated         bool            `json:"is_automated"`
	AutomationConfig    json.RawMessage `json:"automation_config"`
}

type RiskIndicatorInput struct {
	Name                string          `json:"name"`
	Description         *string         `json:"description,omitempty"`
	MetricType          string          `json:"metric_type"`
	MeasurementUnit     *string         `json:"measurement_unit,omitempty"`
	CollectionFrequency string          `json:"collection_frequency,omitempty"`
	DataSource          *string         `json:"data_source,omitempty"`
	ThresholdGreen      *float64        `json:"threshold_green,omitempty"`
	ThresholdAmber      *float64        `json:"threshold_amber,omitempty"`
	ThresholdRed        *float64        `json:"threshold_red,omitempty"`
	OwnerUserID         *string         `json:"owner_user_id,omitempty"`
	IsAutomated         bool            `json:"is_automated,omitempty"`
	AutomationConfig    json.RawMessage `json:"automation_config,omitempty"`
}

type RiskIndicatorValue struct {
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	IndicatorID    string    `json:"indicator_id"`
	OrganizationID string    `json:"organization_id"`
	Value          float64   `json:"value"`
	Status         string    `json:"status"`
	MeasuredAt     time.Time `json:"measured_at"`
	MeasuredBy     *string   `json:"measured_by,omitempty"`
	Notes          *string   `json:"notes,omitempty"`
}

type RiskIndicatorValueInput struct {
	Value      float64    `json:"value"`
	MeasuredAt *time.Time `json:"measured_at,omitempty"`
	Notes      *string    `json:"notes,omitempty"`
}

type RiskMatrix struct {
	BaseModel
	OrganizationID  string          `json:"organization_id"`
	Name            string          `json:"name"`
	Description     *string         `json:"description,omitempty"`
	LikelihoodScale json.RawMessage `json:"likelihood_scale"`
	ImpactScale     json.RawMessage `json:"impact_scale"`
	RiskLevels      json.RawMessage `json:"risk_levels"`
	MatrixSize      int             `json:"matrix_size"`
	IsDefault       bool            `json:"is_default"`
}

type RiskMatrixCell struct {
	Likelihood int `json:"likelihood"`
	Impact     int `json:"impact"`
	Count      int `json:"count"`
}

type RiskMatrixView struct {
	Dimension     string           `json:"dimension"`
	Configuration *RiskMatrix      `json:"configuration,omitempty"`
	Cells         []RiskMatrixCell `json:"cells"`
}

type RiskHeatmapEntry struct {
	RiskID             string   `json:"risk_id"`
	RiskRef            string   `json:"risk_ref"`
	Title              string   `json:"title"`
	Status             string   `json:"status"`
	InherentLikelihood *int     `json:"inherent_likelihood,omitempty"`
	InherentImpact     *int     `json:"inherent_impact,omitempty"`
	InherentRiskScore  *float64 `json:"inherent_risk_score,omitempty"`
	InherentRiskLevel  *string  `json:"inherent_risk_level,omitempty"`
	ResidualLikelihood *int     `json:"residual_likelihood,omitempty"`
	ResidualImpact     *int     `json:"residual_impact,omitempty"`
	ResidualRiskScore  *float64 `json:"residual_risk_score,omitempty"`
	ResidualRiskLevel  *string  `json:"residual_risk_level,omitempty"`
	TargetLikelihood   *int     `json:"target_likelihood,omitempty"`
	TargetImpact       *int     `json:"target_impact,omitempty"`
	TargetRiskScore    *float64 `json:"target_risk_score,omitempty"`
	TargetRiskLevel    *string  `json:"target_risk_level,omitempty"`
	OwnerUserID        *string  `json:"owner_user_id,omitempty"`
	IsEmerging         bool     `json:"is_emerging"`
}
