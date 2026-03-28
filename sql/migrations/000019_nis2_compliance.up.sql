-- Migration 019: NIS2 Compliance Automation
-- ComplianceForge GRC Platform
--
-- Implements the EU Directive 2022/2555 (NIS2) compliance module, covering:
--   - Entity assessment: Determines whether an organization is "essential" or
--     "important" under NIS2 based on sector, size, and turnover thresholds
--     (Articles 2-3). One assessment per organization.
--   - Incident reporting: Enforces the three-phase mandatory reporting timeline
--     to the designated CSIRT / competent authority (Article 23):
--       Phase 1 — Early warning within 24 hours of becoming aware
--       Phase 2 — Incident notification within 72 hours
--       Phase 3 — Final report within one month
--   - Security measures: Tracks implementation of the ten minimum cybersecurity
--     risk-management measures required by Article 21, with links to existing
--     control implementations for traceability.
--   - Management accountability: Records Article 20 obligations — board members
--     must approve risk measures and undergo cybersecurity training.
--
-- Design decisions:
--   - CSIRT contact details are stored per organization (entity_assessment) rather
--     than in a shared reference table, because NIS2 assigns CSIRTs per member
--     state and an organization may interact with different CSIRTs over time.
--   - Incident report deadlines are stored as absolute timestamps (not intervals)
--     so deadline queries are simple comparisons without computation.
--   - report_ref uses NIS2-YYYY-NNNN format, auto-generated per org per year.
--   - linked_control_ids in security_measures is UUID[] rather than a junction
--     table — the array is append-only and queried via GIN, keeping the schema
--     compact for a bounded cardinality (typically 1-10 linked controls).
--   - All tables enforce RLS via get_current_tenant() consistent with the rest
--     of the platform.

-- ============================================================================
-- ENUM TYPES
-- ============================================================================

-- NIS2 classifies entities into "essential" and "important" categories based on
-- sector, size, and criticality. Organizations outside scope are "not_applicable".
CREATE TYPE nis2_entity_type AS ENUM (
    'essential',
    'important',
    'not_applicable'
);

-- Tracks the status of each phase of the mandatory incident reporting timeline.
-- "not_required" allows marking phases that are waived (e.g., if the incident
-- does not meet the significance threshold for a particular phase).
CREATE TYPE nis2_report_phase_status AS ENUM (
    'not_required',
    'pending',
    'submitted',
    'overdue'
);

-- Implementation lifecycle for Article 21 security measures.
CREATE TYPE nis2_measure_status AS ENUM (
    'not_started',
    'in_progress',
    'implemented',
    'verified'
);

-- ============================================================================
-- TABLE: nis2_entity_assessment
-- ============================================================================
-- Captures the NIS2 scoping assessment for an organization: entity type,
-- sector classification, size criteria, and the designated CSIRT / competent
-- authority in the relevant EU member state.
-- One row per organization (UNIQUE constraint).

CREATE TABLE nis2_entity_assessment (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,

    -- Classification
    entity_type             nis2_entity_type NOT NULL,
    sector                  VARCHAR(200) NOT NULL,
    sub_sector              VARCHAR(200),
    assessment_criteria     JSONB NOT NULL DEFAULT '{}',

    -- Size thresholds (Article 2 criteria)
    employee_count          INT,
    annual_turnover_eur     DECIMAL(15,2),

    -- Assessment metadata
    assessment_date         DATE NOT NULL DEFAULT CURRENT_DATE,
    assessed_by             UUID REFERENCES users(id) ON DELETE SET NULL,
    is_in_scope             BOOLEAN NOT NULL DEFAULT false,

    -- Member state & authority
    member_state            VARCHAR(5),
    competent_authority     VARCHAR(200),
    csirt_name              VARCHAR(200),
    csirt_contact_email     VARCHAR(200),
    csirt_reporting_url     TEXT,

    notes                   TEXT,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One assessment per organization
    CONSTRAINT uq_nis2_entity_org UNIQUE (organization_id)
);

CREATE INDEX idx_nis2_entity_org ON nis2_entity_assessment(organization_id);
CREATE INDEX idx_nis2_entity_type ON nis2_entity_assessment(entity_type);
CREATE INDEX idx_nis2_entity_scope ON nis2_entity_assessment(is_in_scope) WHERE is_in_scope = true;
CREATE INDEX idx_nis2_entity_assessed_by ON nis2_entity_assessment(assessed_by) WHERE assessed_by IS NOT NULL;

