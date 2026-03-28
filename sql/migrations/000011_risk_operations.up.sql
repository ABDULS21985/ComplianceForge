-- Migration 011: Risk Operations — Assessments, Treatments, KRIs, Control Mappings & Scenarios
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - risk_assessments stores the full history of every assessment, enabling
--     trend analysis and audit trails. Each assessment captures before/after
--     scores to track risk evolution.
--   - risk_treatments supports the four ISO 31000 treatment types (mitigate,
--     transfer, avoid, accept) with cost tracking and progress monitoring
--   - risk_indicators (KRIs) support both manual and automated collection with
--     traffic-light thresholds (green/amber/red)
--   - risk_indicator_values stores historical KRI measurements for trend analysis
--   - risk_control_mappings links risks to their mitigating controls with
--     effectiveness ratings and contribution percentages
--   - risk_scenarios supports stress testing and scenario analysis as required
--     by EBA guidelines, PRA requirements, and DORA

-- ============================================================================
-- TABLE: risk_assessments
-- ============================================================================

CREATE TABLE risk_assessments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    risk_id             UUID NOT NULL REFERENCES risks(id) ON DELETE CASCADE,
    assessment_type     VARCHAR(30) NOT NULL,
    assessor_user_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    assessment_date     DATE NOT NULL DEFAULT CURRENT_DATE,
    likelihood_before   INT,
    impact_before       INT,
    score_before        DECIMAL(5,2),
    level_before        VARCHAR(20),
    likelihood_after    INT,
    impact_after        INT,
    score_after         DECIMAL(5,2),
    level_after         VARCHAR(20),
    assessment_notes    TEXT,
    methodology         VARCHAR(50),
    confidence_level    VARCHAR(20),
    data_sources        TEXT[] DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_assessment_type CHECK (
        assessment_type IN ('initial', 'periodic', 'triggered', 'post_incident', 'ad_hoc')
    ),
    CONSTRAINT chk_assessment_methodology CHECK (
        methodology IS NULL OR methodology IN ('qualitative', 'semi_quantitative', 'quantitative', 'monte_carlo', 'bayesian', 'bow_tie')
    ),
    CONSTRAINT chk_assessment_confidence CHECK (
        confidence_level IS NULL OR confidence_level IN ('low', 'medium', 'high', 'very_high')
    )
);

CREATE INDEX idx_risk_assess_org ON risk_assessments(organization_id);
CREATE INDEX idx_risk_assess_risk ON risk_assessments(risk_id);
CREATE INDEX idx_risk_assess_date ON risk_assessments(risk_id, assessment_date DESC);
CREATE INDEX idx_risk_assess_type ON risk_assessments(assessment_type);
CREATE INDEX idx_risk_assess_assessor ON risk_assessments(assessor_user_id) WHERE assessor_user_id IS NOT NULL;

ALTER TABLE risk_assessments ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk_assessments FORCE ROW LEVEL SECURITY;

CREATE POLICY risk_assess_tenant_select ON risk_assessments FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY risk_assess_tenant_insert ON risk_assessments FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY risk_assess_tenant_update ON risk_assessments FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY risk_assess_tenant_delete ON risk_assessments FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE risk_assessments IS 'Historical record of every risk assessment. Captures before/after scores for trend analysis and audit trails.';
COMMENT ON COLUMN risk_assessments.methodology IS 'Assessment methodology: qualitative (expert judgement), semi_quantitative (scales), quantitative (statistical), monte_carlo (simulation), bayesian (probabilistic), bow_tie (cause/effect).';

-- ============================================================================
-- TABLE: risk_treatments
-- ============================================================================

