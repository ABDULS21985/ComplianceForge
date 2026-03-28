package service

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

// ComplianceReport holds data for a compliance report export.
type ComplianceReport struct {
	GeneratedAt     time.Time           `json:"generated_at"`
	OrganizationID  string              `json:"organization_id"`
	ReportTitle     string              `json:"report_title"`
	Period          string              `json:"period"`
	OverallScore    float64             `json:"overall_score"`
	FrameworkScores []ComplianceScore   `json:"framework_scores"`
	ControlSummary  ControlSummary      `json:"control_summary"`
	Recommendations []string            `json:"recommendations"`
}

// ControlSummary provides aggregate control statistics for reporting.
type ControlSummary struct {
	Total              int `json:"total"`
	Compliant          int `json:"compliant"`
	NonCompliant       int `json:"non_compliant"`
	PartiallyCompliant int `json:"partially_compliant"`
	NotAssessed        int `json:"not_assessed"`
}

// RiskReport holds data for a risk report export.
type RiskReport struct {
	GeneratedAt    time.Time        `json:"generated_at"`
	OrganizationID string           `json:"organization_id"`
	ReportTitle    string           `json:"report_title"`
	RiskSummary    RiskSummary      `json:"risk_summary"`
	RisksByLevel   map[string]int   `json:"risks_by_level"`
	TopRisks       []models.Risk    `json:"top_risks"`
	MitigationProgress MitigationProgress `json:"mitigation_progress"`
}

// MitigationProgress provides aggregate mitigation statistics.
type MitigationProgress struct {
	Open        int `json:"open"`
	InProgress  int `json:"in_progress"`
	Mitigated   int `json:"mitigated"`
	Accepted    int `json:"accepted"`
	Transferred int `json:"transferred"`
}

// AuditReport holds data for an audit report export.
type AuditReport struct {
	GeneratedAt    time.Time             `json:"generated_at"`
	OrganizationID string                `json:"organization_id"`
	ReportTitle    string                `json:"report_title"`
	AuditID        string                `json:"audit_id"`
	AuditTitle     string                `json:"audit_title"`
	AuditType      models.AuditType      `json:"audit_type"`
	Status         models.AuditStatus    `json:"status"`
	Scope          string                `json:"scope"`
	StartDate      *time.Time            `json:"start_date"`
	EndDate        *time.Time            `json:"end_date"`
	Findings       []models.AuditFinding `json:"findings"`
	FindingSummary FindingSummary        `json:"finding_summary"`
}

// FindingSummary provides aggregate finding statistics for an audit.
type FindingSummary struct {
	Total      int `json:"total"`
	Open       int `json:"open"`
	InProgress int `json:"in_progress"`
	Resolved   int `json:"resolved"`
	Closed     int `json:"closed"`
	Accepted   int `json:"accepted"`
}

// ExecutiveSummary provides a high-level overview for executive stakeholders.
type ExecutiveSummary struct {
	GeneratedAt      time.Time         `json:"generated_at"`
	OrganizationID   string            `json:"organization_id"`
	ReportTitle      string            `json:"report_title"`
	Period           string            `json:"period"`
	OverallCompliance float64          `json:"overall_compliance"`
	FrameworkScores  []ComplianceScore `json:"framework_scores"`
	RiskSummary      RiskSummary       `json:"risk_summary"`
	AuditSummary     AuditSummaryStats `json:"audit_summary"`
	IncidentSummary  IncidentSummaryStats `json:"incident_summary"`
	KeyMetrics       []KeyMetric       `json:"key_metrics"`
	Recommendations  []string          `json:"recommendations"`
}

// AuditSummaryStats provides aggregate audit statistics for executive reporting.
type AuditSummaryStats struct {
	TotalAudits   int `json:"total_audits"`
	Completed     int `json:"completed"`
	InProgress    int `json:"in_progress"`
	Planned       int `json:"planned"`
	OpenFindings  int `json:"open_findings"`
}

