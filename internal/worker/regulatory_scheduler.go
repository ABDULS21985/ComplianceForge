package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

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
	rows, err := rs.pool.Query(ctx, `
		SELECT i.id, i.organization_id, i.title,
		       COALESCE(i.detected_at, i.created_at) AS breach_detected_at,
		       i.notification_deadline
		FROM incidents i
		WHERE i.is_breach_notifiable = true
		  AND i.dpa_notified_at IS NULL
		  AND i.deleted_at IS NULL
		  AND i.status NOT IN ('Closed', 'Resolved')
	`)
	if err != nil {
		return fmt.Errorf("query GDPR breach incidents: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	// Thresholds define the remaining hours at which we emit events.
	type threshold struct {
		hours    float64
		severity string
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
		var incidentID, orgID, title string
		var detectedAt time.Time
		var notificationDeadline *time.Time

		if err := rows.Scan(&incidentID, &orgID, &title, &detectedAt, &notificationDeadline); err != nil {
			log.Error().Err(err).Msg("scan GDPR breach row")
			continue
		}

		// Calculate the deadline: 72 hours from detection.
		deadline := detectedAt.Add(72 * time.Hour)
		if notificationDeadline != nil {
			deadline = *notificationDeadline
		}

		hoursRemaining := deadline.Sub(now).Hours()

		// Find the appropriate threshold to emit.
		for _, t := range thresholds {
			if hoursRemaining <= t.hours {
				rs.bus.Publish(service.Event{
					Type:       "gdpr.breach_" + t.eventSuffix,
					Severity:   t.severity,
					OrgID:      orgID,
					EntityType: "incident",
					EntityID:   incidentID,
					EntityRef:  title,
					Data: map[string]interface{}{
						"incident_title":  title,
						"detected_at":     detectedAt.Format(time.RFC3339),
						"deadline":        deadline.Format(time.RFC3339),
						"hours_remaining": fmt.Sprintf("%.1f", hoursRemaining),
						"is_exceeded":     hoursRemaining <= 0,
					},
					Timestamp: now,
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
	rows, err := rs.pool.Query(ctx, `
		SELECT nr.id, nr.organization_id, nr.incident_id, nr.phase,
		       nr.deadline, i.title
		FROM nis2_incident_reports nr
		JOIN incidents i ON i.id = nr.incident_id
		WHERE nr.status = 'pending'
		  AND nr.submitted_at IS NULL
		  AND nr.deadline IS NOT NULL
		ORDER BY nr.deadline ASC
	`)
	if err != nil {
		return fmt.Errorf("query NIS2 reports: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	for rows.Next() {
		var reportID, orgID, incidentID, phase, title string
		var deadline time.Time

		if err := rows.Scan(&reportID, &orgID, &incidentID, &phase, &deadline, &title); err != nil {
			log.Error().Err(err).Msg("scan NIS2 report row")
			continue
		}

		hoursRemaining := deadline.Sub(now).Hours()

		var severity, eventType string
		switch {
		case hoursRemaining <= 0:
			severity = "critical"
			eventType = "nis2.deadline_exceeded"
		case hoursRemaining <= 2:
			severity = "critical"
			eventType = "nis2.deadline_imminent"
		case hoursRemaining <= 12:
			severity = "high"
			eventType = "nis2.deadline_approaching"
		case hoursRemaining <= 24:
			severity = "medium"
			eventType = "nis2.deadline_warning"
		default:
			continue // Not close enough to emit a notification.
		}

		rs.bus.Publish(service.Event{
			Type:       eventType,
			Severity:   severity,
			OrgID:      orgID,
			EntityType: "nis2_report",
			EntityID:   reportID,
			EntityRef:  fmt.Sprintf("NIS2-%s: %s", phase, title),
			Data: map[string]interface{}{
				"incident_id":     incidentID,
				"incident_title":  title,
				"phase":           phase,
				"deadline":        deadline.Format(time.RFC3339),
				"hours_remaining": fmt.Sprintf("%.1f", hoursRemaining),
			},
			Timestamp: now,
		})
	}

	return rows.Err()
}

// CheckPolicyReviews queries policies where next_review_date is approaching or overdue.
func (rs *RegulatoryScheduler) CheckPolicyReviews(ctx context.Context) error {
	rows, err := rs.pool.Query(ctx, `
		SELECT p.id, p.organization_id, p.title, p.next_review_date, p.owner_id
		FROM policies p
		WHERE p.deleted_at IS NULL
		  AND p.status IN ('Approved', 'Published')
		  AND p.next_review_date IS NOT NULL
		  AND p.next_review_date <= NOW() + INTERVAL '30 days'
	`)
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
			log.Error().Err(err).Msg("scan policy review row")
			continue
		}

		daysUntilReview := nextReview.Sub(now).Hours() / 24

		var severity, eventType string
		switch {
		case daysUntilReview < 0:
			severity = "high"
			eventType = "policy.review_overdue"
		case daysUntilReview <= 7:
			severity = "high"
			eventType = "policy.review_due_soon"
		case daysUntilReview <= 14:
			severity = "medium"
			eventType = "policy.review_approaching"
		case daysUntilReview <= 30:
			severity = "low"
			eventType = "policy.review_reminder"
		default:
			continue
		}

		data := map[string]interface{}{
			"policy_title":    title,
			"next_review_date": nextReview.Format("2006-01-02"),
			"days_until_review": fmt.Sprintf("%.0f", daysUntilReview),
		}
		if ownerID != nil {
			data["owner_id"] = *ownerID
		}

		rs.bus.Publish(service.Event{
			Type:       eventType,
			Severity:   severity,
			OrgID:      orgID,
			EntityType: "policy",
			EntityID:   policyID,
			EntityRef:  title,
			Data:       data,
			Timestamp:  now,
		})
	}

	return rows.Err()
}

// CheckFindingRemediations queries audit findings past their due date.
func (rs *RegulatoryScheduler) CheckFindingRemediations(ctx context.Context) error {
	rows, err := rs.pool.Query(ctx, `
		SELECT af.id, af.organization_id, af.title, af.due_date, af.severity,
		       af.assignee_id, a.title AS audit_title
		FROM audit_findings af
		JOIN audits a ON a.id = af.audit_id
		WHERE af.deleted_at IS NULL
		  AND af.status NOT IN ('Closed', 'Remediated', 'Accepted')
		  AND af.due_date IS NOT NULL
		  AND af.due_date <= NOW() + INTERVAL '14 days'
	`)
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
			log.Error().Err(err).Msg("scan finding remediation row")
			continue
		}

		daysUntilDue := dueDate.Sub(now).Hours() / 24

		var severity, eventType string
		switch {
		case daysUntilDue < 0:
			severity = "high"
			eventType = "finding.remediation_overdue"
		case daysUntilDue <= 3:
			severity = "high"
			eventType = "finding.remediation_due_soon"
		case daysUntilDue <= 7:
			severity = "medium"
			eventType = "finding.remediation_approaching"
		case daysUntilDue <= 14:
			severity = "low"
			eventType = "finding.remediation_reminder"
		default:
			continue
		}

		data := map[string]interface{}{
			"finding_title":   title,
			"audit_title":     auditTitle,
			"due_date":        dueDate.Format("2006-01-02"),
			"finding_severity": findingSeverity,
			"days_until_due":   fmt.Sprintf("%.0f", daysUntilDue),
		}
		if assigneeID != nil {
			data["assignee_id"] = *assigneeID
			data["owner_id"] = *assigneeID
		}

		rs.bus.Publish(service.Event{
			Type:       eventType,
			Severity:   severity,
			OrgID:      orgID,
			EntityType: "audit_finding",
			EntityID:   findingID,
			EntityRef:  title,
			Data:       data,
			Timestamp:  now,
		})
	}

	return rows.Err()
}

// CheckVendorAssessments queries vendors where next_assessment_date is approaching.
func (rs *RegulatoryScheduler) CheckVendorAssessments(ctx context.Context) error {
	rows, err := rs.pool.Query(ctx, `
		SELECT v.id, v.organization_id, v.name, v.next_assessment_date, v.owner_id, v.risk_tier
		FROM vendors v
		WHERE v.deleted_at IS NULL
		  AND v.status = 'Active'
		  AND v.next_assessment_date IS NOT NULL
		  AND v.next_assessment_date <= NOW() + INTERVAL '30 days'
	`)
	if err != nil {
		return fmt.Errorf("query vendor assessments: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	for rows.Next() {
		var vendorID, orgID, name string
		var nextAssessment time.Time
		var ownerID *string
		var riskTier *string

		if err := rows.Scan(&vendorID, &orgID, &name, &nextAssessment, &ownerID, &riskTier); err != nil {
			log.Error().Err(err).Msg("scan vendor assessment row")
			continue
		}

		daysUntilAssessment := nextAssessment.Sub(now).Hours() / 24

		var severity, eventType string
		switch {
		case daysUntilAssessment < 0:
			severity = "high"
			eventType = "vendor.assessment_overdue"
		case daysUntilAssessment <= 7:
			severity = "high"
			eventType = "vendor.assessment_due_soon"
		case daysUntilAssessment <= 14:
			severity = "medium"
			eventType = "vendor.assessment_approaching"
		case daysUntilAssessment <= 30:
			severity = "low"
			eventType = "vendor.assessment_reminder"
		default:
			continue
		}

		data := map[string]interface{}{
			"vendor_name":          name,
			"next_assessment_date": nextAssessment.Format("2006-01-02"),
			"days_until_assessment": fmt.Sprintf("%.0f", daysUntilAssessment),
		}
		if ownerID != nil {
			data["owner_id"] = *ownerID
		}
		if riskTier != nil {
			data["risk_tier"] = *riskTier
		}

		rs.bus.Publish(service.Event{
			Type:       eventType,
			Severity:   severity,
			OrgID:      orgID,
			EntityType: "vendor",
			EntityID:   vendorID,
			EntityRef:  name,
			Data:       data,
			Timestamp:  now,
		})
	}

	return rows.Err()
}

// CheckRiskReviews queries risks where next_review_date is approaching.
func (rs *RegulatoryScheduler) CheckRiskReviews(ctx context.Context) error {
	rows, err := rs.pool.Query(ctx, `
		SELECT r.id, r.organization_id, r.title, r.next_review_date, r.owner_id, r.risk_level
		FROM risks r
		WHERE r.deleted_at IS NULL
		  AND r.status NOT IN ('Closed', 'Archived')
		  AND r.next_review_date IS NOT NULL
		  AND r.next_review_date <= NOW() + INTERVAL '30 days'
	`)
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
			log.Error().Err(err).Msg("scan risk review row")
			continue
		}

		daysUntilReview := nextReview.Sub(now).Hours() / 24

		var severity, eventType string
		switch {
		case daysUntilReview < 0:
			severity = "high"
			eventType = "risk.review_overdue"
		case daysUntilReview <= 7:
			severity = "high"
			eventType = "risk.review_due_soon"
		case daysUntilReview <= 14:
			severity = "medium"
			eventType = "risk.review_approaching"
		case daysUntilReview <= 30:
			severity = "low"
			eventType = "risk.review_reminder"
		default:
			continue
		}

		data := map[string]interface{}{
			"risk_title":       title,
			"next_review_date": nextReview.Format("2006-01-02"),
			"days_until_review": fmt.Sprintf("%.0f", daysUntilReview),
		}
		if ownerID != nil {
			data["owner_id"] = *ownerID
		}
		if riskLevel != nil {
			data["risk_level"] = *riskLevel
		}

		rs.bus.Publish(service.Event{
			Type:       eventType,
			Severity:   severity,
			OrgID:      orgID,
			EntityType: "risk",
			EntityID:   riskID,
			EntityRef:  title,
			Data:       data,
			Timestamp:  now,
		})
	}

	return rows.Err()
}

// CheckDSRDeadlines queries data subject requests (DSRs) where the response
// deadline is approaching.
func (rs *RegulatoryScheduler) CheckDSRDeadlines(ctx context.Context) error {
	rows, err := rs.pool.Query(ctx, `
		SELECT d.id, d.organization_id, d.request_type, d.subject_name,
		       d.response_deadline, d.assignee_id
		FROM dsr_requests d
		WHERE d.deleted_at IS NULL
		  AND d.status NOT IN ('Completed', 'Closed', 'Rejected')
		  AND d.response_deadline IS NOT NULL
		  AND d.response_deadline <= NOW() + INTERVAL '14 days'
	`)
	if err != nil {
		return fmt.Errorf("query DSR deadlines: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	for rows.Next() {
		var dsrID, orgID, requestType, subjectName string
		var responseDeadline time.Time
		var assigneeID *string

		if err := rows.Scan(&dsrID, &orgID, &requestType, &subjectName, &responseDeadline, &assigneeID); err != nil {
			log.Error().Err(err).Msg("scan DSR deadline row")
			continue
		}

		daysRemaining := responseDeadline.Sub(now).Hours() / 24

		var severity, eventType string
		switch {
		case daysRemaining < 0:
			severity = "critical"
			eventType = "dsr.deadline_exceeded"
		case daysRemaining <= 3:
			severity = "critical"
			eventType = "dsr.deadline_imminent"
		case daysRemaining <= 7:
			severity = "high"
			eventType = "dsr.deadline_approaching"
		case daysRemaining <= 14:
			severity = "medium"
			eventType = "dsr.deadline_warning"
		default:
			continue
		}

		data := map[string]interface{}{
			"request_type":      requestType,
			"subject_name":      subjectName,
			"response_deadline": responseDeadline.Format("2006-01-02"),
			"days_remaining":    fmt.Sprintf("%.0f", daysRemaining),
		}
		if assigneeID != nil {
			data["assignee_id"] = *assigneeID
			data["owner_id"] = *assigneeID
		}

		rs.bus.Publish(service.Event{
			Type:       eventType,
			Severity:   severity,
			OrgID:      orgID,
			EntityType: "dsr_request",
			EntityID:   dsrID,
			EntityRef:  fmt.Sprintf("DSR-%s: %s", requestType, subjectName),
			Data:       data,
			Timestamp:  now,
		})
	}

	return rows.Err()
}
