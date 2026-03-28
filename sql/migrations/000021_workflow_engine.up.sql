-- Migration 021: Workflow Engine
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - Generic workflow engine supporting policy approvals, risk acceptances,
--     exception requests, finding remediations, vendor onboarding, and more
--   - Definition/instance separation: workflow_definitions are reusable templates,
--     workflow_instances are concrete executions tied to a specific entity
--   - Step-based execution model with support for approval, review, task,
--     notification, conditional branching, parallel gates, timers, and auto-actions
--   - SLA tracking at both instance and step level with on_track/at_risk/breached
--     states enabling proactive escalation before deadlines are missed
--   - Delegation rules allow users to designate alternates during absence,
--     scoped by workflow type and date range
--   - System-default workflow definitions (organization_id IS NULL, is_system = true)
--     are visible to all tenants and serve as starting templates
--   - JSONB configs (trigger_conditions, sla_config, condition_expression, auto_action)
--     allow flexible configuration without schema migrations
--   - approval_mode supports any_one (first approver wins), all_required (unanimous),
--     and majority (>50% of approvers must approve)

-- ============================================================================
-- ENUM TYPES
-- ============================================================================

CREATE TYPE workflow_type AS ENUM (
    'policy_approval',
    'risk_acceptance',
    'exception_request',
    'finding_remediation',
    'vendor_onboarding',
    'vendor_assessment',
    'change_request',
    'access_request',
    'evidence_review',
    'dsr_processing',
    'incident_response',
    'custom'
);

CREATE TYPE workflow_status AS ENUM (
    'draft',
    'active',
    'deprecated'
);

CREATE TYPE workflow_instance_status AS ENUM (
    'active',
    'completed',
    'cancelled',
    'suspended',
    'failed'
);

CREATE TYPE workflow_step_type AS ENUM (
    'approval',
    'review',
    'task',
    'notification',
    'condition',
    'parallel_gate',
    'timer',
    'auto_action'
);

CREATE TYPE workflow_step_exec_status AS ENUM (
    'pending',
    'in_progress',
    'approved',
    'rejected',
    'completed',
    'skipped',
    'escalated',
    'delegated',
    'timed_out'
);

CREATE TYPE workflow_action AS ENUM (
    'approve',
    'reject',
    'complete',
    'delegate',
    'skip',
    'escalate',
    'request_info'
);

CREATE TYPE approval_mode AS ENUM (
    'any_one',
    'all_required',
    'majority'
);

CREATE TYPE sla_tracking_status AS ENUM (
    'on_track',
    'at_risk',
    'breached'
);

CREATE TYPE workflow_completion_outcome AS ENUM (
    'approved',
    'rejected',
    'completed',
    'cancelled',
    'timed_out'
);

-- ============================================================================
-- TABLE: workflow_definitions
-- Reusable workflow templates. System defaults have organization_id = NULL
-- and is_system = true, making them available to all tenants as starting points.
-- ============================================================================

CREATE TABLE workflow_definitions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID REFERENCES organizations(id) ON DELETE CASCADE,
    name                VARCHAR(200) NOT NULL,
    description         TEXT,
    workflow_type       workflow_type NOT NULL,
    entity_type         VARCHAR(100),
    version             INT NOT NULL DEFAULT 1,
    status              workflow_status NOT NULL DEFAULT 'draft',
    trigger_conditions  JSONB,
    sla_config          JSONB,
    metadata            JSONB,
    created_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    is_system           BOOLEAN NOT NULL DEFAULT false,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE workflow_definitions IS 'Reusable workflow templates defining the sequence of steps for a business process. System defaults (organization_id IS NULL) are visible to all tenants. trigger_conditions JSONB defines when a workflow auto-starts (e.g., on entity status change). sla_config JSONB holds overall workflow SLA settings.';
COMMENT ON COLUMN workflow_definitions.entity_type IS 'The type of entity this workflow applies to (e.g., policy, risk, finding, vendor). Used for matching when auto-triggering workflows.';
COMMENT ON COLUMN workflow_definitions.version IS 'Monotonically increasing version number. New versions are created as separate rows sharing the same name but incremented version.';
COMMENT ON COLUMN workflow_definitions.trigger_conditions IS 'JSONB defining auto-trigger rules, e.g. {"on_status_change": "pending_approval", "entity_type": "policy"}';
COMMENT ON COLUMN workflow_definitions.sla_config IS 'JSONB with overall SLA settings, e.g. {"total_hours": 72, "warning_threshold_pct": 75}';

CREATE INDEX idx_wf_defs_org ON workflow_definitions(organization_id);
CREATE INDEX idx_wf_defs_org_type ON workflow_definitions(organization_id, workflow_type);
CREATE INDEX idx_wf_defs_org_status ON workflow_definitions(organization_id, status);
CREATE INDEX idx_wf_defs_entity_type ON workflow_definitions(entity_type);
CREATE INDEX idx_wf_defs_created_by ON workflow_definitions(created_by);

