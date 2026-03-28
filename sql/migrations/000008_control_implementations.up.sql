-- Migration 008: Organization Framework Adoption, Control Implementations, Evidence & Testing
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - organization_frameworks tracks which frameworks each org has adopted and their
--     certification status — critical for ISO 27001 where certification tracking is a
--     sellable feature (#147)
--   - control_implementations is the core operational table — it records HOW each org
--     implements each control, at what maturity level, with what gaps
--   - Maturity levels follow CMMI: 0=Non-existent, 1=Initial/Ad-hoc, 2=Managed/Repeatable,
--     3=Defined/Documented, 4=Quantitatively Managed/Measured, 5=Optimizing/Continuous Improvement
--   - control_evidence stores proof of implementation with integrity hashing (SHA-256)
--     and evidence lifecycle (valid_from/valid_until) for continuous compliance
--   - control_test_results supports both design and operating effectiveness testing
--     as required by SOX/ISAE 3402 and ISO 27001 certification audits
--   - All tables have organization_id for RLS tenant isolation

-- ============================================================================
-- TABLE: organization_frameworks
-- ============================================================================

CREATE TABLE organization_frameworks (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    framework_id            UUID NOT NULL REFERENCES compliance_frameworks(id) ON DELETE CASCADE,
    status                  VARCHAR(30) NOT NULL DEFAULT 'not_started',
    adoption_date           DATE,
    target_completion_date  DATE,
    certification_date      DATE,
    certification_expiry    DATE,
    certifying_body         VARCHAR(255),
    certificate_number      VARCHAR(100),
    scope_description       TEXT,
    scope_business_units    UUID[] DEFAULT '{}',
    compliance_score        DECIMAL(5,2) NOT NULL DEFAULT 0.00,
    last_assessment_date    TIMESTAMPTZ,
    assessment_frequency    VARCHAR(20) DEFAULT 'quarterly',
    responsible_user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    metadata                JSONB DEFAULT '{}',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_org_framework UNIQUE (organization_id, framework_id),
    CONSTRAINT chk_org_fw_status CHECK (
        status IN ('not_started', 'in_progress', 'implemented', 'certified', 'expired', 'suspended')
    ),
    CONSTRAINT chk_org_fw_assessment_freq CHECK (
        assessment_frequency IS NULL OR assessment_frequency IN ('monthly', 'quarterly', 'semi_annually', 'annually')
    ),
    CONSTRAINT chk_org_fw_score CHECK (compliance_score >= 0.00 AND compliance_score <= 100.00),
    CONSTRAINT chk_org_fw_cert_dates CHECK (
        certification_expiry IS NULL OR certification_date IS NULL OR certification_expiry > certification_date
    )
);

-- Indexes
CREATE INDEX idx_org_frameworks_org ON organization_frameworks(organization_id);
CREATE INDEX idx_org_frameworks_framework ON organization_frameworks(framework_id);
CREATE INDEX idx_org_frameworks_status ON organization_frameworks(organization_id, status);
CREATE INDEX idx_org_frameworks_cert_expiry ON organization_frameworks(certification_expiry)
    WHERE certification_expiry IS NOT NULL;
CREATE INDEX idx_org_frameworks_responsible ON organization_frameworks(responsible_user_id)
    WHERE responsible_user_id IS NOT NULL;
CREATE INDEX idx_org_frameworks_score ON organization_frameworks(organization_id, compliance_score DESC);

CREATE TRIGGER trg_org_frameworks_updated_at
    BEFORE UPDATE ON organization_frameworks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE organization_frameworks IS 'Tracks which frameworks each organization has adopted, their compliance/certification status, and overall score.';
COMMENT ON COLUMN organization_frameworks.compliance_score IS 'Calculated score 0-100. Updated by the compliance engine when control statuses change.';
COMMENT ON COLUMN organization_frameworks.scope_business_units IS 'Array of business unit UUIDs included in the framework scope (references future business_units table).';

-- ============================================================================
-- TABLE: control_implementations
-- ============================================================================

CREATE TABLE control_implementations (
    id                                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id                     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    framework_control_id                UUID NOT NULL REFERENCES framework_controls(id) ON DELETE CASCADE,
    org_framework_id                    UUID NOT NULL REFERENCES organization_frameworks(id) ON DELETE CASCADE,
    status                              VARCHAR(30) NOT NULL DEFAULT 'not_implemented',
    implementation_status               VARCHAR(30) NOT NULL DEFAULT 'not_started',
    maturity_level                      INT NOT NULL DEFAULT 0,
    owner_user_id                       UUID REFERENCES users(id) ON DELETE SET NULL,
    reviewer_user_id                    UUID REFERENCES users(id) ON DELETE SET NULL,
    implementation_description          TEXT,
    implementation_notes                TEXT,
    compensating_control_description    TEXT,
    gap_description                     TEXT,
    remediation_plan                    TEXT,
    remediation_due_date                DATE,
    test_frequency                      VARCHAR(20),
    last_tested_at                      TIMESTAMPTZ,
    last_tested_by                      UUID REFERENCES users(id) ON DELETE SET NULL,
    last_test_result                    VARCHAR(20),
    effectiveness_score                 DECIMAL(5,2),
    risk_if_not_implemented             VARCHAR(10),
    automation_level                    VARCHAR(20) DEFAULT 'manual',
    tags                                TEXT[] DEFAULT '{}',
    metadata                            JSONB DEFAULT '{}',
    created_at                          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at                          TIMESTAMPTZ,

    CONSTRAINT uq_control_impl UNIQUE (organization_id, framework_control_id),
    CONSTRAINT chk_impl_status CHECK (
        status IN ('not_applicable', 'not_implemented', 'planned', 'partial', 'implemented', 'effective')
    ),
    CONSTRAINT chk_impl_impl_status CHECK (
        implementation_status IN ('not_started', 'in_progress', 'completed', 'failed')
    ),
    CONSTRAINT chk_impl_maturity CHECK (maturity_level >= 0 AND maturity_level <= 5),
    CONSTRAINT chk_impl_test_freq CHECK (
        test_frequency IS NULL OR test_frequency IN ('continuous', 'daily', 'weekly', 'monthly', 'quarterly', 'annually')
    ),
    CONSTRAINT chk_impl_test_result CHECK (
        last_test_result IS NULL OR last_test_result IN ('pass', 'fail', 'partial', 'not_tested')
    ),
    CONSTRAINT chk_impl_effectiveness CHECK (
        effectiveness_score IS NULL OR (effectiveness_score >= 0.00 AND effectiveness_score <= 100.00)
    ),
    CONSTRAINT chk_impl_risk CHECK (
        risk_if_not_implemented IS NULL OR risk_if_not_implemented IN ('critical', 'high', 'medium', 'low')
    ),
    CONSTRAINT chk_impl_automation CHECK (
        automation_level IS NULL OR automation_level IN ('fully_automated', 'semi_automated', 'manual')
    )
);

-- Indexes
CREATE INDEX idx_ctrl_impl_org ON control_implementations(organization_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_ctrl_impl_control ON control_implementations(framework_control_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_ctrl_impl_org_fw ON control_implementations(org_framework_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_ctrl_impl_status ON control_implementations(organization_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_ctrl_impl_impl_status ON control_implementations(organization_id, implementation_status) WHERE deleted_at IS NULL;
CREATE INDEX idx_ctrl_impl_maturity ON control_implementations(organization_id, maturity_level) WHERE deleted_at IS NULL;
CREATE INDEX idx_ctrl_impl_owner ON control_implementations(owner_user_id) WHERE owner_user_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_ctrl_impl_reviewer ON control_implementations(reviewer_user_id) WHERE reviewer_user_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_ctrl_impl_remediation ON control_implementations(remediation_due_date)
    WHERE remediation_due_date IS NOT NULL AND status NOT IN ('implemented', 'effective', 'not_applicable') AND deleted_at IS NULL;
CREATE INDEX idx_ctrl_impl_last_tested ON control_implementations(last_tested_at DESC NULLS LAST)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_ctrl_impl_tags ON control_implementations USING gin (tags) WHERE deleted_at IS NULL;
CREATE INDEX idx_ctrl_impl_deleted ON control_implementations(deleted_at) WHERE deleted_at IS NOT NULL;

CREATE TRIGGER trg_control_implementations_updated_at
    BEFORE UPDATE ON control_implementations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE control_implementations IS 'Records how each organization implements each framework control. Core operational table for compliance tracking.';
COMMENT ON COLUMN control_implementations.maturity_level IS 'CMMI-based: 0=Non-existent, 1=Initial, 2=Managed, 3=Defined, 4=Quantitatively Managed, 5=Optimizing';
COMMENT ON COLUMN control_implementations.effectiveness_score IS 'Calculated score 0-100 based on test results, maturity, and evidence quality.';
COMMENT ON COLUMN control_implementations.compensating_control_description IS 'Required when status is not_applicable — explains the alternative control or risk acceptance rationale.';

-- ============================================================================
-- TABLE: control_evidence
-- ============================================================================

CREATE TABLE control_evidence (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    control_implementation_id   UUID NOT NULL REFERENCES control_implementations(id) ON DELETE CASCADE,
    title                       VARCHAR(500) NOT NULL,
    description                 TEXT,
    evidence_type               VARCHAR(30) NOT NULL,
    file_path                   TEXT,
    file_name                   VARCHAR(255),
    file_size_bytes             BIGINT,
    mime_type                   VARCHAR(100),
    file_hash                   VARCHAR(128),
    collection_method           VARCHAR(30) NOT NULL DEFAULT 'manual_upload',
    collected_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    collected_by                UUID REFERENCES users(id) ON DELETE SET NULL,
    valid_from                  DATE,
    valid_until                 DATE,
    is_current                  BOOLEAN NOT NULL DEFAULT true,
    review_status               VARCHAR(20) NOT NULL DEFAULT 'pending',
    reviewed_by                 UUID REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at                 TIMESTAMPTZ,
    review_notes                TEXT,
    metadata                    JSONB DEFAULT '{}',
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at                  TIMESTAMPTZ,

    CONSTRAINT chk_evidence_type CHECK (
        evidence_type IN ('document', 'screenshot', 'log', 'configuration', 'report', 'certificate', 'interview_notes', 'test_result', 'policy', 'procedure', 'training_record')
    ),
    CONSTRAINT chk_evidence_collection CHECK (
        collection_method IN ('manual_upload', 'automated', 'api_pull', 'scan_result', 'integration')
    ),
    CONSTRAINT chk_evidence_review CHECK (
        review_status IN ('pending', 'accepted', 'rejected', 'expired')
    ),
    CONSTRAINT chk_evidence_validity CHECK (
        valid_until IS NULL OR valid_from IS NULL OR valid_until >= valid_from
    )
);

-- Indexes
CREATE INDEX idx_evidence_org ON control_evidence(organization_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_evidence_impl ON control_evidence(control_implementation_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_evidence_type ON control_evidence(evidence_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_evidence_review_status ON control_evidence(organization_id, review_status) WHERE deleted_at IS NULL;
CREATE INDEX idx_evidence_current ON control_evidence(control_implementation_id, is_current)
    WHERE is_current = true AND deleted_at IS NULL;
CREATE INDEX idx_evidence_validity ON control_evidence(valid_until)
    WHERE valid_until IS NOT NULL AND is_current = true AND deleted_at IS NULL;
CREATE INDEX idx_evidence_collected_by ON control_evidence(collected_by)
    WHERE collected_by IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_evidence_hash ON control_evidence(file_hash) WHERE file_hash IS NOT NULL;
CREATE INDEX idx_evidence_deleted ON control_evidence(deleted_at) WHERE deleted_at IS NOT NULL;

CREATE TRIGGER trg_control_evidence_updated_at
    BEFORE UPDATE ON control_evidence
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE control_evidence IS 'Proof of control implementation. Supports file uploads, automated collection, and evidence lifecycle management.';
COMMENT ON COLUMN control_evidence.file_hash IS 'SHA-256 hash for integrity verification. Prevents evidence tampering — critical for audit trails.';
COMMENT ON COLUMN control_evidence.is_current IS 'False for superseded evidence. Allows historical evidence trail while filtering to current state.';

-- ============================================================================
-- TABLE: control_test_results
-- ============================================================================

CREATE TABLE control_test_results (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    control_implementation_id   UUID NOT NULL REFERENCES control_implementations(id) ON DELETE CASCADE,
    test_type                   VARCHAR(30) NOT NULL,
    test_procedure              TEXT,
    result                      VARCHAR(20) NOT NULL,
    findings                    TEXT,
    recommendations             TEXT,
    tested_by                   UUID REFERENCES users(id) ON DELETE SET NULL,
    tested_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    evidence_ids                UUID[] DEFAULT '{}',
    next_test_date              DATE,
    metadata                    JSONB DEFAULT '{}',
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_test_type CHECK (
        test_type IN ('design_effectiveness', 'operating_effectiveness', 'compliance_check', 'vulnerability_scan', 'penetration_test', 'walkthrough')
    ),
    CONSTRAINT chk_test_result CHECK (
        result IN ('pass', 'fail', 'partial', 'inconclusive')
    )
);

-- Indexes
CREATE INDEX idx_test_results_org ON control_test_results(organization_id);
CREATE INDEX idx_test_results_impl ON control_test_results(control_implementation_id);
CREATE INDEX idx_test_results_type ON control_test_results(test_type);
CREATE INDEX idx_test_results_result ON control_test_results(organization_id, result);
CREATE INDEX idx_test_results_tested_at ON control_test_results(tested_at DESC);
CREATE INDEX idx_test_results_next ON control_test_results(next_test_date)
    WHERE next_test_date IS NOT NULL;
CREATE INDEX idx_test_results_tested_by ON control_test_results(tested_by) WHERE tested_by IS NOT NULL;

COMMENT ON TABLE control_test_results IS 'Results from control effectiveness testing. Supports design and operating effectiveness testing for SOX/ISAE 3402/ISO 27001.';
COMMENT ON COLUMN control_test_results.test_type IS 'design_effectiveness=control is properly designed; operating_effectiveness=control operates as intended over time.';
COMMENT ON COLUMN control_test_results.evidence_ids IS 'Array of control_evidence UUIDs linked to this test result.';

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- organization_frameworks
ALTER TABLE organization_frameworks ENABLE ROW LEVEL SECURITY;
ALTER TABLE organization_frameworks FORCE ROW LEVEL SECURITY;

CREATE POLICY org_fw_tenant_select ON organization_frameworks FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY org_fw_tenant_insert ON organization_frameworks FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY org_fw_tenant_update ON organization_frameworks FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY org_fw_tenant_delete ON organization_frameworks FOR DELETE
    USING (organization_id = get_current_tenant());

-- control_implementations
ALTER TABLE control_implementations ENABLE ROW LEVEL SECURITY;
ALTER TABLE control_implementations FORCE ROW LEVEL SECURITY;

CREATE POLICY ctrl_impl_tenant_select ON control_implementations FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY ctrl_impl_tenant_insert ON control_implementations FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY ctrl_impl_tenant_update ON control_implementations FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY ctrl_impl_tenant_delete ON control_implementations FOR DELETE
    USING (organization_id = get_current_tenant());

-- control_evidence
ALTER TABLE control_evidence ENABLE ROW LEVEL SECURITY;
ALTER TABLE control_evidence FORCE ROW LEVEL SECURITY;

CREATE POLICY evidence_tenant_select ON control_evidence FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY evidence_tenant_insert ON control_evidence FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY evidence_tenant_update ON control_evidence FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY evidence_tenant_delete ON control_evidence FOR DELETE
    USING (organization_id = get_current_tenant());

-- control_test_results
ALTER TABLE control_test_results ENABLE ROW LEVEL SECURITY;
ALTER TABLE control_test_results FORCE ROW LEVEL SECURITY;

CREATE POLICY test_results_tenant_select ON control_test_results FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY test_results_tenant_insert ON control_test_results FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY test_results_tenant_update ON control_test_results FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY test_results_tenant_delete ON control_test_results FOR DELETE
    USING (organization_id = get_current_tenant());
