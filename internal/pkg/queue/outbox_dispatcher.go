package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

type EnvelopePublisher interface {
	PublishEnvelope(context.Context, string, Envelope) error
}

// OutboxDispatcher moves committed envelopes to RabbitMQ. A message is only
// marked published after RabbitMQ confirms it, so a crash can cause a safe
// duplicate but cannot silently lose a committed domain event.
type OutboxDispatcher struct {
	store     OutboxStore
	publisher EnvelopePublisher
	config    OutboxConfig
	observer  Observer
}

func NewOutboxDispatcher(store OutboxStore, publisher EnvelopePublisher, config OutboxConfig) (*OutboxDispatcher, error) {
	if store == nil || publisher == nil {
		return nil, fmt.Errorf("outbox store and publisher are required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &OutboxDispatcher{store: store, publisher: publisher, config: config, observer: noopObserver{}}, nil
}

func (d *OutboxDispatcher) SetObserver(observer Observer) {
	if observer == nil {
		observer = noopObserver{}
	}
	d.observer = observer
}

func (d *OutboxDispatcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(d.config.PollInterval)
	defer ticker.Stop()
	for {
		processed, err := d.DispatchOnce(ctx)
		if err != nil && ctx.Err() == nil {
			log.Error().Err(err).Msg("outbox dispatch cycle failed")
		}
		if ctx.Err() != nil {
			return nil
		}
		if processed == d.config.BatchSize {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (d *OutboxDispatcher) DispatchOnce(ctx context.Context) (processed int, dispatchErr error) {
	started := time.Now()
	defer func() {
		outcome := "success"
		if dispatchErr != nil {
			outcome = "error"
		}
		d.observer.ObserveQueueOperation("outbox", outcome, time.Since(started))
	}()
	messages, err := d.store.ClaimBatch(ctx)
	if err != nil {
		return 0, err
	}
	for index, message := range messages {
		messageCtx := ExtractTraceContext(ctx, message.Envelope)
		publishCtx, cancel := context.WithTimeout(messageCtx, d.config.PublishTimeout)
		err := d.publisher.PublishEnvelope(publishCtx, message.QueueName, message.Envelope)
		cancel()
		if err == nil {
			if markErr := d.store.MarkPublished(ctx, message); markErr != nil {
				return index + 1, fmt.Errorf("confirm outbox publication: %w", markErr)
			}
			continue
		}

		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), d.config.PublishTimeout)
		if isPermanentBrokerError(err) {
			markErr := d.store.MarkDead(cleanupCtx, message, err)
			cleanupCancel()
			if markErr != nil {
				return index + 1, errors.Join(err, markErr)
			}
			continue
		}
		_, markErr := d.store.MarkFailed(cleanupCtx, message, err)
		cleanupCancel()
		if markErr != nil {
			return index + 1, errors.Join(err, markErr)
		}
		// Release messages claimed later in this batch immediately instead of
		// waiting for their leases to expire after a broker outage. They do not
		// consume a retry because no publish was attempted for them.
		for _, pending := range messages[index+1:] {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), d.config.PublishTimeout)
			releaseErr := d.store.ReleaseUnattempted(cleanupCtx, pending, err)
			cleanupCancel()
			if releaseErr != nil {
				markErr = errors.Join(markErr, releaseErr)
			}
		}
		return index + 1, errors.Join(err, markErr)
	}
	return len(messages), nil
}
