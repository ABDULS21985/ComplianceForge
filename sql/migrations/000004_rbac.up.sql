-- Migration 004: Role-Based Access Control (RBAC)
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - Roles can be system-default (organization_id IS NULL, is_system_role = true)
--     or custom per-org (organization_id IS NOT NULL, is_custom = true)
--   - Permissions are a flat resource × action matrix — simple, auditable, and
--     sufficient for GRC where resources map cleanly to compliance domains
--   - user_roles is a many-to-many with org context, enabling users to hold
--     different roles in different orgs (for consultants/auditors)
--   - user_entity_permissions provides object-level granularity — e.g., a
--     policy_owner who can only approve policies in their department's scope
--   - expires_at on entity permissions supports time-limited access for
--     external auditors (common in ISO 27001 certification cycles)

-- ============================================================================
-- TABLE: roles
-- ============================================================================

CREATE TABLE roles (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE,
    name            VARCHAR(100) NOT NULL,
    slug            VARCHAR(100) NOT NULL,
    description     TEXT,
    is_system_role  BOOLEAN NOT NULL DEFAULT false,
    is_custom       BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,

    -- System roles have globally unique slugs; custom roles are unique per org.
    CONSTRAINT uq_roles_org_slug UNIQUE NULLS NOT DISTINCT (organization_id, slug)
);

-- Indexes
CREATE INDEX idx_roles_organization ON roles(organization_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_roles_system ON roles(is_system_role) WHERE is_system_role = true AND deleted_at IS NULL;
CREATE INDEX idx_roles_deleted_at ON roles(deleted_at) WHERE deleted_at IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_roles_updated_at
    BEFORE UPDATE ON roles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE roles IS 'RBAC roles. System roles (org_id NULL) are seeded defaults; custom roles are org-specific.';
COMMENT ON COLUMN roles.is_system_role IS 'System roles are created during seeding and cannot be deleted by org admins.';

-- ============================================================================
-- TABLE: permissions
-- ============================================================================

CREATE TABLE permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource    VARCHAR(100) NOT NULL,
    action      permission_action NOT NULL,
    description TEXT,

    CONSTRAINT uq_permissions_resource_action UNIQUE (resource, action)
);

COMMENT ON TABLE permissions IS 'Flat resource × action permission matrix. Resources map to GRC domains (frameworks, controls, risks, etc.).';

-- ============================================================================
-- TABLE: role_permissions (many-to-many join)
-- ============================================================================

CREATE TABLE role_permissions (
    role_id       UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (role_id, permission_id)
);

CREATE INDEX idx_role_permissions_permission ON role_permissions(permission_id);

COMMENT ON TABLE role_permissions IS 'Maps roles to permissions. Deleting a role cascades to remove its permission mappings.';

-- ============================================================================
-- TABLE: user_roles (many-to-many with org context)
-- ============================================================================

CREATE TABLE user_roles (
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id         UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    assigned_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    assigned_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (user_id, role_id, organization_id)
);

CREATE INDEX idx_user_roles_user ON user_roles(user_id);
CREATE INDEX idx_user_roles_role ON user_roles(role_id);
CREATE INDEX idx_user_roles_org ON user_roles(organization_id);

COMMENT ON TABLE user_roles IS 'Assigns roles to users within an org context. A user can hold different roles in different orgs.';

-- ============================================================================
-- TABLE: user_entity_permissions (object-level granular access)
-- ============================================================================

CREATE TABLE user_entity_permissions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id  UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entity_type      VARCHAR(50) NOT NULL,
    entity_id        UUID NOT NULL,
    permission_level VARCHAR(20) NOT NULL
                     CHECK (permission_level IN ('viewer', 'editor', 'approver', 'owner')),
    granted_by       UUID REFERENCES users(id) ON DELETE SET NULL,
    expires_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Prevent duplicate grants for the same user + entity + level.
    CONSTRAINT uq_user_entity_perm UNIQUE (user_id, entity_type, entity_id, permission_level)
);

CREATE INDEX idx_entity_perms_user ON user_entity_permissions(user_id);
CREATE INDEX idx_entity_perms_org ON user_entity_permissions(organization_id);
CREATE INDEX idx_entity_perms_entity ON user_entity_permissions(entity_type, entity_id);
CREATE INDEX idx_entity_perms_expires ON user_entity_permissions(expires_at)
    WHERE expires_at IS NOT NULL;

COMMENT ON TABLE user_entity_permissions IS 'Granular per-object permissions. Supports time-limited access for external auditors.';
COMMENT ON COLUMN user_entity_permissions.entity_type IS 'Polymorphic type: framework, policy, audit, risk_register, vendor, etc.';
COMMENT ON COLUMN user_entity_permissions.expires_at IS 'Optional expiry for time-limited access (e.g., external auditor access during ISO 27001 certification).';