CREATE TRIGGER trg_workflow_definitions_updated_at
    BEFORE UPDATE ON workflow_definitions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE workflow_definitions ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_definitions FORCE ROW LEVEL SECURITY;

-- System definitions (organization_id IS NULL) are visible to all tenants
CREATE POLICY wf_defs_tenant_select ON workflow_definitions FOR SELECT
    USING (organization_id IS NULL OR organization_id = get_current_tenant());
CREATE POLICY wf_defs_tenant_insert ON workflow_definitions FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY wf_defs_tenant_update ON workflow_definitions FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY wf_defs_tenant_delete ON workflow_definitions FOR DELETE
    USING (organization_id = get_current_tenant());

-- ============================================================================
-- TABLE: workflow_steps
-- Individual steps within a workflow definition. Step order determines
-- execution sequence. Conditional steps can branch to different step_orders.
-- ============================================================================

CREATE TABLE workflow_steps (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_definition_id      UUID NOT NULL REFERENCES workflow_definitions(id) ON DELETE CASCADE,
    organization_id             UUID REFERENCES organizations(id) ON DELETE CASCADE,
    step_order                  INT NOT NULL,
    name                        VARCHAR(200) NOT NULL,
    description                 TEXT,
    step_type                   workflow_step_type NOT NULL,

    -- Approval configuration
    approver_type               VARCHAR(50),
    approver_ids                UUID[],
    approval_mode               approval_mode NOT NULL DEFAULT 'any_one',
    minimum_approvals           INT NOT NULL DEFAULT 1,

    -- Task configuration
    task_description            TEXT,
    task_assignee_type          VARCHAR(50),
    task_assignee_ids           UUID[],

    -- Condition configuration (for 'condition' step_type)
    condition_expression        JSONB,
    condition_true_step_id      UUID,
    condition_false_step_id     UUID,

    -- Auto-action configuration (for 'auto_action' step_type)
    auto_action                 JSONB,

    -- Timer configuration
    timer_hours                 INT,
    timer_business_hours_only   BOOLEAN NOT NULL DEFAULT true,

    -- SLA and escalation
    sla_hours                   INT,
    escalation_user_ids         UUID[],
    is_optional                 BOOLEAN NOT NULL DEFAULT false,
    can_delegate                BOOLEAN NOT NULL DEFAULT true,

    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_wf_step_order UNIQUE (workflow_definition_id, step_order)
);

COMMENT ON TABLE workflow_steps IS 'Individual steps within a workflow definition. step_order determines execution sequence. Approval steps require approver_type and approver_ids. Condition steps use condition_expression to evaluate branching logic. Auto-action steps execute automated operations via the auto_action JSONB config.';
COMMENT ON COLUMN workflow_steps.approver_type IS 'How to resolve approvers: "specific" (use approver_ids), "role" (all users with role), "entity_owner" (owner of the target entity), "manager" (assignee manager)';
COMMENT ON COLUMN workflow_steps.approval_mode IS 'any_one: first approver decides; all_required: unanimous; majority: >50% must approve';
COMMENT ON COLUMN workflow_steps.condition_expression IS 'JSONB condition for branching, e.g. {"field": "risk_level", "operator": "in", "value": ["critical","high"]}';
COMMENT ON COLUMN workflow_steps.auto_action IS 'JSONB defining the automated action, e.g. {"action": "update_status", "target": "entity", "value": "approved"}';
COMMENT ON COLUMN workflow_steps.condition_true_step_id IS 'UUID of the workflow_step to jump to when condition evaluates true. References a step in the same definition.';
COMMENT ON COLUMN workflow_steps.condition_false_step_id IS 'UUID of the workflow_step to jump to when condition evaluates false. References a step in the same definition.';

CREATE INDEX idx_wf_steps_def ON workflow_steps(workflow_definition_id);
CREATE INDEX idx_wf_steps_org ON workflow_steps(organization_id);
CREATE INDEX idx_wf_steps_def_order ON workflow_steps(workflow_definition_id, step_order);
CREATE INDEX idx_wf_steps_type ON workflow_steps(step_type);

CREATE TRIGGER trg_workflow_steps_updated_at
    BEFORE UPDATE ON workflow_steps
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE workflow_steps ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_steps FORCE ROW LEVEL SECURITY;

-- System steps (organization_id IS NULL) are visible to all tenants
CREATE POLICY wf_steps_tenant_select ON workflow_steps FOR SELECT
    USING (organization_id IS NULL OR organization_id = get_current_tenant());
