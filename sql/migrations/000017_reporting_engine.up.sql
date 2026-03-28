-- Migration 017: Reporting Engine — Definitions, Schedules & Run History
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - report_definitions stores reusable report templates with JSONB filters
--     and sections, allowing flexible configuration without schema changes
--   - is_template flag enables sharing of common report configurations across
--     the organization (e.g. "Monthly Board Risk Report")
--   - report_schedules decouples scheduling from definitions so one definition
--     can have multiple schedules with different recipients and frequencies
--   - day_of_week (0=Sun..6=Sat) and day_of_month (1-28, avoids month-end
--     ambiguity) are optional depending on frequency
--   - report_runs provides full audit trail of every generation attempt,
--     including file hash for integrity verification and generation_time_ms
--     for performance monitoring
--   - generated_by is nullable on report_runs to support scheduled (system)
--     generations vs. ad-hoc user-triggered runs
--   - classification defaults to 'internal' aligned with typical GRC document
--     classification schemes (public, internal, confidential, restricted)

-- ============================================================================
-- ENUM TYPES
-- ============================================================================

CREATE TYPE report_type AS ENUM (
    'compliance_status',
    'risk_register',
    'risk_heatmap',
    'audit_summary',
    'audit_findings',
    'incident_summary',
    'breach_register',
    'vendor_risk',
    'policy_status',
    'attestation_report',
    'gap_analysis',
    'cross_framework_mapping',
    'executive_summary',
    'kri_dashboard',
    'treatment_progress',
    'custom'
);

CREATE TYPE report_format AS ENUM (
    'pdf',
    'xlsx',
    'csv',
    'json'
);

CREATE TYPE report_schedule_frequency AS ENUM (
    'daily',
    'weekly',
    'monthly',
    'quarterly',
    'annually'
);

CREATE TYPE report_run_status AS ENUM (
    'pending',
    'generating',
    'completed',
    'failed'
);

-- ============================================================================
-- TABLE: report_definitions
-- Stores reusable report configurations — what to generate, which filters to
-- apply, visual branding, and section layout. Templates (is_template = true)
-- serve as org-wide starting points that users can clone.
-- ============================================================================

