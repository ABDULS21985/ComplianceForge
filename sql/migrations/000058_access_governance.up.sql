-- Bounded access certification, role-pair SoD, and enforced assignment windows.
ALTER TABLE user_roles
    ADD COLUMN assignment_id UUID NOT NULL DEFAULT gen_random_uuid(),
    ADD CONSTRAINT uq_user_roles_assignment_id UNIQUE (assignment_id),
    ADD COLUMN valid_from TIMESTAMPTZ NOT NULL DEFAULT '-infinity',
    ADD COLUMN expires_at TIMESTAMPTZ,
    ADD COLUMN version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN assignment_reason VARCHAR(1000),
    ADD COLUMN window_approved_by UUID,
    ADD CONSTRAINT fk_user_roles_window_approver FOREIGN KEY (organization_id,window_approved_by)
        REFERENCES users(organization_id,id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_user_roles_window CHECK (expires_at IS NULL OR expires_at > valid_from),
    ADD CONSTRAINT chk_user_roles_version CHECK (version > 0),
    ADD CONSTRAINT chk_user_roles_timed_reason CHECK (expires_at IS NULL OR
        (assignment_reason IS NOT NULL AND length(BTRIM(assignment_reason)) BETWEEN 3 AND 1000)),
    ADD CONSTRAINT chk_user_roles_approval CHECK (
        (expires_at IS NULL AND valid_from='-infinity'::timestamptz AND window_approved_by IS NULL)
        OR (expires_at IS NOT NULL AND isfinite(valid_from) AND isfinite(expires_at)
            AND expires_at<=valid_from+INTERVAL '90 days'
            AND window_approved_by IS NOT NULL AND window_approved_by<>user_id));

CREATE TABLE access_review_campaigns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    name VARCHAR(200) NOT NULL CHECK (length(BTRIM(name)) BETWEEN 3 AND 200),
    reviewer_id UUID NOT NULL,
    created_by UUID NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'open' CHECK (status IN ('open','completed','cancelled')),
    due_at TIMESTAMPTZ NOT NULL,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id,id),
    FOREIGN KEY (organization_id,reviewer_id) REFERENCES users(organization_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (organization_id,created_by) REFERENCES users(organization_id,id) ON DELETE RESTRICT,
    CHECK (reviewer_id <> created_by)
);
CREATE TABLE access_review_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    campaign_id UUID NOT NULL,
    subject_id UUID NOT NULL,
    kind VARCHAR(20) NOT NULL CHECK (kind IN ('role_assignment','object_grant')),
    resource_id UUID NOT NULL,
    snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot)='object' AND pg_column_size(snapshot)<=65536),
    snapshot_sha256 CHAR(64) NOT NULL CHECK (snapshot_sha256 ~ '^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id,id),
    UNIQUE (organization_id,campaign_id,subject_id,kind,resource_id),
    FOREIGN KEY (organization_id,campaign_id) REFERENCES access_review_campaigns(organization_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (organization_id,subject_id) REFERENCES users(organization_id,id) ON DELETE RESTRICT
);
CREATE TABLE access_review_decisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    item_id UUID NOT NULL,
    reviewer_id UUID NOT NULL,
    decision VARCHAR(12) NOT NULL CHECK (decision IN ('retain','revoke')),
    reason VARCHAR(1000) NOT NULL CHECK (length(BTRIM(reason)) BETWEEN 3 AND 1000),
    request_id VARCHAR(64) NOT NULL CHECK (length(BTRIM(request_id)) BETWEEN 1 AND 64),
    snapshot_sha256 CHAR(64) NOT NULL CHECK (snapshot_sha256 ~ '^[0-9a-f]{64}$'),
    decided_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id,item_id),
    UNIQUE (organization_id,request_id),
    FOREIGN KEY (organization_id,item_id) REFERENCES access_review_items(organization_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (organization_id,reviewer_id) REFERENCES users(organization_id,id) ON DELETE RESTRICT
);
CREATE TABLE access_sod_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    name VARCHAR(200) NOT NULL CHECK (length(BTRIM(name)) BETWEEN 3 AND 200),
    role_a_id UUID NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    role_b_id UUID NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version>0),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id,id),
    UNIQUE (organization_id,role_a_id,role_b_id),
    FOREIGN KEY (organization_id,created_by) REFERENCES users(organization_id,id) ON DELETE RESTRICT,
    CHECK (role_a_id < role_b_id)
);
CREATE TABLE access_sod_exceptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    rule_id UUID NOT NULL,
    subject_id UUID NOT NULL,
    requested_by UUID NOT NULL,
    approved_by UUID,
    status VARCHAR(12) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','revoked')),
    reason VARCHAR(1000) NOT NULL CHECK (length(BTRIM(reason)) BETWEEN 3 AND 1000),
    valid_from TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version>0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id,id),
    FOREIGN KEY (organization_id,rule_id) REFERENCES access_sod_rules(organization_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (organization_id,subject_id) REFERENCES users(organization_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (organization_id,requested_by) REFERENCES users(organization_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (organization_id,approved_by) REFERENCES users(organization_id,id) ON DELETE RESTRICT,
    CHECK (expires_at>valid_from AND expires_at<=valid_from+INTERVAL '90 days'),
    CHECK (approved_by IS NULL OR (approved_by<>subject_id AND approved_by<>requested_by)),
    CHECK ((status='approved' AND approved_by IS NOT NULL) OR status<>'approved')
);
CREATE UNIQUE INDEX uq_access_sod_exception_live ON access_sod_exceptions(organization_id,rule_id,subject_id)
    WHERE status IN ('pending','approved');
CREATE TABLE access_sod_violations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    rule_id UUID NOT NULL,
    subject_id UUID NOT NULL,
    detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (organization_id,rule_id) REFERENCES access_sod_rules(organization_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (organization_id,subject_id) REFERENCES users(organization_id,id) ON DELETE RESTRICT
);
CREATE TABLE access_governance_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    entity_id UUID NOT NULL,
    event_type VARCHAR(64) NOT NULL CHECK (event_type ~ '^[a-z][a-z0-9_]{1,63}$'),
    actor_id UUID NOT NULL,
    reason VARCHAR(1000) NOT NULL CHECK (length(BTRIM(reason)) BETWEEN 3 AND 1000),
    details JSONB NOT NULL CHECK (jsonb_typeof(details)='object' AND pg_column_size(details)<=65536),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (organization_id,actor_id) REFERENCES users(organization_id,id) ON DELETE RESTRICT
);
CREATE INDEX idx_access_review_campaigns_tenant ON access_review_campaigns(organization_id,created_at DESC,id);
CREATE INDEX idx_access_review_items_campaign ON access_review_items(organization_id,campaign_id,id);
CREATE INDEX idx_access_governance_events_tenant ON access_governance_events(organization_id,created_at DESC,id);
CREATE INDEX idx_access_sod_violations_tenant ON access_sod_violations(organization_id,detected_at DESC,id);

DO $rls$
DECLARE target TEXT;
BEGIN
    FOREACH target IN ARRAY ARRAY['access_review_campaigns','access_review_items','access_review_decisions',
        'access_sod_rules','access_sod_exceptions','access_sod_violations','access_governance_events'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',target);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',target);
        EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',target);
        EXECUTE format('CREATE POLICY tenant_select ON %I FOR SELECT USING (organization_id=get_current_tenant())',target);
        EXECUTE format('CREATE POLICY tenant_insert ON %I FOR INSERT WITH CHECK (organization_id=get_current_tenant())',target);
        IF target IN ('access_review_campaigns','access_sod_rules','access_sod_exceptions') THEN
            EXECUTE format('CREATE POLICY tenant_update ON %I FOR UPDATE USING (organization_id=get_current_tenant()) WITH CHECK (organization_id=get_current_tenant())',target);
        ELSE
            EXECUTE format('CREATE TRIGGER no_update BEFORE UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION prevent_policy_access_evidence_modification()',target);
            EXECUTE format('CREATE TRIGGER no_delete BEFORE DELETE ON %I FOR EACH ROW EXECUTE FUNCTION prevent_policy_access_evidence_modification()',target);
        END IF;
        IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='complianceforge_api') THEN
            EXECUTE format('GRANT SELECT,INSERT ON %I TO complianceforge_api',target);
            IF target IN ('access_review_campaigns','access_sod_rules','access_sod_exceptions') THEN
                EXECUTE format('GRANT UPDATE ON %I TO complianceforge_api',target);
            END IF;
        END IF;
    END LOOP;
