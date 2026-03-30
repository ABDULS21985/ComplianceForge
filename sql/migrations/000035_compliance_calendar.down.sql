-- Migration 035 DOWN: Compliance Calendar
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- calendar_sync_configs
DROP POLICY IF EXISTS calendar_sync_configs_tenant_delete ON calendar_sync_configs;
DROP POLICY IF EXISTS calendar_sync_configs_tenant_update ON calendar_sync_configs;
DROP POLICY IF EXISTS calendar_sync_configs_tenant_insert ON calendar_sync_configs;
DROP POLICY IF EXISTS calendar_sync_configs_tenant_select ON calendar_sync_configs;

-- calendar_subscriptions
DROP POLICY IF EXISTS calendar_subscriptions_tenant_delete ON calendar_subscriptions;
DROP POLICY IF EXISTS calendar_subscriptions_tenant_update ON calendar_subscriptions;
DROP POLICY IF EXISTS calendar_subscriptions_tenant_insert ON calendar_subscriptions;
DROP POLICY IF EXISTS calendar_subscriptions_tenant_select ON calendar_subscriptions;

-- calendar_events
DROP POLICY IF EXISTS calendar_events_tenant_delete ON calendar_events;
DROP POLICY IF EXISTS calendar_events_tenant_update ON calendar_events;
DROP POLICY IF EXISTS calendar_events_tenant_insert ON calendar_events;
DROP POLICY IF EXISTS calendar_events_tenant_select ON calendar_events;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_calendar_events_generate_ref ON calendar_events;
DROP TRIGGER IF EXISTS trg_calendar_sync_configs_updated_at ON calendar_sync_configs;
DROP TRIGGER IF EXISTS trg_calendar_subscriptions_updated_at ON calendar_subscriptions;
DROP TRIGGER IF EXISTS trg_calendar_events_updated_at ON calendar_events;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS calendar_sync_configs;
DROP TABLE IF EXISTS calendar_subscriptions;
DROP TABLE IF EXISTS calendar_events;

-- ============================================================================
-- DROP FUNCTIONS
-- ============================================================================

DROP FUNCTION IF EXISTS generate_calendar_event_ref();