// IncidentSummaryStats provides aggregate incident statistics for executive reporting.
type IncidentSummaryStats struct {
	TotalIncidents      int `json:"total_incidents"`
	Open                int `json:"open"`
	Resolved            int `json:"resolved"`
	BreachNotifiable    int `json:"breach_notifiable"`
	AverageResolutionHours float64 `json:"average_resolution_hours"`
}

// KeyMetric represents a key performance indicator for the executive summary.
type KeyMetric struct {
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	Trend       string  `json:"trend"` // "up", "down", "stable"
	Description string  `json:"description"`
}

// ReportingService generates structured reports from aggregated data
// across multiple repositories.
type ReportingService struct {
	complianceEngine *ComplianceEngine
	riskRepo         RiskRepository
	auditRepo        AuditRepository
	incidentRepo     IncidentRepository
	frameworkRepo    FrameworkRepository
	controlRepo      ControlRepository
	logger           zerolog.Logger
}

// NewReportingService constructs a new ReportingService.
func NewReportingService(
	complianceEngine *ComplianceEngine,
	riskRepo RiskRepository,
	auditRepo AuditRepository,
	incidentRepo IncidentRepository,
	frameworkRepo FrameworkRepository,
	controlRepo ControlRepository,
	logger zerolog.Logger,
) *ReportingService {
	return &ReportingService{
		complianceEngine: complianceEngine,
		riskRepo:         riskRepo,
		auditRepo:        auditRepo,
		incidentRepo:     incidentRepo,
		frameworkRepo:    frameworkRepo,
		controlRepo:      controlRepo,
		logger:           logger.With().Str("service", "reporting").Logger(),
	}
}

// GenerateComplianceReport produces a compliance report for an organization,
// aggregating scores across all active frameworks.
func (s *ReportingService) GenerateComplianceReport(ctx context.Context, orgID string) (*ComplianceReport, error) {
	dashboard, err := s.complianceEngine.GetComplianceDashboard(ctx, orgID)
	if err != nil {
		s.logger.Error().Err(err).Str("org_id", orgID).Msg("failed to generate compliance report")
		return nil, err
	}

	// Aggregate control summary across all frameworks.
	var summary ControlSummary
	for _, score := range dashboard.FrameworkScores {
		summary.Total += score.TotalControls
		summary.Compliant += score.CompliantControls
		summary.NonCompliant += score.NonCompliant
		summary.PartiallyCompliant += score.PartiallyCompliant
		summary.NotAssessed += score.NotAssessed
	}

	// Generate recommendations based on compliance posture.
	var recommendations []string
	if dashboard.OverallScore < 50 {
		recommendations = append(recommendations, "Overall compliance score is below 50% - immediate remediation plan required")
	}
	for _, fs := range dashboard.FrameworkScores {
		if fs.NonCompliant > fs.TotalControls/4 {
			recommendations = append(recommendations, "Framework '"+fs.FrameworkName+"' has over 25% non-compliant controls - prioritize remediation")
		}
		if fs.NotAssessed > fs.TotalControls/3 {
			recommendations = append(recommendations, "Framework '"+fs.FrameworkName+"' has over 33% unassessed controls - schedule assessment")
		}
	}

	report := &ComplianceReport{
		GeneratedAt:     time.Now(),
		OrganizationID:  orgID,
		ReportTitle:     "Compliance Status Report",
		Period:          time.Now().Format("January 2006"),
		OverallScore:    dashboard.OverallScore,
		FrameworkScores: dashboard.FrameworkScores,
		ControlSummary:  summary,
		Recommendations: recommendations,
	}

	s.logger.Info().Str("org_id", orgID).Float64("score", dashboard.OverallScore).Msg("compliance report generated")
	return report, nil
}

