-- Seed Data: Default ABAC Policies
-- ComplianceForge GRC Platform
--
-- These are organization-scoped seed policies. They must be inserted per-org
-- (unlike system-level seeds). The application's onboarding flow should clone
-- these templates into each new organization.
--
-- For seeding, we use a placeholder org UUID that the application replaces.
-- Fixed policy UUIDs for referenceability in tests and documentation.
--
-- Policy evaluation order: lower priority number = evaluated first.
-- Deny policies should have lower priority numbers than allow policies
-- to ensure "deny overrides" behavior.

BEGIN;

-- ============================================================================
-- HELPER: Template organization ID placeholder
-- Replace '00000000-0000-0000-0000-000000000000' with the actual org ID
-- when cloning these policies during onboarding.
-- ============================================================================

-- ============================================================================
-- POLICY a) Org Admin Full Access
-- Org admins have unrestricted access to all resources and actions.
-- ============================================================================

INSERT INTO access_policies (id, organization_id, name, description, priority, effect, is_active, subject_conditions, resource_type, resource_conditions, actions, environment_conditions)
VALUES (
    'c0000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000000',
    'Org Admin Full Access',
    'Grants organization administrators unrestricted access to all resources and actions. This is the highest-privilege policy and should only be assigned to trusted administrators.',
    10,
    'allow',
    true,
    '{"roles": ["org_admin"]}',
    '*',
    NULL,
    ARRAY['create', 'read', 'update', 'delete', 'approve', 'assign', 'export', 'configure'],
    NULL
);

INSERT INTO access_policy_assignments (id, organization_id, access_policy_id, assignee_type, assignee_id)
VALUES (
    'c1000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000000',
    'c0000000-0000-0000-0000-000000000001',
    'role',
    NULL
);

-- ============================================================================
-- POLICY b) Control Owner — Own Controls
-- Control owners can read and update control implementations they own.
-- ============================================================================

INSERT INTO access_policies (id, organization_id, name, description, priority, effect, is_active, subject_conditions, resource_type, resource_conditions, actions, environment_conditions)
VALUES (
    'c0000000-0000-0000-0000-000000000002',
    '00000000-0000-0000-0000-000000000000',
    'Control Owner — Own Controls',
    'Allows control owners to view and update control implementations where they are the assigned owner. Enforces ownership-based access so control owners cannot modify controls outside their scope.',
    50,
    'allow',
    true,
    '{"roles": ["compliance_manager", "policy_owner"]}',
    'control_implementation',
    '{"owner_id": "$subject.id"}',
    ARRAY['read', 'update'],
    NULL
);

INSERT INTO access_policy_assignments (id, organization_id, access_policy_id, assignee_type, assignee_id)
VALUES (
    'c1000000-0000-0000-0000-000000000002',
    '00000000-0000-0000-0000-000000000000',
    'c0000000-0000-0000-0000-000000000002',
    'role',
    NULL
);

-- ============================================================================
-- POLICY c) DPO — Privacy Incidents (Data Breaches)
-- DPO can read, update, and approve incidents flagged as data breaches.
-- ============================================================================

INSERT INTO access_policies (id, organization_id, name, description, priority, effect, is_active, subject_conditions, resource_type, resource_conditions, actions, environment_conditions)
VALUES (
    'c0000000-0000-0000-0000-000000000003',
    '00000000-0000-0000-0000-000000000000',
    'DPO — Privacy Incidents',
    'Grants the Data Protection Officer access to incidents classified as data breaches. Required for GDPR Article 33/34 breach notification workflow. DPO can view, update, and approve incident responses.',
    40,
    'allow',
    true,
    '{"roles": ["dpo"]}',
    'incident',
    '{"is_data_breach": true}',
    ARRAY['read', 'update', 'approve'],
    NULL
);

INSERT INTO access_policy_assignments (id, organization_id, access_policy_id, assignee_type, assignee_id)
VALUES (
    'c1000000-0000-0000-0000-000000000003',
    '00000000-0000-0000-0000-000000000000',
    'c0000000-0000-0000-0000-000000000003',
    'role',
    NULL
);

-- ============================================================================
-- POLICY d) DPO — All Data Subject Requests
-- DPO has full access to all DSR requests for GDPR compliance.
-- ============================================================================

