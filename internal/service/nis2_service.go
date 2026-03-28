package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// NIS2Service manages NIS2 Directive (2022/2555) compliance.
type NIS2Service struct {
	pool *pgxpool.Pool
	bus  *EventBus
}

// NIS2EntityAssessment holds the NIS2 scoping assessment for an organization.
type NIS2EntityAssessment struct {
	ID             string  `json:"id"`
	OrgID          string  `json:"organization_id"`
	EntityType     string  `json:"entity_type"`
	Sector         string  `json:"sector"`
	SubSector      string  `json:"sub_sector"`
	IsInScope      bool    `json:"is_in_scope"`
	EmployeeCount  int     `json:"employee_count"`
	AnnualTurnover float64 `json:"annual_turnover_eur"`
	MemberState    string  `json:"member_state"`
	CompetentAuth  string  `json:"competent_authority"`
	CSIRTName      string  `json:"csirt_name"`
}

// NIS2IncidentReport represents the three-phase NIS2 Article 23 report.
type NIS2IncidentReport struct {
	ID                      string  `json:"id"`
	IncidentID              string  `json:"incident_id"`
	ReportRef               string  `json:"report_ref"`
	EarlyWarningStatus      string  `json:"early_warning_status"`
	EarlyWarningDeadline    string  `json:"early_warning_deadline"`
	EarlyWarningSubmittedAt *string `json:"early_warning_submitted_at"`
	NotificationStatus      string  `json:"notification_status"`
	NotificationDeadline    string  `json:"notification_deadline"`
	NotificationSubmittedAt *string `json:"notification_submitted_at"`
	FinalReportStatus       string  `json:"final_report_status"`
	FinalReportDeadline     string  `json:"final_report_deadline"`
	FinalReportSubmittedAt  *string `json:"final_report_submitted_at"`
}

// NIS2SecurityMeasure tracks an Article 21 security measure.
type NIS2SecurityMeasure struct {
	ID                   string   `json:"id"`
	MeasureCode          string   `json:"measure_code"`
	MeasureTitle         string   `json:"measure_title"`
	ArticleReference     string   `json:"article_reference"`
	ImplementationStatus string   `json:"implementation_status"`
	OwnerUserID          *string  `json:"owner_user_id"`
	LinkedControlIDs     []string `json:"linked_control_ids"`
}

// NIS2ManagementRecord tracks Article 20 management accountability.
type NIS2ManagementRecord struct {
	ID                    string  `json:"id"`
	OrgID                 string  `json:"organization_id"`
	BoardMemberName       string  `json:"board_member_name"`
	BoardMemberRole       string  `json:"board_member_role"`
	TrainingCompleted     bool    `json:"training_completed"`
	TrainingDate          *string `json:"training_date"`
	TrainingProvider      *string `json:"training_provider"`
	RiskMeasuresApproved  bool    `json:"risk_measures_approved"`
	ApprovalDate          *string `json:"approval_date"`
	NextTrainingDue       *string `json:"next_training_due"`
}

// NIS2Dashboard provides an overview of NIS2 compliance status.
type NIS2Dashboard struct {
	EntityType                string `json:"entity_type"`
	IsInScope                 bool   `json:"is_in_scope"`
	MeasuresTotal             int    `json:"measures_total"`
	MeasuresImplemented       int    `json:"measures_implemented"`
	MeasuresInProgress        int    `json:"measures_in_progress"`
	IncidentReportsTotal      int    `json:"incident_reports_total"`
	EarlyWarningsOverdue      int    `json:"early_warnings_overdue"`
	NotificationsOverdue      int    `json:"notifications_overdue"`
	FinalReportsOverdue       int    `json:"final_reports_overdue"`
	ManagementTrainingCurrent int    `json:"management_training_current"`
	ManagementTrainingOverdue int    `json:"management_training_overdue"`
}

// NewNIS2Service creates a new NIS2Service.
func NewNIS2Service(pool *pgxpool.Pool, bus *EventBus) *NIS2Service {
	return &NIS2Service{pool: pool, bus: bus}
}