END
$rls$;

-- SECURITY INVOKER is essential: FORCE-RLS must still run as the request role.
-- Any uncovered active role-pair conflict denies BOTH implicated roles. Expiry
-- is evaluated at statement time; no worker/cache refresh is needed to revoke.
CREATE VIEW effective_user_roles WITH (security_invoker=true,security_barrier=true) AS
SELECT ur.* FROM user_roles ur
WHERE ur.valid_from<=statement_timestamp() AND (ur.expires_at IS NULL OR ur.expires_at>statement_timestamp())
AND EXISTS (SELECT 1 FROM users subject WHERE subject.organization_id=ur.organization_id AND subject.id=ur.user_id
    AND subject.status='active' AND subject.deleted_at IS NULL)
AND EXISTS (SELECT 1 FROM roles role WHERE role.id=ur.role_id AND role.deleted_at IS NULL
    AND (role.organization_id=ur.organization_id OR (role.organization_id IS NULL AND role.is_system_role)))
AND NOT EXISTS (
    SELECT 1 FROM access_sod_rules rule
    WHERE rule.organization_id=ur.organization_id AND rule.enabled
      AND ur.role_id IN (rule.role_a_id,rule.role_b_id)
      AND EXISTS (SELECT 1 FROM user_roles other
          WHERE other.organization_id=ur.organization_id AND other.user_id=ur.user_id
            AND other.role_id=CASE WHEN ur.role_id=rule.role_a_id THEN rule.role_b_id ELSE rule.role_a_id END
            AND other.valid_from<=statement_timestamp()
            AND (other.expires_at IS NULL OR other.expires_at>statement_timestamp()))
      AND NOT EXISTS (SELECT 1 FROM access_sod_exceptions exception
          WHERE exception.organization_id=ur.organization_id AND exception.rule_id=rule.id
            AND exception.subject_id=ur.user_id AND exception.status='approved'
            AND exception.valid_from<=statement_timestamp() AND exception.expires_at>statement_timestamp())
);
REVOKE ALL ON effective_user_roles FROM PUBLIC;
DO $grant$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='complianceforge_api') THEN
        GRANT SELECT ON effective_user_roles TO complianceforge_api;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='complianceforge_scheduler') THEN
        GRANT SELECT ON effective_user_roles,access_sod_rules,access_sod_exceptions TO complianceforge_scheduler;
    END IF;