// GenerateRiskReport produces a risk report for an organization.
func (s *ReportingService) GenerateRiskReport(ctx context.Context, orgID string) (*RiskReport, error) {
	riskCounts, err := s.riskRepo.CountByRiskLevel(ctx, orgID)
	if err != nil {
		s.logger.Error().Err(err).Str("org_id", orgID).Msg("failed to count risks by level")
		return nil, err
	}

	topRisks, err := s.riskRepo.GetTopRisks(ctx, orgID, 10)
	if err != nil {
		s.logger.Error().Err(err).Str("org_id", orgID).Msg("failed to get top risks")
		return nil, err
	}

	risksByLevel := make(map[string]int)
	for level, count := range riskCounts {
		risksByLevel[string(level)] = count
	}

	summary := RiskSummary{
		CriticalRisk: riskCounts[models.RiskLevelCritical],
		HighRisk:     riskCounts[models.RiskLevelHigh],
		MediumRisk:   riskCounts[models.RiskLevelMedium],
		LowRisk:      riskCounts[models.RiskLevelLow] + riskCounts[models.RiskLevelVeryLow],
	}
	summary.TotalRisks = summary.CriticalRisk + summary.HighRisk + summary.MediumRisk + summary.LowRisk

	// TODO: Compute MitigationProgress from risk mitigation status counts.
	mitigation := MitigationProgress{}

	report := &RiskReport{
		GeneratedAt:        time.Now(),
		OrganizationID:     orgID,
		ReportTitle:        "Risk Assessment Report",
		RiskSummary:        summary,
		RisksByLevel:       risksByLevel,
		TopRisks:           topRisks,
		MitigationProgress: mitigation,
	}

	s.logger.Info().Str("org_id", orgID).Int("total_risks", summary.TotalRisks).Msg("risk report generated")
	return report, nil
}

// GenerateAuditReport produces a detailed report for a specific audit engagement.
func (s *ReportingService) GenerateAuditReport(ctx context.Context, auditID string) (*AuditReport, error) {
	audit, err := s.auditRepo.GetByID(ctx, auditID)
	if err != nil {
		s.logger.Error().Err(err).Str("audit_id", auditID).Msg("audit not found for report")
		return nil, ErrAuditNotFound
	}

	findings, _, err := s.auditRepo.ListFindings(ctx, auditID, 1, 1000)
	if err != nil {
		s.logger.Error().Err(err).Str("audit_id", auditID).Msg("failed to list findings for report")
		return nil, err
	}

	// Compute finding summary.
	var summary FindingSummary
	summary.Total = len(findings)
	for _, f := range findings {
		switch f.Status {
		case models.FindingStatusOpen:
			summary.Open++
		case models.FindingStatusInProgress:
			summary.InProgress++
		case models.FindingStatusResolved:
			summary.Resolved++
		case models.FindingStatusClosed:
			summary.Closed++
		case models.FindingStatusAccepted:
			summary.Accepted++
		}
	}

	report := &AuditReport{
		GeneratedAt:    time.Now(),
		OrganizationID: audit.OrganizationID,
		ReportTitle:    "Audit Report: " + audit.Title,
		AuditID:        audit.ID,
		AuditTitle:     audit.Title,
		AuditType:      audit.Type,
		Status:         audit.Status,
		Scope:          audit.Scope,
		StartDate:      audit.ActualStartDate,
		EndDate:        audit.ActualEndDate,
		Findings:       findings,
		FindingSummary: summary,
	}

	s.logger.Info().Str("audit_id", auditID).Int("findings", summary.Total).Msg("audit report generated")
	return report, nil
}

