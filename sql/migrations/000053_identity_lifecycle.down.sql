-- Rollback Migration 053: identity lifecycle, MFA, passkeys, and session control.

DROP TRIGGER IF EXISTS trg_identity_security_events_immutable ON identity_security_events;
DROP FUNCTION IF EXISTS reject_identity_security_event_mutation();

DROP TABLE IF EXISTS identity_security_events;
DROP TABLE IF EXISTS identity_step_up_grants;
DROP TABLE IF EXISTS identity_authentication_challenges;
DROP TRIGGER IF EXISTS trg_identity_passkeys_updated_at ON identity_passkeys;
DROP TABLE IF EXISTS identity_passkeys;
DROP TABLE IF EXISTS identity_mfa_recovery_codes;
DROP TABLE IF EXISTS identity_email_verification_tokens;
DROP TABLE IF EXISTS identity_invitations;
DROP TRIGGER IF EXISTS trg_tenant_identity_policies_updated_at ON tenant_identity_policies;
DROP TABLE IF EXISTS tenant_identity_policies;

DROP INDEX IF EXISTS idx_user_sessions_inventory;
DROP INDEX IF EXISTS uq_user_sessions_refresh_hash;
DROP INDEX IF EXISTS uq_user_sessions_token_hash;
ALTER TABLE user_sessions
    DROP CONSTRAINT IF EXISTS chk_user_session_revocation,
    DROP CONSTRAINT IF EXISTS chk_user_session_device_name,
    DROP CONSTRAINT IF EXISTS fk_user_sessions_revoker,
    DROP CONSTRAINT IF EXISTS fk_user_sessions_tenant_user,
    DROP CONSTRAINT IF EXISTS uq_user_sessions_org_id,
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS revoke_reason,
    DROP COLUMN IF EXISTS revoked_by,
    DROP COLUMN IF EXISTS mfa_verified_at,
    DROP COLUMN IF EXISTS authentication_method,
    DROP COLUMN IF EXISTS last_seen_at,
    DROP COLUMN IF EXISTS device_name;

DROP POLICY IF EXISTS password_reset_tenant_delete ON password_reset_tokens;
DROP POLICY IF EXISTS password_reset_tenant_update ON password_reset_tokens;
DROP POLICY IF EXISTS password_reset_tenant_insert ON password_reset_tokens;
DROP POLICY IF EXISTS password_reset_tenant_select ON password_reset_tokens;
ALTER TABLE password_reset_tokens DISABLE ROW LEVEL SECURITY;
DROP INDEX IF EXISTS idx_password_reset_tenant_user_active;
DROP INDEX IF EXISTS uq_password_reset_token_hash;
ALTER TABLE password_reset_tokens
    DROP CONSTRAINT IF EXISTS chk_password_reset_terminal,
    DROP CONSTRAINT IF EXISTS fk_password_reset_tenant_user,
    DROP COLUMN IF EXISTS request_id,
    DROP COLUMN IF EXISTS invalidated_at,
    DROP COLUMN IF EXISTS used_ip,
    DROP COLUMN IF EXISTS requested_user_agent,
    DROP COLUMN IF EXISTS requested_ip,
    DROP COLUMN IF EXISTS delivery_secret_ciphertext,
    DROP COLUMN IF EXISTS organization_id;

DROP POLICY IF EXISTS user_mfa_tenant_delete ON user_mfa;
DROP POLICY IF EXISTS user_mfa_tenant_update ON user_mfa;
DROP POLICY IF EXISTS user_mfa_tenant_insert ON user_mfa;
DROP POLICY IF EXISTS user_mfa_tenant_select ON user_mfa;
ALTER TABLE user_mfa DISABLE ROW LEVEL SECURITY;
DROP INDEX IF EXISTS idx_user_mfa_tenant_user_active;
DROP INDEX IF EXISTS uq_user_mfa_primary_active;
CREATE UNIQUE INDEX idx_user_mfa_primary ON user_mfa(user_id) WHERE is_primary=true;
ALTER TABLE user_mfa
    DROP CONSTRAINT IF EXISTS chk_user_mfa_disabled,
    DROP CONSTRAINT IF EXISTS chk_user_mfa_display_name,
    DROP CONSTRAINT IF EXISTS fk_user_mfa_disabler,
    DROP CONSTRAINT IF EXISTS fk_user_mfa_tenant_user,
    DROP CONSTRAINT IF EXISTS uq_user_mfa_org_id,
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS last_totp_step,
    DROP COLUMN IF EXISTS disable_reason,
    DROP COLUMN IF EXISTS disabled_by,
    DROP COLUMN IF EXISTS disabled_at,
    DROP COLUMN IF EXISTS verified_at,
    DROP COLUMN IF EXISTS display_name,
    DROP COLUMN IF EXISTS organization_id;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS chk_users_mfa_exemption,
    DROP COLUMN IF EXISTS mfa_exempt_reason,
    DROP COLUMN IF EXISTS mfa_exempt_until,
    DROP COLUMN IF EXISTS identity_version,
    DROP COLUMN IF EXISTS email_verified_at;
