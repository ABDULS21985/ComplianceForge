-- Migration 056 DOWN: remove evidence lifecycle extensions while preserving
-- the legacy control_evidence rows and their latest review projection.

BEGIN;

-- A structural rollback cannot represent immutable review/custody history in
-- schema 055. Refuse silent audit loss after the feature has been used. A DBA
-- may set this transaction-local override only after exporting and approving
-- destruction of the lifecycle ledger.
SET LOCAL ROLE complianceforge_evidence_chain_owner;
DO $$
BEGIN
    IF current_setting('app.allow_lossy_evidence_lifecycle_downgrade', true)
           IS DISTINCT FROM 'true'
       AND (
           EXISTS (SELECT 1 FROM evidence_reviews)
           OR EXISTS (
               SELECT 1 FROM evidence_custody_events AS event
               WHERE event.chain_sequence <> 1
                  OR event.reason <> 'Evidence record migrated into immutable custody history'
           )
           OR EXISTS (
               SELECT 1 FROM control_evidence AS evidence
               WHERE evidence.version_number <> 1
                  OR evidence.supersedes_evidence_id IS NOT NULL
                  OR evidence.superseded_by_evidence_id IS NOT NULL
           )
       ) THEN
        RAISE EXCEPTION 'migration 056 downgrade would destroy evidence review/custody history; export it and SET app.allow_lossy_evidence_lifecycle_downgrade=true only with explicit approval'
            USING ERRCODE = '55000';
    END IF;
END;
$$;
RESET ROLE;

-- The migration owner must remove reserved proof metadata and validate the
-- restored schema-055 foreign keys across every tenant. FORCE RLS would make
-- populated parent rows invisible to a NOSUPERUSER/NOBYPASSRLS migration
-- owner and cause false FK violations. RLS remains enabled throughout; these
-- ACCESS EXCLUSIVE changes are transaction-local and every relation is
-- re-forced before commit.
ALTER TABLE users NO FORCE ROW LEVEL SECURITY;
ALTER TABLE control_implementations NO FORCE ROW LEVEL SECURITY;
ALTER TABLE control_evidence NO FORCE ROW LEVEL SECURITY;
ALTER TABLE evidence_collection_configs NO FORCE ROW LEVEL SECURITY;
ALTER TABLE evidence_collection_runs NO FORCE ROW LEVEL SECURITY;

DROP FUNCTION IF EXISTS evidence_due_tenants(INTEGER, UUID);
DROP FUNCTION IF EXISTS evidence_due_tenants(INTEGER);
DROP TRIGGER IF EXISTS trg_organizations_register_evidence_scheduler ON organizations;
DROP FUNCTION IF EXISTS register_evidence_scheduler_tenant();
DROP TABLE IF EXISTS evidence_scheduler_tenants;

DROP TRIGGER IF EXISTS trg_legal_holds_lifecycle_guard ON legal_holds;
DROP TRIGGER IF EXISTS trg_legal_hold_records_immutable ON legal_hold_records;
DROP TRIGGER IF EXISTS trg_legal_hold_custodians_immutable ON legal_hold_custodians;
DROP TRIGGER IF EXISTS trg_legal_hold_records_evidence_custody_release ON legal_hold_records;
DROP TRIGGER IF EXISTS trg_legal_hold_records_evidence_custody_insert ON legal_hold_records;
DROP FUNCTION IF EXISTS record_evidence_legal_hold_event();
DROP FUNCTION IF EXISTS enforce_legal_hold_lifecycle();
DROP FUNCTION IF EXISTS enforce_legal_hold_record_immutability();
DROP FUNCTION IF EXISTS enforce_legal_hold_custodian_immutability();

DROP FUNCTION IF EXISTS append_evidence_access_custody_event(UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT);
DROP FUNCTION IF EXISTS submit_evidence_review(UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT);
DROP FUNCTION IF EXISTS expire_due_evidence(UUID, INTEGER);

DROP TRIGGER IF EXISTS trg_evidence_reviews_project ON evidence_reviews;
DROP TRIGGER IF EXISTS trg_evidence_reviews_validate ON evidence_reviews;
DROP FUNCTION IF EXISTS project_evidence_review();
DROP FUNCTION IF EXISTS validate_evidence_review();

DROP TRIGGER IF EXISTS trg_control_evidence_custody_transition ON control_evidence;
DROP TRIGGER IF EXISTS trg_control_evidence_custody_upload ON control_evidence;
DROP TRIGGER IF EXISTS trg_control_evidence_series_consistency ON control_evidence;
DROP TRIGGER IF EXISTS trg_control_evidence_immutable_content ON control_evidence;
DROP TRIGGER IF EXISTS trg_control_evidence_initialize_version ON control_evidence;
DROP FUNCTION IF EXISTS record_control_evidence_transition();
DROP FUNCTION IF EXISTS record_control_evidence_upload();
DROP FUNCTION IF EXISTS validate_control_evidence_series();
DROP FUNCTION IF EXISTS enforce_control_evidence_immutability();
DROP FUNCTION IF EXISTS initialize_control_evidence_version();

