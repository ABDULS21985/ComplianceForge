package service

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrComplianceCalculation = errors.New("failed to calculate compliance score")
)

// ComplianceScore represents the calculated compliance posture for a single framework.
type ComplianceScore struct {
	FrameworkID        string    `json:"framework_id"`
	FrameworkName      string    `json:"framework_name"`
	TotalControls      int       `json:"total_controls"`
	CompliantControls  int       `json:"compliant_controls"`
	NonCompliant       int       `json:"non_compliant"`
	PartiallyCompliant int       `json:"partially_compliant"`
	NotAssessed        int       `json:"not_assessed"`
	ScorePercentage    float64   `json:"score_percentage"`
	LastCalculated     time.Time `json:"last_calculated"`
}

// RiskSummary provides an aggregate view of risk levels.
type RiskSummary struct {
	TotalRisks   int `json:"total_risks"`
	CriticalRisk int `json:"critical_risk"`
	HighRisk     int `json:"high_risk"`
	MediumRisk   int `json:"medium_risk"`
	LowRisk      int `json:"low_risk"`
}

// ComplianceDashboard aggregates compliance, risk, and operational data for executive view.
type ComplianceDashboard struct {
	OverallScore    float64           `json:"overall_score"`
	FrameworkScores []ComplianceScore `json:"framework_scores"`
	RiskSummary     RiskSummary       `json:"risk_summary"`
	TopRisks        []models.Risk     `json:"top_risks"`
	RecentIncidents []models.Incident `json:"recent_incidents"`
	UpcomingAudits  []models.Audit    `json:"upcoming_audits"`
}

// ControlMapping represents a mapping between controls across two frameworks.
type ControlMapping struct {
	SourceControl     models.Control `json:"source_control"`
	TargetControl     models.Control `json:"target_control"`
	MappingConfidence float64        `json:"mapping_confidence"` // 0.0 to 1.0
}

// RiskRepository defines the data access interface for risks.
type RiskRepository interface {
	Create(ctx context.Context, risk *models.Risk) error
	GetByID(ctx context.Context, id string) (*models.Risk, error)
	Update(ctx context.Context, risk *models.Risk) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, orgID string, page, pageSize int) ([]models.Risk, int, error)
	ListByRiskLevel(ctx context.Context, orgID string, level models.RiskLevel) ([]models.Risk, error)
	GetTopRisks(ctx context.Context, orgID string, limit int) ([]models.Risk, error)
	CountByRiskLevel(ctx context.Context, orgID string) (map[models.RiskLevel]int, error)
}

// ComplianceEngine is the core service that calculates compliance scores,
// builds dashboards, and maps controls across frameworks.
type ComplianceEngine struct {
	controlRepo   ControlRepository
	frameworkRepo FrameworkRepository
	riskRepo      RiskRepository
	logger        zerolog.Logger
}

// NewComplianceEngine constructs a new ComplianceEngine.
func NewComplianceEngine(
	controlRepo ControlRepository,
	frameworkRepo FrameworkRepository,
	riskRepo RiskRepository,
	logger zerolog.Logger,
) *ComplianceEngine {
	return &ComplianceEngine{
		controlRepo:   controlRepo,
		frameworkRepo: frameworkRepo,
		riskRepo:      riskRepo,
		logger:        logger.With().Str("service", "compliance_engine").Logger(),
	}
}

