package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	ErrExceptionNotFound       = fmt.Errorf("exception not found")
	ErrExceptionInvalidStatus  = fmt.Errorf("invalid exception status for this operation")
	ErrMaxRenewalsExceeded     = fmt.Errorf("maximum renewal count exceeded for temporary exception")
	ErrExceptionMissingFields  = fmt.Errorf("required fields missing for approval submission")
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// Exception represents a compliance exception record.
type Exception struct {
	ID                  string                 `json:"id"`
	OrgID               string                 `json:"organization_id"`
	ExceptionRef        string                 `json:"exception_ref"`
	Title               string                 `json:"title"`
	Description         string                 `json:"description"`
	ExceptionType       string                 `json:"exception_type"` // temporary, permanent, conditional
	Status              string                 `json:"status"`         // draft, pending_approval, approved, rejected, revoked, expired
	RiskLevel           string                 `json:"risk_level"`     // critical, high, medium, low
	Justification       string                 `json:"justification"`
	CompensatingControls string                `json:"compensating_controls"`
	ControlIDs          []string               `json:"control_ids"`
	RequestedBy         string                 `json:"requested_by"`
	ApprovedBy          *string                `json:"approved_by"`
	ApprovedAt          *string                `json:"approved_at"`
	RejectedBy          *string                `json:"rejected_by"`
	RejectedAt          *string                `json:"rejected_at"`
	RejectionReason     *string                `json:"rejection_reason"`
	EffectiveDate       string                 `json:"effective_date"`
	ExpirationDate      *string                `json:"expiration_date"`
	NextReviewDate      *string                `json:"next_review_date"`
	RenewalCount        int                    `json:"renewal_count"`
	MaxRenewals         int                    `json:"max_renewals"`
	Metadata            map[string]interface{} `json:"metadata"`
	WorkflowInstanceID  *string                `json:"workflow_instance_id"`
	CreatedAt           string                 `json:"created_at"`
	UpdatedAt           string                 `json:"updated_at"`
}

// CreateExceptionRequest holds input for creating a new exception.
type CreateExceptionRequest struct {
	Title                string                 `json:"title"`
	Description          string                 `json:"description"`
	ExceptionType        string                 `json:"exception_type"`
	RiskLevel            string                 `json:"risk_level"`
	Justification        string                 `json:"justification"`
	CompensatingControls string                 `json:"compensating_controls"`
	ControlIDs           []string               `json:"control_ids"`
	RequestedBy          string                 `json:"requested_by"`
	EffectiveDate        string                 `json:"effective_date"`
	ExpirationDate       *string                `json:"expiration_date"`
	Metadata             map[string]interface{} `json:"metadata"`
}

// ExceptionReview holds the outcome of a periodic exception review.
type ExceptionReview struct {
	ReviewerID     string  `json:"reviewer_id"`
	Outcome        string  `json:"outcome"` // continue, modify, revoke, renew, escalate
	Comments       string  `json:"comments"`
	NewRiskLevel   *string `json:"new_risk_level"`
	NewExpiration  *string `json:"new_expiration_date"`
}

// ExceptionDashboard provides aggregate exception statistics.
type ExceptionDashboard struct {
	TotalActive     int            `json:"total_active"`
	ByRiskLevel     map[string]int `json:"by_risk_level"`
	Expiring30Days  int            `json:"expiring_30_days"`
	Expiring60Days  int            `json:"expiring_60_days"`
	Expiring90Days  int            `json:"expiring_90_days"`
	OverdueReviews  int            `json:"overdue_reviews"`
	AvgAgeDays      float64        `json:"avg_age_days"`
}