CREATE TRIGGER trg_nis2_entity_assessment_updated_at
    BEFORE UPDATE ON nis2_entity_assessment
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE nis2_entity_assessment ENABLE ROW LEVEL SECURITY;
ALTER TABLE nis2_entity_assessment FORCE ROW LEVEL SECURITY;

CREATE POLICY nis2_entity_tenant_select ON nis2_entity_assessment FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY nis2_entity_tenant_insert ON nis2_entity_assessment FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY nis2_entity_tenant_update ON nis2_entity_assessment FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY nis2_entity_tenant_delete ON nis2_entity_assessment FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE nis2_entity_assessment IS 'NIS2 entity scoping assessment. Determines whether an organization is essential, important, or not applicable under Directive 2022/2555 Articles 2-3.';
COMMENT ON COLUMN nis2_entity_assessment.assessment_criteria IS 'JSON document recording the criteria evaluated: {"size_test": true, "sector_match": "energy", "member_state_designation": false, ...}';
COMMENT ON COLUMN nis2_entity_assessment.member_state IS 'ISO 3166-1 alpha-2 code of the EU member state whose CSIRT/authority has jurisdiction.';

-- ============================================================================
-- TABLE: nis2_incident_reports
-- ============================================================================
-- Implements the Article 23 three-phase incident reporting obligation:
--   Phase 1: Early warning — 24 hours from awareness
--   Phase 2: Incident notification — 72 hours from awareness
--   Phase 3: Final report — 1 month from awareness
--
-- Each row links to an existing incident record and tracks the three
-- submission phases independently with absolute deadline timestamps.

CREATE TABLE nis2_incident_reports (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id                 UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    incident_id                     UUID NOT NULL,
    report_ref                      VARCHAR(30) NOT NULL,

    -- Phase 1: Early Warning (24 hours)
    early_warning_status            nis2_report_phase_status NOT NULL DEFAULT 'pending',
    early_warning_deadline          TIMESTAMPTZ NOT NULL,
    early_warning_submitted_at      TIMESTAMPTZ,
    early_warning_submitted_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    early_warning_content           JSONB,
    early_warning_csirt_reference   VARCHAR(100),

    -- Phase 2: Incident Notification (72 hours)
    notification_status             nis2_report_phase_status NOT NULL DEFAULT 'pending',
    notification_deadline           TIMESTAMPTZ NOT NULL,
    notification_submitted_at       TIMESTAMPTZ,
    notification_submitted_by       UUID REFERENCES users(id) ON DELETE SET NULL,
    notification_content            JSONB,
    notification_csirt_reference    VARCHAR(100),

    -- Phase 3: Final Report (1 month)
    final_report_status             nis2_report_phase_status NOT NULL DEFAULT 'pending',
    final_report_deadline           TIMESTAMPTZ NOT NULL,
    final_report_submitted_at       TIMESTAMPTZ,
    final_report_submitted_by       UUID REFERENCES users(id) ON DELETE SET NULL,
    final_report_content            JSONB,
    final_report_document_path      TEXT,

    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One NIS2 report series per org + reference
    CONSTRAINT uq_nis2_report_ref UNIQUE (organization_id, report_ref)
);

CREATE INDEX idx_nis2_incident_org ON nis2_incident_reports(organization_id);
CREATE INDEX idx_nis2_incident_incident ON nis2_incident_reports(incident_id);
CREATE INDEX idx_nis2_incident_ew_status ON nis2_incident_reports(organization_id, early_warning_status)
    WHERE early_warning_status IN ('pending', 'overdue');
CREATE INDEX idx_nis2_incident_notif_status ON nis2_incident_reports(organization_id, notification_status)
    WHERE notification_status IN ('pending', 'overdue');
CREATE INDEX idx_nis2_incident_final_status ON nis2_incident_reports(organization_id, final_report_status)
    WHERE final_report_status IN ('pending', 'overdue');
CREATE INDEX idx_nis2_incident_ew_deadline ON nis2_incident_reports(early_warning_deadline)
    WHERE early_warning_status = 'pending';
CREATE INDEX idx_nis2_incident_notif_deadline ON nis2_incident_reports(notification_deadline)
    WHERE notification_status = 'pending';
CREATE INDEX idx_nis2_incident_final_deadline ON nis2_incident_reports(final_report_deadline)
    WHERE final_report_status = 'pending';
