package service

import (
	"context"
	"fmt"
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
	ErrArticleNotFound = fmt.Errorf("knowledge article not found")
	ErrInvalidSearch   = fmt.Errorf("search query must not be empty")
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// SearchRequest holds full-text search parameters.
type SearchRequest struct {
	Query      string   `json:"query"`
	Types      []string `json:"types"`       // entity types to include
	Tags       []string `json:"tags"`
	Status     *string  `json:"status"`
	SortBy     string   `json:"sort_by"`     // relevance, date, title
	Page       int      `json:"page"`
	PageSize   int      `json:"page_size"`
}

// SearchResult is a single hit returned by search.
type SearchResult struct {
	EntityType  string  `json:"entity_type"`
	EntityID    string  `json:"entity_id"`
	EntityRef   string  `json:"entity_ref"`
	Title       string  `json:"title"`
	Snippet     string  `json:"snippet"`
	Rank        float64 `json:"rank"`
	Status      string  `json:"status"`
	Tags        string  `json:"tags"`
	UpdatedAt   string  `json:"updated_at"`
}

// SearchFacet is a count-per-type aggregation.
type SearchFacet struct {
	EntityType string `json:"entity_type"`
	Count      int    `json:"count"`
}

// SearchResponse wraps search results with metadata.
type SearchResponse struct {
	Results    []SearchResult `json:"results"`
	Facets     []SearchFacet  `json:"facets"`
	TotalCount int            `json:"total_count"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
	QueryTime  float64        `json:"query_time_ms"`
}

// AutocompleteResult is a lightweight prefix suggestion.
type AutocompleteResult struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Title      string `json:"title"`
	EntityRef  string `json:"entity_ref"`
}

// RelatedEntity links an entity to contextually related items.
type RelatedEntity struct {
	EntityType   string `json:"entity_type"`
	EntityID     string `json:"entity_id"`
	EntityRef    string `json:"entity_ref"`
	Title        string `json:"title"`
	Relationship string `json:"relationship"` // linked, shared_tag, same_framework
}

// KnowledgeArticle represents a knowledge base article.
type KnowledgeArticle struct {
	ID             string   `json:"id"`
	Slug           string   `json:"slug"`
	Title          string   `json:"title"`
	Summary        string   `json:"summary"`
	Content        string   `json:"content"`
	Category       string   `json:"category"`
	Tags           []string `json:"tags"`
	FrameworkCodes []string `json:"framework_codes"`
	ControlCodes   []string `json:"control_codes"`
	Author         string   `json:"author"`
	Status         string   `json:"status"`
	ViewCount      int      `json:"view_count"`
	HelpfulCount   int      `json:"helpful_count"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

// KnowledgeFilter is a filter set for browsing knowledge articles.
type KnowledgeFilter struct {
	Category       *string  `json:"category"`
	Tags           []string `json:"tags"`
	FrameworkCode  *string  `json:"framework_code"`
	SearchQuery    *string  `json:"search_query"`
	Page           int      `json:"page"`
	PageSize       int      `json:"page_size"`
}

// Bookmark represents a user's saved article bookmark.
type Bookmark struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	ArticleID string `json:"article_id"`
	Title     string `json:"title"`
	Slug      string `json:"slug"`
	CreatedAt string `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// SearchService provides full-text search and knowledge base capabilities.
type SearchService struct {
	pool *pgxpool.Pool
}

// NewSearchService creates a SearchService.
func NewSearchService(pool *pgxpool.Pool) *SearchService {
	return &SearchService{pool: pool}
}

// ---------------------------------------------------------------------------
// Indexing
// ---------------------------------------------------------------------------

// IndexEntity fetches a single entity and upserts its search_index record.
func (s *SearchService) IndexEntity(ctx context.Context, orgID, entityType, entityID string) error {
	var title, content, ref, status, tags string
	query := indexQuery(entityType)
	if query == "" {
		return fmt.Errorf("search: unsupported entity type %q", entityType)
	}
	err := s.pool.QueryRow(ctx, query, orgID, entityID).Scan(&title, &content, &ref, &status, &tags)
	if err == pgx.ErrNoRows {
		log.Warn().Str("entity_type", entityType).Str("entity_id", entityID).Msg("search: entity not found for indexing")
		return nil
	}
	if err != nil {
		return fmt.Errorf("search: fetch entity for index: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO search_index
			(id, organization_id, entity_type, entity_id, entity_ref, title,
			 content, status, tags, search_vector, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8,
				setweight(to_tsvector('english', $5), 'A') ||
				setweight(to_tsvector('english', $6), 'B') ||
				setweight(to_tsvector('english', COALESCE($8, '')), 'C'),
				NOW())
		ON CONFLICT (organization_id, entity_type, entity_id) DO UPDATE
		  SET title         = EXCLUDED.title,
			  content       = EXCLUDED.content,
			  entity_ref    = EXCLUDED.entity_ref,
			  status        = EXCLUDED.status,
			  tags          = EXCLUDED.tags,
			  search_vector = EXCLUDED.search_vector,
			  updated_at    = NOW()`,
		orgID, entityType, entityID, ref, title, content, status, tags)
	if err != nil {
		return fmt.Errorf("search: upsert index: %w", err)
	}
	return nil
}

