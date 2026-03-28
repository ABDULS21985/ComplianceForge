-- Migration 023: Onboarding Progress & Subscription Plans (v2)
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - subscription_plans is a global catalog (no RLS) — all orgs can see available
--     plans. This replaces the loose plan_name string on the v1 subscriptions table
--     with a proper normalized plan entity including pricing, limits, and features.
--   - organization_subscriptions_v2 is the new subscription table with explicit
--     Stripe integration fields, usage snapshots, and richer status lifecycle
--     (trialing, active, past_due, cancelled, paused). One subscription per org
--     enforced via UNIQUE on organization_id.
--   - onboarding_progress tracks a 7-step wizard with per-step completion data:
--     org profile, industry assessment, framework selection, team invitations,
--     risk appetite configuration, quick compliance assessment, and review/launch.
--     Each step's data is stored in a dedicated JSONB column for type safety.
--   - usage_events captures metered usage for billing (user logins, API calls,
--     storage consumption, etc.) with a composite index for fast aggregation.
--   - 0 in max_risks / max_vendors means unlimited (application interprets this).

-- ============================================================================
-- TABLE: subscription_plans (global catalog — no RLS)
-- ============================================================================

CREATE TABLE subscription_plans (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                    VARCHAR(100) NOT NULL,
    slug                    VARCHAR(50) NOT NULL,
    description             TEXT,
    tier                    org_tier NOT NULL,
    pricing_eur_monthly     DECIMAL(10,2),
    pricing_eur_annual      DECIMAL(10,2),
    max_users               INT NOT NULL DEFAULT 5,
    max_frameworks          INT NOT NULL DEFAULT 2,
    max_risks               INT NOT NULL DEFAULT 0,       -- 0 = unlimited
    max_vendors             INT NOT NULL DEFAULT 0,       -- 0 = unlimited
    max_storage_gb          INT NOT NULL DEFAULT 5,
    features                JSONB NOT NULL DEFAULT '{}',
    is_active               BOOLEAN NOT NULL DEFAULT true,
    sort_order              INT NOT NULL DEFAULT 0,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_subscription_plans_slug UNIQUE (slug)
);

-- Indexes
CREATE INDEX idx_subscription_plans_active ON subscription_plans(is_active, sort_order)
    WHERE is_active = true;
CREATE INDEX idx_subscription_plans_tier ON subscription_plans(tier);

-- Trigger
CREATE TRIGGER trg_subscription_plans_updated_at
    BEFORE UPDATE ON subscription_plans
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE subscription_plans IS 'Global catalog of available subscription plans with pricing, user/feature limits, and feature flags. Not tenant-scoped — all organizations can view available plans.';
COMMENT ON COLUMN subscription_plans.max_risks IS '0 means unlimited. Application layer interprets this convention.';
COMMENT ON COLUMN subscription_plans.max_vendors IS '0 means unlimited. Application layer interprets this convention.';
COMMENT ON COLUMN subscription_plans.features IS 'JSONB feature flags: {"sso": true, "api_access": true, "custom_branding": false, "advanced_reporting": true, ...}';

-- ============================================================================
-- TABLE: organization_subscriptions_v2
-- ============================================================================

CREATE TABLE organization_subscriptions_v2 (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    plan_id                     UUID NOT NULL REFERENCES subscription_plans(id) ON DELETE RESTRICT,
    status                      VARCHAR(20) NOT NULL DEFAULT 'trialing'
                                CHECK (status IN ('trialing', 'active', 'past_due', 'cancelled', 'paused')),
    billing_cycle               VARCHAR(10) NOT NULL DEFAULT 'monthly'
                                CHECK (billing_cycle IN ('monthly', 'annual')),
    current_period_start        TIMESTAMPTZ,
    current_period_end          TIMESTAMPTZ,
    trial_ends_at               TIMESTAMPTZ,
    cancelled_at                TIMESTAMPTZ,
    cancel_reason               TEXT,
    stripe_customer_id          VARCHAR(200),
    stripe_subscription_id      VARCHAR(200),
    usage_snapshot              JSONB NOT NULL DEFAULT '{}',
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One subscription per organization.
    CONSTRAINT uq_org_subscriptions_v2_org UNIQUE (organization_id)
);

-- Indexes
CREATE INDEX idx_org_subs_v2_plan ON organization_subscriptions_v2(plan_id);
CREATE INDEX idx_org_subs_v2_status ON organization_subscriptions_v2(status);
CREATE INDEX idx_org_subs_v2_period_end ON organization_subscriptions_v2(current_period_end);
CREATE INDEX idx_org_subs_v2_stripe_customer ON organization_subscriptions_v2(stripe_customer_id)
    WHERE stripe_customer_id IS NOT NULL;
