package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/service"
)

// RegulatoryScheduler checks for regulatory deadlines and emits notification events.
// It is designed to be called every 15 minutes by the background worker.
type RegulatoryScheduler struct {
	pool *pgxpool.Pool
	bus  *service.EventBus
}

// NewRegulatoryScheduler creates a new RegulatoryScheduler.
func NewRegulatoryScheduler(pool *pgxpool.Pool, bus *service.EventBus) *RegulatoryScheduler {
	return &RegulatoryScheduler{
		pool: pool,
		bus:  bus,
	}
}

// Run executes all regulatory deadline checks. Each check queries the database
// and emits appropriate Event objects to the EventBus when deadlines are approaching.
func (rs *RegulatoryScheduler) Run(ctx context.Context) error {
	log.Info().Msg("regulatory scheduler: starting deadline checks")

	checks := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"GDPR breach deadlines", rs.CheckGDPRBreachDeadlines},
		{"NIS2 deadlines", rs.CheckNIS2Deadlines},
		{"policy reviews", rs.CheckPolicyReviews},
		{"finding remediations", rs.CheckFindingRemediations},
		{"vendor assessments", rs.CheckVendorAssessments},
		{"risk reviews", rs.CheckRiskReviews},
		{"DSR deadlines", rs.CheckDSRDeadlines},
	}

	var firstErr error
	for _, check := range checks {
		if err := check.fn(ctx); err != nil {
			log.Error().Err(err).Str("check", check.name).Msg("regulatory check failed")
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", check.name, err)
			}
		}
	}

	log.Info().Msg("regulatory scheduler: deadline checks completed")
	return firstErr
}

// CheckGDPRBreachDeadlines queries incidents where is_breach_notifiable=true AND
// dpa_notified_at IS NULL, calculates hours remaining from the 72-hour GDPR window,
// and emits events at 48h, 12h, 6h, 1h, and 0h (exceeded) thresholds.
func (rs *RegulatoryScheduler) CheckGDPRBreachDeadlines(ctx context.Context) error {
	if rs == nil || rs.pool == nil || rs.bus == nil {
		return fmt.Errorf("GDPR breach scheduler is not configured")
	}
	return runForScheduledTenants(ctx, rs.pool, "GDPR breach deadlines", rs.checkGDPRBreachDeadlinesForTenant)
}