CREATE INDEX idx_nis2_incident_ew_submitted_by ON nis2_incident_reports(early_warning_submitted_by)
    WHERE early_warning_submitted_by IS NOT NULL;
CREATE INDEX idx_nis2_incident_notif_submitted_by ON nis2_incident_reports(notification_submitted_by)
    WHERE notification_submitted_by IS NOT NULL;
CREATE INDEX idx_nis2_incident_final_submitted_by ON nis2_incident_reports(final_report_submitted_by)
    WHERE final_report_submitted_by IS NOT NULL;

-- Auto-generate report_ref: NIS2-YYYY-NNNN (per organization, per year)
CREATE OR REPLACE FUNCTION nis2_incident_report_generate_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.report_ref IS NULL OR NEW.report_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE WHEN report_ref ~ ('^NIS2-' || current_year || '-[0-9]+$')
                 THEN CAST(SUBSTRING(report_ref FROM 11) AS INT)
                 ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM nis2_incident_reports
        WHERE organization_id = NEW.organization_id;

        NEW.report_ref := 'NIS2-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_nis2_incident_report_generate_ref
    BEFORE INSERT ON nis2_incident_reports
    FOR EACH ROW EXECUTE FUNCTION nis2_incident_report_generate_ref();

CREATE TRIGGER trg_nis2_incident_reports_updated_at
    BEFORE UPDATE ON nis2_incident_reports
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE nis2_incident_reports ENABLE ROW LEVEL SECURITY;
ALTER TABLE nis2_incident_reports FORCE ROW LEVEL SECURITY;

CREATE POLICY nis2_incident_tenant_select ON nis2_incident_reports FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY nis2_incident_tenant_insert ON nis2_incident_reports FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY nis2_incident_tenant_update ON nis2_incident_reports FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY nis2_incident_tenant_delete ON nis2_incident_reports FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE nis2_incident_reports IS 'NIS2 Article 23 incident reporting. Tracks the three mandatory reporting phases (early warning 24h, notification 72h, final report 1 month) with deadlines and submission records.';
COMMENT ON COLUMN nis2_incident_reports.report_ref IS 'Auto-generated reference: NIS2-YYYY-NNNN, sequential per organization per calendar year.';
COMMENT ON COLUMN nis2_incident_reports.incident_id IS 'Links to the platform incident record. FK not enforced here to allow flexible incident table location.';
COMMENT ON COLUMN nis2_incident_reports.early_warning_content IS 'JSON payload submitted to CSIRT: {"type_of_incident": "...", "suspected_cause": "...", "cross_border_impact": false, ...}';

-- ============================================================================
-- TABLE: nis2_security_measures
-- ============================================================================
-- Tracks implementation of the Article 21 minimum cybersecurity risk-management
-- measures. Each row represents one measure mapped to a specific article
-- reference, with optional links to existing control implementations.
--
-- The ten Article 21(2) measure domains include:
--   (a) Risk analysis and information system security policies
--   (b) Incident handling
--   (c) Business continuity and crisis management
--   (d) Supply chain security
--   (e) Security in network and information systems acquisition
--   (f) Policies for assessing the effectiveness of measures
--   (g) Basic cyber hygiene practices and cybersecurity training
--   (h) Policies on the use of cryptography and encryption
--   (i) Human resources security, access control, and asset management
--   (j) Multi-factor authentication and secured communications

CREATE TABLE nis2_security_measures (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,

    measure_code            VARCHAR(20) NOT NULL,
    measure_title           VARCHAR(500) NOT NULL,
    measure_description     TEXT,
    article_reference       VARCHAR(50) NOT NULL,

    implementation_status   nis2_measure_status NOT NULL DEFAULT 'not_started',

    owner_user_id           UUID REFERENCES users(id) ON DELETE SET NULL,
    evidence_description    TEXT,

    last_assessed_at        TIMESTAMPTZ,
    next_assessment_date    DATE,

    -- Links to existing control_implementations by UUID; queried via GIN index.
    linked_control_ids      UUID[] DEFAULT '{}',

    notes                   TEXT,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One measure code per organization
    CONSTRAINT uq_nis2_measure_org_code UNIQUE (organization_id, measure_code)
);

