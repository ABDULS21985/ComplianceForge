-- Migration 035: Compliance Calendar (Prompt 31)
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - calendar_events is the central table for all compliance deadlines, review
--     dates, audit schedules, certification renewals, and regulatory filings.
--     Events are linked to source entities (policies, controls, risks, etc.)
--     via polymorphic source_entity_type + source_entity_id.
--   - Deduplication enforced via UNIQUE(organization_id, source_entity_type,
--     source_entity_id, event_type) so the same entity cannot produce duplicate
--     events of the same type.
--   - Recurring events use RRULE format (recurrence_rule) with optional
--     parent_event_id for generated occurrences.
--   - calendar_subscriptions allow users to filter and subscribe to specific
--     event types/categories, with optional iCal export.
--   - calendar_sync_configs control how different source entity types auto-
--     generate calendar events.
--   - Refs auto-generated: CEV-YYYY-NNNN.
--   - All tables are tenant-isolated via RLS on organization_id.

-- ============================================================================
-- TABLE: calendar_events
-- ============================================================================

CREATE TABLE calendar_events (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    event_ref                   VARCHAR(30),
    title                       VARCHAR(500) NOT NULL,
    description                 TEXT,
    event_type                  VARCHAR(50) NOT NULL
                                CHECK (event_type IN (
                                    'policy_review', 'policy_expiry', 'policy_approval_due',
                                    'control_assessment', 'control_testing', 'control_review',
                                    'risk_review', 'risk_reassessment', 'risk_treatment_due',
                                    'audit_start', 'audit_fieldwork', 'audit_report_due', 'audit_followup',
                                    'certification_renewal', 'certification_audit',
                                    'regulatory_filing', 'regulatory_deadline', 'regulatory_effective_date',
                                    'training_due', 'training_renewal',
                                    'vendor_review', 'vendor_contract_renewal', 'vendor_assessment_due',
                                    'incident_followup', 'incident_review',
                                    'board_meeting', 'committee_meeting',
                                    'custom'
                                )),
    category                    VARCHAR(50),
    priority                    VARCHAR(20)
                                CHECK (priority IS NULL OR priority IN ('critical', 'high', 'medium', 'low')),
    source_entity_type          VARCHAR(100) NOT NULL,
    source_entity_id            UUID NOT NULL,
    source_entity_ref           VARCHAR(50),
    start_date                  DATE NOT NULL,
    start_time                  TIME,
    end_date                    DATE,
    end_time                    TIME,
    is_all_day                  BOOLEAN NOT NULL DEFAULT true,
    timezone                    VARCHAR(50) NOT NULL DEFAULT 'Europe/London',
    is_recurring                BOOLEAN NOT NULL DEFAULT false,
    recurrence_rule             VARCHAR(200),
    recurrence_end_date         DATE,
    parent_event_id             UUID REFERENCES calendar_events(id) ON DELETE CASCADE,
    status                      VARCHAR(30) NOT NULL DEFAULT 'upcoming'
                                CHECK (status IN (
                                    'upcoming', 'in_progress', 'completed', 'overdue', 'cancelled', 'snoozed'
                                )),
    completed_at                TIMESTAMPTZ,
    completed_by                UUID REFERENCES users(id) ON DELETE SET NULL,
    completion_notes            TEXT,
    assigned_to                 UUID REFERENCES users(id) ON DELETE SET NULL,
    assigned_role               VARCHAR(100),
    watchers                    UUID[],
    reminder_days_before        INT[] DEFAULT '{7,3,1,0}',
    reminders_sent              JSONB NOT NULL DEFAULT '{}',
    escalation_days_overdue     INT NOT NULL DEFAULT 3,
    escalation_sent             BOOLEAN NOT NULL DEFAULT false,
    escalation_user_ids         UUID[],
    metadata                    JSONB,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_calendar_events_dedup UNIQUE (organization_id, source_entity_type, source_entity_id, event_type)
);

