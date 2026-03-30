-- Migration 030: Exception Management
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - compliance_exceptions are formal records of accepted deviations from
--     control requirements. They follow a full lifecycle: draft -> risk assessment
--     -> approval -> active -> expiry/revocation/renewal. Each exception targets
--     a specific scope (single control, control group, framework domain, policy
--     or standard requirement) and must include a risk justification.
--   - Compensating controls can be linked to demonstrate residual risk mitigation.
--     Effectiveness is tracked to ensure compensating measures remain viable.
--   - exception_reviews track periodic, triggered, audit, and renewal reviews
--     with risk reassessment and compensating control validation.
--   - exception_audit_trail is an immutable append-only log of all status
--     transitions and significant actions on an exception.
--   - Refs auto-generated: EXC-YYYY-NNNN.
--   - All tables are tenant-isolated via RLS on organization_id.

-- ============================================================================
-- TABLE: compliance_exceptions
-- ============================================================================

CREATE TABLE compliance_exceptions (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id                 UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    exception_ref                   VARCHAR(20) NOT NULL,
    title                           VARCHAR(300) NOT NULL,
    description                     TEXT,
    exception_type                  VARCHAR(20) NOT NULL
                                    CHECK (exception_type IN ('permanent', 'temporary', 'conditional')),
    status                          VARCHAR(30) NOT NULL DEFAULT 'draft'
                                    CHECK (status IN ('draft', 'pending_risk_assessment', 'pending_approval', 'approved', 'rejected', 'expired', 'revoked', 'renewal_pending')),
    priority                        VARCHAR(20) NOT NULL DEFAULT 'medium'
                                    CHECK (priority IN ('critical', 'high', 'medium', 'low')),
    scope_type                      VARCHAR(30) NOT NULL
                                    CHECK (scope_type IN ('single_control', 'control_group', 'framework_domain', 'policy_requirement', 'standard_requirement')),

    -- Scope details
    control_implementation_ids      UUID[],
    framework_control_codes         TEXT[],
    policy_id                       UUID REFERENCES policies(id) ON DELETE SET NULL,
    scope_description               TEXT,

    -- Risk justification
    risk_justification              TEXT NOT NULL,
    residual_risk_description       TEXT,
    residual_risk_level             VARCHAR(20),
    risk_accepted_by                UUID REFERENCES users(id) ON DELETE SET NULL,
    risk_accepted_at                TIMESTAMPTZ,

    -- Compensating controls
    has_compensating_controls       BOOLEAN NOT NULL DEFAULT false,
    compensating_controls_description TEXT,
    compensating_control_ids        UUID[],
    compensating_effectiveness      VARCHAR(10)
                                    CHECK (compensating_effectiveness IS NULL OR compensating_effectiveness IN ('full', 'partial', 'minimal', 'none')),

    -- Requestor & approval
    requested_by                    UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    requested_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    approved_by                     UUID REFERENCES users(id) ON DELETE SET NULL,
    approved_at                     TIMESTAMPTZ,
    approval_comments               TEXT,
    rejection_reason                TEXT,
    workflow_instance_id            UUID REFERENCES workflow_instances(id) ON DELETE SET NULL,

    -- Validity & review
    effective_date                  DATE NOT NULL,
    expiry_date                     DATE,
    review_frequency_months         INT DEFAULT 12,
    next_review_date                DATE,
    last_review_date                DATE,
    renewal_count                   INT NOT NULL DEFAULT 0,

    -- Additional details
    conditions                      TEXT,
    business_impact_if_implemented  TEXT,
    regulatory_notification_required BOOLEAN NOT NULL DEFAULT false,
    audit_evidence_path             TEXT,
    tags                            TEXT[],
    metadata                        JSONB,

    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at                      TIMESTAMPTZ,

    CONSTRAINT uq_compliance_exceptions_org_ref UNIQUE (organization_id, exception_ref)
);

