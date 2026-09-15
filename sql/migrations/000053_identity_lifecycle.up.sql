-- Migration 053: secure identity lifecycle, MFA, passkeys, and session control.

ALTER TABLE users
    ADD COLUMN email_verified_at TIMESTAMPTZ,
    ADD COLUMN identity_version BIGINT NOT NULL DEFAULT 1 CHECK (identity_version > 0),
    ADD COLUMN mfa_exempt_until TIMESTAMPTZ,
    ADD COLUMN mfa_exempt_reason VARCHAR(1000),
    ADD CONSTRAINT chk_users_mfa_exemption CHECK (
        (mfa_exempt_until IS NULL AND mfa_exempt_reason IS NULL)
        OR (mfa_exempt_until IS NOT NULL AND mfa_exempt_reason=BTRIM(mfa_exempt_reason)
            AND length(mfa_exempt_reason) BETWEEN 3 AND 1000)
    );

-- Accounts that were already active before email-verification state existed
-- are grandfathered as verified. New registrations remain pending until the
-- one-time verification credential is consumed.
UPDATE users
SET email_verified_at = COALESCE(email_verified_at, created_at)
WHERE status = 'active' AND deleted_at IS NULL;

-- Existing MFA and password-reset tables predate tenant-aware foreign keys.
-- Backfill the owning tenant, then force every subsequent access through RLS.
ALTER TABLE user_mfa ADD COLUMN organization_id UUID;
UPDATE user_mfa factor
SET organization_id = account.organization_id
FROM users account
WHERE account.id = factor.user_id;
ALTER TABLE user_mfa
    ALTER COLUMN organization_id SET NOT NULL,
    ADD COLUMN display_name VARCHAR(120),
    ADD COLUMN verified_at TIMESTAMPTZ,
    ADD COLUMN disabled_at TIMESTAMPTZ,
    ADD COLUMN disabled_by UUID,
    ADD COLUMN disable_reason VARCHAR(1000),
    ADD COLUMN last_totp_step BIGINT,
    ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    ADD CONSTRAINT uq_user_mfa_org_id UNIQUE (organization_id, id),
    ADD CONSTRAINT fk_user_mfa_tenant_user FOREIGN KEY (organization_id, user_id)
        REFERENCES users(organization_id, id) ON DELETE CASCADE,
    ADD CONSTRAINT fk_user_mfa_disabler FOREIGN KEY (organization_id, disabled_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_user_mfa_display_name CHECK (
        display_name IS NULL OR (display_name=BTRIM(display_name) AND length(display_name) BETWEEN 2 AND 120)
    ),
    ADD CONSTRAINT chk_user_mfa_disabled CHECK (
        (disabled_at IS NULL AND disabled_by IS NULL AND disable_reason IS NULL)
        OR (disabled_at IS NOT NULL AND disabled_by IS NOT NULL
            AND disable_reason=BTRIM(disable_reason) AND length(disable_reason) BETWEEN 3 AND 1000)
    );

UPDATE user_mfa SET verified_at=COALESCE(verified_at, created_at) WHERE is_verified;
DROP INDEX idx_user_mfa_primary;
CREATE UNIQUE INDEX uq_user_mfa_primary_active
    ON user_mfa (organization_id, user_id)
    WHERE is_primary AND disabled_at IS NULL;
CREATE INDEX idx_user_mfa_tenant_user_active
    ON user_mfa (organization_id, user_id, method)
    WHERE is_verified AND disabled_at IS NULL;

ALTER TABLE user_mfa ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_mfa FORCE ROW LEVEL SECURITY;
CREATE POLICY user_mfa_tenant_select ON user_mfa FOR SELECT
    USING (organization_id=get_current_tenant());
CREATE POLICY user_mfa_tenant_insert ON user_mfa FOR INSERT
    WITH CHECK (organization_id=get_current_tenant());
CREATE POLICY user_mfa_tenant_update ON user_mfa FOR UPDATE
    USING (organization_id=get_current_tenant()) WITH CHECK (organization_id=get_current_tenant());
CREATE POLICY user_mfa_tenant_delete ON user_mfa FOR DELETE
    USING (organization_id=get_current_tenant());

ALTER TABLE password_reset_tokens ADD COLUMN organization_id UUID;
UPDATE password_reset_tokens reset_token
SET organization_id = account.organization_id
FROM users account
WHERE account.id = reset_token.user_id;
ALTER TABLE password_reset_tokens
    ALTER COLUMN organization_id SET NOT NULL,
    ADD COLUMN delivery_secret_ciphertext TEXT,
    ADD COLUMN requested_ip INET,
    ADD COLUMN requested_user_agent VARCHAR(500),
    ADD COLUMN used_ip INET,
    ADD COLUMN invalidated_at TIMESTAMPTZ,
    ADD COLUMN request_id VARCHAR(255),
    ADD CONSTRAINT fk_password_reset_tenant_user FOREIGN KEY (organization_id, user_id)
        REFERENCES users(organization_id, id) ON DELETE CASCADE,
    ADD CONSTRAINT chk_password_reset_terminal CHECK (
        NOT (used_at IS NOT NULL AND invalidated_at IS NOT NULL)
    );
CREATE UNIQUE INDEX uq_password_reset_token_hash ON password_reset_tokens(token_hash);
CREATE INDEX idx_password_reset_tenant_user_active
    ON password_reset_tokens(organization_id, user_id, expires_at DESC)
    WHERE used_at IS NULL AND invalidated_at IS NULL;

ALTER TABLE password_reset_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE password_reset_tokens FORCE ROW LEVEL SECURITY;
CREATE POLICY password_reset_tenant_select ON password_reset_tokens FOR SELECT
    USING (organization_id=get_current_tenant());
CREATE POLICY password_reset_tenant_insert ON password_reset_tokens FOR INSERT
    WITH CHECK (organization_id=get_current_tenant());
CREATE POLICY password_reset_tenant_update ON password_reset_tokens FOR UPDATE
    USING (organization_id=get_current_tenant()) WITH CHECK (organization_id=get_current_tenant());
CREATE POLICY password_reset_tenant_delete ON password_reset_tokens FOR DELETE
    USING (organization_id=get_current_tenant());

ALTER TABLE user_sessions
    ADD COLUMN device_name VARCHAR(120),
    ADD COLUMN last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN authentication_method VARCHAR(24) NOT NULL DEFAULT 'password'
        CHECK (authentication_method IN ('password','totp','recovery_code','passkey')),
    ADD COLUMN mfa_verified_at TIMESTAMPTZ,
    ADD COLUMN revoked_by UUID,
    ADD COLUMN revoke_reason VARCHAR(1000),
    ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    ADD CONSTRAINT uq_user_sessions_org_id UNIQUE (organization_id,id),
    ADD CONSTRAINT fk_user_sessions_tenant_user FOREIGN KEY (organization_id, user_id)
        REFERENCES users(organization_id, id) ON DELETE CASCADE,
    ADD CONSTRAINT fk_user_sessions_revoker FOREIGN KEY (organization_id, revoked_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_user_session_device_name CHECK (
        device_name IS NULL OR (device_name=BTRIM(device_name) AND length(device_name) BETWEEN 2 AND 120)
    );
UPDATE user_sessions SET revoke_reason='legacy revocation' WHERE revoked_at IS NOT NULL;
ALTER TABLE user_sessions ADD CONSTRAINT chk_user_session_revocation CHECK (
        (revoked_at IS NULL AND revoked_by IS NULL AND revoke_reason IS NULL)
        OR (revoked_at IS NOT NULL AND revoked_by IS NULL AND revoke_reason IS NULL)
        OR (revoked_at IS NOT NULL AND revoke_reason=BTRIM(revoke_reason)
            AND length(revoke_reason) BETWEEN 3 AND 1000)
    );
CREATE UNIQUE INDEX uq_user_sessions_token_hash ON user_sessions(token_hash);
CREATE UNIQUE INDEX uq_user_sessions_refresh_hash ON user_sessions(refresh_token_hash)
    WHERE refresh_token_hash IS NOT NULL;
CREATE INDEX idx_user_sessions_inventory
    ON user_sessions(organization_id, user_id, last_seen_at DESC, id DESC);

CREATE TABLE tenant_identity_policies (
    organization_id UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    require_mfa BOOLEAN NOT NULL DEFAULT false,
    require_mfa_for_admins BOOLEAN NOT NULL DEFAULT true,
    allowed_methods TEXT[] NOT NULL DEFAULT ARRAY['totp','passkey']::TEXT[],
    enrollment_grace_hours INTEGER NOT NULL DEFAULT 24 CHECK (enrollment_grace_hours BETWEEN 0 AND 720),
    authentication_challenge_minutes INTEGER NOT NULL DEFAULT 5
        CHECK (authentication_challenge_minutes BETWEEN 1 AND 15),
    step_up_ttl_minutes INTEGER NOT NULL DEFAULT 10 CHECK (step_up_ttl_minutes BETWEEN 1 AND 30),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_by UUID NOT NULL,
    update_reason VARCHAR(1000) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_identity_policy_updater FOREIGN KEY (organization_id, updated_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_identity_policy_methods CHECK (
        cardinality(allowed_methods) BETWEEN 1 AND 2
        AND allowed_methods <@ ARRAY['totp','passkey']::TEXT[]
        AND (cardinality(allowed_methods)=1 OR allowed_methods[1]<>allowed_methods[2])
    ),
    CONSTRAINT chk_identity_policy_reason CHECK (
        update_reason=BTRIM(update_reason) AND length(update_reason) BETWEEN 3 AND 1000
    )
);
CREATE TRIGGER trg_tenant_identity_policies_updated_at
    BEFORE UPDATE ON tenant_identity_policies
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE identity_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    email VARCHAR(320) NOT NULL,
    token_hash CHAR(64) NOT NULL,
    delivery_secret_ciphertext TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_by UUID NOT NULL,
    request_id VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_identity_invitation_user FOREIGN KEY (organization_id, user_id)
        REFERENCES users(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_identity_invitation_creator FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_identity_invitation_email CHECK (email=lower(BTRIM(email))),
    CONSTRAINT chk_identity_invitation_terminal CHECK (NOT (accepted_at IS NOT NULL AND revoked_at IS NOT NULL)),
    CONSTRAINT chk_identity_invitation_expiry CHECK (expires_at > created_at)
);
CREATE UNIQUE INDEX uq_identity_invitation_token ON identity_invitations(token_hash);
CREATE UNIQUE INDEX uq_identity_invitation_user_active
    ON identity_invitations(organization_id,user_id)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;
CREATE INDEX idx_identity_invitation_expiry ON identity_invitations(expires_at)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

CREATE TABLE identity_email_verification_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    token_hash CHAR(64) NOT NULL,
    delivery_secret_ciphertext TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ,
    invalidated_at TIMESTAMPTZ,
    request_id VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_email_verification_user FOREIGN KEY (organization_id, user_id)
        REFERENCES users(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_email_verification_terminal CHECK (NOT (verified_at IS NOT NULL AND invalidated_at IS NOT NULL)),
    CONSTRAINT chk_email_verification_expiry CHECK (expires_at > created_at)
);
CREATE UNIQUE INDEX uq_email_verification_token ON identity_email_verification_tokens(token_hash);
CREATE UNIQUE INDEX uq_email_verification_user_active
    ON identity_email_verification_tokens(organization_id,user_id)
    WHERE verified_at IS NULL AND invalidated_at IS NULL;
CREATE INDEX idx_email_verification_expiry ON identity_email_verification_tokens(expires_at)
    WHERE verified_at IS NULL AND invalidated_at IS NULL;

CREATE TABLE identity_mfa_recovery_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    mfa_id UUID NOT NULL,
    code_hash VARCHAR(255) NOT NULL,
    position SMALLINT NOT NULL CHECK (position BETWEEN 1 AND 20),
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_recovery_code_user FOREIGN KEY (organization_id,user_id)
        REFERENCES users(organization_id,id) ON DELETE CASCADE,
    CONSTRAINT fk_recovery_code_mfa FOREIGN KEY (organization_id,mfa_id)
        REFERENCES user_mfa(organization_id,id) ON DELETE CASCADE,
    CONSTRAINT uq_recovery_code_position UNIQUE (organization_id,mfa_id,position)
);
CREATE INDEX idx_recovery_codes_active ON identity_mfa_recovery_codes(organization_id,user_id,mfa_id)
    WHERE consumed_at IS NULL;

CREATE TABLE identity_passkeys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    credential_id BYTEA NOT NULL,
    credential_ciphertext TEXT NOT NULL,
    attestation_type VARCHAR(64) NOT NULL DEFAULT 'none',
    transport TEXT[] NOT NULL DEFAULT '{}'::TEXT[],
    sign_count BIGINT NOT NULL DEFAULT 0 CHECK (sign_count >= 0),
    clone_warning BOOLEAN NOT NULL DEFAULT false,
    backup_eligible BOOLEAN NOT NULL DEFAULT false,
    backup_state BOOLEAN NOT NULL DEFAULT false,
    device_name VARCHAR(120) NOT NULL,
    aaguid UUID,
    last_used_at TIMESTAMPTZ,
    removed_at TIMESTAMPTZ,
    removed_by UUID,
    removal_reason VARCHAR(1000),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_identity_passkey_org_id UNIQUE (organization_id,id),
    CONSTRAINT fk_identity_passkey_user FOREIGN KEY (organization_id,user_id)
        REFERENCES users(organization_id,id) ON DELETE CASCADE,
    CONSTRAINT fk_identity_passkey_remover FOREIGN KEY (organization_id,removed_by)
        REFERENCES users(organization_id,id) ON DELETE RESTRICT,
    CONSTRAINT chk_identity_passkey_name CHECK (device_name=BTRIM(device_name) AND length(device_name) BETWEEN 2 AND 120),
    CONSTRAINT chk_identity_passkey_transport CHECK (transport <@ ARRAY['ble','hybrid','internal','nfc','usb']::TEXT[]),
    CONSTRAINT chk_identity_passkey_removed CHECK (
        (removed_at IS NULL AND removed_by IS NULL AND removal_reason IS NULL)
        OR (removed_at IS NOT NULL AND removed_by IS NOT NULL AND removal_reason=BTRIM(removal_reason)
            AND length(removal_reason) BETWEEN 3 AND 1000)
    )
);
CREATE UNIQUE INDEX uq_identity_passkey_credential ON identity_passkeys(credential_id);
CREATE INDEX idx_identity_passkey_user_active ON identity_passkeys(organization_id,user_id,created_at DESC)
    WHERE removed_at IS NULL;
CREATE TRIGGER trg_identity_passkeys_updated_at BEFORE UPDATE ON identity_passkeys
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE identity_authentication_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    session_id UUID,
    token_hash CHAR(64) NOT NULL,
    purpose VARCHAR(32) NOT NULL CHECK (purpose IN ('login','step_up','passkey_registration','passkey_authentication')),
    allowed_methods TEXT[] NOT NULL DEFAULT '{}'::TEXT[],
    webauthn_session JSONB,
    step_up_purpose VARCHAR(80),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    failed_attempts SMALLINT NOT NULL DEFAULT 0 CHECK (failed_attempts BETWEEN 0 AND 10),
    max_attempts SMALLINT NOT NULL DEFAULT 5 CHECK (max_attempts BETWEEN 1 AND 10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_identity_challenge_user FOREIGN KEY (organization_id,user_id)
        REFERENCES users(organization_id,id) ON DELETE CASCADE,
    CONSTRAINT fk_identity_challenge_session FOREIGN KEY (organization_id,session_id)
        REFERENCES user_sessions(organization_id,id) ON DELETE CASCADE,
    CONSTRAINT chk_identity_challenge_expiry CHECK (expires_at > created_at),
    CONSTRAINT chk_identity_challenge_webauthn CHECK (
        (purpose IN ('passkey_registration','passkey_authentication') AND jsonb_typeof(webauthn_session)='object')
        OR (purpose IN ('login','step_up') AND (webauthn_session IS NULL OR jsonb_typeof(webauthn_session)='object'))
    ),
    CONSTRAINT chk_identity_challenge_step_up CHECK (
        (purpose='step_up' AND step_up_purpose IS NOT NULL AND session_id IS NOT NULL)
        OR (purpose<>'step_up' AND step_up_purpose IS NULL)
    )
);
CREATE UNIQUE INDEX uq_identity_challenge_token ON identity_authentication_challenges(token_hash);
CREATE INDEX idx_identity_challenge_active ON identity_authentication_challenges(organization_id,user_id,purpose,expires_at)
    WHERE consumed_at IS NULL;

CREATE TABLE identity_step_up_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    session_id UUID NOT NULL,
    token_hash CHAR(64) NOT NULL,
    purpose VARCHAR(80) NOT NULL,
    authentication_method VARCHAR(24) NOT NULL CHECK (authentication_method IN ('totp','recovery_code','passkey')),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_step_up_user FOREIGN KEY (organization_id,user_id)
        REFERENCES users(organization_id,id) ON DELETE CASCADE,
    CONSTRAINT fk_step_up_session FOREIGN KEY (organization_id,session_id)
        REFERENCES user_sessions(organization_id,id) ON DELETE CASCADE,
    CONSTRAINT chk_step_up_purpose CHECK (purpose ~ '^[a-z][a-z0-9_.:-]{2,79}$'),
    CONSTRAINT chk_step_up_expiry CHECK (expires_at > created_at),
    CONSTRAINT chk_step_up_terminal CHECK (NOT (consumed_at IS NOT NULL AND revoked_at IS NOT NULL))
);
CREATE UNIQUE INDEX uq_step_up_grant_token ON identity_step_up_grants(token_hash);
CREATE INDEX idx_step_up_grant_active ON identity_step_up_grants(organization_id,user_id,session_id,purpose,expires_at)
    WHERE consumed_at IS NULL AND revoked_at IS NULL;

