-- Migration 055: tenant-safe SCIM 2.0 provisioning and credential lifecycle.

-- Preserve SCIM attributes that do not have canonical directory columns while
-- continuing to use users and directory_groups as the only account authority.
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS chk_users_deprovision_state,
    ADD COLUMN scim_external_id VARCHAR(255),
    ADD COLUMN scim_profile JSONB NOT NULL DEFAULT '{}'::JSONB,
    ADD COLUMN scim_managed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD CONSTRAINT chk_users_scim_external_id CHECK (
        scim_external_id IS NULL OR (
            scim_external_id=BTRIM(scim_external_id) AND length(scim_external_id) BETWEEN 1 AND 255
        )
    ),
    ADD CONSTRAINT chk_users_scim_profile CHECK (
        jsonb_typeof(scim_profile)='object' AND pg_column_size(scim_profile)<=131072
    ),
    ADD CONSTRAINT chk_users_deprovision_state CHECK (
        deprovisioned_at IS NULL OR (
            status='inactive' AND deprovisioned_by IS NOT NULL AND deprovision_reason IS NOT NULL
        )
    );

CREATE UNIQUE INDEX uq_users_scim_external_active
    ON users (organization_id, scim_external_id)
    WHERE scim_external_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_users_scim_managed
    ON users (organization_id, updated_at DESC, id)
    WHERE scim_managed AND deleted_at IS NULL;

ALTER TABLE directory_groups
    ADD COLUMN scim_external_id VARCHAR(255),
    ADD COLUMN scim_managed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD CONSTRAINT chk_directory_groups_scim_external_id CHECK (
        scim_external_id IS NULL OR (
            scim_external_id=BTRIM(scim_external_id) AND length(scim_external_id) BETWEEN 1 AND 255
        )
    ),
    ADD CONSTRAINT chk_directory_groups_scim_static CHECK (
        NOT scim_managed OR (group_type='static' AND membership_rule='{}'::JSONB)
    );

CREATE UNIQUE INDEX uq_directory_groups_scim_external_active
    ON directory_groups (organization_id, scim_external_id)
    WHERE scim_external_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_directory_groups_scim_managed
    ON directory_groups (organization_id, updated_at DESC, id)
    WHERE scim_managed AND deleted_at IS NULL;

CREATE FUNCTION scim_scopes_are_unique(requested_scopes TEXT[])
RETURNS BOOLEAN LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT COUNT(*)=COUNT(DISTINCT scope_value) FROM unnest(requested_scopes) scope_value
$$;
GRANT EXECUTE ON FUNCTION scim_scopes_are_unique(TEXT[]) TO PUBLIC;

CREATE TABLE scim_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL,
    token_prefix VARCHAR(16) NOT NULL,
    token_hash CHAR(64) NOT NULL,
    scopes TEXT[] NOT NULL,
    rate_limit_per_minute INTEGER NOT NULL DEFAULT 120,
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    last_used_ip INET,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID NOT NULL,
    revoked_by UUID,
    revoked_at TIMESTAMPTZ,
    revoke_reason VARCHAR(1000),
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_scim_tokens_org_id UNIQUE (organization_id,id),
    CONSTRAINT uq_scim_tokens_prefix UNIQUE (token_prefix),
    CONSTRAINT uq_scim_tokens_hash UNIQUE (token_hash),
    CONSTRAINT fk_scim_tokens_creator_tenant FOREIGN KEY (organization_id,created_by)
        REFERENCES users(organization_id,id) ON DELETE RESTRICT,
    CONSTRAINT fk_scim_tokens_revoker_tenant FOREIGN KEY (organization_id,revoked_by)
        REFERENCES users(organization_id,id) ON DELETE RESTRICT,
    CONSTRAINT chk_scim_tokens_name CHECK (name=BTRIM(name) AND length(name) BETWEEN 2 AND 120),
    CONSTRAINT chk_scim_tokens_prefix CHECK (token_prefix ~ '^cfs_[A-Za-z0-9_-]{12}$'),
    CONSTRAINT chk_scim_tokens_hash CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_scim_tokens_scopes CHECK (
        cardinality(scopes) BETWEEN 1 AND 4
        AND scopes <@ ARRAY[
            'scim:users:read','scim:users:write','scim:groups:read','scim:groups:write'
        ]::TEXT[]
        AND scim_scopes_are_unique(scopes)
    ),
    CONSTRAINT chk_scim_tokens_rate CHECK (rate_limit_per_minute BETWEEN 1 AND 10000),
    CONSTRAINT chk_scim_tokens_expiry CHECK (expires_at IS NULL OR expires_at>created_at),
    CONSTRAINT chk_scim_tokens_version CHECK (version>0),
    CONSTRAINT chk_scim_tokens_revocation CHECK (
        (is_active AND revoked_at IS NULL AND revoked_by IS NULL AND revoke_reason IS NULL)
        OR (NOT is_active AND revoked_at IS NOT NULL AND revoked_by IS NOT NULL
            AND revoke_reason=BTRIM(revoke_reason) AND length(revoke_reason) BETWEEN 3 AND 1000)
    )
);

