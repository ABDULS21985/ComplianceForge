-- Migration 028 DOWN: Business Impact Analysis & Business Continuity
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- process_dependencies_map
DROP POLICY IF EXISTS proc_deps_tenant_delete ON process_dependencies_map;
DROP POLICY IF EXISTS proc_deps_tenant_update ON process_dependencies_map;
DROP POLICY IF EXISTS proc_deps_tenant_insert ON process_dependencies_map;
DROP POLICY IF EXISTS proc_deps_tenant_select ON process_dependencies_map;

-- bc_exercises
DROP POLICY IF EXISTS bc_exercises_tenant_delete ON bc_exercises;
DROP POLICY IF EXISTS bc_exercises_tenant_update ON bc_exercises;
DROP POLICY IF EXISTS bc_exercises_tenant_insert ON bc_exercises;
DROP POLICY IF EXISTS bc_exercises_tenant_select ON bc_exercises;

-- continuity_plans
DROP POLICY IF EXISTS cont_plans_tenant_delete ON continuity_plans;
DROP POLICY IF EXISTS cont_plans_tenant_update ON continuity_plans;
DROP POLICY IF EXISTS cont_plans_tenant_insert ON continuity_plans;
DROP POLICY IF EXISTS cont_plans_tenant_select ON continuity_plans;

-- bia_scenarios
DROP POLICY IF EXISTS bia_scenarios_tenant_delete ON bia_scenarios;
DROP POLICY IF EXISTS bia_scenarios_tenant_update ON bia_scenarios;
DROP POLICY IF EXISTS bia_scenarios_tenant_insert ON bia_scenarios;
DROP POLICY IF EXISTS bia_scenarios_tenant_select ON bia_scenarios;

-- business_processes
DROP POLICY IF EXISTS biz_processes_tenant_delete ON business_processes;
DROP POLICY IF EXISTS biz_processes_tenant_update ON business_processes;
DROP POLICY IF EXISTS biz_processes_tenant_insert ON business_processes;
DROP POLICY IF EXISTS biz_processes_tenant_select ON business_processes;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_bc_exercises_generate_ref ON bc_exercises;
DROP TRIGGER IF EXISTS trg_bc_exercises_updated_at ON bc_exercises;
DROP TRIGGER IF EXISTS trg_cont_plans_generate_ref ON continuity_plans;
DROP TRIGGER IF EXISTS trg_cont_plans_updated_at ON continuity_plans;
DROP TRIGGER IF EXISTS trg_bia_scenarios_generate_ref ON bia_scenarios;
DROP TRIGGER IF EXISTS trg_bia_scenarios_updated_at ON bia_scenarios;
DROP TRIGGER IF EXISTS trg_biz_processes_generate_ref ON business_processes;
DROP TRIGGER IF EXISTS trg_biz_processes_updated_at ON business_processes;
DROP TRIGGER IF EXISTS trg_proc_deps_updated_at ON process_dependencies_map;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS process_dependencies_map;
DROP TABLE IF EXISTS bc_exercises;
DROP TABLE IF EXISTS continuity_plans;
DROP TABLE IF EXISTS bia_scenarios;
DROP TABLE IF EXISTS business_processes;

-- ============================================================================
-- DROP FUNCTIONS
-- ============================================================================

DROP FUNCTION IF EXISTS generate_bc_exercise_ref();
DROP FUNCTION IF EXISTS generate_continuity_plan_ref();
DROP FUNCTION IF EXISTS generate_bia_scenario_ref();
DROP FUNCTION IF EXISTS generate_business_process_ref();
