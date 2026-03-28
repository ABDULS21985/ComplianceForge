-- Migration 026: Marketplace
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - marketplace_publishers represent organizations that publish compliance
--     content packages (policy templates, control mappings, risk libraries, etc.).
--     Publishers can be verified/official for trust signals.
--   - marketplace_packages are versioned content bundles with rich metadata:
--     applicable frameworks, regions, industries, pricing model, and a JSONB
--     contents_summary describing what the package contains.
--   - marketplace_package_versions enable safe upgrades with breaking-change
--     flags and migration notes. package_data holds the actual importable content.
--   - marketplace_installations track what each org has installed (one install
--     per package per org). Configuration and import summary are stored for
--     auditing what was imported and how it was customized.
--   - marketplace_reviews provide community feedback with verified-install badge.
--   - RLS applies to installations and reviews (tenant data). Publishers and
--     packages are public catalog data (no RLS).

-- ============================================================================
-- TABLE: marketplace_publishers (public catalog — no RLS)
-- ============================================================================

CREATE TABLE marketplace_publishers (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID REFERENCES organizations(id) ON DELETE SET NULL,
    publisher_name      VARCHAR(200) NOT NULL,
    publisher_slug      VARCHAR(100) NOT NULL,
    description         TEXT,
    website             VARCHAR(500),
    logo_url            VARCHAR(500),
    is_verified         BOOLEAN NOT NULL DEFAULT false,
    is_official         BOOLEAN NOT NULL DEFAULT false,
    total_packages      INT NOT NULL DEFAULT 0,
    total_downloads     INT NOT NULL DEFAULT 0,
    rating_avg          DECIMAL(3,2) NOT NULL DEFAULT 0,
    rating_count        INT NOT NULL DEFAULT 0,
    contact_email       VARCHAR(300),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_marketplace_publishers_slug UNIQUE (publisher_slug)
);

-- Indexes
CREATE INDEX idx_mp_publishers_org ON marketplace_publishers(organization_id) WHERE organization_id IS NOT NULL;
CREATE INDEX idx_mp_publishers_verified ON marketplace_publishers(is_verified) WHERE is_verified = true;
CREATE INDEX idx_mp_publishers_official ON marketplace_publishers(is_official) WHERE is_official = true;

-- Trigger
CREATE TRIGGER trg_mp_publishers_updated_at
    BEFORE UPDATE ON marketplace_publishers
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE marketplace_publishers IS 'Public catalog of marketplace content publishers. Publishers may be verified (identity confirmed) or official (platform-provided). Not tenant-scoped — visible to all organizations.';
COMMENT ON COLUMN marketplace_publishers.publisher_slug IS 'URL-safe unique identifier for the publisher (e.g., "complianceforge-official").';

-- ============================================================================
-- TABLE: marketplace_packages (public catalog — no RLS)
-- ============================================================================

CREATE TABLE marketplace_packages (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    publisher_id            UUID NOT NULL REFERENCES marketplace_publishers(id) ON DELETE CASCADE,
    package_slug            VARCHAR(100) NOT NULL,
    name                    VARCHAR(300) NOT NULL,
    description             TEXT,
    long_description        TEXT,
    package_type            VARCHAR(30) NOT NULL
                            CHECK (package_type IN ('policy_templates', 'control_mappings', 'risk_library', 'assessment_templates', 'report_templates', 'framework_pack', 'integration', 'workflow', 'bundle')),
    category                VARCHAR(100),
    applicable_frameworks   TEXT[],
    applicable_regions      TEXT[],
    applicable_industries   TEXT[],
    tags                    TEXT[],
    current_version         VARCHAR(50),
    pricing_model           VARCHAR(20) NOT NULL DEFAULT 'free'
                            CHECK (pricing_model IN ('free', 'one_time', 'subscription', 'freemium')),
    price_eur               DECIMAL(10,2),
    download_count          INT NOT NULL DEFAULT 0,
    install_count           INT NOT NULL DEFAULT 0,
    rating_avg              DECIMAL(3,2) NOT NULL DEFAULT 0,
    rating_count            INT NOT NULL DEFAULT 0,
    featured                BOOLEAN NOT NULL DEFAULT false,
    contents_summary        JSONB,
    status                  VARCHAR(20) NOT NULL DEFAULT 'draft'
                            CHECK (status IN ('draft', 'in_review', 'published', 'deprecated', 'removed')),
    published_at            TIMESTAMPTZ,
    license                 VARCHAR(100),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_marketplace_packages_pub_slug UNIQUE (publisher_id, package_slug)
);

