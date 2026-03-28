-- Rollback Migration 009: Drop compliance analytics views.

DROP VIEW IF EXISTS v_evidence_expiry_tracker CASCADE;
DROP VIEW IF EXISTS v_framework_summary CASCADE;
DROP VIEW IF EXISTS v_cross_framework_coverage CASCADE;
DROP VIEW IF EXISTS v_control_gap_analysis CASCADE;
DROP VIEW IF EXISTS v_compliance_score_by_framework CASCADE;