// GenerateExecutiveSummary produces a high-level report combining compliance,
// risk, audit, and incident data for executive stakeholders.
func (s *ReportingService) GenerateExecutiveSummary(ctx context.Context, orgID string) (*ExecutiveSummary, error) {
	// Compliance data.
	dashboard, err := s.complianceEngine.GetComplianceDashboard(ctx, orgID)
	if err != nil {
		s.logger.Warn().Err(err).Msg("failed to get compliance dashboard for executive summary")
		dashboard = &ComplianceDashboard{}
	}

	// Risk data.
	riskCounts, err := s.riskRepo.CountByRiskLevel(ctx, orgID)
	if err != nil {
		s.logger.Warn().Err(err).Msg("failed to get risk counts for executive summary")
		riskCounts = make(map[models.RiskLevel]int)
	}

	riskSummary := RiskSummary{
		CriticalRisk: riskCounts[models.RiskLevelCritical],
		HighRisk:     riskCounts[models.RiskLevelHigh],
		MediumRisk:   riskCounts[models.RiskLevelMedium],
		LowRisk:      riskCounts[models.RiskLevelLow] + riskCounts[models.RiskLevelVeryLow],
	}
	riskSummary.TotalRisks = riskSummary.CriticalRisk + riskSummary.HighRisk + riskSummary.MediumRisk + riskSummary.LowRisk

	// Audit data.
	audits, _, err := s.auditRepo.List(ctx, orgID, 1, 1000)
	if err != nil {
		s.logger.Warn().Err(err).Msg("failed to get audits for executive summary")
	}

	auditSummary := AuditSummaryStats{TotalAudits: len(audits)}
	for _, a := range audits {
		switch a.Status {
		case models.AuditStatusCompleted:
			auditSummary.Completed++
		case models.AuditStatusInProgress:
			auditSummary.InProgress++
		case models.AuditStatusPlanned:
			auditSummary.Planned++
		}
	}
	// TODO: Count open findings across all audits for auditSummary.OpenFindings.

	// Incident data.
	incidents, _, err := s.incidentRepo.List(ctx, orgID, 1, 1000)
	if err != nil {
		s.logger.Warn().Err(err).Msg("failed to get incidents for executive summary")
	}

	incidentSummary := IncidentSummaryStats{TotalIncidents: len(incidents)}
	var totalResolutionHours float64
	resolvedCount := 0
	for _, inc := range incidents {
		switch inc.Status {
		case models.IncidentStatusOpen, models.IncidentStatusInvestigating, models.IncidentStatusContained:
			incidentSummary.Open++
		case models.IncidentStatusResolved, models.IncidentStatusClosed:
			incidentSummary.Resolved++
			if inc.DetectedAt != nil && inc.ResolvedAt != nil {
				totalResolutionHours += inc.ResolvedAt.Sub(*inc.DetectedAt).Hours()
				resolvedCount++
			}
		}
		if inc.IsBreachNotifiable {
			incidentSummary.BreachNotifiable++
		}
	}
	if resolvedCount > 0 {
		incidentSummary.AverageResolutionHours = totalResolutionHours / float64(resolvedCount)
	}

	// Build key metrics.
	metrics := []KeyMetric{
		{
			Name:        "Overall Compliance",
			Value:       dashboard.OverallScore,
			Unit:        "%",
			Trend:       "stable",
			Description: "Weighted average compliance score across all active frameworks",
		},
		{
			Name:        "Open Risks",
			Value:       float64(riskSummary.TotalRisks),
			Unit:        "count",
			Trend:       "stable",
			Description: "Total number of identified risks in the risk register",
		},
		{
			Name:        "Critical Risks",
			Value:       float64(riskSummary.CriticalRisk),
			Unit:        "count",
			Trend:       "stable",
			Description: "Number of risks rated as critical severity",
		},
		{
			Name:        "Open Incidents",
			Value:       float64(incidentSummary.Open),
			Unit:        "count",
			Trend:       "stable",
			Description: "Number of unresolved security incidents",
		},
	}

	// Generate executive recommendations.
	var recommendations []string
	if dashboard.OverallScore < 70 {
		recommendations = append(recommendations, "Overall compliance score is below 70% - executive attention required")
	}
	if riskSummary.CriticalRisk > 0 {
		recommendations = append(recommendations, "Critical risks identified - ensure mitigation plans are in place and tracked")
	}
	if incidentSummary.BreachNotifiable > 0 {
		recommendations = append(recommendations, "Breach-notifiable incidents exist - verify GDPR notification obligations are met")
	}

	summary := &ExecutiveSummary{
		GeneratedAt:       time.Now(),
		OrganizationID:    orgID,
		ReportTitle:       "Executive Summary Report",
		Period:            time.Now().Format("January 2006"),
		OverallCompliance: dashboard.OverallScore,
		FrameworkScores:   dashboard.FrameworkScores,
		RiskSummary:       riskSummary,
		AuditSummary:      auditSummary,
		IncidentSummary:   incidentSummary,
		KeyMetrics:        metrics,
		Recommendations:   recommendations,
	}

	s.logger.Info().Str("org_id", orgID).Msg("executive summary report generated")
	return summary, nil
}