CREATE TABLE report_definitions (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                        VARCHAR(200) NOT NULL,
    description                 TEXT,
    report_type                 report_type NOT NULL,
    format                      report_format NOT NULL DEFAULT 'pdf',
    filters                     JSONB NOT NULL DEFAULT '{}',
    sections                    JSONB NOT NULL DEFAULT '[]',
    classification              VARCHAR(50) NOT NULL DEFAULT 'internal',
    include_executive_summary   BOOLEAN NOT NULL DEFAULT true,
    include_appendices          BOOLEAN NOT NULL DEFAULT true,
    branding                    JSONB NOT NULL DEFAULT '{}',
    created_by                  UUID REFERENCES users(id) ON DELETE SET NULL,
    is_template                 BOOLEAN NOT NULL DEFAULT false,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_report_def_org ON report_definitions(organization_id);
CREATE INDEX idx_report_def_type ON report_definitions(organization_id, report_type);
CREATE INDEX idx_report_def_created_by ON report_definitions(created_by) WHERE created_by IS NOT NULL;

CREATE TRIGGER trg_report_definitions_updated_at
    BEFORE UPDATE ON report_definitions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE report_definitions ENABLE ROW LEVEL SECURITY;
ALTER TABLE report_definitions FORCE ROW LEVEL SECURITY;

CREATE POLICY report_def_tenant_select ON report_definitions FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY report_def_tenant_insert ON report_definitions FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY report_def_tenant_update ON report_definitions FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY report_def_tenant_delete ON report_definitions FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE report_definitions IS 'Reusable report configurations defining type, filters, sections, and branding. Templates (is_template) provide org-wide starting points for common GRC reports.';

-- ============================================================================
-- TABLE: report_schedules
-- Controls automated report generation — frequency, timing, recipients, and
-- delivery method. Linked to a report_definition; cascades on delete so
-- orphan schedules are impossible.
-- ============================================================================

CREATE TABLE report_schedules (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    report_definition_id        UUID NOT NULL REFERENCES report_definitions(id) ON DELETE CASCADE,
    name                        VARCHAR(200),
    frequency                   report_schedule_frequency NOT NULL,
    day_of_week                 INT CHECK (day_of_week >= 0 AND day_of_week <= 6),
    day_of_month                INT CHECK (day_of_month >= 1 AND day_of_month <= 28),
    time_of_day                 TIME NOT NULL DEFAULT '08:00',
    timezone                    VARCHAR(50) NOT NULL DEFAULT 'Europe/London',
    recipient_user_ids          UUID[] NOT NULL DEFAULT '{}',
    recipient_emails            TEXT[] NOT NULL DEFAULT '{}',
    delivery_channel            VARCHAR(20) NOT NULL DEFAULT 'email'
                                CHECK (delivery_channel IN ('email', 'storage', 'both')),
    is_active                   BOOLEAN NOT NULL DEFAULT true,
    last_run_at                 TIMESTAMPTZ,
    next_run_at                 TIMESTAMPTZ,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_report_sched_org ON report_schedules(organization_id);
CREATE INDEX idx_report_sched_active_next ON report_schedules(is_active, next_run_at)
    WHERE is_active = true;
CREATE INDEX idx_report_sched_def ON report_schedules(report_definition_id);

CREATE TRIGGER trg_report_schedules_updated_at
    BEFORE UPDATE ON report_schedules
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE report_schedules ENABLE ROW LEVEL SECURITY;
ALTER TABLE report_schedules FORCE ROW LEVEL SECURITY;

CREATE POLICY report_sched_tenant_select ON report_schedules FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY report_sched_tenant_insert ON report_schedules FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY report_sched_tenant_update ON report_schedules FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY report_sched_tenant_delete ON report_schedules FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE report_schedules IS 'Automated report generation schedules with frequency, timing, and delivery configuration. Multiple schedules can reference the same report definition for different audiences.';

-- ============================================================================
-- TABLE: report_runs
-- Immutable audit trail of every report generation attempt — tracks status,
-- output file metadata, performance metrics, and errors. generated_by is
-- NULL for system-scheduled runs.
-- ============================================================================

CREATE TABLE report_runs (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    report_definition_id        UUID NOT NULL REFERENCES report_definitions(id) ON DELETE CASCADE,
    schedule_id                 UUID REFERENCES report_schedules(id) ON DELETE SET NULL,
    status                      report_run_status NOT NULL DEFAULT 'pending',
    format                      report_format NOT NULL,
    file_path                   TEXT,
    file_size_bytes             BIGINT,
    file_hash                   VARCHAR(128),
    page_count                  INT,
    generation_time_ms          INT,
    parameters                  JSONB NOT NULL DEFAULT '{}',
    generated_by                UUID REFERENCES users(id) ON DELETE SET NULL,
    error_message               TEXT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at                TIMESTAMPTZ
);

CREATE INDEX idx_report_runs_org ON report_runs(organization_id);
CREATE INDEX idx_report_runs_status ON report_runs(organization_id, status);
CREATE INDEX idx_report_runs_def ON report_runs(report_definition_id);
CREATE INDEX idx_report_runs_sched ON report_runs(schedule_id) WHERE schedule_id IS NOT NULL;
CREATE INDEX idx_report_runs_created ON report_runs(created_at DESC);

-- RLS
ALTER TABLE report_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE report_runs FORCE ROW LEVEL SECURITY;

CREATE POLICY report_runs_tenant_select ON report_runs FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY report_runs_tenant_insert ON report_runs FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY report_runs_tenant_update ON report_runs FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY report_runs_tenant_delete ON report_runs FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE report_runs IS 'Immutable audit trail of report generation attempts. Tracks status lifecycle (pending → generating → completed/failed), output file integrity (hash), and performance metrics.';
