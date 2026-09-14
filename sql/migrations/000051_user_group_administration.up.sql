-- Migration 051: enterprise tenant user directory, groups, imports, and history.

ALTER TABLE users
    ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    ADD COLUMN manager_user_id UUID,
    ADD COLUMN employee_id VARCHAR(100),
    ADD COLUMN location VARCHAR(200),
    ADD COLUMN invitation_status VARCHAR(20) NOT NULL DEFAULT 'accepted'
        CHECK (invitation_status IN ('not_required','ready','sent','accepted','expired','revoked')),
    ADD COLUMN invited_at TIMESTAMPTZ,
    ADD COLUMN invitation_expires_at TIMESTAMPTZ,
    ADD COLUMN invited_by UUID,
    ADD COLUMN suspended_at TIMESTAMPTZ,
    ADD COLUMN suspended_by UUID,
    ADD COLUMN suspension_reason VARCHAR(1000),
    ADD COLUMN reactivated_at TIMESTAMPTZ,
    ADD COLUMN deprovisioned_at TIMESTAMPTZ,
    ADD COLUMN deprovisioned_by UUID,
    ADD COLUMN deprovision_reason VARCHAR(1000),
    ADD COLUMN updated_by UUID,
    ADD COLUMN directory_search TSVECTOR GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', COALESCE(email, '')), 'A') ||
        setweight(to_tsvector('simple', COALESCE(first_name, '') || ' ' || COALESCE(last_name, '')), 'A') ||
        setweight(to_tsvector('simple', COALESCE(employee_id, '')), 'B') ||
        setweight(to_tsvector('simple', COALESCE(job_title, '')), 'C') ||
        setweight(to_tsvector('simple', COALESCE(department, '')), 'C') ||
        setweight(to_tsvector('simple', COALESCE(location, '')), 'D')
    ) STORED,
    ADD CONSTRAINT fk_users_manager_tenant FOREIGN KEY (organization_id, manager_user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT fk_users_inviter_tenant FOREIGN KEY (organization_id, invited_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT fk_users_suspender_tenant FOREIGN KEY (organization_id, suspended_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT fk_users_deprovisioner_tenant FOREIGN KEY (organization_id, deprovisioned_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT fk_users_updater_tenant FOREIGN KEY (organization_id, updated_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_users_manager_not_self CHECK (manager_user_id IS NULL OR manager_user_id <> id),
    ADD CONSTRAINT chk_users_directory_text CHECK (
        (employee_id IS NULL OR (employee_id=BTRIM(employee_id) AND length(employee_id) BETWEEN 1 AND 100)) AND
        (location IS NULL OR (location=BTRIM(location) AND length(location) BETWEEN 1 AND 200)) AND
        (suspension_reason IS NULL OR (suspension_reason=BTRIM(suspension_reason) AND length(suspension_reason) BETWEEN 3 AND 1000)) AND
        (deprovision_reason IS NULL OR (deprovision_reason=BTRIM(deprovision_reason) AND length(deprovision_reason) BETWEEN 3 AND 1000))
    ),
    ADD CONSTRAINT chk_users_invitation_window CHECK (
        invitation_expires_at IS NULL OR invited_at IS NULL OR invitation_expires_at > invited_at
    ),
    ADD CONSTRAINT chk_users_deprovision_state CHECK (
        deprovisioned_at IS NULL OR (deleted_at IS NOT NULL AND status='inactive' AND deprovisioned_by IS NOT NULL
            AND deprovision_reason IS NOT NULL)
    );

CREATE UNIQUE INDEX uq_users_org_email_ci
    ON users (organization_id, lower(email));
CREATE UNIQUE INDEX uq_users_org_employee_active
    ON users (organization_id, lower(employee_id))
    WHERE employee_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_users_directory_search ON users USING GIN (directory_search);
CREATE INDEX idx_users_org_manager ON users (organization_id, manager_user_id)
    WHERE manager_user_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_users_org_location ON users (organization_id, location)
    WHERE location IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_users_org_directory_updated ON users (organization_id, updated_at DESC, id)
    WHERE deleted_at IS NULL;

CREATE TABLE directory_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(160) NOT NULL,
    slug VARCHAR(120) NOT NULL,
    description VARCHAR(2000),
    group_type VARCHAR(16) NOT NULL DEFAULT 'static'
        CHECK (group_type IN ('static','dynamic')),
    membership_rule JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(membership_rule)='object' AND pg_column_size(membership_rule) <= 16384),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by UUID NOT NULL,
    updated_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_directory_groups_org_id UNIQUE (organization_id, id),
    CONSTRAINT fk_directory_groups_creator FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_directory_groups_updater FOREIGN KEY (organization_id, updated_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_directory_group_name CHECK (name=BTRIM(name) AND length(name) BETWEEN 2 AND 160),
    CONSTRAINT chk_directory_group_slug CHECK (slug ~ '^[a-z][a-z0-9_-]{1,119}$'),
    CONSTRAINT chk_directory_group_description CHECK (
        description IS NULL OR (description=BTRIM(description) AND length(description) BETWEEN 3 AND 2000)
    ),
    CONSTRAINT chk_directory_group_rule CHECK (
        (group_type='static' AND membership_rule='{}'::jsonb)
        OR (group_type='dynamic' AND membership_rule<>'{}'::jsonb)
    )
);

CREATE UNIQUE INDEX uq_directory_groups_name_active
    ON directory_groups (organization_id, lower(name)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_directory_groups_slug_active
    ON directory_groups (organization_id, lower(slug)) WHERE deleted_at IS NULL;
CREATE INDEX idx_directory_groups_org_updated
    ON directory_groups (organization_id, updated_at DESC, id) WHERE deleted_at IS NULL;

CREATE TRIGGER trg_directory_groups_updated_at
    BEFORE UPDATE ON directory_groups
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE directory_group_memberships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    group_id UUID NOT NULL,
    user_id UUID NOT NULL,
    added_by UUID NOT NULL,
    add_reason VARCHAR(1000) NOT NULL,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    removed_by UUID,
    remove_reason VARCHAR(1000),
    removed_at TIMESTAMPTZ,
    CONSTRAINT fk_directory_membership_group FOREIGN KEY (organization_id, group_id)
        REFERENCES directory_groups(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_directory_membership_user FOREIGN KEY (organization_id, user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_directory_membership_adder FOREIGN KEY (organization_id, added_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_directory_membership_remover FOREIGN KEY (organization_id, removed_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_directory_membership_add_reason CHECK (
        add_reason=BTRIM(add_reason) AND length(add_reason) BETWEEN 3 AND 1000
    ),
    CONSTRAINT chk_directory_membership_remove CHECK (
        (removed_at IS NULL AND removed_by IS NULL AND remove_reason IS NULL)
        OR (removed_at IS NOT NULL AND removed_by IS NOT NULL AND remove_reason=BTRIM(remove_reason)
            AND length(remove_reason) BETWEEN 3 AND 1000)
    )
);

CREATE UNIQUE INDEX uq_directory_group_members_active
    ON directory_group_memberships (organization_id, group_id, user_id) WHERE removed_at IS NULL;
CREATE INDEX idx_directory_group_members_user
    ON directory_group_memberships (organization_id, user_id, group_id) WHERE removed_at IS NULL;
CREATE INDEX idx_directory_group_members_group
    ON directory_group_memberships (organization_id, group_id, added_at DESC) WHERE removed_at IS NULL;

CREATE TABLE directory_imports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    idempotency_key VARCHAR(160) NOT NULL,
    content_sha256 CHAR(64) NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    row_count INTEGER NOT NULL CHECK (row_count BETWEEN 1 AND 500),
    created_count INTEGER NOT NULL CHECK (created_count BETWEEN 0 AND 500),
    updated_count INTEGER NOT NULL CHECK (updated_count BETWEEN 0 AND 500),
    skipped_count INTEGER NOT NULL CHECK (skipped_count BETWEEN 0 AND 500),
    result JSONB NOT NULL CHECK (jsonb_typeof(result)='object' AND pg_column_size(result) <= 262144),
    created_by UUID NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_directory_import_idempotency UNIQUE (organization_id, idempotency_key),
    CONSTRAINT uq_directory_import_org_id UNIQUE (organization_id, id),
    CONSTRAINT fk_directory_import_creator FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_directory_import_counts CHECK (created_count + updated_count + skipped_count = row_count),
    CONSTRAINT chk_directory_import_key CHECK (
        idempotency_key=BTRIM(idempotency_key) AND length(idempotency_key) BETWEEN 8 AND 160
    )
);

CREATE INDEX idx_directory_imports_org_applied
    ON directory_imports (organization_id, applied_at DESC, id DESC);

CREATE TABLE directory_change_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entity_type VARCHAR(16) NOT NULL CHECK (entity_type IN ('user','group','import')),
    entity_id UUID NOT NULL,
    target_user_id UUID,
    event_type VARCHAR(40) NOT NULL CHECK (event_type IN (
        'user_created','user_updated','user_suspended','user_reactivated','user_deprovisioned',
        'ownership_transferred','group_created','group_updated','group_deleted',
        'member_added','member_removed','members_bulk_changed','import_applied'
    )),
    actor_user_id UUID NOT NULL,
    entity_version BIGINT NOT NULL CHECK (entity_version > 0),
    reason VARCHAR(1000) NOT NULL,
    before_state JSONB CHECK (before_state IS NULL OR jsonb_typeof(before_state)='object'),
    after_state JSONB CHECK (after_state IS NULL OR jsonb_typeof(after_state)='object'),
    request_id VARCHAR(160),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_directory_event_actor FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_directory_event_target FOREIGN KEY (organization_id, target_user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_directory_event_reason CHECK (
        reason=BTRIM(reason) AND length(reason) BETWEEN 3 AND 1000
    )
);

CREATE INDEX idx_directory_events_entity
    ON directory_change_events (organization_id, entity_type, entity_id, created_at DESC, id DESC);
CREATE INDEX idx_directory_events_target
    ON directory_change_events (organization_id, target_user_id, created_at DESC, id DESC)
    WHERE target_user_id IS NOT NULL;

CREATE FUNCTION validate_directory_change_event_scope()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.entity_type='user' AND NOT EXISTS (
        SELECT 1 FROM users value WHERE value.organization_id=NEW.organization_id AND value.id=NEW.entity_id
    ) THEN
        RAISE EXCEPTION 'directory user event entity does not belong to tenant' USING ERRCODE='23514';
    ELSIF NEW.entity_type='group' AND NOT EXISTS (
        SELECT 1 FROM directory_groups value WHERE value.organization_id=NEW.organization_id AND value.id=NEW.entity_id
    ) THEN
        RAISE EXCEPTION 'directory group event entity does not belong to tenant' USING ERRCODE='23514';
    ELSIF NEW.entity_type='import' AND NOT EXISTS (
        SELECT 1 FROM directory_imports value WHERE value.organization_id=NEW.organization_id AND value.id=NEW.entity_id
    ) THEN
        RAISE EXCEPTION 'directory import event entity does not belong to tenant' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_directory_change_events_scope
    BEFORE INSERT ON directory_change_events
    FOR EACH ROW EXECUTE FUNCTION validate_directory_change_event_scope();

CREATE FUNCTION prevent_directory_immutable_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION '% is append-only: % is not permitted', TG_TABLE_NAME, TG_OP
        USING ERRCODE='55000';
END;
$$;

CREATE TRIGGER trg_directory_change_events_immutable
    BEFORE UPDATE OR DELETE ON directory_change_events
    FOR EACH ROW EXECUTE FUNCTION prevent_directory_immutable_mutation();
CREATE TRIGGER trg_directory_imports_immutable
    BEFORE UPDATE OR DELETE ON directory_imports
    FOR EACH ROW EXECUTE FUNCTION prevent_directory_immutable_mutation();

ALTER TABLE directory_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE directory_groups FORCE ROW LEVEL SECURITY;
ALTER TABLE directory_group_memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE directory_group_memberships FORCE ROW LEVEL SECURITY;
ALTER TABLE directory_imports ENABLE ROW LEVEL SECURITY;
ALTER TABLE directory_imports FORCE ROW LEVEL SECURITY;
ALTER TABLE directory_change_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE directory_change_events FORCE ROW LEVEL SECURITY;

CREATE POLICY directory_groups_select ON directory_groups FOR SELECT
    USING (organization_id=get_current_tenant());
CREATE POLICY directory_groups_insert ON directory_groups FOR INSERT
    WITH CHECK (organization_id=get_current_tenant());
CREATE POLICY directory_groups_update ON directory_groups FOR UPDATE
    USING (organization_id=get_current_tenant()) WITH CHECK (organization_id=get_current_tenant());
CREATE POLICY directory_groups_delete ON directory_groups FOR DELETE
    USING (organization_id=get_current_tenant());

CREATE POLICY directory_memberships_select ON directory_group_memberships FOR SELECT
    USING (organization_id=get_current_tenant());
CREATE POLICY directory_memberships_insert ON directory_group_memberships FOR INSERT
    WITH CHECK (organization_id=get_current_tenant());
CREATE POLICY directory_memberships_update ON directory_group_memberships FOR UPDATE
    USING (organization_id=get_current_tenant()) WITH CHECK (organization_id=get_current_tenant());
CREATE POLICY directory_memberships_delete ON directory_group_memberships FOR DELETE
    USING (organization_id=get_current_tenant());

CREATE POLICY directory_imports_select ON directory_imports FOR SELECT
    USING (organization_id=get_current_tenant());
CREATE POLICY directory_imports_insert ON directory_imports FOR INSERT
    WITH CHECK (organization_id=get_current_tenant());

CREATE POLICY directory_events_select ON directory_change_events FOR SELECT
    USING (organization_id=get_current_tenant());
CREATE POLICY directory_events_insert ON directory_change_events FOR INSERT
    WITH CHECK (organization_id=get_current_tenant());

COMMENT ON TABLE directory_groups IS 'Tenant-owned static or safely evaluated dynamic user groups.';
COMMENT ON TABLE directory_group_memberships IS 'Reasoned static-group membership history; active rows have removed_at IS NULL.';
COMMENT ON TABLE directory_change_events IS 'Append-only reasoned change history for user, group, membership, ownership-transfer, and import operations.';
COMMENT ON TABLE directory_imports IS 'Immutable results for bounded, content-bound idempotent CSV user imports.';
