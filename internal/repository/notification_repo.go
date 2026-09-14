package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
)

// NotificationStateRepository is the persistence contract for recipient and
// provider-driven terminal notification state changes.
type NotificationStateRepository interface {
	Acknowledge(context.Context, string, string, string) (time.Time, error)
	RecordBounce(context.Context, string, string, string) error
}

// PostgresNotificationRepository persists notification state through the
// request/worker's tenant-scoped querier so FORCE RLS remains authoritative.
type PostgresNotificationRepository struct {
	pool *pgxpool.Pool
}

func NewNotificationRepository(pool *pgxpool.Pool) *PostgresNotificationRepository {
	return &PostgresNotificationRepository{pool: pool}
}

func (repository *PostgresNotificationRepository) Acknowledge(
	ctx context.Context,
	organizationID, userID, notificationID string,
) (time.Time, error) {
	var acknowledgedAt time.Time
	err := database.QuerierFromContext(ctx, repository.pool).QueryRow(ctx, `
		UPDATE notifications
		SET acknowledged_at=COALESCE(acknowledged_at,NOW())
		WHERE id=$1 AND organization_id=$2 AND recipient_user_id=$3
		  AND status IN ('sent','delivered')
		RETURNING acknowledged_at`, notificationID, organizationID, userID).Scan(&acknowledgedAt)
	if err != nil {
		return time.Time{}, err
	}
	return acknowledgedAt.UTC(), nil
}

func (repository *PostgresNotificationRepository) RecordBounce(
	ctx context.Context,
	organizationID, notificationID, providerCode string,
) error {
	result, err := database.QuerierFromContext(ctx, repository.pool).Exec(ctx, `
		UPDATE notifications
		SET status='bounced',failure_code=$1,error_message='notification bounced (' || $1 || ')',
		    dead_at=NOW(),next_retry_at=NULL,lease_owner=NULL,lease_token=NULL,leased_until=NULL
		WHERE id=$2 AND organization_id=$3`, providerCode, notificationID, organizationID)
	if err != nil {
		return fmt.Errorf("record notification bounce: %w", err)
	}
	if result.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return nil
}

var _ NotificationStateRepository = (*PostgresNotificationRepository)(nil)
