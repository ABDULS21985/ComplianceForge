-- Migration 027: Regulatory Change Management
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - regulatory_sources represent external regulatory feeds (EU Official Journal,
--     national DPAs, industry bodies, etc.) with configurable scan frequencies.
--     These are global catalog entries (no RLS) — all orgs benefit from the same
--     source definitions.
--   - regulatory_changes capture individual regulatory updates (new law, amendment,
--     guidance, enforcement action) with rich metadata: severity, affected frameworks,
--     regions, industries, and control codes. Each change can be linked to a
--     remediation plan for structured response.
--   - regulatory_subscriptions allow orgs to subscribe to specific sources with
--     notification filtering by severity. auto_assess triggers AI-based impact
--     assessment when a new change is detected.
--   - regulatory_impact_assessments record per-org analysis of how a regulatory
--     change affects their compliance posture, with gap analysis, existing coverage
--     measurement, and effort/cost estimates. Supports both AI and human assessments.
--   - change_ref (RCH-YYYY-NNNN) is auto-generated per year (global, not per-org,
--     since changes are shared across organizations).

-- ============================================================================
-- TABLE: regulatory_sources (global catalog — no RLS)
-- ============================================================================

CREATE TABLE regulatory_sources (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                    VARCHAR(300) NOT NULL,
    source_type             VARCHAR(30) NOT NULL
                            CHECK (source_type IN ('government', 'regulator', 'standards_body', 'industry_body', 'news_aggregator', 'legal_database', 'custom')),
    country_code            VARCHAR(5),
    region                  VARCHAR(100),
    url                     VARCHAR(500),
    rss_feed_url            VARCHAR(500),
    api_url                 VARCHAR(500),
    relevance_frameworks    TEXT[],
    scan_frequency          VARCHAR(20) NOT NULL DEFAULT 'daily'
                            CHECK (scan_frequency IN ('hourly', 'daily', 'weekly', 'monthly', 'manual')),
    last_scanned_at         TIMESTAMPTZ,
    is_active               BOOLEAN NOT NULL DEFAULT true,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_reg_sources_active ON regulatory_sources(is_active) WHERE is_active = true;
CREATE INDEX idx_reg_sources_type ON regulatory_sources(source_type);
CREATE INDEX idx_reg_sources_country ON regulatory_sources(country_code) WHERE country_code IS NOT NULL;
CREATE INDEX idx_reg_sources_scan_freq ON regulatory_sources(scan_frequency, last_scanned_at);
CREATE INDEX idx_reg_sources_frameworks ON regulatory_sources USING GIN (relevance_frameworks);

-- Trigger
CREATE TRIGGER trg_reg_sources_updated_at
    BEFORE UPDATE ON regulatory_sources
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE regulatory_sources IS 'Global catalog of regulatory information sources (government sites, regulators, standards bodies). Not tenant-scoped — shared across all organizations.';
COMMENT ON COLUMN regulatory_sources.relevance_frameworks IS 'Array of framework codes this source is relevant to: ["GDPR", "NIS2", "DORA"].';
COMMENT ON COLUMN regulatory_sources.scan_frequency IS 'How often the platform checks this source for new regulatory changes.';

-- ============================================================================
-- TABLE: regulatory_changes (global — no RLS)
-- ============================================================================

CREATE TABLE regulatory_changes (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id               UUID REFERENCES regulatory_sources(id) ON DELETE SET NULL,
    change_ref              VARCHAR(20) NOT NULL,
    title                   VARCHAR(500) NOT NULL,
    summary                 TEXT,
    full_text_url           VARCHAR(500),
    published_date          DATE,
    effective_date          DATE,
    change_type             VARCHAR(30) NOT NULL
                            CHECK (change_type IN ('new_regulation', 'amendment', 'guidance', 'enforcement_action', 'standard_update', 'court_ruling', 'consultation', 'repeal')),
    severity                VARCHAR(20) NOT NULL DEFAULT 'medium'
                            CHECK (severity IN ('critical', 'high', 'medium', 'low', 'informational')),
    status                  VARCHAR(20) NOT NULL DEFAULT 'new'
                            CHECK (status IN ('new', 'under_review', 'assessed', 'action_required', 'no_action', 'archived')),
    affected_frameworks     TEXT[],
    affected_regions        TEXT[],
    affected_industries     TEXT[],
    affected_control_codes  TEXT[],
    impact_assessment       TEXT,
    impact_level            VARCHAR(20)
                            CHECK (impact_level IS NULL OR impact_level IN ('critical', 'high', 'medium', 'low', 'none')),
    compliance_gap_created  BOOLEAN,
    required_actions        TEXT,
    deadline                DATE,
    assessed_by             UUID REFERENCES users(id) ON DELETE SET NULL,
    response_plan_id        UUID,
    assigned_to             UUID REFERENCES users(id) ON DELETE SET NULL,
    tags                    TEXT[],
    metadata                JSONB,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_regulatory_changes_ref UNIQUE (change_ref)
);

-- Indexes
CREATE INDEX idx_reg_changes_source ON regulatory_changes(source_id) WHERE source_id IS NOT NULL;
CREATE INDEX idx_reg_changes_status ON regulatory_changes(status);
CREATE INDEX idx_reg_changes_severity ON regulatory_changes(severity);
CREATE INDEX idx_reg_changes_type ON regulatory_changes(change_type);
CREATE INDEX idx_reg_changes_published ON regulatory_changes(published_date DESC) WHERE published_date IS NOT NULL;
CREATE INDEX idx_reg_changes_effective ON regulatory_changes(effective_date) WHERE effective_date IS NOT NULL;
CREATE INDEX idx_reg_changes_deadline ON regulatory_changes(deadline) WHERE deadline IS NOT NULL;
CREATE INDEX idx_reg_changes_impact ON regulatory_changes(impact_level) WHERE impact_level IS NOT NULL;
CREATE INDEX idx_reg_changes_gap ON regulatory_changes(compliance_gap_created) WHERE compliance_gap_created = true;
CREATE INDEX idx_reg_changes_assessed_by ON regulatory_changes(assessed_by) WHERE assessed_by IS NOT NULL;
CREATE INDEX idx_reg_changes_assigned ON regulatory_changes(assigned_to) WHERE assigned_to IS NOT NULL;
CREATE INDEX idx_reg_changes_frameworks ON regulatory_changes USING GIN (affected_frameworks);
CREATE INDEX idx_reg_changes_regions ON regulatory_changes USING GIN (affected_regions);
CREATE INDEX idx_reg_changes_industries ON regulatory_changes USING GIN (affected_industries);
CREATE INDEX idx_reg_changes_control_codes ON regulatory_changes USING GIN (affected_control_codes);
CREATE INDEX idx_reg_changes_tags ON regulatory_changes USING GIN (tags);

-- Trigger
CREATE TRIGGER trg_reg_changes_updated_at
    BEFORE UPDATE ON regulatory_changes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE regulatory_changes IS 'Global register of regulatory changes detected from monitored sources. Each change is classified by type, severity, and affected frameworks/regions. Not tenant-scoped — shared across organizations; per-org impact is tracked in regulatory_impact_assessments.';
COMMENT ON COLUMN regulatory_changes.change_ref IS 'Auto-generated global reference: RCH-YYYY-NNNN.';
COMMENT ON COLUMN regulatory_changes.compliance_gap_created IS 'True if this change creates a new compliance gap for organizations subject to the affected frameworks.';
COMMENT ON COLUMN regulatory_changes.response_plan_id IS 'Optional link to a remediation_plan created in response to this change.';

-- ============================================================================
-- TABLE: regulatory_subscriptions (tenant-scoped)
-- ============================================================================

CREATE TABLE regulatory_subscriptions (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id                 UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    source_id                       UUID NOT NULL REFERENCES regulatory_sources(id) ON DELETE CASCADE,
    is_active                       BOOLEAN NOT NULL DEFAULT true,
    notification_on_new             BOOLEAN NOT NULL DEFAULT true,
    notification_severity_filter    TEXT[],
    auto_assess                     BOOLEAN NOT NULL DEFAULT false,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_reg_subscriptions_org_source UNIQUE (organization_id, source_id)
);

-- Indexes
CREATE INDEX idx_reg_subs_org ON regulatory_subscriptions(organization_id);
CREATE INDEX idx_reg_subs_source ON regulatory_subscriptions(source_id);
CREATE INDEX idx_reg_subs_active ON regulatory_subscriptions(organization_id, is_active) WHERE is_active = true;

COMMENT ON TABLE regulatory_subscriptions IS 'Per-organization subscriptions to regulatory sources. Controls notification preferences and whether AI auto-assessment is triggered for new changes.';
COMMENT ON COLUMN regulatory_subscriptions.notification_severity_filter IS 'Only notify for these severity levels: ["critical", "high"]. Empty/NULL means notify for all.';
COMMENT ON COLUMN regulatory_subscriptions.auto_assess IS 'When true, automatically generates an AI impact assessment when a new change is detected from this source.';

-- ============================================================================
-- TABLE: regulatory_impact_assessments (tenant-scoped)
-- ============================================================================

CREATE TABLE regulatory_impact_assessments (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    change_id               UUID NOT NULL REFERENCES regulatory_changes(id) ON DELETE CASCADE,
    status                  VARCHAR(20) NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('pending', 'ai_assessed', 'human_review', 'completed', 'not_applicable')),
    impact_on_frameworks    JSONB,
    gap_analysis            JSONB,
    existing_coverage       DECIMAL(5,2)
                            CHECK (existing_coverage IS NULL OR (existing_coverage >= 0 AND existing_coverage <= 100)),
    estimated_effort_hours  DECIMAL(8,2),
    estimated_cost_eur      DECIMAL(12,2),
    ai_assessment           TEXT,
    human_assessment        TEXT,
    assessed_by             UUID REFERENCES users(id) ON DELETE SET NULL,
    remediation_plan_id     UUID,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_reg_impact_org_change UNIQUE (organization_id, change_id)
);

