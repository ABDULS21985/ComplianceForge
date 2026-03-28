-- Rollback Migration 007: Drop compliance frameworks schema.

-- Drop RLS policies
DROP POLICY IF EXISTS frameworks_tenant_select ON compliance_frameworks;
DROP POLICY IF EXISTS frameworks_tenant_insert ON compliance_frameworks;
DROP POLICY IF EXISTS frameworks_tenant_update ON compliance_frameworks;
DROP POLICY IF EXISTS frameworks_tenant_delete ON compliance_frameworks;

-- Drop triggers
DROP TRIGGER IF EXISTS trg_control_mappings_updated_at ON framework_control_mappings;
DROP TRIGGER IF EXISTS trg_framework_controls_updated_at ON framework_controls;
DROP TRIGGER IF EXISTS trg_framework_domains_updated_at ON framework_domains;
DROP TRIGGER IF EXISTS trg_compliance_frameworks_updated_at ON compliance_frameworks;

-- Drop tables in dependency order
DROP TABLE IF EXISTS framework_control_mappings CASCADE;
DROP TABLE IF EXISTS framework_controls CASCADE;
DROP TABLE IF EXISTS framework_domains CASCADE;
DROP TABLE IF EXISTS compliance_frameworks CASCADE;
