-- Migration 015: Policy Analytics Views
-- ComplianceForge GRC Platform

-- ============================================================================
-- VIEW: v_policy_compliance_status
-- Policy attestation rates, review status, and overdue counts per org.
-- ============================================================================

CREATE OR REPLACE VIEW v_policy_compliance_status AS
SELECT
    p.organization_id,
    p.id AS policy_id,
    p.policy_ref,
    p.title,
    p.status,
    p.classification,
    pc.name AS category_name,
    p.owner_user_id,
    ou.first_name || ' ' || ou.last_name AS owner_name,
    p.current_version,
    p.effective_date,
    p.review_frequency_months,
    p.last_review_date,
    p.next_review_date,
    p.review_status,
    p.requires_attestation,
    p.is_mandatory,

    -- Attestation statistics (for current version)
    (SELECT COUNT(*) FROM policy_attestations pa
     WHERE pa.policy_id = p.id AND pa.policy_version_id = p.current_version_id
    ) AS total_attestation_requests,

    (SELECT COUNT(*) FROM policy_attestations pa
     WHERE pa.policy_id = p.id AND pa.policy_version_id = p.current_version_id
       AND pa.status = 'attested'
    ) AS attested_count,

    (SELECT COUNT(*) FROM policy_attestations pa
     WHERE pa.policy_id = p.id AND pa.policy_version_id = p.current_version_id
       AND pa.status = 'pending'
    ) AS pending_count,

    (SELECT COUNT(*) FROM policy_attestations pa
     WHERE pa.policy_id = p.id AND pa.policy_version_id = p.current_version_id
       AND pa.status = 'overdue'
    ) AS overdue_count,

    (SELECT COUNT(*) FROM policy_attestations pa
     WHERE pa.policy_id = p.id AND pa.policy_version_id = p.current_version_id
       AND pa.status = 'declined'
    ) AS declined_count,

    -- Attestation completion rate
    CASE
        WHEN (SELECT COUNT(*) FROM policy_attestations pa
              WHERE pa.policy_id = p.id AND pa.policy_version_id = p.current_version_id) = 0 THEN 0
        ELSE ROUND(
            (SELECT COUNT(*) FROM policy_attestations pa
             WHERE pa.policy_id = p.id AND pa.policy_version_id = p.current_version_id
               AND pa.status = 'attested')::DECIMAL
            /
            (SELECT COUNT(*) FROM policy_attestations pa
             WHERE pa.policy_id = p.id AND pa.policy_version_id = p.current_version_id)::DECIMAL
            * 100, 2
        )
    END AS attestation_rate,

    -- Review overdue?
    CASE
        WHEN p.next_review_date IS NOT NULL AND p.next_review_date < CURRENT_DATE THEN true
        ELSE false
    END AS is_review_overdue,

    -- Days until next review
    CASE
        WHEN p.next_review_date IS NOT NULL THEN p.next_review_date - CURRENT_DATE
        ELSE NULL
    END AS days_until_review,

    -- Active exceptions count
    (SELECT COUNT(*) FROM policy_exceptions pe
     WHERE pe.policy_id = p.id AND pe.status = 'approved'
       AND (pe.expiry_date IS NULL OR pe.expiry_date >= CURRENT_DATE)
    ) AS active_exceptions,

    -- Version count
    (SELECT COUNT(*) FROM policy_versions pv WHERE pv.policy_id = p.id) AS total_versions,

    -- Control mapping count
    (SELECT COUNT(*) FROM policy_control_mappings pcm WHERE pcm.policy_id = p.id) AS mapped_controls

FROM policies p
LEFT JOIN policy_categories pc ON pc.id = p.category_id
LEFT JOIN users ou ON ou.id = p.owner_user_id
WHERE p.deleted_at IS NULL;

COMMENT ON VIEW v_policy_compliance_status IS 'Comprehensive policy status dashboard: attestation rates, review deadlines, exception counts, and control mappings.';

-- ============================================================================
-- VIEW: v_policy_gap_analysis
-- Framework controls without linked policies.
-- ============================================================================

CREATE OR REPLACE VIEW v_policy_gap_analysis AS
SELECT
    ci.organization_id,
    cf.code AS framework_code,
    cf.name AS framework_name,
    fd.code AS domain_code,
    fd.name AS domain_name,
    fc.id AS control_id,
    fc.code AS control_code,
    fc.title AS control_title,
    fc.control_type,
    fc.priority AS control_priority,
    ci.id AS implementation_id,
    ci.status AS implementation_status,
    ci.maturity_level,

    -- Does this control have a linked policy?
    CASE
        WHEN EXISTS (
            SELECT 1 FROM policy_control_mappings pcm
            JOIN policies p ON p.id = pcm.policy_id AND p.deleted_at IS NULL
              AND p.status IN ('published', 'approved')
            WHERE pcm.framework_control_id = fc.id
              AND pcm.organization_id = ci.organization_id
        ) THEN true
        ELSE false
    END AS has_policy,

    -- Policy details if mapped
    (SELECT json_agg(json_build_object(
        'policy_id', p.id,
        'policy_ref', p.policy_ref,
        'title', p.title,
        'status', p.status,
        'coverage', pcm.coverage
    ))
    FROM policy_control_mappings pcm
    JOIN policies p ON p.id = pcm.policy_id AND p.deleted_at IS NULL
    WHERE pcm.framework_control_id = fc.id
      AND pcm.organization_id = ci.organization_id
    ) AS linked_policies

FROM control_implementations ci
JOIN organization_frameworks ofw ON ofw.id = ci.org_framework_id
JOIN compliance_frameworks cf ON cf.id = ofw.framework_id AND cf.deleted_at IS NULL
JOIN framework_controls fc ON fc.id = ci.framework_control_id
LEFT JOIN framework_domains fd ON fd.id = fc.domain_id
WHERE ci.deleted_at IS NULL;