// indexQuery returns the SQL to fetch indexable fields for a given entity type.
func indexQuery(entityType string) string {
	switch entityType {
	case "policy":
		return `SELECT title, COALESCE(content,''), policy_ref, status, '' FROM policies WHERE organization_id=$1 AND id=$2`
	case "risk":
		return `SELECT title, COALESCE(description,''), risk_ref, status, '' FROM risks WHERE organization_id=$1 AND id=$2`
	case "control":
		return `SELECT title, COALESCE(description,''), control_code, implementation_status, '' FROM controls WHERE organization_id=$1 AND id=$2`
	case "vendor":
		return `SELECT name, COALESCE(description,''), vendor_ref, status, '' FROM vendors WHERE organization_id=$1 AND id=$2`
	case "incident":
		return `SELECT title, COALESCE(description,''), incident_ref, status, '' FROM incidents WHERE organization_id=$1 AND id=$2`
	case "evidence":
		return `SELECT title, COALESCE(description,''), evidence_ref, status, '' FROM evidence_items WHERE organization_id=$1 AND id=$2`
	case "exception":
		return `SELECT title, COALESCE(justification,''), exception_ref, status, '' FROM compliance_exceptions WHERE organization_id=$1 AND id=$2`
	case "audit_finding":
		return `SELECT title, COALESCE(description,''), audit_ref, status, '' FROM audit_findings WHERE organization_id=$1 AND id=$2`
	default:
		return ""
	}
}

// IndexAllEntities performs a full re-index of all supported entity types.
func (s *SearchService) IndexAllEntities(ctx context.Context, orgID string) error {
	start := time.Now()
	log.Info().Str("org_id", orgID).Msg("search: full re-index starting")

	entityTypes := []string{"policy", "risk", "control", "vendor", "incident", "evidence", "exception", "audit_finding"}
	totalIndexed := 0
	for _, et := range entityTypes {
		ids, err := s.entityIDs(ctx, orgID, et)
		if err != nil {
			log.Error().Err(err).Str("entity_type", et).Msg("search: failed to list entity IDs")
			continue
		}
		for _, id := range ids {
			if err := s.IndexEntity(ctx, orgID, et, id); err != nil {
				log.Error().Err(err).Str("entity_type", et).Str("entity_id", id).Msg("search: index failed")
				continue
			}
			totalIndexed++
		}
	}
	log.Info().Int("total", totalIndexed).Dur("elapsed", time.Since(start)).Msg("search: full re-index complete")
	return nil
}

