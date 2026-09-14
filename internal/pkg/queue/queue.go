package queue

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

// QueueService preserves the simple byte-oriented API while RabbitMQService
// also exposes the versioned Envelope API used by production workers.
type QueueService interface {
	Publish(ctx context.Context, queueName string, message []byte) error
	Subscribe(ctx context.Context, queueName string, handler func([]byte) error) error
	Close() error
}

type EnvelopeHandler func(context.Context, Envelope) error

type RabbitMQService struct {
	config       Config
	deduplicator Deduplicator

	mu               sync.Mutex
	connection       *amqp.Connection
	publisher        *amqp.Channel
	publisherReturns <-chan amqp.Return
	closed           bool
	publishGate      chan struct{}
}

// NewRabbitMQService uses production-safe defaults and connects lazily, which
// lets long-running workers survive RabbitMQ restarts and initial unavailability.
func NewRabbitMQService(amqpURL string) (*RabbitMQService, error) {
	return NewRabbitMQServiceWithConfig(DefaultConfig(amqpURL), nil)
}

// NewRabbitMQServiceWithConfig permits topology and delivery tuning. A custom
// Deduplicator should be durable when cross-process exactly-once effects matter.
func NewRabbitMQServiceWithConfig(config Config, deduplicator Deduplicator) (*RabbitMQService, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if deduplicator == nil {
		memoryStore, err := NewMemoryDeduplicatorWithLease(config.IdempotencyLease, config.IdempotencyTTL, config.IdempotencyMaxMessages)
		if err != nil {
			return nil, err
		}
		deduplicator = memoryStore
	}
	return &RabbitMQService{config: config, deduplicator: deduplicator, publishGate: make(chan struct{}, 1)}, nil
}

type legacyPayload struct {
	Data []byte `json:"data"`
}

func (r *RabbitMQService) Publish(ctx context.Context, queueName string, message []byte) error {
	envelope, err := NewEnvelope("legacy.raw", "", legacyPayload{Data: message})
	if err != nil {
		return err
	}
	return r.PublishEnvelope(ctx, queueName, envelope)
}

func (r *RabbitMQService) Subscribe(ctx context.Context, queueName string, handler func([]byte) error) error {
	if handler == nil {
		return fmt.Errorf("queue handler is required")
	}
	return r.SubscribeEnvelope(ctx, queueName, func(_ context.Context, envelope Envelope) error {
		if envelope.Type != "legacy.raw" {
			return Permanent(fmt.Errorf("unexpected message type %q on legacy subscription", envelope.Type))
		}
		var payload legacyPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return Permanent(fmt.Errorf("decode legacy payload: %w", err))
		}
		return handler(payload.Data)
	})
}

func (r *RabbitMQService) PublishEnvelope(ctx context.Context, queueName string, envelope Envelope) error {
	topology, err := TopologyFor(queueName, r.config)
	if err != nil {
		return err
	}
	envelope = envelope.normalized()
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("validate queue envelope: %w", err)
	}
	body, err := encodeEnvelope(envelope)
	if err != nil {
		return err
	}
	if len(body) > r.config.MaxMessageBytes {
		return fmt.Errorf("encoded message is %d bytes; limit is %d", len(body), r.config.MaxMessageBytes)
	}
	return r.publishWithReconnect(ctx, topology, r.config.Exchange, topology.RoutingKey, publishingForEnvelope(envelope, body))
}

// SubscribeEnvelope blocks until ctx is cancelled or the service is closed.
// Transient broker/channel failures reconnect with capped exponential backoff.
func (r *RabbitMQService) SubscribeEnvelope(ctx context.Context, queueName string, handler EnvelopeHandler) error {
	if handler == nil {
		return fmt.Errorf("queue handler is required")
	}
	topology, err := TopologyFor(queueName, r.config)
	if err != nil {
		return err
	}

	backoff := r.config.ReconnectMin
	for {
		if ctx.Err() != nil {
			return nil
		}
		sessionStarted := time.Now()
		err = r.consumeOnce(ctx, topology, handler)
		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(err, ErrClosed) {
			return ErrClosed
		}
		if isPermanentBrokerError(err) {
			return err
		}
		if time.Since(sessionStarted) >= r.config.ReconnectMax {
			backoff = r.config.ReconnectMin
		}
		log.Warn().Err(err).Str("queue", queueName).Dur("retry_in", backoff).Msg("queue consumer disconnected; reconnecting")
		if err := waitForContext(ctx, backoff); err != nil {
			return nil
		}
		backoff = nextBackoff(backoff, r.config.ReconnectMax)
	}
}

