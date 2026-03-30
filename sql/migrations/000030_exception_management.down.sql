-- Migration 030 DOWN: Exception Management
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- exception_audit_trail
DROP POLICY IF EXISTS exc_audit_trail_tenant_delete ON exception_audit_trail;
DROP POLICY IF EXISTS exc_audit_trail_tenant_update ON exception_audit_trail;
DROP POLICY IF EXISTS exc_audit_trail_tenant_insert ON exception_audit_trail;
DROP POLICY IF EXISTS exc_audit_trail_tenant_select ON exception_audit_trail;

-- exception_reviews
DROP POLICY IF EXISTS exc_reviews_tenant_delete ON exception_reviews;
DROP POLICY IF EXISTS exc_reviews_tenant_update ON exception_reviews;
DROP POLICY IF EXISTS exc_reviews_tenant_insert ON exception_reviews;
DROP POLICY IF EXISTS exc_reviews_tenant_select ON exception_reviews;

-- compliance_exceptions
DROP POLICY IF EXISTS comp_exceptions_tenant_delete ON compliance_exceptions;
DROP POLICY IF EXISTS comp_exceptions_tenant_update ON compliance_exceptions;
DROP POLICY IF EXISTS comp_exceptions_tenant_insert ON compliance_exceptions;
DROP POLICY IF EXISTS comp_exceptions_tenant_select ON compliance_exceptions;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_comp_exceptions_generate_ref ON compliance_exceptions;
DROP TRIGGER IF EXISTS trg_comp_exceptions_updated_at ON compliance_exceptions;
DROP TRIGGER IF EXISTS trg_exc_audit_trail_immutable ON exception_audit_trail;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS exception_audit_trail;
DROP TABLE IF EXISTS exception_reviews;
DROP TABLE IF EXISTS compliance_exceptions;

-- ============================================================================
-- DROP FUNCTIONS
-- ============================================================================

DROP FUNCTION IF EXISTS generate_exception_ref();
DROP FUNCTION IF EXISTS prevent_audit_trail_modification();
