-- Rollback Migration 015: Drop policy analytics views.

DROP VIEW IF EXISTS v_attestation_campaign_progress CASCADE;
DROP VIEW IF EXISTS v_policy_review_calendar CASCADE;
DROP VIEW IF EXISTS v_policy_gap_analysis CASCADE;
DROP VIEW IF EXISTS v_policy_compliance_status CASCADE;
