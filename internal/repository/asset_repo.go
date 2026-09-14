package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
)

var ErrAssetVersionConflict = errors.New("asset version conflict")
var ErrAssetOwnerInvalid = errors.New("asset owner is not an active user in the tenant")

type AssetRepository interface {
	Create(context.Context, string, string, models.AssetCreateInput) (*models.Asset, error)
	GetByID(context.Context, string, string) (*models.Asset, error)
	Update(context.Context, string, string, string, models.AssetPatch) (*models.Asset, error)
	Delete(context.Context, string, string, string, *int64) error
	List(context.Context, string, models.AssetListFilter) ([]models.Asset, int, error)
	Stats(context.Context, string) (*models.AssetStats, error)
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.AssetLifecycleEvent, int, error)
}

type assetRepo struct {
	pool        *pgxpool.Pool
	outbox      queuepkg.OutboxEnqueuer
	outboxQueue string
}

func NewAssetRepository(pool *pgxpool.Pool, outbox queuepkg.OutboxEnqueuer, outboxQueue string) (AssetRepository, error) {
	if pool == nil {
		return nil, errors.New("asset repository database pool is required")
	}
	if outbox == nil {
		return nil, errors.New("asset repository outbox is required")
	}
	outboxQueue = strings.TrimSpace(outboxQueue)
	if outboxQueue == "" {
		return nil, errors.New("asset repository outbox queue is required")
	}
	return &assetRepo{pool: pool, outbox: outbox, outboxQueue: outboxQueue}, nil
}

var _ AssetRepository = (*assetRepo)(nil)

const assetSelectColumns = `
	a.id,a.organization_id,a.asset_ref,a.name,a.asset_type,
	COALESCE(a.category,''),COALESCE(a.description,''),a.criticality,
	a.owner_user_id,COALESCE(a.location,''),a.ip_address::text,
	a.classification,a.processes_personal_data,a.linked_vendor_id,
	a.status,a.tags,a.metadata,a.version,a.created_by,
	a.created_at,a.updated_at,a.deleted_at,
	owner.id,COALESCE(owner.first_name,''),COALESCE(owner.last_name,''),COALESCE(owner.email,'')`