// ComplianceImpact represents the compliance score before and after an exception.
type ComplianceImpact struct {
	ExceptionID       string  `json:"exception_id"`
	ScoreBefore       float64 `json:"score_before"`
	ScoreAfter        float64 `json:"score_after"`
	Delta             float64 `json:"delta"`
	AffectedControls  int     `json:"affected_controls"`
	AffectedFrameworks int    `json:"affected_frameworks"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// ExceptionService manages compliance exceptions and their lifecycle.
type ExceptionService struct {
	pool     *pgxpool.Pool
	bus      *EventBus
	workflow *WorkflowEngine
}

// NewExceptionService creates a new ExceptionService.
func NewExceptionService(pool *pgxpool.Pool, bus *EventBus, workflow *WorkflowEngine) *ExceptionService {
	return &ExceptionService{pool: pool, bus: bus, workflow: workflow}
}

// CreateException creates a new exception with auto-generated EXC-YYYY-NNNN reference,
// validates controls, and creates an audit trail entry.
func (s *ExceptionService) CreateException(ctx context.Context, orgID string, req CreateExceptionRequest) (*Exception, error) {
	controlIDsJSON, err := json.Marshal(req.ControlIDs)
	if err != nil {
		return nil, fmt.Errorf("marshal control_ids: %w", err)
	}
	metadataJSON, _ := json.Marshal(req.Metadata)
	if metadataJSON == nil {
		metadataJSON = []byte("{}")
	}

	// Validate that all control IDs exist for this org.
	if len(req.ControlIDs) > 0 {
		var validCount int
		err = s.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM control_implementations
			WHERE organization_id = $1 AND id = ANY($2)`, orgID, req.ControlIDs).Scan(&validCount)
		if err != nil {
			return nil, fmt.Errorf("validate controls: %w", err)
		}
		if validCount != len(req.ControlIDs) {
			return nil, fmt.Errorf("one or more control IDs are invalid")
		}
	}

	// Determine max_renewals based on exception type.
	maxRenewals := 0
	if req.ExceptionType == "temporary" {
		maxRenewals = 2
	}

	var exc Exception
	err = s.pool.QueryRow(ctx, `
		INSERT INTO compliance_exceptions (
			organization_id, title, description, exception_type, status,
			risk_level, justification, compensating_controls, control_ids,
			requested_by, effective_date, expiration_date, max_renewals, metadata
		) VALUES (
			$1, $2, $3, $4, 'draft',
			$5, $6, $7, $8,
			$9, $10, $11, $12, $13
		)
		RETURNING id, exception_ref, title, description, exception_type, status,
			risk_level, justification, compensating_controls, requested_by,
			effective_date, renewal_count, max_renewals,
			created_at, updated_at`,
		orgID, req.Title, req.Description, req.ExceptionType,
		req.RiskLevel, req.Justification, req.CompensatingControls, controlIDsJSON,
		req.RequestedBy, req.EffectiveDate, req.ExpirationDate, maxRenewals, metadataJSON,
	).Scan(
		&exc.ID, &exc.ExceptionRef, &exc.Title, &exc.Description, &exc.ExceptionType,
		&exc.Status, &exc.RiskLevel, &exc.Justification, &exc.CompensatingControls,
		&exc.RequestedBy, &exc.EffectiveDate, &exc.RenewalCount, &exc.MaxRenewals,
		&exc.CreatedAt, &exc.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert exception: %w", err)
	}
	exc.OrgID = orgID
	exc.ControlIDs = req.ControlIDs

	// Audit trail.
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO exception_audit_trail (organization_id, exception_id, action, performed_by, description)
		VALUES ($1, $2, 'created', $3, $4)`,
		orgID, exc.ID, req.RequestedBy,
		fmt.Sprintf("Exception %s created: %s", exc.ExceptionRef, req.Title))

	s.bus.Publish(Event{
		Type:       "exception.created",
		Severity:   req.RiskLevel,
		OrgID:      orgID,
		EntityType: "exception",
		EntityID:   exc.ID,
		EntityRef:  exc.ExceptionRef,
		Data:       map[string]interface{}{"exception_type": req.ExceptionType, "risk_level": req.RiskLevel},
		Timestamp:  time.Now(),
	})

	log.Info().
		Str("exception_id", exc.ID).
		Str("ref", exc.ExceptionRef).
		Str("type", req.ExceptionType).
		Msg("exception created")

	return &exc, nil
}

// SubmitForApproval validates required fields and starts the approval workflow.
func (s *ExceptionService) SubmitForApproval(ctx context.Context, orgID, exceptionID, submittedBy string) error {
	var status, excType, justification, compensating string
	var controlIDs []byte
	err := s.pool.QueryRow(ctx, `
		SELECT status, exception_type, justification, compensating_controls, control_ids
		FROM compliance_exceptions WHERE id = $1 AND organization_id = $2`,
		exceptionID, orgID).Scan(&status, &excType, &justification, &compensating, &controlIDs)
	if err != nil {
		return ErrExceptionNotFound
	}
	if status != "draft" {
		return ErrExceptionInvalidStatus
	}
	if justification == "" || compensating == "" {
		return ErrExceptionMissingFields
	}

	// Start workflow.
	var excRef string
	_ = s.pool.QueryRow(ctx, `SELECT exception_ref FROM compliance_exceptions WHERE id = $1`, exceptionID).Scan(&excRef)

	instance, err := s.workflow.StartWorkflow(ctx, orgID, "exception_approval", "exception", exceptionID, excRef, submittedBy)
	if err != nil {
		return fmt.Errorf("start workflow: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE compliance_exceptions
		SET status = 'pending_approval', workflow_instance_id = $1, updated_at = NOW()
		WHERE id = $2`, instance.ID, exceptionID)
	if err != nil {
		return fmt.Errorf("update exception status: %w", err)
	}

	_, _ = s.pool.Exec(ctx, `
		INSERT INTO exception_audit_trail (organization_id, exception_id, action, performed_by, description)
		VALUES ($1, $2, 'submitted_for_approval', $3, 'Exception submitted for approval')`,
		orgID, exceptionID, submittedBy)

	log.Info().Str("exception_id", exceptionID).Msg("exception submitted for approval")
	return nil
}

