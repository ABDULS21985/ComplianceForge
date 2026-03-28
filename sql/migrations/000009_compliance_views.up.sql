-- Migration 009: Compliance Analytics Views
-- ComplianceForge GRC Platform
--
-- These views provide pre-calculated compliance analytics used by dashboards,
-- reports, and the compliance engine. They avoid complex joins in application code
-- and ensure consistent calculation logic across the platform.
--
-- Design decisions:
--   - Views (not materialized) to always reflect real-time data. For large datasets,
--     the application can cache results with a TTL.
--   - All views include organization_id so RLS continues to apply when queried
--     through the tenant-scoped connection.
--   - Scoring formula: each control status maps to a weight:
--       effective=1.0, implemented=0.85, partial=0.5, planned=0.15,
--       not_implemented=0, not_applicable=excluded from denominator

-- ============================================================================
-- VIEW: v_compliance_score_by_framework
-- Aggregated compliance scores per organization per framework.
-- Used by: compliance dashboard, executive reports, framework overview.
-- ============================================================================

CREATE OR REPLACE VIEW v_compliance_score_by_framework AS
SELECT
    ofw.organization_id,
    ofw.id AS org_framework_id,
    ofw.framework_id,
    cf.code AS framework_code,
    cf.name AS framework_name,
    cf.version AS framework_version,
    ofw.status AS adoption_status,
    ofw.certification_date,
    ofw.certification_expiry,

    -- Control counts by status
    COUNT(ci.id) FILTER (WHERE ci.deleted_at IS NULL) AS total_controls,
    COUNT(ci.id) FILTER (WHERE ci.status = 'effective' AND ci.deleted_at IS NULL) AS effective_count,
    COUNT(ci.id) FILTER (WHERE ci.status = 'implemented' AND ci.deleted_at IS NULL) AS implemented_count,
    COUNT(ci.id) FILTER (WHERE ci.status = 'partial' AND ci.deleted_at IS NULL) AS partial_count,
    COUNT(ci.id) FILTER (WHERE ci.status = 'planned' AND ci.deleted_at IS NULL) AS planned_count,
    COUNT(ci.id) FILTER (WHERE ci.status = 'not_implemented' AND ci.deleted_at IS NULL) AS not_implemented_count,
    COUNT(ci.id) FILTER (WHERE ci.status = 'not_applicable' AND ci.deleted_at IS NULL) AS not_applicable_count,

    -- Maturity distribution
    COUNT(ci.id) FILTER (WHERE ci.maturity_level = 0 AND ci.deleted_at IS NULL) AS maturity_0_count,
    COUNT(ci.id) FILTER (WHERE ci.maturity_level = 1 AND ci.deleted_at IS NULL) AS maturity_1_count,
    COUNT(ci.id) FILTER (WHERE ci.maturity_level = 2 AND ci.deleted_at IS NULL) AS maturity_2_count,
    COUNT(ci.id) FILTER (WHERE ci.maturity_level = 3 AND ci.deleted_at IS NULL) AS maturity_3_count,
    COUNT(ci.id) FILTER (WHERE ci.maturity_level = 4 AND ci.deleted_at IS NULL) AS maturity_4_count,
    COUNT(ci.id) FILTER (WHERE ci.maturity_level = 5 AND ci.deleted_at IS NULL) AS maturity_5_count,

    -- Average maturity level (excluding not_applicable)
    ROUND(AVG(ci.maturity_level) FILTER (
        WHERE ci.status != 'not_applicable' AND ci.deleted_at IS NULL
    ), 2) AS avg_maturity_level,

    -- Weighted compliance score (0-100)
    -- Formula: sum(status_weight) / count(applicable_controls) * 100
    ROUND(
        CASE
            WHEN COUNT(ci.id) FILTER (WHERE ci.status != 'not_applicable' AND ci.deleted_at IS NULL) = 0 THEN 0
            ELSE (
                SUM(
                    CASE ci.status
                        WHEN 'effective' THEN 1.00
                        WHEN 'implemented' THEN 0.85
                        WHEN 'partial' THEN 0.50
                        WHEN 'planned' THEN 0.15
                        WHEN 'not_implemented' THEN 0.00
                        ELSE 0.00
                    END
                ) FILTER (WHERE ci.status != 'not_applicable' AND ci.deleted_at IS NULL)
                /
                COUNT(ci.id) FILTER (WHERE ci.status != 'not_applicable' AND ci.deleted_at IS NULL)::DECIMAL
            ) * 100
        END,
    2) AS compliance_score,

    -- Average effectiveness score (where tested)
    ROUND(AVG(ci.effectiveness_score) FILTER (
        WHERE ci.effectiveness_score IS NOT NULL AND ci.deleted_at IS NULL
    ), 2) AS avg_effectiveness_score,

    -- Remediation stats
    COUNT(ci.id) FILTER (
        WHERE ci.remediation_due_date IS NOT NULL
          AND ci.remediation_due_date < CURRENT_DATE
          AND ci.status NOT IN ('implemented', 'effective', 'not_applicable')
          AND ci.deleted_at IS NULL
    ) AS overdue_remediations,

    -- Testing stats
    COUNT(ci.id) FILTER (
        WHERE ci.last_tested_at IS NOT NULL AND ci.deleted_at IS NULL
    ) AS tested_controls,
    COUNT(ci.id) FILTER (
        WHERE ci.last_test_result = 'fail' AND ci.deleted_at IS NULL
    ) AS failed_tests,

    ofw.last_assessment_date,
    ofw.responsible_user_id