CREATE INDEX idx_scim_tokens_tenant_created
    ON scim_tokens (organization_id,created_at DESC,id DESC);
CREATE INDEX idx_scim_tokens_tenant_active
    ON scim_tokens (organization_id,expires_at,id)
    WHERE is_active AND revoked_at IS NULL;
CREATE TRIGGER trg_scim_tokens_updated_at
    BEFORE UPDATE ON scim_tokens
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Authentication must resolve the tenant before tenant RLS can be established.
-- This global boundary contains only an opaque high-entropy prefix and row IDs;
-- hashes, scopes, expiry, activity, and usage remain in the protected table.
CREATE TABLE scim_token_tenant_index (
    token_prefix VARCHAR(16) PRIMARY KEY,
    token_id UUID NOT NULL UNIQUE REFERENCES scim_tokens(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_scim_token_index_prefix CHECK (token_prefix ~ '^cfs_[A-Za-z0-9_-]{12}$')
);

CREATE FUNCTION maintain_scim_token_tenant_index()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path=pg_catalog,public
AS $$
BEGIN
    IF TG_OP='DELETE' THEN
        DELETE FROM public.scim_token_tenant_index WHERE token_id=OLD.id;
        RETURN OLD;
    END IF;
    INSERT INTO public.scim_token_tenant_index(token_prefix,token_id,organization_id)
    VALUES(NEW.token_prefix,NEW.id,NEW.organization_id)
    ON CONFLICT(token_id) DO UPDATE
        SET token_prefix=EXCLUDED.token_prefix,organization_id=EXCLUDED.organization_id;
    RETURN NEW;
END;
$$;
REVOKE ALL ON FUNCTION maintain_scim_token_tenant_index() FROM PUBLIC;

CREATE TRIGGER trg_scim_tokens_tenant_index
    AFTER INSERT OR UPDATE OF token_prefix,organization_id OR DELETE ON scim_tokens
    FOR EACH ROW EXECUTE FUNCTION maintain_scim_token_tenant_index();

CREATE FUNCTION resolve_scim_token_tenant(requested_prefix TEXT)
RETURNS TABLE(token_id UUID,organization_id UUID)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path=pg_catalog,public
AS $$
    SELECT idx.token_id,idx.organization_id
    FROM public.scim_token_tenant_index idx
    WHERE length(requested_prefix)=16 AND idx.token_prefix=requested_prefix
$$;
REVOKE ALL ON FUNCTION resolve_scim_token_tenant(TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION resolve_scim_token_tenant(TEXT) TO PUBLIC;

CREATE TABLE scim_token_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    token_id UUID NOT NULL,
    event_type VARCHAR(24) NOT NULL,
    actor_user_id UUID NOT NULL,
    reason VARCHAR(1000) NOT NULL,
    token_version BIGINT NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_scim_token_events_token FOREIGN KEY (organization_id,token_id)
        REFERENCES scim_tokens(organization_id,id) ON DELETE RESTRICT,
    CONSTRAINT fk_scim_token_events_actor FOREIGN KEY (organization_id,actor_user_id)
        REFERENCES users(organization_id,id) ON DELETE RESTRICT,
    CONSTRAINT chk_scim_token_events_type CHECK (event_type IN ('created','rotated','revoked','auto_revoked')),
    CONSTRAINT chk_scim_token_events_reason CHECK (
        reason=BTRIM(reason) AND length(reason) BETWEEN 3 AND 1000
    ),
    CONSTRAINT chk_scim_token_events_version CHECK (token_version>0),
    CONSTRAINT chk_scim_token_events_details CHECK (
        jsonb_typeof(details)='object' AND pg_column_size(details)<=32768
    )
);
CREATE INDEX idx_scim_token_events_tenant_token
    ON scim_token_events (organization_id,token_id,created_at DESC,id DESC);

CREATE TABLE scim_resource_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    token_id UUID NOT NULL,
    actor_user_id UUID NOT NULL,
    resource_type VARCHAR(16) NOT NULL,
    resource_id UUID NOT NULL,
    event_type VARCHAR(24) NOT NULL,
    resource_version BIGINT NOT NULL,
    reason VARCHAR(1000) NOT NULL,
    before_state JSONB,
    after_state JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_scim_resource_events_token FOREIGN KEY (organization_id,token_id)
        REFERENCES scim_tokens(organization_id,id) ON DELETE RESTRICT,
    CONSTRAINT fk_scim_resource_events_actor FOREIGN KEY (organization_id,actor_user_id)
        REFERENCES users(organization_id,id) ON DELETE RESTRICT,
    CONSTRAINT chk_scim_resource_events_resource CHECK (resource_type IN ('User','Group')),
    CONSTRAINT chk_scim_resource_events_type CHECK (
        event_type IN ('created','replaced','patched','deprovisioned','reactivated','deleted')
    ),
    CONSTRAINT chk_scim_resource_events_version CHECK (resource_version>0),
    CONSTRAINT chk_scim_resource_events_reason CHECK (
        reason=BTRIM(reason) AND length(reason) BETWEEN 3 AND 1000
    ),
    CONSTRAINT chk_scim_resource_events_states CHECK (
        (before_state IS NULL OR (jsonb_typeof(before_state)='object' AND pg_column_size(before_state)<=262144))
        AND (after_state IS NULL OR (jsonb_typeof(after_state)='object' AND pg_column_size(after_state)<=262144))
    )
);
CREATE INDEX idx_scim_resource_events_resource
    ON scim_resource_events (organization_id,resource_type,resource_id,created_at DESC,id DESC);

