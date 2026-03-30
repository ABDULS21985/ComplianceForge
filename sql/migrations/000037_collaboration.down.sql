-- Migration 037 DOWN: Collaboration & Comments
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- user_read_markers
DROP POLICY IF EXISTS user_read_markers_tenant_delete ON user_read_markers;
DROP POLICY IF EXISTS user_read_markers_tenant_update ON user_read_markers;
DROP POLICY IF EXISTS user_read_markers_tenant_insert ON user_read_markers;
DROP POLICY IF EXISTS user_read_markers_tenant_select ON user_read_markers;

-- user_follows
DROP POLICY IF EXISTS user_follows_tenant_delete ON user_follows;
DROP POLICY IF EXISTS user_follows_tenant_update ON user_follows;
DROP POLICY IF EXISTS user_follows_tenant_insert ON user_follows;
DROP POLICY IF EXISTS user_follows_tenant_select ON user_follows;

-- activity_feed
DROP POLICY IF EXISTS activity_feed_tenant_delete ON activity_feed;
DROP POLICY IF EXISTS activity_feed_tenant_update ON activity_feed;
DROP POLICY IF EXISTS activity_feed_tenant_insert ON activity_feed;
DROP POLICY IF EXISTS activity_feed_tenant_select ON activity_feed;

-- comments
DROP POLICY IF EXISTS comments_tenant_delete ON comments;
DROP POLICY IF EXISTS comments_tenant_update ON comments;
DROP POLICY IF EXISTS comments_tenant_insert ON comments;
DROP POLICY IF EXISTS comments_tenant_select ON comments;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_comments_updated_at ON comments;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS user_read_markers;
DROP TABLE IF EXISTS user_follows;
DROP TABLE IF EXISTS activity_feed;
DROP TABLE IF EXISTS comments;
