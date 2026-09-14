-- Rollback Migration 049: Enterprise RBAC administration.

DROP TRIGGER IF EXISTS trg_role_change_events_tenant_scope ON role_change_events;
DROP FUNCTION IF EXISTS validate_role_change_event_scope();
DROP TABLE IF EXISTS role_change_events;
DROP TRIGGER IF EXISTS trg_user_roles_tenant_scope ON user_roles;
DROP FUNCTION IF EXISTS validate_user_role_tenant_scope();
ALTER TABLE user_roles DROP CONSTRAINT IF EXISTS fk_user_roles_user_tenant;
ALTER TABLE user_roles DROP CONSTRAINT IF EXISTS fk_user_roles_assigner_tenant;
ALTER TABLE roles DROP CONSTRAINT IF EXISTS fk_roles_updater_tenant;
ALTER TABLE roles DROP CONSTRAINT IF EXISTS fk_roles_creator_tenant;
ALTER TABLE roles DROP CONSTRAINT IF EXISTS chk_roles_scope_kind;
DROP INDEX IF EXISTS uq_roles_org_name_active;
ALTER TABLE roles
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS version;
