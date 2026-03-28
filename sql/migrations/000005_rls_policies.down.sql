-- Rollback Migration 005: Disable RLS and drop policies and helper functions.

-- Drop all RLS policies
DROP POLICY IF EXISTS organizations_tenant_isolation_select ON organizations;
DROP POLICY IF EXISTS organizations_tenant_isolation_update ON organizations;
DROP POLICY IF EXISTS organizations_tenant_isolation_insert ON organizations;
DROP POLICY IF EXISTS organizations_tenant_isolation_delete ON organizations;

DROP POLICY IF EXISTS org_subscriptions_tenant_isolation_select ON organization_subscriptions;
DROP POLICY IF EXISTS org_subscriptions_tenant_isolation_insert ON organization_subscriptions;
DROP POLICY IF EXISTS org_subscriptions_tenant_isolation_update ON organization_subscriptions;
DROP POLICY IF EXISTS org_subscriptions_tenant_isolation_delete ON organization_subscriptions;

DROP POLICY IF EXISTS users_tenant_isolation_select ON users;
DROP POLICY IF EXISTS users_tenant_isolation_insert ON users;
DROP POLICY IF EXISTS users_tenant_isolation_update ON users;
DROP POLICY IF EXISTS users_tenant_isolation_delete ON users;

DROP POLICY IF EXISTS sessions_tenant_isolation_select ON user_sessions;
DROP POLICY IF EXISTS sessions_tenant_isolation_insert ON user_sessions;
DROP POLICY IF EXISTS sessions_tenant_isolation_update ON user_sessions;
DROP POLICY IF EXISTS sessions_tenant_isolation_delete ON user_sessions;

DROP POLICY IF EXISTS roles_tenant_isolation_select ON roles;
DROP POLICY IF EXISTS roles_tenant_isolation_insert ON roles;
DROP POLICY IF EXISTS roles_tenant_isolation_update ON roles;
DROP POLICY IF EXISTS roles_tenant_isolation_delete ON roles;

DROP POLICY IF EXISTS user_roles_tenant_isolation_select ON user_roles;
DROP POLICY IF EXISTS user_roles_tenant_isolation_insert ON user_roles;
DROP POLICY IF EXISTS user_roles_tenant_isolation_update ON user_roles;
DROP POLICY IF EXISTS user_roles_tenant_isolation_delete ON user_roles;

DROP POLICY IF EXISTS entity_perms_tenant_isolation_select ON user_entity_permissions;
DROP POLICY IF EXISTS entity_perms_tenant_isolation_insert ON user_entity_permissions;
DROP POLICY IF EXISTS entity_perms_tenant_isolation_update ON user_entity_permissions;
DROP POLICY IF EXISTS entity_perms_tenant_isolation_delete ON user_entity_permissions;

-- Disable RLS
ALTER TABLE organizations DISABLE ROW LEVEL SECURITY;
ALTER TABLE organization_subscriptions DISABLE ROW LEVEL SECURITY;
ALTER TABLE users DISABLE ROW LEVEL SECURITY;
ALTER TABLE user_sessions DISABLE ROW LEVEL SECURITY;
ALTER TABLE roles DISABLE ROW LEVEL SECURITY;
ALTER TABLE user_roles DISABLE ROW LEVEL SECURITY;
ALTER TABLE user_entity_permissions DISABLE ROW LEVEL SECURITY;

-- Drop helper functions
DROP FUNCTION IF EXISTS get_current_tenant();
DROP FUNCTION IF EXISTS set_tenant(UUID);
