package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// SearchIndexer handles background search indexing tasks including nightly
// full reindexing, incremental indexing from the event bus, and health checks.
type SearchIndexer struct {
	pool *pgxpool.Pool
}

func NewSearchIndexer(pool *pgxpool.Pool) *SearchIndexer {
	return &SearchIndexer{pool: pool}
}

// indexableEntity defines a source table and the columns used to build the
// search index entry.
type indexableEntity struct {
	Table      string
	IDColumn   string
	TypeName   string
	TitleCol   string
	ContentCol string
}

var indexableEntities = []indexableEntity{
	{Table: "risks", IDColumn: "id", TypeName: "risk", TitleCol: "title", ContentCol: "description"},
	{Table: "controls", IDColumn: "id", TypeName: "control", TitleCol: "title", ContentCol: "description"},
	{Table: "policies", IDColumn: "id", TypeName: "policy", TitleCol: "title", ContentCol: "content"},
	{Table: "incidents", IDColumn: "id", TypeName: "incident", TitleCol: "title", ContentCol: "description"},
	{Table: "audit_findings", IDColumn: "id", TypeName: "finding", TitleCol: "title", ContentCol: "description"},
	{Table: "evidence_items", IDColumn: "id", TypeName: "evidence", TitleCol: "title", ContentCol: "description"},
	{Table: "assets", IDColumn: "id", TypeName: "asset", TitleCol: "name", ContentCol: "description"},
	{Table: "vendors", IDColumn: "id", TypeName: "vendor", TitleCol: "name", ContentCol: "description"},
}

// NightlyReindex runs at 03:00 UTC and performs a full reindex of all entities
// for each active organization.
func (si *SearchIndexer) NightlyReindex(ctx context.Context) error {
	now := time.Now().UTC()
	if now.Hour() != 3 {
		return nil
	}

	log.Info().Msg("search_indexer: starting nightly reindex")

	orgIDs, err := si.getActiveOrganizations(ctx)
	if err != nil {
		return err
	}

	var totalIndexed int
	for _, orgID := range orgIDs {
		count, err := si.indexAllEntities(ctx, orgID)
		if err != nil {
			log.Error().Err(err).Str("org_id", orgID).Msg("search_indexer: reindex failed for org")
			continue
		}
		totalIndexed += count
	}

	log.Info().
		Int("org_count", len(orgIDs)).
		Int("total_indexed", totalIndexed).
		Msg("search_indexer: nightly reindex complete")
	return nil
}

// IncrementalIndex processes a single entity change event from the event bus
// and updates the search index accordingly.
func (si *SearchIndexer) IncrementalIndex(ctx context.Context, entityType, entityID, orgID, action string) error {
	log.Debug().
		Str("entity_type", entityType).
		Str("entity_id", entityID).
		Str("action", action).
		Msg("search_indexer: incremental index")

	if action == "delete" {
		return si.removeFromIndex(ctx, entityType, entityID)
	}

	entity := si.findEntityDef(entityType)
	if entity == nil {
		return fmt.Errorf("unknown entity type: %s", entityType)
	}

	query := fmt.Sprintf(`
		SELECT %s, %s, %s, organization_id
		FROM %s
		WHERE %s = $1 AND deleted_at IS NULL
	`, entity.IDColumn, entity.TitleCol, entity.ContentCol, entity.Table, entity.IDColumn)

	var id, title, content, entityOrgID string
	err := si.pool.QueryRow(ctx, query, entityID).Scan(&id, &title, &content, &entityOrgID)
	if err != nil {
		return fmt.Errorf("fetching entity %s/%s: %w", entityType, entityID, err)
	}

	_, err = si.pool.Exec(ctx, `
		INSERT INTO search_index (entity_type, entity_id, organization_id, title, content, search_vector, indexed_at)
		VALUES ($1, $2, $3, $4, $5, to_tsvector('english', $4 || ' ' || $5), NOW())
		ON CONFLICT (entity_type, entity_id)
		DO UPDATE SET title = $4, content = $5,
		             search_vector = to_tsvector('english', $4 || ' ' || $5),
		             indexed_at = NOW()
	`, entityType, entityID, entityOrgID, title, content)

	return err
}

