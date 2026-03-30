package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	ErrCommentNotFound   = fmt.Errorf("comment not found")
	ErrCommentEditDenied = fmt.Errorf("only the author may edit within 24 hours")
	ErrCommentPinDenied  = fmt.Errorf("only admin or entity owner may pin comments")
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// Comment represents a discussion comment on any GRC entity.
type Comment struct {
	ID           string      `json:"id"`
	OrgID        string      `json:"organization_id"`
	EntityType   string      `json:"entity_type"`
	EntityID     string      `json:"entity_id"`
	ParentID     *string     `json:"parent_id"`
	AuthorID     string      `json:"author_id"`
	AuthorName   string      `json:"author_name"`
	AuthorAvatar *string     `json:"author_avatar"`
	ContentRaw   string      `json:"content_raw"`
	ContentHTML  string      `json:"content_html"`
	Attachments  []string    `json:"attachments"`
	IsPinned     bool        `json:"is_pinned"`
	IsDeleted    bool        `json:"is_deleted"`
	Reactions    map[string]int `json:"reactions"`
	Children     []Comment   `json:"children,omitempty"`
	CreatedAt    string      `json:"created_at"`
	UpdatedAt    string      `json:"updated_at"`
}

// ActivityEntry represents a single item in the activity feed.
type ActivityEntry struct {
	ID          string                 `json:"id"`
	OrgID       string                 `json:"organization_id"`
	UserID      string                 `json:"user_id"`
	UserName    string                 `json:"user_name"`
	Action      string                 `json:"action"`
	EntityType  string                 `json:"entity_type"`
	EntityID    string                 `json:"entity_id"`
	EntityRef   string                 `json:"entity_ref"`
	EntityTitle string                 `json:"entity_title"`
	Description string                 `json:"description"`
	Changes     map[string]interface{} `json:"changes,omitempty"`
	CreatedAt   string                 `json:"created_at"`
}

// ActivityFilter controls which activity entries to return.
type ActivityFilter struct {
	EntityType *string `json:"entity_type"`
	EntityID   *string `json:"entity_id"`
	UserID     *string `json:"user_id"`
	Action     *string `json:"action"`
}

// UnreadCount tracks how many unread activities exist for an entity.
type UnreadCount struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Count      int    `json:"count"`
}

// FollowedEntity represents an entity a user follows.
type FollowedEntity struct {
	EntityType  string `json:"entity_type"`
	EntityID    string `json:"entity_id"`
	EntityRef   string `json:"entity_ref"`
	EntityTitle string `json:"entity_title"`
	FollowedAt  string `json:"followed_at"`
}

// ---------------------------------------------------------------------------
// Regex for @mention parsing
// ---------------------------------------------------------------------------

var mentionUserRe = regexp.MustCompile(`@user\[([a-zA-Z0-9\-]+)\]`)
var mentionRoleRe = regexp.MustCompile(`@role\[([a-zA-Z0-9_\-]+)\]`)

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// CollaborationService manages comments, activity feeds, and entity following.
type CollaborationService struct {
	pool *pgxpool.Pool
	bus  *EventBus
}

// NewCollaborationService creates a CollaborationService.
func NewCollaborationService(pool *pgxpool.Pool, bus *EventBus) *CollaborationService {
	return &CollaborationService{pool: pool, bus: bus}
}

// ---------------------------------------------------------------------------
// Comments
// ---------------------------------------------------------------------------

