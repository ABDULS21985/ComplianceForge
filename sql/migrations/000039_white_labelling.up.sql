-- Migration 039: White-Labelling & Branding (Prompt 35)
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - tenant_branding stores per-organization visual customization: logos, colors,
--     typography, layout preferences, custom domain, and feature visibility toggles.
--     One branding record per org (UNIQUE on organization_id).
--   - white_label_partners supports reseller/MSP model where partners manage
--     multiple tenant organizations under their own brand with revenue sharing.
--   - partner_tenant_mappings links partners to their managed organizations.
--   - Custom domain fields support vanity URLs with SSL verification status.
--   - All tenant-scoped tables use RLS on organization_id.

-- ============================================================================
-- TABLE: tenant_branding
-- ============================================================================

CREATE TABLE tenant_branding (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    -- Product identity
    product_name                VARCHAR(200) NOT NULL DEFAULT 'ComplianceForge',
    tagline                     VARCHAR(500),
    company_name                VARCHAR(300),
    support_email               VARCHAR(300),
    support_url                 TEXT,
    privacy_policy_url          TEXT,
    terms_url                   TEXT,
    -- Logos
    logo_url                    TEXT,
    logo_icon_url               TEXT,
    logo_dark_url               TEXT,
    login_background_url        TEXT,
    email_header_logo_url       TEXT,
    report_logo_url             TEXT,
    -- Colors
    primary_color               VARCHAR(7) NOT NULL DEFAULT '#4F46E5',
    primary_hover_color         VARCHAR(7),
    primary_light_color         VARCHAR(7),
    secondary_color             VARCHAR(7),
    accent_color                VARCHAR(7),
    success_color               VARCHAR(7),
    warning_color               VARCHAR(7),
    danger_color                VARCHAR(7),
    info_color                  VARCHAR(7),
    sidebar_bg_color            VARCHAR(7),
    sidebar_text_color          VARCHAR(7),
    sidebar_active_bg_color     VARCHAR(7),
    sidebar_active_text_color   VARCHAR(7),
    topbar_bg_color             VARCHAR(7),
    topbar_text_color           VARCHAR(7),
    login_bg_color              VARCHAR(7),
    -- Typography
    font_family                 VARCHAR(200) NOT NULL DEFAULT 'Inter',
    font_url                    TEXT,
    heading_font_family         VARCHAR(200),
    -- Layout
    sidebar_style               VARCHAR(20) NOT NULL DEFAULT 'light'
                                CHECK (sidebar_style IN ('light', 'dark', 'branded')),
    corner_radius               VARCHAR(20) NOT NULL DEFAULT 'medium'
                                CHECK (corner_radius IN ('none', 'small', 'medium', 'large', 'full')),
    density                     VARCHAR(20) NOT NULL DEFAULT 'default'
                                CHECK (density IN ('compact', 'default', 'comfortable')),
    -- Custom domain
    custom_domain               VARCHAR(300),
    custom_domain_verified      BOOLEAN NOT NULL DEFAULT false,
    custom_domain_ssl_status    VARCHAR(20)
                                CHECK (custom_domain_ssl_status IS NULL OR custom_domain_ssl_status IN (
                                    'pending', 'active', 'expired', 'failed'
                                )),
    -- Custom CSS
    custom_css                  TEXT,
    -- Feature visibility
    show_powered_by             BOOLEAN NOT NULL DEFAULT true,
    show_help_widget            BOOLEAN NOT NULL DEFAULT true,
    show_marketplace            BOOLEAN NOT NULL DEFAULT true,
    show_knowledge_base         BOOLEAN NOT NULL DEFAULT true,
    -- Timestamps
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_tenant_branding_org UNIQUE (organization_id)
);

