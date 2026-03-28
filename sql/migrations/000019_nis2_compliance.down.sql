-- Rollback Migration 019: Drop NIS2 Compliance Automation tables, functions, and types.

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- nis2_management_accountability
DROP POLICY IF EXISTS nis2_mgmt_tenant_select ON nis2_management_accountability;
DROP POLICY IF EXISTS nis2_mgmt_tenant_insert ON nis2_management_accountability;
DROP POLICY IF EXISTS nis2_mgmt_tenant_update ON nis2_management_accountability;
DROP POLICY IF EXISTS nis2_mgmt_tenant_delete ON nis2_management_accountability;

-- nis2_security_measures
DROP POLICY IF EXISTS nis2_measures_tenant_select ON nis2_security_measures;
DROP POLICY IF EXISTS nis2_measures_tenant_insert ON nis2_security_measures;
DROP POLICY IF EXISTS nis2_measures_tenant_update ON nis2_security_measures;
DROP POLICY IF EXISTS nis2_measures_tenant_delete ON nis2_security_measures;

-- nis2_incident_reports
DROP POLICY IF EXISTS nis2_incident_tenant_select ON nis2_incident_reports;
DROP POLICY IF EXISTS nis2_incident_tenant_insert ON nis2_incident_reports;
DROP POLICY IF EXISTS nis2_incident_tenant_update ON nis2_incident_reports;
DROP POLICY IF EXISTS nis2_incident_tenant_delete ON nis2_incident_reports;

-- nis2_entity_assessment
DROP POLICY IF EXISTS nis2_entity_tenant_select ON nis2_entity_assessment;
DROP POLICY IF EXISTS nis2_entity_tenant_insert ON nis2_entity_assessment;
DROP POLICY IF EXISTS nis2_entity_tenant_update ON nis2_entity_assessment;
DROP POLICY IF EXISTS nis2_entity_tenant_delete ON nis2_entity_assessment;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_nis2_management_accountability_updated_at ON nis2_management_accountability;
DROP TRIGGER IF EXISTS trg_nis2_security_measures_updated_at ON nis2_security_measures;
DROP TRIGGER IF EXISTS trg_nis2_incident_reports_updated_at ON nis2_incident_reports;
DROP TRIGGER IF EXISTS trg_nis2_incident_report_generate_ref ON nis2_incident_reports;
DROP TRIGGER IF EXISTS trg_nis2_entity_assessment_updated_at ON nis2_entity_assessment;

-- ============================================================================
-- DROP FUNCTIONS
-- ============================================================================

DROP FUNCTION IF EXISTS nis2_incident_report_generate_ref();

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS nis2_management_accountability CASCADE;
DROP TABLE IF EXISTS nis2_security_measures CASCADE;
DROP TABLE IF EXISTS nis2_incident_reports CASCADE;
DROP TABLE IF EXISTS nis2_entity_assessment CASCADE;

-- ============================================================================
-- DROP ENUM TYPES
-- ============================================================================

DROP TYPE IF EXISTS nis2_measure_status;
DROP TYPE IF EXISTS nis2_report_phase_status;
DROP TYPE IF EXISTS nis2_entity_type;
