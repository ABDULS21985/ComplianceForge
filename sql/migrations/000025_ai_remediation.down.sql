-- Migration 025 DOWN: AI Remediation Planner
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- ai_interaction_logs
DROP POLICY IF EXISTS ai_logs_tenant_delete ON ai_interaction_logs;
DROP POLICY IF EXISTS ai_logs_tenant_update ON ai_interaction_logs;
DROP POLICY IF EXISTS ai_logs_tenant_insert ON ai_interaction_logs;
DROP POLICY IF EXISTS ai_logs_tenant_select ON ai_interaction_logs;

-- remediation_actions
DROP POLICY IF EXISTS remediation_actions_tenant_delete ON remediation_actions;
DROP POLICY IF EXISTS remediation_actions_tenant_update ON remediation_actions;
DROP POLICY IF EXISTS remediation_actions_tenant_insert ON remediation_actions;
DROP POLICY IF EXISTS remediation_actions_tenant_select ON remediation_actions;

-- remediation_plans
DROP POLICY IF EXISTS remediation_plans_tenant_delete ON remediation_plans;
DROP POLICY IF EXISTS remediation_plans_tenant_update ON remediation_plans;
DROP POLICY IF EXISTS remediation_plans_tenant_insert ON remediation_plans;
DROP POLICY IF EXISTS remediation_plans_tenant_select ON remediation_plans;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_remediation_actions_generate_ref ON remediation_actions;
DROP TRIGGER IF EXISTS trg_remediation_actions_updated_at ON remediation_actions;
DROP TRIGGER IF EXISTS trg_remediation_plans_generate_ref ON remediation_plans;
DROP TRIGGER IF EXISTS trg_remediation_plans_updated_at ON remediation_plans;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS ai_interaction_logs;
DROP TABLE IF EXISTS remediation_actions;
DROP TABLE IF EXISTS remediation_plans;

-- ============================================================================
-- DROP FUNCTIONS
-- ============================================================================

DROP FUNCTION IF EXISTS generate_remediation_action_ref();
DROP FUNCTION IF EXISTS generate_remediation_plan_ref();