-- Indexes
CREATE INDEX idx_mp_packages_publisher ON marketplace_packages(publisher_id);
CREATE INDEX idx_mp_packages_type ON marketplace_packages(package_type);
CREATE INDEX idx_mp_packages_status ON marketplace_packages(status);
CREATE INDEX idx_mp_packages_category ON marketplace_packages(category) WHERE category IS NOT NULL;
CREATE INDEX idx_mp_packages_featured ON marketplace_packages(featured) WHERE featured = true;
CREATE INDEX idx_mp_packages_pricing ON marketplace_packages(pricing_model);
CREATE INDEX idx_mp_packages_rating ON marketplace_packages(rating_avg DESC);
CREATE INDEX idx_mp_packages_downloads ON marketplace_packages(download_count DESC);
CREATE INDEX idx_mp_packages_published ON marketplace_packages(published_at DESC) WHERE published_at IS NOT NULL;
CREATE INDEX idx_mp_packages_frameworks ON marketplace_packages USING GIN (applicable_frameworks);
CREATE INDEX idx_mp_packages_regions ON marketplace_packages USING GIN (applicable_regions);
CREATE INDEX idx_mp_packages_industries ON marketplace_packages USING GIN (applicable_industries);
CREATE INDEX idx_mp_packages_tags ON marketplace_packages USING GIN (tags);

-- Trigger
CREATE TRIGGER trg_mp_packages_updated_at
    BEFORE UPDATE ON marketplace_packages
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE marketplace_packages IS 'Public catalog of marketplace content packages. Each package is a versioned bundle of compliance content (policies, controls, risk libraries, etc.) published by a verified publisher.';
COMMENT ON COLUMN marketplace_packages.contents_summary IS 'JSONB summary of package contents: {"policies": 12, "controls": 45, "risk_scenarios": 8, "report_templates": 3}';
COMMENT ON COLUMN marketplace_packages.applicable_frameworks IS 'Array of framework codes this package applies to: ["ISO27001", "SOC2", "GDPR"].';

-- ============================================================================
-- TABLE: marketplace_package_versions
-- ============================================================================

CREATE TABLE marketplace_package_versions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id          UUID NOT NULL REFERENCES marketplace_packages(id) ON DELETE CASCADE,
    version             VARCHAR(50) NOT NULL,
    release_notes       TEXT,
    package_data        JSONB,
    package_hash        VARCHAR(128),
    file_size_bytes     BIGINT,
    is_breaking_change  BOOLEAN NOT NULL DEFAULT false,
    migration_notes     TEXT,
    published_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_mp_versions_package_version UNIQUE (package_id, version)
);

-- Indexes
CREATE INDEX idx_mp_versions_package ON marketplace_package_versions(package_id);
CREATE INDEX idx_mp_versions_published ON marketplace_package_versions(published_at DESC) WHERE published_at IS NOT NULL;

COMMENT ON TABLE marketplace_package_versions IS 'Version history for marketplace packages. Each version contains the full importable content (package_data), integrity hash, and upgrade metadata (breaking changes, migration notes).';
COMMENT ON COLUMN marketplace_package_versions.package_data IS 'JSONB payload containing the full importable content for this version.';
COMMENT ON COLUMN marketplace_package_versions.package_hash IS 'SHA-512 hash of package_data for integrity verification.';

-- ============================================================================
-- TABLE: marketplace_installations (tenant-scoped)
-- ============================================================================