CREATE POLICY wf_steps_tenant_insert ON workflow_steps FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY wf_steps_tenant_update ON workflow_steps FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY wf_steps_tenant_delete ON workflow_steps FOR DELETE
    USING (organization_id = get_current_tenant());

-- ============================================================================
-- TABLE: workflow_instances
-- Concrete execution of a workflow definition, tied to a specific entity.
-- Tracks progress through steps, overall SLA, and completion outcome.
-- ============================================================================

CREATE TABLE workflow_instances (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    workflow_definition_id      UUID NOT NULL REFERENCES workflow_definitions(id) ON DELETE RESTRICT,
    entity_type                 VARCHAR(100) NOT NULL,
    entity_id                   UUID NOT NULL,
    entity_ref                  VARCHAR(50),
    status                      workflow_instance_status NOT NULL DEFAULT 'active',
    current_step_id             UUID REFERENCES workflow_steps(id) ON DELETE SET NULL,
    current_step_order          INT,
    started_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_by                  UUID REFERENCES users(id) ON DELETE SET NULL,
    completed_at                TIMESTAMPTZ,
    completion_outcome          workflow_completion_outcome,
    total_duration_hours        DECIMAL(10,2),
    sla_status                  sla_tracking_status NOT NULL DEFAULT 'on_track',
    sla_deadline                TIMESTAMPTZ,
    metadata                    JSONB,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE workflow_instances IS 'A running or completed execution of a workflow definition. Each instance is bound to a specific entity (e.g., a policy, risk, finding) via entity_type + entity_id. Tracks the current step, SLA status, and final outcome.';
COMMENT ON COLUMN workflow_instances.entity_ref IS 'Human-readable reference for the entity (e.g., "POL-2026-001"), denormalized for display in workflow lists.';
COMMENT ON COLUMN workflow_instances.total_duration_hours IS 'Calculated when workflow completes: difference between started_at and completed_at in hours.';
COMMENT ON COLUMN workflow_instances.sla_deadline IS 'Absolute deadline for the entire workflow, calculated from sla_config on the definition.';

CREATE INDEX idx_wf_inst_org ON workflow_instances(organization_id);
CREATE INDEX idx_wf_inst_org_status ON workflow_instances(organization_id, status);
CREATE INDEX idx_wf_inst_def ON workflow_instances(workflow_definition_id);
CREATE INDEX idx_wf_inst_entity ON workflow_instances(entity_type, entity_id);
CREATE INDEX idx_wf_inst_current_step ON workflow_instances(current_step_id);
CREATE INDEX idx_wf_inst_started_by ON workflow_instances(started_by);
CREATE INDEX idx_wf_inst_sla_deadline ON workflow_instances(sla_deadline) WHERE status = 'active';
CREATE INDEX idx_wf_inst_sla_status ON workflow_instances(organization_id, sla_status) WHERE status = 'active';

CREATE TRIGGER trg_workflow_instances_updated_at
    BEFORE UPDATE ON workflow_instances
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE workflow_instances ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_instances FORCE ROW LEVEL SECURITY;

CREATE POLICY wf_inst_tenant_select ON workflow_instances FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY wf_inst_tenant_insert ON workflow_instances FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY wf_inst_tenant_update ON workflow_instances FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY wf_inst_tenant_delete ON workflow_instances FOR DELETE
    USING (organization_id = get_current_tenant());

-- ============================================================================
-- TABLE: workflow_step_executions
-- Execution record for each step within an instance. Tracks who acted,
-- what action was taken, SLA compliance, delegation, and escalation.
-- ============================================================================

CREATE TABLE workflow_step_executions (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    workflow_instance_id        UUID NOT NULL REFERENCES workflow_instances(id) ON DELETE CASCADE,
    workflow_step_id            UUID NOT NULL REFERENCES workflow_steps(id) ON DELETE RESTRICT,
    step_order                  INT NOT NULL,
    status                      workflow_step_exec_status NOT NULL DEFAULT 'pending',

    -- Assignment
    assigned_to                 UUID REFERENCES users(id) ON DELETE SET NULL,

    -- Delegation
    delegated_to                UUID REFERENCES users(id) ON DELETE SET NULL,
    delegated_by                UUID REFERENCES users(id) ON DELETE SET NULL,
    delegated_at                TIMESTAMPTZ,

    -- Action taken
    action_taken_by             UUID REFERENCES users(id) ON DELETE SET NULL,
    action_taken_at             TIMESTAMPTZ,
    action                      workflow_action,
    comments                    TEXT,
    decision_reason             TEXT,
    attachments_paths           TEXT[],

    -- SLA tracking
    sla_deadline                TIMESTAMPTZ,
    sla_status                  sla_tracking_status,

    -- Escalation
    escalated_at                TIMESTAMPTZ,
    escalated_to                UUID REFERENCES users(id) ON DELETE SET NULL,

    -- Timing
    started_at                  TIMESTAMPTZ,
    completed_at                TIMESTAMPTZ,
    duration_hours              DECIMAL(10,2),

    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE workflow_step_executions IS 'Execution record for each step within a workflow instance. One row per step execution (a step may execute multiple times if retried). Captures the full audit trail: who was assigned, who acted, what action was taken, delegation chain, SLA compliance, and timing.';
COMMENT ON COLUMN workflow_step_executions.decision_reason IS 'Structured reason for approval/rejection, useful for compliance audit trails.';
COMMENT ON COLUMN workflow_step_executions.attachments_paths IS 'Array of file storage paths for supporting documents attached during this step.';
COMMENT ON COLUMN workflow_step_executions.duration_hours IS 'Calculated when step completes: difference between started_at and completed_at in hours.';

CREATE INDEX idx_wf_step_exec_org ON workflow_step_executions(organization_id);
CREATE INDEX idx_wf_step_exec_instance ON workflow_step_executions(workflow_instance_id);
CREATE INDEX idx_wf_step_exec_step ON workflow_step_executions(workflow_step_id);
CREATE INDEX idx_wf_step_exec_assigned_status ON workflow_step_executions(assigned_to, status) WHERE status IN ('pending', 'in_progress');
CREATE INDEX idx_wf_step_exec_org_status ON workflow_step_executions(organization_id, status);
CREATE INDEX idx_wf_step_exec_sla_deadline ON workflow_step_executions(sla_deadline) WHERE status IN ('pending', 'in_progress');
CREATE INDEX idx_wf_step_exec_action_by ON workflow_step_executions(action_taken_by);
CREATE INDEX idx_wf_step_exec_delegated_to ON workflow_step_executions(delegated_to) WHERE delegated_to IS NOT NULL;
CREATE INDEX idx_wf_step_exec_escalated_to ON workflow_step_executions(escalated_to) WHERE escalated_to IS NOT NULL;

ALTER TABLE workflow_step_executions ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_step_executions FORCE ROW LEVEL SECURITY;

CREATE POLICY wf_step_exec_tenant_select ON workflow_step_executions FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY wf_step_exec_tenant_insert ON workflow_step_executions FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY wf_step_exec_tenant_update ON workflow_step_executions FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY wf_step_exec_tenant_delete ON workflow_step_executions FOR DELETE
    USING (organization_id = get_current_tenant());

-- ============================================================================
-- TABLE: workflow_delegation_rules
-- Allows users to delegate their workflow responsibilities to another user
-- for a defined period (e.g., during vacation or leave).
-- ============================================================================

CREATE TABLE workflow_delegation_rules (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    delegator_user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    delegate_user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workflow_types              TEXT[],
    valid_from                  DATE NOT NULL,
    valid_until                 DATE NOT NULL,
    reason                      TEXT,
    is_active                   BOOLEAN NOT NULL DEFAULT true,
    created_by                  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_delegation_dates CHECK (valid_until >= valid_from)
);

COMMENT ON TABLE workflow_delegation_rules IS 'Delegation rules allowing a user to designate an alternate for workflow approvals/tasks during a date range. workflow_types array scopes which workflows are delegated (NULL means all). The engine checks active delegation rules when assigning step executions.';
COMMENT ON COLUMN workflow_delegation_rules.workflow_types IS 'Array of workflow_type values this delegation applies to. NULL or empty means delegation applies to all workflow types.';

CREATE INDEX idx_wf_deleg_org ON workflow_delegation_rules(organization_id);
CREATE INDEX idx_wf_deleg_delegator ON workflow_delegation_rules(delegator_user_id);
CREATE INDEX idx_wf_deleg_delegate ON workflow_delegation_rules(delegate_user_id);
CREATE INDEX idx_wf_deleg_active ON workflow_delegation_rules(organization_id, is_active)
    WHERE is_active = true;
CREATE INDEX idx_wf_deleg_dates ON workflow_delegation_rules(valid_from, valid_until)
    WHERE is_active = true;
CREATE INDEX idx_wf_deleg_created_by ON workflow_delegation_rules(created_by);

CREATE TRIGGER trg_workflow_delegation_rules_updated_at
    BEFORE UPDATE ON workflow_delegation_rules
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE workflow_delegation_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_delegation_rules FORCE ROW LEVEL SECURITY;

CREATE POLICY wf_deleg_tenant_select ON workflow_delegation_rules FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY wf_deleg_tenant_insert ON workflow_delegation_rules FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY wf_deleg_tenant_update ON workflow_delegation_rules FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY wf_deleg_tenant_delete ON workflow_delegation_rules FOR DELETE
    USING (organization_id = get_current_tenant());