// CreateComment adds a comment to an entity, parses mentions, notifies, and auto-follows.
func (s *CollaborationService) CreateComment(ctx context.Context, orgID, userID, entityType, entityID, content string, parentID *string, attachments []string) (*Comment, error) {
	mentionedUsers, mentionedRoles := parseMentions(content)
	html := renderMarkdownToHTML(content)

	attachJSON, _ := json.Marshal(attachments)
	var c Comment
	err := s.pool.QueryRow(ctx, `
		INSERT INTO comments
			(id, organization_id, entity_type, entity_id, parent_id, author_id,
			 content_raw, content_html, attachments, is_pinned, is_deleted, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, false, false, NOW(), NOW())
		RETURNING id, organization_id, entity_type, entity_id, parent_id, author_id,
				  content_raw, content_html, is_pinned, is_deleted, created_at, updated_at`,
		orgID, entityType, entityID, parentID, userID, content, html, string(attachJSON)).Scan(
		&c.ID, &c.OrgID, &c.EntityType, &c.EntityID, &c.ParentID, &c.AuthorID,
		&c.ContentRaw, &c.ContentHTML, &c.IsPinned, &c.IsDeleted, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("collaboration: create comment: %w", err)
	}
	c.Attachments = attachments
	c.Reactions = map[string]int{}

	// Auto-follow author
	_ = s.followEntity(ctx, orgID, userID, entityType, entityID)

	// Notify mentioned users
	for _, uid := range mentionedUsers {
		s.bus.Publish(Event{
			Type: "comment.mention", Severity: "medium", OrgID: orgID,
			EntityType: entityType, EntityID: entityID,
			Data: map[string]interface{}{
				"comment_id": c.ID, "mentioned_user": uid, "author": userID,
			},
			Timestamp: time.Now(),
		})
	}

	// Resolve role mentions to users and notify
	for _, roleSlug := range mentionedRoles {
		roleUserIDs, _ := s.resolveRoleUsers(ctx, orgID, roleSlug)
		for _, uid := range roleUserIDs {
			s.bus.Publish(Event{
				Type: "comment.mention", Severity: "medium", OrgID: orgID,
				EntityType: entityType, EntityID: entityID,
				Data: map[string]interface{}{
					"comment_id": c.ID, "mentioned_user": uid, "author": userID, "via_role": roleSlug,
				},
				Timestamp: time.Now(),
			})
		}
	}

	// Notify followers
	s.notifyFollowers(ctx, orgID, entityType, entityID, userID, "comment.created", map[string]interface{}{
		"comment_id": c.ID, "author": userID,
	})

	// Record activity
	_ = s.RecordActivity(ctx, orgID, userID, "comment_added", entityType, entityID, "", "",
		fmt.Sprintf("Added a comment on %s", entityType), nil)

	log.Info().Str("comment_id", c.ID).Str("entity", entityType+"/"+entityID).Msg("collaboration: comment created")
	return &c, nil
}

// EditComment allows the author to edit within 24 hours.
func (s *CollaborationService) EditComment(ctx context.Context, orgID, userID, commentID, newContent string) error {
	var authorID, createdAt string
	err := s.pool.QueryRow(ctx, `
		SELECT author_id, created_at FROM comments
		WHERE id = $1 AND organization_id = $2 AND is_deleted = false`, commentID, orgID).Scan(&authorID, &createdAt)
	if err == pgx.ErrNoRows {
		return ErrCommentNotFound
	}
	if err != nil {
		return fmt.Errorf("collaboration: fetch comment for edit: %w", err)
	}
	if authorID != userID {
		return ErrCommentEditDenied
	}

	created, _ := time.Parse(time.RFC3339, createdAt)
	if time.Since(created) > 24*time.Hour {
		return ErrCommentEditDenied
	}

	html := renderMarkdownToHTML(newContent)
	_, err = s.pool.Exec(ctx, `
		UPDATE comments SET content_raw = $1, content_html = $2, updated_at = NOW()
		WHERE id = $3 AND organization_id = $4`, newContent, html, commentID, orgID)
	if err != nil {
		return fmt.Errorf("collaboration: edit comment: %w", err)
	}
	log.Info().Str("comment_id", commentID).Msg("collaboration: comment edited")
	return nil
}

// DeleteComment performs a soft delete on a comment.
func (s *CollaborationService) DeleteComment(ctx context.Context, orgID, userID, commentID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE comments SET is_deleted = true, updated_at = NOW()
		WHERE id = $1 AND organization_id = $2 AND (author_id = $3 OR EXISTS (
			SELECT 1 FROM user_roles WHERE user_id = $3 AND role = 'admin' AND organization_id = $2
		))`, commentID, orgID, userID)
	if err != nil {
		return fmt.Errorf("collaboration: delete comment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCommentNotFound
	}
	log.Info().Str("comment_id", commentID).Msg("collaboration: comment soft-deleted")
	return nil
}

// PinComment pins or unpins a comment (admin/owner only).
func (s *CollaborationService) PinComment(ctx context.Context, orgID, commentID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE comments SET is_pinned = NOT is_pinned, updated_at = NOW()
		WHERE id = $1 AND organization_id = $2`, commentID, orgID)
	if err != nil {
		return fmt.Errorf("collaboration: pin comment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCommentNotFound
	}
	return nil
}