// CalculateComplianceScore computes the compliance posture for a specific
// framework within an organization by aggregating control statuses.
func (e *ComplianceEngine) CalculateComplianceScore(ctx context.Context, orgID, frameworkID string) (*ComplianceScore, error) {
	framework, err := e.frameworkRepo.GetByID(ctx, frameworkID)
	if err != nil {
		e.logger.Error().Err(err).Str("framework_id", frameworkID).Msg("framework not found for score calculation")
		return nil, ErrFrameworkNotFound
	}

	statusCounts, err := e.controlRepo.CountByStatus(ctx, frameworkID)
	if err != nil {
		e.logger.Error().Err(err).Str("framework_id", frameworkID).Msg("failed to count control statuses")
		return nil, ErrComplianceCalculation
	}

	compliant := statusCounts[models.ComplianceStatusCompliant]
	nonCompliant := statusCounts[models.ComplianceStatusNonCompliant]
	partial := statusCounts[models.ComplianceStatusPartiallyCompliant]
	notAssessed := statusCounts[models.ComplianceStatusNotAssessed]
	total := compliant + nonCompliant + partial + notAssessed

	var scorePercentage float64
	if total > 0 {
		// Fully compliant controls count 100%, partially compliant count 50%.
		scorePercentage = (float64(compliant) + float64(partial)*0.5) / float64(total) * 100
	}

	score := &ComplianceScore{
		FrameworkID:        framework.ID,
		FrameworkName:      framework.Name,
		TotalControls:      total,
		CompliantControls:  compliant,
		NonCompliant:       nonCompliant,
		PartiallyCompliant: partial,
		NotAssessed:        notAssessed,
		ScorePercentage:    scorePercentage,
		LastCalculated:     time.Now(),
	}

	e.logger.Info().
		Str("org_id", orgID).
		Str("framework_id", frameworkID).
		Float64("score", scorePercentage).
		Msg("compliance score calculated")

	return score, nil
}

// GetComplianceDashboard builds a comprehensive dashboard combining compliance
// scores across all active frameworks, risk summary, top risks, and recent activity.
func (e *ComplianceEngine) GetComplianceDashboard(ctx context.Context, orgID string) (*ComplianceDashboard, error) {
	// Fetch all frameworks for the org.
	frameworks, _, err := e.frameworkRepo.List(ctx, orgID, 1, 100)
	if err != nil {
		e.logger.Error().Err(err).Str("org_id", orgID).Msg("failed to list frameworks for dashboard")
		return nil, err
	}

	var frameworkScores []ComplianceScore
	var totalScore float64
	scoredCount := 0

	for _, fw := range frameworks {
		if !fw.IsActive {
			continue
		}
		score, err := e.CalculateComplianceScore(ctx, orgID, fw.ID)
		if err != nil {
			e.logger.Warn().Err(err).Str("framework_id", fw.ID).Msg("skipping framework in dashboard")
			continue
		}
		frameworkScores = append(frameworkScores, *score)
		totalScore += score.ScorePercentage
		scoredCount++
	}

	var overallScore float64
	if scoredCount > 0 {
		overallScore = totalScore / float64(scoredCount)
	}

	// Build risk summary.
	riskCounts, err := e.riskRepo.CountByRiskLevel(ctx, orgID)
	if err != nil {
		e.logger.Warn().Err(err).Str("org_id", orgID).Msg("failed to get risk counts for dashboard")
		riskCounts = make(map[models.RiskLevel]int)
	}

	riskSummary := RiskSummary{
		CriticalRisk: riskCounts[models.RiskLevelCritical],
		HighRisk:     riskCounts[models.RiskLevelHigh],
		MediumRisk:   riskCounts[models.RiskLevelMedium],
		LowRisk:      riskCounts[models.RiskLevelLow] + riskCounts[models.RiskLevelVeryLow],
	}
	riskSummary.TotalRisks = riskSummary.CriticalRisk + riskSummary.HighRisk + riskSummary.MediumRisk + riskSummary.LowRisk

	topRisks, err := e.riskRepo.GetTopRisks(ctx, orgID, 5)
	if err != nil {
		e.logger.Warn().Err(err).Msg("failed to get top risks for dashboard")
		topRisks = nil
	}

	dashboard := &ComplianceDashboard{
		OverallScore:    overallScore,
		FrameworkScores: frameworkScores,
		RiskSummary:     riskSummary,
		TopRisks:        topRisks,
		// RecentIncidents and UpcomingAudits would need their own repositories.
		// They are left nil here; the handler layer can populate them separately
		// or this service can be extended with IncidentRepository and AuditRepository.
		RecentIncidents: nil,
		UpcomingAudits:  nil,
	}

	e.logger.Info().
		Str("org_id", orgID).
		Float64("overall_score", overallScore).
		Int("framework_count", scoredCount).
		Msg("compliance dashboard generated")

	return dashboard, nil
}

