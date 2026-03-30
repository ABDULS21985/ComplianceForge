-- Migration 039 DOWN: White-Labelling & Branding
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- partner_tenant_mappings
DROP POLICY IF EXISTS partner_tenant_mappings_tenant_delete ON partner_tenant_mappings;
DROP POLICY IF EXISTS partner_tenant_mappings_tenant_update ON partner_tenant_mappings;
DROP POLICY IF EXISTS partner_tenant_mappings_tenant_insert ON partner_tenant_mappings;
DROP POLICY IF EXISTS partner_tenant_mappings_tenant_select ON partner_tenant_mappings;

-- tenant_branding
DROP POLICY IF EXISTS tenant_branding_tenant_delete ON tenant_branding;
DROP POLICY IF EXISTS tenant_branding_tenant_update ON tenant_branding;
DROP POLICY IF EXISTS tenant_branding_tenant_insert ON tenant_branding;
DROP POLICY IF EXISTS tenant_branding_tenant_select ON tenant_branding;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_white_label_partners_updated_at ON white_label_partners;
DROP TRIGGER IF EXISTS trg_tenant_branding_updated_at ON tenant_branding;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS partner_tenant_mappings;
DROP TABLE IF EXISTS white_label_partners;
DROP TABLE IF EXISTS tenant_branding;
