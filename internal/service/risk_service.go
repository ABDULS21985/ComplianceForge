package service

import (
	"context"
	"errors"
	"math"

	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrRiskNotFound         = errors.New("risk not found")
	ErrInvalidRiskParameter = errors.New("likelihood and impact must be between 1 and 5")
)

// RiskMatrixCell represents a single cell in the risk assessment matrix.
type RiskMatrixCell struct {
	Likelihood int             `json:"likelihood"`
	Impact     int             `json:"impact"`
	Score      float64         `json:"score"`
	Level      models.RiskLevel `json:"level"`
}

// RiskHeatmapEntry represents a risk plotted on the heatmap.
type RiskHeatmapEntry struct {
	RiskID     string          `json:"risk_id"`
	Title      string          `json:"title"`
	Likelihood int             `json:"likelihood"`
	Impact     int             `json:"impact"`
	Score      float64         `json:"score"`
	Level      models.RiskLevel `json:"level"`
}

// RiskService handles business logic for risk management.
type RiskService struct {
	riskRepo RiskRepository
	logger   zerolog.Logger
}

// NewRiskService constructs a new RiskService.
func NewRiskService(riskRepo RiskRepository, logger zerolog.Logger) *RiskService {
	return &RiskService{
		riskRepo: riskRepo,
		logger:   logger.With().Str("service", "risk").Logger(),
	}
}

// Create persists a new risk with a calculated risk score and level.
func (s *RiskService) Create(ctx context.Context, risk *models.Risk) error {
	if risk.Likelihood < 1 || risk.Likelihood > 5 || risk.Impact < 1 || risk.Impact > 5 {
		return ErrInvalidRiskParameter
	}

	score, level := s.CalculateRiskScore(risk.Likelihood, risk.Impact)
	risk.RiskScore = score
	risk.RiskLevel = models.RiskLevel(level)

	if risk.MitigationStatus == "" {
		risk.MitigationStatus = models.MitigationStatusOpen
	}

	if err := s.riskRepo.Create(ctx, risk); err != nil {
		s.logger.Error().Err(err).Str("title", risk.Title).Msg("failed to create risk")
		return err
	}

	s.logger.Info().
		Str("risk_id", risk.ID).
		Str("level", level).
		Float64("score", score).
		Msg("risk created")
	return nil
}

// GetByID retrieves a risk by its unique identifier.
func (s *RiskService) GetByID(ctx context.Context, id string) (*models.Risk, error) {
	risk, err := s.riskRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrRiskNotFound
	}
	return risk, nil
}

// Update modifies an existing risk and recalculates its score.
func (s *RiskService) Update(ctx context.Context, risk *models.Risk) error {
	if _, err := s.riskRepo.GetByID(ctx, risk.ID); err != nil {
		return ErrRiskNotFound
	}

	if risk.Likelihood >= 1 && risk.Likelihood <= 5 && risk.Impact >= 1 && risk.Impact <= 5 {
		score, level := s.CalculateRiskScore(risk.Likelihood, risk.Impact)
		risk.RiskScore = score
		risk.RiskLevel = models.RiskLevel(level)
	}

	if err := s.riskRepo.Update(ctx, risk); err != nil {
		s.logger.Error().Err(err).Str("risk_id", risk.ID).Msg("failed to update risk")
		return err
	}

	s.logger.Info().Str("risk_id", risk.ID).Msg("risk updated")
	return nil
}

// Delete soft-deletes a risk by ID.
func (s *RiskService) Delete(ctx context.Context, id string) error {
	if _, err := s.riskRepo.GetByID(ctx, id); err != nil {
		return ErrRiskNotFound
	}

	if err := s.riskRepo.Delete(ctx, id); err != nil {
		s.logger.Error().Err(err).Str("risk_id", id).Msg("failed to delete risk")
		return err
	}

	s.logger.Info().Str("risk_id", id).Msg("risk deleted")
	return nil
}

// List returns a paginated list of risks for an organization.
func (s *RiskService) List(ctx context.Context, orgID string, page, pageSize int) ([]models.Risk, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	risks, total, err := s.riskRepo.List(ctx, orgID, page, pageSize)
	if err != nil {
		s.logger.Error().Err(err).Str("org_id", orgID).Msg("failed to list risks")
		return nil, 0, err
	}
	return risks, total, nil
}

// CalculateRiskScore computes the risk score from likelihood and impact (1-5 each)
// and returns both the numeric score and the textual risk level.
// Score = likelihood * impact, normalized to 0-25 range.
func (s *RiskService) CalculateRiskScore(likelihood, impact int) (float64, string) {
	score := float64(likelihood * impact)
	score = math.Round(score*100) / 100

	var level string
	switch {
	case score >= 20:
		level = string(models.RiskLevelCritical)
	case score >= 15:
		level = string(models.RiskLevelHigh)
	case score >= 9:
		level = string(models.RiskLevelMedium)
	case score >= 4:
		level = string(models.RiskLevelLow)
	default:
		level = string(models.RiskLevelVeryLow)
	}

	return score, level
}

// GetRiskMatrix returns the full 5x5 risk assessment matrix with pre-computed scores and levels.
func (s *RiskService) GetRiskMatrix() [][]RiskMatrixCell {
	matrix := make([][]RiskMatrixCell, 5)
	for i := 0; i < 5; i++ {
		matrix[i] = make([]RiskMatrixCell, 5)
		for j := 0; j < 5; j++ {
			likelihood := i + 1
			impact := j + 1
			score, level := s.CalculateRiskScore(likelihood, impact)
			matrix[i][j] = RiskMatrixCell{
				Likelihood: likelihood,
				Impact:     impact,
				Score:      score,
				Level:      models.RiskLevel(level),
			}
		}
	}
	return matrix
}

// GetRiskHeatmap returns all risks for an organization plotted on a heatmap
// with their likelihood, impact, score, and level.
func (s *RiskService) GetRiskHeatmap(ctx context.Context, orgID string) ([]RiskHeatmapEntry, error) {
	risks, _, err := s.riskRepo.List(ctx, orgID, 1, 1000) // Fetch all risks for heatmap.
	if err != nil {
		s.logger.Error().Err(err).Str("org_id", orgID).Msg("failed to fetch risks for heatmap")
		return nil, err
	}

	entries := make([]RiskHeatmapEntry, 0, len(risks))
	for _, r := range risks {
		entries = append(entries, RiskHeatmapEntry{
			RiskID:     r.ID,
			Title:      r.Title,
			Likelihood: r.Likelihood,
			Impact:     r.Impact,
			Score:      r.RiskScore,
			Level:      r.RiskLevel,
		})
	}

	return entries, nil
}
