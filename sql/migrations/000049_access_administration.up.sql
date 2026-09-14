-- Migration 049: Enterprise RBAC administration and immutable change history.

ALTER TABLE roles
    ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    ADD COLUMN created_by UUID,
    ADD COLUMN updated_by UUID;

CREATE UNIQUE INDEX uq_roles_org_name_active
    ON roles (organization_id, lower(name))
    WHERE organization_id IS NOT NULL AND deleted_at IS NULL;

-- Earlier schema documentation already defined every tenant-owned role as a
-- custom role, but the column default predated the administration API. Bring
-- legacy tenant roles into that declared invariant before enforcing it.
UPDATE roles
SET is_custom = true
WHERE organization_id IS NOT NULL AND NOT is_system_role AND NOT is_custom;

ALTER TABLE roles
    ADD CONSTRAINT chk_roles_scope_kind CHECK (
        (is_system_role AND organization_id IS NULL AND NOT is_custom)
        OR (NOT is_system_role AND organization_id IS NOT NULL AND is_custom)
    ),
    ADD CONSTRAINT fk_roles_creator_tenant FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT fk_roles_updater_tenant FOREIGN KEY (organization_id, updated_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT;

-- Existing independent foreign keys allow a malicious direct writer to pair a
-- tenant marker with a user from another organization. This composite key
-- closes that gap while retaining global system roles.
ALTER TABLE user_roles
    ADD CONSTRAINT fk_user_roles_user_tenant
        FOREIGN KEY (organization_id, user_id)
        REFERENCES users(organization_id, id) ON DELETE CASCADE,
    ADD CONSTRAINT fk_user_roles_assigner_tenant
        FOREIGN KEY (organization_id, assigned_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM user_roles assignment
        JOIN roles role ON role.id=assignment.role_id
        WHERE role.deleted_at IS NOT NULL
           OR NOT ((role.is_system_role AND role.organization_id IS NULL)
                   OR (role.is_custom AND role.organization_id=assignment.organization_id))
    ) THEN
        RAISE EXCEPTION 'existing user-role assignments contain an invalid tenant or deleted role';
    END IF;
END;
$$;

CREATE FUNCTION validate_user_role_tenant_scope()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roles role
        WHERE role.id = NEW.role_id
          AND role.deleted_at IS NULL
          AND ((role.is_system_role AND role.organization_id IS NULL)
               OR (role.is_custom AND role.organization_id = NEW.organization_id))
    ) THEN
        RAISE EXCEPTION 'role is not available in the assignment tenant'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.assigned_by IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM users actor
        WHERE actor.id = NEW.assigned_by
          AND actor.organization_id = NEW.organization_id
          AND actor.deleted_at IS NULL
    ) THEN
        RAISE EXCEPTION 'role assigner is not an active member of the assignment tenant'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_user_roles_tenant_scope
    BEFORE INSERT OR UPDATE ON user_roles
    FOR EACH ROW EXECUTE FUNCTION validate_user_role_tenant_scope();

-- Permission mappings inherit their tenant boundary from the owning role.
-- System-role grants remain readable by every tenant, while only mappings for
-- an active custom role in the current tenant may be created or removed.
ALTER TABLE role_permissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE role_permissions FORCE ROW LEVEL SECURITY;

CREATE POLICY role_permissions_tenant_select ON role_permissions FOR SELECT
    USING (EXISTS (
        SELECT 1 FROM roles role
        WHERE role.id = role_permissions.role_id
          AND (role.organization_id = get_current_tenant()
               OR (role.organization_id IS NULL AND role.is_system_role))
    ));

CREATE POLICY role_permissions_tenant_insert ON role_permissions FOR INSERT
    WITH CHECK (EXISTS (
        SELECT 1 FROM roles role
        WHERE role.id = role_permissions.role_id
          AND role.organization_id = get_current_tenant()
          AND role.is_custom
          AND NOT role.is_system_role
          AND role.deleted_at IS NULL
    ));

CREATE POLICY role_permissions_tenant_delete ON role_permissions FOR DELETE
    USING (EXISTS (
        SELECT 1 FROM roles role
        WHERE role.id = role_permissions.role_id
          AND role.organization_id = get_current_tenant()
          AND role.is_custom
          AND NOT role.is_system_role
          AND role.deleted_at IS NULL
    ));

CREATE TABLE role_change_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role_id UUID NOT NULL,
    target_user_id UUID,
    event_type VARCHAR(32) NOT NULL CHECK (event_type IN (
        'created','updated','cloned','deleted','assigned','unassigned'
    )),
    actor_user_id UUID NOT NULL,
    role_version BIGINT NOT NULL CHECK (role_version > 0),
    reason VARCHAR(1000),
    before_state JSONB CHECK (before_state IS NULL OR jsonb_typeof(before_state) = 'object'),
    after_state JSONB CHECK (after_state IS NULL OR jsonb_typeof(after_state) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_role_events_actor_tenant FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_role_events_target_tenant FOREIGN KEY (organization_id, target_user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_role_events_reason CHECK (
        reason IS NULL OR (reason=BTRIM(reason) AND length(reason) BETWEEN 3 AND 1000)
    )
);

CREATE FUNCTION validate_role_change_event_scope()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roles role
        WHERE role.id=NEW.role_id
          AND ((role.is_system_role AND role.organization_id IS NULL)
               OR role.organization_id=NEW.organization_id)
    ) THEN
        RAISE EXCEPTION 'role event does not belong to the event tenant'
            USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_role_change_events_tenant_scope
    BEFORE INSERT ON role_change_events
    FOR EACH ROW EXECUTE FUNCTION validate_role_change_event_scope();

CREATE INDEX idx_role_events_role
    ON role_change_events (organization_id, role_id, created_at DESC, id DESC);
CREATE INDEX idx_role_events_target
    ON role_change_events (organization_id, target_user_id, created_at DESC)
    WHERE target_user_id IS NOT NULL;

ALTER TABLE role_change_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE role_change_events FORCE ROW LEVEL SECURITY;

CREATE POLICY role_change_events_tenant_select ON role_change_events FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY role_change_events_tenant_insert ON role_change_events FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());

COMMENT ON COLUMN roles.version IS
    'Monotonic optimistic-concurrency token for custom-role administration.';
COMMENT ON TABLE role_change_events IS
    'Append-only audit history for role definitions and tenant assignments.';