func (r *RabbitMQService) consumeOnce(ctx context.Context, topology Topology, handler EnvelopeHandler) error {
	connection, err := r.ensureConnection()
	if err != nil {
		return err
	}
	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open consumer channel: %w", err)
	}
	defer channel.Close()

	if err := declareTopology(channel, r.config, topology); err != nil {
		return err
	}
	if err := channel.Qos(r.config.Prefetch, 0, false); err != nil {
		return fmt.Errorf("set consumer QoS: %w", err)
	}
	consumerTag := r.config.ConnectionName + "-" + uuid.NewString()
	deliveries, err := channel.Consume(topology.Queue, consumerTag, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume queue %s: %w", topology.Queue, err)
	}
	channelClosed := channel.NotifyClose(make(chan *amqp.Error, 1))

	for {
		select {
		case <-ctx.Done():
			_ = channel.Cancel(consumerTag, false)
			return nil
		case channelErr, ok := <-channelClosed:
			if !ok || channelErr == nil {
				return fmt.Errorf("consumer channel closed")
			}
			return fmt.Errorf("consumer channel closed: %w", channelErr)
		case delivery, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("delivery stream closed")
			}
			if err := r.processDelivery(ctx, topology, delivery, handler); err != nil {
				return err
			}
		}
	}
}

func (r *RabbitMQService) processDelivery(ctx context.Context, topology Topology, delivery amqp.Delivery, handler EnvelopeHandler) error {
	if len(delivery.Body) > r.config.MaxMessageBytes {
		return r.quarantineDelivery(ctx, topology, delivery, fmt.Errorf("message exceeds %d-byte limit", r.config.MaxMessageBytes))
	}
	envelope, err := decodeEnvelope(delivery.Body)
	if err == nil {
		err = applyDeliveryMetadata(&envelope, delivery)
	}
	if err == nil {
		err = envelope.Validate()
	}
	if err != nil {
		return r.quarantineDelivery(ctx, topology, delivery, err)
	}

	deliveryEnvelope := envelope
	status, err := r.deduplicator.Begin(ctx, deliveryEnvelope)
	if err != nil {
		return nackDelivery(delivery, true, fmt.Errorf("begin message deduplication: %w", err))
	}
	switch status {
	case DeduplicationComplete:
		return ackDelivery(delivery)
	case DeduplicationInProgress:
		return nackDelivery(delivery, true, ErrDeduplicationInProgress)
	}

	handlerErr := r.invokeWithLeaseHeartbeat(ctx, handler, envelope)
	if handlerErr == nil {
		if err := r.deduplicator.Complete(ctx, deliveryEnvelope); err != nil {
			return nackDelivery(delivery, true, fmt.Errorf("complete message deduplication: %w", err))
		}
		return ackDelivery(delivery)
	}

	route := failureRoute(envelope.Attempt, r.config.MaxAttempts, handlerErr)
	if route == FailureRetry {
		envelope.Attempt++
		envelope.Metadata = metadataWithFailure(envelope.Metadata, "last_failure", failureReason(handlerErr))
		if err := r.publishEnvelopeTo(ctx, topology, r.config.RetryExchange, topology.RoutingKey, envelope); err != nil {
			_ = r.deduplicator.Forget(ctx, deliveryEnvelope)
			return nackDelivery(delivery, true, fmt.Errorf("publish retry: %w", err))
		}
		r.markAttemptComplete(ctx, deliveryEnvelope)
		return ackDelivery(delivery)
	}

	envelope.Metadata = metadataWithFailure(envelope.Metadata, "terminal_failure", failureReason(handlerErr))
	if err := r.publishEnvelopeTo(ctx, topology, r.config.DeadLetterExchange, topology.RoutingKey, envelope); err != nil {
		_ = r.deduplicator.Forget(ctx, deliveryEnvelope)
		return nackDelivery(delivery, true, fmt.Errorf("publish dead letter: %w", err))
	}
	r.markAttemptComplete(ctx, deliveryEnvelope)
	return ackDelivery(delivery)
}

