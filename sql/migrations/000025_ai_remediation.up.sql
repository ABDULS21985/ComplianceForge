-- Migration 025: AI Remediation Planner
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - remediation_plans aggregate one or more remediation_actions into a
--     coherent plan scoped to specific frameworks. Plans can be AI-generated
--     (with confidence scoring) or manually created, with a human review gate.
--   - remediation_actions are the atomic work items: each links to a control
--     implementation, finding, or risk treatment and carries AI-generated
--     guidance (implementation steps, evidence suggestions, tool recommendations,
--     cross-framework benefit analysis, and risk-if-deferred narrative).
--   - ai_interaction_logs is a shared, append-only audit table for every AI
--     call across the platform — token counts, latency, cost, and user feedback
--     are captured for observability, cost management, and model evaluation.
--   - plan_ref (RMP-YYYY-NNNN) and action_ref (RMA-YYYY-NNNN) are auto-generated
--     per organization per year via trigger functions.
--   - All tables are tenant-isolated via RLS on organization_id.

-- ============================================================================
-- TABLE: remediation_plans
-- ============================================================================

CREATE TABLE remediation_plans (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    plan_ref                    VARCHAR(20) NOT NULL,
    name                        VARCHAR(300) NOT NULL,
    description                 TEXT,
    plan_type                   VARCHAR(30) NOT NULL DEFAULT 'gap_remediation'
                                CHECK (plan_type IN ('gap_remediation', 'risk_treatment', 'audit_finding', 'incident_response', 'continuous_improvement', 'framework_adoption')),
    status                      VARCHAR(20) NOT NULL DEFAULT 'draft'
                                CHECK (status IN ('draft', 'pending_review', 'approved', 'in_progress', 'on_hold', 'completed', 'cancelled')),
    scope_framework_ids         UUID[],
    priority                    VARCHAR(10) NOT NULL DEFAULT 'medium'
                                CHECK (priority IN ('critical', 'high', 'medium', 'low')),

    -- AI generation metadata
    ai_generated                BOOLEAN NOT NULL DEFAULT false,
    ai_model                    VARCHAR(100),
    ai_confidence_score         DECIMAL(3,2)
                                CHECK (ai_confidence_score IS NULL OR (ai_confidence_score >= 0 AND ai_confidence_score <= 1)),

    -- Human review gate
    human_reviewed              BOOLEAN NOT NULL DEFAULT false,
    human_reviewed_by           UUID REFERENCES users(id) ON DELETE SET NULL,

    -- Planning
    target_completion_date      DATE,
    estimated_total_hours       DECIMAL(10,2),
    estimated_total_cost        DECIMAL(12,2),
    completion_percentage       DECIMAL(5,2) NOT NULL DEFAULT 0
                                CHECK (completion_percentage >= 0 AND completion_percentage <= 100),

    -- Ownership
    owner_user_id               UUID REFERENCES users(id) ON DELETE SET NULL,
    created_by                  UUID REFERENCES users(id) ON DELETE SET NULL,
    approved_by                 UUID REFERENCES users(id) ON DELETE SET NULL,

    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_remediation_plans_org_ref UNIQUE (organization_id, plan_ref)
);

-- Indexes
CREATE INDEX idx_remediation_plans_org ON remediation_plans(organization_id);
CREATE INDEX idx_remediation_plans_org_status ON remediation_plans(organization_id, status);
CREATE INDEX idx_remediation_plans_org_priority ON remediation_plans(organization_id, priority);
CREATE INDEX idx_remediation_plans_org_type ON remediation_plans(organization_id, plan_type);
CREATE INDEX idx_remediation_plans_owner ON remediation_plans(owner_user_id) WHERE owner_user_id IS NOT NULL;
CREATE INDEX idx_remediation_plans_created_by ON remediation_plans(created_by) WHERE created_by IS NOT NULL;
CREATE INDEX idx_remediation_plans_approved_by ON remediation_plans(approved_by) WHERE approved_by IS NOT NULL;
CREATE INDEX idx_remediation_plans_reviewed_by ON remediation_plans(human_reviewed_by) WHERE human_reviewed_by IS NOT NULL;
CREATE INDEX idx_remediation_plans_target_date ON remediation_plans(target_completion_date) WHERE target_completion_date IS NOT NULL;
CREATE INDEX idx_remediation_plans_ai ON remediation_plans(organization_id, ai_generated) WHERE ai_generated = true;

