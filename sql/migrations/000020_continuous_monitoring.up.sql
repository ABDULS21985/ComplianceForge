-- Migration 020: Continuous Monitoring & Evidence Collection
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - Automated evidence collection via multiple methods: API fetch, file watch,
--     script execution, email parsing, and webhook receive — replacing manual-only
--     workflows that cannot scale for continuous compliance programs
--   - Collection configs tie directly to control_implementations, enabling automated
--     evidence gathering per control with acceptance criteria validation
--   - Compliance monitors provide a generic watchdog system that checks control
--     effectiveness, evidence freshness, KRI thresholds, policy attestations,
--     vendor assessments, and training completion on a cron schedule
--   - Drift events capture any deviation from the desired compliance posture,
--     with severity classification and full acknowledgement/resolution workflow
--   - All tables have organization_id for RLS tenant isolation
--   - JSONB configs (api_config, file_config, script_config, webhook_config) allow
--     flexible per-method configuration without schema migrations
--   - consecutive_failures tracking enables circuit-breaker patterns in the
--     collection scheduler to avoid hammering broken integrations

-- ============================================================================
-- ENUM TYPES
-- ============================================================================

CREATE TYPE collection_method AS ENUM (
    'manual',
    'api_fetch',
    'file_watch',
    'script_execution',
    'email_parse',
    'webhook_receive'
);

CREATE TYPE collection_run_status AS ENUM (
    'scheduled',
    'running',
    'success',
    'failed',
    'timeout',
    'validation_failed'
);

CREATE TYPE monitor_type AS ENUM (
    'control_effectiveness',
    'evidence_freshness',
    'kri_threshold',
    'policy_attestation',
    'vendor_assessment',
    'training_completion'
);

CREATE TYPE monitor_check_status AS ENUM (
    'passing',
    'failing',
    'unknown'
);

CREATE TYPE drift_type AS ENUM (
    'control_degraded',
    'evidence_expired',
    'kri_breached',
    'policy_unattested',
    'vendor_overdue',
    'training_expired',
    'score_dropped'
);

-- ============================================================================
-- TABLE: evidence_collection_configs
-- ============================================================================