FROM organization_frameworks ofw
JOIN compliance_frameworks cf ON cf.id = ofw.framework_id AND cf.deleted_at IS NULL
LEFT JOIN control_implementations ci ON ci.org_framework_id = ofw.id
GROUP BY
    ofw.organization_id, ofw.id, ofw.framework_id,
    cf.code, cf.name, cf.version,
    ofw.status, ofw.certification_date, ofw.certification_expiry,
    ofw.last_assessment_date, ofw.responsible_user_id;

COMMENT ON VIEW v_compliance_score_by_framework IS 'Aggregated compliance scores per org per framework. Scoring: effective=1.0, implemented=0.85, partial=0.5, planned=0.15, not_implemented=0. N/A excluded from denominator.';

-- ============================================================================
-- VIEW: v_control_gap_analysis
-- Controls that need attention: not implemented, partially implemented, or failed testing.
-- Used by: gap analysis reports, remediation planning, audit prep.
-- ============================================================================

CREATE OR REPLACE VIEW v_control_gap_analysis AS
SELECT
    ci.organization_id,
    ci.id AS control_implementation_id,
    ci.org_framework_id,
    cf.code AS framework_code,
    cf.name AS framework_name,
    fd.code AS domain_code,
    fd.name AS domain_name,
    fc.code AS control_code,
    fc.title AS control_title,
    fc.description AS control_description,
    fc.priority AS control_priority,
    fc.control_type,
    ci.status,
    ci.implementation_status,
    ci.maturity_level,
    ci.gap_description,
    ci.remediation_plan,
    ci.remediation_due_date,
    ci.risk_if_not_implemented,
    ci.owner_user_id,
    ci.last_tested_at,
    ci.last_test_result,
    ci.effectiveness_score,

    -- Is remediation overdue?
    CASE
        WHEN ci.remediation_due_date IS NOT NULL AND ci.remediation_due_date < CURRENT_DATE THEN true
        ELSE false
    END AS is_overdue,

    -- Days until/since remediation due date
    CASE
        WHEN ci.remediation_due_date IS NOT NULL THEN ci.remediation_due_date - CURRENT_DATE
        ELSE NULL
    END AS days_until_due,

    -- Priority score for sorting (lower = more urgent)
    CASE ci.risk_if_not_implemented
        WHEN 'critical' THEN 1
        WHEN 'high' THEN 2
        WHEN 'medium' THEN 3
        WHEN 'low' THEN 4
        ELSE 5
    END AS risk_priority_sort,

    -- Evidence count
    (SELECT COUNT(*) FROM control_evidence ce
     WHERE ce.control_implementation_id = ci.id
       AND ce.is_current = true
       AND ce.deleted_at IS NULL) AS current_evidence_count