CREATE FUNCTION validate_scim_resource_event_scope()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.resource_type='User' AND NOT EXISTS(
        SELECT 1 FROM users resource
        WHERE resource.organization_id=NEW.organization_id AND resource.id=NEW.resource_id
    ) THEN
        RAISE EXCEPTION 'SCIM user event resource is outside the tenant' USING ERRCODE='23514';
    ELSIF NEW.resource_type='Group' AND NOT EXISTS(
        SELECT 1 FROM directory_groups resource
        WHERE resource.organization_id=NEW.organization_id AND resource.id=NEW.resource_id
    ) THEN
        RAISE EXCEPTION 'SCIM group event resource is outside the tenant' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER trg_scim_resource_events_scope
    BEFORE INSERT ON scim_resource_events
    FOR EACH ROW EXECUTE FUNCTION validate_scim_resource_event_scope();

CREATE FUNCTION reject_scim_event_mutation()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'SCIM security history is immutable' USING ERRCODE='55000';
END;
$$;
CREATE TRIGGER trg_scim_token_events_immutable
    BEFORE UPDATE OR DELETE ON scim_token_events
    FOR EACH ROW EXECUTE FUNCTION reject_scim_event_mutation();
CREATE TRIGGER trg_scim_resource_events_immutable
    BEFORE UPDATE OR DELETE ON scim_resource_events
    FOR EACH ROW EXECUTE FUNCTION reject_scim_event_mutation();

ALTER TABLE scim_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE scim_tokens FORCE ROW LEVEL SECURITY;
ALTER TABLE scim_token_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE scim_token_events FORCE ROW LEVEL SECURITY;
ALTER TABLE scim_resource_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE scim_resource_events FORCE ROW LEVEL SECURITY;

CREATE POLICY scim_tokens_select ON scim_tokens FOR SELECT
    USING (organization_id=get_current_tenant());
CREATE POLICY scim_tokens_insert ON scim_tokens FOR INSERT
    WITH CHECK (organization_id=get_current_tenant());
CREATE POLICY scim_tokens_update ON scim_tokens FOR UPDATE
    USING (organization_id=get_current_tenant()) WITH CHECK (organization_id=get_current_tenant());

CREATE POLICY scim_token_events_select ON scim_token_events FOR SELECT
    USING (organization_id=get_current_tenant());
CREATE POLICY scim_token_events_insert ON scim_token_events FOR INSERT
    WITH CHECK (organization_id=get_current_tenant());

CREATE POLICY scim_resource_events_select ON scim_resource_events FOR SELECT
    USING (organization_id=get_current_tenant());
CREATE POLICY scim_resource_events_insert ON scim_resource_events FOR INSERT
    WITH CHECK (organization_id=get_current_tenant());

COMMENT ON TABLE scim_tokens IS
    'Tenant-bound SCIM bearer credentials. Only SHA-256 hashes are stored; raw credentials are returned once.';
COMMENT ON TABLE scim_token_tenant_index IS
    'Minimal pre-RLS lookup boundary containing opaque prefixes and tenant identifiers, never credential hashes or scopes.';
COMMENT ON TABLE scim_token_events IS
    'Append-only, reasoned audit history for SCIM credential creation, rotation, revocation, and automatic offboarding.';
COMMENT ON TABLE scim_resource_events IS
    'Append-only provisioning history for canonical directory users and groups.';
