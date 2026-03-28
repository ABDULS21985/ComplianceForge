-- Migration 012: Risk Analytics Views
-- ComplianceForge GRC Platform
--
-- Pre-calculated views for risk dashboards, heatmaps, and reports.
-- All views include organization_id so RLS continues to apply.

-- ============================================================================
-- VIEW: v_risk_heatmap
-- Data source for the risk heatmap visualization.
-- ============================================================================

CREATE OR REPLACE VIEW v_risk_heatmap AS
SELECT
    r.organization_id,
    r.id AS risk_id,
    r.risk_ref,
    r.title,
    r.status,
    rc.name AS category_name,
    rc.code AS category_code,
    rc.color_hex AS category_color,
    r.risk_source,

    -- Inherent risk
    r.inherent_likelihood,
    r.inherent_impact,
    r.inherent_risk_score,
    r.inherent_risk_level,

    -- Residual risk
    r.residual_likelihood,
    r.residual_impact,
    r.residual_risk_score,
    r.residual_risk_level,

    -- Target risk
    r.target_likelihood,
    r.target_impact,
    r.target_risk_score,
    r.target_risk_level,

    -- Risk movement (inherent → residual)
    CASE
        WHEN r.inherent_risk_score IS NOT NULL AND r.residual_risk_score IS NOT NULL
        THEN r.inherent_risk_score - r.residual_risk_score
        ELSE NULL
    END AS risk_reduction,

    -- Gap to target
    CASE
        WHEN r.residual_risk_score IS NOT NULL AND r.target_risk_score IS NOT NULL
        THEN r.residual_risk_score - r.target_risk_score
        ELSE NULL
    END AS gap_to_target,

    r.risk_velocity,
    r.risk_proximity,
    r.financial_impact_eur,
    r.is_emerging,
    r.owner_user_id,
    ou.first_name || ' ' || ou.last_name AS owner_name,
    r.next_review_date,
    r.last_assessed_date,

    -- Treatment count
    (SELECT COUNT(*) FROM risk_treatments rt
     WHERE rt.risk_id = r.id AND rt.status NOT IN ('completed', 'cancelled')) AS active_treatments,

    -- Control coverage count
    (SELECT COUNT(*) FROM risk_control_mappings rcm
     WHERE rcm.risk_id = r.id) AS mapped_controls

FROM risks r
LEFT JOIN risk_categories rc ON rc.id = r.risk_category_id
LEFT JOIN users ou ON ou.id = r.owner_user_id
WHERE r.deleted_at IS NULL;

COMMENT ON VIEW v_risk_heatmap IS 'Risk heatmap data with inherent/residual/target scores, risk movement, and owner details.';

-- ============================================================================
-- VIEW: v_risk_treatment_progress
-- Treatment status summary with overdue tracking.
-- ============================================================================

CREATE OR REPLACE VIEW v_risk_treatment_progress AS
SELECT
    rt.organization_id,
    rt.id AS treatment_id,
    rt.risk_id,
    r.risk_ref,
    r.title AS risk_title,
    r.residual_risk_level,
    rt.title AS treatment_title,
    rt.treatment_type,
    rt.status,
    rt.priority,
    rt.owner_user_id,
    tu.first_name || ' ' || tu.last_name AS owner_name,
    rt.start_date,
    rt.target_date,
    rt.completed_date,
    rt.progress_percentage,
    rt.estimated_cost_eur,
    rt.actual_cost_eur,
    rt.expected_risk_reduction,

    -- Overdue calculation
    CASE
        WHEN rt.status IN ('completed', 'cancelled') THEN false
        WHEN rt.target_date IS NOT NULL AND rt.target_date < CURRENT_DATE THEN true
        ELSE false
    END AS is_overdue,

    -- Days until/since target
    CASE
        WHEN rt.target_date IS NOT NULL THEN rt.target_date - CURRENT_DATE
        ELSE NULL
    END AS days_until_target,

    -- Cost variance
    CASE
        WHEN rt.estimated_cost_eur IS NOT NULL AND rt.actual_cost_eur IS NOT NULL
        THEN rt.actual_cost_eur - rt.estimated_cost_eur
        ELSE NULL
    END AS cost_variance_eur

FROM risk_treatments rt
JOIN risks r ON r.id = rt.risk_id AND r.deleted_at IS NULL
LEFT JOIN users tu ON tu.id = rt.owner_user_id;

COMMENT ON VIEW v_risk_treatment_progress IS 'Treatment plans with overdue tracking, cost variance, and associated risk context.';

-- ============================================================================
-- VIEW: v_kri_dashboard
-- Current KRI values with status and trends.
-- ============================================================================

