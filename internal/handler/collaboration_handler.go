package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

// ---------- service interface ----------

// CollaborationService defines the methods required by CollaborationHandler.
type CollaborationService interface {
	// Comments
	ListComments(ctx context.Context, orgID, entityType, entityID string, pagination models.PaginationRequest) ([]Comment, int, error)
	CreateComment(ctx context.Context, orgID, userID, entityType, entityID string, comment *Comment) error
	UpdateComment(ctx context.Context, orgID, userID, commentID string, comment *Comment) error
	DeleteComment(ctx context.Context, orgID, userID, commentID string) error
	PinComment(ctx context.Context, orgID, userID, commentID string) error
	ReactToComment(ctx context.Context, orgID, userID, commentID string, reaction *CommentReaction) error

	// Activity feed
	GetUserFeed(ctx context.Context, orgID, userID string, pagination models.PaginationRequest) ([]ActivityEntry, int, error)
	GetOrgFeed(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]ActivityEntry, int, error)
	GetEntityActivity(ctx context.Context, orgID, entityType, entityID string, pagination models.PaginationRequest) ([]ActivityEntry, int, error)
	GetUnreadCount(ctx context.Context, orgID, userID string) (*UnreadActivityCount, error)
	MarkEntityRead(ctx context.Context, orgID, userID, entityType, entityID string) error

	// Following
	ListFollowing(ctx context.Context, orgID, userID string) ([]FollowedEntity, error)
	Follow(ctx context.Context, orgID, userID, entityType, entityID string) error
	Unfollow(ctx context.Context, orgID, userID, entityType, entityID string) error
}

// ---------- request / response types ----------

// Comment represents a comment on an entity.
type Comment struct {
	ID             string            `json:"id"`
	OrganizationID string            `json:"organization_id"`
	EntityType     string            `json:"entity_type"`
	EntityID       string            `json:"entity_id"`
	ParentID       string            `json:"parent_id,omitempty"` // for threaded replies
	Content        string            `json:"content" validate:"required"`
	ContentHTML    string            `json:"content_html,omitempty"`
	AuthorID       string            `json:"author_id"`
	AuthorName     string            `json:"author_name,omitempty"`
	IsPinned       bool              `json:"is_pinned"`
	Reactions      []CommentReaction `json:"reactions,omitempty"`
	Mentions       []string          `json:"mentions,omitempty"`
	Attachments    []string          `json:"attachments,omitempty"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
}

// CommentReaction represents a reaction on a comment.
type CommentReaction struct {
	Emoji  string `json:"emoji" validate:"required"`
	UserID string `json:"user_id,omitempty"`
	Count  int    `json:"count,omitempty"`
}

// ActivityEntry represents an activity feed entry.
type ActivityEntry struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	ActorID        string `json:"actor_id"`
	ActorName      string `json:"actor_name,omitempty"`
	Action         string `json:"action"` // created, updated, deleted, commented, approved, rejected, assigned, etc.
	EntityType     string `json:"entity_type"`
	EntityID       string `json:"entity_id"`
	EntityTitle    string `json:"entity_title,omitempty"`
	Description    string `json:"description,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	IsRead         bool   `json:"is_read"`
	CreatedAt      string `json:"created_at"`
}

// UnreadActivityCount holds unread activity counts by type.
type UnreadActivityCount struct {
	Total      int            `json:"total"`
	ByType     map[string]int `json:"by_type,omitempty"`
}

// FollowedEntity represents an entity a user is following.
type FollowedEntity struct {
	EntityType  string `json:"entity_type"`
	EntityID    string `json:"entity_id"`
	EntityTitle string `json:"entity_title,omitempty"`
	FollowedAt  string `json:"followed_at"`
}

// ---------- handler ----------

// CollaborationHandler handles comments, activity feed, and following endpoints.
type CollaborationHandler struct {
	svc CollaborationService
}

// NewCollaborationHandler creates a new CollaborationHandler with the given service.
func NewCollaborationHandler(svc CollaborationService) *CollaborationHandler {
	return &CollaborationHandler{svc: svc}
}