// HealthCheck compares entity counts between source tables and the search_index
// to detect indexing drift.
func (si *SearchIndexer) HealthCheck(ctx context.Context) error {
	log.Info().Msg("search_indexer: running health check")

	var issues int
	for _, entity := range indexableEntities {
		var sourceCount int
		err := si.pool.QueryRow(ctx, fmt.Sprintf(
			`SELECT COUNT(*) FROM %s WHERE deleted_at IS NULL`, entity.Table,
		)).Scan(&sourceCount)
		if err != nil {
			log.Error().Err(err).Str("table", entity.Table).Msg("search_indexer: counting source")
			continue
		}

		var indexCount int
		err = si.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM search_index WHERE entity_type = $1
		`, entity.TypeName).Scan(&indexCount)
		if err != nil {
			log.Error().Err(err).Str("type", entity.TypeName).Msg("search_indexer: counting index")
			continue
		}

		if sourceCount != indexCount {
			issues++
			log.Warn().
				Str("entity_type", entity.TypeName).
				Int("source_count", sourceCount).
				Int("index_count", indexCount).
				Int("drift", sourceCount-indexCount).
				Msg("search_indexer: index drift detected")
		}
	}

	if issues == 0 {
		log.Info().Msg("search_indexer: health check passed, no drift detected")
	} else {
		log.Warn().Int("issues", issues).Msg("search_indexer: health check found drift")
	}
	return nil
}

func (si *SearchIndexer) getActiveOrganizations(ctx context.Context) ([]string, error) {
	rows, err := si.pool.Query(ctx, `
		SELECT id FROM organizations WHERE status = 'active' AND deleted_at IS NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("querying organizations: %w", err)
	}
	defer rows.Close()

	var orgIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		orgIDs = append(orgIDs, id)
	}
	return orgIDs, nil
}

func (si *SearchIndexer) indexAllEntities(ctx context.Context, orgID string) (int, error) {
	var totalIndexed int

	for _, entity := range indexableEntities {
		query := fmt.Sprintf(`
			SELECT %s, %s, %s
			FROM %s
			WHERE organization_id = $1 AND deleted_at IS NULL
		`, entity.IDColumn, entity.TitleCol, entity.ContentCol, entity.Table)

		rows, err := si.pool.Query(ctx, query, orgID)
		if err != nil {
			log.Error().Err(err).Str("table", entity.Table).Msg("search_indexer: querying entities")
			continue
		}

		for rows.Next() {
			var id, title, content string
			if err := rows.Scan(&id, &title, &content); err != nil {
				continue
			}

			_, err = si.pool.Exec(ctx, `
				INSERT INTO search_index (entity_type, entity_id, organization_id, title, content, search_vector, indexed_at)
				VALUES ($1, $2, $3, $4, $5, to_tsvector('english', $4 || ' ' || $5), NOW())
				ON CONFLICT (entity_type, entity_id)
				DO UPDATE SET title = $4, content = $5,
				             search_vector = to_tsvector('english', $4 || ' ' || $5),
				             indexed_at = NOW()
			`, entity.TypeName, id, orgID, title, content)
			if err != nil {
				log.Error().Err(err).Str("entity_id", id).Msg("search_indexer: upsert index")
				continue
			}
			totalIndexed++
		}
		rows.Close()
	}

	return totalIndexed, nil
}

func (si *SearchIndexer) removeFromIndex(ctx context.Context, entityType, entityID string) error {
	_, err := si.pool.Exec(ctx, `
		DELETE FROM search_index WHERE entity_type = $1 AND entity_id = $2
	`, entityType, entityID)
	return err
}

func (si *SearchIndexer) findEntityDef(entityType string) *indexableEntity {
	for _, e := range indexableEntities {
		if e.TypeName == entityType {
			return &e
		}
	}
	return nil
}