-- Indexes
CREATE INDEX idx_reg_impact_org ON regulatory_impact_assessments(organization_id);
CREATE INDEX idx_reg_impact_change ON regulatory_impact_assessments(change_id);
CREATE INDEX idx_reg_impact_status ON regulatory_impact_assessments(organization_id, status);
CREATE INDEX idx_reg_impact_assessed_by ON regulatory_impact_assessments(assessed_by) WHERE assessed_by IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_reg_impact_updated_at
    BEFORE UPDATE ON regulatory_impact_assessments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE regulatory_impact_assessments IS 'Per-organization impact assessment of a regulatory change. Combines AI and human analysis with gap analysis, existing coverage measurement, and effort/cost estimates. Links to a remediation plan for structured response.';
COMMENT ON COLUMN regulatory_impact_assessments.impact_on_frameworks IS 'JSONB mapping of affected frameworks to impact details: {"ISO27001": {"affected_controls": ["A.5.1", "A.6.1"], "gap_delta": -5.2}, ...}';
COMMENT ON COLUMN regulatory_impact_assessments.gap_analysis IS 'JSONB gap analysis: {"new_requirements": [...], "modified_requirements": [...], "current_gaps": [...]}';
COMMENT ON COLUMN regulatory_impact_assessments.existing_coverage IS 'Percentage of the new requirements already covered by existing controls (0–100).';

