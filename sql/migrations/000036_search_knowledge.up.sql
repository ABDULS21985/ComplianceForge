-- Migration 036: Advanced Search & Knowledge Base (Prompt 32)
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - search_index is a denormalized full-text search table that aggregates
--     searchable content from all entity types. Updated via triggers/workers.
--     Uses PostgreSQL tsvector for fast full-text search with GIN indexes.
--   - knowledge_articles provide built-in compliance guidance, best practices,
--     and how-to content. System articles (is_system=true, organization_id=NULL)
--     ship with the platform; tenant articles are org-scoped.
--   - knowledge_bookmarks let users save articles for quick access.
--   - recent_searches tracks search history for autocomplete and analytics.
--   - All tenant-scoped tables use RLS on organization_id.

-- ============================================================================
-- TABLE: search_index
-- ============================================================================

CREATE TABLE search_index (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entity_type                 VARCHAR(50) NOT NULL,
    entity_id                   UUID NOT NULL,
    entity_ref                  VARCHAR(50),
    title                       TEXT NOT NULL,
    body                        TEXT,
    tags                        TEXT[],
    framework_codes             TEXT[],
    status                      VARCHAR(50),
    severity                    VARCHAR(20),
    category                    VARCHAR(100),
    owner_name                  VARCHAR(200),
    department                  VARCHAR(200),
    classification              VARCHAR(50),
    created_date                DATE,
    updated_date                DATE,
    search_vector               TSVECTOR,
    metadata                    JSONB,

    CONSTRAINT uq_search_index_entity UNIQUE (organization_id, entity_type, entity_id)
);

-- Indexes
CREATE INDEX idx_search_index_org ON search_index(organization_id);
CREATE INDEX idx_search_index_org_type ON search_index(organization_id, entity_type);
CREATE INDEX idx_search_index_vector ON search_index USING GIN (search_vector);
CREATE INDEX idx_search_index_tags ON search_index USING GIN (tags);
CREATE INDEX idx_search_index_framework_codes ON search_index USING GIN (framework_codes);
CREATE INDEX idx_search_index_org_status ON search_index(organization_id, status) WHERE status IS NOT NULL;
CREATE INDEX idx_search_index_org_severity ON search_index(organization_id, severity) WHERE severity IS NOT NULL;
CREATE INDEX idx_search_index_org_category ON search_index(organization_id, category) WHERE category IS NOT NULL;
CREATE INDEX idx_search_index_org_department ON search_index(organization_id, department) WHERE department IS NOT NULL;
CREATE INDEX idx_search_index_org_classification ON search_index(organization_id, classification) WHERE classification IS NOT NULL;
CREATE INDEX idx_search_index_created ON search_index(created_date DESC) WHERE created_date IS NOT NULL;
CREATE INDEX idx_search_index_updated ON search_index(updated_date DESC) WHERE updated_date IS NOT NULL;
CREATE INDEX idx_search_index_metadata ON search_index USING GIN (metadata) WHERE metadata IS NOT NULL;

-- Auto-update search_vector on insert/update
CREATE OR REPLACE FUNCTION search_index_update_vector()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.title, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(NEW.entity_ref, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(array_to_string(NEW.tags, ' '), '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(array_to_string(NEW.framework_codes, ' '), '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(NEW.body, '')), 'C') ||
        setweight(to_tsvector('english', COALESCE(NEW.owner_name, '')), 'D') ||
        setweight(to_tsvector('english', COALESCE(NEW.department, '')), 'D');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_search_index_vector
    BEFORE INSERT OR UPDATE ON search_index
    FOR EACH ROW EXECUTE FUNCTION search_index_update_vector();

COMMENT ON TABLE search_index IS 'Denormalized full-text search index aggregating searchable content from all GRC entity types. Updated via background workers or triggers.';
COMMENT ON COLUMN search_index.search_vector IS 'PostgreSQL tsvector for full-text search. Weighted: A=title/ref, B=tags/frameworks, C=body, D=owner/department.';
COMMENT ON COLUMN search_index.entity_type IS 'Source entity type: "policy", "control", "risk", "audit", "vendor", "incident", "document", etc.';

-- ============================================================================
-- TABLE: knowledge_articles
-- ============================================================================

