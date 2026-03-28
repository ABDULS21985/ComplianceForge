-- Migration 029 DOWN: Advanced Analytics
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- analytics_custom_dashboards
DROP POLICY IF EXISTS custom_dashboards_tenant_delete ON analytics_custom_dashboards;
DROP POLICY IF EXISTS custom_dashboards_tenant_update ON analytics_custom_dashboards;
DROP POLICY IF EXISTS custom_dashboards_tenant_insert ON analytics_custom_dashboards;
DROP POLICY IF EXISTS custom_dashboards_tenant_select ON analytics_custom_dashboards;

-- analytics_risk_predictions
DROP POLICY IF EXISTS risk_predictions_tenant_delete ON analytics_risk_predictions;
DROP POLICY IF EXISTS risk_predictions_tenant_update ON analytics_risk_predictions;
DROP POLICY IF EXISTS risk_predictions_tenant_insert ON analytics_risk_predictions;
DROP POLICY IF EXISTS risk_predictions_tenant_select ON analytics_risk_predictions;

-- analytics_compliance_trends
DROP POLICY IF EXISTS comp_trends_tenant_delete ON analytics_compliance_trends;
DROP POLICY IF EXISTS comp_trends_tenant_update ON analytics_compliance_trends;
DROP POLICY IF EXISTS comp_trends_tenant_insert ON analytics_compliance_trends;
DROP POLICY IF EXISTS comp_trends_tenant_select ON analytics_compliance_trends;

-- analytics_snapshots
DROP POLICY IF EXISTS analytics_snapshots_tenant_delete ON analytics_snapshots;
DROP POLICY IF EXISTS analytics_snapshots_tenant_update ON analytics_snapshots;
DROP POLICY IF EXISTS analytics_snapshots_tenant_insert ON analytics_snapshots;
DROP POLICY IF EXISTS analytics_snapshots_tenant_select ON analytics_snapshots;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_custom_dashboards_updated_at ON analytics_custom_dashboards;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS analytics_widget_types;
DROP TABLE IF EXISTS analytics_custom_dashboards;
DROP TABLE IF EXISTS analytics_benchmarks;
DROP TABLE IF EXISTS analytics_risk_predictions;
DROP TABLE IF EXISTS analytics_compliance_trends;
DROP TABLE IF EXISTS analytics_snapshots;