FROM control_implementations ci
JOIN organization_frameworks ofw ON ofw.id = ci.org_framework_id
JOIN compliance_frameworks cf ON cf.id = ofw.framework_id AND cf.deleted_at IS NULL
JOIN framework_controls fc ON fc.id = ci.framework_control_id
LEFT JOIN framework_domains fd ON fd.id = fc.domain_id
WHERE ci.deleted_at IS NULL
  AND ci.status NOT IN ('implemented', 'effective', 'not_applicable');

COMMENT ON VIEW v_control_gap_analysis IS 'Controls with gaps: not yet implemented, partially implemented, or failed testing. Sorted by risk priority for remediation planning.';

-- ============================================================================
-- VIEW: v_cross_framework_coverage
-- Shows how implementing controls in one framework covers controls in another.
-- Used by: framework adoption planning, compliance overlap analysis, ROI calculations.
-- ============================================================================

CREATE OR REPLACE VIEW v_cross_framework_coverage AS
SELECT
    ci.organization_id,

    -- Source framework (the one you have implemented)
    src_fw.id AS source_framework_id,
    src_fw.code AS source_framework_code,
    src_fw.name AS source_framework_name,
    src_ctrl.id AS source_control_id,
    src_ctrl.code AS source_control_code,
    src_ctrl.title AS source_control_title,

    -- Target framework (the one you want to cover)
    tgt_fw.id AS target_framework_id,
    tgt_fw.code AS target_framework_code,
    tgt_fw.name AS target_framework_name,
    tgt_ctrl.id AS target_control_id,
    tgt_ctrl.code AS target_control_code,
    tgt_ctrl.title AS target_control_title,

    -- Mapping details
    fcm.mapping_type,
    fcm.mapping_strength,
    fcm.is_verified,

    -- Implementation status of the source control
    ci.status AS source_implementation_status,
    ci.maturity_level AS source_maturity_level,
    ci.effectiveness_score AS source_effectiveness_score,

    -- Effective coverage of the target control based on mapping strength and implementation
    ROUND(
        fcm.mapping_strength *
        CASE ci.status
            WHEN 'effective' THEN 1.00
            WHEN 'implemented' THEN 0.85
            WHEN 'partial' THEN 0.50
            WHEN 'planned' THEN 0.15
            ELSE 0.00
        END,
    2) AS effective_coverage

FROM control_implementations ci
JOIN framework_controls src_ctrl ON src_ctrl.id = ci.framework_control_id
JOIN compliance_frameworks src_fw ON src_fw.id = src_ctrl.framework_id AND src_fw.deleted_at IS NULL
JOIN framework_control_mappings fcm ON fcm.source_control_id = src_ctrl.id
JOIN framework_controls tgt_ctrl ON tgt_ctrl.id = fcm.target_control_id
JOIN compliance_frameworks tgt_fw ON tgt_fw.id = tgt_ctrl.framework_id AND tgt_fw.deleted_at IS NULL
WHERE ci.deleted_at IS NULL
  AND ci.status != 'not_applicable';

COMMENT ON VIEW v_cross_framework_coverage IS 'Shows how implementing controls in one framework covers controls in another, weighted by mapping strength and implementation status.';

-- ============================================================================
-- VIEW: v_framework_summary
-- Quick summary stats for each framework in the system.
-- Used by: framework selection page, admin overview.
-- ============================================================================