CREATE TABLE knowledge_articles (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID REFERENCES organizations(id) ON DELETE CASCADE,
    article_type                VARCHAR(30)
                                CHECK (article_type IS NULL OR article_type IN (
                                    'guide', 'best_practice', 'template', 'faq', 'glossary',
                                    'framework_overview', 'regulatory_summary', 'how_to'
                                )),
    title                       VARCHAR(500) NOT NULL,
    slug                        VARCHAR(200),
    content_markdown            TEXT,
    summary                     TEXT,
    applicable_frameworks       TEXT[],
    applicable_control_codes    TEXT[],
    tags                        TEXT[],
    difficulty                  VARCHAR(20)
                                CHECK (difficulty IS NULL OR difficulty IN ('beginner', 'intermediate', 'advanced')),
    reading_time_minutes        INT,
    author_name                 VARCHAR(200),
    is_system                   BOOLEAN NOT NULL DEFAULT false,
    is_published                BOOLEAN NOT NULL DEFAULT false,
    view_count                  INT NOT NULL DEFAULT 0,
    helpful_count               INT NOT NULL DEFAULT 0,
    not_helpful_count           INT NOT NULL DEFAULT 0,
    search_vector               TSVECTOR,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_knowledge_articles_org ON knowledge_articles(organization_id) WHERE organization_id IS NOT NULL;
CREATE INDEX idx_knowledge_articles_system ON knowledge_articles(is_system) WHERE is_system = true;
CREATE INDEX idx_knowledge_articles_published ON knowledge_articles(is_published) WHERE is_published = true;
CREATE INDEX idx_knowledge_articles_type ON knowledge_articles(article_type) WHERE article_type IS NOT NULL;
CREATE INDEX idx_knowledge_articles_slug ON knowledge_articles(slug) WHERE slug IS NOT NULL;
CREATE INDEX idx_knowledge_articles_difficulty ON knowledge_articles(difficulty) WHERE difficulty IS NOT NULL;
CREATE INDEX idx_knowledge_articles_vector ON knowledge_articles USING GIN (search_vector);
CREATE INDEX idx_knowledge_articles_tags ON knowledge_articles USING GIN (tags);
CREATE INDEX idx_knowledge_articles_frameworks ON knowledge_articles USING GIN (applicable_frameworks);
CREATE INDEX idx_knowledge_articles_controls ON knowledge_articles USING GIN (applicable_control_codes);
CREATE INDEX idx_knowledge_articles_views ON knowledge_articles(view_count DESC);

-- Auto-update search_vector on insert/update
CREATE OR REPLACE FUNCTION knowledge_articles_update_vector()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.title, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(NEW.summary, '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(array_to_string(NEW.tags, ' '), '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(array_to_string(NEW.applicable_frameworks, ' '), '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(NEW.content_markdown, '')), 'C');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_knowledge_articles_vector
    BEFORE INSERT OR UPDATE ON knowledge_articles
    FOR EACH ROW EXECUTE FUNCTION knowledge_articles_update_vector();

-- Trigger
CREATE TRIGGER trg_knowledge_articles_updated_at
    BEFORE UPDATE ON knowledge_articles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE knowledge_articles IS 'Built-in and tenant-specific compliance knowledge base articles. System articles (is_system=true, organization_id=NULL) ship with the platform.';
COMMENT ON COLUMN knowledge_articles.slug IS 'URL-friendly slug for article routing, e.g. "iso-27001-access-control-guide".';
COMMENT ON COLUMN knowledge_articles.applicable_frameworks IS 'Framework codes this article relates to: ["ISO27001", "SOC2", "GDPR"].';
COMMENT ON COLUMN knowledge_articles.applicable_control_codes IS 'Specific control codes: ["A.9.1.1", "CC6.1"].';

-- ============================================================================
-- TABLE: knowledge_bookmarks
-- ============================================================================

CREATE TABLE knowledge_bookmarks (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    article_id                  UUID NOT NULL REFERENCES knowledge_articles(id) ON DELETE CASCADE,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_knowledge_bookmarks_user_article UNIQUE (user_id, article_id)
);

-- Indexes
CREATE INDEX idx_knowledge_bookmarks_org ON knowledge_bookmarks(organization_id);
CREATE INDEX idx_knowledge_bookmarks_user ON knowledge_bookmarks(user_id);
CREATE INDEX idx_knowledge_bookmarks_article ON knowledge_bookmarks(article_id);

COMMENT ON TABLE knowledge_bookmarks IS 'User bookmarks for knowledge base articles.';

-- ============================================================================
-- TABLE: recent_searches
-- ============================================================================

CREATE TABLE recent_searches (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    query                       TEXT NOT NULL,
    result_count                INT,
    clicked_entity_type         VARCHAR(50),
    clicked_entity_id           UUID,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_recent_searches_org ON recent_searches(organization_id);
CREATE INDEX idx_recent_searches_user ON recent_searches(user_id);
CREATE INDEX idx_recent_searches_org_user ON recent_searches(organization_id, user_id);
CREATE INDEX idx_recent_searches_created ON recent_searches(created_at DESC);
CREATE INDEX idx_recent_searches_user_created ON recent_searches(user_id, created_at DESC);

COMMENT ON TABLE recent_searches IS 'User search history for autocomplete suggestions and search analytics.';

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- search_index
ALTER TABLE search_index ENABLE ROW LEVEL SECURITY;
ALTER TABLE search_index FORCE ROW LEVEL SECURITY;

CREATE POLICY search_index_tenant_select ON search_index FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY search_index_tenant_insert ON search_index FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY search_index_tenant_update ON search_index FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY search_index_tenant_delete ON search_index FOR DELETE
    USING (organization_id = get_current_tenant());

-- knowledge_articles (system articles visible to all; tenant articles org-scoped)
ALTER TABLE knowledge_articles ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_articles FORCE ROW LEVEL SECURITY;

CREATE POLICY knowledge_articles_tenant_select ON knowledge_articles FOR SELECT
    USING (organization_id IS NULL OR organization_id = get_current_tenant());
CREATE POLICY knowledge_articles_tenant_insert ON knowledge_articles FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY knowledge_articles_tenant_update ON knowledge_articles FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY knowledge_articles_tenant_delete ON knowledge_articles FOR DELETE
    USING (organization_id = get_current_tenant());

-- knowledge_bookmarks
ALTER TABLE knowledge_bookmarks ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_bookmarks FORCE ROW LEVEL SECURITY;

CREATE POLICY knowledge_bookmarks_tenant_select ON knowledge_bookmarks FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY knowledge_bookmarks_tenant_insert ON knowledge_bookmarks FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY knowledge_bookmarks_tenant_update ON knowledge_bookmarks FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY knowledge_bookmarks_tenant_delete ON knowledge_bookmarks FOR DELETE
    USING (organization_id = get_current_tenant());

-- recent_searches
ALTER TABLE recent_searches ENABLE ROW LEVEL SECURITY;
ALTER TABLE recent_searches FORCE ROW LEVEL SECURITY;

CREATE POLICY recent_searches_tenant_select ON recent_searches FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY recent_searches_tenant_insert ON recent_searches FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY recent_searches_tenant_update ON recent_searches FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY recent_searches_tenant_delete ON recent_searches FOR DELETE
    USING (organization_id = get_current_tenant());
