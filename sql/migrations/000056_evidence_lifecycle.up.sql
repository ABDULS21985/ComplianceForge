-- Migration 056: immutable evidence versions, reviewer sign-off, expiry,
-- chain-of-custody history, and legal-hold lifecycle evidence.
--
-- Evidence bytes remain private objects. This migration protects their
-- database metadata with a canonical SHA-256 fingerprint and records every
-- security-relevant lifecycle action in a per-evidence append-only hash chain.

BEGIN;

-- SECURITY DEFINER capabilities are owned by non-login, non-bypass roles.
-- A superuser is permitted to bootstrap these roles for local installations;
-- production migration identities must be pre-provisioned as members so role
-- ownership remains an explicit DBA decision.
DO $roles$
DECLARE
    migration_is_superuser BOOLEAN;
    role_record RECORD;
    required_role TEXT;
BEGIN
    SELECT role.rolsuper INTO migration_is_superuser
    FROM pg_catalog.pg_roles AS role WHERE role.rolname = current_user;

    FOREACH required_role IN ARRAY ARRAY[
        'complianceforge_evidence_chain_owner',
        'complianceforge_evidence_registry_owner'
    ] LOOP
        IF NOT EXISTS (
            SELECT 1 FROM pg_catalog.pg_roles AS role WHERE role.rolname = required_role
        ) THEN
            IF NOT migration_is_superuser THEN
                RAISE EXCEPTION '% must be pre-provisioned as NOLOGIN/NOSUPERUSER/NOBYPASSRLS and granted to migration role %',
                    required_role, current_user USING ERRCODE = '42501';
            END IF;
            EXECUTE format(
                'CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS',
                required_role
            );
        END IF;

        SELECT role.* INTO role_record
        FROM pg_catalog.pg_roles AS role WHERE role.rolname = required_role;
        IF role_record.rolcanlogin OR role_record.rolsuper OR role_record.rolbypassrls
           OR role_record.rolcreatedb OR role_record.rolcreaterole
           OR role_record.rolreplication OR role_record.rolinherit THEN
            RAISE EXCEPTION '% must be NOLOGIN, NOSUPERUSER, NOBYPASSRLS, NOCREATEDB, NOCREATEROLE, NOREPLICATION, and NOINHERIT', required_role
                USING ERRCODE = '42501';
        END IF;
        IF NOT migration_is_superuser
           AND NOT pg_catalog.pg_has_role(current_user, required_role, 'SET') THEN
            RAISE EXCEPTION 'migration role % must be permitted to SET ROLE % to transfer ownership',
                current_user, required_role USING ERRCODE = '42501';
        END IF;
    END LOOP;
END;
$roles$;

-- PostgreSQL requires a prospective function/table owner to have CREATE on
-- the containing schema. Keep that grant only for the ownership-transfer
-- window; neither capability owner may create arbitrary public objects after
-- migration completion.
GRANT USAGE, CREATE ON SCHEMA public
    TO complianceforge_evidence_chain_owner,
       complianceforge_evidence_registry_owner;

-- Migrations own these legacy relations but FORCE RLS would hide populated
-- tenants from a non-bypass owner. Removing FORCE (RLS remains enabled) under
-- ACCESS EXCLUSIVE migration locks makes the backfill complete; every relation
-- is re-forced before this atomic migration commits.
ALTER TABLE organizations NO FORCE ROW LEVEL SECURITY;
ALTER TABLE users NO FORCE ROW LEVEL SECURITY;
ALTER TABLE control_implementations NO FORCE ROW LEVEL SECURITY;
ALTER TABLE control_evidence NO FORCE ROW LEVEL SECURITY;
ALTER TABLE evidence_collection_configs NO FORCE ROW LEVEL SECURITY;
ALTER TABLE evidence_collection_runs NO FORCE ROW LEVEL SECURITY;
ALTER TABLE legal_holds NO FORCE ROW LEVEL SECURITY;
ALTER TABLE legal_hold_records NO FORCE ROW LEVEL SECURITY;

ALTER TABLE control_evidence
    ADD COLUMN series_id UUID,
    ADD COLUMN version_number INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN supersedes_evidence_id UUID,
    ADD COLUMN superseded_by_evidence_id UUID,
    ADD COLUMN superseded_at TIMESTAMPTZ,
    ADD COLUMN lifecycle_status VARCHAR(16),
    ADD COLUMN expires_at TIMESTAMPTZ,
    ADD COLUMN version_reason VARCHAR(1000) NOT NULL DEFAULT 'Initial evidence version',
    ADD COLUMN content_fingerprint CHAR(64);

-- Legacy rows cannot be linked into version families without guessing user
-- intent. Each becomes a one-record series. Previously non-current records are
-- retained as explicitly labelled legacy supersessions; expired rows retain
-- their more specific expiry state.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM control_evidence
        WHERE review_status = 'expired' AND is_current
    ) THEN
        RAISE EXCEPTION 'legacy expired evidence cannot be marked current; reconcile it before migration 056'
            USING ERRCODE = '23514';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM control_evidence AS evidence
        JOIN evidence_collection_runs AS run
          ON run.organization_id = evidence.organization_id
         AND run.evidence_id = evidence.id
        WHERE run.status = 'success'
          AND COALESCE(evidence.metadata, '{}'::JSONB) ? 'automated_proof'
    ) THEN
        RAISE EXCEPTION 'legacy evidence metadata already uses reserved automated_proof key; reconcile before migration 056'
            USING ERRCODE = '23514';
    END IF;
END;
$$;

UPDATE control_evidence
SET series_id = id,
    lifecycle_status = CASE
        WHEN review_status = 'expired' THEN 'expired'
        WHEN NOT is_current THEN 'superseded'
        ELSE 'active'
    END,
    superseded_at = CASE
        WHEN NOT is_current AND review_status <> 'expired'
            THEN COALESCE(updated_at, collected_at)
        ELSE NULL
    END,
    expires_at = CASE WHEN valid_until IS NULL THEN NULL
        ELSE ((valid_until + 1)::TIMESTAMP AT TIME ZONE 'UTC') END,
    version_reason = CASE
        WHEN NOT is_current AND review_status <> 'expired'
            THEN 'Migrated legacy non-current evidence'
        ELSE 'Migrated initial evidence version'
    END;

-- Hash an explicit, versioned allow-list of immutable evidence content and
-- provenance. Timestamp instants are converted to Unix seconds so the digest
-- is independent of session TimeZone; future columns cannot silently alter the
-- canonical document. Mutable projections have their own custody events.
CREATE FUNCTION calculate_control_evidence_fingerprint(evidence_document JSONB)
RETURNS TEXT
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
SET search_path = pg_catalog, public
AS $$
    SELECT encode(public.digest(convert_to(jsonb_build_object(
        'schema', 'control-evidence/v1',
        'id', evidence_document->>'id',
        'organization_id', evidence_document->>'organization_id',
        'control_implementation_id', evidence_document->>'control_implementation_id',
        'title', evidence_document->'title',
        'description', evidence_document->'description',
        'evidence_type', evidence_document->'evidence_type',
        'file_path', evidence_document->'file_path',
        'file_name', evidence_document->'file_name',
        'file_size_bytes', evidence_document->'file_size_bytes',
        'mime_type', evidence_document->'mime_type',
        'file_hash', evidence_document->'file_hash',
        'collection_method', evidence_document->'collection_method',
        'collected_at_epoch', extract(epoch FROM NULLIF(evidence_document->>'collected_at', '')::TIMESTAMPTZ),
        'collected_by', evidence_document->'collected_by',
        'valid_from', evidence_document->'valid_from',
        'valid_until', evidence_document->'valid_until',
        'metadata', evidence_document->'metadata',
        'created_at_epoch', extract(epoch FROM NULLIF(evidence_document->>'created_at', '')::TIMESTAMPTZ),
        'series_id', evidence_document->>'series_id',
        'version_number', evidence_document->'version_number',
        'supersedes_evidence_id', evidence_document->'supersedes_evidence_id',
        'expires_at_epoch', extract(epoch FROM NULLIF(evidence_document->>'expires_at', '')::TIMESTAMPTZ),
        'version_reason', evidence_document->'version_reason'
    )::TEXT, 'UTF8'), 'sha256'), 'hex')
$$;

-- Bind legacy automated proof rows before calculating the immutable evidence
-- fingerprint. Multiple successful runs pointing at one evidence row are
-- ambiguous and must be reconciled instead of selecting one nondeterministically.
DO $$
BEGIN
    IF EXISTS (
        SELECT run.organization_id, run.evidence_id
        FROM evidence_collection_runs AS run
        WHERE run.status = 'success' AND run.evidence_id IS NOT NULL
        GROUP BY run.organization_id, run.evidence_id
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'multiple successful collection runs reference one evidence record; reconcile before migration 056'
            USING ERRCODE = '23514';
    END IF;
END;
$$;

UPDATE control_evidence AS evidence
SET metadata = COALESCE(evidence.metadata, '{}'::JSONB) || jsonb_build_object(
    'automated_proof', jsonb_build_object(
        'proof_schema', 'automated-evidence/postgresql-jsonb-v1',
        'collection_run_id', run.id,
        'collected_data_sha256', encode(public.digest(
            convert_to(COALESCE(run.collected_data, 'null'::JSONB)::TEXT, 'UTF8'), 'sha256'
        ), 'hex'),
        'validation_results_sha256', encode(public.digest(
            convert_to(COALESCE(run.validation_results, 'null'::JSONB)::TEXT, 'UTF8'), 'sha256'
        ), 'hex'),
        'all_criteria_passed', run.all_criteria_passed
    )
)
FROM evidence_collection_runs AS run
WHERE run.organization_id = evidence.organization_id
  AND run.evidence_id = evidence.id
  AND run.status = 'success';

UPDATE control_evidence AS evidence
SET content_fingerprint = calculate_control_evidence_fingerprint(to_jsonb(evidence));

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM control_evidence AS evidence
        JOIN users AS account ON account.id = evidence.collected_by
        WHERE account.organization_id <> evidence.organization_id
    ) OR EXISTS (
        SELECT 1 FROM control_evidence AS evidence
        JOIN users AS account ON account.id = evidence.reviewed_by
        WHERE account.organization_id <> evidence.organization_id
    ) OR EXISTS (
        SELECT 1 FROM control_evidence AS evidence
        JOIN control_implementations AS implementation
          ON implementation.id = evidence.control_implementation_id
        WHERE implementation.organization_id <> evidence.organization_id
    ) OR EXISTS (
        SELECT 1 FROM evidence_collection_configs AS config
        JOIN control_implementations AS implementation
          ON implementation.id = config.control_implementation_id
        WHERE implementation.organization_id <> config.organization_id
    ) OR EXISTS (
        SELECT 1 FROM evidence_collection_runs AS run
        JOIN evidence_collection_configs AS config ON config.id = run.config_id
        JOIN control_implementations AS implementation
          ON implementation.id = run.control_implementation_id
        LEFT JOIN control_evidence AS evidence ON evidence.id = run.evidence_id
        WHERE config.organization_id <> run.organization_id
           OR implementation.organization_id <> run.organization_id
           OR (run.evidence_id IS NOT NULL AND evidence.organization_id <> run.organization_id)
    ) THEN
        RAISE EXCEPTION 'cross-tenant legacy evidence relationships require manual remediation';
    END IF;
END;
$$;

-- Bind evidence to an implementation in the same tenant. The legacy
-- single-column foreign key proved only that an implementation UUID existed.
ALTER TABLE control_implementations
    ADD CONSTRAINT uq_control_implementations_org_id UNIQUE (organization_id, id);

