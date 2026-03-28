-- Seed Data: Default System Roles & Permissions
-- ComplianceForge GRC Platform
--
-- These are system-level defaults inserted with organization_id = NULL.
-- Every new organization automatically gets these roles available.
-- Org admins can create additional custom roles.

BEGIN;

-- ============================================================================
-- PERMISSIONS: resource × action matrix
-- ============================================================================
-- Resources: frameworks, controls, risks, policies, audits, incidents,
--            vendors, reports, settings, users, organizations, assets

INSERT INTO permissions (id, resource, action, description) VALUES
    -- Frameworks
    (gen_random_uuid(), 'frameworks', 'create',    'Create compliance frameworks'),
    (gen_random_uuid(), 'frameworks', 'read',      'View compliance frameworks'),
    (gen_random_uuid(), 'frameworks', 'update',    'Update compliance frameworks'),
    (gen_random_uuid(), 'frameworks', 'delete',    'Delete compliance frameworks'),
    (gen_random_uuid(), 'frameworks', 'export',    'Export framework data'),
    -- Controls
    (gen_random_uuid(), 'controls', 'create',      'Create controls'),
    (gen_random_uuid(), 'controls', 'read',        'View controls'),
    (gen_random_uuid(), 'controls', 'update',      'Update controls'),
    (gen_random_uuid(), 'controls', 'delete',      'Delete controls'),
    (gen_random_uuid(), 'controls', 'approve',     'Approve control implementation'),
    (gen_random_uuid(), 'controls', 'assign',      'Assign control owners'),
    (gen_random_uuid(), 'controls', 'export',      'Export control data'),
    -- Risks
    (gen_random_uuid(), 'risks', 'create',         'Create risk entries'),
    (gen_random_uuid(), 'risks', 'read',           'View risk entries'),
    (gen_random_uuid(), 'risks', 'update',         'Update risk entries'),
    (gen_random_uuid(), 'risks', 'delete',         'Delete risk entries'),
    (gen_random_uuid(), 'risks', 'approve',        'Approve risk treatment plans'),
    (gen_random_uuid(), 'risks', 'assign',         'Assign risk owners'),
    (gen_random_uuid(), 'risks', 'export',         'Export risk data'),
    -- Policies
    (gen_random_uuid(), 'policies', 'create',      'Create policies'),
    (gen_random_uuid(), 'policies', 'read',        'View policies'),
    (gen_random_uuid(), 'policies', 'update',      'Update policies'),
    (gen_random_uuid(), 'policies', 'delete',      'Delete policies'),
    (gen_random_uuid(), 'policies', 'approve',     'Approve policy changes'),
    (gen_random_uuid(), 'policies', 'assign',      'Assign policy owners'),
    (gen_random_uuid(), 'policies', 'export',      'Export policy documents'),
    -- Audits
    (gen_random_uuid(), 'audits', 'create',        'Create audits'),
    (gen_random_uuid(), 'audits', 'read',          'View audits'),
    (gen_random_uuid(), 'audits', 'update',        'Update audits'),
    (gen_random_uuid(), 'audits', 'delete',        'Delete audits'),
    (gen_random_uuid(), 'audits', 'approve',       'Approve audit findings'),
    (gen_random_uuid(), 'audits', 'assign',        'Assign auditors'),
    (gen_random_uuid(), 'audits', 'export',        'Export audit reports'),
    -- Incidents
    (gen_random_uuid(), 'incidents', 'create',     'Report incidents'),
    (gen_random_uuid(), 'incidents', 'read',       'View incidents'),
    (gen_random_uuid(), 'incidents', 'update',     'Update incidents'),
    (gen_random_uuid(), 'incidents', 'delete',     'Delete incidents'),
    (gen_random_uuid(), 'incidents', 'approve',    'Approve incident closure'),
    (gen_random_uuid(), 'incidents', 'assign',     'Assign incident handlers'),
    (gen_random_uuid(), 'incidents', 'export',     'Export incident data'),
    -- Vendors
    (gen_random_uuid(), 'vendors', 'create',       'Create vendor records'),
    (gen_random_uuid(), 'vendors', 'read',         'View vendor records'),
    (gen_random_uuid(), 'vendors', 'update',       'Update vendor records'),
    (gen_random_uuid(), 'vendors', 'delete',       'Delete vendor records'),
    (gen_random_uuid(), 'vendors', 'approve',      'Approve vendor assessments'),
    (gen_random_uuid(), 'vendors', 'export',       'Export vendor data'),
    -- Assets
    (gen_random_uuid(), 'assets', 'create',        'Create asset records'),
    (gen_random_uuid(), 'assets', 'read',          'View asset records'),
    (gen_random_uuid(), 'assets', 'update',        'Update asset records'),
    (gen_random_uuid(), 'assets', 'delete',        'Delete asset records'),
    (gen_random_uuid(), 'assets', 'export',        'Export asset data'),
    -- Reports
    (gen_random_uuid(), 'reports', 'read',         'View reports'),
    (gen_random_uuid(), 'reports', 'create',       'Generate reports'),
    (gen_random_uuid(), 'reports', 'export',       'Export reports'),
    -- Users
    (gen_random_uuid(), 'users', 'create',         'Create user accounts'),
    (gen_random_uuid(), 'users', 'read',           'View user profiles'),
    (gen_random_uuid(), 'users', 'update',         'Update user accounts'),
    (gen_random_uuid(), 'users', 'delete',         'Deactivate user accounts'),
    (gen_random_uuid(), 'users', 'assign',         'Assign roles to users'),
    -- Settings
    (gen_random_uuid(), 'settings', 'read',        'View organization settings'),
    (gen_random_uuid(), 'settings', 'configure',   'Configure organization settings'),
    -- Organizations
    (gen_random_uuid(), 'organizations', 'read',   'View organization details'),
    (gen_random_uuid(), 'organizations', 'update', 'Update organization details'),
    (gen_random_uuid(), 'organizations', 'configure', 'Configure organization');

