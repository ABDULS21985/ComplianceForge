-- Migration 031 DOWN: Evidence Templates & Testing
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- evidence_test_runs
DROP POLICY IF EXISTS evidence_runs_tenant_delete ON evidence_test_runs;
DROP POLICY IF EXISTS evidence_runs_tenant_update ON evidence_test_runs;
DROP POLICY IF EXISTS evidence_runs_tenant_insert ON evidence_test_runs;
DROP POLICY IF EXISTS evidence_runs_tenant_select ON evidence_test_runs;

-- evidence_test_cases
DROP POLICY IF EXISTS evidence_cases_tenant_delete ON evidence_test_cases;
DROP POLICY IF EXISTS evidence_cases_tenant_update ON evidence_test_cases;
DROP POLICY IF EXISTS evidence_cases_tenant_insert ON evidence_test_cases;
DROP POLICY IF EXISTS evidence_cases_tenant_select ON evidence_test_cases;

-- evidence_test_suites
DROP POLICY IF EXISTS evidence_suites_tenant_delete ON evidence_test_suites;
DROP POLICY IF EXISTS evidence_suites_tenant_update ON evidence_test_suites;
DROP POLICY IF EXISTS evidence_suites_tenant_insert ON evidence_test_suites;
DROP POLICY IF EXISTS evidence_suites_tenant_select ON evidence_test_suites;

-- evidence_requirements
DROP POLICY IF EXISTS evidence_reqs_tenant_delete ON evidence_requirements;
DROP POLICY IF EXISTS evidence_reqs_tenant_update ON evidence_requirements;
DROP POLICY IF EXISTS evidence_reqs_tenant_insert ON evidence_requirements;
DROP POLICY IF EXISTS evidence_reqs_tenant_select ON evidence_requirements;

-- evidence_templates
DROP POLICY IF EXISTS evidence_templates_tenant_delete ON evidence_templates;
DROP POLICY IF EXISTS evidence_templates_tenant_update ON evidence_templates;
DROP POLICY IF EXISTS evidence_templates_tenant_insert ON evidence_templates;
DROP POLICY IF EXISTS evidence_templates_tenant_select ON evidence_templates;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_evidence_templates_updated_at ON evidence_templates;
DROP TRIGGER IF EXISTS trg_evidence_reqs_updated_at ON evidence_requirements;
DROP TRIGGER IF EXISTS trg_evidence_suites_updated_at ON evidence_test_suites;
DROP TRIGGER IF EXISTS trg_evidence_cases_updated_at ON evidence_test_cases;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS evidence_test_runs;
DROP TABLE IF EXISTS evidence_test_cases;
DROP TABLE IF EXISTS evidence_test_suites;
DROP TABLE IF EXISTS evidence_requirements;
DROP TABLE IF EXISTS evidence_templates;