ALTER TABLE control_evidence
    ALTER COLUMN series_id SET NOT NULL,
    ALTER COLUMN lifecycle_status SET NOT NULL,
    ALTER COLUMN lifecycle_status SET DEFAULT 'active',
    ALTER COLUMN content_fingerprint SET NOT NULL,
    ADD CONSTRAINT uq_control_evidence_org_id UNIQUE (organization_id, id),
    ADD CONSTRAINT uq_control_evidence_org_id_series UNIQUE (organization_id, id, series_id),
    DROP CONSTRAINT control_evidence_control_implementation_id_fkey,
    DROP CONSTRAINT control_evidence_collected_by_fkey,
    DROP CONSTRAINT control_evidence_reviewed_by_fkey,
    ADD CONSTRAINT fk_control_evidence_implementation_tenant
        FOREIGN KEY (organization_id, control_implementation_id)
        REFERENCES control_implementations(organization_id, id)
        ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT fk_control_evidence_collector_tenant FOREIGN KEY (organization_id, collected_by)
        REFERENCES users(organization_id, id) DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT fk_control_evidence_reviewer_tenant FOREIGN KEY (organization_id, reviewed_by)
        REFERENCES users(organization_id, id) DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT fk_control_evidence_series FOREIGN KEY (organization_id, series_id)
        REFERENCES control_evidence(organization_id, id)
        DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT fk_control_evidence_predecessor FOREIGN KEY (organization_id, supersedes_evidence_id)
        REFERENCES control_evidence(organization_id, id)
        DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT fk_control_evidence_successor FOREIGN KEY (organization_id, superseded_by_evidence_id)
        REFERENCES control_evidence(organization_id, id)
        DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT chk_control_evidence_version CHECK (version_number > 0),
    ADD CONSTRAINT chk_control_evidence_version_reason CHECK (
        version_reason = BTRIM(version_reason) AND length(version_reason) BETWEEN 3 AND 1000
    ),
    ADD CONSTRAINT chk_control_evidence_fingerprint CHECK (
        content_fingerprint ~ '^[0-9a-f]{64}$'
    ),
    ADD CONSTRAINT chk_control_evidence_lifecycle_status CHECK (
        lifecycle_status IN ('active','superseded','expired')
    ),
    ADD CONSTRAINT chk_control_evidence_version_links CHECK (
        (supersedes_evidence_id IS NULL OR supersedes_evidence_id <> id)
        AND (superseded_by_evidence_id IS NULL OR superseded_by_evidence_id <> id)
        AND (supersedes_evidence_id IS NULL OR supersedes_evidence_id <> superseded_by_evidence_id)
    ),
    ADD CONSTRAINT chk_control_evidence_expiry_projection CHECK (
        (valid_until IS NULL AND expires_at IS NULL)
        OR (
            valid_until IS NOT NULL AND expires_at IS NOT NULL
            AND expires_at = ((valid_until + 1)::TIMESTAMP AT TIME ZONE 'UTC')
        )
    ),
    ADD CONSTRAINT chk_control_evidence_lifecycle_shape CHECK (
        (NOT is_current OR lifecycle_status = 'active')
        AND (lifecycle_status <> 'active' OR review_status <> 'expired')
        AND (
            (lifecycle_status = 'active' AND superseded_by_evidence_id IS NULL AND superseded_at IS NULL)
            OR (lifecycle_status = 'superseded' AND NOT is_current AND superseded_at IS NOT NULL)
            OR (lifecycle_status = 'expired' AND NOT is_current AND review_status = 'expired')
        )
    );

ALTER TABLE evidence_collection_configs
    ADD CONSTRAINT uq_evidence_collection_configs_org_id UNIQUE (organization_id, id),
    DROP CONSTRAINT evidence_collection_configs_control_implementation_id_fkey,
    ADD CONSTRAINT fk_evidence_collection_config_implementation_tenant
        FOREIGN KEY (organization_id, control_implementation_id)
        REFERENCES control_implementations(organization_id, id)
        ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE evidence_collection_runs
    DROP CONSTRAINT evidence_collection_runs_config_id_fkey,
    DROP CONSTRAINT evidence_collection_runs_control_implementation_id_fkey,
    DROP CONSTRAINT evidence_collection_runs_evidence_id_fkey,
    ADD CONSTRAINT fk_evidence_collection_run_config_tenant
        FOREIGN KEY (organization_id, config_id)
        REFERENCES evidence_collection_configs(organization_id, id)
        ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT fk_evidence_collection_run_implementation_tenant
        FOREIGN KEY (organization_id, control_implementation_id)
        REFERENCES control_implementations(organization_id, id)
        ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT fk_evidence_collection_run_evidence_tenant
        FOREIGN KEY (organization_id, evidence_id)
        REFERENCES control_evidence(organization_id, id)
        ON DELETE SET NULL (evidence_id) DEFERRABLE INITIALLY DEFERRED;

CREATE UNIQUE INDEX uq_evidence_collection_runs_active_config
    ON evidence_collection_runs (organization_id, config_id)
    WHERE status IN ('scheduled', 'running');

-- A completed collection run is the structured proof behind automated
-- evidence. Permit the normal scheduled/running terminal transitions, then
-- make the result immutable. Referential cleanup remains possible only after
-- the relevant parent has actually disappeared in the same cascade.
CREATE FUNCTION enforce_evidence_collection_run_immutability()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF NOT EXISTS (
            SELECT 1 FROM public.evidence_collection_configs AS config
            WHERE config.organization_id = OLD.organization_id
              AND config.id = OLD.config_id
        ) THEN
            RETURN OLD;
        END IF;
        RAISE EXCEPTION 'evidence collection run history cannot be deleted'
            USING ERRCODE = '55000';
    END IF;

    IF ROW(NEW.id, NEW.organization_id, NEW.config_id,
           NEW.control_implementation_id, NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.id, OLD.organization_id, OLD.config_id,
           OLD.control_implementation_id, OLD.created_at) THEN
        RAISE EXCEPTION 'evidence collection run identity is immutable'
            USING ERRCODE = '55000';
    END IF;

    IF OLD.status IN ('success', 'failed', 'timeout', 'validation_failed') THEN
        IF OLD.evidence_id IS NOT NULL AND NEW.evidence_id IS NULL
           AND ROW(NEW.status, NEW.started_at, NEW.completed_at, NEW.duration_ms,
                   NEW.collected_data, NEW.validation_results,
                   NEW.all_criteria_passed, NEW.error_message, NEW.metadata)
               IS NOT DISTINCT FROM
               ROW(OLD.status, OLD.started_at, OLD.completed_at, OLD.duration_ms,
                   OLD.collected_data, OLD.validation_results,
                   OLD.all_criteria_passed, OLD.error_message, OLD.metadata)
           AND NOT EXISTS (
               SELECT 1 FROM public.control_evidence AS evidence
               WHERE evidence.organization_id = OLD.organization_id
                 AND evidence.id = OLD.evidence_id
           ) THEN
            RETURN NEW;
        END IF;
        IF to_jsonb(NEW) IS DISTINCT FROM to_jsonb(OLD) THEN
            RAISE EXCEPTION 'completed evidence collection runs are immutable'
                USING ERRCODE = '55000';
        END IF;
        RETURN NEW;
    END IF;

    IF NEW.status IN ('success', 'validation_failed', 'failed', 'timeout') THEN
        IF NEW.completed_at IS NULL OR NEW.duration_ms IS NULL OR NEW.duration_ms < 0 THEN
            RAISE EXCEPTION 'terminal evidence collection runs require completion timing'
                USING ERRCODE = '23514';
        END IF;
        IF NEW.status = 'success'
           AND (NEW.collected_data IS NULL OR NEW.validation_results IS NULL
                OR NEW.all_criteria_passed IS DISTINCT FROM TRUE
                OR NEW.evidence_id IS NULL OR NEW.error_message IS NOT NULL) THEN
            RAISE EXCEPTION 'successful evidence collection run is incomplete'
                USING ERRCODE = '23514';
        ELSIF NEW.status = 'validation_failed'
              AND (NEW.collected_data IS NULL OR NEW.validation_results IS NULL
                   OR NEW.all_criteria_passed IS DISTINCT FROM FALSE
                   OR NEW.evidence_id IS NOT NULL OR NEW.error_message IS NOT NULL) THEN
            RAISE EXCEPTION 'failed validation evidence collection run is inconsistent'
                USING ERRCODE = '23514';
        ELSIF NEW.status IN ('failed', 'timeout')
              AND (NEW.evidence_id IS NOT NULL
                   OR length(BTRIM(COALESCE(NEW.error_message, ''))) < 1) THEN
            RAISE EXCEPTION 'failed evidence collection run requires an error and no evidence'
                USING ERRCODE = '23514';
        END IF;
    ELSIF NEW.status NOT IN ('scheduled', 'running') THEN
        RAISE EXCEPTION 'invalid evidence collection run transition'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_evidence_collection_runs_immutable
    BEFORE UPDATE OR DELETE ON evidence_collection_runs
    FOR EACH ROW EXECUTE FUNCTION enforce_evidence_collection_run_immutability();

CREATE UNIQUE INDEX uq_control_evidence_series_version
    ON control_evidence (organization_id, series_id, version_number);
CREATE UNIQUE INDEX uq_control_evidence_series_current
    ON control_evidence (organization_id, series_id)
    WHERE is_current AND deleted_at IS NULL;