func (r *assetRepo) Create(ctx context.Context, organizationID, actorID string, input models.AssetCreateInput) (*models.Asset, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var created *models.Asset
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if input.OwnerUserID != nil {
			if err := ensureAssetOwner(ctx, tx, organizationID, *input.OwnerUserID); err != nil {
				return err
			}
		}
		var sequence int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO asset_reference_sequences (organization_id,next_value)
			VALUES ($1::uuid,2)
			ON CONFLICT (organization_id)
			DO UPDATE SET next_value=asset_reference_sequences.next_value+1
			RETURNING next_value-1`, organizationID).Scan(&sequence); err != nil {
			return fmt.Errorf("allocate asset reference: %w", err)
		}
		assetID := ""
		if err := tx.QueryRow(ctx, `
			INSERT INTO assets (
				organization_id,asset_ref,name,asset_type,category,description,
				criticality,owner_user_id,location,ip_address,classification,
				processes_personal_data,linked_vendor_id,tags,metadata,created_by
			) VALUES (
				$1::uuid,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,
				$8::uuid,NULLIF($9,''),NULLIF($10,'')::inet,$11,$12,
				$13::uuid,$14,COALESCE($15::jsonb,'{}'::jsonb),$16::uuid
			)
			RETURNING id`,
			organizationID, fmt.Sprintf("AST-%06d", sequence), input.Name, input.AssetType,
			input.Category, input.Description, input.Criticality, input.OwnerUserID,
			input.Location, input.IPAddress, input.Classification, input.ProcessesPersonalData,
			input.LinkedVendorID, input.Tags, nullableJSON(input.Metadata), actorID,
		).Scan(&assetID); err != nil {
			return fmt.Errorf("insert asset: %w", err)
		}
		var err error
		created, err = getAssetWithQuerier(ctx, tx, organizationID, assetID)
		if err != nil {
			return err
		}
		return r.recordAssetEvent(ctx, tx, created, actorID, "created", map[string]any{"version": created.Version})
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (r *assetRepo) GetByID(ctx context.Context, organizationID, id string) (*models.Asset, error) {
	return getAssetWithQuerier(ctx, database.QuerierFromContext(ctx, r.pool), organizationID, id)
}

func getAssetWithQuerier(ctx context.Context, querier database.Querier, organizationID, id string) (*models.Asset, error) {
	return scanAsset(querier.QueryRow(ctx, `
		SELECT `+assetSelectColumns+`
		FROM assets a
		LEFT JOIN users owner
		  ON owner.id=a.owner_user_id
		 AND owner.organization_id=a.organization_id
		 AND owner.deleted_at IS NULL
		WHERE a.organization_id=$1::uuid AND a.id=$2::uuid AND a.deleted_at IS NULL`,
		organizationID, id))
}

func (r *assetRepo) Update(ctx context.Context, organizationID, id, actorID string, patch models.AssetPatch) (*models.Asset, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var updated *models.Asset
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if patch.OwnerUserID != nil && !patch.ClearOwner {
			if err := ensureAssetOwner(ctx, tx, organizationID, *patch.OwnerUserID); err != nil {
				return err
			}
		}
		var previousStatus models.AssetStatus
		var previousVersion int64
		if err := tx.QueryRow(ctx, `
			SELECT status,version FROM assets
			WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL
			FOR UPDATE`, organizationID, id).Scan(&previousStatus, &previousVersion); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return pgx.ErrNoRows
			}
			return fmt.Errorf("lock asset for update: %w", err)
		}
		if patch.ExpectedVersion != nil && previousVersion != *patch.ExpectedVersion {
			return ErrAssetVersionConflict
		}
		var nextVersion int64
		err := tx.QueryRow(ctx, `
			UPDATE assets
			SET name=COALESCE($3,name),
			    asset_type=COALESCE($4,asset_type),
			    category=CASE WHEN $5::text IS NULL THEN category ELSE NULLIF($5,'') END,
			    description=CASE WHEN $6::text IS NULL THEN description ELSE NULLIF($6,'') END,
			    criticality=COALESCE($7,criticality),
			    owner_user_id=CASE
			        WHEN $9 THEN NULL
			        WHEN $8::text IS NOT NULL THEN $8::uuid
			        ELSE owner_user_id END,
			    location=CASE WHEN $10::text IS NULL THEN location ELSE NULLIF($10,'') END,
			    ip_address=CASE
			        WHEN $12 THEN NULL
			        WHEN $11::text IS NOT NULL THEN NULLIF($11,'')::inet
			        ELSE ip_address END,
			    classification=COALESCE($13,classification),
			    processes_personal_data=COALESCE($14,processes_personal_data),
			    linked_vendor_id=CASE
			        WHEN $16 THEN NULL
			        WHEN $15::text IS NOT NULL THEN $15::uuid
			        ELSE linked_vendor_id END,
			    status=COALESCE($17,status),
			    tags=COALESCE($18,tags),
			    metadata=COALESCE($19::jsonb,metadata),
			    version=version+1
			WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL
			  AND ($20::bigint IS NULL OR version=$20)
			RETURNING version`,
			organizationID, id, patch.Name, patch.AssetType, patch.Category, patch.Description,
			patch.Criticality, patch.OwnerUserID, patch.ClearOwner, patch.Location,
			patch.IPAddress, patch.ClearIPAddress, patch.Classification,
			patch.ProcessesPersonalData, patch.LinkedVendorID, patch.ClearLinkedVendor,
			patch.Status, patch.Tags, nullableJSON(patch.Metadata), patch.ExpectedVersion,
		).Scan(&nextVersion)
		if err != nil {
			return classifyAssetMutationFailure(ctx, tx, organizationID, id, patch.ExpectedVersion, err)
		}
		var loadErr error
		updated, loadErr = getAssetWithQuerier(ctx, tx, organizationID, id)
		if loadErr != nil {
			return loadErr
		}
		eventType := "updated"
		if patch.Status != nil {
			eventType = "status_changed"
			if *patch.Status == models.AssetStatusDecommissioned {
				eventType = "decommissioned"
			}
		}
		details := map[string]any{"version": nextVersion}
		if patch.Status != nil {
			details["from_status"] = previousStatus
			details["to_status"] = updated.Status
		}
		return r.recordAssetEvent(ctx, tx, updated, actorID, eventType, details)
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (r *assetRepo) Delete(ctx context.Context, organizationID, id, actorID string, expectedVersion *int64) error {
	querier := database.QuerierFromContext(ctx, r.pool)
	return withTransaction(ctx, querier, func(tx pgx.Tx) error {
		deleted := &models.Asset{TenantModel: models.TenantModel{OrganizationID: organizationID}}
		deleted.ID = id
		err := tx.QueryRow(ctx, `
			UPDATE assets
			SET deleted_at=NOW(),status='decommissioned',version=version+1
			WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL
			  AND ($3::bigint IS NULL OR version=$3)
			RETURNING asset_ref,name,asset_type,criticality,classification,
			          processes_personal_data,status,version`, organizationID, id, expectedVersion).Scan(
			&deleted.AssetRef, &deleted.Name, &deleted.AssetType, &deleted.Criticality,
			&deleted.Classification, &deleted.ProcessesPersonalData, &deleted.Status, &deleted.Version,
		)
		if err != nil {
			return classifyAssetMutationFailure(ctx, tx, organizationID, id, expectedVersion, err)
		}
		return r.recordAssetEvent(ctx, tx, deleted, actorID, "deleted", map[string]any{"version": deleted.Version})
	})
}

func (r *assetRepo) List(ctx context.Context, organizationID string, filter models.AssetListFilter) ([]models.Asset, int, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	args := []any{
		organizationID, filter.AssetType, filter.Criticality, filter.Classification,
		filter.Status, filter.OwnerUserID, filter.ProcessesPersonalData, filter.Search,
		filter.Tag,
	}
	predicate := `
		FROM assets a
		WHERE a.organization_id=$1::uuid AND a.deleted_at IS NULL
		  AND ($2='' OR a.asset_type=$2)
		  AND ($3='' OR a.criticality=$3)
		  AND ($4='' OR a.classification=$4)
		  AND ($5='' OR a.status=$5)
		  AND ($6='' OR a.owner_user_id=$6::uuid)
		  AND ($7::boolean IS NULL OR a.processes_personal_data=$7)
		  AND ($8='' OR a.search_vector @@ websearch_to_tsquery('simple',$8))
		  AND ($9='' OR $9=ANY(a.tags))`
	var total int
	if err := querier.QueryRow(ctx, `SELECT COUNT(*)`+predicate, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count assets: %w", err)
	}

	sortColumn := assetSortColumn(filter.SortBy)
	direction := "DESC"
	if strings.EqualFold(filter.SortDirection, "asc") {
		direction = "ASC"
	}
	query := `SELECT ` + assetSelectColumns + `
		FROM assets a
		LEFT JOIN users owner
		  ON owner.id=a.owner_user_id
		 AND owner.organization_id=a.organization_id
		 AND owner.deleted_at IS NULL` +
		strings.Replace(predicate, "FROM assets a", "", 1) +
		` ORDER BY ` + sortColumn + ` ` + direction + `,a.id ` + direction + `
		LIMIT $10 OFFSET $11`
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := querier.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list assets: %w", err)
	}
	defer rows.Close()
	items := make([]models.Asset, 0, filter.PageSize)
	for rows.Next() {
		item, err := scanAsset(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate assets: %w", err)
	}
	return items, total, nil
}

func (r *assetRepo) Stats(ctx context.Context, organizationID string) (*models.AssetStats, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	stats := &models.AssetStats{ByType: make(map[string]int)}
	if err := querier.QueryRow(ctx, `
		SELECT COUNT(*)::int,
		       COUNT(*) FILTER (WHERE criticality='critical')::int,
		       COUNT(*) FILTER (WHERE processes_personal_data)::int,
		       COUNT(*) FILTER (WHERE status='active')::int
		FROM assets WHERE organization_id=$1::uuid AND deleted_at IS NULL`,
		organizationID).Scan(&stats.Total, &stats.Critical, &stats.PersonalData, &stats.Active); err != nil {
		return nil, fmt.Errorf("aggregate asset statistics: %w", err)
	}
	rows, err := querier.Query(ctx, `
		SELECT asset_type,COUNT(*)::int
		FROM assets
		WHERE organization_id=$1::uuid AND deleted_at IS NULL
		GROUP BY asset_type ORDER BY asset_type`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("aggregate asset types: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var assetType string
		var count int
		if err := rows.Scan(&assetType, &count); err != nil {
			return nil, fmt.Errorf("scan asset type statistic: %w", err)
		}
		stats.ByType[assetType] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate asset type statistics: %w", err)
	}
	return stats, nil
}

func (r *assetRepo) ListEvents(ctx context.Context, organizationID, assetID string, pagination models.PaginationRequest) ([]models.AssetLifecycleEvent, int, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var total int
	if err := querier.QueryRow(ctx, `
		SELECT COUNT(*) FROM asset_events
		WHERE organization_id=$1::uuid AND asset_id=$2::uuid`,
		organizationID, assetID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count asset events: %w", err)
	}
	rows, err := querier.Query(ctx, `
		SELECT id,asset_id,event_type,actor_user_id,asset_version,details,created_at
		FROM asset_events
		WHERE organization_id=$1::uuid AND asset_id=$2::uuid
		ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4`,
		organizationID, assetID, pagination.PageSize, (pagination.Page-1)*pagination.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list asset events: %w", err)
	}
	defer rows.Close()
	events := make([]models.AssetLifecycleEvent, 0, pagination.PageSize)
	for rows.Next() {
		var event models.AssetLifecycleEvent
		if err := rows.Scan(&event.ID, &event.AssetID, &event.EventType, &event.ActorID, &event.Version, &event.Details, &event.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan asset event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate asset events: %w", err)
	}
	return events, total, nil
}

type assetScanner interface{ Scan(...any) error }

func scanAsset(row assetScanner) (*models.Asset, error) {
	var item models.Asset
	var ownerID *string
	var ownerFirst, ownerLast, ownerEmail string
	if err := row.Scan(
		&item.ID, &item.OrganizationID, &item.AssetRef, &item.Name, &item.AssetType,
		&item.Category, &item.Description, &item.Criticality, &item.OwnerUserID,
		&item.Location, &item.IPAddress, &item.Classification,
		&item.ProcessesPersonalData, &item.LinkedVendorID, &item.Status,
		&item.Tags, &item.Metadata, &item.Version, &item.CreatedBy,
		&item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
		&ownerID, &ownerFirst, &ownerLast, &ownerEmail,
	); err != nil {
		return nil, err
	}
	if ownerID != nil {
		item.Owner = &models.AssetPerson{ID: *ownerID, FirstName: ownerFirst, LastName: ownerLast, Email: ownerEmail}
	}
	return &item, nil
}

func (r *assetRepo) recordAssetEvent(ctx context.Context, tx pgx.Tx, item *models.Asset, actorID, eventType string, details map[string]any) error {
	encoded, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode asset event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO asset_events
			(organization_id,asset_id,event_type,actor_user_id,asset_version,details)
		VALUES ($1::uuid,$2::uuid,$3,$4::uuid,$5,$6::jsonb)`,
		item.OrganizationID, item.ID, eventType, actorID, item.Version, encoded); err != nil {
		return fmt.Errorf("append asset event: %w", err)
	}
	payload := map[string]any{
		"type": eventTypeToAssetNotification(eventType), "severity": item.Criticality,
		"org_id": item.OrganizationID, "entity_type": "asset", "entity_id": item.ID,
		"entity_ref": item.AssetRef, "data": map[string]any{
			"asset_id": item.ID, "asset_ref": item.AssetRef, "name": item.Name,
			"asset_type": item.AssetType, "criticality": item.Criticality,
			"classification": item.Classification, "processes_personal_data": item.ProcessesPersonalData,
			"status": item.Status, "version": item.Version,
		}, "timestamp": time.Now().UTC(),
	}
	envelope, err := queuepkg.NewEnvelope("notification.event", item.OrganizationID, payload)
	if err != nil {
		return fmt.Errorf("create asset outbox envelope: %w", err)
	}
	envelope.CausationID = item.ID
	envelope.Metadata = map[string]string{"entity_type": "asset", "entity_id": item.ID, "event_type": eventType}
	if err := r.outbox.Enqueue(ctx, tx, r.outboxQueue, envelope); err != nil {
		return fmt.Errorf("enqueue asset event: %w", err)
	}
	return nil
}

