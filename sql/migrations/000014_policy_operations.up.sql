-- Migration 014: Policy Operations — Workflows, Attestations, Exceptions, Reviews & Mappings
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - policy_approval_workflows supports multi-step sequential/parallel approval
--     chains as required by enterprise governance (e.g., author → compliance → legal → CISO)
--   - policy_attestation_campaigns enables bulk attestation drives with progress
--     tracking, auto-reminders, and escalation — a key sellable feature
--   - policy_attestations tracks individual employee acknowledgments with IP logging
--     for audit evidence
--   - policy_exceptions supports formal exception management with risk assessment
--     and compensating controls (ISO 27001 requirement)
--   - policy_reviews captures the full review lifecycle, linking to new versions
--     when changes are made
--   - policy_comments provides inline review threading similar to Google Docs
--   - policy_control_mappings connects policies to framework controls for gap analysis

-- ============================================================================
-- TABLE: policy_approval_workflows
-- ============================================================================

CREATE TABLE policy_approval_workflows (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_id           UUID NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
    policy_version_id   UUID REFERENCES policy_versions(id) ON DELETE SET NULL,
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    workflow_type       VARCHAR(20) NOT NULL,
    status              VARCHAR(20) NOT NULL DEFAULT 'pending',
    initiated_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    initiated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ,
    due_date            DATE,
    current_step        INT NOT NULL DEFAULT 1,
    total_steps         INT NOT NULL DEFAULT 1,
    comments            TEXT,
    metadata            JSONB DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_wf_type CHECK (
        workflow_type IN ('new_policy', 'review', 'amendment', 'retirement')
    ),
    CONSTRAINT chk_wf_status CHECK (
        status IN ('pending', 'in_progress', 'approved', 'rejected', 'cancelled')
    ),
    CONSTRAINT chk_wf_steps CHECK (current_step >= 1 AND total_steps >= 1 AND current_step <= total_steps + 1)
);

CREATE INDEX idx_wf_policy ON policy_approval_workflows(policy_id);
CREATE INDEX idx_wf_org ON policy_approval_workflows(organization_id);
CREATE INDEX idx_wf_status ON policy_approval_workflows(organization_id, status);
CREATE INDEX idx_wf_due ON policy_approval_workflows(due_date) WHERE due_date IS NOT NULL AND status IN ('pending', 'in_progress');

CREATE TRIGGER trg_policy_workflows_updated_at
    BEFORE UPDATE ON policy_approval_workflows
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE policy_approval_workflows ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_approval_workflows FORCE ROW LEVEL SECURITY;

CREATE POLICY wf_tenant_select ON policy_approval_workflows FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY wf_tenant_insert ON policy_approval_workflows FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY wf_tenant_update ON policy_approval_workflows FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY wf_tenant_delete ON policy_approval_workflows FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE policy_approval_workflows IS 'Multi-step approval workflows for policy lifecycle events (creation, review, amendment, retirement).';

-- ============================================================================
-- TABLE: policy_approval_steps
-- ============================================================================

CREATE TABLE policy_approval_steps (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id             UUID NOT NULL REFERENCES policy_approval_workflows(id) ON DELETE CASCADE,
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    step_number             INT NOT NULL,
    approver_user_id        UUID REFERENCES users(id) ON DELETE SET NULL,
    approver_role           VARCHAR(50),
    status                  VARCHAR(20) NOT NULL DEFAULT 'pending',
    decision_date           TIMESTAMPTZ,
    comments                TEXT,
    digital_signature       TEXT,
    delegation_user_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    due_date                DATE,
    reminder_sent           BOOLEAN NOT NULL DEFAULT false,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_approval_step UNIQUE (workflow_id, step_number),
    CONSTRAINT chk_step_status CHECK (
        status IN ('pending', 'approved', 'rejected', 'skipped', 'delegated')
    )
);

CREATE INDEX idx_step_workflow ON policy_approval_steps(workflow_id);
CREATE INDEX idx_step_org ON policy_approval_steps(organization_id);
CREATE INDEX idx_step_approver ON policy_approval_steps(approver_user_id) WHERE approver_user_id IS NOT NULL;
CREATE INDEX idx_step_status ON policy_approval_steps(status) WHERE status = 'pending';
CREATE INDEX idx_step_due ON policy_approval_steps(due_date) WHERE due_date IS NOT NULL AND status = 'pending';

CREATE TRIGGER trg_approval_steps_updated_at
    BEFORE UPDATE ON policy_approval_steps
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE policy_approval_steps ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_approval_steps FORCE ROW LEVEL SECURITY;