-- Indexes
CREATE INDEX idx_calendar_events_org ON calendar_events(organization_id);
CREATE INDEX idx_calendar_events_org_type ON calendar_events(organization_id, event_type);
CREATE INDEX idx_calendar_events_org_status ON calendar_events(organization_id, status);
CREATE INDEX idx_calendar_events_org_category ON calendar_events(organization_id, category) WHERE category IS NOT NULL;
CREATE INDEX idx_calendar_events_org_priority ON calendar_events(organization_id, priority) WHERE priority IS NOT NULL;
CREATE INDEX idx_calendar_events_start_date ON calendar_events(start_date);
CREATE INDEX idx_calendar_events_org_start ON calendar_events(organization_id, start_date);
CREATE INDEX idx_calendar_events_org_end ON calendar_events(organization_id, end_date) WHERE end_date IS NOT NULL;
CREATE INDEX idx_calendar_events_source ON calendar_events(source_entity_type, source_entity_id);
CREATE INDEX idx_calendar_events_assigned ON calendar_events(assigned_to) WHERE assigned_to IS NOT NULL;
CREATE INDEX idx_calendar_events_completed_by ON calendar_events(completed_by) WHERE completed_by IS NOT NULL;
CREATE INDEX idx_calendar_events_parent ON calendar_events(parent_event_id) WHERE parent_event_id IS NOT NULL;
CREATE INDEX idx_calendar_events_recurring ON calendar_events(organization_id, is_recurring) WHERE is_recurring = true;
CREATE INDEX idx_calendar_events_overdue ON calendar_events(organization_id, start_date, status) WHERE status = 'overdue';
CREATE INDEX idx_calendar_events_watchers ON calendar_events USING GIN (watchers);
CREATE INDEX idx_calendar_events_escalation ON calendar_events USING GIN (escalation_user_ids);
CREATE INDEX idx_calendar_events_metadata ON calendar_events USING GIN (metadata) WHERE metadata IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_calendar_events_updated_at
    BEFORE UPDATE ON calendar_events
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE calendar_events IS 'Central compliance calendar. Every deadline, review date, audit schedule, certification renewal, and regulatory filing is represented as an event linked to its source entity.';
COMMENT ON COLUMN calendar_events.event_ref IS 'Auto-generated reference per org per year: CEV-YYYY-NNNN.';
COMMENT ON COLUMN calendar_events.source_entity_type IS 'Polymorphic source: "policy", "control", "risk", "audit", "vendor", "incident", "training", etc.';
COMMENT ON COLUMN calendar_events.recurrence_rule IS 'RRULE-format recurrence specification, e.g. "FREQ=YEARLY;BYMONTH=3;BYMONTHDAY=31".';
COMMENT ON COLUMN calendar_events.reminder_days_before IS 'Days before start_date to send reminders. Default: {7,3,1,0}.';
COMMENT ON COLUMN calendar_events.reminders_sent IS 'Tracks which reminders have been sent: {"7": "2026-03-21T10:00:00Z", "3": "2026-03-25T10:00:00Z"}.';
COMMENT ON COLUMN calendar_events.escalation_user_ids IS 'Users to escalate to when event is overdue beyond escalation_days_overdue threshold.';

-- ============================================================================
-- TABLE: calendar_subscriptions
-- ============================================================================

