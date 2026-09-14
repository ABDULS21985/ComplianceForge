-- Migration 045: Enterprise audit management foundation
--
-- Audits and audit findings were previously represented only by disconnected
-- Go stubs. These tables provide a tenant-isolated, lifecycle-constrained
-- source of truth with atomic, non-reused human-readable references.

CREATE TABLE audit_reference_sequences (
    organization_id      UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    next_audit_number    BIGINT NOT NULL DEFAULT 1 CHECK (next_audit_number > 0),
    next_finding_number  BIGINT NOT NULL DEFAULT 1 CHECK (next_finding_number > 0),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE audits (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id       UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    audit_ref             VARCHAR(30) NOT NULL,
    title                 VARCHAR(200) NOT NULL,
    description           TEXT NOT NULL DEFAULT '',
    audit_type            VARCHAR(30) NOT NULL,
    status                VARCHAR(30) NOT NULL DEFAULT 'planned',
    lead_auditor_id       UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    scope                 TEXT NOT NULL,
    scheduled_start_date  DATE NOT NULL,
    scheduled_end_date    DATE NOT NULL,
    actual_start_date     DATE,
    actual_end_date       DATE,
    framework_id          UUID REFERENCES compliance_frameworks(id) ON DELETE SET NULL,
    created_by            UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    metadata              JSONB NOT NULL DEFAULT '{}',
    search_vector         TSVECTOR GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(audit_ref, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(description, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(scope, '')), 'B')
    ) STORED,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at            TIMESTAMPTZ,

    CONSTRAINT uq_audits_org_ref UNIQUE (organization_id, audit_ref),
    CONSTRAINT chk_audits_type CHECK (audit_type IN ('internal', 'external', 'certification')),
    CONSTRAINT chk_audits_status CHECK (status IN ('planned', 'in_progress', 'completed', 'closed', 'cancelled')),
    CONSTRAINT chk_audits_schedule CHECK (scheduled_end_date >= scheduled_start_date),
    CONSTRAINT chk_audits_actual_dates CHECK (actual_end_date IS NULL OR (actual_start_date IS NOT NULL AND actual_end_date >= actual_start_date)),
    CONSTRAINT chk_audits_metadata_object CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE TABLE audit_findings (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id       UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    audit_id              UUID NOT NULL REFERENCES audits(id) ON DELETE CASCADE,
    finding_ref           VARCHAR(30) NOT NULL,
    control_id            UUID REFERENCES framework_controls(id) ON DELETE SET NULL,
    title                 VARCHAR(200) NOT NULL,
    description           TEXT NOT NULL,
    severity              VARCHAR(30) NOT NULL,
    status                VARCHAR(30) NOT NULL DEFAULT 'open',
    finding_type          VARCHAR(100) NOT NULL,
    root_cause            TEXT NOT NULL DEFAULT '',
    recommendation        TEXT NOT NULL,
    remediation_plan      TEXT NOT NULL DEFAULT '',
    responsible_user_id   UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    due_date              DATE NOT NULL,
    resolved_at           TIMESTAMPTZ,
    accepted_risk_reason  TEXT,
    created_by            UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    metadata              JSONB NOT NULL DEFAULT '{}',
    search_vector         TSVECTOR GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(finding_ref, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(description, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(root_cause, '')), 'C') ||
        setweight(to_tsvector('english', coalesce(recommendation, '')), 'C')
    ) STORED,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at            TIMESTAMPTZ,

    CONSTRAINT uq_audit_findings_org_ref UNIQUE (organization_id, finding_ref),
    CONSTRAINT chk_audit_findings_severity CHECK (severity IN ('critical', 'high', 'medium', 'low', 'informational')),
    CONSTRAINT chk_audit_findings_status CHECK (status IN ('open', 'in_progress', 'resolved', 'closed', 'accepted')),
    CONSTRAINT chk_audit_findings_resolution CHECK ((status IN ('resolved', 'closed')) = (resolved_at IS NOT NULL) OR status = 'accepted'),
    CONSTRAINT chk_audit_findings_acceptance CHECK (status <> 'accepted' OR length(trim(coalesce(accepted_risk_reason, ''))) > 0),
    CONSTRAINT chk_audit_findings_metadata_object CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX idx_audits_org_active ON audits (organization_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_audits_org_status ON audits (organization_id, status, scheduled_start_date) WHERE deleted_at IS NULL;
CREATE INDEX idx_audits_lead ON audits (organization_id, lead_auditor_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_audits_search ON audits USING GIN (search_vector);
CREATE INDEX idx_findings_audit_active ON audit_findings (organization_id, audit_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_findings_org_status_due ON audit_findings (organization_id, status, due_date) WHERE deleted_at IS NULL;
CREATE INDEX idx_findings_org_severity ON audit_findings (organization_id, severity, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_findings_responsible ON audit_findings (organization_id, responsible_user_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_findings_search ON audit_findings USING GIN (search_vector);

CREATE TRIGGER trg_audit_reference_sequences_updated_at
    BEFORE UPDATE ON audit_reference_sequences
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER trg_audits_updated_at
    BEFORE UPDATE ON audits
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER trg_audit_findings_updated_at
    BEFORE UPDATE ON audit_findings
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE audit_reference_sequences ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_reference_sequences FORCE ROW LEVEL SECURITY;
ALTER TABLE audits ENABLE ROW LEVEL SECURITY;
ALTER TABLE audits FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_findings FORCE ROW LEVEL SECURITY;

CREATE POLICY audit_reference_sequences_tenant_all ON audit_reference_sequences
    FOR ALL USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY audits_tenant_all ON audits
    FOR ALL USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY audit_findings_tenant_all ON audit_findings
    FOR ALL USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());

COMMENT ON TABLE audits IS 'Tenant-isolated audit engagements with an enforced planning and execution lifecycle.';
COMMENT ON TABLE audit_findings IS 'Tenant-isolated audit observations and remediation state linked to an audit engagement.';
COMMENT ON TABLE audit_reference_sequences IS 'Atomic per-tenant counters for non-reused AUD and FND display references.';
