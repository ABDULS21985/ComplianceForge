-- Migration 052: tenant data-lifecycle governance, retention, disposition,
-- legal holds, and tamper-evident governance history.

CREATE FUNCTION data_region_array_is_valid(values_to_check TEXT[])
RETURNS BOOLEAN
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT cardinality(values_to_check) BETWEEN 1 AND 50
       AND COALESCE(bool_and(value ~ '^[a-z][a-z0-9-]{1,31}$'), false)
    FROM unnest(values_to_check) AS value
$$;

CREATE TABLE tenant_data_governance_policies (
    organization_id UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    primary_region VARCHAR(32) NOT NULL
        CHECK (primary_region ~ '^[a-z][a-z0-9-]{1,31}$'),
    allowed_regions TEXT[] NOT NULL,
    cross_border_transfer_mode VARCHAR(32) NOT NULL DEFAULT 'approved_regions'
        CHECK (cross_border_transfer_mode IN (
            'prohibited','approved_regions','contractual_safeguards'
        )),
    default_retention_days INTEGER NOT NULL DEFAULT 2555
        CHECK (default_retention_days BETWEEN 1 AND 36500),
    default_archive_after_days INTEGER
        CHECK (default_archive_after_days IS NULL OR default_archive_after_days BETWEEN 1 AND 36499),
    deletion_grace_days INTEGER NOT NULL DEFAULT 30
        CHECK (deletion_grace_days BETWEEN 0 AND 365),
    disposition_approval_mode VARCHAR(16) NOT NULL DEFAULT 'single'
        CHECK (disposition_approval_mode IN ('none','single','dual')),
    require_processor_confirmation BOOLEAN NOT NULL DEFAULT true,
    legal_hold_enabled BOOLEAN NOT NULL DEFAULT true,
    policy_statement TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_by UUID NOT NULL,
    updated_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_data_governance_policy_creator FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_data_governance_policy_updater FOREIGN KEY (organization_id, updated_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_data_governance_regions CHECK (
        data_region_array_is_valid(allowed_regions)
        AND primary_region = ANY(allowed_regions)
    ),
    CONSTRAINT chk_data_governance_archive_before_retention CHECK (
        default_archive_after_days IS NULL
        OR default_archive_after_days < default_retention_days
    ),
    CONSTRAINT chk_data_governance_statement CHECK (length(policy_statement) <= 20000)
);

CREATE TRIGGER trg_tenant_data_governance_policies_updated_at
    BEFORE UPDATE ON tenant_data_governance_policies
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE retention_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(200) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    record_type VARCHAR(40) NOT NULL,
    data_classification VARCHAR(32),
    jurisdiction VARCHAR(16),
    trigger_event VARCHAR(40) NOT NULL DEFAULT 'record_created',
    legal_basis TEXT NOT NULL,
    retention_days INTEGER NOT NULL CHECK (retention_days BETWEEN 1 AND 36500),
    archive_after_days INTEGER
        CHECK (archive_after_days IS NULL OR archive_after_days BETWEEN 1 AND 36499),
    disposition_action VARCHAR(16) NOT NULL DEFAULT 'review'
        CHECK (disposition_action IN ('review','delete','anonymize','archive')),
    review_required BOOLEAN NOT NULL DEFAULT true,
    priority INTEGER NOT NULL DEFAULT 100 CHECK (priority BETWEEN 1 AND 10000),
    status VARCHAR(16) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft','active','retired')),
    effective_from DATE NOT NULL DEFAULT CURRENT_DATE,
    effective_until DATE,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by UUID NOT NULL,
    updated_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_retention_schedules_org_id UNIQUE (organization_id, id),
    CONSTRAINT fk_retention_schedule_creator FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_retention_schedule_updater FOREIGN KEY (organization_id, updated_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_retention_schedule_name CHECK (
        name = btrim(name) AND length(name) BETWEEN 3 AND 200
    ),
    CONSTRAINT chk_retention_schedule_description CHECK (length(description) <= 20000),
    CONSTRAINT chk_retention_schedule_record_type CHECK (
        record_type IN (
            'asset','audit','audit_finding','comment','control','evidence',
            'incident','policy','report','risk','vendor'
        )
    ),
    CONSTRAINT chk_retention_schedule_classification CHECK (
        data_classification IS NULL OR data_classification IN (
            'public','internal','confidential','restricted','personal','special_category'
        )
    ),
    CONSTRAINT chk_retention_schedule_jurisdiction CHECK (
        jurisdiction IS NULL OR jurisdiction ~ '^[A-Z0-9-]{2,16}$'
    ),
    CONSTRAINT chk_retention_schedule_trigger CHECK (
        trigger_event IN (
            'record_created','record_closed','contract_ended','employment_ended',
            'consent_withdrawn','superseded','case_closed'
        )
    ),
    CONSTRAINT chk_retention_schedule_legal_basis CHECK (
        legal_basis = btrim(legal_basis) AND length(legal_basis) BETWEEN 3 AND 4000
    ),
    CONSTRAINT chk_retention_schedule_archive_before_disposition CHECK (
        archive_after_days IS NULL OR archive_after_days < retention_days
    ),
    CONSTRAINT chk_retention_schedule_effective_window CHECK (
        effective_until IS NULL OR effective_until >= effective_from
    )
);

CREATE UNIQUE INDEX uq_retention_schedules_name_active
    ON retention_schedules (organization_id, lower(name)) WHERE deleted_at IS NULL;
CREATE INDEX idx_retention_schedules_resolution
    ON retention_schedules (
        organization_id, record_type, status, priority, effective_from, effective_until
    ) WHERE deleted_at IS NULL;
CREATE TRIGGER trg_retention_schedules_updated_at
    BEFORE UPDATE ON retention_schedules
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE record_retention_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    schedule_id UUID NOT NULL,
    record_type VARCHAR(40) NOT NULL,
    record_id UUID NOT NULL,
    data_classification VARCHAR(32),
    jurisdiction VARCHAR(16),
    trigger_event VARCHAR(40) NOT NULL,
    retention_started_at TIMESTAMPTZ NOT NULL,
    archive_eligible_at TIMESTAMPTZ,
    disposition_due_at TIMESTAMPTZ NOT NULL,
    disposition_action VARCHAR(16) NOT NULL,
    review_required BOOLEAN NOT NULL,
    review_status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (review_status IN ('not_required','pending','approved','rejected')),
    reviewed_by UUID,
    reviewed_at TIMESTAMPTZ,
    review_reason TEXT,
    state VARCHAR(24) NOT NULL DEFAULT 'active'
        CHECK (state IN ('active','archived','disposition_pending','disposed','exception')),
    source VARCHAR(16) NOT NULL DEFAULT 'manual'
        CHECK (source IN ('automatic','manual','import')),
    reason TEXT NOT NULL,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    disposed_at TIMESTAMPTZ,
    CONSTRAINT uq_record_retention_assignments_org_id UNIQUE (organization_id, id),
    CONSTRAINT fk_record_retention_schedule FOREIGN KEY (organization_id, schedule_id)
        REFERENCES retention_schedules(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_record_retention_creator FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_record_retention_reviewer FOREIGN KEY (organization_id, reviewed_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_record_retention_type CHECK (
        record_type IN (
            'asset','audit','audit_finding','comment','control','evidence',
            'incident','policy','report','risk','vendor'
        )
    ),
    CONSTRAINT chk_record_retention_classification CHECK (
        data_classification IS NULL OR data_classification IN (
            'public','internal','confidential','restricted','personal','special_category'
        )
    ),
    CONSTRAINT chk_record_retention_jurisdiction CHECK (
        jurisdiction IS NULL OR jurisdiction ~ '^[A-Z0-9-]{2,16}$'
    ),
    CONSTRAINT chk_record_retention_trigger CHECK (
        trigger_event IN (
            'record_created','record_closed','contract_ended','employment_ended',
            'consent_withdrawn','superseded','case_closed'
        )
    ),
    CONSTRAINT chk_record_retention_dates CHECK (
        disposition_due_at > retention_started_at
        AND (archive_eligible_at IS NULL OR (
            archive_eligible_at > retention_started_at
            AND archive_eligible_at < disposition_due_at
        ))
        AND (disposed_at IS NULL OR disposed_at >= retention_started_at)
    ),
    CONSTRAINT chk_record_retention_action CHECK (
        disposition_action IN ('review','delete','anonymize','archive')
    ),
    CONSTRAINT chk_record_retention_review CHECK (
        (review_status IN ('not_required','pending')
            AND reviewed_by IS NULL AND reviewed_at IS NULL AND review_reason IS NULL)
        OR (review_status IN ('approved','rejected')
            AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL
            AND length(btrim(coalesce(review_reason, ''))) BETWEEN 3 AND 2000)
    ),
    CONSTRAINT chk_record_retention_reason CHECK (
        reason = btrim(reason) AND length(reason) BETWEEN 3 AND 2000
    )
);

CREATE UNIQUE INDEX uq_record_retention_active
    ON record_retention_assignments (organization_id, record_type, record_id)
    WHERE state <> 'disposed';
CREATE INDEX idx_record_retention_due
    ON record_retention_assignments (organization_id, disposition_due_at, id)
    WHERE state IN ('active','archived','disposition_pending','exception');
CREATE INDEX idx_record_retention_archive
    ON record_retention_assignments (organization_id, archive_eligible_at, id)
    WHERE state = 'active' AND archive_eligible_at IS NOT NULL;
CREATE TRIGGER trg_record_retention_assignments_updated_at
    BEFORE UPDATE ON record_retention_assignments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE retention_exceptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    assignment_id UUID NOT NULL,
    requested_until TIMESTAMPTZ NOT NULL,
    reason TEXT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','approved','rejected','revoked')),
    requested_by UUID NOT NULL,
    decided_by UUID,
    decided_at TIMESTAMPTZ,
    decision_reason TEXT,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_retention_exceptions_org_id UNIQUE (organization_id, id),
    CONSTRAINT fk_retention_exception_assignment FOREIGN KEY (organization_id, assignment_id)
        REFERENCES record_retention_assignments(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_retention_exception_requester FOREIGN KEY (organization_id, requested_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_retention_exception_decider FOREIGN KEY (organization_id, decided_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_retention_exception_reason CHECK (
        reason = btrim(reason) AND length(reason) BETWEEN 3 AND 2000
    ),
    CONSTRAINT chk_retention_exception_decision CHECK (
        (status = 'pending' AND decided_by IS NULL AND decided_at IS NULL AND decision_reason IS NULL)
        OR (status <> 'pending' AND decided_by IS NOT NULL AND decided_at IS NOT NULL
            AND length(btrim(coalesce(decision_reason, ''))) BETWEEN 3 AND 2000)
    )
);

CREATE UNIQUE INDEX uq_retention_exception_pending
    ON retention_exceptions (organization_id, assignment_id) WHERE status = 'pending';
CREATE INDEX idx_retention_exception_status
    ON retention_exceptions (organization_id, status, created_at DESC);
CREATE TRIGGER trg_retention_exceptions_updated_at
    BEFORE UPDATE ON retention_exceptions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE legal_hold_reference_sequences (
    organization_id UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    next_value BIGINT NOT NULL DEFAULT 1 CHECK (next_value > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_legal_hold_reference_sequences_updated_at
    BEFORE UPDATE ON legal_hold_reference_sequences
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE legal_holds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    hold_ref VARCHAR(32) NOT NULL,
    name VARCHAR(200) NOT NULL,
    matter_reference VARCHAR(200),
    description TEXT NOT NULL,
    legal_authority TEXT NOT NULL,
    scope JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(scope) = 'object'),
    status VARCHAR(16) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active','released','cancelled')),
    owner_user_id UUID NOT NULL,
    placed_by UUID NOT NULL,
    placed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    review_due_at TIMESTAMPTZ,
    released_by UUID,
    released_at TIMESTAMPTZ,
    release_reason TEXT,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_legal_holds_org_id UNIQUE (organization_id, id),
    CONSTRAINT uq_legal_holds_ref UNIQUE (organization_id, hold_ref),
    CONSTRAINT fk_legal_hold_owner FOREIGN KEY (organization_id, owner_user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_legal_hold_placer FOREIGN KEY (organization_id, placed_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_legal_hold_releaser FOREIGN KEY (organization_id, released_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_legal_hold_name CHECK (
        name = btrim(name) AND length(name) BETWEEN 3 AND 200
    ),
    CONSTRAINT chk_legal_hold_description CHECK (
        description = btrim(description) AND length(description) BETWEEN 3 AND 20000
    ),
    CONSTRAINT chk_legal_hold_authority CHECK (
        legal_authority = btrim(legal_authority) AND length(legal_authority) BETWEEN 3 AND 4000
    ),
    CONSTRAINT chk_legal_hold_matter CHECK (
        matter_reference IS NULL OR length(btrim(matter_reference)) BETWEEN 1 AND 200
    ),
    CONSTRAINT chk_legal_hold_release CHECK (
        (status = 'active' AND released_by IS NULL AND released_at IS NULL AND release_reason IS NULL)
        OR (status IN ('released','cancelled') AND released_by IS NOT NULL AND released_at IS NOT NULL
            AND released_at >= placed_at
            AND length(btrim(coalesce(release_reason, ''))) BETWEEN 3 AND 4000)
    )
);

CREATE UNIQUE INDEX uq_legal_holds_matter_active
    ON legal_holds (organization_id, lower(matter_reference))
    WHERE matter_reference IS NOT NULL AND status = 'active';
CREATE INDEX idx_legal_holds_status
    ON legal_holds (organization_id, status, placed_at DESC, id);
CREATE INDEX idx_legal_holds_review
    ON legal_holds (organization_id, review_due_at)
    WHERE status = 'active' AND review_due_at IS NOT NULL;
CREATE TRIGGER trg_legal_holds_updated_at
    BEFORE UPDATE ON legal_holds
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE legal_hold_custodians (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    hold_id UUID NOT NULL,
    user_id UUID NOT NULL,
    added_by UUID NOT NULL,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at TIMESTAMPTZ,
    released_by UUID,
    release_reason TEXT,
    PRIMARY KEY (organization_id, hold_id, user_id),
    CONSTRAINT fk_legal_hold_custodian_hold FOREIGN KEY (organization_id, hold_id)
        REFERENCES legal_holds(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_legal_hold_custodian_user FOREIGN KEY (organization_id, user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_legal_hold_custodian_adder FOREIGN KEY (organization_id, added_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_legal_hold_custodian_releaser FOREIGN KEY (organization_id, released_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_legal_hold_custodian_release CHECK (
        (released_at IS NULL AND released_by IS NULL AND release_reason IS NULL)
        OR (released_at IS NOT NULL AND released_by IS NOT NULL
            AND length(btrim(coalesce(release_reason, ''))) BETWEEN 3 AND 2000)
    )
);

CREATE TABLE legal_hold_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    hold_id UUID NOT NULL,
    record_type VARCHAR(40) NOT NULL,
    record_id UUID NOT NULL,
    reason TEXT NOT NULL,
    placed_by UUID NOT NULL,
    placed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at TIMESTAMPTZ,
    released_by UUID,
    release_reason TEXT,
    CONSTRAINT uq_legal_hold_records_org_id UNIQUE (organization_id, id),
    CONSTRAINT fk_legal_hold_record_hold FOREIGN KEY (organization_id, hold_id)
        REFERENCES legal_holds(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_legal_hold_record_placer FOREIGN KEY (organization_id, placed_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_legal_hold_record_releaser FOREIGN KEY (organization_id, released_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_legal_hold_record_type CHECK (
        record_type IN (
            'asset','audit','audit_finding','comment','control','evidence',
            'incident','policy','report','risk','vendor'
        )
    ),
    CONSTRAINT chk_legal_hold_record_reason CHECK (
        reason = btrim(reason) AND length(reason) BETWEEN 3 AND 2000
    ),
    CONSTRAINT chk_legal_hold_record_release CHECK (
        (released_at IS NULL AND released_by IS NULL AND release_reason IS NULL)
        OR (released_at IS NOT NULL AND released_by IS NOT NULL
            AND released_at >= placed_at
            AND length(btrim(coalesce(release_reason, ''))) BETWEEN 3 AND 2000)
    )
);

CREATE UNIQUE INDEX uq_legal_hold_record_active
    ON legal_hold_records (organization_id, hold_id, record_type, record_id)
    WHERE released_at IS NULL;
CREATE INDEX idx_legal_hold_record_lookup
    ON legal_hold_records (organization_id, record_type, record_id, placed_at DESC)
    WHERE released_at IS NULL;

CREATE TABLE data_governance_event_chain_heads (
    organization_id UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    last_sequence BIGINT NOT NULL DEFAULT 0 CHECK (last_sequence >= 0),
    last_hash BYTEA NOT NULL CHECK (octet_length(last_hash) = 32),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE data_governance_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    chain_sequence BIGINT NOT NULL CHECK (chain_sequence > 0),
    previous_hash BYTEA NOT NULL CHECK (octet_length(previous_hash) = 32),
    event_hash BYTEA NOT NULL CHECK (octet_length(event_hash) = 32),
    entity_type VARCHAR(40) NOT NULL CHECK (entity_type IN (
        'policy','retention_schedule','retention_assignment','retention_exception','legal_hold'
    )),
    entity_id UUID NOT NULL,
    event_type VARCHAR(40) NOT NULL CHECK (event_type IN (
        'policy_saved','schedule_created','schedule_updated','schedule_retired',
        'assignment_created','disposition_approved','disposition_rejected','record_disposed',
        'exception_requested','exception_approved','exception_rejected','exception_revoked',
        'hold_created','hold_updated','hold_record_added','hold_record_released',
        'hold_released','hold_cancelled'
    )),
    actor_user_id UUID NOT NULL,
    reason TEXT NOT NULL,
    before_state JSONB CHECK (before_state IS NULL OR jsonb_typeof(before_state) = 'object'),
    after_state JSONB CHECK (after_state IS NULL OR jsonb_typeof(after_state) = 'object'),
    source VARCHAR(16) NOT NULL DEFAULT 'api' CHECK (source IN ('api','scheduler','system','import')),
    request_id VARCHAR(160),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_data_governance_event_sequence UNIQUE (organization_id, chain_sequence),
    CONSTRAINT fk_data_governance_event_actor FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_data_governance_event_reason CHECK (
        reason = btrim(reason) AND length(reason) BETWEEN 3 AND 2000
    )
);

CREATE INDEX idx_data_governance_events_entity
    ON data_governance_events (organization_id, entity_type, entity_id, chain_sequence DESC);
CREATE INDEX idx_data_governance_events_time
    ON data_governance_events (organization_id, created_at DESC, id DESC);

CREATE FUNCTION calculate_data_governance_event_hash(
    event_organization_id UUID,
    event_sequence BIGINT,
    event_previous_hash BYTEA,
    event_entity_type TEXT,
    event_entity_id UUID,
    event_type_value TEXT,
    event_actor_user_id UUID,
    event_reason TEXT,
    event_before_state JSONB,
    event_after_state JSONB,
    event_source TEXT,
    event_request_id TEXT,
    event_created_at TIMESTAMPTZ
)
RETURNS BYTEA
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT digest(convert_to(concat_ws('|',
        event_organization_id::text,
        event_sequence::text,
        encode(event_previous_hash, 'hex'),
        event_entity_type,
        event_entity_id::text,
        event_type_value,
        event_actor_user_id::text,
        event_reason,
        COALESCE(event_before_state, 'null'::jsonb)::text,
        COALESCE(event_after_state, 'null'::jsonb)::text,
        event_source,
        COALESCE(event_request_id, ''),
        to_char(event_created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
    ), 'UTF8'), 'sha256')
$$;

CREATE FUNCTION append_data_governance_event_chain()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    current_tenant UUID;
    prior_sequence BIGINT;
    prior_hash BYTEA;
BEGIN
    current_tenant := get_current_tenant();
    IF current_tenant IS NULL OR current_tenant <> NEW.organization_id THEN
        RAISE EXCEPTION 'data governance event tenant does not match request tenant'
            USING ERRCODE = '42501';
    END IF;

    INSERT INTO data_governance_event_chain_heads (organization_id, last_sequence, last_hash)
    VALUES (NEW.organization_id, 0, decode(repeat('00', 32), 'hex'))
    ON CONFLICT (organization_id) DO NOTHING;

    SELECT last_sequence, last_hash INTO prior_sequence, prior_hash
    FROM data_governance_event_chain_heads
    WHERE organization_id = NEW.organization_id
    FOR UPDATE;

    NEW.chain_sequence := prior_sequence + 1;
    NEW.previous_hash := prior_hash;
    NEW.event_hash := calculate_data_governance_event_hash(
        NEW.organization_id, NEW.chain_sequence, NEW.previous_hash,
        NEW.entity_type, NEW.entity_id, NEW.event_type, NEW.actor_user_id,
        NEW.reason, NEW.before_state, NEW.after_state, NEW.source,
        NEW.request_id, NEW.created_at
    );

    UPDATE data_governance_event_chain_heads
    SET last_sequence = NEW.chain_sequence,
        last_hash = NEW.event_hash,
        updated_at = NOW()
    WHERE organization_id = NEW.organization_id;
    RETURN NEW;
END;
$$;

CREATE FUNCTION prevent_data_governance_event_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'data governance history is append-only: % is not permitted', TG_OP
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER trg_data_governance_event_chain
    BEFORE INSERT ON data_governance_events
    FOR EACH ROW EXECUTE FUNCTION append_data_governance_event_chain();
CREATE TRIGGER trg_data_governance_events_immutable
    BEFORE UPDATE OR DELETE ON data_governance_events
    FOR EACH ROW EXECUTE FUNCTION prevent_data_governance_event_mutation();

CREATE FUNCTION enforce_governed_record_disposition()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    record_kind TEXT := TG_ARGV[0];
    request_tenant UUID := get_current_tenant();
BEGIN
    -- A NULL context is reserved for privileged migration/teardown tooling.
    -- Normal application roles cannot see the row without RLS context.
    IF request_tenant IS NOT NULL AND request_tenant <> OLD.organization_id THEN
        RAISE EXCEPTION 'record disposition tenant does not match request tenant'
            USING ERRCODE = '42501';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM legal_hold_records held_record
        JOIN legal_holds hold
          ON hold.organization_id = held_record.organization_id
         AND hold.id = held_record.hold_id
        WHERE held_record.organization_id = OLD.organization_id
          AND held_record.record_type = record_kind
          AND held_record.record_id = OLD.id
          AND held_record.released_at IS NULL
          AND hold.status = 'active'
    ) THEN
        RAISE EXCEPTION 'record is protected by an active legal hold'
            USING ERRCODE = '55000';
    END IF;

    -- Once a tenant opts in to a governance policy, unmanaged deletion fails
    -- closed. A completed assignment remains as disposition evidence and is
    -- therefore sufficient to allow the corresponding physical operation.
    IF EXISTS (
        SELECT 1 FROM tenant_data_governance_policies policy
        WHERE policy.organization_id = OLD.organization_id
    ) AND NOT EXISTS (
        SELECT 1 FROM record_retention_assignments assignment
        WHERE assignment.organization_id = OLD.organization_id
          AND assignment.record_type = record_kind
          AND assignment.record_id = OLD.id
    ) THEN
        RAISE EXCEPTION 'record has no governed retention assignment'
            USING ERRCODE = '55000';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM record_retention_assignments assignment
        WHERE assignment.organization_id = OLD.organization_id
          AND assignment.record_type = record_kind
          AND assignment.record_id = OLD.id
          AND assignment.state <> 'disposed'
          AND (
              assignment.state = 'exception'
              OR
              assignment.disposition_due_at > statement_timestamp()
              OR (assignment.review_required AND assignment.review_status <> 'approved')
          )
    ) THEN
        RAISE EXCEPTION 'record retention or disposition approval prevents deletion'
            USING ERRCODE = '55000';
    END IF;
    RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$;

CREATE FUNCTION prevent_legacy_legal_hold_disposition()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'record is protected by an active legal hold'
        USING ERRCODE = '55000';
END;
$$;

-- Defence in depth: these triggers protect both application soft deletion and
-- direct/cascading hard deletion for the governed record families.
CREATE TRIGGER trg_assets_governed_hard_delete BEFORE DELETE ON assets
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_record_disposition('asset');
CREATE TRIGGER trg_assets_governed_soft_delete BEFORE UPDATE OF deleted_at ON assets
    FOR EACH ROW WHEN (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
    EXECUTE FUNCTION enforce_governed_record_disposition('asset');
CREATE TRIGGER trg_audits_governed_hard_delete BEFORE DELETE ON audits
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_record_disposition('audit');
CREATE TRIGGER trg_audits_governed_soft_delete BEFORE UPDATE OF deleted_at ON audits
    FOR EACH ROW WHEN (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
    EXECUTE FUNCTION enforce_governed_record_disposition('audit');
CREATE TRIGGER trg_audit_findings_governed_hard_delete BEFORE DELETE ON audit_findings
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_record_disposition('audit_finding');
CREATE TRIGGER trg_audit_findings_governed_soft_delete BEFORE UPDATE OF deleted_at ON audit_findings
    FOR EACH ROW WHEN (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
    EXECUTE FUNCTION enforce_governed_record_disposition('audit_finding');
CREATE TRIGGER trg_comments_governed_hard_delete BEFORE DELETE ON comments
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_record_disposition('comment');
CREATE TRIGGER trg_comments_governed_soft_delete BEFORE UPDATE OF deleted_at, is_deleted ON comments
    FOR EACH ROW WHEN (
        (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
        OR (NOT OLD.is_deleted AND NEW.is_deleted)
    ) EXECUTE FUNCTION enforce_governed_record_disposition('comment');
CREATE TRIGGER trg_control_evidence_governed_hard_delete BEFORE DELETE ON control_evidence
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_record_disposition('evidence');
CREATE TRIGGER trg_control_evidence_governed_soft_delete BEFORE UPDATE OF deleted_at ON control_evidence
    FOR EACH ROW WHEN (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
    EXECUTE FUNCTION enforce_governed_record_disposition('evidence');
CREATE TRIGGER trg_control_implementations_governed_hard_delete BEFORE DELETE ON control_implementations
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_record_disposition('control');
CREATE TRIGGER trg_control_implementations_governed_soft_delete BEFORE UPDATE OF deleted_at ON control_implementations
    FOR EACH ROW WHEN (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
    EXECUTE FUNCTION enforce_governed_record_disposition('control');
CREATE TRIGGER trg_incidents_governed_hard_delete BEFORE DELETE ON incidents
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_record_disposition('incident');
CREATE TRIGGER trg_incidents_governed_soft_delete BEFORE UPDATE OF deleted_at ON incidents
    FOR EACH ROW WHEN (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
    EXECUTE FUNCTION enforce_governed_record_disposition('incident');
CREATE TRIGGER trg_incidents_legacy_hold_hard_delete BEFORE DELETE ON incidents
    FOR EACH ROW WHEN (OLD.legal_hold)
    EXECUTE FUNCTION prevent_legacy_legal_hold_disposition();
CREATE TRIGGER trg_incidents_legacy_hold_soft_delete BEFORE UPDATE OF deleted_at ON incidents
    FOR EACH ROW WHEN (OLD.legal_hold AND OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
    EXECUTE FUNCTION prevent_legacy_legal_hold_disposition();
CREATE TRIGGER trg_policies_governed_hard_delete BEFORE DELETE ON policies
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_record_disposition('policy');
CREATE TRIGGER trg_policies_governed_soft_delete BEFORE UPDATE OF deleted_at ON policies
    FOR EACH ROW WHEN (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
    EXECUTE FUNCTION enforce_governed_record_disposition('policy');
CREATE TRIGGER trg_report_runs_governed_hard_delete BEFORE DELETE ON report_runs
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_record_disposition('report');
CREATE TRIGGER trg_risks_governed_hard_delete BEFORE DELETE ON risks
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_record_disposition('risk');
CREATE TRIGGER trg_risks_governed_soft_delete BEFORE UPDATE OF deleted_at ON risks
    FOR EACH ROW WHEN (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
    EXECUTE FUNCTION enforce_governed_record_disposition('risk');
CREATE TRIGGER trg_vendors_governed_hard_delete BEFORE DELETE ON vendors
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_record_disposition('vendor');
CREATE TRIGGER trg_vendors_governed_soft_delete BEFORE UPDATE OF deleted_at ON vendors
    FOR EACH ROW WHEN (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
    EXECUTE FUNCTION enforce_governed_record_disposition('vendor');
CREATE TRIGGER trg_vendors_legacy_hold_hard_delete BEFORE DELETE ON vendors
    FOR EACH ROW WHEN (OLD.legal_hold)
    EXECUTE FUNCTION prevent_legacy_legal_hold_disposition();
CREATE TRIGGER trg_vendors_legacy_hold_soft_delete BEFORE UPDATE OF deleted_at ON vendors
    FOR EACH ROW WHEN (OLD.legal_hold AND OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
    EXECUTE FUNCTION prevent_legacy_legal_hold_disposition();

ALTER TABLE tenant_data_governance_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_data_governance_policies FORCE ROW LEVEL SECURITY;
ALTER TABLE retention_schedules ENABLE ROW LEVEL SECURITY;
ALTER TABLE retention_schedules FORCE ROW LEVEL SECURITY;
ALTER TABLE record_retention_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE record_retention_assignments FORCE ROW LEVEL SECURITY;
ALTER TABLE retention_exceptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE retention_exceptions FORCE ROW LEVEL SECURITY;
ALTER TABLE legal_hold_reference_sequences ENABLE ROW LEVEL SECURITY;
ALTER TABLE legal_hold_reference_sequences FORCE ROW LEVEL SECURITY;
ALTER TABLE legal_holds ENABLE ROW LEVEL SECURITY;
ALTER TABLE legal_holds FORCE ROW LEVEL SECURITY;
ALTER TABLE legal_hold_custodians ENABLE ROW LEVEL SECURITY;
ALTER TABLE legal_hold_custodians FORCE ROW LEVEL SECURITY;
ALTER TABLE legal_hold_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE legal_hold_records FORCE ROW LEVEL SECURITY;
ALTER TABLE data_governance_event_chain_heads ENABLE ROW LEVEL SECURITY;
ALTER TABLE data_governance_event_chain_heads FORCE ROW LEVEL SECURITY;
ALTER TABLE data_governance_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE data_governance_events FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_data_governance_policies_all ON tenant_data_governance_policies
    FOR ALL USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY retention_schedules_all ON retention_schedules
    FOR ALL USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY record_retention_assignments_all ON record_retention_assignments
    FOR ALL USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY retention_exceptions_all ON retention_exceptions
    FOR ALL USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY legal_hold_reference_sequences_all ON legal_hold_reference_sequences
    FOR ALL USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY legal_holds_all ON legal_holds
    FOR ALL USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY legal_hold_custodians_all ON legal_hold_custodians
    FOR ALL USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY legal_hold_records_all ON legal_hold_records
    FOR ALL USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY data_governance_event_chain_heads_select ON data_governance_event_chain_heads
    FOR SELECT USING (organization_id = get_current_tenant());
CREATE POLICY data_governance_events_select ON data_governance_events
    FOR SELECT USING (organization_id = get_current_tenant());
CREATE POLICY data_governance_events_insert ON data_governance_events
    FOR INSERT WITH CHECK (organization_id = get_current_tenant());

COMMENT ON TABLE tenant_data_governance_policies IS
    'Versioned tenant residency, transfer, retention, approval, and processor-confirmation policy.';
COMMENT ON TABLE retention_schedules IS
    'Change-controlled record schedules resolved by record type, classification, jurisdiction, priority, and effective date.';
COMMENT ON TABLE record_retention_assignments IS
    'Materialised per-record retention and disposition decision used by database deletion guards.';
COMMENT ON TABLE retention_exceptions IS
    'Reasoned, approved extensions or exceptions to a materialised retention assignment.';
COMMENT ON TABLE legal_holds IS
    'Legal or regulatory matters that suspend disposal for explicit records and custodians.';
COMMENT ON TABLE legal_hold_records IS
    'Explicit record preservation scope; active rows are enforced by database deletion triggers.';
COMMENT ON TABLE data_governance_events IS
    'Append-only SHA-256 hash-chained history for data governance, retention, disposition, and hold decisions.';
