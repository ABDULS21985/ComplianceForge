-- Migration 018: GDPR Data Subject Request (DSR) Management
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - Implements GDPR Articles 15–22 data subject rights: access (Art. 15),
--     erasure (Art. 17), rectification (Art. 16), portability (Art. 20),
--     restriction (Art. 18), objection (Art. 21), automated decision (Art. 22)
--   - PII fields use _encrypted suffix convention — application layer handles
--     encryption/decryption via AES-256-GCM; DB stores ciphertext only
--   - 30-day statutory deadline auto-calculated on INSERT via trigger, with
--     optional single extension to 60 days (Art. 12(3)) tracked explicitly
--   - dsr_audit_trail is append-only (immutable) — no UPDATE/DELETE allowed,
--     supporting accountability under Art. 5(2) and supervisory audits
--   - dsr_response_templates support system-wide defaults (organization_id NULL)
--     and per-org customizations, with multi-language support (Art. 12(1))
--   - Request reference auto-generated as DSR-YYYY-NNNN per organization per year
--   - SLA tracking (on_track / at_risk / overdue) enables dashboard monitoring
--   - Task workflow decomposition via dsr_tasks supports complex multi-system
--     requests common in enterprise data landscapes

-- ============================================================================
-- ENUM TYPES
-- ============================================================================

-- GDPR Articles 15–22 request types
CREATE TYPE dsr_request_type AS ENUM (
    'access',               -- Art. 15: Right of access
    'erasure',              -- Art. 17: Right to erasure ("right to be forgotten")
    'rectification',        -- Art. 16: Right to rectification
    'portability',          -- Art. 20: Right to data portability
    'restriction',          -- Art. 18: Right to restriction of processing
    'objection',            -- Art. 21: Right to object
    'automated_decision'    -- Art. 22: Automated individual decision-making
);

-- DSR lifecycle statuses
CREATE TYPE dsr_status AS ENUM (
    'received',                 -- Initial intake
    'identity_verification',    -- Verifying data subject identity (Art. 12(6))
    'in_progress',              -- Actively being processed
    'extended',                 -- Deadline extended under Art. 12(3)
    'completed',                -- Response sent to data subject
    'rejected',                 -- Rejected (e.g., manifestly unfounded, Art. 12(5))
    'withdrawn'                 -- Data subject withdrew the request
);

-- Processing priority
CREATE TYPE dsr_priority AS ENUM (
    'standard',     -- Normal 30-day timeline
    'urgent',       -- Expedited processing needed
    'complex'       -- Multi-system, high-volume, or legally complex
);

-- Granular task types for DSR workflow decomposition
CREATE TYPE dsr_task_type AS ENUM (
    'verify_identity',          -- Confirm data subject identity
    'locate_data',              -- Search systems for subject's data
    'extract_data',             -- Extract data from identified systems
    'review_data',              -- Review extracted data for exemptions
    'compile_response',         -- Assemble response package
    'notify_processors',        -- Notify data processors (Art. 28)
    'execute_erasure',          -- Perform data deletion
    'confirm_erasure',          -- Verify deletion completed
    'send_response',            -- Deliver response to data subject
    'notify_third_parties',     -- Notify recipients of rectification/erasure (Art. 19)
    'verify_correction',        -- Confirm data correction accuracy
    'execute_correction',       -- Apply data corrections
    'extract_machine_readable', -- Export in structured format (portability)
    'review_exemptions'         -- Assess legal exemptions/restrictions
);

-- Task processing status
CREATE TYPE dsr_task_status AS ENUM (
    'pending',          -- Not yet started
    'in_progress',      -- Currently being worked on
    'completed',        -- Task finished
    'blocked',          -- Waiting on dependency or external party
    'not_applicable'    -- Task not relevant for this request
);

-- SLA health indicator for dashboard monitoring
CREATE TYPE dsr_sla_status AS ENUM (
    'on_track',     -- Sufficient time remaining
    'at_risk',      -- Approaching deadline (e.g., <7 days remaining)
    'overdue'       -- Past statutory deadline
);

-- ============================================================================
-- TABLE: dsr_requests
-- Core DSR register — one row per data subject request
-- ============================================================================

