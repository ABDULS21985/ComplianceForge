-- Migration 020 DOWN: Continuous Monitoring & Evidence Collection
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- compliance_drift_events
DROP POLICY IF EXISTS drift_events_tenant_delete ON compliance_drift_events;
DROP POLICY IF EXISTS drift_events_tenant_update ON compliance_drift_events;
DROP POLICY IF EXISTS drift_events_tenant_insert ON compliance_drift_events;
DROP POLICY IF EXISTS drift_events_tenant_select ON compliance_drift_events;

-- compliance_monitor_results
DROP POLICY IF EXISTS comp_mon_results_tenant_delete ON compliance_monitor_results;
DROP POLICY IF EXISTS comp_mon_results_tenant_update ON compliance_monitor_results;
DROP POLICY IF EXISTS comp_mon_results_tenant_insert ON compliance_monitor_results;
DROP POLICY IF EXISTS comp_mon_results_tenant_select ON compliance_monitor_results;

-- compliance_monitors
DROP POLICY IF EXISTS comp_monitors_tenant_delete ON compliance_monitors;
DROP POLICY IF EXISTS comp_monitors_tenant_update ON compliance_monitors;
DROP POLICY IF EXISTS comp_monitors_tenant_insert ON compliance_monitors;
DROP POLICY IF EXISTS comp_monitors_tenant_select ON compliance_monitors;

-- evidence_collection_runs
DROP POLICY IF EXISTS ev_collect_runs_tenant_delete ON evidence_collection_runs;
DROP POLICY IF EXISTS ev_collect_runs_tenant_update ON evidence_collection_runs;
DROP POLICY IF EXISTS ev_collect_runs_tenant_insert ON evidence_collection_runs;
DROP POLICY IF EXISTS ev_collect_runs_tenant_select ON evidence_collection_runs;

-- evidence_collection_configs
DROP POLICY IF EXISTS ev_collect_cfg_tenant_delete ON evidence_collection_configs;
DROP POLICY IF EXISTS ev_collect_cfg_tenant_update ON evidence_collection_configs;
DROP POLICY IF EXISTS ev_collect_cfg_tenant_insert ON evidence_collection_configs;
DROP POLICY IF EXISTS ev_collect_cfg_tenant_select ON evidence_collection_configs;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_comp_monitors_updated_at ON compliance_monitors;
DROP TRIGGER IF EXISTS trg_ev_collect_cfg_updated_at ON evidence_collection_configs;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS compliance_drift_events;
DROP TABLE IF EXISTS compliance_monitor_results;
DROP TABLE IF EXISTS compliance_monitors;
DROP TABLE IF EXISTS evidence_collection_runs;
DROP TABLE IF EXISTS evidence_collection_configs;

-- ============================================================================
-- DROP ENUM TYPES
-- ============================================================================

DROP TYPE IF EXISTS drift_type;
DROP TYPE IF EXISTS monitor_check_status;
DROP TYPE IF EXISTS monitor_type;
DROP TYPE IF EXISTS collection_run_status;
DROP TYPE IF EXISTS collection_method;