func (s *SearchService) entityIDs(ctx context.Context, orgID, entityType string) ([]string, error) {
	table := entityTable(entityType)
	if table == "" {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, fmt.Sprintf("SELECT id FROM %s WHERE organization_id = $1", table), orgID)
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

func entityTable(et string) string {
	m := map[string]string{
		"policy": "policies", "risk": "risks", "control": "controls",
		"vendor": "vendors", "incident": "incidents", "evidence": "evidence_items",
		"exception": "compliance_exceptions", "audit_finding": "audit_findings",
	}
	return m[et]
}

// ---------------------------------------------------------------------------
// Full-text search
// ---------------------------------------------------------------------------

// Search performs a full-text search with ranking, snippets, and facets.
func (s *SearchService) Search(ctx context.Context, orgID string, req SearchRequest) (*SearchResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, ErrInvalidSearch
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}
	start := time.Now()
	tsQuery := toTSQuery(req.Query)
	offset := (req.Page - 1) * req.PageSize

	// Build WHERE
	where := "organization_id = $1 AND search_vector @@ to_tsquery('english', $2)"
	args := []interface{}{orgID, tsQuery}
	idx := 3
	if len(req.Types) > 0 {
		where += fmt.Sprintf(" AND entity_type = ANY($%d)", idx)
		args = append(args, req.Types)
		idx++
	}
	if req.Status != nil {
		where += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, *req.Status)
		idx++
	}

	orderBy := "rank DESC"
	if req.SortBy == "date" {
		orderBy = "updated_at DESC"
	} else if req.SortBy == "title" {
		orderBy = "title ASC"
	}

	// Count
	var totalCount int
	_ = s.pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM search_index WHERE %s", where), args...).Scan(&totalCount)

	// Facets
	facetRows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT entity_type, COUNT(*) FROM search_index WHERE %s GROUP BY entity_type ORDER BY COUNT(*) DESC`, where), args...)
	if err != nil {
		return nil, fmt.Errorf("search: facet query: %w", err)
	}
	defer facetRows.Close()
	var facets []SearchFacet
	for facetRows.Next() {
		var f SearchFacet
		if err := facetRows.Scan(&f.EntityType, &f.Count); err != nil {
			continue
		}
		facets = append(facets, f)
	}

	// Results
	resultArgs := append(args, req.PageSize, offset)
	limitIdx := idx
	resultQuery := fmt.Sprintf(`
		SELECT entity_type, entity_id, entity_ref, title,
			   ts_headline('english', content, to_tsquery('english', $2),
						   'MaxFragments=2, MaxWords=30, MinWords=10') AS snippet,
			   ts_rank_cd(search_vector, to_tsquery('english', $2)) AS rank,
			   status, tags, updated_at
		FROM search_index
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d`, where, orderBy, limitIdx, limitIdx+1)

	resultRows, err := s.pool.Query(ctx, resultQuery, resultArgs...)
	if err != nil {
		return nil, fmt.Errorf("search: result query: %w", err)
	}
	defer resultRows.Close()

	var results []SearchResult
	for resultRows.Next() {
		var r SearchResult
		if err := resultRows.Scan(&r.EntityType, &r.EntityID, &r.EntityRef, &r.Title,
			&r.Snippet, &r.Rank, &r.Status, &r.Tags, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("search: scan result: %w", err)
		}
		results = append(results, r)
	}

	elapsed := time.Since(start).Seconds() * 1000
	return &SearchResponse{
		Results:    results,
		Facets:     facets,
		TotalCount: totalCount,
		Page:       req.Page,
		PageSize:   req.PageSize,
		QueryTime:  elapsed,
	}, nil
}

// toTSQuery converts a user query string into a tsquery-compatible format.
func toTSQuery(q string) string {
	words := strings.Fields(strings.TrimSpace(q))
	for i, w := range words {
		words[i] = strings.ReplaceAll(w, "'", "") + ":*"
	}
	return strings.Join(words, " & ")
}

// Autocomplete provides fast prefix-based suggestions.
func (s *SearchService) Autocomplete(ctx context.Context, orgID, prefix string, limit int) ([]AutocompleteResult, error) {
	if limit < 1 || limit > 20 {
		limit = 10
	}
	tsq := toTSQuery(prefix)
	rows, err := s.pool.Query(ctx, `
		SELECT entity_type, entity_id, title, entity_ref
		FROM search_index
		WHERE organization_id = $1
		  AND search_vector @@ to_tsquery('english', $2)
		ORDER BY ts_rank_cd(search_vector, to_tsquery('english', $2)) DESC
		LIMIT $3`, orgID, tsq, limit)
	if err != nil {
		return nil, fmt.Errorf("search: autocomplete: %w", err)
	}
	defer rows.Close()

	var results []AutocompleteResult
	for rows.Next() {
		var r AutocompleteResult
		if err := rows.Scan(&r.EntityType, &r.EntityID, &r.Title, &r.EntityRef); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Related entities
// ---------------------------------------------------------------------------

// GetRelatedEntities finds items contextually related via shared tags or relationships.
func (s *SearchService) GetRelatedEntities(ctx context.Context, orgID, entityType, entityID string) ([]RelatedEntity, error) {
	rows, err := s.pool.Query(ctx, `
		WITH source AS (
			SELECT tags, search_vector FROM search_index
			WHERE organization_id = $1 AND entity_type = $2 AND entity_id = $3
		)
		SELECT si.entity_type, si.entity_id, si.entity_ref, si.title, 'shared_tag' AS relationship
		FROM search_index si, source src
		WHERE si.organization_id = $1
		  AND NOT (si.entity_type = $2 AND si.entity_id = $3)
		  AND si.tags != '' AND src.tags != ''
		  AND string_to_array(si.tags, ',') && string_to_array(src.tags, ',')
		ORDER BY ts_rank_cd(si.search_vector, src.search_vector) DESC
		LIMIT 10`, orgID, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("search: related entities: %w", err)
	}
	defer rows.Close()

	var results []RelatedEntity
	for rows.Next() {
		var r RelatedEntity
		if err := rows.Scan(&r.EntityType, &r.EntityID, &r.EntityRef, &r.Title, &r.Relationship); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Knowledge base
// ---------------------------------------------------------------------------

// ListKnowledgeArticles browses published KB articles with filters.
func (s *SearchService) ListKnowledgeArticles(ctx context.Context, orgID string, filters KnowledgeFilter) ([]KnowledgeArticle, int, error) {
	if filters.Page < 1 {
		filters.Page = 1
	}
	if filters.PageSize < 1 || filters.PageSize > 50 {
		filters.PageSize = 20
	}
	where := "status = 'published'"
	args := []interface{}{}
	idx := 1

	if filters.Category != nil {
		where += fmt.Sprintf(" AND category = $%d", idx)
		args = append(args, *filters.Category)
		idx++
	}
	if len(filters.Tags) > 0 {
		where += fmt.Sprintf(" AND tags && $%d", idx)
		args = append(args, filters.Tags)
		idx++
	}
	if filters.FrameworkCode != nil {
		where += fmt.Sprintf(" AND $%d = ANY(framework_codes)", idx)
		args = append(args, *filters.FrameworkCode)
		idx++
	}
	if filters.SearchQuery != nil && *filters.SearchQuery != "" {
		tsq := toTSQuery(*filters.SearchQuery)
		where += fmt.Sprintf(" AND to_tsvector('english', title || ' ' || content) @@ to_tsquery('english', $%d)", idx)
		args = append(args, tsq)
		idx++
	}

	var total int
	_ = s.pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM knowledge_articles WHERE %s", where), args...).Scan(&total)

	offset := (filters.Page - 1) * filters.PageSize
	queryArgs := append(args, filters.PageSize, offset)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, slug, title, summary, '', category, tags, framework_codes,
			   control_codes, author, status, view_count, helpful_count, created_at, updated_at
		FROM knowledge_articles
		WHERE %s
		ORDER BY updated_at DESC
		LIMIT $%d OFFSET $%d`, where, idx, idx+1), queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("search: list articles: %w", err)
	}
	defer rows.Close()

	var articles []KnowledgeArticle
	for rows.Next() {
		var a KnowledgeArticle
		if err := rows.Scan(&a.ID, &a.Slug, &a.Title, &a.Summary, &a.Content,
			&a.Category, &a.Tags, &a.FrameworkCodes, &a.ControlCodes,
			&a.Author, &a.Status, &a.ViewCount, &a.HelpfulCount,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("search: scan article: %w", err)
		}
		articles = append(articles, a)
	}
	return articles, total, nil
}

