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
// Domain types
// ---------------------------------------------------------------------------

// GeneratePlanRequest holds the input for plan generation.
type GeneratePlanRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Frameworks  []string `json:"frameworks"`
	Industry    string   `json:"industry"`
	OrgSize     string   `json:"org_size"`
	UseAI       bool     `json:"use_ai"`
	GapIDs      []string `json:"gap_ids"`
}

// RemediationPlanResponse is the response after plan creation.
type RemediationPlanResponse struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Status      string              `json:"status"`
	TotalActions int                `json:"total_actions"`
	Actions     []RemediationAction `json:"actions"`
	AIGenerated bool                `json:"ai_generated"`
	CreatedAt   time.Time           `json:"created_at"`
}

// RemediationAction is a single action within a plan.
type RemediationAction struct {
	ID                      string   `json:"id"`
	PlanID                  string   `json:"plan_id"`
	ControlCode             string   `json:"control_code"`
	ControlTitle            string   `json:"control_title"`
	ActionDescription       string   `json:"action_description"`
	Priority                string   `json:"priority"`
	Status                  string   `json:"status"`
	EstimatedEffortHours    int      `json:"estimated_effort_hours"`
	AssignedTo              *string  `json:"assigned_to"`
	DueDate                 *string  `json:"due_date"`
	EvidenceRequired        []string `json:"evidence_required"`
	Notes                   string   `json:"notes"`
	CompletedAt             *string  `json:"completed_at"`
	ControlImplementationID *string  `json:"control_implementation_id"`
}

// PlanProgress aggregates the status of actions in a plan.
type PlanProgress struct {
	PlanID           string  `json:"plan_id"`
	PlanName         string  `json:"plan_name"`
	TotalActions     int     `json:"total_actions"`
	CompletedActions int     `json:"completed_actions"`
	InProgressActions int    `json:"in_progress_actions"`
	PendingActions   int     `json:"pending_actions"`
	OverdueActions   int     `json:"overdue_actions"`
	ProgressPercent  float64 `json:"progress_percent"`
}

// RemediationPlanSummary is a short summary for listing.
type RemediationPlanSummary struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Status          string  `json:"status"`
	TotalActions    int     `json:"total_actions"`
	CompletedActions int    `json:"completed_actions"`
	ProgressPercent float64 `json:"progress_percent"`
	CreatedAt       string  `json:"created_at"`
	ApprovedAt      *string `json:"approved_at"`
}

