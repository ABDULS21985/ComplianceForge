-- Rollback Migration 051: enterprise user and group administration.

DROP TRIGGER IF EXISTS trg_directory_change_events_immutable ON directory_change_events;
DROP TRIGGER IF EXISTS trg_directory_imports_immutable ON directory_imports;
DROP FUNCTION IF EXISTS prevent_directory_immutable_mutation();
DROP TRIGGER IF EXISTS trg_directory_change_events_scope ON directory_change_events;
DROP FUNCTION IF EXISTS validate_directory_change_event_scope();
DROP TABLE IF EXISTS directory_change_events;
DROP TABLE IF EXISTS directory_imports;
DROP TABLE IF EXISTS directory_group_memberships;
DROP TRIGGER IF EXISTS trg_directory_groups_updated_at ON directory_groups;
DROP TABLE IF EXISTS directory_groups;

DROP INDEX IF EXISTS idx_users_org_directory_updated;
DROP INDEX IF EXISTS idx_users_org_location;
DROP INDEX IF EXISTS idx_users_org_manager;
DROP INDEX IF EXISTS idx_users_directory_search;
DROP INDEX IF EXISTS uq_users_org_employee_active;
DROP INDEX IF EXISTS uq_users_org_email_ci;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS chk_users_deprovision_state,
    DROP CONSTRAINT IF EXISTS chk_users_invitation_window,
    DROP CONSTRAINT IF EXISTS chk_users_directory_text,
    DROP CONSTRAINT IF EXISTS chk_users_manager_not_self,
    DROP CONSTRAINT IF EXISTS fk_users_updater_tenant,
    DROP CONSTRAINT IF EXISTS fk_users_deprovisioner_tenant,
    DROP CONSTRAINT IF EXISTS fk_users_suspender_tenant,
    DROP CONSTRAINT IF EXISTS fk_users_inviter_tenant,
    DROP CONSTRAINT IF EXISTS fk_users_manager_tenant,
    DROP COLUMN IF EXISTS directory_search,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS deprovision_reason,
    DROP COLUMN IF EXISTS deprovisioned_by,
    DROP COLUMN IF EXISTS deprovisioned_at,
    DROP COLUMN IF EXISTS reactivated_at,
    DROP COLUMN IF EXISTS suspension_reason,
    DROP COLUMN IF EXISTS suspended_by,
    DROP COLUMN IF EXISTS suspended_at,
    DROP COLUMN IF EXISTS invited_by,
    DROP COLUMN IF EXISTS invitation_expires_at,
    DROP COLUMN IF EXISTS invited_at,
    DROP COLUMN IF EXISTS invitation_status,
    DROP COLUMN IF EXISTS location,
    DROP COLUMN IF EXISTS employee_id,
    DROP COLUMN IF EXISTS manager_user_id,
    DROP COLUMN IF EXISTS version;