// ApproveException approves an exception, sets approved_by/at, calculates next_review_date,
// updates control implementations, and emits an event.
func (s *ExceptionService) ApproveException(ctx context.Context, orgID, exceptionID, approverID, comments string) error {
	var status, excRef, riskLevel string
	err := s.pool.QueryRow(ctx, `
		SELECT status, exception_ref, risk_level FROM compliance_exceptions
		WHERE id = $1 AND organization_id = $2`,
		exceptionID, orgID).Scan(&status, &excRef, &riskLevel)
	if err != nil {
		return ErrExceptionNotFound
	}
	if status != "pending_approval" {
		return ErrExceptionInvalidStatus
	}

	// Calculate next review date: critical=30d, high=60d, medium=90d, low=180d.
	reviewDays := 90
	switch riskLevel {
	case "critical":
		reviewDays = 30
	case "high":
		reviewDays = 60
	case "medium":
		reviewDays = 90
	case "low":
		reviewDays = 180
	}
	nextReview := time.Now().AddDate(0, 0, reviewDays).Format("2006-01-02")

	_, err = s.pool.Exec(ctx, `
		UPDATE compliance_exceptions
		SET status = 'approved', approved_by = $1, approved_at = NOW(),
			next_review_date = $2, updated_at = NOW()
		WHERE id = $3`,
		approverID, nextReview, exceptionID)
	if err != nil {
		return fmt.Errorf("approve exception: %w", err)
	}

	// Update control implementations to reflect exception status.
	_, _ = s.pool.Exec(ctx, `
		UPDATE control_implementations
		SET exception_status = 'exception_approved', exception_id = $1, updated_at = NOW()
		WHERE organization_id = $2 AND id = ANY(
			SELECT jsonb_array_elements_text(control_ids) FROM compliance_exceptions WHERE id = $1
		)`, exceptionID, orgID)

	_, _ = s.pool.Exec(ctx, `
		INSERT INTO exception_audit_trail (organization_id, exception_id, action, performed_by, description)
		VALUES ($1, $2, 'approved', $3, $4)`,
		orgID, exceptionID, approverID,
		fmt.Sprintf("Exception approved. Next review: %s. Comments: %s", nextReview, comments))

	s.bus.Publish(Event{
		Type:       "exception.approved",
		Severity:   riskLevel,
		OrgID:      orgID,
		EntityType: "exception",
		EntityID:   exceptionID,
		EntityRef:  excRef,
		Data:       map[string]interface{}{"approved_by": approverID, "next_review": nextReview},
		Timestamp:  time.Now(),
	})

	log.Info().Str("exception_id", exceptionID).Str("approved_by", approverID).Msg("exception approved")
	return nil
}