// RemediationPlanDetail is the full plan with all actions.
type RemediationPlanDetail struct {
	ID              string              `json:"id"`
	OrgID           string              `json:"organization_id"`
	Name            string              `json:"name"`
	Description     string              `json:"description"`
	Status          string              `json:"status"`
	AIGenerated     bool                `json:"ai_generated"`
	AIContent       string              `json:"ai_content"`
	TotalActions    int                 `json:"total_actions"`
	CompletedActions int               `json:"completed_actions"`
	ProgressPercent float64             `json:"progress_percent"`
	Actions         []RemediationAction `json:"actions"`
	ApprovedBy      *string             `json:"approved_by"`
	ApprovedAt      *string             `json:"approved_at"`
	CreatedBy       string              `json:"created_by"`
	CreatedAt       string              `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// RemediationPlanner generates and tracks compliance remediation plans.
type RemediationPlanner struct {
	pool      *pgxpool.Pool
	aiService *AIService
	bus       *EventBus
}

// NewRemediationPlanner creates a new RemediationPlanner.
func NewRemediationPlanner(pool *pgxpool.Pool, aiService *AIService, bus *EventBus) *RemediationPlanner {
	return &RemediationPlanner{pool: pool, aiService: aiService, bus: bus}
}

// GeneratePlan creates a remediation plan from compliance gaps, optionally using AI.
func (rp *RemediationPlanner) GeneratePlan(ctx context.Context, orgID, userID string, request GeneratePlanRequest) (*RemediationPlanResponse, error) {
	// Fetch gaps with control information.
	gapQuery := `
		SELECT ci.id, ci.control_code, ci.control_title, ci.implementation_status,
		       ci.gap_description, cf.code AS framework_code
		FROM control_implementations ci
		JOIN compliance_frameworks cf ON ci.framework_id = cf.id
		WHERE ci.organization_id = $1
		  AND ci.implementation_status IN ('not_implemented', 'partially_implemented')
	`
	args := []interface{}{orgID}

	if len(request.GapIDs) > 0 {
		gapQuery += " AND ci.id = ANY($2)"
		args = append(args, request.GapIDs)
	}
	gapQuery += " ORDER BY CASE ci.implementation_status WHEN 'not_implemented' THEN 1 ELSE 2 END"

	rows, err := rp.pool.Query(ctx, gapQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("querying gaps: %w", err)
	}
	defer rows.Close()

	var gaps []GapInput
	type gapRecord struct {
		implID       string
		controlCode  string
		controlTitle string
		status       string
		gapDesc      *string
		framework    string
	}
	var records []gapRecord

	for rows.Next() {
		var r gapRecord
		if err := rows.Scan(&r.implID, &r.controlCode, &r.controlTitle, &r.status, &r.gapDesc, &r.framework); err != nil {
			return nil, fmt.Errorf("scanning gap: %w", err)
		}
		records = append(records, r)

		desc := ""
		if r.gapDesc != nil {
			desc = *r.gapDesc
		}
		severity := "medium"
		if r.status == "not_implemented" {
			severity = "high"
		}
		gaps = append(gaps, GapInput{
			ControlCode:  r.controlCode,
			ControlTitle: r.controlTitle,
			GapType:      r.status,
			Severity:     severity,
			Description:  desc,
			Framework:    r.framework,
		})
	}

	if len(gaps) == 0 {
		return nil, fmt.Errorf("no compliance gaps found matching the criteria")
	}

	// Generate AI content if requested.
	aiContent := ""
	aiGenerated := false
	if request.UseAI && rp.aiService != nil {
		content, err := rp.aiService.GenerateRemediationPlan(ctx, orgID, gaps, request.Frameworks, request.Industry, request.OrgSize)
		if err != nil {
			log.Warn().Err(err).Msg("remediation_planner: AI generation failed, falling back to rule-based")
		} else {
			aiContent = content
			aiGenerated = true
		}
	}

	// Create plan in a transaction.
	tx, err := rp.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var planID string
	var createdAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO remediation_plans (
			organization_id, name, description, status,
			ai_generated, ai_content, created_by, created_at
		) VALUES ($1, $2, $3, 'draft', $4, $5, $6, NOW())
		RETURNING id, created_at
	`, orgID, request.Name, request.Description, aiGenerated, aiContent, userID).Scan(&planID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("inserting plan: %w", err)
	}

	// Create actions for each gap.
	var actions []RemediationAction
	for i, r := range records {
		priority := "medium"
		effort := 16
		if r.status == "not_implemented" {
			priority = "high"
			effort = 24
		}

		desc := fmt.Sprintf("Implement control %s — %s", r.controlCode, r.controlTitle)
		if r.gapDesc != nil && *r.gapDesc != "" {
			desc = fmt.Sprintf("Remediate: %s", *r.gapDesc)
		}

		evidence := []string{"implementation_evidence", "configuration_screenshot", "policy_document"}
		evidenceJSON, _ := json.Marshal(evidence)

		var actionID string
		err = tx.QueryRow(ctx, `
			INSERT INTO remediation_actions (
				plan_id, control_implementation_id,
				control_code, control_title,
				action_description, priority, status,
				estimated_effort_hours, evidence_required, sort_order,
				created_at
			) VALUES ($1, $2, $3, $4, $5, $6, 'pending', $7, $8, $9, NOW())
			RETURNING id
		`, planID, r.implID, r.controlCode, r.controlTitle,
			desc, priority, effort, evidenceJSON, i+1).Scan(&actionID)
		if err != nil {
			return nil, fmt.Errorf("inserting action %d: %w", i+1, err)
		}

		actions = append(actions, RemediationAction{
			ID:                      actionID,
			PlanID:                  planID,
			ControlCode:             r.controlCode,
			ControlTitle:            r.controlTitle,
			ActionDescription:       desc,
			Priority:                priority,
			Status:                  "pending",
			EstimatedEffortHours:    effort,
			EvidenceRequired:        evidence,
			ControlImplementationID: &r.implID,
		})
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	log.Info().Str("plan_id", planID).Int("actions", len(actions)).Bool("ai", aiGenerated).Msg("remediation_planner: plan created")

	if rp.bus != nil {
		rp.bus.Publish(Event{
			Type:       "remediation.plan_created",
			Severity:   "medium",
			OrgID:      orgID,
			EntityType: "remediation_plan",
			EntityID:   planID,
			Data:       map[string]interface{}{"actions_count": len(actions), "ai_generated": aiGenerated},
			Timestamp:  time.Now().UTC(),
		})
	}

	return &RemediationPlanResponse{
		ID:           planID,
		Name:         request.Name,
		Status:       "draft",
		TotalActions: len(actions),
		Actions:      actions,
		AIGenerated:  aiGenerated,
		CreatedAt:    createdAt,
	}, nil
}

