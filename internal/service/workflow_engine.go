package service

import (
	"context"
	"encoding/json"
	"errors"
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
	ErrWorkflowDefinitionNotFound = errors.New("workflow definition not found")
	ErrWorkflowInstanceNotFound   = errors.New("workflow instance not found")
	ErrStepExecutionNotFound      = errors.New("step execution not found")
	ErrStepNotActionable          = errors.New("step execution is not in an actionable state")
	ErrDelegationNotAllowed       = errors.New("delegation not allowed for this step")
	ErrNoMoreSteps                = errors.New("no more steps in workflow")
	ErrWorkflowAlreadyCompleted   = errors.New("workflow already completed or cancelled")
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// WorkflowEngine manages multi-step approval/task workflows.
type WorkflowEngine struct {
	pool *pgxpool.Pool
	bus  *EventBus
}

// WorkflowDefinition describes a reusable workflow template.
type WorkflowDefinition struct {
	ID           string                 `json:"id"`
	OrgID        string                 `json:"organization_id"`
	Name         string                 `json:"name"`
	WorkflowType string                 `json:"workflow_type"`
	EntityType   string                 `json:"entity_type"`
	Version      int                    `json:"version"`
	Status       string                 `json:"status"`
	SLAConfig    map[string]interface{} `json:"sla_config"`
	IsSystem     bool                   `json:"is_system"`
}

// WorkflowStep is one step inside a definition.
type WorkflowStep struct {
	ID                   string                 `json:"id"`
	DefinitionID         string                 `json:"workflow_definition_id"`
	StepOrder            int                    `json:"step_order"`
	Name                 string                 `json:"name"`
	StepType             string                 `json:"step_type"` // approval, review, condition, parallel_gate, auto_action, notification, timer
	ApproverType         *string                `json:"approver_type"`
	ApproverIDs          []string               `json:"approver_ids"`
	ApprovalMode         string                 `json:"approval_mode"` // any, all
	ConditionExpression  map[string]interface{} `json:"condition_expression"`
	ConditionTrueStepID  *string                `json:"condition_true_step_id"`
	ConditionFalseStepID *string                `json:"condition_false_step_id"`
	AutoAction           map[string]interface{} `json:"auto_action"`
	SLAHours             *int                   `json:"sla_hours"`
	EscalationUserIDs    []string               `json:"escalation_user_ids"`
	IsOptional           bool                   `json:"is_optional"`
	CanDelegate          bool                   `json:"can_delegate"`
}

// WorkflowInstance is a running (or completed) workflow for a specific entity.
type WorkflowInstance struct {
	ID                string     `json:"id"`
	OrgID             string     `json:"organization_id"`
	DefinitionID      string     `json:"workflow_definition_id"`
	EntityType        string     `json:"entity_type"`
	EntityID          string     `json:"entity_id"`
	EntityRef         string     `json:"entity_ref"`
	Status            string     `json:"status"` // active, completed, cancelled
	CurrentStepOrder  int        `json:"current_step_order"`
	StartedAt         time.Time  `json:"started_at"`
	StartedBy         string     `json:"started_by"`
	CompletedAt       *time.Time `json:"completed_at"`
	CompletionOutcome *string    `json:"completion_outcome"` // approved, rejected, completed, cancelled
	SLAStatus         string     `json:"sla_status"`
}

// StepExecution tracks a single step's state within an instance.
type StepExecution struct {
	ID             string     `json:"id"`
	InstanceID     string     `json:"workflow_instance_id"`
	StepID         string     `json:"workflow_step_id"`
	StepOrder      int        `json:"step_order"`
	StepName       string     `json:"step_name"`
	Status         string     `json:"status"` // pending, in_progress, completed, rejected, skipped, cancelled
	AssignedTo     *string    `json:"assigned_to"`
	AssignedToName *string    `json:"assigned_to_name"`
	ActionTakenBy  *string    `json:"action_taken_by"`
	Action         *string    `json:"action"`
	Comments       *string    `json:"comments"`
	DecisionReason *string    `json:"decision_reason"`
	SLADeadline    *time.Time `json:"sla_deadline"`
	SLAStatus      *string    `json:"sla_status"`
	StartedAt      *time.Time `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at"`
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// NewWorkflowEngine creates a new WorkflowEngine.
func NewWorkflowEngine(pool *pgxpool.Pool, bus *EventBus) *WorkflowEngine {
	return &WorkflowEngine{pool: pool, bus: bus}
}

// ---------------------------------------------------------------------------
// StartWorkflow
// ---------------------------------------------------------------------------

// StartWorkflow creates a new workflow instance for the given entity, resolves
// the first step and creates its execution record.
func (we *WorkflowEngine) StartWorkflow(
	ctx context.Context,
	orgID, workflowType, entityType, entityID, entityRef, startedBy string,
) (*WorkflowInstance, error) {

	// 1. Find active definition (org-specific first, then system fallback).
	def, err := we.findDefinition(ctx, orgID, workflowType, entityType)
	if err != nil {
		return nil, err
	}

	// 2. Insert instance.
	inst := WorkflowInstance{
		OrgID:            orgID,
		DefinitionID:     def.ID,
		EntityType:       entityType,
		EntityID:         entityID,
		EntityRef:        entityRef,
		Status:           "active",
		CurrentStepOrder: 1,
		StartedAt:        time.Now(),
		StartedBy:        startedBy,
		SLAStatus:        "on_track",
	}

	err = we.pool.QueryRow(ctx, `
		INSERT INTO workflow_instances
			(organization_id, workflow_definition_id, entity_type, entity_id, entity_ref,
			 status, current_step_order, started_at, started_by, sla_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id`,
		inst.OrgID, inst.DefinitionID, inst.EntityType, inst.EntityID, inst.EntityRef,
		inst.Status, inst.CurrentStepOrder, inst.StartedAt, inst.StartedBy, inst.SLAStatus,
	).Scan(&inst.ID)
	if err != nil {
		return nil, fmt.Errorf("insert workflow instance: %w", err)
	}

	// 3. Get first step.
	step, err := we.getStepByOrder(ctx, def.ID, 1)
	if err != nil {
		return nil, fmt.Errorf("get first step: %w", err)
	}

	// 4. Advance to first step (creates execution).
	if err := we.advanceToStep(ctx, orgID, &inst, step); err != nil {
		return nil, fmt.Errorf("advance to first step: %w", err)
	}

	// 5. Emit event.
	we.bus.Publish(Event{
		Type:       "workflow.started",
		Severity:   "medium",
		OrgID:      orgID,
		EntityType: entityType,
		EntityID:   entityID,
		EntityRef:  entityRef,
		Data: map[string]interface{}{
			"workflow_instance_id":    inst.ID,
			"workflow_definition_id":  def.ID,
			"workflow_type":           workflowType,
			"started_by":             startedBy,
		},
		Timestamp: time.Now(),
	})

	log.Info().
		Str("instance_id", inst.ID).
		Str("entity", entityRef).
		Str("workflow_type", workflowType).
		Msg("workflow started")

	return &inst, nil
}

// ---------------------------------------------------------------------------
// ProcessStep
// ---------------------------------------------------------------------------

// ProcessStep handles an action (approve, reject, complete, delegate) on a
// step execution. It is the core state-machine driver of the engine.
func (we *WorkflowEngine) ProcessStep(
	ctx context.Context,
	orgID, executionID string,
	action, actorID, comments, reason string,
) error {

	// 1. Fetch execution and verify actionable.
	exec, err := we.getExecution(ctx, executionID)
	if err != nil {
		return err
	}
	if exec.Status != "pending" && exec.Status != "in_progress" {
		return ErrStepNotActionable
	}

	// Load instance to verify org and status.
	inst, err := we.getInstance(ctx, exec.InstanceID)
	if err != nil {
		return err
	}
	if inst.OrgID != orgID {
		return ErrWorkflowInstanceNotFound
	}
	if inst.Status != "active" {
		return ErrWorkflowAlreadyCompleted
	}

	now := time.Now()

	// 2. Update execution.
	_, err = we.pool.Exec(ctx, `
		UPDATE workflow_step_executions
		SET status          = $1,
			action_taken_by = $2,
			action          = $3,
			comments        = $4,
			decision_reason = $5,
			completed_at    = $6,
			updated_at      = NOW()
		WHERE id = $7`,
		we.actionToStatus(action), actorID, action, nilIfEmpty(comments), nilIfEmpty(reason), now, executionID,
	)
	if err != nil {
		return fmt.Errorf("update step execution: %w", err)
	}

	// 3. Handle reject: cancel entire workflow.
	if action == "reject" {
		return we.completeWorkflow(ctx, inst, "cancelled", "rejected", &now)
	}

	// 4. Handle approve / complete: check parallel gate, then advance.
	if action == "approve" || action == "complete" {
		// Check if all parallel executions for this step order are done.
		allDone, err := we.allParallelExecutionsDone(ctx, exec.InstanceID, exec.StepOrder)
		if err != nil {
			return err
		}
		if !allDone {
			// Still waiting for other parallel approvers.
			return nil
		}

		// Load definition to get steps.
		step, err := we.getStepByID(ctx, exec.StepID)
		if err != nil {
			return fmt.Errorf("get current step: %w", err)
		}

		// Find next step.
		nextStep, err := we.getStepByOrder(ctx, step.DefinitionID, exec.StepOrder+1)
		if err != nil {
			if errors.Is(err, ErrNoMoreSteps) {
				// Workflow complete.
				outcome := "approved"
				if action == "complete" {
					outcome = "completed"
				}
				return we.completeWorkflow(ctx, inst, "completed", outcome, &now)
			}
			return err
		}

		// Update instance current step.
		_, err = we.pool.Exec(ctx, `
			UPDATE workflow_instances
			SET current_step_order = $1, updated_at = NOW()
			WHERE id = $2`,
			nextStep.StepOrder, inst.ID,
		)
		if err != nil {
			return fmt.Errorf("update instance step order: %w", err)
		}
		inst.CurrentStepOrder = nextStep.StepOrder

		if err := we.advanceToStep(ctx, orgID, inst, nextStep); err != nil {
			return fmt.Errorf("advance to next step: %w", err)
		}
	}

	// 5. Emit event.
	we.bus.Publish(Event{
		Type:       "workflow.step." + action,
		Severity:   "medium",
		OrgID:      orgID,
		EntityType: inst.EntityType,
		EntityID:   inst.EntityID,
		EntityRef:  inst.EntityRef,
		Data: map[string]interface{}{
			"workflow_instance_id": inst.ID,
			"execution_id":        executionID,
			"action":              action,
			"actor_id":            actorID,
		},
		Timestamp: now,
	})

	return nil
}

// ---------------------------------------------------------------------------
// advanceToStep
// ---------------------------------------------------------------------------

// advanceToStep resolves the next step and creates execution(s).
func (we *WorkflowEngine) advanceToStep(
	ctx context.Context,
	orgID string,
	instance *WorkflowInstance,
	step *WorkflowStep,
) error {
	switch step.StepType {
	case "approval", "review":
		approverIDs, err := we.resolveApprover(ctx, orgID, step, instance.EntityType, instance.EntityID)
		if err != nil {
			return fmt.Errorf("resolve approver: %w", err)
		}

		for _, approverID := range approverIDs {
			// Check delegation.
			finalApprover := approverID
			delegatee, err := we.checkDelegation(ctx, orgID, approverID)
			if err != nil {
				log.Warn().Err(err).Str("approver", approverID).Msg("delegation check failed, using original approver")
			} else if delegatee != nil {
				finalApprover = *delegatee
			}

			if err := we.createExecution(ctx, instance, step, &finalApprover); err != nil {
				return err
			}
		}

	case "condition":
		// Evaluate condition expression against entity data.
		matches, err := we.evaluateCondition(ctx, step.ConditionExpression, instance.EntityType, instance.EntityID)
		if err != nil {
			return fmt.Errorf("evaluate condition: %w", err)
		}

		var targetStepID *string
		if matches {
			targetStepID = step.ConditionTrueStepID
		} else {
			targetStepID = step.ConditionFalseStepID
		}

		if targetStepID == nil {
			// No branch configured; skip to next sequential step.
			nextStep, err := we.getStepByOrder(ctx, step.DefinitionID, step.StepOrder+1)
			if err != nil {
				return err
			}
			return we.advanceToStep(ctx, orgID, instance, nextStep)
		}

		branchStep, err := we.getStepByID(ctx, *targetStepID)
		if err != nil {
			return fmt.Errorf("get branch step: %w", err)
		}

		// Record condition evaluation as a completed execution.
		if err := we.createCompletedExecution(ctx, instance, step, "condition_evaluated"); err != nil {
			return err
		}

		return we.advanceToStep(ctx, orgID, instance, branchStep)

	case "parallel_gate":
		// Create an execution for each approver listed.
		for _, approverID := range step.ApproverIDs {
			aid := approverID
			if err := we.createExecution(ctx, instance, step, &aid); err != nil {
				return err
			}
		}

	case "auto_action":
		// Execute auto-action (e.g., update entity field).
		if err := we.executeAutoAction(ctx, orgID, step.AutoAction, instance.EntityType, instance.EntityID); err != nil {
			log.Error().Err(err).Str("step_id", step.ID).Msg("auto action failed")
		}
		if err := we.createCompletedExecution(ctx, instance, step, "auto_completed"); err != nil {
			return err
		}
		// Advance to next step.
		nextStep, err := we.getStepByOrder(ctx, step.DefinitionID, step.StepOrder+1)
		if err != nil {
			if errors.Is(err, ErrNoMoreSteps) {
				now := time.Now()
				return we.completeWorkflow(ctx, instance, "completed", "completed", &now)
			}
			return err
		}
		return we.advanceToStep(ctx, orgID, instance, nextStep)

	case "notification":
		// Emit notification event and auto-advance.
		we.bus.Publish(Event{
			Type:       "workflow.notification",
			Severity:   "low",
			OrgID:      orgID,
			EntityType: instance.EntityType,
			EntityID:   instance.EntityID,
			EntityRef:  instance.EntityRef,
			Data: map[string]interface{}{
				"workflow_instance_id": instance.ID,
				"step_id":             step.ID,
				"step_name":           step.Name,
			},
			Timestamp: time.Now(),
		})
		if err := we.createCompletedExecution(ctx, instance, step, "notification_sent"); err != nil {
			return err
		}
		nextStep, err := we.getStepByOrder(ctx, step.DefinitionID, step.StepOrder+1)
		if err != nil {
			if errors.Is(err, ErrNoMoreSteps) {
				now := time.Now()
				return we.completeWorkflow(ctx, instance, "completed", "completed", &now)
			}
			return err
		}
		return we.advanceToStep(ctx, orgID, instance, nextStep)

	case "timer":
		// Schedule a delayed advance. Create a pending execution with SLA as the timer.
		if err := we.createExecution(ctx, instance, step, nil); err != nil {
			return err
		}
		// The timer will be picked up by a background SLA checker to auto-complete.

	default:
		return fmt.Errorf("unsupported step type: %s", step.StepType)
	}

	return nil
}

// ---------------------------------------------------------------------------
// DelegateStep
// ---------------------------------------------------------------------------

// DelegateStep reassigns a pending/in-progress step execution to another user.
func (we *WorkflowEngine) DelegateStep(
	ctx context.Context,
	orgID, executionID, delegatorID, delegateID string,
) error {
	exec, err := we.getExecution(ctx, executionID)
	if err != nil {
		return err
	}
	if exec.Status != "pending" && exec.Status != "in_progress" {
		return ErrStepNotActionable
	}

	// Verify the step allows delegation.
	step, err := we.getStepByID(ctx, exec.StepID)
	if err != nil {
		return err
	}
	if !step.CanDelegate {
		return ErrDelegationNotAllowed
	}

	// Verify org ownership.
	inst, err := we.getInstance(ctx, exec.InstanceID)
	if err != nil {
		return err
	}
	if inst.OrgID != orgID {
		return ErrWorkflowInstanceNotFound
	}

	_, err = we.pool.Exec(ctx, `
		UPDATE workflow_step_executions
		SET assigned_to = $1,
			comments    = COALESCE(comments, '') || $2,
			updated_at  = NOW()
		WHERE id = $3`,
		delegateID,
		fmt.Sprintf("\n[Delegated from %s to %s at %s]", delegatorID, delegateID, time.Now().Format(time.RFC3339)),
		executionID,
	)
	if err != nil {
		return fmt.Errorf("delegate step: %w", err)
	}

	we.bus.Publish(Event{
		Type:       "workflow.step.delegated",
		Severity:   "low",
		OrgID:      orgID,
		EntityType: inst.EntityType,
		EntityID:   inst.EntityID,
		EntityRef:  inst.EntityRef,
		Data: map[string]interface{}{
			"execution_id": executionID,
			"delegator":    delegatorID,
			"delegate":     delegateID,
		},
		Timestamp: time.Now(),
	})

	log.Info().
		Str("execution_id", executionID).
		Str("from", delegatorID).
		Str("to", delegateID).
		Msg("step delegated")

	return nil
}

// ---------------------------------------------------------------------------
// GetPendingApprovals
// ---------------------------------------------------------------------------

// GetPendingApprovals returns all pending step executions assigned to a user.
func (we *WorkflowEngine) GetPendingApprovals(
	ctx context.Context,
	orgID, userID string,
	page, pageSize int,
) ([]StepExecution, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	err := we.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM workflow_step_executions e
		JOIN workflow_instances i ON i.id = e.workflow_instance_id
		WHERE i.organization_id = $1
		  AND e.assigned_to = $2
		  AND e.status IN ('pending','in_progress')`,
		orgID, userID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count pending approvals: %w", err)
	}

	rows, err := we.pool.Query(ctx, `
		SELECT e.id, e.workflow_instance_id, e.workflow_step_id,
			   e.step_order, e.step_name, e.status,
			   e.assigned_to, e.assigned_to_name,
			   e.action_taken_by, e.action, e.comments, e.decision_reason,
			   e.sla_deadline, e.sla_status,
			   e.started_at, e.completed_at
		FROM workflow_step_executions e
		JOIN workflow_instances i ON i.id = e.workflow_instance_id
		WHERE i.organization_id = $1
		  AND e.assigned_to = $2
		  AND e.status IN ('pending','in_progress')
		ORDER BY e.sla_deadline ASC NULLS LAST, e.started_at ASC
		LIMIT $3 OFFSET $4`,
		orgID, userID, pageSize, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query pending approvals: %w", err)
	}
	defer rows.Close()

	var results []StepExecution
	for rows.Next() {
		var e StepExecution
		if err := rows.Scan(
			&e.ID, &e.InstanceID, &e.StepID,
			&e.StepOrder, &e.StepName, &e.Status,
			&e.AssignedTo, &e.AssignedToName,
			&e.ActionTakenBy, &e.Action, &e.Comments, &e.DecisionReason,
			&e.SLADeadline, &e.SLAStatus,
			&e.StartedAt, &e.CompletedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan execution: %w", err)
		}
		results = append(results, e)
	}

	return results, total, nil
}

// ---------------------------------------------------------------------------
// GetWorkflowHistory
// ---------------------------------------------------------------------------

// GetWorkflowHistory returns all workflow instances for a given entity.
func (we *WorkflowEngine) GetWorkflowHistory(
	ctx context.Context,
	orgID, entityType, entityID string,
) ([]WorkflowInstance, error) {
	rows, err := we.pool.Query(ctx, `
		SELECT id, organization_id, workflow_definition_id,
			   entity_type, entity_id, entity_ref,
			   status, current_step_order,
			   started_at, started_by, completed_at, completion_outcome, sla_status
		FROM workflow_instances
		WHERE organization_id = $1
		  AND entity_type = $2
		  AND entity_id = $3
		ORDER BY started_at DESC`,
		orgID, entityType, entityID,
	)
	if err != nil {
		return nil, fmt.Errorf("query workflow history: %w", err)
	}
	defer rows.Close()

	var results []WorkflowInstance
	for rows.Next() {
		var i WorkflowInstance
		if err := rows.Scan(
			&i.ID, &i.OrgID, &i.DefinitionID,
			&i.EntityType, &i.EntityID, &i.EntityRef,
			&i.Status, &i.CurrentStepOrder,
			&i.StartedAt, &i.StartedBy, &i.CompletedAt, &i.CompletionOutcome, &i.SLAStatus,
		); err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		results = append(results, i)
	}

	return results, nil
}

// ---------------------------------------------------------------------------
// CancelWorkflow
// ---------------------------------------------------------------------------

// CancelWorkflow cancels an active workflow instance.
func (we *WorkflowEngine) CancelWorkflow(
	ctx context.Context,
	orgID, instanceID, cancelledBy, reason string,
) error {
	inst, err := we.getInstance(ctx, instanceID)
	if err != nil {
		return err
	}
	if inst.OrgID != orgID {
		return ErrWorkflowInstanceNotFound
	}
	if inst.Status != "active" {
		return ErrWorkflowAlreadyCompleted
	}

	now := time.Now()

	// Cancel all pending executions.
	_, err = we.pool.Exec(ctx, `
		UPDATE workflow_step_executions
		SET status     = 'cancelled',
			comments   = $1,
			completed_at = $2,
			updated_at = NOW()
		WHERE workflow_instance_id = $3
		  AND status IN ('pending','in_progress')`,
		fmt.Sprintf("Cancelled by %s: %s", cancelledBy, reason), now, instanceID,
	)
	if err != nil {
		return fmt.Errorf("cancel pending executions: %w", err)
	}

	if err := we.completeWorkflow(ctx, inst, "cancelled", "cancelled", &now); err != nil {
		return err
	}

	we.bus.Publish(Event{
		Type:       "workflow.cancelled",
		Severity:   "medium",
		OrgID:      orgID,
		EntityType: inst.EntityType,
		EntityID:   inst.EntityID,
		EntityRef:  inst.EntityRef,
		Data: map[string]interface{}{
			"workflow_instance_id": instanceID,
			"cancelled_by":        cancelledBy,
			"reason":              reason,
		},
		Timestamp: now,
	})

	return nil
}

// ---------------------------------------------------------------------------
// ListDefinitions
// ---------------------------------------------------------------------------

// ListDefinitions returns workflow definitions visible to an org (including
// system definitions).
func (we *WorkflowEngine) ListDefinitions(ctx context.Context, orgID string) ([]WorkflowDefinition, error) {
	rows, err := we.pool.Query(ctx, `
		SELECT id, organization_id, name, workflow_type, entity_type,
			   version, status, sla_config, is_system
		FROM workflow_definitions
		WHERE (organization_id = $1 OR is_system = true)
		  AND status = 'active'
		ORDER BY is_system ASC, name ASC`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("list definitions: %w", err)
	}
	defer rows.Close()

	var defs []WorkflowDefinition
	for rows.Next() {
		var d WorkflowDefinition
		var slaJSON []byte
		if err := rows.Scan(
			&d.ID, &d.OrgID, &d.Name, &d.WorkflowType, &d.EntityType,
			&d.Version, &d.Status, &slaJSON, &d.IsSystem,
		); err != nil {
			return nil, fmt.Errorf("scan definition: %w", err)
		}
		if slaJSON != nil {
			_ = json.Unmarshal(slaJSON, &d.SLAConfig)
		}
		defs = append(defs, d)
	}

	return defs, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// findDefinition locates the active definition for the given workflow type and
// entity type. It prefers org-specific definitions over system ones.
func (we *WorkflowEngine) findDefinition(ctx context.Context, orgID, workflowType, entityType string) (*WorkflowDefinition, error) {
	var d WorkflowDefinition
	var slaJSON []byte

	err := we.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, workflow_type, entity_type,
			   version, status, sla_config, is_system
		FROM workflow_definitions
		WHERE workflow_type = $1
		  AND entity_type = $2
		  AND status = 'active'
		  AND (organization_id = $3 OR is_system = true)
		ORDER BY
			CASE WHEN organization_id = $3 THEN 0 ELSE 1 END,
			version DESC
		LIMIT 1`,
		workflowType, entityType, orgID,
	).Scan(
		&d.ID, &d.OrgID, &d.Name, &d.WorkflowType, &d.EntityType,
		&d.Version, &d.Status, &slaJSON, &d.IsSystem,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWorkflowDefinitionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find definition: %w", err)
	}
	if slaJSON != nil {
		_ = json.Unmarshal(slaJSON, &d.SLAConfig)
	}
	return &d, nil
}

// getStepByOrder fetches a step by its order within a definition.
func (we *WorkflowEngine) getStepByOrder(ctx context.Context, definitionID string, order int) (*WorkflowStep, error) {
	return we.scanStep(ctx, `
		SELECT id, workflow_definition_id, step_order, name, step_type,
			   approver_type, approver_ids, approval_mode,
			   condition_expression, condition_true_step_id, condition_false_step_id,
			   auto_action, sla_hours, escalation_user_ids,
			   is_optional, can_delegate
		FROM workflow_steps
		WHERE workflow_definition_id = $1 AND step_order = $2`,
		definitionID, order,
	)
}

// getStepByID fetches a step by primary key.
func (we *WorkflowEngine) getStepByID(ctx context.Context, stepID string) (*WorkflowStep, error) {
	return we.scanStep(ctx, `
		SELECT id, workflow_definition_id, step_order, name, step_type,
			   approver_type, approver_ids, approval_mode,
			   condition_expression, condition_true_step_id, condition_false_step_id,
			   auto_action, sla_hours, escalation_user_ids,
			   is_optional, can_delegate
		FROM workflow_steps
		WHERE id = $1`,
		stepID,
	)
}

// scanStep scans a WorkflowStep from a single-row query.
func (we *WorkflowEngine) scanStep(ctx context.Context, query string, args ...interface{}) (*WorkflowStep, error) {
	var s WorkflowStep
	var approverIDsJSON, condJSON, autoJSON, escalationJSON []byte

	err := we.pool.QueryRow(ctx, query, args...).Scan(
		&s.ID, &s.DefinitionID, &s.StepOrder, &s.Name, &s.StepType,
		&s.ApproverType, &approverIDsJSON, &s.ApprovalMode,
		&condJSON, &s.ConditionTrueStepID, &s.ConditionFalseStepID,
		&autoJSON, &s.SLAHours, &escalationJSON,
		&s.IsOptional, &s.CanDelegate,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoMoreSteps
	}
	if err != nil {
		return nil, fmt.Errorf("scan step: %w", err)
	}

	if approverIDsJSON != nil {
		_ = json.Unmarshal(approverIDsJSON, &s.ApproverIDs)
	}
	if condJSON != nil {
		_ = json.Unmarshal(condJSON, &s.ConditionExpression)
	}
	if autoJSON != nil {
		_ = json.Unmarshal(autoJSON, &s.AutoAction)
	}
	if escalationJSON != nil {
		_ = json.Unmarshal(escalationJSON, &s.EscalationUserIDs)
	}

	return &s, nil
}

// getInstance loads a workflow instance by ID.
func (we *WorkflowEngine) getInstance(ctx context.Context, instanceID string) (*WorkflowInstance, error) {
	var i WorkflowInstance
	err := we.pool.QueryRow(ctx, `
		SELECT id, organization_id, workflow_definition_id,
			   entity_type, entity_id, entity_ref,
			   status, current_step_order,
			   started_at, started_by, completed_at, completion_outcome, sla_status
		FROM workflow_instances
		WHERE id = $1`,
		instanceID,
	).Scan(
		&i.ID, &i.OrgID, &i.DefinitionID,
		&i.EntityType, &i.EntityID, &i.EntityRef,
		&i.Status, &i.CurrentStepOrder,
		&i.StartedAt, &i.StartedBy, &i.CompletedAt, &i.CompletionOutcome, &i.SLAStatus,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWorkflowInstanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	return &i, nil
}

// getExecution loads a step execution by ID.
func (we *WorkflowEngine) getExecution(ctx context.Context, executionID string) (*StepExecution, error) {
	var e StepExecution
	err := we.pool.QueryRow(ctx, `
		SELECT id, workflow_instance_id, workflow_step_id,
			   step_order, step_name, status,
			   assigned_to, assigned_to_name,
			   action_taken_by, action, comments, decision_reason,
			   sla_deadline, sla_status,
			   started_at, completed_at
		FROM workflow_step_executions
		WHERE id = $1`,
		executionID,
	).Scan(
		&e.ID, &e.InstanceID, &e.StepID,
		&e.StepOrder, &e.StepName, &e.Status,
		&e.AssignedTo, &e.AssignedToName,
		&e.ActionTakenBy, &e.Action, &e.Comments, &e.DecisionReason,
		&e.SLADeadline, &e.SLAStatus,
		&e.StartedAt, &e.CompletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrStepExecutionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get execution: %w", err)
	}
	return &e, nil
}

// createExecution inserts a new pending step execution.
func (we *WorkflowEngine) createExecution(ctx context.Context, inst *WorkflowInstance, step *WorkflowStep, assignedTo *string) error {
	now := time.Now()
	var slaDeadline *time.Time
	if step.SLAHours != nil && *step.SLAHours > 0 {
		d := now.Add(time.Duration(*step.SLAHours) * time.Hour)
		slaDeadline = &d
	}

	_, err := we.pool.Exec(ctx, `
		INSERT INTO workflow_step_executions
			(workflow_instance_id, workflow_step_id, step_order, step_name,
			 status, assigned_to, sla_deadline, sla_status, started_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		inst.ID, step.ID, step.StepOrder, step.Name,
		"pending", assignedTo, slaDeadline, nilOrString(slaDeadline, "on_track"), now,
	)
	if err != nil {
		return fmt.Errorf("create execution: %w", err)
	}
	return nil
}

// createCompletedExecution inserts an already-completed execution (for auto/condition/notification steps).
func (we *WorkflowEngine) createCompletedExecution(ctx context.Context, inst *WorkflowInstance, step *WorkflowStep, action string) error {
	now := time.Now()
	_, err := we.pool.Exec(ctx, `
		INSERT INTO workflow_step_executions
			(workflow_instance_id, workflow_step_id, step_order, step_name,
			 status, action, started_at, completed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		inst.ID, step.ID, step.StepOrder, step.Name,
		"completed", action, now, now,
	)
	if err != nil {
		return fmt.Errorf("create completed execution: %w", err)
	}
	return nil
}

// completeWorkflow marks an instance as completed/cancelled.
func (we *WorkflowEngine) completeWorkflow(ctx context.Context, inst *WorkflowInstance, status, outcome string, completedAt *time.Time) error {
	_, err := we.pool.Exec(ctx, `
		UPDATE workflow_instances
		SET status            = $1,
			completion_outcome = $2,
			completed_at      = $3,
			updated_at        = NOW()
		WHERE id = $4`,
		status, outcome, completedAt, inst.ID,
	)
	if err != nil {
		return fmt.Errorf("complete workflow: %w", err)
	}

	we.bus.Publish(Event{
		Type:       "workflow." + status,
		Severity:   "medium",
		OrgID:      inst.OrgID,
		EntityType: inst.EntityType,
		EntityID:   inst.EntityID,
		EntityRef:  inst.EntityRef,
		Data: map[string]interface{}{
			"workflow_instance_id": inst.ID,
			"outcome":             outcome,
		},
		Timestamp: time.Now(),
	})

	log.Info().
		Str("instance_id", inst.ID).
		Str("status", status).
		Str("outcome", outcome).
		Msg("workflow finished")

	return nil
}

// allParallelExecutionsDone checks whether every execution for a given step
// order within an instance has been completed (or skipped).
func (we *WorkflowEngine) allParallelExecutionsDone(ctx context.Context, instanceID string, stepOrder int) (bool, error) {
	var pendingCount int
	err := we.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM workflow_step_executions
		WHERE workflow_instance_id = $1
		  AND step_order = $2
		  AND status IN ('pending','in_progress')`,
		instanceID, stepOrder,
	).Scan(&pendingCount)
	if err != nil {
		return false, fmt.Errorf("check parallel executions: %w", err)
	}
	return pendingCount == 0, nil
}

// resolveApprover determines who should approve based on the step configuration.
func (we *WorkflowEngine) resolveApprover(
	ctx context.Context,
	orgID string,
	step *WorkflowStep,
	entityType, entityID string,
) ([]string, error) {
	// If explicit approver IDs are set, use them.
	if len(step.ApproverIDs) > 0 {
		return step.ApproverIDs, nil
	}

	// Resolve by approver_type.
	if step.ApproverType == nil {
		return nil, fmt.Errorf("no approver configured for step %s", step.ID)
	}

	switch *step.ApproverType {
	case "entity_owner":
		// Look up the owner of the entity.
		var ownerID string
		err := we.pool.QueryRow(ctx, `
			SELECT owner_id FROM entity_owners
			WHERE organization_id = $1
			  AND entity_type = $2
			  AND entity_id = $3`,
			orgID, entityType, entityID,
		).Scan(&ownerID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no owner found for %s/%s", entityType, entityID)
		}
		if err != nil {
			return nil, fmt.Errorf("lookup entity owner: %w", err)
		}
		return []string{ownerID}, nil

	case "role":
		// Find users with the specified role.
		rows, err := we.pool.Query(ctx, `
			SELECT user_id FROM user_roles
			WHERE organization_id = $1
			  AND role = $2
			  AND is_active = true`,
			orgID, step.ApprovalMode, // approval_mode reused to store role name when approver_type=role
		)
		if err != nil {
			return nil, fmt.Errorf("lookup role users: %w", err)
		}
		defer rows.Close()
		var ids []string
		for rows.Next() {
			var uid string
			if err := rows.Scan(&uid); err != nil {
				return nil, fmt.Errorf("scan role user: %w", err)
			}
			ids = append(ids, uid)
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("no users with role for step %s", step.ID)
		}
		return ids, nil

	case "manager":
		// Resolve the reporting manager of the entity owner.
		var managerID string
		err := we.pool.QueryRow(ctx, `
			SELECT u.manager_id FROM users u
			JOIN entity_owners eo ON eo.owner_id = u.id
			WHERE eo.organization_id = $1
			  AND eo.entity_type = $2
			  AND eo.entity_id = $3
			  AND u.manager_id IS NOT NULL`,
			orgID, entityType, entityID,
		).Scan(&managerID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no manager found for entity owner of %s/%s", entityType, entityID)
		}
		if err != nil {
			return nil, fmt.Errorf("lookup manager: %w", err)
		}
		return []string{managerID}, nil

	default:
		return nil, fmt.Errorf("unknown approver type: %s", *step.ApproverType)
	}
}

// checkDelegation checks if the user has an active delegation rule and returns
// the delegatee user ID if so.
func (we *WorkflowEngine) checkDelegation(ctx context.Context, orgID, userID string) (*string, error) {
	var delegateID string
	err := we.pool.QueryRow(ctx, `
		SELECT delegate_user_id
		FROM workflow_delegations
		WHERE organization_id = $1
		  AND delegator_user_id = $2
		  AND is_active = true
		  AND (starts_at IS NULL OR starts_at <= NOW())
		  AND (ends_at IS NULL OR ends_at >= NOW())
		ORDER BY created_at DESC
		LIMIT 1`,
		orgID, userID,
	).Scan(&delegateID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("check delegation: %w", err)
	}
	return &delegateID, nil
}

// evaluateCondition evaluates a simple condition expression against entity data.
// The condition_expression is expected to have the shape:
//
//	{"field": "status", "operator": "eq", "value": "approved"}
func (we *WorkflowEngine) evaluateCondition(
	ctx context.Context,
	expr map[string]interface{},
	entityType, entityID string,
) (bool, error) {
	if expr == nil {
		return true, nil
	}

	field, _ := expr["field"].(string)
	operator, _ := expr["operator"].(string)
	expected, _ := expr["value"].(string)

	if field == "" {
		return true, nil
	}

	// Fetch the actual field value from the entity table.
	// We use a parameterised column lookup via a CASE expression to avoid SQL injection.
	var actual *string
	query := fmt.Sprintf(`
		SELECT %s::text FROM %s WHERE id = $1`,
		pgSafeIdentifier(field), pgSafeIdentifier(entityType+"s"),
	)
	err := we.pool.QueryRow(ctx, query, entityID).Scan(&actual)
	if err != nil {
		return false, fmt.Errorf("evaluate condition field %s: %w", field, err)
	}

	val := ""
	if actual != nil {
		val = *actual
	}

	switch operator {
	case "eq":
		return val == expected, nil
	case "neq":
		return val != expected, nil
	case "contains":
		return len(val) > 0 && len(expected) > 0 && strContains(val, expected), nil
	default:
		return false, fmt.Errorf("unknown condition operator: %s", operator)
	}
}

// executeAutoAction performs an automated action on an entity.
func (we *WorkflowEngine) executeAutoAction(
	ctx context.Context,
	orgID string,
	action map[string]interface{},
	entityType, entityID string,
) error {
	if action == nil {
		return nil
	}

	actionType, _ := action["type"].(string)
	switch actionType {
	case "update_field":
		field, _ := action["field"].(string)
		value, _ := action["value"].(string)
		if field == "" {
			return nil
		}
		query := fmt.Sprintf(`UPDATE %s SET %s = $1, updated_at = NOW() WHERE id = $2`,
			pgSafeIdentifier(entityType+"s"), pgSafeIdentifier(field))
		_, err := we.pool.Exec(ctx, query, value, entityID)
		return err

	case "create_task":
		title, _ := action["title"].(string)
		assignee, _ := action["assignee"].(string)
		_, err := we.pool.Exec(ctx, `
			INSERT INTO tasks (organization_id, title, assigned_to, entity_type, entity_id, status)
			VALUES ($1,$2,$3,$4,$5,'open')`,
			orgID, title, assignee, entityType, entityID)
		return err

	default:
		log.Warn().Str("action_type", actionType).Msg("unknown auto action type, skipping")
		return nil
	}
}

// actionToStatus maps user actions to execution status values.
func (we *WorkflowEngine) actionToStatus(action string) string {
	switch action {
	case "approve", "complete":
		return "completed"
	case "reject":
		return "rejected"
	case "skip":
		return "skipped"
	default:
		return "completed"
	}
}

// pgSafeIdentifier allows only alphanumeric and underscore characters in
// identifiers to prevent SQL injection in dynamic column/table names.
func pgSafeIdentifier(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			out = append(out, c)
		}
	}
	return string(out)
}

// strContains is a simple substring check.
func strContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// nilIfEmpty returns nil if the string is empty, otherwise a pointer.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nilOrString returns nil if the pointer is nil, otherwise the given default value.
func nilOrString(ptr *time.Time, val string) *string {
	if ptr == nil {
		return nil
	}
	return &val
}