CREATE INDEX idx_control_evidence_lifecycle
    ON control_evidence (organization_id, lifecycle_status, collected_at DESC, id DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_control_evidence_expiry_due
    ON control_evidence (expires_at, organization_id, id)
    WHERE lifecycle_status = 'active' AND is_current AND deleted_at IS NULL
      AND expires_at IS NOT NULL;

CREATE TABLE evidence_reviews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    evidence_id UUID NOT NULL,
    decision VARCHAR(16) NOT NULL,
    comment TEXT,
    reviewer_id UUID NOT NULL,
    evidence_sha256 CHAR(64) NOT NULL,
    request_id VARCHAR(160),
    metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_evidence_reviews_evidence FOREIGN KEY (organization_id, evidence_id)
        REFERENCES control_evidence(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_evidence_reviews_reviewer FOREIGN KEY (organization_id, reviewer_id)
        REFERENCES users(organization_id, id) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT chk_evidence_reviews_decision CHECK (decision IN ('accepted','rejected')),
    CONSTRAINT chk_evidence_reviews_comment CHECK (
        (decision = 'rejected' AND comment IS NOT NULL
            AND comment = BTRIM(comment) AND length(comment) BETWEEN 1 AND 4000)
        OR (decision = 'accepted' AND (
            comment IS NULL OR (comment = BTRIM(comment) AND length(comment) BETWEEN 1 AND 4000)
        ))
    ),
    CONSTRAINT chk_evidence_reviews_fingerprint CHECK (evidence_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_evidence_reviews_request CHECK (
        request_id IS NULL OR (
            request_id = BTRIM(request_id) AND length(request_id) BETWEEN 1 AND 160
            AND request_id !~ E'[\\r\\n]'
        )
    ),
    CONSTRAINT chk_evidence_reviews_metadata CHECK (
        jsonb_typeof(metadata) = 'object' AND octet_length(metadata::TEXT) <= 16384
    )
);

CREATE INDEX idx_evidence_reviews_history
    ON evidence_reviews (organization_id, evidence_id, created_at DESC, id DESC);
CREATE UNIQUE INDEX uq_evidence_reviews_request
    ON evidence_reviews (organization_id, evidence_id, request_id)
    WHERE request_id IS NOT NULL;

CREATE TABLE evidence_custody_chain_heads (
    organization_id UUID NOT NULL,
    evidence_id UUID NOT NULL,
    series_id UUID NOT NULL,
    last_sequence BIGINT NOT NULL DEFAULT 0 CHECK (last_sequence >= 0),
    last_hash BYTEA NOT NULL CHECK (octet_length(last_hash) = 32),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, evidence_id),
    CONSTRAINT fk_evidence_custody_head_evidence
        FOREIGN KEY (organization_id, evidence_id, series_id)
        REFERENCES control_evidence(organization_id, id, series_id) ON DELETE CASCADE
);

CREATE TABLE evidence_custody_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    evidence_id UUID NOT NULL,
    series_id UUID NOT NULL,
    chain_sequence BIGINT NOT NULL CHECK (chain_sequence > 0),
    previous_hash BYTEA NOT NULL CHECK (octet_length(previous_hash) = 32),
    event_hash BYTEA NOT NULL CHECK (octet_length(event_hash) = 32),
    event_type VARCHAR(32) NOT NULL,
    actor_user_id UUID,
    actor_type VARCHAR(16) NOT NULL,
    reason TEXT NOT NULL,
    object_sha256 VARCHAR(64),
    request_id VARCHAR(160),
    details JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_evidence_custody_sequence UNIQUE (organization_id, evidence_id, chain_sequence),
    CONSTRAINT fk_evidence_custody_event_evidence
        FOREIGN KEY (organization_id, evidence_id, series_id)
        REFERENCES control_evidence(organization_id, id, series_id) ON DELETE CASCADE,
    CONSTRAINT fk_evidence_custody_event_actor FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES users(organization_id, id) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT chk_evidence_custody_event_type CHECK (event_type IN (
        'uploaded','download_authorized','reviewed','superseded','expired','deleted',
        'integrity_verified','integrity_failed','legal_hold_placed','legal_hold_released'
    )),
    CONSTRAINT chk_evidence_custody_actor CHECK (
        (actor_type = 'user' AND actor_user_id IS NOT NULL)
        OR (actor_type = 'system' AND actor_user_id IS NULL)
    ),
    CONSTRAINT chk_evidence_custody_reason CHECK (
        reason = BTRIM(reason) AND length(reason) BETWEEN 3 AND 2000
    ),
    CONSTRAINT chk_evidence_custody_object_hash CHECK (
        object_sha256 IS NULL OR object_sha256 ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_evidence_custody_request CHECK (
        request_id IS NULL OR (
            request_id = BTRIM(request_id) AND length(request_id) BETWEEN 1 AND 160
            AND request_id !~ E'[\\r\\n]'
        )
    ),
    CONSTRAINT chk_evidence_custody_details CHECK (
        jsonb_typeof(details) = 'object' AND octet_length(details::TEXT) <= 16384
    )
);

CREATE INDEX idx_evidence_custody_history
    ON evidence_custody_events (organization_id, evidence_id, chain_sequence);
CREATE INDEX idx_evidence_custody_series
    ON evidence_custody_events (organization_id, series_id, created_at DESC, id DESC);

CREATE FUNCTION calculate_evidence_custody_event_hash(
    custody_id UUID,
    custody_organization_id UUID,
    custody_evidence_id UUID,
    custody_series_id UUID,
    custody_sequence BIGINT,
    custody_previous_hash BYTEA,
    custody_event_type TEXT,
    custody_actor_user_id UUID,
    custody_actor_type TEXT,
    custody_reason TEXT,
    custody_object_sha256 TEXT,
    custody_request_id TEXT,
    custody_details JSONB,
    custody_created_at TIMESTAMPTZ
)
RETURNS BYTEA
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
SET search_path = pg_catalog, public
AS $$
    -- jsonb has a canonical textual representation and preserves nulls and
    -- field boundaries, avoiding delimiter collisions in attacker-controlled
    -- reason, request-id, and metadata values.
    SELECT public.digest(convert_to(jsonb_build_object(
        'schema', 'evidence-custody/v1',
        'id', custody_id,
        'organization_id', custody_organization_id,
        'evidence_id', custody_evidence_id,
        'series_id', custody_series_id,
        'sequence', custody_sequence,
        'previous_hash', encode(custody_previous_hash, 'hex'),
        'event_type', custody_event_type,
        'actor_user_id', custody_actor_user_id,
        'actor_type', custody_actor_type,
        'reason', custody_reason,
        'object_sha256', custody_object_sha256,
        'request_id', custody_request_id,
        'details', custody_details,
        'created_at', to_char(
            custody_created_at AT TIME ZONE 'UTC',
            'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'
        )
    )::TEXT, 'UTF8'), 'sha256')
$$;

-- Start every pre-existing record with a deterministic migration event. The
-- legal-hold count captures the hold state observed at the trust boundary;
-- subsequent placements and releases receive their own events below.
WITH event_seed AS (
    SELECT gen_random_uuid() AS event_id,
           evidence.organization_id,
           evidence.id AS evidence_id,
           evidence.series_id,
           evidence.collected_by AS actor_user_id,
           CASE WHEN evidence.collected_by IS NULL THEN 'system' ELSE 'user' END AS actor_type,
           CASE WHEN evidence.file_hash ~* '^[0-9a-f]{64}$' THEN lower(evidence.file_hash) END AS object_sha256,
           jsonb_build_object(
               'migration', '056',
               'content_fingerprint', evidence.content_fingerprint,
               'version_number', evidence.version_number,
               'lifecycle_status', evidence.lifecycle_status,
               'is_current', evidence.is_current,
               'superseded_by_evidence_id', evidence.superseded_by_evidence_id,
               'superseded_at', evidence.superseded_at,
               'review_status', evidence.review_status,
               'reviewed_by', evidence.reviewed_by,
               'reviewed_at', evidence.reviewed_at,
               'review_notes', evidence.review_notes,
               'deleted_at', evidence.deleted_at,
               'active_legal_hold_count', (
                   SELECT count(*) FROM legal_hold_records AS held
                   JOIN legal_holds AS hold
                     ON hold.organization_id = held.organization_id AND hold.id = held.hold_id
                   WHERE held.organization_id = evidence.organization_id
                     AND held.record_type = 'evidence' AND held.record_id = evidence.id
                     AND held.released_at IS NULL AND hold.status = 'active'
               )
           ) AS details,
           evidence.collected_at AS created_at
    FROM control_evidence AS evidence
), event_values AS (
    SELECT event_id, organization_id, evidence_id, series_id,
           1::BIGINT AS chain_sequence,
           decode(repeat('00', 32), 'hex') AS previous_hash,
           'uploaded'::TEXT AS event_type,
           actor_user_id, actor_type,
           'Evidence record migrated into immutable custody history'::TEXT AS reason,
           object_sha256, NULL::TEXT AS request_id, details, created_at
    FROM event_seed
)
INSERT INTO evidence_custody_events (
    id, organization_id, evidence_id, series_id, chain_sequence, previous_hash,
    event_hash, event_type, actor_user_id, actor_type, reason, object_sha256,
    request_id, details, created_at
)
SELECT event_id, organization_id, evidence_id, series_id, chain_sequence, previous_hash,
       calculate_evidence_custody_event_hash(
           event_id, organization_id, evidence_id, series_id, chain_sequence,
           previous_hash, event_type, actor_user_id, actor_type, reason,
           object_sha256, request_id, details, created_at
       ),
       event_type, actor_user_id, actor_type, reason, object_sha256,
       request_id, details, created_at
FROM event_values;

INSERT INTO evidence_custody_chain_heads (
    organization_id, evidence_id, series_id, last_sequence, last_hash, updated_at
)
SELECT organization_id, evidence_id, series_id, chain_sequence, event_hash, created_at
FROM evidence_custody_events;

CREATE FUNCTION append_evidence_custody_event_chain()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    current_tenant UUID;
    actual_series UUID;
    prior_sequence BIGINT;
    prior_hash BYTEA;
BEGIN
    current_tenant := public.get_current_tenant();
    IF current_tenant IS NULL OR current_tenant <> NEW.organization_id THEN
        RAISE EXCEPTION 'evidence custody tenant does not match request tenant'
            USING ERRCODE = '42501';
    END IF;

    SELECT evidence.series_id INTO actual_series
    FROM public.control_evidence AS evidence
    WHERE evidence.organization_id = NEW.organization_id
      AND evidence.id = NEW.evidence_id;
    IF NOT FOUND OR actual_series <> NEW.series_id THEN
        RAISE EXCEPTION 'evidence custody target is invalid' USING ERRCODE = '23503';
    END IF;

    NEW.created_at := statement_timestamp();
    NEW.reason := BTRIM(NEW.reason);
    NEW.request_id := NULLIF(BTRIM(NEW.request_id), '');

    INSERT INTO public.evidence_custody_chain_heads (
        organization_id, evidence_id, series_id, last_sequence, last_hash
    ) VALUES (
        NEW.organization_id, NEW.evidence_id, NEW.series_id,
        0, decode(repeat('00', 32), 'hex')
    ) ON CONFLICT (organization_id, evidence_id) DO NOTHING;

    SELECT head.last_sequence, head.last_hash INTO prior_sequence, prior_hash
    FROM public.evidence_custody_chain_heads AS head
    WHERE head.organization_id = NEW.organization_id AND head.evidence_id = NEW.evidence_id
    FOR UPDATE;

    NEW.chain_sequence := prior_sequence + 1;
    NEW.previous_hash := prior_hash;
    NEW.event_hash := public.calculate_evidence_custody_event_hash(
        NEW.id, NEW.organization_id, NEW.evidence_id, NEW.series_id,
        NEW.chain_sequence, NEW.previous_hash, NEW.event_type,
        NEW.actor_user_id, NEW.actor_type, NEW.reason, NEW.object_sha256,
        NEW.request_id, NEW.details, NEW.created_at
    );

    UPDATE public.evidence_custody_chain_heads
    SET last_sequence = NEW.chain_sequence,
        last_hash = NEW.event_hash,
        updated_at = statement_timestamp()
    WHERE organization_id = NEW.organization_id AND evidence_id = NEW.evidence_id;
    RETURN NEW;
END;
$$;
REVOKE ALL ON FUNCTION append_evidence_custody_event_chain() FROM PUBLIC;

CREATE FUNCTION prevent_evidence_history_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
	-- A governed parent deletion (for example tenant erasure after retention and
	-- hold checks) may cascade its now-orphaned history. Direct history deletion
	-- remains impossible while the evidence record exists.
	IF TG_OP = 'DELETE' AND NOT EXISTS (
		SELECT 1 FROM public.control_evidence AS evidence
		WHERE evidence.organization_id = OLD.organization_id
		  AND evidence.id = OLD.evidence_id
	) THEN
		RETURN OLD;
	END IF;
    RAISE EXCEPTION 'evidence history is append-only: % is not permitted', TG_OP
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER trg_evidence_custody_event_chain
    BEFORE INSERT ON evidence_custody_events
    FOR EACH ROW EXECUTE FUNCTION append_evidence_custody_event_chain();
CREATE TRIGGER trg_evidence_custody_events_immutable
    BEFORE UPDATE OR DELETE ON evidence_custody_events
    FOR EACH ROW EXECUTE FUNCTION prevent_evidence_history_mutation();
CREATE TRIGGER trg_evidence_reviews_immutable
    BEFORE UPDATE OR DELETE ON evidence_reviews
    FOR EACH ROW EXECUTE FUNCTION prevent_evidence_history_mutation();

-- Project every pre-existing evidence hold record into the new custody chain.
-- The genesis event above records the aggregate trust-boundary state; these
-- events bind each placement/release decision and its material fields.
DO $backfill_evidence_holds$
DECLARE
    held RECORD;
BEGIN
    FOR held IN
        SELECT record.organization_id, record.id AS legal_hold_record_id,
               record.hold_id, record.record_type, record.record_id,
               record.reason AS placement_reason, record.placed_by,
               record.placed_at, record.released_by, record.released_at,
               record.release_reason, evidence.series_id, evidence.file_hash
        FROM legal_hold_records AS record
        JOIN control_evidence AS evidence
          ON evidence.organization_id = record.organization_id
         AND evidence.id = record.record_id
        WHERE record.record_type = 'evidence'
        ORDER BY record.organization_id, record.record_id,
                 record.placed_at, record.id
    LOOP
        PERFORM set_config('app.current_tenant', held.organization_id::TEXT, TRUE);
        INSERT INTO evidence_custody_events (
            organization_id, evidence_id, series_id, event_type, actor_user_id,
            actor_type, reason, object_sha256, details
        ) VALUES (
            held.organization_id, held.record_id, held.series_id,
            'legal_hold_placed', held.placed_by, 'user',
            'Evidence legal hold migrated into custody history',
            CASE WHEN held.file_hash ~* '^[0-9a-f]{64}$' THEN lower(held.file_hash) END,
            jsonb_build_object(
                'migration', '056',
                'legal_hold_id', held.hold_id,
                'legal_hold_record_id', held.legal_hold_record_id,
                'record_type', held.record_type,
                'record_id', held.record_id,
                'placement_reason', held.placement_reason,
                'placed_by', held.placed_by,
                'placed_at', held.placed_at
            )
        );
        IF held.released_at IS NOT NULL THEN
            INSERT INTO evidence_custody_events (
                organization_id, evidence_id, series_id, event_type, actor_user_id,
                actor_type, reason, object_sha256, details
            ) VALUES (
                held.organization_id, held.record_id, held.series_id,
                'legal_hold_released', held.released_by, 'user',
                'Evidence legal hold release migrated into custody history',
                CASE WHEN held.file_hash ~* '^[0-9a-f]{64}$' THEN lower(held.file_hash) END,
                jsonb_build_object(
                    'migration', '056',
                    'legal_hold_id', held.hold_id,
                    'legal_hold_record_id', held.legal_hold_record_id,
                    'record_type', held.record_type,
                    'record_id', held.record_id,
                    'placement_reason', held.placement_reason,
                    'placed_by', held.placed_by,
                    'placed_at', held.placed_at,
                    'released_by', held.released_by,
                    'released_at', held.released_at,
                    'release_reason', held.release_reason
                )
            );
        END IF;
    END LOOP;
    PERFORM set_config('app.current_tenant', '', TRUE);
END;
$backfill_evidence_holds$;

CREATE FUNCTION verify_evidence_custody_chain(
    requested_organization_id UUID,
    requested_evidence_id UUID
)
RETURNS TABLE (
    valid BOOLEAN,
    event_count BIGINT,
    head_sequence BIGINT,
    head_hash TEXT,
    first_invalid_sequence BIGINT
)
LANGUAGE plpgsql
STABLE
SET search_path = pg_catalog, public
AS $$
DECLARE
    event_record RECORD;
    calculated_hash BYTEA;
    expected_previous BYTEA := decode(repeat('00', 32), 'hex');
    expected_sequence BIGINT := 0;
    stored_head_sequence BIGINT;
    stored_head_hash BYTEA;
BEGIN
    IF public.get_current_tenant() IS NULL
       OR public.get_current_tenant() <> requested_organization_id THEN
        RAISE EXCEPTION 'evidence custody verification tenant mismatch'
            USING ERRCODE = '42501';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM public.control_evidence AS evidence
        WHERE evidence.organization_id = requested_organization_id
          AND evidence.id = requested_evidence_id
    ) THEN
        RETURN;
    END IF;

    event_count := 0;
    first_invalid_sequence := NULL;
    FOR event_record IN
        SELECT custody.*
        FROM public.evidence_custody_events AS custody
        WHERE custody.organization_id = requested_organization_id
          AND custody.evidence_id = requested_evidence_id
        ORDER BY custody.chain_sequence, custody.id
    LOOP
        event_count := event_count + 1;
        expected_sequence := expected_sequence + 1;
        calculated_hash := public.calculate_evidence_custody_event_hash(
            event_record.id, event_record.organization_id, event_record.evidence_id,
            event_record.series_id, event_record.chain_sequence,
            event_record.previous_hash, event_record.event_type,
            event_record.actor_user_id, event_record.actor_type, event_record.reason,
            event_record.object_sha256, event_record.request_id,
            event_record.details, event_record.created_at
        );
        IF first_invalid_sequence IS NULL AND (
            event_record.chain_sequence <> expected_sequence
            OR event_record.previous_hash <> expected_previous
            OR event_record.event_hash <> calculated_hash
        ) THEN
            first_invalid_sequence := expected_sequence;
        END IF;
        expected_previous := event_record.event_hash;
    END LOOP;

    SELECT head.last_sequence, head.last_hash
    INTO stored_head_sequence, stored_head_hash
    FROM public.evidence_custody_chain_heads AS head
    WHERE head.organization_id = requested_organization_id
      AND head.evidence_id = requested_evidence_id;

    IF NOT FOUND THEN
        valid := FALSE;
        head_sequence := 0;
        head_hash := '';
        first_invalid_sequence := COALESCE(first_invalid_sequence, 1);
        RETURN NEXT;
        RETURN;
    END IF;

    head_sequence := stored_head_sequence;
    head_hash := encode(stored_head_hash, 'hex');
    IF first_invalid_sequence IS NULL AND (
        event_count = 0 OR stored_head_sequence <> expected_sequence
        OR stored_head_hash <> expected_previous
    ) THEN
        first_invalid_sequence := expected_sequence + 1;
    END IF;
    valid := first_invalid_sequence IS NULL;
    RETURN NEXT;
