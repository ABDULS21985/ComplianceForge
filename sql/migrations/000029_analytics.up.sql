-- Migration 029: Advanced Analytics
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - analytics_snapshots store periodic metric snapshots (daily/weekly/monthly)
--     as JSONB blobs. The snapshot_type + date + org composite unique constraint
--     prevents duplicate snapshots. This is the foundation for trend analysis.
--   - analytics_compliance_trends track per-framework compliance scores over time
--     with pre-computed deltas (7d/30d/90d) and trend direction for dashboard
--     rendering without expensive on-the-fly calculations.
--   - analytics_risk_predictions store ML model outputs: predicted risk values
--     with confidence intervals, model version tracking, and actual-value feedback
--     for model accuracy evaluation.
--   - analytics_benchmarks are aggregated, anonymized cross-org metrics (no RLS,
--     no organization_id). Percentile distributions enable "how do we compare?"
--     dashboards without exposing individual org data.
--   - analytics_custom_dashboards store user-created dashboard layouts as JSONB,
--     with sharing and default flags. Each dashboard references widget types.
--   - analytics_widget_types is a global catalog of available widget definitions
--     with available metrics and default configuration. No RLS — shared across orgs.

-- ============================================================================
-- TABLE: analytics_snapshots
-- ============================================================================

CREATE TABLE analytics_snapshots (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    snapshot_type       VARCHAR(30) NOT NULL
                        CHECK (snapshot_type IN ('daily_summary', 'weekly_summary', 'monthly_summary', 'compliance_posture', 'risk_posture', 'vendor_posture', 'custom')),
    snapshot_date       DATE NOT NULL,
    metrics             JSONB NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_analytics_snapshots_org_type_date UNIQUE (organization_id, snapshot_type, snapshot_date)
);

-- Indexes
CREATE INDEX idx_analytics_snapshots_org ON analytics_snapshots(organization_id);
CREATE INDEX idx_analytics_snapshots_org_type ON analytics_snapshots(organization_id, snapshot_type);
CREATE INDEX idx_analytics_snapshots_org_date ON analytics_snapshots(organization_id, snapshot_date DESC);
CREATE INDEX idx_analytics_snapshots_date ON analytics_snapshots(snapshot_date DESC);
CREATE INDEX idx_analytics_snapshots_metrics ON analytics_snapshots USING GIN (metrics);

COMMENT ON TABLE analytics_snapshots IS 'Periodic metric snapshots for trend analysis. Each snapshot captures a point-in-time view of organizational metrics as JSONB. Unique per org + type + date.';
COMMENT ON COLUMN analytics_snapshots.metrics IS 'JSONB metrics payload — structure varies by snapshot_type. E.g., {"total_controls": 142, "implemented": 98, "compliance_score": 69.01, "open_risks": 12, "critical_risks": 2}';
COMMENT ON COLUMN analytics_snapshots.snapshot_type IS 'Category of snapshot: daily/weekly/monthly summaries, or domain-specific posture snapshots.';

-- ============================================================================
-- TABLE: analytics_compliance_trends
-- ============================================================================

CREATE TABLE analytics_compliance_trends (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    framework_id            UUID,
    framework_code          VARCHAR(50),
    measurement_date        DATE NOT NULL,
    compliance_score        DECIMAL(5,2),
    controls_implemented    INT,
    controls_total          INT,
    maturity_avg            DECIMAL(3,2),
    score_change_7d         DECIMAL(5,2),
    score_change_30d        DECIMAL(5,2),
    score_change_90d        DECIMAL(5,2),
    trend_direction         VARCHAR(10)
                            CHECK (trend_direction IS NULL OR trend_direction IN ('improving', 'stable', 'declining')),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_compliance_trends_org_fw_date UNIQUE (organization_id, framework_id, measurement_date)
);

-- Indexes
CREATE INDEX idx_comp_trends_org ON analytics_compliance_trends(organization_id);
CREATE INDEX idx_comp_trends_org_fw ON analytics_compliance_trends(organization_id, framework_id);
CREATE INDEX idx_comp_trends_org_date ON analytics_compliance_trends(organization_id, measurement_date DESC);
CREATE INDEX idx_comp_trends_fw_date ON analytics_compliance_trends(framework_id, measurement_date DESC) WHERE framework_id IS NOT NULL;
CREATE INDEX idx_comp_trends_direction ON analytics_compliance_trends(organization_id, trend_direction) WHERE trend_direction IS NOT NULL;

COMMENT ON TABLE analytics_compliance_trends IS 'Per-framework compliance score time series with pre-computed deltas and trend direction. Enables fast trend dashboard rendering without on-the-fly calculation.';
COMMENT ON COLUMN analytics_compliance_trends.score_change_7d IS 'Change in compliance_score vs. 7 days ago. Positive = improvement.';
COMMENT ON COLUMN analytics_compliance_trends.trend_direction IS 'Derived trend direction based on recent score changes: improving, stable, or declining.';
COMMENT ON COLUMN analytics_compliance_trends.maturity_avg IS 'Average maturity level across all controls in the framework (0.00–5.00).';