CREATE TABLE marketplace_installations (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    package_id          UUID NOT NULL REFERENCES marketplace_packages(id) ON DELETE CASCADE,
    version_id          UUID REFERENCES marketplace_package_versions(id) ON DELETE SET NULL,
    installed_version   VARCHAR(50),
    status              VARCHAR(20) NOT NULL DEFAULT 'installed'
                        CHECK (status IN ('installing', 'installed', 'update_available', 'updating', 'failed', 'uninstalled')),
    installed_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    installed_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    configuration       JSONB,
    import_summary      JSONB,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_mp_installations_org_package UNIQUE (organization_id, package_id)
);

-- Indexes
CREATE INDEX idx_mp_installations_org ON marketplace_installations(organization_id);
CREATE INDEX idx_mp_installations_package ON marketplace_installations(package_id);
CREATE INDEX idx_mp_installations_status ON marketplace_installations(organization_id, status);
CREATE INDEX idx_mp_installations_installed_by ON marketplace_installations(installed_by) WHERE installed_by IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_mp_installations_updated_at
    BEFORE UPDATE ON marketplace_installations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE marketplace_installations IS 'Tracks which packages each organization has installed, at which version, with configuration and import summary for audit purposes.';
COMMENT ON COLUMN marketplace_installations.configuration IS 'Org-specific configuration applied during installation: {"import_policies": true, "prefix": "ISO-", "overwrite_existing": false}';
COMMENT ON COLUMN marketplace_installations.import_summary IS 'Summary of what was imported: {"policies_created": 12, "controls_mapped": 45, "skipped": 2, "errors": []}';

-- ============================================================================
-- TABLE: marketplace_reviews (tenant-scoped)
-- ============================================================================

CREATE TABLE marketplace_reviews (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id          UUID NOT NULL REFERENCES marketplace_packages(id) ON DELETE CASCADE,
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating              INT NOT NULL CHECK (rating >= 1 AND rating <= 5),
    title               VARCHAR(200),
    review_text         TEXT,
    helpful_count       INT NOT NULL DEFAULT 0,
    is_verified_install BOOLEAN NOT NULL DEFAULT false,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_mp_reviews_package_org UNIQUE (package_id, organization_id)
);

-- Indexes
CREATE INDEX idx_mp_reviews_package ON marketplace_reviews(package_id);
CREATE INDEX idx_mp_reviews_org ON marketplace_reviews(organization_id);
CREATE INDEX idx_mp_reviews_user ON marketplace_reviews(user_id);
CREATE INDEX idx_mp_reviews_rating ON marketplace_reviews(package_id, rating);

-- Trigger
CREATE TRIGGER trg_mp_reviews_updated_at
    BEFORE UPDATE ON marketplace_reviews
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE marketplace_reviews IS 'Community reviews for marketplace packages. One review per organization per package, with verified-install badge and helpful vote count.';
COMMENT ON COLUMN marketplace_reviews.is_verified_install IS 'True if the reviewing organization has an active installation of the package.';

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- marketplace_publishers: NO RLS (public catalog)
-- marketplace_packages: NO RLS (public catalog)
-- marketplace_package_versions: NO RLS (public catalog)

-- marketplace_installations
ALTER TABLE marketplace_installations ENABLE ROW LEVEL SECURITY;
ALTER TABLE marketplace_installations FORCE ROW LEVEL SECURITY;

CREATE POLICY mp_installations_tenant_select ON marketplace_installations FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY mp_installations_tenant_insert ON marketplace_installations FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY mp_installations_tenant_update ON marketplace_installations FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY mp_installations_tenant_delete ON marketplace_installations FOR DELETE
    USING (organization_id = get_current_tenant());

-- marketplace_reviews
ALTER TABLE marketplace_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE marketplace_reviews FORCE ROW LEVEL SECURITY;

CREATE POLICY mp_reviews_tenant_select ON marketplace_reviews FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY mp_reviews_tenant_insert ON marketplace_reviews FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY mp_reviews_tenant_update ON marketplace_reviews FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY mp_reviews_tenant_delete ON marketplace_reviews FOR DELETE
    USING (organization_id = get_current_tenant());
