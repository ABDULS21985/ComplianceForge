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

// SearchService defines the methods required by SearchHandler.
type SearchService interface {
	// Global search
	Search(ctx context.Context, orgID string, params SearchParams) (*SearchResults, error)
	Autocomplete(ctx context.Context, orgID, query string, limit int) ([]AutocompleteResult, error)
	GetRelated(ctx context.Context, orgID, entityType, entityID string) ([]RelatedEntity, error)
	Reindex(ctx context.Context, orgID, userID string, req *ReindexRequest) (*ReindexStatus, error)

	// Knowledge base
	SearchKnowledgeArticles(ctx context.Context, pagination models.PaginationRequest, filters KnowledgeFilters) ([]KnowledgeArticle, int, error)
	GetKnowledgeArticle(ctx context.Context, slug string) (*KnowledgeArticle, error)
	GetArticlesForControl(ctx context.Context, frameworkCode, controlCode string) ([]KnowledgeArticle, error)
	GetRecommendedArticles(ctx context.Context, orgID string) ([]KnowledgeArticle, error)
	CreateArticle(ctx context.Context, orgID, userID string, article *KnowledgeArticle) error
	UpdateArticle(ctx context.Context, orgID, userID, articleID string, article *KnowledgeArticle) error
	SubmitArticleFeedback(ctx context.Context, orgID, userID, articleID string, feedback *ArticleFeedback) error

	// Bookmarks
	ListBookmarks(ctx context.Context, orgID, userID string) ([]KnowledgeBookmark, error)
	CreateBookmark(ctx context.Context, orgID, userID, articleID string) error
	DeleteBookmark(ctx context.Context, orgID, userID, articleID string) error
}

// ---------- request / response types ----------

// SearchParams holds parameters for global search.
type SearchParams struct {
	Query      string   `json:"query"`
	Types      []string `json:"types,omitempty"` // risk, control, policy, audit, vendor, etc.
	Status     string   `json:"status,omitempty"`
	DateFrom   string   `json:"date_from,omitempty"`
	DateTo     string   `json:"date_to,omitempty"`
	Page       int      `json:"page"`
	PageSize   int      `json:"page_size"`
	Sort       string   `json:"sort,omitempty"` // relevance, date, name
	Highlight  bool     `json:"highlight"`
}

// SearchResults holds global search results.
type SearchResults struct {
	Query      string             `json:"query"`
	TotalHits  int                `json:"total_hits"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
	Results    []SearchResultItem `json:"results"`
	Facets     map[string][]Facet `json:"facets,omitempty"`
	TimeTakenMs int               `json:"time_taken_ms"`
}

// SearchResultItem represents a single search result.
type SearchResultItem struct {
	EntityType  string            `json:"entity_type"`
	EntityID    string            `json:"entity_id"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Status      string            `json:"status,omitempty"`
	Score       float64           `json:"score"`
	Highlights  map[string]string `json:"highlights,omitempty"`
	UpdatedAt   string            `json:"updated_at,omitempty"`
}

// Facet represents a search facet.
type Facet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// AutocompleteResult represents a single autocomplete suggestion.
type AutocompleteResult struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Title      string `json:"title"`
	Subtitle   string `json:"subtitle,omitempty"`
}

// RelatedEntity represents an entity related to a given entity.
type RelatedEntity struct {
	EntityType   string `json:"entity_type"`
	EntityID     string `json:"entity_id"`
	Title        string `json:"title"`
	Relationship string `json:"relationship"` // parent, child, linked, referenced
}

// ReindexRequest is the payload for POST /search/reindex.
type ReindexRequest struct {
	EntityTypes []string `json:"entity_types,omitempty"` // empty means all
	Force       bool     `json:"force"`
}