// GetArticle retrieves a single KB article by slug.
func (s *SearchService) GetArticle(ctx context.Context, slug string) (*KnowledgeArticle, error) {
	var a KnowledgeArticle
	err := s.pool.QueryRow(ctx, `
		SELECT id, slug, title, summary, content, category, tags, framework_codes,
			   control_codes, author, status, view_count, helpful_count, created_at, updated_at
		FROM knowledge_articles
		WHERE slug = $1 AND status = 'published'`, slug).Scan(
		&a.ID, &a.Slug, &a.Title, &a.Summary, &a.Content,
		&a.Category, &a.Tags, &a.FrameworkCodes, &a.ControlCodes,
		&a.Author, &a.Status, &a.ViewCount, &a.HelpfulCount,
		&a.CreatedAt, &a.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, ErrArticleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("search: get article: %w", err)
	}
	// Increment view count
	_, _ = s.pool.Exec(ctx, "UPDATE knowledge_articles SET view_count = view_count + 1 WHERE id = $1", a.ID)
	return &a, nil
}

// GetArticlesForControl returns guidance articles for a specific control.
func (s *SearchService) GetArticlesForControl(ctx context.Context, controlCode, frameworkCode string) ([]KnowledgeArticle, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, slug, title, summary, '', category, tags, framework_codes,
			   control_codes, author, status, view_count, helpful_count, created_at, updated_at
		FROM knowledge_articles
		WHERE status = 'published'
		  AND ($1 = ANY(control_codes) OR $2 = ANY(framework_codes))
		ORDER BY helpful_count DESC, view_count DESC
		LIMIT 10`, controlCode, frameworkCode)
	if err != nil {
		return nil, fmt.Errorf("search: articles for control: %w", err)
	}
	defer rows.Close()

	var articles []KnowledgeArticle
	for rows.Next() {
		var a KnowledgeArticle
		if err := rows.Scan(&a.ID, &a.Slug, &a.Title, &a.Summary, &a.Content,
			&a.Category, &a.Tags, &a.FrameworkCodes, &a.ControlCodes,
			&a.Author, &a.Status, &a.ViewCount, &a.HelpfulCount,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			continue
		}
		articles = append(articles, a)
	}
	return articles, nil
}

// GetRecommendedArticles returns articles personalized by the user's role and frameworks.
func (s *SearchService) GetRecommendedArticles(ctx context.Context, orgID, userID string) ([]KnowledgeArticle, error) {
	rows, err := s.pool.Query(ctx, `
		WITH user_frameworks AS (
			SELECT DISTINCT f.code
			FROM user_roles ur
			JOIN frameworks f ON f.organization_id = ur.organization_id
			WHERE ur.user_id = $2 AND ur.organization_id = $1
		)
		SELECT ka.id, ka.slug, ka.title, ka.summary, '', ka.category, ka.tags,
			   ka.framework_codes, ka.control_codes, ka.author, ka.status,
			   ka.view_count, ka.helpful_count, ka.created_at, ka.updated_at
		FROM knowledge_articles ka, user_frameworks uf
		WHERE ka.status = 'published'
		  AND uf.code = ANY(ka.framework_codes)
		  AND ka.id NOT IN (
			  SELECT article_id FROM article_engagements WHERE user_id = $2 AND action = 'view'
		  )
		ORDER BY ka.helpful_count DESC
		LIMIT 10`, orgID, userID)
	if err != nil {
		return nil, fmt.Errorf("search: recommended articles: %w", err)
	}
	defer rows.Close()

	var articles []KnowledgeArticle
	for rows.Next() {
		var a KnowledgeArticle
		if err := rows.Scan(&a.ID, &a.Slug, &a.Title, &a.Summary, &a.Content,
			&a.Category, &a.Tags, &a.FrameworkCodes, &a.ControlCodes,
			&a.Author, &a.Status, &a.ViewCount, &a.HelpfulCount,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			continue
		}
		articles = append(articles, a)
	}
	return articles, nil
}

// ---------------------------------------------------------------------------
// Engagement tracking
// ---------------------------------------------------------------------------

// TrackEngagement records a user interaction with an article.
func (s *SearchService) TrackEngagement(ctx context.Context, articleID, userID, action string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO article_engagements (id, article_id, user_id, action, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, NOW())
		ON CONFLICT (article_id, user_id, action) DO UPDATE SET created_at = NOW()`,
		articleID, userID, action)
	if err != nil {
		return fmt.Errorf("search: track engagement: %w", err)
	}
	if action == "helpful" {
		_, _ = s.pool.Exec(ctx, "UPDATE knowledge_articles SET helpful_count = helpful_count + 1 WHERE id = $1", articleID)
	}
	log.Debug().Str("article_id", articleID).Str("user_id", userID).Str("action", action).Msg("search: engagement tracked")
	return nil
}