CREATE TABLE dsr_requests (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id                 UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    request_ref                     VARCHAR(20) NOT NULL,

    -- Request classification
    request_type                    dsr_request_type NOT NULL,
    status                          dsr_status NOT NULL DEFAULT 'received',
    priority                        dsr_priority NOT NULL DEFAULT 'standard',

    -- Data subject PII (encrypted at rest — application-level AES-256-GCM)
    data_subject_name_encrypted     TEXT NOT NULL,
    data_subject_email_encrypted    TEXT NOT NULL,
    data_subject_phone_encrypted    TEXT,
    data_subject_address_encrypted  TEXT,

    -- Identity verification (Art. 12(6))
    data_subject_id_verified        BOOLEAN DEFAULT false,
    identity_verification_method    VARCHAR(100),
    identity_verified_at            TIMESTAMPTZ,
    identity_verified_by            UUID REFERENCES users(id) ON DELETE SET NULL,

    -- Request details
    request_description             TEXT NOT NULL,
    request_source                  VARCHAR(20) CHECK (request_source IN (
                                        'email', 'form', 'phone', 'letter', 'in_person', 'portal'
                                    )),
    received_date                   DATE NOT NULL,
    acknowledged_at                 TIMESTAMPTZ,

    -- Deadline management (Art. 12(3): 30 days, extendable by 60 days)
    response_deadline               DATE NOT NULL,
    extended_deadline               DATE,
    extension_reason                TEXT,
    extension_notified_at           TIMESTAMPTZ,

    -- Processing
    assigned_to                     UUID REFERENCES users(id) ON DELETE SET NULL,
    data_systems_affected           TEXT[],
    data_categories_affected        TEXT[],
    third_parties_notified          TEXT[],
    processing_notes                TEXT,

    -- Completion
    completed_at                    TIMESTAMPTZ,
    completed_by                    UUID REFERENCES users(id) ON DELETE SET NULL,
    response_method                 VARCHAR(20),
    response_document_path          TEXT,
    rejection_reason                TEXT,
    rejection_legal_basis           TEXT,

    -- SLA tracking
    sla_status                      dsr_sla_status NOT NULL DEFAULT 'on_track',
    days_remaining                  INT,
    was_extended                    BOOLEAN DEFAULT false,
    was_completed_on_time           BOOLEAN,

    -- Extensible metadata
    metadata                        JSONB NOT NULL DEFAULT '{}',

    -- Timestamps
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at                      TIMESTAMPTZ,

    -- Unique ref per organization
    CONSTRAINT uq_dsr_org_ref UNIQUE (organization_id, request_ref)
);

-- Indexes
CREATE INDEX idx_dsr_requests_org ON dsr_requests(organization_id);
CREATE INDEX idx_dsr_requests_org_status ON dsr_requests(organization_id, status);
CREATE INDEX idx_dsr_requests_org_sla ON dsr_requests(organization_id, sla_status);
CREATE INDEX idx_dsr_requests_org_type ON dsr_requests(organization_id, request_type);
CREATE INDEX idx_dsr_requests_deadline ON dsr_requests(response_deadline) WHERE deleted_at IS NULL;
CREATE INDEX idx_dsr_requests_assigned ON dsr_requests(assigned_to) WHERE assigned_to IS NOT NULL;
CREATE INDEX idx_dsr_requests_verified_by ON dsr_requests(identity_verified_by) WHERE identity_verified_by IS NOT NULL;
CREATE INDEX idx_dsr_requests_completed_by ON dsr_requests(completed_by) WHERE completed_by IS NOT NULL;

CREATE TRIGGER trg_dsr_requests_updated_at
    BEFORE UPDATE ON dsr_requests
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE dsr_requests IS 'GDPR Data Subject Request register. Tracks requests under Articles 15–22 with encrypted PII, SLA monitoring, and full audit trail.';
COMMENT ON COLUMN dsr_requests.request_ref IS 'Auto-generated reference per org per year: DSR-YYYY-NNNN.';
COMMENT ON COLUMN dsr_requests.data_subject_name_encrypted IS 'Data subject name — AES-256-GCM encrypted at application layer.';
COMMENT ON COLUMN dsr_requests.data_subject_email_encrypted IS 'Data subject email — AES-256-GCM encrypted at application layer.';
COMMENT ON COLUMN dsr_requests.response_deadline IS 'Statutory deadline: received_date + 30 days (auto-calculated on INSERT).';
COMMENT ON COLUMN dsr_requests.extended_deadline IS 'Extended deadline if Art. 12(3) extension applied: received_date + 90 days max.';
COMMENT ON COLUMN dsr_requests.sla_status IS 'Dashboard indicator: on_track, at_risk (<7 days), overdue (past deadline).';