// GetEntityAssessment retrieves the NIS2 entity assessment for an organization.
func (s *NIS2Service) GetEntityAssessment(ctx context.Context, orgID string) (*NIS2EntityAssessment, error) {
	var a NIS2EntityAssessment
	var subSector, memberState, competentAuth, csirtName *string
	var employeeCount *int
	var annualTurnover *float64

	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, entity_type, sector, sub_sector, is_in_scope,
		       employee_count, annual_turnover_eur, member_state,
		       competent_authority, csirt_name
		FROM nis2_entity_assessment
		WHERE organization_id = $1`, orgID,
	).Scan(
		&a.ID, &a.OrgID, &a.EntityType, &a.Sector, &subSector, &a.IsInScope,
		&employeeCount, &annualTurnover, &memberState,
		&competentAuth, &csirtName,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("NIS2 entity assessment not found for this organization")
		}
		return nil, fmt.Errorf("get NIS2 entity assessment: %w", err)
	}

	if subSector != nil {
		a.SubSector = *subSector
	}
	if employeeCount != nil {
		a.EmployeeCount = *employeeCount
	}
	if annualTurnover != nil {
		a.AnnualTurnover = *annualTurnover
	}
	if memberState != nil {
		a.MemberState = *memberState
	}
	if competentAuth != nil {
		a.CompetentAuth = *competentAuth
	}
	if csirtName != nil {
		a.CSIRTName = *csirtName
	}

	return &a, nil
}

// CreateEntityAssessment creates or updates the NIS2 entity assessment.
func (s *NIS2Service) CreateEntityAssessment(ctx context.Context, orgID string, assessment NIS2EntityAssessment) (*NIS2EntityAssessment, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO nis2_entity_assessment (
			organization_id, entity_type, sector, sub_sector, is_in_scope,
			employee_count, annual_turnover_eur, member_state,
			competent_authority, csirt_name
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT ON CONSTRAINT uq_nis2_entity_org
		DO UPDATE SET
			entity_type = EXCLUDED.entity_type,
			sector = EXCLUDED.sector,
			sub_sector = EXCLUDED.sub_sector,
			is_in_scope = EXCLUDED.is_in_scope,
			employee_count = EXCLUDED.employee_count,
			annual_turnover_eur = EXCLUDED.annual_turnover_eur,
			member_state = EXCLUDED.member_state,
			competent_authority = EXCLUDED.competent_authority,
			csirt_name = EXCLUDED.csirt_name
		RETURNING id`,
		orgID, assessment.EntityType, assessment.Sector, assessment.SubSector,
		assessment.IsInScope, assessment.EmployeeCount, assessment.AnnualTurnover,
		assessment.MemberState, assessment.CompetentAuth, assessment.CSIRTName,
	).Scan(&assessment.ID)
	if err != nil {
		return nil, fmt.Errorf("upsert NIS2 entity assessment: %w", err)
	}

	assessment.OrgID = orgID

	s.bus.Publish(Event{
		Type:       "nis2.assessment_updated",
		Severity:   "low",
		OrgID:      orgID,
		EntityType: "nis2_entity_assessment",
		EntityID:   assessment.ID,
		Data: map[string]interface{}{
			"entity_type": assessment.EntityType,
			"is_in_scope": assessment.IsInScope,
			"sector":      assessment.Sector,
		},
		Timestamp: time.Now(),
	})

	log.Info().
		Str("org_id", orgID).
		Str("entity_type", assessment.EntityType).
		Bool("in_scope", assessment.IsInScope).
		Msg("NIS2 entity assessment created/updated")

	return &assessment, nil
}

