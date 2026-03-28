-- Migration 021 DOWN: Workflow Engine
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- workflow_delegation_rules
DROP POLICY IF EXISTS wf_deleg_tenant_delete ON workflow_delegation_rules;
DROP POLICY IF EXISTS wf_deleg_tenant_update ON workflow_delegation_rules;
DROP POLICY IF EXISTS wf_deleg_tenant_insert ON workflow_delegation_rules;
DROP POLICY IF EXISTS wf_deleg_tenant_select ON workflow_delegation_rules;

-- workflow_step_executions
DROP POLICY IF EXISTS wf_step_exec_tenant_delete ON workflow_step_executions;
DROP POLICY IF EXISTS wf_step_exec_tenant_update ON workflow_step_executions;
DROP POLICY IF EXISTS wf_step_exec_tenant_insert ON workflow_step_executions;
DROP POLICY IF EXISTS wf_step_exec_tenant_select ON workflow_step_executions;

-- workflow_instances
DROP POLICY IF EXISTS wf_inst_tenant_delete ON workflow_instances;
DROP POLICY IF EXISTS wf_inst_tenant_update ON workflow_instances;
DROP POLICY IF EXISTS wf_inst_tenant_insert ON workflow_instances;
DROP POLICY IF EXISTS wf_inst_tenant_select ON workflow_instances;

-- workflow_steps
DROP POLICY IF EXISTS wf_steps_tenant_delete ON workflow_steps;
DROP POLICY IF EXISTS wf_steps_tenant_update ON workflow_steps;
DROP POLICY IF EXISTS wf_steps_tenant_insert ON workflow_steps;
DROP POLICY IF EXISTS wf_steps_tenant_select ON workflow_steps;

-- workflow_definitions
DROP POLICY IF EXISTS wf_defs_tenant_delete ON workflow_definitions;
DROP POLICY IF EXISTS wf_defs_tenant_update ON workflow_definitions;
DROP POLICY IF EXISTS wf_defs_tenant_insert ON workflow_definitions;
DROP POLICY IF EXISTS wf_defs_tenant_select ON workflow_definitions;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_workflow_delegation_rules_updated_at ON workflow_delegation_rules;
DROP TRIGGER IF EXISTS trg_workflow_instances_updated_at ON workflow_instances;
DROP TRIGGER IF EXISTS trg_workflow_steps_updated_at ON workflow_steps;
DROP TRIGGER IF EXISTS trg_workflow_definitions_updated_at ON workflow_definitions;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS workflow_delegation_rules;
DROP TABLE IF EXISTS workflow_step_executions;
DROP TABLE IF EXISTS workflow_instances;
DROP TABLE IF EXISTS workflow_steps;
DROP TABLE IF EXISTS workflow_definitions;

-- ============================================================================
-- DROP ENUM TYPES
-- ============================================================================

DROP TYPE IF EXISTS workflow_completion_outcome;
DROP TYPE IF EXISTS sla_tracking_status;
DROP TYPE IF EXISTS approval_mode;
DROP TYPE IF EXISTS workflow_action;
DROP TYPE IF EXISTS workflow_step_exec_status;
DROP TYPE IF EXISTS workflow_step_type;
DROP TYPE IF EXISTS workflow_instance_status;
DROP TYPE IF EXISTS workflow_status;
DROP TYPE IF EXISTS workflow_type;