DROP FUNCTION IF EXISTS verify_evidence_custody_chain(UUID, UUID);
DROP TRIGGER IF EXISTS trg_evidence_reviews_immutable ON evidence_reviews;
DROP TRIGGER IF EXISTS trg_evidence_custody_events_immutable ON evidence_custody_events;
DROP TRIGGER IF EXISTS trg_evidence_custody_event_chain ON evidence_custody_events;
DROP FUNCTION IF EXISTS append_evidence_custody_event_chain();
DROP FUNCTION IF EXISTS prevent_evidence_history_mutation();

DROP POLICY IF EXISTS evidence_custody_events_insert ON evidence_custody_events;
DROP POLICY IF EXISTS evidence_custody_events_select ON evidence_custody_events;
DROP POLICY IF EXISTS evidence_custody_chain_heads_chain_update ON evidence_custody_chain_heads;
DROP POLICY IF EXISTS evidence_custody_chain_heads_chain_insert ON evidence_custody_chain_heads;
DROP POLICY IF EXISTS evidence_custody_chain_heads_select ON evidence_custody_chain_heads;
DROP POLICY IF EXISTS evidence_reviews_insert ON evidence_reviews;
DROP POLICY IF EXISTS evidence_reviews_select ON evidence_reviews;

DROP TABLE IF EXISTS evidence_custody_events;
DROP TABLE IF EXISTS evidence_custody_chain_heads;
DROP TABLE IF EXISTS evidence_reviews;

DROP FUNCTION IF EXISTS calculate_evidence_custody_event_hash(
    UUID, UUID, UUID, UUID, BIGINT, BYTEA, TEXT, UUID, TEXT, TEXT,
    TEXT, TEXT, JSONB, TIMESTAMPTZ
);

DROP TRIGGER IF EXISTS trg_evidence_collection_runs_immutable ON evidence_collection_runs;
DROP FUNCTION IF EXISTS enforce_evidence_collection_run_immutability();

DROP INDEX IF EXISTS idx_control_evidence_expiry_due;
DROP INDEX IF EXISTS idx_control_evidence_lifecycle;
DROP INDEX IF EXISTS uq_control_evidence_series_current;
DROP INDEX IF EXISTS uq_control_evidence_series_version;
DROP INDEX IF EXISTS uq_evidence_collection_runs_active_config;

ALTER TABLE evidence_collection_runs
    DROP CONSTRAINT IF EXISTS fk_evidence_collection_run_evidence_tenant,
    DROP CONSTRAINT IF EXISTS fk_evidence_collection_run_implementation_tenant,
    DROP CONSTRAINT IF EXISTS fk_evidence_collection_run_config_tenant;
ALTER TABLE evidence_collection_configs
    DROP CONSTRAINT IF EXISTS fk_evidence_collection_config_implementation_tenant;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_constraint
        WHERE conrelid = 'evidence_collection_configs'::regclass
          AND conname = 'evidence_collection_configs_control_implementation_id_fkey'
    ) THEN
        ALTER TABLE evidence_collection_configs
            ADD CONSTRAINT evidence_collection_configs_control_implementation_id_fkey
            FOREIGN KEY (control_implementation_id)
            REFERENCES control_implementations(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_constraint
        WHERE conrelid = 'evidence_collection_runs'::regclass
          AND conname = 'evidence_collection_runs_config_id_fkey'
    ) THEN
        ALTER TABLE evidence_collection_runs
            ADD CONSTRAINT evidence_collection_runs_config_id_fkey FOREIGN KEY (config_id)
            REFERENCES evidence_collection_configs(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_constraint
        WHERE conrelid = 'evidence_collection_runs'::regclass
          AND conname = 'evidence_collection_runs_control_implementation_id_fkey'
    ) THEN
        ALTER TABLE evidence_collection_runs
            ADD CONSTRAINT evidence_collection_runs_control_implementation_id_fkey
            FOREIGN KEY (control_implementation_id)
            REFERENCES control_implementations(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_constraint
        WHERE conrelid = 'evidence_collection_runs'::regclass
          AND conname = 'evidence_collection_runs_evidence_id_fkey'
    ) THEN
        ALTER TABLE evidence_collection_runs
            ADD CONSTRAINT evidence_collection_runs_evidence_id_fkey FOREIGN KEY (evidence_id)
            REFERENCES control_evidence(id) ON DELETE SET NULL;
    END IF;
END;
$$;

ALTER TABLE evidence_collection_configs
    DROP CONSTRAINT IF EXISTS uq_evidence_collection_configs_org_id;