END $grant$;

CREATE FUNCTION validate_access_governance_scope() RETURNS TRIGGER
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $fn$
DECLARE campaign access_review_campaigns; item access_review_items;
BEGIN
    PERFORM 1 FROM organizations WHERE id=NEW.organization_id FOR UPDATE;
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.organization_id::text,580));
    IF TG_TABLE_NAME='access_sod_rules' THEN
        IF EXISTS (SELECT 1 FROM roles WHERE id IN (NEW.role_a_id,NEW.role_b_id)
            AND (deleted_at IS NOT NULL OR (organization_id IS NOT NULL AND organization_id<>NEW.organization_id)))
            OR (SELECT count(*) FROM roles WHERE id IN (NEW.role_a_id,NEW.role_b_id))<>2 THEN
            RAISE EXCEPTION 'SoD roles must be live and in tenant' USING ERRCODE='23514';
        END IF;
        IF NEW.enabled AND EXISTS (SELECT 1 FROM role_permissions rp JOIN permissions p ON p.id=rp.permission_id
            WHERE rp.role_id IN (NEW.role_a_id,NEW.role_b_id) AND p.resource='settings' AND p.action='configure')
          AND NOT EXISTS (SELECT 1 FROM users u WHERE u.organization_id=NEW.organization_id
            AND u.status='active' AND u.deleted_at IS NULL AND (u.is_super_admin OR EXISTS (
                SELECT 1 FROM effective_user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id
                JOIN permissions p ON p.id=rp.permission_id WHERE ur.organization_id=u.organization_id AND ur.user_id=u.id
                AND ur.role_id NOT IN (NEW.role_a_id,NEW.role_b_id) AND ur.expires_at IS NULL
                AND p.resource='settings' AND p.action='configure'
                AND NOT EXISTS (SELECT 1 FROM access_sod_rules other WHERE other.organization_id=ur.organization_id
                    AND other.enabled AND ur.role_id IN (other.role_a_id,other.role_b_id))))) THEN
            RAISE EXCEPTION 'SoD rule requires an unaffected permanent administrator' USING ERRCODE='23514';
        END IF;
        IF TG_OP='UPDATE' AND (NEW.organization_id<>OLD.organization_id OR NEW.role_a_id<>OLD.role_a_id
            OR NEW.role_b_id<>OLD.role_b_id OR NEW.created_by<>OLD.created_by OR NEW.created_at<>OLD.created_at
            OR NEW.version<>OLD.version+1) THEN
            RAISE EXCEPTION 'SoD rule identity/version is immutable' USING ERRCODE='23514';
        END IF;
    ELSIF TG_TABLE_NAME='access_review_items' THEN
        SELECT * INTO STRICT campaign FROM access_review_campaigns
            WHERE organization_id=NEW.organization_id AND id=NEW.campaign_id FOR SHARE;
        IF campaign.status<>'open' OR campaign.reviewer_id=NEW.subject_id THEN
            RAISE EXCEPTION 'Self-review or closed campaign' USING ERRCODE='23514';
        END IF;
    ELSIF TG_TABLE_NAME='access_review_decisions' THEN
        SELECT * INTO STRICT item FROM access_review_items WHERE organization_id=NEW.organization_id AND id=NEW.item_id;
        SELECT * INTO STRICT campaign FROM access_review_campaigns
            WHERE organization_id=NEW.organization_id AND id=item.campaign_id FOR SHARE;
        IF campaign.status<>'open' OR campaign.reviewer_id<>NEW.reviewer_id OR item.subject_id=NEW.reviewer_id
            OR campaign.created_by=NEW.reviewer_id OR item.snapshot_sha256<>NEW.snapshot_sha256 THEN
            RAISE EXCEPTION 'Review separation or snapshot mismatch' USING ERRCODE='23514';
        END IF;
    ELSIF TG_TABLE_NAME='access_review_campaigns' AND TG_OP='UPDATE' THEN
        IF OLD.status<>'open' OR NEW.status NOT IN ('completed','cancelled') OR NEW.version<>OLD.version+1
            OR ROW(NEW.id,NEW.organization_id,NEW.name,NEW.reviewer_id,NEW.created_by,NEW.due_at,NEW.created_at)
               IS DISTINCT FROM ROW(OLD.id,OLD.organization_id,OLD.name,OLD.reviewer_id,OLD.created_by,OLD.due_at,OLD.created_at)
            OR (NEW.status='completed' AND EXISTS (SELECT 1 FROM access_review_items i
                WHERE i.organization_id=OLD.organization_id AND i.campaign_id=OLD.id
                  AND NOT EXISTS (SELECT 1 FROM access_review_decisions d WHERE d.organization_id=i.organization_id AND d.item_id=i.id))) THEN
            RAISE EXCEPTION 'Invalid campaign transition' USING ERRCODE='23514';
        END IF;
    ELSIF TG_TABLE_NAME='access_sod_exceptions' AND TG_OP='INSERT' THEN
        IF NEW.status<>'pending' OR NEW.approved_by IS NOT NULL OR NEW.version<>1 THEN
            RAISE EXCEPTION 'Exception must begin pending' USING ERRCODE='23514';
        END IF;
    ELSIF TG_TABLE_NAME='access_sod_exceptions' AND TG_OP='UPDATE' THEN
        IF ROW(NEW.id,NEW.organization_id,NEW.rule_id,NEW.subject_id,NEW.requested_by,NEW.reason,NEW.valid_from,NEW.expires_at,NEW.created_at)
            IS DISTINCT FROM ROW(OLD.id,OLD.organization_id,OLD.rule_id,OLD.subject_id,OLD.requested_by,OLD.reason,OLD.valid_from,OLD.expires_at,OLD.created_at)
            OR NEW.version<>OLD.version+1 OR NOT ((OLD.status='pending' AND NEW.status IN ('approved','rejected'))
                OR (OLD.status='approved' AND NEW.status='revoked'))
            OR (OLD.status='approved' AND NEW.approved_by IS DISTINCT FROM OLD.approved_by) THEN
            RAISE EXCEPTION 'Invalid exception transition' USING ERRCODE='23514';
        END IF;
        IF NEW.status='approved' AND (NEW.expires_at<=statement_timestamp() OR NOT EXISTS(
            SELECT 1 FROM users WHERE organization_id=NEW.organization_id AND id=NEW.approved_by
                AND status='active' AND deleted_at IS NULL)) THEN
            RAISE EXCEPTION 'Exception approver/window is inactive' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NEW;