-- ============================================================================
-- SYSTEM ROLES
-- ============================================================================
-- organization_id IS NULL = system-wide defaults available to all orgs

-- Org Admin: full access to everything within their org
INSERT INTO roles (id, organization_id, name, slug, description, is_system_role) VALUES
    ('10000000-0000-0000-0000-000000000001', NULL, 'Organization Admin', 'org_admin',
     'Full administrative access to all organization resources and settings.', true);

-- Compliance Manager: manages frameworks, controls, policies
INSERT INTO roles (id, organization_id, name, slug, description, is_system_role) VALUES
    ('10000000-0000-0000-0000-000000000002', NULL, 'Compliance Manager', 'compliance_manager',
     'Manages compliance frameworks, controls, and policies. Can approve control implementations.', true);

-- Risk Manager: manages risk register and assessments
INSERT INTO roles (id, organization_id, name, slug, description, is_system_role) VALUES
    ('10000000-0000-0000-0000-000000000003', NULL, 'Risk Manager', 'risk_manager',
     'Manages risk register, risk assessments, and risk treatment plans.', true);

-- Auditor: manages audits, findings, evidence
INSERT INTO roles (id, organization_id, name, slug, description, is_system_role) VALUES
    ('10000000-0000-0000-0000-000000000004', NULL, 'Auditor', 'auditor',
     'Plans and executes audits, creates findings, and manages evidence collection.', true);

-- Policy Owner: manages policy lifecycle
INSERT INTO roles (id, organization_id, name, slug, description, is_system_role) VALUES
    ('10000000-0000-0000-0000-000000000005', NULL, 'Policy Owner', 'policy_owner',
     'Creates and manages policies through the review/approval lifecycle.', true);

-- DPO (Data Protection Officer): GDPR-specific role
INSERT INTO roles (id, organization_id, name, slug, description, is_system_role) VALUES
    ('10000000-0000-0000-0000-000000000006', NULL, 'Data Protection Officer', 'dpo',
     'Oversees GDPR compliance, data breach notifications, and data processing activities.', true);

-- CISO: read access to everything, approve access to security-related items
INSERT INTO roles (id, organization_id, name, slug, description, is_system_role) VALUES
    ('10000000-0000-0000-0000-000000000007', NULL, 'CISO', 'ciso',
     'Chief Information Security Officer. Broad read access with approval rights on security matters.', true);

-- Viewer: read-only access
INSERT INTO roles (id, organization_id, name, slug, description, is_system_role) VALUES
    ('10000000-0000-0000-0000-000000000008', NULL, 'Viewer', 'viewer',
     'Read-only access to compliance data, reports, and dashboards.', true);