func eventTypeToAssetNotification(eventType string) string { return "asset." + eventType }

func classifyAssetMutationFailure(ctx context.Context, querier database.Querier, organizationID, id string, expectedVersion *int64, cause error) error {
	if !errors.Is(cause, pgx.ErrNoRows) {
		return cause
	}
	var exists bool
	if err := querier.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM assets
		WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL)`,
		organizationID, id).Scan(&exists); err != nil {
		return err
	}
	if exists && expectedVersion != nil {
		return ErrAssetVersionConflict
	}
	return pgx.ErrNoRows
}

func assetSortColumn(value string) string {
	switch value {
	case "name":
		return "a.name"
	case "asset_ref":
		return "a.asset_ref"
	case "asset_type":
		return "a.asset_type"
	case "criticality":
		return "a.criticality"
	case "created_at":
		return "a.created_at"
	default:
		return "a.updated_at"
	}
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func ensureAssetOwner(ctx context.Context, querier database.Querier, organizationID, userID string) error {
	var valid bool
	if err := querier.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM users
			WHERE organization_id=$1::uuid AND id=$2::uuid
			  AND status='active' AND deleted_at IS NULL
		)`, organizationID, userID).Scan(&valid); err != nil {
		return fmt.Errorf("validate asset owner: %w", err)
	}
	if !valid {
		return ErrAssetOwnerInvalid
	}
	return nil
}
