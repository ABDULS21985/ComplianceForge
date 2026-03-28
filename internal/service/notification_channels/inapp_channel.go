package notification_channels

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// InAppChannel stores notifications in the database for in-app display.
type InAppChannel struct {
	pool *pgxpool.Pool
}

func NewInAppChannel(pool *pgxpool.Pool) *InAppChannel {
	return &InAppChannel{pool: pool}
}

// Send stores an in-app notification and marks it as delivered.
func (ch *InAppChannel) Send(ctx context.Context, notificationID string) error {
	_, err := ch.pool.Exec(ctx, `
		UPDATE notifications
		SET status = 'delivered', delivered_at = $2
		WHERE id = $1
	`, notificationID, time.Now().UTC())
	if err != nil {
		log.Error().Err(err).Str("notification_id", notificationID).Msg("inapp_channel: failed to mark delivered")
		return err
	}
	log.Debug().Str("notification_id", notificationID).Msg("inapp_channel: marked as delivered")
	return nil
}