// TrackProgress returns aggregate progress statistics for a plan.
func (rp *RemediationPlanner) TrackProgress(ctx context.Context, orgID, planID string) (*PlanProgress, error) {
	var progress PlanProgress
	progress.PlanID = planID

	err := rp.pool.QueryRow(ctx, `
		SELECT rp.name,
			COUNT(ra.id)::int AS total,
			COUNT(ra.id) FILTER (WHERE ra.status = 'completed')::int AS completed,
			COUNT(ra.id) FILTER (WHERE ra.status = 'in_progress')::int AS in_progress,
			COUNT(ra.id) FILTER (WHERE ra.status = 'pending')::int AS pending,
			COUNT(ra.id) FILTER (WHERE ra.status != 'completed' AND ra.due_date < NOW())::int AS overdue
		FROM remediation_plans rp
		LEFT JOIN remediation_actions ra ON ra.plan_id = rp.id
		WHERE rp.id = $1 AND rp.organization_id = $2
		GROUP BY rp.id, rp.name
	`, planID, orgID).Scan(
		&progress.PlanName,
		&progress.TotalActions,
		&progress.CompletedActions,
		&progress.InProgressActions,
		&progress.PendingActions,
		&progress.OverdueActions,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("plan not found")
		}
		return nil, fmt.Errorf("querying progress: %w", err)
	}

	if progress.TotalActions > 0 {
		progress.ProgressPercent = float64(progress.CompletedActions) / float64(progress.TotalActions) * 100.0
	}

	return &progress, nil
}