// RejectException rejects an exception with a reason.
func (s *ExceptionService) RejectException(ctx context.Context, orgID, exceptionID, rejectorID, reason string) error {
	var status, excRef string
	err := s.pool.QueryRow(ctx, `
		SELECT status, exception_ref FROM compliance_exceptions
		WHERE id = $1 AND organization_id = $2`,
		exceptionID, orgID).Scan(&status, &excRef)
	if err != nil {
		return ErrExceptionNotFound
	}
	if status != "pending_approval" {
		return ErrExceptionInvalidStatus
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE compliance_exceptions
		SET status = 'rejected', rejected_by = $1, rejected_at = NOW(),
			rejection_reason = $2, updated_at = NOW()
		WHERE id = $3`, rejectorID, reason, exceptionID)
	if err != nil {
		return fmt.Errorf("reject exception: %w", err)
	}

	_, _ = s.pool.Exec(ctx, `
		INSERT INTO exception_audit_trail (organization_id, exception_id, action, performed_by, description)
		VALUES ($1, $2, 'rejected', $3, $4)`,
		orgID, exceptionID, rejectorID, fmt.Sprintf("Exception rejected: %s", reason))

	s.bus.Publish(Event{
		Type:       "exception.rejected",
		Severity:   "medium",
		OrgID:      orgID,
		EntityType: "exception",
		EntityID:   exceptionID,
		EntityRef:  excRef,
		Data:       map[string]interface{}{"rejected_by": rejectorID, "reason": reason},
		Timestamp:  time.Now(),
	})

	log.Info().Str("exception_id", exceptionID).Str("rejected_by", rejectorID).Msg("exception rejected")
	return nil
}

// RevokeException revokes an approved exception, reverts control status, and creates a remediation action.
func (s *ExceptionService) RevokeException(ctx context.Context, orgID, exceptionID, revokedBy, reason string) error {
	var status, excRef, title string
	err := s.pool.QueryRow(ctx, `
		SELECT status, exception_ref, title FROM compliance_exceptions
		WHERE id = $1 AND organization_id = $2`,
		exceptionID, orgID).Scan(&status, &excRef, &title)
	if err != nil {
		return ErrExceptionNotFound
	}
	if status != "approved" {
		return ErrExceptionInvalidStatus
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE compliance_exceptions
		SET status = 'revoked', updated_at = NOW()
		WHERE id = $1`, exceptionID)
	if err != nil {
		return fmt.Errorf("revoke exception: %w", err)
	}

	// Revert control implementation statuses.
	_, _ = s.pool.Exec(ctx, `
		UPDATE control_implementations
		SET exception_status = NULL, exception_id = NULL, updated_at = NOW()
		WHERE organization_id = $1 AND exception_id = $2`, orgID, exceptionID)

	// Create remediation action.
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO remediation_actions (
			organization_id, entity_type, entity_id, title, description,
			priority, status, created_by
		) VALUES ($1, 'exception', $2, $3, $4, 'high', 'open', $5)`,
		orgID, exceptionID,
		fmt.Sprintf("Remediate controls after exception %s revocation", excRef),
		fmt.Sprintf("Exception '%s' was revoked: %s. Affected controls require remediation.", title, reason),
		revokedBy)

	_, _ = s.pool.Exec(ctx, `
		INSERT INTO exception_audit_trail (organization_id, exception_id, action, performed_by, description)
		VALUES ($1, $2, 'revoked', $3, $4)`,
		orgID, exceptionID, revokedBy, fmt.Sprintf("Exception revoked: %s", reason))

	s.bus.Publish(Event{
		Type:       "exception.revoked",
		Severity:   "high",
		OrgID:      orgID,
		EntityType: "exception",
		EntityID:   exceptionID,
		EntityRef:  excRef,
		Data:       map[string]interface{}{"revoked_by": revokedBy, "reason": reason},
		Timestamp:  time.Now(),
	})

	log.Info().Str("exception_id", exceptionID).Str("revoked_by", revokedBy).Msg("exception revoked")
	return nil
}

// RenewException renews a temporary exception. Requires a fresh review.
// Increments renewal_count and enforces a max of 2 renewals for temporary exceptions.
func (s *ExceptionService) RenewException(ctx context.Context, orgID, exceptionID, renewedBy string, newExpiration string) error {
	var status, excType string
	var renewalCount, maxRenewals int
	err := s.pool.QueryRow(ctx, `
		SELECT status, exception_type, renewal_count, max_renewals
		FROM compliance_exceptions WHERE id = $1 AND organization_id = $2`,
		exceptionID, orgID).Scan(&status, &excType, &renewalCount, &maxRenewals)
	if err != nil {
		return ErrExceptionNotFound
	}
	if status != "approved" {
		return ErrExceptionInvalidStatus
	}
	if excType == "temporary" && renewalCount >= maxRenewals {
		return ErrMaxRenewalsExceeded
	}

	reviewDays := 90
	nextReview := time.Now().AddDate(0, 0, reviewDays).Format("2006-01-02")

	_, err = s.pool.Exec(ctx, `
		UPDATE compliance_exceptions
		SET expiration_date = $1, renewal_count = renewal_count + 1,
			next_review_date = $2, updated_at = NOW()
		WHERE id = $3`, newExpiration, nextReview, exceptionID)
	if err != nil {
		return fmt.Errorf("renew exception: %w", err)
	}

	_, _ = s.pool.Exec(ctx, `
		INSERT INTO exception_audit_trail (organization_id, exception_id, action, performed_by, description)
		VALUES ($1, $2, 'renewed', $3, $4)`,
		orgID, exceptionID, renewedBy,
		fmt.Sprintf("Exception renewed. New expiration: %s, renewal %d of %d", newExpiration, renewalCount+1, maxRenewals))

	log.Info().
		Str("exception_id", exceptionID).
		Int("renewal", renewalCount+1).
		Msg("exception renewed")
	return nil
}

// ReviewException processes a periodic review with outcomes: continue, modify, revoke, renew, escalate.
func (s *ExceptionService) ReviewException(ctx context.Context, orgID, exceptionID string, review ExceptionReview) error {
	var status string
	err := s.pool.QueryRow(ctx, `
		SELECT status FROM compliance_exceptions WHERE id = $1 AND organization_id = $2`,
		exceptionID, orgID).Scan(&status)
	if err != nil {
		return ErrExceptionNotFound
	}
	if status != "approved" {
		return ErrExceptionInvalidStatus
	}

	switch review.Outcome {
	case "continue":
		nextReview := time.Now().AddDate(0, 3, 0).Format("2006-01-02")
		_, err = s.pool.Exec(ctx, `
			UPDATE compliance_exceptions SET next_review_date = $1, updated_at = NOW() WHERE id = $2`,
			nextReview, exceptionID)
	case "modify":
		if review.NewRiskLevel != nil {
			_, err = s.pool.Exec(ctx, `
				UPDATE compliance_exceptions SET risk_level = $1, updated_at = NOW() WHERE id = $2`,
				*review.NewRiskLevel, exceptionID)
		}
		if review.NewExpiration != nil {
			_, err = s.pool.Exec(ctx, `
				UPDATE compliance_exceptions SET expiration_date = $1, updated_at = NOW() WHERE id = $2`,
				*review.NewExpiration, exceptionID)
		}
	case "revoke":
		return s.RevokeException(ctx, orgID, exceptionID, review.ReviewerID, review.Comments)
	case "renew":
		if review.NewExpiration == nil {
			return fmt.Errorf("new_expiration_date required for renewal")
		}
		return s.RenewException(ctx, orgID, exceptionID, review.ReviewerID, *review.NewExpiration)
	case "escalate":
		_, err = s.pool.Exec(ctx, `
			UPDATE compliance_exceptions SET risk_level = 'critical', updated_at = NOW() WHERE id = $1`,
			exceptionID)
		s.bus.Publish(Event{
			Type: "exception.escalated", Severity: "critical", OrgID: orgID,
			EntityType: "exception", EntityID: exceptionID,
			Data: map[string]interface{}{"comments": review.Comments}, Timestamp: time.Now(),
		})
	default:
		return fmt.Errorf("invalid review outcome: %s", review.Outcome)
	}
	if err != nil {
		return fmt.Errorf("update exception: %w", err)
	}

	_, _ = s.pool.Exec(ctx, `
		INSERT INTO exception_audit_trail (organization_id, exception_id, action, performed_by, description)
		VALUES ($1, $2, 'reviewed', $3, $4)`,
		orgID, exceptionID, review.ReviewerID,
		fmt.Sprintf("Review outcome: %s. %s", review.Outcome, review.Comments))

	log.Info().Str("exception_id", exceptionID).Str("outcome", review.Outcome).Msg("exception reviewed")
	return nil
}

// GetExpiringExceptions returns exceptions expiring within the given number of days.
func (s *ExceptionService) GetExpiringExceptions(ctx context.Context, orgID string, withinDays int) ([]Exception, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, exception_ref, title, description, exception_type,
			status, risk_level, justification, compensating_controls,
			requested_by, effective_date, expiration_date, next_review_date,
			renewal_count, max_renewals, created_at, updated_at
		FROM compliance_exceptions
		WHERE organization_id = $1 AND status = 'approved'
			AND expiration_date IS NOT NULL
			AND expiration_date <= CURRENT_DATE + ($2 || ' days')::INTERVAL
		ORDER BY expiration_date ASC`, orgID, withinDays)
	if err != nil {
		return nil, fmt.Errorf("query expiring exceptions: %w", err)
	}
	defer rows.Close()

	return s.scanExceptions(rows)
}