-- ============================================================================
-- TRIGGER FUNCTIONS
-- ============================================================================

-- Auto-generate change reference: RCH-YYYY-NNNN (global, not per-org)
CREATE OR REPLACE FUNCTION generate_regulatory_change_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.change_ref IS NULL OR NEW.change_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN change_ref ~ ('^RCH-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(change_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM regulatory_changes;

        NEW.change_ref := 'RCH-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_reg_changes_generate_ref
    BEFORE INSERT ON regulatory_changes
    FOR EACH ROW EXECUTE FUNCTION generate_regulatory_change_ref();

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- regulatory_sources: NO RLS (global catalog)
-- regulatory_changes: NO RLS (global register)

-- regulatory_subscriptions
ALTER TABLE regulatory_subscriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE regulatory_subscriptions FORCE ROW LEVEL SECURITY;

CREATE POLICY reg_subs_tenant_select ON regulatory_subscriptions FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY reg_subs_tenant_insert ON regulatory_subscriptions FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY reg_subs_tenant_update ON regulatory_subscriptions FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY reg_subs_tenant_delete ON regulatory_subscriptions FOR DELETE
    USING (organization_id = get_current_tenant());

-- regulatory_impact_assessments
ALTER TABLE regulatory_impact_assessments ENABLE ROW LEVEL SECURITY;
ALTER TABLE regulatory_impact_assessments FORCE ROW LEVEL SECURITY;

CREATE POLICY reg_impact_tenant_select ON regulatory_impact_assessments FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY reg_impact_tenant_insert ON regulatory_impact_assessments FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY reg_impact_tenant_update ON regulatory_impact_assessments FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY reg_impact_tenant_delete ON regulatory_impact_assessments FOR DELETE
    USING (organization_id = get_current_tenant());
