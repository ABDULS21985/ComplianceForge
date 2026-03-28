-- Migration 013: Policy Management Core — Categories, Policies & Versions
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - policy_categories mirrors the risk_categories pattern: organization_id NULL
--     for system defaults, non-NULL for custom org categories
--   - policies tracks the current state of each policy with ownership, review cycle,
--     applicability scoping, and regulatory linkage
--   - policy_versions stores full version history with HTML and plain text content,
--     enabling rich editing and full-text search simultaneously
--   - policy_translations supports EU multi-language requirements — GDPR Art. 12
--     requires policies to be provided in a language data subjects understand
--   - search_vector on policies is trigger-maintained (references join data);
--     search_vector on policy_versions is GENERATED ALWAYS (self-contained)
--   - Hierarchical policies via parent_policy_id supports group-to-subsidiary
--     policy inheritance common in European corporate structures
--   - supersedes_policy_id enables clean policy succession tracking

-- ============================================================================
-- TABLE: policy_categories
-- ============================================================================

CREATE TABLE policy_categories (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID REFERENCES organizations(id) ON DELETE CASCADE,
    name                VARCHAR(255) NOT NULL,
    code                VARCHAR(50) NOT NULL,
    description         TEXT,
    parent_category_id  UUID REFERENCES policy_categories(id) ON DELETE SET NULL,
    sort_order          INT NOT NULL DEFAULT 0,
    is_system_default   BOOLEAN NOT NULL DEFAULT false,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_policy_cat_org_code UNIQUE NULLS NOT DISTINCT (organization_id, code)
);

CREATE INDEX idx_policy_cat_org ON policy_categories(organization_id);
CREATE INDEX idx_policy_cat_parent ON policy_categories(parent_category_id) WHERE parent_category_id IS NOT NULL;
CREATE INDEX idx_policy_cat_system ON policy_categories(is_system_default) WHERE is_system_default = true;

CREATE TRIGGER trg_policy_categories_updated_at
    BEFORE UPDATE ON policy_categories
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS: system defaults visible to all, custom scoped to org
ALTER TABLE policy_categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_categories FORCE ROW LEVEL SECURITY;

CREATE POLICY policy_cat_tenant_select ON policy_categories FOR SELECT
    USING (organization_id IS NULL OR organization_id = get_current_tenant());
CREATE POLICY policy_cat_tenant_insert ON policy_categories FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY policy_cat_tenant_update ON policy_categories FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY policy_cat_tenant_delete ON policy_categories FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE policy_categories IS 'Hierarchical policy taxonomy. System defaults (org_id NULL) cover common compliance domains; orgs add custom categories.';

-- ============================================================================
-- TABLE: policy_versions (created before policies so policies can FK to it)
-- ============================================================================

CREATE TABLE policy_versions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_id           UUID NOT NULL,  -- FK added after policies table creation
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    version_number      INT NOT NULL,
    version_label       VARCHAR(20) NOT NULL,
    title               VARCHAR(500) NOT NULL,
    content_html        TEXT,
    content_text        TEXT,
    summary             TEXT,
    change_description  TEXT,
    change_type         VARCHAR(20),
    language            VARCHAR(10) NOT NULL DEFAULT 'en',
    word_count          INT,
    status              VARCHAR(20) NOT NULL DEFAULT 'draft',
    created_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    published_at        TIMESTAMPTZ,
    published_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    file_path           TEXT,
    file_hash           VARCHAR(128),
    metadata            JSONB DEFAULT '{}',
    search_vector       tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(summary, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(content_text, '')), 'C')
    ) STORED,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_pv_change_type CHECK (
        change_type IS NULL OR change_type IN ('major', 'minor', 'editorial')
    ),
    CONSTRAINT chk_pv_status CHECK (
        status IN ('draft', 'under_review', 'approved', 'published', 'archived')
    ),
    CONSTRAINT chk_pv_version_number CHECK (version_number > 0)
);

