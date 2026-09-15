DROP TRIGGER IF EXISTS trg_vendors_legacy_hold_soft_delete ON vendors;
DROP TRIGGER IF EXISTS trg_vendors_legacy_hold_hard_delete ON vendors;
DROP TRIGGER IF EXISTS trg_vendors_governed_soft_delete ON vendors;
DROP TRIGGER IF EXISTS trg_vendors_governed_hard_delete ON vendors;
DROP TRIGGER IF EXISTS trg_risks_governed_soft_delete ON risks;
DROP TRIGGER IF EXISTS trg_risks_governed_hard_delete ON risks;
DROP TRIGGER IF EXISTS trg_report_runs_governed_hard_delete ON report_runs;
DROP TRIGGER IF EXISTS trg_policies_governed_soft_delete ON policies;
DROP TRIGGER IF EXISTS trg_policies_governed_hard_delete ON policies;
DROP TRIGGER IF EXISTS trg_incidents_legacy_hold_soft_delete ON incidents;
DROP TRIGGER IF EXISTS trg_incidents_legacy_hold_hard_delete ON incidents;
DROP TRIGGER IF EXISTS trg_incidents_governed_soft_delete ON incidents;
DROP TRIGGER IF EXISTS trg_incidents_governed_hard_delete ON incidents;
DROP TRIGGER IF EXISTS trg_control_evidence_governed_soft_delete ON control_evidence;
DROP TRIGGER IF EXISTS trg_control_evidence_governed_hard_delete ON control_evidence;
DROP TRIGGER IF EXISTS trg_control_implementations_governed_soft_delete ON control_implementations;
DROP TRIGGER IF EXISTS trg_control_implementations_governed_hard_delete ON control_implementations;
DROP TRIGGER IF EXISTS trg_comments_governed_soft_delete ON comments;
DROP TRIGGER IF EXISTS trg_comments_governed_hard_delete ON comments;
DROP TRIGGER IF EXISTS trg_audit_findings_governed_soft_delete ON audit_findings;
DROP TRIGGER IF EXISTS trg_audit_findings_governed_hard_delete ON audit_findings;
DROP TRIGGER IF EXISTS trg_audits_governed_soft_delete ON audits;
DROP TRIGGER IF EXISTS trg_audits_governed_hard_delete ON audits;
DROP TRIGGER IF EXISTS trg_assets_governed_soft_delete ON assets;
DROP TRIGGER IF EXISTS trg_assets_governed_hard_delete ON assets;

DROP FUNCTION IF EXISTS enforce_governed_record_disposition();
DROP FUNCTION IF EXISTS prevent_legacy_legal_hold_disposition();
DROP TRIGGER IF EXISTS trg_data_governance_events_immutable ON data_governance_events;
DROP TRIGGER IF EXISTS trg_data_governance_event_chain ON data_governance_events;
DROP FUNCTION IF EXISTS prevent_data_governance_event_mutation();
DROP FUNCTION IF EXISTS append_data_governance_event_chain();
DROP FUNCTION IF EXISTS calculate_data_governance_event_hash(
    UUID, BIGINT, BYTEA, TEXT, UUID, TEXT, UUID, TEXT, JSONB, JSONB,
    TEXT, TEXT, TIMESTAMPTZ
);

DROP TABLE IF EXISTS data_governance_events;
DROP TABLE IF EXISTS data_governance_event_chain_heads;
DROP TABLE IF EXISTS legal_hold_records;
DROP TABLE IF EXISTS legal_hold_custodians;
DROP TABLE IF EXISTS legal_holds;
DROP TABLE IF EXISTS legal_hold_reference_sequences;
DROP TABLE IF EXISTS retention_exceptions;
DROP TABLE IF EXISTS record_retention_assignments;
DROP TABLE IF EXISTS retention_schedules;
DROP TABLE IF EXISTS tenant_data_governance_policies;
DROP FUNCTION IF EXISTS data_region_array_is_valid(TEXT[]);