// GetExceptionDashboard returns aggregate statistics for exceptions.
func (s *ExceptionService) GetExceptionDashboard(ctx context.Context, orgID string) (*ExceptionDashboard, error) {
	dash := &ExceptionDashboard{ByRiskLevel: make(map[string]int)}

	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM compliance_exceptions
		WHERE organization_id = $1 AND status = 'approved'`, orgID).Scan(&dash.TotalActive)
	if err != nil {
		return nil, fmt.Errorf("count active: %w", err)
	}

	// By risk level.
	rows, err := s.pool.Query(ctx, `
		SELECT risk_level, COUNT(*) FROM compliance_exceptions
		WHERE organization_id = $1 AND status = 'approved'
		GROUP BY risk_level`, orgID)
	if err != nil {
		return nil, fmt.Errorf("by risk level: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var level string
		var count int
		if err := rows.Scan(&level, &count); err == nil {
			dash.ByRiskLevel[level] = count
		}
	}

	// Expiring in 30/60/90 days.
	_ = s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE expiration_date <= CURRENT_DATE + INTERVAL '30 days'),
			COUNT(*) FILTER (WHERE expiration_date <= CURRENT_DATE + INTERVAL '60 days'),
			COUNT(*) FILTER (WHERE expiration_date <= CURRENT_DATE + INTERVAL '90 days')
		FROM compliance_exceptions
		WHERE organization_id = $1 AND status = 'approved' AND expiration_date IS NOT NULL`,
		orgID).Scan(&dash.Expiring30Days, &dash.Expiring60Days, &dash.Expiring90Days)

	// Overdue reviews.
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM compliance_exceptions
		WHERE organization_id = $1 AND status = 'approved'
			AND next_review_date IS NOT NULL AND next_review_date < CURRENT_DATE`,
		orgID).Scan(&dash.OverdueReviews)

	// Average age.
	_ = s.pool.QueryRow(ctx, `
		SELECT COALESCE(AVG(EXTRACT(EPOCH FROM (NOW() - created_at)) / 86400), 0)
		FROM compliance_exceptions
		WHERE organization_id = $1 AND status = 'approved'`,
		orgID).Scan(&dash.AvgAgeDays)

	return dash, nil
}

// CalculateComplianceImpact calculates the compliance score before and after an exception.
func (s *ExceptionService) CalculateComplianceImpact(ctx context.Context, orgID, exceptionID string) (*ComplianceImpact, error) {
	impact := &ComplianceImpact{ExceptionID: exceptionID}

	// Get affected controls count.
	err := s.pool.QueryRow(ctx, `
		SELECT jsonb_array_length(control_ids)
		FROM compliance_exceptions WHERE id = $1 AND organization_id = $2`,
		exceptionID, orgID).Scan(&impact.AffectedControls)
	if err != nil {
		return nil, ErrExceptionNotFound
	}

	// Score before: all controls counted normally.
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT COUNT(*) FILTER (WHERE ci.implementation_status = 'implemented') * 100.0 / NULLIF(COUNT(*), 0)
			 FROM control_implementations ci
			 WHERE ci.organization_id = $1), 0)`, orgID).Scan(&impact.ScoreBefore)
	if err != nil {
		return nil, fmt.Errorf("score before: %w", err)
	}

	// Score after: exception controls excluded from denominator.
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT COUNT(*) FILTER (WHERE ci.implementation_status = 'implemented') * 100.0
				/ NULLIF(COUNT(*) FILTER (WHERE ci.exception_id IS NULL OR ci.exception_id != $2), 0)
			 FROM control_implementations ci
			 WHERE ci.organization_id = $1), 0)`,
		orgID, exceptionID).Scan(&impact.ScoreAfter)
	if err != nil {
		return nil, fmt.Errorf("score after: %w", err)
	}

	impact.Delta = impact.ScoreAfter - impact.ScoreBefore

	// Count affected frameworks.
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT fc.framework_id)
		FROM control_implementations ci
		JOIN framework_controls fc ON ci.control_id = fc.id
		WHERE ci.organization_id = $1 AND ci.exception_id = $2`,
		orgID, exceptionID).Scan(&impact.AffectedFrameworks)

	return impact, nil
}