CREATE TRIGGER trg_policy_versions_updated_at
    BEFORE UPDATE ON policy_versions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- TABLE: policies
-- ============================================================================

CREATE TABLE policies (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id                 UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    policy_ref                      VARCHAR(30) NOT NULL,
    title                           VARCHAR(500) NOT NULL,
    category_id                     UUID REFERENCES policy_categories(id) ON DELETE SET NULL,
    status                          VARCHAR(30) NOT NULL DEFAULT 'draft',
    classification                  VARCHAR(20) NOT NULL DEFAULT 'internal',

    -- Ownership
    owner_user_id                   UUID REFERENCES users(id) ON DELETE SET NULL,
    author_user_id                  UUID REFERENCES users(id) ON DELETE SET NULL,
    approver_user_id                UUID REFERENCES users(id) ON DELETE SET NULL,
    department_id                   UUID,

    -- Versioning
    current_version                 INT NOT NULL DEFAULT 1,
    current_version_id              UUID REFERENCES policy_versions(id) ON DELETE SET NULL,

    -- Review cycle
    review_frequency_months         INT NOT NULL DEFAULT 12,
    last_review_date                DATE,
    next_review_date                DATE,
    review_status                   VARCHAR(20) NOT NULL DEFAULT 'current',

    -- Applicability
    applies_to_all                  BOOLEAN NOT NULL DEFAULT true,
    applicable_departments          UUID[] DEFAULT '{}',
    applicable_roles                TEXT[] DEFAULT '{}',
    applicable_locations            TEXT[] DEFAULT '{}',

    -- Regulatory linkage
    linked_framework_ids            UUID[] DEFAULT '{}',
    linked_control_ids              UUID[] DEFAULT '{}',
    linked_risk_ids                 UUID[] DEFAULT '{}',

    -- Hierarchy
    parent_policy_id                UUID REFERENCES policies(id) ON DELETE SET NULL,
    supersedes_policy_id            UUID REFERENCES policies(id) ON DELETE SET NULL,

    -- Dates and metadata
    effective_date                  DATE,
    expiry_date                     DATE,
    tags                            TEXT[] DEFAULT '{}',
    priority                        VARCHAR(10),
    is_mandatory                    BOOLEAN NOT NULL DEFAULT true,
    requires_attestation            BOOLEAN NOT NULL DEFAULT true,
    attestation_frequency_months    INT NOT NULL DEFAULT 12,
    metadata                        JSONB DEFAULT '{}',
    search_vector                   tsvector,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at                      TIMESTAMPTZ,

    CONSTRAINT uq_policy_org_ref UNIQUE (organization_id, policy_ref),
    CONSTRAINT chk_policy_status CHECK (
        status IN ('draft', 'under_review', 'pending_approval', 'approved', 'published', 'archived', 'retired', 'superseded')
    ),
    CONSTRAINT chk_policy_classification CHECK (
        classification IN ('public', 'internal', 'confidential', 'restricted')
    ),
    CONSTRAINT chk_policy_review_status CHECK (
        review_status IN ('current', 'review_due', 'overdue', 'not_applicable')
    ),
    CONSTRAINT chk_policy_priority CHECK (
        priority IS NULL OR priority IN ('critical', 'high', 'medium', 'low')
    ),
    CONSTRAINT chk_policy_review_freq CHECK (review_frequency_months > 0 AND review_frequency_months <= 60),
    CONSTRAINT chk_policy_attest_freq CHECK (attestation_frequency_months > 0 AND attestation_frequency_months <= 60)
);

-- Now add the FK from policy_versions to policies
ALTER TABLE policy_versions
    ADD CONSTRAINT fk_pv_policy FOREIGN KEY (policy_id) REFERENCES policies(id) ON DELETE CASCADE;