CREATE POLICY step_tenant_select ON policy_approval_steps FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY step_tenant_insert ON policy_approval_steps FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY step_tenant_update ON policy_approval_steps FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY step_tenant_delete ON policy_approval_steps FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE policy_approval_steps IS 'Individual steps within an approval workflow. Supports delegation, digital signatures, and reminder tracking.';

-- ============================================================================
-- TABLE: policy_attestation_campaigns
-- ============================================================================

CREATE TABLE policy_attestation_campaigns (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                        VARCHAR(255) NOT NULL,
    description                 TEXT,
    policy_ids                  UUID[] NOT NULL DEFAULT '{}',
    target_audience             JSONB NOT NULL DEFAULT '{"all": true}',
    status                      VARCHAR(20) NOT NULL DEFAULT 'draft',
    start_date                  DATE,
    due_date                    DATE,
    total_recipients            INT NOT NULL DEFAULT 0,
    attested_count              INT NOT NULL DEFAULT 0,
    completion_rate             DECIMAL(5,2) NOT NULL DEFAULT 0.00,
    auto_remind                 BOOLEAN NOT NULL DEFAULT true,
    reminder_frequency_days     INT NOT NULL DEFAULT 7,
    escalation_after_days       INT NOT NULL DEFAULT 30,
    escalation_to               UUID REFERENCES users(id) ON DELETE SET NULL,
    created_by                  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_campaign_status CHECK (
        status IN ('draft', 'active', 'completed', 'cancelled')
    ),
    CONSTRAINT chk_campaign_completion CHECK (completion_rate >= 0 AND completion_rate <= 100),
    CONSTRAINT chk_campaign_reminder CHECK (reminder_frequency_days > 0),
    CONSTRAINT chk_campaign_escalation CHECK (escalation_after_days > 0)
);

CREATE INDEX idx_campaign_org ON policy_attestation_campaigns(organization_id);
CREATE INDEX idx_campaign_status ON policy_attestation_campaigns(organization_id, status);
CREATE INDEX idx_campaign_due ON policy_attestation_campaigns(due_date) WHERE status = 'active';
CREATE INDEX idx_campaign_policies ON policy_attestation_campaigns USING gin (policy_ids);

CREATE TRIGGER trg_campaigns_updated_at
    BEFORE UPDATE ON policy_attestation_campaigns
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE policy_attestation_campaigns ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_attestation_campaigns FORCE ROW LEVEL SECURITY;

CREATE POLICY campaign_tenant_select ON policy_attestation_campaigns FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY campaign_tenant_insert ON policy_attestation_campaigns FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY campaign_tenant_update ON policy_attestation_campaigns FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY campaign_tenant_delete ON policy_attestation_campaigns FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE policy_attestation_campaigns IS 'Bulk attestation drives with progress tracking, auto-reminders, and escalation workflows.';
COMMENT ON COLUMN policy_attestation_campaigns.target_audience IS 'JSON targeting: {"all":true} or {"departments":["..."],"roles":["viewer","auditor"],"locations":["GB","DE"]}';

-- ============================================================================
-- TABLE: policy_attestations
-- ============================================================================

CREATE TABLE policy_attestations (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_id               UUID NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
    policy_version_id       UUID REFERENCES policy_versions(id) ON DELETE SET NULL,
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id                 UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    campaign_id             UUID REFERENCES policy_attestation_campaigns(id) ON DELETE SET NULL,
    status                  VARCHAR(20) NOT NULL DEFAULT 'pending',
    attested_at             TIMESTAMPTZ,
    attested_from_ip        INET,
    attestation_method      VARCHAR(20),
    attestation_text        TEXT DEFAULT 'I have read, understood, and agree to comply with this policy.',
    declined_reason         TEXT,
    due_date                DATE,
    reminder_count          INT NOT NULL DEFAULT 0,
    last_reminder_at        TIMESTAMPTZ,
    expires_at              TIMESTAMPTZ,
    metadata                JSONB DEFAULT '{}',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_attest_status CHECK (
        status IN ('pending', 'attested', 'declined', 'expired', 'overdue')
    ),
    CONSTRAINT chk_attest_method CHECK (
        attestation_method IS NULL OR attestation_method IN ('digital_click', 'digital_signature', 'email_reply', 'sso_confirmation')
    )
);

CREATE INDEX idx_attest_org ON policy_attestations(organization_id);
CREATE INDEX idx_attest_policy ON policy_attestations(policy_id);
CREATE INDEX idx_attest_user ON policy_attestations(user_id);
CREATE INDEX idx_attest_campaign ON policy_attestations(campaign_id) WHERE campaign_id IS NOT NULL;
CREATE INDEX idx_attest_status ON policy_attestations(organization_id, status);
CREATE INDEX idx_attest_user_policy ON policy_attestations(organization_id, user_id, policy_id);
CREATE INDEX idx_attest_due ON policy_attestations(due_date) WHERE status = 'pending';
CREATE INDEX idx_attest_expires ON policy_attestations(expires_at) WHERE expires_at IS NOT NULL;