END;
$$;
REVOKE ALL ON FUNCTION verify_evidence_custody_chain(UUID, UUID) FROM PUBLIC;

CREATE FUNCTION initialize_control_evidence_version()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
DECLARE
    predecessor RECORD;
BEGIN
    IF NEW.id IS NULL THEN
        NEW.id := gen_random_uuid();
    END IF;
    NEW.version_reason := BTRIM(COALESCE(NULLIF(NEW.version_reason, ''), 'Initial evidence version'));
    NEW.expires_at := CASE WHEN NEW.valid_until IS NULL THEN NULL
        ELSE ((NEW.valid_until + 1)::TIMESTAMP AT TIME ZONE 'UTC') END;
    -- A review projection can only be produced by an immutable
    -- evidence_reviews row after insertion.
    NEW.review_status := 'pending';
    NEW.reviewed_by := NULL;
    NEW.reviewed_at := NULL;
    NEW.review_notes := NULL;

    IF NEW.supersedes_evidence_id IS NULL THEN
        NEW.series_id := NEW.id;
        NEW.version_number := 1;
        NEW.is_current := TRUE;
        NEW.lifecycle_status := 'active';
        NEW.superseded_by_evidence_id := NULL;
        NEW.superseded_at := NULL;
    ELSE
        SELECT evidence.id, evidence.series_id, evidence.version_number,
               evidence.lifecycle_status, evidence.is_current,
               evidence.superseded_by_evidence_id, evidence.version_reason
        INTO predecessor
        FROM public.control_evidence AS evidence
        WHERE evidence.organization_id = NEW.organization_id
          AND evidence.id = NEW.supersedes_evidence_id
          AND evidence.control_implementation_id = NEW.control_implementation_id
          AND evidence.deleted_at IS NULL
        FOR UPDATE;
        IF NOT FOUND
           OR predecessor.superseded_by_evidence_id IS NOT NULL
           OR (predecessor.lifecycle_status = 'active' AND NOT predecessor.is_current)
           OR (predecessor.lifecycle_status = 'superseded'
               AND predecessor.version_reason <> 'Migrated legacy non-current evidence') THEN
            RAISE EXCEPTION 'evidence predecessor is not the latest replaceable version'
                USING ERRCODE = '23514';
        END IF;
        NEW.series_id := predecessor.series_id;
        NEW.version_number := predecessor.version_number + 1;
        NEW.is_current := FALSE;
        NEW.lifecycle_status := 'active';
        NEW.superseded_by_evidence_id := NULL;
        NEW.superseded_at := NULL;
        IF NEW.version_reason = 'Initial evidence version' THEN
            NEW.version_reason := 'Superseded evidence version';
        END IF;
    END IF;

    NEW.content_fingerprint := public.calculate_control_evidence_fingerprint(to_jsonb(NEW));
    RETURN NEW;
END;
$$;

CREATE FUNCTION enforce_control_evidence_immutability()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
DECLARE
    latest_review RECORD;
    linked_version RECORD;
    lifecycle_changed BOOLEAN;
    review_changed BOOLEAN;