-- External Auditor: time-limited read access for third-party audit firms
INSERT INTO roles (id, organization_id, name, slug, description, is_system_role) VALUES
    ('10000000-0000-0000-0000-000000000009', NULL, 'External Auditor', 'external_auditor',
     'Limited read-only access for external audit firms. Typically time-limited via entity permissions.', true);

-- ============================================================================
-- ROLE → PERMISSION MAPPINGS
-- ============================================================================

-- Helper: map a role to all permissions for given resources
-- We'll do this explicitly for clarity and auditability.

-- Org Admin gets ALL permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT '10000000-0000-0000-0000-000000000001', id FROM permissions;

-- Compliance Manager: full access to frameworks, controls, policies; read on risks, audits, incidents, vendors, assets, reports
INSERT INTO role_permissions (role_id, permission_id)
SELECT '10000000-0000-0000-0000-000000000002', id FROM permissions
WHERE (resource IN ('frameworks', 'controls', 'policies'))
   OR (resource IN ('risks', 'audits', 'incidents', 'vendors', 'assets', 'reports', 'users', 'organizations') AND action = 'read')
   OR (resource = 'reports' AND action IN ('create', 'export'));

-- Risk Manager: full access to risks, read on frameworks/controls/policies/incidents/vendors/assets/reports
INSERT INTO role_permissions (role_id, permission_id)
SELECT '10000000-0000-0000-0000-000000000003', id FROM permissions
WHERE (resource = 'risks')
   OR (resource IN ('frameworks', 'controls', 'policies', 'incidents', 'vendors', 'assets', 'reports', 'users', 'organizations') AND action = 'read')
   OR (resource IN ('vendors') AND action IN ('update', 'approve'))
   OR (resource = 'reports' AND action IN ('create', 'export'));

-- Auditor: full access to audits, read on most things
INSERT INTO role_permissions (role_id, permission_id)
SELECT '10000000-0000-0000-0000-000000000004', id FROM permissions
WHERE (resource = 'audits')
   OR (resource IN ('frameworks', 'controls', 'risks', 'policies', 'incidents', 'vendors', 'assets', 'reports', 'users', 'organizations') AND action = 'read')
   OR (resource = 'reports' AND action IN ('create', 'export'));

-- Policy Owner: full access to policies, read on frameworks/controls
INSERT INTO role_permissions (role_id, permission_id)
SELECT '10000000-0000-0000-0000-000000000005', id FROM permissions
WHERE (resource = 'policies')
   OR (resource IN ('frameworks', 'controls', 'risks', 'reports', 'users', 'organizations') AND action = 'read')
   OR (resource = 'reports' AND action IN ('create', 'export'));

-- DPO: full access to incidents (data breaches), read on everything else, plus vendors/policies
INSERT INTO role_permissions (role_id, permission_id)
SELECT '10000000-0000-0000-0000-000000000006', id FROM permissions
WHERE (resource IN ('incidents'))
   OR (resource IN ('vendors', 'policies') AND action IN ('read', 'update', 'approve'))
   OR (resource IN ('frameworks', 'controls', 'risks', 'audits', 'assets', 'reports', 'users', 'organizations') AND action = 'read')
   OR (resource = 'reports' AND action IN ('create', 'export'));

-- CISO: read everything, approve on controls/risks/incidents, export reports
INSERT INTO role_permissions (role_id, permission_id)
SELECT '10000000-0000-0000-0000-000000000007', id FROM permissions
WHERE (action = 'read')
   OR (resource IN ('controls', 'risks', 'incidents') AND action = 'approve')
   OR (resource = 'reports' AND action IN ('create', 'export'))
   OR (resource = 'settings' AND action = 'configure');

-- Viewer: read-only on everything except settings/users/organizations management
INSERT INTO role_permissions (role_id, permission_id)
SELECT '10000000-0000-0000-0000-000000000008', id FROM permissions
WHERE action = 'read'
  AND resource NOT IN ('settings');

-- External Auditor: read on audits, frameworks, controls, policies, evidence-related
INSERT INTO role_permissions (role_id, permission_id)
SELECT '10000000-0000-0000-0000-000000000009', id FROM permissions
WHERE action = 'read'
  AND resource IN ('audits', 'frameworks', 'controls', 'policies', 'risks', 'reports');

COMMIT;
