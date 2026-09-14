package queue

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestEnvelopeRoundTripAndPublishingProperties(t *testing.T) {
	tenantID := uuid.NewString()
	envelope, err := NewEnvelope("search.index", tenantID, map[string]string{"entity_id": "risk-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := envelope.Validate(); err != nil {
		t.Fatalf("Envelope.Validate() error = %v", err)
	}
	body, err := encodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeEnvelope(body)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ID != envelope.ID || decoded.TenantID != tenantID || decoded.Type != "search.index" {
		t.Fatalf("decoded envelope mismatch: %+v", decoded)
	}

	publishing := publishingForEnvelope(envelope, body)
	if publishing.DeliveryMode != amqp.Persistent || publishing.ContentType != "application/json" {
		t.Fatalf("publishing is not persistent JSON: %+v", publishing)
	}
	if publishing.MessageId != envelope.ID || publishing.CorrelationId != envelope.CorrelationID {
		t.Fatalf("publishing identifiers mismatch: %+v", publishing)
	}
	if publishing.Headers["x-tenant-id"] != tenantID || publishing.Headers["x-schema-version"] != int64(CurrentSchemaVersion) {
		t.Fatalf("publishing metadata mismatch: %+v", publishing.Headers)
	}
}

func TestDecodeEnvelopeRejectsUnknownFields(t *testing.T) {
	envelope, err := NewEnvelope("system.job", "", map[string]bool{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	body[len(body)-1] = ','
	body = append(body, []byte(`"unexpected":true}`)...)
	if _, err := decodeEnvelope(body); err == nil {
		t.Fatal("expected unknown envelope field error")
	}
}

func TestEnvelopeNormalization(t *testing.T) {
	envelope := (Envelope{Type: "system.job", Payload: json.RawMessage(`{"ok":true}`)}).normalized()
	if envelope.ID == "" || envelope.CorrelationID != envelope.ID || envelope.Attempt != 1 || envelope.SchemaVersion != CurrentSchemaVersion || envelope.CreatedAt.IsZero() {
		t.Fatalf("envelope not normalized: %+v", envelope)
	}
}

func TestEnvelopeValidation(t *testing.T) {
	valid, err := NewEnvelope("system.job", "", map[string]bool{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*Envelope)
	}{
		{name: "message ID", mutate: func(e *Envelope) { e.ID = "not-a-uuid" }},
		{name: "type", mutate: func(e *Envelope) { e.Type = "bad type" }},
		{name: "schema", mutate: func(e *Envelope) { e.SchemaVersion = 99 }},
		{name: "tenant", mutate: func(e *Envelope) { e.TenantID = "tenant" }},
		{name: "correlation", mutate: func(e *Envelope) { e.CorrelationID = "" }},
		{name: "timestamp", mutate: func(e *Envelope) { e.CreatedAt = time.Time{} }},
		{name: "attempt", mutate: func(e *Envelope) { e.Attempt = 0 }},
		{name: "payload", mutate: func(e *Envelope) { e.Payload = json.RawMessage(`{`) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := valid
			tt.mutate(&envelope)
			if err := envelope.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestIntegerHeaderSupportsBrokerIntegerTypes(t *testing.T) {
	for _, value := range []any{int32(3), int64(3), uint16(3), "3"} {
		if got, ok := integerHeader(value); !ok || got != 3 {
			t.Fatalf("integerHeader(%T(%v)) = %d, %t", value, value, got, ok)
		}
	}
}

func TestApplyDeliveryMetadataRejectsTenantMismatch(t *testing.T) {
	envelope, err := NewEnvelope("system.job", uuid.NewString(), map[string]bool{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	delivery := amqp.Delivery{
		MessageId: envelope.ID, CorrelationId: envelope.CorrelationID,
		Type: envelope.Type,
		Headers: amqp.Table{
			"x-schema-version": int32(envelope.SchemaVersion),
			"x-attempt":        int32(envelope.Attempt),
			"x-tenant-id":      uuid.NewString(),
		},
	}
	if err := applyDeliveryMetadata(&envelope, delivery); err == nil {
		t.Fatal("expected tenant mismatch error")
	}
}

func TestApplyDeliveryMetadataAcceptsPublisherContract(t *testing.T) {
	envelope, err := NewEnvelope("system.job", uuid.NewString(), map[string]bool{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	envelope.CausationID = uuid.NewString()
	publishing := publishingForEnvelope(envelope, nil)
	delivery := amqp.Delivery{
		Headers:       publishing.Headers,
		MessageId:     publishing.MessageId,
		CorrelationId: publishing.CorrelationId,
		Type:          publishing.Type,
	}
	if err := applyDeliveryMetadata(&envelope, delivery); err != nil {
		t.Fatalf("applyDeliveryMetadata() error = %v", err)
	}
}

func TestDeduplicationKeySeparatesRetryAttempts(t *testing.T) {
	envelope, err := NewEnvelope("system.job", "", map[string]bool{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	first := deduplicationKey(envelope)
	envelope.Attempt++
	if second := deduplicationKey(envelope); first == second {
		t.Fatalf("retry attempt reused deduplication key %q", first)
	}
}