func (rs *RegulatoryScheduler) checkGDPRBreachDeadlinesForTenant(ctx context.Context, tenantID string) error {
	rows, err := database.QuerierFromContext(ctx, rs.pool).Query(ctx, `
		SELECT i.id, i.incident_ref, i.title, i.detected_at, i.notification_deadline
		FROM incidents AS i
		WHERE i.organization_id = $1
		  AND i.is_breach_notifiable
		  AND i.dpa_notified_at IS NULL
		  AND i.notification_deadline IS NOT NULL
		  AND i.notification_deadline <= statement_timestamp() + INTERVAL '48 hours'
		  AND i.deleted_at IS NULL
		  AND i.status NOT IN ('closed', 'cancelled')
		ORDER BY i.notification_deadline, i.id
	`, tenantID)
	if err != nil {
		return fmt.Errorf("query due GDPR breach incidents: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	// Thresholds define the remaining hours at which we emit events.
	type threshold struct {
		hours       float64
		severity    string
		eventSuffix string
	}
	thresholds := []threshold{
		{0, "critical", "deadline_exceeded"},
		{1, "critical", "deadline_1h"},
		{6, "critical", "deadline_6h"},
		{12, "high", "deadline_12h"},
		{48, "high", "deadline_48h"},
	}

	for rows.Next() {
		var incidentID, incidentRef, title string
		var detectedAt, notificationDeadline time.Time

		if err := rows.Scan(&incidentID, &incidentRef, &title, &detectedAt, &notificationDeadline); err != nil {
			return fmt.Errorf("scan due GDPR breach incident: %w", err)
		}

		hoursRemaining := notificationDeadline.Sub(now).Hours()

		// Find the appropriate threshold to emit.
		for _, t := range thresholds {
			if hoursRemaining <= t.hours {
				thresholdTime := notificationDeadline
				if t.hours > 0 {
					thresholdTime = notificationDeadline.Add(-time.Duration(t.hours * float64(time.Hour)))
				}
				rs.bus.Publish(service.Event{
					ID:         scheduledEventID(tenantID, "gdpr.breach_"+t.eventSuffix, incidentID, t.eventSuffix, notificationDeadline),
					Type:       "gdpr.breach_" + t.eventSuffix,
					Severity:   t.severity,
					OrgID:      tenantID,
					EntityType: "incident",
					EntityID:   incidentID,
					EntityRef:  incidentRef,
					Data: map[string]interface{}{
						"incident_title":  title,
						"incident_ref":    incidentRef,
						"detected_at":     detectedAt.Format(time.RFC3339),
						"deadline":        notificationDeadline.Format(time.RFC3339),
						"hours_remaining": fmt.Sprintf("%.1f", t.hours),
						"is_exceeded":     hoursRemaining <= 0,
					},
					Timestamp: thresholdTime,
				})
				break // Only emit the most urgent threshold.
			}
		}
	}

	return rows.Err()
}

// CheckNIS2Deadlines queries NIS2 incident reports with pending phases and checks
// whether their respective deadlines are approaching.
func (rs *RegulatoryScheduler) CheckNIS2Deadlines(ctx context.Context) error {
	return runForScheduledTenants(ctx, rs.pool, "NIS2 deadlines", rs.checkNIS2DeadlinesForTenant)
}

func (rs *RegulatoryScheduler) checkNIS2DeadlinesForTenant(ctx context.Context, organizationID string) error {
	querier := database.QuerierFromContext(ctx, rs.pool)
	rows, err := querier.Query(ctx, `
		SELECT nr.id, nr.organization_id, nr.incident_id, nr.report_ref,
		       phase.name, phase.deadline, i.title
		FROM nis2_incident_reports nr
		JOIN incidents i ON i.organization_id = nr.organization_id AND i.id = nr.incident_id
		CROSS JOIN LATERAL (VALUES
			('early_warning', nr.early_warning_deadline, nr.early_warning_submitted_at, nr.early_warning_status::text),
			('notification', nr.notification_deadline, nr.notification_submitted_at, nr.notification_status::text),
			('final_report', nr.final_report_deadline, nr.final_report_submitted_at, nr.final_report_status::text)
		) AS phase(name, deadline, submitted_at, status)
		WHERE nr.organization_id = $1::uuid
		  AND phase.status IN ('pending', 'overdue')
		  AND phase.submitted_at IS NULL
		  AND phase.deadline <= statement_timestamp() + INTERVAL '24 hours'
		ORDER BY phase.deadline ASC, nr.id, phase.name
	`, organizationID)
	if err != nil {
		return fmt.Errorf("query NIS2 reports: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	for rows.Next() {
		var reportID, orgID, incidentID, reportRef, phase, title string
		var deadline time.Time

		if err := rows.Scan(&reportID, &orgID, &incidentID, &reportRef, &phase, &deadline, &title); err != nil {
			return fmt.Errorf("scan NIS2 report row: %w", err)
		}

		hoursRemaining := deadline.Sub(now).Hours()

		var severity, eventType, threshold string
		var thresholdHours float64
		switch {
		case hoursRemaining <= 0:
			severity = "critical"
			eventType = "nis2.deadline_exceeded"
			threshold = "exceeded"
		case hoursRemaining <= 2:
			severity = "critical"
			eventType = "nis2.deadline_imminent"
			threshold = "2h"
			thresholdHours = 2
		case hoursRemaining <= 12:
			severity = "high"
			eventType = "nis2.deadline_approaching"
			threshold = "12h"
			thresholdHours = 12
		case hoursRemaining <= 24:
			severity = "medium"
			eventType = "nis2.deadline_warning"
			threshold = "24h"
			thresholdHours = 24
		default:
			continue // Not close enough to emit a notification.
		}

		thresholdTime := deadline
		if thresholdHours > 0 {
			thresholdTime = deadline.Add(-time.Duration(thresholdHours * float64(time.Hour)))
		}
		rs.bus.Publish(service.Event{
			ID:         scheduledEventID(orgID, eventType, reportID, threshold+":"+phase, deadline),
			Type:       eventType,
			Severity:   severity,
			OrgID:      orgID,
			EntityType: "nis2_report",
			EntityID:   reportID,
			EntityRef:  fmt.Sprintf("%s/%s", reportRef, phase),
			Data: map[string]interface{}{
				"incident_id":     incidentID,
				"incident_title":  title,
				"phase":           phase,
				"deadline":        deadline.Format(time.RFC3339),
				"hours_remaining": fmt.Sprintf("%.1f", thresholdHours),
			},
			Timestamp: thresholdTime,
		})
	}

	return rows.Err()
}

// CheckPolicyReviews queries policies where next_review_date is approaching or overdue.
func (rs *RegulatoryScheduler) CheckPolicyReviews(ctx context.Context) error {
	return runForScheduledTenants(ctx, rs.pool, "policy review deadlines", rs.checkPolicyReviewsForTenant)
}

func (rs *RegulatoryScheduler) checkPolicyReviewsForTenant(ctx context.Context, organizationID string) error {
	querier := database.QuerierFromContext(ctx, rs.pool)
	rows, err := querier.Query(ctx, `
		SELECT p.id, p.organization_id, p.title, p.next_review_date, p.owner_user_id
		FROM policies p
		WHERE p.organization_id = $1::uuid
		  AND p.deleted_at IS NULL
		  AND p.status IN ('approved', 'published')
		  AND p.next_review_date IS NOT NULL
		  AND p.next_review_date <= NOW() + INTERVAL '30 days'
	`, organizationID)
	if err != nil {
		return fmt.Errorf("query policy reviews: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	for rows.Next() {
		var policyID, orgID, title string
		var nextReview time.Time
		var ownerID *string

		if err := rows.Scan(&policyID, &orgID, &title, &nextReview, &ownerID); err != nil {
			return fmt.Errorf("scan policy review row: %w", err)
		}

		daysUntilReview := nextReview.Sub(now).Hours() / 24

		var severity, eventType string
		var thresholdDays int
		switch {
		case daysUntilReview < 0:
			severity = "high"
			eventType = "policy.review_overdue"
		case daysUntilReview <= 7:
			severity = "high"
			eventType = "policy.review_due_soon"
			thresholdDays = 7
		case daysUntilReview <= 14:
			severity = "medium"
			eventType = "policy.review_approaching"
			thresholdDays = 14
		case daysUntilReview <= 30:
			severity = "low"
			eventType = "policy.review_reminder"
			thresholdDays = 30
		default:
			continue
		}

		data := map[string]interface{}{
			"policy_title":      title,
			"next_review_date":  nextReview.Format("2006-01-02"),
			"days_until_review": fmt.Sprintf("%d", thresholdDays),
		}
		if ownerID != nil {
			data["owner_id"] = *ownerID
		}

		thresholdTime := nextReview.AddDate(0, 0, -thresholdDays)
		rs.bus.Publish(service.Event{
			ID:         scheduledEventID(orgID, eventType, policyID, fmt.Sprintf("%dd", thresholdDays), nextReview),
			Type:       eventType,
			Severity:   severity,
			OrgID:      orgID,
			EntityType: "policy",
			EntityID:   policyID,
			EntityRef:  title,
			Data:       data,
			Timestamp:  thresholdTime,
		})
	}

	return rows.Err()
}

// CheckFindingRemediations queries audit findings past their due date.
func (rs *RegulatoryScheduler) CheckFindingRemediations(ctx context.Context) error {
	return runForScheduledTenants(ctx, rs.pool, "finding remediation deadlines", rs.checkFindingRemediationsForTenant)
}

func (rs *RegulatoryScheduler) checkFindingRemediationsForTenant(ctx context.Context, organizationID string) error {
	querier := database.QuerierFromContext(ctx, rs.pool)
	rows, err := querier.Query(ctx, `
		SELECT af.id, af.organization_id, af.title, af.due_date, af.severity,
		       af.responsible_user_id, a.title AS audit_title
		FROM audit_findings af
		JOIN audits a ON a.organization_id = af.organization_id AND a.id = af.audit_id
		WHERE af.organization_id = $1::uuid
		  AND af.deleted_at IS NULL
		  AND af.status NOT IN ('resolved', 'closed', 'accepted')
		  AND af.due_date IS NOT NULL
		  AND af.due_date <= NOW() + INTERVAL '14 days'
	`, organizationID)
	if err != nil {
		return fmt.Errorf("query audit findings: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	for rows.Next() {
		var findingID, orgID, title, findingSeverity, auditTitle string
		var dueDate time.Time
		var assigneeID *string

		if err := rows.Scan(&findingID, &orgID, &title, &dueDate, &findingSeverity, &assigneeID, &auditTitle); err != nil {
			return fmt.Errorf("scan finding remediation row: %w", err)
		}

		daysUntilDue := dueDate.Sub(now).Hours() / 24

		var severity, eventType string
		var thresholdDays int
		switch {
		case daysUntilDue < 0:
			severity = "high"
			eventType = "finding.remediation_overdue"
		case daysUntilDue <= 3:
			severity = "high"
			eventType = "finding.remediation_due_soon"
			thresholdDays = 3
		case daysUntilDue <= 7:
			severity = "medium"
			eventType = "finding.remediation_approaching"
			thresholdDays = 7
		case daysUntilDue <= 14:
			severity = "low"
			eventType = "finding.remediation_reminder"
			thresholdDays = 14
		default:
			continue
		}

		data := map[string]interface{}{
			"finding_title":    title,
			"audit_title":      auditTitle,
			"due_date":         dueDate.Format("2006-01-02"),
			"finding_severity": findingSeverity,
			"days_until_due":   fmt.Sprintf("%d", thresholdDays),
		}
		if assigneeID != nil {
			data["assignee_id"] = *assigneeID
			data["owner_id"] = *assigneeID
		}

		thresholdTime := dueDate.AddDate(0, 0, -thresholdDays)
		rs.bus.Publish(service.Event{
			ID:         scheduledEventID(orgID, eventType, findingID, fmt.Sprintf("%dd", thresholdDays), dueDate),
			Type:       eventType,
			Severity:   severity,
			OrgID:      orgID,
			EntityType: "audit_finding",
			EntityID:   findingID,
			EntityRef:  title,
			Data:       data,
			Timestamp:  thresholdTime,
		})
	}

	return rows.Err()
}

// CheckVendorAssessments queries vendors where next_assessment_date is approaching.
func (rs *RegulatoryScheduler) CheckVendorAssessments(ctx context.Context) error {
	if rs == nil || rs.pool == nil || rs.bus == nil {
		return fmt.Errorf("vendor assessment scheduler is not configured")
	}
	return runForScheduledTenants(ctx, rs.pool, "vendor assessment deadlines", rs.checkVendorAssessmentsForTenant)
}

func (rs *RegulatoryScheduler) checkVendorAssessmentsForTenant(ctx context.Context, tenantID string) error {
	rows, err := database.QuerierFromContext(ctx, rs.pool).Query(ctx, `
		SELECT v.id,v.vendor_ref,v.name,v.next_assessment_date,v.owner_user_id,v.risk_tier
		FROM vendors AS v
		WHERE v.organization_id=$1::uuid
		  AND v.deleted_at IS NULL
		  AND v.status IN ('active','suspended')
		  AND v.next_assessment_date IS NOT NULL
		  AND v.next_assessment_date <= CURRENT_DATE + 30
		ORDER BY v.next_assessment_date,v.id
	`, tenantID)
	if err != nil {
		return fmt.Errorf("query due vendor assessments: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	for rows.Next() {
		var vendorID, vendorRef, name, riskTier string
		var nextAssessment time.Time
		var ownerID *string

		if err := rows.Scan(&vendorID, &vendorRef, &name, &nextAssessment, &ownerID, &riskTier); err != nil {
			return fmt.Errorf("scan due vendor assessment: %w", err)
		}

		daysUntilAssessment := nextAssessment.Sub(now).Hours() / 24

		var severity, eventType string
		var thresholdDays int
		switch {
		case daysUntilAssessment < 0:
			severity = "high"
			eventType = "vendor.assessment_overdue"
		case daysUntilAssessment <= 7:
			severity = "high"
			eventType = "vendor.assessment_due_soon"
			thresholdDays = 7
		case daysUntilAssessment <= 14:
			severity = "medium"
			eventType = "vendor.assessment_approaching"
			thresholdDays = 14
		case daysUntilAssessment <= 30:
			severity = "low"
			eventType = "vendor.assessment_reminder"
			thresholdDays = 30
		default:
			continue
		}

		data := map[string]interface{}{
			"vendor_name":           name,
			"next_assessment_date":  nextAssessment.Format("2006-01-02"),
			"days_until_assessment": fmt.Sprintf("%d", thresholdDays),
		}
		if ownerID != nil {
			data["owner_id"] = *ownerID
		}
		data["risk_tier"] = riskTier

		thresholdTime := nextAssessment.AddDate(0, 0, -thresholdDays)
		rs.bus.Publish(service.Event{
			ID:         scheduledEventID(tenantID, eventType, vendorID, fmt.Sprintf("%dd", thresholdDays), nextAssessment),
			Type:       eventType,
			Severity:   severity,
			OrgID:      tenantID,
			EntityType: "vendor",
			EntityID:   vendorID,
			EntityRef:  vendorRef,
			Data:       data,
			Timestamp:  thresholdTime,
		})
	}

	return rows.Err()
}

// CheckRiskReviews queries risks where next_review_date is approaching.
func (rs *RegulatoryScheduler) CheckRiskReviews(ctx context.Context) error {
	return runForScheduledTenants(ctx, rs.pool, "risk review deadlines", rs.checkRiskReviewsForTenant)
}

func (rs *RegulatoryScheduler) checkRiskReviewsForTenant(ctx context.Context, organizationID string) error {
	querier := database.QuerierFromContext(ctx, rs.pool)
	rows, err := querier.Query(ctx, `
		SELECT r.id, r.organization_id, r.title, r.next_review_date,
		       r.owner_user_id, r.residual_risk_level
		FROM risks r
		WHERE r.organization_id = $1::uuid
		  AND r.deleted_at IS NULL
		  AND r.status <> 'closed'
		  AND r.next_review_date IS NOT NULL
		  AND r.next_review_date <= NOW() + INTERVAL '30 days'
	`, organizationID)
	if err != nil {
		return fmt.Errorf("query risk reviews: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	for rows.Next() {
		var riskID, orgID, title string
		var nextReview time.Time
		var ownerID *string
		var riskLevel *string

		if err := rows.Scan(&riskID, &orgID, &title, &nextReview, &ownerID, &riskLevel); err != nil {
			return fmt.Errorf("scan risk review row: %w", err)
		}

		daysUntilReview := nextReview.Sub(now).Hours() / 24

		var severity, eventType string
		var thresholdDays int
		switch {
		case daysUntilReview < 0:
			severity = "high"
			eventType = "risk.review_overdue"
		case daysUntilReview <= 7:
			severity = "high"
			eventType = "risk.review_due_soon"
			thresholdDays = 7
		case daysUntilReview <= 14:
			severity = "medium"
			eventType = "risk.review_approaching"
			thresholdDays = 14
		case daysUntilReview <= 30:
			severity = "low"
			eventType = "risk.review_reminder"
			thresholdDays = 30
		default:
			continue
		}

		data := map[string]interface{}{
			"risk_title":        title,
			"next_review_date":  nextReview.Format("2006-01-02"),
			"days_until_review": fmt.Sprintf("%d", thresholdDays),
		}
		if ownerID != nil {
			data["owner_id"] = *ownerID
		}
		if riskLevel != nil {
			data["risk_level"] = *riskLevel
		}

		thresholdTime := nextReview.AddDate(0, 0, -thresholdDays)
		rs.bus.Publish(service.Event{
			ID:         scheduledEventID(orgID, eventType, riskID, fmt.Sprintf("%dd", thresholdDays), nextReview),
			Type:       eventType,
			Severity:   severity,
			OrgID:      orgID,
			EntityType: "risk",
			EntityID:   riskID,
			EntityRef:  title,
			Data:       data,
			Timestamp:  thresholdTime,
		})
	}

	return rows.Err()
}

// CheckDSRDeadlines queries data subject requests (DSRs) where the response
// deadline is approaching.
func (rs *RegulatoryScheduler) CheckDSRDeadlines(ctx context.Context) error {
	return runForScheduledTenants(ctx, rs.pool, "DSR response deadlines", rs.checkDSRDeadlinesForTenant)
}

func (rs *RegulatoryScheduler) checkDSRDeadlinesForTenant(ctx context.Context, organizationID string) error {
	querier := database.QuerierFromContext(ctx, rs.pool)
	rows, err := querier.Query(ctx, `
		SELECT d.id, d.organization_id, d.request_ref, d.request_type,
		       CASE WHEN d.status = 'extended'
		            THEN COALESCE(d.extended_deadline, d.response_deadline)
		            ELSE d.response_deadline
		       END AS effective_deadline,
		       d.assigned_to
		FROM dsr_requests d
		WHERE d.organization_id = $1::uuid
		  AND d.deleted_at IS NULL
		  AND d.status NOT IN ('completed', 'rejected', 'withdrawn')
		  AND d.response_deadline IS NOT NULL
		  AND (CASE WHEN d.status = 'extended'
		            THEN COALESCE(d.extended_deadline, d.response_deadline)
		            ELSE d.response_deadline
		       END) <= CURRENT_DATE + 14
	`, organizationID)
	if err != nil {
		return fmt.Errorf("query DSR deadlines: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	for rows.Next() {
		var dsrID, orgID, requestRef, requestType string
		var responseDeadline time.Time
		var assigneeID *string

		if err := rows.Scan(&dsrID, &orgID, &requestRef, &requestType, &responseDeadline, &assigneeID); err != nil {
			return fmt.Errorf("scan DSR deadline row: %w", err)
		}

		daysRemaining := responseDeadline.Sub(now).Hours() / 24

		var severity, eventType string
		var thresholdDays int
		switch {
		case daysRemaining < 0:
			severity = "critical"
			eventType = "dsr.deadline_exceeded"
		case daysRemaining <= 3:
			severity = "critical"
			eventType = "dsr.deadline_imminent"
			thresholdDays = 3
		case daysRemaining <= 7:
			severity = "high"
			eventType = "dsr.deadline_approaching"
			thresholdDays = 7
		case daysRemaining <= 14:
			severity = "medium"
			eventType = "dsr.deadline_warning"
			thresholdDays = 14
		default:
			continue
		}

		data := map[string]interface{}{
			"request_type":      requestType,
			"request_ref":       requestRef,
			"response_deadline": responseDeadline.Format("2006-01-02"),
			"days_remaining":    fmt.Sprintf("%d", thresholdDays),
		}
		if assigneeID != nil {
			data["assignee_id"] = *assigneeID
			data["owner_id"] = *assigneeID
		}

		thresholdTime := responseDeadline.AddDate(0, 0, -thresholdDays)
		rs.bus.Publish(service.Event{
			ID:         scheduledEventID(orgID, eventType, dsrID, fmt.Sprintf("%dd", thresholdDays), responseDeadline),
			Type:       eventType,
			Severity:   severity,
			OrgID:      orgID,
			EntityType: "dsr_request",
			EntityID:   dsrID,
			EntityRef:  requestRef,
			Data:       data,
			Timestamp:  thresholdTime,
		})
	}

	return rows.Err()
}
