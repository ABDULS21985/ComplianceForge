-- Migration 054: tenant-safe policy constraints, object grants, field controls,
-- and immutable access-decision evidence.
--
-- RBAC remains the upper authorization bound. These tables can only constrain
-- an RBAC allow; an object grant never creates a permission on its own.

-- Preserve legacy condition documents for administrator review. Migration 024
-- stored a different, untyped object grammar which cannot always be translated
-- without changing its meaning. Empty legacy documents are safely normalized;
-- non-empty legacy policies are retained but disabled until explicitly replaced
-- through the typed API.
ALTER TABLE access_policies
    ADD COLUMN version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN updated_by UUID,
    ADD COLUMN deleted_at TIMESTAMPTZ,
    ADD COLUMN legacy_definition JSONB;

UPDATE access_policies
SET legacy_definition = jsonb_build_object(
        'subject_conditions', subject_conditions,
        'resource_conditions', resource_conditions,
        'environment_conditions', environment_conditions,
        'resource_type', resource_type,
        'actions', to_jsonb(actions)
    ),
    is_active = FALSE
WHERE (jsonb_typeof(subject_conditions) <> 'array' AND subject_conditions <> '{}'::jsonb)
   OR (resource_conditions IS NOT NULL AND jsonb_typeof(resource_conditions) <> 'array'
       AND resource_conditions <> '{}'::jsonb)
   OR (environment_conditions IS NOT NULL AND jsonb_typeof(environment_conditions) <> 'array'
       AND environment_conditions <> '{}'::jsonb);

UPDATE access_policies
SET subject_conditions = CASE
        WHEN jsonb_typeof(subject_conditions) = 'array' THEN subject_conditions
        ELSE '[]'::jsonb
    END,
    resource_conditions = CASE
        WHEN jsonb_typeof(resource_conditions) = 'array' THEN resource_conditions
        ELSE '[]'::jsonb
    END,
    environment_conditions = CASE
        WHEN jsonb_typeof(environment_conditions) = 'array' THEN environment_conditions
        ELSE '[]'::jsonb
    END,
    resource_type = CASE lower(resource_type)
        WHEN 'risk' THEN 'risks'
        WHEN 'policy' THEN 'policies'
        WHEN 'control' THEN 'controls'
        WHEN 'control_implementation' THEN 'controls'
        WHEN 'framework' THEN 'frameworks'
        WHEN 'audit' THEN 'audits'
        WHEN 'finding' THEN 'findings'
        WHEN 'incident' THEN 'incidents'
        WHEN 'asset' THEN 'assets'
        WHEN 'vendor' THEN 'vendors'
        WHEN 'report' THEN 'reports'
        WHEN 'user' THEN 'users'
        WHEN 'organization' THEN 'organizations'
        ELSE lower(resource_type)
    END,
    actions = ARRAY(SELECT lower(BTRIM(value)) FROM unnest(actions) AS value ORDER BY lower(BTRIM(value))),
    updated_by = created_by;