// ListComments handles GET /comments/{entityType}/{entityId}.
func (h *CollaborationHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	entityType := chi.URLParam(r, "entityType")
	entityID := chi.URLParam(r, "entityId")
	if entityType == "" || entityID == "" {
		writeError(w, http.StatusBadRequest, "Missing entityType or entityId", "")
		return
	}

	pagination := parsePagination(r)

	comments, total, err := h.svc.ListComments(r.Context(), orgID, entityType, entityID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list comments", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": comments,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateComment handles POST /comments/{entityType}/{entityId}.
func (h *CollaborationHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	entityType := chi.URLParam(r, "entityType")
	entityID := chi.URLParam(r, "entityId")
	if entityType == "" || entityID == "" {
		writeError(w, http.StatusBadRequest, "Missing entityType or entityId", "")
		return
	}

	var comment Comment
	if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if comment.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required", "")
		return
	}

	comment.EntityType = entityType
	comment.EntityID = entityID
	comment.AuthorID = userID
	comment.OrganizationID = orgID

	if err := h.svc.CreateComment(r.Context(), orgID, userID, entityType, entityID, &comment); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create comment", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, comment)
}

// UpdateComment handles PUT /comments/{id}.
func (h *CollaborationHandler) UpdateComment(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	commentID := chi.URLParam(r, "id")
	if commentID == "" {
		writeError(w, http.StatusBadRequest, "Missing comment ID", "")
		return
	}

	var comment Comment
	if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	comment.ID = commentID

	if err := h.svc.UpdateComment(r.Context(), orgID, userID, commentID, &comment); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update comment", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, comment)
}

// DeleteComment handles DELETE /comments/{id}.
func (h *CollaborationHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	commentID := chi.URLParam(r, "id")
	if commentID == "" {
		writeError(w, http.StatusBadRequest, "Missing comment ID", "")
		return
	}

	if err := h.svc.DeleteComment(r.Context(), orgID, userID, commentID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete comment", err.Error())
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// PinComment handles POST /comments/{id}/pin.
func (h *CollaborationHandler) PinComment(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	commentID := chi.URLParam(r, "id")
	if commentID == "" {
		writeError(w, http.StatusBadRequest, "Missing comment ID", "")
		return
	}

	if err := h.svc.PinComment(r.Context(), orgID, userID, commentID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to pin comment", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Comment pinned"})
}

// ReactToComment handles POST /comments/{id}/react.
func (h *CollaborationHandler) ReactToComment(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	commentID := chi.URLParam(r, "id")
	if commentID == "" {
		writeError(w, http.StatusBadRequest, "Missing comment ID", "")
		return
	}

	var reaction CommentReaction
	if err := json.NewDecoder(r.Body).Decode(&reaction); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if reaction.Emoji == "" {
		writeError(w, http.StatusBadRequest, "emoji is required", "")
		return
	}

	if err := h.svc.ReactToComment(r.Context(), orgID, userID, commentID, &reaction); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to react to comment", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Reaction added"})
}

// GetUserFeed handles GET /activity/feed.
func (h *CollaborationHandler) GetUserFeed(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	pagination := parsePagination(r)

	entries, total, err := h.svc.GetUserFeed(r.Context(), orgID, userID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get activity feed", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": entries,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetOrgFeed handles GET /activity/org.
func (h *CollaborationHandler) GetOrgFeed(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	entries, total, err := h.svc.GetOrgFeed(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get organization activity", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": entries,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetEntityActivity handles GET /activity/{entityType}/{entityId}.
func (h *CollaborationHandler) GetEntityActivity(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	entityType := chi.URLParam(r, "entityType")
	entityID := chi.URLParam(r, "entityId")
	if entityType == "" || entityID == "" {
		writeError(w, http.StatusBadRequest, "Missing entityType or entityId", "")
		return
	}

	pagination := parsePagination(r)

	entries, total, err := h.svc.GetEntityActivity(r.Context(), orgID, entityType, entityID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get entity activity", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": entries,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetUnreadCount handles GET /activity/unread.
func (h *CollaborationHandler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	count, err := h.svc.GetUnreadCount(r.Context(), orgID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get unread count", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, count)
}

// MarkEntityRead handles POST /activity/{entityType}/{entityId}/mark-read.
func (h *CollaborationHandler) MarkEntityRead(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	entityType := chi.URLParam(r, "entityType")
	entityID := chi.URLParam(r, "entityId")
	if entityType == "" || entityID == "" {
		writeError(w, http.StatusBadRequest, "Missing entityType or entityId", "")
		return
	}

	if err := h.svc.MarkEntityRead(r.Context(), orgID, userID, entityType, entityID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to mark as read", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Marked as read"})
}

// ListFollowing handles GET /following.
func (h *CollaborationHandler) ListFollowing(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	following, err := h.svc.ListFollowing(r.Context(), orgID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list followed entities", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": following})
}

// Follow handles POST /following/{entityType}/{entityId}.
func (h *CollaborationHandler) Follow(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	entityType := chi.URLParam(r, "entityType")
	entityID := chi.URLParam(r, "entityId")
	if entityType == "" || entityID == "" {
		writeError(w, http.StatusBadRequest, "Missing entityType or entityId", "")
		return
	}

	if err := h.svc.Follow(r.Context(), orgID, userID, entityType, entityID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to follow entity", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "Now following"})
}

// Unfollow handles DELETE /following/{entityType}/{entityId}.
func (h *CollaborationHandler) Unfollow(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	entityType := chi.URLParam(r, "entityType")
	entityID := chi.URLParam(r, "entityId")
	if entityType == "" || entityID == "" {
		writeError(w, http.StatusBadRequest, "Missing entityType or entityId", "")
		return
	}

	if err := h.svc.Unfollow(r.Context(), orgID, userID, entityType, entityID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to unfollow entity", err.Error())
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}