// ReactToComment toggles a reaction on a comment.
func (s *CollaborationService) ReactToComment(ctx context.Context, orgID, userID, commentID, reaction string) error {
	// Try insert; if conflict, delete (toggle behavior)
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO comment_reactions (id, comment_id, user_id, reaction, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, NOW())
		ON CONFLICT (comment_id, user_id, reaction) DO NOTHING`, commentID, userID, reaction)
	if err != nil {
		return fmt.Errorf("collaboration: react: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Already existed — remove it (toggle off)
		_, err = s.pool.Exec(ctx, `
			DELETE FROM comment_reactions WHERE comment_id = $1 AND user_id = $2 AND reaction = $3`,
			commentID, userID, reaction)
		if err != nil {
			return fmt.Errorf("collaboration: unreact: %w", err)
		}
	}
	return nil
}

// GetComments retrieves threaded comments for an entity.
func (s *CollaborationService) GetComments(ctx context.Context, orgID, entityType, entityID, sortBy string) ([]Comment, error) {
	order := "c.created_at ASC"
	if sortBy == "newest" {
		order = "c.created_at DESC"
	}

	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT c.id, c.organization_id, c.entity_type, c.entity_id, c.parent_id,
			   c.author_id, COALESCE(u.full_name, 'Unknown'), u.avatar_url,
			   c.content_raw, c.content_html, c.attachments,
			   c.is_pinned, c.is_deleted, c.created_at, c.updated_at
		FROM comments c
		LEFT JOIN users u ON u.id = c.author_id
		WHERE c.organization_id = $1 AND c.entity_type = $2 AND c.entity_id = $3
		ORDER BY c.is_pinned DESC, %s`, order), orgID, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("collaboration: get comments: %w", err)
	}
	defer rows.Close()

	commentMap := map[string]*Comment{}
	var roots []string
	for rows.Next() {
		var c Comment
		var attachJSON string
		if err := rows.Scan(&c.ID, &c.OrgID, &c.EntityType, &c.EntityID, &c.ParentID,
			&c.AuthorID, &c.AuthorName, &c.AuthorAvatar,
			&c.ContentRaw, &c.ContentHTML, &attachJSON,
			&c.IsPinned, &c.IsDeleted, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("collaboration: scan comment: %w", err)
		}
		_ = json.Unmarshal([]byte(attachJSON), &c.Attachments)
		if c.Attachments == nil {
			c.Attachments = []string{}
		}
		c.Reactions = map[string]int{}
		c.Children = []Comment{}
		commentMap[c.ID] = &c
		if c.ParentID == nil {
			roots = append(roots, c.ID)
		}
	}

	// Load reactions
	reactionRows, err := s.pool.Query(ctx, `
		SELECT comment_id, reaction, COUNT(*) FROM comment_reactions
		WHERE comment_id = ANY(
			SELECT id FROM comments WHERE organization_id = $1 AND entity_type = $2 AND entity_id = $3
		)
		GROUP BY comment_id, reaction`, orgID, entityType, entityID)
	if err == nil {
		defer reactionRows.Close()
		for reactionRows.Next() {
			var cid, reaction string
			var cnt int
			if err := reactionRows.Scan(&cid, &reaction, &cnt); err != nil {
				continue
			}
			if cm, ok := commentMap[cid]; ok {
				cm.Reactions[reaction] = cnt
			}
		}
	}

	// Build tree
	for _, c := range commentMap {
		if c.ParentID != nil {
			if parent, ok := commentMap[*c.ParentID]; ok {
				parent.Children = append(parent.Children, *c)
			}
		}
	}

	var result []Comment
	for _, id := range roots {
		if c, ok := commentMap[id]; ok {
			result = append(result, *c)
		}
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Activity feed
// ---------------------------------------------------------------------------

// RecordActivity creates an activity feed entry and notifies entity followers.
func (s *CollaborationService) RecordActivity(ctx context.Context, orgID, userID, action, entityType, entityID, entityRef, entityTitle, description string, changes map[string]interface{}) error {
	changesJSON, _ := json.Marshal(changes)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO activity_feed
			(id, organization_id, user_id, action, entity_type, entity_id,
			 entity_ref, entity_title, description, changes, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())`,
		orgID, userID, action, entityType, entityID, entityRef, entityTitle, description, string(changesJSON))
	if err != nil {
		return fmt.Errorf("collaboration: record activity: %w", err)
	}

	s.notifyFollowers(ctx, orgID, entityType, entityID, userID, "activity."+action, map[string]interface{}{
		"entity_ref": entityRef, "entity_title": entityTitle, "description": description,
	})
	return nil
}

// GetActivityFeed retrieves the activity feed with filters and pagination.
func (s *CollaborationService) GetActivityFeed(ctx context.Context, orgID, userID string, filters ActivityFilter, page, pageSize int) ([]ActivityEntry, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 25
	}

	where := "af.organization_id = $1"
	args := []interface{}{orgID}
	idx := 2

	if filters.EntityType != nil {
		where += fmt.Sprintf(" AND af.entity_type = $%d", idx)
		args = append(args, *filters.EntityType)
		idx++
	}
	if filters.EntityID != nil {
		where += fmt.Sprintf(" AND af.entity_id = $%d", idx)
		args = append(args, *filters.EntityID)
		idx++
	}
	if filters.UserID != nil {
		where += fmt.Sprintf(" AND af.user_id = $%d", idx)
		args = append(args, *filters.UserID)
		idx++
	}
	if filters.Action != nil {
		where += fmt.Sprintf(" AND af.action = $%d", idx)
		args = append(args, *filters.Action)
		idx++
	}

	var total int
	_ = s.pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM activity_feed af WHERE %s", where), args...).Scan(&total)

	offset := (page - 1) * pageSize
	qArgs := append(args, pageSize, offset)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT af.id, af.organization_id, af.user_id, COALESCE(u.full_name, 'System'),
			   af.action, af.entity_type, af.entity_id, af.entity_ref, af.entity_title,
			   af.description, af.changes, af.created_at
		FROM activity_feed af
		LEFT JOIN users u ON u.id = af.user_id
		WHERE %s
		ORDER BY af.created_at DESC
		LIMIT $%d OFFSET $%d`, where, idx, idx+1), qArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("collaboration: get activity feed: %w", err)
	}
	defer rows.Close()

	var entries []ActivityEntry
	for rows.Next() {
		var e ActivityEntry
		var changesJSON string
		if err := rows.Scan(&e.ID, &e.OrgID, &e.UserID, &e.UserName,
			&e.Action, &e.EntityType, &e.EntityID, &e.EntityRef, &e.EntityTitle,
			&e.Description, &changesJSON, &e.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("collaboration: scan activity: %w", err)
		}
		_ = json.Unmarshal([]byte(changesJSON), &e.Changes)
		entries = append(entries, e)
	}
	return entries, total, nil
}