func (r *RabbitMQService) markAttemptComplete(ctx context.Context, envelope Envelope) {
	if err := r.deduplicator.Complete(ctx, envelope); err != nil {
		log.Error().Err(err).Str("deduplication_key", deduplicationKey(envelope)).Msg("could not record completed queue attempt")
	}
}

func (r *RabbitMQService) invokeWithLeaseHeartbeat(ctx context.Context, handler EnvelopeHandler, envelope Envelope) error {
	handlerCtx, cancelHandler := context.WithCancel(ctx)
	defer cancelHandler()
	heartbeatDone := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(r.config.IdempotencyLease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-handlerCtx.Done():
				heartbeatDone <- nil
				return
			case <-ticker.C:
				if err := r.deduplicator.Renew(handlerCtx, envelope); err != nil {
					if handlerCtx.Err() != nil {
						heartbeatDone <- nil
						return
					}
					cancelHandler()
					heartbeatDone <- fmt.Errorf("renew message processing lease: %w", err)
					return
				}
			}
		}
	}()

	handlerErr := invokeHandler(handlerCtx, handler, envelope)
	cancelHandler()
	if heartbeatErr := <-heartbeatDone; heartbeatErr != nil {
		return heartbeatErr
	}
	return handlerErr
}

func (r *RabbitMQService) quarantineDelivery(ctx context.Context, topology Topology, delivery amqp.Delivery, cause error) error {
	publishing := quarantinePublishing(delivery, topology.Queue, cause, r.config.MaxMessageBytes)
	if err := r.publishWithReconnect(ctx, topology, r.config.QuarantineExchange, topology.RoutingKey, publishing); err != nil {
		return nackDelivery(delivery, true, fmt.Errorf("publish quarantine message: %w", err))
	}
	return ackDelivery(delivery)
}

func quarantinePublishing(delivery amqp.Delivery, queueName string, cause error, maxBodyBytes int) amqp.Publishing {
	messageID := uuid.NewString()
	correlationID := boundedAMQPString(delivery.CorrelationId, 255)
	if correlationID == "" {
		correlationID = messageID
	}
	headers := amqp.Table{
		"x-original-queue":    queueName,
		"x-quarantine-reason": failureReason(cause),
		"x-quarantined-at":    time.Now().UTC().Format(time.RFC3339Nano),
	}
	if originalMessageID := boundedAMQPString(delivery.MessageId, 255); originalMessageID != "" {
		headers["x-original-message-id"] = originalMessageID
	}
	if originalType := boundedAMQPString(delivery.Type, 255); originalType != "" {
		headers["x-original-message-type"] = originalType
	}
	if tenantID, ok := delivery.Headers["x-tenant-id"].(string); ok {
		headers["x-original-tenant-id"] = boundedAMQPString(tenantID, 255)
	}
	if schemaVersion, ok := integerHeader(delivery.Headers["x-schema-version"]); ok {
		headers["x-original-schema-version"] = int64(schemaVersion)
	}
	if attempt, ok := integerHeader(delivery.Headers["x-attempt"]); ok {
		headers["x-original-attempt"] = int64(attempt)
	}

	body := append([]byte(nil), delivery.Body...)
	if len(body) > maxBodyBytes {
		digest := sha256.Sum256(body)
		headers["x-original-body-bytes"] = int64(len(body))
		headers["x-original-body-sha256"] = fmt.Sprintf("%x", digest)
		headers["x-body-truncated"] = true
		body = body[:maxBodyBytes]
	}
	return amqp.Publishing{
		Headers:       headers,
		ContentType:   "application/octet-stream",
		DeliveryMode:  amqp.Persistent,
		CorrelationId: correlationID,
		MessageId:     messageID,
		Timestamp:     time.Now().UTC(),
		Type:          "quarantine.invalid",
		AppId:         "complianceforge-quarantine",
		Body:          body,
	}
}