-- Trigger
CREATE TRIGGER trg_remediation_plans_updated_at
    BEFORE UPDATE ON remediation_plans
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE remediation_plans IS 'Remediation plans aggregate actions to close compliance gaps, treat risks, or address audit findings. Plans can be AI-generated with confidence scoring and require human review before approval.';
COMMENT ON COLUMN remediation_plans.plan_ref IS 'Auto-generated reference per org per year: RMP-YYYY-NNNN.';
COMMENT ON COLUMN remediation_plans.ai_confidence_score IS 'AI model confidence in the generated plan (0.00–1.00). NULL for manually created plans.';
COMMENT ON COLUMN remediation_plans.scope_framework_ids IS 'Array of framework IDs this plan targets. Enables cross-framework remediation planning.';

-- ============================================================================
-- TABLE: remediation_actions
-- ============================================================================

CREATE TABLE remediation_actions (
    id                                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id                     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    plan_id                             UUID NOT NULL REFERENCES remediation_plans(id) ON DELETE CASCADE,
    action_ref                          VARCHAR(20) NOT NULL,
    sort_order                          INT NOT NULL DEFAULT 0,
    title                               VARCHAR(500) NOT NULL,
    description                         TEXT,
    action_type                         VARCHAR(30) NOT NULL DEFAULT 'implement'
                                        CHECK (action_type IN ('implement', 'document', 'configure', 'train', 'review', 'test', 'monitor', 'mitigate', 'accept', 'transfer')),

    -- Linkages
    linked_control_implementation_id    UUID,
    linked_finding_id                   UUID,
    linked_risk_treatment_id            UUID,
    framework_control_code              VARCHAR(50),

    -- Planning
    priority                            VARCHAR(10) NOT NULL DEFAULT 'medium'
                                        CHECK (priority IN ('critical', 'high', 'medium', 'low')),
    estimated_hours                     DECIMAL(8,2),
    estimated_cost                      DECIMAL(10,2),
    required_skills                     TEXT[],
    dependencies                        UUID[],

    -- Assignment
    assigned_to                         UUID REFERENCES users(id) ON DELETE SET NULL,
    target_start_date                   DATE,
    target_end_date                     DATE,

    -- Execution
    status                              VARCHAR(20) NOT NULL DEFAULT 'pending'
                                        CHECK (status IN ('pending', 'in_progress', 'blocked', 'completed', 'verified', 'skipped', 'cancelled')),
    actual_start_date                   DATE,
    actual_end_date                     DATE,
    actual_hours                        DECIMAL(8,2),
    actual_cost                         DECIMAL(10,2),
    completion_notes                    TEXT,
    evidence_paths                      TEXT[],

    -- AI guidance
    ai_implementation_guidance          TEXT,
    ai_evidence_suggestions             TEXT[],
    ai_tool_recommendations             TEXT[],
    ai_risk_if_deferred                 TEXT,
    ai_cross_framework_benefit          TEXT,

    created_at                          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_remediation_actions_org_ref UNIQUE (organization_id, action_ref)
);

-- Indexes
CREATE INDEX idx_remediation_actions_org ON remediation_actions(organization_id);
CREATE INDEX idx_remediation_actions_plan ON remediation_actions(plan_id);
CREATE INDEX idx_remediation_actions_plan_sort ON remediation_actions(plan_id, sort_order);
CREATE INDEX idx_remediation_actions_org_status ON remediation_actions(organization_id, status);
CREATE INDEX idx_remediation_actions_assigned ON remediation_actions(assigned_to) WHERE assigned_to IS NOT NULL;
CREATE INDEX idx_remediation_actions_control ON remediation_actions(linked_control_implementation_id) WHERE linked_control_implementation_id IS NOT NULL;
CREATE INDEX idx_remediation_actions_finding ON remediation_actions(linked_finding_id) WHERE linked_finding_id IS NOT NULL;
CREATE INDEX idx_remediation_actions_risk ON remediation_actions(linked_risk_treatment_id) WHERE linked_risk_treatment_id IS NOT NULL;
CREATE INDEX idx_remediation_actions_target_end ON remediation_actions(target_end_date) WHERE target_end_date IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_remediation_actions_updated_at
    BEFORE UPDATE ON remediation_actions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE remediation_actions IS 'Atomic work items within a remediation plan. Each action links to a control, finding, or risk treatment and carries AI-generated guidance including implementation steps, evidence suggestions, tool recommendations, and cross-framework benefit analysis.';
COMMENT ON COLUMN remediation_actions.action_ref IS 'Auto-generated reference per org per year: RMA-YYYY-NNNN.';
COMMENT ON COLUMN remediation_actions.dependencies IS 'Array of remediation_action UUIDs that must be completed before this action can start.';
COMMENT ON COLUMN remediation_actions.ai_risk_if_deferred IS 'AI-generated narrative describing risk exposure if this action is postponed.';
COMMENT ON COLUMN remediation_actions.ai_cross_framework_benefit IS 'AI analysis of how completing this action benefits compliance across multiple frameworks.';