CREATE TABLE identity_security_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID,
    actor_user_id UUID,
    session_id UUID,
    event_type VARCHAR(80) NOT NULL,
    reason VARCHAR(1000) NOT NULL,
    request_id VARCHAR(255),
    ip_address INET,
    user_agent VARCHAR(500),
    details JSONB NOT NULL DEFAULT '{}'::JSONB CHECK (jsonb_typeof(details)='object' AND pg_column_size(details)<=32768),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_identity_event_user FOREIGN KEY (organization_id,user_id)
        REFERENCES users(organization_id,id) ON DELETE SET NULL (user_id),
    CONSTRAINT fk_identity_event_actor FOREIGN KEY (organization_id,actor_user_id)
        REFERENCES users(organization_id,id) ON DELETE SET NULL (actor_user_id),
    CONSTRAINT fk_identity_event_session FOREIGN KEY (organization_id,session_id)
        REFERENCES user_sessions(organization_id,id) ON DELETE SET NULL (session_id),
    CONSTRAINT chk_identity_event_type CHECK (event_type ~ '^[a-z][a-z0-9_.-]{2,79}$'),
    CONSTRAINT chk_identity_event_reason CHECK (reason=BTRIM(reason) AND length(reason) BETWEEN 3 AND 1000)
);
CREATE INDEX idx_identity_security_events_tenant_time
    ON identity_security_events(organization_id,created_at DESC,id DESC);
