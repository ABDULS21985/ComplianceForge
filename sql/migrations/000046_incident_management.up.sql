-- Migration 046: Enterprise incident management
--
-- Earlier releases shipped incident API stubs and several read-side queries,
-- but no canonical incidents table. This migration establishes the source of
-- truth those contracts require, including an immutable timeline, assignment
-- history, GDPR Article 33 tracking, optimistic concurrency, and FORCE RLS.

CREATE TABLE incident_reference_sequences (
    organization_id       UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    next_incident_number  BIGINT NOT NULL DEFAULT 1 CHECK (next_incident_number > 0),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Composite user references make it impossible to attach a responder from a
-- different tenant even if a future caller bypasses repository validation.
CREATE UNIQUE INDEX uq_users_org_id_incident_fk ON users (organization_id, id);

CREATE TABLE incidents (
    id                           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id              UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    incident_ref                 VARCHAR(30) NOT NULL,
    title                        VARCHAR(200) NOT NULL,
    description                  TEXT NOT NULL,
    category                     VARCHAR(100) NOT NULL,
    severity                     VARCHAR(20) NOT NULL,
    status                       VARCHAR(30) NOT NULL DEFAULT 'reported',
    reporter_id                  UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    assigned_to                  UUID REFERENCES users(id) ON DELETE RESTRICT,
    detected_at                  TIMESTAMPTZ NOT NULL,
    reported_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    occurred_at                  TIMESTAMPTZ,
    triaged_at                   TIMESTAMPTZ,
    investigation_started_at     TIMESTAMPTZ,
    contained_at                 TIMESTAMPTZ,
    resolved_at                  TIMESTAMPTZ,
    closed_at                    TIMESTAMPTZ,
    cancelled_at                 TIMESTAMPTZ,
    cancellation_reason          TEXT,
    reopened_at                  TIMESTAMPTZ,
    root_cause                   TEXT NOT NULL DEFAULT '',
    impact                       TEXT NOT NULL DEFAULT '',
    lessons_learned              TEXT NOT NULL DEFAULT '',
    related_asset_id             UUID,
    followup_date                DATE,

    -- GDPR Article 33 assessment and supervisory-authority notification.
    is_data_breach               BOOLEAN NOT NULL DEFAULT FALSE,
    breach_assessment_status     VARCHAR(30) NOT NULL DEFAULT 'pending',
    is_breach_notifiable         BOOLEAN NOT NULL DEFAULT FALSE,
    breach_assessment_reason     TEXT,
    breach_assessed_at           TIMESTAMPTZ,
    breach_assessed_by           UUID REFERENCES users(id) ON DELETE RESTRICT,
    breach_awareness_at          TIMESTAMPTZ,
    notification_deadline        TIMESTAMPTZ,
    data_subjects_affected       INTEGER,
    records_affected             BIGINT,
    data_categories              TEXT[] NOT NULL DEFAULT '{}',
    special_category_data        BOOLEAN NOT NULL DEFAULT FALSE,
    cross_border                 BOOLEAN NOT NULL DEFAULT FALSE,
    breach_nature                TEXT NOT NULL DEFAULT '',
    likely_consequences          TEXT NOT NULL DEFAULT '',
    mitigation_measures          TEXT NOT NULL DEFAULT '',
    dpa_notified_at              TIMESTAMPTZ,
    dpa_notification_reference   VARCHAR(200),
    dpa_notification_reason      TEXT,
    dpa_notification_key         UUID,

    version                      BIGINT NOT NULL DEFAULT 1,
    retention_until              TIMESTAMPTZ,
    legal_hold                   BOOLEAN NOT NULL DEFAULT FALSE,
    metadata                     JSONB NOT NULL DEFAULT '{}',
    search_vector                TSVECTOR GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(incident_ref, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(description, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(category, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(root_cause, '')), 'C') ||
        setweight(to_tsvector('english', coalesce(impact, '')), 'C')
    ) STORED,
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at                   TIMESTAMPTZ,

    CONSTRAINT uq_incidents_org_ref UNIQUE (organization_id, incident_ref),
    CONSTRAINT uq_incidents_org_id UNIQUE (organization_id, id),
    CONSTRAINT uq_incidents_dpa_key UNIQUE (organization_id, dpa_notification_key),
	CONSTRAINT fk_incidents_reporter_tenant FOREIGN KEY (organization_id, reporter_id)
		REFERENCES users(organization_id, id) ON DELETE RESTRICT,
	CONSTRAINT fk_incidents_assignee_tenant FOREIGN KEY (organization_id, assigned_to)
		REFERENCES users(organization_id, id) ON DELETE RESTRICT,
	CONSTRAINT fk_incidents_breach_assessor_tenant FOREIGN KEY (organization_id, breach_assessed_by)
		REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_incidents_title CHECK (title = BTRIM(title) AND length(title) BETWEEN 1 AND 200),
    CONSTRAINT chk_incidents_description CHECK (length(BTRIM(description)) BETWEEN 1 AND 20000),
    CONSTRAINT chk_incidents_category CHECK (category = BTRIM(category) AND length(category) BETWEEN 1 AND 100),
    CONSTRAINT chk_incidents_severity CHECK (severity IN ('critical', 'high', 'medium', 'low')),
    CONSTRAINT chk_incidents_status CHECK (status IN (
        'reported', 'triaged', 'investigating', 'contained', 'resolved', 'closed', 'cancelled'
    )),
    CONSTRAINT chk_incidents_event_times CHECK (
        (triaged_at IS NULL OR triaged_at >= reported_at) AND
        (investigation_started_at IS NULL OR investigation_started_at >= reported_at) AND
        (contained_at IS NULL OR contained_at >= reported_at) AND
        (resolved_at IS NULL OR resolved_at >= reported_at) AND
        (closed_at IS NULL OR closed_at >= reported_at) AND
        (cancelled_at IS NULL OR cancelled_at >= reported_at)
    ),
    CONSTRAINT chk_incidents_cancel_reason CHECK (
        status <> 'cancelled' OR length(BTRIM(coalesce(cancellation_reason, ''))) BETWEEN 3 AND 4000
    ),
    CONSTRAINT chk_incidents_breach_assessment_status CHECK (
        breach_assessment_status IN ('pending', 'not_notifiable', 'notifiable')
    ),
    CONSTRAINT chk_incidents_breach_assessment_consistency CHECK (
        (breach_assessment_status = 'pending' AND NOT is_breach_notifiable)
        OR (breach_assessment_status = 'not_notifiable' AND NOT is_breach_notifiable
            AND breach_assessed_at IS NOT NULL AND breach_assessed_by IS NOT NULL
            AND length(BTRIM(coalesce(breach_assessment_reason, ''))) >= 3)
        OR (breach_assessment_status = 'notifiable' AND is_data_breach AND is_breach_notifiable
            AND breach_assessed_at IS NOT NULL AND breach_assessed_by IS NOT NULL
            AND length(BTRIM(coalesce(breach_assessment_reason, ''))) >= 3
            AND breach_awareness_at IS NOT NULL AND notification_deadline IS NOT NULL)
    ),
    CONSTRAINT chk_incidents_gdpr_deadline CHECK (
        notification_deadline IS NULL
        OR notification_deadline = breach_awareness_at + INTERVAL '72 hours'
    ),
    CONSTRAINT chk_incidents_affected_counts CHECK (
        data_subjects_affected IS NULL OR data_subjects_affected >= 0
    ),
    CONSTRAINT chk_incidents_record_counts CHECK (records_affected IS NULL OR records_affected >= 0),
    CONSTRAINT chk_incidents_dpa_notification CHECK (
        (dpa_notified_at IS NULL AND dpa_notification_reference IS NULL
            AND dpa_notification_reason IS NULL AND dpa_notification_key IS NULL)
        OR (dpa_notified_at IS NOT NULL AND dpa_notification_key IS NOT NULL
            AND length(BTRIM(coalesce(dpa_notification_reference, ''))) BETWEEN 1 AND 200
            AND length(BTRIM(coalesce(dpa_notification_reason, ''))) BETWEEN 3 AND 4000)
    ),
    CONSTRAINT chk_incidents_version CHECK (version > 0),
    CONSTRAINT chk_incidents_metadata_object CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT chk_incidents_retention CHECK (retention_until IS NULL OR retention_until >= created_at)
);

CREATE TABLE incident_events (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    incident_id      UUID NOT NULL,
    event_type       VARCHAR(80) NOT NULL,
    actor_user_id    UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    from_status      VARCHAR(30),
    to_status        VARCHAR(30),
    summary          VARCHAR(500) NOT NULL,
    details          JSONB NOT NULL DEFAULT '{}',
    occurred_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_incident_events_incident FOREIGN KEY (organization_id, incident_id)
        REFERENCES incidents(organization_id, id) ON DELETE CASCADE,
	CONSTRAINT fk_incident_events_actor_tenant FOREIGN KEY (organization_id, actor_user_id)
		REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_incident_events_type CHECK (
        event_type = BTRIM(event_type) AND length(event_type) BETWEEN 1 AND 80
    ),
    CONSTRAINT chk_incident_events_summary CHECK (
        summary = BTRIM(summary) AND length(summary) BETWEEN 1 AND 500
    ),
    CONSTRAINT chk_incident_events_statuses CHECK (
        (from_status IS NULL OR from_status IN ('reported','triaged','investigating','contained','resolved','closed','cancelled'))
        AND (to_status IS NULL OR to_status IN ('reported','triaged','investigating','contained','resolved','closed','cancelled'))
    ),
    CONSTRAINT chk_incident_events_details_object CHECK (jsonb_typeof(details) = 'object')
);

CREATE TABLE incident_assignments (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id   UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    incident_id       UUID NOT NULL,
    assignee_user_id  UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    assignment_role   VARCHAR(30) NOT NULL DEFAULT 'investigator',
    assigned_by       UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    reason            TEXT NOT NULL,
    assigned_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    unassigned_at     TIMESTAMPTZ,
    unassigned_by     UUID REFERENCES users(id) ON DELETE RESTRICT,
    unassign_reason   TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_incident_assignments_incident FOREIGN KEY (organization_id, incident_id)
        REFERENCES incidents(organization_id, id) ON DELETE CASCADE,
	CONSTRAINT fk_incident_assignments_assignee_tenant FOREIGN KEY (organization_id, assignee_user_id)
		REFERENCES users(organization_id, id) ON DELETE RESTRICT,
	CONSTRAINT fk_incident_assignments_assigner_tenant FOREIGN KEY (organization_id, assigned_by)
		REFERENCES users(organization_id, id) ON DELETE RESTRICT,
	CONSTRAINT fk_incident_assignments_unassigner_tenant FOREIGN KEY (organization_id, unassigned_by)
		REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_incident_assignments_role CHECK (assignment_role IN ('primary', 'investigator', 'observer')),
    CONSTRAINT chk_incident_assignments_reason CHECK (length(BTRIM(reason)) BETWEEN 3 AND 2000),
    CONSTRAINT chk_incident_assignments_unassignment CHECK (
        (unassigned_at IS NULL AND unassigned_by IS NULL AND unassign_reason IS NULL)
        OR (unassigned_at IS NOT NULL AND unassigned_by IS NOT NULL
            AND length(BTRIM(coalesce(unassign_reason, ''))) BETWEEN 3 AND 2000
            AND unassigned_at >= assigned_at)
    )
);

CREATE UNIQUE INDEX uq_incident_active_assignment
    ON incident_assignments (organization_id, incident_id, assignee_user_id, assignment_role)
    WHERE unassigned_at IS NULL;
CREATE UNIQUE INDEX uq_incident_active_primary
    ON incident_assignments (organization_id, incident_id)
    WHERE assignment_role = 'primary' AND unassigned_at IS NULL;

CREATE INDEX idx_incidents_org_active ON incidents (organization_id, reported_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_incidents_org_status ON incidents (organization_id, status, severity, reported_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_incidents_org_assignee ON incidents (organization_id, assigned_to, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_incidents_org_breach_deadline ON incidents (organization_id, notification_deadline)
    WHERE deleted_at IS NULL AND is_breach_notifiable AND dpa_notified_at IS NULL;
CREATE INDEX idx_incidents_retention ON incidents (retention_until) WHERE deleted_at IS NOT NULL AND NOT legal_hold;
CREATE INDEX idx_incidents_search ON incidents USING GIN (search_vector);
CREATE INDEX idx_incident_events_timeline ON incident_events (organization_id, incident_id, occurred_at DESC, id DESC);
CREATE INDEX idx_incident_assignments_active ON incident_assignments (organization_id, incident_id, assigned_at DESC)
    WHERE unassigned_at IS NULL;

CREATE TRIGGER trg_incident_reference_sequences_updated_at
    BEFORE UPDATE ON incident_reference_sequences
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER trg_incidents_updated_at
    BEFORE UPDATE ON incidents
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER trg_incident_assignments_updated_at
    BEFORE UPDATE ON incident_assignments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE OR REPLACE FUNCTION severity_ord(value TEXT)
RETURNS INTEGER
LANGUAGE SQL IMMUTABLE PARALLEL SAFE
AS $$
    SELECT CASE lower(value)
        WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 WHEN 'low' THEN 4 ELSE 5
    END
$$;

-- FORCE RLS deliberately prevents the scheduler's unscoped pool connection
-- from inspecting incident content. This narrowly scoped function exposes
-- only tenant UUIDs that have breach deadlines in the scheduler's 48-hour
-- window; the scheduler must then enter each tenant context before reading
-- any incident row or publishing a notification event.
CREATE FUNCTION incident_due_tenants(requested_limit INTEGER DEFAULT 100)
RETURNS TABLE (organization_id UUID)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
    SELECT i.organization_id
    FROM public.incidents AS i
    WHERE i.is_breach_notifiable
      AND i.dpa_notified_at IS NULL
      AND i.notification_deadline IS NOT NULL
      AND i.notification_deadline <= statement_timestamp() + INTERVAL '48 hours'
      AND i.deleted_at IS NULL
      AND i.status NOT IN ('closed', 'cancelled')
    GROUP BY i.organization_id
    ORDER BY MIN(i.notification_deadline), i.organization_id
    LIMIT LEAST(GREATEST(COALESCE(requested_limit, 100), 1), 1000)
$$;

REVOKE ALL ON FUNCTION incident_due_tenants(INTEGER) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION incident_due_tenants(INTEGER) TO PUBLIC;

COMMENT ON FUNCTION incident_due_tenants(INTEGER) IS
    'Returns at most 1000 tenant UUIDs with an unreported GDPR breach deadline within 48 hours; incident content remains protected by FORCE RLS.';

ALTER TABLE incident_reference_sequences ENABLE ROW LEVEL SECURITY;
ALTER TABLE incident_reference_sequences FORCE ROW LEVEL SECURITY;
ALTER TABLE incidents ENABLE ROW LEVEL SECURITY;
ALTER TABLE incidents FORCE ROW LEVEL SECURITY;
ALTER TABLE incident_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE incident_events FORCE ROW LEVEL SECURITY;
ALTER TABLE incident_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE incident_assignments FORCE ROW LEVEL SECURITY;

CREATE POLICY incident_reference_sequences_tenant_all ON incident_reference_sequences
    FOR ALL USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY incidents_tenant_all ON incidents
    FOR ALL USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY incident_events_tenant_select ON incident_events
    FOR SELECT USING (organization_id = get_current_tenant());
CREATE POLICY incident_events_tenant_insert ON incident_events
    FOR INSERT WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY incident_assignments_tenant_all ON incident_assignments
    FOR ALL USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());

COMMENT ON TABLE incidents IS 'Tenant-isolated incident register with lifecycle, GDPR Article 33, retention, and optimistic-concurrency controls.';
COMMENT ON TABLE incident_events IS 'Append-only tenant-isolated incident timeline; UPDATE and DELETE are intentionally not granted to the API role.';
COMMENT ON TABLE incident_assignments IS 'Assignment history for incident owners, investigators, and observers.';
COMMENT ON TABLE incident_reference_sequences IS 'Atomic per-tenant counter for non-reused INC display references.';