BEGIN
    IF OLD.content_fingerprint <> public.calculate_control_evidence_fingerprint(to_jsonb(OLD)) THEN
        RAISE EXCEPTION 'stored evidence content fingerprint is invalid' USING ERRCODE = '55000';
    END IF;
    IF NEW.content_fingerprint <> OLD.content_fingerprint
       OR public.calculate_control_evidence_fingerprint(to_jsonb(NEW)) <> OLD.content_fingerprint THEN
        RAISE EXCEPTION 'evidence content and provenance are immutable; create a new version'
            USING ERRCODE = '55000';
    END IF;

    IF OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS DISTINCT FROM OLD.deleted_at THEN
        RAISE EXCEPTION 'deleted evidence cannot be restored or re-deleted'
            USING ERRCODE = '55000';
    ELSIF OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL THEN
        NEW.deleted_at := statement_timestamp();
    END IF;

    lifecycle_changed := ROW(
        NEW.lifecycle_status, NEW.is_current,
        NEW.superseded_by_evidence_id, NEW.superseded_at
    ) IS DISTINCT FROM ROW(
        OLD.lifecycle_status, OLD.is_current,
        OLD.superseded_by_evidence_id, OLD.superseded_at
    );

    IF lifecycle_changed AND OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL THEN
        RAISE EXCEPTION 'evidence lifecycle transition and deletion must be separate operations'
            USING ERRCODE = '55000';
    END IF;

    IF lifecycle_changed THEN
        -- A replacement is first inserted as a non-current active row. Linking
        -- it from the predecessor is the only operation that may supersede a
        -- current row (or attach a successor to a migrated/expired row).
        IF OLD.superseded_by_evidence_id IS NULL
           AND NEW.superseded_by_evidence_id IS NOT NULL
           AND NOT NEW.is_current
           AND (
               (OLD.lifecycle_status = 'active' AND OLD.is_current
                    AND NEW.lifecycle_status = 'superseded')
               OR (OLD.lifecycle_status = 'expired' AND NOT OLD.is_current
                    AND NEW.lifecycle_status = 'expired')
               OR (OLD.lifecycle_status = 'superseded' AND NOT OLD.is_current
                    AND OLD.version_reason = 'Migrated legacy non-current evidence'
                    AND NEW.lifecycle_status = 'superseded')
           ) THEN
            SELECT successor.id INTO linked_version
            FROM public.control_evidence AS successor
            WHERE successor.organization_id = NEW.organization_id
              AND successor.id = NEW.superseded_by_evidence_id
              AND successor.series_id = NEW.series_id
              AND successor.supersedes_evidence_id = NEW.id
              AND successor.version_number = NEW.version_number + 1
              AND successor.lifecycle_status = 'active'
              AND NOT successor.is_current
              AND successor.deleted_at IS NULL;
            IF NOT FOUND THEN
                RAISE EXCEPTION 'evidence successor link is invalid'
                    USING ERRCODE = '23514';
            END IF;
            NEW.superseded_at := statement_timestamp();

        -- Activation is allowed only after the predecessor has reciprocally
        -- linked the newly inserted replacement.
        ELSIF OLD.lifecycle_status = 'active' AND NOT OLD.is_current
              AND NEW.lifecycle_status = 'active' AND NEW.is_current
              AND OLD.supersedes_evidence_id IS NOT NULL
              AND NEW.supersedes_evidence_id = OLD.supersedes_evidence_id
              AND NEW.superseded_by_evidence_id IS NOT DISTINCT FROM OLD.superseded_by_evidence_id
              AND NEW.superseded_at IS NOT DISTINCT FROM OLD.superseded_at THEN
            SELECT predecessor.id INTO linked_version
            FROM public.control_evidence AS predecessor
            WHERE predecessor.organization_id = NEW.organization_id
              AND predecessor.id = NEW.supersedes_evidence_id
              AND predecessor.series_id = NEW.series_id
              AND predecessor.superseded_by_evidence_id = NEW.id
              AND predecessor.version_number + 1 = NEW.version_number
              AND NOT predecessor.is_current
              AND predecessor.lifecycle_status IN ('superseded', 'expired');
            IF NOT FOUND THEN
                RAISE EXCEPTION 'replacement evidence has not been linked by its predecessor'
                    USING ERRCODE = '23514';
            END IF;

        -- Expiry is one-way and may happen only once the immutable validity
        -- boundary has actually elapsed.
        ELSIF OLD.lifecycle_status = 'active' AND OLD.is_current
              AND NEW.lifecycle_status = 'expired' AND NOT NEW.is_current
              AND NEW.review_status = 'expired'
              AND OLD.superseded_by_evidence_id IS NULL
              AND NEW.superseded_by_evidence_id IS NULL
              AND OLD.superseded_at IS NULL AND NEW.superseded_at IS NULL
              AND OLD.expires_at IS NOT NULL
              AND OLD.expires_at <= statement_timestamp() THEN
            NULL;
        ELSE
            RAISE EXCEPTION 'invalid evidence lifecycle transition'
                USING ERRCODE = '55000';
        END IF;
    END IF;

    review_changed := ROW(NEW.review_status, NEW.reviewed_by, NEW.reviewed_at, NEW.review_notes)
        IS DISTINCT FROM ROW(OLD.review_status, OLD.reviewed_by, OLD.reviewed_at, OLD.review_notes);
    IF review_changed
       AND NOT (OLD.lifecycle_status = 'active' AND OLD.is_current
                AND NEW.lifecycle_status = 'expired' AND NOT NEW.is_current
                AND NEW.review_status = 'expired'
                AND ROW(NEW.reviewed_by, NEW.reviewed_at, NEW.review_notes)
                    IS NOT DISTINCT FROM
                    ROW(OLD.reviewed_by, OLD.reviewed_at, OLD.review_notes)) THEN
        IF OLD.lifecycle_status <> 'active' OR NEW.lifecycle_status <> 'active'
           OR NOT OLD.is_current OR NOT NEW.is_current THEN
            RAISE EXCEPTION 'only current active evidence may receive a review projection'
                USING ERRCODE = '55000';
        END IF;
        SELECT review.decision, review.reviewer_id, review.created_at, review.comment
        INTO latest_review
        FROM public.evidence_reviews AS review
        WHERE review.organization_id = NEW.organization_id AND review.evidence_id = NEW.id
        ORDER BY review.created_at DESC, review.id DESC
        LIMIT 1;
        IF NOT FOUND OR ROW(
            NEW.review_status, NEW.reviewed_by, NEW.reviewed_at, NEW.review_notes
        ) IS DISTINCT FROM ROW(
            latest_review.decision, latest_review.reviewer_id,
            latest_review.created_at, latest_review.comment
        ) THEN
            RAISE EXCEPTION 'evidence review projection requires an immutable review record'
                USING ERRCODE = '55000';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE FUNCTION validate_control_evidence_series()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
DECLARE
    target_organization UUID := COALESCE(NEW.organization_id, OLD.organization_id);
    target_series UUID := COALESCE(NEW.series_id, OLD.series_id);
BEGIN
    IF EXISTS (
        SELECT 1 FROM public.control_evidence AS evidence
        WHERE evidence.organization_id = target_organization
          AND evidence.series_id = target_series AND evidence.deleted_at IS NULL
          AND evidence.lifecycle_status = 'active' AND NOT evidence.is_current
    ) THEN
        RAISE EXCEPTION 'active evidence series must have exactly one current version'
            USING ERRCODE = '23514';
    END IF;
    IF (
        SELECT count(*) FROM public.control_evidence AS evidence
        WHERE evidence.organization_id = target_organization
          AND evidence.series_id = target_series AND evidence.deleted_at IS NULL
          AND evidence.lifecycle_status = 'active' AND evidence.is_current
    ) > 1 THEN
        RAISE EXCEPTION 'evidence series has multiple current versions' USING ERRCODE = '23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.control_evidence AS evidence
        WHERE evidence.organization_id = target_organization
          AND evidence.series_id = target_series AND evidence.deleted_at IS NULL
          AND evidence.lifecycle_status = 'superseded'
          AND evidence.superseded_by_evidence_id IS NULL
          AND evidence.version_reason <> 'Migrated legacy non-current evidence'
    ) THEN
        RAISE EXCEPTION 'superseded evidence must identify its successor' USING ERRCODE = '23514';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM public.control_evidence AS evidence
        JOIN public.control_evidence AS successor
          ON successor.organization_id = evidence.organization_id
         AND successor.id = evidence.superseded_by_evidence_id
        WHERE evidence.organization_id = target_organization
          AND evidence.series_id = target_series AND evidence.deleted_at IS NULL
          AND (successor.series_id <> evidence.series_id
               OR successor.supersedes_evidence_id <> evidence.id
               OR successor.version_number <> evidence.version_number + 1)
    ) OR EXISTS (
        SELECT 1
        FROM public.control_evidence AS evidence
        JOIN public.control_evidence AS predecessor
          ON predecessor.organization_id = evidence.organization_id
         AND predecessor.id = evidence.supersedes_evidence_id
        WHERE evidence.organization_id = target_organization
          AND evidence.series_id = target_series AND evidence.deleted_at IS NULL
          AND (predecessor.series_id <> evidence.series_id
               OR predecessor.superseded_by_evidence_id <> evidence.id
               OR evidence.version_number <> predecessor.version_number + 1)
    ) THEN
        RAISE EXCEPTION 'evidence version links are not reciprocal and sequential'
            USING ERRCODE = '23514';
    END IF;
    RETURN NULL;
END;
$$;

CREATE FUNCTION record_control_evidence_upload()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
BEGIN
    INSERT INTO public.evidence_custody_events (
        organization_id, evidence_id, series_id, event_type, actor_user_id,
        actor_type, reason, object_sha256, details
    ) VALUES (
        NEW.organization_id, NEW.id, NEW.series_id, 'uploaded', NEW.collected_by,
        CASE WHEN NEW.collected_by IS NULL THEN 'system' ELSE 'user' END,
        CASE WHEN NEW.version_number = 1 THEN 'Initial evidence version uploaded'
             ELSE 'Replacement evidence version uploaded' END,
        CASE WHEN NEW.file_hash ~ '^[0-9a-f]{64}$' THEN NEW.file_hash END,
        jsonb_build_object(
            'version_number', NEW.version_number,
            'content_fingerprint', NEW.content_fingerprint,
            'collection_method', NEW.collection_method
        )
    );
    RETURN NEW;
END;
$$;

CREATE FUNCTION record_control_evidence_transition()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    transition_actor UUID;
BEGIN
    IF NEW.superseded_by_evidence_id IS DISTINCT FROM OLD.superseded_by_evidence_id
       AND NEW.superseded_by_evidence_id IS NOT NULL THEN
        SELECT successor.collected_by INTO transition_actor
        FROM public.control_evidence AS successor
        WHERE successor.organization_id = NEW.organization_id
          AND successor.id = NEW.superseded_by_evidence_id;
        INSERT INTO public.evidence_custody_events (
            organization_id, evidence_id, series_id, event_type, actor_user_id,
            actor_type, reason, object_sha256, details
        ) VALUES (
            NEW.organization_id, NEW.id, NEW.series_id, 'superseded', transition_actor,
            CASE WHEN transition_actor IS NULL THEN 'system' ELSE 'user' END,
            'Evidence version superseded',
            CASE WHEN NEW.file_hash ~ '^[0-9a-f]{64}$' THEN NEW.file_hash END,
            jsonb_build_object('superseded_by_evidence_id', NEW.superseded_by_evidence_id)
        );
    ELSIF OLD.lifecycle_status = 'active' AND NEW.lifecycle_status = 'expired' THEN
        INSERT INTO public.evidence_custody_events (
            organization_id, evidence_id, series_id, event_type,
            actor_type, reason, object_sha256, details
        ) VALUES (
            NEW.organization_id, NEW.id, NEW.series_id, 'expired',
            'system', 'Evidence validity period expired',
            CASE WHEN NEW.file_hash ~ '^[0-9a-f]{64}$' THEN NEW.file_hash END,
            jsonb_build_object('expires_at', NEW.expires_at)
        );
    ELSIF OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL THEN
        INSERT INTO public.evidence_custody_events (
            organization_id, evidence_id, series_id, event_type,
            actor_type, reason, object_sha256, details
        ) VALUES (
            NEW.organization_id, NEW.id, NEW.series_id, 'deleted',
            'system', 'Evidence record soft-deleted through governed disposition',
            CASE WHEN NEW.file_hash ~ '^[0-9a-f]{64}$' THEN NEW.file_hash END,
            jsonb_build_object('deleted_at', NEW.deleted_at)
        );
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_control_evidence_initialize_version
    BEFORE INSERT ON control_evidence
    FOR EACH ROW EXECUTE FUNCTION initialize_control_evidence_version();
CREATE TRIGGER trg_control_evidence_immutable_content
    BEFORE UPDATE ON control_evidence
    FOR EACH ROW EXECUTE FUNCTION enforce_control_evidence_immutability();
CREATE CONSTRAINT TRIGGER trg_control_evidence_series_consistency
    AFTER INSERT OR UPDATE OR DELETE ON control_evidence
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION validate_control_evidence_series();
CREATE TRIGGER trg_control_evidence_custody_upload
    AFTER INSERT ON control_evidence
    FOR EACH ROW EXECUTE FUNCTION record_control_evidence_upload();
CREATE TRIGGER trg_control_evidence_custody_transition
    AFTER UPDATE OF lifecycle_status, superseded_by_evidence_id, deleted_at ON control_evidence
    FOR EACH ROW EXECUTE FUNCTION record_control_evidence_transition();

CREATE FUNCTION validate_evidence_review()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    evidence_fingerprint TEXT;
BEGIN
    IF public.get_current_tenant() IS NULL
       OR public.get_current_tenant() <> NEW.organization_id THEN
        RAISE EXCEPTION 'evidence review tenant does not match request tenant'
            USING ERRCODE = '42501';
    END IF;
    SELECT evidence.content_fingerprint INTO evidence_fingerprint
    FROM public.control_evidence AS evidence
    WHERE evidence.organization_id = NEW.organization_id
      AND evidence.id = NEW.evidence_id
      AND evidence.deleted_at IS NULL
      AND evidence.lifecycle_status = 'active' AND evidence.is_current
    FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'only current active evidence can be reviewed' USING ERRCODE = '23514';
    END IF;
    NEW.decision := lower(BTRIM(NEW.decision));
    NEW.comment := NULLIF(BTRIM(NEW.comment), '');
    NEW.request_id := NULLIF(BTRIM(NEW.request_id), '');
    NEW.evidence_sha256 := evidence_fingerprint;
    NEW.created_at := statement_timestamp();
    RETURN NEW;