// ListExceptions returns paginated exceptions for an organization.
func (s *ExceptionService) ListExceptions(ctx context.Context, orgID string, page, pageSize int) ([]Exception, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM compliance_exceptions WHERE organization_id = $1`, orgID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count exceptions: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, exception_ref, title, description, exception_type,
			status, risk_level, justification, compensating_controls,
			requested_by, effective_date, expiration_date, next_review_date,
			renewal_count, max_renewals, created_at, updated_at
		FROM compliance_exceptions
		WHERE organization_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, orgID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list exceptions: %w", err)
	}
	defer rows.Close()

	exceptions, err := s.scanExceptions(rows)
	if err != nil {
		return nil, 0, err
	}
	return exceptions, total, nil
}

// GetException retrieves a single exception by ID.
func (s *ExceptionService) GetException(ctx context.Context, orgID, exceptionID string) (*Exception, error) {
	var exc Exception
	var controlIDsJSON []byte
	var metadataJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, exception_ref, title, description, exception_type,
			status, risk_level, justification, compensating_controls, control_ids,
			requested_by, approved_by, approved_at, rejected_by, rejected_at, rejection_reason,
			effective_date, expiration_date, next_review_date,
			renewal_count, max_renewals, metadata, workflow_instance_id,
			created_at, updated_at
		FROM compliance_exceptions
		WHERE id = $1 AND organization_id = $2`,
		exceptionID, orgID,
	).Scan(
		&exc.ID, &exc.OrgID, &exc.ExceptionRef, &exc.Title, &exc.Description, &exc.ExceptionType,
		&exc.Status, &exc.RiskLevel, &exc.Justification, &exc.CompensatingControls, &controlIDsJSON,
		&exc.RequestedBy, &exc.ApprovedBy, &exc.ApprovedAt, &exc.RejectedBy, &exc.RejectedAt, &exc.RejectionReason,
		&exc.EffectiveDate, &exc.ExpirationDate, &exc.NextReviewDate,
		&exc.RenewalCount, &exc.MaxRenewals, &metadataJSON, &exc.WorkflowInstanceID,
		&exc.CreatedAt, &exc.UpdatedAt,
	)
	if err != nil {
		return nil, ErrExceptionNotFound
	}

	_ = json.Unmarshal(controlIDsJSON, &exc.ControlIDs)
	_ = json.Unmarshal(metadataJSON, &exc.Metadata)
	return &exc, nil
}

// scanExceptions scans rows into Exception slices.
func (s *ExceptionService) scanExceptions(rows pgx.Rows) ([]Exception, error) {
	var result []Exception
	for rows.Next() {
		var exc Exception
		err := rows.Scan(
			&exc.ID, &exc.OrgID, &exc.ExceptionRef, &exc.Title, &exc.Description, &exc.ExceptionType,
			&exc.Status, &exc.RiskLevel, &exc.Justification, &exc.CompensatingControls,
			&exc.RequestedBy, &exc.EffectiveDate, &exc.ExpirationDate, &exc.NextReviewDate,
			&exc.RenewalCount, &exc.MaxRenewals, &exc.CreatedAt, &exc.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan exception: %w", err)
		}
		result = append(result, exc)
	}
	return result, nil
}
