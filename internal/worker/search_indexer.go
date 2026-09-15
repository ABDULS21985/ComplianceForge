package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/database"
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
	{
		Table: `(SELECT implementation.id, implementation.organization_id,
			control.title, COALESCE(implementation.implementation_description, control.description) AS description,
			implementation.deleted_at
			FROM control_implementations AS implementation
			JOIN framework_controls AS control ON control.id = implementation.framework_control_id) AS searchable_controls`,
		IDColumn: "id", TypeName: "control", TitleCol: "title", ContentCol: "description",
	},
	{
		Table: `(SELECT policy.id, policy.organization_id, policy.title,
			COALESCE(version.content_text, version.summary, '') AS body,
			policy.deleted_at
			FROM policies AS policy
			LEFT JOIN policy_versions AS version
			  ON version.organization_id = policy.organization_id
			 AND version.id = policy.current_version_id) AS searchable_policies`,
		IDColumn: "id", TypeName: "policy", TitleCol: "title", ContentCol: "body",
	},
	{Table: "incidents", IDColumn: "id", TypeName: "incident", TitleCol: "title", ContentCol: "description"},
	{Table: "audit_findings", IDColumn: "id", TypeName: "finding", TitleCol: "title", ContentCol: "description"},
	{Table: "control_evidence", IDColumn: "id", TypeName: "evidence", TitleCol: "title", ContentCol: "description"},
	{Table: "assets", IDColumn: "id", TypeName: "asset", TitleCol: "name", ContentCol: "description"},
	{Table: "vendors", IDColumn: "id", TypeName: "vendor", TitleCol: "name", ContentCol: "description"},
}

// searchIndexUpsertSQL intentionally names the tenant key in the conflict
// target. Entity identifiers are only unique within an organization, and the
// search_index schema stores searchable text in body (not the legacy content
// column). The table trigger derives search_vector from these source fields.
const searchIndexUpsertSQL = `
	INSERT INTO search_index (
		entity_type, entity_id, organization_id, title, body, created_date, updated_date
	)
	VALUES ($1, $2, $3, $4, $5, CURRENT_DATE, CURRENT_DATE)
	ON CONFLICT (organization_id, entity_type, entity_id)
	DO UPDATE SET title = EXCLUDED.title,
	              body = EXCLUDED.body,
	              updated_date = CURRENT_DATE
`

// NightlyReindex runs at 03:00 UTC and performs a full reindex of all entities
// for each active organization.
func (si *SearchIndexer) NightlyReindex(ctx context.Context) error {
	now := time.Now().UTC()
	if now.Hour() != 3 {
		return nil
	}

	log.Info().Msg("search_indexer: starting nightly reindex")

	var totalIndexed int
	err := runForScheduledTenants(ctx, si.pool, "search reindex", func(tenantCtx context.Context, orgID string) error {
		count, err := si.indexAllEntities(tenantCtx, orgID)
		if err != nil {
			return err
		}
		totalIndexed += count
		return nil
	})
	if err != nil {
		return err
	}

	log.Info().
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

	return database.WithTenantConnection(ctx, si.pool, orgID, func(tenantCtx context.Context) error {
		return si.incrementalIndexForTenant(tenantCtx, entityType, entityID, orgID, action)
	})
}