// GetUnreadCounts returns unread activity counts per entity for a user.
func (s *CollaborationService) GetUnreadCounts(ctx context.Context, orgID, userID string) ([]UnreadCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT af.entity_type, af.entity_id, COUNT(*) AS cnt
		FROM activity_feed af
		JOIN entity_follows ef ON ef.organization_id = af.organization_id
			AND ef.entity_type = af.entity_type AND ef.entity_id = af.entity_id
			AND ef.user_id = $2
		LEFT JOIN read_markers rm ON rm.user_id = $2
			AND rm.entity_type = af.entity_type AND rm.entity_id = af.entity_id
		WHERE af.organization_id = $1
		  AND af.user_id != $2
		  AND (rm.last_read_at IS NULL OR af.created_at > rm.last_read_at)
		GROUP BY af.entity_type, af.entity_id
		ORDER BY cnt DESC`, orgID, userID)
	if err != nil {
		return nil, fmt.Errorf("collaboration: unread counts: %w", err)
	}
	defer rows.Close()

	var counts []UnreadCount
	for rows.Next() {
		var uc UnreadCount
		if err := rows.Scan(&uc.EntityType, &uc.EntityID, &uc.Count); err != nil {
			continue
		}
		counts = append(counts, uc)
	}
	return counts, nil
}

// ---------------------------------------------------------------------------
// Follow / unfollow
// ---------------------------------------------------------------------------

// FollowEntity subscribes a user to updates on an entity.
func (s *CollaborationService) FollowEntity(ctx context.Context, orgID, userID, entityType, entityID string) error {
	return s.followEntity(ctx, orgID, userID, entityType, entityID)
}

func (s *CollaborationService) followEntity(ctx context.Context, orgID, userID, entityType, entityID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO entity_follows (id, organization_id, user_id, entity_type, entity_id, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, NOW())
		ON CONFLICT (organization_id, user_id, entity_type, entity_id) DO NOTHING`,
		orgID, userID, entityType, entityID)
	if err != nil {
		return fmt.Errorf("collaboration: follow entity: %w", err)
	}
	return nil
}