func boundedAMQPString(value string, limit int) string {
	value = strings.ToValidUTF8(value, "?")
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func (r *RabbitMQService) publishEnvelopeTo(ctx context.Context, topology Topology, exchange, routingKey string, envelope Envelope) error {
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("validate queue envelope: %w", err)
	}
	body, err := encodeEnvelope(envelope)
	if err != nil {
		return err
	}
	if len(body) > r.config.MaxMessageBytes {
		return fmt.Errorf("encoded message is %d bytes; limit is %d", len(body), r.config.MaxMessageBytes)
	}
	return r.publishWithReconnect(ctx, topology, exchange, routingKey, publishingForEnvelope(envelope, body))
}

func (r *RabbitMQService) publishWithReconnect(ctx context.Context, topology Topology, exchange, routingKey string, publishing amqp.Publishing) error {
	select {
	case r.publishGate <- struct{}{}:
		defer func() { <-r.publishGate }()
	case <-ctx.Done():
		return ctx.Err()
	}

	backoff := r.config.ReconnectMin
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		channel, returns, err := r.ensurePublisher(topology)
		if err == nil {
			err = r.publishConfirmed(ctx, channel, returns, exchange, routingKey, publishing)
		}
		if err == nil {
			return nil
		}
		r.invalidatePublisher(channel)
		if errors.Is(err, ErrClosed) || isPermanentBrokerError(err) {
			return err
		}
		log.Warn().Err(err).Str("exchange", exchange).Str("routing_key", routingKey).Dur("retry_in", backoff).Msg("queue publish failed; reconnecting")
		if err := waitForContext(ctx, backoff); err != nil {
			return err
		}
		backoff = nextBackoff(backoff, r.config.ReconnectMax)
	}
}

func (r *RabbitMQService) publishConfirmed(ctx context.Context, channel *amqp.Channel, returns <-chan amqp.Return, exchange, routingKey string, publishing amqp.Publishing) error {
	publishCtx, cancel := context.WithTimeout(ctx, r.config.PublishTimeout)
	defer cancel()
	deferred, err := channel.PublishWithDeferredConfirm(exchange, routingKey, true, false, publishing)
	if err != nil {
		return fmt.Errorf("publish message: %w", err)
	}
	if deferred == nil {
		return fmt.Errorf("publisher confirm mode is not active")
	}

	for {
		select {
		case returned, ok := <-returns:
			if !ok {
				return ErrReturnStream
			}
			if returned.MessageId == publishing.MessageId {
				return &UnroutableError{Exchange: exchange, RoutingKey: routingKey, ReplyCode: returned.ReplyCode, ReplyText: returned.ReplyText}
			}
		case <-deferred.Done():
			select {
			case returned, ok := <-returns:
				if ok && returned.MessageId == publishing.MessageId {
					return &UnroutableError{Exchange: exchange, RoutingKey: routingKey, ReplyCode: returned.ReplyCode, ReplyText: returned.ReplyText}
				}
			default:
			}
			if !deferred.Acked() {
				return ErrPublishNack
			}
			return nil
		case <-publishCtx.Done():
			return fmt.Errorf("wait for publisher confirmation: %w", publishCtx.Err())
		}
	}
}

func (r *RabbitMQService) ensureConnection() (*amqp.Connection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, ErrClosed
	}
	if r.connection != nil && !r.connection.IsClosed() {
		return r.connection, nil
	}

	properties := amqp.NewConnectionProperties()
	properties["connection_name"] = r.config.ConnectionName
	connection, err := amqp.DialConfig(r.config.URL, amqp.Config{
		Heartbeat:  r.config.Heartbeat,
		Locale:     "en_US",
		Properties: properties,
		Dial:       dialWithTimeouts(r.config.ConnectTimeout, r.config.PublishTimeout),
	})
	if err != nil {
		return nil, fmt.Errorf("connect to RabbitMQ %s: %s", redactedEndpoint(r.config.URL), sanitizedBrokerError(r.config.URL, err))
	}
	r.connection = connection
	r.publisher = nil
	r.publisherReturns = nil
	log.Info().Str("endpoint", redactedEndpoint(r.config.URL)).Msg("connected to RabbitMQ")
	return connection, nil
}

type writeDeadlineConnection struct {
	net.Conn
	timeout time.Duration
}

func (c *writeDeadlineConnection) Write(payload []byte) (int, error) {
	if err := c.Conn.SetWriteDeadline(time.Now().Add(c.timeout)); err != nil {
		return 0, err
	}
	written, writeErr := c.Conn.Write(payload)
	clearErr := c.Conn.SetWriteDeadline(time.Time{})
	if writeErr != nil {
		return written, writeErr
	}
	return written, clearErr
}