COMMENT ON VIEW v_policy_gap_analysis IS 'Shows which controls have (or lack) supporting policies. Key for ISO 27001 A.5.1 and regulatory compliance.';

-- ============================================================================
-- VIEW: v_policy_review_calendar
-- Upcoming and overdue policy reviews.
-- ============================================================================

CREATE OR REPLACE VIEW v_policy_review_calendar AS
SELECT
    p.organization_id,
    p.id AS policy_id,
    p.policy_ref,
    p.title,
    p.status AS policy_status,
    pc.name AS category_name,
    p.owner_user_id,
    ou.first_name || ' ' || ou.last_name AS owner_name,
    p.last_review_date,
    p.next_review_date,
    p.review_frequency_months,
    p.review_status,

    -- Status categorization
    CASE
        WHEN p.next_review_date IS NULL THEN 'no_review_scheduled'
        WHEN p.next_review_date < CURRENT_DATE THEN 'overdue'
        WHEN p.next_review_date < CURRENT_DATE + INTERVAL '30 days' THEN 'due_soon'
        WHEN p.next_review_date < CURRENT_DATE + INTERVAL '90 days' THEN 'upcoming'
        ELSE 'on_track'
    END AS review_urgency,

    -- Days until/since review
    CASE
        WHEN p.next_review_date IS NOT NULL THEN p.next_review_date - CURRENT_DATE
        ELSE NULL
    END AS days_until_review,

    -- Active review record if any
    (SELECT pr.status FROM policy_reviews pr
     WHERE pr.policy_id = p.id AND pr.status IN ('scheduled', 'in_progress')
     ORDER BY pr.due_date ASC LIMIT 1
    ) AS active_review_status,

    -- Days since last review
    CASE
        WHEN p.last_review_date IS NOT NULL THEN CURRENT_DATE - p.last_review_date
        ELSE NULL
    END AS days_since_last_review

FROM policies p
LEFT JOIN policy_categories pc ON pc.id = p.category_id
LEFT JOIN users ou ON ou.id = p.owner_user_id
WHERE p.deleted_at IS NULL
  AND p.status IN ('published', 'approved')
ORDER BY p.next_review_date ASC NULLS LAST;

COMMENT ON VIEW v_policy_review_calendar IS 'Policy review calendar showing upcoming, due, and overdue reviews. Used for compliance monitoring dashboards.';

-- ============================================================================
-- VIEW: v_attestation_campaign_progress
-- Campaign completion metrics.
-- ============================================================================

CREATE OR REPLACE VIEW v_attestation_campaign_progress AS
SELECT
    pac.organization_id,
    pac.id AS campaign_id,
    pac.name AS campaign_name,
    pac.status AS campaign_status,
    pac.start_date,
    pac.due_date,
    pac.total_recipients,
    pac.attested_count,
    pac.completion_rate,
    pac.auto_remind,
    pac.reminder_frequency_days,
    pac.escalation_after_days,

    -- Calculated counts from actual attestations
    (SELECT COUNT(*) FROM policy_attestations pa
     WHERE pa.campaign_id = pac.id AND pa.status = 'attested') AS actual_attested,
    (SELECT COUNT(*) FROM policy_attestations pa
     WHERE pa.campaign_id = pac.id AND pa.status = 'pending') AS actual_pending,
    (SELECT COUNT(*) FROM policy_attestations pa
     WHERE pa.campaign_id = pac.id AND pa.status = 'overdue') AS actual_overdue,
    (SELECT COUNT(*) FROM policy_attestations pa
     WHERE pa.campaign_id = pac.id AND pa.status = 'declined') AS actual_declined,
    (SELECT COUNT(*) FROM policy_attestations pa
     WHERE pa.campaign_id = pac.id) AS actual_total,

    -- Calculated completion rate
    CASE
        WHEN (SELECT COUNT(*) FROM policy_attestations pa WHERE pa.campaign_id = pac.id) = 0 THEN 0
        ELSE ROUND(
            (SELECT COUNT(*) FROM policy_attestations pa
             WHERE pa.campaign_id = pac.id AND pa.status = 'attested')::DECIMAL
            /
            (SELECT COUNT(*) FROM policy_attestations pa WHERE pa.campaign_id = pac.id)::DECIMAL
            * 100, 2
        )
    END AS calculated_completion_rate,

    -- Is campaign overdue?
    CASE
        WHEN pac.status = 'active' AND pac.due_date < CURRENT_DATE THEN true
        ELSE false
    END AS is_overdue,

    -- Days remaining
    CASE
        WHEN pac.due_date IS NOT NULL THEN pac.due_date - CURRENT_DATE
        ELSE NULL
    END AS days_remaining,

    -- Escalation needed?
    CASE
        WHEN pac.status = 'active'
         AND pac.start_date IS NOT NULL
         AND (CURRENT_DATE - pac.start_date) > pac.escalation_after_days
         AND (SELECT COUNT(*) FROM policy_attestations pa
              WHERE pa.campaign_id = pac.id AND pa.status = 'pending') > 0
        THEN true
        ELSE false
    END AS needs_escalation,

    -- Number of policies in campaign
    COALESCE(array_length(pac.policy_ids, 1), 0) AS policy_count,

    pac.created_by,
    cu.first_name || ' ' || cu.last_name AS created_by_name

FROM policy_attestation_campaigns pac
LEFT JOIN users cu ON cu.id = pac.created_by;

COMMENT ON VIEW v_attestation_campaign_progress IS 'Campaign completion metrics with overdue and escalation detection. Drives the attestation dashboard.';