CREATE TABLE evidence_collection_configs (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    control_implementation_id   UUID NOT NULL REFERENCES control_implementations(id) ON DELETE CASCADE,
    name                        VARCHAR(200) NOT NULL,
    collection_method           collection_method NOT NULL,

    -- Scheduling
    schedule_cron               VARCHAR(100),
    schedule_description        VARCHAR(200),

    -- Method-specific configuration (only the relevant one is populated)
    api_config                  JSONB NOT NULL DEFAULT '{}',
    file_config                 JSONB NOT NULL DEFAULT '{}',
    script_config               JSONB NOT NULL DEFAULT '{}',
    webhook_config              JSONB NOT NULL DEFAULT '{}',

    -- Validation
    acceptance_criteria         JSONB NOT NULL DEFAULT '[]',
    failure_threshold           INT NOT NULL DEFAULT 1,
    auto_update_control_status  BOOLEAN NOT NULL DEFAULT false,

    -- State
    is_active                   BOOLEAN NOT NULL DEFAULT true,
    last_collection_at          TIMESTAMPTZ,
    last_collection_status      VARCHAR(50),
    next_collection_at          TIMESTAMPTZ,
    consecutive_failures        INT NOT NULL DEFAULT 0,

    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_ev_collect_cfg_org ON evidence_collection_configs(organization_id);
CREATE INDEX idx_ev_collect_cfg_ctrl ON evidence_collection_configs(control_implementation_id);
CREATE INDEX idx_ev_collect_cfg_method ON evidence_collection_configs(organization_id, collection_method);
CREATE INDEX idx_ev_collect_cfg_active ON evidence_collection_configs(organization_id, is_active)
    WHERE is_active = true;
CREATE INDEX idx_ev_collect_cfg_next ON evidence_collection_configs(next_collection_at)
    WHERE is_active = true AND next_collection_at IS NOT NULL;
CREATE INDEX idx_ev_collect_cfg_failures ON evidence_collection_configs(consecutive_failures)
    WHERE consecutive_failures > 0;

CREATE TRIGGER trg_ev_collect_cfg_updated_at
    BEFORE UPDATE ON evidence_collection_configs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE evidence_collection_configs IS 'Defines how evidence is automatically collected for each control implementation. Supports multiple collection methods (API, file watch, script, email, webhook) with cron-based scheduling, acceptance criteria validation, and circuit-breaker pattern via consecutive_failures tracking.';
COMMENT ON COLUMN evidence_collection_configs.api_config IS 'API collection settings: url, method, headers, auth_type, auth_credentials_encrypted, response_path, expected_format. Only populated when collection_method = api_fetch.';
COMMENT ON COLUMN evidence_collection_configs.acceptance_criteria IS 'JSON array of validation rules applied to collected data. Each criterion specifies a field, operator, and expected value.';
COMMENT ON COLUMN evidence_collection_configs.failure_threshold IS 'Number of consecutive failures before the config is auto-deactivated or escalated. Works with consecutive_failures for circuit-breaker logic.';
COMMENT ON COLUMN evidence_collection_configs.auto_update_control_status IS 'When true, successful/failed collection runs automatically update the linked control_implementation status.';

-- ============================================================================
-- TABLE: evidence_collection_runs
-- ============================================================================

CREATE TABLE evidence_collection_runs (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    config_id                   UUID NOT NULL REFERENCES evidence_collection_configs(id) ON DELETE CASCADE,
    control_implementation_id   UUID NOT NULL REFERENCES control_implementations(id) ON DELETE CASCADE,
    status                      collection_run_status NOT NULL DEFAULT 'scheduled',

    -- Timing
    started_at                  TIMESTAMPTZ,
    completed_at                TIMESTAMPTZ,
    duration_ms                 INT,

    -- Results
    collected_data              JSONB,
    validation_results          JSONB,
    all_criteria_passed         BOOLEAN,
    evidence_id                 UUID REFERENCES control_evidence(id) ON DELETE SET NULL,

    -- Error handling
    error_message               TEXT,
    metadata                    JSONB NOT NULL DEFAULT '{}',

    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_ev_collect_runs_config_time ON evidence_collection_runs(config_id, created_at DESC);
CREATE INDEX idx_ev_collect_runs_status ON evidence_collection_runs(status);
CREATE INDEX idx_ev_collect_runs_org ON evidence_collection_runs(organization_id);
CREATE INDEX idx_ev_collect_runs_ctrl ON evidence_collection_runs(control_implementation_id);
CREATE INDEX idx_ev_collect_runs_evidence ON evidence_collection_runs(evidence_id)
    WHERE evidence_id IS NOT NULL;

COMMENT ON TABLE evidence_collection_runs IS 'Individual execution records for evidence collection configs. Tracks each collection attempt with timing, collected data, validation results, and linkage to the created evidence record. Immutable audit trail — runs are never updated after completion.';
COMMENT ON COLUMN evidence_collection_runs.all_criteria_passed IS 'True when all acceptance_criteria from the config were satisfied. Null if validation was not performed.';
COMMENT ON COLUMN evidence_collection_runs.evidence_id IS 'References the control_evidence record created from this run, if collection and validation succeeded.';
COMMENT ON COLUMN evidence_collection_runs.duration_ms IS 'Wall-clock duration of the collection run in milliseconds. Used for performance monitoring and timeout detection.';

-- ============================================================================
-- TABLE: compliance_monitors
-- ============================================================================

CREATE TABLE compliance_monitors (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                    VARCHAR(200) NOT NULL,
    monitor_type            monitor_type NOT NULL,

    -- Target
    target_entity_type      VARCHAR(50) NOT NULL,
    target_entity_id        UUID,

    -- Schedule & conditions
    check_frequency_cron    VARCHAR(100),
    conditions              JSONB NOT NULL DEFAULT '{}',

    -- Alerting
    alert_on_failure        BOOLEAN NOT NULL DEFAULT true,
    alert_severity          VARCHAR(20) NOT NULL DEFAULT 'high',

    -- State
    is_active               BOOLEAN NOT NULL DEFAULT true,
    last_check_at           TIMESTAMPTZ,
    last_check_status       monitor_check_status NOT NULL DEFAULT 'unknown',
    consecutive_failures    INT NOT NULL DEFAULT 0,
    failure_since           TIMESTAMPTZ,

    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_monitor_alert_severity CHECK (
        alert_severity IN ('critical', 'high', 'medium', 'low')
    )
);

-- Indexes
CREATE INDEX idx_comp_monitors_org ON compliance_monitors(organization_id);
CREATE INDEX idx_comp_monitors_type ON compliance_monitors(organization_id, monitor_type);
CREATE INDEX idx_comp_monitors_target ON compliance_monitors(target_entity_type, target_entity_id);
CREATE INDEX idx_comp_monitors_active ON compliance_monitors(organization_id, is_active)
    WHERE is_active = true;
CREATE INDEX idx_comp_monitors_status ON compliance_monitors(organization_id, last_check_status);
CREATE INDEX idx_comp_monitors_failing ON compliance_monitors(failure_since)
    WHERE last_check_status = 'failing';

CREATE TRIGGER trg_comp_monitors_updated_at
    BEFORE UPDATE ON compliance_monitors
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE compliance_monitors IS 'Configurable watchdogs that continuously check compliance posture. Each monitor targets a specific entity (control, framework, vendor, etc.) and evaluates conditions on a cron schedule. Supports control effectiveness, evidence freshness, KRI thresholds, policy attestation, vendor assessment, and training completion checks.';
COMMENT ON COLUMN compliance_monitors.conditions IS 'JSONB object defining the check logic: thresholds, comparison operators, lookback periods, and entity-specific parameters.';
COMMENT ON COLUMN compliance_monitors.failure_since IS 'Timestamp of when the monitor first entered failing state in the current failure streak. NULL when passing.';

-- ============================================================================
-- TABLE: compliance_monitor_results
-- ============================================================================

CREATE TABLE compliance_monitor_results (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    monitor_id          UUID NOT NULL REFERENCES compliance_monitors(id) ON DELETE CASCADE,
    status              monitor_check_status NOT NULL,
    check_time          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    result_data         JSONB,
    message             TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_comp_mon_results_monitor_time ON compliance_monitor_results(monitor_id, check_time DESC);
CREATE INDEX idx_comp_mon_results_org ON compliance_monitor_results(organization_id);
CREATE INDEX idx_comp_mon_results_status ON compliance_monitor_results(organization_id, status);

COMMENT ON TABLE compliance_monitor_results IS 'Historical record of every compliance monitor check. Provides an audit trail of compliance posture over time and feeds into trend analysis and drift detection.';
COMMENT ON COLUMN compliance_monitor_results.result_data IS 'Raw check output including measured values, thresholds evaluated, and any entity-specific details.';

-- ============================================================================
-- TABLE: compliance_drift_events
-- ============================================================================

CREATE TABLE compliance_drift_events (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    drift_type          drift_type NOT NULL,
    severity            VARCHAR(20) NOT NULL,

    -- Target entity
    entity_type         VARCHAR(50) NOT NULL,
    entity_id           UUID,
    entity_ref          VARCHAR(50),

    -- Description
    description         TEXT NOT NULL,
    previous_state      VARCHAR(100),
    current_state       VARCHAR(100),

    -- Timeline
    detected_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acknowledged_at     TIMESTAMPTZ,
    acknowledged_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    resolved_at         TIMESTAMPTZ,
    resolved_by         UUID REFERENCES users(id) ON DELETE SET NULL,
    resolution_notes    TEXT,

    -- Notification
    notification_sent   BOOLEAN NOT NULL DEFAULT false,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_drift_severity CHECK (
        severity IN ('critical', 'high', 'medium', 'low')
    )
);

-- Indexes
CREATE INDEX idx_drift_events_org_type ON compliance_drift_events(organization_id, drift_type);
CREATE INDEX idx_drift_events_org_severity ON compliance_drift_events(organization_id, severity);
CREATE INDEX idx_drift_events_active ON compliance_drift_events(organization_id, resolved_at)
    WHERE resolved_at IS NULL;
CREATE INDEX idx_drift_events_entity ON compliance_drift_events(entity_type, entity_id);
CREATE INDEX idx_drift_events_detected ON compliance_drift_events(detected_at DESC);
CREATE INDEX idx_drift_events_ack_by ON compliance_drift_events(acknowledged_by)
    WHERE acknowledged_by IS NOT NULL;
CREATE INDEX idx_drift_events_resolved_by ON compliance_drift_events(resolved_by)
    WHERE resolved_by IS NOT NULL;

COMMENT ON TABLE compliance_drift_events IS 'Records deviations from the desired compliance posture. Drift events are generated by compliance monitors, evidence collection failures, or external triggers. Supports a full lifecycle: detection, acknowledgement, and resolution with audit trail. Unresolved events (resolved_at IS NULL) represent active compliance gaps.';
COMMENT ON COLUMN compliance_drift_events.entity_ref IS 'Human-readable reference for the affected entity (e.g., control code, policy number). Useful for notifications and reports without requiring a join.';
COMMENT ON COLUMN compliance_drift_events.previous_state IS 'State of the entity before the drift was detected (e.g., "effective", "score:92").';
COMMENT ON COLUMN compliance_drift_events.current_state IS 'State of the entity after the drift was detected (e.g., "degraded", "score:74").';

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- evidence_collection_configs
ALTER TABLE evidence_collection_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE evidence_collection_configs FORCE ROW LEVEL SECURITY;

CREATE POLICY ev_collect_cfg_tenant_select ON evidence_collection_configs FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY ev_collect_cfg_tenant_insert ON evidence_collection_configs FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY ev_collect_cfg_tenant_update ON evidence_collection_configs FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY ev_collect_cfg_tenant_delete ON evidence_collection_configs FOR DELETE
    USING (organization_id = get_current_tenant());

-- evidence_collection_runs
ALTER TABLE evidence_collection_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE evidence_collection_runs FORCE ROW LEVEL SECURITY;

CREATE POLICY ev_collect_runs_tenant_select ON evidence_collection_runs FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY ev_collect_runs_tenant_insert ON evidence_collection_runs FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY ev_collect_runs_tenant_update ON evidence_collection_runs FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY ev_collect_runs_tenant_delete ON evidence_collection_runs FOR DELETE
    USING (organization_id = get_current_tenant());

-- compliance_monitors
ALTER TABLE compliance_monitors ENABLE ROW LEVEL SECURITY;
ALTER TABLE compliance_monitors FORCE ROW LEVEL SECURITY;

CREATE POLICY comp_monitors_tenant_select ON compliance_monitors FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY comp_monitors_tenant_insert ON compliance_monitors FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY comp_monitors_tenant_update ON compliance_monitors FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY comp_monitors_tenant_delete ON compliance_monitors FOR DELETE
    USING (organization_id = get_current_tenant());

-- compliance_monitor_results
ALTER TABLE compliance_monitor_results ENABLE ROW LEVEL SECURITY;
ALTER TABLE compliance_monitor_results FORCE ROW LEVEL SECURITY;

CREATE POLICY comp_mon_results_tenant_select ON compliance_monitor_results FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY comp_mon_results_tenant_insert ON compliance_monitor_results FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY comp_mon_results_tenant_update ON compliance_monitor_results FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY comp_mon_results_tenant_delete ON compliance_monitor_results FOR DELETE
    USING (organization_id = get_current_tenant());

-- compliance_drift_events
ALTER TABLE compliance_drift_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE compliance_drift_events FORCE ROW LEVEL SECURITY;

CREATE POLICY drift_events_tenant_select ON compliance_drift_events FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY drift_events_tenant_insert ON compliance_drift_events FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY drift_events_tenant_update ON compliance_drift_events FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY drift_events_tenant_delete ON compliance_drift_events FOR DELETE
    USING (organization_id = get_current_tenant());