ALTER TABLE policy_attestations ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_attestations FORCE ROW LEVEL SECURITY;

CREATE POLICY attest_tenant_select ON policy_attestations FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY attest_tenant_insert ON policy_attestations FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY attest_tenant_update ON policy_attestations FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY attest_tenant_delete ON policy_attestations FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE policy_attestations IS 'Individual employee policy acknowledgments. Tracks attestation with IP logging for audit evidence.';

-- ============================================================================
-- TABLE: policy_exceptions
-- ============================================================================

CREATE TABLE policy_exceptions (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    policy_id                   UUID NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
    exception_ref               VARCHAR(20) NOT NULL,
    title                       VARCHAR(500) NOT NULL,
    description                 TEXT,
    justification               TEXT NOT NULL,
    risk_assessment             TEXT,
    compensating_controls       TEXT,
    status                      VARCHAR(20) NOT NULL DEFAULT 'requested',
    requested_by                UUID REFERENCES users(id) ON DELETE SET NULL,
    approved_by                 UUID REFERENCES users(id) ON DELETE SET NULL,
    approved_at                 TIMESTAMPTZ,
    effective_date              DATE,
    expiry_date                 DATE,
    review_date                 DATE,
    risk_level                  VARCHAR(10),
    linked_risk_id              UUID REFERENCES risks(id) ON DELETE SET NULL,
    conditions                  TEXT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_exception_org_ref UNIQUE (organization_id, exception_ref),
    CONSTRAINT chk_exc_status CHECK (
        status IN ('requested', 'under_review', 'approved', 'rejected', 'expired', 'revoked')
    ),
    CONSTRAINT chk_exc_risk_level CHECK (
        risk_level IS NULL OR risk_level IN ('critical', 'high', 'medium', 'low')
    ),
    CONSTRAINT chk_exc_dates CHECK (
        expiry_date IS NULL OR effective_date IS NULL OR expiry_date >= effective_date
    )
);

-- Auto-generate exception_ref: EXC-XXXX
CREATE OR REPLACE FUNCTION generate_exception_ref()
RETURNS TRIGGER AS $$
DECLARE
    next_num INT;
