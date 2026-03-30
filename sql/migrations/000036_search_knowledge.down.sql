-- Migration 036 DOWN: Advanced Search & Knowledge Base
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- recent_searches
DROP POLICY IF EXISTS recent_searches_tenant_delete ON recent_searches;
DROP POLICY IF EXISTS recent_searches_tenant_update ON recent_searches;
DROP POLICY IF EXISTS recent_searches_tenant_insert ON recent_searches;
DROP POLICY IF EXISTS recent_searches_tenant_select ON recent_searches;

-- knowledge_bookmarks
DROP POLICY IF EXISTS knowledge_bookmarks_tenant_delete ON knowledge_bookmarks;
DROP POLICY IF EXISTS knowledge_bookmarks_tenant_update ON knowledge_bookmarks;
DROP POLICY IF EXISTS knowledge_bookmarks_tenant_insert ON knowledge_bookmarks;
DROP POLICY IF EXISTS knowledge_bookmarks_tenant_select ON knowledge_bookmarks;

-- knowledge_articles
DROP POLICY IF EXISTS knowledge_articles_tenant_delete ON knowledge_articles;
DROP POLICY IF EXISTS knowledge_articles_tenant_update ON knowledge_articles;
DROP POLICY IF EXISTS knowledge_articles_tenant_insert ON knowledge_articles;
DROP POLICY IF EXISTS knowledge_articles_tenant_select ON knowledge_articles;

-- search_index
DROP POLICY IF EXISTS search_index_tenant_delete ON search_index;
DROP POLICY IF EXISTS search_index_tenant_update ON search_index;
DROP POLICY IF EXISTS search_index_tenant_insert ON search_index;
DROP POLICY IF EXISTS search_index_tenant_select ON search_index;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_knowledge_articles_updated_at ON knowledge_articles;
DROP TRIGGER IF EXISTS trg_knowledge_articles_vector ON knowledge_articles;
DROP TRIGGER IF EXISTS trg_search_index_vector ON search_index;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS recent_searches;
DROP TABLE IF EXISTS knowledge_bookmarks;
DROP TABLE IF EXISTS knowledge_articles;
DROP TABLE IF EXISTS search_index;

-- ============================================================================
-- DROP FUNCTIONS
-- ============================================================================

DROP FUNCTION IF EXISTS knowledge_articles_update_vector();
DROP FUNCTION IF EXISTS search_index_update_vector();