CREATE OR REPLACE VIEW v_framework_summary AS
SELECT
    cf.id AS framework_id,
    cf.organization_id,
    cf.code,
    cf.name,
    cf.version,
    cf.issuing_body,
    cf.category,
    cf.applicable_regions,
    cf.is_system_framework,
    cf.is_active,
    cf.effective_date,
    cf.sunset_date,
    cf.color_hex,

    -- Control statistics
    COUNT(DISTINCT fc.id) AS total_controls,
    COUNT(DISTINCT fd.id) AS total_domains,
    COUNT(DISTINCT fc.id) FILTER (WHERE fc.is_mandatory = true) AS mandatory_controls,
    COUNT(DISTINCT fc.id) FILTER (WHERE fc.is_mandatory = false) AS optional_controls,

    -- Control type distribution
    COUNT(DISTINCT fc.id) FILTER (WHERE fc.control_type = 'preventive') AS preventive_controls,
    COUNT(DISTINCT fc.id) FILTER (WHERE fc.control_type = 'detective') AS detective_controls,
    COUNT(DISTINCT fc.id) FILTER (WHERE fc.control_type = 'corrective') AS corrective_controls,

    -- Implementation type distribution
    COUNT(DISTINCT fc.id) FILTER (WHERE fc.implementation_type = 'technical') AS technical_controls,
    COUNT(DISTINCT fc.id) FILTER (WHERE fc.implementation_type = 'administrative') AS administrative_controls,
    COUNT(DISTINCT fc.id) FILTER (WHERE fc.implementation_type = 'physical') AS physical_controls,

    -- Cross-mapping stats
    (SELECT COUNT(DISTINCT fcm.target_control_id)
     FROM framework_control_mappings fcm
     JOIN framework_controls fc2 ON fc2.id = fcm.source_control_id
     WHERE fc2.framework_id = cf.id) AS outgoing_mappings,

    -- How many orgs have adopted this framework
    COUNT(DISTINCT ofw.organization_id) AS adoption_count

FROM compliance_frameworks cf
LEFT JOIN framework_domains fd ON fd.framework_id = cf.id
LEFT JOIN framework_controls fc ON fc.framework_id = cf.id
LEFT JOIN organization_frameworks ofw ON ofw.framework_id = cf.id
WHERE cf.deleted_at IS NULL
GROUP BY cf.id, cf.organization_id, cf.code, cf.name, cf.version,
         cf.issuing_body, cf.category, cf.applicable_regions,
         cf.is_system_framework, cf.is_active, cf.effective_date,
         cf.sunset_date, cf.color_hex;

COMMENT ON VIEW v_framework_summary IS 'Summary statistics for each framework: control counts, type distributions, mapping counts, and adoption figures.';

-- ============================================================================
-- VIEW: v_evidence_expiry_tracker
-- Evidence approaching or past expiry date.
-- Used by: compliance monitoring, evidence renewal reminders.
-- ============================================================================

CREATE OR REPLACE VIEW v_evidence_expiry_tracker AS
SELECT
    ce.organization_id,
    ce.id AS evidence_id,
    ce.title AS evidence_title,
    ce.evidence_type,
    ce.valid_from,
    ce.valid_until,
    ce.review_status,
    ci.id AS control_implementation_id,
    fc.code AS control_code,
    fc.title AS control_title,
    cf.code AS framework_code,
    cf.name AS framework_name,

    CASE
        WHEN ce.valid_until < CURRENT_DATE THEN 'expired'
        WHEN ce.valid_until < CURRENT_DATE + INTERVAL '30 days' THEN 'expiring_soon'
        WHEN ce.valid_until < CURRENT_DATE + INTERVAL '90 days' THEN 'expiring_notice'
        ELSE 'valid'
    END AS expiry_status,

    ce.valid_until - CURRENT_DATE AS days_until_expiry

FROM control_evidence ce
JOIN control_implementations ci ON ci.id = ce.control_implementation_id AND ci.deleted_at IS NULL
JOIN framework_controls fc ON fc.id = ci.framework_control_id
JOIN organization_frameworks ofw ON ofw.id = ci.org_framework_id
JOIN compliance_frameworks cf ON cf.id = ofw.framework_id AND cf.deleted_at IS NULL
WHERE ce.deleted_at IS NULL
  AND ce.is_current = true
  AND ce.valid_until IS NOT NULL
ORDER BY ce.valid_until ASC;

COMMENT ON VIEW v_evidence_expiry_tracker IS 'Tracks evidence approaching or past expiry. Used for continuous compliance monitoring and renewal reminders.';