ALTER TABLE access_policies
    ALTER COLUMN resource_conditions SET DEFAULT '[]'::jsonb,
    ALTER COLUMN resource_conditions SET NOT NULL,
    ALTER COLUMN environment_conditions SET DEFAULT '[]'::jsonb,
    ALTER COLUMN environment_conditions SET NOT NULL,
    DROP CONSTRAINT IF EXISTS access_policies_created_by_fkey,
    ADD CONSTRAINT uq_access_policies_org_id UNIQUE (organization_id, id),
    ADD CONSTRAINT fk_access_policies_creator_tenant FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT fk_access_policies_updater_tenant FOREIGN KEY (organization_id, updated_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_access_policies_version CHECK (version > 0),
    ADD CONSTRAINT chk_access_policies_name CHECK (
        name = BTRIM(name) AND length(name) BETWEEN 2 AND 200
    ),
    ADD CONSTRAINT chk_access_policies_priority CHECK (priority BETWEEN 0 AND 10000),
    ADD CONSTRAINT chk_access_policies_resource_type CHECK (
        resource_type = '*' OR resource_type ~ '^[a-z][a-z0-9_-]{0,63}$'
    ),
    ADD CONSTRAINT chk_access_policies_actions CHECK (
        cardinality(actions) BETWEEN 1 AND 16
        AND actions <@ ARRAY['*','create','read','update','delete','approve','assign','export','configure']::TEXT[]
        AND (NOT ('*'=ANY(actions)) OR cardinality(actions)=1)
    ),
    ADD CONSTRAINT chk_access_policies_condition_shapes CHECK (
        jsonb_typeof(subject_conditions)='array'
        AND jsonb_typeof(resource_conditions)='array'
        AND jsonb_typeof(environment_conditions)='array'
        AND pg_column_size(subject_conditions) <= 16384
        AND pg_column_size(resource_conditions) <= 16384
        AND pg_column_size(environment_conditions) <= 16384
    ),
    ADD CONSTRAINT chk_access_policies_valid_window CHECK (
        valid_until IS NULL OR valid_from IS NULL OR valid_until > valid_from
    );

CREATE UNIQUE INDEX uq_access_policies_org_name_active
    ON access_policies (organization_id, lower(name)) WHERE deleted_at IS NULL;
CREATE INDEX idx_access_policies_evaluation
    ON access_policies (organization_id, resource_type, priority, id)
    WHERE is_active AND deleted_at IS NULL;

-- Reject cross-tenant policy links before making them structurally impossible.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM access_policy_assignments assignment
        JOIN access_policies policy ON policy.id=assignment.access_policy_id
        WHERE policy.organization_id <> assignment.organization_id
    ) THEN
        RAISE EXCEPTION 'cross-tenant access policy assignments require manual remediation';
    END IF;
    IF EXISTS (
        SELECT 1 FROM access_policy_assignments
        WHERE (assignee_type='all_users') <> (assignee_id IS NULL)
           OR created_by IS NULL
           OR (valid_from IS NOT NULL AND valid_until IS NOT NULL AND valid_until <= valid_from)
    ) THEN
        RAISE EXCEPTION 'invalid legacy access policy assignments require manual remediation';
    END IF;
END;
$$;

ALTER TABLE access_policy_assignments
    ALTER COLUMN created_by SET NOT NULL,
    DROP CONSTRAINT IF EXISTS access_policy_assignments_access_policy_id_fkey,
    DROP CONSTRAINT IF EXISTS access_policy_assignments_created_by_fkey,
    ADD CONSTRAINT fk_access_policy_assignments_policy_tenant
        FOREIGN KEY (organization_id, access_policy_id)
        REFERENCES access_policies(organization_id, id) ON DELETE CASCADE,
    ADD CONSTRAINT fk_access_policy_assignments_creator_tenant
        FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_access_policy_assignments_subject CHECK (
        (assignee_type='all_users' AND assignee_id IS NULL)
        OR (assignee_type IN ('user','role','group') AND assignee_id IS NOT NULL)
    ),
    ADD CONSTRAINT chk_access_policy_assignments_window CHECK (
        valid_until IS NULL OR valid_from IS NULL OR valid_until > valid_from
    ),
    ADD CONSTRAINT uq_access_policy_assignments_scope
        UNIQUE NULLS NOT DISTINCT (
            organization_id, access_policy_id, assignee_type, assignee_id, valid_from, valid_until
        );
CREATE INDEX idx_access_policy_assignments_evaluation
    ON access_policy_assignments (organization_id, assignee_type, assignee_id, access_policy_id);

-- Repair migration-024 field naming and add explicit classification/masking.
ALTER TABLE field_level_permissions
    RENAME COLUMN field_name TO field_path;
ALTER TABLE field_level_permissions
    RENAME COLUMN permission TO visibility;
ALTER TABLE field_level_permissions
    ADD COLUMN classification VARCHAR(32) NOT NULL DEFAULT 'internal',
    ADD COLUMN mask_strategy VARCHAR(20),
    ADD COLUMN version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN created_by UUID,
    ADD COLUMN updated_by UUID,
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE field_level_permissions field
SET mask_strategy = CASE
        WHEN visibility='masked' AND mask_pattern IS NOT NULL THEN 'custom'
        WHEN visibility='masked' THEN 'redact'
        ELSE NULL
    END,
    mask_pattern = CASE WHEN visibility='masked' THEN mask_pattern ELSE NULL END,
    created_by = policy.created_by,
    updated_by = COALESCE(policy.updated_by, policy.created_by)
FROM access_policies policy
WHERE policy.organization_id=field.organization_id AND policy.id=field.access_policy_id;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM field_level_permissions WHERE created_by IS NULL OR updated_by IS NULL) THEN
        RAISE EXCEPTION 'legacy field permissions without an accountable actor require manual remediation';
    END IF;
END;
$$;