-- Indexes
CREATE INDEX idx_tenant_branding_org ON tenant_branding(organization_id);
CREATE INDEX idx_tenant_branding_domain ON tenant_branding(custom_domain) WHERE custom_domain IS NOT NULL;
CREATE INDEX idx_tenant_branding_domain_verified ON tenant_branding(custom_domain, custom_domain_verified) WHERE custom_domain IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_tenant_branding_updated_at
    BEFORE UPDATE ON tenant_branding
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE tenant_branding IS 'Per-organization visual customization: logos, colors, typography, layout, custom domain, and feature visibility. One record per org.';
COMMENT ON COLUMN tenant_branding.primary_color IS 'Hex color code for the primary brand color, e.g. "#4F46E5".';
COMMENT ON COLUMN tenant_branding.custom_domain IS 'Vanity domain for white-label access, e.g. "grc.acmecorp.com".';
COMMENT ON COLUMN tenant_branding.custom_domain_ssl_status IS 'SSL certificate provisioning status for custom domain: pending, active, expired, failed.';
COMMENT ON COLUMN tenant_branding.custom_css IS 'Optional CSS overrides injected into the application. Sanitized server-side before rendering.';
COMMENT ON COLUMN tenant_branding.sidebar_style IS 'Sidebar theme: light (white bg), dark (dark bg), branded (uses sidebar_bg_color).';
COMMENT ON COLUMN tenant_branding.corner_radius IS 'UI corner radius preset: none (0px), small (2px), medium (6px), large (12px), full (9999px).';

-- ============================================================================
-- TABLE: white_label_partners
-- ============================================================================

CREATE TABLE white_label_partners (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_name                VARCHAR(300) NOT NULL,
    partner_slug                VARCHAR(100) NOT NULL,
    contact_email               VARCHAR(300),
    default_branding_id         UUID REFERENCES tenant_branding(id) ON DELETE SET NULL,
    revenue_share_percent       DECIMAL(5,2),
    max_tenants                 INT,
    is_active                   BOOLEAN NOT NULL DEFAULT true,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_white_label_partners_slug UNIQUE (partner_slug)
);

-- Indexes
CREATE INDEX idx_wl_partners_slug ON white_label_partners(partner_slug);
CREATE INDEX idx_wl_partners_active ON white_label_partners(is_active) WHERE is_active = true;
CREATE INDEX idx_wl_partners_branding ON white_label_partners(default_branding_id) WHERE default_branding_id IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_white_label_partners_updated_at
    BEFORE UPDATE ON white_label_partners
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE white_label_partners IS 'Reseller/MSP partners who manage multiple tenant organizations under their own brand. Supports revenue sharing and tenant limits.';
COMMENT ON COLUMN white_label_partners.partner_slug IS 'URL-safe unique identifier for the partner, e.g. "acme-consulting".';
COMMENT ON COLUMN white_label_partners.revenue_share_percent IS 'Partner revenue share percentage, e.g. 20.00 for 20%.';
COMMENT ON COLUMN white_label_partners.default_branding_id IS 'Default branding template applied to new tenants onboarded by this partner.';

-- ============================================================================
-- TABLE: partner_tenant_mappings
-- ============================================================================

CREATE TABLE partner_tenant_mappings (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id                  UUID NOT NULL REFERENCES white_label_partners(id) ON DELETE CASCADE,
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    onboarded_at                TIMESTAMPTZ,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_partner_tenant_partner ON partner_tenant_mappings(partner_id);
CREATE INDEX idx_partner_tenant_org ON partner_tenant_mappings(organization_id);
CREATE INDEX idx_partner_tenant_pair ON partner_tenant_mappings(partner_id, organization_id);
CREATE INDEX idx_partner_tenant_onboarded ON partner_tenant_mappings(onboarded_at DESC) WHERE onboarded_at IS NOT NULL;

COMMENT ON TABLE partner_tenant_mappings IS 'Links white-label partners to the tenant organizations they manage.';

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- tenant_branding
ALTER TABLE tenant_branding ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_branding FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_branding_tenant_select ON tenant_branding FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY tenant_branding_tenant_insert ON tenant_branding FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY tenant_branding_tenant_update ON tenant_branding FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY tenant_branding_tenant_delete ON tenant_branding FOR DELETE
    USING (organization_id = get_current_tenant());

-- partner_tenant_mappings (RLS on organization_id)
ALTER TABLE partner_tenant_mappings ENABLE ROW LEVEL SECURITY;
ALTER TABLE partner_tenant_mappings FORCE ROW LEVEL SECURITY;

CREATE POLICY partner_tenant_mappings_tenant_select ON partner_tenant_mappings FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY partner_tenant_mappings_tenant_insert ON partner_tenant_mappings FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY partner_tenant_mappings_tenant_update ON partner_tenant_mappings FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY partner_tenant_mappings_tenant_delete ON partner_tenant_mappings FOR DELETE
    USING (organization_id = get_current_tenant());