BEGIN
    IF NEW.exception_ref IS NULL OR NEW.exception_ref = '' THEN
        SELECT COALESCE(MAX(
            CASE WHEN exception_ref ~ '^EXC-[0-9]+$'
                 THEN CAST(SUBSTRING(exception_ref FROM 5) AS INT)
                 ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM policy_exceptions
        WHERE organization_id = NEW.organization_id;

        NEW.exception_ref := 'EXC-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_exceptions_generate_ref
    BEFORE INSERT ON policy_exceptions
    FOR EACH ROW EXECUTE FUNCTION generate_exception_ref();

CREATE INDEX idx_exc_org ON policy_exceptions(organization_id);
CREATE INDEX idx_exc_policy ON policy_exceptions(policy_id);
CREATE INDEX idx_exc_status ON policy_exceptions(organization_id, status);
CREATE INDEX idx_exc_expiry ON policy_exceptions(expiry_date) WHERE expiry_date IS NOT NULL AND status = 'approved';
CREATE INDEX idx_exc_risk ON policy_exceptions(linked_risk_id) WHERE linked_risk_id IS NOT NULL;

CREATE TRIGGER trg_exceptions_updated_at
    BEFORE UPDATE ON policy_exceptions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE policy_exceptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_exceptions FORCE ROW LEVEL SECURITY;

CREATE POLICY exc_tenant_select ON policy_exceptions FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY exc_tenant_insert ON policy_exceptions FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY exc_tenant_update ON policy_exceptions FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY exc_tenant_delete ON policy_exceptions FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE policy_exceptions IS 'Formal policy exception management with risk assessment and compensating controls. Required by ISO 27001.';

-- ============================================================================
-- TABLE: policy_reviews
-- ============================================================================

CREATE TABLE policy_reviews (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    policy_id           UUID NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
    review_type         VARCHAR(20) NOT NULL,
    status              VARCHAR(20) NOT NULL DEFAULT 'scheduled',
    reviewer_user_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    review_date         DATE,
    due_date            DATE,
    completed_date      DATE,
    outcome             VARCHAR(20),
    findings            TEXT,
    recommendations     TEXT,
    triggered_by        VARCHAR(100),
    new_version_id      UUID REFERENCES policy_versions(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_review_type CHECK (
        review_type IN ('scheduled', 'triggered', 'regulatory_change', 'incident_driven', 'ad_hoc')
    ),
    CONSTRAINT chk_review_status CHECK (
        status IN ('scheduled', 'in_progress', 'completed', 'overdue', 'cancelled')
    ),
    CONSTRAINT chk_review_outcome CHECK (
        outcome IS NULL OR outcome IN ('no_change', 'minor_update', 'major_revision', 'retirement')
    )
);

CREATE INDEX idx_review_org ON policy_reviews(organization_id);
CREATE INDEX idx_review_policy ON policy_reviews(policy_id);
CREATE INDEX idx_review_status ON policy_reviews(organization_id, status);
CREATE INDEX idx_review_due ON policy_reviews(due_date) WHERE status IN ('scheduled', 'in_progress');
CREATE INDEX idx_review_reviewer ON policy_reviews(reviewer_user_id) WHERE reviewer_user_id IS NOT NULL;

CREATE TRIGGER trg_policy_reviews_updated_at
    BEFORE UPDATE ON policy_reviews
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE policy_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_reviews FORCE ROW LEVEL SECURITY;

CREATE POLICY review_tenant_select ON policy_reviews FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY review_tenant_insert ON policy_reviews FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY review_tenant_update ON policy_reviews FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY review_tenant_delete ON policy_reviews FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE policy_reviews IS 'Policy review lifecycle records. Links review outcomes to new versions when revisions are made.';

-- ============================================================================
-- TABLE: policy_comments
-- ============================================================================

CREATE TABLE policy_comments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_version_id   UUID NOT NULL REFERENCES policy_versions(id) ON DELETE CASCADE,
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    parent_comment_id   UUID REFERENCES policy_comments(id) ON DELETE CASCADE,
    content             TEXT NOT NULL,
    content_reference   JSONB,
    status              VARCHAR(20) NOT NULL DEFAULT 'open',
    resolved_by         UUID REFERENCES users(id) ON DELETE SET NULL,
    resolved_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_comment_status CHECK (status IN ('open', 'resolved', 'deferred'))
);

CREATE INDEX idx_comment_version ON policy_comments(policy_version_id);
CREATE INDEX idx_comment_org ON policy_comments(organization_id);
CREATE INDEX idx_comment_user ON policy_comments(user_id);
CREATE INDEX idx_comment_parent ON policy_comments(parent_comment_id) WHERE parent_comment_id IS NOT NULL;
CREATE INDEX idx_comment_status ON policy_comments(policy_version_id, status) WHERE status = 'open';

CREATE TRIGGER trg_policy_comments_updated_at
    BEFORE UPDATE ON policy_comments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE policy_comments ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_comments FORCE ROW LEVEL SECURITY;

CREATE POLICY comment_tenant_select ON policy_comments FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY comment_tenant_insert ON policy_comments FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY comment_tenant_update ON policy_comments FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY comment_tenant_delete ON policy_comments FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE policy_comments IS 'Inline review comments on policy versions. Supports threaded replies and content position references.';
COMMENT ON COLUMN policy_comments.content_reference IS 'Position reference: {"paragraph":5,"text_selection":"...","position":{"start":120,"end":150}}';

-- ============================================================================
-- TABLE: policy_control_mappings
-- ============================================================================

CREATE TABLE policy_control_mappings (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    policy_id               UUID NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
    framework_control_id    UUID NOT NULL REFERENCES framework_controls(id) ON DELETE CASCADE,
    mapping_notes           TEXT,
    coverage                VARCHAR(20) NOT NULL DEFAULT 'full',
    created_by              UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_policy_control_map UNIQUE (organization_id, policy_id, framework_control_id),
    CONSTRAINT chk_pcm_coverage CHECK (coverage IN ('full', 'partial', 'referenced'))
);

CREATE INDEX idx_pcm_org ON policy_control_mappings(organization_id);
CREATE INDEX idx_pcm_policy ON policy_control_mappings(policy_id);
CREATE INDEX idx_pcm_control ON policy_control_mappings(framework_control_id);

CREATE TRIGGER trg_policy_control_mappings_updated_at
    BEFORE UPDATE ON policy_control_mappings
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE policy_control_mappings ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_control_mappings FORCE ROW LEVEL SECURITY;

CREATE POLICY pcm_tenant_select ON policy_control_mappings FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY pcm_tenant_insert ON policy_control_mappings FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY pcm_tenant_update ON policy_control_mappings FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY pcm_tenant_delete ON policy_control_mappings FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE policy_control_mappings IS 'Maps policies to framework controls. Enables gap analysis: which controls lack supporting policies?';