// UpdateActionStatus updates the status of a remediation action.
func (rp *RemediationPlanner) UpdateActionStatus(ctx context.Context, orgID, actionID, status, notes string, userID string) error {
	tag, err := rp.pool.Exec(ctx, `
		UPDATE remediation_actions ra
		SET status = $3, notes = COALESCE(NULLIF($4, ''), ra.notes), updated_at = NOW(),
		    completed_at = CASE WHEN $3 = 'completed' THEN NOW() ELSE ra.completed_at END
		FROM remediation_plans rp
		WHERE ra.id = $2 AND ra.plan_id = rp.id AND rp.organization_id = $1
	`, orgID, actionID, status, notes)
	if err != nil {
		return fmt.Errorf("updating action status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("action not found")
	}

	log.Info().Str("action_id", actionID).Str("status", status).Str("user", userID).Msg("remediation_planner: action updated")
	return nil
}

// CompleteAction marks an action as completed with notes and evidence paths.
func (rp *RemediationPlanner) CompleteAction(ctx context.Context, orgID, actionID, notes string, evidencePaths []string, userID string) error {
	evidenceJSON, _ := json.Marshal(evidencePaths)

	tag, err := rp.pool.Exec(ctx, `
		UPDATE remediation_actions ra
		SET status = 'completed',
		    notes = COALESCE(NULLIF($4, ''), ra.notes),
		    evidence_paths = $5,
		    completed_at = NOW(),
		    completed_by = $6,
		    updated_at = NOW()
		FROM remediation_plans rp
		WHERE ra.id = $2 AND ra.plan_id = rp.id AND rp.organization_id = $1
	`, orgID, actionID, nil, notes, evidenceJSON, userID)
	if err != nil {
		return fmt.Errorf("completing action: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("action not found")
	}

	// Check if all actions are completed; if so, mark plan as completed.
	var planID string
	var remaining int
	err = rp.pool.QueryRow(ctx, `
		SELECT ra.plan_id, COUNT(*) FILTER (WHERE ra2.status != 'completed')::int
		FROM remediation_actions ra
		JOIN remediation_actions ra2 ON ra2.plan_id = ra.plan_id
		WHERE ra.id = $1
		GROUP BY ra.plan_id
	`, actionID).Scan(&planID, &remaining)
	if err == nil && remaining == 0 {
		_, _ = rp.pool.Exec(ctx, `
			UPDATE remediation_plans SET status = 'completed', completed_at = NOW() WHERE id = $1
		`, planID)

		if rp.bus != nil {
			rp.bus.Publish(Event{
				Type:       "remediation.plan_completed",
				Severity:   "low",
				OrgID:      orgID,
				EntityType: "remediation_plan",
				EntityID:   planID,
				Data:       map[string]interface{}{"completed_by": userID},
				Timestamp:  time.Now().UTC(),
			})
		}
	}

	log.Info().Str("action_id", actionID).Str("user", userID).Msg("remediation_planner: action completed")
	return nil
}

// ListPlans returns paginated remediation plans for an organisation.
func (rp *RemediationPlanner) ListPlans(ctx context.Context, orgID string, page, pageSize int) ([]RemediationPlanSummary, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	err := rp.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM remediation_plans WHERE organization_id = $1
	`, orgID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting plans: %w", err)
	}

	rows, err := rp.pool.Query(ctx, `
		SELECT rp.id, rp.name, rp.status,
			COUNT(ra.id)::int AS total_actions,
			COUNT(ra.id) FILTER (WHERE ra.status = 'completed')::int AS completed,
			TO_CHAR(rp.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			CASE WHEN rp.approved_at IS NOT NULL THEN TO_CHAR(rp.approved_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') END
		FROM remediation_plans rp
		LEFT JOIN remediation_actions ra ON ra.plan_id = rp.id
		WHERE rp.organization_id = $1
		GROUP BY rp.id
		ORDER BY rp.created_at DESC
		LIMIT $2 OFFSET $3
	`, orgID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying plans: %w", err)
	}
	defer rows.Close()

	var plans []RemediationPlanSummary
	for rows.Next() {
		var p RemediationPlanSummary
		if err := rows.Scan(&p.ID, &p.Name, &p.Status, &p.TotalActions, &p.CompletedActions, &p.CreatedAt, &p.ApprovedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning plan: %w", err)
		}
		if p.TotalActions > 0 {
			p.ProgressPercent = float64(p.CompletedActions) / float64(p.TotalActions) * 100.0
		}
		plans = append(plans, p)
	}

	return plans, total, nil
}

// GetPlan returns full plan detail including all actions.
func (rp *RemediationPlanner) GetPlan(ctx context.Context, orgID, planID string) (*RemediationPlanDetail, error) {
	var plan RemediationPlanDetail
	var aiContentPtr *string
	var descPtr *string
	err := rp.pool.QueryRow(ctx, `
		SELECT rp.id, rp.organization_id, rp.name, rp.description, rp.status,
			rp.ai_generated, rp.ai_content, rp.approved_by,
			CASE WHEN rp.approved_at IS NOT NULL THEN TO_CHAR(rp.approved_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') END,
			rp.created_by,
			TO_CHAR(rp.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM remediation_plans rp
		WHERE rp.id = $1 AND rp.organization_id = $2
	`, planID, orgID).Scan(
		&plan.ID, &plan.OrgID, &plan.Name, &descPtr, &plan.Status,
		&plan.AIGenerated, &aiContentPtr, &plan.ApprovedBy,
		&plan.ApprovedAt, &plan.CreatedBy, &plan.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("plan not found")
		}
		return nil, fmt.Errorf("querying plan: %w", err)
	}
	if descPtr != nil {
		plan.Description = *descPtr
	}
	if aiContentPtr != nil {
		plan.AIContent = *aiContentPtr
	}

	// Fetch actions.
	actionRows, err := rp.pool.Query(ctx, `
		SELECT ra.id, ra.plan_id, ra.control_code, ra.control_title,
			ra.action_description, ra.priority, ra.status,
			ra.estimated_effort_hours, ra.assigned_to,
			CASE WHEN ra.due_date IS NOT NULL THEN TO_CHAR(ra.due_date, 'YYYY-MM-DD') END,
			ra.evidence_required, ra.notes,
			CASE WHEN ra.completed_at IS NOT NULL THEN TO_CHAR(ra.completed_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') END,
			ra.control_implementation_id
		FROM remediation_actions ra
		WHERE ra.plan_id = $1
		ORDER BY ra.sort_order
	`, planID)
	if err != nil {
		return nil, fmt.Errorf("querying actions: %w", err)
	}
	defer actionRows.Close()

	for actionRows.Next() {
		var a RemediationAction
		var evidenceJSON []byte
		var notesPtr *string
		if err := actionRows.Scan(
			&a.ID, &a.PlanID, &a.ControlCode, &a.ControlTitle,
			&a.ActionDescription, &a.Priority, &a.Status,
			&a.EstimatedEffortHours, &a.AssignedTo, &a.DueDate,
			&evidenceJSON, &notesPtr, &a.CompletedAt, &a.ControlImplementationID,
		); err != nil {
			return nil, fmt.Errorf("scanning action: %w", err)
		}
		if notesPtr != nil {
			a.Notes = *notesPtr
		}
		if evidenceJSON != nil {
			_ = json.Unmarshal(evidenceJSON, &a.EvidenceRequired)
		}
		plan.Actions = append(plan.Actions, a)
	}

	plan.TotalActions = len(plan.Actions)
	for _, a := range plan.Actions {
		if a.Status == "completed" {
			plan.CompletedActions++
		}
	}
	if plan.TotalActions > 0 {
		plan.ProgressPercent = float64(plan.CompletedActions) / float64(plan.TotalActions) * 100.0
	}

	return &plan, nil
}

// ApprovePlan marks a draft plan as approved.
func (rp *RemediationPlanner) ApprovePlan(ctx context.Context, orgID, planID, userID string) error {
	tag, err := rp.pool.Exec(ctx, `
		UPDATE remediation_plans
		SET status = 'approved', approved_by = $3, approved_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND organization_id = $2 AND status = 'draft'
	`, planID, orgID, userID)
	if err != nil {
		return fmt.Errorf("approving plan: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("plan not found or not in draft status")
	}

	// Move all pending actions to in_progress.
	_, _ = rp.pool.Exec(ctx, `
		UPDATE remediation_actions SET status = 'in_progress', updated_at = NOW()
		WHERE plan_id = $1 AND status = 'pending'
	`, planID)

	if rp.bus != nil {
		rp.bus.Publish(Event{
			Type:       "remediation.plan_approved",
			Severity:   "low",
			OrgID:      orgID,
			EntityType: "remediation_plan",
			EntityID:   planID,
			Data:       map[string]interface{}{"approved_by": userID},
			Timestamp:  time.Now().UTC(),
		})
	}

	log.Info().Str("plan_id", planID).Str("approved_by", userID).Msg("remediation_planner: plan approved")
	return nil
}