ALTER TABLE field_level_permissions
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN updated_by SET NOT NULL,
    DROP CONSTRAINT IF EXISTS field_level_permissions_access_policy_id_fkey,
    DROP CONSTRAINT IF EXISTS field_level_permissions_permission_check,
    ADD CONSTRAINT fk_field_permissions_policy_tenant
        FOREIGN KEY (organization_id, access_policy_id)
        REFERENCES access_policies(organization_id, id) ON DELETE CASCADE,
    ADD CONSTRAINT fk_field_permissions_creator_tenant FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT fk_field_permissions_updater_tenant FOREIGN KEY (organization_id, updated_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_field_permissions_version CHECK (version > 0),
    ADD CONSTRAINT chk_field_permissions_resource CHECK (
        resource_type ~ '^[a-z][a-z0-9_-]{0,63}$'
    ),
    ADD CONSTRAINT chk_field_permissions_path CHECK (
        field_path ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){0,7}$' AND length(field_path) <= 200
    ),
    ADD CONSTRAINT chk_field_permissions_classification CHECK (
        classification IN ('public','internal','confidential','restricted','personal','financial','legal','security_sensitive')
    ),
    ADD CONSTRAINT chk_field_permissions_visibility CHECK (
        visibility IN ('visible','masked','hidden')
    ),
    ADD CONSTRAINT chk_field_permissions_mask CHECK (
        (visibility IN ('visible','hidden') AND mask_strategy IS NULL AND mask_pattern IS NULL)
        OR (visibility='masked' AND mask_strategy IN ('redact','last4','email') AND mask_pattern IS NULL)
        OR (visibility='masked' AND mask_strategy='custom' AND mask_pattern IS NOT NULL
            AND length(mask_pattern) BETWEEN 1 AND 50)
    ),
    ADD CONSTRAINT uq_field_permissions_policy_path
        UNIQUE (organization_id, access_policy_id, resource_type, field_path);
CREATE TRIGGER trg_field_level_permissions_updated_at
    BEFORE UPDATE ON field_level_permissions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE INDEX idx_field_permissions_evaluation
    ON field_level_permissions (organization_id, resource_type, access_policy_id, field_path);

-- Evolve the canonical object-grant table; do not create a parallel authority.
ALTER TABLE user_entity_permissions
    ADD COLUMN actions TEXT[],
    ADD COLUMN status VARCHAR(16),
    ADD COLUMN approved_by UUID,
    ADD COLUMN approved_at TIMESTAMPTZ,
    ADD COLUMN valid_from TIMESTAMPTZ,
    ADD COLUMN allow_download BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN require_watermark BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN watermark_text VARCHAR(200),
    ADD COLUMN grant_reason VARCHAR(1000),
    ADD COLUMN decision_reason VARCHAR(1000),
    ADD COLUMN revoked_at TIMESTAMPTZ,
    ADD COLUMN revoked_by UUID,
    ADD COLUMN revocation_reason VARCHAR(1000),
    ADD COLUMN version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE user_entity_permissions
SET entity_type = CASE lower(entity_type)
        WHEN 'risk' THEN 'risks'
        WHEN 'policy' THEN 'policies'
        WHEN 'control' THEN 'controls'
        WHEN 'control_implementation' THEN 'controls'
        WHEN 'framework' THEN 'frameworks'
        WHEN 'audit' THEN 'audits'
        WHEN 'finding' THEN 'findings'
        WHEN 'incident' THEN 'incidents'
        WHEN 'asset' THEN 'assets'
        WHEN 'vendor' THEN 'vendors'
        WHEN 'report' THEN 'reports'
        WHEN 'user' THEN 'users'
        WHEN 'organization' THEN 'organizations'
        ELSE lower(entity_type)
    END,
    actions = CASE permission_level
        WHEN 'viewer' THEN ARRAY['read']::TEXT[]
        WHEN 'editor' THEN ARRAY['read','update']::TEXT[]
        WHEN 'approver' THEN ARRAY['approve','read']::TEXT[]
        ELSE ARRAY['*']::TEXT[]
    END,
    status = CASE
        WHEN granted_by IS NOT NULL AND granted_by <> user_id THEN 'approved'
        ELSE 'revoked'
    END,
    approved_by = CASE WHEN granted_by IS NOT NULL AND granted_by <> user_id THEN granted_by END,
    approved_at = CASE WHEN granted_by IS NOT NULL AND granted_by <> user_id THEN created_at END,
    valid_from = created_at,
    expires_at = COALESCE(expires_at, 'infinity'::timestamptz),
    grant_reason = 'Migrated legacy object permission',
    revoked_at = CASE WHEN granted_by IS NULL OR granted_by=user_id THEN created_at END,
    revocation_reason = CASE
        WHEN granted_by IS NULL OR granted_by=user_id THEN 'Legacy permission lacked independent sponsorship'
    END;

