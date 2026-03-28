-- Rollback Migration 008: Drop control implementations, evidence, and test results.

-- Drop RLS policies
DROP POLICY IF EXISTS test_results_tenant_select ON control_test_results;
DROP POLICY IF EXISTS test_results_tenant_insert ON control_test_results;
DROP POLICY IF EXISTS test_results_tenant_update ON control_test_results;
DROP POLICY IF EXISTS test_results_tenant_delete ON control_test_results;

DROP POLICY IF EXISTS evidence_tenant_select ON control_evidence;
DROP POLICY IF EXISTS evidence_tenant_insert ON control_evidence;
DROP POLICY IF EXISTS evidence_tenant_update ON control_evidence;
DROP POLICY IF EXISTS evidence_tenant_delete ON control_evidence;

DROP POLICY IF EXISTS ctrl_impl_tenant_select ON control_implementations;
DROP POLICY IF EXISTS ctrl_impl_tenant_insert ON control_implementations;
DROP POLICY IF EXISTS ctrl_impl_tenant_update ON control_implementations;
DROP POLICY IF EXISTS ctrl_impl_tenant_delete ON control_implementations;

DROP POLICY IF EXISTS org_fw_tenant_select ON organization_frameworks;
DROP POLICY IF EXISTS org_fw_tenant_insert ON organization_frameworks;
DROP POLICY IF EXISTS org_fw_tenant_update ON organization_frameworks;
DROP POLICY IF EXISTS org_fw_tenant_delete ON organization_frameworks;

-- Drop triggers
DROP TRIGGER IF EXISTS trg_control_evidence_updated_at ON control_evidence;
DROP TRIGGER IF EXISTS trg_control_implementations_updated_at ON control_implementations;
DROP TRIGGER IF EXISTS trg_org_frameworks_updated_at ON organization_frameworks;

-- Drop tables in dependency order
DROP TABLE IF EXISTS control_test_results CASCADE;
DROP TABLE IF EXISTS control_evidence CASCADE;
DROP TABLE IF EXISTS control_implementations CASCADE;
DROP TABLE IF EXISTS organization_frameworks CASCADE;