func (si *SearchIndexer) incrementalIndexForTenant(ctx context.Context, entityType, entityID, orgID, action string) error {
	querier := database.QuerierFromContext(ctx, si.pool)
	if action == "delete" {
		return si.removeFromIndex(ctx, querier, entityType, entityID, orgID)
	}

	entity := si.findEntityDef(entityType)
	if entity == nil {
		return fmt.Errorf("unknown entity type: %s", entityType)
	}

	query := fmt.Sprintf(`
		SELECT %s, COALESCE(%s,''), COALESCE(%s,''), organization_id
		FROM %s
		WHERE %s = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, entity.IDColumn, entity.TitleCol, entity.ContentCol, entity.Table, entity.IDColumn)

	var id, title, content, entityOrgID string
	err := querier.QueryRow(ctx, query, entityID, orgID).Scan(&id, &title, &content, &entityOrgID)
	if err != nil {
		return fmt.Errorf("fetching entity %s/%s: %w", entityType, entityID, err)
	}

	_, err = querier.Exec(ctx, searchIndexUpsertSQL, entityType, entityID, entityOrgID, title, content)

	return err
}

// HealthCheck compares entity counts between source tables and the search_index
// to detect indexing drift.
func (si *SearchIndexer) HealthCheck(ctx context.Context) error {
	log.Info().Msg("search_indexer: running health check")
	return runForScheduledTenants(ctx, si.pool, "search health", si.healthCheckForTenant)
}

func (si *SearchIndexer) healthCheckForTenant(ctx context.Context, organizationID string) error {
	querier := database.QuerierFromContext(ctx, si.pool)
	var issues int
	var checkErrors []error
	for _, entity := range indexableEntities {
		var sourceCount int
		err := querier.QueryRow(ctx, fmt.Sprintf(
			`SELECT COUNT(*) FROM %s WHERE organization_id=$1::uuid AND deleted_at IS NULL`, entity.Table,
		), organizationID).Scan(&sourceCount)
		if err != nil {
			log.Error().Err(err).Str("table", entity.Table).Msg("search_indexer: counting source")
			checkErrors = append(checkErrors, fmt.Errorf("count %s source: %w", entity.TypeName, err))
			continue
		}

		var indexCount int
		err = querier.QueryRow(ctx, `
			SELECT COUNT(*) FROM search_index WHERE organization_id=$1::uuid AND entity_type=$2
		`, organizationID, entity.TypeName).Scan(&indexCount)
		if err != nil {
			log.Error().Err(err).Str("type", entity.TypeName).Msg("search_indexer: counting index")
			checkErrors = append(checkErrors, fmt.Errorf("count %s index: %w", entity.TypeName, err))
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
	return errors.Join(checkErrors...)
}

func (si *SearchIndexer) indexAllEntities(ctx context.Context, orgID string) (int, error) {
	querier := database.QuerierFromContext(ctx, si.pool)
	var totalIndexed int
	var indexErrors []error

	for _, entity := range indexableEntities {
		query := fmt.Sprintf(`
			SELECT %s, COALESCE(%s,''), COALESCE(%s,'')
			FROM %s
			WHERE organization_id = $1 AND deleted_at IS NULL
		`, entity.IDColumn, entity.TitleCol, entity.ContentCol, entity.Table)

		rows, err := querier.Query(ctx, query, orgID)
		if err != nil {
			log.Error().Err(err).Str("table", entity.Table).Msg("search_indexer: querying entities")
			indexErrors = append(indexErrors, fmt.Errorf("query %s entities: %w", entity.TypeName, err))
			continue
		}

		type sourceEntity struct{ id, title, content string }
		entities := make([]sourceEntity, 0)
		for rows.Next() {
			var source sourceEntity
			if err := rows.Scan(&source.id, &source.title, &source.content); err != nil {
				indexErrors = append(indexErrors, fmt.Errorf("scan %s entity: %w", entity.TypeName, err))
				continue
			}
			entities = append(entities, source)
		}
		if err := rows.Err(); err != nil {
			indexErrors = append(indexErrors, fmt.Errorf("iterate %s entities: %w", entity.TypeName, err))
		}
		rows.Close()

		for _, source := range entities {
			_, err = querier.Exec(ctx, searchIndexUpsertSQL,
				entity.TypeName, source.id, orgID, source.title, source.content)
			if err != nil {
				log.Error().Err(err).Str("entity_id", source.id).Msg("search_indexer: upsert index")
				indexErrors = append(indexErrors, fmt.Errorf("upsert %s entity %s: %w", entity.TypeName, source.id, err))
				continue
			}
			totalIndexed++
		}
	}

	return totalIndexed, errors.Join(indexErrors...)
}

func (si *SearchIndexer) removeFromIndex(ctx context.Context, querier database.Querier, entityType, entityID, orgID string) error {
	_, err := querier.Exec(ctx, `
		DELETE FROM search_index
		WHERE entity_type = $1 AND entity_id = $2 AND organization_id = $3
	`, entityType, entityID, orgID)
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