END;
$$;

CREATE FUNCTION project_evidence_review()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
BEGIN
    UPDATE public.control_evidence
    SET review_status = NEW.decision,
        reviewed_by = NEW.reviewer_id,
        reviewed_at = NEW.created_at,
        review_notes = NEW.comment,
        updated_at = statement_timestamp()
    WHERE organization_id = NEW.organization_id AND id = NEW.evidence_id;

    INSERT INTO public.evidence_custody_events (
        organization_id, evidence_id, series_id, event_type, actor_user_id,
        actor_type, reason, object_sha256, request_id, details
    )
    SELECT evidence.organization_id, evidence.id, evidence.series_id,
           'reviewed', NEW.reviewer_id, 'user',
           'Evidence reviewer decision recorded',
           CASE WHEN evidence.file_hash ~ '^[0-9a-f]{64}$' THEN evidence.file_hash END,
           NEW.request_id,
           jsonb_build_object('review_id', NEW.id, 'decision', NEW.decision,
		                      'comment', NEW.comment, 'metadata', NEW.metadata,
		                      'reviewer_id', NEW.reviewer_id,
		                      'reviewed_at', NEW.created_at,
		                      'content_fingerprint', NEW.evidence_sha256)
    FROM public.control_evidence AS evidence
    WHERE evidence.organization_id = NEW.organization_id AND evidence.id = NEW.evidence_id;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_evidence_reviews_validate
    BEFORE INSERT ON evidence_reviews
    FOR EACH ROW EXECUTE FUNCTION validate_evidence_review();
CREATE TRIGGER trg_evidence_reviews_project
    AFTER INSERT ON evidence_reviews
    FOR EACH ROW EXECUTE FUNCTION project_evidence_review();

-- Runtime roles cannot write the review ledger directly. This narrow
-- capability binds the requested review to an existing control, evidence
-- version, and same-tenant active reviewer; table triggers canonicalize and
-- project the immutable decision.
CREATE FUNCTION submit_evidence_review(
    requested_organization_id UUID,
    requested_control_id UUID,
    requested_evidence_id UUID,
    requested_reviewer_id UUID,
    requested_decision TEXT,
    requested_comment TEXT,
    requested_request_id TEXT DEFAULT NULL
)
RETURNS TABLE (review_id UUID)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF public.get_current_tenant() IS NULL
       OR public.get_current_tenant() <> requested_organization_id THEN
        RAISE EXCEPTION 'evidence review tenant does not match request tenant'
            USING ERRCODE = '42501';
    END IF;

    RETURN QUERY
    INSERT INTO public.evidence_reviews (
        organization_id, evidence_id, decision, comment, reviewer_id, request_id
    )
    SELECT requested_organization_id, evidence.id, requested_decision,
           requested_comment, reviewer.id, NULLIF(BTRIM(requested_request_id), '')
    FROM public.control_evidence AS evidence
    JOIN public.control_implementations AS implementation
      ON implementation.organization_id = evidence.organization_id
     AND implementation.id = evidence.control_implementation_id
    JOIN public.users AS reviewer
      ON reviewer.organization_id = evidence.organization_id
     AND reviewer.id = requested_reviewer_id
    WHERE evidence.organization_id = requested_organization_id
      AND evidence.id = requested_evidence_id
      AND implementation.framework_control_id = requested_control_id
      AND evidence.deleted_at IS NULL AND implementation.deleted_at IS NULL
      AND reviewer.deleted_at IS NULL
      AND evidence.lifecycle_status = 'active' AND evidence.is_current
    ON CONFLICT (organization_id, evidence_id, request_id)
        WHERE request_id IS NOT NULL DO NOTHING
    RETURNING id;
END;
$$;

-- Downloads and object-integrity checks are the only API-originated custody
-- events. Reasons, hashes, actor type, and details are derived here rather
-- than accepted from the caller, preventing a runtime login from minting
-- legal-hold or lifecycle events.
CREATE FUNCTION append_evidence_access_custody_event(
    requested_organization_id UUID,
    requested_control_id UUID,
    requested_evidence_id UUID,
    requested_actor_id UUID,
    requested_event_type TEXT,
    requested_request_id TEXT DEFAULT NULL,
    requested_delivery_mode TEXT DEFAULT NULL
)
RETURNS TABLE (event_id UUID)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    normalized_event_type TEXT := lower(BTRIM(requested_event_type));
    normalized_delivery_mode TEXT := lower(BTRIM(requested_delivery_mode));
    event_reason TEXT;
    event_details JSONB;
BEGIN
    IF public.get_current_tenant() IS NULL
       OR public.get_current_tenant() <> requested_organization_id THEN
        RAISE EXCEPTION 'evidence custody tenant does not match request tenant'
            USING ERRCODE = '42501';
    END IF;

    CASE normalized_event_type
        WHEN 'download_authorized' THEN
            IF normalized_delivery_mode IS NULL
               OR normalized_delivery_mode NOT IN ('private_stream', 'signed_url') THEN
                RAISE EXCEPTION 'invalid evidence delivery mode' USING ERRCODE = '22023';
            END IF;
            event_reason := 'Authorized evidence download';
            event_details := jsonb_build_object('delivery_mode', normalized_delivery_mode);
        WHEN 'integrity_verified' THEN
            IF requested_delivery_mode IS NOT NULL THEN
                RAISE EXCEPTION 'delivery mode is not valid for an integrity event' USING ERRCODE = '22023';
            END IF;
            event_reason := 'Evidence object integrity verified';
            event_details := jsonb_build_object('verification_method', 'sha256_and_size');
        WHEN 'integrity_failed' THEN
            IF requested_delivery_mode IS NOT NULL THEN
                RAISE EXCEPTION 'delivery mode is not valid for an integrity event' USING ERRCODE = '22023';
            END IF;
            event_reason := 'Evidence object integrity verification failed';
            event_details := jsonb_build_object('verification_method', 'sha256_and_size');
        ELSE
            RAISE EXCEPTION 'unsupported API-originated evidence custody event'
                USING ERRCODE = '22023';
    END CASE;

    RETURN QUERY
    INSERT INTO public.evidence_custody_events (
        organization_id, evidence_id, series_id, event_type, actor_user_id,
        actor_type, reason, object_sha256, request_id, details
    )
    SELECT evidence.organization_id, evidence.id, evidence.series_id,
           normalized_event_type, actor.id, 'user', event_reason,
           CASE WHEN evidence.file_hash ~ '^[0-9a-f]{64}$' THEN evidence.file_hash END,
           NULLIF(BTRIM(requested_request_id), ''), event_details
    FROM public.control_evidence AS evidence
    JOIN public.control_implementations AS implementation
      ON implementation.organization_id = evidence.organization_id
     AND implementation.id = evidence.control_implementation_id
    JOIN public.users AS actor
      ON actor.organization_id = evidence.organization_id
     AND actor.id = requested_actor_id
    WHERE evidence.organization_id = requested_organization_id
      AND evidence.id = requested_evidence_id
      AND implementation.framework_control_id = requested_control_id
      AND evidence.deleted_at IS NULL AND implementation.deleted_at IS NULL
      AND actor.deleted_at IS NULL
    RETURNING id;
END;
$$;