ALTER TABLE user_entity_permissions
    ALTER COLUMN actions SET DEFAULT ARRAY['read']::TEXT[],
    ALTER COLUMN actions SET NOT NULL,
    ALTER COLUMN status SET DEFAULT 'pending',
    ALTER COLUMN status SET NOT NULL,
    ALTER COLUMN valid_from SET DEFAULT NOW(),
    ALTER COLUMN valid_from SET NOT NULL,
    ALTER COLUMN expires_at SET NOT NULL,
    ALTER COLUMN grant_reason SET NOT NULL,
    DROP CONSTRAINT IF EXISTS uq_user_entity_perm,
    DROP CONSTRAINT IF EXISTS user_entity_permissions_user_id_fkey,
    DROP CONSTRAINT IF EXISTS user_entity_permissions_granted_by_fkey,
    ADD CONSTRAINT uq_user_entity_permissions_org_id UNIQUE (organization_id, id),
    ADD CONSTRAINT fk_entity_permissions_subject_tenant FOREIGN KEY (organization_id, user_id)
        REFERENCES users(organization_id, id) ON DELETE CASCADE,
    ADD CONSTRAINT fk_entity_permissions_sponsor_tenant FOREIGN KEY (organization_id, granted_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT fk_entity_permissions_approver_tenant FOREIGN KEY (organization_id, approved_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT fk_entity_permissions_revoker_tenant FOREIGN KEY (organization_id, revoked_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_entity_permissions_type CHECK (
        entity_type ~ '^[a-z][a-z0-9_-]{0,63}$'
    ),
    ADD CONSTRAINT chk_entity_permissions_actions CHECK (
        cardinality(actions) BETWEEN 1 AND 16
        AND actions <@ ARRAY['*','create','read','update','delete','approve','assign','export','configure']::TEXT[]
        AND (NOT ('*'=ANY(actions)) OR cardinality(actions)=1)
    ),
    ADD CONSTRAINT chk_entity_permissions_status CHECK (
        status IN ('pending','approved','rejected','revoked')
    ),
    ADD CONSTRAINT chk_entity_permissions_window CHECK (expires_at > valid_from),
    ADD CONSTRAINT chk_entity_permissions_sponsorship CHECK (
        status IN ('rejected','revoked')
        OR (granted_by IS NOT NULL AND granted_by <> user_id)
    ),
    ADD CONSTRAINT chk_entity_permissions_decision CHECK (
        (status='pending' AND approved_by IS NULL AND approved_at IS NULL AND decision_reason IS NULL)
        OR (status='approved' AND approved_by IS NOT NULL AND approved_by <> user_id
            AND approved_at IS NOT NULL)
        OR (status='rejected' AND approved_by IS NULL AND approved_at IS NULL
            AND decision_reason IS NOT NULL)
        OR (status='revoked' AND revoked_at IS NOT NULL AND revocation_reason IS NOT NULL)
    ),
    ADD CONSTRAINT chk_entity_permissions_watermark CHECK (
        (NOT require_watermark AND watermark_text IS NULL)
        OR (require_watermark AND allow_download AND watermark_text IS NOT NULL
            AND watermark_text=BTRIM(watermark_text) AND length(watermark_text) BETWEEN 1 AND 200)
    ),
    ADD CONSTRAINT chk_entity_permissions_reasons CHECK (
        grant_reason=BTRIM(grant_reason) AND length(grant_reason) BETWEEN 3 AND 1000
        AND (decision_reason IS NULL OR (decision_reason=BTRIM(decision_reason)
            AND length(decision_reason) BETWEEN 3 AND 1000))
        AND (revocation_reason IS NULL OR (revocation_reason=BTRIM(revocation_reason)
            AND length(revocation_reason) BETWEEN 3 AND 1000))
    ),
    ADD CONSTRAINT chk_entity_permissions_version CHECK (version > 0);

CREATE UNIQUE INDEX uq_entity_permissions_active_grant
    ON user_entity_permissions (organization_id, user_id, entity_type, entity_id)
    WHERE status IN ('pending','approved');
CREATE INDEX idx_entity_permissions_evaluation
    ON user_entity_permissions (organization_id, user_id, entity_type, entity_id, expires_at)
    WHERE status='approved';

-- Replace the table-specific legacy trigger with one reusable immutable-evidence
-- guard before extending the decision record.
DROP TRIGGER IF EXISTS trg_access_audit_no_update ON access_audit_log;
DROP TRIGGER IF EXISTS trg_access_audit_no_delete ON access_audit_log;

ALTER TABLE access_audit_log
    ADD COLUMN rbac_allowed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN constraint_outcome VARCHAR(24) NOT NULL DEFAULT 'not_applicable',
    ADD COLUMN reason_code VARCHAR(100) NOT NULL DEFAULT 'legacy_decision',
    ADD COLUMN matched_policy_ids UUID[] NOT NULL DEFAULT '{}'::UUID[],
    ADD COLUMN matched_object_grant_id UUID,
    ADD COLUMN request_id VARCHAR(64);

UPDATE access_audit_log
SET rbac_allowed = (decision='allow'),
    constraint_outcome = CASE WHEN decision='allow' THEN 'allow' ELSE 'deny' END,
    matched_policy_ids = CASE
        WHEN matched_policy_id IS NULL THEN '{}'::UUID[] ELSE ARRAY[matched_policy_id]
    END;

ALTER TABLE access_audit_log
    DROP CONSTRAINT IF EXISTS access_audit_log_user_id_fkey,
    DROP CONSTRAINT IF EXISTS access_audit_log_matched_policy_id_fkey,
    ADD CONSTRAINT fk_access_audit_subject_tenant FOREIGN KEY (organization_id, user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT fk_access_audit_policy_tenant FOREIGN KEY (organization_id, matched_policy_id)
        REFERENCES access_policies(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT fk_access_audit_object_grant_tenant FOREIGN KEY (organization_id, matched_object_grant_id)
        REFERENCES user_entity_permissions(organization_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_access_audit_constraint_outcome CHECK (
        constraint_outcome IN ('not_applicable','allow','deny')
    ),
    ADD CONSTRAINT chk_access_audit_reason_code CHECK (
        reason_code ~ '^[a-z][a-z0-9_]{1,99}$'
    ),
    ADD CONSTRAINT chk_access_audit_request_id CHECK (
        request_id IS NULL OR (request_id=BTRIM(request_id) AND length(request_id) BETWEEN 1 AND 64)
    );
CREATE INDEX idx_access_audit_request ON access_audit_log (organization_id, request_id)
    WHERE request_id IS NOT NULL;

CREATE OR REPLACE FUNCTION prevent_policy_access_evidence_modification()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION '% is immutable; UPDATE and DELETE are not permitted', TG_TABLE_NAME;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_access_audit_no_update
    BEFORE UPDATE ON access_audit_log FOR EACH ROW
    EXECUTE FUNCTION prevent_policy_access_evidence_modification();
CREATE TRIGGER trg_access_audit_no_delete
    BEFORE DELETE ON access_audit_log FOR EACH ROW
    EXECUTE FUNCTION prevent_policy_access_evidence_modification();

CREATE TABLE access_policy_change_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    access_policy_id UUID,
    entity_type VARCHAR(32) NOT NULL,
    entity_id UUID NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    actor_user_id UUID NOT NULL,
    request_id VARCHAR(64) NOT NULL,
    reason VARCHAR(1000) NOT NULL,
    before_state JSONB,
    after_state JSONB,
    evidence_sha256 CHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_access_policy_events_policy_tenant FOREIGN KEY (organization_id, access_policy_id)
        REFERENCES access_policies(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_access_policy_events_actor_tenant FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_access_policy_events_entity CHECK (entity_type ~ '^[a-z][a-z0-9_]{1,31}$'),
    CONSTRAINT chk_access_policy_events_type CHECK (event_type ~ '^[a-z][a-z0-9_]{1,63}$'),
    CONSTRAINT chk_access_policy_events_request CHECK (
        request_id=BTRIM(request_id) AND length(request_id) BETWEEN 1 AND 64
    ),
    CONSTRAINT chk_access_policy_events_reason CHECK (
        reason=BTRIM(reason) AND length(reason) BETWEEN 3 AND 1000
    ),
    CONSTRAINT chk_access_policy_events_state CHECK (
        (before_state IS NULL OR pg_column_size(before_state) <= 65536)
        AND (after_state IS NULL OR pg_column_size(after_state) <= 65536)
    ),
    CONSTRAINT chk_access_policy_events_hash CHECK (evidence_sha256 ~ '^[0-9a-f]{64}$')
);
CREATE INDEX idx_access_policy_events_policy
    ON access_policy_change_events (organization_id, access_policy_id, created_at DESC, id);
CREATE INDEX idx_access_policy_events_entity
    ON access_policy_change_events (organization_id, entity_type, entity_id, created_at DESC, id);

ALTER TABLE access_policy_change_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE access_policy_change_events FORCE ROW LEVEL SECURITY;
CREATE POLICY access_policy_events_tenant_select ON access_policy_change_events FOR SELECT
    USING (organization_id=get_current_tenant());
CREATE POLICY access_policy_events_tenant_insert ON access_policy_change_events FOR INSERT
    WITH CHECK (organization_id=get_current_tenant());
CREATE TRIGGER trg_access_policy_events_no_update
    BEFORE UPDATE ON access_policy_change_events FOR EACH ROW
    EXECUTE FUNCTION prevent_policy_access_evidence_modification();
CREATE TRIGGER trg_access_policy_events_no_delete
    BEFORE DELETE ON access_policy_change_events FOR EACH ROW
    EXECUTE FUNCTION prevent_policy_access_evidence_modification();

CREATE TABLE access_policy_certifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    access_policy_id UUID NOT NULL,
    policy_version BIGINT NOT NULL,
    certified_by UUID NOT NULL,
    decision VARCHAR(24) NOT NULL,
    reason VARCHAR(1000) NOT NULL,
    policy_snapshot JSONB NOT NULL,
    snapshot_sha256 CHAR(64) NOT NULL,
    request_id VARCHAR(64) NOT NULL,
    certified_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_access_policy_certifications_policy_tenant
        FOREIGN KEY (organization_id, access_policy_id)
        REFERENCES access_policies(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_access_policy_certifications_actor_tenant
        FOREIGN KEY (organization_id, certified_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_access_policy_certifications_version CHECK (policy_version > 0),
    CONSTRAINT chk_access_policy_certifications_decision CHECK (
        decision IN ('certified','changes_required')
    ),
    CONSTRAINT chk_access_policy_certifications_reason CHECK (
        reason=BTRIM(reason) AND length(reason) BETWEEN 3 AND 1000
    ),
    CONSTRAINT chk_access_policy_certifications_snapshot CHECK (pg_column_size(policy_snapshot) <= 65536),
    CONSTRAINT chk_access_policy_certifications_hash CHECK (snapshot_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_access_policy_certifications_request CHECK (
        request_id=BTRIM(request_id) AND length(request_id) BETWEEN 1 AND 64
    )
);
CREATE INDEX idx_access_policy_certifications_policy
    ON access_policy_certifications (organization_id, access_policy_id, certified_at DESC, id);

ALTER TABLE access_policy_certifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE access_policy_certifications FORCE ROW LEVEL SECURITY;
CREATE POLICY access_policy_certifications_tenant_select ON access_policy_certifications FOR SELECT
    USING (organization_id=get_current_tenant());
CREATE POLICY access_policy_certifications_tenant_insert ON access_policy_certifications FOR INSERT
    WITH CHECK (organization_id=get_current_tenant());
CREATE TRIGGER trg_access_policy_certifications_no_update
    BEFORE UPDATE ON access_policy_certifications FOR EACH ROW
    EXECUTE FUNCTION prevent_policy_access_evidence_modification();
CREATE TRIGGER trg_access_policy_certifications_no_delete
    BEFORE DELETE ON access_policy_certifications FOR EACH ROW
    EXECUTE FUNCTION prevent_policy_access_evidence_modification();

COMMENT ON COLUMN access_policies.legacy_definition IS
    'Original migration-024 condition document retained when automatic semantic conversion was unsafe; such policies are disabled pending administrator review.';
COMMENT ON TABLE user_entity_permissions IS
    'Sponsor-controlled, time-bounded object grants subordinate to RBAC and tenant policy constraints.';
COMMENT ON TABLE access_audit_log IS
    'Immutable evidence for RBAC and composed policy decisions. An allow is not returned until its row is durably inserted.';
COMMENT ON TABLE access_policy_change_events IS
    'Immutable before/after evidence for access-policy, assignment, field-rule, grant, and certification changes.';
COMMENT ON TABLE access_policy_certifications IS
    'Immutable policy-version certification snapshots with application-generated SHA-256 evidence.';