CREATE TABLE risk_treatments (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    risk_id                     UUID NOT NULL REFERENCES risks(id) ON DELETE CASCADE,
    treatment_type              VARCHAR(20) NOT NULL,
    title                       VARCHAR(500) NOT NULL,
    description                 TEXT,
    status                      VARCHAR(20) NOT NULL DEFAULT 'planned',
    priority                    VARCHAR(10),
    owner_user_id               UUID REFERENCES users(id) ON DELETE SET NULL,
    start_date                  DATE,
    target_date                 DATE,
    completed_date              DATE,
    estimated_cost_eur          DECIMAL(12,2),
    actual_cost_eur             DECIMAL(12,2),
    expected_risk_reduction     DECIMAL(5,2),
    progress_percentage         INT NOT NULL DEFAULT 0,
    linked_control_ids          UUID[] DEFAULT '{}',
    notes                       TEXT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_treatment_type CHECK (
        treatment_type IN ('mitigate', 'transfer', 'avoid', 'accept')
    ),
    CONSTRAINT chk_treatment_status CHECK (
        status IN ('planned', 'in_progress', 'completed', 'overdue', 'cancelled')
    ),
    CONSTRAINT chk_treatment_priority CHECK (
        priority IS NULL OR priority IN ('critical', 'high', 'medium', 'low')
    ),
    CONSTRAINT chk_treatment_progress CHECK (progress_percentage >= 0 AND progress_percentage <= 100),
    CONSTRAINT chk_treatment_risk_reduction CHECK (
        expected_risk_reduction IS NULL OR (expected_risk_reduction >= 0 AND expected_risk_reduction <= 100)
    ),
    CONSTRAINT chk_treatment_dates CHECK (
        target_date IS NULL OR start_date IS NULL OR target_date >= start_date
    )
);

CREATE INDEX idx_treatment_org ON risk_treatments(organization_id);
CREATE INDEX idx_treatment_risk ON risk_treatments(risk_id);
CREATE INDEX idx_treatment_status ON risk_treatments(organization_id, status);
CREATE INDEX idx_treatment_owner ON risk_treatments(owner_user_id) WHERE owner_user_id IS NOT NULL;
CREATE INDEX idx_treatment_target ON risk_treatments(target_date) WHERE target_date IS NOT NULL AND status NOT IN ('completed', 'cancelled');
CREATE INDEX idx_treatment_overdue ON risk_treatments(organization_id)
    WHERE status = 'overdue' OR (status IN ('planned', 'in_progress') AND target_date < CURRENT_DATE);

CREATE TRIGGER trg_risk_treatments_updated_at
    BEFORE UPDATE ON risk_treatments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE risk_treatments ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk_treatments FORCE ROW LEVEL SECURITY;

CREATE POLICY treatment_tenant_select ON risk_treatments FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY treatment_tenant_insert ON risk_treatments FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY treatment_tenant_update ON risk_treatments FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY treatment_tenant_delete ON risk_treatments FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE risk_treatments IS 'Risk treatment/mitigation plans. Supports ISO 31000 treatment types: mitigate, transfer, avoid, accept.';
COMMENT ON COLUMN risk_treatments.expected_risk_reduction IS 'Percentage of risk score reduction expected (0-100). Used to forecast residual risk after treatment completion.';

-- ============================================================================
-- TABLE: risk_indicators (KRIs)
-- ============================================================================

CREATE TABLE risk_indicators (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    risk_id                 UUID REFERENCES risks(id) ON DELETE SET NULL,
    name                    VARCHAR(255) NOT NULL,
    description             TEXT,
    metric_type             VARCHAR(30) NOT NULL,
    measurement_unit        VARCHAR(50),
    collection_frequency    VARCHAR(20) NOT NULL DEFAULT 'monthly',
    data_source             VARCHAR(255),
    threshold_green         DECIMAL(12,2),
    threshold_amber         DECIMAL(12,2),
    threshold_red           DECIMAL(12,2),
    current_value           DECIMAL(12,2),
    trend                   VARCHAR(20),
    owner_user_id           UUID REFERENCES users(id) ON DELETE SET NULL,
    last_updated_at         TIMESTAMPTZ,
    is_automated            BOOLEAN NOT NULL DEFAULT false,
    automation_config       JSONB DEFAULT '{}',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_kri_metric_type CHECK (
        metric_type IN ('count', 'percentage', 'currency', 'duration', 'ratio', 'score', 'boolean')
    ),
    CONSTRAINT chk_kri_frequency CHECK (
        collection_frequency IN ('real_time', 'daily', 'weekly', 'monthly', 'quarterly')
    ),
    CONSTRAINT chk_kri_trend CHECK (
        trend IS NULL OR trend IN ('improving', 'stable', 'deteriorating')
    )
);