// CreateIncidentReport creates a three-phase NIS2 incident report with deadlines
// calculated from the incident's detected_at timestamp.
func (s *NIS2Service) CreateIncidentReport(ctx context.Context, orgID, incidentID string) (*NIS2IncidentReport, error) {
	// Get the incident's detected_at for deadline calculation.
	var detectedAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(detected_at, created_at) FROM incidents
		WHERE id = $1 AND organization_id = $2`,
		incidentID, orgID).Scan(&detectedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("incident not found")
		}
		return nil, fmt.Errorf("get incident detected_at: %w", err)
	}

	earlyWarningDeadline := detectedAt.Add(24 * time.Hour)
	notificationDeadline := detectedAt.Add(72 * time.Hour)
	finalReportDeadline := detectedAt.AddDate(0, 1, 0) // +1 month

	var report NIS2IncidentReport
	err = s.pool.QueryRow(ctx, `
		INSERT INTO nis2_incident_reports (
			organization_id, incident_id,
			early_warning_deadline, notification_deadline, final_report_deadline
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING id, incident_id, report_ref,
		          early_warning_status, early_warning_deadline,
		          notification_status, notification_deadline,
		          final_report_status, final_report_deadline`,
		orgID, incidentID,
		earlyWarningDeadline, notificationDeadline, finalReportDeadline,
	).Scan(
		&report.ID, &report.IncidentID, &report.ReportRef,
		&report.EarlyWarningStatus, &report.EarlyWarningDeadline,
		&report.NotificationStatus, &report.NotificationDeadline,
		&report.FinalReportStatus, &report.FinalReportDeadline,
	)
	if err != nil {
		return nil, fmt.Errorf("insert NIS2 incident report: %w", err)
	}

	// Format deadlines.
	report.EarlyWarningDeadline = earlyWarningDeadline.Format(time.RFC3339)
	report.NotificationDeadline = notificationDeadline.Format(time.RFC3339)
	report.FinalReportDeadline = finalReportDeadline.Format(time.RFC3339)

	s.bus.Publish(Event{
		Type:       "nis2.incident_report_created",
		Severity:   "critical",
		OrgID:      orgID,
		EntityType: "nis2_incident_report",
		EntityID:   report.ID,
		EntityRef:  report.ReportRef,
		Data: map[string]interface{}{
			"incident_id":            incidentID,
			"early_warning_deadline": earlyWarningDeadline.Format(time.RFC3339),
		},
		Timestamp: time.Now(),
	})

	log.Info().
		Str("report_id", report.ID).
		Str("ref", report.ReportRef).
		Str("incident_id", incidentID).
		Msg("NIS2 incident report created with 3-phase deadlines")

	return &report, nil
}

// SubmitEarlyWarning marks the early warning phase as submitted.
func (s *NIS2Service) SubmitEarlyWarning(ctx context.Context, orgID, reportID string, content map[string]interface{}, submittedBy string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE nis2_incident_reports
		SET early_warning_status = 'submitted',
		    early_warning_submitted_at = NOW(),
		    early_warning_submitted_by = $1,
		    early_warning_content = $2
		WHERE id = $3 AND organization_id = $4
		  AND early_warning_status IN ('pending', 'overdue')`,
		submittedBy, content, reportID, orgID)
	if err != nil {
		return fmt.Errorf("submit early warning: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("NIS2 report not found or early warning already submitted")
	}

	s.bus.Publish(Event{
		Type:       "nis2.early_warning_submitted",
		Severity:   "high",
		OrgID:      orgID,
		EntityType: "nis2_incident_report",
		EntityID:   reportID,
		Timestamp:  time.Now(),
	})

	log.Info().Str("report_id", reportID).Msg("NIS2 early warning submitted")
	return nil
}

// SubmitNotification marks the incident notification phase as submitted.
func (s *NIS2Service) SubmitNotification(ctx context.Context, orgID, reportID string, content map[string]interface{}, submittedBy string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE nis2_incident_reports
		SET notification_status = 'submitted',
		    notification_submitted_at = NOW(),
		    notification_submitted_by = $1,
		    notification_content = $2
		WHERE id = $3 AND organization_id = $4
		  AND notification_status IN ('pending', 'overdue')`,
		submittedBy, content, reportID, orgID)
	if err != nil {
		return fmt.Errorf("submit notification: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("NIS2 report not found or notification already submitted")
	}

	log.Info().Str("report_id", reportID).Msg("NIS2 incident notification submitted")
	return nil
}

// SubmitFinalReport marks the final report phase as submitted.
func (s *NIS2Service) SubmitFinalReport(ctx context.Context, orgID, reportID string, content map[string]interface{}, documentPath, submittedBy string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE nis2_incident_reports
		SET final_report_status = 'submitted',
		    final_report_submitted_at = NOW(),
		    final_report_submitted_by = $1,
		    final_report_content = $2,
		    final_report_document_path = $3
		WHERE id = $4 AND organization_id = $5
		  AND final_report_status IN ('pending', 'overdue')`,
		submittedBy, content, documentPath, reportID, orgID)
	if err != nil {
		return fmt.Errorf("submit final report: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("NIS2 report not found or final report already submitted")
	}

	s.bus.Publish(Event{
		Type:       "nis2.final_report_submitted",
		Severity:   "medium",
		OrgID:      orgID,
		EntityType: "nis2_incident_report",
		EntityID:   reportID,
		Timestamp:  time.Now(),
	})

	log.Info().Str("report_id", reportID).Msg("NIS2 final report submitted")
	return nil
}

// GetSecurityMeasures returns all Article 21 security measures for an organization.
func (s *NIS2Service) GetSecurityMeasures(ctx context.Context, orgID string) ([]NIS2SecurityMeasure, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, measure_code, measure_title, article_reference,
		       implementation_status, owner_user_id, linked_control_ids
		FROM nis2_security_measures
		WHERE organization_id = $1
		ORDER BY measure_code`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query NIS2 security measures: %w", err)
	}
	defer rows.Close()

	var measures []NIS2SecurityMeasure
	for rows.Next() {
		var m NIS2SecurityMeasure
		var linkedIDs []string
		if err := rows.Scan(&m.ID, &m.MeasureCode, &m.MeasureTitle, &m.ArticleReference,
			&m.ImplementationStatus, &m.OwnerUserID, &linkedIDs); err != nil {
			return nil, fmt.Errorf("scan NIS2 measure: %w", err)
		}
		m.LinkedControlIDs = linkedIDs
		if m.LinkedControlIDs == nil {
			m.LinkedControlIDs = []string{}
		}
		measures = append(measures, m)
	}

	return measures, nil
}

// UpdateSecurityMeasure updates the implementation status, owner, and evidence
// for a specific NIS2 security measure.
func (s *NIS2Service) UpdateSecurityMeasure(ctx context.Context, orgID, measureID, status, ownerID, evidence string) error {
	var ownerPtr *string
	if ownerID != "" {
		ownerPtr = &ownerID
	}
	var evidencePtr *string
	if evidence != "" {
		evidencePtr = &evidence
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE nis2_security_measures
		SET implementation_status = $1,
		    owner_user_id = $2,
		    evidence_description = $3,
		    last_assessed_at = NOW()
		WHERE id = $4 AND organization_id = $5`,
		status, ownerPtr, evidencePtr, measureID, orgID)
	if err != nil {
		return fmt.Errorf("update NIS2 security measure: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("NIS2 security measure not found")
	}

	log.Info().
		Str("measure_id", measureID).
		Str("status", status).
		Msg("NIS2 security measure updated")
	return nil
}

// GetManagementAccountability returns all Article 20 management records.
func (s *NIS2Service) GetManagementAccountability(ctx context.Context, orgID string) ([]NIS2ManagementRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, board_member_name, board_member_role,
		       training_completed, training_date, training_provider,
		       risk_measures_approved, approval_date, next_training_due
		FROM nis2_management_accountability
		WHERE organization_id = $1
		ORDER BY board_member_name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query NIS2 management accountability: %w", err)
	}
	defer rows.Close()

	var records []NIS2ManagementRecord
	for rows.Next() {
		var r NIS2ManagementRecord
		var trainingDate, approvalDate, nextDue *time.Time
		if err := rows.Scan(&r.ID, &r.OrgID, &r.BoardMemberName, &r.BoardMemberRole,
			&r.TrainingCompleted, &trainingDate, &r.TrainingProvider,
			&r.RiskMeasuresApproved, &approvalDate, &nextDue); err != nil {
			return nil, fmt.Errorf("scan NIS2 management record: %w", err)
		}
		if trainingDate != nil {
			td := trainingDate.Format("2006-01-02")
			r.TrainingDate = &td
		}
		if approvalDate != nil {
			ad := approvalDate.Format("2006-01-02")
			r.ApprovalDate = &ad
		}
		if nextDue != nil {
			nd := nextDue.Format("2006-01-02")
			r.NextTrainingDue = &nd
		}
		records = append(records, r)
	}

	return records, nil
}

