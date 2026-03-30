-- Migration 037: Collaboration & Comments (Prompt 33)
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - comments supports threaded discussions on any entity via polymorphic
--     entity_type + entity_id. Threads via parent_comment_id self-reference.
--     Supports @mentions, attachments, reactions, pinning, and soft-delete.
--   - activity_feed is an append-only audit trail of all user and system
--     actions across the platform for timeline views and notification feeds.
--   - user_follows tracks which entities a user is watching/participating in
--     to control notification delivery.
--   - user_read_markers tracks per-entity read state for unread indicators.
--   - All tables are tenant-isolated via RLS on organization_id.

-- ============================================================================
-- TABLE: comments
-- ============================================================================

CREATE TABLE comments (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entity_type                 VARCHAR(50) NOT NULL,
    entity_id                   UUID NOT NULL,
    parent_comment_id           UUID REFERENCES comments(id) ON DELETE CASCADE,
    author_user_id              UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content                     TEXT NOT NULL,
    content_html                TEXT,
    is_internal                 BOOLEAN NOT NULL DEFAULT true,
    is_resolution_note          BOOLEAN NOT NULL DEFAULT false,
    is_pinned                   BOOLEAN NOT NULL DEFAULT false,
    mentioned_user_ids          UUID[],
    mentioned_role_slugs        TEXT[],
    attachment_paths            TEXT[],
    attachment_names            TEXT[],
    attachment_sizes            BIGINT[],
    reactions                   JSONB NOT NULL DEFAULT '{}',
    is_edited                   BOOLEAN NOT NULL DEFAULT false,
    edited_at                   TIMESTAMPTZ,
    is_deleted                  BOOLEAN NOT NULL DEFAULT false,
    deleted_at                  TIMESTAMPTZ,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_comments_org ON comments(organization_id);
CREATE INDEX idx_comments_entity ON comments(entity_type, entity_id);
CREATE INDEX idx_comments_org_entity ON comments(organization_id, entity_type, entity_id);
CREATE INDEX idx_comments_parent ON comments(parent_comment_id) WHERE parent_comment_id IS NOT NULL;
CREATE INDEX idx_comments_author ON comments(author_user_id);
CREATE INDEX idx_comments_org_created ON comments(organization_id, created_at DESC);
CREATE INDEX idx_comments_entity_created ON comments(entity_type, entity_id, created_at DESC);
CREATE INDEX idx_comments_pinned ON comments(organization_id, entity_type, entity_id, is_pinned) WHERE is_pinned = true;
CREATE INDEX idx_comments_resolution ON comments(organization_id, entity_type, entity_id, is_resolution_note) WHERE is_resolution_note = true;
CREATE INDEX idx_comments_not_deleted ON comments(organization_id, is_deleted) WHERE is_deleted = false;
CREATE INDEX idx_comments_mentioned_users ON comments USING GIN (mentioned_user_ids);
CREATE INDEX idx_comments_mentioned_roles ON comments USING GIN (mentioned_role_slugs);
CREATE INDEX idx_comments_reactions ON comments USING GIN (reactions) WHERE reactions != '{}';

-- Trigger
CREATE TRIGGER trg_comments_updated_at
    BEFORE UPDATE ON comments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE comments IS 'Threaded comments on any GRC entity. Supports @mentions, attachments, emoji reactions, pinning, resolution notes, and soft-delete.';
COMMENT ON COLUMN comments.entity_type IS 'Polymorphic target: "policy", "control", "risk", "audit", "incident", "vendor", "evidence", etc.';
COMMENT ON COLUMN comments.is_internal IS 'Internal comments visible only to platform users. External (false) may be shared with vendors/auditors.';
COMMENT ON COLUMN comments.reactions IS 'Emoji reaction counts: {"thumbs_up": ["user-uuid-1", "user-uuid-2"], "check": ["user-uuid-3"]}.';
COMMENT ON COLUMN comments.mentioned_role_slugs IS 'Role slugs mentioned in the comment for role-based notifications: ["compliance_officer", "risk_manager"].';

-- ============================================================================
-- TABLE: activity_feed
-- ============================================================================

CREATE TABLE activity_feed (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    actor_user_id               UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action                      VARCHAR(100) NOT NULL,
    entity_type                 VARCHAR(50),
    entity_id                   UUID,
    entity_ref                  VARCHAR(50),
    entity_title                VARCHAR(500),
    description                 TEXT NOT NULL,
    changes                     JSONB,
    is_system                   BOOLEAN NOT NULL DEFAULT false,
    visibility                  VARCHAR(20) NOT NULL DEFAULT 'all'
                                CHECK (visibility IN ('all', 'internal', 'admins_only')),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_activity_feed_org ON activity_feed(organization_id);
CREATE INDEX idx_activity_feed_org_created ON activity_feed(organization_id, created_at DESC);
CREATE INDEX idx_activity_feed_actor ON activity_feed(actor_user_id);
CREATE INDEX idx_activity_feed_entity ON activity_feed(entity_type, entity_id);
CREATE INDEX idx_activity_feed_org_entity ON activity_feed(organization_id, entity_type, entity_id, created_at DESC);
CREATE INDEX idx_activity_feed_org_action ON activity_feed(organization_id, action);
CREATE INDEX idx_activity_feed_org_visibility ON activity_feed(organization_id, visibility);
CREATE INDEX idx_activity_feed_system ON activity_feed(organization_id, is_system) WHERE is_system = true;
CREATE INDEX idx_activity_feed_changes ON activity_feed USING GIN (changes) WHERE changes IS NOT NULL;

COMMENT ON TABLE activity_feed IS 'Append-only audit trail of all user and system actions. Powers timeline views, notification feeds, and activity dashboards.';
COMMENT ON COLUMN activity_feed.action IS 'Action performed: "created", "updated", "deleted", "approved", "rejected", "commented", "assigned", "status_changed", etc.';
COMMENT ON COLUMN activity_feed.changes IS 'JSONB diff of changed fields: {"status": {"old": "draft", "new": "published"}, "assigned_to": {"old": null, "new": "uuid"}}.';
COMMENT ON COLUMN activity_feed.visibility IS 'Who can see this activity: all (everyone), internal (platform users only), admins_only.';

-- ============================================================================
-- TABLE: user_follows
-- ============================================================================

CREATE TABLE user_follows (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entity_type                 VARCHAR(50) NOT NULL,
    entity_id                   UUID NOT NULL,
    follow_type                 VARCHAR(20) NOT NULL
                                CHECK (follow_type IN ('watching', 'participating', 'mentioned')),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_user_follows_entity UNIQUE (user_id, entity_type, entity_id)
);

-- Indexes
CREATE INDEX idx_user_follows_org ON user_follows(organization_id);
CREATE INDEX idx_user_follows_user ON user_follows(user_id);
CREATE INDEX idx_user_follows_entity ON user_follows(entity_type, entity_id);
CREATE INDEX idx_user_follows_org_entity ON user_follows(organization_id, entity_type, entity_id);
CREATE INDEX idx_user_follows_user_type ON user_follows(user_id, follow_type);

COMMENT ON TABLE user_follows IS 'Tracks which entities a user is watching, participating in, or was mentioned on. Controls notification delivery scope.';
COMMENT ON COLUMN user_follows.follow_type IS 'Follow level: watching (all updates), participating (only when involved), mentioned (one-time from @mention).';

-- ============================================================================
-- TABLE: user_read_markers
-- ============================================================================

CREATE TABLE user_read_markers (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entity_type                 VARCHAR(50) NOT NULL,
    entity_id                   UUID NOT NULL,
    last_read_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    unread_count                INT NOT NULL DEFAULT 0,

    CONSTRAINT uq_user_read_markers_entity UNIQUE (user_id, entity_type, entity_id)
);

-- Indexes
CREATE INDEX idx_user_read_markers_org ON user_read_markers(organization_id);
CREATE INDEX idx_user_read_markers_user ON user_read_markers(user_id);
CREATE INDEX idx_user_read_markers_entity ON user_read_markers(entity_type, entity_id);
CREATE INDEX idx_user_read_markers_user_unread ON user_read_markers(user_id, unread_count) WHERE unread_count > 0;

COMMENT ON TABLE user_read_markers IS 'Per-entity read state tracking for unread indicators in the UI.';

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- comments
ALTER TABLE comments ENABLE ROW LEVEL SECURITY;
ALTER TABLE comments FORCE ROW LEVEL SECURITY;

CREATE POLICY comments_tenant_select ON comments FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY comments_tenant_insert ON comments FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY comments_tenant_update ON comments FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY comments_tenant_delete ON comments FOR DELETE
    USING (organization_id = get_current_tenant());

-- activity_feed
ALTER TABLE activity_feed ENABLE ROW LEVEL SECURITY;
ALTER TABLE activity_feed FORCE ROW LEVEL SECURITY;

CREATE POLICY activity_feed_tenant_select ON activity_feed FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY activity_feed_tenant_insert ON activity_feed FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY activity_feed_tenant_update ON activity_feed FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY activity_feed_tenant_delete ON activity_feed FOR DELETE
    USING (organization_id = get_current_tenant());

-- user_follows
ALTER TABLE user_follows ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_follows FORCE ROW LEVEL SECURITY;

CREATE POLICY user_follows_tenant_select ON user_follows FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY user_follows_tenant_insert ON user_follows FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY user_follows_tenant_update ON user_follows FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY user_follows_tenant_delete ON user_follows FOR DELETE
    USING (organization_id = get_current_tenant());

-- user_read_markers
ALTER TABLE user_read_markers ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_read_markers FORCE ROW LEVEL SECURITY;

CREATE POLICY user_read_markers_tenant_select ON user_read_markers FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY user_read_markers_tenant_insert ON user_read_markers FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY user_read_markers_tenant_update ON user_read_markers FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY user_read_markers_tenant_delete ON user_read_markers FOR DELETE
    USING (organization_id = get_current_tenant());
