-- Migration 026 DOWN: Marketplace
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- marketplace_reviews
DROP POLICY IF EXISTS mp_reviews_tenant_delete ON marketplace_reviews;
DROP POLICY IF EXISTS mp_reviews_tenant_update ON marketplace_reviews;
DROP POLICY IF EXISTS mp_reviews_tenant_insert ON marketplace_reviews;
DROP POLICY IF EXISTS mp_reviews_tenant_select ON marketplace_reviews;

-- marketplace_installations
DROP POLICY IF EXISTS mp_installations_tenant_delete ON marketplace_installations;
DROP POLICY IF EXISTS mp_installations_tenant_update ON marketplace_installations;
DROP POLICY IF EXISTS mp_installations_tenant_insert ON marketplace_installations;
DROP POLICY IF EXISTS mp_installations_tenant_select ON marketplace_installations;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_mp_reviews_updated_at ON marketplace_reviews;
DROP TRIGGER IF EXISTS trg_mp_installations_updated_at ON marketplace_installations;
DROP TRIGGER IF EXISTS trg_mp_packages_updated_at ON marketplace_packages;
DROP TRIGGER IF EXISTS trg_mp_publishers_updated_at ON marketplace_publishers;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS marketplace_reviews;
DROP TABLE IF EXISTS marketplace_installations;
DROP TABLE IF EXISTS marketplace_package_versions;
DROP TABLE IF EXISTS marketplace_packages;
DROP TABLE IF EXISTS marketplace_publishers;