CREATE INDEX idx_nis2_measures_org ON nis2_security_measures(organization_id);
CREATE INDEX idx_nis2_measures_org_status ON nis2_security_measures(organization_id, implementation_status);
CREATE INDEX idx_nis2_measures_owner ON nis2_security_measures(owner_user_id) WHERE owner_user_id IS NOT NULL;
CREATE INDEX idx_nis2_measures_next_assess ON nis2_security_measures(next_assessment_date) WHERE next_assessment_date IS NOT NULL;
CREATE INDEX idx_nis2_measures_article ON nis2_security_measures(article_reference);
CREATE INDEX idx_nis2_measures_linked_controls ON nis2_security_measures USING GIN (linked_control_ids);

CREATE TRIGGER trg_nis2_security_measures_updated_at
    BEFORE UPDATE ON nis2_security_measures
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE nis2_security_measures ENABLE ROW LEVEL SECURITY;
ALTER TABLE nis2_security_measures FORCE ROW LEVEL SECURITY;

CREATE POLICY nis2_measures_tenant_select ON nis2_security_measures FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY nis2_measures_tenant_insert ON nis2_security_measures FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY nis2_measures_tenant_update ON nis2_security_measures FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY nis2_measures_tenant_delete ON nis2_security_measures FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE nis2_security_measures IS 'NIS2 Article 21 cybersecurity risk-management measures. Tracks implementation status of the ten minimum measure domains with links to existing controls.';
COMMENT ON COLUMN nis2_security_measures.measure_code IS 'Short code identifying the measure, e.g., "ART21-A", "ART21-B", ... "ART21-J" for the ten Article 21(2) domains.';
COMMENT ON COLUMN nis2_security_measures.linked_control_ids IS 'UUIDs of control_implementations that satisfy this NIS2 measure. Enables cross-framework traceability.';
COMMENT ON COLUMN nis2_security_measures.article_reference IS 'NIS2 directive article reference, e.g., "Article 21(2)(a)", "Article 21(2)(d)".';

-- ============================================================================
-- TABLE: nis2_management_accountability
-- ============================================================================
-- Article 20 requires management bodies of essential and important entities to:
--   1. Approve cybersecurity risk-management measures
--   2. Oversee implementation of those measures
--   3. Undergo cybersecurity training
--   4. Be held personally liable for infringements
--
-- This table tracks individual board member compliance with these obligations.

CREATE TABLE nis2_management_accountability (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,

    board_member_name           VARCHAR(200) NOT NULL,
    board_member_role           VARCHAR(200) NOT NULL,

    -- Training obligations (Article 20(2))
    training_completed          BOOLEAN NOT NULL DEFAULT false,
    training_date               DATE,
    training_provider           VARCHAR(200),
    training_certificate_path   TEXT,

    -- Risk measure approval obligations (Article 20(1))
    risk_measures_approved      BOOLEAN NOT NULL DEFAULT false,
    approval_date               DATE,
    approval_document_path      TEXT,

    next_training_due           DATE,
    notes                       TEXT,

    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_nis2_mgmt_org ON nis2_management_accountability(organization_id);
CREATE INDEX idx_nis2_mgmt_training_due ON nis2_management_accountability(next_training_due)
    WHERE next_training_due IS NOT NULL;
CREATE INDEX idx_nis2_mgmt_training_incomplete ON nis2_management_accountability(organization_id, training_completed)
    WHERE training_completed = false;

CREATE TRIGGER trg_nis2_management_accountability_updated_at
    BEFORE UPDATE ON nis2_management_accountability
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE nis2_management_accountability ENABLE ROW LEVEL SECURITY;
ALTER TABLE nis2_management_accountability FORCE ROW LEVEL SECURITY;

CREATE POLICY nis2_mgmt_tenant_select ON nis2_management_accountability FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY nis2_mgmt_tenant_insert ON nis2_management_accountability FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY nis2_mgmt_tenant_update ON nis2_management_accountability FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY nis2_mgmt_tenant_delete ON nis2_management_accountability FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE nis2_management_accountability IS 'NIS2 Article 20 management body accountability. Tracks board member cybersecurity training completion and risk-measure approval obligations.';
COMMENT ON COLUMN nis2_management_accountability.training_completed IS 'Whether the board member has completed mandatory cybersecurity training per Article 20(2).';
COMMENT ON COLUMN nis2_management_accountability.risk_measures_approved IS 'Whether the board member has formally approved the Article 21 risk-management measures per Article 20(1).';
