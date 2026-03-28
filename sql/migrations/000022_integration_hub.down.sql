-- Migration 022: Integration Hub (rollback)
-- ComplianceForge GRC Platform

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- api_keys
DROP POLICY IF EXISTS api_keys_tenant_isolation_delete ON api_keys;
DROP POLICY IF EXISTS api_keys_tenant_isolation_update ON api_keys;
DROP POLICY IF EXISTS api_keys_tenant_isolation_insert ON api_keys;
DROP POLICY IF EXISTS api_keys_tenant_isolation_select ON api_keys;

-- sso_configurations
DROP POLICY IF EXISTS sso_config_tenant_isolation_delete ON sso_configurations;
DROP POLICY IF EXISTS sso_config_tenant_isolation_update ON sso_configurations;
DROP POLICY IF EXISTS sso_config_tenant_isolation_insert ON sso_configurations;
DROP POLICY IF EXISTS sso_config_tenant_isolation_select ON sso_configurations;

-- integration_sync_logs
DROP POLICY IF EXISTS sync_logs_tenant_isolation_delete ON integration_sync_logs;
DROP POLICY IF EXISTS sync_logs_tenant_isolation_update ON integration_sync_logs;
DROP POLICY IF EXISTS sync_logs_tenant_isolation_insert ON integration_sync_logs;
DROP POLICY IF EXISTS sync_logs_tenant_isolation_select ON integration_sync_logs;

-- integrations
DROP POLICY IF EXISTS integrations_tenant_isolation_delete ON integrations;
DROP POLICY IF EXISTS integrations_tenant_isolation_update ON integrations;
DROP POLICY IF EXISTS integrations_tenant_isolation_insert ON integrations;
DROP POLICY IF EXISTS integrations_tenant_isolation_select ON integrations;

-- ============================================================================
-- DROP TABLES (reverse order of creation for FK safety)
-- ============================================================================

DROP TABLE IF EXISTS api_keys CASCADE;
DROP TABLE IF EXISTS sso_configurations CASCADE;
DROP TABLE IF EXISTS integration_sync_logs CASCADE;
DROP TABLE IF EXISTS integrations CASCADE;

-- ============================================================================
-- DROP ENUM TYPES
-- ============================================================================

DROP TYPE IF EXISTS sso_protocol;
DROP TYPE IF EXISTS sync_status;
DROP TYPE IF EXISTS integration_health;
DROP TYPE IF EXISTS integration_status;
DROP TYPE IF EXISTS integration_type;
