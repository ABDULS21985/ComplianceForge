-- Migration 033 DOWN: Data Classification & Records of Processing Activities (ROPA)
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- ropa_exports
DROP POLICY IF EXISTS ropa_exports_tenant_delete ON ropa_exports;
DROP POLICY IF EXISTS ropa_exports_tenant_update ON ropa_exports;
DROP POLICY IF EXISTS ropa_exports_tenant_insert ON ropa_exports;
DROP POLICY IF EXISTS ropa_exports_tenant_select ON ropa_exports;

-- data_flow_maps
DROP POLICY IF EXISTS data_flows_tenant_delete ON data_flow_maps;
DROP POLICY IF EXISTS data_flows_tenant_update ON data_flow_maps;
DROP POLICY IF EXISTS data_flows_tenant_insert ON data_flow_maps;
DROP POLICY IF EXISTS data_flows_tenant_select ON data_flow_maps;

-- processing_activities
DROP POLICY IF EXISTS proc_activities_tenant_delete ON processing_activities;
DROP POLICY IF EXISTS proc_activities_tenant_update ON processing_activities;
DROP POLICY IF EXISTS proc_activities_tenant_insert ON processing_activities;
DROP POLICY IF EXISTS proc_activities_tenant_select ON processing_activities;

-- data_categories
DROP POLICY IF EXISTS data_cat_tenant_delete ON data_categories;
DROP POLICY IF EXISTS data_cat_tenant_update ON data_categories;
DROP POLICY IF EXISTS data_cat_tenant_insert ON data_categories;
DROP POLICY IF EXISTS data_cat_tenant_select ON data_categories;

-- data_classifications
DROP POLICY IF EXISTS data_class_tenant_delete ON data_classifications;
DROP POLICY IF EXISTS data_class_tenant_update ON data_classifications;
DROP POLICY IF EXISTS data_class_tenant_insert ON data_classifications;
DROP POLICY IF EXISTS data_class_tenant_select ON data_classifications;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_ropa_exports_generate_ref ON ropa_exports;
DROP TRIGGER IF EXISTS trg_proc_activities_generate_ref ON processing_activities;
DROP TRIGGER IF EXISTS trg_proc_activities_updated_at ON processing_activities;
DROP TRIGGER IF EXISTS trg_data_flows_updated_at ON data_flow_maps;
DROP TRIGGER IF EXISTS trg_data_cat_updated_at ON data_categories;
DROP TRIGGER IF EXISTS trg_data_class_updated_at ON data_classifications;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS ropa_exports;
DROP TABLE IF EXISTS data_flow_maps;
DROP TABLE IF EXISTS processing_activities;
DROP TABLE IF EXISTS data_categories;
DROP TABLE IF EXISTS data_classifications;

-- ============================================================================
-- DROP FUNCTIONS
-- ============================================================================

DROP FUNCTION IF EXISTS generate_ropa_export_ref();
DROP FUNCTION IF EXISTS generate_processing_activity_ref();
