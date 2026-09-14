-- Migration 047: tenant-isolated enterprise asset inventory.

CREATE FUNCTION asset_tags_are_valid(values_to_check TEXT[])
RETURNS BOOLEAN
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT COALESCE(bool_and(length(btrim(tag)) BETWEEN 1 AND 64), true)
    FROM unnest(values_to_check) AS tag
$$;

CREATE TABLE asset_reference_sequences (
    organization_id UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    next_value BIGINT NOT NULL DEFAULT 1 CHECK (next_value > 0)
);

CREATE TABLE assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    asset_ref VARCHAR(32) NOT NULL,
    name VARCHAR(200) NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    asset_type VARCHAR(32) NOT NULL
        CHECK (asset_type IN ('hardware','software','data','service','network','people','facility')),
    category VARCHAR(100),
    description TEXT CHECK (description IS NULL OR length(description) <= 10000),
    criticality VARCHAR(16) NOT NULL DEFAULT 'medium'
        CHECK (criticality IN ('critical','high','medium','low')),
    owner_user_id UUID,
    location VARCHAR(200),
    ip_address INET,
    classification VARCHAR(20) NOT NULL DEFAULT 'internal'
        CHECK (classification IN ('public','internal','confidential','restricted')),
    processes_personal_data BOOLEAN NOT NULL DEFAULT false,
    -- A foreign key is added by the vendor foundation migration. UUID typing
    -- still prevents malformed identifiers while allowing either module to be
    -- installed first in the canonical sequence.
    linked_vendor_id UUID,
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active','inactive','decommissioned')),
    tags TEXT[] NOT NULL DEFAULT '{}',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    search_vector TSVECTOR GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(asset_ref, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(category, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(description, '')), 'C')
    ) STORED,
    CONSTRAINT uq_assets_org_ref UNIQUE (organization_id, asset_ref),
    CONSTRAINT uq_assets_org_id UNIQUE (organization_id, id),
    CONSTRAINT fk_assets_owner_tenant FOREIGN KEY (organization_id, owner_user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_assets_creator_tenant FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_assets_tags_bounded CHECK (cardinality(tags) <= 50),
    CONSTRAINT chk_assets_tags_values CHECK (asset_tags_are_valid(tags))
);

CREATE TABLE asset_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    asset_id UUID NOT NULL,
    event_type VARCHAR(32) NOT NULL
        CHECK (event_type IN ('created','updated','status_changed','decommissioned','deleted')),
    actor_user_id UUID NOT NULL,
    asset_version BIGINT NOT NULL CHECK (asset_version > 0),
    details JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_asset_events_asset_tenant FOREIGN KEY (organization_id, asset_id)
        REFERENCES assets(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_asset_events_actor_tenant FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT
);

-- Migration 046 intentionally left this reference unbound because assets did
-- not exist yet. The composite key prevents an incident from naming an asset
-- owned by another tenant even if application validation is bypassed.
ALTER TABLE incidents
    ADD CONSTRAINT fk_incidents_related_asset_tenant
    FOREIGN KEY (organization_id, related_asset_id)
    REFERENCES assets(organization_id, id) ON DELETE RESTRICT;

CREATE INDEX idx_assets_org_updated
    ON assets(organization_id, updated_at DESC, id)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_org_type
    ON assets(organization_id, asset_type)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_org_criticality
    ON assets(organization_id, criticality)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_org_owner
    ON assets(organization_id, owner_user_id)
    WHERE deleted_at IS NULL AND owner_user_id IS NOT NULL;
CREATE INDEX idx_assets_org_personal_data
    ON assets(organization_id, processes_personal_data)
    WHERE deleted_at IS NULL AND processes_personal_data;
CREATE INDEX idx_assets_tags ON assets USING GIN(tags);
CREATE INDEX idx_assets_search ON assets USING GIN(search_vector);
CREATE INDEX idx_asset_events_asset
    ON asset_events(organization_id, asset_id, created_at DESC, id);

CREATE TRIGGER trg_assets_updated_at
    BEFORE UPDATE ON assets
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE asset_reference_sequences ENABLE ROW LEVEL SECURITY;
ALTER TABLE asset_reference_sequences FORCE ROW LEVEL SECURITY;
ALTER TABLE assets ENABLE ROW LEVEL SECURITY;
ALTER TABLE assets FORCE ROW LEVEL SECURITY;
ALTER TABLE asset_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE asset_events FORCE ROW LEVEL SECURITY;

CREATE POLICY asset_reference_sequences_tenant_all
    ON asset_reference_sequences
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY assets_tenant_select
    ON assets FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY assets_tenant_insert
    ON assets FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY assets_tenant_update
    ON assets FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY assets_tenant_delete
    ON assets FOR DELETE
    USING (organization_id = get_current_tenant());

CREATE POLICY asset_events_tenant_select
    ON asset_events FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY asset_events_tenant_insert
    ON asset_events FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());

COMMENT ON TABLE assets IS
    'Tenant-owned asset inventory used for risk, control, privacy, vendor, and continuity scoping.';
COMMENT ON COLUMN assets.version IS
    'Monotonic optimistic-concurrency version incremented by API mutations.';
COMMENT ON COLUMN assets.linked_vendor_id IS
    'Optional vendor UUID; the canonical vendor migration adds the deferred foreign key.';
COMMENT ON TABLE asset_events IS
    'Append-only asset lifecycle history. No UPDATE or DELETE RLS policies are defined.';