CREATE INDEX idx_identity_security_events_user_time
    ON identity_security_events(organization_id,user_id,created_at DESC)
    WHERE user_id IS NOT NULL;

CREATE FUNCTION reject_identity_security_event_mutation()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'identity security events are immutable' USING ERRCODE='55000';
END;
$$;
CREATE TRIGGER trg_identity_security_events_immutable
    BEFORE UPDATE OR DELETE ON identity_security_events
    FOR EACH ROW EXECUTE FUNCTION reject_identity_security_event_mutation();

-- Uniform tenant isolation for every identity lifecycle table.
ALTER TABLE tenant_identity_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_identity_policies FORCE ROW LEVEL SECURITY;
ALTER TABLE identity_invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity_invitations FORCE ROW LEVEL SECURITY;
ALTER TABLE identity_email_verification_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity_email_verification_tokens FORCE ROW LEVEL SECURITY;
ALTER TABLE identity_mfa_recovery_codes ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity_mfa_recovery_codes FORCE ROW LEVEL SECURITY;
ALTER TABLE identity_passkeys ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity_passkeys FORCE ROW LEVEL SECURITY;
ALTER TABLE identity_authentication_challenges ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity_authentication_challenges FORCE ROW LEVEL SECURITY;
ALTER TABLE identity_step_up_grants ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity_step_up_grants FORCE ROW LEVEL SECURITY;
ALTER TABLE identity_security_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity_security_events FORCE ROW LEVEL SECURITY;

