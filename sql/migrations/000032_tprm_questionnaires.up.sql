-- Migration 032: TPRM Questionnaires & Vendor Assessments
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - assessment_questionnaires define reusable assessment templates with sections
--     and weighted questions. System questionnaires (organization_id IS NULL) are
--     global; org-specific ones extend or customize.
--   - questionnaire_sections group questions within a questionnaire, each with
--     optional weight and framework domain mapping.
--   - questionnaire_questions support 9 question types (text, textarea, single/multi
--     choice, number, date, file, yes_no, scale) with conditional logic, evidence
--     requirements, and risk impact mapping.
--   - vendor_assessments track the full lifecycle of sending a questionnaire to a
--     vendor: creation -> sent -> in_progress -> submitted -> review -> complete.
--     Includes scoring, risk rating, and pass/fail determination.
--   - vendor_assessment_responses capture per-question answers with reviewer flags.
--   - vendor_portal_sessions track vendor portal access for audit/security purposes.
--   - Assessment refs auto-generated: VAS-YYYY-NNNN.
--   - All org-scoped tables are tenant-isolated via RLS on organization_id.

-- ============================================================================
-- TABLE: assessment_questionnaires
-- ============================================================================

CREATE TABLE assessment_questionnaires (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id                 UUID REFERENCES organizations(id) ON DELETE CASCADE,
    name                            VARCHAR(300) NOT NULL,
    description                     TEXT,
    questionnaire_type              VARCHAR(30) NOT NULL
                                    CHECK (questionnaire_type IN (
                                        'security', 'privacy', 'compliance', 'operational',
                                        'financial', 'esg', 'comprehensive'
                                    )),
    version                         INT NOT NULL DEFAULT 1,
    status                          VARCHAR(20) NOT NULL DEFAULT 'draft'
                                    CHECK (status IN ('draft', 'active', 'archived', 'deprecated')),
    total_questions                 INT NOT NULL DEFAULT 0,
    total_sections                  INT NOT NULL DEFAULT 0,
    estimated_completion_minutes    INT,
    scoring_method                  VARCHAR(20) NOT NULL DEFAULT 'weighted'
                                    CHECK (scoring_method IN ('weighted', 'simple_average', 'pass_fail', 'custom')),
    pass_threshold                  DECIMAL(5,2),
    risk_tier_thresholds            JSONB,
    applicable_vendor_tiers         TEXT[],
    is_system                       BOOLEAN NOT NULL DEFAULT false,
    is_template                     BOOLEAN NOT NULL DEFAULT true,
    created_by                      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_assess_quest_org ON assessment_questionnaires(organization_id) WHERE organization_id IS NOT NULL;
CREATE INDEX idx_assess_quest_system ON assessment_questionnaires(is_system) WHERE is_system = true;
CREATE INDEX idx_assess_quest_type ON assessment_questionnaires(questionnaire_type);
CREATE INDEX idx_assess_quest_status ON assessment_questionnaires(status);
CREATE INDEX idx_assess_quest_org_status ON assessment_questionnaires(organization_id, status);
CREATE INDEX idx_assess_quest_template ON assessment_questionnaires(is_template) WHERE is_template = true;
CREATE INDEX idx_assess_quest_vendor_tiers ON assessment_questionnaires USING GIN (applicable_vendor_tiers);
CREATE INDEX idx_assess_quest_risk_tiers ON assessment_questionnaires USING GIN (risk_tier_thresholds);
CREATE INDEX idx_assess_quest_created_by ON assessment_questionnaires(created_by) WHERE created_by IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_assess_quest_updated_at
    BEFORE UPDATE ON assessment_questionnaires
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE assessment_questionnaires IS 'Reusable TPRM assessment questionnaire templates. System questionnaires (organization_id IS NULL) are global; org-specific ones customize for their vendor program. Supports weighted, simple average, pass/fail, and custom scoring methods.';
COMMENT ON COLUMN assessment_questionnaires.risk_tier_thresholds IS 'JSONB risk tier thresholds: {"critical": {"min": 0, "max": 40}, "high": {"min": 40, "max": 60}, "medium": {"min": 60, "max": 80}, "low": {"min": 80, "max": 100}}';
COMMENT ON COLUMN assessment_questionnaires.applicable_vendor_tiers IS 'Vendor tiers this questionnaire applies to: ["tier_1", "tier_2"]. Empty = all tiers.';

-- ============================================================================
-- TABLE: questionnaire_sections
-- ============================================================================

CREATE TABLE questionnaire_sections (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    questionnaire_id        UUID NOT NULL REFERENCES assessment_questionnaires(id) ON DELETE CASCADE,
    name                    VARCHAR(300) NOT NULL,
    description             TEXT,
    sort_order              INT NOT NULL DEFAULT 0,
    weight                  DECIMAL(5,2) NOT NULL DEFAULT 1.00,
    framework_domain_code   VARCHAR(50),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_quest_sections_questionnaire ON questionnaire_sections(questionnaire_id);
CREATE INDEX idx_quest_sections_sort ON questionnaire_sections(questionnaire_id, sort_order);
CREATE INDEX idx_quest_sections_fw_domain ON questionnaire_sections(framework_domain_code) WHERE framework_domain_code IS NOT NULL;

COMMENT ON TABLE questionnaire_sections IS 'Logical groupings of questions within a questionnaire. Sections have weights that contribute to the overall scoring calculation.';
COMMENT ON COLUMN questionnaire_sections.framework_domain_code IS 'Optional mapping to a framework domain code for compliance alignment.';

-- ============================================================================
-- TABLE: questionnaire_questions
-- ============================================================================

CREATE TABLE questionnaire_questions (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    section_id              UUID NOT NULL REFERENCES questionnaire_sections(id) ON DELETE CASCADE,
    question_text           TEXT NOT NULL,
    question_type           VARCHAR(20) NOT NULL
                            CHECK (question_type IN (
                                'text', 'textarea', 'single_choice', 'multi_choice',
                                'number', 'date', 'file_upload', 'yes_no', 'scale'
                            )),
    options                 JSONB,
    is_required             BOOLEAN NOT NULL DEFAULT true,
    weight                  DECIMAL(5,2) NOT NULL DEFAULT 1.00,
    risk_impact             VARCHAR(20)
                            CHECK (risk_impact IS NULL OR risk_impact IN ('critical', 'high', 'medium', 'low', 'informational')),
    guidance_text           TEXT,
    evidence_required       BOOLEAN NOT NULL DEFAULT false,
    evidence_guidance       TEXT,
    mapped_control_codes    TEXT[],
    conditional_on          JSONB,
    sort_order              INT NOT NULL DEFAULT 0,
    tags                    TEXT[],
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_quest_questions_section ON questionnaire_questions(section_id);
CREATE INDEX idx_quest_questions_sort ON questionnaire_questions(section_id, sort_order);
CREATE INDEX idx_quest_questions_type ON questionnaire_questions(question_type);
CREATE INDEX idx_quest_questions_required ON questionnaire_questions(section_id, is_required) WHERE is_required = true;
CREATE INDEX idx_quest_questions_risk ON questionnaire_questions(risk_impact) WHERE risk_impact IS NOT NULL;
CREATE INDEX idx_quest_questions_evidence ON questionnaire_questions(evidence_required) WHERE evidence_required = true;
CREATE INDEX idx_quest_questions_controls ON questionnaire_questions USING GIN (mapped_control_codes);
CREATE INDEX idx_quest_questions_tags ON questionnaire_questions USING GIN (tags);
CREATE INDEX idx_quest_questions_conditional ON questionnaire_questions USING GIN (conditional_on);

COMMENT ON TABLE questionnaire_questions IS 'Individual questions within a questionnaire section. Supports 9 question types with optional conditional logic, evidence requirements, and control code mapping for compliance traceability.';
COMMENT ON COLUMN questionnaire_questions.options IS 'JSONB options for choice questions: [{"value": "yes", "label": "Yes", "score": 100}, {"value": "no", "label": "No", "score": 0}]';
COMMENT ON COLUMN questionnaire_questions.conditional_on IS 'JSONB conditional display logic: {"question_id": "...", "operator": "equals", "value": "yes"}. Question is shown only if condition is met.';

-- ============================================================================
-- TABLE: vendor_assessments
-- ============================================================================

CREATE TABLE vendor_assessments (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    vendor_id               UUID NOT NULL,
    questionnaire_id        UUID NOT NULL REFERENCES assessment_questionnaires(id) ON DELETE RESTRICT,
    assessment_ref          VARCHAR(20) NOT NULL,
    status                  VARCHAR(20) NOT NULL DEFAULT 'draft'
                            CHECK (status IN ('draft', 'sent', 'in_progress', 'submitted', 'under_review', 'completed', 'expired', 'cancelled')),
    sent_at                 TIMESTAMPTZ,
    sent_to_email           VARCHAR(300),
    access_token_hash       VARCHAR(128),
    reminder_count          INT NOT NULL DEFAULT 0,
    due_date                DATE,
    submitted_at            TIMESTAMPTZ,
    overall_score           DECIMAL(5,2),
    risk_rating             VARCHAR(20),
    section_scores          JSONB,
    critical_findings       INT NOT NULL DEFAULT 0,
    high_findings           INT NOT NULL DEFAULT 0,
    pass_fail               VARCHAR(20)
                            CHECK (pass_fail IS NULL OR pass_fail IN ('pass', 'fail', 'conditional_pass')),
    reviewed_by             UUID REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at             TIMESTAMPTZ,
    review_notes            TEXT,
    follow_up_required      BOOLEAN NOT NULL DEFAULT false,
    follow_up_items         JSONB,
    next_assessment_date    DATE,
    metadata                JSONB,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_vendor_assessments_org_ref UNIQUE (organization_id, assessment_ref)
);

-- Indexes
CREATE INDEX idx_vendor_assess_org ON vendor_assessments(organization_id);
CREATE INDEX idx_vendor_assess_vendor ON vendor_assessments(vendor_id);
CREATE INDEX idx_vendor_assess_org_vendor ON vendor_assessments(organization_id, vendor_id);
CREATE INDEX idx_vendor_assess_questionnaire ON vendor_assessments(questionnaire_id);
CREATE INDEX idx_vendor_assess_org_status ON vendor_assessments(organization_id, status);
CREATE INDEX idx_vendor_assess_due_date ON vendor_assessments(due_date) WHERE due_date IS NOT NULL;
CREATE INDEX idx_vendor_assess_risk ON vendor_assessments(organization_id, risk_rating) WHERE risk_rating IS NOT NULL;
CREATE INDEX idx_vendor_assess_pass_fail ON vendor_assessments(organization_id, pass_fail) WHERE pass_fail IS NOT NULL;
CREATE INDEX idx_vendor_assess_reviewed_by ON vendor_assessments(reviewed_by) WHERE reviewed_by IS NOT NULL;
CREATE INDEX idx_vendor_assess_follow_up ON vendor_assessments(organization_id, follow_up_required) WHERE follow_up_required = true;
CREATE INDEX idx_vendor_assess_next_date ON vendor_assessments(next_assessment_date) WHERE next_assessment_date IS NOT NULL;
CREATE INDEX idx_vendor_assess_token ON vendor_assessments(access_token_hash) WHERE access_token_hash IS NOT NULL;
CREATE INDEX idx_vendor_assess_section_scores ON vendor_assessments USING GIN (section_scores);
CREATE INDEX idx_vendor_assess_metadata ON vendor_assessments USING GIN (metadata);

-- Trigger
CREATE TRIGGER trg_vendor_assess_updated_at
    BEFORE UPDATE ON vendor_assessments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE vendor_assessments IS 'Vendor assessment instances — a specific questionnaire sent to a specific vendor. Tracks full lifecycle from draft through scoring and review. vendor_id references the vendor entity (no FK constraint as the vendor table may be in a different module).';
COMMENT ON COLUMN vendor_assessments.assessment_ref IS 'Auto-generated reference per org per year: VAS-YYYY-NNNN.';
COMMENT ON COLUMN vendor_assessments.access_token_hash IS 'SHA-256 hash of the vendor portal access token. Token itself is never stored.';
COMMENT ON COLUMN vendor_assessments.section_scores IS 'JSONB per-section scores: [{"section_id": "...", "name": "Security", "score": 85.5, "weight": 2.0, "weighted_score": 171.0}]';
COMMENT ON COLUMN vendor_assessments.follow_up_items IS 'JSONB follow-up items: [{"finding": "No MFA", "severity": "high", "remediation": "Enable MFA", "due_date": "2026-06-01", "status": "open"}]';

-- ============================================================================
-- TABLE: vendor_assessment_responses
-- ============================================================================

CREATE TABLE vendor_assessment_responses (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    assessment_id           UUID NOT NULL REFERENCES vendor_assessments(id) ON DELETE CASCADE,
    question_id             UUID NOT NULL REFERENCES questionnaire_questions(id) ON DELETE CASCADE,
    answer_value            TEXT,
    answer_score            DECIMAL(5,2),
    evidence_paths          TEXT[],
    evidence_notes          TEXT,
    reviewer_comment        TEXT,
    reviewer_flag           VARCHAR(25)
                            CHECK (reviewer_flag IS NULL OR reviewer_flag IN ('accepted', 'flagged', 'requires_evidence', 'requires_follow_up')),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_vendor_responses_org ON vendor_assessment_responses(organization_id);
CREATE INDEX idx_vendor_responses_assessment ON vendor_assessment_responses(assessment_id);
CREATE INDEX idx_vendor_responses_question ON vendor_assessment_responses(question_id);
CREATE INDEX idx_vendor_responses_assess_question ON vendor_assessment_responses(assessment_id, question_id);
CREATE INDEX idx_vendor_responses_flag ON vendor_assessment_responses(reviewer_flag) WHERE reviewer_flag IS NOT NULL;
CREATE INDEX idx_vendor_responses_evidence ON vendor_assessment_responses USING GIN (evidence_paths);

-- Trigger
CREATE TRIGGER trg_vendor_responses_updated_at
    BEFORE UPDATE ON vendor_assessment_responses
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE vendor_assessment_responses IS 'Per-question responses from vendors for a specific assessment. Includes answer value, calculated score, evidence attachments, and reviewer flags for follow-up.';

-- ============================================================================
-- TABLE: vendor_portal_sessions
-- ============================================================================

CREATE TABLE vendor_portal_sessions (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    assessment_id           UUID NOT NULL REFERENCES vendor_assessments(id) ON DELETE CASCADE,
    access_token_hash       VARCHAR(128) NOT NULL,
    vendor_email            VARCHAR(300) NOT NULL,
    ip_address              VARCHAR(45),
    user_agent              TEXT,
    started_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_activity_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at            TIMESTAMPTZ,
    progress_percentage     DECIMAL(5,2) NOT NULL DEFAULT 0.00,
    is_active               BOOLEAN NOT NULL DEFAULT true
);

-- Indexes
CREATE INDEX idx_portal_sessions_assessment ON vendor_portal_sessions(assessment_id);
CREATE INDEX idx_portal_sessions_token ON vendor_portal_sessions(access_token_hash);
CREATE INDEX idx_portal_sessions_email ON vendor_portal_sessions(vendor_email);
CREATE INDEX idx_portal_sessions_active ON vendor_portal_sessions(is_active) WHERE is_active = true;
CREATE INDEX idx_portal_sessions_started ON vendor_portal_sessions(started_at DESC);
CREATE INDEX idx_portal_sessions_ip ON vendor_portal_sessions(ip_address) WHERE ip_address IS NOT NULL;

COMMENT ON TABLE vendor_portal_sessions IS 'Tracks vendor portal access sessions for audit and security purposes. Records IP, user agent, progress, and activity timestamps. No RLS — accessed via token-based authentication at the application layer.';

-- ============================================================================
-- TRIGGER FUNCTIONS
-- ============================================================================

-- Auto-generate vendor assessment reference: VAS-YYYY-NNNN
CREATE OR REPLACE FUNCTION generate_vendor_assessment_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.assessment_ref IS NULL OR NEW.assessment_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN assessment_ref ~ ('^VAS-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(assessment_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM vendor_assessments
        WHERE organization_id = NEW.organization_id;

        NEW.assessment_ref := 'VAS-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_vendor_assess_generate_ref
    BEFORE INSERT ON vendor_assessments
    FOR EACH ROW EXECUTE FUNCTION generate_vendor_assessment_ref();

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- assessment_questionnaires (org_id nullable for system questionnaires)
ALTER TABLE assessment_questionnaires ENABLE ROW LEVEL SECURITY;
ALTER TABLE assessment_questionnaires FORCE ROW LEVEL SECURITY;

CREATE POLICY assess_quest_tenant_select ON assessment_questionnaires FOR SELECT
    USING (organization_id IS NULL OR organization_id = get_current_tenant());
CREATE POLICY assess_quest_tenant_insert ON assessment_questionnaires FOR INSERT
    WITH CHECK (organization_id IS NULL OR organization_id = get_current_tenant());
CREATE POLICY assess_quest_tenant_update ON assessment_questionnaires FOR UPDATE
    USING (organization_id IS NULL OR organization_id = get_current_tenant())
    WITH CHECK (organization_id IS NULL OR organization_id = get_current_tenant());
CREATE POLICY assess_quest_tenant_delete ON assessment_questionnaires FOR DELETE
    USING (organization_id IS NULL OR organization_id = get_current_tenant());

-- questionnaire_sections (no org_id — inherits from questionnaire via CASCADE)
-- No RLS needed — access controlled through questionnaire join

-- questionnaire_questions (no org_id — inherits from section via CASCADE)
-- No RLS needed — access controlled through section/questionnaire join

-- vendor_assessments
ALTER TABLE vendor_assessments ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendor_assessments FORCE ROW LEVEL SECURITY;

CREATE POLICY vendor_assess_tenant_select ON vendor_assessments FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY vendor_assess_tenant_insert ON vendor_assessments FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY vendor_assess_tenant_update ON vendor_assessments FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY vendor_assess_tenant_delete ON vendor_assessments FOR DELETE
    USING (organization_id = get_current_tenant());

-- vendor_assessment_responses
ALTER TABLE vendor_assessment_responses ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendor_assessment_responses FORCE ROW LEVEL SECURITY;

CREATE POLICY vendor_responses_tenant_select ON vendor_assessment_responses FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY vendor_responses_tenant_insert ON vendor_assessment_responses FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY vendor_responses_tenant_update ON vendor_assessment_responses FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY vendor_responses_tenant_delete ON vendor_assessment_responses FOR DELETE
    USING (organization_id = get_current_tenant());

-- vendor_portal_sessions: NO RLS (token-based access, no organization_id)
