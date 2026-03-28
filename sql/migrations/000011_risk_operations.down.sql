-- Rollback Migration 011: Drop risk operations tables.

-- Drop RLS policies
DROP POLICY IF EXISTS scenario_tenant_select ON risk_scenarios;
DROP POLICY IF EXISTS scenario_tenant_insert ON risk_scenarios;
DROP POLICY IF EXISTS scenario_tenant_update ON risk_scenarios;
DROP POLICY IF EXISTS scenario_tenant_delete ON risk_scenarios;

DROP POLICY IF EXISTS rcm_tenant_select ON risk_control_mappings;
DROP POLICY IF EXISTS rcm_tenant_insert ON risk_control_mappings;
DROP POLICY IF EXISTS rcm_tenant_update ON risk_control_mappings;
DROP POLICY IF EXISTS rcm_tenant_delete ON risk_control_mappings;

DROP POLICY IF EXISTS kri_val_tenant_select ON risk_indicator_values;
DROP POLICY IF EXISTS kri_val_tenant_insert ON risk_indicator_values;
DROP POLICY IF EXISTS kri_val_tenant_update ON risk_indicator_values;
DROP POLICY IF EXISTS kri_val_tenant_delete ON risk_indicator_values;

DROP POLICY IF EXISTS kri_tenant_select ON risk_indicators;
DROP POLICY IF EXISTS kri_tenant_insert ON risk_indicators;
DROP POLICY IF EXISTS kri_tenant_update ON risk_indicators;
DROP POLICY IF EXISTS kri_tenant_delete ON risk_indicators;

DROP POLICY IF EXISTS treatment_tenant_select ON risk_treatments;
DROP POLICY IF EXISTS treatment_tenant_insert ON risk_treatments;
DROP POLICY IF EXISTS treatment_tenant_update ON risk_treatments;
DROP POLICY IF EXISTS treatment_tenant_delete ON risk_treatments;

DROP POLICY IF EXISTS risk_assess_tenant_select ON risk_assessments;
DROP POLICY IF EXISTS risk_assess_tenant_insert ON risk_assessments;
DROP POLICY IF EXISTS risk_assess_tenant_update ON risk_assessments;
DROP POLICY IF EXISTS risk_assess_tenant_delete ON risk_assessments;

-- Drop triggers
DROP TRIGGER IF EXISTS trg_risk_scenarios_updated_at ON risk_scenarios;
DROP TRIGGER IF EXISTS trg_risk_control_mappings_updated_at ON risk_control_mappings;
DROP TRIGGER IF EXISTS trg_risk_indicators_updated_at ON risk_indicators;
DROP TRIGGER IF EXISTS trg_risk_treatments_updated_at ON risk_treatments;

-- Drop tables in dependency order
DROP TABLE IF EXISTS risk_scenarios CASCADE;
DROP TABLE IF EXISTS risk_control_mappings CASCADE;
DROP TABLE IF EXISTS risk_indicator_values CASCADE;
DROP TABLE IF EXISTS risk_indicators CASCADE;
DROP TABLE IF EXISTS risk_treatments CASCADE;
DROP TABLE IF EXISTS risk_assessments CASCADE;