END $fn$;
REVOKE ALL ON FUNCTION validate_access_governance_scope() FROM PUBLIC;
CREATE TRIGGER governance_scope BEFORE INSERT OR UPDATE ON access_review_campaigns FOR EACH ROW EXECUTE FUNCTION validate_access_governance_scope();
CREATE TRIGGER governance_scope BEFORE INSERT ON access_review_items FOR EACH ROW EXECUTE FUNCTION validate_access_governance_scope();
CREATE TRIGGER governance_scope BEFORE INSERT ON access_review_decisions FOR EACH ROW EXECUTE FUNCTION validate_access_governance_scope();
CREATE TRIGGER governance_scope BEFORE INSERT OR UPDATE ON access_sod_rules FOR EACH ROW EXECUTE FUNCTION validate_access_governance_scope();
CREATE TRIGGER governance_scope BEFORE INSERT OR UPDATE ON access_sod_exceptions FOR EACH ROW EXECUTE FUNCTION validate_access_governance_scope();

CREATE FUNCTION enforce_governed_role_assignment() RETURNS TRIGGER
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $fn$
BEGIN
    PERFORM 1 FROM organizations WHERE id=NEW.organization_id FOR UPDATE;
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.organization_id::text,580));
    IF TG_OP='UPDATE' THEN
        IF ROW(NEW.user_id,NEW.role_id,NEW.organization_id,NEW.assigned_by,NEW.assigned_at,NEW.assignment_id)
            IS DISTINCT FROM ROW(OLD.user_id,OLD.role_id,OLD.organization_id,OLD.assigned_by,OLD.assigned_at,OLD.assignment_id) THEN
            RAISE EXCEPTION 'Assignment identity immutable' USING ERRCODE='23514';
        END IF;
        NEW.version=OLD.version+1;
    END IF;
    IF NEW.expires_at IS NOT NULL AND NEW.window_approved_by=NEW.user_id THEN
        RAISE EXCEPTION 'Self-approved timed assignment' USING ERRCODE='23514';
    END IF;
    IF NEW.expires_at IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users
        WHERE organization_id=NEW.organization_id AND id=NEW.window_approved_by
            AND status='active' AND deleted_at IS NULL) THEN
        RAISE EXCEPTION 'Timed assignment approver must be active in tenant' USING ERRCODE='23514';
    END IF;
    IF NEW.expires_at IS NOT NULL AND EXISTS(SELECT 1 FROM role_permissions rp JOIN permissions p ON p.id=rp.permission_id
        WHERE rp.role_id=NEW.role_id AND p.resource='settings' AND p.action='configure')
      AND NOT EXISTS(SELECT 1 FROM users u WHERE u.organization_id=NEW.organization_id AND u.id<>NEW.user_id
        AND u.status='active' AND u.deleted_at IS NULL AND (u.is_super_admin OR EXISTS (
            SELECT 1 FROM effective_user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id
            JOIN permissions p ON p.id=rp.permission_id WHERE ur.organization_id=u.organization_id AND ur.user_id=u.id
            AND ur.expires_at IS NULL AND p.resource='settings' AND p.action='configure'
            AND NOT EXISTS(SELECT 1 FROM access_sod_rules rule WHERE rule.organization_id=ur.organization_id
                AND rule.enabled AND ur.role_id IN(rule.role_a_id,rule.role_b_id))))) THEN
        RAISE EXCEPTION 'Timed administrative access requires another permanent administrator' USING ERRCODE='23514';
    END IF;
    IF EXISTS (SELECT 1 FROM access_sod_rules rule JOIN user_roles other
        ON other.organization_id=rule.organization_id AND other.user_id=NEW.user_id
        AND other.role_id=CASE WHEN NEW.role_id=rule.role_a_id THEN rule.role_b_id ELSE rule.role_a_id END
        WHERE rule.organization_id=NEW.organization_id AND rule.enabled
          AND NEW.role_id IN (rule.role_a_id,rule.role_b_id)
          AND tstzrange(NEW.valid_from,NEW.expires_at,'[)') && tstzrange(other.valid_from,other.expires_at,'[)')
          AND NOT EXISTS (SELECT 1 FROM access_sod_exceptions e WHERE e.organization_id=rule.organization_id
              AND e.rule_id=rule.id AND e.subject_id=NEW.user_id AND e.status='approved'
              AND tstzrange(e.valid_from,e.expires_at,'[)') @>
                  (tstzrange(NEW.valid_from,NEW.expires_at,'[)') * tstzrange(other.valid_from,other.expires_at,'[)')))) THEN
        RAISE EXCEPTION 'Role assignment violates segregation of duties' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $fn$;
REVOKE ALL ON FUNCTION enforce_governed_role_assignment() FROM PUBLIC;
CREATE TRIGGER governed_role_assignment BEFORE INSERT OR UPDATE ON user_roles
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_role_assignment();

COMMENT ON VIEW effective_user_roles IS 'Canonical current role authority, with window and uncovered SoD conflicts enforced at statement time; raw user_roles is administrative history only.';