CREATE INDEX idx_org_subs_v2_stripe_sub ON organization_subscriptions_v2(stripe_subscription_id)
    WHERE stripe_subscription_id IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_org_subs_v2_updated_at
    BEFORE UPDATE ON organization_subscriptions_v2
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE organization_subscriptions_v2 IS 'Active subscription binding between an organization and a subscription plan. Supports Stripe integration, billing cycles, trial tracking, and usage snapshots. Replaces the v1 organization_subscriptions table with richer lifecycle management.';
COMMENT ON COLUMN organization_subscriptions_v2.usage_snapshot IS 'Periodic snapshot of current usage: {"users": 12, "frameworks": 3, "risks": 47, "vendors": 8, "storage_gb": 2.3}';
COMMENT ON COLUMN organization_subscriptions_v2.cancel_reason IS 'Free-text reason captured when the organization cancels their subscription.';

-- ============================================================================
-- TABLE: onboarding_progress
-- ============================================================================

CREATE TABLE onboarding_progress (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    current_step                INT NOT NULL DEFAULT 1,
    total_steps                 INT NOT NULL DEFAULT 7,
    completed_steps             JSONB NOT NULL DEFAULT '[]',
    is_completed                BOOLEAN NOT NULL DEFAULT false,
    completed_at                TIMESTAMPTZ,
    skipped_steps               INT[] NOT NULL DEFAULT '{}',
    org_profile_data            JSONB,
    industry_assessment_data    JSONB,
    selected_framework_ids      UUID[],
    team_invitations            JSONB,
    risk_appetite_data          JSONB,
    quick_assessment_data       JSONB,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One onboarding record per organization.
    CONSTRAINT uq_onboarding_progress_org UNIQUE (organization_id)
);

-- Indexes
CREATE INDEX idx_onboarding_progress_completed ON onboarding_progress(is_completed)
    WHERE is_completed = false;

-- Trigger
CREATE TRIGGER trg_onboarding_progress_updated_at
    BEFORE UPDATE ON onboarding_progress
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE onboarding_progress IS 'Tracks the multi-step onboarding wizard for each organization. Stores per-step data (org profile, industry assessment, framework selection, team invitations, risk appetite, quick assessment) to allow resumption and review.';
COMMENT ON COLUMN onboarding_progress.completed_steps IS 'JSONB array of completed step objects: [{"step": 1, "completed_at": "...", "duration_seconds": 45}, ...]';
COMMENT ON COLUMN onboarding_progress.skipped_steps IS 'Array of step numbers the user chose to skip during onboarding.';
COMMENT ON COLUMN onboarding_progress.team_invitations IS 'JSONB array of invited team members: [{"email": "...", "role": "...", "sent_at": "...", "accepted": false}, ...]';

-- ============================================================================
-- TABLE: usage_events
-- ============================================================================

CREATE TABLE usage_events (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    event_type          VARCHAR(100) NOT NULL,
    quantity            DECIMAL(10,2) NOT NULL DEFAULT 1,
    metadata            JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_usage_events_org_type_time ON usage_events(organization_id, event_type, created_at DESC);
CREATE INDEX idx_usage_events_type ON usage_events(event_type);
CREATE INDEX idx_usage_events_created ON usage_events(created_at DESC);

COMMENT ON TABLE usage_events IS 'Append-only log of metered usage events for billing aggregation. Event types include user_login, api_call, storage_upload, report_generated, etc.';
COMMENT ON COLUMN usage_events.quantity IS 'Quantity for the event (e.g., 1 for a login, 2.5 for MB uploaded). Defaults to 1.';
COMMENT ON COLUMN usage_events.metadata IS 'Additional context: {"user_id": "...", "endpoint": "/api/v1/risks", "response_time_ms": 234}';

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- subscription_plans: NO RLS (global catalog, all users can read)

-- organization_subscriptions_v2
ALTER TABLE organization_subscriptions_v2 ENABLE ROW LEVEL SECURITY;
ALTER TABLE organization_subscriptions_v2 FORCE ROW LEVEL SECURITY;

CREATE POLICY org_subs_v2_tenant_select ON organization_subscriptions_v2 FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY org_subs_v2_tenant_insert ON organization_subscriptions_v2 FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY org_subs_v2_tenant_update ON organization_subscriptions_v2 FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY org_subs_v2_tenant_delete ON organization_subscriptions_v2 FOR DELETE
    USING (organization_id = get_current_tenant());

-- onboarding_progress
ALTER TABLE onboarding_progress ENABLE ROW LEVEL SECURITY;
ALTER TABLE onboarding_progress FORCE ROW LEVEL SECURITY;

CREATE POLICY onboarding_tenant_select ON onboarding_progress FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY onboarding_tenant_insert ON onboarding_progress FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY onboarding_tenant_update ON onboarding_progress FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY onboarding_tenant_delete ON onboarding_progress FOR DELETE
    USING (organization_id = get_current_tenant());

-- usage_events
ALTER TABLE usage_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE usage_events FORCE ROW LEVEL SECURITY;

CREATE POLICY usage_events_tenant_select ON usage_events FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY usage_events_tenant_insert ON usage_events FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY usage_events_tenant_update ON usage_events FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY usage_events_tenant_delete ON usage_events FOR DELETE
    USING (organization_id = get_current_tenant());
