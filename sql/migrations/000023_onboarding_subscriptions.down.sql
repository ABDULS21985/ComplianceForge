-- Migration 023 DOWN: Onboarding Progress & Subscription Plans (v2)
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- usage_events
DROP POLICY IF EXISTS usage_events_tenant_delete ON usage_events;
DROP POLICY IF EXISTS usage_events_tenant_update ON usage_events;
DROP POLICY IF EXISTS usage_events_tenant_insert ON usage_events;
DROP POLICY IF EXISTS usage_events_tenant_select ON usage_events;

-- onboarding_progress
DROP POLICY IF EXISTS onboarding_tenant_delete ON onboarding_progress;
DROP POLICY IF EXISTS onboarding_tenant_update ON onboarding_progress;
DROP POLICY IF EXISTS onboarding_tenant_insert ON onboarding_progress;
DROP POLICY IF EXISTS onboarding_tenant_select ON onboarding_progress;

-- organization_subscriptions_v2
DROP POLICY IF EXISTS org_subs_v2_tenant_delete ON organization_subscriptions_v2;
DROP POLICY IF EXISTS org_subs_v2_tenant_update ON organization_subscriptions_v2;
DROP POLICY IF EXISTS org_subs_v2_tenant_insert ON organization_subscriptions_v2;
DROP POLICY IF EXISTS org_subs_v2_tenant_select ON organization_subscriptions_v2;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_onboarding_progress_updated_at ON onboarding_progress;
DROP TRIGGER IF EXISTS trg_org_subs_v2_updated_at ON organization_subscriptions_v2;
DROP TRIGGER IF EXISTS trg_subscription_plans_updated_at ON subscription_plans;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS usage_events;
DROP TABLE IF EXISTS onboarding_progress;
DROP TABLE IF EXISTS organization_subscriptions_v2;
DROP TABLE IF EXISTS subscription_plans;