// ---------------------------------------------------------------------------
// Bookmarks
// ---------------------------------------------------------------------------

// ManageBookmarks adds, removes, or lists a user's article bookmarks.
func (s *SearchService) ManageBookmarks(ctx context.Context, userID, action, articleID string) ([]Bookmark, error) {
	switch action {
	case "add":
		_, err := s.pool.Exec(ctx, `
			INSERT INTO article_bookmarks (id, user_id, article_id, created_at)
			VALUES (gen_random_uuid(), $1, $2, NOW())
			ON CONFLICT (user_id, article_id) DO NOTHING`, userID, articleID)
		if err != nil {
			return nil, fmt.Errorf("search: add bookmark: %w", err)
		}
	case "remove":
		_, err := s.pool.Exec(ctx, `
			DELETE FROM article_bookmarks WHERE user_id = $1 AND article_id = $2`, userID, articleID)
		if err != nil {
			return nil, fmt.Errorf("search: remove bookmark: %w", err)
		}
	}

	rows, err := s.pool.Query(ctx, `
		SELECT ab.id, ab.user_id, ab.article_id, ka.title, ka.slug, ab.created_at
		FROM article_bookmarks ab
		JOIN knowledge_articles ka ON ka.id = ab.article_id
		WHERE ab.user_id = $1
		ORDER BY ab.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("search: list bookmarks: %w", err)
	}
	defer rows.Close()

	var bookmarks []Bookmark
	for rows.Next() {
		var b Bookmark
		if err := rows.Scan(&b.ID, &b.UserID, &b.ArticleID, &b.Title, &b.Slug, &b.CreatedAt); err != nil {
			continue
		}
		bookmarks = append(bookmarks, b)
	}
	return bookmarks, nil
}
