-- Migration 003: Users, Authentication & MFA
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - email is unique per organization (not globally) — supports consultants
--     working across multiple client orgs with the same email
--   - password_hash uses pgcrypto-compatible format (bcrypt via application layer)
--   - failed_login_attempts + locked_until implement automatic account lockout
--   - INET type for IP addresses supports both IPv4 and IPv6
--   - user_sessions stores hashed tokens, never plaintext — the app layer hashes
--     before insert/lookup
--   - password_reset_tokens have an explicit used_at to prevent replay attacks
--   - user_mfa.secret_encrypted is BYTEA — the application encrypts the TOTP
--     secret with a server-side key before storage

-- ============================================================================
-- TABLE: users
-- ============================================================================

CREATE TABLE users (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email                       VARCHAR(320) NOT NULL,  -- RFC 5321 max email length
    password_hash               VARCHAR(255),
    first_name                  VARCHAR(100),
    last_name                   VARCHAR(100),
    job_title                   VARCHAR(200),
    department                  VARCHAR(200),
    phone                       VARCHAR(50),
    avatar_url                  TEXT,
    status                      user_status NOT NULL DEFAULT 'pending_verification',
    is_super_admin              BOOLEAN NOT NULL DEFAULT false,
    timezone                    VARCHAR(50),
    language                    VARCHAR(10) DEFAULT 'en',
    last_login_at               TIMESTAMPTZ,
    last_login_ip               INET,
    password_changed_at         TIMESTAMPTZ,
    failed_login_attempts       INT NOT NULL DEFAULT 0,
    locked_until                TIMESTAMPTZ,
    notification_preferences    JSONB DEFAULT '{}',
    metadata                    JSONB DEFAULT '{}',
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at                  TIMESTAMPTZ,

    -- Email is unique within an organization (not globally).
    CONSTRAINT uq_users_org_email UNIQUE (organization_id, email)
);

-- Indexes
CREATE INDEX idx_users_organization ON users(organization_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_email ON users(email) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_status ON users(status) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_department ON users(organization_id, department) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_last_login ON users(last_login_at DESC NULLS LAST) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_locked ON users(locked_until) WHERE locked_until IS NOT NULL;
CREATE INDEX idx_users_deleted_at ON users(deleted_at) WHERE deleted_at IS NOT NULL;
-- Trigram index for user search by name
CREATE INDEX idx_users_name_trgm ON users USING gin (
    (first_name || ' ' || last_name) gin_trgm_ops
);

-- Trigger
CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE users IS 'User accounts scoped to an organization. Email uniqueness is per-org to support multi-tenant consultants.';
COMMENT ON COLUMN users.is_super_admin IS 'Platform-level admin flag. Only settable via direct DB access or super-admin API.';
COMMENT ON COLUMN users.failed_login_attempts IS 'Incremented on failed auth. Reset to 0 on success. Triggers lockout at threshold (app-configured).';

-- ============================================================================
-- TABLE: user_mfa
-- ============================================================================

CREATE TABLE user_mfa (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    method              mfa_method NOT NULL,
    secret_encrypted    BYTEA,                      -- AES-256-GCM encrypted TOTP secret
    is_primary          BOOLEAN NOT NULL DEFAULT false,
    is_verified         BOOLEAN NOT NULL DEFAULT false,
    recovery_codes_hash TEXT[],                      -- bcrypt hashes of one-time recovery codes
    last_used_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_user_mfa_user ON user_mfa(user_id);
-- Ensure only one primary MFA method per user
CREATE UNIQUE INDEX idx_user_mfa_primary ON user_mfa(user_id) WHERE is_primary = true;

-- Trigger
CREATE TRIGGER trg_user_mfa_updated_at
    BEFORE UPDATE ON user_mfa
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE user_mfa IS 'Multi-factor authentication methods per user. secret_encrypted is AES-256-GCM encrypted by the application.';
COMMENT ON COLUMN user_mfa.recovery_codes_hash IS 'Array of bcrypt-hashed one-time recovery codes. Each code is removed from the array after use.';

-- ============================================================================
-- TABLE: user_sessions
-- ============================================================================

CREATE TABLE user_sessions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    token_hash          VARCHAR(255) NOT NULL,       -- SHA-256 hash of the JWT/session token
    refresh_token_hash  VARCHAR(255),                -- SHA-256 hash of the refresh token
    ip_address          INET,
    user_agent          TEXT,
    expires_at          TIMESTAMPTZ NOT NULL,
    revoked_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_sessions_user ON user_sessions(user_id);
CREATE INDEX idx_sessions_org ON user_sessions(organization_id);
CREATE INDEX idx_sessions_token ON user_sessions(token_hash);
CREATE INDEX idx_sessions_refresh ON user_sessions(refresh_token_hash) WHERE refresh_token_hash IS NOT NULL;
CREATE INDEX idx_sessions_expires ON user_sessions(expires_at);
-- Partial index for active (non-revoked, non-expired) sessions
CREATE INDEX idx_sessions_active ON user_sessions(user_id, organization_id)
    WHERE revoked_at IS NULL AND expires_at > NOW();

COMMENT ON TABLE user_sessions IS 'Active user sessions. Tokens are stored as SHA-256 hashes — never in plaintext.';
COMMENT ON COLUMN user_sessions.revoked_at IS 'Set when user logs out or session is force-revoked by admin. Checked on every request.';

-- ============================================================================
-- TABLE: password_reset_tokens
-- ============================================================================

CREATE TABLE password_reset_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  VARCHAR(255) NOT NULL,               -- SHA-256 hash of the reset token
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,                         -- set on use; prevents replay
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_password_reset_user ON password_reset_tokens(user_id);
CREATE INDEX idx_password_reset_token ON password_reset_tokens(token_hash);
-- Partial index for unused, non-expired tokens
CREATE INDEX idx_password_reset_active ON password_reset_tokens(token_hash)
    WHERE used_at IS NULL AND expires_at > NOW();

COMMENT ON TABLE password_reset_tokens IS 'One-time password reset tokens. used_at prevents replay; expires_at enforces time limit.';