INSERT INTO access_policies (id, organization_id, name, description, priority, effect, is_active, subject_conditions, resource_type, resource_conditions, actions, environment_conditions)
VALUES (
    'c0000000-0000-0000-0000-000000000004',
    '00000000-0000-0000-0000-000000000000',
    'DPO — All Data Subject Requests',
    'Grants the Data Protection Officer full access to all data subject requests (DSRs). Required for GDPR Articles 15-22 compliance. DPO manages the entire DSR lifecycle.',
    40,
    'allow',
    true,
    '{"roles": ["dpo"]}',
    'dsr_request',
    NULL,
    ARRAY['create', 'read', 'update', 'delete', 'approve', 'assign', 'export'],
    NULL
);

INSERT INTO access_policy_assignments (id, organization_id, access_policy_id, assignee_type, assignee_id)
VALUES (
    'c1000000-0000-0000-0000-000000000004',
    '00000000-0000-0000-0000-000000000000',
    'c0000000-0000-0000-0000-000000000004',
    'role',
    NULL
);

-- ============================================================================
-- POLICY e) External Auditor — Read-Only (MFA Required)
-- External auditors can only read audit-related resources, and must have MFA.
-- ============================================================================

INSERT INTO access_policies (id, organization_id, name, description, priority, effect, is_active, subject_conditions, resource_type, resource_conditions, actions, environment_conditions)
VALUES (
    'c0000000-0000-0000-0000-000000000005',
    '00000000-0000-0000-0000-000000000000',
    'External Auditor — Read-Only (MFA Required)',
    'Grants external auditors read-only access to audit workspaces, findings, control implementations, and control evidence. Requires MFA verification for every session. Supports ISO 27001 and SOC 2 audit cycles with time-limited access.',
    60,
    'allow',
    true,
    '{"roles": ["external_auditor"]}',
    'audit',
    NULL,
    ARRAY['read'],
    '{"mfa_verified": true}'
);

-- Additional resource types for external auditor (findings, controls, evidence)
INSERT INTO access_policies (id, organization_id, name, description, priority, effect, is_active, subject_conditions, resource_type, resource_conditions, actions, environment_conditions)
VALUES (
    'c0000000-0000-0000-0000-000000000015',
    '00000000-0000-0000-0000-000000000000',
    'External Auditor — Findings Read-Only (MFA Required)',
    'Extends external auditor read access to findings.',
    60,
    'allow',
    true,
    '{"roles": ["external_auditor"]}',
    'finding',
    NULL,
    ARRAY['read'],
    '{"mfa_verified": true}'
),
(
    'c0000000-0000-0000-0000-000000000025',
    '00000000-0000-0000-0000-000000000000',
    'External Auditor — Control Implementations Read-Only (MFA Required)',
    'Extends external auditor read access to control implementations.',
    60,
    'allow',
    true,
    '{"roles": ["external_auditor"]}',
    'control_implementation',
    NULL,
    ARRAY['read'],
    '{"mfa_verified": true}'
),
(
    'c0000000-0000-0000-0000-000000000035',
    '00000000-0000-0000-0000-000000000000',
    'External Auditor — Control Evidence Read-Only (MFA Required)',
    'Extends external auditor read access to control evidence.',
    60,
    'allow',
    true,
    '{"roles": ["external_auditor"]}',
    'control_evidence',
    NULL,
    ARRAY['read'],
    '{"mfa_verified": true}'
);

INSERT INTO access_policy_assignments (id, organization_id, access_policy_id, assignee_type, assignee_id)
VALUES
    ('c1000000-0000-0000-0000-000000000005', '00000000-0000-0000-0000-000000000000', 'c0000000-0000-0000-0000-000000000005', 'role', NULL),
    ('c1000000-0000-0000-0000-000000000015', '00000000-0000-0000-0000-000000000000', 'c0000000-0000-0000-0000-000000000015', 'role', NULL),
    ('c1000000-0000-0000-0000-000000000025', '00000000-0000-0000-0000-000000000000', 'c0000000-0000-0000-0000-000000000025', 'role', NULL),
    ('c1000000-0000-0000-0000-000000000035', '00000000-0000-0000-0000-000000000000', 'c0000000-0000-0000-0000-000000000035', 'role', NULL);