-- ============================================================================
-- TABLE: analytics_risk_predictions
-- ============================================================================

CREATE TABLE analytics_risk_predictions (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    risk_id                 UUID,
    prediction_date         DATE NOT NULL,
    prediction_type         VARCHAR(30) NOT NULL
                            CHECK (prediction_type IN ('risk_score', 'likelihood', 'impact', 'velocity', 'emerging_risk', 'trend_forecast')),
    predicted_value         DECIMAL(10,4) NOT NULL,
    confidence_interval_low DECIMAL(10,4),
    confidence_interval_high DECIMAL(10,4),
    confidence_level        DECIMAL(3,2)
                            CHECK (confidence_level IS NULL OR (confidence_level >= 0 AND confidence_level <= 1)),
    model_version           VARCHAR(50),
    input_features          JSONB,
    actual_value            DECIMAL(10,4),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_risk_predictions_org ON analytics_risk_predictions(organization_id);
CREATE INDEX idx_risk_predictions_risk ON analytics_risk_predictions(risk_id) WHERE risk_id IS NOT NULL;
CREATE INDEX idx_risk_predictions_org_date ON analytics_risk_predictions(organization_id, prediction_date DESC);
CREATE INDEX idx_risk_predictions_type ON analytics_risk_predictions(organization_id, prediction_type);
CREATE INDEX idx_risk_predictions_model ON analytics_risk_predictions(model_version) WHERE model_version IS NOT NULL;

COMMENT ON TABLE analytics_risk_predictions IS 'ML-generated risk predictions with confidence intervals. Tracks predicted vs. actual values for model accuracy evaluation over time.';
COMMENT ON COLUMN analytics_risk_predictions.confidence_level IS 'Statistical confidence level (0.00–1.00), e.g., 0.95 for a 95% confidence interval.';
COMMENT ON COLUMN analytics_risk_predictions.input_features IS 'JSONB snapshot of input features used for prediction: {"control_coverage": 0.72, "incident_count_30d": 3, "vendor_risk_avg": 2.1}';
COMMENT ON COLUMN analytics_risk_predictions.actual_value IS 'Actual observed value (populated retroactively) for model accuracy tracking.';

-- ============================================================================
-- TABLE: analytics_benchmarks (global — no RLS, no organization_id)
-- ============================================================================

CREATE TABLE analytics_benchmarks (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    benchmark_type      VARCHAR(30) NOT NULL
                        CHECK (benchmark_type IN ('compliance_score', 'risk_score', 'maturity_level', 'incident_rate', 'remediation_time', 'vendor_risk', 'control_coverage')),
    category            VARCHAR(100),
    metric_name         VARCHAR(200) NOT NULL,
    period              VARCHAR(20) NOT NULL,
    percentile_25       DECIMAL(10,4),
    percentile_50       DECIMAL(10,4),
    percentile_75       DECIMAL(10,4),
    percentile_90       DECIMAL(10,4),
    sample_size         INT NOT NULL DEFAULT 0,
    calculated_at       TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_benchmarks_type ON analytics_benchmarks(benchmark_type);
CREATE INDEX idx_benchmarks_category ON analytics_benchmarks(category) WHERE category IS NOT NULL;
CREATE INDEX idx_benchmarks_period ON analytics_benchmarks(period);
CREATE INDEX idx_benchmarks_calculated ON analytics_benchmarks(calculated_at DESC);
CREATE INDEX idx_benchmarks_type_category_period ON analytics_benchmarks(benchmark_type, category, period);

COMMENT ON TABLE analytics_benchmarks IS 'Aggregated, anonymized cross-organization benchmark data. Percentile distributions enable comparative dashboards without exposing individual organization data. No RLS — globally accessible.';
COMMENT ON COLUMN analytics_benchmarks.category IS 'Benchmark category for grouping: industry, region, company size, framework. E.g., "healthcare", "EU", "SME", "ISO27001".';
COMMENT ON COLUMN analytics_benchmarks.sample_size IS 'Number of organizations contributing to this benchmark calculation. Minimum threshold should be enforced at application layer.';

-- ============================================================================
-- TABLE: analytics_custom_dashboards
-- ============================================================================

CREATE TABLE analytics_custom_dashboards (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                VARCHAR(200) NOT NULL,
    description         TEXT,
    layout              JSONB NOT NULL,
    is_default          BOOLEAN NOT NULL DEFAULT false,
    is_shared           BOOLEAN NOT NULL DEFAULT false,
    owner_user_id       UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_custom_dashboards_org ON analytics_custom_dashboards(organization_id);
CREATE INDEX idx_custom_dashboards_owner ON analytics_custom_dashboards(owner_user_id) WHERE owner_user_id IS NOT NULL;
CREATE INDEX idx_custom_dashboards_default ON analytics_custom_dashboards(organization_id, is_default) WHERE is_default = true;
CREATE INDEX idx_custom_dashboards_shared ON analytics_custom_dashboards(organization_id, is_shared) WHERE is_shared = true;

-- Trigger
CREATE TRIGGER trg_custom_dashboards_updated_at
    BEFORE UPDATE ON analytics_custom_dashboards
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE analytics_custom_dashboards IS 'User-created dashboard configurations with flexible JSONB layouts. Dashboards can be shared within the organization or set as the org default.';
COMMENT ON COLUMN analytics_custom_dashboards.layout IS 'JSONB layout definition: {"rows": [{"widgets": [{"widget_type": "compliance_gauge", "config": {...}, "width": 4, "height": 2}]}]}';

-- ============================================================================
-- TABLE: analytics_widget_types (global catalog — no RLS)
-- ============================================================================

CREATE TABLE analytics_widget_types (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    widget_type         VARCHAR(50) NOT NULL,
    name                VARCHAR(200) NOT NULL,
    description         TEXT,
    available_metrics   TEXT[],
    default_config      JSONB,
    min_width           INT NOT NULL DEFAULT 1,
    min_height          INT NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_widget_types_type UNIQUE (widget_type)
);

-- Indexes
CREATE INDEX idx_widget_types_type ON analytics_widget_types(widget_type);
CREATE INDEX idx_widget_types_metrics ON analytics_widget_types USING GIN (available_metrics);

COMMENT ON TABLE analytics_widget_types IS 'Global catalog of available dashboard widget types with available metrics, default configuration, and minimum size constraints. Not tenant-scoped — shared across organizations.';
COMMENT ON COLUMN analytics_widget_types.widget_type IS 'Unique widget type identifier: "compliance_gauge", "risk_heatmap", "trend_line", "kpi_card", etc.';
COMMENT ON COLUMN analytics_widget_types.available_metrics IS 'Array of metric keys this widget can display: ["compliance_score", "controls_implemented", "risk_count"].';
COMMENT ON COLUMN analytics_widget_types.default_config IS 'JSONB default configuration: {"refresh_interval_sec": 300, "color_scheme": "default", "show_legend": true}';

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- analytics_benchmarks: NO RLS (global aggregated data)
-- analytics_widget_types: NO RLS (global catalog)

-- analytics_snapshots
ALTER TABLE analytics_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE analytics_snapshots FORCE ROW LEVEL SECURITY;

CREATE POLICY analytics_snapshots_tenant_select ON analytics_snapshots FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY analytics_snapshots_tenant_insert ON analytics_snapshots FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY analytics_snapshots_tenant_update ON analytics_snapshots FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY analytics_snapshots_tenant_delete ON analytics_snapshots FOR DELETE
    USING (organization_id = get_current_tenant());

-- analytics_compliance_trends
ALTER TABLE analytics_compliance_trends ENABLE ROW LEVEL SECURITY;
ALTER TABLE analytics_compliance_trends FORCE ROW LEVEL SECURITY;

CREATE POLICY comp_trends_tenant_select ON analytics_compliance_trends FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY comp_trends_tenant_insert ON analytics_compliance_trends FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY comp_trends_tenant_update ON analytics_compliance_trends FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY comp_trends_tenant_delete ON analytics_compliance_trends FOR DELETE
    USING (organization_id = get_current_tenant());

-- analytics_risk_predictions
ALTER TABLE analytics_risk_predictions ENABLE ROW LEVEL SECURITY;
ALTER TABLE analytics_risk_predictions FORCE ROW LEVEL SECURITY;

CREATE POLICY risk_predictions_tenant_select ON analytics_risk_predictions FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY risk_predictions_tenant_insert ON analytics_risk_predictions FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY risk_predictions_tenant_update ON analytics_risk_predictions FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY risk_predictions_tenant_delete ON analytics_risk_predictions FOR DELETE
    USING (organization_id = get_current_tenant());

-- analytics_custom_dashboards
ALTER TABLE analytics_custom_dashboards ENABLE ROW LEVEL SECURITY;
ALTER TABLE analytics_custom_dashboards FORCE ROW LEVEL SECURITY;

CREATE POLICY custom_dashboards_tenant_select ON analytics_custom_dashboards FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY custom_dashboards_tenant_insert ON analytics_custom_dashboards FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY custom_dashboards_tenant_update ON analytics_custom_dashboards FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY custom_dashboards_tenant_delete ON analytics_custom_dashboards FOR DELETE
    USING (organization_id = get_current_tenant());