// ReindexStatus holds the status of a reindex operation.
type ReindexStatus struct {
	Status       string `json:"status"` // queued, running, completed, error
	StartedAt    string `json:"started_at,omitempty"`
	CompletedAt  string `json:"completed_at,omitempty"`
	TotalDocs    int    `json:"total_docs"`
	IndexedDocs  int    `json:"indexed_docs"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// KnowledgeFilters holds filter parameters for knowledge base articles.
type KnowledgeFilters struct {
	Category string `json:"category"`
	Tag      string `json:"tag"`
	Search   string `json:"search"`
}

// KnowledgeArticle represents a knowledge base article.
type KnowledgeArticle struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id,omitempty"`
	Slug           string   `json:"slug"`
	Title          string   `json:"title" validate:"required"`
	Content        string   `json:"content" validate:"required"`
	Summary        string   `json:"summary,omitempty"`
	Category       string   `json:"category,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	FrameworkCodes []string `json:"framework_codes,omitempty"`
	ControlCodes   []string `json:"control_codes,omitempty"`
	IsPublic       bool     `json:"is_public"`
	ViewCount      int      `json:"view_count"`
	HelpfulCount   int      `json:"helpful_count"`
	AuthorID       string   `json:"author_id,omitempty"`
	AuthorName     string   `json:"author_name,omitempty"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

// ArticleFeedback is the payload for POST /knowledge/articles/{id}/feedback.
type ArticleFeedback struct {
	Helpful  bool   `json:"helpful"`
	Comments string `json:"comments,omitempty"`
}

// KnowledgeBookmark represents a user's bookmarked article.
type KnowledgeBookmark struct {
	ArticleID  string `json:"article_id"`
	Title      string `json:"title"`
	Slug       string `json:"slug"`
	Category   string `json:"category,omitempty"`
	BookmarkedAt string `json:"bookmarked_at"`
}

// ---------- handler ----------

// SearchHandler handles global search and knowledge base endpoints.
type SearchHandler struct {
	svc SearchService
}

// NewSearchHandler creates a new SearchHandler with the given service.
func NewSearchHandler(svc SearchService) *SearchHandler {
	return &SearchHandler{svc: svc}
}

// Search handles GET /search.
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	params := SearchParams{
		Query:     r.URL.Query().Get("q"),
		Status:    r.URL.Query().Get("status"),
		DateFrom:  r.URL.Query().Get("date_from"),
		DateTo:    r.URL.Query().Get("date_to"),
		Sort:      r.URL.Query().Get("sort"),
		Highlight: r.URL.Query().Get("highlight") == "true",
		Page:      pagination.Page,
		PageSize:  pagination.PageSize,
	}

	if params.Query == "" {
		writeError(w, http.StatusBadRequest, "Query parameter 'q' is required", "")
		return
	}

	results, err := h.svc.Search(r.Context(), orgID, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Search failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, results)
}

// Autocomplete handles GET /search/autocomplete.
func (h *SearchHandler) Autocomplete(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "Query parameter 'q' is required", "")
		return
	}

	limit := 10
	results, err := h.svc.Autocomplete(r.Context(), orgID, query, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Autocomplete failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": results})
}

// GetRelated handles GET /search/related/{entityType}/{entityId}.
func (h *SearchHandler) GetRelated(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	entityType := chi.URLParam(r, "entityType")
	entityID := chi.URLParam(r, "entityId")
	if entityType == "" || entityID == "" {
		writeError(w, http.StatusBadRequest, "Missing entityType or entityId", "")
		return
	}

	related, err := h.svc.GetRelated(r.Context(), orgID, entityType, entityID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get related entities", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": related})
}