-- Indexes: policies
CREATE INDEX idx_policies_org ON policies(organization_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_policies_status ON policies(organization_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_policies_category ON policies(category_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_policies_owner ON policies(owner_user_id) WHERE owner_user_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_policies_approver ON policies(approver_user_id) WHERE approver_user_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_policies_review_date ON policies(next_review_date) WHERE next_review_date IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_policies_review_status ON policies(organization_id, review_status) WHERE deleted_at IS NULL;
CREATE INDEX idx_policies_parent ON policies(parent_policy_id) WHERE parent_policy_id IS NOT NULL;
CREATE INDEX idx_policies_supersedes ON policies(supersedes_policy_id) WHERE supersedes_policy_id IS NOT NULL;
CREATE INDEX idx_policies_effective ON policies(effective_date) WHERE effective_date IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_policies_tags ON policies USING gin (tags) WHERE deleted_at IS NULL;
CREATE INDEX idx_policies_fw_ids ON policies USING gin (linked_framework_ids) WHERE deleted_at IS NULL;
CREATE INDEX idx_policies_ctrl_ids ON policies USING gin (linked_control_ids) WHERE deleted_at IS NULL;
CREATE INDEX idx_policies_risk_ids ON policies USING gin (linked_risk_ids) WHERE deleted_at IS NULL;
CREATE INDEX idx_policies_search ON policies USING gin (search_vector);
CREATE INDEX idx_policies_deleted ON policies(deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX idx_policies_classification ON policies(organization_id, classification) WHERE deleted_at IS NULL;

-- Indexes: policy_versions
CREATE INDEX idx_pv_policy ON policy_versions(policy_id);
CREATE INDEX idx_pv_org ON policy_versions(organization_id);
CREATE INDEX idx_pv_status ON policy_versions(policy_id, status);
CREATE INDEX idx_pv_version ON policy_versions(policy_id, version_number DESC);
CREATE INDEX idx_pv_search ON policy_versions USING gin (search_vector);
CREATE INDEX idx_pv_language ON policy_versions(language);
CREATE INDEX idx_pv_hash ON policy_versions(file_hash) WHERE file_hash IS NOT NULL;

CREATE TRIGGER trg_policies_updated_at
    BEFORE UPDATE ON policies
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- TABLE: policy_translations
-- ============================================================================

CREATE TABLE policy_translations (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_version_id       UUID NOT NULL REFERENCES policy_versions(id) ON DELETE CASCADE,
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    language                VARCHAR(10) NOT NULL,
    title                   VARCHAR(500) NOT NULL,
    content_html            TEXT,
    content_text            TEXT,
    summary                 TEXT,
    translated_by           VARCHAR(50) NOT NULL DEFAULT 'human',
    translator_user_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    approved                BOOLEAN NOT NULL DEFAULT false,
    approved_by             UUID REFERENCES users(id) ON DELETE SET NULL,
    approved_at             TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_policy_translation UNIQUE (policy_version_id, language),
    CONSTRAINT chk_pt_translated_by CHECK (translated_by IN ('human', 'machine', 'reviewed')),
    CONSTRAINT chk_pt_language CHECK (language ~ '^[a-z]{2}(-[A-Z]{2})?$')
);

CREATE INDEX idx_pt_version ON policy_translations(policy_version_id);
CREATE INDEX idx_pt_org ON policy_translations(organization_id);
CREATE INDEX idx_pt_language ON policy_translations(language);

CREATE TRIGGER trg_policy_translations_updated_at
    BEFORE UPDATE ON policy_translations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE policy_translations ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_translations FORCE ROW LEVEL SECURITY;

CREATE POLICY pt_tenant_select ON policy_translations FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY pt_tenant_insert ON policy_translations FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY pt_tenant_update ON policy_translations FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY pt_tenant_delete ON policy_translations FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE policy_translations IS 'Multi-language policy content. Supports GDPR Art. 12 requirement to provide policies in data subject languages.';

-- ============================================================================
-- FUNCTIONS & TRIGGERS
-- ============================================================================

-- Auto-generate policy_ref: POL-XXXX
CREATE OR REPLACE FUNCTION generate_policy_ref()
RETURNS TRIGGER AS $$
DECLARE
    next_num INT;
BEGIN
    IF NEW.policy_ref IS NULL OR NEW.policy_ref = '' THEN
        SELECT COALESCE(MAX(
            CASE WHEN policy_ref ~ '^POL-[0-9]+$'
                 THEN CAST(SUBSTRING(policy_ref FROM 5) AS INT)
                 ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM policies
        WHERE organization_id = NEW.organization_id;

        NEW.policy_ref := 'POL-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_policies_generate_ref
    BEFORE INSERT ON policies
    FOR EACH ROW EXECUTE FUNCTION generate_policy_ref();

-- Auto-update search_vector on policies
CREATE OR REPLACE FUNCTION policies_search_vector_update()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', coalesce(NEW.policy_ref, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(NEW.title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(NEW.metadata->>'description', '')), 'B');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_policies_search_vector
    BEFORE INSERT OR UPDATE OF title, policy_ref, metadata ON policies
    FOR EACH ROW EXECUTE FUNCTION policies_search_vector_update();

-- On policy status → 'published': auto-set effective_date, schedule next review
CREATE OR REPLACE FUNCTION policy_on_publish()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status = 'published' AND (OLD.status IS NULL OR OLD.status != 'published') THEN
        -- Set effective date if not already set
        IF NEW.effective_date IS NULL THEN
            NEW.effective_date := CURRENT_DATE;
        END IF;
        -- Schedule next review
        NEW.last_review_date := CURRENT_DATE;
        NEW.next_review_date := CURRENT_DATE + (NEW.review_frequency_months || ' months')::INTERVAL;
        NEW.review_status := 'current';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_policies_on_publish
    BEFORE UPDATE OF status ON policies
    FOR EACH ROW EXECUTE FUNCTION policy_on_publish();

-- On policy_versions insert: update policies.current_version and current_version_id
CREATE OR REPLACE FUNCTION policy_version_on_insert()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE policies
    SET current_version = NEW.version_number,
        current_version_id = NEW.id,
        updated_at = NOW()
    WHERE id = NEW.policy_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_policy_version_on_insert
    AFTER INSERT ON policy_versions
    FOR EACH ROW EXECUTE FUNCTION policy_version_on_insert();

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

ALTER TABLE policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE policies FORCE ROW LEVEL SECURITY;

CREATE POLICY policies_tenant_select ON policies FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY policies_tenant_insert ON policies FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY policies_tenant_update ON policies FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY policies_tenant_delete ON policies FOR DELETE
    USING (organization_id = get_current_tenant());

ALTER TABLE policy_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_versions FORCE ROW LEVEL SECURITY;

CREATE POLICY pv_tenant_select ON policy_versions FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY pv_tenant_insert ON policy_versions FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY pv_tenant_update ON policy_versions FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY pv_tenant_delete ON policy_versions FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE policies IS 'Master policy register with ownership, review cycle, applicability scoping, and regulatory linkage.';
COMMENT ON TABLE policy_versions IS 'Complete version history with rich text content and full-text search. Each version is immutable once published.';
COMMENT ON COLUMN policies.policy_ref IS 'Auto-generated sequential reference per org: POL-0001, POL-0002, etc.';
COMMENT ON COLUMN policies.review_status IS 'Auto-updated: current (within cycle), review_due (within 30 days), overdue (past next_review_date).';
COMMENT ON COLUMN policies.linked_framework_ids IS 'UUIDs of compliance frameworks this policy supports. Used for gap analysis.';
COMMENT ON COLUMN policies.parent_policy_id IS 'For group-to-subsidiary policy inheritance. Child policies inherit from group-level parent.';
