-- Migration 031: Evidence Templates & Testing
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - evidence_templates define reusable blueprints for evidence collection. System
--     templates (organization_id IS NULL, is_system = true) are shared globally;
--     org templates override or extend with custom requirements.
--   - evidence_requirements link templates to specific control implementations,
--     tracking collection status, validation, assignment, and due dates.
--   - evidence_test_suites group related test cases for automated evidence
--     validation. Suites can run on schedule (cron) or be triggered manually/CI.
--   - evidence_test_cases define individual validation checks with configurable
--     test types and expected results.
--   - evidence_test_runs capture execution results with pass/fail/skip counts
--     and detailed per-case results in JSONB.
--   - All org-scoped tables are tenant-isolated via RLS on organization_id.

-- ============================================================================
-- TABLE: evidence_templates
-- ============================================================================

CREATE TABLE evidence_templates (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id                 UUID REFERENCES organizations(id) ON DELETE CASCADE,
    framework_control_code          VARCHAR(50),
    framework_code                  VARCHAR(20),
    name                            VARCHAR(300) NOT NULL,
    description                     TEXT,
    evidence_category               VARCHAR(30) NOT NULL
                                    CHECK (evidence_category IN (
                                        'policy_document', 'procedure_document', 'configuration_screenshot',
                                        'log_export', 'access_review', 'audit_report', 'training_record',
                                        'vulnerability_scan', 'penetration_test', 'risk_assessment',
                                        'vendor_assessment', 'incident_report', 'change_record',
                                        'meeting_minutes', 'attestation'
                                    )),
    collection_method               VARCHAR(20) NOT NULL
                                    CHECK (collection_method IN (
                                        'manual_upload', 'automated_pull', 'api_integration',
                                        'screenshot', 'attestation', 'system_export', 'interview'
                                    )),
    collection_instructions         TEXT,
    collection_frequency            VARCHAR(20) NOT NULL DEFAULT 'quarterly'
                                    CHECK (collection_frequency IN (
                                        'real_time', 'daily', 'weekly', 'monthly',
                                        'quarterly', 'semi_annual', 'annual', 'on_demand'
                                    )),
    typical_collection_time_minutes INT,
    validation_rules                JSONB,
    acceptance_criteria             TEXT,
    common_rejection_reasons        TEXT[],
    template_fields                 JSONB,
    sample_evidence_description     TEXT,
    applicable_to                   TEXT[],
    difficulty                      VARCHAR(10) NOT NULL DEFAULT 'moderate'
                                    CHECK (difficulty IN ('easy', 'moderate', 'complex')),
    auditor_priority                VARCHAR(15) NOT NULL DEFAULT 'should_have'
                                    CHECK (auditor_priority IN ('must_have', 'should_have', 'nice_to_have')),
    is_system                       BOOLEAN NOT NULL DEFAULT false,
    tags                            TEXT[],
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_evidence_templates_org ON evidence_templates(organization_id) WHERE organization_id IS NOT NULL;
CREATE INDEX idx_evidence_templates_system ON evidence_templates(is_system) WHERE is_system = true;
CREATE INDEX idx_evidence_templates_fw_code ON evidence_templates(framework_code) WHERE framework_code IS NOT NULL;
CREATE INDEX idx_evidence_templates_fw_ctrl ON evidence_templates(framework_control_code) WHERE framework_control_code IS NOT NULL;
CREATE INDEX idx_evidence_templates_category ON evidence_templates(evidence_category);
CREATE INDEX idx_evidence_templates_method ON evidence_templates(collection_method);
CREATE INDEX idx_evidence_templates_frequency ON evidence_templates(collection_frequency);
CREATE INDEX idx_evidence_templates_difficulty ON evidence_templates(difficulty);
CREATE INDEX idx_evidence_templates_priority ON evidence_templates(auditor_priority);
CREATE INDEX idx_evidence_templates_tags ON evidence_templates USING GIN (tags);
CREATE INDEX idx_evidence_templates_applicable ON evidence_templates USING GIN (applicable_to);
CREATE INDEX idx_evidence_templates_validation ON evidence_templates USING GIN (validation_rules);
CREATE INDEX idx_evidence_templates_fields ON evidence_templates USING GIN (template_fields);

-- Trigger
CREATE TRIGGER trg_evidence_templates_updated_at
    BEFORE UPDATE ON evidence_templates
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE evidence_templates IS 'Reusable blueprints for evidence collection. System templates (organization_id IS NULL) are global; org-specific templates extend or override. Each template defines what evidence is needed, how to collect it, and acceptance criteria.';
COMMENT ON COLUMN evidence_templates.validation_rules IS 'JSONB validation rules: {"file_types": ["pdf", "png"], "max_size_mb": 50, "required_fields": ["date", "reviewer"], "freshness_days": 90}';
COMMENT ON COLUMN evidence_templates.template_fields IS 'JSONB field definitions for structured evidence: [{"name": "review_date", "type": "date", "required": true}, {"name": "reviewer", "type": "text"}]';
COMMENT ON COLUMN evidence_templates.common_rejection_reasons IS 'Common reasons evidence is rejected: ["outdated", "incomplete", "wrong_format", "missing_signature"].';

-- ============================================================================
-- TABLE: evidence_requirements
-- ============================================================================

CREATE TABLE evidence_requirements (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    control_implementation_id   UUID REFERENCES control_implementations(id) ON DELETE CASCADE,
    evidence_template_id        UUID REFERENCES evidence_templates(id) ON DELETE SET NULL,
    status                      VARCHAR(20) NOT NULL DEFAULT 'not_started'
                                CHECK (status IN ('not_started', 'in_progress', 'collected', 'validated', 'expired', 'rejected')),
    is_mandatory                BOOLEAN NOT NULL DEFAULT true,
    assigned_to                 UUID REFERENCES users(id) ON DELETE SET NULL,
    due_date                    DATE,
    last_collected_at           TIMESTAMPTZ,
    last_validated_at           TIMESTAMPTZ,
    last_evidence_id            UUID,
    validation_status           VARCHAR(10)
                                CHECK (validation_status IS NULL OR validation_status IN ('pending', 'passed', 'failed', 'warning')),
    validation_results          JSONB,
    next_collection_due         DATE,
    consecutive_failures        INT NOT NULL DEFAULT 0,
    notes                       TEXT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_evidence_reqs_org ON evidence_requirements(organization_id);
CREATE INDEX idx_evidence_reqs_control ON evidence_requirements(control_implementation_id) WHERE control_implementation_id IS NOT NULL;
CREATE INDEX idx_evidence_reqs_template ON evidence_requirements(evidence_template_id) WHERE evidence_template_id IS NOT NULL;
CREATE INDEX idx_evidence_reqs_org_status ON evidence_requirements(organization_id, status);
CREATE INDEX idx_evidence_reqs_assigned ON evidence_requirements(assigned_to) WHERE assigned_to IS NOT NULL;
CREATE INDEX idx_evidence_reqs_due ON evidence_requirements(due_date) WHERE due_date IS NOT NULL;
CREATE INDEX idx_evidence_reqs_next_due ON evidence_requirements(next_collection_due) WHERE next_collection_due IS NOT NULL;
CREATE INDEX idx_evidence_reqs_validation ON evidence_requirements(organization_id, validation_status) WHERE validation_status IS NOT NULL;
CREATE INDEX idx_evidence_reqs_mandatory ON evidence_requirements(organization_id, is_mandatory) WHERE is_mandatory = true;
CREATE INDEX idx_evidence_reqs_failures ON evidence_requirements(organization_id, consecutive_failures) WHERE consecutive_failures > 0;

-- Trigger
CREATE TRIGGER trg_evidence_reqs_updated_at
    BEFORE UPDATE ON evidence_requirements
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE evidence_requirements IS 'Links evidence templates to specific control implementations. Tracks collection status, validation outcomes, assignment, due dates, and consecutive failure counts for escalation.';
COMMENT ON COLUMN evidence_requirements.validation_results IS 'JSONB validation outcome details: {"checks": [{"rule": "freshness", "passed": true}, {"rule": "file_type", "passed": false, "message": "Expected PDF"}]}';
COMMENT ON COLUMN evidence_requirements.consecutive_failures IS 'Number of consecutive validation failures. Used for escalation thresholds.';

-- ============================================================================
-- TABLE: evidence_test_suites
-- ============================================================================

CREATE TABLE evidence_test_suites (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                    VARCHAR(300) NOT NULL,
    description             TEXT,
    test_type               VARCHAR(20) NOT NULL DEFAULT 'validation'
                            CHECK (test_type IN ('validation', 'completeness', 'freshness', 'compliance', 'integration')),
    schedule_cron           VARCHAR(100),
    is_active               BOOLEAN NOT NULL DEFAULT true,
    last_run_at             TIMESTAMPTZ,
    last_run_status         VARCHAR(20)
                            CHECK (last_run_status IS NULL OR last_run_status IN ('passed', 'failed', 'partial', 'error')),
    pass_threshold_percent  DECIMAL(5,2) NOT NULL DEFAULT 80.00,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_evidence_suites_org ON evidence_test_suites(organization_id);
CREATE INDEX idx_evidence_suites_org_active ON evidence_test_suites(organization_id, is_active) WHERE is_active = true;
CREATE INDEX idx_evidence_suites_type ON evidence_test_suites(organization_id, test_type);
CREATE INDEX idx_evidence_suites_last_run ON evidence_test_suites(last_run_at DESC) WHERE last_run_at IS NOT NULL;
CREATE INDEX idx_evidence_suites_status ON evidence_test_suites(organization_id, last_run_status) WHERE last_run_status IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_evidence_suites_updated_at
    BEFORE UPDATE ON evidence_test_suites
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE evidence_test_suites IS 'Groups of related evidence test cases. Suites can run on a cron schedule or be triggered manually. Pass threshold determines overall suite pass/fail.';
COMMENT ON COLUMN evidence_test_suites.schedule_cron IS 'Cron expression for scheduled runs: "0 6 * * 1" (every Monday at 6 AM).';

-- ============================================================================
-- TABLE: evidence_test_cases
-- ============================================================================

CREATE TABLE evidence_test_cases (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    test_suite_id               UUID NOT NULL REFERENCES evidence_test_suites(id) ON DELETE CASCADE,
    name                        VARCHAR(300) NOT NULL,
    description                 TEXT,
    test_type                   VARCHAR(20) NOT NULL
                                CHECK (test_type IN ('existence', 'freshness', 'completeness', 'format', 'content', 'cross_reference', 'automated')),
    target_control_code         VARCHAR(50),
    target_evidence_template_id UUID REFERENCES evidence_templates(id) ON DELETE SET NULL,
    test_config                 JSONB NOT NULL,
    expected_result             TEXT,
    sort_order                  INT NOT NULL DEFAULT 0,
    is_critical                 BOOLEAN NOT NULL DEFAULT false,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_evidence_cases_org ON evidence_test_cases(organization_id);
CREATE INDEX idx_evidence_cases_suite ON evidence_test_cases(test_suite_id);
CREATE INDEX idx_evidence_cases_type ON evidence_test_cases(test_type);
CREATE INDEX idx_evidence_cases_control ON evidence_test_cases(target_control_code) WHERE target_control_code IS NOT NULL;
CREATE INDEX idx_evidence_cases_template ON evidence_test_cases(target_evidence_template_id) WHERE target_evidence_template_id IS NOT NULL;
CREATE INDEX idx_evidence_cases_critical ON evidence_test_cases(test_suite_id, is_critical) WHERE is_critical = true;
CREATE INDEX idx_evidence_cases_sort ON evidence_test_cases(test_suite_id, sort_order);
CREATE INDEX idx_evidence_cases_config ON evidence_test_cases USING GIN (test_config);

-- Trigger
CREATE TRIGGER trg_evidence_cases_updated_at
    BEFORE UPDATE ON evidence_test_cases
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE evidence_test_cases IS 'Individual evidence validation test cases within a suite. Each case checks a specific aspect of evidence quality: existence, freshness, completeness, format, content, or cross-references.';
COMMENT ON COLUMN evidence_test_cases.test_config IS 'JSONB test configuration: {"max_age_days": 90, "required_fields": ["date"], "file_types": ["pdf"], "min_file_size_bytes": 1024}';

-- ============================================================================
-- TABLE: evidence_test_runs
-- ============================================================================

CREATE TABLE evidence_test_runs (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    test_suite_id           UUID NOT NULL REFERENCES evidence_test_suites(id) ON DELETE CASCADE,
    status                  VARCHAR(20) NOT NULL DEFAULT 'running'
                            CHECK (status IN ('running', 'completed', 'failed', 'cancelled')),
    started_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at            TIMESTAMPTZ,
    total_tests             INT NOT NULL DEFAULT 0,
    passed                  INT NOT NULL DEFAULT 0,
    failed                  INT NOT NULL DEFAULT 0,
    skipped                 INT NOT NULL DEFAULT 0,
    errors                  INT NOT NULL DEFAULT 0,
    pass_rate               DECIMAL(5,2),
    threshold_met           BOOLEAN,
    results                 JSONB,
    triggered_by            VARCHAR(20) NOT NULL DEFAULT 'manual'
                            CHECK (triggered_by IN ('schedule', 'manual', 'ci_cd', 'pre_audit')),
    triggered_by_user       UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_evidence_runs_org ON evidence_test_runs(organization_id);
CREATE INDEX idx_evidence_runs_suite ON evidence_test_runs(test_suite_id);
CREATE INDEX idx_evidence_runs_org_status ON evidence_test_runs(organization_id, status);
CREATE INDEX idx_evidence_runs_started ON evidence_test_runs(started_at DESC);
CREATE INDEX idx_evidence_runs_triggered ON evidence_test_runs(triggered_by);
CREATE INDEX idx_evidence_runs_threshold ON evidence_test_runs(organization_id, threshold_met) WHERE threshold_met IS NOT NULL;
CREATE INDEX idx_evidence_runs_user ON evidence_test_runs(triggered_by_user) WHERE triggered_by_user IS NOT NULL;
CREATE INDEX idx_evidence_runs_results ON evidence_test_runs USING GIN (results);

COMMENT ON TABLE evidence_test_runs IS 'Execution records for evidence test suites. Captures pass/fail/skip/error counts, pass rate, threshold evaluation, and detailed per-case results in JSONB.';
COMMENT ON COLUMN evidence_test_runs.results IS 'JSONB per-case results: [{"test_case_id": "...", "status": "passed", "message": "...", "duration_ms": 120}, ...]';

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- evidence_templates (org_id nullable for system templates)
ALTER TABLE evidence_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE evidence_templates FORCE ROW LEVEL SECURITY;

CREATE POLICY evidence_templates_tenant_select ON evidence_templates FOR SELECT
    USING (organization_id IS NULL OR organization_id = get_current_tenant());
CREATE POLICY evidence_templates_tenant_insert ON evidence_templates FOR INSERT
    WITH CHECK (organization_id IS NULL OR organization_id = get_current_tenant());
CREATE POLICY evidence_templates_tenant_update ON evidence_templates FOR UPDATE
    USING (organization_id IS NULL OR organization_id = get_current_tenant())
    WITH CHECK (organization_id IS NULL OR organization_id = get_current_tenant());
CREATE POLICY evidence_templates_tenant_delete ON evidence_templates FOR DELETE
    USING (organization_id IS NULL OR organization_id = get_current_tenant());

-- evidence_requirements
ALTER TABLE evidence_requirements ENABLE ROW LEVEL SECURITY;
ALTER TABLE evidence_requirements FORCE ROW LEVEL SECURITY;

CREATE POLICY evidence_reqs_tenant_select ON evidence_requirements FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY evidence_reqs_tenant_insert ON evidence_requirements FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY evidence_reqs_tenant_update ON evidence_requirements FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY evidence_reqs_tenant_delete ON evidence_requirements FOR DELETE
    USING (organization_id = get_current_tenant());

-- evidence_test_suites
ALTER TABLE evidence_test_suites ENABLE ROW LEVEL SECURITY;
ALTER TABLE evidence_test_suites FORCE ROW LEVEL SECURITY;

CREATE POLICY evidence_suites_tenant_select ON evidence_test_suites FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY evidence_suites_tenant_insert ON evidence_test_suites FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY evidence_suites_tenant_update ON evidence_test_suites FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY evidence_suites_tenant_delete ON evidence_test_suites FOR DELETE
    USING (organization_id = get_current_tenant());

-- evidence_test_cases
ALTER TABLE evidence_test_cases ENABLE ROW LEVEL SECURITY;
ALTER TABLE evidence_test_cases FORCE ROW LEVEL SECURITY;

CREATE POLICY evidence_cases_tenant_select ON evidence_test_cases FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY evidence_cases_tenant_insert ON evidence_test_cases FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY evidence_cases_tenant_update ON evidence_test_cases FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY evidence_cases_tenant_delete ON evidence_test_cases FOR DELETE
    USING (organization_id = get_current_tenant());

-- evidence_test_runs
ALTER TABLE evidence_test_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE evidence_test_runs FORCE ROW LEVEL SECURITY;

CREATE POLICY evidence_runs_tenant_select ON evidence_test_runs FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY evidence_runs_tenant_insert ON evidence_test_runs FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY evidence_runs_tenant_update ON evidence_test_runs FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY evidence_runs_tenant_delete ON evidence_test_runs FOR DELETE
    USING (organization_id = get_current_tenant());