// UnfollowEntity unsubscribes a user from entity updates.
func (s *CollaborationService) UnfollowEntity(ctx context.Context, orgID, userID, entityType, entityID string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM entity_follows
		WHERE organization_id = $1 AND user_id = $2 AND entity_type = $3 AND entity_id = $4`,
		orgID, userID, entityType, entityID)
	if err != nil {
		return fmt.Errorf("collaboration: unfollow entity: %w", err)
	}
	return nil
}

// GetFollowedEntities returns all entities a user follows.
func (s *CollaborationService) GetFollowedEntities(ctx context.Context, orgID, userID string) ([]FollowedEntity, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ef.entity_type, ef.entity_id,
			   COALESCE(si.entity_ref, ''), COALESCE(si.title, ''), ef.created_at
		FROM entity_follows ef
		LEFT JOIN search_index si ON si.entity_type = ef.entity_type AND si.entity_id = ef.entity_id
		WHERE ef.organization_id = $1 AND ef.user_id = $2
		ORDER BY ef.created_at DESC`, orgID, userID)
	if err != nil {
		return nil, fmt.Errorf("collaboration: followed entities: %w", err)
	}
	defer rows.Close()

	var result []FollowedEntity
	for rows.Next() {
		var fe FollowedEntity
		if err := rows.Scan(&fe.EntityType, &fe.EntityID, &fe.EntityRef, &fe.EntityTitle, &fe.FollowedAt); err != nil {
			continue
		}
		result = append(result, fe)
	}
	return result, nil
}

// MarkAsRead updates the read marker for a user on an entity.
func (s *CollaborationService) MarkAsRead(ctx context.Context, userID, entityType, entityID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO read_markers (id, user_id, entity_type, entity_id, last_read_at)
		VALUES (gen_random_uuid(), $1, $2, $3, NOW())
		ON CONFLICT (user_id, entity_type, entity_id) DO UPDATE SET last_read_at = NOW()`,
		userID, entityType, entityID)
	if err != nil {
		return fmt.Errorf("collaboration: mark as read: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseMentions extracts user IDs and role slugs from @mention syntax.
func parseMentions(content string) ([]string, []string) {
	var userIDs, roleSlugs []string
	for _, m := range mentionUserRe.FindAllStringSubmatch(content, -1) {
		if len(m) > 1 {
			userIDs = append(userIDs, m[1])
		}
	}
	for _, m := range mentionRoleRe.FindAllStringSubmatch(content, -1) {
		if len(m) > 1 {
			roleSlugs = append(roleSlugs, m[1])
		}
	}
	return userIDs, roleSlugs
}

func (s *CollaborationService) resolveRoleUsers(ctx context.Context, orgID, roleSlug string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_id FROM user_roles WHERE organization_id = $1 AND role = $2`, orgID, roleSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *CollaborationService) notifyFollowers(ctx context.Context, orgID, entityType, entityID, excludeUser, eventType string, data map[string]interface{}) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_id FROM entity_follows
		WHERE organization_id = $1 AND entity_type = $2 AND entity_id = $3 AND user_id != $4`,
		orgID, entityType, entityID, excludeUser)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			continue
		}
		d := map[string]interface{}{"follower": uid}
		for k, v := range data {
			d[k] = v
		}
		s.bus.Publish(Event{
			Type: eventType, Severity: "low", OrgID: orgID,
			EntityType: entityType, EntityID: entityID,
			Data: d, Timestamp: time.Now(),
		})
	}
}

// renderMarkdownToHTML performs basic markdown-to-HTML conversion for comments.
func renderMarkdownToHTML(md string) string {
	// Minimal conversion — bold, italic, code, links, mentions
	h := strings.ReplaceAll(md, "&", "&amp;")
	h = strings.ReplaceAll(h, "<", "&lt;")
	h = strings.ReplaceAll(h, ">", "&gt;")
	h = regexp.MustCompile(`\*\*(.+?)\*\*`).ReplaceAllString(h, "<strong>$1</strong>")
	h = regexp.MustCompile(`\*(.+?)\*`).ReplaceAllString(h, "<em>$1</em>")
	h = regexp.MustCompile("`(.+?)`").ReplaceAllString(h, "<code>$1</code>")
	h = mentionUserRe.ReplaceAllString(h, `<span class="mention" data-user-id="$1">@$1</span>`)
	h = mentionRoleRe.ReplaceAllString(h, `<span class="mention mention-role" data-role="$1">@$1</span>`)
	h = strings.ReplaceAll(h, "\n", "<br>")
	return h
}