// Reindex handles POST /search/reindex.
func (h *SearchHandler) Reindex(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var req ReindexRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	status, err := h.svc.Reindex(r.Context(), orgID, userID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to start reindex", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, status)
}

// SearchKnowledge handles GET /knowledge.
func (h *SearchHandler) SearchKnowledge(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)

	filters := KnowledgeFilters{
		Category: r.URL.Query().Get("category"),
		Tag:      r.URL.Query().Get("tag"),
		Search:   r.URL.Query().Get("search"),
	}

	articles, total, err := h.svc.SearchKnowledgeArticles(r.Context(), pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to search knowledge base", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": articles,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetKnowledgeArticle handles GET /knowledge/{slug}.
func (h *SearchHandler) GetKnowledgeArticle(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "Missing article slug", "")
		return
	}

	article, err := h.svc.GetKnowledgeArticle(r.Context(), slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "Article not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, article)
}

// GetArticlesForControl handles GET /knowledge/for-control/{frameworkCode}/{controlCode}.
func (h *SearchHandler) GetArticlesForControl(w http.ResponseWriter, r *http.Request) {
	frameworkCode := chi.URLParam(r, "frameworkCode")
	controlCode := chi.URLParam(r, "controlCode")
	if frameworkCode == "" || controlCode == "" {
		writeError(w, http.StatusBadRequest, "Missing frameworkCode or controlCode", "")
		return
	}

	articles, err := h.svc.GetArticlesForControl(r.Context(), frameworkCode, controlCode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get articles for control", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": articles})
}

// GetRecommendedArticles handles GET /knowledge/recommended.
func (h *SearchHandler) GetRecommendedArticles(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	articles, err := h.svc.GetRecommendedArticles(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get recommended articles", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": articles})
}

// CreateArticle handles POST /knowledge/articles.
func (h *SearchHandler) CreateArticle(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var article KnowledgeArticle
	if err := json.NewDecoder(r.Body).Decode(&article); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if article.Title == "" || article.Content == "" {
		writeError(w, http.StatusBadRequest, "title and content are required", "")
		return
	}

	if err := h.svc.CreateArticle(r.Context(), orgID, userID, &article); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create article", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, article)
}

// UpdateArticle handles PUT /knowledge/articles/{id}.
func (h *SearchHandler) UpdateArticle(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	articleID := chi.URLParam(r, "id")
	if articleID == "" {
		writeError(w, http.StatusBadRequest, "Missing article ID", "")
		return
	}

	var article KnowledgeArticle
	if err := json.NewDecoder(r.Body).Decode(&article); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.UpdateArticle(r.Context(), orgID, userID, articleID, &article); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update article", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, article)
}

// SubmitArticleFeedback handles POST /knowledge/articles/{id}/feedback.
func (h *SearchHandler) SubmitArticleFeedback(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	articleID := chi.URLParam(r, "id")
	if articleID == "" {
		writeError(w, http.StatusBadRequest, "Missing article ID", "")
		return
	}

	var feedback ArticleFeedback
	if err := json.NewDecoder(r.Body).Decode(&feedback); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.SubmitArticleFeedback(r.Context(), orgID, userID, articleID, &feedback); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to submit feedback", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Feedback submitted"})
}

// ListBookmarks handles GET /knowledge/bookmarks.
func (h *SearchHandler) ListBookmarks(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	bookmarks, err := h.svc.ListBookmarks(r.Context(), orgID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list bookmarks", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": bookmarks})
}

// CreateBookmark handles POST /knowledge/bookmarks/{articleId}.
func (h *SearchHandler) CreateBookmark(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	articleID := chi.URLParam(r, "articleId")
	if articleID == "" {
		writeError(w, http.StatusBadRequest, "Missing article ID", "")
		return
	}

	if err := h.svc.CreateBookmark(r.Context(), orgID, userID, articleID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create bookmark", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "Bookmark created"})
}

// DeleteBookmark handles DELETE /knowledge/bookmarks/{articleId}.
func (h *SearchHandler) DeleteBookmark(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	articleID := chi.URLParam(r, "articleId")
	if articleID == "" {
		writeError(w, http.StatusBadRequest, "Missing article ID", "")
		return
	}

	if err := h.svc.DeleteBookmark(r.Context(), orgID, userID, articleID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete bookmark", err.Error())
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}