-- ============================================================================
-- TABLE: dsr_tasks
-- Granular workflow tasks for each DSR
-- ============================================================================

CREATE TABLE dsr_tasks (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    dsr_request_id      UUID NOT NULL REFERENCES dsr_requests(id) ON DELETE CASCADE,

    -- Task definition
    task_type           dsr_task_type NOT NULL,
    description         TEXT NOT NULL,
    system_name         VARCHAR(200),

    -- Assignment and status
    assigned_to         UUID REFERENCES users(id) ON DELETE SET NULL,
    status              dsr_task_status NOT NULL DEFAULT 'pending',
    due_date            DATE,

    -- Completion
    completed_at        TIMESTAMPTZ,
    completed_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    notes               TEXT,
    evidence_path       TEXT,

    -- Ordering
    sort_order          INT NOT NULL DEFAULT 0,

    -- Timestamps
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_dsr_tasks_org ON dsr_tasks(organization_id);
CREATE INDEX idx_dsr_tasks_request ON dsr_tasks(dsr_request_id);
CREATE INDEX idx_dsr_tasks_assigned ON dsr_tasks(assigned_to) WHERE assigned_to IS NOT NULL;
CREATE INDEX idx_dsr_tasks_completed_by ON dsr_tasks(completed_by) WHERE completed_by IS NOT NULL;
CREATE INDEX idx_dsr_tasks_status ON dsr_tasks(dsr_request_id, status);

CREATE TRIGGER trg_dsr_tasks_updated_at
    BEFORE UPDATE ON dsr_tasks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE dsr_tasks IS 'Workflow tasks for DSR processing. Each request is decomposed into granular tasks assigned to specific team members and systems.';
COMMENT ON COLUMN dsr_tasks.system_name IS 'Name of the data system this task relates to (e.g., "SAP HR", "Salesforce CRM").';
COMMENT ON COLUMN dsr_tasks.evidence_path IS 'Path to evidence file (e.g., screenshot of deletion confirmation).';

-- ============================================================================
-- TABLE: dsr_audit_trail
-- Immutable append-only log for DSR accountability (Art. 5(2))
-- ============================================================================

CREATE TABLE dsr_audit_trail (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    dsr_request_id      UUID NOT NULL REFERENCES dsr_requests(id) ON DELETE CASCADE,

    -- Event details
    action              VARCHAR(100) NOT NULL,
    performed_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    description         TEXT,
    metadata            JSONB NOT NULL DEFAULT '{}',

    -- Timestamp only — no updated_at or deleted_at (immutable)
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_dsr_audit_org ON dsr_audit_trail(organization_id);
CREATE INDEX idx_dsr_audit_request_time ON dsr_audit_trail(dsr_request_id, created_at DESC);
CREATE INDEX idx_dsr_audit_performed_by ON dsr_audit_trail(performed_by) WHERE performed_by IS NOT NULL;

-- Immutability enforcement: prevent UPDATE on audit trail
CREATE OR REPLACE FUNCTION dsr_audit_trail_prevent_update()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'dsr_audit_trail is immutable — UPDATE operations are not permitted';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_dsr_audit_trail_no_update
    BEFORE UPDATE ON dsr_audit_trail
    FOR EACH ROW EXECUTE FUNCTION dsr_audit_trail_prevent_update();

COMMENT ON TABLE dsr_audit_trail IS 'Immutable audit log for DSR processing. Supports GDPR Art. 5(2) accountability. No updates or soft deletes permitted.';
COMMENT ON COLUMN dsr_audit_trail.action IS 'Action performed, e.g., "status_changed", "task_completed", "identity_verified", "response_sent".';

-- ============================================================================
-- TABLE: dsr_response_templates
-- Reusable response templates with multi-language support
-- ============================================================================

CREATE TABLE dsr_response_templates (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID REFERENCES organizations(id) ON DELETE CASCADE,

    -- Template definition
    request_type        dsr_request_type NOT NULL,
    name                VARCHAR(200) NOT NULL,
    subject             TEXT,
    body_html           TEXT,
    body_text           TEXT,

    -- Classification
    is_system           BOOLEAN NOT NULL DEFAULT false,
    language            VARCHAR(10) NOT NULL DEFAULT 'en',

    -- Timestamps
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_dsr_templates_org ON dsr_response_templates(organization_id);
CREATE INDEX idx_dsr_templates_type ON dsr_response_templates(request_type);
CREATE INDEX idx_dsr_templates_system ON dsr_response_templates(is_system) WHERE is_system = true;

CREATE TRIGGER trg_dsr_response_templates_updated_at
    BEFORE UPDATE ON dsr_response_templates
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE dsr_response_templates IS 'Response templates for DSR communications. System templates (org_id NULL) provide defaults; orgs can customize per language.';
COMMENT ON COLUMN dsr_response_templates.is_system IS 'True for platform-provided templates. System templates are read-only for tenants.';
COMMENT ON COLUMN dsr_response_templates.language IS 'ISO 639-1 language code. Supports Art. 12(1) requirement for clear, plain language.';

-- ============================================================================
-- TRIGGER FUNCTIONS
-- ============================================================================

-- Auto-generate DSR reference: DSR-YYYY-NNNN (per organization, per year)
CREATE OR REPLACE FUNCTION generate_dsr_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.request_ref IS NULL OR NEW.request_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN request_ref ~ ('^DSR-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(request_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM dsr_requests
        WHERE organization_id = NEW.organization_id;

        NEW.request_ref := 'DSR-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_dsr_requests_generate_ref
    BEFORE INSERT ON dsr_requests
    FOR EACH ROW EXECUTE FUNCTION generate_dsr_ref();

-- Auto-calculate response_deadline as received_date + 30 days on INSERT
CREATE OR REPLACE FUNCTION calculate_dsr_response_deadline()
RETURNS TRIGGER AS $$
BEGIN
    -- Only set if not explicitly provided (allows override for special cases)
    IF NEW.response_deadline IS NULL THEN
        NEW.response_deadline := NEW.received_date + INTERVAL '30 days';
    END IF;
    -- Calculate initial days remaining
    NEW.days_remaining := (NEW.response_deadline - CURRENT_DATE);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_dsr_requests_calc_deadline
    BEFORE INSERT ON dsr_requests
    FOR EACH ROW EXECUTE FUNCTION calculate_dsr_response_deadline();

-- ============================================================================
-- ROW LEVEL SECURITY
-- ============================================================================

-- dsr_requests: standard tenant isolation
ALTER TABLE dsr_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsr_requests FORCE ROW LEVEL SECURITY;

CREATE POLICY dsr_requests_tenant_select ON dsr_requests FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY dsr_requests_tenant_insert ON dsr_requests FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY dsr_requests_tenant_update ON dsr_requests FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY dsr_requests_tenant_delete ON dsr_requests FOR DELETE
    USING (organization_id = get_current_tenant());

-- dsr_tasks: standard tenant isolation
ALTER TABLE dsr_tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsr_tasks FORCE ROW LEVEL SECURITY;

CREATE POLICY dsr_tasks_tenant_select ON dsr_tasks FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY dsr_tasks_tenant_insert ON dsr_tasks FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY dsr_tasks_tenant_update ON dsr_tasks FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY dsr_tasks_tenant_delete ON dsr_tasks FOR DELETE
    USING (organization_id = get_current_tenant());

-- dsr_audit_trail: standard tenant isolation (no delete policy — immutable)
ALTER TABLE dsr_audit_trail ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsr_audit_trail FORCE ROW LEVEL SECURITY;

CREATE POLICY dsr_audit_tenant_select ON dsr_audit_trail FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY dsr_audit_tenant_insert ON dsr_audit_trail FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());

-- dsr_response_templates: system templates visible to all, custom scoped to org
ALTER TABLE dsr_response_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsr_response_templates FORCE ROW LEVEL SECURITY;

CREATE POLICY dsr_templates_tenant_select ON dsr_response_templates FOR SELECT
    USING (organization_id IS NULL OR organization_id = get_current_tenant());
CREATE POLICY dsr_templates_tenant_insert ON dsr_response_templates FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY dsr_templates_tenant_update ON dsr_response_templates FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY dsr_templates_tenant_delete ON dsr_response_templates FOR DELETE
    USING (organization_id = get_current_tenant());