-- ============================================================================
-- TABLE: ai_interaction_logs (append-only)
-- ============================================================================

CREATE TABLE ai_interaction_logs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    interaction_type    VARCHAR(100) NOT NULL,
    prompt_text         TEXT,
    response_text       TEXT,
    model               VARCHAR(100) NOT NULL,
    input_tokens        INT,
    output_tokens       INT,
    latency_ms          INT,
    cost_eur            DECIMAL(8,4),
    user_id             UUID REFERENCES users(id) ON DELETE SET NULL,
    rating              INT CHECK (rating IS NULL OR (rating >= 1 AND rating <= 5)),
    feedback            TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_ai_logs_org ON ai_interaction_logs(organization_id);
CREATE INDEX idx_ai_logs_org_type ON ai_interaction_logs(organization_id, interaction_type);
CREATE INDEX idx_ai_logs_org_time ON ai_interaction_logs(organization_id, created_at DESC);
CREATE INDEX idx_ai_logs_model ON ai_interaction_logs(model);
CREATE INDEX idx_ai_logs_user ON ai_interaction_logs(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX idx_ai_logs_created ON ai_interaction_logs(created_at DESC);

COMMENT ON TABLE ai_interaction_logs IS 'Append-only audit log for all AI interactions across the platform. Captures prompt/response, token usage, cost, latency, and optional user feedback for observability and model evaluation.';
COMMENT ON COLUMN ai_interaction_logs.interaction_type IS 'Category of AI interaction: remediation_plan_generation, risk_assessment, gap_analysis, evidence_suggestion, etc.';
COMMENT ON COLUMN ai_interaction_logs.cost_eur IS 'Estimated cost in EUR for this AI call, calculated from token counts and model pricing.';
COMMENT ON COLUMN ai_interaction_logs.rating IS 'Optional user quality rating (1–5) for the AI response.';

-- ============================================================================
-- TRIGGER FUNCTIONS
-- ============================================================================

-- Auto-generate plan reference: RMP-YYYY-NNNN (per organization, per year)
CREATE OR REPLACE FUNCTION generate_remediation_plan_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.plan_ref IS NULL OR NEW.plan_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN plan_ref ~ ('^RMP-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(plan_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM remediation_plans
        WHERE organization_id = NEW.organization_id;

        NEW.plan_ref := 'RMP-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_remediation_plans_generate_ref
    BEFORE INSERT ON remediation_plans
    FOR EACH ROW EXECUTE FUNCTION generate_remediation_plan_ref();

-- Auto-generate action reference: RMA-YYYY-NNNN (per organization, per year)
CREATE OR REPLACE FUNCTION generate_remediation_action_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.action_ref IS NULL OR NEW.action_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN action_ref ~ ('^RMA-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(action_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM remediation_actions
        WHERE organization_id = NEW.organization_id;

        NEW.action_ref := 'RMA-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_remediation_actions_generate_ref
    BEFORE INSERT ON remediation_actions
    FOR EACH ROW EXECUTE FUNCTION generate_remediation_action_ref();

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- remediation_plans
ALTER TABLE remediation_plans ENABLE ROW LEVEL SECURITY;
ALTER TABLE remediation_plans FORCE ROW LEVEL SECURITY;

CREATE POLICY remediation_plans_tenant_select ON remediation_plans FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY remediation_plans_tenant_insert ON remediation_plans FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY remediation_plans_tenant_update ON remediation_plans FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY remediation_plans_tenant_delete ON remediation_plans FOR DELETE
    USING (organization_id = get_current_tenant());

-- remediation_actions
ALTER TABLE remediation_actions ENABLE ROW LEVEL SECURITY;
ALTER TABLE remediation_actions FORCE ROW LEVEL SECURITY;

CREATE POLICY remediation_actions_tenant_select ON remediation_actions FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY remediation_actions_tenant_insert ON remediation_actions FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY remediation_actions_tenant_update ON remediation_actions FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY remediation_actions_tenant_delete ON remediation_actions FOR DELETE
    USING (organization_id = get_current_tenant());

-- ai_interaction_logs
ALTER TABLE ai_interaction_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_interaction_logs FORCE ROW LEVEL SECURITY;

CREATE POLICY ai_logs_tenant_select ON ai_interaction_logs FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY ai_logs_tenant_insert ON ai_interaction_logs FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY ai_logs_tenant_update ON ai_interaction_logs FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY ai_logs_tenant_delete ON ai_interaction_logs FOR DELETE
    USING (organization_id = get_current_tenant());