-- Expiry is a worker capability rather than direct table DML. It validates the
-- tenant context, bounds and locks the batch, performs the lifecycle mutation
-- (whose trigger appends custody), and returns exactly the data needed to
-- enqueue the notification in the caller's surrounding transaction.
CREATE FUNCTION expire_due_evidence(
    requested_organization_id UUID,
    requested_limit INTEGER DEFAULT 100
)
RETURNS TABLE (
    evidence_id UUID,
    organization_id UUID,
    control_implementation_id UUID,
    title TEXT,
    expires_at TIMESTAMPTZ,
    collected_by UUID,
    control_code TEXT,
    custody_event_id UUID,
    custody_created_at TIMESTAMPTZ
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    expired_record RECORD;
BEGIN
    IF public.get_current_tenant() IS NULL
       OR public.get_current_tenant() <> requested_organization_id THEN
        RAISE EXCEPTION 'evidence expiry tenant does not match request tenant'
            USING ERRCODE = '42501';
    END IF;
    IF requested_limit IS NULL OR requested_limit < 1 OR requested_limit > 1000 THEN
        RAISE EXCEPTION 'evidence expiry batch limit must be between 1 and 1000'
            USING ERRCODE = '22023';
    END IF;

    FOR expired_record IN
        WITH candidates AS (
            SELECT evidence.id
            FROM public.control_evidence AS evidence
            WHERE evidence.organization_id = requested_organization_id
              AND evidence.deleted_at IS NULL
              AND evidence.lifecycle_status = 'active'
              AND evidence.is_current
              AND evidence.expires_at <= statement_timestamp()
            ORDER BY evidence.expires_at, evidence.id
            FOR UPDATE SKIP LOCKED
            LIMIT requested_limit
        ), expired AS (
            UPDATE public.control_evidence AS evidence
            SET lifecycle_status = 'expired',
                is_current = FALSE,
                review_status = 'expired',
                updated_at = statement_timestamp()
            FROM candidates
            WHERE evidence.organization_id = requested_organization_id
              AND evidence.id = candidates.id
            RETURNING evidence.id, evidence.organization_id,
                      evidence.control_implementation_id, evidence.title,
                      evidence.expires_at, evidence.collected_by
        )
        SELECT expired.id, expired.organization_id,
               expired.control_implementation_id, expired.title,
               expired.expires_at, expired.collected_by,
               control.code AS control_code
        FROM expired
        JOIN public.control_implementations AS implementation
          ON implementation.organization_id = expired.organization_id
         AND implementation.id = expired.control_implementation_id
        JOIN public.framework_controls AS control
          ON control.id = implementation.framework_control_id
        ORDER BY expired.expires_at, expired.id
    LOOP
        SELECT event.id, event.created_at
        INTO custody_event_id, custody_created_at
        FROM public.evidence_custody_events AS event
        WHERE event.organization_id = expired_record.organization_id
          AND event.evidence_id = expired_record.id
          AND event.event_type = 'expired'
        ORDER BY event.chain_sequence DESC
        LIMIT 1;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'evidence expiry custody event was not recorded'
                USING ERRCODE = '55000';
        END IF;

        evidence_id := expired_record.id;
        organization_id := expired_record.organization_id;
        control_implementation_id := expired_record.control_implementation_id;
        title := expired_record.title;
        expires_at := expired_record.expires_at;
        collected_by := expired_record.collected_by;
        control_code := expired_record.control_code;
        RETURN NEXT;
    END LOOP;
END;
$$;

-- Legal-hold placement is an immutable preservation decision. A record may be
-- released once, but its identity cannot be rewritten to evade disposition
-- guards and neither placed nor released rows may be directly deleted.
CREATE FUNCTION enforce_legal_hold_record_immutability()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.released_at IS NOT NULL OR NEW.released_by IS NOT NULL OR NEW.release_reason IS NOT NULL THEN
            RAISE EXCEPTION 'new legal hold records cannot contain release metadata'
                USING ERRCODE = '55000';
        END IF;
        -- The row lock serializes placement against a concurrent parent
        -- release/cancellation; after waiting, READ COMMITTED re-checks status.
        PERFORM 1 FROM public.legal_holds AS hold
        WHERE hold.organization_id = NEW.organization_id
          AND hold.id = NEW.hold_id AND hold.status = 'active'
        FOR UPDATE;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'legal hold records may be placed only on an active hold'
                USING ERRCODE = '55000';
        END IF;
        IF NEW.record_type = 'evidence' THEN
            -- Serialize placement against both soft and hard evidence
            -- disposition. FOR UPDATE conflicts with deletion and with the
            -- non-key UPDATE used to set deleted_at.
            PERFORM 1 FROM public.control_evidence AS evidence
            WHERE evidence.organization_id = NEW.organization_id
              AND evidence.id = NEW.record_id AND evidence.deleted_at IS NULL
            FOR UPDATE;
            IF NOT FOUND THEN
                RAISE EXCEPTION 'legal hold evidence target does not exist'
                    USING ERRCODE = '23503';
            END IF;
        END IF;
        RETURN NEW;
    END IF;

    IF TG_OP = 'DELETE' THEN
        -- Permit only a referential cascade whose parent hold is already being
        -- removed (ultimately through governed tenant erasure).
        IF NOT EXISTS (
            SELECT 1 FROM public.legal_holds AS hold
            WHERE hold.organization_id = OLD.organization_id AND hold.id = OLD.hold_id
        ) THEN
            RETURN OLD;
        END IF;
        RAISE EXCEPTION 'legal hold record history cannot be deleted; release the record instead'
            USING ERRCODE = '55000';
    END IF;

    IF ROW(
        NEW.id, NEW.organization_id, NEW.hold_id, NEW.record_type,
        NEW.record_id, NEW.reason, NEW.placed_by, NEW.placed_at
    ) IS DISTINCT FROM ROW(
        OLD.id, OLD.organization_id, OLD.hold_id, OLD.record_type,
        OLD.record_id, OLD.reason, OLD.placed_by, OLD.placed_at
    ) THEN
        RAISE EXCEPTION 'legal hold record placement is immutable'
            USING ERRCODE = '55000';
    END IF;

    IF OLD.released_at IS NOT NULL THEN
        IF ROW(NEW.released_at, NEW.released_by, NEW.release_reason)
           IS DISTINCT FROM ROW(OLD.released_at, OLD.released_by, OLD.release_reason) THEN
            RAISE EXCEPTION 'legal hold record release is immutable'
                USING ERRCODE = '55000';
        END IF;
    ELSIF NEW.released_at IS NOT NULL THEN
        PERFORM 1 FROM public.legal_holds AS hold
        WHERE hold.organization_id = NEW.organization_id
          AND hold.id = NEW.hold_id AND hold.status = 'active'
        FOR UPDATE;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'legal hold records may be released only while the parent hold is active'
                USING ERRCODE = '55000';
        END IF;
        NEW.released_at := statement_timestamp();
        NEW.release_reason := BTRIM(NEW.release_reason);
    END IF;
    RETURN NEW;
END;
$$;

CREATE FUNCTION enforce_legal_hold_custodian_immutability()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.released_at IS NOT NULL OR NEW.released_by IS NOT NULL OR NEW.release_reason IS NOT NULL THEN
            RAISE EXCEPTION 'new legal hold custodians cannot contain release metadata'
                USING ERRCODE = '55000';
        END IF;
        PERFORM 1 FROM public.legal_holds AS hold
        WHERE hold.organization_id = NEW.organization_id
          AND hold.id = NEW.hold_id AND hold.status = 'active'
        FOR UPDATE;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'legal hold custodians may be added only to an active hold'
                USING ERRCODE = '55000';
        END IF;
        NEW.added_at := statement_timestamp();
        RETURN NEW;
    END IF;

    IF TG_OP = 'DELETE' THEN
        IF NOT EXISTS (
            SELECT 1 FROM public.legal_holds AS hold
            WHERE hold.organization_id = OLD.organization_id AND hold.id = OLD.hold_id
        ) THEN
            RETURN OLD;
        END IF;
        RAISE EXCEPTION 'legal hold custodian history cannot be deleted; release the custodian instead'
            USING ERRCODE = '55000';
    END IF;

    IF ROW(NEW.organization_id, NEW.hold_id, NEW.user_id, NEW.added_by, NEW.added_at)
       IS DISTINCT FROM ROW(OLD.organization_id, OLD.hold_id, OLD.user_id, OLD.added_by, OLD.added_at) THEN
        RAISE EXCEPTION 'legal hold custodian assignment is immutable'
            USING ERRCODE = '55000';
    END IF;

    IF OLD.released_at IS NOT NULL THEN
        IF ROW(NEW.released_at, NEW.released_by, NEW.release_reason)
           IS DISTINCT FROM ROW(OLD.released_at, OLD.released_by, OLD.release_reason) THEN
            RAISE EXCEPTION 'legal hold custodian release is immutable'
                USING ERRCODE = '55000';
        END IF;
    ELSIF NEW.released_at IS NOT NULL THEN
        PERFORM 1 FROM public.legal_holds AS hold
        WHERE hold.organization_id = NEW.organization_id
          AND hold.id = NEW.hold_id AND hold.status = 'active'
        FOR UPDATE;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'legal hold custodians may be released only while the parent hold is active'
                USING ERRCODE = '55000';
        END IF;
        NEW.released_at := statement_timestamp();
        NEW.release_reason := BTRIM(NEW.release_reason);
    END IF;
    RETURN NEW;
END;
$$;

CREATE FUNCTION enforce_legal_hold_lifecycle()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF NOT EXISTS (
            SELECT 1 FROM public.organizations AS organization
            WHERE organization.id = OLD.organization_id
        ) THEN
            RETURN OLD;
        END IF;
        RAISE EXCEPTION 'legal hold history cannot be deleted; release or cancel the hold instead'
            USING ERRCODE = '55000';
    END IF;

    IF ROW(NEW.id, NEW.organization_id, NEW.hold_ref, NEW.placed_by, NEW.placed_at, NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.id, OLD.organization_id, OLD.hold_ref, OLD.placed_by, OLD.placed_at, OLD.created_at) THEN
        RAISE EXCEPTION 'legal hold placement identity is immutable'
            USING ERRCODE = '55000';
    END IF;

    IF OLD.status <> 'active' THEN
        IF ROW(
            NEW.name, NEW.matter_reference, NEW.description, NEW.legal_authority,
            NEW.scope, NEW.status, NEW.owner_user_id, NEW.review_due_at,
            NEW.released_by, NEW.released_at, NEW.release_reason, NEW.version
        ) IS DISTINCT FROM ROW(
            OLD.name, OLD.matter_reference, OLD.description, OLD.legal_authority,
            OLD.scope, OLD.status, OLD.owner_user_id, OLD.review_due_at,
            OLD.released_by, OLD.released_at, OLD.release_reason, OLD.version
        ) THEN
            RAISE EXCEPTION 'released or cancelled legal holds are immutable'
                USING ERRCODE = '55000';
        END IF;
        RETURN NEW;
    END IF;

    IF NEW.status <> 'active' THEN
        IF NEW.status NOT IN ('released', 'cancelled')
           OR NEW.released_by IS NULL OR NEW.released_at IS NULL
           OR NEW.release_reason IS NULL OR length(BTRIM(NEW.release_reason)) < 3
           OR NEW.version <> OLD.version + 1 THEN
            RAISE EXCEPTION 'invalid legal hold terminal transition'
                USING ERRCODE = '55000';
        END IF;
        IF ROW(
            NEW.name, NEW.matter_reference, NEW.description, NEW.legal_authority,
            NEW.scope, NEW.owner_user_id, NEW.review_due_at
        ) IS DISTINCT FROM ROW(
            OLD.name, OLD.matter_reference, OLD.description, OLD.legal_authority,
            OLD.scope, OLD.owner_user_id, OLD.review_due_at
        ) THEN
            RAISE EXCEPTION 'legal hold scope cannot change during its terminal transition'
                USING ERRCODE = '55000';
        END IF;
        IF EXISTS (
            SELECT 1 FROM public.legal_hold_records AS record
            WHERE record.organization_id = OLD.organization_id
              AND record.hold_id = OLD.id AND record.released_at IS NULL
        ) OR EXISTS (
            SELECT 1 FROM public.legal_hold_custodians AS custodian
            WHERE custodian.organization_id = OLD.organization_id
              AND custodian.hold_id = OLD.id AND custodian.released_at IS NULL
        ) THEN
            RAISE EXCEPTION 'release legal hold records and custodians before closing the hold'
                USING ERRCODE = '55000';
        END IF;
        NEW.released_at := statement_timestamp();
        NEW.release_reason := BTRIM(NEW.release_reason);
    ELSIF ROW(NEW.released_by, NEW.released_at, NEW.release_reason)
          IS DISTINCT FROM ROW(OLD.released_by, OLD.released_at, OLD.release_reason) THEN
        RAISE EXCEPTION 'active legal holds cannot contain release metadata'
            USING ERRCODE = '55000';
    ELSIF NEW.version <> OLD.version + 1 THEN
        RAISE EXCEPTION 'active legal hold updates must advance the version exactly once'
            USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_legal_hold_records_immutable
    BEFORE INSERT OR UPDATE OR DELETE ON legal_hold_records
    FOR EACH ROW EXECUTE FUNCTION enforce_legal_hold_record_immutability();
CREATE TRIGGER trg_legal_hold_custodians_immutable
    BEFORE INSERT OR UPDATE OR DELETE ON legal_hold_custodians
    FOR EACH ROW EXECUTE FUNCTION enforce_legal_hold_custodian_immutability();
CREATE TRIGGER trg_legal_holds_lifecycle_guard
    BEFORE UPDATE OR DELETE ON legal_holds
    FOR EACH ROW EXECUTE FUNCTION enforce_legal_hold_lifecycle();

CREATE FUNCTION record_evidence_legal_hold_event()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    target_series UUID;
    target_hash TEXT;
    event_actor UUID;
    event_name TEXT;
BEGIN
    IF NEW.record_type <> 'evidence' THEN
        RETURN NEW;
    END IF;
    SELECT evidence.series_id,
           CASE WHEN evidence.file_hash ~ '^[0-9a-f]{64}$' THEN evidence.file_hash END
    INTO target_series, target_hash
    FROM public.control_evidence AS evidence
    WHERE evidence.organization_id = NEW.organization_id AND evidence.id = NEW.record_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'legal hold evidence target does not exist' USING ERRCODE = '23503';
    END IF;

    IF TG_OP = 'INSERT' THEN
        event_actor := NEW.placed_by;
        event_name := 'legal_hold_placed';
    ELSIF OLD.released_at IS NULL AND NEW.released_at IS NOT NULL THEN
        event_actor := NEW.released_by;
        event_name := 'legal_hold_released';
    ELSE
        RETURN NEW;
    END IF;

    INSERT INTO public.evidence_custody_events (
        organization_id, evidence_id, series_id, event_type, actor_user_id,
        actor_type, reason, object_sha256, details
    ) VALUES (
        NEW.organization_id, NEW.record_id, target_series, event_name,
        event_actor, 'user',
        CASE WHEN event_name = 'legal_hold_placed' THEN 'Evidence legal hold placed'
             ELSE 'Evidence legal hold released' END,
        target_hash,
        jsonb_build_object(
            'legal_hold_id', NEW.hold_id,
            'legal_hold_record_id', NEW.id,
            'record_type', NEW.record_type,
            'record_id', NEW.record_id,
            'placement_reason', NEW.reason,
            'placed_by', NEW.placed_by,
            'placed_at', NEW.placed_at,
            'released_by', NEW.released_by,
            'released_at', NEW.released_at,
            'release_reason', NEW.release_reason
        )
    );
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_legal_hold_records_evidence_custody_insert
    AFTER INSERT ON legal_hold_records
    FOR EACH ROW WHEN (NEW.record_type = 'evidence')
    EXECUTE FUNCTION record_evidence_legal_hold_event();
CREATE TRIGGER trg_legal_hold_records_evidence_custody_release
    AFTER UPDATE OF released_at ON legal_hold_records
    FOR EACH ROW WHEN (
        NEW.record_type = 'evidence' AND OLD.released_at IS NULL AND NEW.released_at IS NOT NULL
    ) EXECUTE FUNCTION record_evidence_legal_hold_event();

REVOKE ALL ON FUNCTION record_control_evidence_upload() FROM PUBLIC;
REVOKE ALL ON FUNCTION record_control_evidence_transition() FROM PUBLIC;
REVOKE ALL ON FUNCTION validate_evidence_review() FROM PUBLIC;
REVOKE ALL ON FUNCTION project_evidence_review() FROM PUBLIC;
REVOKE ALL ON FUNCTION submit_evidence_review(UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION append_evidence_access_custody_event(UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION expire_due_evidence(UUID, INTEGER) FROM PUBLIC;
REVOKE ALL ON FUNCTION record_evidence_legal_hold_event() FROM PUBLIC;

GRANT USAGE ON SCHEMA public TO complianceforge_evidence_chain_owner;
GRANT SELECT ON control_evidence, control_implementations, users,
    compliance_frameworks, framework_controls
    TO complianceforge_evidence_chain_owner;
GRANT SELECT, INSERT ON evidence_reviews, evidence_custody_events
    TO complianceforge_evidence_chain_owner;
GRANT SELECT, INSERT, UPDATE ON evidence_custody_chain_heads
    TO complianceforge_evidence_chain_owner;
GRANT UPDATE (review_status, reviewed_by, reviewed_at, review_notes, updated_at)
    ON control_evidence TO complianceforge_evidence_chain_owner;
GRANT UPDATE (lifecycle_status, is_current, review_status, updated_at)
    ON control_evidence TO complianceforge_evidence_chain_owner;

DO $grant_api$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'complianceforge_api') THEN
        EXECUTE 'GRANT SELECT ON TABLE public.evidence_reviews, public.evidence_custody_events, public.evidence_custody_chain_heads TO complianceforge_api';
        EXECUTE 'GRANT EXECUTE ON FUNCTION public.submit_evidence_review(UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT) TO complianceforge_api';
        EXECUTE 'GRANT EXECUTE ON FUNCTION public.append_evidence_access_custody_event(UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT) TO complianceforge_api';
        EXECUTE 'GRANT EXECUTE ON FUNCTION public.verify_evidence_custody_chain(UUID, UUID) TO complianceforge_api';
    END IF;
END;
$grant_api$;

-- Keep cross-tenant scheduling independent from FORCE RLS. The registry holds
-- only tenant identifiers (no tenant content), is populated for every existing
-- organization, and is maintained on organization creation. Runtime roles have
-- no direct table privileges; a separately provisioned worker group may invoke
-- the bounded keyset function below.
CREATE TABLE evidence_scheduler_tenants (
    organization_id UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    is_active BOOLEAN NOT NULL,
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO evidence_scheduler_tenants (organization_id, is_active)
SELECT organization.id,
       organization.status = 'active' AND organization.deleted_at IS NULL
FROM organizations AS organization
ON CONFLICT (organization_id) DO NOTHING;
REVOKE ALL ON evidence_scheduler_tenants FROM PUBLIC;

CREATE FUNCTION register_evidence_scheduler_tenant()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
BEGIN
    INSERT INTO public.evidence_scheduler_tenants (organization_id, is_active)
    VALUES (NEW.id, NEW.status = 'active' AND NEW.deleted_at IS NULL)
    ON CONFLICT (organization_id) DO UPDATE
    SET is_active = EXCLUDED.is_active;
    RETURN NEW;
END;
$$;
REVOKE ALL ON FUNCTION register_evidence_scheduler_tenant() FROM PUBLIC;
CREATE TRIGGER trg_organizations_register_evidence_scheduler
    AFTER INSERT OR UPDATE OF status, deleted_at ON organizations
    FOR EACH ROW EXECUTE FUNCTION register_evidence_scheduler_tenant();

CREATE FUNCTION evidence_due_tenants(
    requested_limit INTEGER DEFAULT 100,
    requested_after UUID DEFAULT NULL
)
RETURNS TABLE (organization_id UUID)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
    SELECT tenant.organization_id
    FROM public.evidence_scheduler_tenants AS tenant
    WHERE tenant.is_active
      AND (requested_after IS NULL OR tenant.organization_id > requested_after)
    ORDER BY tenant.organization_id
    LIMIT LEAST(GREATEST(COALESCE(requested_limit, 100), 1), 1000)
$$;
REVOKE ALL ON FUNCTION evidence_due_tenants(INTEGER, UUID) FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO complianceforge_evidence_registry_owner;
DO $grant$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'complianceforge_scheduler') THEN
        EXECUTE 'GRANT EXECUTE ON FUNCTION public.evidence_due_tenants(INTEGER, UUID) TO complianceforge_scheduler';
        EXECUTE 'GRANT EXECUTE ON FUNCTION public.expire_due_evidence(UUID, INTEGER) TO complianceforge_scheduler';
        EXECUTE 'GRANT SELECT ON TABLE public.evidence_collection_configs, public.control_implementations, public.control_evidence, public.compliance_frameworks, public.framework_controls TO complianceforge_scheduler';
    END IF;
END;
$grant$;

-- Flush deferred relationship checks created by the legacy-history backfill
-- before changing RLS metadata on the same tables.
SET CONSTRAINTS ALL IMMEDIATE;

ALTER TABLE evidence_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE evidence_reviews FORCE ROW LEVEL SECURITY;
ALTER TABLE evidence_custody_chain_heads ENABLE ROW LEVEL SECURITY;
ALTER TABLE evidence_custody_chain_heads FORCE ROW LEVEL SECURITY;
ALTER TABLE evidence_custody_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE evidence_custody_events FORCE ROW LEVEL SECURITY;

CREATE POLICY evidence_reviews_select ON evidence_reviews
    FOR SELECT USING (
        organization_id = get_current_tenant()
        OR current_user = 'complianceforge_evidence_chain_owner'
    );
CREATE POLICY evidence_reviews_insert ON evidence_reviews
    FOR INSERT WITH CHECK (
        organization_id = get_current_tenant()
        AND current_user = 'complianceforge_evidence_chain_owner'
    );
CREATE POLICY evidence_custody_chain_heads_select ON evidence_custody_chain_heads
    FOR SELECT USING (
        organization_id = get_current_tenant()
        OR current_user = 'complianceforge_evidence_chain_owner'
    );
-- Only the exact non-login SECURITY DEFINER owner may advance a head. Direct
-- runtime DML is denied even if a broad table grant is accidentally present.
CREATE POLICY evidence_custody_chain_heads_chain_insert ON evidence_custody_chain_heads
    FOR INSERT WITH CHECK (
        organization_id = get_current_tenant()
        AND current_user = 'complianceforge_evidence_chain_owner'
    );
CREATE POLICY evidence_custody_chain_heads_chain_update ON evidence_custody_chain_heads
    FOR UPDATE USING (
        organization_id = get_current_tenant()
        AND current_user = 'complianceforge_evidence_chain_owner'
    ) WITH CHECK (
        organization_id = get_current_tenant()
        AND current_user = 'complianceforge_evidence_chain_owner'
    );
CREATE POLICY evidence_custody_events_select ON evidence_custody_events
    FOR SELECT USING (
        organization_id = get_current_tenant()
        OR current_user = 'complianceforge_evidence_chain_owner'
    );
CREATE POLICY evidence_custody_events_insert ON evidence_custody_events
    FOR INSERT WITH CHECK (
        organization_id = get_current_tenant()
        AND current_user = 'complianceforge_evidence_chain_owner'
    );

COMMENT ON COLUMN control_evidence.series_id IS
    'Stable root evidence identifier shared by every immutable version.';
COMMENT ON COLUMN control_evidence.content_fingerprint IS
    'Canonical SHA-256 over immutable evidence content and provenance metadata.';
COMMENT ON TABLE evidence_reviews IS
    'Append-only reviewer sign-off records bound to the exact evidence content fingerprint.';
COMMENT ON TABLE evidence_custody_events IS
    'Append-only, per-evidence SHA-256 chain of custody and lifecycle events.';
COMMENT ON FUNCTION evidence_due_tenants(INTEGER, UUID) IS
    'Restricted, keyset-paginated scheduler capability returning tenant UUIDs with due evidence work.';

-- Ownership transfer is deliberately last: it leaves the immutable ledgers
-- and every trusted writer owned by exact, non-login principals. Migration
-- credentials may retain membership for controlled rollback, but production
-- API/worker identities must never be members (startup posture checks enforce
-- this boundary).
ALTER TABLE evidence_reviews OWNER TO complianceforge_evidence_chain_owner;
ALTER TABLE evidence_custody_events OWNER TO complianceforge_evidence_chain_owner;
ALTER TABLE evidence_custody_chain_heads OWNER TO complianceforge_evidence_chain_owner;
ALTER FUNCTION append_evidence_custody_event_chain()
    OWNER TO complianceforge_evidence_chain_owner;
ALTER FUNCTION record_control_evidence_upload()
    OWNER TO complianceforge_evidence_chain_owner;
ALTER FUNCTION record_control_evidence_transition()
    OWNER TO complianceforge_evidence_chain_owner;
ALTER FUNCTION validate_evidence_review()
    OWNER TO complianceforge_evidence_chain_owner;
ALTER FUNCTION project_evidence_review()
    OWNER TO complianceforge_evidence_chain_owner;
ALTER FUNCTION submit_evidence_review(UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT)
    OWNER TO complianceforge_evidence_chain_owner;
ALTER FUNCTION append_evidence_access_custody_event(UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT)
    OWNER TO complianceforge_evidence_chain_owner;
ALTER FUNCTION expire_due_evidence(UUID, INTEGER)
    OWNER TO complianceforge_evidence_chain_owner;
ALTER FUNCTION record_evidence_legal_hold_event()
    OWNER TO complianceforge_evidence_chain_owner;

ALTER TABLE evidence_scheduler_tenants
    OWNER TO complianceforge_evidence_registry_owner;
ALTER FUNCTION register_evidence_scheduler_tenant()
    OWNER TO complianceforge_evidence_registry_owner;
ALTER FUNCTION evidence_due_tenants(INTEGER, UUID)
    OWNER TO complianceforge_evidence_registry_owner;

REVOKE CREATE ON SCHEMA public
    FROM complianceforge_evidence_chain_owner,
         complianceforge_evidence_registry_owner;

ALTER TABLE organizations FORCE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
ALTER TABLE control_implementations FORCE ROW LEVEL SECURITY;
ALTER TABLE control_evidence FORCE ROW LEVEL SECURITY;
ALTER TABLE evidence_collection_configs FORCE ROW LEVEL SECURITY;
ALTER TABLE evidence_collection_runs FORCE ROW LEVEL SECURITY;
ALTER TABLE legal_holds FORCE ROW LEVEL SECURITY;
ALTER TABLE legal_hold_records FORCE ROW LEVEL SECURITY;

COMMIT;
