//go:build integration

package queue

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRabbitMQDeliveryContract(t *testing.T) {
	amqpURL := os.Getenv("RABBITMQ_URL")
	if amqpURL == "" {
		t.Skip("RABBITMQ_URL is required for the integration test")
	}

	token := uuid.NewString()
	config := DefaultConfig(amqpURL)
	config.Exchange = "cf.integration." + token
	config.RetryExchange = config.Exchange + ".retry"
	config.DeadLetterExchange = config.Exchange + ".dead"
	config.QuarantineExchange = config.Exchange + ".quarantine"
	config.ConnectionName = "cf-integration-test"
	config.MaxAttempts = 2
	config.RetryDelay = time.Second
	config.ReconnectMin = 100 * time.Millisecond
	config.ReconnectMax = time.Second
	config.PublishTimeout = 5 * time.Second
	config.ConnectTimeout = 5 * time.Second
	queueName := "cf.integration.worker." + token
	topology, err := TopologyFor(queueName, config)
	if err != nil {
		t.Fatal(err)
	}

	service, err := NewRabbitMQServiceWithConfig(config, nil)
	if err != nil {
		t.Fatal(err)
	}
	consumerCtx, cancelConsumer := context.WithCancel(context.Background())
	consumerDone := make(chan error, 1)
	retryDone := make(chan Envelope, 1)
	deadHandled := make(chan struct{}, 1)
	var retryCalls atomic.Int32
	go func() {
		consumerDone <- service.SubscribeEnvelope(consumerCtx, queueName, func(_ context.Context, envelope Envelope) error {
			switch envelope.Type {
			case "integration.retry":
				if retryCalls.Add(1) == 1 {
					return errors.New("transient integration failure")
				}
				retryDone <- envelope
				return nil
			case "integration.dead":
				deadHandled <- struct{}{}
				return Permanent(errors.New("permanent integration failure"))
			default:
				return Permanent(errors.New("unexpected integration message type"))
			}
		})
	}()

	rawConnection, err := amqp.Dial(amqpURL)
	if err != nil {
		cancelConsumer()
		_ = service.Close()
		t.Fatal(err)
	}
	rawChannel, err := rawConnection.Channel()
	if err != nil {
		cancelConsumer()
		_ = service.Close()
		_ = rawConnection.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancelConsumer()
		_ = service.Close()
		_, _ = rawChannel.QueueDelete(topology.Queue, false, false, false)
		_, _ = rawChannel.QueueDelete(topology.RetryQueue, false, false, false)
		_, _ = rawChannel.QueueDelete(topology.DeadLetterQueue, false, false, false)
		_, _ = rawChannel.QueueDelete(topology.QuarantineQueue, false, false, false)
		_ = rawChannel.ExchangeDelete(config.Exchange, false, false)
		_ = rawChannel.ExchangeDelete(config.RetryExchange, false, false)
		_ = rawChannel.ExchangeDelete(config.DeadLetterExchange, false, false)
		_ = rawChannel.ExchangeDelete(config.QuarantineExchange, false, false)
		_ = rawChannel.Close()
		_ = rawConnection.Close()
	})

	retryEnvelope, err := NewEnvelope("integration.retry", "", map[string]bool{"retry": true})
	if err != nil {
		t.Fatal(err)
	}
	publishCtx, cancelPublish := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelPublish()
	if err := service.PublishEnvelope(publishCtx, queueName, retryEnvelope); err != nil {
		t.Fatal(err)
	}
	select {
	case delivered := <-retryDone:
		if delivered.Attempt != 2 || retryCalls.Load() != 2 {
			t.Fatalf("retry delivery attempt=%d calls=%d", delivered.Attempt, retryCalls.Load())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for delayed retry")
	}

	unroutableEnvelope, err := NewEnvelope("integration.unroutable", "", map[string]bool{"unroutable": true})
	if err != nil {
		t.Fatal(err)
	}
	unroutableBody, err := encodeEnvelope(unroutableEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	err = service.publishWithReconnect(
		publishCtx,
		topology,
		config.Exchange,
		"missing."+token,
		publishingForEnvelope(unroutableEnvelope, unroutableBody),
	)
	var unroutable *UnroutableError
	if !errors.As(err, &unroutable) {
		t.Fatalf("mandatory publish error = %v, want UnroutableError", err)
	}

	deadEnvelope, err := NewEnvelope("integration.dead", "", map[string]bool{"dead": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.PublishEnvelope(publishCtx, queueName, deadEnvelope); err != nil {
		t.Fatal(err)
	}
	select {
	case <-deadHandled:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for permanent handler failure")
	}
	deadDelivery := waitForDelivery(t, rawChannel, topology.DeadLetterQueue, 5*time.Second)
	deadLetter, err := decodeEnvelope(deadDelivery.Body)
	if err != nil {
		t.Fatal(err)
	}
	if deadLetter.ID != deadEnvelope.ID || deadLetter.Metadata["terminal_failure"] == "" {
		t.Fatalf("dead-letter envelope = %+v", deadLetter)
	}

	if err := rawChannel.PublishWithContext(publishCtx, config.Exchange, topology.RoutingKey, true, false, amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		Body:         []byte(`{"invalid":`),
	}); err != nil {
		t.Fatal(err)
	}
	quarantined := waitForDelivery(t, rawChannel, topology.QuarantineQueue, 5*time.Second)
	if quarantined.DeliveryMode != amqp.Persistent || quarantined.Type != "quarantine.invalid" {
		t.Fatalf("quarantine delivery properties = %+v", quarantined)
	}
	if quarantined.Headers["x-quarantine-reason"] == "" {
		t.Fatalf("quarantine reason missing: %+v", quarantined.Headers)
	}

	cancelConsumer()
	select {
	case err := <-consumerDone:
		if err != nil {
			t.Fatalf("consumer shutdown error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consumer did not drain after cancellation")
	}
}

func waitForDelivery(t *testing.T, channel *amqp.Channel, queueName string, timeout time.Duration) amqp.Delivery {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		delivery, found, err := channel.Get(queueName, true)
		if err != nil {
			t.Fatal(err)
		}
		if found {
			return delivery
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for delivery from %s", queueName)
	return amqp.Delivery{}
}
