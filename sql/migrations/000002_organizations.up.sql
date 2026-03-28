-- Migration 002: Organizations & Subscriptions
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - slug provides URL-safe org identifiers (e.g., /org/acme-corp/dashboard)
--   - parent_organization_id supports corporate group structures common in EU enterprises
--   - headquarters_address is JSONB to accommodate international address formats
--   - settings/branding/metadata are JSONB for schema-flexible org configuration
--   - employee_count_range and annual_revenue_range are text ranges rather than exact
--     figures — enterprises rarely share exact numbers, and ranges suffice for
--     compliance scoping (e.g., NIS2 thresholds)
--   - supported_languages is TEXT[] for multi-language EU compliance document generation

-- ============================================================================
-- HELPER: updated_at trigger function (reused across all tables)
-- ============================================================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- TABLE: organizations
-- ============================================================================

CREATE TABLE organizations (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                    VARCHAR(255) NOT NULL,
    slug                    VARCHAR(100) NOT NULL,
    legal_name              VARCHAR(500),
    registration_number     VARCHAR(100),
    tax_id                  VARCHAR(100),
    industry                VARCHAR(100),
    sector                  VARCHAR(100),
    country_code            CHAR(2),               -- ISO 3166-1 alpha-2
    headquarters_address    JSONB DEFAULT '{}',
    status                  org_status NOT NULL DEFAULT 'trial',
    tier                    org_tier NOT NULL DEFAULT 'starter',
    settings                JSONB DEFAULT '{}',
    branding                JSONB DEFAULT '{}',
    timezone                VARCHAR(50) DEFAULT 'Europe/London',
    default_language        VARCHAR(10) DEFAULT 'en',
    supported_languages     TEXT[] DEFAULT '{en}',
    employee_count_range    VARCHAR(20),
    annual_revenue_range    VARCHAR(30),
    parent_organization_id  UUID REFERENCES organizations(id) ON DELETE SET NULL,
    metadata                JSONB DEFAULT '{}',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ,

    -- Slugs must be globally unique among non-deleted orgs.
    CONSTRAINT uq_organizations_slug UNIQUE (slug)
);

-- Indexes
CREATE INDEX idx_organizations_status ON organizations(status) WHERE deleted_at IS NULL;
CREATE INDEX idx_organizations_tier ON organizations(tier) WHERE deleted_at IS NULL;
CREATE INDEX idx_organizations_country ON organizations(country_code) WHERE deleted_at IS NULL;
CREATE INDEX idx_organizations_parent ON organizations(parent_organization_id) WHERE parent_organization_id IS NOT NULL;
CREATE INDEX idx_organizations_industry ON organizations(industry) WHERE deleted_at IS NULL;
-- GIN index on name for fast trigram/fuzzy search
CREATE INDEX idx_organizations_name_trgm ON organizations USING gin (name gin_trgm_ops);
CREATE INDEX idx_organizations_deleted_at ON organizations(deleted_at) WHERE deleted_at IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_organizations_updated_at
    BEFORE UPDATE ON organizations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE organizations IS 'Root tenant entity. Every row in every other table references back to an organization for RLS isolation.';
COMMENT ON COLUMN organizations.slug IS 'URL-safe unique identifier, e.g. acme-corp. Used in routes and API paths.';
COMMENT ON COLUMN organizations.headquarters_address IS 'JSONB: {street, city, state, postal_code, country}. Flexible to support international formats.';
COMMENT ON COLUMN organizations.employee_count_range IS 'Text range (e.g. 251-1000). Used for NIS2/DORA threshold scoping.';

-- ============================================================================
-- TABLE: organization_subscriptions
-- ============================================================================

CREATE TABLE organization_subscriptions (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    plan_name               VARCHAR(100) NOT NULL,
    status                  subscription_status NOT NULL DEFAULT 'trialing',
    max_users               INT NOT NULL DEFAULT 5,
    max_frameworks          INT NOT NULL DEFAULT 2,
    features_enabled        JSONB DEFAULT '{}',
    billing_cycle           VARCHAR(20) NOT NULL DEFAULT 'monthly'
                            CHECK (billing_cycle IN ('monthly', 'annual')),
    current_period_start    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    current_period_end      TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '30 days'),
    canceled_at             TIMESTAMPTZ,
    trial_ends_at           TIMESTAMPTZ,
    external_subscription_id VARCHAR(255),          -- Stripe/payment-provider reference
    metadata                JSONB DEFAULT '{}',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_org_subscriptions_org ON organization_subscriptions(organization_id);
CREATE INDEX idx_org_subscriptions_status ON organization_subscriptions(status);
CREATE INDEX idx_org_subscriptions_period_end ON organization_subscriptions(current_period_end);

-- Trigger
CREATE TRIGGER trg_org_subscriptions_updated_at
    BEFORE UPDATE ON organization_subscriptions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE organization_subscriptions IS 'Tracks billing plan, feature gates, and user/framework limits per org.';
COMMENT ON COLUMN organization_subscriptions.features_enabled IS 'JSONB feature flags: {"ai_scoring": true, "api_access": true, "sso": false, ...}';