CREATE OR REPLACE VIEW v_kri_dashboard AS
SELECT
    ki.organization_id,
    ki.id AS indicator_id,
    ki.name,
    ki.description,
    ki.metric_type,
    ki.measurement_unit,
    ki.collection_frequency,
    ki.data_source,
    ki.threshold_green,
    ki.threshold_amber,
    ki.threshold_red,
    ki.current_value,
    ki.trend,
    ki.is_automated,
    ki.last_updated_at,
    ki.owner_user_id,
    ku.first_name || ' ' || ku.last_name AS owner_name,

    -- Current status based on thresholds
    CASE
        WHEN ki.current_value IS NULL THEN 'no_data'
        WHEN ki.threshold_red IS NOT NULL AND ki.current_value >= ki.threshold_red THEN 'red'
        WHEN ki.threshold_amber IS NOT NULL AND ki.current_value >= ki.threshold_amber THEN 'amber'
        WHEN ki.threshold_green IS NOT NULL AND ki.current_value <= ki.threshold_green THEN 'green'
        ELSE 'green'
    END AS current_status,

    -- Linked risk info
    ki.risk_id,
    r.risk_ref,
    r.title AS risk_title,
    r.residual_risk_level,

    -- Days since last update
    CASE
        WHEN ki.last_updated_at IS NOT NULL
        THEN EXTRACT(DAY FROM NOW() - ki.last_updated_at)::INT
        ELSE NULL
    END AS days_since_update,

    -- Is collection overdue?
    CASE
        WHEN ki.last_updated_at IS NULL THEN true
        WHEN ki.collection_frequency = 'daily' AND ki.last_updated_at < NOW() - INTERVAL '2 days' THEN true
        WHEN ki.collection_frequency = 'weekly' AND ki.last_updated_at < NOW() - INTERVAL '10 days' THEN true
        WHEN ki.collection_frequency = 'monthly' AND ki.last_updated_at < NOW() - INTERVAL '35 days' THEN true
        WHEN ki.collection_frequency = 'quarterly' AND ki.last_updated_at < NOW() - INTERVAL '100 days' THEN true
        ELSE false
    END AS is_collection_overdue,

    -- Latest 3 historical values (for sparkline)
    (SELECT json_agg(sub ORDER BY sub.measured_at DESC)
     FROM (
         SELECT value, status, measured_at
         FROM risk_indicator_values riv
         WHERE riv.indicator_id = ki.id
         ORDER BY riv.measured_at DESC
         LIMIT 3
     ) sub
    ) AS recent_values

FROM risk_indicators ki
LEFT JOIN users ku ON ku.id = ki.owner_user_id
LEFT JOIN risks r ON r.id = ki.risk_id AND r.deleted_at IS NULL;

COMMENT ON VIEW v_kri_dashboard IS 'KRI dashboard with current status, trends, collection overdue warnings, and sparkline data.';

-- ============================================================================
-- VIEW: v_risk_control_coverage
-- Risks and their mapped controls with effectiveness.
-- ============================================================================

CREATE OR REPLACE VIEW v_risk_control_coverage AS
SELECT
    rcm.organization_id,
    r.id AS risk_id,
    r.risk_ref,
    r.title AS risk_title,
    r.residual_risk_level,
    r.residual_risk_score,
    r.status AS risk_status,

    rcm.id AS mapping_id,
    rcm.effectiveness,
    rcm.contribution_percentage,

    ci.id AS control_implementation_id,
    ci.status AS implementation_status,
    ci.maturity_level,
    ci.effectiveness_score AS control_effectiveness_score,

    fc.code AS control_code,
    fc.title AS control_title,
    cf.code AS framework_code,
    cf.name AS framework_name,

    -- Is this control actually helping?
    CASE
        WHEN rcm.effectiveness = 'effective' AND ci.status IN ('implemented', 'effective') THEN true
        ELSE false
    END AS is_actively_mitigating

FROM risk_control_mappings rcm
JOIN risks r ON r.id = rcm.risk_id AND r.deleted_at IS NULL
JOIN control_implementations ci ON ci.id = rcm.control_implementation_id AND ci.deleted_at IS NULL
JOIN framework_controls fc ON fc.id = ci.framework_control_id
JOIN organization_frameworks ofw ON ofw.id = ci.org_framework_id
JOIN compliance_frameworks cf ON cf.id = ofw.framework_id AND cf.deleted_at IS NULL;

COMMENT ON VIEW v_risk_control_coverage IS 'Maps risks to their mitigating controls with effectiveness status. Used for coverage analysis and gap identification.';

-- ============================================================================
-- VIEW: v_top_risks
-- Top risks by residual score per organization.
-- ============================================================================