-- ============================================================================
-- POLICY f) Viewer — Read All Non-Confidential Resources
-- Viewers can read any resource that is not classified as confidential or restricted.
-- ============================================================================

INSERT INTO access_policies (id, organization_id, name, description, priority, effect, is_active, subject_conditions, resource_type, resource_conditions, actions, environment_conditions)
VALUES (
    'c0000000-0000-0000-0000-000000000006',
    '00000000-0000-0000-0000-000000000000',
    'Viewer — Read Non-Confidential',
    'Grants viewers read access to all resources except those classified as confidential or restricted. Ensures information segregation while allowing broad visibility for oversight and reporting roles.',
    70,
    'allow',
    true,
    '{"roles": ["viewer"]}',
    '*',
    '{"classification": {"$nin": ["confidential", "restricted"]}}',
    ARRAY['read'],
    NULL
);

INSERT INTO access_policy_assignments (id, organization_id, access_policy_id, assignee_type, assignee_id)
VALUES (
    'c1000000-0000-0000-0000-000000000006',
    '00000000-0000-0000-0000-000000000000',
    'c0000000-0000-0000-0000-000000000006',
    'role',
    NULL
);

-- ============================================================================
-- POLICY g) Export Restriction — After Hours (Deny)
-- Deny export of reports outside business hours (09:00-18:00).
-- Deny policies have lower priority numbers to override allow policies.
-- ============================================================================

INSERT INTO access_policies (id, organization_id, name, description, priority, effect, is_active, subject_conditions, resource_type, resource_conditions, actions, environment_conditions)
VALUES (
    'c0000000-0000-0000-0000-000000000007',
    '00000000-0000-0000-0000-000000000000',
    'Export Restriction — After Hours',
    'Denies report export operations outside business hours (09:00-18:00 local time). Prevents bulk data exfiltration during off-hours when security monitoring may be reduced. Applies to all users regardless of role.',
    5,
    'deny',
    true,
    '{}',
    'report',
    NULL,
    ARRAY['export'],
    '{"time_range": {"not_between": {"start": "09:00", "end": "18:00"}}}'
);

INSERT INTO access_policy_assignments (id, organization_id, access_policy_id, assignee_type, assignee_id)
VALUES (
    'c1000000-0000-0000-0000-000000000007',
    '00000000-0000-0000-0000-000000000000',
    'c0000000-0000-0000-0000-000000000007',
    'all_users',
    NULL
);

-- ============================================================================
-- POLICY h) Field Mask — Financial Impact for Non-Managers
-- Masks the financial_impact_eur field on risks for users who are not
-- org_admin, risk_manager, or ciso. Uses field_level_permissions.
-- ============================================================================

INSERT INTO access_policies (id, organization_id, name, description, priority, effect, is_active, subject_conditions, resource_type, resource_conditions, actions, environment_conditions)
VALUES (
    'c0000000-0000-0000-0000-000000000008',
    '00000000-0000-0000-0000-000000000000',
    'Field Mask — Financial Impact for Non-Managers',
    'Masks the financial_impact_eur field on risk records for users who are not org admins, risk managers, or CISOs. Ensures sensitive financial data is only visible to authorized management roles. Non-authorized users see masked values (e.g., "EUR ***.**").',
    80,
    'allow',
    true,
    '{"roles": {"$nin": ["org_admin", "risk_manager", "ciso"]}}',
    'risk',
    NULL,
    ARRAY['read'],
    NULL
);

INSERT INTO field_level_permissions (id, organization_id, access_policy_id, resource_type, field_name, permission, mask_pattern)
VALUES (
    'c2000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000000',
    'c0000000-0000-0000-0000-000000000008',
    'risk',
    'financial_impact_eur',
    'masked',
    'EUR ***.**'
);

INSERT INTO access_policy_assignments (id, organization_id, access_policy_id, assignee_type, assignee_id)
VALUES (
    'c1000000-0000-0000-0000-000000000008',
    '00000000-0000-0000-0000-000000000000',
    'c0000000-0000-0000-0000-000000000008',
    'all_users',
    NULL
);

COMMIT;
