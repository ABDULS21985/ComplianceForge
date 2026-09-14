-- Migration 041: integration lifecycle and API-key tenant resolution

-- Integration status is an operational state, not a deletion marker. Keep the
-- enum constrained and record deletion separately so history remains auditable.
ALTER TABLE integrations
    ADD COLUMN deleted_at TIMESTAMPTZ;

ALTER TABLE integrations
    ADD CONSTRAINT chk_integrations_sync_frequency
        CHECK (sync_frequency_minutes >= 0) NOT VALID,
    ADD CONSTRAINT chk_integrations_error_count
        CHECK (error_count >= 0) NOT VALID,
    ADD CONSTRAINT chk_integrations_deleted_state
        CHECK (deleted_at IS NULL OR status = 'inactive') NOT VALID;

ALTER TABLE integrations VALIDATE CONSTRAINT chk_integrations_sync_frequency;
ALTER TABLE integrations VALIDATE CONSTRAINT chk_integrations_error_count;
ALTER TABLE integrations VALIDATE CONSTRAINT chk_integrations_deleted_state;

ALTER TABLE api_keys
    ADD CONSTRAINT chk_api_keys_rate_limit
        CHECK (rate_limit_per_minute > 0) NOT VALID,
    ADD CONSTRAINT chk_api_keys_hash_format
        CHECK (key_hash ~ '^[0-9a-f]{64}$') NOT VALID,
    ADD CONSTRAINT chk_api_keys_expiry
        CHECK (expires_at IS NULL OR expires_at > created_at) NOT VALID;

ALTER TABLE api_keys VALIDATE CONSTRAINT chk_api_keys_rate_limit;
ALTER TABLE api_keys VALIDATE CONSTRAINT chk_api_keys_hash_format;
ALTER TABLE api_keys VALIDATE CONSTRAINT chk_api_keys_expiry;

CREATE UNIQUE INDEX uq_api_keys_hash ON api_keys(key_hash);

CREATE INDEX idx_integrations_org_active
    ON integrations(organization_id, name)
    WHERE deleted_at IS NULL;

COMMENT ON COLUMN integrations.deleted_at IS
    'Soft-deletion timestamp. Deleted integrations retain immutable sync history but are excluded from active application queries.';

-- API-key authentication happens before a tenant is known, while api_keys is
-- correctly protected by tenant RLS. This minimal index exposes only the
-- information required to establish tenant context; hashes, scopes, expiry,
-- and activity state remain in the RLS-protected table.
CREATE TABLE api_key_tenant_index (
    key_prefix       VARCHAR(10) PRIMARY KEY,
    key_id           UUID NOT NULL UNIQUE REFERENCES api_keys(id) ON DELETE CASCADE,
    organization_id  UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE api_key_tenant_index IS
    'Minimal non-tenant lookup boundary used to resolve API-key requests before establishing RLS context. Contains no credential hash or permissions.';

INSERT INTO api_key_tenant_index (key_prefix, key_id, organization_id)
SELECT key_prefix, id, organization_id
FROM api_keys;

CREATE FUNCTION maintain_api_key_tenant_index()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        DELETE FROM public.api_key_tenant_index WHERE key_id = OLD.id;
        RETURN OLD;
    END IF;

    INSERT INTO public.api_key_tenant_index (key_prefix, key_id, organization_id)
    VALUES (NEW.key_prefix, NEW.id, NEW.organization_id)
    ON CONFLICT (key_id) DO UPDATE
    SET key_prefix = EXCLUDED.key_prefix,
        organization_id = EXCLUDED.organization_id;

    RETURN NEW;
END;
$$;

REVOKE ALL ON FUNCTION maintain_api_key_tenant_index() FROM PUBLIC;

CREATE TRIGGER trg_api_keys_tenant_index
    AFTER INSERT OR UPDATE OF key_prefix, organization_id OR DELETE ON api_keys
    FOR EACH ROW EXECUTE FUNCTION maintain_api_key_tenant_index();

-- Callers receive a row only for an exact high-entropy prefix. The function
-- avoids granting direct enumeration access to the global index table.
CREATE FUNCTION resolve_api_key_tenant(requested_prefix TEXT)
RETURNS TABLE (key_id UUID, organization_id UUID)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
    SELECT idx.key_id, idx.organization_id
    FROM public.api_key_tenant_index AS idx
    WHERE idx.key_prefix = requested_prefix
$$;

REVOKE ALL ON FUNCTION resolve_api_key_tenant(TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION resolve_api_key_tenant(TEXT) TO PUBLIC;