CREATE INDEX idx_kri_org ON risk_indicators(organization_id);
CREATE INDEX idx_kri_risk ON risk_indicators(risk_id) WHERE risk_id IS NOT NULL;
CREATE INDEX idx_kri_owner ON risk_indicators(owner_user_id) WHERE owner_user_id IS NOT NULL;
CREATE INDEX idx_kri_automated ON risk_indicators(is_automated) WHERE is_automated = true;

CREATE TRIGGER trg_risk_indicators_updated_at
    BEFORE UPDATE ON risk_indicators
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE risk_indicators ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk_indicators FORCE ROW LEVEL SECURITY;

CREATE POLICY kri_tenant_select ON risk_indicators FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY kri_tenant_insert ON risk_indicators FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY kri_tenant_update ON risk_indicators FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY kri_tenant_delete ON risk_indicators FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE risk_indicators IS 'Key Risk Indicators (KRIs). Support manual and automated collection with traffic-light thresholds.';
COMMENT ON COLUMN risk_indicators.automation_config IS 'Configuration for automated collection: {"source":"api","endpoint":"...","field":"...","transform":"..."}';

-- ============================================================================
-- TABLE: risk_indicator_values (KRI Historical Measurements)
-- ============================================================================

CREATE TABLE risk_indicator_values (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    indicator_id    UUID NOT NULL REFERENCES risk_indicators(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    value           DECIMAL(12,2) NOT NULL,
    status          VARCHAR(10) NOT NULL,
    measured_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    measured_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_kri_value_status CHECK (status IN ('green', 'amber', 'red'))
);

CREATE INDEX idx_kri_values_indicator ON risk_indicator_values(indicator_id);
CREATE INDEX idx_kri_values_org ON risk_indicator_values(organization_id);
CREATE INDEX idx_kri_values_measured ON risk_indicator_values(indicator_id, measured_at DESC);
CREATE INDEX idx_kri_values_status ON risk_indicator_values(organization_id, status);

ALTER TABLE risk_indicator_values ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk_indicator_values FORCE ROW LEVEL SECURITY;

CREATE POLICY kri_val_tenant_select ON risk_indicator_values FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY kri_val_tenant_insert ON risk_indicator_values FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY kri_val_tenant_update ON risk_indicator_values FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY kri_val_tenant_delete ON risk_indicator_values FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE risk_indicator_values IS 'Historical KRI measurements for trend analysis. Immutable records — update the indicator, don''t modify old values.';

-- ============================================================================
-- TABLE: risk_control_mappings
-- ============================================================================

CREATE TABLE risk_control_mappings (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    risk_id                     UUID NOT NULL REFERENCES risks(id) ON DELETE CASCADE,
    control_implementation_id   UUID NOT NULL REFERENCES control_implementations(id) ON DELETE CASCADE,
    effectiveness               VARCHAR(20) NOT NULL DEFAULT 'not_tested',
    contribution_percentage     DECIMAL(5,2),
    notes                       TEXT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_risk_control_map UNIQUE (risk_id, control_implementation_id),
    CONSTRAINT chk_rcm_effectiveness CHECK (
        effectiveness IN ('effective', 'partially_effective', 'ineffective', 'not_tested')
    ),
    CONSTRAINT chk_rcm_contribution CHECK (
        contribution_percentage IS NULL OR (contribution_percentage >= 0 AND contribution_percentage <= 100)
    )
);

CREATE INDEX idx_rcm_org ON risk_control_mappings(organization_id);
CREATE INDEX idx_rcm_risk ON risk_control_mappings(risk_id);
CREATE INDEX idx_rcm_control ON risk_control_mappings(control_implementation_id);
CREATE INDEX idx_rcm_effectiveness ON risk_control_mappings(organization_id, effectiveness);

CREATE TRIGGER trg_risk_control_mappings_updated_at
    BEFORE UPDATE ON risk_control_mappings
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE risk_control_mappings ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk_control_mappings FORCE ROW LEVEL SECURITY;

CREATE POLICY rcm_tenant_select ON risk_control_mappings FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY rcm_tenant_insert ON risk_control_mappings FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY rcm_tenant_update ON risk_control_mappings FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY rcm_tenant_delete ON risk_control_mappings FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE risk_control_mappings IS 'Maps risks to their mitigating controls with effectiveness ratings. Enables risk-control coverage analysis.';
COMMENT ON COLUMN risk_control_mappings.contribution_percentage IS 'How much this control contributes to reducing the risk (0-100). All controls for a risk should ideally sum to ~100%.';

-- ============================================================================
-- TABLE: risk_scenarios
-- ============================================================================

CREATE TABLE risk_scenarios (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                    VARCHAR(255) NOT NULL,
    description             TEXT,
    scenario_type           VARCHAR(30) NOT NULL,
    assumptions             JSONB DEFAULT '[]',
    parameters              JSONB DEFAULT '{}',
    results                 JSONB DEFAULT '{}',
    risk_ids                UUID[] DEFAULT '{}',
    probability             DECIMAL(5,4),
    estimated_impact_eur    DECIMAL(15,2),
    time_horizon            VARCHAR(20),
    created_by              UUID REFERENCES users(id) ON DELETE SET NULL,
    approved_by             UUID REFERENCES users(id) ON DELETE SET NULL,
    status                  VARCHAR(20) NOT NULL DEFAULT 'draft',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_scenario_type CHECK (
        scenario_type IN ('best_case', 'worst_case', 'expected', 'stress_test', 'reverse_stress', 'what_if')
    ),
    CONSTRAINT chk_scenario_status CHECK (
        status IN ('draft', 'approved', 'archived')
    ),
    CONSTRAINT chk_scenario_time_horizon CHECK (
        time_horizon IS NULL OR time_horizon IN ('1_year', '3_years', '5_years', '10_years')
    ),
    CONSTRAINT chk_scenario_probability CHECK (
        probability IS NULL OR (probability >= 0 AND probability <= 1)
    )
);

CREATE INDEX idx_scenario_org ON risk_scenarios(organization_id);
CREATE INDEX idx_scenario_type ON risk_scenarios(scenario_type);
CREATE INDEX idx_scenario_status ON risk_scenarios(organization_id, status);
CREATE INDEX idx_scenario_risk_ids ON risk_scenarios USING gin (risk_ids);

CREATE TRIGGER trg_risk_scenarios_updated_at
    BEFORE UPDATE ON risk_scenarios
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE risk_scenarios ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk_scenarios FORCE ROW LEVEL SECURITY;

CREATE POLICY scenario_tenant_select ON risk_scenarios FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY scenario_tenant_insert ON risk_scenarios FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY scenario_tenant_update ON risk_scenarios FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY scenario_tenant_delete ON risk_scenarios FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE risk_scenarios IS 'Risk scenario analysis and stress testing. Supports EBA/PRA requirements and DORA ICT risk scenarios.';
COMMENT ON COLUMN risk_scenarios.parameters IS 'Scenario input variables: {"affected_systems": 50, "downtime_hours": 72, "data_records_compromised": 1000000}';
COMMENT ON COLUMN risk_scenarios.results IS 'Calculated outcomes: {"total_financial_impact": 5000000, "recovery_time_hours": 168, "regulatory_fines": 2000000}';