-- Indexes
CREATE INDEX idx_comp_exceptions_org ON compliance_exceptions(organization_id);
CREATE INDEX idx_comp_exceptions_org_status ON compliance_exceptions(organization_id, status);
CREATE INDEX idx_comp_exceptions_org_type ON compliance_exceptions(organization_id, exception_type);
CREATE INDEX idx_comp_exceptions_org_priority ON compliance_exceptions(organization_id, priority);
CREATE INDEX idx_comp_exceptions_org_scope ON compliance_exceptions(organization_id, scope_type);
CREATE INDEX idx_comp_exceptions_requested_by ON compliance_exceptions(requested_by);
CREATE INDEX idx_comp_exceptions_approved_by ON compliance_exceptions(approved_by) WHERE approved_by IS NOT NULL;
CREATE INDEX idx_comp_exceptions_risk_accepted_by ON compliance_exceptions(risk_accepted_by) WHERE risk_accepted_by IS NOT NULL;
CREATE INDEX idx_comp_exceptions_policy ON compliance_exceptions(policy_id) WHERE policy_id IS NOT NULL;
CREATE INDEX idx_comp_exceptions_workflow ON compliance_exceptions(workflow_instance_id) WHERE workflow_instance_id IS NOT NULL;
CREATE INDEX idx_comp_exceptions_effective_date ON compliance_exceptions(effective_date);
CREATE INDEX idx_comp_exceptions_expiry_date ON compliance_exceptions(expiry_date) WHERE expiry_date IS NOT NULL;
CREATE INDEX idx_comp_exceptions_next_review ON compliance_exceptions(next_review_date) WHERE next_review_date IS NOT NULL;
CREATE INDEX idx_comp_exceptions_deleted ON compliance_exceptions(deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_comp_exceptions_tags ON compliance_exceptions USING GIN (tags);
CREATE INDEX idx_comp_exceptions_metadata ON compliance_exceptions USING GIN (metadata);
CREATE INDEX idx_comp_exceptions_control_ids ON compliance_exceptions USING GIN (control_implementation_ids);
CREATE INDEX idx_comp_exceptions_fw_codes ON compliance_exceptions USING GIN (framework_control_codes);

-- Trigger
CREATE TRIGGER trg_comp_exceptions_updated_at
    BEFORE UPDATE ON compliance_exceptions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE compliance_exceptions IS 'Formal records of accepted deviations from compliance/control requirements. Full lifecycle: draft -> risk assessment -> approval -> active -> expiry/revocation/renewal. Scope can target individual controls, control groups, framework domains, or policy/standard requirements.';
COMMENT ON COLUMN compliance_exceptions.exception_ref IS 'Auto-generated reference per org per year: EXC-YYYY-NNNN.';
COMMENT ON COLUMN compliance_exceptions.control_implementation_ids IS 'Array of control_implementation UUIDs affected by this exception.';
COMMENT ON COLUMN compliance_exceptions.framework_control_codes IS 'Array of framework control codes (e.g., ["ISO27001-A.8.1", "SOC2-CC6.1"]) covered by this exception.';
COMMENT ON COLUMN compliance_exceptions.compensating_effectiveness IS 'Assessed effectiveness of compensating controls: full, partial, minimal, or none.';
COMMENT ON COLUMN compliance_exceptions.conditions IS 'Conditions that must be met for the exception to remain valid.';

-- ============================================================================
-- TABLE: exception_reviews
-- ============================================================================

CREATE TABLE exception_reviews (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id                 UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    exception_id                    UUID NOT NULL REFERENCES compliance_exceptions(id) ON DELETE CASCADE,
    review_type                     VARCHAR(20) NOT NULL
                                    CHECK (review_type IN ('periodic', 'triggered', 'audit', 'renewal')),
    review_date                     DATE NOT NULL,
    reviewer_user_id                UUID REFERENCES users(id) ON DELETE SET NULL,
    outcome                         VARCHAR(20) NOT NULL
                                    CHECK (outcome IN ('continue', 'modify', 'revoke', 'renew', 'escalate')),
    risk_reassessment               TEXT,
    new_risk_level                  VARCHAR(20),
    compensating_control_effective  BOOLEAN,
    conditions_still_valid          BOOLEAN,
    review_notes                    TEXT,
    next_review_date                DATE,
    evidence_path                   TEXT,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_exc_reviews_org ON exception_reviews(organization_id);
CREATE INDEX idx_exc_reviews_exception ON exception_reviews(exception_id);
CREATE INDEX idx_exc_reviews_org_type ON exception_reviews(organization_id, review_type);
CREATE INDEX idx_exc_reviews_reviewer ON exception_reviews(reviewer_user_id) WHERE reviewer_user_id IS NOT NULL;
CREATE INDEX idx_exc_reviews_outcome ON exception_reviews(organization_id, outcome);
CREATE INDEX idx_exc_reviews_date ON exception_reviews(review_date DESC);
CREATE INDEX idx_exc_reviews_next_date ON exception_reviews(next_review_date) WHERE next_review_date IS NOT NULL;

COMMENT ON TABLE exception_reviews IS 'Periodic, triggered, audit, and renewal reviews of compliance exceptions. Each review reassesses risk, validates compensating controls, and determines whether the exception should continue, be modified, revoked, renewed, or escalated.';
COMMENT ON COLUMN exception_reviews.outcome IS 'Review outcome: continue (unchanged), modify (update conditions), revoke, renew, or escalate to higher authority.';

-- ============================================================================
-- TABLE: exception_audit_trail
-- ============================================================================

CREATE TABLE exception_audit_trail (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    exception_id        UUID NOT NULL REFERENCES compliance_exceptions(id) ON DELETE CASCADE,
    action              VARCHAR(100) NOT NULL,
    performed_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    previous_status     VARCHAR(30),
    new_status          VARCHAR(30),
    details             TEXT,
    metadata            JSONB,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Immutability: prevent UPDATE and DELETE on audit trail
CREATE OR REPLACE FUNCTION prevent_audit_trail_modification()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'exception_audit_trail is immutable: % operations are not allowed', TG_OP;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_exc_audit_trail_immutable
    BEFORE UPDATE OR DELETE ON exception_audit_trail
    FOR EACH ROW EXECUTE FUNCTION prevent_audit_trail_modification();

-- Indexes
CREATE INDEX idx_exc_audit_trail_org ON exception_audit_trail(organization_id);
CREATE INDEX idx_exc_audit_trail_exception ON exception_audit_trail(exception_id);
CREATE INDEX idx_exc_audit_trail_action ON exception_audit_trail(organization_id, action);
CREATE INDEX idx_exc_audit_trail_performed_by ON exception_audit_trail(performed_by) WHERE performed_by IS NOT NULL;
CREATE INDEX idx_exc_audit_trail_created ON exception_audit_trail(created_at DESC);
CREATE INDEX idx_exc_audit_trail_status ON exception_audit_trail(organization_id, new_status) WHERE new_status IS NOT NULL;
CREATE INDEX idx_exc_audit_trail_metadata ON exception_audit_trail USING GIN (metadata);

COMMENT ON TABLE exception_audit_trail IS 'Immutable append-only audit log for compliance exceptions. Records all status transitions and significant actions. UPDATE and DELETE are prevented by trigger.';
COMMENT ON COLUMN exception_audit_trail.action IS 'Action performed: created, submitted, risk_assessed, approved, rejected, expired, revoked, renewed, reviewed, modified, etc.';

-- ============================================================================
-- TRIGGER FUNCTIONS
-- ============================================================================

-- Auto-generate exception reference: EXC-YYYY-NNNN
CREATE OR REPLACE FUNCTION generate_exception_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.exception_ref IS NULL OR NEW.exception_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN exception_ref ~ ('^EXC-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(exception_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM compliance_exceptions
        WHERE organization_id = NEW.organization_id;

        NEW.exception_ref := 'EXC-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_comp_exceptions_generate_ref
    BEFORE INSERT ON compliance_exceptions
    FOR EACH ROW EXECUTE FUNCTION generate_exception_ref();

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- compliance_exceptions
ALTER TABLE compliance_exceptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE compliance_exceptions FORCE ROW LEVEL SECURITY;

CREATE POLICY comp_exceptions_tenant_select ON compliance_exceptions FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY comp_exceptions_tenant_insert ON compliance_exceptions FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY comp_exceptions_tenant_update ON compliance_exceptions FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY comp_exceptions_tenant_delete ON compliance_exceptions FOR DELETE
    USING (organization_id = get_current_tenant());

-- exception_reviews
ALTER TABLE exception_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE exception_reviews FORCE ROW LEVEL SECURITY;

CREATE POLICY exc_reviews_tenant_select ON exception_reviews FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY exc_reviews_tenant_insert ON exception_reviews FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY exc_reviews_tenant_update ON exception_reviews FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY exc_reviews_tenant_delete ON exception_reviews FOR DELETE
    USING (organization_id = get_current_tenant());

-- exception_audit_trail
ALTER TABLE exception_audit_trail ENABLE ROW LEVEL SECURITY;
ALTER TABLE exception_audit_trail FORCE ROW LEVEL SECURITY;

CREATE POLICY exc_audit_trail_tenant_select ON exception_audit_trail FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY exc_audit_trail_tenant_insert ON exception_audit_trail FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY exc_audit_trail_tenant_update ON exception_audit_trail FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY exc_audit_trail_tenant_delete ON exception_audit_trail FOR DELETE
    USING (organization_id = get_current_tenant());