// RecordManagementTraining creates or updates a management accountability record.
type RecordTrainingInput struct {
	BoardMemberName     string `json:"board_member_name"`
	BoardMemberRole     string `json:"board_member_role"`
	TrainingProvider    string `json:"training_provider"`
	TrainingDate        string `json:"training_date"`
	CertificatePath     string `json:"certificate_path"`
	NextTrainingDue     string `json:"next_training_due"`
}

func (s *NIS2Service) RecordManagementTraining(ctx context.Context, orgID string, record RecordTrainingInput) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO nis2_management_accountability (
			organization_id, board_member_name, board_member_role,
			training_completed, training_date, training_provider,
			training_certificate_path, next_training_due
		) VALUES ($1, $2, $3, true, $4, $5, $6, $7)
		ON CONFLICT (organization_id, board_member_name, board_member_role)
		DO UPDATE SET
			training_completed = true,
			training_date = EXCLUDED.training_date,
			training_provider = EXCLUDED.training_provider,
			training_certificate_path = EXCLUDED.training_certificate_path,
			next_training_due = EXCLUDED.next_training_due`,
		orgID, record.BoardMemberName, record.BoardMemberRole,
		record.TrainingDate, record.TrainingProvider,
		record.CertificatePath, record.NextTrainingDue,
	)
	if err != nil {
		return fmt.Errorf("record management training: %w", err)
	}

	log.Info().
		Str("org_id", orgID).
		Str("member", record.BoardMemberName).
		Msg("NIS2 management training recorded")
	return nil
}

// GetDashboard returns a NIS2 compliance dashboard for the organization.
func (s *NIS2Service) GetDashboard(ctx context.Context, orgID string) (*NIS2Dashboard, error) {
	d := &NIS2Dashboard{}

	// Entity assessment info.
	_ = s.pool.QueryRow(ctx, `
		SELECT entity_type, is_in_scope FROM nis2_entity_assessment
		WHERE organization_id = $1`, orgID).Scan(&d.EntityType, &d.IsInScope)

	// Security measures summary.
	_ = s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE implementation_status IN ('implemented', 'verified')),
			COUNT(*) FILTER (WHERE implementation_status = 'in_progress')
		FROM nis2_security_measures
		WHERE organization_id = $1`, orgID).
		Scan(&d.MeasuresTotal, &d.MeasuresImplemented, &d.MeasuresInProgress)

	// Incident reports summary.
	_ = s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE early_warning_status = 'overdue'
			                    OR (early_warning_status = 'pending' AND early_warning_deadline < NOW())),
			COUNT(*) FILTER (WHERE notification_status = 'overdue'
			                    OR (notification_status = 'pending' AND notification_deadline < NOW())),
			COUNT(*) FILTER (WHERE final_report_status = 'overdue'
			                    OR (final_report_status = 'pending' AND final_report_deadline < NOW()))
		FROM nis2_incident_reports
		WHERE organization_id = $1`, orgID).
		Scan(&d.IncidentReportsTotal, &d.EarlyWarningsOverdue, &d.NotificationsOverdue, &d.FinalReportsOverdue)

	// Management training summary.
	_ = s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE training_completed = true
			                    AND (next_training_due IS NULL OR next_training_due >= CURRENT_DATE)),
			COUNT(*) FILTER (WHERE training_completed = false
			                    OR (next_training_due IS NOT NULL AND next_training_due < CURRENT_DATE))
		FROM nis2_management_accountability
		WHERE organization_id = $1`, orgID).
		Scan(&d.ManagementTrainingCurrent, &d.ManagementTrainingOverdue)

	return d, nil
}
