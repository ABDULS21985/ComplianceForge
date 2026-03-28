-- Rollback Migration 012: Drop risk analytics views.

DROP VIEW IF EXISTS v_risk_appetite_compliance CASCADE;
DROP VIEW IF EXISTS v_top_risks CASCADE;
DROP VIEW IF EXISTS v_risk_control_coverage CASCADE;
DROP VIEW IF EXISTS v_kri_dashboard CASCADE;
DROP VIEW IF EXISTS v_risk_treatment_progress CASCADE;
DROP VIEW IF EXISTS v_risk_heatmap CASCADE;
