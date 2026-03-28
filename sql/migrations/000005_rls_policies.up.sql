-- Migration 005: Row-Level Security (RLS) Policies
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - RLS is the primary tenant isolation mechanism. Even if application code has
--     a bug that omits a WHERE organization_id = ... clause, PostgreSQL will
--     enforce the boundary at the database level.
--   - The session variable app.current_tenant is set per-connection by the
--     application middleware before executing any query.
--   - We use FORCE ROW LEVEL SECURITY so that even table owners are subject to
--     RLS (defense in depth).
--   - The bypass_rls role is reserved for migration runners and admin scripts.
--   - Policies are named consistently: {table}_tenant_isolation_{operation}
--   - SELECT/UPDATE/DELETE use USING clause; INSERT uses WITH CHECK clause.
--   - Organizations table has a special policy: users can only see their own org.

-- ============================================================================
-- HELPER FUNCTIONS
-- ============================================================================

-- set_tenant: called by middleware to set the current org context for the connection.
CREATE OR REPLACE FUNCTION set_tenant(tenant_id UUID)
RETURNS VOID AS $$
BEGIN
    PERFORM set_config('app.current_tenant', tenant_id::TEXT, false);
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- get_current_tenant: retrieves the current tenant UUID from the session variable.
-- Returns NULL if not set (which means RLS will deny all access — safe default).
CREATE OR REPLACE FUNCTION get_current_tenant()
RETURNS UUID AS $$
BEGIN
    RETURN NULLIF(current_setting('app.current_tenant', true), '')::UUID;
EXCEPTION
    WHEN OTHERS THEN
        RETURN NULL;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;

COMMENT ON FUNCTION set_tenant(UUID) IS 'Sets the current tenant context. Called by app middleware on each request.';
COMMENT ON FUNCTION get_current_tenant() IS 'Returns the current tenant UUID. Returns NULL if unset (RLS denies all).';

-- ============================================================================
-- ENABLE RLS ON ALL TENANT-SCOPED TABLES
-- ============================================================================

-- organizations: users can only see their own org
ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizations FORCE ROW LEVEL SECURITY;

-- organization_subscriptions: scoped by organization_id
ALTER TABLE organization_subscriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE organization_subscriptions FORCE ROW LEVEL SECURITY;

-- users: scoped by organization_id
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;

-- user_sessions: scoped by organization_id
ALTER TABLE user_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_sessions FORCE ROW LEVEL SECURITY;

-- roles: system roles (org_id NULL) visible to all; custom roles scoped
ALTER TABLE roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE roles FORCE ROW LEVEL SECURITY;

-- user_roles: scoped by organization_id
ALTER TABLE user_roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_roles FORCE ROW LEVEL SECURITY;

-- user_entity_permissions: scoped by organization_id
ALTER TABLE user_entity_permissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_entity_permissions FORCE ROW LEVEL SECURITY;

-- ============================================================================
-- RLS POLICIES: organizations
-- ============================================================================

-- Users can only see their own organization.
CREATE POLICY organizations_tenant_isolation_select
    ON organizations FOR SELECT
    USING (id = get_current_tenant());

CREATE POLICY organizations_tenant_isolation_update
    ON organizations FOR UPDATE
    USING (id = get_current_tenant())
    WITH CHECK (id = get_current_tenant());

-- Only super_admin can INSERT or DELETE orgs (handled at app level);
-- RLS prevents cross-tenant manipulation regardless.
CREATE POLICY organizations_tenant_isolation_insert
    ON organizations FOR INSERT
    WITH CHECK (id = get_current_tenant());

CREATE POLICY organizations_tenant_isolation_delete
    ON organizations FOR DELETE
    USING (id = get_current_tenant());

-- ============================================================================
-- RLS POLICIES: organization_subscriptions
-- ============================================================================

CREATE POLICY org_subscriptions_tenant_isolation_select
    ON organization_subscriptions FOR SELECT
    USING (organization_id = get_current_tenant());

CREATE POLICY org_subscriptions_tenant_isolation_insert
    ON organization_subscriptions FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY org_subscriptions_tenant_isolation_update
    ON organization_subscriptions FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY org_subscriptions_tenant_isolation_delete
    ON organization_subscriptions FOR DELETE
    USING (organization_id = get_current_tenant());

-- ============================================================================
-- RLS POLICIES: users
-- ============================================================================

CREATE POLICY users_tenant_isolation_select
    ON users FOR SELECT
    USING (organization_id = get_current_tenant());

CREATE POLICY users_tenant_isolation_insert
    ON users FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY users_tenant_isolation_update
    ON users FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY users_tenant_isolation_delete
    ON users FOR DELETE
    USING (organization_id = get_current_tenant());

-- ============================================================================
-- RLS POLICIES: user_sessions
-- ============================================================================

CREATE POLICY sessions_tenant_isolation_select
    ON user_sessions FOR SELECT
    USING (organization_id = get_current_tenant());

CREATE POLICY sessions_tenant_isolation_insert
    ON user_sessions FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY sessions_tenant_isolation_update
    ON user_sessions FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY sessions_tenant_isolation_delete
    ON user_sessions FOR DELETE
    USING (organization_id = get_current_tenant());

-- ============================================================================
-- RLS POLICIES: roles
-- ============================================================================

-- System roles (organization_id IS NULL) are visible to all tenants.
-- Custom roles are only visible to the owning org.
CREATE POLICY roles_tenant_isolation_select
    ON roles FOR SELECT
    USING (
        organization_id IS NULL  -- system roles visible to all
        OR organization_id = get_current_tenant()
    );

CREATE POLICY roles_tenant_isolation_insert
    ON roles FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY roles_tenant_isolation_update
    ON roles FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY roles_tenant_isolation_delete
    ON roles FOR DELETE
    USING (organization_id = get_current_tenant());

-- ============================================================================
-- RLS POLICIES: user_roles
-- ============================================================================

CREATE POLICY user_roles_tenant_isolation_select
    ON user_roles FOR SELECT
    USING (organization_id = get_current_tenant());

CREATE POLICY user_roles_tenant_isolation_insert
    ON user_roles FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY user_roles_tenant_isolation_update
    ON user_roles FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY user_roles_tenant_isolation_delete
    ON user_roles FOR DELETE
    USING (organization_id = get_current_tenant());

-- ============================================================================
-- RLS POLICIES: user_entity_permissions
-- ============================================================================

CREATE POLICY entity_perms_tenant_isolation_select
    ON user_entity_permissions FOR SELECT
    USING (organization_id = get_current_tenant());

CREATE POLICY entity_perms_tenant_isolation_insert
    ON user_entity_permissions FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY entity_perms_tenant_isolation_update
    ON user_entity_permissions FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY entity_perms_tenant_isolation_delete
    ON user_entity_permissions FOR DELETE
    USING (organization_id = get_current_tenant());