CREATE TABLE calendar_subscriptions (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id                     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_types                 TEXT[],
    categories                  TEXT[],
    assigned_to_me_only         BOOLEAN NOT NULL DEFAULT false,
    ical_export_enabled         BOOLEAN NOT NULL DEFAULT false,
    ical_token_hash             VARCHAR(128),
    notification_preferences    JSONB,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_calendar_subscriptions_org ON calendar_subscriptions(organization_id);
CREATE INDEX idx_calendar_subscriptions_user ON calendar_subscriptions(user_id);
CREATE INDEX idx_calendar_subscriptions_org_user ON calendar_subscriptions(organization_id, user_id);
CREATE INDEX idx_calendar_subscriptions_ical ON calendar_subscriptions(ical_token_hash) WHERE ical_token_hash IS NOT NULL;
CREATE INDEX idx_calendar_subscriptions_types ON calendar_subscriptions USING GIN (event_types);
CREATE INDEX idx_calendar_subscriptions_categories ON calendar_subscriptions USING GIN (categories);

-- Trigger
CREATE TRIGGER trg_calendar_subscriptions_updated_at
    BEFORE UPDATE ON calendar_subscriptions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE calendar_subscriptions IS 'User subscriptions to calendar event types/categories with optional iCal feed export.';
COMMENT ON COLUMN calendar_subscriptions.ical_token_hash IS 'SHA-256 hash of the iCal export token. Token itself is never stored.';
COMMENT ON COLUMN calendar_subscriptions.notification_preferences IS 'Per-subscription notification overrides: {"email": true, "push": true, "in_app": true}.';

-- ============================================================================
-- TABLE: calendar_sync_configs
-- ============================================================================

CREATE TABLE calendar_sync_configs (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    sync_source                 VARCHAR(100) NOT NULL,
    event_type                  VARCHAR(50) NOT NULL,
    is_enabled                  BOOLEAN NOT NULL DEFAULT true,
    default_priority            VARCHAR(20)
                                CHECK (default_priority IS NULL OR default_priority IN ('critical', 'high', 'medium', 'low')),
    default_reminder_days       INT[],
    default_escalation_days     INT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_calendar_sync_configs_org ON calendar_sync_configs(organization_id);
CREATE INDEX idx_calendar_sync_configs_org_source ON calendar_sync_configs(organization_id, sync_source);
CREATE INDEX idx_calendar_sync_configs_org_type ON calendar_sync_configs(organization_id, event_type);
CREATE INDEX idx_calendar_sync_configs_enabled ON calendar_sync_configs(organization_id, is_enabled) WHERE is_enabled = true;

-- Trigger
CREATE TRIGGER trg_calendar_sync_configs_updated_at
    BEFORE UPDATE ON calendar_sync_configs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE calendar_sync_configs IS 'Configuration for automatic calendar event generation from source entities. Controls which entity types auto-create events and their default settings.';
COMMENT ON COLUMN calendar_sync_configs.sync_source IS 'Source entity type that triggers event creation: "policy", "control", "risk", "audit", "vendor", etc.';

-- ============================================================================
-- TRIGGER FUNCTIONS
-- ============================================================================

-- Auto-generate calendar event reference: CEV-YYYY-NNNN
CREATE OR REPLACE FUNCTION generate_calendar_event_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.event_ref IS NULL OR NEW.event_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN event_ref ~ ('^CEV-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(event_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM calendar_events
        WHERE organization_id = NEW.organization_id;

        NEW.event_ref := 'CEV-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_calendar_events_generate_ref
    BEFORE INSERT ON calendar_events
    FOR EACH ROW EXECUTE FUNCTION generate_calendar_event_ref();

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- calendar_events
ALTER TABLE calendar_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_events FORCE ROW LEVEL SECURITY;

CREATE POLICY calendar_events_tenant_select ON calendar_events FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY calendar_events_tenant_insert ON calendar_events FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY calendar_events_tenant_update ON calendar_events FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY calendar_events_tenant_delete ON calendar_events FOR DELETE
    USING (organization_id = get_current_tenant());

-- calendar_subscriptions
ALTER TABLE calendar_subscriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_subscriptions FORCE ROW LEVEL SECURITY;

CREATE POLICY calendar_subscriptions_tenant_select ON calendar_subscriptions FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY calendar_subscriptions_tenant_insert ON calendar_subscriptions FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY calendar_subscriptions_tenant_update ON calendar_subscriptions FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY calendar_subscriptions_tenant_delete ON calendar_subscriptions FOR DELETE
    USING (organization_id = get_current_tenant());

-- calendar_sync_configs
ALTER TABLE calendar_sync_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_sync_configs FORCE ROW LEVEL SECURITY;

CREATE POLICY calendar_sync_configs_tenant_select ON calendar_sync_configs FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY calendar_sync_configs_tenant_insert ON calendar_sync_configs FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY calendar_sync_configs_tenant_update ON calendar_sync_configs FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY calendar_sync_configs_tenant_delete ON calendar_sync_configs FOR DELETE
    USING (organization_id = get_current_tenant());