// MapControlsAcrossFrameworks identifies potential mappings between controls
// in a source framework and a target framework based on category and title similarity.
func (e *ComplianceEngine) MapControlsAcrossFrameworks(ctx context.Context, orgID, sourceFrameworkID, targetFrameworkID string) ([]ControlMapping, error) {
	sourceControls, err := e.controlRepo.ListByFrameworkID(ctx, sourceFrameworkID)
	if err != nil {
		e.logger.Error().Err(err).Str("source_framework_id", sourceFrameworkID).Msg("failed to load source controls")
		return nil, err
	}

	targetControls, err := e.controlRepo.ListByFrameworkID(ctx, targetFrameworkID)
	if err != nil {
		e.logger.Error().Err(err).Str("target_framework_id", targetFrameworkID).Msg("failed to load target controls")
		return nil, err
	}

	var mappings []ControlMapping

	for _, src := range sourceControls {
		bestMatch := models.Control{}
		bestConfidence := 0.0

		for _, tgt := range targetControls {
			confidence := calculateMappingConfidence(src, tgt)
			if confidence > bestConfidence {
				bestConfidence = confidence
				bestMatch = tgt
			}
		}

		// Only include mappings with a minimum confidence threshold.
		if bestConfidence >= 0.3 {
			mappings = append(mappings, ControlMapping{
				SourceControl:     src,
				TargetControl:     bestMatch,
				MappingConfidence: bestConfidence,
			})
		}
	}

	e.logger.Info().
		Str("org_id", orgID).
		Str("source_framework_id", sourceFrameworkID).
		Str("target_framework_id", targetFrameworkID).
		Int("mappings_found", len(mappings)).
		Msg("cross-framework control mapping completed")

	return mappings, nil
}

// calculateMappingConfidence computes a confidence score (0.0-1.0) for how well
// two controls map to each other based on category, title, and code similarity.
func calculateMappingConfidence(source, target models.Control) float64 {
	var score float64

	// Exact category match contributes 0.4.
	if source.Category != "" && source.Category == target.Category {
		score += 0.4
	}

	// Simple word overlap in title contributes up to 0.4.
	sourceWords := tokenize(source.Title)
	targetWords := tokenize(target.Title)
	if len(sourceWords) > 0 && len(targetWords) > 0 {
		overlap := wordOverlap(sourceWords, targetWords)
		maxLen := len(sourceWords)
		if len(targetWords) > maxLen {
			maxLen = len(targetWords)
		}
		score += 0.4 * (float64(overlap) / float64(maxLen))
	}

	// Priority match contributes 0.2.
	if source.Priority != "" && source.Priority == target.Priority {
		score += 0.2
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// tokenize splits a string into lowercase word tokens.
func tokenize(s string) []string {
	var words []string
	current := ""
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			if r >= 'A' && r <= 'Z' {
				r = r + 32 // toLower
			}
			current += string(r)
		} else {
			if current != "" {
				words = append(words, current)
				current = ""
			}
		}
	}
	if current != "" {
		words = append(words, current)
	}
	return words
}

// wordOverlap counts how many words from a appear in b.
func wordOverlap(a, b []string) int {
	set := make(map[string]struct{}, len(b))
	for _, w := range b {
		set[w] = struct{}{}
	}
	count := 0
	for _, w := range a {
		if _, ok := set[w]; ok {
			count++
		}
	}
	return count
}