REVOKE UPDATE (lifecycle_status, is_current, review_status, updated_at)
    ON control_evidence FROM complianceforge_evidence_chain_owner;
REVOKE UPDATE (review_status, reviewed_by, reviewed_at, review_notes, updated_at)
    ON control_evidence FROM complianceforge_evidence_chain_owner;

UPDATE control_evidence
SET metadata = metadata - 'automated_proof'
WHERE metadata->'automated_proof'->>'proof_schema' IN (
    'automated-evidence/v1',
    'automated-evidence/postgresql-jsonb-v1'
);

ALTER TABLE control_evidence
    DROP CONSTRAINT IF EXISTS chk_control_evidence_lifecycle_shape,
    DROP CONSTRAINT IF EXISTS chk_control_evidence_expiry_projection,
    DROP CONSTRAINT IF EXISTS chk_control_evidence_version_links,
    DROP CONSTRAINT IF EXISTS chk_control_evidence_lifecycle_status,
    DROP CONSTRAINT IF EXISTS chk_control_evidence_fingerprint,
    DROP CONSTRAINT IF EXISTS chk_control_evidence_version_reason,
    DROP CONSTRAINT IF EXISTS chk_control_evidence_version,
    DROP CONSTRAINT IF EXISTS fk_control_evidence_successor,
    DROP CONSTRAINT IF EXISTS fk_control_evidence_predecessor,
    DROP CONSTRAINT IF EXISTS fk_control_evidence_series,
    DROP CONSTRAINT IF EXISTS fk_control_evidence_reviewer_tenant,
    DROP CONSTRAINT IF EXISTS fk_control_evidence_collector_tenant,
    DROP CONSTRAINT IF EXISTS fk_control_evidence_implementation_tenant,
    DROP CONSTRAINT IF EXISTS uq_control_evidence_org_id_series,
    DROP CONSTRAINT IF EXISTS uq_control_evidence_org_id,
    DROP COLUMN IF EXISTS content_fingerprint,
    DROP COLUMN IF EXISTS version_reason,
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS lifecycle_status,
    DROP COLUMN IF EXISTS superseded_at,
    DROP COLUMN IF EXISTS superseded_by_evidence_id,
    DROP COLUMN IF EXISTS supersedes_evidence_id,
    DROP COLUMN IF EXISTS version_number,
    DROP COLUMN IF EXISTS series_id;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_constraint
        WHERE conrelid = 'control_evidence'::regclass
          AND conname = 'control_evidence_collected_by_fkey'
    ) THEN
        ALTER TABLE control_evidence
            ADD CONSTRAINT control_evidence_collected_by_fkey FOREIGN KEY (collected_by)
            REFERENCES users(id) ON DELETE SET NULL;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_constraint
        WHERE conrelid = 'control_evidence'::regclass
          AND conname = 'control_evidence_reviewed_by_fkey'
    ) THEN
        ALTER TABLE control_evidence
            ADD CONSTRAINT control_evidence_reviewed_by_fkey FOREIGN KEY (reviewed_by)
            REFERENCES users(id) ON DELETE SET NULL;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_constraint
        WHERE conrelid = 'control_evidence'::regclass
          AND conname = 'control_evidence_control_implementation_id_fkey'
    ) THEN
        ALTER TABLE control_evidence
            ADD CONSTRAINT control_evidence_control_implementation_id_fkey
            FOREIGN KEY (control_implementation_id)
            REFERENCES control_implementations(id) ON DELETE CASCADE;
    END IF;
END;
$$;

ALTER TABLE control_implementations
    DROP CONSTRAINT IF EXISTS uq_control_implementations_org_id;

DROP FUNCTION IF EXISTS calculate_control_evidence_fingerprint(JSONB);

-- Restore the schema-055 ACL surface. These grants exist only so the exact
-- non-login owner can execute migration-056's trusted writers.
REVOKE SELECT ON control_evidence, control_implementations, users,
    compliance_frameworks, framework_controls
    FROM complianceforge_evidence_chain_owner;
REVOKE USAGE ON SCHEMA public
    FROM complianceforge_evidence_chain_owner,
         complianceforge_evidence_registry_owner;

DO $revoke_scheduler$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'complianceforge_scheduler') THEN
        EXECUTE 'REVOKE SELECT ON TABLE public.evidence_collection_configs, public.control_implementations, public.control_evidence, public.compliance_frameworks, public.framework_controls FROM complianceforge_scheduler';
    END IF;
END;
$revoke_scheduler$;

ALTER TABLE users FORCE ROW LEVEL SECURITY;
ALTER TABLE control_implementations FORCE ROW LEVEL SECURITY;
ALTER TABLE control_evidence FORCE ROW LEVEL SECURITY;
ALTER TABLE evidence_collection_configs FORCE ROW LEVEL SECURITY;
ALTER TABLE evidence_collection_runs FORCE ROW LEVEL SECURITY;

COMMIT;