DO $$
DECLARE table_name TEXT;
BEGIN
    FOREACH table_name IN ARRAY ARRAY[
        'tenant_identity_policies','identity_invitations','identity_email_verification_tokens',
        'identity_mfa_recovery_codes','identity_passkeys','identity_authentication_challenges',
        'identity_step_up_grants','identity_security_events'
    ] LOOP
        EXECUTE format('CREATE POLICY %I ON %I FOR SELECT USING (organization_id=get_current_tenant())', table_name || '_select', table_name);
        EXECUTE format('CREATE POLICY %I ON %I FOR INSERT WITH CHECK (organization_id=get_current_tenant())', table_name || '_insert', table_name);
        EXECUTE format('CREATE POLICY %I ON %I FOR UPDATE USING (organization_id=get_current_tenant()) WITH CHECK (organization_id=get_current_tenant())', table_name || '_update', table_name);
        EXECUTE format('CREATE POLICY %I ON %I FOR DELETE USING (organization_id=get_current_tenant())', table_name || '_delete', table_name);
    END LOOP;
END;
$$;

COMMENT ON TABLE identity_invitations IS 'One-time, tenant-bound invitation credentials; plaintext tokens are never stored.';
COMMENT ON TABLE identity_authentication_challenges IS 'Short-lived, one-time login, step-up, and WebAuthn ceremony state.';
COMMENT ON TABLE identity_step_up_grants IS 'Session-bound, purpose-bound proof of recent strong authentication.';
COMMENT ON TABLE identity_security_events IS 'Append-only identity security history; mutations are rejected by trigger.';
