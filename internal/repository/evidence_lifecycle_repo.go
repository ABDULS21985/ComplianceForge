package repository

import (
	"context"
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

type EvidenceLifecycleRepository interface {
	GetEvidenceLifecycle(context.Context, string, string, string) (*models.EvidenceLifecycleRecord, error)
	GetEvidenceIntegrity(context.Context, string, string, string) (*models.EvidenceIntegrityTarget, error)
	AppendEvidenceCustodyEvent(context.Context, string, string, string, models.EvidenceCustodyEventInput) (*models.EvidenceCustodyEvent, error)
	DueEvidenceTenants(context.Context, int) ([]string, error)
	ExpireDueEvidence(context.Context, string, int) ([]models.ExpiredEvidenceNotice, error)
}

type evidenceLifecycleRepo struct {
	pool        *pgxpool.Pool
	outbox      queuepkg.OutboxEnqueuer
	outboxQueue string
}

var ErrEvidenceLifecycleHistoryLimitExceeded = errors.New("evidence lifecycle history exceeds the bounded response limit")

type EvidenceLifecycleRepositoryOption func(*evidenceLifecycleRepo) error

func WithEvidenceLifecycleOutbox(outbox queuepkg.OutboxEnqueuer, queueName string) EvidenceLifecycleRepositoryOption {
	return func(repository *evidenceLifecycleRepo) error {
		if outbox == nil {
			return errors.New("evidence lifecycle outbox is required")
		}
		queueName = strings.TrimSpace(queueName)
		if queueName == "" {
			return errors.New("evidence lifecycle outbox queue is required")
		}
		repository.outbox = outbox
		repository.outboxQueue = queueName
		return nil
	}
}

const (
	maxEvidenceLifecycleBatchSize   = 1000
	maxEvidenceLifecycleHistoryRows = 1000
)

func NewEvidenceLifecycleRepository(pool *pgxpool.Pool, options ...EvidenceLifecycleRepositoryOption) (EvidenceLifecycleRepository, error) {
	if pool == nil {
		return nil, errors.New("evidence lifecycle database pool is required")
	}
	repository := &evidenceLifecycleRepo{pool: pool}
	for _, option := range options {
		if option == nil {
			return nil, errors.New("evidence lifecycle repository option is required")
		}
		if err := option(repository); err != nil {
			return nil, err
		}
	}
	return repository, nil
}

var _ EvidenceLifecycleRepository = (*evidenceLifecycleRepo)(nil)

const evidenceLifecycleColumns = `ce.id,ce.organization_id,ce.control_implementation_id,
	ce.title,ce.description,ce.evidence_type,ce.file_path,ce.file_name,ce.file_size_bytes,
	ce.mime_type,ce.file_hash,ce.collection_method,ce.collected_at,ce.collected_by,
	ce.valid_from,ce.valid_until,ce.is_current,ce.review_status,ce.reviewed_by,
	ce.reviewed_at,ce.review_notes,ce.metadata,ce.created_at,ce.updated_at,ce.deleted_at,
	ce.series_id,ce.version_number,ce.supersedes_evidence_id,ce.superseded_by_evidence_id,
	ce.superseded_at,ce.lifecycle_status,ce.expires_at,ce.version_reason,ce.content_fingerprint`

func (r *evidenceLifecycleRepo) GetEvidenceLifecycle(
	ctx context.Context, organizationID, controlID, evidenceID string,
) (*models.EvidenceLifecycleRecord, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.EvidenceLifecycleRecord
	err := withEvidenceReadSnapshot(ctx, querier, func(tx pgx.Tx) error {
		evidence, err := getLifecycleEvidence(ctx, tx, organizationID, controlID, evidenceID)
		if err != nil {
			return err
		}
		if evidence.SeriesID == nil {
			return errors.New("evidence lifecycle series is missing")
		}
		versions, versionsTruncated, err := listEvidenceVersions(ctx, tx, organizationID, controlID, *evidence.SeriesID)
		if err != nil {
			return err
		}
		if versionsTruncated {
			return fmt.Errorf("%w: versions", ErrEvidenceLifecycleHistoryLimitExceeded)
		}
		reviews, reviewsTruncated, err := listEvidenceReviews(ctx, tx, organizationID, evidenceID)
		if err != nil {
			return err
		}
		if reviewsTruncated {
			return fmt.Errorf("%w: reviews", ErrEvidenceLifecycleHistoryLimitExceeded)
		}
		events, eventsTruncated, err := listEvidenceCustodyEvents(ctx, tx, organizationID, evidenceID)
		if err != nil {
			return err
		}
		if eventsTruncated {
			return fmt.Errorf("%w: custody events", ErrEvidenceLifecycleHistoryLimitExceeded)
		}
		chain, err := verifyEvidenceCustodyChain(ctx, tx, organizationID, evidenceID)
		if err != nil {
			return err
		}
		var legalHoldActive bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM legal_hold_records held
			JOIN legal_holds hold ON hold.organization_id=held.organization_id AND hold.id=held.hold_id
			WHERE held.organization_id=$1::uuid AND held.record_type='evidence'
			  AND held.record_id=$2::uuid AND held.released_at IS NULL AND hold.status='active'
		)`, organizationID, evidenceID).Scan(&legalHoldActive); err != nil {
			return fmt.Errorf("check evidence legal hold: %w", err)
		}
		result = &models.EvidenceLifecycleRecord{
			Evidence: *evidence, Versions: versions, Reviews: reviews,
			CustodyEvents: events,
			Chain:         *chain, LegalHoldActive: legalHoldActive,
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get evidence lifecycle: %w", err)
	}
	return result, nil
}

func (r *evidenceLifecycleRepo) GetEvidenceIntegrity(
	ctx context.Context, organizationID, controlID, evidenceID string,
) (*models.EvidenceIntegrityTarget, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.EvidenceIntegrityTarget
	err := withEvidenceReadSnapshot(ctx, querier, func(tx pgx.Tx) error {
		target := &models.EvidenceIntegrityTarget{
			EvidenceID: evidenceID, OrganizationID: organizationID,
		}
		if err := tx.QueryRow(ctx, `SELECT ce.file_path,ce.file_hash,ce.file_size_bytes
			FROM control_evidence ce
			JOIN control_implementations ci
			  ON ci.organization_id=ce.organization_id
			 AND ci.id=ce.control_implementation_id
			WHERE ce.organization_id=$1::uuid AND ci.framework_control_id=$2::uuid
			  AND ce.id=$3::uuid AND ce.deleted_at IS NULL AND ci.deleted_at IS NULL`,
			organizationID, controlID, evidenceID).Scan(
			&target.ObjectKey, &target.SHA256, &target.SizeBytes,
		); err != nil {
			return err
		}
		chain, err := verifyEvidenceCustodyChain(ctx, tx, organizationID, evidenceID)
		if err != nil {
			return err
		}
		target.Chain = *chain
		result = target
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get evidence integrity target: %w", err)
	}
	return result, nil
}

func getLifecycleEvidence(ctx context.Context, querier database.Querier, organizationID, controlID, evidenceID string) (*models.ControlEvidence, error) {
	return scanLifecycleEvidence(querier.QueryRow(ctx, `SELECT `+evidenceLifecycleColumns+`
		FROM control_evidence ce
		JOIN control_implementations ci ON ci.id=ce.control_implementation_id
		WHERE ce.organization_id=$1::uuid AND ci.organization_id=$1::uuid
		  AND ci.framework_control_id=$2::uuid AND ce.id=$3::uuid
		  AND ce.deleted_at IS NULL AND ci.deleted_at IS NULL`, organizationID, controlID, evidenceID))
}

func listEvidenceVersions(ctx context.Context, querier database.Querier, organizationID, controlID, seriesID string) ([]models.ControlEvidence, bool, error) {
	rows, err := querier.Query(ctx, `SELECT `+evidenceLifecycleColumns+`
		FROM control_evidence ce
		JOIN control_implementations ci ON ci.id=ce.control_implementation_id
		WHERE ce.organization_id=$1::uuid AND ci.organization_id=$1::uuid
		  AND ci.framework_control_id=$2::uuid AND ce.series_id=$3::uuid
		  AND ce.deleted_at IS NULL AND ci.deleted_at IS NULL
		ORDER BY ce.version_number DESC,ce.id DESC LIMIT $4`, organizationID, controlID, seriesID, maxEvidenceLifecycleHistoryRows+1)
	if err != nil {
		return nil, false, fmt.Errorf("list evidence versions: %w", err)
	}
	defer rows.Close()
	result := make([]models.ControlEvidence, 0)
	for rows.Next() {
		item, err := scanLifecycleEvidence(rows)
		if err != nil {
			return nil, false, err
		}
		result = append(result, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("list evidence versions: %w", err)
	}
	truncated := len(result) > maxEvidenceLifecycleHistoryRows
	if truncated {
		result = result[:maxEvidenceLifecycleHistoryRows]
	}
	return result, truncated, nil
}

func listEvidenceReviews(ctx context.Context, querier database.Querier, organizationID, evidenceID string) ([]models.EvidenceReview, bool, error) {
	rows, err := querier.Query(ctx, `SELECT id,organization_id,evidence_id,decision,comment,
		reviewer_id,evidence_sha256,request_id,metadata,created_at
		FROM evidence_reviews WHERE organization_id=$1::uuid AND evidence_id=$2::uuid
		ORDER BY created_at DESC,id DESC LIMIT $3`, organizationID, evidenceID, maxEvidenceLifecycleHistoryRows+1)
	if err != nil {
		return nil, false, fmt.Errorf("list evidence reviews: %w", err)
	}
	defer rows.Close()
	result := make([]models.EvidenceReview, 0)
	for rows.Next() {
		var item models.EvidenceReview
		var metadata []byte
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.EvidenceID, &item.Decision,
			&item.Comment, &item.ReviewerID, &item.EvidenceSHA256, &item.RequestID,
			&metadata, &item.CreatedAt); err != nil {
			return nil, false, fmt.Errorf("scan evidence review: %w", err)
		}
		item.Metadata = metadata
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("list evidence reviews: %w", err)
	}
	truncated := len(result) > maxEvidenceLifecycleHistoryRows
	if truncated {
		result = result[:maxEvidenceLifecycleHistoryRows]
	}
	return result, truncated, nil
}

func listEvidenceCustodyEvents(ctx context.Context, querier database.Querier, organizationID, evidenceID string) ([]models.EvidenceCustodyEvent, bool, error) {
	rows, err := querier.Query(ctx, `SELECT id,organization_id,evidence_id,series_id,chain_sequence,
		encode(previous_hash,'hex'),encode(event_hash,'hex'),event_type,actor_user_id,
		actor_type,reason,object_sha256,request_id,details,created_at
		FROM evidence_custody_events WHERE organization_id=$1::uuid AND evidence_id=$2::uuid
		ORDER BY chain_sequence DESC,id DESC LIMIT $3`, organizationID, evidenceID, maxEvidenceLifecycleHistoryRows+1)
	if err != nil {
		return nil, false, fmt.Errorf("list evidence custody: %w", err)
	}
	defer rows.Close()
	result := make([]models.EvidenceCustodyEvent, 0)
	for rows.Next() {
		item, err := scanEvidenceCustodyEvent(rows)
		if err != nil {
			return nil, false, err
		}
		result = append(result, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("list evidence custody: %w", err)
	}
	truncated := len(result) > maxEvidenceLifecycleHistoryRows
	if truncated {
		result = result[:maxEvidenceLifecycleHistoryRows]
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result, truncated, nil
}

func verifyEvidenceCustodyChain(ctx context.Context, querier database.Querier, organizationID, evidenceID string) (*models.EvidenceCustodyChainVerification, error) {
	result := &models.EvidenceCustodyChainVerification{EvidenceID: evidenceID}
	err := querier.QueryRow(ctx, `SELECT valid,event_count,head_sequence,head_hash,first_invalid_sequence
		FROM verify_evidence_custody_chain($1::uuid,$2::uuid)`, organizationID, evidenceID).Scan(
		&result.Valid, &result.EventCount, &result.HeadSequence, &result.HeadHash, &result.FirstInvalidSequence,
	)
	if err != nil {
		return nil, fmt.Errorf("verify evidence custody chain: %w", err)
	}
	return result, nil
}

func (r *evidenceLifecycleRepo) AppendEvidenceCustodyEvent(
	ctx context.Context, organizationID, controlID, evidenceID string, input models.EvidenceCustodyEventInput,
) (*models.EvidenceCustodyEvent, error) {
	if input.ActorUserID == nil || strings.TrimSpace(*input.ActorUserID) == "" || input.ActorType != "user" {
		return nil, errors.New("API-originated evidence custody events require an authenticated user actor")
	}
	var deliveryMode *string
	switch input.EventType {
	case models.EvidenceCustodyDownloadAuthorized:
		mode, ok := input.Details["delivery_mode"].(string)
		if !ok || (mode != "private_stream" && mode != "signed_url") {
			return nil, errors.New("authorized evidence downloads require a valid delivery mode")
		}
		deliveryMode = &mode
	case models.EvidenceCustodyIntegrityVerified, models.EvidenceCustodyIntegrityFailed:
		// The database capability derives fixed integrity-event details.
	default:
		return nil, errors.New("unsupported API-originated evidence custody event")
	}
	querier := database.QuerierFromContext(ctx, r.pool)
	var eventID string
	if err := querier.QueryRow(ctx, `SELECT event_id
		FROM append_evidence_access_custody_event(
			$1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7)`,
		organizationID, controlID, evidenceID, *input.ActorUserID,
		string(input.EventType), input.RequestID, deliveryMode).Scan(&eventID); err != nil {
		return nil, err
	}
	row := querier.QueryRow(ctx, `SELECT id,organization_id,evidence_id,series_id,chain_sequence,
		encode(previous_hash,'hex'),encode(event_hash,'hex'),event_type,actor_user_id,
		actor_type,reason,object_sha256,request_id,details,created_at
		FROM evidence_custody_events
		WHERE organization_id=$1::uuid AND evidence_id=$2::uuid AND id=$3::uuid`,
		organizationID, evidenceID, eventID)
	return scanEvidenceCustodyEvent(row)
}

func (r *evidenceLifecycleRepo) DueEvidenceTenants(ctx context.Context, limit int) ([]string, error) {
	if limit < 1 || limit > maxEvidenceLifecycleBatchSize {
		return nil, fmt.Errorf("evidence tenant batch limit must be between 1 and %d", maxEvidenceLifecycleBatchSize)
	}
	result := make([]string, 0)
	var after *string
	for {
		rows, err := r.pool.Query(ctx, `SELECT organization_id FROM evidence_due_tenants($1,$2::uuid)`, limit, after)
		if err != nil {
			return nil, fmt.Errorf("discover due evidence tenants: %w", err)
		}
		pageCount := 0
		for rows.Next() {
			var organizationID string
			if err := rows.Scan(&organizationID); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan due evidence tenant: %w", err)
			}
			result = append(result, organizationID)
			after = &result[len(result)-1]
			pageCount++
		}
		rowErr := rows.Err()
		rows.Close()
		if rowErr != nil {
			return nil, fmt.Errorf("iterate due evidence tenants: %w", rowErr)
		}
		if pageCount < limit {
			break
		}
	}
	return result, nil
}

func (r *evidenceLifecycleRepo) ExpireDueEvidence(ctx context.Context, organizationID string, limit int) ([]models.ExpiredEvidenceNotice, error) {
	if limit < 1 || limit > maxEvidenceLifecycleBatchSize {
		return nil, fmt.Errorf("evidence expiry batch limit must be between 1 and %d", maxEvidenceLifecycleBatchSize)
	}
	querier := database.QuerierFromContext(ctx, r.pool)
	result := make([]models.ExpiredEvidenceNotice, 0)
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT evidence_id,organization_id,
			control_implementation_id,title,expires_at,collected_by,control_code,
			custody_event_id,custody_created_at
			FROM expire_due_evidence($1::uuid,$2)
			ORDER BY expires_at,evidence_id`, organizationID, limit)
		if err != nil {
			return fmt.Errorf("expire due evidence: %w", err)
		}
		custodyEventIDs := make([]string, 0)
		custodyCreatedAts := make([]time.Time, 0)
		for rows.Next() {
			var item models.ExpiredEvidenceNotice
			var custodyEventID string
			var custodyCreatedAt time.Time
			if err := rows.Scan(&item.EvidenceID, &item.OrganizationID, &item.ControlImplementationID,
				&item.Title, &item.ExpiresAt, &item.CollectedBy, &item.ControlCode,
				&custodyEventID, &custodyCreatedAt); err != nil {
				rows.Close()
				return fmt.Errorf("scan expired evidence: %w", err)
			}
			result = append(result, item)
			custodyEventIDs = append(custodyEventIDs, custodyEventID)
			custodyCreatedAts = append(custodyCreatedAts, custodyCreatedAt)
		}
		rowErr := rows.Err()
		rows.Close()
		if rowErr != nil {
			return fmt.Errorf("expire due evidence: %w", rowErr)
		}

		if r.outbox == nil {
			return nil
		}
		for index := range result {
			notice := &result[index]
			custodyEventID := custodyEventIDs[index]
			custodyCreatedAt := custodyCreatedAts[index]
			data := map[string]any{
				"evidence_title":         notice.Title,
				"expires_at":             notice.ExpiresAt.UTC().Format(time.RFC3339),
				"control_implementation": notice.ControlImplementationID,
			}
			if notice.CollectedBy != nil {
				data["owner_id"] = *notice.CollectedBy
			}
			if notice.ControlCode != nil {
				data["control_code"] = *notice.ControlCode
			}
			payload := map[string]any{
				"type": "evidence.expired", "severity": "high",
				"org_id": notice.OrganizationID, "entity_type": "control_evidence",
				"entity_id": notice.EvidenceID, "entity_ref": notice.Title,
				"data": data, "timestamp": custodyCreatedAt.UTC(),
			}
			envelope, err := queuepkg.NewEnvelope("notification.event", notice.OrganizationID, payload)
			if err != nil {
				return fmt.Errorf("create evidence expiry outbox envelope: %w", err)
			}
			envelope.ID = custodyEventID
			envelope.CorrelationID = custodyEventID
			envelope.CausationID = notice.EvidenceID
			envelope.CreatedAt = custodyCreatedAt.UTC()
			envelope.Metadata = map[string]string{
				"entity_type": "control_evidence", "entity_id": notice.EvidenceID,
				"event_type": "evidence.expired",
			}
			if err := r.outbox.Enqueue(ctx, tx, r.outboxQueue, envelope); err != nil {
				return fmt.Errorf("enqueue evidence expiry notification: %w", err)
			}
			notice.NotificationQueued = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

type evidenceReadTransactionBeginner interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

func withEvidenceReadSnapshot(
	ctx context.Context,
	querier database.Querier,
	fn func(pgx.Tx) error,
) (returnErr error) {
	if transaction, ok := querier.(pgx.Tx); ok {
		var isolation string
		if err := transaction.QueryRow(ctx, `SHOW transaction_isolation`).Scan(&isolation); err != nil {
			return fmt.Errorf("inspect evidence lifecycle snapshot isolation: %w", err)
		}
		if isolation != "repeatable read" && isolation != "serializable" {
			return fmt.Errorf("evidence lifecycle snapshot requires repeatable read or serializable isolation, got %s", isolation)
		}
		return fn(transaction)
	}
	beginner, ok := querier.(evidenceReadTransactionBeginner)
	if !ok {
		return errors.New("evidence lifecycle database executor cannot start a snapshot transaction")
	}
	transaction, err := beginner.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return fmt.Errorf("begin evidence lifecycle snapshot: %w", err)
	}
	defer func() {
		if rollbackErr := transaction.Rollback(context.Background()); rollbackErr != nil &&
			!errors.Is(rollbackErr, pgx.ErrTxClosed) {
			returnErr = errors.Join(returnErr, fmt.Errorf("rollback evidence lifecycle snapshot: %w", rollbackErr))
		}
	}()
	if err := fn(transaction); err != nil {
		return err
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit evidence lifecycle snapshot: %w", err)
	}
	return nil
}

type evidenceLifecycleRow interface{ Scan(...any) error }

func scanLifecycleEvidence(row evidenceLifecycleRow) (*models.ControlEvidence, error) {
	item := new(models.ControlEvidence)
	var metadata []byte
	var lifecycleStatus string
	err := row.Scan(&item.ID, &item.OrganizationID, &item.ControlImplementationID,
		&item.Title, &item.Description, &item.EvidenceType, &item.ObjectKey,
		&item.FileName, &item.FileSizeBytes, &item.MIMEType, &item.FileHash,
		&item.CollectionMethod, &item.CollectedAt, &item.CollectedBy,
		&item.ValidFrom, &item.ValidUntil, &item.IsCurrent, &item.ReviewStatus,
		&item.ReviewedBy, &item.ReviewedAt, &item.ReviewNotes, &metadata,
		&item.CreatedAt, &item.UpdatedAt, &item.DeletedAt, &item.SeriesID,
		&item.VersionNumber, &item.SupersedesEvidenceID, &item.SupersededByEvidenceID,
		&item.SupersededAt, &lifecycleStatus, &item.ExpiresAt, &item.VersionReason,
		&item.ContentFingerprint)
	if err != nil {
		return nil, fmt.Errorf("scan evidence lifecycle record: %w", err)
	}
	item.Metadata = metadata
	item.LifecycleStatus = models.EvidenceLifecycleStatus(lifecycleStatus)
	return item, nil
}

func scanEvidenceCustodyEvent(row evidenceLifecycleRow) (*models.EvidenceCustodyEvent, error) {
	item := new(models.EvidenceCustodyEvent)
	var eventType string
	var details []byte
	err := row.Scan(&item.ID, &item.OrganizationID, &item.EvidenceID, &item.SeriesID,
		&item.Sequence, &item.PreviousHash, &item.EventHash, &eventType,
		&item.ActorUserID, &item.ActorType, &item.Reason, &item.ObjectSHA256,
		&item.RequestID, &details, &item.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan evidence custody event: %w", err)
	}
	item.EventType = models.EvidenceCustodyEventType(eventType)
	item.Details = details
	return item, nil
}
