-- Rollback Migration 055: tenant-safe SCIM 2.0 provisioning.

DROP FUNCTION IF EXISTS resolve_scim_token_tenant(TEXT);
DROP TRIGGER IF EXISTS trg_scim_tokens_tenant_index ON scim_tokens;
DROP FUNCTION IF EXISTS maintain_scim_token_tenant_index();
DROP TABLE IF EXISTS scim_token_tenant_index;

DROP TRIGGER IF EXISTS trg_scim_resource_events_immutable ON scim_resource_events;
DROP TRIGGER IF EXISTS trg_scim_token_events_immutable ON scim_token_events;
DROP FUNCTION IF EXISTS reject_scim_event_mutation();
DROP TRIGGER IF EXISTS trg_scim_resource_events_scope ON scim_resource_events;
DROP FUNCTION IF EXISTS validate_scim_resource_event_scope();
DROP TABLE IF EXISTS scim_resource_events;
DROP TABLE IF EXISTS scim_token_events;
DROP TRIGGER IF EXISTS trg_scim_tokens_updated_at ON scim_tokens;
DROP TABLE IF EXISTS scim_tokens;
DROP FUNCTION IF EXISTS scim_scopes_are_unique(TEXT[]);

DROP INDEX IF EXISTS idx_directory_groups_scim_managed;
DROP INDEX IF EXISTS uq_directory_groups_scim_external_active;
ALTER TABLE directory_groups
    DROP CONSTRAINT IF EXISTS chk_directory_groups_scim_static,
    DROP CONSTRAINT IF EXISTS chk_directory_groups_scim_external_id,
    DROP COLUMN IF EXISTS scim_managed,
    DROP COLUMN IF EXISTS scim_external_id;

-- The pre-055 constraint required every deprovisioned user to be soft-deleted.
-- Normalize SCIM active=false rows before restoring that stricter historical
-- invariant so a rollback never fails halfway through or restores invalid data.
UPDATE users
SET deleted_at=COALESCE(deleted_at,deprovisioned_at)
WHERE deprovisioned_at IS NOT NULL AND deleted_at IS NULL;

DROP INDEX IF EXISTS idx_users_scim_managed;
DROP INDEX IF EXISTS uq_users_scim_external_active;
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS chk_users_deprovision_state,
    DROP CONSTRAINT IF EXISTS chk_users_scim_profile,
    DROP CONSTRAINT IF EXISTS chk_users_scim_external_id,
    DROP COLUMN IF EXISTS scim_managed,
    DROP COLUMN IF EXISTS scim_profile,
    DROP COLUMN IF EXISTS scim_external_id,
    ADD CONSTRAINT chk_users_deprovision_state CHECK (
        deprovisioned_at IS NULL OR (
            deleted_at IS NOT NULL AND status='inactive' AND deprovisioned_by IS NOT NULL
            AND deprovision_reason IS NOT NULL
        )
    );