func dialWithTimeouts(connectTimeout, writeTimeout time.Duration) func(string, string) (net.Conn, error) {
	dial := amqp.DefaultDial(connectTimeout)
	return func(network, address string) (net.Conn, error) {
		connection, err := dial(network, address)
		if err != nil {
			return nil, err
		}
		return &writeDeadlineConnection{Conn: connection, timeout: writeTimeout}, nil
	}
}

func (r *RabbitMQService) ensurePublisher(topology Topology) (*amqp.Channel, <-chan amqp.Return, error) {
	connection, err := r.ensureConnection()
	if err != nil {
		return nil, nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, nil, ErrClosed
	}
	if r.publisher == nil || r.publisher.IsClosed() {
		channel, err := connection.Channel()
		if err != nil {
			return nil, nil, fmt.Errorf("open publisher channel: %w", err)
		}
		if err := channel.Confirm(false); err != nil {
			_ = channel.Close()
			return nil, nil, fmt.Errorf("enable publisher confirms: %w", err)
		}
		r.publisher = channel
		r.publisherReturns = channel.NotifyReturn(make(chan amqp.Return, 16))
	}
	if err := declareTopology(r.publisher, r.config, topology); err != nil {
		return r.publisher, r.publisherReturns, err
	}
	return r.publisher, r.publisherReturns, nil
}

func (r *RabbitMQService) invalidatePublisher(channel *amqp.Channel) {
	if channel == nil {
		return
	}
	r.mu.Lock()
	if r.publisher == channel {
		_ = r.publisher.Close()
		r.publisher = nil
		r.publisherReturns = nil
	}
	r.mu.Unlock()
}

func (r *RabbitMQService) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	publisher := r.publisher
	connection := r.connection
	r.publisher = nil
	r.connection = nil
	r.mu.Unlock()

	var closeErrors []error
	if publisher != nil && !publisher.IsClosed() {
		if err := publisher.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			closeErrors = append(closeErrors, fmt.Errorf("close publisher channel: %w", err))
		}
	}
	if connection != nil && !connection.IsClosed() {
		if err := connection.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			closeErrors = append(closeErrors, fmt.Errorf("close RabbitMQ connection: %w", err))
		}
	}
	return errors.Join(closeErrors...)
}

func invokeHandler(ctx context.Context, handler EnvelopeHandler, envelope Envelope) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("queue handler panic: %v", recovered)
		}
	}()
	return handler(ctx, envelope)
}

func ackDelivery(delivery amqp.Delivery) error {
	if err := delivery.Ack(false); err != nil {
		return fmt.Errorf("acknowledge delivery: %w", err)
	}
	return nil
}

func nackDelivery(delivery amqp.Delivery, requeue bool, cause error) error {
	if err := delivery.Nack(false, requeue); err != nil {
		return fmt.Errorf("reject delivery: %w", err)
	}
	return cause
}

func nextBackoff(current, maximum time.Duration) time.Duration {
	next := current * 2
	if next > maximum || next < current {
		return maximum
	}
	return next
}

func waitForContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func cloneMetadata(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source)+1)
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func metadataWithFailure(source map[string]string, key, value string) map[string]string {
	metadata := cloneMetadata(source)
	delete(metadata, "last_failure")
	delete(metadata, "terminal_failure")
	if len(metadata) < maxEnvelopeMetadataEntries {
		metadata[key] = value
	}
	return metadata
}

func sanitizedBrokerError(rawURL string, brokerErr error) string {
	message := brokerErr.Error()
	parsed, err := amqp.ParseURI(rawURL)
	if err == nil {
		if parsed.Username != "" {
			message = strings.ReplaceAll(message, parsed.Username, "***")
		}
		if parsed.Password != "" {
			message = strings.ReplaceAll(message, parsed.Password, "***")
		}
	}
	return message
}

func isPermanentBrokerError(err error) bool {
	var amqpErr *amqp.Error
	if errors.As(err, &amqpErr) {
		return amqpErr.Code == 403 || amqpErr.Code == 406 || amqpErr.Code == 530
	}
	var unroutable *UnroutableError
	return errors.As(err, &unroutable)
}
