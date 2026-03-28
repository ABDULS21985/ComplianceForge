-- Migration 024 DOWN: Attribute-Based Access Control (ABAC)
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- field_level_permissions
DROP POLICY IF EXISTS field_perms_tenant_delete ON field_level_permissions;
DROP POLICY IF EXISTS field_perms_tenant_update ON field_level_permissions;
DROP POLICY IF EXISTS field_perms_tenant_insert ON field_level_permissions;
DROP POLICY IF EXISTS field_perms_tenant_select ON field_level_permissions;

-- access_audit_log
DROP POLICY IF EXISTS access_audit_tenant_delete ON access_audit_log;
DROP POLICY IF EXISTS access_audit_tenant_update ON access_audit_log;
DROP POLICY IF EXISTS access_audit_tenant_insert ON access_audit_log;
DROP POLICY IF EXISTS access_audit_tenant_select ON access_audit_log;

-- access_policy_assignments
DROP POLICY IF EXISTS policy_assignments_tenant_delete ON access_policy_assignments;
DROP POLICY IF EXISTS policy_assignments_tenant_update ON access_policy_assignments;
DROP POLICY IF EXISTS policy_assignments_tenant_insert ON access_policy_assignments;
DROP POLICY IF EXISTS policy_assignments_tenant_select ON access_policy_assignments;

-- access_policies
DROP POLICY IF EXISTS access_policies_tenant_delete ON access_policies;
DROP POLICY IF EXISTS access_policies_tenant_update ON access_policies;
DROP POLICY IF EXISTS access_policies_tenant_insert ON access_policies;
DROP POLICY IF EXISTS access_policies_tenant_select ON access_policies;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_access_audit_no_delete ON access_audit_log;
DROP TRIGGER IF EXISTS trg_access_audit_no_update ON access_audit_log;
DROP TRIGGER IF EXISTS trg_access_policies_updated_at ON access_policies;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS field_level_permissions;
DROP TABLE IF EXISTS access_audit_log;
DROP TABLE IF EXISTS access_policy_assignments;
DROP TABLE IF EXISTS access_policies;

-- ============================================================================
-- DROP FUNCTIONS
-- ============================================================================

DROP FUNCTION IF EXISTS prevent_access_audit_modification();