CREATE OR REPLACE VIEW v_top_risks AS
SELECT
    r.organization_id,
    r.id AS risk_id,
    r.risk_ref,
    r.title,
    r.description,
    rc.name AS category_name,
    rc.color_hex AS category_color,
    r.status,
    r.risk_source,
    r.inherent_risk_score,
    r.inherent_risk_level,
    r.residual_risk_score,
    r.residual_risk_level,
    r.target_risk_score,
    r.financial_impact_eur,
    r.risk_velocity,
    r.risk_proximity,
    r.is_emerging,
    r.owner_user_id,
    ou.first_name || ' ' || ou.last_name AS owner_name,
    r.next_review_date,
    r.last_assessed_date,
    r.identified_date,

    -- Treatment summary
    (SELECT COUNT(*) FROM risk_treatments rt WHERE rt.risk_id = r.id) AS total_treatments,
    (SELECT COUNT(*) FROM risk_treatments rt WHERE rt.risk_id = r.id AND rt.status = 'completed') AS completed_treatments,
    (SELECT COUNT(*) FROM risk_treatments rt WHERE rt.risk_id = r.id
     AND rt.status NOT IN ('completed', 'cancelled') AND rt.target_date < CURRENT_DATE) AS overdue_treatments,

    -- Control coverage
    (SELECT COUNT(*) FROM risk_control_mappings rcm WHERE rcm.risk_id = r.id) AS total_controls,
    (SELECT COUNT(*) FROM risk_control_mappings rcm WHERE rcm.risk_id = r.id AND rcm.effectiveness = 'effective') AS effective_controls,

    -- Ranking within org
    ROW_NUMBER() OVER (
        PARTITION BY r.organization_id
        ORDER BY r.residual_risk_score DESC NULLS LAST,
                 r.financial_impact_eur DESC NULLS LAST
    ) AS risk_rank

FROM risks r
LEFT JOIN risk_categories rc ON rc.id = r.risk_category_id
LEFT JOIN users ou ON ou.id = r.owner_user_id
WHERE r.deleted_at IS NULL
  AND r.status NOT IN ('closed');

COMMENT ON VIEW v_top_risks IS 'All active risks ranked by residual score. Filter by risk_rank <= 10 or <= 20 for top-N dashboards.';

-- ============================================================================
-- VIEW: v_risk_appetite_compliance
-- Shows whether current risk levels are within appetite/tolerance.
-- ============================================================================

CREATE OR REPLACE VIEW v_risk_appetite_compliance AS
SELECT
    r.organization_id,
    r.id AS risk_id,
    r.risk_ref,
    r.title,
    r.residual_risk_score,
    r.residual_risk_level,
    r.financial_impact_eur,
    rc.name AS category_name,
    rc.code AS category_code,
    ras.appetite_level,
    ras.tolerance_level,
    ras.quantitative_threshold_low,
    ras.quantitative_threshold_high,
    ras.threshold_metric,

    -- Is risk within appetite?
    CASE
        WHEN ras.id IS NULL THEN 'no_appetite_set'
        WHEN ras.appetite_level = 'averse' AND r.residual_risk_level IN ('critical', 'high', 'medium') THEN 'exceeds_appetite'
        WHEN ras.appetite_level = 'minimal' AND r.residual_risk_level IN ('critical', 'high') THEN 'exceeds_appetite'
        WHEN ras.appetite_level = 'cautious' AND r.residual_risk_level = 'critical' THEN 'exceeds_appetite'
        WHEN ras.appetite_level IN ('open', 'hungry') THEN 'within_appetite'
        ELSE 'within_appetite'
    END AS appetite_status,

    -- Is financial impact within quantitative threshold?
    CASE
        WHEN ras.quantitative_threshold_high IS NOT NULL AND r.financial_impact_eur IS NOT NULL
             AND r.financial_impact_eur > ras.quantitative_threshold_high THEN 'exceeds_threshold'
        WHEN ras.quantitative_threshold_low IS NOT NULL AND r.financial_impact_eur IS NOT NULL
             AND r.financial_impact_eur > ras.quantitative_threshold_low THEN 'approaching_threshold'
        ELSE 'within_threshold'
    END AS threshold_status

FROM risks r
LEFT JOIN risk_categories rc ON rc.id = r.risk_category_id
LEFT JOIN risk_appetite_statements ras ON ras.organization_id = r.organization_id
    AND ras.risk_category_id = r.risk_category_id
    AND ras.status = 'approved'
WHERE r.deleted_at IS NULL
  AND r.status NOT IN ('closed');

COMMENT ON VIEW v_risk_appetite_compliance IS 'Compares current risk levels against approved appetite/tolerance statements. Highlights risks exceeding organisational risk appetite.';
