-- Migration 027 DOWN: Regulatory Change Management
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- regulatory_impact_assessments
DROP POLICY IF EXISTS reg_impact_tenant_delete ON regulatory_impact_assessments;
DROP POLICY IF EXISTS reg_impact_tenant_update ON regulatory_impact_assessments;
DROP POLICY IF EXISTS reg_impact_tenant_insert ON regulatory_impact_assessments;
DROP POLICY IF EXISTS reg_impact_tenant_select ON regulatory_impact_assessments;

-- regulatory_subscriptions
DROP POLICY IF EXISTS reg_subs_tenant_delete ON regulatory_subscriptions;
DROP POLICY IF EXISTS reg_subs_tenant_update ON regulatory_subscriptions;
DROP POLICY IF EXISTS reg_subs_tenant_insert ON regulatory_subscriptions;
DROP POLICY IF EXISTS reg_subs_tenant_select ON regulatory_subscriptions;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_reg_changes_generate_ref ON regulatory_changes;
DROP TRIGGER IF EXISTS trg_reg_impact_updated_at ON regulatory_impact_assessments;
DROP TRIGGER IF EXISTS trg_reg_changes_updated_at ON regulatory_changes;
DROP TRIGGER IF EXISTS trg_reg_sources_updated_at ON regulatory_sources;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS regulatory_impact_assessments;
DROP TABLE IF EXISTS regulatory_subscriptions;
DROP TABLE IF EXISTS regulatory_changes;
DROP TABLE IF EXISTS regulatory_sources;

-- ============================================================================
-- DROP FUNCTIONS
-- ============================================================================

DROP FUNCTION IF EXISTS generate_regulatory_change_ref();
