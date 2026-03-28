-- Rollback Migration 013: Drop policy management core tables.

-- Drop RLS policies
DROP POLICY IF EXISTS pv_tenant_select ON policy_versions;
DROP POLICY IF EXISTS pv_tenant_insert ON policy_versions;
DROP POLICY IF EXISTS pv_tenant_update ON policy_versions;
DROP POLICY IF EXISTS pv_tenant_delete ON policy_versions;

DROP POLICY IF EXISTS policies_tenant_select ON policies;
DROP POLICY IF EXISTS policies_tenant_insert ON policies;
DROP POLICY IF EXISTS policies_tenant_update ON policies;
DROP POLICY IF EXISTS policies_tenant_delete ON policies;

DROP POLICY IF EXISTS pt_tenant_select ON policy_translations;
DROP POLICY IF EXISTS pt_tenant_insert ON policy_translations;
DROP POLICY IF EXISTS pt_tenant_update ON policy_translations;
DROP POLICY IF EXISTS pt_tenant_delete ON policy_translations;

DROP POLICY IF EXISTS policy_cat_tenant_select ON policy_categories;
DROP POLICY IF EXISTS policy_cat_tenant_insert ON policy_categories;
DROP POLICY IF EXISTS policy_cat_tenant_update ON policy_categories;
DROP POLICY IF EXISTS policy_cat_tenant_delete ON policy_categories;

-- Drop triggers and functions
DROP TRIGGER IF EXISTS trg_policy_version_on_insert ON policy_versions;
DROP TRIGGER IF EXISTS trg_policies_on_publish ON policies;
DROP TRIGGER IF EXISTS trg_policies_search_vector ON policies;
DROP TRIGGER IF EXISTS trg_policies_generate_ref ON policies;
DROP TRIGGER IF EXISTS trg_policies_updated_at ON policies;
DROP TRIGGER IF EXISTS trg_policy_versions_updated_at ON policy_versions;
DROP TRIGGER IF EXISTS trg_policy_translations_updated_at ON policy_translations;
DROP TRIGGER IF EXISTS trg_policy_categories_updated_at ON policy_categories;

DROP FUNCTION IF EXISTS policy_version_on_insert();
DROP FUNCTION IF EXISTS policy_on_publish();
DROP FUNCTION IF EXISTS policies_search_vector_update();
DROP FUNCTION IF EXISTS generate_policy_ref();

-- Drop tables in dependency order
DROP TABLE IF EXISTS policy_translations CASCADE;
DROP TABLE IF EXISTS policy_versions CASCADE;
DROP TABLE IF EXISTS policies CASCADE;
DROP TABLE IF EXISTS policy_categories CASCADE;
