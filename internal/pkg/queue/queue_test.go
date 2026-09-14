package queue

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type acknowledgementRecorder struct {
	acks     int
	nacks    int
	requeues int
}

func (r *acknowledgementRecorder) Ack(_ uint64, _ bool) error {
	r.acks++
	return nil
}

func (r *acknowledgementRecorder) Nack(_ uint64, _ bool, requeue bool) error {
	r.nacks++
	if requeue {
		r.requeues++
	}
	return nil
}

func (r *acknowledgementRecorder) Reject(_ uint64, requeue bool) error {
	return r.Nack(0, false, requeue)
}

func deliveryForEnvelope(t *testing.T, envelope Envelope, acknowledger amqp.Acknowledger) amqp.Delivery {
	t.Helper()
	body, err := encodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	publishing := publishingForEnvelope(envelope, body)
	return amqp.Delivery{
		Acknowledger:  acknowledger,
		DeliveryTag:   1,
		Headers:       publishing.Headers,
		ContentType:   publishing.ContentType,
		DeliveryMode:  publishing.DeliveryMode,
		CorrelationId: publishing.CorrelationId,
		MessageId:     publishing.MessageId,
		Timestamp:     publishing.Timestamp,
		Type:          publishing.Type,
		Body:          publishing.Body,
	}
}

func TestProcessDeliveryAcknowledgesSuccessfulAttemptAndDeduplicatesRedelivery(t *testing.T) {
	config := DefaultConfig("amqp://localhost:5672/")
	service, err := NewRabbitMQServiceWithConfig(config, nil)
	if err != nil {
		t.Fatal(err)
	}
	topology, err := TopologyFor("worker.test", config)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := NewEnvelope("system.job", "", map[string]bool{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	acknowledger := &acknowledgementRecorder{}
	delivery := deliveryForEnvelope(t, envelope, acknowledger)
	handlerCalls := 0
	handler := func(context.Context, Envelope) error {
		handlerCalls++
		return nil
	}
	if err := service.processDelivery(context.Background(), topology, delivery, handler); err != nil {
		t.Fatal(err)
	}
	delivery.Redelivered = true
	if err := service.processDelivery(context.Background(), topology, delivery, handler); err != nil {
		t.Fatal(err)
	}
	if handlerCalls != 1 || acknowledger.acks != 2 || acknowledger.nacks != 0 {
		t.Fatalf("calls=%d acks=%d nacks=%d", handlerCalls, acknowledger.acks, acknowledger.nacks)
	}
}

func TestQuarantinePublishingIsPersistentBoundedAndForensic(t *testing.T) {
	delivery := amqp.Delivery{
		Headers: amqp.Table{
			"x-schema-version": int32(99),
			"x-attempt":        int32(4),
			"x-tenant-id":      "tenant-1",
		},
		MessageId:     "original-message",
		CorrelationId: "correlation-1",
		Type:          "unknown.job",
		Body:          []byte(strings.Repeat("x", 100)),
	}
	publishing := quarantinePublishing(delivery, "worker.test", errors.New("invalid envelope"), 16)
	if publishing.DeliveryMode != amqp.Persistent || publishing.Type != "quarantine.invalid" {
		t.Fatalf("quarantine publish options = %+v", publishing)
	}
	if len(publishing.Body) != 16 || publishing.Headers["x-body-truncated"] != true {
		t.Fatalf("quarantine body was not bounded: body=%d headers=%+v", len(publishing.Body), publishing.Headers)
	}
	if publishing.Headers["x-original-body-bytes"] != int64(100) || publishing.Headers["x-original-body-sha256"] == "" {
		t.Fatalf("quarantine forensic metadata missing: %+v", publishing.Headers)
	}
	if publishing.Headers["x-original-message-id"] != delivery.MessageId || publishing.Headers["x-original-message-type"] != delivery.Type {
		t.Fatalf("quarantine identity metadata missing: %+v", publishing.Headers)
	}
	if publishing.MessageId == delivery.MessageId || publishing.CorrelationId != delivery.CorrelationId {
		t.Fatalf("quarantine identifiers are unsafe: %+v", publishing)
	}
}

func TestBoundedAMQPStringPreservesValidUTF8(t *testing.T) {
	value := strings.Repeat("é", 10)
	bounded := boundedAMQPString(value, 9)
	if len(bounded) > 9 || strings.ContainsRune(bounded, '\uFFFD') {
		t.Fatalf("bounded value = %q (%d bytes)", bounded, len(bounded))
	}
}

func TestWriteDeadlineConnectionBoundsBlockedBrokerWrite(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	connection := &writeDeadlineConnection{Conn: client, timeout: 50 * time.Millisecond}
	started := time.Now()
	_, err := connection.Write([]byte("blocked without a reader"))
	if err == nil {
		t.Fatal("expected blocked write to time out")
	}
	var networkError net.Error
	if !errors.As(err, &networkError) || !networkError.Timeout() {
		t.Fatalf("write error = %v, want network timeout", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("blocked write exceeded deadline: %s", elapsed)
	}
}

func TestRabbitMQServiceConstructionIsLazy(t *testing.T) {
	service, err := NewRabbitMQService("amqp://127.0.0.1:1/")
	if err != nil {
		t.Fatalf("constructor attempted an unavailable broker connection: %v", err)
	}
	if service.connection != nil || service.publisher != nil {
		t.Fatal("constructor eagerly allocated broker resources")
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPublishGateHonorsCallerCancellation(t *testing.T) {
	service, err := NewRabbitMQService("amqp://127.0.0.1:1/")
	if err != nil {
		t.Fatal(err)
	}
	service.publishGate <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = service.publishWithReconnect(ctx, Topology{}, "exchange", "key", amqp.Publishing{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("publish error = %v, want context cancellation", err)
	}
	<-service.publishGate
}
